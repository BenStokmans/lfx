package cpu_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

func mustUint32(t *testing.T, value int) uint32 {
	t.Helper()
	maxUint32 := int(^uint32(0))
	if value < 0 || value > maxUint32 {
		t.Fatalf("value %d does not fit in uint32", value)
	}
	//nolint:gosec // guarded by bounds check above
	return uint32(value)
}

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
			//nolint:gosec
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
			//nolint:gosec
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

func TestEvaluatorSamplePointsMatchesScalarForChromaBloom(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "chroma_bloom.lfx"), compiler.Options{
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
		Width:  8,
		Height: 8,
		Points: make([]runtime.Point, 0, 64),
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			layout.Points = append(layout.Points, runtime.Point{
				Index: mustUint32(t, len(layout.Points)),
				X:     float32(x),
				Y:     float32(y),
			})
		}
	}

	evaluator := cpu.NewEvaluator(result.IR)
	pointIndices := make([]int, len(layout.Points))
	for idx := range pointIndices {
		pointIndices[idx] = idx
	}

	batched, err := evaluator.SamplePoints(layout, pointIndices, 0.37, params)
	if err != nil {
		t.Fatalf("sample points: %v", err)
	}

	channels := result.IR.Output.Channels()
	if got, want := len(batched), len(pointIndices)*channels; got != want {
		t.Fatalf("batched output len = %d, want %d", got, want)
	}

	for idx := range pointIndices {
		scalar, err := evaluator.SamplePoint(layout, idx, 0.37, params)
		if err != nil {
			t.Fatalf("sample point %d: %v", idx, err)
		}
		base := idx * channels
		for channel := 0; channel < channels; channel++ {
			if math.Abs(float64(batched[base+channel]-scalar[channel])) > 1e-6 {
				t.Fatalf("point %d channel %d = %f, want %f", idx, channel, batched[base+channel], scalar[channel])
			}
		}
	}
}

func TestEvaluatorSamplePointsMatchesScalarForFillIris(t *testing.T) {
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
	batched, err := evaluator.SamplePoints(layout, []int{0, 1, 2}, 0.3, params)
	if err != nil {
		t.Fatalf("sample points: %v", err)
	}
	if len(batched) != 3 {
		t.Fatalf("batched len = %d, want 3", len(batched))
	}

	for idx := range layout.Points {
		scalar, err := evaluator.SamplePoint(layout, idx, 0.3, params)
		if err != nil {
			t.Fatalf("sample point %d: %v", idx, err)
		}
		if math.Abs(float64(batched[idx]-scalar[0])) > 1e-6 {
			t.Fatalf("point %d = %f, want %f", idx, batched[idx], scalar[0])
		}
	}
}

func TestEvaluatorSamplePointsMatchesScalarForPerlin501(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "perlin_501.lfx"), compiler.Options{
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
		Width:  8,
		Height: 8,
		Points: make([]runtime.Point, 0, 64),
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			layout.Points = append(layout.Points, runtime.Point{
				Index: mustUint32(t, len(layout.Points)),
				X:     float32(x),
				Y:     float32(y),
			})
		}
	}

	evaluator := cpu.NewEvaluator(result.IR)
	pointIndices := make([]int, len(layout.Points))
	for idx := range pointIndices {
		pointIndices[idx] = idx
	}

	batched, err := evaluator.SamplePoints(layout, pointIndices, 0.37, params)
	if err != nil {
		t.Fatalf("sample points: %v", err)
	}
	if got, want := len(batched), len(pointIndices); got != want {
		t.Fatalf("batched output len = %d, want %d", got, want)
	}

	for idx := range layout.Points {
		scalar, err := evaluator.SamplePoint(layout, idx, 0.37, params)
		if err != nil {
			t.Fatalf("sample point %d: %v", idx, err)
		}
		if math.Abs(float64(batched[idx]-scalar[0])) > 1e-6 {
			t.Fatalf("point %d = %f, want %f", idx, batched[idx], scalar[0])
		}
	}
}

func TestEvaluatorSamplePointsMatchesScalarForEffectCorpus(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	entries, err := os.ReadDir(filepath.Join(root, "effects"))
	if err != nil {
		t.Fatalf("read effects dir: %v", err)
	}

	layout := makeGridTestLayout(8, 8)
	pointIndices := make([]int, len(layout.Points))
	for idx := range pointIndices {
		pointIndices[idx] = idx
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lfx") {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), ".lfx"), func(t *testing.T) {
			result, err := compiler.CompileFile(filepath.Join(root, "effects", entry.Name()), compiler.Options{
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

			evaluator := cpu.NewEvaluator(result.IR)
			batched, err := evaluator.SamplePoints(layout, pointIndices, 0.37, params)
			if err != nil {
				t.Fatalf("sample points: %v", err)
			}

			channels := result.IR.Output.Channels()
			if got, want := len(batched), len(pointIndices)*channels; got != want {
				t.Fatalf("batched output len = %d, want %d", got, want)
			}

			for idx := range pointIndices {
				scalar, err := evaluator.SamplePoint(layout, idx, 0.37, params)
				if err != nil {
					t.Fatalf("sample point %d: %v", idx, err)
				}
				base := idx * channels
				for channel := 0; channel < channels; channel++ {
					if math.Abs(float64(batched[base+channel]-scalar[channel])) > 1e-6 {
						t.Fatalf("point %d channel %d = %f, want %f", idx, channel, batched[base+channel], scalar[channel])
					}
				}
			}
		})
	}
}

func makeGridTestLayout(width, height int) runtime.Layout {
	points := make([]runtime.Point, 0, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			points = append(points, runtime.Point{
				//nolint:gosec
				Index: uint32(len(points)),
				X:     float32(x),
				Y:     float32(y),
			})
		}
	}
	return runtime.Layout{
		Width:  float32(width),
		Height: float32(height),
		Points: points,
	}
}
