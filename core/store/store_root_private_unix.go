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
	namespace     *os.Root
	lock          *os.File
	namespaceInfo os.FileInfo
	valid         bool
}

func storeTransactionNamespacePath(root string) (string, error) {
	canonical, err := canonicalStoreRootPath(root)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	name := ".zsh-pro-transactions-" + hex.EncodeToString(digest[:16])
	return filepath.Join(filepath.Dir(canonical), name), nil
}

func withStoreRootTransactionLock(root string, fn func(*storeRootTransactionGuard) error) error {
	if fn == nil {
		return ErrStoreTransactionLockUnavailable
	}
	namespacePath, err := storeTransactionNamespacePath(root)
	if err != nil {
		return ErrStoreTransactionLockUnavailable
	}
	parent, err := os.OpenRoot(filepath.Dir(namespacePath))
	if err != nil {
		return ErrStoreTransactionLockUnavailable
	}
	defer func() { _ = parent.Close() }()

	name := filepath.Base(namespacePath)
	info, err := parent.Lstat(name)
	if os.IsNotExist(err) {
		if err := parent.Mkdir(name, 0o700); err != nil && !os.IsExist(err) {
			return ErrStoreTransactionLockUnavailable
		}
		info, err = parent.Lstat(name)
	}
	if err != nil || !validTransactionNamespaceInfo(info) {
		return ErrStoreTransactionLockUnavailable
	}
	namespace, err := parent.OpenRoot(name)
	if err != nil {
		return ErrStoreTransactionLockUnavailable
	}
	defer func() { _ = namespace.Close() }()
	openedInfo, err := namespace.Stat(".")
	if err != nil || !validTransactionNamespaceInfo(openedInfo) || !os.SameFile(info, openedInfo) {
		return ErrStoreTransactionLockUnavailable
	}

	lock, err := openTransactionLock(namespace)
	if err != nil {
		return ErrStoreTransactionLockUnavailable
	}
	guard := &storeRootTransactionGuard{
		namespace:     namespace,
		lock:          lock,
		namespaceInfo: openedInfo,
	}
	defer func() {
		if guard.lock != nil {
			_ = unlockStoreRootTransaction(guard.lock)
			_ = guard.lock.Close()
			guard.lock = nil
		}
		guard.valid = false
	}()

	if err := lockStoreRootTransaction(lock); err != nil {
		return ErrStoreTransactionLockUnavailable
	}
	guard.valid = true
	if !guard.authenticationValid() {
		return ErrStoreTransactionLockUnavailable
	}
	callbackErr := fn(guard)
	if callbackErr != nil {
		return callbackErr
	}
	if !guard.authenticationValid() {
		return ErrStoreTransactionLockUnavailable
	}
	return nil
}

func (g *storeRootTransactionGuard) discardForTest() {
	if g == nil {
		return
	}
	if g.lock != nil {
		_ = g.lock.Close()
		g.lock = nil
	}
	g.valid = false
}

func (g *storeRootTransactionGuard) withAuthenticatedMutation(fn func(*os.Root) error) error {
	if fn == nil || !g.authenticationValid() {
		return ErrStoreTransactionLockUnavailable
	}
	return fn(g.namespace)
}

func (g *storeRootTransactionGuard) authenticationValid() bool {
	if g == nil || !g.valid || g.namespace == nil || g.lock == nil || g.namespaceInfo == nil {
		return false
	}
	lockInfo, err := g.lock.Stat()
	if err != nil || !validTransactionLockInfo(lockInfo) {
		return false
	}
	current, err := g.namespace.Stat(".")
	return err == nil && validTransactionNamespaceInfo(current) && os.SameFile(g.namespaceInfo, current)
}

func canonicalStoreRootPath(root string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return filepath.Clean(resolved), nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func openTransactionLock(namespace *os.Root) (*os.File, error) {
	info, err := namespace.Lstat("lock")
	if os.IsNotExist(err) {
		file, createErr := namespace.OpenFile(
			"lock",
			os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
			0o600,
		)
		if createErr == nil {
			if syncErr := syncTransactionRoot(namespace); syncErr != nil {
				_ = file.Close()
				return nil, syncErr
			}
			info, err = namespace.Lstat("lock")
			if err != nil || !validTransactionLockInfo(info) {
				_ = file.Close()
				return nil, ErrStoreTransactionLockUnavailable
			}
			openedInfo, statErr := file.Stat()
			if statErr != nil || !validTransactionLockInfo(openedInfo) || !os.SameFile(info, openedInfo) {
				_ = file.Close()
				return nil, ErrStoreTransactionLockUnavailable
			}
			return file, nil
		}
		if !os.IsExist(createErr) {
			return nil, createErr
		}
		info, err = namespace.Lstat("lock")
	}
	if err != nil || !validTransactionLockInfo(info) {
		return nil, ErrStoreTransactionLockUnavailable
	}
	file, err := namespace.OpenFile("lock", os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !validTransactionLockInfo(openedInfo) || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, ErrStoreTransactionLockUnavailable
	}
	return file, nil
}

func validTransactionNamespaceInfo(info os.FileInfo) bool {
	return info != nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm() == 0o700 && fileInfoOwnedByCurrentEUID(info)
}

func validTransactionLockInfo(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm() == 0o600 && fileInfoOwnedByCurrentEUID(info)
}

func fileInfoOwnedByCurrentEUID(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func lockStoreRootTransaction(file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if err == syscall.EINTR {
			continue
		}
		return err
	}
}

func unlockStoreRootTransaction(file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		if err == syscall.EINTR {
			continue
		}
		return err
	}
}

func syncTransactionRoot(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
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
