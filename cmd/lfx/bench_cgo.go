//go:build cgo

package main

import "fmt"

func runBench(args []string) error {
	return fmt.Errorf("bench requires CGO_ENABLED=0 when built with github.com/gogpu/wgpu")
}
