package lfx_test

import (
	"os"
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
				values, err := evaluator.SamplePoint(layout, i, 0.25, params)
				if err != nil {
					t.Fatalf("cpu sample point %d: %v", i, err)
				}
				for channel, value := range values {
					if value < 0 || value > 1 {
						t.Fatalf("cpu sample point %d channel %d out of range: %f", i, channel, value)
					}
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

func TestRGBAndRGBWEffectsCompileAcrossBackends(t *testing.T) {
	cases := []struct {
		name       string
		output     string
		returns    string
		channels   int
		wgslNeedle string
	}{
		{name: "rgb", output: "output rgb", returns: "return x, y, phase", channels: 3, wgslNeedle: "vec3<f32>"},
		{name: "rgbw", output: "output rgbw", returns: "return x, y, phase, 0.25", channels: 4, wgslNeedle: "vec4<f32>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			effectsDir := filepath.Join(root, "effects")
			//nolint:gosec
			if err := os.MkdirAll(effectsDir, 0o755); err != nil {
				t.Fatalf("create effects dir: %v", err)
			}
			filePath := filepath.Join(effectsDir, tc.name+".lfx")
			source := `module "effects/` + tc.name + `"
effect "` + tc.name + `"
` + tc.output + `
function sample(width, height, x, y, index, phase, params)
  ` + tc.returns + `
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
			layout := runtime.Layout{Width: 2, Height: 2, Points: []runtime.Point{{Index: 0, X: 0.2, Y: 0.4}}}
			values, err := cpu.NewEvaluator(result.IR).SamplePoint(layout, 0, 0.7, params)
			if err != nil {
				t.Fatalf("sample point: %v", err)
			}
			if len(values) != tc.channels {
				t.Fatalf("value count = %d, want %d", len(values), tc.channels)
			}
			wgslSource, err := wgsl.Emit(result.IR)
			if err != nil {
				t.Fatalf("emit wgsl: %v", err)
			}
			if !strings.Contains(wgslSource, tc.wgslNeedle) {
				t.Fatalf("wgsl output missing %q:\n%s", tc.wgslNeedle, wgslSource)
			}
		})
	}
}
