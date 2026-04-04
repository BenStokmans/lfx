package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// SymbolKind classifies what a symbol refers to.
type SymbolKind int

const (
	SymLocal    SymbolKind = iota
	SymParam
	SymFunction
	SymImport
	SymBuiltin
)

// Symbol is a named entity visible in a scope.
type Symbol struct {
	Name     string
	Kind     SymbolKind
	Pos      parser.Pos
	FuncDecl *parser.FuncDecl // non-nil when Kind == SymFunction
	Module   string           // module path when Kind == SymImport
}

// Scope is a lexical scope that holds symbol definitions.
type Scope struct {
	Parent  *Scope
	Symbols map[string]*Symbol
}

// NewScope returns a fresh scope with the given parent.
func NewScope(parent *Scope) *Scope {
	return &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
}

// Define adds a symbol to this scope. It returns an error if a symbol with the
// same name is already defined in this (not parent) scope.
func (s *Scope) Define(name string, sym *Symbol) error {
	if _, exists := s.Symbols[name]; exists {
		return fmt.Errorf("symbol %q already defined in this scope", name)
	}
	s.Symbols[name] = sym
	return nil
}

// Lookup searches for a symbol by name, walking the parent chain.
func (s *Scope) Lookup(name string) *Symbol {
	for cur := s; cur != nil; cur = cur.Parent {
		if sym, ok := cur.Symbols[name]; ok {
			return sym
		}
	}
	return nil
}
