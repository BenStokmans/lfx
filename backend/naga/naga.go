package naga

import (
	"fmt"

	nagalib "github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/hlsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/naga/spirv"
)

// Target identifies a shader output format.
type Target int

const (
	TargetSPIRV Target = iota
	TargetMSL
	TargetGLSL
	TargetHLSL
)

var targetNames = map[Target]string{
	TargetSPIRV: "spirv",
	TargetMSL:   "msl",
	TargetGLSL:  "glsl",
	TargetHLSL:  "hlsl",
}

func (t Target) String() string {
	if name, ok := targetNames[t]; ok {
		return name
	}
	return fmt.Sprintf("Target(%d)", int(t))
}

// ParseTarget converts a string name to a Target.
func ParseTarget(s string) (Target, error) {
	switch s {
	case "spirv":
		return TargetSPIRV, nil
	case "msl":
		return TargetMSL, nil
	case "glsl":
		return TargetGLSL, nil
	case "hlsl":
		return TargetHLSL, nil
	default:
		return 0, fmt.Errorf("unknown target %q (valid: spirv, msl, glsl, hlsl)", s)
	}
}

// Result holds the output of a shader compilation.
type Result struct {
	Code  string // text output for MSL, GLSL, HLSL
	Bytes []byte // binary output for SPIR-V
}

// Compile translates WGSL source code to the specified target format.
func Compile(wgslSource string, target Target) (*Result, error) {
	module, err := parseAndLower(wgslSource)
	if err != nil {
		return nil, err
	}

	switch target {
	case TargetSPIRV:
		return compileSPIRV(module)
	case TargetMSL:
		return compileMSL(module)
	case TargetGLSL:
		return compileGLSL(module)
	case TargetHLSL:
		return compileHLSL(module)
	default:
		return nil, fmt.Errorf("unsupported target: %s", target)
	}
}

func parseAndLower(wgslSource string) (*ir.Module, error) {
	ast, err := nagalib.Parse(wgslSource)
	if err != nil {
		return nil, fmt.Errorf("naga: parse: %w", err)
	}
	module, err := nagalib.LowerWithSource(ast, wgslSource)
	if err != nil {
		return nil, fmt.Errorf("naga: lower: %w", err)
	}
	return module, nil
}

func compileSPIRV(module *ir.Module) (*Result, error) {
	opts := spirv.Options{
		Version: spirv.Version1_3,
	}
	backend := spirv.NewBackend(opts)
	bytes, err := backend.Compile(module)
	if err != nil {
		return nil, fmt.Errorf("naga: spirv: %w", err)
	}
	return &Result{Bytes: bytes}, nil
}

func compileMSL(module *ir.Module) (*Result, error) {
	code, _, err := msl.Compile(module, msl.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("naga: msl: %w", err)
	}
	return &Result{Code: code}, nil
}

func compileGLSL(module *ir.Module) (*Result, error) {
	code, _, err := glsl.Compile(module, glsl.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("naga: glsl: %w", err)
	}
	return &Result{Code: code}, nil
}

func compileHLSL(module *ir.Module) (*Result, error) {
	code, _, err := hlsl.Compile(module, hlsl.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("naga: hlsl: %w", err)
	}
	return &Result{Code: code}, nil
}
