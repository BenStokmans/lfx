package modules

import "path/filepath"

// DefaultRoots returns standard module root directories.
func DefaultRoots(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "stdlib"),
		filepath.Join(baseDir, "effects"),
		baseDir,
	}
}
