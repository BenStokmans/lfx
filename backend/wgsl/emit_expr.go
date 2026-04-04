package wgsl

import (
	"fmt"
	"strings"

	"github.com/BenStokmans/lfx/ir"
)

func (e *Emitter) emitExpr(expr ir.IRExpr) string {
	switch ex := expr.(type) {
	case *ir.Const:
		return e.emitConst(ex)

	case *ir.LocalRef:
		return sanitizeName(ex.Name)

	case *ir.ParamRef:
		return fmt.Sprintf("uniforms.param_%s", ex.Name)

	case *ir.BinaryOp:
		left := e.emitExpr(ex.Left)
		right := e.emitExpr(ex.Right)
		left = e.coerceExpr(left, ex.Left.ResultType(), ex.Typ)
		right = e.coerceExpr(right, ex.Right.ResultType(), ex.Typ)
		if ex.Op == ir.OpMod {
			return fmt.Sprintf("(%s - %s * floor(%s / %s))", left, right, left, right)
		}
		if isComparisonOp(ex.Op) {
			op := mapBinaryOp(ex.Op)
			return fmt.Sprintf("select(0.0, 1.0, (%s %s %s))", left, op, right)
		}
		if ex.Op == ir.OpAnd {
			return fmt.Sprintf("select(0.0, 1.0, ((%s != 0.0) && (%s != 0.0)))", left, right)
		}
		if ex.Op == ir.OpOr {
			return fmt.Sprintf("select(0.0, 1.0, ((%s != 0.0) || (%s != 0.0)))", left, right)
		}
		op := mapBinaryOp(ex.Op)
		return fmt.Sprintf("(%s %s %s)", left, op, right)

	case *ir.UnaryOp:
		operand := e.emitExpr(ex.Operand)
		switch ex.Op {
		case ir.OpNeg:
			return fmt.Sprintf("(-(%s))", operand)
		case ir.OpNot:
			return fmt.Sprintf("select(0.0, 1.0, !((%s) != 0.0))", operand)
		default:
			return fmt.Sprintf("/* unknown unary op */ %s", operand)
		}

	case *ir.Call:
		args := make([]string, len(ex.Args))
		for i, a := range ex.Args {
			args[i] = e.emitExpr(a)
		}
		return fmt.Sprintf("%s(%s)", sanitizeName(ex.Function), strings.Join(args, ", "))

	case *ir.BuiltinCall:
		args := make([]string, len(ex.Args))
		for i, a := range ex.Args {
			args[i] = e.emitExpr(a)
		}
		return e.emitBuiltinCall(ex, args)

	case *ir.TupleRef:
		inner := e.emitExpr(ex.Tuple)
		return fmt.Sprintf("%s.v%d", inner, ex.Index)

	case *ir.ComponentRef:
		inner := e.emitExpr(ex.Vector)
		return fmt.Sprintf("%s.%s", inner, componentName(ex.Index))

	default:
		return fmt.Sprintf("/* unknown expr %T */", expr)
	}
}

func isComparisonOp(op ir.Op) bool {
	switch op {
	case ir.OpEq, ir.OpNeq, ir.OpLt, ir.OpGt, ir.OpLte, ir.OpGte:
		return true
	default:
		return false
	}
}

func (e *Emitter) emitConst(c *ir.Const) string {
	switch v := c.Value.(type) {
	case float64:
		return formatFloat(v)
	case float32:
		return formatFloat(float64(v))
	case int:
		return formatFloat(float64(v))
	case int64:
		return formatFloat(float64(v))
	case int32:
		return formatFloat(float64(v))
	case bool:
		if v {
			return "1.0"
		}
		return "0.0"
	default:
		return "0.0"
	}
}

func (e *Emitter) coerceExpr(expr string, from, to ir.Type) string {
	if !to.IsVector() || from.IsVector() || from == ir.TypeUnknown {
		return expr
	}
	return fmt.Sprintf("%s(%s)", wgslType(to), expr)
}

// formatFloat formats a float64 as a WGSL f32 literal, always with a decimal point.
func formatFloat(f float64) string {
	s := fmt.Sprintf("%g", f)
	// Ensure there is a decimal point so WGSL interprets this as f32.
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

func componentName(index int) string {
	switch index {
	case 0:
		return "x"
	case 1:
		return "y"
	case 2:
		return "z"
	default:
		return "w"
	}
}

func mapBinaryOp(op ir.Op) string {
	switch op {
	case ir.OpAdd:
		return "+"
	case ir.OpSub:
		return "-"
	case ir.OpMul:
		return "*"
	case ir.OpDiv:
		return "/"
	case ir.OpEq:
		return "=="
	case ir.OpNeq:
		return "!="
	case ir.OpLt:
		return "<"
	case ir.OpGt:
		return ">"
	case ir.OpLte:
		return "<="
	case ir.OpGte:
		return ">="
	case ir.OpAnd:
		return "&&"
	case ir.OpOr:
		return "||"
	default:
		return fmt.Sprintf("/* unknown op %d */", int(op))
	}
}
