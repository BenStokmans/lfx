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
