package ir

import "fmt"

// Type represents a primitive type in the IR type system.
type Type int

const (
	TypeF32    Type = iota
	TypeI32
	TypeBool
	TypeString // only for param enums, not general use
	TypeVoid
)

func (t Type) String() string {
	switch t {
	case TypeF32:
		return "f32"
	case TypeI32:
		return "i32"
	case TypeBool:
		return "bool"
	case TypeString:
		return "string"
	case TypeVoid:
		return "void"
	default:
		return fmt.Sprintf("Type(%d)", int(t))
	}
}
