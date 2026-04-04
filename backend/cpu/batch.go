package cpu

import (
	"errors"
	"fmt"
	"math"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/runtime"
)

type batchValue struct {
	Typ   ir.Type
	Lanes [4][]float64
}

type batchFrame struct {
	locals     []batchValue
	params     *runtime.BoundParams
	count      int
	returnVals []batchValue
}

type batchUnsupportedError struct {
	reason string
}

func (e *batchUnsupportedError) Error() string {
	return e.reason
}

// SamplePoints evaluates many points at once. It uses the batched executor when
// the compiled IR stays within the currently supported straight-line subset and
// falls back to point-by-point interpretation otherwise.
func (e *Evaluator) SamplePoints(layout runtime.Layout, pointIndices []int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	if e.module == nil || e.module.Sample == nil {
		return nil, fmt.Errorf("sample function is not available")
	}
	if len(pointIndices) == 0 {
		return []float32{}, nil
	}

	results, err := e.samplePointsBatch(layout, pointIndices, phase, params)
	if err == nil {
		return results, nil
	}

	var unsupported *batchUnsupportedError
	if !errors.As(err, &unsupported) {
		return nil, err
	}

	channels := e.module.Output.Channels()
	out := make([]float32, len(pointIndices)*channels)
	for idx, pointIndex := range pointIndices {
		values, sampleErr := e.SamplePoint(layout, pointIndex, phase, params)
		if sampleErr != nil {
			return nil, sampleErr
		}
		copy(out[idx*channels:], values)
	}
	return out, nil
}

func (e *Evaluator) samplePointsBatch(layout runtime.Layout, pointIndices []int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	for _, pointIndex := range pointIndices {
		if pointIndex < 0 || pointIndex >= len(layout.Points) {
			return nil, fmt.Errorf("point index %d out of range", pointIndex)
		}
	}

	args := buildBatchArgs(layout, pointIndices, phase)
	results, err := e.execFuncBatch(e.module.Sample, args, params)
	if err != nil {
		return nil, err
	}

	channels := e.module.Output.Channels()
	out := make([]float32, len(pointIndices)*channels)
	switch {
	case len(results) == 1 && results[0].Typ.IsVector():
		if results[0].laneCount() < channels {
			return nil, fmt.Errorf("sample vector returned %d values, expected %d", results[0].laneCount(), channels)
		}
		for pointIdx := range pointIndices {
			base := pointIdx * channels
			for channel := 0; channel < channels; channel++ {
				out[base+channel] = float32(clampOutput(results[0].Lanes[channel][pointIdx]))
			}
		}
	case len(results) >= channels:
		for pointIdx := range pointIndices {
			base := pointIdx * channels
			for channel := 0; channel < channels; channel++ {
				out[base+channel] = float32(clampOutput(results[channel].Lanes[0][pointIdx]))
			}
		}
	default:
		return nil, fmt.Errorf("sample function returned %d values, expected %d", len(results), channels)
	}
	return out, nil
}

func buildBatchArgs(layout runtime.Layout, pointIndices []int, phase float32) []batchValue {
	count := len(pointIndices)
	xs := make([]float64, count)
	ys := make([]float64, count)
	indices := make([]float64, count)
	for idx, pointIndex := range pointIndices {
		pt := layout.Points[pointIndex]
		xs[idx] = float64(pt.X)
		ys[idx] = float64(pt.Y)
		indices[idx] = float64(pt.Index)
	}
	return []batchValue{
		filledBatchScalar(ir.TypeF32, count, float64(layout.Width)),
		filledBatchScalar(ir.TypeF32, count, float64(layout.Height)),
		scalarBatchValue(ir.TypeF32, xs),
		scalarBatchValue(ir.TypeF32, ys),
		scalarBatchValue(ir.TypeF32, indices),
		filledBatchScalar(ir.TypeF32, count, float64(phase)),
		filledBatchScalar(ir.TypeF32, count, 0),
	}
}

func (e *Evaluator) execFuncBatch(fn *ir.Function, args []batchValue, params *runtime.BoundParams) ([]batchValue, error) {
	if fn == nil {
		return nil, fmt.Errorf("function is nil")
	}
	count := 0
	if len(args) > 0 {
		count = args[0].count()
	}

	f := &batchFrame{
		locals: make([]batchValue, len(fn.Locals)),
		params: params,
		count:  count,
	}
	for idx := range fn.Locals {
		f.locals[idx] = zeroBatchValue(fn.Locals[idx].Type, count)
	}
	for idx, arg := range args {
		if idx < len(fn.Params) {
			f.locals[idx] = arg
		}
	}

	active := make([]bool, count)
	for idx := range active {
		active[idx] = true
	}
	active, err := e.execBlockBatch(fn.Body, f, active)
	if err != nil {
		return nil, err
	}
	if f.returnVals != nil {
		return f.returnVals, nil
	}
	if anyMask(active) {
		return []batchValue{filledBatchScalar(ir.TypeF32, count, 0)}, nil
	}
	return []batchValue{filledBatchScalar(ir.TypeF32, count, 0)}, nil
}

