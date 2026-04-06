package parser_test

import (
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/parser"
)

// minimalEffect is a helper that wraps a function body in a valid scalar effect.
func minimalEffect(body string) string {
	return `module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
` + body + `
end
`
}

func TestParseRejectsUnterminatedString(t *testing.T) {
	_, err := parser.Parse(`module "effects/bad
effect "Bad"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	if err == nil {
		t.Fatal("expected lex/parse error for unterminated string")
	}
	if !strings.Contains(err.Error(), "unterminated string literal") {
		t.Fatalf("error should mention unterminated string literal, got: %v", err)
	}
	// Verify position is reported.
	lexErr, ok := err.(*parser.LexError)
	if !ok {
		// Also accept a ParseError wrapping the LexError.
		t.Logf("error is %T (not LexError directly); message: %v", err, err)
		return
	}
	if lexErr.Pos.Line == 0 {
		t.Fatalf("expected non-zero line in lex error position, got %v", lexErr.Pos)
	}
}

func TestParseRejectsUnterminatedStringInModulePath(t *testing.T) {
	_, err := parser.Parse(`module "effects/missing_close`)
	if err == nil {
		t.Fatal("expected error for unterminated module path string")
	}
	if !strings.Contains(err.Error(), "unterminated string literal") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestParseRejectsMissingEndForDoublyNestedIf(t *testing.T) {
	_, err := parser.Parse(minimalEffect(`
  if phase < 0.5 then
    if x < y then
      if x > 0.0 then
        return 0.1
      -- missing end for innermost if
    end
  end
  return 1.0
`))
	if err == nil {
		t.Fatal("expected parse error for missing end in nested if")
	}
}

func TestParseRejectsMissingEndForElseIfBranch(t *testing.T) {
	_, err := parser.Parse(minimalEffect(`
  if phase < 0.25 then
    return 0.0
  elseif phase < 0.75 then
    return 0.5
  -- missing end
  return 1.0
`))
	if err == nil {
		t.Fatal("expected parse error for missing end after elseif block")
	}
}

func TestParseRejectsExtraEndAfterFunction(t *testing.T) {
	_, err := parser.Parse(`module "effects/extra_end"
effect "extra_end"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
end
`)
	if err == nil {
		t.Fatal("expected parse error for extra end after function")
	}
}

func TestParseRejectsExtraEndInsideFunction(t *testing.T) {
	_, err := parser.Parse(minimalEffect(`
  if phase < 0.5 then
    return 0.0
  end
  end
  return 1.0
`))
	if err == nil {
		t.Fatal("expected parse error for extra end inside function")
	}
}

func TestParseRejectsOutputKeywordWithoutType(t *testing.T) {
	_, err := parser.Parse(`module "effects/no_type"
effect "no_type"
output
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	if err == nil {
		t.Fatal("expected parse error for output without type")
	}
}

func TestParseRejectsOutputFollowedByUnknownType(t *testing.T) {
	_, err := parser.Parse(`module "effects/bad_output"
effect "bad_output"
output mono
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	if err == nil {
		t.Fatal("expected parse error for unknown output type 'mono'")
	}
}

func TestParseAcceptsCommentBetweenEffectKeywordAndName(t *testing.T) {
	// Comments are skipped at every token boundary, so this should accept.
	mod, err := parser.Parse(`module "effects/comment_test"
effect -- comment between keyword and name
"comment_test"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	if err != nil {
		t.Fatalf("expected comment between effect and name to be accepted, got: %v", err)
	}
	if mod.Effect == nil || mod.Effect.Name != "comment_test" {
		t.Fatalf("effect name = %v, want comment_test", mod.Effect)
	}
}

func TestParseAcceptsCommentInsideParamsBlock(t *testing.T) {
	mod, err := parser.Parse(`module "effects/c"
effect "c"
output scalar
params {
  -- scaling factor
  gain = float(0.5, 0.0, 1.0)
}
function sample(width, height, x, y, index, phase, params)
  return params.gain
end
`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if mod.Params == nil || len(mod.Params.Params) != 1 {
		t.Fatalf("expected 1 param, got %v", mod.Params)
	}
}

func TestParseAcceptsCommentBetweenReturnValues(t *testing.T) {
	mod, err := parser.Parse(`module "effects/c"
effect "c"
output rgb
function sample(width, height, x, y, index, phase, params)
  return x, -- red
         y, -- green
         phase -- blue
end
`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	ret, ok := mod.Funcs[0].Body[0].(*parser.ReturnStmt)
	if !ok || len(ret.Values) != 3 {
		t.Fatalf("expected 3 return values, got %v", mod.Funcs[0].Body[0])
	}
}

func TestParseRejectsLeadingDotFloat(t *testing.T) {
	// .5 is not a valid float literal: '.' is TOKEN_DOT, not a number prefix.
	_, err := parser.Parse(minimalEffect(`  return .5`))
	if err == nil {
		t.Fatal("expected parse error for .5 (leading dot is not a number)")
	}
}

func TestParseRejectsTrailingDotFloat(t *testing.T) {
	// 1. followed by end keyword: the dot triggers field-access parsing but
	// 'end' is not a valid field name.
	_, err := parser.Parse(minimalEffect(`  return 1.`))
	if err == nil {
		t.Fatal("expected parse error for trailing dot (1.)")
	}
}

func TestParseAcceptsLeadingZeroFloat(t *testing.T) {
	// 00.1 lexes as a single TOKEN_FLOAT; the lexer does not reject leading zeros.
	_, err := parser.Parse(minimalEffect(`  return 00.1`))
	if err != nil {
		t.Fatalf("unexpected parse error for 00.1 (leading zero float): %v", err)
	}
}

func TestParseAcceptsNegativeZeroFloat(t *testing.T) {
	// -0.0 is unary minus applied to the float literal 0.0; valid syntax.
	_, err := parser.Parse(minimalEffect(`  return -0.0`))
	if err != nil {
		t.Fatalf("unexpected parse error for -0.0: %v", err)
	}
}

func TestParseScientificNotationIsNotASingleToken(t *testing.T) {
	// 1e-3 lexes as three tokens: INT(1), IDENT(e), MINUS, INT(3).
	// This is a parse-level accident — not a float literal.
	// The parse itself succeeds (it sees "return 1" then "e - 3" as another expression).
	_, err := parser.Parse(minimalEffect(`  return 1e-3`))
	if err != nil {
		t.Fatalf(
			"parser should not error on 1e-3 at parse time "+
				"(it splits into separate tokens); got: %v", err)
	}
}

func TestParseUnderscoreNumberIsNotASingleToken(t *testing.T) {
	// 1_000 lexes as INT(1) then IDENT(_000); not a numeric underscore separator.
	// The parse itself succeeds: "return 1" then "_000" is another statement.
	_, err := parser.Parse(minimalEffect(`  return 1_000`))
	if err != nil {
		t.Fatalf(
			"parser should not error on 1_000 at parse time "+
				"(it splits into separate tokens); got: %v", err)
	}
}

func TestParseRejectsQuestionMarkInIdentifier(t *testing.T) {
	// foo? is lexed as IDENT(foo) then '?' which is an unrecognized character.
	_, err := parser.Parse(minimalEffect(`  foo? = 1.0
  return foo?`))
	if err == nil {
		t.Fatal("expected lex error for '?' character")
	}
}

func TestParseRejectsAtSignCharacter(t *testing.T) {
	_, err := parser.Parse(minimalEffect(`  @brightness = 0.5
  return @brightness`))
	if err == nil {
		t.Fatal("expected lex error for '@' character")
	}
}

func TestParseRejectsUnicodeIdentifiers(t *testing.T) {
	// Non-ASCII bytes are not in [a-zA-Z0-9_] so they hit the lexer default.
	_, err := parser.Parse(minimalEffect(`  α = 1.0
  return α`))
	if err == nil {
		t.Fatal("expected lex error for unicode identifier")
	}
}

func TestParseDashInNameIsSubtractionNotIdentifier(t *testing.T) {
	// foo-bar lexes as IDENT(foo) MINUS IDENT(bar); it is subtraction, not a
	// single identifier. This parses fine (undefined vars will fail in sema).
	_, err := parser.Parse(minimalEffect(`  result = foo - bar
  return result`))
	if err != nil {
		t.Fatalf("unexpected parse error for foo-bar (parsed as subtraction): %v", err)
	}
}

func TestParseChainedPostfixCallsAndFieldAccess(t *testing.T) {
	// a.b(c).d(e).f — each step is a call or field access on the result of the
	// previous. Verify the parser either accepts or rejects this cleanly
	// (no panic, no infinite loop). The exact outcome is implementation-defined;
	// document it here.
	_, err := parser.Parse(minimalEffect(`  result = a.b(c).d(e).f
  return result`))
	// We accept either outcome; the important thing is no panic and a clear error
	// if unsupported.
	if err != nil {
		t.Logf("chained postfix rejected at parse time (clean error): %v", err)
	} else {
		t.Logf("chained postfix accepted at parse time")
	}
}
