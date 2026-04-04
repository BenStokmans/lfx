package compiler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

// Options configures the shared parse/check/compile pipeline.
type Options struct {
	BaseDir  string
	Resolver modules.Resolver
}

// Result captures the intermediate products of a compilation run.
type Result struct {
	FilePath string
	BaseDir  string
	Source   []byte

	Entry    *parser.Module
	Graph    *modules.ModuleGraph
	Modules  map[string]*parser.Module
	Imports  map[string]*parser.Module
	Infos    map[string]*sema.Info
	Info     *sema.Info
	Warnings []sema.Warning

	IR *ir.Module
}

// Diagnostics is a stable multi-error container for compiler diagnostics.
type Diagnostics struct {
	Items []error
}

func (d *Diagnostics) Append(err error) {
	if err == nil {
		return
	}
	var nested *Diagnostics
	if errors.As(err, &nested) {
		d.Items = append(d.Items, nested.Items...)
		return
	}
	d.Items = append(d.Items, err)
}

func (d *Diagnostics) Empty() bool {
	return d == nil || len(d.Items) == 0
}

func (d *Diagnostics) Error() string {
	if d == nil || len(d.Items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(d.Items))
	for _, item := range d.Items {
		lines = append(lines, item.Error())
	}
	return strings.Join(lines, "\n")
}

// ParseFile parses an entry source file into an AST.
func ParseFile(filePath string) (*parser.Module, []byte, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	mod, err := parser.Parse(string(source))
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	return mod, source, nil
}

// CheckFile parses the entry file, resolves imports, and runs semantic checks.
func CheckFile(filePath string, opts Options) (*Result, error) {
	entry, source, err := ParseFile(filePath)
	if err != nil {
		return nil, err
	}

	baseDir := opts.BaseDir
	if baseDir == "" {
		baseDir = DetectBaseDir(filePath)
	}

	resolver := opts.Resolver
	if resolver == nil {
		resolver = modules.NewFileResolver(modules.DefaultRoots(baseDir)...)
	}

	graph, err := modules.Build(entry.ModPath, source, resolver)
	if err != nil {
		return nil, err
	}

	parsedModules := make(map[string]*parser.Module, len(graph.Nodes))
	for path, node := range graph.Nodes {
		mod, err := parser.Parse(string(node.Source))
		if err != nil {
			return nil, fmt.Errorf("parsing module %q: %w", path, err)
		}
		parsedModules[path] = mod
	}
	if parsedEntry := parsedModules[entry.ModPath]; parsedEntry != nil {
		entry = parsedEntry
	}

	imports := importMapFor(entry, parsedModules)
	diags := &Diagnostics{}

	paths := topoModules(graph)
	infos := make(map[string]*sema.Info, len(parsedModules))
	var warnings []sema.Warning

	for _, path := range paths {
		mod := parsedModules[path]
		importedInfo := importInfoFor(mod, parsedModules, infos)
		info, errs, warns := sema.AnalyzeModule(mod, importMapFor(mod, parsedModules), importedInfo)
		for _, semaErr := range errs {
			err := semaErr
			diags.Append(&err)
		}
		warnings = append(warnings, warns...)
		if info != nil {
			infos[path] = info
		}
	}

	if !diags.Empty() {
		return nil, diags
	}

	return &Result{
		FilePath: filePath,
		BaseDir:  baseDir,
		Source:   source,
		Entry:    entry,
		Graph:    graph,
		Modules:  parsedModules,
		Imports:  imports,
		Infos:    infos,
		Info:     infos[entry.ModPath],
		Warnings: warnings,
	}, nil
}

// CompileFile runs the full shared pipeline through IR lowering.
func CompileFile(filePath string, opts Options) (*Result, error) {
	result, err := CheckFile(filePath, opts)
	if err != nil {
		return nil, err
	}

	irmod, err := lower.Lower(result.Entry, result.Imports, result.Info, importInfoFor(result.Entry, result.Modules, result.Infos))
	if err != nil {
		return nil, err
	}
	lower.ConstFold(irmod)
	result.IR = irmod
	return result, nil
}

// DetectBaseDir walks upward from filePath to find the project root.
func DetectBaseDir(filePath string) string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filepath.Dir(filePath)
	}

	dir := filepath.Dir(absPath)
	for {
		if looksLikeProjectRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(absPath)
		}
		dir = parent
	}
}

func looksLikeProjectRoot(dir string) bool {
	for _, marker := range []string{"go.mod", "stdlib", "effects"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func importMapFor(mod *parser.Module, parsedModules map[string]*parser.Module) map[string]*parser.Module {
	imports := make(map[string]*parser.Module, len(mod.Imports))
	for _, imp := range mod.Imports {
		alias := imp.Alias
		if alias == "" {
			alias = imp.Path
		}
		if imported := parsedModules[imp.Path]; imported != nil {
			imports[alias] = imported
		}
	}
	return imports
}

func importInfoFor(mod *parser.Module, parsedModules map[string]*parser.Module, infos map[string]*sema.Info) map[string]*sema.Info {
	imports := make(map[string]*sema.Info, len(mod.Imports))
	for _, imp := range mod.Imports {
		alias := imp.Alias
		if alias == "" {
			alias = imp.Path
		}
		if parsedModules[imp.Path] != nil && infos[imp.Path] != nil {
			imports[alias] = infos[imp.Path]
		}
	}
	return imports
}

func topoModules(graph *modules.ModuleGraph) []string {
	order := make([]string, 0, len(graph.Nodes))
	visited := make(map[string]bool, len(graph.Nodes))

	var visit func(string)
	visit = func(path string) {
		if visited[path] {
			return
		}
		visited[path] = true
		for _, dep := range graph.Edges[path] {
			visit(dep)
		}
		order = append(order, path)
	}

	visit(graph.Entry)
	for path := range graph.Nodes {
		visit(path)
	}
	return order
}
