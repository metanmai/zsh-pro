package cli

import "os"

const cacheLoaderName = "loader.zsh"

// cacheDirectoryState owns the descriptor-authenticated cached-loader root for
// one install transaction. The fields are intentionally shared by the
// platform-specific implementations so unsupported platforms can fail closed
// without falling back to pathname-based cache writes.
type cacheDirectoryState struct {
	path         string
	directory    *os.File
	parent       *os.File
	name         string
	existed      bool
	created      bool
	originalMode os.FileMode
	modeChanged  bool
}

// preparedCacheWrite is a staged loader candidate whose basename is resolved
// only relative to its authenticated cache-directory descriptor.
type preparedCacheWrite struct {
	directory *cacheDirectoryState
	temp      string
	promoted  bool
}

// cacheWriteRollback is the descriptor-relative recovery state for the
// original loader. A missing original is recovered by unlinking the promoted
// loader without following any final symlink.
type cacheWriteRollback struct {
	directory *cacheDirectoryState
	temp      string
	remove    bool
}
