package ir

// Module is the flattened compilation unit consumed by backends.
type Module struct {
	Name       string
	SourcePath string
	Params     []ParamSpec
	Functions  []*Function
	Sample     *Function // pointer to the sample function
	Presets    []PresetSpec
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

// PresetSpec describes a named preset configuration.
type PresetSpec struct {
	Name      string
	Speed     float64
	Start     float64
	LoopStart float64
	LoopEnd   float64
	Finish    float64
}
