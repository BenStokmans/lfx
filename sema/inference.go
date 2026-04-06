package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/parser"
)

type FuncSignature struct {
	Params     []ir.Type
	ReturnType ir.Type
	MultiRet   int
}

type Info struct {
	ExprTypes map[parser.Expr]ir.Type
	FuncTypes map[*parser.FuncDecl]FuncSignature
	Locals    map[*parser.FuncDecl]map[string]ir.Type
	Exports   map[string]FuncSignature
}

type inferencer struct {
	mod          *parser.Module
	imports      map[string]*parser.Module
	importedInfo map[string]*Info
	info         *Info
	errors       []Error
	warnings     []Warning
	changed      bool
	funcByName   map[string]*parser.FuncDecl
}

func AnalyzeModule(mod *parser.Module, importedModules map[string]*parser.Module, importedInfo map[string]*Info) (*Info, []Error, []Warning) {
	a := newAnalyzer(mod, importedModules)

	a.buildModuleScope()
	a.checkModuleConstraints()
	a.validateParams()
	a.validateTimeline()
	for _, fn := range a.mod.Funcs {
		a.resolveFunc(fn)
	}
	a.checkRecursion()

	if len(a.errors) != 0 {
		return nil, a.errors, a.warnings
	}

	inf := newInferencer(mod, importedModules, importedInfo)
	info := inf.infer()
	allErrs := append([]Error{}, a.errors...)
	allErrs = append(allErrs, inf.errors...)
	allWarns := append([]Warning{}, a.warnings...)
	allWarns = append(allWarns, inf.warnings...)
	if len(allErrs) != 0 {
		return nil, allErrs, allWarns
	}
	return info, nil, allWarns
}

func newInferencer(mod *parser.Module, importedModules map[string]*parser.Module, importedInfo map[string]*Info) *inferencer {
	info := &Info{
		ExprTypes: make(map[parser.Expr]ir.Type),
		FuncTypes: make(map[*parser.FuncDecl]FuncSignature, len(mod.Funcs)),
		Locals:    make(map[*parser.FuncDecl]map[string]ir.Type, len(mod.Funcs)),
		Exports:   make(map[string]FuncSignature),
	}

	funcByName := make(map[string]*parser.FuncDecl, len(mod.Funcs))
	for _, fn := range mod.Funcs {
		funcByName[fn.Name] = fn
		params := make([]ir.Type, len(fn.Params))
		for i := range params {
			params[i] = ir.TypeUnknown
		}
		if fn.Name == "sample" {
			for i := range params {
				params[i] = ir.TypeF32
			}
		}
		info.FuncTypes[fn] = FuncSignature{
			Params:     params,
			ReturnType: ir.TypeUnknown,
		}
		info.Locals[fn] = make(map[string]ir.Type, len(fn.Params))
	}

	return &inferencer{
		mod:          mod,
		imports:      importedModules,
		importedInfo: importedInfo,
		info:         info,
		funcByName:   funcByName,
	}
}

func (i *inferencer) infer() *Info {
	for pass := 0; pass < len(i.mod.Funcs)+4; pass++ {
		i.changed = false
		for _, fn := range i.mod.Funcs {
			i.inferFunc(fn)
		}
		if !i.changed {
			break
		}
	}

	for _, fn := range i.mod.Funcs {
		i.finalizeFunc(fn)
		if fn.Exported {
			i.info.Exports[fn.Name] = i.info.FuncTypes[fn]
		}
	}

	return i.info
}

func (i *inferencer) inferFunc(fn *parser.FuncDecl) {
	sig := i.info.FuncTypes[fn]
	env := make(map[string]ir.Type, len(i.info.Locals[fn])+len(fn.Params))
	for idx, name := range fn.Params {
		env[name] = sig.Params[idx]
	}
	for name, typ := range i.info.Locals[fn] {
		env[name] = typ
	}
	i.walkStmts(fn, fn.Body, env)
	for idx, name := range fn.Params {
		i.setParamType(fn, idx, env[name])
	}
	for name, typ := range env {
		if indexOf(fn.Params, name) >= 0 {
			continue
		}
		i.setLocalType(fn, name, typ)
	}
}

