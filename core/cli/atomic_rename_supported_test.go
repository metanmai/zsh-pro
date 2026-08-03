//go:build (linux || darwin) && !phase6_test_unsupported_atomic

package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestAtomicRenameAtExchangePreservesBothInodes(t *testing.T) {
	dir := t.TempDir()
	writeAtomicRenameFixture(t, dir, "left", []byte("left"))
	writeAtomicRenameFixture(t, dir, "right", []byte("right"))
	leftBefore := atomicRenameFixtureInfo(t, dir, "left")
	rightBefore := atomicRenameFixtureInfo(t, dir, "right")

	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := atomicRenameAt(int(root.Fd()), "left", "right", atomicRenameExchange); err != nil {
		t.Fatal(err)
	}

	if !os.SameFile(rightBefore, atomicRenameFixtureInfo(t, dir, "left")) {
		t.Fatal("exchange did not move the right inode to the left name")
	}
	if !os.SameFile(leftBefore, atomicRenameFixtureInfo(t, dir, "right")) {
		t.Fatal("exchange did not move the left inode to the right name")
	}
}

func TestAtomicRenameAtNoReplaceCreatesAbsentTarget(t *testing.T) {
	dir := t.TempDir()
	writeAtomicRenameFixture(t, dir, "candidate", []byte("candidate"))
	before := atomicRenameFixtureInfo(t, dir, "candidate")
	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := atomicRenameAt(int(root.Fd()), "candidate", "target", atomicRenameNoReplace); err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, atomicRenameFixtureInfo(t, dir, "target")) {
		t.Fatal("no-replace did not move the candidate inode to the absent target")
	}
	if _, err := os.Lstat(dir + "/candidate"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate remains after no-replace: %v", err)
	}
}

func TestAtomicRenameAtNoReplacePreservesLateTarget(t *testing.T) {
	dir := t.TempDir()
	writeAtomicRenameFixture(t, dir, "candidate", []byte("candidate"))
	writeAtomicRenameFixture(t, dir, "target", []byte("late"))
	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = atomicRenameAt(int(root.Fd()), "candidate", "target", atomicRenameNoReplace)
	if !errors.Is(err, syscall.EEXIST) {
		t.Fatalf("no-replace error = %v, want EEXIST", err)
	}
	if got, err := os.ReadFile(dir + "/target"); err != nil || string(got) != "late" {
		t.Fatalf("late target changed: bytes=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(dir + "/candidate"); err != nil || string(got) != "candidate" {
		t.Fatalf("candidate changed: bytes=%q err=%v", got, err)
	}
}

func TestAtomicRenameAtUnsupportedLeavesBothNames(t *testing.T) {
	dir := t.TempDir()
	writeAtomicRenameFixture(t, dir, "left", []byte("left"))
	writeAtomicRenameFixture(t, dir, "right", []byte("right"))
	root, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	calls := 0
	call := func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno) {
		calls++
		return 0, 0, syscall.ENOSYS
	}
	if err := atomicRenameAtWithSyscall(call, int(root.Fd()), "left", "right", atomicRenameExchange); !errors.Is(err, ErrAtomicRenameUnsupported) {
		t.Fatalf("injected unsupported error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("raw calls = %d, want 1", calls)
	}
	if got, _ := os.ReadFile(dir + "/left"); string(got) != "left" {
		t.Fatalf("left changed to %q", got)
	}
	if got, _ := os.ReadFile(dir + "/right"); string(got) != "right" {
		t.Fatalf("right changed to %q", got)
	}
}

func writeAtomicRenameFixture(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(dir+"/"+name, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func atomicRenameFixtureInfo(t *testing.T, dir, name string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(dir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
