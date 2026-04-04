# LFX

LFX is a small DSL and toolchain for procedural lighting effects. An LFX program evaluates a single point in a lighting layout at a normalized phase value, then returns a scalar that a host can use to drive brightness, color mixing, or other effect behavior.

This repository contains:

- the language frontend and compiler pipeline
- a command-line tool for parsing, checking, sampling, and emitting WGSL
- a Wails-based preview app for experimenting with effects in a UI
- example effects and the language reference

## Quick Start

```bash
go test ./...
go run ./cmd/lfx check effects/fill_iris.lfx
```

## Command-Line Tool

The CLI lives in `cmd/lfx` and supports these subcommands:

- `parse` - parse a source file and print the AST as JSON
- `check` - parse, resolve imports, and run semantic validation
- `graph` - print the resolved import graph as JSON
- `sample` - evaluate an effect against a layout JSON file
- `emit-wgsl` - lower an effect and print WGSL

Examples:

```bash
go run ./cmd/lfx parse effects/fill_iris.lfx
go run ./cmd/lfx check effects/fill_iris.lfx
go run ./cmd/lfx graph effects/fill_iris.lfx
go run ./cmd/lfx emit-wgsl effects/fill_iris.lfx
go run ./cmd/lfx sample --layout layout.json --phase 0.5 effects/fill_iris.lfx
```

The `sample` command expects a layout JSON file with the following shape:

```json
{
	"width": 4,
	"height": 2,
	"points": [
		{"index": 0, "x": 0, "y": 0},
		{"index": 1, "x": 1, "y": 0}
	]
}
```

You can also pass parameter overrides with repeated `--param name=value` flags, for example:

```bash
go run ./cmd/lfx sample --layout layout.json --phase 0.5 --param intensity=0.75 effects/fill_iris.lfx
```

## Preview App

The `preview/` directory contains a separate Wails application for interactive effect previewing.

```bash
cd preview
wails dev
```

For a production build:

```bash
cd preview
wails build
```

## Language Reference

The current language description is documented in [docs/LANGUAGE.md](docs/LANGUAGE.md). It covers the source format, imports, parameters, presets, expressions, builtins, and the semantic rules enforced by this repository.

## Repository Layout

- `cmd/lfx` - CLI entry point
- `compiler` - parse, resolve, validate, and lower pipeline
- `parser` - lexer, AST, and parser
- `modules` - import graph construction and resolution
- `sema` - semantic checks and identifier resolution
- `lower` - AST-to-IR lowering
- `ir` - shared intermediate representation
- `runtime` - layouts, params, presets, and sampling contracts
- `backend` - CPU evaluator and WGSL backend
- `stdlib` - built-in library modules
- `effects` - example effect programs
- `preview` - Wails preview application

## Notes

- The module path is `github.com/BenStokmans/lfx`.
- The project targets Go 1.26.1.
- Import resolution defaults to the repository roots under `stdlib/`, `effects/`, and the repo root unless a different base directory is supplied.
