package modules

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// ModuleGraph tracks the dependency graph.
type ModuleGraph struct {
	Entry string
	Nodes map[string]*ResolvedModule
	Edges map[string][]string // module path -> imported module paths
}

// ResolvedModule holds the resolved source and metadata for a module.
type ResolvedModule struct {
	Path   string
	Source []byte
	IsLib  bool
}

// Build constructs the full import graph starting from entry.
// It parses each module to find imports, resolves them, detects cycles,
// and validates that effect modules do not import other effect modules.
func Build(entry string, source []byte, resolver Resolver) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Entry: entry,
		Nodes: make(map[string]*ResolvedModule),
		Edges: make(map[string][]string),
	}

	// Track visit state for cycle detection.
	const (
		stateUnvisited = 0
		stateVisiting  = 1
		stateVisited   = 2
	)
	state := make(map[string]int)

	var visit func(path string, src []byte) error
	visit = func(path string, src []byte) error {
		if state[path] == stateVisited {
			return nil
		}
		if state[path] == stateVisiting {
			return fmt.Errorf("import cycle detected involving %q", path)
		}
		state[path] = stateVisiting

		mod, err := parser.Parse(string(src))
		if err != nil {
			return fmt.Errorf("parsing module %q: %w", path, err)
		}
		g.Nodes[path] = &ResolvedModule{
			Path:   path,
			Source: src,
			IsLib:  mod.Kind == parser.ModuleKindLibrary,
		}

		currentIsEffect := mod.Kind == parser.ModuleKindEffect

		for _, imp := range mod.Imports {
			importPath := imp.Path
			g.Edges[path] = append(g.Edges[path], importPath)

			// Resolve the imported module if not already loaded.
			depNode, loaded := g.Nodes[importPath]
			if !loaded {
				depSrc, err := resolver.Resolve(importPath)
				if err != nil {
					return fmt.Errorf("resolving import %q from %q: %w", importPath, path, err)
				}
				// Recurse into the dependency.
				if err := visit(importPath, depSrc); err != nil {
					return err
				}
				depNode = g.Nodes[importPath]
			} else if state[importPath] == stateVisiting {
				return fmt.Errorf("import cycle detected involving %q", importPath)
			}

			// Validate: an effect must not import another effect.
			if currentIsEffect && !depNode.IsLib {
				return fmt.Errorf("effect %q cannot import effect %q", path, importPath)
			}
		}

		state[path] = stateVisited
		return nil
	}

	if err := visit(entry, source); err != nil {
		return nil, err
	}
	return g, nil
}
