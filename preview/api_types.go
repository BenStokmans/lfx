package main

import lfxruntime "github.com/BenStokmans/lfx/runtime"

type BootstrapData struct {
	DefaultWorkspace string         `json:"defaultWorkspace"`
	Workspace        *WorkspaceData `json:"workspace"`
	Layouts          []LayoutData   `json:"layouts"`
}

type WorkspaceData struct {
	Root    string           `json:"root"`
	Effects []EffectFileData `json:"effects"`
}

type EffectFileData struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	ModulePath   string `json:"modulePath,omitempty"`
}

type LayoutData struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	BuiltIn bool               `json:"builtIn"`
	Path    string             `json:"path,omitempty"`
	Width   float32            `json:"width"`
	Height  float32            `json:"height"`
	Points  []lfxruntime.Point `json:"points"`
}

type SaveSourceRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type CompileRequest struct {
	WorkspaceRoot string         `json:"workspaceRoot"`
	FilePath      string         `json:"filePath"`
	Overrides     map[string]any `json:"overrides"`
}

type CompileResponse struct {
	CompilationID string           `json:"compilationId,omitempty"`
	WorkspaceRoot string           `json:"workspaceRoot"`
	FilePath      string           `json:"filePath"`
	ModulePath    string           `json:"modulePath,omitempty"`
	WGSL          string           `json:"wgsl,omitempty"`
	Params        []ParamData      `json:"params"`
	BoundParams   map[string]any   `json:"boundParams,omitempty"`
	Presets       []PresetData     `json:"presets"`
	Diagnostics   []DiagnosticData `json:"diagnostics"`
}

type ParamData struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	DefaultValue any      `json:"defaultValue"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	EnumValues   []string `json:"enumValues,omitempty"`
}

type PresetData struct {
	Name      string  `json:"name"`
	Speed     float64 `json:"speed"`
	Start     float64 `json:"start"`
	LoopStart float64 `json:"loopStart"`
	LoopEnd   float64 `json:"loopEnd"`
	Finish    float64 `json:"finish"`
}

type DiagnosticData struct {
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	FilePath string `json:"filePath,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

type SampleRequest struct {
	CompilationID string         `json:"compilationId"`
	Layout        LayoutData     `json:"layout"`
	Phase         float32        `json:"phase"`
	Overrides     map[string]any `json:"overrides"`
	Limit         int            `json:"limit"`
}

type SampleResponse struct {
	Points []SamplePointData `json:"points"`
}

type SamplePointData struct {
	Index uint32  `json:"index"`
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	Value float32 `json:"value"`
}
