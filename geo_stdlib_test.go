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

func TestGeoStdlibEffectSamplesAndEmitsWGSL(t *testing.T) {
	root := "."
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "geo_stdlib.lfx"), compiler.Options{
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
		Height: 8,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 4, Y: 4},
			{Index: 2, X: 7, Y: 7},
		},
	}

	evaluator := cpu.NewEvaluator(result.IR)
	values := make([]float32, len(layout.Points))
	for i := range layout.Points {
		value, err := evaluator.SamplePoint(layout, i, 0.0, params)
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

	// corner vs center should differ (SDF varies radially)
	if values[0] == values[1] {
		t.Fatalf("geo effect should vary between corner and center, got %v", values)
	}

	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}
	if !strings.Contains(wgslSource, "geo__sdf_circle") {
		t.Fatalf("wgsl output missing geo__sdf_circle")
	}
	if strings.Contains(wgslSource, "unknown") {
		t.Fatalf("wgsl output contains unknown placeholder:\n%s", wgslSource)
	}
}
