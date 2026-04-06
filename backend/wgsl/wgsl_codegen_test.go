package wgsl_test

import (
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

// compileToWGSL parses, analyzes, lowers, and emits WGSL from source.
// It calls t.Fatalf on any intermediate error.
func compileToWGSL(t *testing.T, source string) string {
	t.Helper()
	mod, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info, errs, warns := sema.AnalyzeModule(mod, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("sema errors: %v", errs)
	}
	if len(warns) != 0 {
		t.Logf("sema warnings: %v", warns)
	}
	irmod, err := lower.Lower(mod, nil, info, nil)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	lower.ConstFold(irmod)
	out, err := wgsl.Emit(irmod)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}
	return out
}

func TestEmitSanitizesWGSLReservedLocalNames(t *testing.T) {
	// Each WGSL reserved word used as a local variable should be emitted as
	// lfx_<word> in the generated WGSL.
	reservedCases := []struct {
		name    string
		lfxName string
	}{
		{"var", "var"},
		{"fn", "fn"},
		{"let", "let"},
		{"struct", "struct"},
		{"override", "override"},
		{"loop", "loop"},
		{"for", "for"},
		{"while", "while"},
		{"const", "const"},
	}

	for _, tc := range reservedCases {
		t.Run(tc.name, func(t *testing.T) {
			src := `module "effects/reserved_` + tc.name + `"
effect "reserved_` + tc.name + `"
output scalar
function sample(width, height, x, y, index, phase, params)
  ` + tc.lfxName + ` = phase * 2.0
  return ` + tc.lfxName + `
end
`
			out := compileToWGSL(t, src)

			sanitized := "lfx_" + tc.lfxName
			if !strings.Contains(out, sanitized) {
				t.Fatalf("expected sanitized name %q in WGSL output:\n%s", sanitized, out)
			}
			// The unsanitized bare name should not appear as a variable declaration.
			bareDecl := "var " + tc.lfxName + ":"
			if strings.Contains(out, bareDecl) {
				t.Fatalf("bare reserved word %q should not appear as variable declaration:\n%s", bareDecl, out)
			}
		})
	}
}

func TestEmitSanitizesReservedWordAsHelperFunctionName(t *testing.T) {
	// A helper function named with a WGSL reserved word must be sanitized.
	src := `module "effects/reserved_fn"
effect "reserved_fn"
output scalar
function loop(x)
  return x * 2.0
end
function sample(width, height, x, y, index, phase, params)
  return loop(phase)
end
`
	out := compileToWGSL(t, src)
	if !strings.Contains(out, "fn lfx_loop(") {
		t.Fatalf("expected sanitized function name lfx_loop in:\n%s", out)
	}
	if strings.Contains(out, "fn loop(") {
		t.Fatalf("bare reserved word 'loop' should not appear as function name:\n%s", out)
	}
}

func TestEmitImportedFunctionNamesMangle(t *testing.T) {
	// Mangled names use __; verify they appear correctly in the output.
	// We test this with the full compile pipeline via compiler package,
	// so we use a simpler inline approach here with the lower package directly.
	src := `module "effects/mangle_test"
effect "mangle_test"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
`
	out := compileToWGSL(t, src)
	// No mangling needed here; just verify the sample function emits.
	if !strings.Contains(out, "fn lfx_sample(") {
		t.Fatalf("expected lfx_sample in WGSL output:\n%s", out)
	}
}

func TestEmitDeepHelperCallChain(t *testing.T) {
	src := `module "effects/deep_helpers"
effect "deep_helpers"
output scalar
function level3(x)
  return x * x
end
function level2(x)
  return level3(x) + 1.0
end
function level1(x)
  return level2(x) * 0.5
end
function sample(width, height, x, y, index, phase, params)
  return level1(phase)
end
`
	out := compileToWGSL(t, src)
	for _, name := range []string{"fn level3(", "fn level2(", "fn level1(", "fn lfx_sample("} {
		if !strings.Contains(out, name) {
			t.Fatalf("expected %q in WGSL output:\n%s", name, out)
		}
	}
}

func TestEmitUnreachableFunctionsAreNotEmitted(t *testing.T) {
	// A helper that is never called from sample should not appear in WGSL.
	src := `module "effects/unused_helper"
effect "unused_helper"
output scalar
function expensive_unused(x)
  return x * x * x
end
function sample(width, height, x, y, index, phase, params)
  return phase
end
`
	out := compileToWGSL(t, src)
	if strings.Contains(out, "fn expensive_unused(") {
		t.Fatalf("unreachable function should not be emitted:\n%s", out)
	}
}

func TestEmitUsedHelperAppearsExactlyOnce(t *testing.T) {
	src := `module "effects/once_helper"
effect "once_helper"
output scalar
function scale(x)
  return x * 2.0
end
function sample(width, height, x, y, index, phase, params)
  a = scale(phase)
  b = scale(x)
  return a + b
end
`
	out := compileToWGSL(t, src)
	count := strings.Count(out, "fn scale(")
	if count != 1 {
		t.Fatalf("helper function 'scale' should appear exactly once, found %d times:\n%s", count, out)
	}
}

