package runtime

// Point represents a single LED position in the layout.
type Point struct {
	Index uint32
	X     float32
	Y     float32
}

// Layout describes the physical arrangement of LEDs.
type Layout struct {
	Width  float32
	Height float32
	Points []Point
}
