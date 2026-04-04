package ir

// IRStmt is the interface implemented by all IR statement nodes.
type IRStmt interface {
	irStmtNode()
}

// LocalDecl declares a new local variable with an optional initializer.
type LocalDecl struct {
	Index          int
	Name           string
	Typ            Type
	Init           IRExpr // may be nil
	MultiRetSource IRExpr // non-nil when this local is part of a multi-assign
}

func (*LocalDecl) irStmtNode() {}

// Assign assigns a value to an existing local variable by slot index.
type Assign struct {
	Index int
	Value IRExpr
}

func (*Assign) irStmtNode() {}

// IRElseIf represents a single else-if branch.
type IRElseIf struct {
	Cond IRExpr
	Body []IRStmt
}

// IfStmt represents a conditional with optional else-if and else branches.
type IfStmt struct {
	Cond     IRExpr
	Then     []IRStmt
	ElseIfs  []IRElseIf
	ElseBody []IRStmt
}

func (*IfStmt) irStmtNode() {}

// Return returns a value from a function.
type Return struct {
	Values []IRExpr // empty for void returns
}

func (*Return) irStmtNode() {}

// ExprStmt wraps an expression used as a statement (e.g. a function call).
type ExprStmt struct {
	Expr IRExpr
}

func (*ExprStmt) irStmtNode() {}

// MultiLocalDecl declares multiple locals from a single multi-return call.
type MultiLocalDecl struct {
	Names   []string
	Indices []int
	Types   []Type
	Source  IRExpr
}

func (*MultiLocalDecl) irStmtNode() {}
