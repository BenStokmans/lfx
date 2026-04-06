package sema_test

import (
	"testing"

	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/sema"
)

// minimalScalarEffect wraps extra LFX source in a minimal valid scalar effect.
func minimalScalarEffect(extra string) string {
	return `module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
` + extra
}

func TestAnalyzeRejectsTimelineLoopStartBelowZero(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_start = -0.1
  loop_end   = 0.5
}
`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrTimelineLoopStartOutOfRange)
}

func TestAnalyzeRejectsTimelineLoopEndAboveOne(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_start = 0.5
  loop_end   = 1.1
}
`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrTimelineLoopEndOutOfRange)
}

func TestAnalyzeRejectsTimelineLoopStartAfterLoopEnd(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_start = 0.9
  loop_end   = 0.1
}
`))
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrTimelineLoopStartAfterLoopEnd)
}

func TestAnalyzeAcceptsTimelineLoopStartEqualsLoopEnd(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_start = 0.5
  loop_end   = 0.5
}
`))
	errs := sema.Analyze(mod, nil)
	if len(errs) != 0 {
		t.Fatalf("loop_start == loop_end should be accepted, got errors: %v", errs)
	}
}

func TestAnalyzeAcceptsTimelineWithOnlyLoopStart(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_start = 0.25
}
`))
	errs := sema.Analyze(mod, nil)
	if len(errs) != 0 {
		t.Fatalf("timeline with only loop_start should be accepted, got: %v", errs)
	}
}

func TestAnalyzeAcceptsTimelineWithOnlyLoopEnd(t *testing.T) {
	mod := parseOrFatal(t, minimalScalarEffect(`timeline {
  loop_end = 0.75
}
`))
	errs := sema.Analyze(mod, nil)
	if len(errs) != 0 {
		t.Fatalf("timeline with only loop_end should be accepted, got: %v", errs)
	}
}

func TestParseRejectsDuplicateTimelineBlocks(t *testing.T) {
	_, err := parser.Parse(`module "effects/dup_tl"
effect "dup_tl"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
timeline {
  loop_start = 0.0
}
timeline {
  loop_end = 1.0
}
`)
	if err == nil {
		t.Fatal("expected parse error for duplicate timeline blocks")
	}
}

func TestParseRejectsUnknownTimelineField(t *testing.T) {
	_, err := parser.Parse(minimalScalarEffect(`timeline {
  speed = 1.0
}
`))
	if err == nil {
		t.Fatal("expected parse error for unknown timeline field 'speed'")
	}
}

func TestAnalyzeRejectsTimelineInLibrary(t *testing.T) {
	mod := parseOrFatal(t, `module "lib/tl"
library "tl"
export function fn(x)
  return x
end
timeline {
  loop_start = 0.0
  loop_end   = 1.0
}
`)
	errs := sema.Analyze(mod, nil)
	expectError(t, errs, sema.ErrLibraryHasTimeline)
}

// ── Boundary acceptance tests ─────────────────────────────────────────────────

func TestAnalyzeAcceptsTimelineExactBoundaries(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"loop_start=0 loop_end=0", "timeline {\n  loop_start = 0.0\n  loop_end = 0.0\n}\n"},
		{"loop_start=1 loop_end=1", "timeline {\n  loop_start = 1.0\n  loop_end = 1.0\n}\n"},
		{"loop_start=0 loop_end=1", "timeline {\n  loop_start = 0.0\n  loop_end = 1.0\n}\n"},
		{"loop_start=0.3 loop_end=0.7", "timeline {\n  loop_start = 0.3\n  loop_end = 0.7\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := parseOrFatal(t, minimalScalarEffect(tc.src))
			errs := sema.Analyze(mod, nil)
			if len(errs) != 0 {
				t.Fatalf("timeline %s should be accepted, got: %v", tc.name, errs)
			}
		})
	}
}