func (e *Evaluator) execBlockBatch(stmts []ir.IRStmt, f *batchFrame, active []bool) ([]bool, error) {
	current := cloneMask(active)
	for _, stmt := range stmts {
		if !anyMask(current) {
			return current, nil
		}
		next, err := e.execStmtBatch(stmt, f, current)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func (e *Evaluator) execStmtBatch(stmt ir.IRStmt, f *batchFrame, active []bool) ([]bool, error) {
	switch s := stmt.(type) {
	case *ir.LocalDecl:
		if s.Init != nil {
			v, err := e.evalExprBatch(s.Init, f)
			if err != nil {
				return nil, err
			}
			maskAssignBatchValue(&f.locals[s.Index], v, active)
		}

	case *ir.MultiLocalDecl:
		vals, err := e.evalExprMultiBatch(s.Source, f)
		if err != nil {
			return nil, err
		}
		for i, idx := range s.Indices {
			if i < len(vals) {
				maskAssignBatchValue(&f.locals[idx], vals[i], active)
			}
		}

	case *ir.Assign:
		v, err := e.evalExprBatch(s.Value, f)
		if err != nil {
			return nil, err
		}
		maskAssignBatchValue(&f.locals[s.Index], v, active)

	case *ir.Return:
		if len(s.Values) == 0 || !anyMask(active) {
			return clearMask(active), nil
		}
		vals := make([]batchValue, len(s.Values))
		for idx, expr := range s.Values {
			value, err := e.evalExprBatch(expr, f)
			if err != nil {
				return nil, err
			}
			vals[idx] = value
		}
		ensureBatchReturns(f, vals)
		for idx := range vals {
			maskAssignBatchValue(&f.returnVals[idx], vals[idx], active)
		}
		return clearMask(active), nil

	case *ir.ExprStmt:
		_, err := e.evalExprBatch(s.Expr, f)
		if err != nil {
			return nil, err
		}

	case *ir.IfStmt:
		cond, err := e.evalExprBatch(s.Cond, f)
		if err != nil {
			return nil, err
		}
		thenMask, remaining := splitMask(active, cond)
		activeAfter := clearMask(active)

		thenActive, err := e.execBlockBatch(s.Then, f, thenMask)
		if err != nil {
			return nil, err
		}
		mergeMasksInto(activeAfter, thenActive)

		for _, elif := range s.ElseIfs {
			if !anyMask(remaining) {
				break
			}
			elifCond, err := e.evalExprBatch(elif.Cond, f)
			if err != nil {
				return nil, err
			}
			elifMask, nextRemaining := splitMask(remaining, elifCond)
			elifActive, err := e.execBlockBatch(elif.Body, f, elifMask)
			if err != nil {
				return nil, err
			}
			mergeMasksInto(activeAfter, elifActive)
			remaining = nextRemaining
		}

		if len(s.ElseBody) > 0 && anyMask(remaining) {
			elseActive, err := e.execBlockBatch(s.ElseBody, f, remaining)
			if err != nil {
				return nil, err
			}
			mergeMasksInto(activeAfter, elseActive)
			remaining = clearMask(remaining)
		}
		mergeMasksInto(activeAfter, remaining)
		return activeAfter, nil

	default:
		return nil, &batchUnsupportedError{reason: fmt.Sprintf("batched evaluator does not support statement %T", stmt)}
	}

	return active, nil
}

func (e *Evaluator) evalExprBatch(expr ir.IRExpr, f *batchFrame) (batchValue, error) {
	vals, err := e.evalExprMultiBatch(expr, f)
	if err != nil {
		return batchValue{}, err
	}
	if len(vals) == 0 {
		return zeroBatchValue(ir.TypeF32, f.count), nil
	}
	return vals[0], nil
}

func (e *Evaluator) evalExprMultiBatch(expr ir.IRExpr, f *batchFrame) ([]batchValue, error) {
	switch ex := expr.(type) {
	case *ir.Const:
		return []batchValue{constToBatchValue(ex, f.count)}, nil

	case *ir.LocalRef:
		return []batchValue{f.locals[ex.Index]}, nil

	case *ir.ParamRef:
		v, ok := f.params.Values[ex.Name]
		if !ok {
			return nil, fmt.Errorf("unknown param %q", ex.Name)
		}
		return []batchValue{paramToBatchValue(ex.Typ, v, f.count)}, nil

	case *ir.BinaryOp:
		left, err := e.evalExprBatch(ex.Left, f)
		if err != nil {
			return nil, err
		}
		right, err := e.evalExprBatch(ex.Right, f)
		if err != nil {
			return nil, err
		}
		return []batchValue{evalBinaryBatchValue(ex.Op, ex.Typ, left, right)}, nil

	case *ir.UnaryOp:
		operand, err := e.evalExprBatch(ex.Operand, f)
		if err != nil {
			return nil, err
		}
		switch ex.Op {
		case ir.OpNeg:
			return []batchValue{mapUnaryBatch(operand, func(x float64) float64 { return -x })}, nil
		case ir.OpNot:
			return []batchValue{mapUnaryBatch(operand, func(x float64) float64 {
				if x == 0 {
					return 1
				}
				return 0
			})}, nil
		default:
			return nil, &batchUnsupportedError{reason: fmt.Sprintf("batched evaluator does not support unary op %v", ex.Op)}
		}

	case *ir.Call:
		fn, ok := e.funcs[ex.Function]
		if !ok {
			return nil, fmt.Errorf("unknown function %q", ex.Function)
		}
		args := make([]batchValue, len(ex.Args))
		for idx, arg := range ex.Args {
			v, err := e.evalExprBatch(arg, f)
			if err != nil {
				return nil, err
			}
			args[idx] = v
		}
		return e.execFuncBatch(fn, args, f.params)

	case *ir.BuiltinCall:
		args := make([]batchValue, len(ex.Args))
		for idx, arg := range ex.Args {
			v, err := e.evalExprBatch(arg, f)
			if err != nil {
				return nil, err
			}
			args[idx] = v
		}
		v, err := callBuiltinBatch(ex.Builtin, args)
		if err != nil {
			return nil, err
		}
		return []batchValue{v}, nil

	case *ir.TupleRef:
		vals, err := e.evalExprMultiBatch(ex.Tuple, f)
		if err != nil {
			return nil, err
		}
		if ex.Index >= len(vals) {
			return nil, fmt.Errorf("tuple index %d out of range (len %d)", ex.Index, len(vals))
		}
		return []batchValue{vals[ex.Index]}, nil

	case *ir.ComponentRef:
		base, err := e.evalExprBatch(ex.Vector, f)
		if err != nil {
			return nil, err
		}
		if ex.Index >= base.laneCount() {
			return nil, fmt.Errorf("component index %d out of range for %s", ex.Index, base.Typ)
		}
		return []batchValue{{Typ: ir.TypeF32, Lanes: [4][]float64{cloneLane(base.Lanes[ex.Index])}}}, nil

	default:
		return nil, &batchUnsupportedError{reason: fmt.Sprintf("batched evaluator does not support expression %T", expr)}
	}
}

func callBuiltinBatch(id ir.BuiltinID, args []batchValue) (batchValue, error) {
	switch id {
	case ir.BuiltinVec2:
		return vectorBatchValue(ir.TypeVec2, args[0].Lanes[0], args[1].Lanes[0]), nil
	case ir.BuiltinVec3:
		return vectorBatchValue(ir.TypeVec3, args[0].Lanes[0], args[1].Lanes[0], args[2].Lanes[0]), nil
	case ir.BuiltinVec4:
		return vectorBatchValue(ir.TypeVec4, args[0].Lanes[0], args[1].Lanes[0], args[2].Lanes[0], args[3].Lanes[0]), nil
	case ir.BuiltinAbs:
		return applyLiftedBatchBuiltin(args[0], math.Abs), nil
	case ir.BuiltinMin:
		return applyLiftedBatchMinBuiltin(args[0], args[1]), nil
	case ir.BuiltinMax:
		return applyLiftedBatchMaxBuiltin(args[0], args[1]), nil
	case ir.BuiltinFloor:
		return applyLiftedBatchBuiltin(args[0], math.Floor), nil
	case ir.BuiltinCeil:
		return applyLiftedBatchBuiltin(args[0], math.Ceil), nil
	case ir.BuiltinSqrt:
		return applyLiftedBatchBuiltin(args[0], math.Sqrt), nil
	case ir.BuiltinSin:
		return applyLiftedBatchBuiltin(args[0], math.Sin), nil
	case ir.BuiltinCos:
		return applyLiftedBatchBuiltin(args[0], math.Cos), nil
	case ir.BuiltinClamp:
		return applyLiftedBatchTernaryBuiltin(args[0], args[1], args[2], func(x, minV, maxV float64) float64 {
			return math.Max(minV, math.Min(x, maxV))
		}), nil
	case ir.BuiltinMix:
		return applyLiftedBatchTernaryBuiltin(args[0], args[1], args[2], func(a, b, t float64) float64 {
			return a + t*(b-a)
		}), nil
	case ir.BuiltinFract:
		return applyLiftedBatchBuiltin(args[0], func(x float64) float64 { return x - math.Floor(x) }), nil
	case ir.BuiltinMod:
		return applyLiftedBatchBinaryBuiltin(args[0], args[1], func(x, y float64) float64 {
			if y == 0 {
				return 0
			}
			return x - y*math.Floor(x/y)
		}), nil
	case ir.BuiltinPow:
		return applyLiftedBatchBinaryBuiltin(args[0], args[1], math.Pow), nil
	case ir.BuiltinIsEven:
		out := zeroBatchValue(ir.TypeBool, args[0].count())
		for idx, x := range args[0].Lanes[0] {
			if int(x)%2 == 0 {
				out.Lanes[0][idx] = 1
			}
		}
		return out, nil
	case ir.BuiltinDot:
		return scalarBatchValue(ir.TypeF32, dotBatchValue(args[0], args[1])), nil
	case ir.BuiltinLength:
		if args[0].Typ.IsVector() {
			return scalarBatchValue(ir.TypeF32, vectorLenBatch(args[0])), nil
		}
		return applyLiftedBatchBuiltin(args[0], math.Abs), nil
	case ir.BuiltinDistance:
		diff := evalBinaryBatchValue(ir.OpSub, args[0].Typ, args[0], args[1])
		if diff.Typ.IsVector() {
			return scalarBatchValue(ir.TypeF32, vectorLenBatch(diff)), nil
		}
		return applyLiftedBatchBuiltin(diff, math.Abs), nil
	case ir.BuiltinNormalize:
		return normalizeBatchValue(args[0]), nil
	case ir.BuiltinCross:
		count := args[0].count()
		out := zeroBatchValue(ir.TypeVec3, count)
		for idx := 0; idx < count; idx++ {
			out.Lanes[0][idx] = args[0].Lanes[1][idx]*args[1].Lanes[2][idx] - args[0].Lanes[2][idx]*args[1].Lanes[1][idx]
			out.Lanes[1][idx] = args[0].Lanes[2][idx]*args[1].Lanes[0][idx] - args[0].Lanes[0][idx]*args[1].Lanes[2][idx]
			out.Lanes[2][idx] = args[0].Lanes[0][idx]*args[1].Lanes[1][idx] - args[0].Lanes[1][idx]*args[1].Lanes[0][idx]
		}
		return out, nil
	case ir.BuiltinProject:
		denom := dotBatchValue(args[1], args[1])
		scale := make([]float64, len(denom))
		for idx, value := range denom {
			if value != 0 {
				scale[idx] = dotBatchValueAt(args[0], args[1], idx) / value
			}
		}
		return evalBinaryBatchValue(ir.OpMul, args[1].Typ, args[1], scalarBatchValue(ir.TypeF32, scale)), nil
	case ir.BuiltinReflect:
		scale := make([]float64, args[0].count())
		for idx := range scale {
			scale[idx] = 2 * dotBatchValueAt(args[1], args[0], idx)
		}
		projected := evalBinaryBatchValue(ir.OpMul, args[1].Typ, args[1], scalarBatchValue(ir.TypeF32, scale))
		return evalBinaryBatchValue(ir.OpSub, args[0].Typ, args[0], projected), nil
	case ir.BuiltinPerlin:
		return scalarBatchValue(ir.TypeF32, evalPerlinBuiltinBatch(args)), nil
	case ir.BuiltinVoronoi:
		return scalarBatchValue(ir.TypeF32, evalVoronoiBuiltinBatch(args)), nil
	case ir.BuiltinVoronoiBorder:
		return scalarBatchValue(ir.TypeF32, evalVoronoiBorderBuiltinBatch(args)), nil
	case ir.BuiltinWorley:
		return scalarBatchValue(ir.TypeF32, evalWorleyBuiltinBatch(args)), nil
	default:
		return batchValue{}, &batchUnsupportedError{reason: fmt.Sprintf("batched evaluator does not support builtin %v", id)}
	}
}

func zeroBatchValue(typ ir.Type, count int) batchValue {
	if typ == ir.TypeUnknown || typ == ir.TypeVoid {
		typ = ir.TypeF32
	}
	out := batchValue{Typ: typ}
	lanes := typ.Lanes()
	for lane := 0; lane < lanes; lane++ {
		out.Lanes[lane] = make([]float64, count)
	}
	return out
}

func scalarBatchValue(typ ir.Type, values []float64) batchValue {
	if typ == ir.TypeUnknown || typ.IsVector() || typ == ir.TypeVoid {
		typ = ir.TypeF32
	}
	return batchValue{
		Typ:   typ,
		Lanes: [4][]float64{cloneLane(values)},
	}
}

func filledBatchScalar(typ ir.Type, count int, value float64) batchValue {
	out := zeroBatchValue(typ, count)
	for idx := range out.Lanes[0] {
		out.Lanes[0][idx] = value
	}
	return out
}

func vectorBatchValue(typ ir.Type, lanes ...[]float64) batchValue {
	out := batchValue{Typ: typ}
	for idx := 0; idx < len(lanes) && idx < 4; idx++ {
		out.Lanes[idx] = cloneLane(lanes[idx])
	}
	return out
}

func (v batchValue) count() int {
	for _, lane := range v.Lanes {
		if lane != nil {
			return len(lane)
		}
	}
	return 0
}

func (v batchValue) laneCount() int {
	if v.Typ.IsVector() {
		return v.Typ.Lanes()
	}
	return 1
}

func broadcastBatchValue(v batchValue, target ir.Type) batchValue {
	if !target.IsVector() || v.Typ.IsVector() {
		return v
	}
	count := v.count()
	out := zeroBatchValue(target, count)
	for lane := 0; lane < target.Lanes(); lane++ {
		copy(out.Lanes[lane], v.Lanes[0])
	}
	return out
}

func liftBatchBinary(left, right batchValue) (batchValue, batchValue, ir.Type) {
	if left.Typ.IsVector() {
		return left, broadcastBatchValue(right, left.Typ), left.Typ
	}
	if right.Typ.IsVector() {
		return broadcastBatchValue(left, right.Typ), right, right.Typ
	}
	return left, right, mergeScalarTypes(left.Typ, right.Typ)
}

func mapUnaryBatch(v batchValue, fn func(float64) float64) batchValue {
	out := zeroBatchValue(v.Typ, v.count())
	for lane := 0; lane < v.laneCount(); lane++ {
		for idx, x := range v.Lanes[lane] {
			out.Lanes[lane][idx] = fn(x)
		}
	}
	return out
}

func mapBinaryBatch(left, right batchValue, target ir.Type, fn func(float64, float64) float64) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		for idx := range out.Lanes[lane] {
			out.Lanes[lane][idx] = fn(left.Lanes[lane][idx], right.Lanes[lane][idx])
		}
	}
	return out
}

