package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// Error is a semantic analysis error with a stable error code.
type Error struct {
	Msg  string
	Pos  parser.Pos
	Code string // stable error code, e.g. "E001"
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: [%s] %s", e.Pos.String(), e.Code, e.Msg)
}

// analyzer holds the state for a single analysis pass.
type analyzer struct {
	mod       *parser.Module
	imports   map[string]*parser.Module
	errors    []Error
	scope     *Scope
	callGraph map[string]map[string]bool // caller -> set of callees
	builtins  map[string]bool
}

// builtinNames is the set of built-in functions available in LFX.
var builtinNames = []string{
	"abs", "min", "max", "floor", "ceil", "sqrt",
	"sin", "cos", "clamp", "mix", "fract", "mod", "pow", "is_even",
	"perlin", "voronoi", "voronoi_border", "worley",
}

func newAnalyzer(mod *parser.Module, importedModules map[string]*parser.Module) *analyzer {
	builtins := make(map[string]bool, len(builtinNames))
	for _, name := range builtinNames {
		builtins[name] = true
	}
	if importedModules == nil {
		importedModules = make(map[string]*parser.Module)
	}
	return &analyzer{
		mod:       mod,
		imports:   importedModules,
		errors:    nil,
		callGraph: make(map[string]map[string]bool),
		builtins:  builtins,
	}
}

func (a *analyzer) addError(pos parser.Pos, code, msg string) {
	a.errors = append(a.errors, Error{Pos: pos, Code: code, Msg: msg})
}

// Analyze performs full semantic analysis on a parsed module.
// It validates:
//   - Effect modules must have exactly one "sample" function with 7 params
//   - Library modules must not have sample or a timeline block
//   - Effect modules must not have export functions
//   - Library modules: exported functions must exist
//   - Parameter constructor validation (types, ranges)
//   - Timeline marker ordering: 0 <= loop_start <= loop_end <= 1
//   - All identifiers resolve (locals, params, imports, builtins)
//   - No recursion (direct or mutual)
//   - No forbidden constructs
func Analyze(mod *parser.Module, importedModules map[string]*parser.Module) []Error {
	a := newAnalyzer(mod, importedModules)

	// 1. Build module-level scope.
	a.buildModuleScope()

	// 2. Check module kind constraints.
	a.checkModuleConstraints()

	// 3. Validate params block.
	a.validateParams()

	// 4. Validate optional timeline block.
	a.validateTimeline()

	// 5. Resolve each function body.
	for _, fn := range a.mod.Funcs {
		a.resolveFunc(fn)
	}

	// 6. Validate sample return arity against output declaration.
	a.validateReturnArity()

	// 7. Check for recursion.
	a.checkRecursion()

	return a.errors
}

// buildModuleScope populates the top-level scope with builtins, params,
// functions, and imports.
func (a *analyzer) buildModuleScope() {
	a.scope = NewScope(nil)

	// Builtins.
	for name := range a.builtins {
		_ = a.scope.Define(name, &Symbol{Name: name, Kind: SymBuiltin})
	}

	// Params — each param is available as a name in scope.
	if a.mod.Params != nil {
		for _, p := range a.mod.Params.Params {
			err := a.scope.Define(p.Name, &Symbol{
				Name: p.Name,
				Kind: SymParam,
				Pos:  p.Pos,
			})
			if err != nil {
				a.addError(p.Pos, ErrDuplicateParamName, fmt.Sprintf("duplicate param name %q", p.Name))
			}
		}
	}

	// Functions.
	for _, fn := range a.mod.Funcs {
		err := a.scope.Define(fn.Name, &Symbol{
			Name:     fn.Name,
			Kind:     SymFunction,
			Pos:      fn.Pos,
			FuncDecl: fn,
		})
		if err != nil {
			a.addError(fn.Pos, ErrDuplicateFunctionName, fmt.Sprintf("duplicate function name %q", fn.Name))
		}
	}

	// Imports.
	for _, imp := range a.mod.Imports {
		alias := imp.Alias
		if alias == "" {
			alias = imp.Path
		}
		err := a.scope.Define(alias, &Symbol{
			Name:   alias,
			Kind:   SymImport,
			Pos:    imp.Pos,
			Module: imp.Path,
		})
		if err != nil {
			a.addError(imp.Pos, ErrDuplicateImportAlias, fmt.Sprintf("duplicate import alias %q", alias))
		}
	}
}

// checkModuleConstraints validates structural rules based on module kind.
func (a *analyzer) checkModuleConstraints() {
	switch a.mod.Kind {
	case parser.ModuleKindEffect:
		a.checkEffectConstraints()
	case parser.ModuleKindLibrary:
		a.checkLibraryConstraints()
	}
}

func (a *analyzer) checkEffectConstraints() {
	// Effect modules must have exactly one "sample" function.
	var sampleFn *parser.FuncDecl
	if a.mod.Output == nil {
		pos := parser.Pos{Line: 1, Col: 1}
		if a.mod.Effect != nil {
			pos = a.mod.Effect.Pos
		}
		a.addError(pos, ErrEffectMissingOutput, "effect modules must declare an output type")
	}
	for _, fn := range a.mod.Funcs {
		if fn.Name == "sample" {
			sampleFn = fn
		}
		if fn.Exported {
			a.addError(fn.Pos, ErrEffectExportedFunction, "effect modules must not have exported functions")
		}
	}
	if sampleFn == nil {
		pos := parser.Pos{Line: 1, Col: 1}
		a.addError(pos, ErrEffectMissingSample, "effect modules must have a \"sample\" function")
	} else if len(sampleFn.Params) != 7 {
		a.addError(sampleFn.Pos, ErrEffectInvalidSampleArity,
			fmt.Sprintf("sample function must have exactly 7 parameters, got %d", len(sampleFn.Params)))
	}
}

func (a *analyzer) checkLibraryConstraints() {
	// Library modules must not have a sample function.
	for _, fn := range a.mod.Funcs {
		if fn.Name == "sample" {
			a.addError(fn.Pos, ErrLibraryHasSample, "library modules must not have a \"sample\" function")
		}
	}

	// Library modules must not have a timeline block.
	if a.mod.Timeline != nil {
		a.addError(a.mod.Timeline.Pos, ErrLibraryHasTimeline, "library modules must not have a timeline block")
	}
	if a.mod.Output != nil {
		a.addError(a.mod.Output.Pos, ErrOutputInLibrary, "library modules must not declare an output type")
	}
}

func (a *analyzer) validateReturnArity() {
	var sampleFn *parser.FuncDecl
	for _, fn := range a.mod.Funcs {
		if fn.Name == "sample" {
			sampleFn = fn
			break
		}
	}
	if sampleFn == nil {
		return
	}

	expected := parser.OutputScalar.Channels()
	if a.mod.Output != nil {
		expected = a.mod.Output.Type.Channels()
	}

	var walk func([]parser.Stmt)
	walk = func(stmts []parser.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *parser.ReturnStmt:
				if len(s.Values) != 0 && len(s.Values) != expected {
					a.addError(s.Pos, ErrReturnArityMismatch, fmt.Sprintf("sample return arity mismatch: output expects %d values, got %d", expected, len(s.Values)))
				}
			case *parser.IfStmt:
				walk(s.Body)
				for _, elseif := range s.ElseIfs {
					walk(elseif.Body)
				}
				walk(s.ElseBody)
			}
		}
	}

	walk(sampleFn.Body)
}
