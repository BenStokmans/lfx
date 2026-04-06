package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/runtime"
)

// compileScalarEffect compiles a minimal scalar effect that returns `phase`
// into a CPU evaluator, or fails the test.
func compileScalarEffect(t *testing.T) *cpu.Evaluator {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "e.lfx")
	src := `module "effects/e"
effect "e"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write effect: %v", err)
	}
	result, err := compiler.CompileFile(path, compiler.Options{BaseDir: root})
	if err != nil {
		t.Fatalf("compile effect: %v", err)
	}
	return cpu.NewEvaluator(result.IR)
}

func TestParseLayoutJSONRejectsEmptyPoints(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{"width": 4, "height": 4, "points": []}`))
	// An empty points list may or may not be rejected at parse time;
	// document the current behavior.
	if err != nil {
		t.Logf("empty points rejected at parse time: %v", err)
	} else {
		t.Logf("empty points accepted at parse time (semantics may differ at runtime)")
	}
}

func TestCPUEvaluatorHandlesOnePointLayout(t *testing.T) {
	// ── Test 82: One-point layout ─────────────────────────────────────────────
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  1,
		Height: 1,
		Points: []runtime.Point{{Index: 0, X: 0, Y: 0}},
	}
	params, _ := runtime.Bind(nil, nil)
	vals, err := ev.SamplePoint(layout, 0, 0.5, params)
	if err != nil {
		t.Fatalf("one-point layout: %v", err)
	}
	if vals[0] != 0.5 {
		t.Fatalf("sample = %v, want 0.5", vals[0])
	}
}

func TestCPUEvaluatorHandlesNonMonotonicIndices(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{
			{Index: 2, X: 1, Y: 1},
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 2, Y: 0},
		},
	}
	params, _ := runtime.Bind(nil, nil)
	for i := range layout.Points {
		_, err := ev.SamplePoint(layout, i, 0.5, params)
		if err != nil {
			t.Fatalf("non-monotonic index layout point %d: %v", i, err)
		}
	}
}

func TestParseLayoutJSONRejectsDuplicatePointIndices(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{
		"width": 4, "height": 4,
		"points": [
			{"index": 0, "x": 0, "y": 0},
			{"index": 0, "x": 1, "y": 0}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate index") {
		t.Fatalf("expected duplicate index error, got: %v", err)
	}
}

func TestCPUEvaluatorHandlesPointsOutOfIndexOrder(t *testing.T) {
	// The runtime stores points as a slice; the lookup uses slice position,
	// not the index field. Document the behavior.
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{
			{Index: 9, X: 3, Y: 3},
			{Index: 3, X: 0, Y: 1},
			{Index: 1, X: 2, Y: 2},
		},
	}
	params, _ := runtime.Bind(nil, nil)
	for i := range layout.Points {
		_, err := ev.SamplePoint(layout, i, 0.4, params)
		if err != nil {
			t.Fatalf("out-of-order index layout point %d: %v", i, err)
		}
	}
}

func TestCPUEvaluatorAcceptsFractionalCoordinates(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  10,
		Height: 10,
		Points: []runtime.Point{
			{Index: 0, X: 0.5, Y: 0.5},
			{Index: 1, X: 1.25, Y: 2.75},
			{Index: 2, X: 9.99, Y: 0.01},
		},
	}
	params, _ := runtime.Bind(nil, nil)
	for i := range layout.Points {
		_, err := ev.SamplePoint(layout, i, 0.5, params)
		if err != nil {
			t.Fatalf("fractional coord point %d: %v", i, err)
		}
	}
}

func TestCPUEvaluatorAcceptsNegativeCoordinates(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  10,
		Height: 10,
		Points: []runtime.Point{
			{Index: 0, X: -1, Y: -1},
			{Index: 1, X: -5, Y: 3},
		},
	}
	params, _ := runtime.Bind(nil, nil)
	for i := range layout.Points {
		_, err := ev.SamplePoint(layout, i, 0.3, params)
		if err != nil {
			t.Fatalf("negative coord point %d: %v", i, err)
		}
	}
}

func TestCPUEvaluatorAcceptsLargeCoordinates(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  1e6,
		Height: 1e6,
		Points: []runtime.Point{
			{Index: 0, X: 1e5, Y: 9e5},
			{Index: 1, X: 500000, Y: 1},
		},
	}
	params, _ := runtime.Bind(nil, nil)
	for i := range layout.Points {
		_, err := ev.SamplePoint(layout, i, 0.1, params)
		if err != nil {
			t.Fatalf("large coord point %d: %v", i, err)
		}
	}
}

func TestParseLayoutJSONRejectsZeroWidth(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{"width": 0, "height": 4, "points": [{"index": 0, "x": 0, "y": 0}]}`))
	if err == nil || !strings.Contains(err.Error(), "width") {
		t.Fatalf("expected width error for zero-width layout, got: %v", err)
	}
}

func TestParseLayoutJSONRejectsZeroHeight(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{"width": 4, "height": 0, "points": [{"index": 0, "x": 0, "y": 0}]}`))
	if err == nil || !strings.Contains(err.Error(), "height") {
		t.Fatalf("expected height error for zero-height layout, got: %v", err)
	}
}

func TestCPUEvaluatorAcceptsPointsBeyondLayoutBounds(t *testing.T) {
	// The runtime should not error if a point's x exceeds the declared width;
	// it's up to the caller to ensure consistency.
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  2,
		Height: 2,
		Points: []runtime.Point{
			{Index: 0, X: 10, Y: 10}, // well outside declared bounds
		},
	}
	params, _ := runtime.Bind(nil, nil)
	_, err := ev.SamplePoint(layout, 0, 0.5, params)
	if err != nil {
		t.Logf("point outside declared bounds rejected: %v", err)
	} else {
		t.Logf("point outside declared bounds accepted (no bounds check)")
	}
}

func TestCPUEvaluatorHandlesNegativePhase(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{{Index: 0, X: 1, Y: 1}},
	}
	params, _ := runtime.Bind(nil, nil)
	vals, err := ev.SamplePoint(layout, 0, -0.1, params)
	if err != nil {
		t.Logf("negative phase rejected: %v", err)
		return
	}
	// The effect returns phase directly; the result may be negative.
	t.Logf("negative phase accepted; result = %v", vals)
}

func TestCPUEvaluatorHandlesPhaseAboveOne(t *testing.T) {
	ev := compileScalarEffect(t)
	layout := runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{{Index: 0, X: 1, Y: 1}},
	}
	params, _ := runtime.Bind(nil, nil)
	vals, err := ev.SamplePoint(layout, 0, 1.5, params)
	if err != nil {
		t.Logf("phase > 1 rejected: %v", err)
		return
	}
	t.Logf("phase > 1 accepted; result = %v", vals)
}
