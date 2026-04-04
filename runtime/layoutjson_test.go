package runtime_test

import (
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/runtime"
)

func TestParseLayoutJSONValid(t *testing.T) {
	layout, err := runtime.ParseLayoutJSON([]byte(`{
		"width": 4,
		"height": 2,
		"points": [
			{"index": 0, "x": 0, "y": 0},
			{"index": 1, "x": 1, "y": 0}
		]
	}`))
	if err != nil {
		t.Fatalf("parse layout json: %v", err)
	}
	if len(layout.Points) != 2 {
		t.Fatalf("point count = %d, want 2", len(layout.Points))
	}
}

func TestParseLayoutJSONMalformed(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{"width": 4, "points": [`))
	if err == nil {
		t.Fatal("expected malformed json error")
	}
}

func TestParseLayoutJSONMissingFields(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{"height": 2, "points": [{"index": 0, "x": 0, "y": 0}]}`))
	if err == nil || !strings.Contains(err.Error(), "layout width must be > 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLayoutJSONDuplicateIndex(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{
		"width": 4,
		"height": 2,
		"points": [
			{"index": 0, "x": 0, "y": 0},
			{"index": 0, "x": 1, "y": 0}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate index") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLayoutJSONOutOfRangeIndex(t *testing.T) {
	_, err := runtime.ParseLayoutJSON([]byte(`{
		"width": 4,
		"height": 2,
		"points": [
			{"index": 2, "x": 0, "y": 0}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "out-of-range index") {
		t.Fatalf("unexpected error: %v", err)
	}
}
