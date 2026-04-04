package lower_test

import (
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestLowerFillIrisCompilesWithoutRemovedStdHelpers(t *testing.T) {
	root := filepath.Clean("..")
	result, err := compiler.CompileFile(filepath.Join(root, "effects", "fill_iris.lfx"), compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err != nil {
		t.Fatalf("compile file: %v", err)
	}

	if result.IR.Sample == nil {
		t.Fatal("sample function missing from IR")
	}
	if len(result.IR.Params) != 2 {
		t.Fatalf("param count = %d, want 2", len(result.IR.Params))
	}

	multiLocalDecls := 0
	for _, stmt := range result.IR.Sample.Body {
		if _, ok := stmt.(*ir.MultiLocalDecl); ok {
			multiLocalDecls++
		}
	}
	if multiLocalDecls != 0 {
		t.Fatalf("multi-local decl count = %d, want 0", multiLocalDecls)
	}
}
