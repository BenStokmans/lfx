package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestCompileForPreviewSuccess(t *testing.T) {
	root := filepath.Clean("..")
	artifact, err := compiler.CompileForPreview(filepath.Join(root, "effects", "fill_iris.lfx"), nil, compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err != nil {
		t.Fatalf("compile for preview: %v", err)
	}

	if artifact.ModulePath != "effects/fill_iris" {
		t.Fatalf("module path = %q", artifact.ModulePath)
	}
	if len(artifact.Params) == 0 {
		t.Fatal("expected params")
	}
	if artifact.Timeline == nil {
		t.Fatal("expected timeline")
	}
	if !strings.Contains(artifact.WGSL, "fn lfx_sample(") {
		t.Fatalf("wgsl missing sample entry:\n%s", artifact.WGSL)
	}
	if artifact.Sampler == nil {
		t.Fatal("expected cpu sampler")
	}
}

func TestCompileForPreviewDiagnostics(t *testing.T) {
	root := filepath.Clean("..")
	_, err := compiler.CompileForPreview(filepath.Join(root, "testdata", "missing_effect.lfx"), nil, compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err == nil {
		t.Fatal("expected preview error")
	}

	previewErr, ok := err.(*compiler.PreviewError)
	if !ok {
		t.Fatalf("unexpected error type %T", err)
	}
	if len(previewErr.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
}

func TestCompileForPreviewReportsSyntaxViolationsAndAcceptsValidSource(t *testing.T) {
	t.Run("invalid syntax returns structured diagnostic", func(t *testing.T) {
		root, filePath := writeTempEffect(t, "missing_then", `module "effects/missing_then"
effect "Missing Then"
function sample(width, height, x, y, index, phase, params)
  if phase < 0.5
    return 0.0
  end
  return 1.0
end
`)

		_, err := compiler.CompileForPreview(filePath, nil, compiler.Options{
			BaseDir:  root,
			Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
		})
		if err == nil {
			t.Fatal("expected preview error")
		}

		previewErr, ok := err.(*compiler.PreviewError)
		if !ok {
			t.Fatalf("unexpected error type %T", err)
		}
		if len(previewErr.Diagnostics) != 1 {
			t.Fatalf("diagnostic count = %d, want 1", len(previewErr.Diagnostics))
		}

		diag := previewErr.Diagnostics[0]
		if diag.Severity != "error" {
			t.Fatalf("severity = %q, want error", diag.Severity)
		}
		if diag.Code != "" {
			t.Fatalf("code = %q, want empty parse diagnostic code", diag.Code)
		}
		if !strings.Contains(diag.Message, "expected then, got return") {
			t.Fatalf("message = %q, want parse error about missing then", diag.Message)
		}
		if diag.Line != 5 || diag.Column != 5 {
			t.Fatalf("position = %d:%d, want 5:5", diag.Line, diag.Column)
		}
		if diag.FilePath != filePath {
			t.Fatalf("file path = %q, want %q", diag.FilePath, filePath)
		}
	})

	t.Run("valid source compiles cleanly", func(t *testing.T) {
		root, filePath := writeTempEffect(t, "with_then", `module "effects/with_then"
effect "With Then"
function sample(width, height, x, y, index, phase, params)
  if phase < 0.5 then
    return 0.0
  end
  return 1.0
end
`)

		artifact, err := compiler.CompileForPreview(filePath, nil, compiler.Options{
			BaseDir:  root,
			Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
		})
		if err != nil {
			t.Fatalf("compile for preview: %v", err)
		}
		if artifact.ModulePath != "effects/with_then" {
			t.Fatalf("module path = %q", artifact.ModulePath)
		}
		if artifact.Diagnostics != nil {
			t.Fatalf("expected nil diagnostics, got %#v", artifact.Diagnostics)
		}
		if !strings.Contains(artifact.WGSL, "fn lfx_sample(") {
			t.Fatalf("wgsl missing sample entry:\n%s", artifact.WGSL)
		}
	})
}

func writeTempEffect(t *testing.T, name, source string) (string, string) {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "effects"), 0o755); err != nil {
		t.Fatalf("create effects dir: %v", err)
	}

	filePath := filepath.Join(root, "effects", name+".lfx")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write effect file: %v", err)
	}

	return root, filePath
}