func (i *inferencer) walkStmts(fn *parser.FuncDecl, stmts []parser.Stmt, env map[string]ir.Type) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parser.LocalStmt:
			if len(s.Names) == 1 && len(s.Values) == 1 {
				typ := i.inferExpr(fn, s.Values[0], env)
				env[s.Names[0]] = mergeTypes(env[s.Names[0]], typ)
				i.setLocalType(fn, s.Names[0], env[s.Names[0]])
			} else if len(s.Names) > 1 && len(s.Values) == 1 {
				call, ok := s.Values[0].(*parser.CallExpr)
				if !ok {
					continue
				}
				_, retCount := i.inferCall(fn, call, env)
				if retCount != len(s.Names) {
					i.addError(s.Pos, ErrReturnArityMismatch, fmt.Sprintf("multi-assignment expects %d values, got %d", len(s.Names), retCount))
					continue
				}
				for _, name := range s.Names {
					env[name] = mergeTypes(env[name], ir.TypeF32)
					i.setLocalType(fn, name, env[name])
				}
			}

		case *parser.AssignStmt:
			typ := i.inferExpr(fn, s.Value, env)
			env[s.Name] = mergeTypes(env[s.Name], typ)
			i.setLocalType(fn, s.Name, env[s.Name])

		case *parser.IfStmt:
			i.requireScalar(s.Pos, i.inferExpr(fn, s.Condition, env), "if condition must be scalar")
			thenEnv := cloneEnv(env)
			i.walkStmts(fn, s.Body, thenEnv)
			for _, elseif := range s.ElseIfs {
				i.requireScalar(elseif.Pos, i.inferExpr(fn, elseif.Condition, env), "elseif condition must be scalar")
				branchEnv := cloneEnv(env)
				i.walkStmts(fn, elseif.Body, branchEnv)
			}
			if len(s.ElseBody) > 0 {
				elseEnv := cloneEnv(env)
				i.walkStmts(fn, s.ElseBody, elseEnv)
			}

		case *parser.ReturnStmt:
			i.inferReturn(fn, s, env)

		case *parser.ExprStmt:
			i.inferExpr(fn, s.Expr, env)
		}
	}
}

func (i *inferencer) inferReturn(fn *parser.FuncDecl, stmt *parser.ReturnStmt, env map[string]ir.Type) {
	sig := i.info.FuncTypes[fn]
	if len(stmt.Values) == 0 {
		return
	}
	if len(stmt.Values) > 1 {
		for _, value := range stmt.Values {
			typ := i.inferExpr(fn, value, env)
			if typ.IsVector() {
				i.addError(stmt.Pos, ErrVectorArityMixed, "multi-value returns must contain scalar expressions")
			}
		}
		if sig.MultiRet == 0 {
			sig.MultiRet = len(stmt.Values)
			sig.ReturnType = ir.TypeF32
			i.info.FuncTypes[fn] = sig
			i.changed = true
		} else if sig.MultiRet != len(stmt.Values) {
			i.addError(stmt.Pos, ErrReturnArityMismatch, fmt.Sprintf("function %q returns %d values here, expected %d", fn.Name, len(stmt.Values), sig.MultiRet))
		}
		return
	}

	retType := i.inferExpr(fn, stmt.Values[0], env)
	if retType == ir.TypeUnknown {
		return
	}
	if fn.Name == "sample" && i.mod.Output != nil {
		switch i.mod.Output.Type {
		case parser.OutputScalar:
			if retType.IsVector() {
				i.addError(stmt.Pos, ErrSampleVectorReturnMismatch, "scalar output sample cannot return a vector")
			}
		case parser.OutputRGB:
			if retType.IsVector() && retType != ir.TypeVec3 {
				i.addError(stmt.Pos, ErrSampleVectorReturnMismatch, "rgb output sample must return vec3 or three scalar values")
			} else if !retType.IsVector() && retType != ir.TypeUnknown {
				i.addError(stmt.Pos, ErrReturnArityMismatch, "sample return arity mismatch: output expects 3 values, got 1")
			}
		case parser.OutputRGBW:
			if retType.IsVector() && retType != ir.TypeVec4 {
				i.addError(stmt.Pos, ErrSampleVectorReturnMismatch, "rgbw output sample must return vec4 or four scalar values")
			} else if !retType.IsVector() && retType != ir.TypeUnknown {
				i.addError(stmt.Pos, ErrReturnArityMismatch, "sample return arity mismatch: output expects 4 values, got 1")
			}
		}
	}
	sig.ReturnType = mergeTypes(sig.ReturnType, retType)
	i.info.FuncTypes[fn] = sig
}

