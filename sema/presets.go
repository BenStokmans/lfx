package sema

import "fmt"

// validatePresets checks that each preset's timing fields are properly ordered:
// 0 <= start <= loop_start <= loop_end <= finish <= 1
func (a *analyzer) validatePresets() {
	for _, p := range a.mod.Presets {
		start, hasStart := p.Fields["start"]
		loopStart, hasLoopStart := p.Fields["loop_start"]
		loopEnd, hasLoopEnd := p.Fields["loop_end"]
		finish, hasFinish := p.Fields["finish"]

		if hasStart && (start < 0 || start > 1) {
			a.addError(p.Pos, ErrPresetStartOutOfRange,
				fmt.Sprintf("preset %q: start (%v) must be in [0, 1]", p.Name, start))
		}
		if hasFinish && (finish < 0 || finish > 1) {
			a.addError(p.Pos, ErrPresetFinishOutOfRange,
				fmt.Sprintf("preset %q: finish (%v) must be in [0, 1]", p.Name, finish))
		}

		if hasStart && hasLoopStart && start > loopStart {
			a.addError(p.Pos, ErrPresetStartAfterLoopStart,
				fmt.Sprintf("preset %q: start (%v) must be <= loop_start (%v)", p.Name, start, loopStart))
		}
		if hasLoopStart && hasLoopEnd && loopStart > loopEnd {
			a.addError(p.Pos, ErrPresetLoopStartAfterLoopEnd,
				fmt.Sprintf("preset %q: loop_start (%v) must be <= loop_end (%v)", p.Name, loopStart, loopEnd))
		}
		if hasLoopEnd && hasFinish && loopEnd > finish {
			a.addError(p.Pos, ErrPresetLoopEndAfterFinish,
				fmt.Sprintf("preset %q: loop_end (%v) must be <= finish (%v)", p.Name, loopEnd, finish))
		}
		if hasStart && hasFinish && start > finish {
			a.addError(p.Pos, ErrPresetStartAfterFinish,
				fmt.Sprintf("preset %q: start (%v) must be <= finish (%v)", p.Name, start, finish))
		}
	}
}
