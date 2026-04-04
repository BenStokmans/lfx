//go:build !cgo

package main

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestResolveComputeEntryPointMetalEscapesReservedMain(t *testing.T) {
	got, err := resolveComputeEntryPoint(`
@compute @workgroup_size(64)
fn main() {}
`, gputypes.BackendMetal, "main")
	if err != nil {
		t.Fatalf("resolveComputeEntryPoint() error = %v", err)
	}
	if got != "main_" {
		t.Fatalf("resolveComputeEntryPoint() = %q, want %q", got, "main_")
	}
}

func TestResolveComputeEntryPointNonMetalKeepsOriginalName(t *testing.T) {
	got, err := resolveComputeEntryPoint(`
@compute @workgroup_size(64)
fn main() {}
`, gputypes.BackendVulkan, "main")
	if err != nil {
		t.Fatalf("resolveComputeEntryPoint() error = %v", err)
	}
	if got != "main" {
		t.Fatalf("resolveComputeEntryPoint() = %q, want %q", got, "main")
	}
}

func TestParseGridSizes(t *testing.T) {
	got, err := parseGridSizes("8, 32,128")
	if err != nil {
		t.Fatalf("parseGridSizes() error = %v", err)
	}
	want := []int{8, 32, 128}
	if len(got) != len(want) {
		t.Fatalf("parseGridSizes() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseGridSizes()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestGenerateGridLayout(t *testing.T) {
	layout := generateGridLayout(3, 2)
	if layout.Width != 3 || layout.Height != 2 {
		t.Fatalf("generateGridLayout() dimensions = %vx%v, want 3x2", layout.Width, layout.Height)
	}
	if len(layout.Points) != 6 {
		t.Fatalf("generateGridLayout() point count = %d, want 6", len(layout.Points))
	}
	last := layout.Points[len(layout.Points)-1]
	if last.Index != 5 || last.X != 2 || last.Y != 1 {
		t.Fatalf("generateGridLayout() last point = %+v, want index=5 x=2 y=1", last)
	}
}
