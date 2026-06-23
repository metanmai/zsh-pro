// Package util holds generic, dependency-free helpers shared across layers.
package util

import (
	"os"
	"path/filepath"
)

// ExpandHome resolves a leading ~ or ~/ to the user's home directory. On any
// failure it returns the path unchanged.
func ExpandHome(p string) string {
	if p == "~" || (len(p) >= 2 && p[:2] == "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
