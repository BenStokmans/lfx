package sema

import "github.com/BenStokmans/lfx/parser"

// checkRecursion builds a call graph from the analyzer's collected call edges
// and rejects any cycles (direct or mutual recursion).
func (a *analyzer) checkRecursion() {
	// For each function, do a DFS looking for cycles.
	for caller := range a.callGraph {
		visited := make(map[string]bool)
		stack := make(map[string]bool)
		if a.hasCycle(caller, visited, stack) {
			// Find the FuncDecl so we can report its position.
			pos := parser.Pos{Line: 1, Col: 1}
			for _, fn := range a.mod.Funcs {
				if fn.Name == caller {
					pos = fn.Pos
					break
				}
			}
			a.addError(pos, ErrRecursionDetected, "recursion detected involving function \""+caller+"\"")
		}
	}
}

// hasCycle performs a DFS cycle detection starting from node.
func (a *analyzer) hasCycle(node string, visited, stack map[string]bool) bool {
	if stack[node] {
		return true
	}
	if visited[node] {
		return false
	}
	visited[node] = true
	stack[node] = true
	for callee := range a.callGraph[node] {
		if a.hasCycle(callee, visited, stack) {
			return true
		}
	}
	stack[node] = false
	return false
}
