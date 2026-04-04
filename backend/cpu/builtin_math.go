package cpu

import (
	"fmt"
	"math"

	"github.com/BenStokmans/lfx/ir"
)

func (e *Evaluator) callBuiltin(id ir.BuiltinID, args []float64) ([]float64, error) {
	switch id {
	case ir.BuiltinAbs:
		return []float64{math.Abs(args[0])}, nil
	case ir.BuiltinMin:
		return []float64{math.Min(args[0], args[1])}, nil
	case ir.BuiltinMax:
		return []float64{math.Max(args[0], args[1])}, nil
	case ir.BuiltinFloor:
		return []float64{math.Floor(args[0])}, nil
	case ir.BuiltinCeil:
		return []float64{math.Ceil(args[0])}, nil
	case ir.BuiltinSqrt:
		return []float64{math.Sqrt(args[0])}, nil
	case ir.BuiltinSin:
		return []float64{math.Sin(args[0])}, nil
	case ir.BuiltinCos:
		return []float64{math.Cos(args[0])}, nil
	case ir.BuiltinClamp:
		return []float64{math.Max(args[1], math.Min(args[0], args[2]))}, nil
	case ir.BuiltinMix:
		// mix(a, b, t) = a + t*(b-a)
		return []float64{args[0] + args[2]*(args[1]-args[0])}, nil
	case ir.BuiltinFract:
		return []float64{args[0] - math.Floor(args[0])}, nil
	case ir.BuiltinMod:
		// GLSL-style mod: x - y*floor(x/y)
		if args[1] == 0 {
			return []float64{0}, nil
		}
		return []float64{args[0] - args[1]*math.Floor(args[0]/args[1])}, nil
	case ir.BuiltinPow:
		return []float64{math.Pow(args[0], args[1])}, nil
	case ir.BuiltinIsEven:
		if int(args[0])%2 == 0 {
			return []float64{1}, nil
		}
		return []float64{0}, nil
	case ir.BuiltinPerlin:
		return []float64{builtinPerlin(args)}, nil
	case ir.BuiltinVoronoi:
		return []float64{builtinVoronoi(args)}, nil
	case ir.BuiltinVoronoiBorder:
		return []float64{builtinVoronoiBorder(args)}, nil
	case ir.BuiltinWorley:
		return []float64{builtinWorley(args)}, nil
	default:
		return nil, fmt.Errorf("unknown builtin %v", id)
	}
}