func (i *inferencer) inferExpr(fn *parser.FuncDecl, expr parser.Expr, env map[string]ir.Type) ir.Type {
	if typ, ok := i.info.ExprTypes[expr]; ok && typ != ir.TypeUnknown {
		return typ
	}

	typ := ir.TypeUnknown
	switch e := expr.(type) {
	case *parser.NumberLit:
		if e.IsInt {
			typ = ir.TypeI32
		} else {
			typ = ir.TypeF32
		}

	case *parser.BoolLit:
		typ = ir.TypeBool

	case *parser.StringLit:
		typ = ir.TypeString

	case *parser.Ident:
		typ = env[e.Name]
		if typ == ir.TypeUnknown {
			if paramType, ok := moduleParamType(i.mod, e.Name); ok {
				typ = paramType
			}
		}

	case *parser.GroupExpr:
		typ = i.inferExpr(fn, e.Inner, env)

	case *parser.UnaryExpr:
		operand := i.inferExpr(fn, e.Operand, env)
		switch e.Op {
		case "-":
			if operand == ir.TypeUnknown || operand.IsNumeric() {
				typ = operand
			} else {
				i.addError(e.Pos, ErrInvalidVectorUnaryOp, "unary - expects a numeric scalar or vector")
			}
		case "not":
			if operand.IsVector() {
				i.addError(e.Pos, ErrInvalidVectorLogic, "logical operators do not support vectors")
			}
			typ = ir.TypeBool
		}

	case *parser.BinaryExpr:
		left := i.inferExpr(fn, e.Left, env)
		right := i.inferExpr(fn, e.Right, env)
		typ = i.inferBinary(e, left, right)

	case *parser.DotExpr:
		typ = i.inferDot(fn, e, env)

	case *parser.CallExpr:
		typ, _ = i.inferCall(fn, e, env)
	}

	if typ == ir.TypeUnknown {
		typ = i.info.ExprTypes[expr]
	}
	i.info.ExprTypes[expr] = typ
	return typ
}

func (i *inferencer) inferBinary(expr *parser.BinaryExpr, left, right ir.Type) ir.Type {
	switch expr.Op {
	case "+", "-", "*", "/", "%":
		if left.IsVector() || right.IsVector() {
			if left == ir.TypeUnknown && right.IsVector() {
				i.setExprType(expr.Left, right)
				left = right
			}
			if right == ir.TypeUnknown && left.IsVector() {
				i.setExprType(expr.Right, left)
				right = left
			}
			if left.IsVector() && right.IsVector() {
				if left != right {
					i.addError(expr.Pos, ErrVectorWidthMismatch, fmt.Sprintf("vector arithmetic requires matching widths, got %s and %s", left, right))
					return left
				}
				return left
			}
			if left.IsVector() && isScalarType(right) {
				return left
			}
			if right.IsVector() && isScalarType(left) {
				return right
			}
			return ir.TypeUnknown
		}
		if left == ir.TypeF32 || right == ir.TypeF32 {
			return ir.TypeF32
		}
		if left == ir.TypeUnknown {
			return right
		}
		if right == ir.TypeUnknown {
			return left
		}
		return ir.TypeI32

	case "==", "!=", "~=", "<", ">", "<=", ">=":
		if left.IsVector() || right.IsVector() {
			i.addError(expr.Pos, ErrInvalidVectorCompare, "comparison operators do not support vectors")
		}
		return ir.TypeBool

	case "and", "or":
		if left.IsVector() || right.IsVector() {
			i.addError(expr.Pos, ErrInvalidVectorLogic, "logical operators do not support vectors")
		}
		return ir.TypeBool
	}
	return ir.TypeUnknown
}

