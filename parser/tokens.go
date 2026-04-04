package parser

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Special tokens
	TOKEN_EOF     TokenType = iota
	TOKEN_ILLEGAL           // unrecognized character or malformed token

	// Literals
	TOKEN_INT    // integer literal
	TOKEN_FLOAT  // floating-point literal
	TOKEN_STRING // double-quoted string literal
	TOKEN_IDENT  // identifier

	// Keywords
	TOKEN_VERSION
	TOKEN_MODULE
	TOKEN_LIBRARY
	TOKEN_IMPORT
	TOKEN_AS
	TOKEN_EXPORT
	TOKEN_EFFECT
	TOKEN_OUTPUT
	TOKEN_PARAMS
	TOKEN_TIMELINE
	TOKEN_FUNCTION
	TOKEN_END
	TOKEN_IF
	TOKEN_THEN
	TOKEN_ELSEIF
	TOKEN_ELSE
	TOKEN_RETURN
	TOKEN_AND
	TOKEN_OR
	TOKEN_NOT
	TOKEN_TRUE
	TOKEN_FALSE

	// Operators
	TOKEN_PLUS    // +
	TOKEN_MINUS   // -
	TOKEN_STAR    // *
	TOKEN_SLASH   // /
	TOKEN_PERCENT // %
	TOKEN_EQ      // ==
	TOKEN_NEQ     // ~=
	TOKEN_LT      // <
	TOKEN_GT      // >
	TOKEN_LTE     // <=
	TOKEN_GTE     // >=
	TOKEN_ASSIGN  // =

	// Punctuation
	TOKEN_LPAREN // (
	TOKEN_RPAREN // )
	TOKEN_LBRACE // {
	TOKEN_RBRACE // }
	TOKEN_COMMA  // ,
	TOKEN_DOT    // .

	// Comment (retained for tooling; typically skipped by the parser)
	TOKEN_COMMENT // -- single line comment
)

// tokenNames maps each TokenType to its human-readable name.
var tokenNames = map[TokenType]string{
	TOKEN_EOF:     "EOF",
	TOKEN_ILLEGAL: "ILLEGAL",

	TOKEN_INT:    "INT",
	TOKEN_FLOAT:  "FLOAT",
	TOKEN_STRING: "STRING",
	TOKEN_IDENT:  "IDENT",

	TOKEN_VERSION:  "version",
	TOKEN_MODULE:   "module",
	TOKEN_LIBRARY:  "library",
	TOKEN_IMPORT:   "import",
	TOKEN_AS:       "as",
	TOKEN_EXPORT:   "export",
	TOKEN_EFFECT:   "effect",
	TOKEN_OUTPUT:   "output",
	TOKEN_PARAMS:   "params",
	TOKEN_TIMELINE: "timeline",
	TOKEN_FUNCTION: "function",
	TOKEN_END:      "end",
	TOKEN_IF:       "if",
	TOKEN_THEN:     "then",
	TOKEN_ELSEIF:   "elseif",
	TOKEN_ELSE:     "else",
	TOKEN_RETURN:   "return",
	TOKEN_AND:      "and",
	TOKEN_OR:       "or",
	TOKEN_NOT:      "not",
	TOKEN_TRUE:     "true",
	TOKEN_FALSE:    "false",

	TOKEN_PLUS:    "+",
	TOKEN_MINUS:   "-",
	TOKEN_STAR:    "*",
	TOKEN_SLASH:   "/",
	TOKEN_PERCENT: "%",
	TOKEN_EQ:      "==",
	TOKEN_NEQ:     "~=",
	TOKEN_LT:      "<",
	TOKEN_GT:      ">",
	TOKEN_LTE:     "<=",
	TOKEN_GTE:     ">=",
	TOKEN_ASSIGN:  "=",

	TOKEN_LPAREN: "(",
	TOKEN_RPAREN: ")",
	TOKEN_LBRACE: "{",
	TOKEN_RBRACE: "}",
	TOKEN_COMMA:  ",",
	TOKEN_DOT:    ".",

	TOKEN_COMMENT: "COMMENT",
}

// String returns the human-readable name of the token type.
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// keywords maps keyword strings to their corresponding token types.
var keywords = map[string]TokenType{
	"version":  TOKEN_VERSION,
	"module":   TOKEN_MODULE,
	"library":  TOKEN_LIBRARY,
	"import":   TOKEN_IMPORT,
	"as":       TOKEN_AS,
	"export":   TOKEN_EXPORT,
	"effect":   TOKEN_EFFECT,
	"output":   TOKEN_OUTPUT,
	"params":   TOKEN_PARAMS,
	"timeline": TOKEN_TIMELINE,
	"function": TOKEN_FUNCTION,
	"end":      TOKEN_END,
	"if":       TOKEN_IF,
	"then":     TOKEN_THEN,
	"elseif":   TOKEN_ELSEIF,
	"else":     TOKEN_ELSE,
	"return":   TOKEN_RETURN,
	"and":      TOKEN_AND,
	"or":       TOKEN_OR,
	"not":      TOKEN_NOT,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
}

// LookupKeyword returns the keyword token type for ident, or TOKEN_IDENT if
// the string is not a reserved keyword.
func LookupKeyword(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

// Pos represents a source position for diagnostics.
type Pos struct {
	Line int // 1-based line number
	Col  int // 1-based column number
}

// String returns a "line:col" representation of the position.
func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// Token is a single lexical token produced by the scanner.
type Token struct {
	Type    TokenType
	Literal string // raw text of the token
	Pos     Pos    // starting position in source
}

// String returns a debug-friendly representation of the token.
func (t Token) String() string {
	return fmt.Sprintf("%s %s %q", t.Pos, t.Type, t.Literal)
}
