package parser

// ---------------------------------------------------------------------------
// Module-level types
// ---------------------------------------------------------------------------

// ModuleKind distinguishes effect modules from library modules.
type ModuleKind int

const (
	ModuleKindEffect  ModuleKind = iota // default: module defines a visual effect
	ModuleKindLibrary                   // module is a reusable library
)

// Module is the top-level AST node for an LFX source file.
type Module struct {
	Version  *VersionDecl
	ModPath  string
	Kind     ModuleKind
	Effect   *EffectDecl  // nil for library modules
	Library  *LibraryDecl // nil for effect modules
	Output   *OutputDecl
	Imports  []*ImportDecl
	Params   *ParamsDecl
	Funcs    []*FuncDecl
	Timeline *TimelineDecl
}

// ---------------------------------------------------------------------------
// Declarations
// ---------------------------------------------------------------------------

// VersionDecl represents a `version "0.1"` declaration.
type VersionDecl struct {
	Pos     Pos
	Version string // e.g. "0.1"
}

// ModuleDecl represents a `module "effects/fill_iris"` declaration.
type ModuleDecl struct {
	Pos  Pos
	Path string // e.g. "effects/fill_iris"
}

// LibraryDecl represents a `library "math_helpers"` declaration.
type LibraryDecl struct {
	Pos  Pos
	Name string
}

// EffectDecl represents an `effect "Fill Iris"` declaration.
type EffectDecl struct {
	Pos  Pos
	Name string
}

// OutputType describes the number of channels produced by sample().
type OutputType int

const (
	OutputScalar OutputType = iota
	OutputRGB
	OutputRGBW
)

func (t OutputType) Channels() int {
	switch t {
	case OutputRGB:
		return 3
	case OutputRGBW:
		return 4
	default:
		return 1
	}
}

// OutputDecl represents an `output ...` declaration.
type OutputDecl struct {
	Pos  Pos
	Type OutputType
}

// ImportDecl represents an `import "path" as alias` declaration.
type ImportDecl struct {
	Pos   Pos
	Path  string
	Alias string // empty when no alias is specified
}

// ParamType enumerates the supported parameter type constructors.
type ParamType int

const (
	ParamInt ParamType = iota
	ParamFloat
	ParamBool
	ParamEnum
)

// ParamDef describes a single entry in a params block.
type ParamDef struct {
	Pos        Pos
	Name       string
	Type       ParamType
	Default    interface{} // int, float64, bool, or string depending on Type
	Min        *float64    // nil when unspecified (int/float only)
	Max        *float64    // nil when unspecified (int/float only)
	EnumValues []string    // non-nil only for ParamEnum
}

// ParamsDecl represents the `params { ... end` block.
type ParamsDecl struct {
	Pos    Pos
	Params []*ParamDef
}

// FuncDecl represents a `function name(args) ... end` declaration.
type FuncDecl struct {
	Pos      Pos
	Name     string
	Params   []string // parameter names
	Body     []Stmt
	Exported bool // true when preceded by the `export` keyword
}

// TimelineDecl represents an optional `timeline { ... }` block that declares
// loop markers for the normalized phase clip.
type TimelineDecl struct {
	Pos       Pos
	LoopStart *float64 // nil when not specified
	LoopEnd   *float64 // nil when not specified
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// Stmt is the interface satisfied by all statement nodes.
type Stmt interface {
	stmtNode()
	StmtPos() Pos
}

// LocalStmt represents `x, y = expr1, expr2` when introducing multiple locals.
type LocalStmt struct {
	Pos    Pos
	Names  []string
	Values []Expr
}

func (s *LocalStmt) stmtNode()    {}
func (s *LocalStmt) StmtPos() Pos { return s.Pos }

// AssignStmt represents `name = expr`.
// A first assignment introduces a new function-local binding.
type AssignStmt struct {
	Pos   Pos
	Name  string
	Value Expr
}

func (s *AssignStmt) stmtNode()    {}
func (s *AssignStmt) StmtPos() Pos { return s.Pos }

// ElseIfClause is a single `elseif cond then ... ` branch inside an IfStmt.
type ElseIfClause struct {
	Pos       Pos
	Condition Expr
	Body      []Stmt
}

// IfStmt represents `if cond then ... elseif ... else ... end`.
type IfStmt struct {
	Pos       Pos
	Condition Expr
	Body      []Stmt
	ElseIfs   []ElseIfClause
	ElseBody  []Stmt
}

func (s *IfStmt) stmtNode()    {}
func (s *IfStmt) StmtPos() Pos { return s.Pos }

// ReturnStmt represents `return expr`.
type ReturnStmt struct {
	Pos    Pos
	Values []Expr // empty for bare `return`
}

func (s *ReturnStmt) stmtNode()    {}
func (s *ReturnStmt) StmtPos() Pos { return s.Pos }

// ExprStmt wraps a bare expression used as a statement (e.g. a function call).
type ExprStmt struct {
	Pos  Pos
	Expr Expr
}

func (s *ExprStmt) stmtNode()    {}
func (s *ExprStmt) StmtPos() Pos { return s.Pos }

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// Expr is the interface satisfied by all expression nodes.
type Expr interface {
	exprNode()
	ExprPos() Pos
}

// NumberLit represents an integer or floating-point literal.
type NumberLit struct {
	Pos   Pos
	Value float64
	IsInt bool
}

func (e *NumberLit) exprNode()    {}
func (e *NumberLit) ExprPos() Pos { return e.Pos }

// StringLit represents a double-quoted string literal.
type StringLit struct {
	Pos   Pos
	Value string
}

func (e *StringLit) exprNode()    {}
func (e *StringLit) ExprPos() Pos { return e.Pos }

// BoolLit represents `true` or `false`.
type BoolLit struct {
	Pos   Pos
	Value bool
}

func (e *BoolLit) exprNode()    {}
func (e *BoolLit) ExprPos() Pos { return e.Pos }

// Ident represents a plain identifier reference.
type Ident struct {
	Pos  Pos
	Name string
}

func (e *Ident) exprNode()    {}
func (e *Ident) ExprPos() Pos { return e.Pos }

// BinaryExpr represents `left op right`.
type BinaryExpr struct {
	Pos   Pos
	Left  Expr
	Op    string
	Right Expr
}

func (e *BinaryExpr) exprNode()    {}
func (e *BinaryExpr) ExprPos() Pos { return e.Pos }

// UnaryExpr represents a prefix unary operation such as `-x` or `not x`.
type UnaryExpr struct {
	Pos     Pos
	Op      string
	Operand Expr
}

func (e *UnaryExpr) exprNode()    {}
func (e *UnaryExpr) ExprPos() Pos { return e.Pos }

// CallExpr represents `function(args)`.
type CallExpr struct {
	Pos      Pos
	Function Expr
	Args     []Expr
}

func (e *CallExpr) exprNode()    {}
func (e *CallExpr) ExprPos() Pos { return e.Pos }

// DotExpr represents `object.field` (e.g. params.name or module.func).
type DotExpr struct {
	Pos    Pos
	Object Expr
	Field  string
}

func (e *DotExpr) exprNode()    {}
func (e *DotExpr) ExprPos() Pos { return e.Pos }

// GroupExpr represents a parenthesized expression `(inner)`.
type GroupExpr struct {
	Pos   Pos
	Inner Expr
}

func (e *GroupExpr) exprNode()    {}
func (e *GroupExpr) ExprPos() Pos { return e.Pos }
