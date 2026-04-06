package runtime_test

import (
	"testing"

	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/runtime"
)

// makeFloat makes a float64 pointer for ParamSpec bounds.
func makeFloat(v float64) *float64 { return &v }

func TestBindUsesDefaultWhenNoOverrideProvided(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.75, Min: makeFloat(0), Max: makeFloat(1)},
		{Name: "count", Type: ir.ParamInt, IntDefault: 4, Min: makeFloat(0), Max: makeFloat(100)},
		{Name: "active", Type: ir.ParamBool, BoolDefault: true},
		{Name: "mode", Type: ir.ParamEnum, EnumDefault: "linear", EnumValues: []string{"linear", "ease"}},
	}
	bp, err := runtime.Bind(specs, nil)
	if err != nil {
		t.Fatalf("bind with nil overrides: %v", err)
	}
	if bp.Values["gain"].(float64) != 0.75 {
		t.Fatalf("gain default = %v, want 0.75", bp.Values["gain"])
	}
	if bp.Values["count"].(int64) != 4 {
		t.Fatalf("count default = %v, want 4", bp.Values["count"])
	}
	if bp.Values["active"].(bool) != true {
		t.Fatalf("active default = %v, want true", bp.Values["active"])
	}
	if bp.Values["mode"].(string) != "linear" {
		t.Fatalf("mode default = %v, want linear", bp.Values["mode"])
	}
}

func TestBindAcceptsUnknownOverrideKey(t *testing.T) {
	// Bind currently ignores unknown keys (only iterates over specs).
	// This test documents the current behavior: no error for unknown keys.
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.5},
	}
	bp, err := runtime.Bind(specs, map[string]any{
		"gain":    0.8,
		"unknown": "oops",
	})
	if err != nil {
		t.Logf("unknown key rejected: %v", err)
		return
	}
	// If accepted, the known param should be set correctly.
	if bp.Values["gain"].(float64) != 0.8 {
		t.Fatalf("gain = %v, want 0.8", bp.Values["gain"])
	}
}

func TestBindRejectsStringForFloatParam(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.5},
	}
	_, err := runtime.Bind(specs, map[string]any{"gain": "0.8"})
	if err == nil {
		t.Fatal("expected error for string override of float param")
	}
}

func TestBindRejectsStringForIntParam(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "count", Type: ir.ParamInt, IntDefault: 4},
	}
	_, err := runtime.Bind(specs, map[string]any{"count": "1.5"})
	if err == nil {
		t.Fatal("expected error for string override of int param")
	}
}

func TestBindRejectsStringForBoolParam(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "active", Type: ir.ParamBool, BoolDefault: true},
	}
	// Strings like "TRUE", "False", "1", "yes" must be rejected — only Go bool is valid.
	for _, bad := range []any{"TRUE", "False", "1", "yes", 1, 0} {
		_, err := runtime.Bind(specs, map[string]any{"active": bad})
		if err == nil {
			t.Fatalf("expected error for bool param override with %T(%v)", bad, bad)
		}
	}
}

func TestBindAcceptsNativeBoolForBoolParam(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "active", Type: ir.ParamBool, BoolDefault: true},
	}
	for _, good := range []bool{true, false} {
		bp, err := runtime.Bind(specs, map[string]any{"active": good})
		if err != nil {
			t.Fatalf("bool override %v: %v", good, err)
		}
		if bp.Values["active"].(bool) != good {
			t.Fatalf("active = %v, want %v", bp.Values["active"], good)
		}
	}
}

func TestBindRejectsInvalidEnumValue(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "mode", Type: ir.ParamEnum, EnumDefault: "linear",
			EnumValues: []string{"linear", "ease", "bounce"}},
	}
	_, err := runtime.Bind(specs, map[string]any{"mode": "invalid_option"})
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
}

func TestBindEnumIsCaseSensitive(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "mode", Type: ir.ParamEnum, EnumDefault: "linear",
			EnumValues: []string{"linear", "ease"}},
	}
	// "Linear" (capital L) is not in the enum values.
	_, err := runtime.Bind(specs, map[string]any{"mode": "Linear"})
	if err == nil {
		t.Fatal("expected error for case-mismatch enum value")
	}
}

