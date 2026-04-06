package cpu_test

import (
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

var benchmarkSink float32

func BenchmarkEvaluatorSampleAll(b *testing.B) {
	root := filepath.Clean(filepath.Join("..", ".."))

	benchmarks := []struct {
		name   string
		effect string
		layout runtime.Layout
	}{
		{name: "FillIris_64x64", effect: "fill_iris.lfx", layout: makeGridLayout(64, 64)},
		{name: "ChromaBloom_64x64", effect: "chroma_bloom.lfx", layout: makeGridLayout(64, 64)},
		{name: "Perlin501_64x64", effect: "perlin_501.lfx", layout: makeGridLayout(64, 64)},
	}

	for _, tc := range benchmarks {
		b.Run(tc.name, func(b *testing.B) {
			result, err := compiler.CompileFile(filepath.Join(root, "effects", tc.effect), compiler.Options{
				BaseDir:  root,
				Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
			})
			if err != nil {
				b.Fatalf("compile file: %v", err)
			}

			params, err := runtime.Bind(result.IR.Params, nil)
			if err != nil {
				b.Fatalf("bind params: %v", err)
			}

			evaluator := cpu.NewEvaluator(result.IR)
			pointCount := len(tc.layout.Points)
			channels := result.IR.Output.Channels()
			b.ReportAllocs()
			b.ReportMetric(float64(pointCount), "points/op")
			b.ReportMetric(float64(pointCount*channels), "samples/op")

			var sink float32
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for pointIndex := range tc.layout.Points {
					values, err := evaluator.SamplePoint(tc.layout, pointIndex, 0.37, params)
					if err != nil {
						b.Fatalf("sample point %d: %v", pointIndex, err)
					}
					sink += values[0]
				}
			}
			b.StopTimer()
			benchmarkSink = sink
		})
	}
}

func BenchmarkEvaluatorSamplePoints(b *testing.B) {
	root := filepath.Clean(filepath.Join("..", ".."))

	benchmarks := []struct {
		name   string
		effect string
		layout runtime.Layout
	}{
		{name: "FillIris_64x64", effect: "fill_iris.lfx", layout: makeGridLayout(64, 64)},
		{name: "ChromaBloom_64x64", effect: "chroma_bloom.lfx", layout: makeGridLayout(64, 64)},
		{name: "Perlin501_64x64", effect: "perlin_501.lfx", layout: makeGridLayout(64, 64)},
	}

	for _, tc := range benchmarks {
		b.Run(tc.name, func(b *testing.B) {
			result, err := compiler.CompileFile(filepath.Join(root, "effects", tc.effect), compiler.Options{
				BaseDir:  root,
				Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
			})
			if err != nil {
				b.Fatalf("compile file: %v", err)
			}

			params, err := runtime.Bind(result.IR.Params, nil)
			if err != nil {
				b.Fatalf("bind params: %v", err)
			}

			evaluator := cpu.NewEvaluator(result.IR)
			pointIndices := make([]int, len(tc.layout.Points))
			for idx := range pointIndices {
				pointIndices[idx] = idx
			}
			pointCount := len(tc.layout.Points)
			channels := result.IR.Output.Channels()
			b.ReportAllocs()
			b.ReportMetric(float64(pointCount), "points/op")
			b.ReportMetric(float64(pointCount*channels), "samples/op")

			var sink float32
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				values, err := evaluator.SamplePoints(tc.layout, pointIndices, 0.37, params)
				if err != nil {
					b.Fatalf("sample points: %v", err)
				}
				sink += values[0]
			}
			b.StopTimer()
			benchmarkSink = sink
		})
	}
}

//nolint:unparam // keep signature for future benchmark variants
func makeGridLayout(width, height int) runtime.Layout {
	points := make([]runtime.Point, 0, width*height)
	index := uint32(0)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			points = append(points, runtime.Point{
				Index: index,
				X:     float32(x),
				Y:     float32(y),
			})
			index++
		}
	}
	return runtime.Layout{
		Width:  float32(width),
		Height: float32(height),
		Points: points,
	}
}
