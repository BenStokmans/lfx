package lfx_test

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BenStokmans/lfx/backend/cpu"
	"github.com/BenStokmans/lfx/backend/wgsl"
	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/lower"
	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/parser"
	"github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/sema"
	"github.com/BenStokmans/lfx/stdlib"
)

func FuzzParser(f *testing.F) {
	// Seed with a mix of valid programs and known-invalid fragments.
	seeds := []string{
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
`,
		`module "effects/t" effect "t"`,
		`module "effects/bad
effect "Bad"`,
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  if phase < 0.5 then
    return 0.0
  return 1.0
end
`,
		``,
		`end end end`,
		`function sample(width, height, x, y, index, phase, params)`,
		`output rgb
return x, y`,
		`timeline { loop_start = -1.0 }`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		// The parser must never panic, regardless of input.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parser panicked on input: %v", r)
			}
		}()
		parser.Parse(source) //nolint:errcheck,gosec
	})
}

func FuzzModuleGraph(f *testing.F) {
	// Seed with simple valid module sources.
	f.Add(`module "a"
library "a"
export function fa(x)
  return x
end
`, `module "b"
library "b"
import "a" as la
export function fb(x)
  return la.fa(x)
end
`)

	f.Fuzz(func(t *testing.T, entrySource, depSource string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("module graph build panicked: %v", r)
			}
		}()
		r := fuzzMapResolver{
			"dep": []byte(depSource),
		}
		modules.Build("entry", []byte(entrySource), r) //nolint:errcheck,gosec
	})
}

func FuzzScalarExpressions(f *testing.F) {
	seed := `module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  return %s
end
`
	exprs := []string{
		"phase",
		"x + y",
		"phase * phase",
		"abs(x - y)",
		"clamp(phase, 0.0, 1.0)",
		"sin(phase) + cos(phase)",
		"sqrt(x * x + y * y)",
		"floor(phase * 10.0) / 10.0",
	}
	for _, expr := range exprs {
		f.Add(fmt.Sprintf(seed, expr))
	}

	f.Fuzz(func(t *testing.T, source string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("scalar expression fuzz panicked: %v", r)
			}
		}()
		mod, err := parser.Parse(source)
		if err != nil {
			return
		}
		errs := sema.Analyze(mod, nil)
		if len(errs) != 0 {
			return
		}
		info, sErrs, _ := sema.AnalyzeModule(mod, nil, nil)
		if len(sErrs) != 0 {
			return
		}
		irmod, err := lower.Lower(mod, nil, info, nil)
		if err != nil {
			return
		}
		lower.ConstFold(irmod)
		// CPU evaluation must not panic.
		ev := cpu.NewEvaluator(irmod)
		layout := runtime.Layout{
			Width: 4, Height: 4,
			Points: []runtime.Point{{Index: 0, X: 1, Y: 1}},
		}
		params, _ := runtime.Bind(irmod.Params, nil)
		ev.SamplePoint(layout, 0, 0.5, params) //nolint:errcheck,gosec
	})
}

