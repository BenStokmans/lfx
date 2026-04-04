package cpu

import (
	"fmt"
	"math"

	"github.com/BenStokmans/lfx/ir"
)

func (e *Evaluator) callBuiltin(id ir.BuiltinID, args []value) (value, error) {
	switch id {
	case ir.BuiltinVec2:
		return vectorValue(ir.TypeVec2, args[0].scalar(), args[1].scalar()), nil
	case ir.BuiltinVec3:
		return vectorValue(ir.TypeVec3, args[0].scalar(), args[1].scalar(), args[2].scalar()), nil
	case ir.BuiltinVec4:
		return vectorValue(ir.TypeVec4, args[0].scalar(), args[1].scalar(), args[2].scalar(), args[3].scalar()), nil
	case ir.BuiltinAbs:
		return applyLiftedBuiltin(args, func(x float64) float64 { return math.Abs(x) }), nil
	case ir.BuiltinFloor:
		return applyLiftedBuiltin(args, math.Floor), nil
	case ir.BuiltinCeil:
		return applyLiftedBuiltin(args, math.Ceil), nil
	case ir.BuiltinSqrt:
		return applyLiftedBuiltin(args, math.Sqrt), nil
	case ir.BuiltinSin:
		return applyLiftedBuiltin(args, math.Sin), nil
	case ir.BuiltinCos:
		return applyLiftedBuiltin(args, math.Cos), nil
	case ir.BuiltinFract:
		return applyLiftedBuiltin(args, func(x float64) float64 { return x - math.Floor(x) }), nil
	case ir.BuiltinMin:
		return applyLiftedBinaryBuiltin(args[0], args[1], math.Min), nil
	case ir.BuiltinMax:
		return applyLiftedBinaryBuiltin(args[0], args[1], math.Max), nil
	case ir.BuiltinClamp:
		return applyLiftedTernaryBuiltin(args[0], args[1], args[2], func(x, minV, maxV float64) float64 {
			return math.Max(minV, math.Min(x, maxV))
		}), nil
	case ir.BuiltinMix:
		return applyLiftedTernaryBuiltin(args[0], args[1], args[2], func(a, b, t float64) float64 {
			return a + t*(b-a)
		}), nil
	case ir.BuiltinMod:
		return applyLiftedBinaryBuiltin(args[0], args[1], func(x, y float64) float64 {
			if y == 0 {
				return 0
			}
			return x - y*math.Floor(x/y)
		}), nil
	case ir.BuiltinPow:
		return applyLiftedBinaryBuiltin(args[0], args[1], math.Pow), nil
	case ir.BuiltinIsEven:
		return boolValue(int(args[0].scalar())%2 == 0), nil
	case ir.BuiltinDot:
		return scalarValue(ir.TypeF32, dotValue(args[0], args[1])), nil
	case ir.BuiltinLength:
		if args[0].Typ.IsVector() {
			return scalarValue(ir.TypeF32, vectorLen(args[0])), nil
		}
		return scalarValue(ir.TypeF32, math.Abs(args[0].scalar())), nil
	case ir.BuiltinDistance:
		diff := evalBinaryValue(ir.OpSub, args[0].Typ, args[0], args[1])
		if diff.Typ.IsVector() {
			return scalarValue(ir.TypeF32, vectorLen(diff)), nil
		}
		return scalarValue(ir.TypeF32, math.Abs(diff.scalar())), nil
	case ir.BuiltinNormalize:
		return normalizeValue(args[0]), nil
	case ir.BuiltinCross:
		return vectorValue(
			ir.TypeVec3,
			args[0].Lanes[1]*args[1].Lanes[2]-args[0].Lanes[2]*args[1].Lanes[1],
			args[0].Lanes[2]*args[1].Lanes[0]-args[0].Lanes[0]*args[1].Lanes[2],
			args[0].Lanes[0]*args[1].Lanes[1]-args[0].Lanes[1]*args[1].Lanes[0],
		), nil
	case ir.BuiltinProject:
		denom := dotValue(args[1], args[1])
		if denom == 0 {
			return zeroValue(args[1].Typ), nil
		}
		scale := scalarValue(ir.TypeF32, dotValue(args[0], args[1])/denom)
		return evalBinaryValue(ir.OpMul, args[1].Typ, args[1], scale), nil
	case ir.BuiltinReflect:
		scale := scalarValue(ir.TypeF32, 2*dotValue(args[1], args[0]))
		projected := evalBinaryValue(ir.OpMul, args[1].Typ, args[1], scale)
		return evalBinaryValue(ir.OpSub, args[0].Typ, args[0], projected), nil
	case ir.BuiltinPerlin:
		return scalarValue(ir.TypeF32, builtinPerlin(flattenNoiseArgs(args))), nil
	case ir.BuiltinVoronoi:
		return scalarValue(ir.TypeF32, builtinVoronoi(flattenNoiseArgs(args))), nil
	case ir.BuiltinVoronoiBorder:
		return scalarValue(ir.TypeF32, builtinVoronoiBorder(flattenNoiseArgs(args))), nil
	case ir.BuiltinWorley:
		return scalarValue(ir.TypeF32, builtinWorley(flattenNoiseArgs(args))), nil
	default:
		return zeroValue(ir.TypeF32), fmt.Errorf("unknown builtin %v", id)
	}
}

func applyLiftedBuiltin(args []value, fn func(float64) float64) value {
	return mapUnary(args[0], fn)
}

func applyLiftedBinaryBuiltin(left, right value, fn func(float64, float64) float64) value {
	left, right, target := liftBinary(left, right)
	return mapBinary(left, right, target, fn)
}

func applyLiftedTernaryBuiltin(a, b, c value, fn func(float64, float64, float64) float64) value {
	a, b, target := liftBinary(a, b)
	if c.Typ.IsVector() {
		a = broadcastValue(a, c.Typ)
		b = broadcastValue(b, c.Typ)
		target = c.Typ
	} else {
		c = broadcastValue(c, target)
	}
	return mapTernary(a, b, c, target, fn)
}

func dotValue(left, right value) float64 {
	left, right, _ = liftBinary(left, right)
	switch left.Typ {
	case ir.TypeVec2:
		return left.Lanes[0]*right.Lanes[0] + left.Lanes[1]*right.Lanes[1]
	case ir.TypeVec3:
		return left.Lanes[0]*right.Lanes[0] + left.Lanes[1]*right.Lanes[1] + left.Lanes[2]*right.Lanes[2]
	case ir.TypeVec4:
		return left.Lanes[0]*right.Lanes[0] + left.Lanes[1]*right.Lanes[1] + left.Lanes[2]*right.Lanes[2] + left.Lanes[3]*right.Lanes[3]
	default:
		return left.Lanes[0] * right.Lanes[0]
	}
}

func normalizeValue(v value) value {
	if !v.Typ.IsVector() {
		length := math.Abs(v.scalar())
		if length == 0 {
			return zeroValue(v.Typ)
		}
		return scalarValue(v.Typ, v.scalar()/length)
	}
	length := vectorLen(v)
	if length == 0 {
		return zeroValue(v.Typ)
	}
	inv := 1 / length
	return mapUnary(v, func(x float64) float64 { return x * inv })
}

func flattenNoiseArgs(args []value) []float64 {
	if len(args) == 1 && args[0].Typ.IsVector() {
		out := make([]float64, args[0].laneCount())
		copy(out, args[0].Lanes[:args[0].laneCount()])
		return out
	}
	out := make([]float64, len(args))
	for idx, arg := range args {
		out[idx] = arg.scalar()
	}
	return out
}
