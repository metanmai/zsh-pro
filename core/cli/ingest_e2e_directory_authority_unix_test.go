//go:build linux || darwin

package cli

import (
	"os"
	"syscall"
	"testing"
)

func requirePrivateCurrentUserDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0o700 || stat.Uid != uint32(os.Geteuid()) {
		t.Fatalf("directory %s lacks mode-0700/current-EUID authority", path)
	}
}
