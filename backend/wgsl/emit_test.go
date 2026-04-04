package wgsl_test

import (
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

func TestEmitConvertsSourceSyntaxToWGSLSafeOutput(t *testing.T) {
	mod, err := parser.Parse(`module "effects/backend_syntax"
effect "Backend Syntax"
params {
  gain = float(0.75, 0.0, 1.0)
}
function helper(value, params)
  active = params.gain > 0.0 and not (value < 0.5 or params.gain < 0.25)
  if active then
    return value * params.gain
  end
  return 0.0
end
function sample(width, height, x, y, index, phase, params)
  return helper(phase, params)
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if errs := sema.Analyze(mod, nil); len(errs) != 0 {
		t.Fatalf("unexpected semantic errors: %v", errs)
	}

	irmod, err := lower.Lower(mod, nil)
	if err != nil {
		t.Fatalf("lower source: %v", err)
	}
	lower.ConstFold(irmod)

	wgslSource, err := wgsl.Emit(irmod)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}

	required := []string{
		"fn lfx_sample(width: f32, height: f32, x: f32, y: f32, index: f32, phase: f32, params: f32) -> f32 {",
		"uniforms.param_gain",
		"&&",
		"||",
		"!((select(",
		"var result = lfx_sample(uniforms.width, uniforms.height, pt.x, pt.y, f32(pt.index), uniforms.phase, 0.0);",
	}
	for _, needle := range required {
		if !strings.Contains(wgslSource, needle) {
			t.Fatalf("wgsl output missing %q:\n%s", needle, wgslSource)
		}
	}

	forbidden := []string{
		"params.gain",
		" and ",
		" or ",
		" not ",
	}
	for _, needle := range forbidden {
		if strings.Contains(wgslSource, needle) {
			t.Fatalf("wgsl output contains backend-invalid source syntax %q:\n%s", needle, wgslSource)
		}
	}
}
