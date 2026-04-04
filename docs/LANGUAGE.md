# LFX Language Reference

This document describes the LFX language as it is represented in this repository. It is grounded in the implementation under [`parser`](../parser), [`sema`](../sema), [`lower`](../lower), and [`runtime`](../runtime), with terminology aligned to the draft specification PDFs.

## What LFX is for

LFX is a small DSL for procedural lighting effects. An LFX program computes a scalar value for a single logical point in a lighting layout at a single normalized phase value. The host runtime is expected to:

- choose presets and playback behavior
- provide layout bounds and point coordinates
- bind and validate parameter values
- clamp or otherwise interpret the returned scalar

In this repo, the sampling contract is represented by [`runtime.Sampler`](../runtime/sample.go):

```go
SamplePoint(layout Layout, pointIndex int, phase float32, params *BoundParams) (float32, error)
```

At the source-language level, effect modules are authored around a `sample` function with this shape:

```lfx
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
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
- zero or more imports
- an optional `params { ... }` block
- helper functions
- one required `sample(...)` function
- zero or more `preset "..." { ... }` blocks

Example:

```lfx
version "0.1"
module "effects/fill_iris"

effect "Fill Iris"

params {
  radius = float(0.35, 0.0, 1.0)
  grid_aligned = bool(false)
}

function sample(width, height, x, y, index, phase, params)
  return 1.0
end

preset "forward" {
  speed = 1200
  start = 0.0
  loop_start = 0.2
  loop_end = 0.8
  finish = 1.0
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
- presets

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

## Presets

Effect modules may declare named presets:

```lfx
preset "forward" {
  speed = 1200
  start = 0.0
  loop_start = 0.2
  loop_end = 0.8
  finish = 1.0
}
```

Recognized fields in lowering/runtime:

- `speed`
- `start`
- `loop_start`
- `loop_end`
- `finish`

Semantic and runtime validation enforce:

```text
0 <= start <= loop_start <= loop_end <= finish <= 1
```

Library modules may not declare presets.

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

Notes from the current implementation:

- variables are function-local by default
- a first assignment introduces a name in the current lexical scope
- duplicate names in the same scope are rejected
- recursion is rejected during semantic analysis

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
- library modules must not define `sample`
- library modules must not define presets
- duplicate params, functions, and import aliases are rejected
- every referenced identifier must resolve in lexical scope
- recursion, including mutual recursion, is rejected

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

- converts params and presets into typed IR specs
- lowers functions defined in the current module and imported exported functions
- mangles imported function names
- lowers `params.name` to `ParamRef`
- lowers recognized builtins to explicit `BuiltinCall` nodes

The IR already has support for:

- arithmetic and comparison operators
- branches
- calls
- returns
- multi-return builtin plumbing

Constant folding exists as a placeholder and is currently a no-op.

## Current implementation gaps

The draft specification and technical plan describe a broader end state than this repo currently implements. Important gaps or rough edges visible in the code today:

- the lowerer currently defaults most function parameters and locals to numeric IR types
- the parser accepts `a, b = expr`, and the IR has multi-return support, but full multi-value source-language behavior is still partial
- module header scanning in the import graph builder is lightweight and line-based rather than a full parse

Those are implementation-status notes, not part of the intended language design.

## Minimal example

```lfx
version "0.1"
module "effects/simple_pulse"
effect "Simple Pulse"

params {
  intensity = float(1.0, 0.0, 1.0)
}

function sample(width, height, x, y, index, phase, params)
  pulse = sin(phase * 6.28318)
  value = (pulse * 0.5 + 0.5) * params.intensity
  return clamp(value, 0.0, 1.0)
end

preset "default" {
  speed = 1200
  start = 0.0
  loop_start = 0.0
  loop_end = 1.0
  finish = 1.0
}
```

## Repository map

- [`parser`](../parser): tokens, lexer, AST, parser
- [`modules`](../modules): root lookup and import graph construction
- [`sema`](../sema): semantic rules and identifier resolution
- [`lower`](../lower): AST-to-IR lowering and name mangling
- [`ir`](../ir): shared intermediate representation
- [`runtime`](../runtime): layout, params, presets, and sampling contracts
