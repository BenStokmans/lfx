package wgsl

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
)

func (e *Emitter) emitStmt(stmt ir.IRStmt) {
	switch s := stmt.(type) {
	case *ir.LocalDecl:
		name := sanitizeName(s.Name)
		if s.Init != nil {
			val := e.emitExpr(s.Init)
			e.writef("%s = %s;\n", name, val)
		}
		// If no Init, local was already pre-declared with default 0.0

	case *ir.MultiLocalDecl:
		e.multiRetCounter++
		tmpName := fmt.Sprintf("_mret%d", e.multiRetCounter)
		srcExpr := e.emitExpr(s.Source)
		e.writef("let %s = %s;\n", tmpName, srcExpr)
		for i, name := range s.Names {
			e.writef("%s = %s.v%d;\n", sanitizeName(name), tmpName, i)
		}

	case *ir.Assign:
		// Look up the local name from the current function.
		name := e.localName(s.Index)
		val := e.emitExpr(s.Value)
		e.writef("%s = %s;\n", name, val)

	case *ir.IfStmt:
		cond := e.emitExpr(s.Cond)
		e.writef("if (%s != 0.0) {\n", cond)
		e.indent++
		for _, inner := range s.Then {
			e.emitStmt(inner)
		}
		e.indent--

		for _, elif := range s.ElseIfs {
			elifCond := e.emitExpr(elif.Cond)
			e.writef("} else if (%s != 0.0) {\n", elifCond)
			e.indent++
			for _, inner := range elif.Body {
				e.emitStmt(inner)
			}
			e.indent--
		}

		if len(s.ElseBody) > 0 {
			e.writeln("} else {")
			e.indent++
			for _, inner := range s.ElseBody {
				e.emitStmt(inner)
			}
			e.indent--
		}
		e.writeln("}")

	case *ir.Return:
		if s.Value == nil {
			e.writeln("return;")
		} else {
			val := e.emitExpr(s.Value)
			e.writef("return %s;\n", val)
		}

	case *ir.ExprStmt:
		val := e.emitExpr(s.Expr)
		e.writef("%s;\n", val)
	}
}

// localName resolves a local slot index to its name in the current function.
func (e *Emitter) localName(index int) string {
	if e.currentFunc != nil && index < len(e.currentFunc.Locals) {
		return sanitizeName(e.currentFunc.Locals[index].Name)
	}
	return fmt.Sprintf("_local%d", index)
}
