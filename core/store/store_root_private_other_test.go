//go:build !linux && !darwin

package store

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

// Store.InitForInstall must share the runtime helper's unsupported-platform
// boundary. In particular, FreeBSD must reject install before it creates a
// profile root that every descriptor-bound sourced runtime verb would reject.
func TestInitForInstallUnsupportedPlatformDoesNotMutateStoreRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := filepath.Join(t.TempDir(), "profiles")
	s, err := New(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InitForInstall(context.Background()); err == nil {
		t.Fatal("unsupported platform initialized a profile store")
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported platform created profile storage at %s: %v", root, err)
	}
}
