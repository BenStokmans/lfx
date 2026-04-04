package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileResolverRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	resolver := NewFileResolver(root)

	if _, err := resolver.Resolve("../secret"); err == nil {
		t.Fatal("expected traversal import to be rejected")
	}
}

func TestFileResolverLoadsOnlyFromConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "safe.lfx"), []byte(`module "safe"`), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	resolver := NewFileResolver(root)
	data, err := resolver.Resolve("safe")
	if err != nil {
		t.Fatalf("resolve safe module: %v", err)
	}
	if string(data) != `module "safe"` {
		t.Fatalf("resolved source = %q", string(data))
	}
}