func mapBinaryBatchAdd(left, right batchValue, target ir.Type) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		simdAddFloat64(left.Lanes[lane], right.Lanes[lane], out.Lanes[lane])
	}
	return out
}

func mapBinaryBatchSub(left, right batchValue, target ir.Type) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		simdSubFloat64(left.Lanes[lane], right.Lanes[lane], out.Lanes[lane])
	}
	return out
}

func mapBinaryBatchMul(left, right batchValue, target ir.Type) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		simdMulFloat64(left.Lanes[lane], right.Lanes[lane], out.Lanes[lane])
	}
	return out
}

func mapBinaryBatchMin(left, right batchValue, target ir.Type) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		simdMinFloat64(left.Lanes[lane], right.Lanes[lane], out.Lanes[lane])
	}
	return out
}

func mapBinaryBatchMax(left, right batchValue, target ir.Type) batchValue {
	out := zeroBatchValue(target, left.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		simdMaxFloat64(left.Lanes[lane], right.Lanes[lane], out.Lanes[lane])
	}
	return out
}

func mapTernaryBatch(a, b, c batchValue, target ir.Type, fn func(float64, float64, float64) float64) batchValue {
	out := zeroBatchValue(target, a.count())
	for lane := 0; lane < out.laneCount(); lane++ {
		for idx := range out.Lanes[lane] {
			out.Lanes[lane][idx] = fn(a.Lanes[lane][idx], b.Lanes[lane][idx], c.Lanes[lane][idx])
		}
	}
	return out
}

