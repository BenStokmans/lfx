package stdlib

import (
	"embed"
	"fmt"
	"slices"

	"github.com/BenStokmans/lfx/modules"
)

const Version = "0.2"

//go:embed math/*.lfx noise/*.lfx wave/*.lfx palette/*.lfx ease/*.lfx geo/*.lfx warp/*.lfx patterns/*.lfx
var files embed.FS

var manifest = map[string]string{
	"std/math":     "math/math.lfx",
	"std/noise":    "noise/noise.lfx",
	"std/wave":     "wave/wave.lfx",
	"std/palette":  "palette/palette.lfx",
	"std/ease":     "ease/ease.lfx",
	"std/geo":      "geo/geo.lfx",
	"std/warp":     "warp/warp.lfx",
	"std/patterns": "patterns/patterns.lfx",
}

// Paths returns the embedded stdlib module paths.
func Paths() []string {
	paths := make([]string, 0, len(manifest))
	for path := range manifest {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Source returns embedded stdlib source by canonical module path.
func Source(path string) ([]byte, error) {
	file, ok := manifest[path]
	if !ok {
		return nil, fmt.Errorf("unknown stdlib module %q", path)
	}
	return files.ReadFile(file)
}

// Resolver resolves embedded stdlib modules first, then falls back to the
// provided filesystem resolver.
type Resolver struct {
	Fallback modules.Resolver
}

func (r Resolver) Resolve(path string) ([]byte, error) {
	if source, err := Source(path); err == nil {
		return source, nil
	}
	if r.Fallback != nil {
		return r.Fallback.Resolve(path)
	}
	return nil, fmt.Errorf("module %q not found", path)
}

// NewResolver wraps a fallback resolver with the embedded stdlib registry.
func NewResolver(fallback modules.Resolver) modules.Resolver {
	return Resolver{Fallback: fallback}
}
