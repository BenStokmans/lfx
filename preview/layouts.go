package main

import (
	"fmt"
	"os"

	lfxruntime "github.com/BenStokmans/lfx/runtime"
)

func builtInLayouts() []LayoutData {
	return []LayoutData{
		layoutFromRuntime("grid-8x8", "Grid 8x8", "", true, gridLayout(8, 8, 1)),
		layoutFromRuntime("grid-32x32", "Grid 32x32", "", true, gridLayout(32, 32, 1)),
		layoutFromRuntime("line-32", "Line 32", "", true, lineLayout(32, 1)),
		layoutFromRuntime("diamond-25", "Diamond 25", "", true, diamondLayout(5, 1)),
	}
}

func loadLayoutFile(path string) (LayoutData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LayoutData{}, fmt.Errorf("read layout %s: %w", path, err)
	}
	layout, err := lfxruntime.ParseLayoutJSON(data)
	if err != nil {
		return LayoutData{}, fmt.Errorf("parse layout %s: %w", path, err)
	}
	return layoutFromRuntime(path, layoutNameFromPath(path), path, false, layout), nil
}

func toRuntimeLayout(layout LayoutData) lfxruntime.Layout {
	return lfxruntime.Layout{
		Width:  layout.Width,
		Height: layout.Height,
		Points: append([]lfxruntime.Point(nil), layout.Points...),
	}
}

func layoutFromRuntime(id, name, path string, builtIn bool, layout lfxruntime.Layout) LayoutData {
	return LayoutData{
		ID:      id,
		Name:    name,
		BuiltIn: builtIn,
		Path:    path,
		Width:   layout.Width,
		Height:  layout.Height,
		Points:  append([]lfxruntime.Point(nil), layout.Points...),
	}
}

func lineLayout(count int, spacing float32) lfxruntime.Layout {
	points := make([]lfxruntime.Point, 0, count)
	for i := range count {
		points = append(points, lfxruntime.Point{
			Index: uint32(i),
			X:     float32(i) * spacing,
			Y:     0,
		})
	}
	return lfxruntime.Layout{
		Width:  float32(max(1, count)),
		Height: 1,
		Points: points,
	}
}

func gridLayout(width, height int, spacing float32) lfxruntime.Layout {
	points := make([]lfxruntime.Point, 0, width*height)
	index := 0
	for y := range height {
		for x := range width {
			points = append(points, lfxruntime.Point{
				Index: uint32(index),
				X:     float32(x) * spacing,
				Y:     float32(y) * spacing,
			})
			index++
		}
	}
	return lfxruntime.Layout{
		Width:  float32(max(1, width)),
		Height: float32(max(1, height)),
		Points: points,
	}
}

func diamondLayout(radius int, spacing float32) lfxruntime.Layout {
	points := make([]lfxruntime.Point, 0, radius*radius)
	index := 0
	for y := -radius + 1; y < radius; y++ {
		for x := -radius + 1; x < radius; x++ {
			if absInt(x)+absInt(y) >= radius {
				continue
			}
			points = append(points, lfxruntime.Point{
				Index: uint32(index),
				X:     float32(x+radius-1) * spacing,
				Y:     float32(y+radius-1) * spacing,
			})
			index++
		}
	}
	return lfxruntime.Layout{
		Width:  float32(max(1, radius*2-1)),
		Height: float32(max(1, radius*2-1)),
		Points: points,
	}
}

func layoutNameFromPath(path string) string {
	base := path
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '/' {
			base = base[i+1:]
			break
		}
	}
	if len(base) > 5 && base[len(base)-5:] == ".json" {
		return base[:len(base)-5]
	}
	return base
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