func (i *inferencer) inferDot(fn *parser.FuncDecl, expr *parser.DotExpr, env map[string]ir.Type) ir.Type {
	if ident, ok := expr.Object.(*parser.Ident); ok && ident.Name == "params" {
		typ, _ := moduleParamType(i.mod, expr.Field)
		return typ
	}

	if ident, ok := expr.Object.(*parser.Ident); ok {
		if sym := i.lookupImport(ident.Name); sym != nil {
			return ir.TypeUnknown
		}
	}

	base := i.inferExpr(fn, expr.Object, env)
	if ident, ok := expr.Object.(*parser.Ident); ok {
		minType := vectorTypeForField(expr.Field)
		if minType != ir.TypeUnknown && !base.IsVector() {
			env[ident.Name] = mergeTypes(env[ident.Name], minType)
			if env[ident.Name] == ir.TypeUnknown || !env[ident.Name].IsVector() {
				env[ident.Name] = minType
			}
			if paramIdx := indexOf(fn.Params, ident.Name); paramIdx >= 0 {
				i.setParamType(fn, paramIdx, env[ident.Name])
			} else {
				i.setLocalType(fn, ident.Name, env[ident.Name])
			}
			i.info.ExprTypes[expr.Object] = env[ident.Name]
			base = env[ident.Name]
		}
	}
	if base == ir.TypeUnknown {
		return ir.TypeF32
	}
	index, ok := vectorFieldIndex(expr.Field)
	if !ok {
		i.addError(expr.Pos, ErrUnknownVectorField, fmt.Sprintf("unknown vector field %q", expr.Field))
		return ir.TypeUnknown
	}
	if !base.IsVector() {
		i.addError(expr.Pos, ErrInvalidVectorFieldAccess, fmt.Sprintf("field %q requires a vector value", expr.Field))
		return ir.TypeUnknown
	}
	if index >= base.Lanes() {
		i.addError(expr.Pos, ErrInvalidVectorFieldAccess, fmt.Sprintf("field %q is not valid on %s", expr.Field, base))
		return ir.TypeUnknown
	}
	return ir.TypeF32
}

func (i *inferencer) inferCall(fn *parser.FuncDecl, call *parser.CallExpr, env map[string]ir.Type) (ir.Type, int) {
	args := make([]ir.Type, len(call.Args))
	for idx, arg := range call.Args {
		args[idx] = i.inferExpr(fn, arg, env)
	}

	switch target := call.Function.(type) {
	case *parser.Ident:
		if builtinFunc, ok := builtinTypeFuncs[target.Name]; ok {
			return builtinFunc(i, call, args)
		}
		if callee := i.funcByName[target.Name]; callee != nil {
			sig := i.info.FuncTypes[callee]
			for idx := range args {
				if idx >= len(sig.Params) {
					break
				}
				i.setParamType(callee, idx, args[idx])
			}
			if sig.MultiRet > 1 {
				return ir.TypeF32, sig.MultiRet
			}
			return sig.ReturnType, 1
		}

	case *parser.DotExpr:
		if ident, ok := target.Object.(*parser.Ident); ok {
			if imported := i.importedInfo[ident.Name]; imported != nil {
				if sig, ok := imported.Exports[target.Field]; ok {
					return sig.ReturnType, max(1, sig.MultiRet)
				}
			}
		}
	}

	return ir.TypeUnknown, 1
}

