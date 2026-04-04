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
func (e *Evaluator) SamplePoint(layout runtime.Layout, pointIndex int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	if pointIndex < 0 || pointIndex >= len(layout.Points) {
		return nil, fmt.Errorf("point index %d out of range", pointIndex)
	}
	if e.module == nil || e.module.Sample == nil {
		return nil, fmt.Errorf("sample function is not available")
	}

	pt := layout.Points[pointIndex]
	args := []value{
		scalarValue(ir.TypeF32, float64(layout.Width)),
		scalarValue(ir.TypeF32, float64(layout.Height)),
		scalarValue(ir.TypeF32, float64(pt.X)),
		scalarValue(ir.TypeF32, float64(pt.Y)),
		scalarValue(ir.TypeF32, float64(pt.Index)),
		scalarValue(ir.TypeF32, float64(phase)),
		scalarValue(ir.TypeF32, 0),
	}

	results, err := e.execFunc(e.module.Sample, args, params)
	if err != nil {
		return nil, err
	}

	channels := e.module.Output.Channels()
	out := make([]float32, channels)
	switch {
	case len(results) == 1 && results[0].Typ.IsVector():
		if results[0].laneCount() < channels {
			return nil, fmt.Errorf("sample vector returned %d values, expected %d", results[0].laneCount(), channels)
		}
		for idx := 0; idx < channels; idx++ {
			out[idx] = float32(clampOutput(results[0].Lanes[idx]))
		}
	case len(results) >= channels:
		for idx := 0; idx < channels; idx++ {
			out[idx] = float32(clampOutput(results[idx].scalar()))
		}
	default:
		return nil, fmt.Errorf("sample function returned %d values, expected %d", len(results), channels)
	}
	return out, nil
}

// frame holds the local variables and bound params for a single function invocation.
type frame struct {
	locals []value
	params *runtime.BoundParams
}

