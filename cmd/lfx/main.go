package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/naga"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "parse":
		return runParse(args[1:])
	case "check":
		return runCheck(args[1:])
	case "preview":
		return runPreview(args[1:])
	case "graph":
		return runGraph(args[1:])
	case "sample":
		return runSample(args[1:])
	case "bench":
		return runBench(args[1:])
	case "emit":
		return runEmit(args[1:])
	case "emit-wgsl":
		return runEmit(append([]string{"--target", "wgsl"}, args[1:]...))
	default:
		return usageError()
	}
}

func runParse(args []string) error {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	filePath, _, err := commonArgs(fs, args)
	if err != nil {
		return err
	}

	mod, _, err := compiler.ParseFile(filePath)
	if err != nil {
		return err
	}
	return writeJSON(mod)
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "print structured diagnostics as JSON")
	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}

	result, err := compiler.CheckFile(filePath, opts)
	if *jsonOutput {
		payload := checkJSONOutput{
			FilePath: filePath,
			OK:       err == nil,
		}
		if err == nil {
			payload.ModulePath = result.Entry.ModPath
			payload.Diagnostics = compiler.DiagnosticsFromWarnings(filePath, result.Entry.ModPath, result.Warnings)
		} else {
			payload.Diagnostics = compiler.DiagnosticsFromError(filePath, "", err)
		}
		return writeJSON(payload)
	}
	if err != nil {
		return err
	}
	fmt.Printf("ok %s (%d modules)\n", result.Entry.ModPath, len(result.Graph.Nodes))
	return nil
}

func runGraph(args []string) error {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}

	result, err := compiler.CheckFile(filePath, opts)
	if err != nil {
		return err
	}

	type node struct {
		Path      string   `json:"path"`
		IsLibrary bool     `json:"is_library"`
		Imports   []string `json:"imports"`
	}

	paths := make([]string, 0, len(result.Graph.Nodes))
	for path := range result.Graph.Nodes {
		paths = append(paths, path)
	}
	slicesSort(paths)

	nodes := make([]node, 0, len(paths))
	for _, path := range paths {
		imports := append([]string(nil), result.Graph.Edges[path]...)
		slicesSort(imports)
		nodes = append(nodes, node{
			Path:      path,
			IsLibrary: result.Graph.Nodes[path].IsLib,
			Imports:   imports,
		})
	}

	return writeJSON(struct {
		Entry string `json:"entry"`
		Nodes []node `json:"nodes"`
	}{
		Entry: result.Graph.Entry,
		Nodes: nodes,
	})
}

func runPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "print preview payload as JSON")
	var params kvFlags
	fs.Var(&params, "param", "parameter override in name=value form")

	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}

	artifact, err := compiler.CompileForPreview(filePath, params.Values(), opts)
	if !*jsonOutput {
		if err != nil {
			return err
		}
		fmt.Printf("ok %s (%s)\n", artifact.ModulePath, artifact.OutputType)
		return nil
	}

	payload := previewJSONOutput{
		FilePath: filePath,
		OK:       err == nil,
	}
	if err != nil {
		payload.Diagnostics = compiler.DiagnosticsFromError(filePath, "", err)
		return writeJSON(payload)
	}

	payload.ModulePath = artifact.ModulePath
	payload.OutputType = artifact.OutputType
	payload.WGSL = artifact.WGSL
	payload.Params = previewParamsFromSpecs(artifact.Params)
	payload.BoundParams = artifact.BoundParams
	payload.Timeline = previewTimelineFromSpec(artifact.Timeline)
	payload.Diagnostics = artifact.Diagnostics
	return writeJSON(payload)
}

func runSample(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	layoutPath := fs.String("layout", "", "path to layout JSON")
	phase := fs.Float64("phase", 0, "normalized phase in [0,1]")
	var params kvFlags
	fs.Var(&params, "param", "parameter override in name=value form")

	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}
	if *layoutPath == "" {
		return errors.New("sample requires --layout")
	}

	result, err := compiler.CompileFile(filePath, opts)
	if err != nil {
		return err
	}

	layout, err := loadLayout(*layoutPath)
	if err != nil {
		return err
	}

	boundParams, err := runtime.Bind(result.IR.Params, params.Values())
	if err != nil {
		return err
	}

	evaluator := cpu.NewEvaluator(result.IR)
	type pointValue struct {
		Index  uint32    `json:"index"`
		X      float32   `json:"x"`
		Y      float32   `json:"y"`
		Values []float32 `json:"values"`
	}
	points := make([]pointValue, 0, len(layout.Points))
	for i, pt := range layout.Points {
		values, err := evaluator.SamplePoint(layout, i, float32(*phase), boundParams)
		if err != nil {
			return err
		}
		points = append(points, pointValue{
			Index:  pt.Index,
			X:      pt.X,
			Y:      pt.Y,
			Values: values,
		})
	}

	return writeJSON(struct {
		Module string       `json:"module"`
		Phase  float64      `json:"phase"`
		Points []pointValue `json:"points"`
	}{
		Module: result.Entry.ModPath,
		Phase:  *phase,
		Points: points,
	})
}

func runEmit(args []string) error {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	target := fs.String("target", "wgsl", "output target: wgsl, spirv, msl, glsl, hlsl")
	output := fs.String("output", "", "output file (default: stdout)")
	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}

	result, err := compiler.CompileFile(filePath, opts)
	if err != nil {
		return err
	}

	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		return err
	}

	if *target == "wgsl" {
		return writeOutput(*output, []byte(wgslSource))
	}

	t, err := naga.ParseTarget(*target)
	if err != nil {
		return err
	}

	compiled, err := naga.Compile(wgslSource, t)
	if err != nil {
		return err
	}

	if compiled.Bytes != nil {
		return writeOutput(*output, compiled.Bytes)
	}
	return writeOutput(*output, []byte(compiled.Code))
}