func (i *inferencer) finalizeFunc(fn *parser.FuncDecl) {
	sig := i.info.FuncTypes[fn]
	for idx, typ := range sig.Params {
		if typ == ir.TypeUnknown {
			sig.Params[idx] = ir.TypeF32
		}
	}
	if sig.ReturnType == ir.TypeUnknown && sig.MultiRet == 0 {
		sig.ReturnType = ir.TypeF32
	}
	i.info.FuncTypes[fn] = sig
}

func (i *inferencer) setExprType(expr parser.Expr, typ ir.Type) {
	current := i.info.ExprTypes[expr]
	next := mergeTypes(current, typ)
	if next != current {
		i.info.ExprTypes[expr] = next
		i.changed = true
	}
}

func (i *inferencer) setLocalType(fn *parser.FuncDecl, name string, typ ir.Type) {
	if typ == ir.TypeUnknown {
		return
	}
	current := i.info.Locals[fn][name]
	next := mergeTypes(current, typ)
	if next != current {
		i.info.Locals[fn][name] = next
		i.changed = true
	}
}

func (i *inferencer) setParamType(fn *parser.FuncDecl, idx int, typ ir.Type) {
	if typ == ir.TypeUnknown {
		return
	}
	sig := i.info.FuncTypes[fn]
	if idx >= len(sig.Params) {
		return
	}
	next := mergeTypes(sig.Params[idx], typ)
	if next != sig.Params[idx] {
		sig.Params[idx] = next
		i.info.FuncTypes[fn] = sig
		i.changed = true
	}
}

func (i *inferencer) requireScalar(pos parser.Pos, typ ir.Type, msg string) {
	if typ.IsVector() {
		i.addError(pos, ErrInvalidVectorLogic, msg)
	}
}

func (i *inferencer) addError(pos parser.Pos, code, msg string) {
	i.errors = append(i.errors, Error{Pos: pos, Code: code, Msg: msg})
}

func moduleParamType(mod *parser.Module, name string) (ir.Type, bool) {
	if mod.Params == nil {
		return ir.TypeUnknown, false
	}
	for _, param := range mod.Params.Params {
		if param.Name != name {
			continue
		}
		switch param.Type {
		case parser.ParamInt:
			return ir.TypeI32, true
		case parser.ParamFloat:
			return ir.TypeF32, true
		case parser.ParamBool:
			return ir.TypeBool, true
		case parser.ParamEnum:
			return ir.TypeString, true
		}
	}
	return ir.TypeUnknown, false
}

func vectorFieldIndex(field string) (int, bool) {
	switch field {
	case "x", "r":
		return 0, true
	case "y", "g":
		return 1, true
	case "z", "b":
		return 2, true
	case "w":
		return 3, true
	default:
		return 0, false
	}
}

func vectorTypeForField(field string) ir.Type {
	switch field {
	case "x", "r", "y", "g":
		return ir.TypeVec2
	case "z", "b":
		return ir.TypeVec3
	case "w":
		return ir.TypeVec4
	default:
		return ir.TypeUnknown
	}
}

func cloneEnv(env map[string]ir.Type) map[string]ir.Type {
	copyEnv := make(map[string]ir.Type, len(env))
	for key, value := range env {
		copyEnv[key] = value
	}
	return copyEnv
}

func mergeTypes(current, next ir.Type) ir.Type {
	if next == ir.TypeUnknown {
		return current
	}
	if current == ir.TypeUnknown {
		return next
	}
	if current == next {
		return current
	}
	if current.IsVector() || next.IsVector() {
		return next
	}
	if isScalarType(current) && isScalarType(next) {
		if current == ir.TypeF32 || next == ir.TypeF32 {
			return ir.TypeF32
		}
		return current
	}
	return next
}

