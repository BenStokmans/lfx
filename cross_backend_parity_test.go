package lfx_test

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

// cpuParityTol is the maximum allowed absolute difference between two f32
// values when comparing CPU evaluation across runs.
const cpuParityTol = 1e-6

// compileEffect compiles an effect from a temp-dir source string.
func compileEffect(t *testing.T, source string) *compiler.Result {
	t.Helper()
	root := t.TempDir()
	effectsDir := filepath.Join(root, "effects")
	//nolint:gosec
	if err := os.MkdirAll(effectsDir, 0o755); err != nil {
		t.Fatalf("mkdir effects: %v", err)
	}
	path := filepath.Join(effectsDir, "parity.lfx")
	//nolint:gosec
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write effect: %v", err)
	}
	result, err := compiler.CompileFile(path, compiler.Options{BaseDir: root})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return result
}

// makeParityLayout builds a deterministic layout for parity tests.
func makeParityLayout() runtime.Layout {
	return runtime.Layout{
		Width:  8,
		Height: 8,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 1, Y: 2},
			{Index: 2, X: 3, Y: 1},
			{Index: 3, X: 7, Y: 7},
			{Index: 4, X: 4, Y: 0},
			{Index: 5, X: 0, Y: 4},
		},
	}
}

func TestCPUEvaluatorIsDeterministicForSameInputs(t *testing.T) {
	result, err := compiler.CompileFile(
		filepath.Join(".", "effects", "fill_iris.lfx"),
		compiler.Options{
			BaseDir:  ".",
			Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(".")...)),
		},
	)
	if err != nil {
		t.Fatalf("compile fill_iris: %v", err)
	}

	params, err := runtime.Bind(result.IR.Params, nil)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	layout := makeParityLayout()
	ev := cpu.NewEvaluator(result.IR)

	const runs = 5
	var baseline [][]float32
	for run := 0; run < runs; run++ {
		var rowValues []float32
		for i := range layout.Points {
			vals, err := ev.SamplePoint(layout, i, 0.4, params)
			if err != nil {
				t.Fatalf("sample run %d point %d: %v", run, i, err)
			}
			rowValues = append(rowValues, vals...)
		}
		if run == 0 {
			baseline = append(baseline, rowValues)
		} else {
			for j, v := range rowValues {
				if math.Abs(float64(v-baseline[0][j])) > cpuParityTol {
					t.Fatalf("run %d point %d: got %f, want %f (non-deterministic)", run, j, v, baseline[0][j])
				}
			}
		}
	}
}

