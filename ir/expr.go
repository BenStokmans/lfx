package ir

import "fmt"

// Op represents a binary or unary operator.
type Op int

const (
	OpAdd Op = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpEq
	OpNeq
	OpLt
	OpGt
	OpLte
	OpGte
	OpAnd
	OpOr
	OpNot
	OpNeg
)

func (o Op) String() string {
	switch o {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpEq:
		return "=="
	case OpNeq:
		return "!="
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLte:
		return "<="
	case OpGte:
		return ">="
	case OpAnd:
		return "&&"
	case OpOr:
		return "||"
	case OpNot:
		return "!"
	case OpNeg:
		return "-"
	default:
		return fmt.Sprintf("Op(%d)", int(o))
	}
}

// BuiltinID identifies a built-in (stdlib intrinsic) function.
type BuiltinID int

const (
	BuiltinAbs BuiltinID = iota
	BuiltinMin
	BuiltinMax
	BuiltinFloor
	BuiltinCeil
	BuiltinSqrt
	BuiltinSin
	BuiltinCos
	BuiltinClamp
	BuiltinMix
	BuiltinFract
	BuiltinMod
	BuiltinPow
	BuiltinIsEven
	BuiltinPerlin
	BuiltinVoronoi
	BuiltinVoronoiBorder
	BuiltinWorley
)

func (b BuiltinID) String() string {
	switch b {
	case BuiltinAbs:
		return "abs"
	case BuiltinMin:
		return "min"
	case BuiltinMax:
		return "max"
	case BuiltinFloor:
		return "floor"
	case BuiltinCeil:
		return "ceil"
	case BuiltinSqrt:
		return "sqrt"
	case BuiltinSin:
		return "sin"
	case BuiltinCos:
		return "cos"
	case BuiltinClamp:
		return "clamp"
	case BuiltinMix:
		return "mix"
	case BuiltinFract:
		return "fract"
	case BuiltinMod:
		return "mod"
	case BuiltinPow:
		return "pow"
	case BuiltinIsEven:
		return "is_even"
	case BuiltinPerlin:
		return "perlin"
	case BuiltinVoronoi:
		return "voronoi"
	case BuiltinVoronoiBorder:
		return "voronoi_border"
	case BuiltinWorley:
		return "worley"
	default:
		return fmt.Sprintf("BuiltinID(%d)", int(b))
	}
}

// IRExpr is the interface implemented by all IR expression nodes.
type IRExpr interface {
	irExprNode()
	ResultType() Type
}

// Const represents a compile-time constant value.
type Const struct {
	Value interface{}
	Typ   Type
}

func (*Const) irExprNode()        {}
func (c *Const) ResultType() Type { return c.Typ }

// LocalRef references a local variable by slot index.
type LocalRef struct {
	Index int
	Name  string
	Typ   Type
}

func (*LocalRef) irExprNode()        {}
func (l *LocalRef) ResultType() Type { return l.Typ }

// ParamRef references an effect parameter by name.
type ParamRef struct {
	Name string
	Typ  Type
}

func (*ParamRef) irExprNode()        {}
func (p *ParamRef) ResultType() Type { return p.Typ }

// BinaryOp represents a binary operation (e.g. add, compare).
type BinaryOp struct {
	Op    Op
	Left  IRExpr
	Right IRExpr
	Typ   Type
}

func (*BinaryOp) irExprNode()        {}
func (b *BinaryOp) ResultType() Type { return b.Typ }

// UnaryOp represents a unary operation (e.g. negate, not).
type UnaryOp struct {
	Op      Op
	Operand IRExpr
	Typ     Type
}

func (*UnaryOp) irExprNode()        {}
func (u *UnaryOp) ResultType() Type { return u.Typ }

// Call represents a call to a user-defined function.
type Call struct {
	Function      string
	Args          []IRExpr
	ReturnType    Type
	MultiRetCount int
}

func (*Call) irExprNode()        {}
func (c *Call) ResultType() Type { return c.ReturnType }

// BuiltinCall represents a call to a stdlib intrinsic function.
type BuiltinCall struct {
	Builtin       BuiltinID
	Args          []IRExpr
	ReturnType    Type
	MultiRetCount int
}

func (*BuiltinCall) irExprNode()        {}
func (b *BuiltinCall) ResultType() Type { return b.ReturnType }

// TupleRef extracts a single value from a multi-return expression.
type TupleRef struct {
	Tuple IRExpr
	Index int
	Typ   Type
}

func (*TupleRef) irExprNode()        {}
func (t *TupleRef) ResultType() Type { return t.Typ }