func applyLiftedBatchBuiltin(arg batchValue, fn func(float64) float64) batchValue {
	return mapUnaryBatch(arg, fn)
}

func applyLiftedBatchBinaryBuiltin(left, right batchValue, fn func(float64, float64) float64) batchValue {
	left, right, target := liftBatchBinary(left, right)
	return mapBinaryBatch(left, right, target, fn)
}

func applyLiftedBatchMinBuiltin(left, right batchValue) batchValue {
	left, right, target := liftBatchBinary(left, right)
	return mapBinaryBatchMin(left, right, target)
}

func applyLiftedBatchMaxBuiltin(left, right batchValue) batchValue {
	left, right, target := liftBatchBinary(left, right)
	return mapBinaryBatchMax(left, right, target)
}

func applyLiftedBatchTernaryBuiltin(a, b, c batchValue, fn func(float64, float64, float64) float64) batchValue {
	a, b, target := liftBatchBinary(a, b)
	if c.Typ.IsVector() {
		a = broadcastBatchValue(a, c.Typ)
		b = broadcastBatchValue(b, c.Typ)
		target = c.Typ
	} else {
		c = broadcastBatchValue(c, target)
	}
	return mapTernaryBatch(a, b, c, target, fn)
}

func vectorLenBatch(v batchValue) []float64 {
	count := v.count()
	out := make([]float64, count)
	switch v.Typ {
	case ir.TypeVec2:
		for idx := range out {
			x := v.Lanes[0][idx]
			y := v.Lanes[1][idx]
			out[idx] = math.Sqrt(x*x + y*y)
		}
	case ir.TypeVec3:
		for idx := range out {
			x := v.Lanes[0][idx]
			y := v.Lanes[1][idx]
			z := v.Lanes[2][idx]
			out[idx] = math.Sqrt(x*x + y*y + z*z)
		}
	case ir.TypeVec4:
		for idx := range out {
			x := v.Lanes[0][idx]
			y := v.Lanes[1][idx]
			z := v.Lanes[2][idx]
			w := v.Lanes[3][idx]
			out[idx] = math.Sqrt(x*x + y*y + z*z + w*w)
		}
	default:
		for idx := range out {
			x := v.Lanes[0][idx]
			out[idx] = math.Sqrt(x * x)
		}
	}
	return out
}

