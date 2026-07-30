//go:build !linux && !darwin

package cli

import (
	"errors"
)

// Fail closed rather than using a pathname-only implementation on platforms
// without the descriptor-relative no-follow primitives required for the cache.
func secureCacheDirectory(string) (*cacheDirectoryState, error) {
	return nil, errors.New("cached loader installation requires no-follow descriptor traversal on this platform")
}

func (s *cacheDirectoryState) prepareLoaderRollback() (cacheWriteRollback, error) {
	return cacheWriteRollback{}, errors.New("cached loader installation is unsupported on this platform")
}

func (s *cacheDirectoryState) prepareValidatedLoader([]byte) (preparedCacheWrite, error) {
	return preparedCacheWrite{}, errors.New("cached loader installation is unsupported on this platform")
}

func (w *preparedCacheWrite) promote() error {
	return errors.New("cached loader installation is unsupported on this platform")
}

func (w *preparedCacheWrite) discard() {}

func (r *cacheWriteRollback) restore() error {
	return errors.New("cached loader installation is unsupported on this platform")
}

func (r *cacheWriteRollback) discard() {}

func rollbackPromotedCache(*preparedCacheWrite, *cacheWriteRollback) error { return nil }

func (s *cacheDirectoryState) rollback() error { return nil }

func (s *cacheDirectoryState) close() error { return nil }
