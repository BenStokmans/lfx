package lfx_test

import (
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

func TestExampleEffectsCompileAcrossBackends(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	layout := runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 1, Y: 1},
			{Index: 2, X: 3, Y: 2},
		},
	}

	effects := []string{
		"filled",
		"fill_layout_order",
		"fill_horizontal",
		"fill_iris",
		"bar_horizontal",
		"noise_stdlib",
		"perlin_501",
	}

	for _, effect := range effects {
		t.Run(effect, func(t *testing.T) {
			result, err := compiler.CompileFile(filepath.Join(root, "effects", effect+".lfx"), compiler.Options{
				BaseDir:  root,
				Resolver: resolver,
			})
			if err != nil {
				t.Fatalf("compile file: %v", err)
			}

			params, err := runtime.Bind(result.IR.Params, nil)
			if err != nil {
				t.Fatalf("bind params: %v", err)
			}

			evaluator := cpu.NewEvaluator(result.IR)
			for i := range layout.Points {
				value, err := evaluator.SamplePoint(layout, i, 0.25, params)
				if err != nil {
					t.Fatalf("cpu sample point %d: %v", i, err)
				}
				if value < 0 || value > 1 {
					t.Fatalf("cpu sample point %d out of range: %f", i, value)
				}
			}

			wgslSource, err := wgsl.Emit(result.IR)
			if err != nil {
				t.Fatalf("emit wgsl: %v", err)
			}
			if !strings.Contains(wgslSource, "fn lfx_sample(") {
				t.Fatal("wgsl output missing lfx_sample")
			}
			if strings.Contains(wgslSource, "unknown") {
				t.Fatalf("wgsl output contains unknown placeholder:\n%s", wgslSource)
			}
		})
	}
}