func dotBatchValue(left, right batchValue) []float64 {
	left, right, _ = liftBatchBinary(left, right)
	out := make([]float64, left.count())
	for idx := range out {
		out[idx] = dotBatchValueAt(left, right, idx)
	}
	return out
}

func dotBatchValueAt(left, right batchValue, idx int) float64 {
	switch left.Typ {
	case ir.TypeVec2:
		return left.Lanes[0][idx]*right.Lanes[0][idx] + left.Lanes[1][idx]*right.Lanes[1][idx]
	case ir.TypeVec3:
		return left.Lanes[0][idx]*right.Lanes[0][idx] + left.Lanes[1][idx]*right.Lanes[1][idx] + left.Lanes[2][idx]*right.Lanes[2][idx]
	case ir.TypeVec4:
		return left.Lanes[0][idx]*right.Lanes[0][idx] + left.Lanes[1][idx]*right.Lanes[1][idx] + left.Lanes[2][idx]*right.Lanes[2][idx] + left.Lanes[3][idx]*right.Lanes[3][idx]
	default:
		return left.Lanes[0][idx] * right.Lanes[0][idx]
	}
}

func normalizeBatchValue(v batchValue) batchValue {
	if !v.Typ.IsVector() {
		return mapUnaryBatch(v, func(x float64) float64 {
			length := math.Abs(x)
			if length == 0 {
				return 0
			}
			return x / length
		})
	}
	lengths := vectorLenBatch(v)
	out := zeroBatchValue(v.Typ, v.count())
	for lane := 0; lane < v.laneCount(); lane++ {
		for idx, x := range v.Lanes[lane] {
			if lengths[idx] == 0 {
				out.Lanes[lane][idx] = 0
				continue
			}
			out.Lanes[lane][idx] = x / lengths[idx]
		}
	}
	return out
}

