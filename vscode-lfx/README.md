# LFX Language Support

LFX Language Support adds first-class editing support for `.lfx` files in Visual Studio Code.

It provides:

- syntax highlighting for LFX effect and library modules
- comment, bracket, auto-closing, folding, and indentation behavior
- snippets for common LFX constructs
- inline parse and semantic diagnostics by running the `lfx` checker
- commands to validate the current file and inspect generated WGSL

This extension is intended for authors working with the LFX language and toolchain. It focuses on fast feedback and solid authoring ergonomics without requiring a full language server.

## Requirements

- Install the `lfx` CLI and ensure it is available on your `PATH`, or set `lfx.path` to the executable location.
- When developing inside the LFX source repository, the extension can fall back to `go run ./cmd/lfx` if no installed binary is found.

## Features

- Language registration for `*.lfx`
- TextMate-based syntax highlighting
- Comment and bracket handling from `language-configuration.json`
- Snippets for effect modules, library modules, params, timeline, stdlib imports, and common return shapes
- Inline diagnostics from `lfx check --json`
- Commands:
  - `LFX: Check Current File`
  - `LFX: Check Workspace`
  - `LFX: Show Generated WGSL`
  - `LFX: Open Module Graph`

## Settings

- `lfx.path`: path to the `lfx` executable. Default: `"lfx"`.
- `lfx.check.onSave`: run checks when saving `.lfx` files. Default: `true`.
- `lfx.check.onType`: run checks while typing using a debounce. Default: `false`.
- `lfx.check.debounceMs`: debounce delay in milliseconds for on-type checks. Default: `400`.
- `lfx.moduleRoots`: additional module roots passed through as repeated `--module-root` flags.
- `lfx.trace.diagnostics`: log CLI tracing and diagnostics flow to the output channel.

## Notes

- The extension uses regular VS Code extension APIs rather than an LSP.
- Semantic tokens, completion, hovers, formatting, rename, and go-to-definition are intentionally out of scope for v1.
