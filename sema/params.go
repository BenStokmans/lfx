package sema

import (
	"fmt"

	"github.com/BenStokmans/lfx/parser"
)

// validateParams checks that all parameter definitions are well-formed.
// It verifies:
//   - int/float params: min <= max when both are present
//   - int/float params: default is within [min, max] when bounds are present
//   - enum params: default value is in the values list
func (a *analyzer) validateParams() {
	if a.mod.Params == nil {
		return
	}

	for _, p := range a.mod.Params.Params {
		switch p.Type {
		case parser.ParamInt, parser.ParamFloat:
			a.validateNumericParam(p)
		case parser.ParamEnum:
			a.validateEnumParam(p)
		case parser.ParamBool:
			// No extra validation needed for bool params.
		}
	}
}

func (a *analyzer) validateNumericParam(p *parser.ParamDef) {
	if p.Min != nil && p.Max != nil {
		if *p.Min > *p.Max {
			a.addError(p.Pos, ErrParamMinGreaterThanMax,
				fmt.Sprintf("param %q: min (%v) must be <= max (%v)", p.Name, *p.Min, *p.Max))
		}
	}

	if p.Default == nil {
		return
	}

	var def float64
	switch d := p.Default.(type) {
	case int:
		def = float64(d)
	case float64:
		def = d
	default:
		a.addError(p.Pos, ErrNumericParamDefaultType,
			fmt.Sprintf("param %q: invalid default type for numeric param", p.Name))
		return
	}

	if p.Min != nil && def < *p.Min {
		a.addError(p.Pos, ErrParamDefaultBelowMin,
			fmt.Sprintf("param %q: default (%v) is below min (%v)", p.Name, def, *p.Min))
	}
	if p.Max != nil && def > *p.Max {
		a.addError(p.Pos, ErrParamDefaultAboveMax,
			fmt.Sprintf("param %q: default (%v) is above max (%v)", p.Name, def, *p.Max))
	}
}

func (a *analyzer) validateEnumParam(p *parser.ParamDef) {
	if p.Default == nil {
		return
	}
	defStr, ok := p.Default.(string)
	if !ok {
		a.addError(p.Pos, ErrEnumDefaultNotString,
			fmt.Sprintf("param %q: enum default must be a string", p.Name))
		return
	}
	for _, v := range p.EnumValues {
		if v == defStr {
			return
		}
	}
	a.addError(p.Pos, ErrEnumDefaultNotInValues,
		fmt.Sprintf("param %q: default %q is not in the enum values list", p.Name, defStr))
}
