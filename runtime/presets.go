package runtime

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
)

// ValidateTimeline checks that a TimelineSpec's loop markers are within range
// and properly ordered: 0 <= loop_start <= loop_end <= 1
func ValidateTimeline(t *ir.TimelineSpec) error {
	if t == nil {
		return nil
	}
	if t.LoopStart != nil {
		if *t.LoopStart < 0 || *t.LoopStart > 1 {
			return fmt.Errorf("timeline: loop_start (%g) must be in [0, 1]", *t.LoopStart)
		}
	}
	if t.LoopEnd != nil {
		if *t.LoopEnd < 0 || *t.LoopEnd > 1 {
			return fmt.Errorf("timeline: loop_end (%g) must be in [0, 1]", *t.LoopEnd)
		}
	}
	if t.LoopStart != nil && t.LoopEnd != nil && *t.LoopStart > *t.LoopEnd {
		return fmt.Errorf("timeline: loop_start (%g) must be <= loop_end (%g)", *t.LoopStart, *t.LoopEnd)
	}
	return nil
}
