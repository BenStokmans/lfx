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

func TestPaletteStdlibEffectSamplesAndEmitsWGSL(t *testing.T) {
	root := "."
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "palette_stdlib.lfx"), compiler.Options{
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
	for i := range layout.Points {
		value, err := evaluator.SamplePoint(layout, i, 0.1, params)
		if err != nil {
			t.Fatalf("sample point %d: %v", i, err)
		}
		// palette returns rgb: check all 3 channels
		for ch := 0; ch < 3; ch++ {
			if math.IsNaN(float64(value[ch])) {
				t.Fatalf("sample point %d channel %d produced NaN", i, ch)
			}
			if value[ch] < 0 || value[ch] > 1 {
				t.Fatalf("sample point %d channel %d out of range: %f", i, ch, value[ch])
			}
		}
	}

	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}
	if !strings.Contains(wgslSource, "palette__rainbow") {
		t.Fatalf("wgsl output missing palette__rainbow")
	}
	if strings.Contains(wgslSource, "unknown") {
		t.Fatalf("wgsl output contains unknown placeholder:\n%s", wgslSource)
	}
}
