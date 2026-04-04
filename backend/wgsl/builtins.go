package wgsl

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
)

// emitBuiltinCall maps a builtin ID and pre-emitted argument strings to a WGSL expression.
func (e *Emitter) emitBuiltinCall(id ir.BuiltinID, args []string) string {
	switch id {
	case ir.BuiltinAbs:
		return fmt.Sprintf("abs(%s)", args[0])
	case ir.BuiltinMin:
		return fmt.Sprintf("min(%s, %s)", args[0], args[1])
	case ir.BuiltinMax:
		return fmt.Sprintf("max(%s, %s)", args[0], args[1])
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
		return fmt.Sprintf("clamp(%s, %s, %s)", args[0], args[1], args[2])
	case ir.BuiltinMix:
		return fmt.Sprintf("lfx_mix(%s, %s, %s)", args[0], args[1], args[2])
	case ir.BuiltinFract:
		return fmt.Sprintf("fract(%s)", args[0])
	case ir.BuiltinMod:
		return fmt.Sprintf("lfx_mod(%s, %s)", args[0], args[1])
	case ir.BuiltinPow:
		return fmt.Sprintf("pow(%s, %s)", args[0], args[1])
	case ir.BuiltinIsEven:
		return fmt.Sprintf("lfx_is_even(%s)", args[0])
	case ir.BuiltinPerlin:
		switch len(args) {
		case 1:
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
		case 2:
			return fmt.Sprintf("lfx_voronoi2(vec2<f32>(%s, %s))", args[0], args[1])
		case 3:
			return fmt.Sprintf("lfx_voronoi3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		default:
			return "0.0"
		}
	case ir.BuiltinVoronoiBorder:
		if len(args) == 3 {
			return fmt.Sprintf("lfx_voronoi_border3(vec3<f32>(%s, %s, %s))", args[0], args[1], args[2])
		}
		return "0.0"
	case ir.BuiltinWorley:
		switch len(args) {
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
		return fmt.Sprintf("/* unknown builtin %d */", int(id))
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
