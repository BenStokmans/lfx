package sema_test

import (
	"testing"

	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

func TestAnalyzeFillIrisAllowsParamsObject(t *testing.T) {
	entry, err := parser.Parse(`
version "0.1"
module "effects/fill_iris"
effect "fill_iris"
output scalar

params {
  rampsize = int(4, 0, 100)
  grid_aligned = bool(false)
}

function sample(width, height, x, y, index, phase, params)
  cx = (width - 1.0) / 2.0
  cy = (height - 1.0) / 2.0
  w_even = is_even(width)
  h_even = is_even(height)
  dx = abs(x - cx)
  dy = abs(y - cy)
  if params.grid_aligned then
    if w_even > 0.0 then
      dx = dx - 0.5
    end
  end
  if params.grid_aligned then
    if h_even > 0.0 then
      dy = dy - 0.5
    end
  end
  dist = sqrt(dx * dx + dy * dy)
  pos = phase - dist
  if pos < 0.0 then
    return 0.0
  end
  if pos < params.rampsize then
    return pos / params.rampsize
  end
  if pos < 10.0 then
    return 1.0
  end
  return 0.0
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	errs := sema.Analyze(entry, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected semantic errors: %v", errs)
	}
}

func TestAnalyzeUnknownParamField(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/bad_param"
effect "bad_param"
output scalar
params {
  width_scale = float(1.0, 0.0, 2.0)
}
function sample(width, height, x, y, index, phase, params)
  return params.missing
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		t.Fatal("expected semantic error")
	}
	if errs[0].Code != sema.ErrUnknownParameter {
		t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrUnknownParameter)
	}
}

func TestAnalyzeParameterAndPresetValidation(t *testing.T) {
	t.Run("rejects numeric param default above max", func(t *testing.T) {
		mod, err := parser.Parse(`
module "effects/bad_param_bounds"
effect "bad_param_bounds"
output scalar
params {
  gain = float(1.5, 0.0, 1.0)
}
function sample(width, height, x, y, index, phase, params)
  return params.gain
end
`)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}

		errs := sema.Analyze(mod, nil)
		if len(errs) == 0 {
			t.Fatal("expected semantic error")
		}
		if errs[0].Code != sema.ErrParamDefaultAboveMax {
			t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrParamDefaultAboveMax)
		}
	})

	t.Run("accepts numeric param default within bounds", func(t *testing.T) {
		mod, err := parser.Parse(`
module "effects/good_param_bounds"
effect "good_param_bounds"
output scalar
params {
  gain = float(0.75, 0.0, 1.0)
}
function sample(width, height, x, y, index, phase, params)
  return params.gain
end
`)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}

		errs := sema.Analyze(mod, nil)
		if len(errs) != 0 {
			t.Fatalf("unexpected semantic errors: %v", errs)
		}
	})

	t.Run("rejects timeline loop ordering violation", func(t *testing.T) {
		mod, err := parser.Parse(`
module "effects/bad_timeline"
effect "bad_timeline"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
timeline {
  loop_start = 0.8
  loop_end = 0.2
}
`)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}

		errs := sema.Analyze(mod, nil)
		if len(errs) == 0 {
			t.Fatal("expected semantic error")
		}
		if errs[0].Code != sema.ErrTimelineLoopStartAfterLoopEnd {
			t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrTimelineLoopStartAfterLoopEnd)
		}
	})

	t.Run("accepts timeline ordering when valid", func(t *testing.T) {
		mod, err := parser.Parse(`
module "effects/good_timeline"
effect "good_timeline"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
timeline {
  loop_start = 0.2
  loop_end = 0.8
}
`)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}

		errs := sema.Analyze(mod, nil)
		if len(errs) != 0 {
			t.Fatalf("unexpected semantic errors: %v", errs)
		}
	})
}

func TestAnalyzeRejectsOutputInLibrary(t *testing.T) {
	mod, err := parser.Parse(`
module "stdlib/rgb"
library "rgb"
output rgb
export function shade(v)
  return v
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		t.Fatal("expected semantic error")
	}
	if errs[0].Code != sema.ErrOutputInLibrary {
		t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrOutputInLibrary)
	}
}

func TestAnalyzeRejectsReturnArityMismatch(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/bad_rgb"
effect "Bad RGB"
output rgb
function sample(width, height, x, y, index, phase, params)
  return phase
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		t.Fatal("expected semantic error")
	}
	if errs[0].Code != sema.ErrReturnArityMismatch {
		t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrReturnArityMismatch)
	}
}

func TestAnalyzeRejectsMissingOutput(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/missing_output"
effect "Missing Output"
function sample(width, height, x, y, index, phase, params)
  return phase
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	errs := sema.Analyze(mod, nil)
	if len(errs) == 0 {
		t.Fatal("expected semantic error")
	}
	if errs[0].Code != sema.ErrEffectMissingOutput {
		t.Fatalf("first error code = %s, want %s", errs[0].Code, sema.ErrEffectMissingOutput)
	}
}

func TestAnalyzeInfersVectorFunctionSignatures(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/vector_infer"
effect "Vector Infer"
output scalar
function helper(pos)
  shifted = normalize(pos + 1.0)
  return shifted.x + shifted.y
end
function sample(width, height, x, y, index, phase, params)
  return helper(vec2(x, y))
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	info, errs, warns := sema.AnalyzeModule(mod, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected semantic errors: %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected semantic warnings: %v", warns)
	}

	helper := mod.Funcs[0]
	sig := info.FuncTypes[helper]
	if len(sig.Params) != 1 || sig.Params[0].String() != "vec2" {
		t.Fatalf("helper params = %#v, want vec2", sig.Params)
	}
	if sig.ReturnType.String() != "f32" {
		t.Fatalf("helper return type = %s, want f32", sig.ReturnType)
	}
}

func TestAnalyzeAllowsShadowingBuiltinWithAssignment(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/shadow_builtin"
effect "Shadow Builtin"
output scalar
function sample(width, height, x, y, index, phase, params)
  cross = x + y
  return cross
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	_, errs, warns := sema.AnalyzeModule(mod, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected semantic errors: %v", errs)
	}
	if len(warns) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warns))
	}
	if warns[0].Code != sema.WarnBuiltinShadowed {
		t.Fatalf("warning code = %s, want %s", warns[0].Code, sema.WarnBuiltinShadowed)
	}
}
