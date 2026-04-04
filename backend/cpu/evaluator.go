package cpu

import (
	"fmt"
	"math"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/runtime"
)

// Evaluator is a CPU-based IR interpreter that implements runtime.Sampler.
type Evaluator struct {
	module *ir.Module
	funcs  map[string]*ir.Function
}

// NewEvaluator creates a new CPU evaluator for the given IR module.
func NewEvaluator(mod *ir.Module) *Evaluator {
	e := &Evaluator{
		module: mod,
		funcs:  make(map[string]*ir.Function, len(mod.Functions)),
	}
	for _, fn := range mod.Functions {
		e.funcs[fn.Name] = fn
	}
	return e
}

// SamplePoint implements runtime.Sampler.
func (e *Evaluator) SamplePoint(layout runtime.Layout, pointIndex int, phase float32, params *runtime.BoundParams) (float32, error) {
	pt := layout.Points[pointIndex]

	args := []float64{
		float64(layout.Width),
		float64(layout.Height),
		float64(pt.X),
		float64(pt.Y),
		float64(pt.Index),
		float64(phase),
		0,
	}

	results, err := e.execFunc(e.module.Sample, args, params)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("sample function returned no value")
	}

	v := results[0]
	v = math.Max(0, math.Min(1, v))
	return float32(v), nil
}

// frame holds the local variables and bound params for a single function invocation.
type frame struct {
	locals []float64
	params *runtime.BoundParams
}

func (e *Evaluator) execFunc(fn *ir.Function, args []float64, params *runtime.BoundParams) ([]float64, error) {
	f := &frame{
		locals: make([]float64, len(fn.Locals)),
		params: params,
	}

	// Bind function parameters to local slots.
	for i, arg := range args {
		if i < len(fn.Params) {
			f.locals[i] = arg
		}
	}

	for _, stmt := range fn.Body {
		retVal, didReturn, err := e.execStmt(stmt, f)
		if err != nil {
			return nil, err
		}
		if didReturn {
			return retVal, nil
		}
	}

	return []float64{0}, nil
}

func (e *Evaluator) execStmt(stmt ir.IRStmt, f *frame) (returnVal []float64, didReturn bool, err error) {
	switch s := stmt.(type) {
	case *ir.LocalDecl:
		if s.Init != nil {
			v, err := e.evalExpr(s.Init, f)
			if err != nil {
				return nil, false, err
			}
			f.locals[s.Index] = v
		}

	case *ir.MultiLocalDecl:
		vals, err := e.evalExprMulti(s.Source, f)
		if err != nil {
			return nil, false, err
		}
		for i, idx := range s.Indices {
			if i < len(vals) {
				f.locals[idx] = vals[i]
			}
		}

	case *ir.Assign:
		v, err := e.evalExpr(s.Value, f)
		if err != nil {
			return nil, false, err
		}
		f.locals[s.Index] = v

	case *ir.IfStmt:
		cond, err := e.evalExpr(s.Cond, f)
		if err != nil {
			return nil, false, err
		}
		if cond != 0 {
			for _, inner := range s.Then {
				retVal, didReturn, err := e.execStmt(inner, f)
				if err != nil || didReturn {
					return retVal, didReturn, err
				}
			}
		} else {
			taken := false
			for _, elif := range s.ElseIfs {
				c, err := e.evalExpr(elif.Cond, f)
				if err != nil {
					return nil, false, err
				}
				if c != 0 {
					for _, inner := range elif.Body {
						retVal, didReturn, err := e.execStmt(inner, f)
						if err != nil || didReturn {
							return retVal, didReturn, err
						}
					}
					taken = true
					break
				}
			}
			if !taken && len(s.ElseBody) > 0 {
				for _, inner := range s.ElseBody {
					retVal, didReturn, err := e.execStmt(inner, f)
					if err != nil || didReturn {
						return retVal, didReturn, err
					}
				}
			}
		}

	case *ir.Return:
		if s.Value == nil {
			return nil, true, nil
		}
		vals, err := e.evalExprMulti(s.Value, f)
		if err != nil {
			return nil, false, err
		}
		return vals, true, nil

	case *ir.ExprStmt:
		_, err := e.evalExpr(s.Expr, f)
		if err != nil {
			return nil, false, err
		}

	default:
		return nil, false, fmt.Errorf("unknown statement type %T", stmt)
	}

	return nil, false, nil
}

