package sema_test

import (
	"testing"

	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

// parseOrFatal is a helper that fails the test if source does not parse.
func parseOrFatal(t *testing.T, source string) *parser.Module {
	t.Helper()
	mod, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return mod
}

// expectError asserts at least one sema error with the given code is present.
func expectError(t *testing.T, errs []sema.Error, code string) {
	t.Helper()
	for _, e := range errs {
		if e.Code == code {
			return
		}
	}
	t.Fatalf("expected sema error %s but got: %v", code, errs)
}

func TestAnalyzeRejectsEffectWithoutOutputDeclaration(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/no_out"
effect "no_out"
function sample(width, height, x, y, index, phase, params)
  return phase
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrEffectMissingOutput)
}

func TestAnalyzeRejectsEffectWithDuplicateSampleFunction(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/two_sample"
effect "two_sample"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
function sample(width, height, x, y, index, phase, params)
  return 1.0
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrDuplicateFunctionName)
}

func TestAnalyzeRejectsSampleWithSixArgs(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/bad_arity"
effect "bad_arity"
output scalar
function sample(width, height, x, y, index, phase)
  return 0.0
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrEffectInvalidSampleArity)
}

func TestAnalyzeRejectsSampleWithEightArgs(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/bad_arity8"
effect "bad_arity8"
output scalar
function sample(width, height, x, y, index, phase, params, extra)
  return 0.0
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrEffectInvalidSampleArity)
}

func TestAnalyzeRejectsLibraryWithSampleFunction(t *testing.T) {
	mod := parseOrFatal(t, `module "lib/bad_lib"
library "bad_lib"
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrLibraryHasSample)
}

func TestAnalyzeRejectsLibraryWithOutputDeclaration(t *testing.T) {
	mod := parseOrFatal(t, `module "lib/bad_out"
library "bad_out"
output rgb
export function shade(v)
  return v
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrOutputInLibrary)
}

func TestAnalyzeRejectsLibraryWithTimelineBlock(t *testing.T) {
	mod := parseOrFatal(t, `module "lib/tl_lib"
library "tl_lib"
export function fn(x)
  return x
end
timeline {
  loop_start = 0.0
  loop_end = 1.0
}
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrLibraryHasTimeline)
}

func TestAnalyzeRejectsEffectWithExportedFunction(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/bad_export"
effect "bad_export"
output scalar
export function helper(x)
  return x
end
function sample(width, height, x, y, index, phase, params)
  return helper(phase)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrEffectExportedFunction)
}

func TestAnalyzeRejectsDuplicateParamNames(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/dup_param"
effect "dup_param"
output scalar
params {
  gain = float(0.5, 0.0, 1.0)
  gain = float(0.75, 0.0, 1.0)
}
function sample(width, height, x, y, index, phase, params)
  return params.gain
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrDuplicateParamName)
}

func TestAnalyzeRejectsDuplicateHelperFunctionNames(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/dup_helper"
effect "dup_helper"
output scalar
function brightness(x)
  return x
end
function brightness(x)
  return x * 2.0
end
function sample(width, height, x, y, index, phase, params)
  return brightness(phase)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrDuplicateFunctionName)
}

func TestAnalyzeRejectsDuplicateLocalNamesInSameScope(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/dup_local"
effect "dup_local"
output scalar
function sample(width, height, x, y, index, phase, params)
  a, b = 1.0, 2.0
  a, c = 3.0, 4.0
  return a
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrDuplicateLocalName)
}

func TestAnalyzeRejectsUndefinedIdentifier(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/undef"
effect "undef"
output scalar
function sample(width, height, x, y, index, phase, params)
  return undefined_var
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrUndefinedIdentifier)
}

func TestAnalyzeRejectsCallToMissingExportedFunction(t *testing.T) {
	libMod := parseOrFatal(t, `module "mylib"
library "mylib"
export function helper(x)
  return x
end
`)
	libInfo, libErrs, _ := sema.AnalyzeModule(libMod, nil, nil)
	if len(libErrs) != 0 {
		t.Fatalf("unexpected library sema errors: %v", libErrs)
	}

	effectMod := parseOrFatal(t, `module "effects/missing_sym"
import "mylib" as mylib
effect "missing_sym"
output scalar
function sample(width, height, x, y, index, phase, params)
  return mylib.missing(x)
end
`)
	imports := map[string]*parser.Module{"mylib": libMod}
	importedInfo := map[string]*sema.Info{"mylib": libInfo}
	_, errs, _ := sema.AnalyzeModule(effectMod, imports, importedInfo)
	expectError(t, errs, sema.ErrImportMissingExportedFunc)
}

// ── Additional: alias shadowing a local function name ────────────────────────

func TestAnalyzeRejectsImportAliasShadowingFunctionName(t *testing.T) {
	libMod := parseOrFatal(t, `module "mylib"
library "mylib"
export function fn(x)
  return x
end
`)
	libInfo, _, _ := sema.AnalyzeModule(libMod, nil, nil)

	effectMod := parseOrFatal(t, `module "effects/alias_fn_shadow"
import "mylib" as helper
effect "alias_fn_shadow"
output scalar
function helper(x)
  return x
end
function sample(width, height, x, y, index, phase, params)
  return helper(phase)
end
`)
	imports := map[string]*parser.Module{"helper": libMod}
	importedInfo := map[string]*sema.Info{"helper": libInfo}
	_, errs, _ := sema.AnalyzeModule(effectMod, imports, importedInfo)
	// The alias "helper" conflicts with the function "helper".
	expectError(t, errs, sema.ErrDuplicateImportAlias)
}

// ── Additional: duplicate import aliases (both sema-level) ───────────────────

func TestAnalyzeRejectsDuplicateImportAliases(t *testing.T) {
	libMod := parseOrFatal(t, `module "lib_coords"
library "lib_coords"
export function normalized(x)
  return x
end
`)
	libInfo, _, _ := sema.AnalyzeModule(libMod, nil, nil)

	effectMod := parseOrFatal(t, `module "effects/dup_alias"
import "lib_coords" as util
import "lib_curves" as util
effect "dup_alias"
output scalar
function sample(width, height, x, y, index, phase, params)
  return util.normalized(x)
end
`)
	imports := map[string]*parser.Module{
		"util": libMod, // only one module for the alias, but two imports
	}
	importedInfo := map[string]*sema.Info{"util": libInfo}
	_, errs, _ := sema.AnalyzeModule(effectMod, imports, importedInfo)
	expectError(t, errs, sema.ErrDuplicateImportAlias)
}