func evalBinaryBatchValue(op ir.Op, resultType ir.Type, left, right batchValue) batchValue {
	switch op {
	case ir.OpEq:
		return compareBatch(left, right, func(l, r float64) bool { return l == r }, equalBatchValues)
	case ir.OpNeq:
		return compareBatch(left, right, func(l, r float64) bool { return l != r }, func(a, b batchValue, idx int) bool { return !equalBatchValues(a, b, idx) })
	case ir.OpLt:
		return compareBatch(left, right, func(l, r float64) bool { return l < r }, nil)
	case ir.OpGt:
		return compareBatch(left, right, func(l, r float64) bool { return l > r }, nil)
	case ir.OpLte:
		return compareBatch(left, right, func(l, r float64) bool { return l <= r }, nil)
	case ir.OpGte:
		return compareBatch(left, right, func(l, r float64) bool { return l >= r }, nil)
	case ir.OpAnd:
		return compareBatch(left, right, func(l, r float64) bool { return l != 0 && r != 0 }, nil)
	case ir.OpOr:
		return compareBatch(left, right, func(l, r float64) bool { return l != 0 || r != 0 }, nil)
	}

	left, right, target := liftBatchBinary(left, right)
	if resultType != ir.TypeUnknown {
		target = resultType
	}
	switch op {
	case ir.OpAdd:
		return mapBinaryBatchAdd(left, right, target)
	case ir.OpSub:
		return mapBinaryBatchSub(left, right, target)
	case ir.OpMul:
		return mapBinaryBatchMul(left, right, target)
	case ir.OpDiv:
		return mapBinaryBatch(left, right, target, func(l, r float64) float64 {
			if r == 0 {
				return 0
			}
			return l / r
		})
	case ir.OpMod:
		return mapBinaryBatch(left, right, target, func(l, r float64) float64 {
			if r == 0 {
				return 0
			}
			return l - r*math.Floor(l/r)
		})
	default:
		return zeroBatchValue(target, left.count())
	}
}

