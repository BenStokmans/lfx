package runtime

import (
	"fmt"
	"slices"

	"github.com/BenStokmans/lfx/ir"
)

// BoundParams holds validated parameter values for execution.
type BoundParams struct {
	Values map[string]any // string -> float64/int64/bool/string
}

// Bind validates parameter values against specs, applying defaults for
// missing values. It clamps numeric values to [Min, Max] when those
// bounds are set, and rejects invalid enum values.
func Bind(specs []ir.ParamSpec, overrides map[string]any) (*BoundParams, error) {
	bp := &BoundParams{Values: make(map[string]any, len(specs))}

	for _, spec := range specs {
		val, provided := overrides[spec.Name]
		if !provided {
			// Apply default value based on param type.
			switch spec.Type {
			case ir.ParamInt:
				bp.Values[spec.Name] = spec.IntDefault
			case ir.ParamFloat:
				bp.Values[spec.Name] = spec.FloatDefault
			case ir.ParamBool:
				bp.Values[spec.Name] = spec.BoolDefault
			case ir.ParamEnum:
				bp.Values[spec.Name] = spec.EnumDefault
			}
			continue
		}

		// Validate and store the provided value.
		switch spec.Type {
		case ir.ParamInt:
			v, ok := toInt64(val)
			if !ok {
				return nil, fmt.Errorf("param %q: expected int, got %T", spec.Name, val)
			}
			v = clampInt64(v, spec.Min, spec.Max)
			bp.Values[spec.Name] = v

		case ir.ParamFloat:
			v, ok := toFloat64(val)
			if !ok {
				return nil, fmt.Errorf("param %q: expected float, got %T", spec.Name, val)
			}
			v = clampFloat64(v, spec.Min, spec.Max)
			bp.Values[spec.Name] = v

		case ir.ParamBool:
			v, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("param %q: expected bool, got %T", spec.Name, val)
			}
			bp.Values[spec.Name] = v

		case ir.ParamEnum:
			v, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("param %q: expected string, got %T", spec.Name, val)
			}
			if !isValidEnum(v, spec.EnumValues) {
				return nil, fmt.Errorf("param %q: invalid enum value %q, valid values: %v",
					spec.Name, v, spec.EnumValues)
			}
			bp.Values[spec.Name] = v

		default:
			return nil, fmt.Errorf("param %q: unknown type %d", spec.Name, spec.Type)
		}
	}

	return bp, nil
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func clampInt64(v int64, min, max *float64) int64 {
	if min != nil && float64(v) < *min {
		v = int64(*min)
	}
	if max != nil && float64(v) > *max {
		v = int64(*max)
	}
	return v
}

func clampFloat64(v float64, min, max *float64) float64 {
	if min != nil && v < *min {
		v = *min
	}
	if max != nil && v > *max {
		v = *max
	}
	return v
}

func isValidEnum(v string, values []string) bool {
	return slices.Contains(values, v)
}
