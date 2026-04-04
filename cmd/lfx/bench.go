//go:build !cgo

package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	lfxruntime "github.com/BenStokmans/lfx/runtime"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

func runBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	layoutPath := fs.String("layout", "", "path to layout JSON (optional; defaults to generated grids)")
	sizesFlag := fs.String("sizes", "8,32,128", "generated grid sizes when --layout is omitted")
	phase := fs.Float64("phase", 0, "normalized phase in [0,1]")
	rounds := fs.Int("rounds", 10, "number of measured rounds")
	warmup := fs.Int("warmup", 2, "number of warmup rounds")
	backend := fs.String("backend", "auto", "GPU backend: auto, metal, vulkan, dx12, gl")
	jsonOutput := fs.Bool("json", false, "emit JSON instead of text")
	var params kvFlags
	fs.Var(&params, "param", "parameter override in name=value form")

	filePath, opts, err := commonArgs(fs, args)
	if err != nil {
		return err
	}
	if *rounds <= 0 {
		return errors.New("bench requires --rounds > 0")
	}
	if *warmup < 0 {
		return errors.New("bench requires --warmup >= 0")
	}

	progress := progressReporter{enabled: !*jsonOutput}
	progress.Printf("compiling %s", filePath)
	result, err := compiler.CompileFile(filePath, opts)
	if err != nil {
		return err
	}

	layoutCases, err := resolveBenchLayouts(*layoutPath, *sizesFlag)
	if err != nil {
		return err
	}

	progress.Printf("binding parameters")
	boundParams, err := lfxruntime.Bind(result.IR.Params, params.Values())
	if err != nil {
		return err
	}

	phase32 := float32(*phase)

	progress.Printf("emitting WGSL")
	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		return err
	}

	summaries := make([]benchSummary, 0, len(layoutCases))
	for _, layoutCase := range layoutCases {
		progress.Printf("loading layout %s", layoutCase.label)
		summary, err := runBenchCase(filePath, wgslSource, result.IR, layoutCase, phase32, *phase, boundParams, *warmup, *rounds, *backend, progress)
		if err != nil {
			return err
		}
		summaries = append(summaries, summary)
	}

	if *jsonOutput {
		if len(summaries) == 1 {
			return writeJSON(summaries[0])
		}
		return writeJSON(struct {
			Cases []benchSummary `json:"cases"`
		}{Cases: summaries})
	}
	progress.Printf("writing summary")
	writeBenchSummaries(summaries)
	return nil
}

type benchLayoutCase struct {
	label  string
	layout lfxruntime.Layout
}

type progressReporter struct {
	enabled bool
}

func (p progressReporter) Printf(format string, args ...any) {
	if !p.enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[lfx bench] %s\n", fmt.Sprintf(format, args...))
}

func runBenchCase(filePath string, wgslSource string, mod *ir.Module, layoutCase benchLayoutCase, phase32 float32, phase64 float64, params *lfxruntime.BoundParams, warmup int, rounds int, backend string, progress progressReporter) (benchSummary, error) {
	pointIndices := make([]int, len(layoutCase.layout.Points))
	for i := range pointIndices {
		pointIndices[i] = i
	}

	progress.Printf("running CPU benchmark (%d warmup, %d measured)", warmup, rounds)
	cpuValues, cpuStats, err := benchmarkCPU(mod, layoutCase.layout, pointIndices, phase32, params, warmup, rounds, progress)
	if err != nil {
		return benchSummary{}, err
	}

	progress.Printf("initializing GPU backend (%s)", backend)
	runner, err := newGPUFrameRunner(wgslSource, mod, layoutCase.layout, phase32, params, backend)
	if err != nil {
		return benchSummary{}, err
	}
	defer runner.Close()

	progress.Printf("running GPU benchmark (%d warmup, %d measured)", warmup, rounds)
	gpuValues, gpuStats, err := runner.Benchmark(warmup, rounds, progress)
	if err != nil {
		return benchSummary{}, err
	}

	summary := benchSummary{
		Module: filePath,
		Layout: layoutCase.label,
		Output: outputTypeName(mod.Output),
		Points: len(layoutCase.layout.Points),
		Phase:  phase64,
		CPU:    cpuStats,
		GPU:    gpuStats,
		Delta: benchDelta{
			MaxAbs: maxAbsDelta(cpuValues, gpuValues),
		},
	}
	if runner.adapterInfo != nil {
		summary.GPU.Adapter = runner.adapterInfo.Name
		summary.GPU.DeviceType = runner.adapterInfo.DeviceType.String()
		summary.GPU.Backend = runner.adapterInfo.Backend.String()
	}
	if runner.timestampSupported {
		summary.GPU.TimestampSupport = "adapter reports timestamp-query support, but this harness uses native wall-clock timing"
	} else {
		summary.GPU.TimestampSupport = "timestamp-query not reported by adapter; using native wall-clock timing"
	}
	return summary, nil
}