func compareBatch(left, right batchValue, scalarFn func(float64, float64) bool, vectorFn func(batchValue, batchValue, int) bool) batchValue {
	left, right, _ = liftBatchBinary(left, right)
	out := zeroBatchValue(ir.TypeBool, left.count())
	if vectorFn != nil && left.Typ.IsVector() {
		for idx := range out.Lanes[0] {
			if vectorFn(left, right, idx) {
				out.Lanes[0][idx] = 1
			}
		}
		return out
	}
	for idx := range out.Lanes[0] {
		if scalarFn(left.Lanes[0][idx], right.Lanes[0][idx]) {
			out.Lanes[0][idx] = 1
		}
	}
	return out
}

func equalBatchValues(left, right batchValue, idx int) bool {
	switch left.Typ {
	case ir.TypeVec2:
		return left.Lanes[0][idx] == right.Lanes[0][idx] && left.Lanes[1][idx] == right.Lanes[1][idx]
	case ir.TypeVec3:
		return left.Lanes[0][idx] == right.Lanes[0][idx] && left.Lanes[1][idx] == right.Lanes[1][idx] && left.Lanes[2][idx] == right.Lanes[2][idx]
	case ir.TypeVec4:
		return left.Lanes[0][idx] == right.Lanes[0][idx] && left.Lanes[1][idx] == right.Lanes[1][idx] && left.Lanes[2][idx] == right.Lanes[2][idx] && left.Lanes[3][idx] == right.Lanes[3][idx]
	default:
		return left.Lanes[0][idx] == right.Lanes[0][idx]
	}
}

func constToBatchValue(c *ir.Const, count int) batchValue {
	switch v := c.Value.(type) {
	case float64:
		return filledBatchScalar(c.Typ, count, v)
	case float32:
		return filledBatchScalar(c.Typ, count, float64(v))
	case int:
		return filledBatchScalar(c.Typ, count, float64(v))
	case int64:
		return filledBatchScalar(c.Typ, count, float64(v))
	case int32:
		return filledBatchScalar(c.Typ, count, float64(v))
	case bool:
		if v {
			return filledBatchScalar(ir.TypeBool, count, 1)
		}
		return filledBatchScalar(ir.TypeBool, count, 0)
	default:
		return zeroBatchValue(c.Typ, count)
	}
}

func paramToBatchValue(typ ir.Type, raw any, count int) batchValue {
	switch typ {
	case ir.TypeBool:
		if b, ok := raw.(bool); ok {
			if b {
				return filledBatchScalar(ir.TypeBool, count, 1)
			}
			return filledBatchScalar(ir.TypeBool, count, 0)
		}
	case ir.TypeI32, ir.TypeF32:
		return filledBatchScalar(typ, count, anyToFloat64(raw))
	}
	return filledBatchScalar(typ, count, anyToFloat64(raw))
}

func cloneLane(values []float64) []float64 {
	if values == nil {
		return nil
	}
	out := make([]float64, len(values))
	copy(out, values)
	return out
}

func evalPerlinBuiltinBatch(args []batchValue) []float64 {
	count := batchArgCount(args)
	out := make([]float64, count)
	if len(args) == 1 && args[0].Typ.IsVector() {
		switch args[0].Typ {
		case ir.TypeVec2:
			for idx := range out {
				out[idx] = builtinPerlin2(args[0].Lanes[0][idx], args[0].Lanes[1][idx])
			}
		case ir.TypeVec3:
			for idx := range out {
				out[idx] = builtinPerlin3(args[0].Lanes[0][idx], args[0].Lanes[1][idx], args[0].Lanes[2][idx])
			}
		default:
			for idx := range out {
				out[idx] = builtinPerlin1(args[0].Lanes[0][idx])
			}
		}
		return out
	}
	switch len(args) {
	case 1:
		for idx := range out {
			out[idx] = builtinPerlin1(args[0].Lanes[0][idx])
		}
	case 2:
		for idx := range out {
			out[idx] = builtinPerlin2(args[0].Lanes[0][idx], args[1].Lanes[0][idx])
		}
	case 3:
		for idx := range out {
			out[idx] = builtinPerlin3(args[0].Lanes[0][idx], args[1].Lanes[0][idx], args[2].Lanes[0][idx])
		}
	}
	return out
}

