# Batched CPU Backend Plan

## Goal

Add a second CPU execution path that evaluates many points at once. The current evaluator in [backend/cpu/evaluator.go](/Users/stokm001/Documents/personal/lfx/backend/cpu/evaluator.go) executes one point per call and keeps one `value` per local slot. That is simple and correct, but it blocks efficient use of slice-oriented SIMD libraries and amplifies per-point allocation and dispatch overhead.

The batched backend should keep the current evaluator as the reference implementation and add a new path optimized for throughput.

## Why The Current Shape Limits SIMD

Today each sample call:

- materializes seven scalar arguments into a fresh `[]value`
- allocates a fresh local frame sized to the function
- walks IR statements for a single point
- returns a fresh `[]float32` for the output channels

The data model is fixed-width per value in [backend/cpu/value.go](/Users/stokm001/Documents/personal/lfx/backend/cpu/value.go), which is good for scalar correctness, but not for batch throughput:

- `value` stores one point worth of lanes as `[4]float64`
- locals are stored as `[]value`, so each local slot is array-of-structs across points
- arithmetic in [backend/cpu/builtin_math.go](/Users/stokm001/Documents/personal/lfx/backend/cpu/builtin_math.go) is performed one expression at a time

Slice SIMD libraries want the opposite shape: one buffer per local slot and per lane.

## Proposed Execution Model

Add a new internal executor that processes `N` points at once, where `N` is a tunable batch size such as `128`, `256`, or `512`.

Execution shape:

- Convert layout point data into structure-of-arrays buffers:
  - `xs []float64`
  - `ys []float64`
  - `indices []float64`
- Broadcast module-global scalars into batch buffers when needed:
  - `width []float64`
  - `height []float64`
  - `phase []float64`
- Store each local slot as a column buffer instead of a per-point `value`.

Suggested slot representation:

```go
type batchSlot struct {
	Typ   ir.Type
	L0    []float64
	L1    []float64
	L2    []float64
	L3    []float64
	Mask  []bool // only for boolean slots if we keep bools separate
}
```

For scalar numeric locals, only `L0` is used. For `vec2`/`vec3`/`vec4`, the active lanes live in `L0..L3`. This matches the existing IR type system in [ir/types.go](/Users/stokm001/Documents/personal/lfx/ir/types.go) and preserves vector semantics without changing the language surface.

## Backend Boundary

Do not replace `runtime.Sampler` immediately. Keep [runtime/sample.go](/Users/stokm001/Documents/personal/lfx/runtime/sample.go) intact and add a batch-oriented internal API first:

```go
type batchEvaluator struct {
	module *ir.Module
	funcs  map[string]*compiledBatchFunc
}

func (e *batchEvaluator) SamplePoints(
	layout runtime.Layout,
	pointIndices []int,
	phase float32,
	params *runtime.BoundParams,
	dst []float32,
) error
```

Then adapt `SamplePoint` to use the old evaluator and add a separate caller for preview/export paths that can consume the batched API. This avoids forcing the whole application to change at once.

## Lowering Strategy

Do not try to batch-interpret arbitrary control flow on day one. Start with straight-line blocks and a limited builtin set, then add masked control flow.

### Phase 1

Support a batched subset that covers most arithmetic-heavy effects:

- `Const`
- `LocalRef`
- `ParamRef`
- `BinaryOp` for `+`, `-`, `*`, `/`
- `UnaryOp` for negation
- `ComponentRef`
- `BuiltinCall` for:
  - `vec2`, `vec3`, `vec4`
  - `abs`, `min`, `max`, `floor`, `ceil`, `fract`
  - `clamp`, `mix`, `mod`, `pow`
  - `dot`, `length`, `distance`, `normalize`
- `Return`

This is enough to cover vector-heavy, mostly branch-light effects like [effects/chroma_bloom.lfx](/Users/stokm001/Documents/personal/lfx/effects/chroma_bloom.lfx).

### Phase 2

Add masked control flow:

- `IfStmt`
- comparison ops
- boolean ops

Execution model:

- maintain an `active []bool` mask for the current batch
- evaluate the condition into a boolean mask
- execute `then` and `else` blocks with derived masks
- keep writes masked so untaken lanes preserve prior values

This is enough to cover [effects/fill_iris.lfx](/Users/stokm001/Documents/personal/lfx/effects/fill_iris.lfx), where divergence is present but localized.

### Phase 3

Add the expensive pieces:

- user-defined function calls
- multi-return calls and `TupleRef`
- noise builtins

For user functions, reuse the same slot-buffer scheme recursively with a stack of batch frames. For noise, start with scalar fallback inside the batch path if necessary, then optimize once the rest of the executor is stable.

## First IR Nodes To Lower

The first nodes worth lowering to slice ops are the ones that are both common and easy to express in bulk:

1. `BinaryOp` arithmetic
2. `BuiltinCall` for `min`, `max`, `clamp`, `mix`
3. `ComponentRef`
4. `BuiltinCall` for `dot` and `length`
5. `BuiltinCall` for `normalize`

Reason:

- they dominate vector-heavy effect math
- they compose cleanly into SoA buffers
- they map to SIMD-friendly kernels without control-flow complications

Examples against current effects:

- [effects/chroma_bloom.lfx](/Users/stokm001/Documents/personal/lfx/effects/chroma_bloom.lfx): mostly vector arithmetic, component reads, and mix/clamp style math
- [effects/fill_iris.lfx](/Users/stokm001/Documents/personal/lfx/effects/fill_iris.lfx): same arithmetic core plus masked branches
- [effects/perlin_501.lfx](/Users/stokm001/Documents/personal/lfx/effects/perlin_501.lfx): likely later because noise dominates and requires separate treatment

## SIMD Library Recommendation

If we pursue external SIMD after the batched executor exists, `github.com/pehringer/simd` is the better fit for this repo than `vek` or `archsimd`:

- it supports both `amd64` and `arm64`
- it exposes direct arithmetic kernels over `[]float64`
- it matches the SoA batch shape proposed here

It still should not be wired into the current per-point interpreter. The right place for it is inside batched numeric kernels such as:

- add/sub/mul/div on scalar buffers
- add/sub/mul/div on vector lane buffers
- min/max clamp helpers

For operations not covered by the library, keep pure Go kernels first and optimize later.

## Memory Plan

Use reusable scratch buffers owned by the batch evaluator:

- one slot buffer per local
- one temp buffer per active lane type
- one boolean mask pool for control flow

Avoid per-node allocation. Batch execution only pays off if buffers are reused across points and frames.

## Benchmark Plan

Add benchmarks before implementing the new backend so the baseline is fixed. The benchmark scaffold lives in [backend/cpu/bench_test.go](/Users/stokm001/Documents/personal/lfx/backend/cpu/bench_test.go) and covers:

- `fill_iris`
- `chroma_bloom`
- `perlin_501`

Each benchmark samples a `64x64` grid through the current evaluator and reports allocations. That gives us the first performance target and highlights whether arithmetic-heavy effects and noise-heavy effects behave differently.

Recommended next measurements:

- `go test -bench BenchmarkEvaluatorSampleAll -benchmem ./backend/cpu`
- compare scalar output, `rgb`, and `rgbw` effects separately once a batch path exists
- add a benchmark that measures full-frame throughput instead of single-point latency

## Rollout Order

1. Land baseline benchmarks.
2. Introduce batch slot buffers and a minimal straight-line executor for one function body.
3. Support arithmetic/vector builtins needed by `chroma_bloom`.
4. Add masked control flow and validate with `fill_iris`.
5. Add user calls and multi-return support.
6. Decide whether an external SIMD library is still justified after the batch path exists.

## Non-Goals For V1

- changing language semantics
- replacing WGSL or IR lowering
- architecture-specific assembly inside this repo
- optimizing noise before the batch executor proves its value on simpler math