func FuzzVectorExpressions(f *testing.F) {
	// Seed with a mix of valid and invalid vector expressions.
	seeds := []string{
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  v = vec2(x, y)
  return length(v)
end
`,
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  a = vec3(x, y, phase)
  b = vec3(y, phase, x)
  return dot(a, b)
end
`,
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  v = vec2(x, y) + vec3(x, y, phase)
  return v.x
end
`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("vector expression fuzz panicked: %v", r)
			}
		}()
		mod, err := parser.Parse(source)
		if err != nil {
			return
		}
		info, errs, _ := sema.AnalyzeModule(mod, nil, nil)
		if len(errs) != 0 {
			return
		}
		irmod, err := lower.Lower(mod, nil, info, nil)
		if err != nil {
			return
		}
		lower.ConstFold(irmod)
		ev := cpu.NewEvaluator(irmod)
		layout := runtime.Layout{
			Width: 4, Height: 4,
			Points: []runtime.Point{{Index: 0, X: 1, Y: 1}},
		}
		params, _ := runtime.Bind(irmod.Params, nil)
		ev.SamplePoint(layout, 0, 0.5, params) //nolint:errcheck,gosec
	})
}

func FuzzRuntimeBind(f *testing.F) {
	f.Add("gain", "float", "0.5")
	f.Add("count", "int", "4")
	f.Add("active", "bool", "true")
	f.Add("mode", "enum", "linear")

	f.Fuzz(func(t *testing.T, name, typeName, valueStr string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("runtime.Bind panicked: %v", r)
			}
		}()

		var specs []ir.ParamSpec
		switch typeName {
		case "float":
			min, max := 0.0, 1.0
			specs = []ir.ParamSpec{{Name: name, Type: ir.ParamFloat, FloatDefault: 0.5, Min: &min, Max: &max}}
		case "int":
			min, max := 0.0, 100.0
			specs = []ir.ParamSpec{{Name: name, Type: ir.ParamInt, IntDefault: 0, Min: &min, Max: &max}}
		case "bool":
			specs = []ir.ParamSpec{{Name: name, Type: ir.ParamBool, BoolDefault: false}}
		default:
			specs = []ir.ParamSpec{{Name: name, Type: ir.ParamEnum, EnumDefault: "a", EnumValues: []string{"a", "b"}}}
		}

		// Provide the fuzz string as the override value — Bind should reject
		// type mismatches without panicking.
		//nolint:errcheck,gosec
		runtime.Bind(specs, map[string]any{name: valueStr})
	})
}

func FuzzWGSLEmitter(f *testing.F) {
	// Seed with known-valid effects.
	validEffects := []string{
		`module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
  return phase
end
`,
		`module "effects/t"
effect "t"
output rgb
function sample(width, height, x, y, index, phase, params)
  return x, y, phase
end
`,
		`module "effects/t"
effect "t"
output scalar
function helper(x)
  return x * 2.0
end
function sample(width, height, x, y, index, phase, params)
  return helper(phase)
end
`,
	}
	for _, src := range validEffects {
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, source string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("wgsl emitter panicked: %v", r)
			}
		}()
		mod, err := parser.Parse(source)
		if err != nil {
			return
		}
		info, errs, _ := sema.AnalyzeModule(mod, nil, nil)
		if len(errs) != 0 {
			return
		}
		irmod, err := lower.Lower(mod, nil, info, nil)
		if err != nil {
			return
		}
		lower.ConstFold(irmod)

		//nolint:errcheck,gosec
		wgsl.Emit(irmod)
	})
}

func FuzzCPUDeterminism(f *testing.F) {
	f.Add(uint64(0), float32(0.0), float32(0.5), float32(0.0), float32(0.0))
	f.Add(uint64(42), float32(3.14), float32(0.7), float32(1.0), float32(2.0))
	f.Add(uint64(999), float32(0.0), float32(1.0), float32(7.0), float32(7.0))

	// Build the evaluator once.
	root, err := filepath.Abs(".")
	if err != nil {
		f.Fatalf("abs: %v", err)
	}
	result, err := compiler.CompileFile(
		filepath.Join(root, "effects", "fill_iris.lfx"),
		compiler.Options{
			BaseDir:  root,
			Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(root)...)),
		},
	)
	if err != nil {
		f.Fatalf("compile fill_iris: %v", err)
	}
	params, _ := runtime.Bind(result.IR.Params, nil)
	ev := cpu.NewEvaluator(result.IR)

	f.Fuzz(func(t *testing.T, _ uint64, phase, x, y float32, wh float32) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("cpu eval panicked: %v", r)
			}
		}()

		if !isFiniteF32(phase) || !isFiniteF32(x) || !isFiniteF32(y) || !isFiniteF32(wh) || wh <= 0 {
			t.Skip()
		}

		layout := runtime.Layout{
			Width:  wh,
			Height: wh,
			Points: []runtime.Point{{Index: 0, X: x, Y: y}},
		}

		v1, err1 := ev.SamplePoint(layout, 0, phase, params)
		v2, err2 := ev.SamplePoint(layout, 0, phase, params)

		if (err1 != nil) != (err2 != nil) {
			t.Fatalf("non-deterministic error: %v vs %v", err1, err2)
		}
		if err1 != nil {
			return
		}
		for i := range v1 {
			if math.Abs(float64(v1[i]-v2[i])) > 1e-6 {
				t.Fatalf("channel %d non-deterministic: %f vs %f", i, v1[i], v2[i])
			}
		}
	})
}

// ── Property test: random corpus — parser never panics ───────────────────────

func TestParserNeverPanicsOnRandomInput(t *testing.T) {
	//nolint:gosec // deterministic RNG for fuzz corpus
	r := rand.New(rand.NewSource(12345))
	const iterations = 500
	for i := range iterations {
		size := r.Intn(200)
		buf := make([]byte, size)
		for j := range buf {
			buf[j] = byte(r.Intn(128)) //nolint:gosec // printable-ish ASCII
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("iteration %d: parser panicked on random input: %v", i, rec)
				}
			}()
			parser.Parse(string(buf)) //nolint:errcheck,gosec
		}()
	}
}

// ── Property test: sema never panics on structurally valid parsed modules ─────

func TestSemaAnalyzeNeverPanicsOnValidParseResult(t *testing.T) {
	//nolint:gosec // deterministic RNG for fuzz corpus
	r := rand.New(rand.NewSource(99999))
	const iterations = 200

	// Generate random effect bodies.
	stmts := []string{
		"  return phase\n",
		"  return x + y\n",
		"  a = x * y\n  return a\n",
		"  if phase < 0.5 then\n    return 0.0\n  end\n  return 1.0\n",
		"  return vec2(x, y).x\n",
	}

	for i := range iterations {
		body := stmts[r.Intn(len(stmts))]
		src := `module "effects/t"
effect "t"
output scalar
function sample(width, height, x, y, index, phase, params)
` + body + `
end
`
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("iteration %d: sema.Analyze panicked: %v", i, rec)
				}
			}()
			mod, err := parser.Parse(src)
			if err != nil {
				return
			}
			sema.Analyze(mod, nil)
		}()
	}
}

// ── Property test: random layouts never panic the CPU evaluator ──────────────

func TestCPUEvaluatorNeverPanicsOnRandomLayouts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "e.lfx")
	//nolint:gosec
	if err := os.WriteFile(path, []byte(`module "effects/e"
effect "e"
output scalar
function sample(width, height, x, y, index, phase, params)
  return clamp(phase + x / (width + 1.0), 0.0, 1.0)
end
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	result, err := compiler.CompileFile(path, compiler.Options{BaseDir: root})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	params, _ := runtime.Bind(result.IR.Params, nil)
	ev := cpu.NewEvaluator(result.IR)

	//nolint:gosec // deterministic RNG for fuzz corpus
	r := rand.New(rand.NewSource(777))
	const iterations = 200
	for i := range iterations {
		w := float32(r.Float64()*100 + 1)
		h := float32(r.Float64()*100 + 1)
		n := r.Intn(10) + 1
		points := make([]runtime.Point, n)
		for j := range points {
			points[j] = runtime.Point{
				Index: uint32(j),
				X:     float32(r.Float64() * float64(w)),
				Y:     float32(r.Float64() * float64(h)),
			}
		}
		layout := runtime.Layout{Width: w, Height: h, Points: points}
		phase := float32(r.Float64())

		pointIdx := r.Intn(n)
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("iteration %d: cpu evaluator panicked: %v", i, rec)
				}
			}()
			ev.SamplePoint(layout, pointIdx, phase, params) //nolint:errcheck,gosec
		}()
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

type fuzzMapResolver map[string][]byte

func (m fuzzMapResolver) Resolve(path string) ([]byte, error) {
	if src, ok := m[path]; ok {
		return src, nil
	}
	return nil, fmt.Errorf("module %q not found", path)
}

func isFiniteF32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

// Guard against t.Skip() in fuzz functions unused import.
var _ = strings.Contains
