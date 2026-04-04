package wgsl

import (
	"fmt"
	"strings"

	"github.com/BenStokmans/lfx/ir"
)

// Emitter lowers flattened IR into a WGSL compute shader.
type Emitter struct {
	mod    *ir.Module
	buf    strings.Builder
	indent int

	// currentFunc is the function currently being emitted (needed for Assign index->name lookup).
	currentFunc *ir.Function

	// neededHelpers tracks which helper functions must be emitted.
	neededHelpers map[string]bool

	// reachable tracks functions transitively called from sample.
	reachable map[*ir.Function]bool

	// multiRetCounter generates unique temp variable names.
	multiRetCounter int
}

// Emit lowers the given IR module into a complete WGSL compute shader string.
func Emit(mod *ir.Module) (string, error) {
	e := &Emitter{
		mod:           mod,
		neededHelpers: make(map[string]bool),
		reachable:     make(map[*ir.Function]bool),
	}

	e.markReachable()

	// First pass: scan only reachable functions for needed helpers.
	for _, fn := range mod.Functions {
		if !e.reachable[fn] {
			continue
		}
		e.scanFunction(fn)
	}

	// Emit header.
	e.writef("// LFX generated WGSL - %s\n\n", mod.Name)

	// Emit structs.
	e.emitStructs()

	// Emit bindings.
	e.writeln("@group(0) @binding(0) var<storage, read> points: array<Point>;")
	e.writeln("@group(0) @binding(1) var<uniform> uniforms: Uniforms;")
	e.writeln("@group(0) @binding(2) var<storage, read_write> output: array<f32>;")
	e.writeln("")

	// Emit needed helper functions.
	e.emitHelpers()

	// Emit user-defined functions (excluding sample).
	for _, fn := range mod.Functions {
		if fn == mod.Sample || !e.reachable[fn] {
			continue
		}
		if err := e.emitFunction(fn); err != nil {
			return "", fmt.Errorf("emitting function %s: %w", fn.Name, err)
		}
	}

	// Emit sample function.
	if mod.Sample != nil {
		if err := e.emitSampleFunction(mod.Sample); err != nil {
			return "", fmt.Errorf("emitting sample function: %w", err)
		}
	}

	// Emit compute entry point.
	e.emitEntryPoint()

	return e.buf.String(), nil
}

func (e *Emitter) emitStructs() {
	// Point struct.
	e.writeln("struct Point {")
	e.indent++
	e.writeln("index: u32,")
	e.writeln("x: f32,")
	e.writeln("y: f32,")
	e.writeln("_pad: f32,")
	e.indent--
	e.writeln("}")
	e.writeln("")

	// Vec2Result struct (only if needed).
	if e.neededHelpers["vec2result"] {
		e.writeln("struct Vec2Result {")
		e.indent++
		e.writeln("v0: f32,")
		e.writeln("v1: f32,")
		e.indent--
		e.writeln("}")
		e.writeln("")
	}

	// Emit multi-return structs for user-defined functions.
	for _, fn := range e.mod.Functions {
		if fn.MultiRet > 1 && e.reachable[fn] {
			structName := multiRetStructName(fn.Name, fn.MultiRet)
			e.writef("struct %s {\n", structName)
			e.indent++
			for i := 0; i < fn.MultiRet; i++ {
				e.writef("v%d: f32,\n", i)
			}
			e.indent--
			e.writeln("}")
			e.writeln("")
		}
	}

	// Uniforms struct.
	e.writeln("struct Uniforms {")
	e.indent++
	e.writeln("width: f32,")
	e.writeln("height: f32,")
	e.writeln("phase: f32,")
	e.writeln("point_count: u32,")
	for _, p := range e.mod.Params {
		if p.Type == ir.ParamEnum {
			continue // enums not supported in WGSL v0.1
		}
		e.writef("param_%s: f32,\n", p.Name)
	}
	e.indent--
	e.writeln("}")
	e.writeln("")
}

func (e *Emitter) markReachable() {
	if e.mod.Sample == nil {
		for _, fn := range e.mod.Functions {
			e.reachable[fn] = true
		}
		return
	}

	byName := make(map[string]*ir.Function, len(e.mod.Functions))
	for _, fn := range e.mod.Functions {
		byName[fn.Name] = fn
	}

	var visit func(fn *ir.Function)
	visit = func(fn *ir.Function) {
		if fn == nil || e.reachable[fn] {
			return
		}
		e.reachable[fn] = true
		for _, stmt := range fn.Body {
			e.markReachableStmt(stmt, byName, visit)
		}
	}

	visit(e.mod.Sample)
}

