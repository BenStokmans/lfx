package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// resolveFunc resolves all names in a function body. It creates a child scope
// with the function's parameters and walks each statement.
func (a *analyzer) resolveFunc(fn *parser.FuncDecl) {
	fnScope := NewScope(a.scope)

	// Define function parameters in the child scope.
	for _, pname := range fn.Params {
		_ = fnScope.Define(pname, &Symbol{
			Name: pname,
			Kind: SymLocal,
			Pos:  fn.Pos,
		})
	}

	// Initialize call-graph entry for this function.
	if a.callGraph[fn.Name] == nil {
		a.callGraph[fn.Name] = make(map[string]bool)
	}

	for _, stmt := range fn.Body {
		a.resolveStmt(stmt, fnScope, fn.Name)
	}
}

// resolveStmt validates a statement and resolves names within it.
func (a *analyzer) resolveStmt(stmt parser.Stmt, scope *Scope, currentFunc string) {
	switch s := stmt.(type) {
	case *parser.LocalStmt:
		// Resolve value expressions first (before the new names are visible).
		for _, v := range s.Values {
			a.resolveExpr(v, scope, currentFunc)
		}
		// Define the new locals.
		for _, name := range s.Names {
			err := scope.Define(name, &Symbol{
				Name: name,
				Kind: SymLocal,
				Pos:  s.Pos,
			})
			if err != nil {
				a.addError(s.Pos, ErrDuplicateLocalName, fmt.Sprintf("variable %q already defined in this scope", name))
			}
		}

	case *parser.AssignStmt:
		// First assignment introduces a new local in the current scope.
		sym := scope.Lookup(s.Name)
		a.resolveExpr(s.Value, scope, currentFunc)
		if sym == nil || sym.Kind == SymBuiltin {
			if sym != nil && sym.Kind == SymBuiltin {
				a.addWarningLen(s.Pos, len(s.Name), WarnBuiltinShadowed, fmt.Sprintf("local %q shadows builtin %q", s.Name, s.Name))
			}
			_ = scope.Define(s.Name, &Symbol{
				Name: s.Name,
				Kind: SymLocal,
				Pos:  s.Pos,
			})
		} else if sym.Kind != SymLocal {
			a.addError(s.Pos, ErrInvalidAssignmentTarget, fmt.Sprintf("cannot assign to %q", s.Name))
		}

	case *parser.IfStmt:
		a.resolveExpr(s.Condition, scope, currentFunc)
		childScope := NewScope(scope)
		for _, st := range s.Body {
			a.resolveStmt(st, childScope, currentFunc)
		}
		for _, elseif := range s.ElseIfs {
			a.resolveExpr(elseif.Condition, scope, currentFunc)
			eifScope := NewScope(scope)
			for _, st := range elseif.Body {
				a.resolveStmt(st, eifScope, currentFunc)
			}
		}
		if len(s.ElseBody) > 0 {
			elseScope := NewScope(scope)
			for _, st := range s.ElseBody {
				a.resolveStmt(st, elseScope, currentFunc)
			}
		}

	case *parser.ReturnStmt:
		for _, value := range s.Values {
			a.resolveExpr(value, scope, currentFunc)
		}

	case *parser.ExprStmt:
		a.resolveExpr(s.Expr, scope, currentFunc)
	}
}

// resolveExpr validates an expression and resolves names within it.
func (a *analyzer) resolveExpr(expr parser.Expr, scope *Scope, currentFunc string) {
	switch e := expr.(type) {
	case *parser.Ident:
		if sym := scope.Lookup(e.Name); sym == nil {
			a.addError(e.Pos, ErrUndefinedIdentifier, fmt.Sprintf("undefined identifier %q", e.Name))
		}

	case *parser.BinaryExpr:
		a.resolveExpr(e.Left, scope, currentFunc)
		a.resolveExpr(e.Right, scope, currentFunc)

	case *parser.UnaryExpr:
		a.resolveExpr(e.Operand, scope, currentFunc)

	case *parser.CallExpr:
		a.resolveExpr(e.Function, scope, currentFunc)
		for _, arg := range e.Args {
			a.resolveExpr(arg, scope, currentFunc)
		}
		// Record local call in the call graph.
		if ident, ok := e.Function.(*parser.Ident); ok {
			if sym := scope.Lookup(ident.Name); sym != nil && sym.Kind == SymFunction {
				if a.callGraph[currentFunc] == nil {
					a.callGraph[currentFunc] = make(map[string]bool)
				}
				a.callGraph[currentFunc][ident.Name] = true
			}
		}

	case *parser.DotExpr:
		if ident, ok := e.Object.(*parser.Ident); ok {
			switch ident.Name {
			case "params":
				if a.mod.Params == nil {
					a.addError(e.Pos, ErrParamsAccessWithoutBlock, "params access used in a module without a params block")
					return
				}
				for _, p := range a.mod.Params.Params {
					if p.Name == e.Field {
						return
					}
				}
				a.addError(e.Pos, ErrUnknownParameter, fmt.Sprintf("unknown parameter %q", e.Field))
				return
			default:
				sym := scope.Lookup(ident.Name)
				if sym != nil && sym.Kind == SymImport {
					imported := a.imports[ident.Name]
					if imported == nil {
						return
					}
					for _, fn := range imported.Funcs {
						if fn.Name == e.Field && fn.Exported {
							return
						}
					}
					a.addError(e.Pos, ErrImportMissingExportedFunc, fmt.Sprintf("module %q has no exported function %q", ident.Name, e.Field))
					return
				}
			}
		}
		a.resolveExpr(e.Object, scope, currentFunc)

	case *parser.GroupExpr:
		a.resolveExpr(e.Inner, scope, currentFunc)

	case *parser.NumberLit, *parser.StringLit, *parser.BoolLit:
		// Literals need no resolution.

	default:
		// Unknown expression type; ignore for forward compatibility.
	}
}