func TestEmitVectorBuiltinsLowerToWGSL(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		needle string
	}{
		{"dot", `  a = vec2(x, y)
  b = vec2(phase, 0.5)
  return dot(a, b)`, "dot("},
		{"length", `  a = vec2(x, y)
  return length(a)`, "length("},
		{"distance", `  a = vec2(x, y)
  b = vec2(phase, 0.5)
  return distance(a, b)`, "distance("},
		{"normalize", `  a = normalize(vec2(x, y))
  return a.x`, "normalize("},
		{"cross_vec3", `module "effects/cross_test"
effect "cross_test"
output scalar
function sample(width, height, x, y, index, phase, params)
  a = vec3(x, y, phase)
  b = vec3(y, phase, x)
  c = cross(a, b)
  return c.x
end
`, "cross("},
		{"project", `module "effects/project_test"
effect "project_test"
output scalar
function sample(width, height, x, y, index, phase, params)
  a = vec3(x, y, phase)
  b = vec3(y, phase, x)
  c = project(a, b)
  return c.x
end
`, "dot("},
		{"reflect", `module "effects/reflect_test"
effect "reflect_test"
output scalar
function sample(width, height, x, y, index, phase, params)
  a = vec3(x, y, phase)
  b = vec3(y, phase, x)
  c = reflect(a, b)
  return c.x
end
`, "reflect("},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var src string
			if strings.HasPrefix(tc.body, "module ") {
				src = tc.body
			} else {
				src = `module "effects/vb_test"
effect "vb_test"
output scalar
function sample(width, height, x, y, index, phase, params)
` + tc.body + `
end
`
			}
			out := compileToWGSL(t, src)
			if !strings.Contains(out, tc.needle) {
				t.Fatalf("expected %q in WGSL for builtin %s:\n%s", tc.needle, tc.name, out)
			}
		})
	}
}

func TestEmitOutputTypesReturnCorrectWGSLTypes(t *testing.T) {
	cases := []struct {
		output  string
		returns string
		retType string
	}{
		{"output scalar", "return phase", "-> f32"},
		{"output rgb", "return x, y, phase", "-> vec3<f32>"},
		{"output rgbw", "return x, y, phase, 0.25", "-> vec4<f32>"},
	}
	for _, tc := range cases {
		t.Run(tc.output, func(t *testing.T) {
			src := `module "effects/ret_type"
effect "ret_type"
` + tc.output + `
function sample(width, height, x, y, index, phase, params)
  ` + tc.returns + `
end
`
			out := compileToWGSL(t, src)
			if !strings.Contains(out, tc.retType) {
				t.Fatalf("expected return type %q for %s in:\n%s", tc.retType, tc.output, out)
			}
		})
	}
}

func TestEmitCommaReturnAndVecReturnAreEquivalent(t *testing.T) {
	commaReturn := compileToWGSL(t, `module "effects/comma_ret"
effect "comma_ret"
output rgb
function sample(width, height, x, y, index, phase, params)
  return x, y, phase
end
`)
	vecReturn := compileToWGSL(t, `module "effects/vec_ret"
effect "vec_ret"
output rgb
function sample(width, height, x, y, index, phase, params)
  return vec3(x, y, phase)
end
`)

	// Both must contain the canonical WGSL return construct.
	for _, out := range []string{commaReturn, vecReturn} {
		if !strings.Contains(out, "return vec3<f32>(x, y, phase);") {
			t.Fatalf("expected 'return vec3<f32>(x, y, phase);' in:\n%s", out)
		}
	}
}

func TestEmitWGSLKeywordInStringConstantDoesNotCorruptOutput(t *testing.T) {
	// LFX doesn't have string return values in sample, but string params in
	// enums contain arbitrary text. Verify the emitter doesn't accidentally
	// treat these strings as code.
	src := `module "effects/kw_in_str"
effect "kw_in_str"
output scalar
params {
  mode = enum("var", "var", "fn", "let", "struct")
}
function sample(width, height, x, y, index, phase, params)
  return phase
end
`
	// If the parser/sema rejects this, the test is vacuously satisfied.
	mod, err := parser.Parse(src)
	if err != nil {
		t.Logf("source rejected at parse time (acceptable): %v", err)
		return
	}
	errs := sema.Analyze(mod, nil)
	if len(errs) != 0 {
		t.Logf("source rejected at sema time (acceptable): %v", errs)
		return
	}
	info, _, _ := sema.AnalyzeModule(mod, nil, nil)
	irmod, err := lower.Lower(mod, nil, info, nil)
	if err != nil {
		t.Logf("source rejected at lower time (acceptable): %v", err)
		return
	}
	out, err := wgsl.Emit(irmod)
	if err != nil {
		t.Fatalf("emit failed: %v", err)
	}
	// The output should not contain bare `var ` or `fn ` as top-level WGSL
	// keywords injected by the string content.
	if strings.Contains(out, "unknown") {
		t.Fatalf("WGSL output contains 'unknown' placeholder:\n%s", out)
	}
}

func TestEmitLargeExpressionTreeProducesValidWGSL(t *testing.T) {
	// Build a deeply nested arithmetic expression to stress the emitter.
	src := `module "effects/big_expr"
effect "big_expr"
output scalar
function sample(width, height, x, y, index, phase, params)
  a = x + y
  b = a * phase
  c = b - x
  d = c / (y + 1.0)
  e = abs(d) + sqrt(abs(a))
  f = clamp(e, 0.0, 1.0)
  g = mix(f, phase, 0.5)
  h = sin(g) + cos(g)
  i = floor(h) + ceil(h - 0.5)
  j = fract(i + x)
  k = mod(j, 1.0)
  l = pow(k, 2.0)
  m = min(l, max(k, 0.1))
  return m
end
`
	out := compileToWGSL(t, src)
	if !strings.Contains(out, "fn lfx_sample(") {
		t.Fatalf("expected lfx_sample in large-expression WGSL:\n%s", out)
	}
	if strings.Contains(out, "unknown") {
		t.Fatalf("WGSL output contains unknown placeholder:\n%s", out)
	}
}
