//go:build darwin

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// cacheTraversalPath recognizes the immutable macOS compatibility aliases
// before descriptor-only traversal. /var and /tmp are root-owned symlinks to
// /private on stock macOS, while the cache traversal deliberately rejects all
// symlinks. Rewriting only after the exact link text is authenticated lets a
// legitimate HOME below either alias work without accepting a caller-selected
// link or reopening the resolved cache path by name.
func cacheTraversalPath(path string) (string, error) {
	for _, alias := range []struct {
		path   string
		target string
	}{
		{path: "/var", target: "/private/var"},
		{path: "/tmp", target: "/private/tmp"},
	} {
		if path != alias.path && !strings.HasPrefix(path, alias.path+string(filepath.Separator)) {
			continue
		}
		info, err := os.Lstat(alias.path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return "", errors.New("cached loader path uses an untrusted macOS compatibility link")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return "", errors.New("cached loader path uses an untrusted macOS compatibility link")
		}
		link, err := os.Readlink(alias.path)
		if err != nil || (link != strings.TrimPrefix(alias.target, "/") && link != alias.target) {
			return "", errors.New("cached loader path uses an untrusted macOS compatibility link")
		}
		return filepath.Clean(alias.target + strings.TrimPrefix(path, alias.path)), nil
	}
	return path, nil
}
