//go:build !linux && !darwin && !freebsd

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateStoreDirUnsupportedDoesNotTouchSubstitutedTarget(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.Mkdir(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "store-link")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("create substituted symlink: %v", err)
	}

	if err := ensurePrivateStoreDir(link); err == nil {
		t.Fatal("unsupported initializer accepted a substituted target")
	}
	info, err := os.Stat(victim)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("unsupported initializer changed substituted target mode = %#o, want 0755", got)
	}
}
