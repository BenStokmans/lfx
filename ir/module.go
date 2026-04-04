package ir

// Module is the flattened compilation unit consumed by backends.
type Module struct {
	Name       string
	SourcePath string
	Params     []ParamSpec
	Functions  []*Function
	Sample     *Function     // pointer to the sample function
	Timeline   *TimelineSpec // optional loop markers; nil when no timeline block was declared
}

// ParamType classifies the kind of an effect parameter.
type ParamType int

const (
	ParamInt   ParamType = iota
	ParamFloat
	ParamBool
	ParamEnum
)

// ParamSpec describes a declared effect parameter.
type ParamSpec struct {
	Name         string
	Type         ParamType
	IntDefault   int64
	FloatDefault float64
	BoolDefault  bool
	EnumDefault  string
	EnumValues   []string
	Min          *float64
	Max          *float64
}

// TimelineSpec carries the optional loop markers declared in a timeline block.
// All phase values are normalized to [0, 1]. Pointer fields are nil when
// the corresponding marker was not specified by the effect author.
type TimelineSpec struct {
	LoopStart *float64 // sustain loop start; nil when not specified
	LoopEnd   *float64 // sustain loop end; nil when not specified
}