func isScalarType(typ ir.Type) bool {
	return !typ.IsVector() && typ != ir.TypeUnknown && typ != ir.TypeVoid
}

func indexOf(values []string, want string) int {
	for idx, value := range values {
		if value == want {
			return idx
		}
	}
	return -1
}

func (i *inferencer) lookupImport(alias string) *parser.Module {
	if i.imports == nil {
		return nil
	}
	return i.imports[alias]
}

func builtinVectorCtor(lanes int) builtinTypeFunc {
	return func(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
		if len(args) != lanes {
			i.addError(call.Pos, ErrBuiltinArityMismatch, fmt.Sprintf("vec%d expects %d arguments", lanes, lanes))
			return ir.TypeUnknown, 1
		}
		for _, arg := range args {
			if arg.IsVector() {
				i.addError(call.Pos, ErrBuiltinArityMismatch, fmt.Sprintf("vec%d constructor arguments must be scalar", lanes))
				return ir.TypeUnknown, 1
			}
		}
		return ir.VectorTypeForLanes(lanes), 1
	}
}

type builtinTypeFunc func(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int)

var builtinTypeFuncs = map[string]builtinTypeFunc{
	"vec2":             builtinVectorCtor(2),
	"vec3":             builtinVectorCtor(3),
	"vec4":             builtinVectorCtor(4),
	"dot":              inferDotBuiltin,
	"length":           inferLengthBuiltin,
	"distance":         inferDistanceBuiltin,
	"normalize":        inferNormalizeBuiltin,
	"cross":            inferCrossBuiltin,
	"project":          inferProjectBuiltin,
	"reflect":          inferReflectBuiltin,
	"abs":              inferLiftedSameTypeBuiltin("abs", 1),
	"min":              inferLiftedSameTypeBuiltin("min", 2),
	"max":              inferLiftedSameTypeBuiltin("max", 2),
	"floor":            inferLiftedSameTypeBuiltin("floor", 1),
	"ceil":             inferLiftedSameTypeBuiltin("ceil", 1),
	"sqrt":             inferLiftedSameTypeBuiltin("sqrt", 1),
	"sin":              inferLiftedSameTypeBuiltin("sin", 1),
	"cos":              inferLiftedSameTypeBuiltin("cos", 1),
	"clamp":            inferLiftedSameTypeBuiltin("clamp", 3),
	"mix":              inferLiftedSameTypeBuiltin("mix", 3),
	"fract":            inferLiftedSameTypeBuiltin("fract", 1),
	"mod":              inferLiftedSameTypeBuiltin("mod", 2),
	"pow":              inferLiftedSameTypeBuiltin("pow", 2),
	"is_even":          inferScalarBuiltin("is_even", 1, ir.TypeF32),
	"__perlin":         inferNoiseBuiltin("__perlin", 1, 3),
	"__voronoi":        inferNoiseBuiltin("__voronoi", 2, 3),
	"__voronoi_border": inferNoiseBuiltin("__voronoi_border", 3, 3),
	"__worley":         inferNoiseBuiltin("__worley", 2, 4),
}

func inferScalarBuiltin(name string, arity int, ret ir.Type) builtinTypeFunc {
	return func(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
		if len(args) != arity {
			i.addError(call.Pos, ErrBuiltinArityMismatch, fmt.Sprintf("%s expects %d arguments", name, arity))
			return ir.TypeUnknown, 1
		}
		for _, arg := range args {
			if arg.IsVector() {
				i.addError(call.Pos, ErrBuiltinVectorMismatch, fmt.Sprintf("%s does not support vector arguments", name))
				return ir.TypeUnknown, 1
			}
		}
		return ret, 1
	}
}

