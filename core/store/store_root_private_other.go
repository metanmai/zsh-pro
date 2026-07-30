//go:build !linux && !darwin

package store

import (
	"fmt"
	"os"
)

// ensurePrivateStoreDir keeps Store.Init's final-root invariant on platforms
// without the descriptor-relative no-follow primitives used by runtime
// capture. Runtime capture itself fails closed on those platforms.
func ensurePrivateStoreDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create profile store root: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect profile store root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("profile store root must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("profile store root must be a directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("make profile store root private: %w", err)
	}
	return nil
}
