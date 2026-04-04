package ir

import "fmt"

// Type represents a primitive type in the IR type system.
type Type int

const (
	TypeUnknown Type = -1
	TypeF32     Type = iota
	TypeI32
	TypeBool
	TypeString // only for param enums, not general use
	TypeVec2
	TypeVec3
	TypeVec4
	TypeVoid
)

func (t Type) String() string {
	switch t {
	case TypeUnknown:
		return "unknown"
	case TypeF32:
		return "f32"
	case TypeI32:
		return "i32"
	case TypeBool:
		return "bool"
	case TypeString:
		return "string"
	case TypeVec2:
		return "vec2"
	case TypeVec3:
		return "vec3"
	case TypeVec4:
		return "vec4"
	case TypeVoid:
		return "void"
	default:
		return fmt.Sprintf("Type(%d)", int(t))
	}
}

func (t Type) IsVector() bool {
	switch t {
	case TypeVec2, TypeVec3, TypeVec4:
		return true
	default:
		return false
	}
}

func (t Type) IsNumeric() bool {
	switch t {
	case TypeF32, TypeI32, TypeVec2, TypeVec3, TypeVec4:
		return true
	default:
		return false
	}
}

func (t Type) Lanes() int {
	switch t {
	case TypeVec2:
		return 2
	case TypeVec3:
		return 3
	case TypeVec4:
		return 4
	default:
		return 1
	}
}

func VectorTypeForLanes(lanes int) Type {
	switch lanes {
	case 2:
		return TypeVec2
	case 3:
		return TypeVec3
	case 4:
		return TypeVec4
	default:
		return TypeVoid
	}
}