func (e *Evaluator) evalExpr(expr ir.IRExpr, f *frame) (float64, error) {
	vals, err := e.evalExprMulti(expr, f)
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, nil
	}
	return vals[0], nil
}

func (e *Evaluator) evalExprMulti(expr ir.IRExpr, f *frame) ([]float64, error) {
	switch ex := expr.(type) {
	case *ir.Const:
		return []float64{constToFloat64(ex)}, nil

	case *ir.LocalRef:
		return []float64{f.locals[ex.Index]}, nil

	case *ir.ParamRef:
		v, ok := f.params.Values[ex.Name]
		if !ok {
			return nil, fmt.Errorf("unknown param %q", ex.Name)
		}
		return []float64{anyToFloat64(v)}, nil

	case *ir.BinaryOp:
		left, err := e.evalExpr(ex.Left, f)
		if err != nil {
			return nil, err
		}
		right, err := e.evalExpr(ex.Right, f)
		if err != nil {
			return nil, err
		}
		return []float64{evalBinaryOp(ex.Op, left, right)}, nil

	case *ir.UnaryOp:
		operand, err := e.evalExpr(ex.Operand, f)
		if err != nil {
			return nil, err
		}
		switch ex.Op {
		case ir.OpNeg:
			return []float64{-operand}, nil
		case ir.OpNot:
			if operand == 0 {
				return []float64{1}, nil
			}
			return []float64{0}, nil
		default:
			return nil, fmt.Errorf("unknown unary op %v", ex.Op)
		}

	case *ir.Call:
		fn, ok := e.funcs[ex.Function]
		if !ok {
			return nil, fmt.Errorf("unknown function %q", ex.Function)
		}
		args := make([]float64, len(ex.Args))
		for i, a := range ex.Args {
			v, err := e.evalExpr(a, f)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return e.execFunc(fn, args, f.params)

	case *ir.BuiltinCall:
		args := make([]float64, len(ex.Args))
		for i, a := range ex.Args {
			v, err := e.evalExpr(a, f)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return e.callBuiltin(ex.Builtin, args)

	case *ir.TupleRef:
		vals, err := e.evalExprMulti(ex.Tuple, f)
		if err != nil {
			return nil, err
		}
		if ex.Index >= len(vals) {
			return nil, fmt.Errorf("tuple index %d out of range (len %d)", ex.Index, len(vals))
		}
		return []float64{vals[ex.Index]}, nil

	default:
		return nil, fmt.Errorf("unknown expression type %T", expr)
	}
}

func constToFloat64(c *ir.Const) float64 {
	switch v := c.Value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func anyToFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func evalBinaryOp(op ir.Op, left, right float64) float64 {
	switch op {
	case ir.OpAdd:
		return left + right
	case ir.OpSub:
		return left - right
	case ir.OpMul:
		return left * right
	case ir.OpDiv:
		if right == 0 {
			return 0
		}
		return left / right
	case ir.OpMod:
		if right == 0 {
			return 0
		}
		return left - right*math.Floor(left/right)
	case ir.OpEq:
		return boolToFloat(left == right)
	case ir.OpNeq:
		return boolToFloat(left != right)
	case ir.OpLt:
		return boolToFloat(left < right)
	case ir.OpGt:
		return boolToFloat(left > right)
	case ir.OpLte:
		return boolToFloat(left <= right)
	case ir.OpGte:
		return boolToFloat(left >= right)
	case ir.OpAnd:
		return boolToFloat(left != 0 && right != 0)
	case ir.OpOr:
		return boolToFloat(left != 0 || right != 0)
	default:
		return 0
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
