# LFX Language Reference

This document describes the LFX language as it is represented in this repository. It is grounded in the implementation under [`parser`](../parser), [`sema`](../sema), [`lower`](../lower), and [`runtime`](../runtime), with terminology aligned to the draft specification PDFs.

## What LFX is for

LFX is a small DSL for procedural lighting effects. An LFX program computes one or more channel values for a single logical point in a lighting layout at a single normalized phase value `phase ∈ [0, 1]`. The host runtime is expected to:

- own playback policy, timing, speed, direction, looping, and release behavior
- provide layout bounds and point coordinates
- bind and validate parameter values
- clamp or otherwise interpret the returned channels

In this repo, the sampling contract is represented by [`runtime.Sampler`](../runtime/sample.go):

```go
SamplePoint(layout Layout, pointIndex int, phase float32, params *BoundParams) ([]float32, error)
```

At the source-language level, effect modules are authored around a `sample` function with this shape:

```lfx
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
```

Effect modules must declare an output type:

```lfx
output scalar
output rgb
output rgbw
```

For effect modules, semantic analysis currently requires exactly one `sample` function and exactly 7 parameters.

## Source file kinds

Every file starts with a module declaration:

```lfx
module "effects/fill_iris"
```

An optional version header may appear first:

```lfx
version "0.1"
module "effects/fill_iris"
```

LFX supports two file kinds.

### Effect modules

Effect modules are end-user effects. They may contain:

- an `effect "Name"` declaration
- a required `output ...` declaration
- zero or more imports
- an optional `params { ... }` block
- helper functions
- one required `sample(...)` function
- an optional `timeline { ... }` block

Example:

```lfx
version "0.1"
module "effects/fill_iris"

effect "Fill Iris"
output scalar

params {
  radius = float(0.35, 0.0, 1.0)
  grid_aligned = bool(false)
}

function sample(width, height, x, y, index, phase, params)
  return 1.0
end

timeline {
  loop_start = 0.2
  loop_end = 0.8
}
```

### Library modules

Library modules are reusable helper modules. They may contain:

- a `library "Name"` declaration
- zero or more imports
- helper functions
- exported functions

Library modules must not define:

- `sample`
- an `output` declaration
- a `timeline` block

Example:

```lfx
module "libs/helpers"
library "helpers"

export function half(width)
  return width / 2
end
```

## Imports and module resolution

Imports are declared near the top of the file when you use library modules:

```lfx
import "libs/helpers" as helpers
```

The resolver in [`modules`](../modules) looks for `<path>.lfx` under configured roots. By default this repo searches:

- `stdlib/`
- `effects/`
- the repo root

Import graph rules enforced in this repo:

- imports must resolve from one of the configured roots
- import cycles are rejected
- effect modules may not import other effect modules
- library modules may import library modules

Imported helpers are flattened during lowering. Non-builtin imported functions are name-mangled using:

```text
alias__function_name
```

For example, `helpers.half(...)` lowers to `helpers__half(...)`.

## Parameters

Effect modules may declare host-visible parameters in a `params` block:

```lfx
params {
  rampsize = int(4, 0, 100)
  opacity = float(0.75, 0.0, 1.0)
  grid_aligned = bool(false)
  shape = enum("iris", "iris", "square", "diamond")
}
```

Supported constructors:

- `int(default, min, max)`
- `float(default, min, max)`
- `bool(default)`
- `enum(default, ...)`

Repository validation rules:

- numeric params may omit `min` and `max`
- if both numeric bounds are present, `min <= max` must hold
- numeric defaults must be within bounds when bounds are present
- enum defaults must match one of the declared enum values
- duplicate parameter names are rejected

Parameter access in user code is via `params.<name>`:

```lfx
function sample(width, height, x, y, index, phase, params)
  if params.grid_aligned then
    return 1.0
  end
  return params.opacity
end
```

At runtime, [`runtime.Bind`](../runtime/params.go) applies defaults, validates types, validates enum members, and clamps numeric overrides to declared bounds.

## Timeline

Effect modules may declare an optional `timeline` block that describes loop markers within the normalized phase clip:

```lfx
timeline {
  loop_start = 0.25
  loop_end = 0.75
}
```

Recognized fields:

- `loop_start` — start of the sustain loop region, in `[0, 1]`
- `loop_end` — end of the sustain loop region, in `[0, 1]`

Both fields are optional. If neither is specified the block may be omitted entirely.

Semantic validation enforces:

```text
0 <= loop_start <= loop_end <= 1
```

The authored clip always spans `phase 0..1`. Timeline markers are descriptive metadata for the runtime — they do not themselves alter how the sample function is called. The runtime uses them to implement sustain-loop playback:

- **start**: play from `phase 0` to `loop_start`
- **hold**: loop in `[loop_start, loop_end)` while input is held
- **release**: continue from `loop_end` to `phase 1`

If no `timeline` block is present, the effect is treated as non-looping by default.

Speed, direction, looping policy, and release behavior are entirely runtime-owned and are not part of the LFX language.

Library modules must not declare a `timeline` block.

## Output Types

Effect modules must declare a module-level output type:

```lfx
output scalar
output rgb
output rgbw
```

This declaration appears after `effect` or `library` and before `params`.

Channel counts by output type:

- `scalar` → 1 channel
- `rgb` → 3 channels: red, green, blue
- `rgbw` → 4 channels: red, green, blue, white

For non-bare `return` statements in `sample`, semantic analysis requires the return arity to match the output channel count.

Library modules must not declare `output`.

## Statements

The parser currently supports these statements:

- declaration-by-assignment
- assignment to an existing name
- `if / elseif / else`
- `return`
- expression statements, typically function calls

Examples:

```lfx
a = 0.5
a = a + 0.25

if phase < 0.5 then
  return 0.0
elseif phase < 0.75 then
  return 0.5
else
  return 1.0
end
```

For multi-channel effects, `return` may contain multiple comma-separated expressions:

```lfx
output rgb

function sample(width, height, x, y, index, phase, params)
  return x / width, y / height, phase
end
```

Notes from the current implementation:

- variables are function-local by default
- a first assignment introduces a name in the current lexical scope
- duplicate names in the same scope are rejected
- recursion is rejected during semantic analysis
- bare `return` is allowed
- in `sample`, non-bare return arity must match the declared output channel count

## Expressions

Supported literals:

- integers
- floats
- strings
- booleans: `true`, `false`

Supported operators:

- arithmetic: `+`, `-`, `*`, `/`, `%`
- comparison: `==`, `~=`, `<`, `>`, `<=`, `>=`
- boolean: `and`, `or`, `not`
- grouping: `( ... )`
- member access: `object.field`
- calls: `name(...)` or `module.func(...)`

Operator precedence in the parser, from low to high:

1. `or`
2. `and`
3. `==`, `~=`
4. `<`, `>`, `<=`, `>=`
5. `+`, `-`
6. `*`, `/`, `%`
7. unary `not`, unary `-`
8. postfix member access and calls

Examples:

```lfx
pulse = sin(phase * 6.28318)
on_axis = x == 0 or y == 0
dist = abs(x) + abs(y)
return clamp(pulse * 0.5 + 0.5, 0.0, 1.0)
```

## Builtins

Semantic analysis recognizes these bare builtin names:

- `abs`
- `min`
- `max`
- `floor`
- `ceil`
- `sqrt`
- `sin`
- `cos`
- `clamp`
- `mix`
- `fract`
- `mod`
- `pow`
- `is_even`

## Comments

Single-line comments start with `--`:

```lfx
-- This is a comment.
v = 1.0
```

The lexer preserves comment tokens, but the parser skips them.

## Semantic rules enforced in this repo

The analyzer in [`sema`](../sema) currently enforces:

- effect modules must not contain exported functions
- effect modules must define exactly one `sample` function
- the `sample` function must have exactly 7 parameters
- effect modules must declare `output`
- library modules must not define `sample`
- library modules must not define `output`
- library modules must not define a `timeline` block
- duplicate params, functions, and import aliases are rejected
- every referenced identifier must resolve in lexical scope
- recursion, including mutual recursion, is rejected
- `sample` return arity must match the declared output channel count

