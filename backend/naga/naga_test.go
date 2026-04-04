package naga_test

import (
	"testing"

	"github.com/BenStokmans/lfx/backend/naga"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

const filledEffect = `module "effects/filled"
effect "filled"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 1.0
end
`

func emitWGSL(t *testing.T, source string) string {
	t.Helper()
	mod, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info, errs, warns := sema.AnalyzeModule(mod, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("sema: %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected semantic warnings: %v", warns)
	}
	irmod, err := lower.Lower(mod, nil, info, nil)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	lower.ConstFold(irmod)
	wgslSource, err := wgsl.Emit(irmod)
	if err != nil {
		t.Fatalf("emit wgsl: %v", err)
	}
	return wgslSource
}

func TestCompileAllTargets(t *testing.T) {
	wgslSource := emitWGSL(t, filledEffect)

	targets := []naga.Target{
		naga.TargetSPIRV,
		naga.TargetMSL,
		naga.TargetGLSL,
		naga.TargetHLSL,
	}

	for _, target := range targets {
		t.Run(target.String(), func(t *testing.T) {
			result, err := naga.Compile(wgslSource, target)
			if err != nil {
				t.Fatalf("compile to %s: %v", target, err)
			}
			if target == naga.TargetSPIRV {
				if len(result.Bytes) == 0 {
					t.Fatal("spirv output is empty")
				}
			} else {
				if result.Code == "" {
					t.Fatalf("%s output is empty", target)
				}
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input string
		want  naga.Target
		err   bool
	}{
		{"spirv", naga.TargetSPIRV, false},
		{"msl", naga.TargetMSL, false},
		{"glsl", naga.TargetGLSL, false},
		{"hlsl", naga.TargetHLSL, false},
		{"invalid", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := naga.ParseTarget(tt.input)
			if (err != nil) != tt.err {
				t.Fatalf("ParseTarget(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			}
			if !tt.err && got != tt.want {
				t.Fatalf("ParseTarget(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompileInvalidWGSL(t *testing.T) {
	_, err := naga.Compile("this is not valid wgsl", naga.TargetSPIRV)
	if err == nil {
		t.Fatal("expected error for invalid WGSL")
	}
}
