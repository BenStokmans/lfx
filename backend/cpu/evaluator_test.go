package cpu_test

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestFillIrisSamplingIsSymmetric(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "fill_iris.lfx"), compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err != nil {
		t.Fatalf("compile file: %v", err)
	}

	params, err := runtime.Bind(result.IR.Params, nil)
	if err != nil {
		t.Fatalf("bind params: %v", err)
	}

	layout := runtime.Layout{
		Width:  5,
		Height: 5,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 2, Y: 2},
			{Index: 2, X: 4, Y: 4},
		},
	}

	evaluator := cpu.NewEvaluator(result.IR)
	cornerA, err := evaluator.SamplePoint(layout, 0, 0.3, params)
	if err != nil {
		t.Fatalf("sample point 0: %v", err)
	}
	center, err := evaluator.SamplePoint(layout, 1, 0.3, params)
	if err != nil {
		t.Fatalf("sample point 1: %v", err)
	}
	cornerB, err := evaluator.SamplePoint(layout, 2, 0.3, params)
	if err != nil {
		t.Fatalf("sample point 2: %v", err)
	}

	if center <= cornerA {
		t.Fatalf("center value %f should exceed corner %f", center, cornerA)
	}
	if math.Abs(float64(cornerA-cornerB)) > 1e-6 {
		t.Fatalf("corner symmetry mismatch: %f vs %f", cornerA, cornerB)
	}
}
