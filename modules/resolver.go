package modules

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Resolver finds module source by path.
type Resolver interface {
	Resolve(path string) ([]byte, error)
}

// FileResolver resolves modules from filesystem roots.
type FileResolver struct {
	Roots []string // directories to search, e.g. ["stdlib/", "effects/"]
}

// NewFileResolver creates a FileResolver with the given root directories.
func NewFileResolver(roots ...string) *FileResolver {
	return &FileResolver{Roots: roots}
}

// Resolve searches each root for path.lfx.
// For example, "std/math" searches each root for "std/math.lfx".
func (r *FileResolver) Resolve(path string) ([]byte, error) {
	normalized, err := normalizeModulePath(path)
	if err != nil {
		return nil, err
	}
	for _, root := range r.Roots {
		for _, full := range candidatePaths(root, normalized) {
			data, err := os.ReadFile(full)
			if err == nil {
				return data, nil
			}
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading %s: %w", full, err)
			}
		}
	}
	return nil, fmt.Errorf("module %q not found in any root", normalized)
}

func candidatePaths(root, modPath string) []string {
	parts := strings.Split(modPath, "/")
	candidates := []string{
		filepath.Join(root, modPath+".lfx"),
	}

	if len(parts) > 1 {
		trimmed := filepath.Join(parts[1:]...)
		candidates = append(candidates,
			filepath.Join(root, trimmed+".lfx"),
			filepath.Join(root, trimmed, parts[len(parts)-1]+".lfx"),
		)
	}

	last := parts[len(parts)-1]
	candidates = append(candidates, filepath.Join(root, last+".lfx"))

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func normalizeModulePath(modPath string) (string, error) {
	if modPath == "" {
		return "", fmt.Errorf("module path is required")
	}
	if strings.Contains(modPath, "\\") {
		return "", fmt.Errorf("invalid module path %q", modPath)
	}

	cleaned := pathpkg.Clean(modPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || pathpkg.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid module path %q", modPath)
	}
	if cleaned != modPath {
		return "", fmt.Errorf("module path %q must be normalized", modPath)
	}

	return cleaned, nil
}
