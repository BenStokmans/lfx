//go:build !cgo

package gpu

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unsafe"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// Evaluator is a GPU-based IR evaluator using WebGPU compute shaders.
// It is not goroutine-safe.
type Evaluator struct {
	// Tier 1: built once in NewEvaluator, never change.
	instance       *wgpu.Instance
	adapter        *wgpu.Adapter
	device         *wgpu.Device
	shader         *wgpu.ShaderModule
	bgl            *wgpu.BindGroupLayout
	pipelineLayout *wgpu.PipelineLayout
	pipeline       *wgpu.ComputePipeline
	entryPoint     string
	mod            *ir.Module
	uniformSize    int
	channels       int
	adapterInfo    *wgpu.AdapterInfo

	// Tier 2: replaced lazily when the layout changes.
	pointsBuffer  *wgpu.Buffer
	uniformBuffer *wgpu.Buffer
	outputBuffer  *wgpu.Buffer
	stagingBuffer *wgpu.Buffer
	bindGroup     *wgpu.BindGroup
	readbackBytes []byte
	pointCount    int
	lastLayout    layoutKey
}

// layoutKey is a cheap identity sentinel for detecting layout changes.
// ptr is the address of layout.Points[0] — an optimization hint, not a
// correctness guarantee. When the pointer matches, only the uniform buffer
// is re-uploaded. When it differs, all layout-dependent buffers are rebuilt.
type layoutKey struct {
	width, height float32
	count         int
	ptr           uintptr
}

// NewEvaluator creates a GPU evaluator for mod using pre-compiled WGSL source.
// backend selects the GPU backend: "auto" (default), "metal", "vulkan", "dx12", or "gl".
// Call Close when done to release GPU resources.
func NewEvaluator(mod *ir.Module, wgslSource string, backend string) (*Evaluator, error) {
	instanceDesc, err := parseInstanceDescriptor(backend)
	if err != nil {
		return nil, err
	}

	instance, err := wgpu.CreateInstance(instanceDesc)
	if err != nil {
		return nil, fmt.Errorf("create GPU instance: %w", err)
	}

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		instance.Release()
		return nil, fmt.Errorf("request GPU adapter: %w", err)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("request GPU device: %w", err)
	}
	if device.HalDevice() == nil {
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("request GPU device: got mock device for adapter %q (%s); no real gogpu/wgpu backend is available in this runtime",
			adapter.Info().Name, adapter.Info().Backend.String())
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "lfx-sample",
		WGSL:  wgslSource,
	})
	if err != nil {
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create shader module: %w", err)
	}

	entryPoint, err := resolveComputeEntryPoint(wgslSource, adapter.Info().Backend, "main")
	if err != nil {
		shader.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("resolve compute entry point: %w", err)
	}

	uniformSize := alignTo16((4 + len(numericParamSpecs(mod.Params))) * 4)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "lfx-bind-group-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: uint64(uniformSize)}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		shader.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create bind group layout: %w", err)
	}

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "lfx-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		bgl.Release()
		shader.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create pipeline layout: %w", err)
	}

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "lfx-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: entryPoint,
	})
	if err != nil {
		pipelineLayout.Release()
		bgl.Release()
		shader.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create compute pipeline: %w", err)
	}

	info := adapter.Info()
	infoCopy := info

	return &Evaluator{
		instance:       instance,
		adapter:        adapter,
		device:         device,
		shader:         shader,
		bgl:            bgl,
		pipelineLayout: pipelineLayout,
		pipeline:       pipeline,
		entryPoint:     entryPoint,
		mod:            mod,
		uniformSize:    uniformSize,
		channels:       mod.Output.Channels(),
		adapterInfo:    &infoCopy,
	}, nil
}