func (e *Emitter) markReachableStmt(stmt ir.IRStmt, byName map[string]*ir.Function, visit func(fn *ir.Function)) {
	switch s := stmt.(type) {
	case *ir.LocalDecl:
		if s.Init != nil {
			e.markReachableExpr(s.Init, byName, visit)
		}
	case *ir.MultiLocalDecl:
		e.markReachableExpr(s.Source, byName, visit)
	case *ir.Assign:
		e.markReachableExpr(s.Value, byName, visit)
	case *ir.IfStmt:
		e.markReachableExpr(s.Cond, byName, visit)
		for _, inner := range s.Then {
			e.markReachableStmt(inner, byName, visit)
		}
		for _, elif := range s.ElseIfs {
			e.markReachableExpr(elif.Cond, byName, visit)
			for _, inner := range elif.Body {
				e.markReachableStmt(inner, byName, visit)
			}
		}
		for _, inner := range s.ElseBody {
			e.markReachableStmt(inner, byName, visit)
		}
	case *ir.Return:
		for _, value := range s.Values {
			e.markReachableExpr(value, byName, visit)
		}
	case *ir.ExprStmt:
		e.markReachableExpr(s.Expr, byName, visit)
	}
}

func (e *Emitter) markReachableExpr(expr ir.IRExpr, byName map[string]*ir.Function, visit func(fn *ir.Function)) {
	switch ex := expr.(type) {
	case *ir.BinaryOp:
		e.markReachableExpr(ex.Left, byName, visit)
		e.markReachableExpr(ex.Right, byName, visit)
	case *ir.UnaryOp:
		e.markReachableExpr(ex.Operand, byName, visit)
	case *ir.Call:
		if target := byName[ex.Function]; target != nil {
			visit(target)
		}
		for _, arg := range ex.Args {
			e.markReachableExpr(arg, byName, visit)
		}
	case *ir.BuiltinCall:
		for _, arg := range ex.Args {
			e.markReachableExpr(arg, byName, visit)
		}
	case *ir.TupleRef:
		e.markReachableExpr(ex.Tuple, byName, visit)
	}
}

func (e *Emitter) emitSampleFunction(fn *ir.Function) error {
	// Emit sample with a dummy params slot so source-level helper calls can pass
	// `params` through even though `params.name` lowers directly to uniforms.
	e.writef("fn lfx_sample(width: f32, height: f32, x: f32, y: f32, index: f32, phase: f32, params: f32) -> %s {\n", sampleReturnType(e.mod.Output))
	e.indent++
	e.currentFunc = fn

	// Emit local variable declarations for non-parameter locals.
	// The first len(fn.Params) locals are the function parameters.
	for i := len(fn.Params); i < len(fn.Locals); i++ {
		local := fn.Locals[i]
		e.writef("var %s: f32 = 0.0;\n", sanitizeName(local.Name))
	}

	for _, stmt := range fn.Body {
		e.emitStmt(stmt)
	}

	e.indent--
	e.writeln("}")
	e.writeln("")
	return nil
}

func (e *Emitter) emitFunction(fn *ir.Function) error {
	e.currentFunc = fn

	retType := "f32"
	if fn.MultiRet > 1 {
		retType = multiRetStructName(fn.Name, fn.MultiRet)
	}

	// Build parameter list.
	var params []string
	for _, p := range fn.Params {
		params = append(params, fmt.Sprintf("%s: f32", sanitizeName(p.Name)))
	}

	e.writef("fn %s(%s) -> %s {\n", sanitizeName(fn.Name), strings.Join(params, ", "), retType)
	e.indent++

	// Emit local variable declarations for non-parameter locals.
	for i := len(fn.Params); i < len(fn.Locals); i++ {
		local := fn.Locals[i]
		e.writef("var %s: f32 = 0.0;\n", sanitizeName(local.Name))
	}

	for _, stmt := range fn.Body {
		e.emitStmt(stmt)
	}

	e.indent--
	e.writeln("}")
	e.writeln("")
	return nil
}

func (e *Emitter) emitEntryPoint() {
	e.writeln("@compute @workgroup_size(64)")
	e.writeln("fn main(@builtin(global_invocation_id) gid: vec3<u32>) {")
	e.indent++
	e.writeln("let idx = gid.x;")
	e.writeln("if (idx >= uniforms.point_count) {")
	e.indent++
	e.writeln("return;")
	e.indent--
	e.writeln("}")
	e.writeln("let pt = points[idx];")
	e.writeln("var result = lfx_sample(uniforms.width, uniforms.height, pt.x, pt.y, f32(pt.index), uniforms.phase, 0.0);")
	switch e.mod.Output {
	case ir.OutputRGB:
		e.writeln("output[idx * 3u + 0u] = clamp(result.x, 0.0, 1.0);")
		e.writeln("output[idx * 3u + 1u] = clamp(result.y, 0.0, 1.0);")
		e.writeln("output[idx * 3u + 2u] = clamp(result.z, 0.0, 1.0);")
	case ir.OutputRGBW:
		e.writeln("output[idx * 4u + 0u] = clamp(result.x, 0.0, 1.0);")
		e.writeln("output[idx * 4u + 1u] = clamp(result.y, 0.0, 1.0);")
		e.writeln("output[idx * 4u + 2u] = clamp(result.z, 0.0, 1.0);")
		e.writeln("output[idx * 4u + 3u] = clamp(result.w, 0.0, 1.0);")
	default:
		e.writeln("result = clamp(result, 0.0, 1.0);")
		e.writeln("output[idx] = result;")
	}
	e.indent--
	e.writeln("}")
	e.writeln("")
}

