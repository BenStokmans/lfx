//go:build cgo

package lfx

import (
	"errors"

	"github.com/BenStokmans/lfx/compiler"
)

func loadGPUEngine(result *compiler.Result, _ string) (*Engine, error) {
	_ = result
	return nil, errors.New("lfx: GPU backend requires CGO_ENABLED=0 (build with github.com/gogpu/wgpu)")
}
