//go:build linux || darwin

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type storeRootTransactionGuard struct {
	valid bool
}

func storeTransactionNamespacePath(root string) (string, error) {
	canonical, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	name := ".zsh-pro-transactions-" + hex.EncodeToString(digest[:8])
	return filepath.Join(filepath.Dir(canonical), name), nil
}

func withStoreRootTransactionLock(root string, fn func(*storeRootTransactionGuard) error) error {
	_, _ = root, fn
	return ErrStoreTransactionLockUnavailable
}

func (g *storeRootTransactionGuard) discardForTest() {
	g.valid = false
}

func (g *storeRootTransactionGuard) withAuthenticatedMutation(fn func(*os.Root) error) error {
	_, _ = g, fn
	return ErrStoreTransactionLockUnavailable
}

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

// restorePrivateStoreDirMode restores an install-time privacy migration only
// through the same no-follow descriptor boundary used for the forward change.
// The descriptor must still name the exact pre-install object, so rollback
// cannot chmod a substituted path or a newly introduced foreign directory.
func restorePrivateStoreDirMode(dir string, original os.FileInfo) error {
	if original == nil {
		return fmt.Errorf("restore profile store root: missing original metadata")
	}
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open profile store root for rollback: %w", err)
	}
	f := os.NewFile(uintptr(fd), "zsh-pro profile store rollback")
	if f == nil {
		_ = syscall.Close(fd)
		return fmt.Errorf("retain profile store rollback descriptor")
	}
	defer func() { _ = f.Close() }()

	current, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat profile store root for rollback: %w", err)
	}
	if !current.IsDir() || !os.SameFile(original, current) {
		return fmt.Errorf("profile store root changed during installation")
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &stat); err != nil {
		return fmt.Errorf("stat profile store root rollback descriptor: %w", err)
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("profile store root is no longer owned by the current user")
	}
	if err := f.Chmod(original.Mode()); err != nil {
		return fmt.Errorf("restore profile store root mode: %w", err)
	}
	return nil
}
