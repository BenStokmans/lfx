package lower

import (
	"fmt"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

// builtinMap maps bare function names to their BuiltinID.
var builtinMap = map[string]ir.BuiltinID{
	"abs":              ir.BuiltinAbs,
	"min":              ir.BuiltinMin,
	"max":              ir.BuiltinMax,
	"floor":            ir.BuiltinFloor,
	"ceil":             ir.BuiltinCeil,
	"sqrt":             ir.BuiltinSqrt,
	"sin":              ir.BuiltinSin,
	"cos":              ir.BuiltinCos,
	"clamp":            ir.BuiltinClamp,
	"mix":              ir.BuiltinMix,
	"fract":            ir.BuiltinFract,
	"mod":              ir.BuiltinMod,
	"pow":              ir.BuiltinPow,
	"is_even":          ir.BuiltinIsEven,
	"vec2":             ir.BuiltinVec2,
	"vec3":             ir.BuiltinVec3,
	"vec4":             ir.BuiltinVec4,
	"dot":              ir.BuiltinDot,
	"length":           ir.BuiltinLength,
	"distance":         ir.BuiltinDistance,
	"normalize":        ir.BuiltinNormalize,
	"cross":            ir.BuiltinCross,
	"project":          ir.BuiltinProject,
	"reflect":          ir.BuiltinReflect,
	"__perlin":         ir.BuiltinPerlin,
	"__voronoi":        ir.BuiltinVoronoi,
	"__voronoi_border": ir.BuiltinVoronoiBorder,
	"__worley":         ir.BuiltinWorley,
}

// multiRetBuiltins lists builtins that return multiple values.
var multiRetBuiltins = map[ir.BuiltinID]int{}

// Lowerer converts a parsed AST Module into an IR Module.
type Lowerer struct {
	mod          *parser.Module
	imports      map[string]*parser.Module // alias -> parsed library module
	info         *sema.Info
	importInfos  map[string]*sema.Info
	irmod        *ir.Module
	localCounter int
	locals       map[string]int // name -> local index in current function
	funcLocals   []ir.Local
	currentMod   *parser.Module
	currentInfo  *sema.Info
	currentFn    *parser.FuncDecl

	// paramNames tracks declared parameter names for ParamRef resolution.
	paramNames map[string]ir.Type
}

// Lower converts a parsed module and its resolved imports into an IR module.
// importedModules maps import alias to the parsed library module.
func Lower(mod *parser.Module, importedModules map[string]*parser.Module, info *sema.Info, importedInfos map[string]*sema.Info) (*ir.Module, error) {
	l := &Lowerer{
		mod:         mod,
		imports:     importedModules,
		info:        info,
		importInfos: importedInfos,
		irmod: &ir.Module{
			Name:       mod.ModPath,
			SourcePath: mod.ModPath,
			Output:     ir.OutputScalar,
		},
		paramNames: make(map[string]ir.Type),
	}
	if mod.Output != nil {
		l.irmod.Output = toIROutputType(mod.Output.Type)
	}

	// Convert params.
	if mod.Params != nil {
		for _, p := range mod.Params.Params {
			spec := convertParamDef(p)
			l.irmod.Params = append(l.irmod.Params, spec)
			l.paramNames[p.Name] = paramTypeToExprType(p.Type)
		}
	}

	// Convert optional timeline block.
	if mod.Timeline != nil {
		l.irmod.Timeline = convertTimelineDecl(mod.Timeline)
	}

	// Lower imported library functions (with mangled names).
	for alias, libMod := range importedModules {
		for _, fn := range libMod.Funcs {
			if !fn.Exported {
				continue
			}
			mangledName := MangleName(alias, fn.Name)
			irFn, err := l.lowerFunction(libMod, importedInfos[alias], fn, mangledName, libMod.ModPath)
			if err != nil {
				return nil, fmt.Errorf("lowering imported function %s.%s: %w", alias, fn.Name, err)
			}
			l.irmod.Functions = append(l.irmod.Functions, irFn)
		}
	}

	// Lower local functions.
	for _, fn := range mod.Funcs {
		irFn, err := l.lowerFunction(mod, info, fn, fn.Name, mod.ModPath)
		if err != nil {
			return nil, fmt.Errorf("lowering function %s: %w", fn.Name, err)
		}
		l.irmod.Functions = append(l.irmod.Functions, irFn)
	}

	// Set Sample to the function named "sample".
	for _, fn := range l.irmod.Functions {
		if fn.Name == "sample" {
			l.irmod.Sample = fn
			break
		}
	}

	return l.irmod, nil
}

// lowerFunction converts an AST FuncDecl to an IR Function.
func (l *Lowerer) lowerFunction(owner *parser.Module, ownerInfo *sema.Info, fn *parser.FuncDecl, name, source string) (*ir.Function, error) {
	l.localCounter = 0
	l.locals = make(map[string]int)
	l.funcLocals = nil
	l.currentMod = owner
	l.currentInfo = ownerInfo
	l.currentFn = fn

	irFn := &ir.Function{
		Name:     name,
		Exported: fn.Exported,
		Source:   source,
	}
	sig := l.lookupFuncSig(fn)
	if sig.MultiRet > 0 {
		irFn.MultiRet = sig.MultiRet
	}

	// Create local slots for function parameters.
	for idx, paramName := range fn.Params {
		paramType := ir.TypeF32
		if idx < len(sig.Params) && sig.Params[idx] != ir.TypeUnknown {
			paramType = sig.Params[idx]
		}
		idx := l.allocLocal(paramName, paramType)
		irFn.Params = append(irFn.Params, ir.FuncParam{
			Name: paramName,
			Type: paramType,
		})
		_ = idx
	}

	// Lower each statement in the body.
	for _, stmt := range fn.Body {
		irStmt, err := l.lowerStmt(stmt)
		if err != nil {
			return nil, err
		}
		if irStmt != nil {
			if ret, ok := irStmt.(*ir.Return); ok && len(ret.Values) > 1 {
				if irFn.MultiRet == 0 {
					irFn.MultiRet = len(ret.Values)
				} else if irFn.MultiRet != len(ret.Values) {
					return nil, fmt.Errorf("inconsistent return arity in function %s", fn.Name)
				}
			}
			irFn.Body = append(irFn.Body, irStmt)
		}
	}

	irFn.Locals = l.funcLocals
	irFn.ReturnType = sig.ReturnType
	if irFn.ReturnType == ir.TypeUnknown {
		irFn.ReturnType = ir.TypeF32
	}

	return irFn, nil
}

func toIROutputType(t parser.OutputType) ir.OutputType {
	switch t {
	case parser.OutputRGB:
		return ir.OutputRGB
	case parser.OutputRGBW:
		return ir.OutputRGBW
	default:
		return ir.OutputScalar
	}
}

// allocLocal allocates a new local variable slot and returns its index.
func (l *Lowerer) allocLocal(name string, typ ir.Type) int {
	idx := l.localCounter
	l.localCounter++
	l.locals[name] = idx
	l.funcLocals = append(l.funcLocals, ir.Local{Name: name, Type: typ})
	return idx
}

// lowerStmt converts an AST Stmt to an IR IRStmt.
func (l *Lowerer) lowerStmt(stmt parser.Stmt) (ir.IRStmt, error) {
	switch s := stmt.(type) {
	case *parser.LocalStmt:
		return l.lowerLocalStmt(s)
	case *parser.AssignStmt:
		return l.lowerAssignStmt(s)
	case *parser.IfStmt:
		return l.lowerIfStmt(s)
	case *parser.ReturnStmt:
		return l.lowerReturnStmt(s)
	case *parser.ExprStmt:
		return l.lowerExprStmt(s)
	default:
		return nil, fmt.Errorf("unsupported statement type %T", stmt)
	}
}

// lowerLocalStmt converts a multi-name binding AST node to either an IR LocalDecl or MultiLocalDecl.
func (l *Lowerer) lowerLocalStmt(s *parser.LocalStmt) (ir.IRStmt, error) {
	// Multi-return: a, b = expr (single value that returns multiple)
	if len(s.Names) > 1 && len(s.Values) == 1 {
		src, err := l.lowerExpr(s.Values[0])
		if err != nil {
			return nil, err
		}
		names := s.Names
		indices := make([]int, len(names))
		types := make([]ir.Type, len(names))
		for i, name := range names {
			localType := l.lookupLocalType(name)
			if localType == ir.TypeUnknown {
				localType = ir.TypeF32
			}
			idx := l.allocLocal(name, localType)
			indices[i] = idx
			types[i] = localType
		}
		return &ir.MultiLocalDecl{
			Names:   names,
			Indices: indices,
			Types:   types,
			Source:  src,
		}, nil
	}

	// Single binding: x = expr
	if len(s.Names) == 1 {
		var init ir.IRExpr
		if len(s.Values) > 0 {
			var err error
			init, err = l.lowerExpr(s.Values[0])
			if err != nil {
				return nil, err
			}
		}
		localType := l.lookupLocalType(s.Names[0])
		if localType == ir.TypeUnknown {
			if init != nil {
				localType = init.ResultType()
			} else {
				localType = ir.TypeF32
			}
		}
		idx := l.allocLocal(s.Names[0], localType)
		return &ir.LocalDecl{
			Index: idx,
			Name:  s.Names[0],
			Typ:   localType,
			Init:  init,
		}, nil
	}

	// Multiple bindings with multiple values: a, b = expr1, expr2
	if len(s.Names) > 0 && len(s.Values) == len(s.Names) {
		return nil, fmt.Errorf("multiple bindings with multiple values are not supported")
	}

	return nil, fmt.Errorf("unsupported local statement with %d names and %d values", len(s.Names), len(s.Values))
}

// lowerAssignStmt converts an AST AssignStmt to an IR Assign.
func (l *Lowerer) lowerAssignStmt(s *parser.AssignStmt) (ir.IRStmt, error) {
	val, err := l.lowerExpr(s.Value)
	if err != nil {
		return nil, err
	}
	idx, ok := l.locals[s.Name]
	if !ok {
		localType := l.lookupLocalType(s.Name)
		if localType == ir.TypeUnknown {
			localType = val.ResultType()
		}
		idx = l.allocLocal(s.Name, localType)
		return &ir.LocalDecl{
			Index: idx,
			Name:  s.Name,
			Typ:   localType,
			Init:  val,
		}, nil
	}
	return &ir.Assign{
		Index: idx,
		Value: val,
	}, nil
}

// lowerIfStmt converts an AST IfStmt to an IR IfStmt.
func (l *Lowerer) lowerIfStmt(s *parser.IfStmt) (ir.IRStmt, error) {
	cond, err := l.lowerExpr(s.Condition)
	if err != nil {
		return nil, err
	}

	thenBody, err := l.lowerStmts(s.Body)
	if err != nil {
		return nil, err
	}

	var elseIfs []ir.IRElseIf
	for _, ei := range s.ElseIfs {
		eiCond, err := l.lowerExpr(ei.Condition)
		if err != nil {
			return nil, err
		}
		eiBody, err := l.lowerStmts(ei.Body)
		if err != nil {
			return nil, err
		}
		elseIfs = append(elseIfs, ir.IRElseIf{
			Cond: eiCond,
			Body: eiBody,
		})
	}

	var elseBody []ir.IRStmt
	if len(s.ElseBody) > 0 {
		elseBody, err = l.lowerStmts(s.ElseBody)
		if err != nil {
			return nil, err
		}
	}

	return &ir.IfStmt{
		Cond:     cond,
		Then:     thenBody,
		ElseIfs:  elseIfs,
		ElseBody: elseBody,
	}, nil
}

// lowerReturnStmt converts an AST ReturnStmt to an IR Return.
func (l *Lowerer) lowerReturnStmt(s *parser.ReturnStmt) (ir.IRStmt, error) {
	if len(s.Values) == 0 {
		return &ir.Return{}, nil
	}

	values, err := l.lowerExprs(s.Values)
	if err != nil {
		return nil, err
	}
	return &ir.Return{Values: values}, nil
}

// lowerExprStmt converts an AST ExprStmt to an IR ExprStmt.
func (l *Lowerer) lowerExprStmt(s *parser.ExprStmt) (ir.IRStmt, error) {
	expr, err := l.lowerExpr(s.Expr)
	if err != nil {
		return nil, err
	}
	return &ir.ExprStmt{Expr: expr}, nil
}

// lowerStmts converts a slice of AST statements to IR statements.
func (l *Lowerer) lowerStmts(stmts []parser.Stmt) ([]ir.IRStmt, error) {
	var result []ir.IRStmt
	for _, s := range stmts {
		irS, err := l.lowerStmt(s)
		if err != nil {
			return nil, err
		}
		if irS != nil {
			result = append(result, irS)
		}
	}
	return result, nil
}

// lowerExpr converts an AST Expr to an IR IRExpr.
func (l *Lowerer) lowerExpr(expr parser.Expr) (ir.IRExpr, error) {
	switch e := expr.(type) {
	case *parser.NumberLit:
		return l.lowerNumberLit(e), nil
	case *parser.BoolLit:
		return l.lowerBoolLit(e), nil
	case *parser.StringLit:
		return l.lowerStringLit(e), nil
	case *parser.Ident:
		return l.lowerIdent(e)
	case *parser.BinaryExpr:
		return l.lowerBinaryExpr(e)
	case *parser.UnaryExpr:
		return l.lowerUnaryExpr(e)
	case *parser.CallExpr:
		return l.lowerCallExpr(e)
	case *parser.DotExpr:
		return l.lowerDotExpr(e)
	case *parser.GroupExpr:
		return l.lowerExpr(e.Inner)
	default:
		return nil, fmt.Errorf("unsupported expression type %T", expr)
	}
}

// lowerNumberLit converts a NumberLit to an IR Const.
func (l *Lowerer) lowerNumberLit(e *parser.NumberLit) ir.IRExpr {
	if e.IsInt {
		return &ir.Const{Value: int32(e.Value), Typ: ir.TypeI32}
	}
	return &ir.Const{Value: float32(e.Value), Typ: ir.TypeF32}
}

// lowerBoolLit converts a BoolLit to an IR Const.
func (l *Lowerer) lowerBoolLit(e *parser.BoolLit) ir.IRExpr {
	return &ir.Const{Value: e.Value, Typ: ir.TypeBool}
}

// lowerStringLit converts a StringLit to an IR Const.
func (l *Lowerer) lowerStringLit(e *parser.StringLit) ir.IRExpr {
	return &ir.Const{Value: e.Value, Typ: ir.TypeString}
}

// lowerIdent converts an Ident to an IR LocalRef or resolves as builtin.
func (l *Lowerer) lowerIdent(e *parser.Ident) (ir.IRExpr, error) {
	if idx, ok := l.locals[e.Name]; ok {
		return &ir.LocalRef{
			Index: idx,
			Name:  e.Name,
			Typ:   l.funcLocals[idx].Type,
		}, nil
	}
	return nil, fmt.Errorf("undefined identifier %q", e.Name)
}

// lowerBinaryExpr converts a BinaryExpr to an IR BinaryOp.
func (l *Lowerer) lowerBinaryExpr(e *parser.BinaryExpr) (ir.IRExpr, error) {
	left, err := l.lowerExpr(e.Left)
	if err != nil {
		return nil, err
	}
	right, err := l.lowerExpr(e.Right)
	if err != nil {
		return nil, err
	}
	op, err := mapBinaryOp(e.Op)
	if err != nil {
		return nil, err
	}

	typ := l.lookupExprType(e)
	if typ == ir.TypeUnknown {
		typ = inferBinaryType(op, left, right)
	}
	return &ir.BinaryOp{
		Op:    op,
		Left:  left,
		Right: right,
		Typ:   typ,
	}, nil
}

// lowerUnaryExpr converts a UnaryExpr to an IR UnaryOp.
func (l *Lowerer) lowerUnaryExpr(e *parser.UnaryExpr) (ir.IRExpr, error) {
	operand, err := l.lowerExpr(e.Operand)
	if err != nil {
		return nil, err
	}
	op, err := mapUnaryOp(e.Op)
	if err != nil {
		return nil, err
	}
	typ := l.lookupExprType(e)
	if typ == ir.TypeUnknown {
		typ = operand.ResultType()
	}
	return &ir.UnaryOp{
		Op:      op,
		Operand: operand,
		Typ:     typ,
	}, nil
}

// lowerCallExpr converts a CallExpr to an IR Call or BuiltinCall.
func (l *Lowerer) lowerCallExpr(e *parser.CallExpr) (ir.IRExpr, error) {
	args, err := l.lowerExprs(e.Args)
	if err != nil {
		return nil, err
	}

	switch fn := e.Function.(type) {
	case *parser.Ident:
		// Check if it's a builtin.
		if bid, ok := builtinMap[fn.Name]; ok {
			retCount := multiRetBuiltins[bid]
			retType := l.lookupExprType(e)
			if retType == ir.TypeUnknown {
				retType = ir.TypeF32
			}
			return &ir.BuiltinCall{
				Builtin:       bid,
				Args:          args,
				ReturnType:    retType,
				MultiRetCount: retCount,
			}, nil
		}
		// Local function call.
		retType := l.lookupExprType(e)
		if retType == ir.TypeUnknown {
			retType = ir.TypeF32
		}
		return &ir.Call{
			Function:      fn.Name,
			Args:          args,
			ReturnType:    retType,
			MultiRetCount: l.lookupCallMultiRet(fn.Name),
		}, nil

	case *parser.DotExpr:
		// module.func(...) call
		if ident, ok := fn.Object.(*parser.Ident); ok {
			alias := ident.Name
			funcName := fn.Field

			// Unknown imported function: emit Call with mangled name.
			mangledName := MangleName(alias, funcName)
			retType := l.lookupImportedCallType(alias, funcName, e)
			return &ir.Call{
				Function:      mangledName,
				Args:          args,
				ReturnType:    retType,
				MultiRetCount: l.lookupImportedMultiRet(alias, funcName),
			}, nil
		}
		return nil, fmt.Errorf("unsupported dot-call target %T", fn.Object)

	default:
		return nil, fmt.Errorf("unsupported call target %T", e.Function)
	}
}

// lowerDotExpr converts a non-call DotExpr (e.g. params.field) to an IR ParamRef.
func (l *Lowerer) lowerDotExpr(e *parser.DotExpr) (ir.IRExpr, error) {
	if ident, ok := e.Object.(*parser.Ident); ok && ident.Name == "params" {
		typ, ok := l.paramNames[e.Field]
		if !ok {
			typ = ir.TypeF32 // default if param not found
		}
		return &ir.ParamRef{
			Name: e.Field,
			Typ:  typ,
		}, nil
	}
	if ident, ok := e.Object.(*parser.Ident); ok {
		if idx, exists := l.locals[ident.Name]; exists && l.funcLocals[idx].Type.IsVector() {
			index, ok := componentIndex(e.Field)
			if !ok {
				return nil, fmt.Errorf("unknown vector field %q", e.Field)
			}
			return &ir.ComponentRef{
				Vector: &ir.LocalRef{
					Index: idx,
					Name:  ident.Name,
					Typ:   l.funcLocals[idx].Type,
				},
				Index: index,
				Typ:   ir.TypeF32,
			}, nil
		}
	}
	base, err := l.lowerExpr(e.Object)
	if err != nil {
		return nil, err
	}
	if base.ResultType().IsVector() {
		index, ok := componentIndex(e.Field)
		if !ok {
			return nil, fmt.Errorf("unknown vector field %q", e.Field)
		}
		return &ir.ComponentRef{
			Vector: base,
			Index:  index,
			Typ:    ir.TypeF32,
		}, nil
	}
	return nil, fmt.Errorf("unsupported dot expression: %T.%s", e.Object, e.Field)
}

// lowerExprs lowers a slice of AST expressions to IR expressions.
func (l *Lowerer) lowerExprs(exprs []parser.Expr) ([]ir.IRExpr, error) {
	result := make([]ir.IRExpr, len(exprs))
	for i, e := range exprs {
		var err error
		result[i], err = l.lowerExpr(e)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// mapBinaryOp converts an AST operator string to an IR Op.
func mapBinaryOp(op string) (ir.Op, error) {
	switch op {
	case "+":
		return ir.OpAdd, nil
	case "-":
		return ir.OpSub, nil
	case "*":
		return ir.OpMul, nil
	case "/":
		return ir.OpDiv, nil
	case "%":
		return ir.OpMod, nil
	case "==":
		return ir.OpEq, nil
	case "!=", "~=":
		return ir.OpNeq, nil
	case "<":
		return ir.OpLt, nil
	case ">":
		return ir.OpGt, nil
	case "<=":
		return ir.OpLte, nil
	case ">=":
		return ir.OpGte, nil
	case "and":
		return ir.OpAnd, nil
	case "or":
		return ir.OpOr, nil
	default:
		return 0, fmt.Errorf("unsupported binary operator %q", op)
	}
}

// mapUnaryOp converts an AST unary operator string to an IR Op.
func mapUnaryOp(op string) (ir.Op, error) {
	switch op {
	case "-":
		return ir.OpNeg, nil
	case "not":
		return ir.OpNot, nil
	default:
		return 0, fmt.Errorf("unsupported unary operator %q", op)
	}
}

// inferBinaryType infers the result type of a binary operation.
func inferBinaryType(op ir.Op, left, right ir.IRExpr) ir.Type {
	switch op {
	case ir.OpEq, ir.OpNeq, ir.OpLt, ir.OpGt, ir.OpLte, ir.OpGte, ir.OpAnd, ir.OpOr:
		return ir.TypeBool
	default:
		if left.ResultType().IsVector() {
			return left.ResultType()
		}
		if right.ResultType().IsVector() {
			return right.ResultType()
		}
		// If either operand is f32, promote to f32.
		if left.ResultType() == ir.TypeF32 || right.ResultType() == ir.TypeF32 {
			return ir.TypeF32
		}
		return ir.TypeI32
	}
}

// convertParamDef converts an AST ParamDef to an IR ParamSpec.
func convertParamDef(p *parser.ParamDef) ir.ParamSpec {
	spec := ir.ParamSpec{
		Name: p.Name,
		Type: paramTypeToIR(p.Type),
		Min:  p.Min,
		Max:  p.Max,
	}
	switch p.Type {
	case parser.ParamInt:
		if v, ok := p.Default.(int); ok {
			spec.IntDefault = int64(v)
		} else if v, ok := p.Default.(float64); ok {
			spec.IntDefault = int64(v)
		}
	case parser.ParamFloat:
		if v, ok := p.Default.(float64); ok {
			spec.FloatDefault = v
		}
	case parser.ParamBool:
		if v, ok := p.Default.(bool); ok {
			spec.BoolDefault = v
		}
	case parser.ParamEnum:
		if v, ok := p.Default.(string); ok {
			spec.EnumDefault = v
		}
		spec.EnumValues = p.EnumValues
	}
	return spec
}

// convertTimelineDecl converts an AST TimelineDecl to an IR TimelineSpec.
func convertTimelineDecl(tl *parser.TimelineDecl) *ir.TimelineSpec {
	spec := &ir.TimelineSpec{}
	if tl.LoopStart != nil {
		v := *tl.LoopStart
		spec.LoopStart = &v
	}
	if tl.LoopEnd != nil {
		v := *tl.LoopEnd
		spec.LoopEnd = &v
	}
	return spec
}

// paramTypeToIR converts a parser ParamType to an IR Type.
func paramTypeToIR(pt parser.ParamType) ir.ParamType {
	switch pt {
	case parser.ParamInt:
		return ir.ParamInt
	case parser.ParamFloat:
		return ir.ParamFloat
	case parser.ParamBool:
		return ir.ParamBool
	case parser.ParamEnum:
		return ir.ParamEnum
	default:
		return ir.ParamFloat
	}
}

// paramTypeToExprType converts a parser ParamType to an IR expression Type.
func paramTypeToExprType(pt parser.ParamType) ir.Type {
	switch pt {
	case parser.ParamInt:
		return ir.TypeI32
	case parser.ParamFloat:
		return ir.TypeF32
	case parser.ParamBool:
		return ir.TypeBool
	case parser.ParamEnum:
		return ir.TypeString
	default:
		return ir.TypeF32
	}
}

func (l *Lowerer) lookupExprType(expr parser.Expr) ir.Type {
	if l.currentInfo == nil {
		return ir.TypeUnknown
	}
	if typ, ok := l.currentInfo.ExprTypes[expr]; ok {
		return typ
	}
	return ir.TypeUnknown
}

func (l *Lowerer) lookupLocalType(name string) ir.Type {
	if l.currentInfo == nil || l.currentFn == nil {
		return ir.TypeUnknown
	}
	locals := l.currentInfo.Locals[l.currentFn]
	if locals == nil {
		return ir.TypeUnknown
	}
	return locals[name]
}

func (l *Lowerer) lookupFuncSig(fn *parser.FuncDecl) sema.FuncSignature {
	if l.currentInfo == nil {
		return sema.FuncSignature{ReturnType: ir.TypeF32}
	}
	if sig, ok := l.currentInfo.FuncTypes[fn]; ok {
		return sig
	}
	return sema.FuncSignature{ReturnType: ir.TypeF32}
}

func (l *Lowerer) lookupCallMultiRet(name string) int {
	if l.currentInfo == nil {
		return 0
	}
	for fn, sig := range l.currentInfo.FuncTypes {
		if fn.Name == name {
			return sig.MultiRet
		}
	}
	return 0
}

func (l *Lowerer) lookupImportedCallType(alias, funcName string, expr parser.Expr) ir.Type {
	typ := l.lookupExprType(expr)
	if typ != ir.TypeUnknown {
		return typ
	}
	if info := l.importInfos[alias]; info != nil {
		if sig, ok := info.Exports[funcName]; ok {
			if sig.ReturnType != ir.TypeUnknown {
				return sig.ReturnType
			}
		}
	}
	return ir.TypeF32
}

func (l *Lowerer) lookupImportedMultiRet(alias, funcName string) int {
	if info := l.importInfos[alias]; info != nil {
		if sig, ok := info.Exports[funcName]; ok {
			return sig.MultiRet
		}
	}
	return 0
}

func componentIndex(field string) (int, bool) {
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
