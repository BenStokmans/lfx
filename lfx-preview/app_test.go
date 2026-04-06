package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSourceFileRejectsOutsideWorkspace(t *testing.T) {
	app := NewApp()
	root := t.TempDir()
	if _, err := app.LoadWorkspace(root); err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.lfx")
	if _, err := app.ReadSourceFile(outside); err == nil {
		t.Fatal("expected outside read to be rejected")
	}
}

func TestSaveSourceFileRejectsOutsideWorkspace(t *testing.T) {
	app := NewApp()
	root := t.TempDir()
	if _, err := app.LoadWorkspace(root); err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.lfx")
	err := app.SaveSourceFile(SaveSourceRequest{Path: outside, Content: `module "outside"`})
	if err == nil {
		t.Fatal("expected outside write to be rejected")
	}
}

func TestCompilePreviewRejectsFileOutsideWorkspace(t *testing.T) {
	app := NewApp()
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "effect.lfx")
	if err := os.WriteFile(outsideFile, []byte(`module "effects/outside"`), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	err := ensurePathInRoot(workspaceRoot, outsideFile, false)
	if err == nil {
		t.Fatal("expected helper to reject outside file")
	}

	_, err = app.CompilePreview(CompileRequest{
		WorkspaceRoot: workspaceRoot,
		FilePath:      outsideFile,
	})
	if err == nil {
		t.Fatal("expected compile outside workspace to be rejected")
	}
	if !strings.Contains(err.Error(), "outside workspace root") {
		t.Fatalf("error = %v, want outside workspace root", err)
	}
}
