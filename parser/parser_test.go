package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/parser"
)

func TestParseFillIrisExample(t *testing.T) {
	sourcePath := filepath.Join("..", "effects", "fill_iris.lfx")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	mod, err := parser.Parse(string(source))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if mod.ModPath != "effects/fill_iris" {
		t.Fatalf("unexpected module path %q", mod.ModPath)
	}
	if len(mod.Imports) != 0 {
		t.Fatalf("expected 0 imports, got %d", len(mod.Imports))
	}
	if mod.Params == nil || len(mod.Params.Params) != 2 {
		t.Fatalf("expected 2 params, got %#v", mod.Params)
	}
	if mod.Timeline == nil {
		t.Fatal("expected timeline block")
	}

	foundSample := false
	for _, fn := range mod.Funcs {
		if fn.Name == "sample" {
			foundSample = true
			if len(fn.Params) != 7 {
				t.Fatalf("sample param count = %d, want 7", len(fn.Params))
			}
		}
	}
	if !foundSample {
		t.Fatal("sample function missing")
	}
}

func TestParseBlockSyntaxRegressionCases(t *testing.T) {
	t.Run("rejects missing end for if block", func(t *testing.T) {
		_, err := parser.Parse(`
module "effects/missing_end"
effect "missing_end"
function sample(width, height, x, y, index, phase, params)
  if phase < 0.5 then
    return 0.0
  return 1.0
end
`)
		if err == nil {
			t.Fatal("expected parse error")
		}

		parseErr, ok := err.(*parser.ParseError)
		if !ok {
			t.Fatalf("unexpected error type %T", err)
		}
		if !strings.Contains(parseErr.Msg, "unexpected end of input in block") {
			t.Fatalf("parse error message = %q", parseErr.Msg)
		}
	})

	t.Run("accepts nested if block with comments", func(t *testing.T) {
		mod, err := parser.Parse(`
module "effects/nested_if"
effect "nested_if"
function sample(width, height, x, y, index, phase, params)
  -- branch on the phase
  if phase < 0.5 then
    if x < y then
      return 0.25
    end
  end
  return 1.0
end
`)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}
		if len(mod.Funcs) != 1 {
			t.Fatalf("function count = %d, want 1", len(mod.Funcs))
		}
	})
}

func TestParseOutputDeclarationAndMultiReturn(t *testing.T) {
	mod, err := parser.Parse(`
module "effects/rgb"
effect "RGB"
output rgb
function sample(width, height, x, y, index, phase, params)
  return x, y, phase
end
`)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if mod.Output == nil || mod.Output.Type != parser.OutputRGB {
		t.Fatalf("output = %#v, want rgb", mod.Output)
	}
	ret, ok := mod.Funcs[0].Body[0].(*parser.ReturnStmt)
	if !ok {
		t.Fatalf("expected return stmt, got %T", mod.Funcs[0].Body[0])
	}
	if len(ret.Values) != 3 {
		t.Fatalf("return value count = %d, want 3", len(ret.Values))
	}
}

func TestParseOutputVariants(t *testing.T) {
	cases := map[string]parser.OutputType{
		"scalar": parser.OutputScalar,
		"rgb":    parser.OutputRGB,
		"rgbw":   parser.OutputRGBW,
	}

	for literal, want := range cases {
		t.Run(literal, func(t *testing.T) {
			mod, err := parser.Parse(`
module "effects/output_test"
effect "Output Test"
output ` + literal + `
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
			if err != nil {
				t.Fatalf("parse source: %v", err)
			}
			if mod.Output == nil || mod.Output.Type != want {
				t.Fatalf("output = %#v, want %v", mod.Output, want)
			}
		})
	}
}
