package lfx

import (
	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/runtime"
)

// BackendKind selects the evaluation backend for an Engine.
type BackendKind int

const (
	// BackendCPU evaluates effects on the CPU. Available under all build configurations.
	BackendCPU BackendKind = iota
	// BackendGPU evaluates effects on the GPU via WebGPU compute shaders.
	// Requires CGO_ENABLED=0.
	BackendGPU
)

// Options configures LoadFile.
type Options struct {
	// Backend selects the evaluation backend. Defaults to BackendCPU.
	Backend BackendKind
	// GPUBackend selects the GPU backend when Backend == BackendGPU.
	// Accepted values: "auto" (default), "metal", "vulkan", "dx12", "gl".
	GPUBackend string
	// BaseDir is the root directory for resolving module imports.
	// If empty, LoadFile attempts to detect it from the source file path.
	BaseDir string
	// Resolver overrides the default module resolver. If nil, the file-system
	// resolver rooted at BaseDir is used.
	Resolver modules.Resolver
}

// Engine is a compiled LFX effect ready for evaluation.
// It is not goroutine-safe.
type Engine struct {
	sampler  backendSampler
	mod      *ir.Module
	channels int
}

// backendSampler is the internal interface over CPU and GPU backends.
type backendSampler interface {
	SamplePoint(layout runtime.Layout, idx int, phase float32, params *runtime.BoundParams) ([]float32, error)
	SamplePoints(layout runtime.Layout, indices []int, phase float32, params *runtime.BoundParams) ([]float32, error)
	Close() error
}

// LoadFile compiles the .lfx file at path and returns an Engine backed by the
// selected backend. Call engine.Close() when done to release any GPU resources.
func LoadFile(path string, opts Options) (*Engine, error) {
	compileOpts := compiler.Options{
		BaseDir:  opts.BaseDir,
		Resolver: opts.Resolver,
	}
	result, err := compiler.CompileFile(path, compileOpts)
	if err != nil {
		return nil, err
	}
	if opts.Backend == BackendGPU {
		return loadGPUEngine(result, opts.GPUBackend)
	}
	return loadCPUEngine(result)
}

func loadCPUEngine(result *compiler.Result) (*Engine, error) {
	return &Engine{
		sampler:  cpuAdapter{cpu.NewEvaluator(result.IR)},
		mod:      result.IR,
		channels: result.IR.Output.Channels(),
	}, nil
}

// cpuAdapter wraps cpu.Evaluator to satisfy backendSampler (adds a no-op Close).
type cpuAdapter struct {
	ev *cpu.Evaluator
}

func (a cpuAdapter) SamplePoint(layout runtime.Layout, idx int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	return a.ev.SamplePoint(layout, idx, phase, params)
}

func (a cpuAdapter) SamplePoints(layout runtime.Layout, indices []int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	return a.ev.SamplePoints(layout, indices, phase, params)
}

func (a cpuAdapter) Close() error { return nil }

// SamplePoint evaluates the effect at a single point.
// Returns one float32 per output channel (clamped to [0, 1]).
func (e *Engine) SamplePoint(layout runtime.Layout, idx int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	return e.sampler.SamplePoint(layout, idx, phase, params)
}

// SamplePoints evaluates the effect at multiple points.
// Returns a flat []float32 of length len(indices) * OutputChannels().
func (e *Engine) SamplePoints(layout runtime.Layout, indices []int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	return e.sampler.SamplePoints(layout, indices, phase, params)
}

// Params returns the effect's parameter specifications.
// Use with runtime.Bind to create a *runtime.BoundParams for sampling.
func (e *Engine) Params() []ir.ParamSpec {
	return e.mod.Params
}

// OutputChannels returns the number of output channels: 1 (scalar), 3 (RGB),
// or 4 (RGBW).
func (e *Engine) OutputChannels() int {
	return e.channels
}

// Close releases any resources held by the Engine. For GPU backends this
// releases the GPU device and all associated buffers. Safe to call on a nil
// Engine.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	return e.sampler.Close()
}