type benchSummary struct {
	Module string     `json:"module"`
	Layout string     `json:"layout"`
	Output string     `json:"output"`
	Points int        `json:"points"`
	Phase  float64    `json:"phase"`
	CPU    cpuSummary `json:"cpu"`
	GPU    gpuSummary `json:"gpu"`
	Delta  benchDelta `json:"delta"`
}

type benchDelta struct {
	MaxAbs float32 `json:"maxAbs"`
}

type cpuSummary struct {
	Rounds int     `json:"rounds"`
	Warmup int     `json:"warmup"`
	AvgMS  float64 `json:"avgMs"`
	MinMS  float64 `json:"minMs"`
	MaxMS  float64 `json:"maxMs"`
}

type gpuSummary struct {
	Rounds           int     `json:"rounds"`
	Warmup           int     `json:"warmup"`
	AvgDispatchMS    float64 `json:"avgDispatchMs"`
	AvgReadbackMS    float64 `json:"avgReadbackMs"`
	AvgTotalMS       float64 `json:"avgTotalMs"`
	MinTotalMS       float64 `json:"minTotalMs"`
	MaxTotalMS       float64 `json:"maxTotalMs"`
	Adapter          string  `json:"adapter,omitempty"`
	DeviceType       string  `json:"deviceType,omitempty"`
	Backend          string  `json:"backend,omitempty"`
	TimestampSupport string  `json:"timestampSupport,omitempty"`
}

type durationStats struct {
	total time.Duration
	min   time.Duration
	max   time.Duration
	count int
}

func (s *durationStats) Add(d time.Duration) {
	if s.count == 0 || d < s.min {
		s.min = d
	}
	if d > s.max {
		s.max = d
	}
	s.total += d
	s.count++
}

func (s durationStats) avgMS() float64 {
	if s.count == 0 {
		return 0
	}
	return durationMilliseconds(s.total) / float64(s.count)
}

func (s durationStats) minMS() float64 {
	return durationMilliseconds(s.min)
}

func (s durationStats) maxMS() float64 {
	return durationMilliseconds(s.max)
}

func benchmarkCPU(mod *ir.Module, layout lfxruntime.Layout, pointIndices []int, phase float32, params *lfxruntime.BoundParams, warmup, rounds int, progress progressReporter) ([]float32, cpuSummary, error) {
	evaluator := cpu.NewEvaluator(mod)

	if warmup > 0 {
		progress.Printf("CPU warmup started")
	}
	for i := 0; i < warmup; i++ {
		if _, err := evaluator.SamplePoints(layout, pointIndices, phase, params); err != nil {
			return nil, cpuSummary{}, err
		}
	}
	if warmup > 0 {
		progress.Printf("CPU warmup complete")
	}

	var stats durationStats
	var values []float32
	progress.Printf("CPU measurement started")
	for i := 0; i < rounds; i++ {
		start := time.Now()
		current, err := evaluator.SamplePoints(layout, pointIndices, phase, params)
		if err != nil {
			return nil, cpuSummary{}, err
		}
		stats.Add(time.Since(start))
		values = current
	}
	progress.Printf("CPU measurement complete")

	return values, cpuSummary{
		Rounds: rounds,
		Warmup: warmup,
		AvgMS:  stats.avgMS(),
		MinMS:  stats.minMS(),
		MaxMS:  stats.maxMS(),
	}, nil
}

