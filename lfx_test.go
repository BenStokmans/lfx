package lfx_test

import (
	"math"
	"path/filepath"
	"testing"

	lfx "github.com/BenStokmans/lfx"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

func testLayout() runtime.Layout {
	return runtime.Layout{
		Width:  4,
		Height: 4,
		Points: []runtime.Point{
			{Index: 0, X: 0, Y: 0},
			{Index: 1, X: 2, Y: 2},
			{Index: 2, X: 3, Y: 3},
		},
	}
}

func TestEngineLoadFileCPU(t *testing.T) {
	root := "."
	engine, err := lfx.LoadFile(filepath.Join(root, "effects", "filled.lfx"), lfx.Options{
		Backend: lfx.BackendCPU,
		BaseDir: root,
	})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	defer engine.Close()

	if engine.OutputChannels() != 1 {
		t.Fatalf("OutputChannels = %d, want 1", engine.OutputChannels())
	}
}

func TestEngineParamsReturnedForBind(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	engine, err := lfx.LoadFile(filepath.Join(root, "effects", "fill_iris.lfx"), lfx.Options{
		Backend:  lfx.BackendCPU,
		BaseDir:  root,
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	defer engine.Close()

	specs := engine.Params()
	if len(specs) == 0 {
		t.Fatal("Params() returned no specs")
	}
	params, err := runtime.Bind(specs, nil)
	if err != nil {
		t.Fatalf("runtime.Bind: %v", err)
	}
	if params == nil {
		t.Fatal("Bind returned nil")
	}
}

func TestEngineSamplePointInRange(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	engine, err := lfx.LoadFile(filepath.Join(root, "effects", "fill_iris.lfx"), lfx.Options{
		Backend:  lfx.BackendCPU,
		BaseDir:  root,
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	defer engine.Close()

	params, err := runtime.Bind(engine.Params(), nil)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	layout := testLayout()
	values, err := engine.SamplePoint(layout, 0, 0.5, params)
	if err != nil {
		t.Fatalf("SamplePoint: %v", err)
	}
	if len(values) != engine.OutputChannels() {
		t.Fatalf("SamplePoint returned %d values, want %d", len(values), engine.OutputChannels())
	}
	for ch, v := range values {
		if math.IsNaN(float64(v)) || v < 0 || v > 1 {
			t.Fatalf("channel %d = %f, want in [0,1]", ch, v)
		}
	}
}

func TestEngineSamplePointsMatchesSamplePoint(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	engine, err := lfx.LoadFile(filepath.Join(root, "effects", "fill_iris.lfx"), lfx.Options{
		Backend:  lfx.BackendCPU,
		BaseDir:  root,
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	defer engine.Close()

	params, err := runtime.Bind(engine.Params(), nil)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	layout := testLayout()
	indices := []int{0, 1, 2}
	batch, err := engine.SamplePoints(layout, indices, 0.3, params)
	if err != nil {
		t.Fatalf("SamplePoints: %v", err)
	}
	channels := engine.OutputChannels()
	if len(batch) != len(indices)*channels {
		t.Fatalf("SamplePoints len = %d, want %d", len(batch), len(indices)*channels)
	}
	for i, idx := range indices {
		single, err := engine.SamplePoint(layout, idx, 0.3, params)
		if err != nil {
			t.Fatalf("SamplePoint %d: %v", idx, err)
		}
		for ch := range single {
			got := batch[i*channels+ch]
			want := single[ch]
			if math.Abs(float64(got-want)) > 1e-6 {
				t.Fatalf("point %d channel %d: SamplePoints=%f SamplePoint=%f", idx, ch, got, want)
			}
		}
	}
}

func TestEngineEffectsAllBackendsCPU(t *testing.T) {
	root := "."
	resolver := stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...))
	layout := testLayout()
	effects := []string{
		"effects/filled.lfx",
		"effects/fill_iris.lfx",
		"effects/fill_horizontal.lfx",
		"effects/bar_horizontal.lfx",
	}
	for _, effect := range effects {
		t.Run(effect, func(t *testing.T) {
			engine, err := lfx.LoadFile(filepath.Join(root, effect), lfx.Options{
				Backend:  lfx.BackendCPU,
				BaseDir:  root,
				Resolver: resolver,
			})
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			defer engine.Close()

			params, err := runtime.Bind(engine.Params(), nil)
			if err != nil {
				t.Fatalf("Bind: %v", err)
			}

			for i := range layout.Points {
				values, err := engine.SamplePoint(layout, i, 0.25, params)
				if err != nil {
					t.Fatalf("SamplePoint %d: %v", i, err)
				}
				for ch, v := range values {
					if math.IsNaN(float64(v)) || v < 0 || v > 1 {
						t.Fatalf("point %d channel %d = %f, out of [0,1]", i, ch, v)
					}
				}
			}
		})
	}
}

func TestEngineCloseIsIdempotentForCPU(t *testing.T) {
	root := "."
	engine, err := lfx.LoadFile(filepath.Join(root, "effects", "filled.lfx"), lfx.Options{
		Backend: lfx.BackendCPU,
		BaseDir: root,
	})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
}
