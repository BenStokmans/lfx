package runtime

import (
	"encoding/json"
	"fmt"
)

// ParseLayoutJSON decodes and validates an LFX JSON layout document.
func ParseLayoutJSON(data []byte) (Layout, error) {
	var layout Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return Layout{}, fmt.Errorf("decode layout json: %w", err)
	}
	if err := ValidateLayout(layout); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// ValidateLayout enforces the initial layout contract used by the preview app.
func ValidateLayout(layout Layout) error {
	if layout.Width <= 0 {
		return fmt.Errorf("layout width must be > 0")
	}
	if layout.Height <= 0 {
		return fmt.Errorf("layout height must be > 0")
	}
	if len(layout.Points) == 0 {
		return fmt.Errorf("layout must contain at least one point")
	}

	seen := make(map[uint32]struct{}, len(layout.Points))
	maxIndex := uint32(len(layout.Points) - 1)
	for i, pt := range layout.Points {
		if _, ok := seen[pt.Index]; ok {
			return fmt.Errorf("point %d has duplicate index %d", i, pt.Index)
		}
		seen[pt.Index] = struct{}{}
		if pt.Index > maxIndex {
			return fmt.Errorf("point %d has out-of-range index %d (max %d)", i, pt.Index, maxIndex)
		}
	}

	return nil
}
