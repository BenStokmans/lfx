package sema_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/sema"
	"github.com/BenStokmans/lfx/stdlib"
)

func TestAnalyzeRejectsDirectRecursion(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/direct_rec"
effect "direct_rec"
output scalar
function loop_forever(x)
  return loop_forever(x)
end
function sample(width, height, x, y, index, phase, params)
  return loop_forever(phase)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrRecursionDetected)
}

func TestAnalyzeRejectsSampleDirectlyCallingItself(t *testing.T) {
	// sample cannot call sample directly in the same module.
	mod := parseOrFatal(t, `module "effects/self_sample"
effect "self_sample"
output scalar
function sample(width, height, x, y, index, phase, params)
  return sample(width, height, x, y, index, phase, params)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrRecursionDetected)
}

func TestAnalyzeRejectsMutualRecursionInSameFile(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/mutual_rec"
effect "mutual_rec"
output scalar
function ping(x)
  return pong(x)
end
function pong(x)
  return ping(x)
end
function sample(width, height, x, y, index, phase, params)
  return ping(phase)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrRecursionDetected)
}

func TestBuildRejectsCrossModuleMutualRecursionAsImportCycle(t *testing.T) {
	// lib_a imports lib_b, lib_b imports lib_a: mutual recursion across modules
	// is structurally an import cycle and is caught by modules.Build.
	root := t.TempDir()
	libAPath := filepath.Join(root, "lib_a.lfx")
	libBPath := filepath.Join(root, "lib_b.lfx")

	libASrc := `module "lib_a"
library "lib_a"
import "lib_b" as lb
export function fa(x)
  return lb.fb(x)
end
`
	libBSrc := `module "lib_b"
library "lib_b"
import "lib_a" as la
export function fb(x)
  return la.fa(x)
end
`
	if err := os.WriteFile(libAPath, []byte(libASrc), 0o644); err != nil {
		t.Fatalf("write lib_a: %v", err)
	}
	if err := os.WriteFile(libBPath, []byte(libBSrc), 0o644); err != nil {
		t.Fatalf("write lib_b: %v", err)
	}

	effectPath := filepath.Join(root, "effect.lfx")
	effectSrc := `module "effects/e"
effect "e"
output scalar
import "lib_a" as la
function sample(width, height, x, y, index, phase, params)
  return la.fa(phase)
end
`
	if err := os.WriteFile(effectPath, []byte(effectSrc), 0o644); err != nil {
		t.Fatalf("write effect: %v", err)
	}

	_, err := compiler.CompileFile(effectPath, compiler.Options{
		BaseDir:  root,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(root)),
	})
	if err == nil {
		t.Fatal("expected error for cross-module mutual recursion (import cycle)")
	}
}

func TestAnalyzeRejectsFourFunctionMutualRecursionCycle(t *testing.T) {
	mod := parseOrFatal(t, `module "effects/four_cycle"
effect "four_cycle"
output scalar
function f1(x)
  return f2(x)
end
function f2(x)
  return f3(x)
end
function f3(x)
  return f4(x)
end
function f4(x)
  return f1(x)
end
function sample(width, height, x, y, index, phase, params)
  return f1(phase)
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrRecursionDetected)
}

func TestAnalyzeRejectsUnusedRecursiveHelper(t *testing.T) {
	// The recursive helper is never called from sample, but should still be rejected.
	mod := parseOrFatal(t, `module "effects/unused_rec"
effect "unused_rec"
output scalar
function dead_recursive(n)
  return dead_recursive(n)
end
function sample(width, height, x, y, index, phase, params)
  return phase
end
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrRecursionDetected)
}
