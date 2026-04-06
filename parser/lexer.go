package parser

import "fmt"

// LexError represents a lexical analysis error with position information.
type LexError struct {
	Msg string
	Pos Pos
}

func (e *LexError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

// Lexer tokenises an LFX source string.
type Lexer struct {
	input  string
	pos    int
	line   int
	col    int
	tokens []Token
}

// NewLexer creates a new Lexer for the given input string.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		pos:    0,
		line:   1,
		col:    1,
		tokens: make([]Token, 0),
	}
}

// next consumes the current character, advancing position tracking.
func (l *Lexer) next() {
	if l.pos >= len(l.input) {
		return
	}
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
}

// peek returns the current character without consuming it.
// Returns 0 if at end of input.
func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// skipWhitespace advances past spaces, tabs, carriage returns, and newlines.
func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.next()
		} else {
			break
		}
	}
}

// readString reads a double-quoted string literal. The opening quote has
// already been consumed. Returns the string contents (without quotes).
func (l *Lexer) readString(startPos Pos) (Token, error) {
	start := l.pos
	for {
		if l.pos >= len(l.input) {
			return Token{}, &LexError{Msg: "unterminated string literal", Pos: startPos}
		}
		ch := l.input[l.pos]
		if ch == '\n' {
			return Token{}, &LexError{Msg: "unterminated string literal", Pos: startPos}
		}
		if ch == '"' {
			literal := l.input[start:l.pos]
			l.next() // consume closing quote
			return Token{Type: TOKEN_STRING, Literal: literal, Pos: startPos}, nil
		}
		l.next()
	}
}

// readNumber reads an integer or float literal starting from the current
// position. The first digit character has NOT been consumed yet.
func (l *Lexer) readNumber(startPos Pos) Token {
	start := l.pos
	isFloat := false

	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.next()
	}

	// Check for decimal point followed by a digit.
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		// Look ahead to confirm a digit follows the dot.
		if l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
			isFloat = true
			l.next() // consume '.'
			for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
				l.next()
			}
		}
	}

	literal := l.input[start:l.pos]
	tokType := TOKEN_INT
	if isFloat {
		tokType = TOKEN_FLOAT
	}
	return Token{Type: tokType, Literal: literal, Pos: startPos}
}

// readIdentifier reads an identifier or keyword starting from the current
// position. The first character has NOT been consumed yet.
func (l *Lexer) readIdentifier(startPos Pos) Token {
	start := l.pos
	for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
		l.next()
	}
	literal := l.input[start:l.pos]
	tokType := LookupKeyword(literal)
	return Token{Type: tokType, Literal: literal, Pos: startPos}
}

// Tokenize scans the entire input and returns the resulting token slice
// (terminated by TOKEN_EOF) or the first lexical error encountered.
func (l *Lexer) Tokenize() ([]Token, error) {
	for {
		l.skipWhitespace()

		if l.pos >= len(l.input) {
			l.tokens = append(l.tokens, Token{Type: TOKEN_EOF, Literal: "", Pos: Pos{Line: l.line, Col: l.col}})
			return l.tokens, nil
		}

		startPos := Pos{Line: l.line, Col: l.col}
		ch := l.peek()

		switch {
		// Comments: --
		case ch == '-' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '-':
			start := l.pos
			l.next() // consume first -
			l.next() // consume second -
			for l.pos < len(l.input) && l.input[l.pos] != '\n' {
				l.next()
			}
			l.tokens = append(l.tokens, Token{Type: TOKEN_COMMENT, Literal: l.input[start:l.pos], Pos: startPos})

		// String literals
		case ch == '"':
			l.next() // consume opening quote
			tok, err := l.readString(startPos)
			if err != nil {
				return nil, err
			}
			l.tokens = append(l.tokens, tok)

		// Number literals
		case isDigit(ch):
			tok := l.readNumber(startPos)
			l.tokens = append(l.tokens, tok)

		// Identifiers and keywords
		case isIdentStart(ch):
			tok := l.readIdentifier(startPos)
			l.tokens = append(l.tokens, tok)

		// Two-character operators and single-character operators/punctuation
		case ch == '=':
			l.next()
			if l.peek() == '=' {
				l.next()
				l.tokens = append(l.tokens, Token{Type: TOKEN_EQ, Literal: "==", Pos: startPos})
			} else {
				l.tokens = append(l.tokens, Token{Type: TOKEN_ASSIGN, Literal: "=", Pos: startPos})
			}

		case ch == '~':
			l.next()
			if l.peek() == '=' {
				l.next()
				l.tokens = append(l.tokens, Token{Type: TOKEN_NEQ, Literal: "~=", Pos: startPos})
			} else {
				return nil, &LexError{Msg: fmt.Sprintf("unexpected character %q", ch), Pos: startPos}
			}

		case ch == '<':
			l.next()
			if l.peek() == '=' {
				l.next()
				l.tokens = append(l.tokens, Token{Type: TOKEN_LTE, Literal: "<=", Pos: startPos})
			} else {
				l.tokens = append(l.tokens, Token{Type: TOKEN_LT, Literal: "<", Pos: startPos})
			}

		case ch == '>':
			l.next()
			if l.peek() == '=' {
				l.next()
				l.tokens = append(l.tokens, Token{Type: TOKEN_GTE, Literal: ">=", Pos: startPos})
			} else {
				l.tokens = append(l.tokens, Token{Type: TOKEN_GT, Literal: ">", Pos: startPos})
			}

		case ch == '+':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_PLUS, Literal: "+", Pos: startPos})
		case ch == '-':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_MINUS, Literal: "-", Pos: startPos})
		case ch == '*':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_STAR, Literal: "*", Pos: startPos})
		case ch == '/':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_SLASH, Literal: "/", Pos: startPos})
		case ch == '%':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_PERCENT, Literal: "%", Pos: startPos})

		// Punctuation
		case ch == '(':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_LPAREN, Literal: "(", Pos: startPos})
		case ch == ')':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_RPAREN, Literal: ")", Pos: startPos})
		case ch == '{':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_LBRACE, Literal: "{", Pos: startPos})
		case ch == '}':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_RBRACE, Literal: "}", Pos: startPos})
		case ch == ',':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_COMMA, Literal: ",", Pos: startPos})
		case ch == '.':
			l.next()
			l.tokens = append(l.tokens, Token{Type: TOKEN_DOT, Literal: ".", Pos: startPos})

		default:
			return nil, &LexError{Msg: fmt.Sprintf("unexpected character %q", ch), Pos: startPos}
		}
	}
}

// isDigit reports whether ch is an ASCII digit.
func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isIdentStart reports whether ch can start an identifier (letter or underscore).
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

// isIdentChar reports whether ch can appear inside an identifier.
func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
