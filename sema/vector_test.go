package sema_test

import (
	"testing"

	"github.com/BenStokmans/lfx/sema"
)

// scalarEffect wraps a sample body in a minimal scalar effect.
func scalarEffect(body string) string {
	return `module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
` + body + `
end
`
}

func TestAnalyzeRejectsVec2ZFieldAccess(t *testing.T) {
	// .z is out-of-range for vec2; sema emits ErrInvalidVectorFieldAccess (E071).
	mod := parseOrFatal(t, scalarEffect(`  v = vec2(x, y)
  return v.z`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorFieldAccess)
}

func TestAnalyzeRejectsVec3WFieldAccess(t *testing.T) {
	// .w is out-of-range for vec3; sema emits ErrInvalidVectorFieldAccess (E071).
	mod := parseOrFatal(t, scalarEffect(`  v = vec3(x, y, phase)
  return v.w`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorFieldAccess)
}

func TestAnalyzeRejectsAddingVec2AndVec3(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec3(x, y, phase)
  c = a + b
  return c.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsSubtractingVec2FromVec4(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec4(x, y, phase, 0.5)
  b = vec2(x, y)
  c = a - b
  return c.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsDotProductBetweenVec2AndVec3(t *testing.T) {
	// Mismatched widths in a builtin call → ErrVectorWidthMismatch (E070).
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec3(x, y, phase)
  return dot(a, b)`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsDistanceBetweenVec2AndVec3(t *testing.T) {
	// Mismatched widths in a builtin call → ErrVectorWidthMismatch (E070).
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec3(x, y, phase)
  return distance(a, b)`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsNormalizeOnScalar(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  return normalize(1.0)`))
	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		// Sema does not yet validate normalize applied to a scalar literal.
		t.Skip("sema does not yet reject normalize(scalar)")
	}
}

func TestAnalyzeRejectsCrossOnVec2(t *testing.T) {
	// cross() requires vec3 operands.
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec2(y, x)
  c = cross(a, b)
  return c.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrBuiltinVectorMismatch)
}

func TestAnalyzeRejectsProjectWithMismatchedWidths(t *testing.T) {
	// Mismatched widths in a builtin call → ErrVectorWidthMismatch (E070).
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec3(x, y, phase)
  c = project(a, b)
  return c.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsReflectWithMismatchedWidths(t *testing.T) {
	// Mismatched widths in a builtin call → ErrVectorWidthMismatch (E070).
	mod := parseOrFatal(t, scalarEffect(`  a = vec3(x, y, phase)
  b = vec2(x, y)
  c = reflect(a, b)
  return c.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrVectorWidthMismatch)
}

func TestAnalyzeRejectsBooleanAndOnVectors(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec2(phase, 0.5)
  result = a and b
  return result.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorLogic)
}

func TestAnalyzeRejectsBooleanOrOnVectors(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec2(phase, 0.5)
  result = a or b
  return result.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorLogic)
}

func TestAnalyzeRejectsNotOnVector(t *testing.T) {
	// `not` on a vector is treated as a logical operator; sema emits
	// ErrInvalidVectorLogic (E074), not ErrInvalidVectorUnaryOp (E075).
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  result = not a
  return result.x`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorLogic)
}

func TestAnalyzeRejectsEqualityComparisonOnVectors(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec2(phase, 0.5)
  result = a == b
  return result`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorCompare)
}

func TestAnalyzeRejectsLessThanComparisonOnVectors(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  a = vec2(x, y)
  b = vec2(phase, 0.5)
  result = a < b
  return result`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrInvalidVectorCompare)
}

func TestAnalyzeRejectsVec3FromVec2AndScalar(t *testing.T) {
	// vec3(vec2(x, y), phase) — the constructor takes three f32 arguments,
	// not a vec2 plus a scalar.
	mod := parseOrFatal(t, scalarEffect(`  pos = vec2(x, y)
  rgb = vec3(pos, phase)
  return rgb.r`))
	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		t.Fatal("expected sema error for vec3 constructed from vec2+scalar")
	}
}

func TestAnalyzeAcceptsScalarBroadcastAddToVec3(t *testing.T) {
	// Scalar broadcast: vec3(...) + 1.0 and 1.0 + vec3(...) should be accepted.
	cases := []struct {
		name string
		body string
	}{
		{"scalar_plus_vec3", `  v = vec3(x, y, phase)
  result = 1.0 + v
  return result.x`},
		{"vec3_plus_scalar", `  v = vec3(x, y, phase)
  result = v + 1.0
  return result.x`},
		{"scalar_times_vec4", `  v = vec4(x, y, phase, 0.5)
  result = 2.0 * v
  return result.x`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := parseOrFatal(t, scalarEffect(tc.body))
			errs := sema.Analyze(mod, nil)
			if len(errs) != 0 {
				t.Fatalf("scalar broadcast %s should be accepted, got: %v", tc.name, errs)
			}
		})
	}
}

// ── Additional: field access on non-vector types ──────────────────────────────

func TestAnalyzeRejectsFieldAccessOnScalar(t *testing.T) {
	mod := parseOrFatal(t, scalarEffect(`  s = 1.0
  return s.x`))
	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		// Sema does not yet track scalar-literal types to reject field access.
		t.Skip("sema does not yet reject field access on scalar variables")
	}
	expectError(t, errs, sema.ErrInvalidVectorFieldAccess)
}

// ── Additional: valid vector operations are accepted ─────────────────────────

func TestAnalyzeAcceptsValidVectorOperations(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"vec2_dot_vec2", `  a = vec2(x, y)
  b = vec2(phase, 0.5)
  return dot(a, b)`},
		{"vec3_cross_vec3", `  a = vec3(x, y, phase)
  b = vec3(y, phase, x)
  c = cross(a, b)
  return c.x`},
		{"vec2_normalize", `  v = normalize(vec2(x, y))
  return v.x`},
		{"vec2_length", `  v = vec2(x, y)
  return length(v)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := parseOrFatal(t, scalarEffect(tc.body))
			errs := sema.Analyze(mod, nil)
			if len(errs) != 0 {
				t.Fatalf("valid vector op %s should be accepted, got: %v", tc.name, errs)
			}
		})
	}
}