// scanFunction walks a function to discover which helpers are needed.
func (e *Emitter) scanFunction(fn *ir.Function) {
	for _, stmt := range fn.Body {
		e.scanStmt(stmt)
	}
}

func (e *Emitter) scanStmt(stmt ir.IRStmt) {
	switch s := stmt.(type) {
	case *ir.LocalDecl:
		if s.Init != nil {
			e.scanExpr(s.Init)
		}
	case *ir.MultiLocalDecl:
		e.scanExpr(s.Source)
	case *ir.Assign:
		e.scanExpr(s.Value)
	case *ir.IfStmt:
		e.scanExpr(s.Cond)
		for _, inner := range s.Then {
			e.scanStmt(inner)
		}
		for _, elif := range s.ElseIfs {
			e.scanExpr(elif.Cond)
			for _, inner := range elif.Body {
				e.scanStmt(inner)
			}
		}
		for _, inner := range s.ElseBody {
			e.scanStmt(inner)
		}
	case *ir.Return:
		for _, value := range s.Values {
			e.scanExpr(value)
		}
	case *ir.ExprStmt:
		e.scanExpr(s.Expr)
	}
}

func (e *Emitter) scanExpr(expr ir.IRExpr) {
	switch ex := expr.(type) {
	case *ir.BinaryOp:
		if ex.Op == ir.OpMod {
			e.neededHelpers["mod"] = true
		}
		e.scanExpr(ex.Left)
		e.scanExpr(ex.Right)
	case *ir.UnaryOp:
		e.scanExpr(ex.Operand)
	case *ir.Call:
		for _, a := range ex.Args {
			e.scanExpr(a)
		}
	case *ir.BuiltinCall:
		switch ex.Builtin {
		case ir.BuiltinMix:
			e.neededHelpers["mix"] = true
		case ir.BuiltinMod:
			e.neededHelpers["mod"] = true
		case ir.BuiltinIsEven:
			e.neededHelpers["is_even"] = true
		case ir.BuiltinPerlin:
			e.neededHelpers["perlin"] = true
		case ir.BuiltinVoronoi, ir.BuiltinVoronoiBorder:
			e.neededHelpers["voronoi"] = true
		case ir.BuiltinWorley:
			e.neededHelpers["worley"] = true
		}
		for _, a := range ex.Args {
			e.scanExpr(a)
		}
	case *ir.TupleRef:
		e.scanExpr(ex.Tuple)
	}
}

// writeln writes an indented line followed by a newline.
func (e *Emitter) writeln(s string) {
	for i := 0; i < e.indent; i++ {
		e.buf.WriteString("    ")
	}
	e.buf.WriteString(s)
	e.buf.WriteByte('\n')
}

// writef writes a formatted, indented line.
func (e *Emitter) writef(format string, args ...interface{}) {
	for i := 0; i < e.indent; i++ {
		e.buf.WriteString("    ")
	}
	fmt.Fprintf(&e.buf, format, args...)
}

func multiRetStructName(funcName string, count int) string {
	return fmt.Sprintf("%s_Ret%d", sanitizeName(funcName), count)
}

func sampleReturnType(output ir.OutputType) string {
	switch output {
	case ir.OutputRGB:
		return "vec3<f32>"
	case ir.OutputRGBW:
		return "vec4<f32>"
	default:
		return "f32"
	}
}

var wgslReservedNames = map[string]struct{}{
	"alias":        {},
	"break":        {},
	"case":         {},
	"const":        {},
	"const_assert": {},
	"continue":     {},
	"continuing":   {},
	"default":      {},
	"diagnostic":   {},
	"discard":      {},
	"else":         {},
	"enable":       {},
	"false":        {},
	"fn":           {},
	"for":          {},
	"if":           {},
	"let":          {},
	"loop":         {},
	"override":     {},
	"requires":     {},
	"return":       {},
	"struct":       {},
	"switch":       {},
	"true":         {},
	"var":          {},
	"while":        {},
	"workgroup":    {},
	"uniform":      {},
	"storage":      {},
	"private":      {},
	"function":     {},
	"read":         {},
	"write":        {},
	"read_write":   {},
}

// sanitizeName replaces characters not valid in WGSL identifiers.
func sanitizeName(name string) string {
	safe := strings.ReplaceAll(name, ".", "_")
	if _, reserved := wgslReservedNames[safe]; reserved {
		return "lfx_" + safe
	}
	return safe
}
