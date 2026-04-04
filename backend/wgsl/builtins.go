package wgsl

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
)

// emitBuiltinCall maps a builtin ID and pre-emitted argument strings to a WGSL expression.
func (e *Emitter) emitBuiltinCall(call *ir.BuiltinCall, args []string) string {
	switch call.Builtin {
	case ir.BuiltinAbs:
		return fmt.Sprintf("abs(%s)", args[0])
	case ir.BuiltinMin:
		return fmt.Sprintf("min(%s, %s)", e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType), e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType))
	case ir.BuiltinMax:
		return fmt.Sprintf("max(%s, %s)", e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType), e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType))
	case ir.BuiltinFloor:
		return fmt.Sprintf("floor(%s)", args[0])
	case ir.BuiltinCeil:
		return fmt.Sprintf("ceil(%s)", args[0])
	case ir.BuiltinSqrt:
		return fmt.Sprintf("sqrt(%s)", args[0])
	case ir.BuiltinSin:
		return fmt.Sprintf("sin(%s)", args[0])
	case ir.BuiltinCos:
		return fmt.Sprintf("cos(%s)", args[0])
	case ir.BuiltinClamp:
		return fmt.Sprintf(
			"clamp(%s, %s, %s)",
			e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType),
			e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType),
			e.coerceExpr(args[2], call.Args[2].ResultType(), call.ReturnType),
		)
	case ir.BuiltinMix:
		a := e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType)
		b := e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType)
		t := e.coerceExpr(args[2], call.Args[2].ResultType(), call.ReturnType)
		return fmt.Sprintf("(%s + %s * (%s - %s))", a, t, b, a)
	case ir.BuiltinFract:
		return fmt.Sprintf("fract(%s)", args[0])
	case ir.BuiltinMod:
		a := e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType)
		b := e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType)
		return fmt.Sprintf("(%s - %s * floor(%s / %s))", a, b, a, b)
	case ir.BuiltinPow:
		return fmt.Sprintf("pow(%s, %s)", e.coerceExpr(args[0], call.Args[0].ResultType(), call.ReturnType), e.coerceExpr(args[1], call.Args[1].ResultType(), call.ReturnType))
	case ir.BuiltinIsEven:
		return fmt.Sprintf("lfx_is_even(%s)", args[0])
	case ir.BuiltinVec2:
		return fmt.Sprintf("vec2<f32>(%s, %s)", args[0], args[1])
	case ir.BuiltinVec3:
		return fmt.Sprintf("vec3<f32>(%s, %s, %s)", args[0], args[1], args[2])
	case ir.BuiltinVec4:
		return fmt.Sprintf("vec4<f32>(%s, %s, %s, %s)", args[0], args[1], args[2], args[3])
	case ir.BuiltinDot:
		return fmt.Sprintf("dot(%s, %s)", args[0], args[1])
	case ir.BuiltinLength:
		return fmt.Sprintf("length(%s)", args[0])
	case ir.BuiltinDistance:
		return fmt.Sprintf("distance(%s, %s)", args[0], args[1])
	case ir.BuiltinNormalize:
		return fmt.Sprintf("normalize(%s)", args[0])
	case ir.BuiltinCross:
		return fmt.Sprintf("cross(%s, %s)", args[0], args[1])
	case ir.BuiltinProject:
		return fmt.Sprintf("(%s * (dot(%s, %s) / max(dot(%s, %s), 0.000001)))", args[1], args[0], args[1], args[1], args[1])
	case ir.BuiltinReflect:
		return fmt.Sprintf("reflect(%s, %s)", args[0], args[1])
	case ir.BuiltinPerlin:
		switch len(args) {
		case 1:
			if call.Args[0].ResultType().IsVector() {
				switch call.Args[0].ResultType().Lanes() {
				case 2:
					return fmt.Sprintf("lfx_perlin2(%s)", args[0])
				case 3:
					return fmt.Sprintf("lfx_perlin3(%s)", args[0])
				default:
					return fmt.Sprintf("lfx_perlin1(%s)", args[0])
				}
			}
			return fmt.Sprintf("lfx_perlin1(%s)", args[0])
		case 2:
			return fmt.Sprintf("lfx_perlin2(vec2<f32>(%s, %s))", args[0], args[1])
		case 3:
			return fmt.Sprintf("lfx_perlin3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		default:
			return "0.0"
		}
	case ir.BuiltinVoronoi:
		switch len(args) {
		case 1:
			return fmt.Sprintf("lfx_voronoi%d(%s)", call.Args[0].ResultType().Lanes(), args[0])
		case 2:
			return fmt.Sprintf("lfx_voronoi2(vec2<f32>(%s, %s))", args[0], args[1])
		case 3:
			return fmt.Sprintf("lfx_voronoi3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		default:
			return "0.0"
		}
	case ir.BuiltinVoronoiBorder:
		if len(args) == 1 && call.Args[0].ResultType().Lanes() == 3 {
			return fmt.Sprintf("lfx_voronoi_border3(%s)", args[0])
		}
		if len(args) == 3 {
			return fmt.Sprintf("lfx_voronoi_border3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		}
		return "0.0"
	case ir.BuiltinWorley:
		switch len(args) {
		case 1:
			return fmt.Sprintf("lfx_worley%d(%s)", call.Args[0].ResultType().Lanes(), args[0])
		case 2:
			return fmt.Sprintf("lfx_worley2(vec2<f32>(%s, %s))", args[0], args[1])
		case 3:
			return fmt.Sprintf("lfx_worley3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		case 4:
			return fmt.Sprintf("lfx_worley4(vec4<f32>(%s, %s, %s, %s))", args[0], args[1], args[2], args[3])
		default:
			return "0.0"
		}
	default:
		return fmt.Sprintf("/* unknown builtin %d */", int(call.Builtin))
	}
}

// emitHelpers writes any needed helper function definitions.
func (e *Emitter) emitHelpers() {
	if e.neededHelpers["mod"] {
		e.writeln("fn lfx_mod(x: f32, y: f32) -> f32 {")
		e.indent++
		e.writeln("return x - y * floor(x / y);")
		e.indent--
		e.writeln("}")
		e.writeln("")
	}

	if e.neededHelpers["mix"] {
		e.writeln("fn lfx_mix(a: f32, b: f32, t: f32) -> f32 {")
		e.indent++
		e.writeln("return a + t * (b - a);")
		e.indent--
		e.writeln("}")
		e.writeln("")
	}

	if e.neededHelpers["is_even"] {
		e.writeln("fn lfx_is_even(x: f32) -> f32 {")
		e.indent++
		e.writeln("return select(0.0, 1.0, (i32(x) % 2) == 0);")
		e.indent--
		e.writeln("}")
		e.writeln("")
	}

	if e.neededHelpers["perlin"] {
		e.emitPerlinHelpers()
	}

	if e.neededHelpers["voronoi"] {
		e.emitVoronoiHelpers()
	}

	if e.neededHelpers["worley"] {
		e.emitWorleyHelpers()
	}
}
