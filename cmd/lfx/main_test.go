package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/BenStokmans/lfx/compiler"
)

func TestRunCheckJSONValidFile(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"check", "--json", filepath.Join("..", "..", "effects", "fill_iris.lfx")}); err != nil {
			t.Fatalf("run check --json: %v", err)
		}
	})

	var payload struct {
		FilePath    string                       `json:"filePath"`
		ModulePath  string                       `json:"modulePath"`
		OK          bool                         `json:"ok"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if !payload.OK {
		t.Fatalf("ok = false, want true: %+v", payload)
	}
	if payload.ModulePath != "effects/fill_iris" {
		t.Fatalf("modulePath = %q, want effects/fill_iris", payload.ModulePath)
	}
	if len(payload.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want empty", payload.Diagnostics)
	}
}

func TestRunCheckJSONParseError(t *testing.T) {
	root, filePath := writeTempEffect(t, "missing_then", `module "effects/missing_then"
effect "Missing Then"
output scalar
function sample(width, height, x, y, index, phase, params)
  if phase < 0.5
    return 0.0
  end
  return 1.0
end
`)

	output := captureStdout(t, func() {
		if err := run([]string{"check", "--json", "--root", root, filePath}); err != nil {
			t.Fatalf("run check --json: %v", err)
		}
	})

	var payload struct {
		OK          bool                         `json:"ok"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if len(payload.Diagnostics) != 1 {
		t.Fatalf("diagnostics len = %d, want 1", len(payload.Diagnostics))
	}
	diag := payload.Diagnostics[0]
	if diag.Severity != "error" {
		t.Fatalf("severity = %q, want error", diag.Severity)
	}
	if diag.Code != "" {
		t.Fatalf("code = %q, want empty", diag.Code)
	}
	if diag.Line != 6 || diag.Column != 5 {
		t.Fatalf("position = %d:%d, want 6:5", diag.Line, diag.Column)
	}
}

func TestRunCheckJSONSemanticError(t *testing.T) {
	root, filePath := writeTempEffect(t, "missing_output", `module "effects/missing_output"
effect "Missing Output"
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)

	output := captureStdout(t, func() {
		if err := run([]string{"check", "--json", "--root", root, filePath}); err != nil {
			t.Fatalf("run check --json: %v", err)
		}
	})

	var payload struct {
		OK          bool                         `json:"ok"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if len(payload.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
	diag := payload.Diagnostics[0]
	if diag.Code != "E006" {
		t.Fatalf("code = %q, want E006", diag.Code)
	}
	if diag.Severity != "error" {
		t.Fatalf("severity = %q, want error", diag.Severity)
	}
}

func TestRunCheckJSONMultiError(t *testing.T) {
	root, filePath := writeTempEffect(t, "multi_error", `module "effects/multi_error"
effect "Multi Error"
function sample(width, height, x, y, index, phase)
  return 0.0
end
`)

	output := captureStdout(t, func() {
		if err := run([]string{"check", "--json", "--root", root, filePath}); err != nil {
			t.Fatalf("run check --json: %v", err)
		}
	})

	var payload struct {
		OK          bool                         `json:"ok"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if len(payload.Diagnostics) < 2 {
		t.Fatalf("diagnostics len = %d, want at least 2", len(payload.Diagnostics))
	}
}

func TestRunPreviewJSONValidFile(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"preview", "--json", filepath.Join("..", "..", "effects", "fill_iris.lfx")}); err != nil {
			t.Fatalf("run preview --json: %v", err)
		}
	})

	var payload struct {
		FilePath    string                       `json:"filePath"`
		ModulePath  string                       `json:"modulePath"`
		OK          bool                         `json:"ok"`
		OutputType  string                       `json:"outputType"`
		WGSL        string                       `json:"wgsl"`
		Params      []map[string]any             `json:"params"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if !payload.OK {
		t.Fatalf("ok = false, want true: %+v", payload)
	}
	if payload.ModulePath != "effects/fill_iris" {
		t.Fatalf("modulePath = %q, want effects/fill_iris", payload.ModulePath)
	}
	if payload.OutputType != "scalar" {
		t.Fatalf("outputType = %q, want scalar", payload.OutputType)
	}
	if payload.WGSL == "" {
		t.Fatal("expected wgsl output")
	}
	if len(payload.Params) == 0 {
		t.Fatal("expected params")
	}
	if len(payload.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want empty", payload.Diagnostics)
	}
}

func TestRunPreviewJSONCompileError(t *testing.T) {
	root, filePath := writeTempEffect(t, "missing_output_preview", `module "effects/missing_output_preview"
effect "Missing Output Preview"
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)

	output := captureStdout(t, func() {
		if err := run([]string{"preview", "--json", "--root", root, filePath}); err != nil {
			t.Fatalf("run preview --json: %v", err)
		}
	})

	var payload struct {
		OK          bool                         `json:"ok"`
		WGSL        string                       `json:"wgsl"`
		Diagnostics []compiler.PreviewDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, output)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.WGSL != "" {
		t.Fatalf("wgsl = %q, want empty", payload.WGSL)
	}
	if len(payload.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
}

func writeTempEffect(t *testing.T, name, source string) (string, string) {
	t.Helper()

	root := t.TempDir()
	//nolint:gosec
	if err := os.MkdirAll(filepath.Join(root, "effects"), 0o755); err != nil {
		t.Fatalf("create effects dir: %v", err)
	}

	filePath := filepath.Join(root, "effects", name+".lfx")
	//nolint:gosec
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write effect file: %v", err)
	}

	return root, filePath
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = writer

	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}