func evalVoronoiBuiltinBatch(args []batchValue) []float64 {
	count := batchArgCount(args)
	out := make([]float64, count)
	if len(args) == 1 && args[0].Typ.IsVector() {
		switch args[0].Typ {
		case ir.TypeVec2:
			for idx := range out {
				out[idx] = builtinVoronoi2(args[0].Lanes[0][idx], args[0].Lanes[1][idx])
			}
		case ir.TypeVec3:
			for idx := range out {
				out[idx] = builtinVoronoi3(args[0].Lanes[0][idx], args[0].Lanes[1][idx], args[0].Lanes[2][idx])
			}
		}
		return out
	}
	switch len(args) {
	case 2:
		for idx := range out {
			out[idx] = builtinVoronoi2(args[0].Lanes[0][idx], args[1].Lanes[0][idx])
		}
	case 3:
		for idx := range out {
			out[idx] = builtinVoronoi3(args[0].Lanes[0][idx], args[1].Lanes[0][idx], args[2].Lanes[0][idx])
		}
	}
	return out
}

func evalVoronoiBorderBuiltinBatch(args []batchValue) []float64 {
	count := batchArgCount(args)
	out := make([]float64, count)
	if len(args) == 1 && args[0].Typ == ir.TypeVec3 {
		for idx := range out {
			out[idx] = builtinVoronoiBorder3(args[0].Lanes[0][idx], args[0].Lanes[1][idx], args[0].Lanes[2][idx])
		}
		return out
	}
	if len(args) == 3 {
		for idx := range out {
			out[idx] = builtinVoronoiBorder3(args[0].Lanes[0][idx], args[1].Lanes[0][idx], args[2].Lanes[0][idx])
		}
	}
	return out
}

func evalWorleyBuiltinBatch(args []batchValue) []float64 {
	count := batchArgCount(args)
	out := make([]float64, count)
	if len(args) == 1 && args[0].Typ.IsVector() {
		switch args[0].Typ {
		case ir.TypeVec2:
			for idx := range out {
				out[idx] = builtinWorley2(args[0].Lanes[0][idx], args[0].Lanes[1][idx])
			}
		case ir.TypeVec3:
			for idx := range out {
				out[idx] = builtinWorley3(args[0].Lanes[0][idx], args[0].Lanes[1][idx], args[0].Lanes[2][idx])
			}
		case ir.TypeVec4:
			for idx := range out {
				out[idx] = builtinWorley4(args[0].Lanes[0][idx], args[0].Lanes[1][idx], args[0].Lanes[2][idx], args[0].Lanes[3][idx])
			}
		}
		return out
	}
	switch len(args) {
	case 2:
		for idx := range out {
			out[idx] = builtinWorley2(args[0].Lanes[0][idx], args[1].Lanes[0][idx])
		}
	case 3:
		for idx := range out {
			out[idx] = builtinWorley3(args[0].Lanes[0][idx], args[1].Lanes[0][idx], args[2].Lanes[0][idx])
		}
	case 4:
		for idx := range out {
			out[idx] = builtinWorley4(args[0].Lanes[0][idx], args[1].Lanes[0][idx], args[2].Lanes[0][idx], args[3].Lanes[0][idx])
		}
	}
	return out
}

func batchArgCount(args []batchValue) int {
	for _, arg := range args {
		if count := arg.count(); count != 0 {
			return count
		}
	}
	return 0
}

func ensureBatchReturns(f *batchFrame, values []batchValue) {
	if f.returnVals != nil {
		return
	}
	f.returnVals = make([]batchValue, len(values))
	for idx, value := range values {
		f.returnVals[idx] = zeroBatchValue(value.Typ, f.count)
	}
}

func maskAssignBatchValue(dst *batchValue, src batchValue, mask []bool) {
	for lane := 0; lane < src.laneCount(); lane++ {
		if dst.Lanes[lane] == nil {
			dst.Lanes[lane] = make([]float64, len(src.Lanes[lane]))
		}
		for idx, active := range mask {
			if active {
				dst.Lanes[lane][idx] = src.Lanes[lane][idx]
			}
		}
	}
	dst.Typ = src.Typ
}

func splitMask(active []bool, cond batchValue) ([]bool, []bool) {
	thenMask := make([]bool, len(active))
	remaining := make([]bool, len(active))
	for idx, on := range active {
		if !on {
			continue
		}
		if cond.Lanes[0][idx] != 0 {
			thenMask[idx] = true
		} else {
			remaining[idx] = true
		}
	}
	return thenMask, remaining
}

func clearMask(src []bool) []bool {
	return make([]bool, len(src))
}

func cloneMask(src []bool) []bool {
	out := make([]bool, len(src))
	copy(out, src)
	return out
}

func mergeMasksInto(dst, src []bool) {
	for idx, active := range src {
		if active {
			dst[idx] = true
		}
	}
}

func anyMask(mask []bool) bool {
	for _, active := range mask {
		if active {
			return true
		}
	}
	return false
}
