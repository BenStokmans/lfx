//go:build !cgo

package lfx

import (
	"github.com/BenStokmans/lfx/backend/gpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
)

func loadGPUEngine(result *compiler.Result, backend string) (*Engine, error) {
	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		return nil, err
	}
	if backend == "" {
		backend = "auto"
	}
	ev, err := gpu.NewEvaluator(result.IR, wgslSource, backend)
	if err != nil {
		return nil, err
	}
	return &Engine{
		sampler:  ev,
		mod:      result.IR,
		channels: result.IR.Output.Channels(),
	}, nil
}