type gpuFrameRunner struct {
	instance           *wgpu.Instance
	adapter            *wgpu.Adapter
	device             *wgpu.Device
	shader             *wgpu.ShaderModule
	bindGroupLayout    *wgpu.BindGroupLayout
	pipelineLayout     *wgpu.PipelineLayout
	bindGroup          *wgpu.BindGroup
	pipeline           *wgpu.ComputePipeline
	pointsBuffer       *wgpu.Buffer
	uniformBuffer      *wgpu.Buffer
	outputBuffer       *wgpu.Buffer
	stagingBuffer      *wgpu.Buffer
	readbackBytes      []byte
	pointCount         int
	channels           int
	adapterInfo        *wgpu.AdapterInfo
	timestampSupported bool
}

func newGPUFrameRunner(wgslSource string, mod *ir.Module, layout lfxruntime.Layout, phase float32, params *lfxruntime.BoundParams, backend string) (*gpuFrameRunner, error) {
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
		return nil, fmt.Errorf("request GPU device: got mock device for adapter %q (%s); no real gogpu/wgpu backend is available in this runtime", adapter.Info().Name, adapter.Info().Backend.String())
	}

	channels := mod.Output.Channels()
	pointsBytes := encodePointsBuffer(layout)
	uniformBytes := encodeUniformBuffer(layout, phase, mod.Params, params)
	outputSize := uint64(len(layout.Points) * channels * 4)

	pointsBuffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-points",
		Size:  uint64(len(pointsBytes)),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create points buffer: %w", err)
	}

	uniformBuffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-uniforms",
		Size:  uint64(len(uniformBytes)),
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create uniform buffer: %w", err)
	}

	outputBuffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-output",
		Size:  outputSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create output buffer: %w", err)
	}

	stagingBuffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lfx-staging",
		Size:  outputSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create staging buffer: %w", err)
	}

	if err := device.Queue().WriteBuffer(pointsBuffer, 0, pointsBytes); err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("write points buffer: %w", err)
	}
	if err := device.Queue().WriteBuffer(uniformBuffer, 0, uniformBytes); err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("write uniform buffer: %w", err)
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "lfx-sample",
		WGSL:  wgslSource,
	})
	if err != nil {
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create shader module: %w", err)
	}

	computeEntryPoint, err := resolveComputeEntryPoint(wgslSource, adapter.Info().Backend, "main")
	if err != nil {
		shader.Release()
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("resolve compute entry point: %w", err)
	}

	bindGroupLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "lfx-bind-group-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: uint64(len(uniformBytes))}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		shader.Release()
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create bind group layout: %w", err)
	}

	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "lfx-bind-group",
		Layout: bindGroupLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: pointsBuffer, Size: uint64(len(pointsBytes))},
			{Binding: 1, Buffer: uniformBuffer, Size: uint64(len(uniformBytes))},
			{Binding: 2, Buffer: outputBuffer, Size: outputSize},
		},
	})
	if err != nil {
		bindGroupLayout.Release()
		shader.Release()
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create bind group: %w", err)
	}

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "lfx-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bindGroupLayout},
	})
	if err != nil {
		bindGroup.Release()
		bindGroupLayout.Release()
		shader.Release()
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create pipeline layout: %w", err)
	}

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "lfx-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: computeEntryPoint,
	})
	if err != nil {
		pipelineLayout.Release()
		bindGroup.Release()
		bindGroupLayout.Release()
		shader.Release()
		stagingBuffer.Release()
		outputBuffer.Release()
		uniformBuffer.Release()
		pointsBuffer.Release()
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("create compute pipeline: %w", err)
	}

	info := adapter.Info()
	infoCopy := info

	return &gpuFrameRunner{
		instance:           instance,
		adapter:            adapter,
		device:             device,
		shader:             shader,
		bindGroupLayout:    bindGroupLayout,
		pipelineLayout:     pipelineLayout,
		bindGroup:          bindGroup,
		pipeline:           pipeline,
		pointsBuffer:       pointsBuffer,
		uniformBuffer:      uniformBuffer,
		outputBuffer:       outputBuffer,
		stagingBuffer:      stagingBuffer,
		readbackBytes:      make([]byte, outputSize),
		pointCount:         len(layout.Points),
		channels:           channels,
		adapterInfo:        &infoCopy,
		timestampSupported: adapter.Features().Contains(gputypes.FeatureTimestampQuery),
	}, nil
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

