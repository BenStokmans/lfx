package lfx_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestPerlin501WGSLDoesNotReferenceBareParamsValue(t *testing.T) {
	root := "."
	artifact, err := compiler.CompileForPreview(filepath.Join(root, "effects", "perlin_501.lfx"), nil, compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err != nil {
		t.Fatalf("compile preview: %v", err)
	}

	if !strings.Contains(artifact.WGSL, "fn lfx_sample(width: f32, height: f32, x: f32, y: f32, index: f32, phase: f32, params: f32) -> f32 {") {
		t.Fatalf("wgsl should expose a dummy params slot for helper passthrough:\n%s", artifact.WGSL)
	}
	if !strings.Contains(artifact.WGSL, "var result = lfx_sample(uniforms.width, uniforms.height, pt.x, pt.y, f32(pt.index), uniforms.phase, 0.0);") {
		t.Fatalf("entrypoint should pass dummy params sentinel into sample:\n%s", artifact.WGSL)
	}
	if strings.Contains(artifact.WGSL, "lfx_voronoi") || strings.Contains(artifact.WGSL, "lfx_worley") {
		t.Fatalf("perlin-only effect should not emit unused voronoi/worley helpers:\n%s", artifact.WGSL)
	}
	if strings.Contains(artifact.WGSL, "var loop:") || strings.Contains(artifact.WGSL, " loop: f32") {
		t.Fatalf("wgsl should sanitize reserved keyword identifiers like loop:\n%s", artifact.WGSL)
	}
	if strings.Contains(artifact.WGSL, "var total_frame_count: f32 = 0.0;") &&
		strings.Contains(artifact.WGSL, "var loop_frame_count: f32 = 0.0;") &&
		strings.Contains(artifact.WGSL, "var wrapped_frame: f32 = 0.0;") {
		return
	}
	t.Fatalf("wgsl should preserve the updated non-reserved frame variable names:\n%s", artifact.WGSL)
}
