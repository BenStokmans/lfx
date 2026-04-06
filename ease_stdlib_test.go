package lfx_test

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestEaseStdlibEffectSamplesAndEmitsWGSL(t *testing.T) {
	root := "."
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "ease_stdlib.lfx"), compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	params, err := runtime.Bind(result.IR.Params, nil)
	if err != nil {
		t.Fatalf("bind params: %v", err)
	}

	layout := runtime.Layout{
		Width:  8,
		Height: 4,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 3, Y: 1},
			{Index: 2, X: 7, Y: 3},
		},
	}

	evaluator := cpu.NewEvaluator(result.IR)
	values := make([]float32, len(layout.Points))
	for i := range layout.Points {
		value, err := evaluator.SamplePoint(layout, i, 0.5, params)
		if err != nil {
			t.Fatalf("sample point %d: %v", i, err)
		}
		if math.IsNaN(float64(value[0])) {
			t.Fatalf("sample point %d produced NaN", i)
		}
		if value[0] < 0 || value[0] > 1 {
			t.Fatalf("sample point %d out of range: %f", i, value[0])
		}
		values[i] = value[0]
	}

	// smoothstep on a spatial ramp should vary across x positions
	if values[0] == values[1] && values[1] == values[2] {
		t.Fatalf("ease effect should vary across points, got %v", values)
	}

	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}
	if !strings.Contains(wgslSource, "ease__smoothstep") {
		t.Fatalf("wgsl output missing ease__smoothstep")
	}
	if strings.Contains(wgslSource, "unknown") {
		t.Fatalf("wgsl output contains unknown placeholder:\n%s", wgslSource)
	}
}