## Variable binding model

LFX does not have global variables. Inside a function, plain assignment introduces a function-local name on first use:

```lfx
function sample(width, height, x, y, index, phase, params)
  pulse = sin(phase * 6.28318)
  value = pulse * 0.5 + 0.5
  return clamp(value, 0.0, 1.0)
end
```

## Intermediate representation and lowering

The parser and semantic passes feed a shared IR in [`ir`](../ir). Lowering currently:

- converts params into typed IR specs
- converts an optional `output` declaration into an IR output type
- converts an optional timeline block into an IR `TimelineSpec` (loop markers only)
- lowers functions defined in the current module and imported exported functions
- mangles imported function names
- lowers `params.name` to `ParamRef`
- lowers recognized builtins to explicit `BuiltinCall` nodes

The IR already has support for:

- arithmetic and comparison operators
- branches
- calls
- returns
- multi-value returns for `sample`

Constant folding exists as a placeholder and is currently a no-op.

## Current implementation gaps

The draft specification and technical plan describe a broader end state than this repo currently implements. Important gaps or rough edges visible in the code today:

- the lowerer currently defaults most function parameters and locals to numeric IR types
- module header scanning in the import graph builder is lightweight and line-based rather than a full parse

Those are implementation-status notes, not part of the intended language design.

## Minimal example

```lfx
version "0.1"
module "effects/simple_pulse"
effect "Simple Pulse"
output scalar

params {
  intensity = float(1.0, 0.0, 1.0)
}

function sample(width, height, x, y, index, phase, params)
  pulse = sin(phase * 6.28318)
  value = (pulse * 0.5 + 0.5) * params.intensity
  return clamp(value, 0.0, 1.0)
end

timeline {
  loop_start = 0.0
  loop_end = 1.0
}
```

## fill_iris example

```lfx
version "0.1"
module "effects/fill_iris"
effect "Fill Iris"
output scalar

params {
  rampsize = int(4, 0, 100)
  grid_aligned = bool(false)
}

function triangle_phase(t)
  if t <= 0.5 then
    return t * 2.0
  else
    return (1.0 - t) * 2.0
  end
end

function sample(width, height, x, y, index, phase, params)
  cx = (width - 1.0) / 2.0
  cy = (height - 1.0) / 2.0
  dx = abs(x - cx)
  dy = abs(y - cy)
  dist = sqrt(dx * dx + dy * dy)
  mx = abs(0.0 - cx)
  my = abs(0.0 - cy)
  max_radius = sqrt(mx * mx + my * my)
  span = ceil(max_radius) + 1.0 + params.rampsize
  t = triangle_phase(phase) * span
  pos = t - dist
  if pos < 0.0 then
    return 0.0
  end
  if pos < params.rampsize then
    return pos / params.rampsize
  end
  if pos < span then
    return 1.0
  end
  return 0.0
end

timeline {
  loop_start = 0.0
  loop_end = 1.0
}
```

## rgb example

```lfx
version "0.1"
module "effects/chroma_bloom"
effect "Chroma Bloom"
output rgb

params {
  bloom = float(0.72, 0.2, 1.6)
}

function sample(width, height, x, y, index, phase, params)
  pulse = sin(phase * 6.28318) * 0.5 + 0.5
  r = clamp(pulse, 0.0, 1.0)
  g = clamp(1.0 - pulse, 0.0, 1.0)
  b = clamp(0.5 + 0.5 * cos(phase * 6.28318), 0.0, 1.0)
  return r, g, b
end

timeline {
  loop_start = 0.0
  loop_end = 1.0
}
```

## Repository map

- [`parser`](../parser): tokens, lexer, AST, parser
- [`modules`](../modules): root lookup and import graph construction
- [`sema`](../sema): semantic rules and identifier resolution
- [`lower`](../lower): AST-to-IR lowering and name mangling
- [`ir`](../ir): shared intermediate representation
- [`runtime`](../runtime): layout, params, timeline, and sampling contracts