// ensureLayout lazily allocates or updates layout-dependent GPU buffers.
// When the layout is unchanged from the previous call only the uniform buffer
// (which carries phase and params) is re-uploaded; no buffer reallocation
// occurs. When the layout changes all tier-2 resources are rebuilt.
func (e *Evaluator) ensureLayout(layout runtime.Layout, phase float32, params *runtime.BoundParams) error {
	if len(layout.Points) == 0 {
		return fmt.Errorf("layout has no points")
	}

	key := layoutKey{
		width:  layout.Width,
		height: layout.Height,
		count:  len(layout.Points),
		ptr:    uintptr(unsafe.Pointer(&layout.Points[0])),
	}

	uniformBytes := encodeUniformBuffer(layout, phase, e.mod.Params, params)

	if key == e.lastLayout {
		return e.device.Queue().WriteBuffer(e.uniformBuffer, 0, uniformBytes)
	}

	// Layout changed: release tier-2 resources then reallocate.
	e.releaseTier2()

	pointsBytes := encodePointsBuffer(layout)
	outputSize := uint64(len(layout.Points) * e.channels * 4)

	pointsBuffer, err := e.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-points",
		Size:  uint64(len(pointsBytes)),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create points buffer: %w", err)
	}

	uniformBuffer, err := e.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-uniforms",
		Size:  uint64(len(uniformBytes)),
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		pointsBuffer.Release()
		return fmt.Errorf("create uniform buffer: %w", err)
	}

	outputBuffer, err := e.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-output",
		Size:  outputSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		uniformBuffer.Release()
		pointsBuffer.Release()
		return fmt.Errorf("create output buffer: %w", err)
	}

	stagingBuffer, err := e.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-staging",
		Size:  outputSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		return fmt.Errorf("create staging buffer: %w", err)
	}

	if err := e.device.Queue().WriteBuffer(pointsBuffer, 0, pointsBytes); err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		return fmt.Errorf("write points buffer: %w", err)
	}
	if err := e.device.Queue().WriteBuffer(uniformBuffer, 0, uniformBytes); err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		return fmt.Errorf("write uniform buffer: %w", err)
	}

	bindGroup, err := e.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "lfx-bind-group",
		Layout: e.bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: pointsBuffer, Size: uint64(len(pointsBytes))},
			{Binding: 1, Buffer: uniformBuffer, Size: uint64(len(uniformBytes))},
			{Binding: 2, Buffer: outputBuffer, Size: outputSize},
		},
	})
	if err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		return fmt.Errorf("create bind group: %w", err)
	}

	e.pointsBuffer = pointsBuffer
	e.uniformBuffer = uniformBuffer
	e.outputBuffer = outputBuffer
	e.stagingBuffer = stagingBuffer
	e.bindGroup = bindGroup
	e.readbackBytes = make([]byte, outputSize)
	e.pointCount = len(layout.Points)
	e.lastLayout = key
	return nil
}

func (e *Evaluator) releaseTier2() {
	if e.bindGroup != nil {
		e.bindGroup.Release()
		e.bindGroup = nil
	}
	if e.stagingBuffer != nil {
		e.stagingBuffer.Release()
		e.stagingBuffer = nil
	}
	if e.outputBuffer != nil {
		e.outputBuffer.Release()
		e.outputBuffer = nil
	}
	if e.uniformBuffer != nil {
		e.uniformBuffer.Release()
		e.uniformBuffer = nil
	}
	if e.pointsBuffer != nil {
		e.pointsBuffer.Release()
		e.pointsBuffer = nil
	}
}

// SampleAll evaluates every point in the layout and returns a flat []float32
// of length len(layout.Points) * OutputChannels().
func (e *Evaluator) SampleAll(layout runtime.Layout, phase float32, params *runtime.BoundParams) ([]float32, error) {
	if err := e.ensureLayout(layout, phase, params); err != nil {
		return nil, err
	}
	if err := e.runOnce(); err != nil {
		return nil, err
	}
	return decodeFloat32s(e.readbackBytes), nil
}

// SamplePoints evaluates all layout points and returns values for only the
// requested indices. The result is a flat []float32 of length
// len(indices) * OutputChannels().
func (e *Evaluator) SamplePoints(layout runtime.Layout, indices []int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	all, err := e.SampleAll(layout, phase, params)
	if err != nil {
		return nil, err
	}
	out := make([]float32, len(indices)*e.channels)
	for i, idx := range indices {
		if idx < 0 || idx >= e.pointCount {
			return nil, fmt.Errorf("point index %d out of range (layout has %d points)", idx, e.pointCount)
		}
		copy(out[i*e.channels:], all[idx*e.channels:(idx+1)*e.channels])
	}
	return out, nil
}

// SamplePoint evaluates a single point.
func (e *Evaluator) SamplePoint(layout runtime.Layout, idx int, phase float32, params *runtime.BoundParams) ([]float32, error) {
	return e.SamplePoints(layout, []int{idx}, phase, params)
}

func (e *Evaluator) runOnce() error {
	encoder, err := e.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}
	pass.SetPipeline(e.pipeline)
	pass.SetBindGroup(0, e.bindGroup, nil)
	pass.Dispatch(uint32((e.pointCount+63)/64), 1, 1)
	if err := pass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}
	encoder.CopyBufferToBuffer(e.outputBuffer, 0, e.stagingBuffer, 0, uint64(len(e.readbackBytes)))

	commandBuffer, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish command encoder: %w", err)
	}

	if _, err := e.device.Queue().Submit(commandBuffer); err != nil {
		return fmt.Errorf("submit GPU commands: %w", err)
	}
	if err := e.device.WaitIdle(); err != nil {
		return fmt.Errorf("wait for GPU completion: %w", err)
	}
	if err := e.device.Queue().ReadBuffer(e.stagingBuffer, 0, e.readbackBytes); err != nil {
		return fmt.Errorf("read GPU output: %w", err)
	}
	return nil
}

