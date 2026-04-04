package cpu

import (
	"math"

	"github.com/BenStokmans/lfx/ir"
)

type value struct {
	Typ   ir.Type
	Lanes [4]float64
}

func zeroValue(typ ir.Type) value {
	if typ == ir.TypeUnknown || typ == ir.TypeVoid {
		typ = ir.TypeF32
	}
	return value{Typ: typ}
}

func scalarValue(typ ir.Type, v float64) value {
	if typ == ir.TypeUnknown || typ.IsVector() || typ == ir.TypeVoid {
		typ = ir.TypeF32
	}
	out := value{Typ: typ}
	out.Lanes[0] = v
	return out
}

func boolValue(v bool) value {
	if v {
		return scalarValue(ir.TypeBool, 1)
	}
	return scalarValue(ir.TypeBool, 0)
}

func vectorValue(typ ir.Type, elems ...float64) value {
	out := value{Typ: typ}
	if len(elems) > 0 {
		out.Lanes[0] = elems[0]
	}
	if len(elems) > 1 {
		out.Lanes[1] = elems[1]
	}
	if len(elems) > 2 {
		out.Lanes[2] = elems[2]
	}
	if len(elems) > 3 {
		out.Lanes[3] = elems[3]
	}
	return out
}

func (v value) laneCount() int {
	if v.Typ.IsVector() {
		return v.Typ.Lanes()
	}
	return 1
}

func (v value) scalar() float64 {
	return v.Lanes[0]
}

func (v value) truthy() bool {
	return v.scalar() != 0
}

func (v value) component(index int) value {
	return scalarValue(ir.TypeF32, v.Lanes[index])
}

func broadcastValue(v value, target ir.Type) value {
	if !target.IsVector() || v.Typ.IsVector() {
		return v
	}
	x := v.scalar()
	switch target {
	case ir.TypeVec2:
		return value{Typ: target, Lanes: [4]float64{x, x}}
	case ir.TypeVec3:
		return value{Typ: target, Lanes: [4]float64{x, x, x}}
	case ir.TypeVec4:
		return value{Typ: target, Lanes: [4]float64{x, x, x, x}}
	default:
		return v
	}
}

func liftBinary(left, right value) (value, value, ir.Type) {
	if left.Typ.IsVector() {
		return left, broadcastValue(right, left.Typ), left.Typ
	}
	if right.Typ.IsVector() {
		return broadcastValue(left, right.Typ), right, right.Typ
	}
	return left, right, mergeScalarTypes(left.Typ, right.Typ)
}

func mergeScalarTypes(left, right ir.Type) ir.Type {
	if left == ir.TypeF32 || right == ir.TypeF32 {
		return ir.TypeF32
	}
	if left == ir.TypeBool || right == ir.TypeBool {
		return ir.TypeBool
	}
	if left == ir.TypeUnknown {
		return right
	}
	if right == ir.TypeUnknown {
		return left
	}
	return ir.TypeI32
}

func vectorLen(v value) float64 {
	switch v.Typ {
	case ir.TypeVec2:
		x := v.Lanes[0]
		y := v.Lanes[1]
		return math.Sqrt(x*x + y*y)
	case ir.TypeVec3:
		x := v.Lanes[0]
		y := v.Lanes[1]
		z := v.Lanes[2]
		return math.Sqrt(x*x + y*y + z*z)
	case ir.TypeVec4:
		x := v.Lanes[0]
		y := v.Lanes[1]
		z := v.Lanes[2]
		w := v.Lanes[3]
		return math.Sqrt(x*x + y*y + z*z + w*w)
	default:
		x := v.Lanes[0]
		return math.Sqrt(x * x)
	}
}

func mapUnary(v value, fn func(float64) float64) value {
	switch v.Typ {
	case ir.TypeVec2:
		return value{Typ: v.Typ, Lanes: [4]float64{fn(v.Lanes[0]), fn(v.Lanes[1])}}
	case ir.TypeVec3:
		return value{Typ: v.Typ, Lanes: [4]float64{fn(v.Lanes[0]), fn(v.Lanes[1]), fn(v.Lanes[2])}}
	case ir.TypeVec4:
		return value{Typ: v.Typ, Lanes: [4]float64{fn(v.Lanes[0]), fn(v.Lanes[1]), fn(v.Lanes[2]), fn(v.Lanes[3])}}
	default:
		return scalarValue(v.Typ, fn(v.Lanes[0]))
	}
}

func mapBinary(left, right value, target ir.Type, fn func(float64, float64) float64) value {
	switch target {
	case ir.TypeVec2:
		return value{Typ: target, Lanes: [4]float64{
			fn(left.Lanes[0], right.Lanes[0]),
			fn(left.Lanes[1], right.Lanes[1]),
		}}
	case ir.TypeVec3:
		return value{Typ: target, Lanes: [4]float64{
			fn(left.Lanes[0], right.Lanes[0]),
			fn(left.Lanes[1], right.Lanes[1]),
			fn(left.Lanes[2], right.Lanes[2]),
		}}
	case ir.TypeVec4:
		return value{Typ: target, Lanes: [4]float64{
			fn(left.Lanes[0], right.Lanes[0]),
			fn(left.Lanes[1], right.Lanes[1]),
			fn(left.Lanes[2], right.Lanes[2]),
			fn(left.Lanes[3], right.Lanes[3]),
		}}
	default:
		return scalarValue(target, fn(left.Lanes[0], right.Lanes[0]))
	}
}

func mapTernary(a, b, c value, target ir.Type, fn func(float64, float64, float64) float64) value {
	switch target {
	case ir.TypeVec2:
		return value{Typ: target, Lanes: [4]float64{
			fn(a.Lanes[0], b.Lanes[0], c.Lanes[0]),
			fn(a.Lanes[1], b.Lanes[1], c.Lanes[1]),
		}}
	case ir.TypeVec3:
		return value{Typ: target, Lanes: [4]float64{
			fn(a.Lanes[0], b.Lanes[0], c.Lanes[0]),
			fn(a.Lanes[1], b.Lanes[1], c.Lanes[1]),
			fn(a.Lanes[2], b.Lanes[2], c.Lanes[2]),
		}}
	case ir.TypeVec4:
		return value{Typ: target, Lanes: [4]float64{
			fn(a.Lanes[0], b.Lanes[0], c.Lanes[0]),
			fn(a.Lanes[1], b.Lanes[1], c.Lanes[1]),
			fn(a.Lanes[2], b.Lanes[2], c.Lanes[2]),
			fn(a.Lanes[3], b.Lanes[3], c.Lanes[3]),
		}}
	default:
		return scalarValue(target, fn(a.Lanes[0], b.Lanes[0], c.Lanes[0]))
	}
}