func inferLiftedSameTypeBuiltin(name string, arity int) builtinTypeFunc {
	return func(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
		if len(args) != arity {
			i.addError(call.Pos, ErrBuiltinArityMismatch, fmt.Sprintf("%s expects %d arguments", name, arity))
			return ir.TypeUnknown, 1
		}
		result := ir.TypeUnknown
		for _, arg := range args {
			if arg.IsVector() {
				if result == ir.TypeUnknown {
					result = arg
					continue
				}
				if result.IsVector() && result != arg {
					i.addError(call.Pos, ErrVectorWidthMismatch, fmt.Sprintf("%s requires matching vector widths", name))
					return ir.TypeUnknown, 1
				}
			}
		}
		if result.IsVector() {
			return result, 1
		}
		for _, arg := range args {
			result = mergeTypes(result, arg)
		}
		if result == ir.TypeUnknown {
			return ir.TypeF32, 1
		}
		return result, 1
	}
}

func inferDotBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 2 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "dot expects 2 arguments")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() && args[1].IsVector() && args[0] != args[1] {
		i.addError(call.Pos, ErrVectorWidthMismatch, "dot requires matching vector widths")
	}
	return ir.TypeF32, 1
}

func inferLengthBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 1 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "length expects 1 argument")
		return ir.TypeUnknown, 1
	}
	return ir.TypeF32, 1
}

func inferDistanceBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 2 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "distance expects 2 arguments")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() && args[1].IsVector() && args[0] != args[1] {
		i.addError(call.Pos, ErrVectorWidthMismatch, "distance requires matching vector widths")
	}
	return ir.TypeF32, 1
}

func inferNormalizeBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 1 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "normalize expects 1 argument")
		return ir.TypeUnknown, 1
	}
	return args[0], 1
}

func inferCrossBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 2 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "cross expects 2 arguments")
		return ir.TypeUnknown, 1
	}
	if args[0] != ir.TypeVec3 || args[1] != ir.TypeVec3 {
		i.addError(call.Pos, ErrBuiltinVectorMismatch, "cross expects vec3 arguments")
		return ir.TypeUnknown, 1
	}
	return ir.TypeVec3, 1
}

func inferProjectBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 2 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "project expects 2 arguments")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() && args[1].IsVector() && args[0] != args[1] {
		i.addError(call.Pos, ErrVectorWidthMismatch, "project requires matching vector widths")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() {
		return args[0], 1
	}
	return args[1], 1
}

func inferReflectBuiltin(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
	if len(args) != 2 {
		i.addError(call.Pos, ErrBuiltinArityMismatch, "reflect expects 2 arguments")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() && args[1].IsVector() && args[0] != args[1] {
		i.addError(call.Pos, ErrVectorWidthMismatch, "reflect requires matching vector widths")
		return ir.TypeUnknown, 1
	}
	if args[0].IsVector() {
		return args[0], 1
	}
	return args[1], 1
}

func inferNoiseBuiltin(name string, minArity, maxArity int) builtinTypeFunc {
	return func(i *inferencer, call *parser.CallExpr, args []ir.Type) (ir.Type, int) {
		if len(args) == 1 && args[0].IsVector() {
			lanes := args[0].Lanes()
			if lanes < minArity || lanes > maxArity {
				i.addError(call.Pos, ErrBuiltinVectorMismatch, fmt.Sprintf("%s does not support %s", name, args[0]))
			}
			return ir.TypeF32, 1
		}
		if len(args) < minArity || len(args) > maxArity {
			i.addError(call.Pos, ErrBuiltinArityMismatch, fmt.Sprintf("%s expects %d-%d scalar arguments", name, minArity, maxArity))
			return ir.TypeUnknown, 1
		}
		for _, arg := range args {
			if arg.IsVector() {
				i.addError(call.Pos, ErrBuiltinVectorMismatch, fmt.Sprintf("%s expects either one vector argument or scalar arguments", name))
				return ir.TypeUnknown, 1
			}
		}
		return ir.TypeF32, 1
	}
}