func writeOutput(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	// #nosec G304 G703 -- CLI tool expected to write to user-provided paths
	return os.WriteFile(path, data, 0600)
}

type previewParamJSON struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	DefaultValue any      `json:"defaultValue"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	EnumValues   []string `json:"enumValues,omitempty"`
}

type previewTimelineJSON struct {
	LoopStart *float64 `json:"loopStart,omitempty"`
	LoopEnd   *float64 `json:"loopEnd,omitempty"`
}

type previewJSONOutput struct {
	FilePath    string                       `json:"filePath"`
	ModulePath  string                       `json:"modulePath,omitempty"`
	OK          bool                         `json:"ok"`
	OutputType  string                       `json:"outputType,omitempty"`
	WGSL        string                       `json:"wgsl,omitempty"`
	Params      []previewParamJSON           `json:"params,omitempty"`
	BoundParams map[string]any               `json:"boundParams,omitempty"`
	Timeline    *previewTimelineJSON         `json:"timeline,omitempty"`
	Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
}

func previewParamsFromSpecs(specs []ir.ParamSpec) []previewParamJSON {
	if len(specs) == 0 {
		return nil
	}

	params := make([]previewParamJSON, 0, len(specs))
	for _, spec := range specs {
		param := previewParamJSON{
			Name: spec.Name,
			Min:  spec.Min,
			Max:  spec.Max,
		}

		switch spec.Type {
		case ir.ParamInt:
			param.Type = "int"
			param.DefaultValue = spec.IntDefault
		case ir.ParamFloat:
			param.Type = "float"
			param.DefaultValue = spec.FloatDefault
		case ir.ParamBool:
			param.Type = "bool"
			param.DefaultValue = spec.BoolDefault
		case ir.ParamEnum:
			param.Type = "enum"
			param.DefaultValue = spec.EnumDefault
			param.EnumValues = append([]string(nil), spec.EnumValues...)
		default:
			param.Type = "unknown"
		}

		params = append(params, param)
	}

	return params
}

func previewTimelineFromSpec(spec *ir.TimelineSpec) *previewTimelineJSON {
	if spec == nil {
		return nil
	}
	return &previewTimelineJSON{
		LoopStart: spec.LoopStart,
		LoopEnd:   spec.LoopEnd,
	}
}

func commonArgs(fs *flag.FlagSet, args []string) (string, compiler.Options, error) {
	root := fs.String("root", "", "module root directory")
	var moduleRoots stringFlags
	fs.Var(&moduleRoots, "module-root", "additional module root directory (repeatable)")
	fs.SetOutput(os.Stderr)
	normalizedArgs, explicitFile := normalizeArgs(args)
	if err := fs.Parse(normalizedArgs); err != nil {
		return "", compiler.Options{}, err
	}
	filePath := explicitFile
	if filePath == "" {
		if fs.NArg() != 1 {
			return "", compiler.Options{}, usageError()
		}
		filePath = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return "", compiler.Options{}, usageError()
	}
	baseDir := *root
	if baseDir == "" {
		baseDir = compiler.DetectBaseDir(filePath)
	}

	roots := moduleRoots.Values()
	if len(roots) == 0 {
		roots = modules.DefaultRoots(baseDir)
	}

	resolver := stdlib.NewResolver(modules.NewFileResolver(roots...))
	return filePath, compiler.Options{
		BaseDir:  baseDir,
		Resolver: resolver,
	}, nil
}

func normalizeArgs(args []string) ([]string, string) {
	if len(args) == 0 {
		return args, ""
	}
	if strings.HasPrefix(args[0], "-") {
		return args, ""
	}
	return args[1:], args[0]
}

func loadLayout(path string) (runtime.Layout, error) {
	// #nosec G304 -- CLI tool expected to read from user-provided paths
	data, err := os.ReadFile(path)
	if err != nil {
		return runtime.Layout{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var layout runtime.Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return runtime.Layout{}, fmt.Errorf("decoding %s: %w", path, err)
	}
	return layout, nil
}

func usageError() error {
	exe := filepath.Base(os.Args[0])
	return fmt.Errorf("usage: %s <parse|check|graph|sample|bench|emit> [flags] <file.lfx>", exe)
}

func writeJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

type checkJSONOutput struct {
	FilePath    string                       `json:"filePath"`
	ModulePath  string                       `json:"modulePath,omitempty"`
	OK          bool                         `json:"ok"`
	Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
}

type kvFlags struct {
	items []string
}

func (k *kvFlags) String() string {
	return strings.Join(k.items, ",")
}

func (k *kvFlags) Set(value string) error {
	if !strings.Contains(value, "=") {
		return fmt.Errorf("invalid param %q", value)
	}
	k.items = append(k.items, value)
	return nil
}

func (k *kvFlags) Values() map[string]any {
	values := make(map[string]any, len(k.items))
	for _, item := range k.items {
		name, raw, _ := strings.Cut(item, "=")
		values[name] = parseValue(raw)
	}
	return values
}

func parseValue(raw string) any {
	switch raw {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(raw); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	return raw
}

type stringFlags struct {
	items []string
}

func (s *stringFlags) String() string {
	return strings.Join(s.items, ",")
}

func (s *stringFlags) Set(value string) error {
	if value == "" {
		return errors.New("value must not be empty")
	}
	s.items = append(s.items, value)
	return nil
}

func (s *stringFlags) Values() []string {
	if len(s.items) == 0 {
		return nil
	}
	out := make([]string, len(s.items))
	copy(out, s.items)
	return out
}

func slicesSort(items []string) {
	if len(items) < 2 {
		return
	}
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
