package cpu

import "github.com/pehringer/simd"

func simdAddFloat64(left, right, dst []float64) {
	n := simd.AddFloat64(left, right, dst)
	for idx := n; idx < len(dst) && idx < len(left) && idx < len(right); idx++ {
		dst[idx] = left[idx] + right[idx]
	}
}

func simdSubFloat64(left, right, dst []float64) {
	n := simd.SubFloat64(left, right, dst)
	for idx := n; idx < len(dst) && idx < len(left) && idx < len(right); idx++ {
		dst[idx] = left[idx] - right[idx]
	}
}

func simdMulFloat64(left, right, dst []float64) {
	n := simd.MulFloat64(left, right, dst)
	for idx := n; idx < len(dst) && idx < len(left) && idx < len(right); idx++ {
		dst[idx] = left[idx] * right[idx]
	}
}

func simdMinFloat64(left, right, dst []float64) {
	n := simd.MinFloat64(left, right, dst)
	for idx := n; idx < len(dst) && idx < len(left) && idx < len(right); idx++ {
		if left[idx] < right[idx] {
			dst[idx] = left[idx]
		} else {
			dst[idx] = right[idx]
		}
	}
}

func simdMaxFloat64(left, right, dst []float64) {
	n := simd.MaxFloat64(left, right, dst)
	for idx := n; idx < len(dst) && idx < len(left) && idx < len(right); idx++ {
		if left[idx] > right[idx] {
			dst[idx] = left[idx]
		} else {
			dst[idx] = right[idx]
		}
	}
}
