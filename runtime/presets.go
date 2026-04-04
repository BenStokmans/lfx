package runtime

import (
	"fmt"

	"github.com/BenStokmans/lfx/ir"
)

// ValidatePreset checks the preset ordering constraint:
// 0 <= start <= loop_start <= loop_end <= finish <= 1
func ValidatePreset(p ir.PresetSpec) error {
	if p.Start < 0 {
		return fmt.Errorf("preset %q: start (%g) must be >= 0", p.Name, p.Start)
	}
	if p.Start > p.LoopStart {
		return fmt.Errorf("preset %q: start (%g) must be <= loop_start (%g)", p.Name, p.Start, p.LoopStart)
	}
	if p.LoopStart > p.LoopEnd {
		return fmt.Errorf("preset %q: loop_start (%g) must be <= loop_end (%g)", p.Name, p.LoopStart, p.LoopEnd)
	}
	if p.LoopEnd > p.Finish {
		return fmt.Errorf("preset %q: loop_end (%g) must be <= finish (%g)", p.Name, p.LoopEnd, p.Finish)
	}
	if p.Finish > 1 {
		return fmt.Errorf("preset %q: finish (%g) must be <= 1", p.Name, p.Finish)
	}
	return nil
}
