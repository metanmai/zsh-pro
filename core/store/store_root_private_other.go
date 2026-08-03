//go:build !linux && !darwin

package store

import (
	"fmt"
	"os"
)

type storeRootTransactionGuard struct{}

func storeTransactionNamespacePath(root string) (string, error) {
	_ = root
	return "", ErrStoreTransactionLockUnavailable
}

func withStoreRootTransactionLock(root string, fn func(*storeRootTransactionGuard) error) error {
	_, _ = root, fn
	return ErrStoreTransactionLockUnavailable
}

func (g *storeRootTransactionGuard) discardForTest() { _ = g }

func (g *storeRootTransactionGuard) withAuthenticatedMutation(fn func(*os.Root) error) error {
	_, _ = g, fn
	return ErrStoreTransactionLockUnavailable
}

// The untagged transaction implementation shares these authentication and
// durability helpers with the supported Unix adapters. Unsupported platforms
// must provide fail-closed definitions so the package can still be compiled
// without accidentally accepting or syncing an unauthenticated namespace.
func validTransactionNamespaceInfo(info os.FileInfo) bool {
	_ = info
	return false
}

func validTransactionLockInfo(info os.FileInfo) bool {
	_ = info
	return false
}

func fileInfoOwnedByCurrentEUID(info os.FileInfo) bool {
	_ = info
	return false
}

func syncTransactionRoot(root *os.Root) error {
	_ = root
	return ErrStoreTransactionLockUnavailable
}

// ensurePrivateStoreDir fails closed on platforms without a supported
// descriptor-relative no-follow open, Fstat, and Fchmod sequence. A
// pathname check followed by chmod can be raced onto a substituted or foreign
// target, so Store.Init must not create or mutate profile storage here.
func ensurePrivateStoreDir(dir string) error {
	return fmt.Errorf("private profile store initialization is unsupported on this platform")
}

func restorePrivateStoreDirMode(dir string, original os.FileInfo) error {
	_ = dir
	_ = original
	return fmt.Errorf("private profile store rollback is unsupported on this platform")
}