func TestCPURGBAOutputsAreConsistentAcrossMultipleEvaluations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		output  string
		returns string
	}{
		{"rgb", "output rgb", "return x, y, phase"},
		{"rgbw", "output rgbw", "return x, y, phase, 0.25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := compileEffect(t, `module "effects/parity"
effect "parity"
`+tc.output+`
function sample(width, height, x, y, index, phase, params)
  `+tc.returns+`
end
`)
			params, _ := runtime.Bind(result.IR.Params, nil)
			layout := makeParityLayout()
			ev := cpu.NewEvaluator(result.IR)

			vals1, err := ev.SamplePoint(layout, 0, 0.3, params)
			if err != nil {
				t.Fatalf("sample 1: %v", err)
			}
			vals2, err := ev.SamplePoint(layout, 0, 0.3, params)
			if err != nil {
				t.Fatalf("sample 2: %v", err)
			}
			for i := range vals1 {
				if math.Abs(float64(vals1[i]-vals2[i])) > cpuParityTol {
					t.Fatalf("channel %d: %f vs %f (non-deterministic)", i, vals1[i], vals2[i])
				}
			}
		})
	}
}

func TestCPUStdlibHelperEffectsAreConsistent(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	result, err := compiler.CompileFile(
		filepath.Join(root, "effects", "noise_stdlib.lfx"),
		compiler.Options{BaseDir: root, Resolver: resolver},
	)
	if err != nil {
		t.Fatalf("compile noise_stdlib: %v", err)
	}

	params, _ := runtime.Bind(result.IR.Params, nil)
	layout := makeParityLayout()
	ev := cpu.NewEvaluator(result.IR)

	var first []float32
	for run := 0; run < 3; run++ {
		vals, err := ev.SamplePoint(layout, 0, 0.5, params)
		if err != nil {
			t.Fatalf("sample run %d: %v", run, err)
		}
		if run == 0 {
			first = vals
		} else {
			for i := range first {
				if math.Abs(float64(vals[i]-first[i])) > cpuParityTol {
					t.Fatalf("run %d channel %d: %f vs %f", run, i, vals[i], first[i])
				}
			}
		}
	}
}

func TestCPUVectorHeavyEffectIsConsistent(t *testing.T) {
	result := compileEffect(t, `module "effects/parity"
effect "parity"
output scalar
function sample(width, height, x, y, index, phase, params)
  pos = normalize(vec2(x, y) + 1.0)
  shifted = pos + vec2(phase, phase)
  d = dot(shifted, vec2(0.5, 0.5))
  return clamp(d, 0.0, 1.0)
end
`)
	params, _ := runtime.Bind(result.IR.Params, nil)
	layout := makeParityLayout()
	ev := cpu.NewEvaluator(result.IR)

	var first []float32
	for run := 0; run < 3; run++ {
		vals, err := ev.SamplePoint(layout, 2, 0.7, params)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if run == 0 {
			first = vals
		} else {
			for i := range first {
				if math.Abs(float64(vals[i]-first[i])) > cpuParityTol {
					t.Fatalf("run %d channel %d: %f vs %f", run, i, vals[i], first[i])
				}
			}
		}
	}
}

func TestCPUSamplingAtEdgePhasesIsConsistent(t *testing.T) {
	result := compileEffect(t, `module "effects/parity"
effect "parity"
output scalar
function sample(width, height, x, y, index, phase, params)
  return clamp(phase + x / width, 0.0, 1.0)
end
`)
	result.IR.Timeline = &ir.TimelineSpec{
		LoopStart: float64ptr(0.25),
		LoopEnd:   float64ptr(0.75),
	}

	params, _ := runtime.Bind(result.IR.Params, nil)
	layout := makeParityLayout()
	ev := cpu.NewEvaluator(result.IR)

	edgePhases := []float64{0.0, 1.0, 0.5, 0.25, 0.75}
	for _, ph := range edgePhases {
		t.Run("phase="+formatPhase(ph), func(t *testing.T) {
			v1, err := ev.SamplePoint(layout, 0, float32(ph), params)
			if err != nil {
				t.Fatalf("sample 1: %v", err)
			}
			v2, err := ev.SamplePoint(layout, 0, float32(ph), params)
			if err != nil {
				t.Fatalf("sample 2: %v", err)
			}
			for i := range v1 {
				if math.Abs(float64(v1[i]-v2[i])) > cpuParityTol {
					t.Fatalf("channel %d non-deterministic at phase %f: %f vs %f", i, ph, v1[i], v2[i])
				}
			}
		})
	}
}

func TestCPUSamplingWithFractionalCoordinatesIsConsistent(t *testing.T) {
	result := compileEffect(t, `module "effects/parity"
effect "parity"
output scalar
function sample(width, height, x, y, index, phase, params)
  nx = x / width
  ny = y / height
  return abs(nx - ny) * phase
end
`)
	params, _ := runtime.Bind(result.IR.Params, nil)
	layout := runtime.Layout{
		Width:  10,
		Height: 10,
		Points: []runtime.Point{
			{Index: 0, X: 0.5, Y: 0.5},
			{Index: 1, X: -0.5, Y: 2.5},
			{Index: 2, X: 9.9, Y: 0.1},
		},
	}
	ev := cpu.NewEvaluator(result.IR)

	//nolint:gosec // deterministic RNG for test coverage
	r := rand.New(rand.NewSource(42))
	for range 20 {
		pointIdx := r.Intn(len(layout.Points))
		phase := float32(r.Float64())
		v1, err := ev.SamplePoint(layout, pointIdx, phase, params)
		if err != nil {
			t.Fatalf("sample 1 point %d phase %f: %v", pointIdx, phase, err)
		}
		v2, err := ev.SamplePoint(layout, pointIdx, phase, params)
		if err != nil {
			t.Fatalf("sample 2 point %d phase %f: %v", pointIdx, phase, err)
		}
		for i := range v1 {
			if math.Abs(float64(v1[i]-v2[i])) > cpuParityTol {
				t.Fatalf("point %d phase %f channel %d: %f vs %f", pointIdx, phase, i, v1[i], v2[i])
			}
		}
	}
}

// ── Additional: WGSL output is structurally valid for all example effects ─────

func TestWGSLEmitForAllExampleEffectsIsStructurallyValid(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))

	entries, err := os.ReadDir(filepath.Join(root, "effects"))
	if err != nil {
		t.Fatalf("read effects dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lfx") {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), ".lfx"), func(t *testing.T) {
			result, err := compiler.CompileFile(
				filepath.Join(root, "effects", entry.Name()),
				compiler.Options{BaseDir: root, Resolver: resolver},
			)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			wgslSrc, err := wgsl.Emit(result.IR)
			if err != nil {
				t.Fatalf("emit wgsl: %v", err)
			}
			if !strings.Contains(wgslSrc, "fn lfx_sample(") {
				t.Fatalf("missing lfx_sample in wgsl:\n%s", wgslSrc)
			}
			if strings.Contains(wgslSrc, "unknown") {
				t.Fatalf("wgsl contains 'unknown' placeholder:\n%s", wgslSrc)
			}
		})
	}
}

// helpers ─────────────────────────────────────────────────────────────────────

func float64ptr(v float64) *float64 { return &v }

func formatPhase(ph float64) string {
	switch ph {
	case 0.0:
		return "0"
	case 1.0:
		return "1"
	case 0.5:
		return "0.5"
	case 0.25:
		return "0.25"
	case 0.75:
		return "0.75"
	}
	return "other"
}
