package runtime

// Sampler is the main evaluation interface for computing LED output values.
type Sampler interface {
	SamplePoint(layout Layout, pointIndex int, phase float32, params *BoundParams) (float32, error)
}
