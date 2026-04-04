package compiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/sema"
)

// PreviewDiagnostic is a UI-friendly compiler diagnostic.
type PreviewDiagnostic struct {
	Severity   string `json:"severity"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	FilePath   string `json:"filePath,omitempty"`
	ModulePath string `json:"modulePath,omitempty"`
	Line       int    `json:"line,omitempty"`
	Column     int    `json:"column,omitempty"`
}

// PreviewArtifact is the full compile output used by the preview app.
type PreviewArtifact struct {
	FilePath     string               `json:"filePath"`
	BaseDir      string               `json:"baseDir"`
	ModulePath   string               `json:"modulePath"`
	OutputType   string               `json:"outputType"`
	WGSL         string               `json:"wgsl"`
	Params       []ir.ParamSpec       `json:"params"`
	BoundParams  map[string]any       `json:"boundParams"`
	Timeline     *ir.TimelineSpec     `json:"timeline,omitempty"`
	Diagnostics  []PreviewDiagnostic  `json:"diagnostics"`
	Result       *Result              `json:"-"`
	Sampler      runtime.Sampler      `json:"-"`
	BoundRuntime *runtime.BoundParams `json:"-"`
}

// PreviewError exposes compiler errors as structured diagnostics.
type PreviewError struct {
	Diagnostics []PreviewDiagnostic
}

func (e *PreviewError) Error() string {
	lines := make([]string, 0, len(e.Diagnostics))
	for _, item := range e.Diagnostics {
		if item.Code != "" {
			lines = append(lines, fmt.Sprintf("[%s] %s", item.Code, item.Message))
			continue
		}
		lines = append(lines, item.Message)
	}
	return strings.Join(lines, "\n")
}

// CompileForPreview runs the full compiler pipeline and prepares CPU and WGSL outputs.
func CompileForPreview(filePath string, overrides map[string]any, opts Options) (*PreviewArtifact, error) {
	result, err := CompileFile(filePath, opts)
	if err != nil {
		return nil, &PreviewError{Diagnostics: DiagnosticsFromError(filePath, "", err)}
	}

	wgslSource, err := wgsl.Emit(result.IR)
	if err != nil {
		return nil, &PreviewError{Diagnostics: DiagnosticsFromError(filePath, result.Entry.ModPath, err)}
	}

	boundParams, err := runtime.Bind(result.IR.Params, overrides)
	if err != nil {
		return nil, &PreviewError{Diagnostics: DiagnosticsFromError(filePath, result.Entry.ModPath, err)}
	}

	return &PreviewArtifact{
		FilePath:     filePath,
		BaseDir:      result.BaseDir,
		ModulePath:   result.Entry.ModPath,
		OutputType:   outputTypeName(result.IR.Output),
		WGSL:         wgslSource,
		Params:       append([]ir.ParamSpec(nil), result.IR.Params...),
		BoundParams:  cloneMap(boundParams.Values),
		Timeline:     result.IR.Timeline,
		Diagnostics:  DiagnosticsFromWarnings(filePath, result.Entry.ModPath, result.Warnings),
		Result:       result,
		Sampler:      cpu.NewEvaluator(result.IR),
		BoundRuntime: boundParams,
	}, nil
}

func outputTypeName(output ir.OutputType) string {
	switch output {
	case ir.OutputRGB:
		return "rgb"
	case ir.OutputRGBW:
		return "rgbw"
	default:
		return "scalar"
	}
}

// DiagnosticsFromError converts compiler errors into UI-friendly diagnostics.
func DiagnosticsFromError(filePath, modulePath string, err error) []PreviewDiagnostic {
	if err == nil {
		return nil
	}

	var multi *Diagnostics
	if errors.As(err, &multi) {
		diags := make([]PreviewDiagnostic, 0, len(multi.Items))
		for _, item := range multi.Items {
			diags = append(diags, DiagnosticsFromError(filePath, modulePath, item)...)
		}
		return diags
	}

	var parseErr *parser.ParseError
	if errors.As(err, &parseErr) {
		return []PreviewDiagnostic{{
			Severity:   "error",
			Message:    parseErr.Msg,
			FilePath:   filePath,
			ModulePath: modulePath,
			Line:       parseErr.Pos.Line,
			Column:     parseErr.Pos.Col,
		}}
	}

	var lexErr *parser.LexError
	if errors.As(err, &lexErr) {
		return []PreviewDiagnostic{{
			Severity:   "error",
			Message:    lexErr.Msg,
			FilePath:   filePath,
			ModulePath: modulePath,
			Line:       lexErr.Pos.Line,
			Column:     lexErr.Pos.Col,
		}}
	}

	var semaErr *sema.Error
	if errors.As(err, &semaErr) {
		return []PreviewDiagnostic{{
			Severity:   "error",
			Code:       semaErr.Code,
			Message:    semaErr.Msg,
			FilePath:   filePath,
			ModulePath: modulePath,
			Line:       semaErr.Pos.Line,
			Column:     semaErr.Pos.Col,
		}}
	}

	return []PreviewDiagnostic{{
		Severity:   "error",
		Message:    err.Error(),
		FilePath:   filePath,
		ModulePath: modulePath,
	}}
}

func DiagnosticsFromWarnings(filePath, modulePath string, warnings []sema.Warning) []PreviewDiagnostic {
	if len(warnings) == 0 {
		return nil
	}
	diags := make([]PreviewDiagnostic, 0, len(warnings))
	for _, warning := range warnings {
		diags = append(diags, PreviewDiagnostic{
			Severity:   "warning",
			Code:       warning.Code,
			Message:    warning.Msg,
			FilePath:   filePath,
			ModulePath: modulePath,
			Line:       warning.Pos.Line,
			Column:     warning.Pos.Col,
		})
	}
	return diags
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
