package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// Error is a semantic analysis error with a stable error code.
type Error struct {
	Msg    string
	Pos    parser.Pos
	Length int
	Code   string // stable error code, e.g. "E001"
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: [%s] %s", e.Pos.String(), e.Code, e.Msg)
}

// Warning is a non-fatal semantic diagnostic.
type Warning struct {
	Msg    string
	Pos    parser.Pos
	Length int
	Code   string // stable warning code, e.g. "W001"
}

// analyzer holds the state for a single analysis pass.
type analyzer struct {
	mod       *parser.Module
	imports   map[string]*parser.Module
	errors    []Error
	warnings  []Warning
	scope     *Scope
	callGraph map[string]map[string]bool // caller -> set of callees
	builtins  map[string]bool
}

// builtinNames is the set of built-in functions available in LFX.
var builtinNames = []string{
	"vec2", "vec3", "vec4",
	"abs", "min", "max", "floor", "ceil", "sqrt",
	"sin", "cos", "clamp", "mix", "fract", "mod", "pow", "is_even",
	"dot", "length", "distance", "normalize", "cross", "project", "reflect",
	"__perlin", "__voronoi", "__voronoi_border", "__worley",
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

func (a *analyzer) addErrorLen(pos parser.Pos, length int, code, msg string) {
	a.errors = append(a.errors, Error{Pos: pos, Length: length, Code: code, Msg: msg})
}

func (a *analyzer) addWarning(pos parser.Pos, code, msg string) {
	a.warnings = append(a.warnings, Warning{Pos: pos, Code: code, Msg: msg})
}

func (a *analyzer) addWarningLen(pos parser.Pos, length int, code, msg string) {
	a.warnings = append(a.warnings, Warning{Pos: pos, Length: length, Code: code, Msg: msg})
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
	_, errs, _ := AnalyzeModule(mod, importedModules, nil)
	return errs
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

const sampleFuncName = "sample"

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
		if fn.Name == sampleFuncName {
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
		if fn.Name == sampleFuncName {
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