func (r *gpuFrameRunner) Close() {
	if r == nil {
		return
	}
	if r.stagingBuffer != nil {
		r.stagingBuffer.Release()
	}
	if r.outputBuffer != nil {
		r.outputBuffer.Release()
	}
	if r.uniformBuffer != nil {
		r.uniformBuffer.Release()
	}
	if r.pointsBuffer != nil {
		r.pointsBuffer.Release()
	}
	if r.pipeline != nil {
		r.pipeline.Release()
	}
	if r.pipelineLayout != nil {
		r.pipelineLayout.Release()
	}
	if r.bindGroup != nil {
		r.bindGroup.Release()
	}
	if r.bindGroupLayout != nil {
		r.bindGroupLayout.Release()
	}
	if r.shader != nil {
		r.shader.Release()
	}
	if r.device != nil {
		r.device.Release()
	}
	if r.adapter != nil {
		r.adapter.Release()
	}
	if r.instance != nil {
		r.instance.Release()
	}
}

func (r *gpuFrameRunner) Benchmark(warmup, rounds int, progress progressReporter) ([]float32, gpuSummary, error) {
	if warmup > 0 {
		progress.Printf("GPU warmup started")
	}
	for i := 0; i < warmup; i++ {
		if _, err := r.runOnce(); err != nil {
			return nil, gpuSummary{}, err
		}
	}
	if warmup > 0 {
		progress.Printf("GPU warmup complete")
	}

	var dispatchStats durationStats
	var readbackStats durationStats
	var totalStats durationStats
	progress.Printf("GPU measurement started")
	for i := 0; i < rounds; i++ {
		metrics, err := r.runOnce()
		if err != nil {
			return nil, gpuSummary{}, err
		}
		dispatchStats.Add(metrics.dispatch)
		readbackStats.Add(metrics.readback)
		totalStats.Add(metrics.total)
	}
	progress.Printf("GPU measurement complete")

	return decodeFloat32s(r.readbackBytes), gpuSummary{
		Rounds:        rounds,
		Warmup:        warmup,
		AvgDispatchMS: dispatchStats.avgMS(),
		AvgReadbackMS: readbackStats.avgMS(),
		AvgTotalMS:    totalStats.avgMS(),
		MinTotalMS:    totalStats.minMS(),
		MaxTotalMS:    totalStats.maxMS(),
	}, nil
}

type gpuRoundMetrics struct {
	dispatch time.Duration
	readback time.Duration
	total    time.Duration
}

func (r *gpuFrameRunner) runOnce() (gpuRoundMetrics, error) {
	encoder, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("create command encoder: %w", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("begin compute pass: %w", err)
	}
	pass.SetPipeline(r.pipeline)
	pass.SetBindGroup(0, r.bindGroup, nil)
	pass.Dispatch(uint32((r.pointCount+63)/64), 1, 1)
	if err := pass.End(); err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("end compute pass: %w", err)
	}
	encoder.CopyBufferToBuffer(r.outputBuffer, 0, r.stagingBuffer, 0, uint64(len(r.readbackBytes)))

	commandBuffer, err := encoder.Finish()
	if err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("finish command encoder: %w", err)
	}

	submitStart := time.Now()
	_, err = r.device.Queue().Submit(commandBuffer)
	if err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("submit GPU commands: %w", err)
	}
	if err := r.device.WaitIdle(); err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("wait for GPU completion: %w", err)
	}
	dispatchDuration := time.Since(submitStart)

	readbackStart := time.Now()
	if err := r.device.Queue().ReadBuffer(r.stagingBuffer, 0, r.readbackBytes); err != nil {
		return gpuRoundMetrics{}, fmt.Errorf("read GPU output: %w", err)
	}
	readbackDuration := time.Since(readbackStart)

	return gpuRoundMetrics{
		dispatch: dispatchDuration,
		readback: readbackDuration,
		total:    dispatchDuration + readbackDuration,
	}, nil
}