// Close releases all GPU resources held by the evaluator.
func (e *Evaluator) Close() error {
	e.releaseTier2()
	if e.pipeline != nil {
		e.pipeline.Release()
	}
	if e.pipelineLayout != nil {
		e.pipelineLayout.Release()
	}
	if e.bgl != nil {
		e.bgl.Release()
	}
	if e.shader != nil {
		e.shader.Release()
	}
	if e.device != nil {
		e.device.Release()
	}
	if e.adapter != nil {
		e.adapter.Release()
	}
	if e.instance != nil {
		e.instance.Release()
	}
	return nil
}

// Helper functions ported from cmd/lfx/bench.go.

func encodePointsBuffer(layout runtime.Layout) []byte {
	data := make([]byte, len(layout.Points)*16)
	for i, pt := range layout.Points {
		offset := i * 16
		binary.LittleEndian.PutUint32(data[offset:], pt.Index)
		binary.LittleEndian.PutUint32(data[offset+4:], math.Float32bits(pt.X))
		binary.LittleEndian.PutUint32(data[offset+8:], math.Float32bits(pt.Y))
	}
	return data
}

func encodeUniformBuffer(layout runtime.Layout, phase float32, specs []ir.ParamSpec, params *runtime.BoundParams) []byte {
	numericSpecs := numericParamSpecs(specs)
	byteLength := alignTo16((4 + len(numericSpecs)) * 4)
	data := make([]byte, byteLength)
	binary.LittleEndian.PutUint32(data[0:], math.Float32bits(layout.Width))
	binary.LittleEndian.PutUint32(data[4:], math.Float32bits(layout.Height))
	binary.LittleEndian.PutUint32(data[8:], math.Float32bits(phase))
	binary.LittleEndian.PutUint32(data[12:], uint32(len(layout.Points)))
	for i, spec := range numericSpecs {
		binary.LittleEndian.PutUint32(data[16+i*4:], math.Float32bits(coerceUniformValue(params.Values[spec.Name])))
	}
	return data
}

func numericParamSpecs(specs []ir.ParamSpec) []ir.ParamSpec {
	filtered := make([]ir.ParamSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Type == ir.ParamEnum {
			continue
		}
		filtered = append(filtered, spec)
	}
	return filtered
}

func coerceUniformValue(value any) float32 {
	switch v := value.(type) {
	case float32:
		return v
	case float64:
		return float32(v)
	case int:
		return float32(v)
	case int64:
		return float32(v)
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func alignTo16(value int) int {
	return (value + 15) &^ 15
}

func decodeFloat32s(data []byte) []float32 {
	values := make([]float32, len(data)/4)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return values
}

func resolveComputeEntryPoint(wgslSource string, backend gputypes.Backend, entryPoint string) (string, error) {
	if backend != gputypes.BackendMetal {
		return entryPoint, nil
	}
	ast, err := naga.Parse(wgslSource)
	if err != nil {
		return "", err
	}
	module, err := naga.LowerWithSource(ast, wgslSource)
	if err != nil {
		return "", err
	}
	_, info, err := msl.Compile(module, msl.DefaultOptions())
	if err != nil {
		return "", err
	}
	if translated, ok := info.EntryPointNames[entryPoint]; ok && translated != "" {
		return translated, nil
	}
	return entryPoint, nil
}

func parseInstanceDescriptor(name string) (*wgpu.InstanceDescriptor, error) {
	switch strings.ToLower(name) {
	case "", "auto":
		return &wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary}, nil
	case "metal":
		return &wgpu.InstanceDescriptor{Backends: wgpu.BackendsMetal}, nil
	case "vulkan":
		return &wgpu.InstanceDescriptor{Backends: wgpu.BackendsVulkan}, nil
	case "dx12":
		return &wgpu.InstanceDescriptor{Backends: wgpu.BackendsDX12}, nil
	case "gl":
		return &wgpu.InstanceDescriptor{Backends: wgpu.BackendsGL}, nil
	default:
		return nil, fmt.Errorf("unknown GPU backend %q (want auto, metal, vulkan, dx12, gl)", name)
	}
}