func (e *Evaluator) execFunc(fn *ir.Function, args []value, params *runtime.BoundParams) ([]value, error) {
	if fn == nil {
		return nil, fmt.Errorf("function is nil")
	}
	f := &frame{
		locals: make([]value, len(fn.Locals)),
		params: params,
	}
	for idx := range fn.Locals {
		f.locals[idx] = zeroValue(fn.Locals[idx].Type)
	}
	for idx, arg := range args {
		if idx < len(fn.Params) {
			f.locals[idx] = arg
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

	return []value{scalarValue(ir.TypeF32, 0)}, nil
}

func (e *Evaluator) execStmt(stmt ir.IRStmt, f *frame) (returnVal []value, didReturn bool, err error) {
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
		if cond.truthy() {
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
				if c.truthy() {
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
		if len(s.Values) == 0 {
			return nil, true, nil
		}
		vals := make([]value, 0, len(s.Values))
		for _, expr := range s.Values {
			value, err := e.evalExpr(expr, f)
			if err != nil {
				return nil, false, err
			}
			vals = append(vals, value)
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

func (e *Evaluator) evalExpr(expr ir.IRExpr, f *frame) (value, error) {
	vals, err := e.evalExprMulti(expr, f)
	if err != nil {
		return zeroValue(ir.TypeF32), err
	}
	if len(vals) == 0 {
		return zeroValue(ir.TypeF32), nil
	}
	return vals[0], nil
}

func (e *Evaluator) evalExprMulti(expr ir.IRExpr, f *frame) ([]value, error) {
	switch ex := expr.(type) {
	case *ir.Const:
		return []value{constToValue(ex)}, nil

	case *ir.LocalRef:
		return []value{f.locals[ex.Index]}, nil

	case *ir.ParamRef:
		v, ok := f.params.Values[ex.Name]
		if !ok {
			return nil, fmt.Errorf("unknown param %q", ex.Name)
		}
		return []value{paramToValue(ex.Typ, v)}, nil

	case *ir.BinaryOp:
		left, err := e.evalExpr(ex.Left, f)
		if err != nil {
			return nil, err
		}
		right, err := e.evalExpr(ex.Right, f)
		if err != nil {
			return nil, err
		}
		return []value{evalBinaryValue(ex.Op, ex.Typ, left, right)}, nil

	case *ir.UnaryOp:
		operand, err := e.evalExpr(ex.Operand, f)
		if err != nil {
			return nil, err
		}
		switch ex.Op {
		case ir.OpNeg:
			return []value{mapUnary(operand, func(x float64) float64 { return -x })}, nil
		case ir.OpNot:
			return []value{boolValue(!operand.truthy())}, nil
		default:
			return nil, fmt.Errorf("unknown unary op %v", ex.Op)
		}

	case *ir.Call:
		fn, ok := e.funcs[ex.Function]
		if !ok {
			return nil, fmt.Errorf("unknown function %q", ex.Function)
		}
		args := make([]value, len(ex.Args))
		for idx, arg := range ex.Args {
			v, err := e.evalExpr(arg, f)
			if err != nil {
				return nil, err
			}
			args[idx] = v
		}
		return e.execFunc(fn, args, f.params)

	case *ir.BuiltinCall:
		args := make([]value, len(ex.Args))
		for idx, arg := range ex.Args {
			v, err := e.evalExpr(arg, f)
			if err != nil {
				return nil, err
			}
			args[idx] = v
		}
		v, err := e.callBuiltin(ex.Builtin, args)
		if err != nil {
			return nil, err
		}
		return []value{v}, nil

	case *ir.TupleRef:
		vals, err := e.evalExprMulti(ex.Tuple, f)
		if err != nil {
			return nil, err
		}
		if ex.Index >= len(vals) {
			return nil, fmt.Errorf("tuple index %d out of range (len %d)", ex.Index, len(vals))
		}
		return []value{vals[ex.Index]}, nil

	case *ir.ComponentRef:
		base, err := e.evalExpr(ex.Vector, f)
		if err != nil {
			return nil, err
		}
		if ex.Index >= base.laneCount() {
			return nil, fmt.Errorf("component index %d out of range for %s", ex.Index, base.Typ)
		}
		return []value{base.component(ex.Index)}, nil

	default:
		return nil, fmt.Errorf("unknown expression type %T", expr)
	}
}

func constToValue(c *ir.Const) value {
	switch v := c.Value.(type) {
	case float64:
		return scalarValue(c.Typ, v)
	case float32:
		return scalarValue(c.Typ, float64(v))
	case int:
		return scalarValue(c.Typ, float64(v))
	case int64:
		return scalarValue(c.Typ, float64(v))
	case int32:
		return scalarValue(c.Typ, float64(v))
	case bool:
		return boolValue(v)
	default:
		return zeroValue(c.Typ)
	}
}

func paramToValue(typ ir.Type, raw any) value {
	switch typ {
	case ir.TypeBool:
		if b, ok := raw.(bool); ok {
			return boolValue(b)
		}
	case ir.TypeI32:
		return scalarValue(typ, anyToFloat64(raw))
	case ir.TypeF32:
		return scalarValue(typ, anyToFloat64(raw))
	}
	return scalarValue(typ, anyToFloat64(raw))
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

func evalBinaryValue(op ir.Op, resultType ir.Type, left, right value) value {
	switch op {
	case ir.OpEq:
		return boolValue(equalValues(left, right))
	case ir.OpNeq:
		return boolValue(!equalValues(left, right))
	case ir.OpLt:
		return boolValue(left.scalar() < right.scalar())
	case ir.OpGt:
		return boolValue(left.scalar() > right.scalar())
	case ir.OpLte:
		return boolValue(left.scalar() <= right.scalar())
	case ir.OpGte:
		return boolValue(left.scalar() >= right.scalar())
	case ir.OpAnd:
		return boolValue(left.truthy() && right.truthy())
	case ir.OpOr:
		return boolValue(left.truthy() || right.truthy())
	}

	left, right, target := liftBinary(left, right)
	if resultType != ir.TypeUnknown {
		target = resultType
	}
	switch op {
	case ir.OpAdd:
		return mapBinary(left, right, target, func(l, r float64) float64 { return l + r })
	case ir.OpSub:
		return mapBinary(left, right, target, func(l, r float64) float64 { return l - r })
	case ir.OpMul:
		return mapBinary(left, right, target, func(l, r float64) float64 { return l * r })
	case ir.OpDiv:
		return mapBinary(left, right, target, func(l, r float64) float64 {
			if r == 0 {
				return 0
			}
			return l / r
		})
	case ir.OpMod:
		return mapBinary(left, right, target, func(l, r float64) float64 {
			if r == 0 {
				return 0
			}
			return l - r*math.Floor(l/r)
		})
	default:
		return zeroValue(target)
	}
}

func equalValues(left, right value) bool {
	left, right, _ = liftBinary(left, right)
	switch left.Typ {
	case ir.TypeVec2:
		return left.Lanes[0] == right.Lanes[0] && left.Lanes[1] == right.Lanes[1]
	case ir.TypeVec3:
		return left.Lanes[0] == right.Lanes[0] && left.Lanes[1] == right.Lanes[1] && left.Lanes[2] == right.Lanes[2]
	case ir.TypeVec4:
		return left.Lanes[0] == right.Lanes[0] && left.Lanes[1] == right.Lanes[1] && left.Lanes[2] == right.Lanes[2] && left.Lanes[3] == right.Lanes[3]
	default:
		return left.Lanes[0] == right.Lanes[0]
	}
}

func clampOutput(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
