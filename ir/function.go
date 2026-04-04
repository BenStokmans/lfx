package ir

// Function represents a single function in the IR.
type Function struct {
	Name       string
	Params     []FuncParam
	ReturnType Type
	MultiRet   int // 0 or 1 for single return, >1 for multi-return
	Locals     []Local
	Body       []IRStmt
	Exported   bool
	Source     string // originating module path for mangled names
}

// FuncParam describes a function parameter.
type FuncParam struct {
	Name string
	Type Type
}

// Local describes a local variable slot.
type Local struct {
	Name string
	Type Type
}
