package parser

import (
	"fmt"
	"strconv"
)

// ParseError represents a parser error with position information.
type ParseError struct {
	Msg string
	Pos Pos
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

// Parser is a recursive-descent parser that consumes a token slice and builds
// an AST Module.
type Parser struct {
	tokens []Token
	pos    int
}

// NewParser creates a new Parser for the given token slice.
func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

// Parse is a convenience function that lexes the input and parses it into a
// Module AST node.
func Parse(input string) (*Module, error) {
	tokens, err := NewLexer(input).Tokenize()
	if err != nil {
		return nil, err
	}
	return NewParser(tokens).ParseModule()
}

// ---------------------------------------------------------------------------
// Token helpers
// ---------------------------------------------------------------------------

// skipComments advances past any TOKEN_COMMENT tokens at the current position.
func (p *Parser) skipComments() {
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type == TOKEN_COMMENT {
		p.pos++
	}
}

// current returns the current token (after skipping comments).
func (p *Parser) current() Token {
	p.skipComments()
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF, Pos: Pos{}}
	}
	return p.tokens[p.pos]
}

// peek returns the current token without advancing (after skipping comments).
func (p *Parser) peek() Token {
	return p.current()
}

// advance consumes the current token and returns it.
func (p *Parser) advance() Token {
	tok := p.current()
	if tok.Type != TOKEN_EOF {
		p.pos++
	}
	return tok
}

// expect consumes a token of the given type, or returns an error.
func (p *Parser) expect(tt TokenType) (Token, error) {
	tok := p.current()
	if tok.Type != tt {
		return tok, &ParseError{
			Msg: fmt.Sprintf("expected %s, got %s", tt, tok.Type),
			Pos: tok.Pos,
		}
	}
	return p.advance(), nil
}

func (p *Parser) expectIdentLike() (Token, error) {
	tok := p.current()
	if tok.Type != TOKEN_IDENT && tok.Type != TOKEN_PARAMS {
		return tok, &ParseError{
			Msg: fmt.Sprintf("expected IDENT, got %s", tok.Type),
			Pos: tok.Pos,
		}
	}
	return p.advance(), nil
}

