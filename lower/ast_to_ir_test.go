package lower_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
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

func TestCompileFileRejectsUnsupportedMultiAssignment(t *testing.T) {
	root := t.TempDir()
	effectsDir := filepath.Join(root, "effects")
	if err := os.MkdirAll(effectsDir, 0o750); err != nil {
		t.Fatalf("create effects dir: %v", err)
	}

	filePath := filepath.Join(effectsDir, "bad_multi_assign.lfx")
	source := `module "effects/bad_multi_assign"
effect "Bad Multi Assign"

function sample(width, height, x, y, index, phase, params)
  a, b = 1.0, 2.0
  return a
end
`
	if err := os.WriteFile(filePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write effect file: %v", err)
	}

	_, err := compiler.CompileFile(filePath, compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
	})
	if err == nil {
		t.Fatal("expected compile to fail for unsupported multi-assignment")
	}
}

func TestLowerPreservesVectorTypes(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/vector_lower"
effect "Vector Lower"
output rgb
function tint(pos)
  return vec3(pos.x, pos.y, 0.25)
end
function sample(width, height, x, y, index, phase, params)
  pos = normalize(vec2(x, y) + 1.0)
  return tint(pos)
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	info, errs, warns := sema.AnalyzeModule(mod, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected semantic errors: %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected semantic warnings: %v", warns)
	}

	irmod, err := lower.Lower(mod, nil, info, nil)
	if err != nil {
		t.Fatalf("lower source: %v", err)
	}

	if irmod.Sample == nil {
		t.Fatal("sample function missing")
	}
	if irmod.Sample.Body[len(irmod.Sample.Body)-1].(*ir.Return).Values[0].ResultType() != ir.TypeVec3 {
		t.Fatalf("sample return type = %s, want vec3", irmod.Sample.Body[len(irmod.Sample.Body)-1].(*ir.Return).Values[0].ResultType())
	}

	var tint *ir.Function
	for _, fn := range irmod.Functions {
		if fn.Name == "tint" {
			tint = fn
			break
		}
	}
	if tint == nil {
		t.Fatal("tint function missing")
	}
	if tint.Params[0].Type != ir.TypeVec2 {
		t.Fatalf("tint param type = %s, want vec2", tint.Params[0].Type)
	}
	if tint.ReturnType != ir.TypeVec3 {
		t.Fatalf("tint return type = %s, want vec3", tint.ReturnType)
	}
}