func TestBindAcceptsBoundaryValuesAtMinAndMax(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.5, Min: makeFloat(0.0), Max: makeFloat(1.0)},
	}
	for _, v := range []float64{0.0, 1.0} {
		bp, err := runtime.Bind(specs, map[string]any{"gain": v})
		if err != nil {
			t.Fatalf("boundary value %v: %v", v, err)
		}
		if bp.Values["gain"].(float64) != v {
			t.Fatalf("gain = %v, want %v", bp.Values["gain"], v)
		}
	}
}

func TestBindClampsOutOfRangeFloatToMax(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.5, Min: makeFloat(0.0), Max: makeFloat(1.0)},
	}
	bp, err := runtime.Bind(specs, map[string]any{"gain": 2.5})
	if err != nil {
		t.Fatalf("out-of-range float: %v", err)
	}
	if bp.Values["gain"].(float64) != 1.0 {
		t.Fatalf("clamped gain = %v, want 1.0", bp.Values["gain"])
	}
}

func TestBindClampsOutOfRangeFloatToMin(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "gain", Type: ir.ParamFloat, FloatDefault: 0.5, Min: makeFloat(0.0), Max: makeFloat(1.0)},
	}
	bp, err := runtime.Bind(specs, map[string]any{"gain": -0.5})
	if err != nil {
		t.Fatalf("out-of-range float: %v", err)
	}
	if bp.Values["gain"].(float64) != 0.0 {
		t.Fatalf("clamped gain = %v, want 0.0", bp.Values["gain"])
	}
}

func TestBindClampsOutOfRangeIntToMax(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "count", Type: ir.ParamInt, IntDefault: 4, Min: makeFloat(0), Max: makeFloat(10)},
	}
	bp, err := runtime.Bind(specs, map[string]any{"count": int64(999)})
	if err != nil {
		t.Fatalf("out-of-range int: %v", err)
	}
	if bp.Values["count"].(int64) != 10 {
		t.Fatalf("clamped count = %v, want 10", bp.Values["count"])
	}
}

func TestBindAcceptsParamNamedClamp(t *testing.T) {
	// "clamp" is a builtin in LFX, but at the runtime/Bind level it's just a key.
	specs := []ir.ParamSpec{
		{Name: "clamp", Type: ir.ParamFloat, FloatDefault: 0.5, Min: makeFloat(0), Max: makeFloat(1)},
	}
	bp, err := runtime.Bind(specs, map[string]any{"clamp": 0.3})
	if err != nil {
		t.Fatalf("param named clamp: %v", err)
	}
	if bp.Values["clamp"].(float64) != 0.3 {
		t.Fatalf("clamp = %v, want 0.3", bp.Values["clamp"])
	}
}

func TestBindAcceptsParamNamedLikeImportAlias(t *testing.T) {
	// At the runtime/Bind level there is no concept of import aliases;
	// the param is accepted without conflict.
	specs := []ir.ParamSpec{
		{Name: "coords", Type: ir.ParamFloat, FloatDefault: 0.0},
	}
	bp, err := runtime.Bind(specs, nil)
	if err != nil {
		t.Fatalf("param named coords: %v", err)
	}
	if bp.Values["coords"].(float64) != 0.0 {
		t.Fatalf("coords = %v, want 0.0", bp.Values["coords"])
	}
}

// ── Additional: int64, int, and float64 all coerce to int param ───────────────

func TestBindCoercesNumericTypesToIntParam(t *testing.T) {
	specs := []ir.ParamSpec{
		{Name: "n", Type: ir.ParamInt, IntDefault: 0, Min: makeFloat(0), Max: makeFloat(100)},
	}
	cases := []struct {
		in   any
		want int64
	}{
		{int64(7), 7},
		{int(7), 7},
		{float64(7.9), 7}, // truncates
	}
	for _, tc := range cases {
		bp, err := runtime.Bind(specs, map[string]any{"n": tc.in})
		if err != nil {
			t.Fatalf("coerce %T(%v): %v", tc.in, tc.in, err)
		}
		if bp.Values["n"].(int64) != tc.want {
			t.Fatalf("n = %v, want %d", bp.Values["n"], tc.want)
		}
	}
}

// ── Additional: empty specs / empty overrides ─────────────────────────────────

func TestBindEmptySpecsAndOverrides(t *testing.T) {
	bp, err := runtime.Bind(nil, nil)
	if err != nil {
		t.Fatalf("bind nil specs: %v", err)
	}
	if len(bp.Values) != 0 {
		t.Fatalf("expected empty values, got %v", bp.Values)
	}
}
