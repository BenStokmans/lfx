package wgsl

// PointBufferStride returns the byte stride of a point in the storage buffer.
// Each point has: index (u32, 4 bytes), x (f32, 4 bytes), y (f32, 4 bytes), _pad (f32, 4 bytes).
func PointBufferStride() int { return 16 }
