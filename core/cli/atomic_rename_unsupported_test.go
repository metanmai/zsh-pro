//go:build (!linux && !darwin) || phase6_test_unsupported_atomic

package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestAtomicRenameAtNativeUnsupportedZeroCalls(t *testing.T) {
	dir := t.TempDir()
	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := atomicRenameAt(int(root.Fd()), "candidate", "target", atomicRenameNoReplace); !errors.Is(err, ErrAtomicRenameUnsupported) {
		t.Fatalf("native other-platform adapter error = %v, want typed unsupported", err)
	}
	calls := 0
	call := func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno) {
		calls++
		return 0, 0, 0
	}
	if err := atomicRenameAtWithSyscall(call, int(root.Fd()), "candidate", "target", atomicRenameNoReplace); !errors.Is(err, ErrAtomicRenameUnsupported) {
		t.Fatalf("injected other-platform adapter error = %v, want typed unsupported", err)
	}
	if calls != 0 {
		t.Fatalf("other-platform raw calls = %d, want 0", calls)
	}
}