func encodePointsBuffer(layout lfxruntime.Layout) []byte {
	data := make([]byte, len(layout.Points)*16)
	for i, pt := range layout.Points {
		offset := i * 16
		binary.LittleEndian.PutUint32(data[offset:], pt.Index)
		binary.LittleEndian.PutUint32(data[offset+4:], math.Float32bits(pt.X))
		binary.LittleEndian.PutUint32(data[offset+8:], math.Float32bits(pt.Y))
	}
	return data
}

func encodeUniformBuffer(layout lfxruntime.Layout, phase float32, specs []ir.ParamSpec, params *lfxruntime.BoundParams) []byte {
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

func maxAbsDelta(left, right []float32) float32 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	var maxDelta float32
	for i := 0; i < n; i++ {
		delta := float32(math.Abs(float64(left[i] - right[i])))
		if delta > maxDelta {
			maxDelta = delta
		}
	}
	return maxDelta
}

func resolveBenchLayouts(layoutPath string, sizesFlag string) ([]benchLayoutCase, error) {
	if layoutPath != "" {
		layout, err := loadLayout(layoutPath)
		if err != nil {
			return nil, err
		}
		return []benchLayoutCase{{
			label:  layoutPath,
			layout: layout,
		}}, nil
	}

	sizes, err := parseGridSizes(sizesFlag)
	if err != nil {
		return nil, err
	}
	cases := make([]benchLayoutCase, 0, len(sizes))
	for _, size := range sizes {
		cases = append(cases, benchLayoutCase{
			label:  fmt.Sprintf("generated %dx%d", size, size),
			layout: generateGridLayout(size, size),
		})
	}
	return cases, nil
}

func parseGridSizes(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	sizes := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		size, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid grid size %q", part)
		}
		if size <= 0 {
			return nil, fmt.Errorf("grid size must be > 0: %d", size)
		}
		sizes = append(sizes, size)
	}
	if len(sizes) == 0 {
		return nil, errors.New("bench requires at least one generated grid size")
	}
	return sizes, nil
}

func generateGridLayout(width int, height int) lfxruntime.Layout {
	points := make([]lfxruntime.Point, 0, width*height)
	var index uint32
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			points = append(points, lfxruntime.Point{
				Index: index,
				X:     float32(x),
				Y:     float32(y),
			})
			index++
		}
	}
	return lfxruntime.Layout{
		Width:  float32(width),
		Height: float32(height),
		Points: points,
	}
}

func durationMilliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func outputTypeName(output ir.OutputType) string {
	switch output {
	case ir.OutputRGB:
		return "rgb"
	case ir.OutputRGBW:
		return "rgbw"
	default:
		return "scalar"
	}
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

func writeBenchText(summary benchSummary) {
	fmt.Printf("module: %s\n", summary.Module)
	fmt.Printf("layout: %s (%d points, %s)\n", summary.Layout, summary.Points, summary.Output)
	fmt.Printf("phase: %.3f\n", summary.Phase)
	if summary.GPU.Adapter != "" {
		fmt.Printf("gpu: %s [%s, %s]\n", summary.GPU.Adapter, summary.GPU.Backend, summary.GPU.DeviceType)
	}
	fmt.Printf("cpu avg: %.3f ms/frame (min %.3f, max %.3f over %d rounds)\n",
		summary.CPU.AvgMS, summary.CPU.MinMS, summary.CPU.MaxMS, summary.CPU.Rounds)
	fmt.Printf("gpu avg: %.3f ms/frame total = %.3f dispatch + %.3f readback (min %.3f, max %.3f over %d rounds)\n",
		summary.GPU.AvgTotalMS, summary.GPU.AvgDispatchMS, summary.GPU.AvgReadbackMS,
		summary.GPU.MinTotalMS, summary.GPU.MaxTotalMS, summary.GPU.Rounds)
	fmt.Printf("timing: %s\n", summary.GPU.TimestampSupport)
	fmt.Printf("max delta: %.6f\n", summary.Delta.MaxAbs)
}

func writeBenchSummaries(summaries []benchSummary) {
	for i, summary := range summaries {
		if i > 0 {
			fmt.Println()
		}
		writeBenchText(summary)
	}
}
