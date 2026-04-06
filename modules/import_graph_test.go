package modules

import (
	"fmt"
	"strings"
	"testing"
)

// mapResolver is an in-memory Resolver used only in tests.
type mapResolver map[string][]byte

func (m mapResolver) Resolve(path string) ([]byte, error) {
	if src, ok := m[path]; ok {
		return src, nil
	}
	return nil, fmt.Errorf("module %q not found", path)
}

func TestBuildRejectsDirectImportCycle(t *testing.T) {
	// a imports b; b imports a → direct cycle.
	// LFX import syntax: imports come after `module` but before `library`/`effect`.
	srcA := []byte(`module "a"
import "b" as b_lib
library "a"
export function fa(x)
  return x
end
`)
	r := mapResolver{
		"b": []byte(`module "b"
import "a" as a_lib
library "b"
export function fb(x)
  return x
end
`),
		"a": srcA,
	}
	_, err := Build("a", srcA, r)
	if err == nil {
		t.Fatal("expected import cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error should mention cycle, got: %v", err)
	}
}

func TestBuildRejectsIndirectImportCycle(t *testing.T) {
	// a → b → c → a: three-module indirect cycle.
	srcA := []byte(`module "a"
import "b" as b_lib
library "a"
export function fa(x)
  return x
end
`)
	r := mapResolver{
		"a": srcA,
		"b": []byte(`module "b"
import "c" as c_lib
library "b"
export function fb(x)
  return x
end
`),
		"c": []byte(`module "c"
import "a" as a_lib
library "c"
export function fc(x)
  return x
end
`),
	}
	_, err := Build("a", srcA, r)
	if err == nil {
		t.Fatal("expected import cycle error for a→b→c→a")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error should mention cycle, got: %v", err)
	}
}

func TestBuildAcceptsDuplicateAliasSourceButSemaRejects(t *testing.T) {
	// The graph builder itself does not check duplicate aliases; sema does.
	// Verify the graph builds without error so the sema layer catches it.
	srcEffect := []byte(`module "effects/dup_alias"
import "lib_coords" as util
import "lib_curves" as util
effect "dup_alias"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	r := mapResolver{
		"lib_coords": []byte(`module "lib_coords"
library "lib_coords"
export function normalized(x)
  return x
end
`),
		"lib_curves": []byte(`module "lib_curves"
library "lib_curves"
export function ease(x)
  return x
end
`),
	}
	// Graph build should succeed (duplicate aliases are a sema concern).
	_, err := Build("effects/dup_alias", srcEffect, r)
	if err != nil {
		t.Fatalf("unexpected graph build error for duplicate alias (sema handles this): %v", err)
	}
}

func TestBuildRejectsMissingImport(t *testing.T) {
	src := []byte(`module "effects/missing_dep"
import "nonexistent/module" as dep
effect "missing_dep"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	_, err := Build("effects/missing_dep", src, mapResolver{})
	if err == nil {
		t.Fatal("expected error for missing import")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should mention module not found, got: %v", err)
	}
}

func TestBuildRejectsEffectImportingEffect(t *testing.T) {
	srcEntry := []byte(`module "effects/bad_effect"
import "effects/other_effect" as other
effect "bad_effect"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 0.0
end
`)
	r := mapResolver{
		"effects/other_effect": []byte(`module "effects/other_effect"
effect "other_effect"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 1.0
end
`),
	}
	_, err := Build("effects/bad_effect", srcEntry, r)
	if err == nil {
		t.Fatal("expected error for effect importing another effect")
	}
	if !strings.Contains(err.Error(), "cannot import effect") {
		t.Fatalf("error should mention cannot import effect, got: %v", err)
	}
}

func TestBuildAllowsLibraryImportingEffectAtGraphLevel(t *testing.T) {
	// The graph builder only blocks effect→effect; library→effect passes through.
	// Downstream sema will fail because effects export nothing.
	srcLib := []byte(`module "lib/weird"
import "effects/dep_effect" as dep
library "weird"
export function wrap(x)
  return x
end
`)
	r := mapResolver{
		"effects/dep_effect": []byte(`module "effects/dep_effect"
effect "dep_effect"
output scalar
function sample(width, height, x, y, index, phase, params)
  return 1.0
end
`),
	}
	// Graph build succeeds — semantic validation is a separate step.
	_, err := Build("lib/weird", srcLib, r)
	if err != nil {
		t.Fatalf("graph build should allow library→effect; got error: %v", err)
	}
}

func TestBuildAcceptsTwoLibrariesWithSameExportedName(t *testing.T) {
	// Both libs export "helper" but have distinct import aliases, so no conflict.
	srcEffect := []byte(`module "effects/two_libs"
import "lib_a" as la
import "lib_b" as lb
effect "two_libs"
output scalar
function sample(width, height, x, y, index, phase, params)
  return la.helper(x) + lb.helper(y)
end
`)
	r := mapResolver{
		"lib_a": []byte(`module "lib_a"
library "lib_a"
export function helper(x)
  return x
end
`),
		"lib_b": []byte(`module "lib_b"
library "lib_b"
export function helper(x)
  return x * 2.0
end
`),
	}
	g, err := Build("effects/two_libs", srcEffect, r)
	if err != nil {
		t.Fatalf("two distinct-alias libraries with same export name should be accepted: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 graph nodes, got %d", len(g.Nodes))
	}
}

func TestBuildAcceptsAliasShadowingFunctionAtGraphLevel(t *testing.T) {
	// The graph builder doesn't check name collisions (sema does).
	srcEffect := []byte(`module "effects/alias_shadow"
import "mylib" as helper
effect "alias_shadow"
output scalar
function helper(x)
  return x
end
function sample(width, height, x, y, index, phase, params)
  return helper(x)
end
`)
	r := mapResolver{
		"mylib": []byte(`module "mylib"
library "mylib"
export function fn(x)
  return x
end
`),
	}
	_, err := Build("effects/alias_shadow", srcEffect, r)
	if err != nil {
		t.Fatalf("graph build should not check alias/function name collision: %v", err)
	}
}

func TestBuildLargeGraphWithSingleUsedLeaf(t *testing.T) {
	// Build a chain: effect → lib0 → lib1 → ... → lib9
	// Only lib0 is directly used; the chain still resolves.
	const depth = 10
	libSrcs := make(map[string][]byte, depth)
	for i := 0; i < depth; i++ {
		name := fmt.Sprintf("lib%d", i)
		// imports must appear after `module` but before `library`
		src := fmt.Sprintf(`module "%s"
`, name)
		if i+1 < depth {
			next := fmt.Sprintf("lib%d", i+1)
			src += fmt.Sprintf(`import "%s" as next_lib
`, next)
		}
		src += fmt.Sprintf(`library "%s"
export function fn%d(x)
  return x
end
`, name, i)
		libSrcs[name] = []byte(src)
	}

	entryModPath := "effects/deep_import"
	srcEffect := []byte(`module "effects/deep_import"
import "lib0" as mylib
effect "deep_import"
output scalar
function sample(width, height, x, y, index, phase, params)
  return mylib.fn0(phase)
end
`)

	r := mapResolver{}
	for name, src := range libSrcs {
		r[name] = src
	}
	r[entryModPath] = srcEffect

	g, err := Build(entryModPath, srcEffect, r)
	if err != nil {
		t.Fatalf("build deep import graph: %v", err)
	}
	// All 10 libs + the effect should be in the graph.
	if want := depth + 1; len(g.Nodes) != want {
		t.Fatalf("graph node count = %d, want %d", len(g.Nodes), want)
	}
}
