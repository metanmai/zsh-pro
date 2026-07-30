//go:build linux || darwin

package store

import (
	"fmt"
	"os"
	"syscall"
)

// ensurePrivateStoreDir creates the profile-store root with the mode required
// by descriptor-bound runtime capture. Existing roots are repaired only after
// a no-follow descriptor verifies that the final object is a directory owned
// by this effective user. In particular, never chmod a symlink, regular file,
// or another user's store into something that looks trustworthy.
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

	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open profile store root without following links: %w", err)
	}
	defer func() { _ = syscall.Close(fd) }()

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat profile store root: %w", err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return fmt.Errorf("profile store root is not a directory")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("profile store root is not owned by the current user")
	}
	if stat.Mode&0o077 == 0 {
		return nil
	}
	if err := syscall.Fchmod(fd, 0o700); err != nil {
		return fmt.Errorf("make profile store root private: %w", err)
	}
	return nil
}
