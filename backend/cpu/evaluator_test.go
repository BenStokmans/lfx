package cpu_test

import (
	"math"
	"os"
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

	if center[0] <= cornerA[0] {
		t.Fatalf("center value %f should exceed corner %f", center[0], cornerA[0])
	}
	if math.Abs(float64(cornerA[0]-cornerB[0])) > 1e-6 {
		t.Fatalf("corner symmetry mismatch: %f vs %f", cornerA[0], cornerB[0])
	}
}

func TestEvaluatorSupportsRGBAndRGBWOutputs(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   []float32
	}{
		{name: "rgb", output: "output rgb", want: []float32{0.25, 0.5, 1}},
		{name: "rgbw", output: "output rgbw", want: []float32{0.25, 0.5, 1, 0.75}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			filePath := filepath.Join(root, "effect.lfx")
			source := `module "effects/output_test"
effect "Output Test"
` + tc.output + `
function sample(width, height, x, y, index, phase, params)
  return 0.25, 0.5, 1.0`
			if tc.name == "rgbw" {
				source += `, 0.75`
			}
			source += `
end
`
			if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
				t.Fatalf("write effect: %v", err)
			}
			result, err := compiler.CompileFile(filePath, compiler.Options{BaseDir: root})
			if err != nil {
				t.Fatalf("compile file: %v", err)
			}
			params, err := runtime.Bind(result.IR.Params, nil)
			if err != nil {
				t.Fatalf("bind params: %v", err)
			}
			layout := runtime.Layout{Width: 1, Height: 1, Points: []runtime.Point{{Index: 0, X: 0, Y: 0}}}
			values, err := cpu.NewEvaluator(result.IR).SamplePoint(layout, 0, 0, params)
			if err != nil {
				t.Fatalf("sample point: %v", err)
			}
			if len(values) != len(tc.want) {
				t.Fatalf("value count = %d, want %d", len(values), len(tc.want))
			}
			for i := range tc.want {
				if values[i] != tc.want[i] {
					t.Fatalf("value[%d] = %f, want %f", i, values[i], tc.want[i])
				}
			}
		})
	}
}

func TestEvaluatorSupportsVectorSampleReturns(t *testing.T) {
	cases := []struct {
		name   string
		output string
		ret    string
		want   []float32
	}{
		{name: "rgb", output: "output rgb", ret: "vec3(0.25, 0.5, 1.0)", want: []float32{0.25, 0.5, 1}},
		{name: "rgbw", output: "output rgbw", ret: "vec4(0.25, 0.5, 1.0, 0.75)", want: []float32{0.25, 0.5, 1, 0.75}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			filePath := filepath.Join(root, "effect.lfx")
			source := `module "effects/vector_output"
effect "Vector Output"
` + tc.output + `
function sample(width, height, x, y, index, phase, params)
  return ` + tc.ret + `
end
`
			if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
				t.Fatalf("write effect: %v", err)
			}
			result, err := compiler.CompileFile(filePath, compiler.Options{BaseDir: root})
			if err != nil {
				t.Fatalf("compile file: %v", err)
			}
			params, err := runtime.Bind(result.IR.Params, nil)
			if err != nil {
				t.Fatalf("bind params: %v", err)
			}
			layout := runtime.Layout{Width: 1, Height: 1, Points: []runtime.Point{{Index: 0, X: 0, Y: 0}}}
			values, err := cpu.NewEvaluator(result.IR).SamplePoint(layout, 0, 0, params)
			if err != nil {
				t.Fatalf("sample point: %v", err)
			}
			for idx := range tc.want {
				if values[idx] != tc.want[idx] {
					t.Fatalf("value[%d] = %f, want %f", idx, values[idx], tc.want[idx])
				}
			}
		})
	}
}
