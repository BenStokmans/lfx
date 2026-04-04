package sema

import "fmt"

// validateTimeline checks that the optional timeline block's loop markers are
// properly ordered and within range: 0 <= loop_start <= loop_end <= 1
func (a *analyzer) validateTimeline() {
	tl := a.mod.Timeline
	if tl == nil {
		return
	}

	if tl.LoopStart != nil {
		if *tl.LoopStart < 0 || *tl.LoopStart > 1 {
			a.addError(tl.Pos, ErrTimelineLoopStartOutOfRange,
				fmt.Sprintf("timeline: loop_start (%v) must be in [0, 1]", *tl.LoopStart))
		}
	}

	if tl.LoopEnd != nil {
		if *tl.LoopEnd < 0 || *tl.LoopEnd > 1 {
			a.addError(tl.Pos, ErrTimelineLoopEndOutOfRange,
				fmt.Sprintf("timeline: loop_end (%v) must be in [0, 1]", *tl.LoopEnd))
		}
	}

	if tl.LoopStart != nil && tl.LoopEnd != nil && *tl.LoopStart > *tl.LoopEnd {
		a.addError(tl.Pos, ErrTimelineLoopStartAfterLoopEnd,
			fmt.Sprintf("timeline: loop_start (%v) must be <= loop_end (%v)", *tl.LoopStart, *tl.LoopEnd))
	}
}