// match checks if the current token is of the given type and consumes it if so.
func (p *Parser) match(tt TokenType) bool {
	if p.current().Type == tt {
		p.advance()
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Top-level: ParseModule
// ---------------------------------------------------------------------------

// ParseModule parses the full token stream into a Module AST.
func (p *Parser) ParseModule() (*Module, error) {
	mod := &Module{}

	// version (optional)
	if p.current().Type == TOKEN_VERSION {
		decl, err := p.parseVersionDecl()
		if err != nil {
			return nil, err
		}
		mod.Version = decl
	}

	// module (required)
	{
		tok, err := p.expect(TOKEN_MODULE)
		if err != nil {
			return nil, err
		}
		pathTok, err := p.expect(TOKEN_STRING)
		if err != nil {
			return nil, &ParseError{Msg: "expected module path string", Pos: tok.Pos}
		}
		mod.ModPath = pathTok.Literal
	}

	// imports (zero or more)
	for p.current().Type == TOKEN_IMPORT {
		decl, err := p.parseImportDecl()
		if err != nil {
			return nil, err
		}
		mod.Imports = append(mod.Imports, decl)
	}

	// effect or library (optional)
	switch p.current().Type {
	case TOKEN_EFFECT:
		tok := p.advance()
		nameTok, err := p.expect(TOKEN_STRING)
		if err != nil {
			return nil, &ParseError{Msg: "expected effect name string", Pos: tok.Pos}
		}
		mod.Kind = ModuleKindEffect
		mod.Effect = &EffectDecl{Pos: tok.Pos, Name: nameTok.Literal}
	case TOKEN_LIBRARY:
		tok := p.advance()
		nameTok, err := p.expect(TOKEN_STRING)
		if err != nil {
			return nil, &ParseError{Msg: "expected library name string", Pos: tok.Pos}
		}
		mod.Kind = ModuleKindLibrary
		mod.Library = &LibraryDecl{Pos: tok.Pos, Name: nameTok.Literal}
	}

	// output (optional)
	if p.current().Type == TOKEN_OUTPUT {
		decl, err := p.parseOutputDecl()
		if err != nil {
			return nil, err
		}
		mod.Output = decl
	}

	// params (optional)
	if p.current().Type == TOKEN_PARAMS {
		decl, err := p.parseParamsDecl()
		if err != nil {
			return nil, err
		}
		mod.Params = decl
	}

	// functions and optional timeline block (zero or more functions, at most one timeline)
	for {
		cur := p.current()
		switch cur.Type {
		case TOKEN_FUNCTION:
			fn, err := p.parseFuncDecl(false)
			if err != nil {
				return nil, err
			}
			mod.Funcs = append(mod.Funcs, fn)
		case TOKEN_EXPORT:
			fn, err := p.parseFuncDecl(true)
			if err != nil {
				return nil, err
			}
			mod.Funcs = append(mod.Funcs, fn)
		case TOKEN_TIMELINE:
			if mod.Timeline != nil {
				return nil, &ParseError{
					Msg: "duplicate timeline block",
					Pos: cur.Pos,
				}
			}
			tl, err := p.parseTimelineDecl()
			if err != nil {
				return nil, err
			}
			mod.Timeline = tl
		case TOKEN_EOF:
			return mod, nil
		default:
			return nil, &ParseError{
				Msg: fmt.Sprintf("unexpected token %s at top level", cur.Type),
				Pos: cur.Pos,
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Declaration parsers
// ---------------------------------------------------------------------------

func (p *Parser) parseVersionDecl() (*VersionDecl, error) {
	tok := p.advance() // consume 'version'
	verTok, err := p.expect(TOKEN_STRING)
	if err != nil {
		return nil, &ParseError{Msg: "expected version string", Pos: tok.Pos}
	}
	return &VersionDecl{Pos: tok.Pos, Version: verTok.Literal}, nil
}

func (p *Parser) parseImportDecl() (*ImportDecl, error) {
	tok := p.advance() // consume 'import'
	pathTok, err := p.expect(TOKEN_STRING)
	if err != nil {
		return nil, &ParseError{Msg: "expected import path string", Pos: tok.Pos}
	}
	decl := &ImportDecl{Pos: tok.Pos, Path: pathTok.Literal}
	if p.match(TOKEN_AS) {
		aliasTok, err := p.expect(TOKEN_IDENT)
		if err != nil {
			return nil, &ParseError{Msg: "expected alias identifier after 'as'", Pos: tok.Pos}
		}
		decl.Alias = aliasTok.Literal
	}
	return decl, nil
}

func (p *Parser) parseOutputDecl() (*OutputDecl, error) {
	tok := p.advance() // consume 'output'
	typeTok, err := p.expect(TOKEN_IDENT)
	if err != nil {
		return nil, &ParseError{Msg: "expected output type (scalar, rgb, rgbw)", Pos: tok.Pos}
	}

	var outputType OutputType
	switch typeTok.Literal {
	case "scalar":
		outputType = OutputScalar
	case "rgb":
		outputType = OutputRGB
	case "rgbw":
		outputType = OutputRGBW
	default:
		return nil, &ParseError{
			Msg: fmt.Sprintf("unknown output type %q", typeTok.Literal),
			Pos: typeTok.Pos,
		}
	}

	return &OutputDecl{Pos: tok.Pos, Type: outputType}, nil
}

func (p *Parser) parseParamsDecl() (*ParamsDecl, error) {
	tok := p.advance() // consume 'params'
	if _, err := p.expect(TOKEN_LBRACE); err != nil {
		return nil, err
	}
	decl := &ParamsDecl{Pos: tok.Pos}
	for p.current().Type != TOKEN_RBRACE {
		if p.current().Type == TOKEN_EOF {
			return nil, &ParseError{Msg: "unterminated params block", Pos: tok.Pos}
		}
		param, err := p.parseParamDef()
		if err != nil {
			return nil, err
		}
		decl.Params = append(decl.Params, param)
	}
	p.advance() // consume '}'
	return decl, nil
}

func (p *Parser) parseParamDef() (*ParamDef, error) {
	nameTok, err := p.expect(TOKEN_IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TOKEN_ASSIGN); err != nil {
		return nil, err
	}
	typeTok, err := p.expect(TOKEN_IDENT)
	if err != nil {
		return nil, &ParseError{Msg: "expected parameter type (int, float, bool, enum)", Pos: nameTok.Pos}
	}
	if _, err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}

	def := &ParamDef{Pos: nameTok.Pos, Name: nameTok.Literal}

	switch typeTok.Literal {
	case "int":
		def.Type = ParamInt
		if err := p.parseIntParam(def); err != nil {
			return nil, err
		}
	case "float":
		def.Type = ParamFloat
		if err := p.parseFloatParam(def); err != nil {
			return nil, err
		}
	case "bool":
		def.Type = ParamBool
		if err := p.parseBoolParam(def); err != nil {
			return nil, err
		}
	case "enum":
		def.Type = ParamEnum
		if err := p.parseEnumParam(def); err != nil {
			return nil, err
		}
	default:
		return nil, &ParseError{
			Msg: fmt.Sprintf("unknown parameter type %q", typeTok.Literal),
			Pos: typeTok.Pos,
		}
	}

	if _, err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}
	return def, nil
}

// parseNumberValue parses an optionally-negative int or float literal and
// returns its float64 value. Used for param default/min/max values.
func (p *Parser) parseNumberValue() (float64, error) {
	neg := false
	if p.current().Type == TOKEN_MINUS {
		neg = true
		p.advance()
	}
	tok := p.current()
	if tok.Type != TOKEN_INT && tok.Type != TOKEN_FLOAT {
		return 0, &ParseError{Msg: "expected number", Pos: tok.Pos}
	}
	p.advance()
	val, err := strconv.ParseFloat(tok.Literal, 64)
	if err != nil {
		return 0, &ParseError{Msg: fmt.Sprintf("invalid number %q", tok.Literal), Pos: tok.Pos}
	}
	if neg {
		val = -val
	}
	return val, nil
}

func (p *Parser) parseIntParam(def *ParamDef) error {
	val, err := p.parseNumberValue()
	if err != nil {
		return err
	}
	def.Default = int(val)
	if p.match(TOKEN_COMMA) {
		minVal, err := p.parseNumberValue()
		if err != nil {
			return err
		}
		def.Min = &minVal
		if p.match(TOKEN_COMMA) {
			maxVal, err := p.parseNumberValue()
			if err != nil {
				return err
			}
			def.Max = &maxVal
		}
	}
	return nil
}

func (p *Parser) parseFloatParam(def *ParamDef) error {
	val, err := p.parseNumberValue()
	if err != nil {
		return err
	}
	def.Default = val
	if p.match(TOKEN_COMMA) {
		minVal, err := p.parseNumberValue()
		if err != nil {
			return err
		}
		def.Min = &minVal
		if p.match(TOKEN_COMMA) {
			maxVal, err := p.parseNumberValue()
			if err != nil {
				return err
			}
			def.Max = &maxVal
		}
	}
	return nil
}

func (p *Parser) parseBoolParam(def *ParamDef) error {
	tok := p.current()
	switch tok.Type {
	case TOKEN_TRUE:
		p.advance()
		def.Default = true
	case TOKEN_FALSE:
		p.advance()
		def.Default = false
	default:
		return &ParseError{Msg: "expected true or false", Pos: tok.Pos}
	}
	return nil
}

func (p *Parser) parseEnumParam(def *ParamDef) error {
	// first string is the default
	tok, err := p.expect(TOKEN_STRING)
	if err != nil {
		return &ParseError{Msg: "expected enum default string", Pos: p.current().Pos}
	}
	def.Default = tok.Literal
	def.EnumValues = []string{tok.Literal}
	for p.match(TOKEN_COMMA) {
		val, err := p.expect(TOKEN_STRING)
		if err != nil {
			return err
		}
		def.EnumValues = append(def.EnumValues, val.Literal)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Function declaration
// ---------------------------------------------------------------------------

func (p *Parser) parseFuncDecl(exported bool) (*FuncDecl, error) {
	var pos Pos
	if exported {
		tok := p.advance() // consume 'export'
		pos = tok.Pos
		if _, err := p.expect(TOKEN_FUNCTION); err != nil {
			return nil, err
		}
	} else {
		tok := p.advance() // consume 'function'
		pos = tok.Pos
	}

	nameTok, err := p.expect(TOKEN_IDENT)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TOKEN_LPAREN); err != nil {
		return nil, err
	}

	var params []string
	if p.current().Type != TOKEN_RPAREN {
		for {
			paramTok, err := p.expectIdentLike()
			if err != nil {
				return nil, err
			}
			params = append(params, paramTok.Literal)
			if !p.match(TOKEN_COMMA) {
				break
			}
		}
	}

	if _, err := p.expect(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	body, err := p.parseBlock(TOKEN_END)
	if err != nil {
		return nil, err
	}
	p.advance() // consume 'end'

	return &FuncDecl{
		Pos:      pos,
		Name:     nameTok.Literal,
		Params:   params,
		Body:     body,
		Exported: exported,
	}, nil
}

// ---------------------------------------------------------------------------
// Timeline declaration
// ---------------------------------------------------------------------------

func (p *Parser) parseTimelineDecl() (*TimelineDecl, error) {
	tok := p.advance() // consume 'timeline'
	if _, err := p.expect(TOKEN_LBRACE); err != nil {
		return nil, err
	}

	decl := &TimelineDecl{Pos: tok.Pos}
	for p.current().Type != TOKEN_RBRACE {
		if p.current().Type == TOKEN_EOF {
			return nil, &ParseError{Msg: "unterminated timeline block", Pos: tok.Pos}
		}
		keyTok, err := p.expect(TOKEN_IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TOKEN_ASSIGN); err != nil {
			return nil, err
		}
		val, err := p.parseNumberValue()
		if err != nil {
			return nil, err
		}
		switch keyTok.Literal {
		case "loop_start":
			v := val
			decl.LoopStart = &v
		case "loop_end":
			v := val
			decl.LoopEnd = &v
		default:
			return nil, &ParseError{
				Msg: fmt.Sprintf("unknown timeline field %q (valid fields: loop_start, loop_end)", keyTok.Literal),
				Pos: keyTok.Pos,
			}
		}
	}
	p.advance() // consume '}'

	return decl, nil
}

// ---------------------------------------------------------------------------
// Statement parsing
// ---------------------------------------------------------------------------

// parseBlock parses statements until one of the stop tokens is encountered.
func (p *Parser) parseBlock(stopTokens ...TokenType) ([]Stmt, error) {
	var stmts []Stmt
	for {
		cur := p.current()
		for _, st := range stopTokens {
			if cur.Type == st {
				return stmts, nil
			}
		}
		if cur.Type == TOKEN_EOF {
			return nil, &ParseError{Msg: "unexpected end of input in block", Pos: cur.Pos}
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}
}

func (p *Parser) parseStmt() (Stmt, error) {
	cur := p.current()
	switch cur.Type {
	case TOKEN_IF:
		return p.parseIfStmt()
	case TOKEN_RETURN:
		return p.parseReturnStmt()
	case TOKEN_IDENT:
		return p.parseIdentLedStmt()
	default:
		return p.parseExprStmt()
	}
}

func (p *Parser) parseIdentLedStmt() (Stmt, error) {
	saved := p.pos
	nameTok, err := p.expect(TOKEN_IDENT)
	if err != nil {
		return nil, err
	}
	names := []string{nameTok.Literal}

	// Name-list assignment introduces multiple locals.
	for p.current().Type == TOKEN_COMMA {
		p.advance() // consume ','
		n, err := p.expect(TOKEN_IDENT)
		if err != nil {
			return nil, err
		}
		names = append(names, n.Literal)
	}

	if p.current().Type != TOKEN_ASSIGN {
		p.pos = saved
		return p.parseExprStmt()
	}
	p.advance() // consume '='

	values, err := p.parseExprList()
	if err != nil {
		return nil, err
	}

	if len(names) == 1 {
		if len(values) != 1 {
			return nil, &ParseError{
				Msg: "single-name assignment must have exactly one value",
				Pos: nameTok.Pos,
			}
		}
		return &AssignStmt{Pos: nameTok.Pos, Name: names[0], Value: values[0]}, nil
	}

	return &LocalStmt{Pos: nameTok.Pos, Names: names, Values: values}, nil
}

func (p *Parser) parseAssignStmt() (Stmt, error) {
	nameTok := p.advance() // consume ident
	p.advance()            // consume '='
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &AssignStmt{Pos: nameTok.Pos, Name: nameTok.Literal, Value: expr}, nil
}

func (p *Parser) parseIfStmt() (Stmt, error) {
	tok := p.advance() // consume 'if'
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TOKEN_THEN); err != nil {
		return nil, err
	}

	body, err := p.parseBlock(TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_END)
	if err != nil {
		return nil, err
	}

	stmt := &IfStmt{Pos: tok.Pos, Condition: cond, Body: body}

	// elseif clauses
	for p.current().Type == TOKEN_ELSEIF {
		eifTok := p.advance() // consume 'elseif'
		eifCond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TOKEN_THEN); err != nil {
			return nil, err
		}
		eifBody, err := p.parseBlock(TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_END)
		if err != nil {
			return nil, err
		}
		stmt.ElseIfs = append(stmt.ElseIfs, ElseIfClause{
			Pos:       eifTok.Pos,
			Condition: eifCond,
			Body:      eifBody,
		})
	}

	// else clause
	if p.current().Type == TOKEN_ELSE {
		p.advance() // consume 'else'
		elseBody, err := p.parseBlock(TOKEN_END)
		if err != nil {
			return nil, err
		}
		stmt.ElseBody = elseBody
	}

	if _, err := p.expect(TOKEN_END); err != nil {
		return nil, err
	}

	return stmt, nil
}

func (p *Parser) parseReturnStmt() (Stmt, error) {
	tok := p.advance() // consume 'return'
	// Check if there is an expression following (not a block terminator).
	cur := p.current()
	switch cur.Type {
	case TOKEN_END, TOKEN_ELSE, TOKEN_ELSEIF, TOKEN_EOF:
		return &ReturnStmt{Pos: tok.Pos}, nil
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	values := []Expr{expr}
	for p.match(TOKEN_COMMA) {
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return &ReturnStmt{Pos: tok.Pos, Values: values}, nil
}

func (p *Parser) parseExprStmt() (Stmt, error) {
	pos := p.current().Pos
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ExprStmt{Pos: pos, Expr: expr}, nil
}

// ---------------------------------------------------------------------------
// Expression parsing (Pratt / precedence climbing)
// ---------------------------------------------------------------------------

// Operator precedence levels.
const (
	precNone  = 0
	precOr    = 1
	precAnd   = 2
	precEq    = 3
	precCmp   = 4
	precAdd   = 5
	precMul   = 6
	precUnary = 7
)

func binaryPrec(tt TokenType) int {
	switch tt {
	case TOKEN_OR:
		return precOr
	case TOKEN_AND:
		return precAnd
	case TOKEN_EQ, TOKEN_NEQ:
		return precEq
	case TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE:
		return precCmp
	case TOKEN_PLUS, TOKEN_MINUS:
		return precAdd
	case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT:
		return precMul
	default:
		return precNone
	}
}

func opString(tt TokenType) string {
	switch tt {
	case TOKEN_OR:
		return "or"
	case TOKEN_AND:
		return "and"
	case TOKEN_EQ:
		return "=="
	case TOKEN_NEQ:
		return "~="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LTE:
		return "<="
	case TOKEN_GTE:
		return ">="
	case TOKEN_PLUS:
		return "+"
	case TOKEN_MINUS:
		return "-"
	case TOKEN_STAR:
		return "*"
	case TOKEN_SLASH:
		return "/"
	case TOKEN_PERCENT:
		return "%"
	default:
		return "?"
	}
}

func (p *Parser) parseExpr() (Expr, error) {
	return p.parseBinaryExpr(precOr)
}

func (p *Parser) parseBinaryExpr(minPrec int) (Expr, error) {
	left, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	for {
		cur := p.current()
		prec := binaryPrec(cur.Type)
		if prec < minPrec {
			break
		}
		op := opString(cur.Type)
		pos := cur.Pos
		p.advance() // consume operator

		right, err := p.parseBinaryExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Pos: pos, Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *Parser) parseUnaryExpr() (Expr, error) {
	cur := p.current()

	if cur.Type == TOKEN_NOT {
		p.advance()
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Pos: cur.Pos, Op: "not", Operand: operand}, nil
	}

	if cur.Type == TOKEN_MINUS {
		p.advance()
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Pos: cur.Pos, Op: "-", Operand: operand}, nil
	}

	return p.parsePostfixExpr()
}

func (p *Parser) parsePostfixExpr() (Expr, error) {
	expr, err := p.parsePrimaryExpr()
	if err != nil {
		return nil, err
	}

	for {
		cur := p.current()
		switch cur.Type {
		case TOKEN_DOT:
			p.advance() // consume '.'
			fieldTok, err := p.expect(TOKEN_IDENT)
			if err != nil {
				return nil, err
			}
			expr = &DotExpr{Pos: cur.Pos, Object: expr, Field: fieldTok.Literal}
		case TOKEN_LPAREN:
			pos := cur.Pos
			p.advance() // consume '('
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(TOKEN_RPAREN); err != nil {
				return nil, err
			}
			expr = &CallExpr{Pos: pos, Function: expr, Args: args}
		default:
			return expr, nil
		}
	}
}

func (p *Parser) parseArgList() ([]Expr, error) {
	var args []Expr
	if p.current().Type == TOKEN_RPAREN {
		return args, nil
	}
	return p.parseExprList()
}

func (p *Parser) parseExprList() ([]Expr, error) {
	var args []Expr
	for {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if !p.match(TOKEN_COMMA) {
			break
		}
	}
	return args, nil
}

func (p *Parser) parsePrimaryExpr() (Expr, error) {
	cur := p.current()

	switch cur.Type {
	case TOKEN_INT:
		p.advance()
		val, err := strconv.ParseFloat(cur.Literal, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid integer %q", cur.Literal), Pos: cur.Pos}
		}
		return &NumberLit{Pos: cur.Pos, Value: val, IsInt: true}, nil

	case TOKEN_FLOAT:
		p.advance()
		val, err := strconv.ParseFloat(cur.Literal, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid float %q", cur.Literal), Pos: cur.Pos}
		}
		return &NumberLit{Pos: cur.Pos, Value: val, IsInt: false}, nil

	case TOKEN_STRING:
		p.advance()
		return &StringLit{Pos: cur.Pos, Value: cur.Literal}, nil

	case TOKEN_TRUE:
		p.advance()
		return &BoolLit{Pos: cur.Pos, Value: true}, nil

	case TOKEN_FALSE:
		p.advance()
		return &BoolLit{Pos: cur.Pos, Value: false}, nil

	case TOKEN_IDENT:
		p.advance()
		return &Ident{Pos: cur.Pos, Name: cur.Literal}, nil

	case TOKEN_PARAMS:
		p.advance()
		return &Ident{Pos: cur.Pos, Name: cur.Literal}, nil

	case TOKEN_LPAREN:
		p.advance() // consume '('
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &GroupExpr{Pos: cur.Pos, Inner: inner}, nil

	default:
		return nil, &ParseError{
			Msg: fmt.Sprintf("unexpected token %s in expression", cur.Type),
			Pos: cur.Pos,
		}
	}
}
