//go:build (linux || darwin) && !phase6_test_unsupported_atomic

package cli

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestTargetRootTransactionLockRetriesNonblockingAndPreservesEINTR(t *testing.T) {
	lock, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	responses := []error{syscall.EINTR, syscall.EWOULDBLOCK, nil}
	flockCalls := 0
	waitCalls := 0
	err = acquireTargetRootTransactionLockWith(
		context.Background(),
		lock,
		func(_ int, operation int) error {
			if operation != syscall.LOCK_EX|syscall.LOCK_NB {
				t.Fatalf("flock operation = %d, want LOCK_EX|LOCK_NB", operation)
			}
			response := responses[flockCalls]
			flockCalls++
			return response
		},
		func(context.Context) error {
			waitCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if flockCalls != 3 || waitCalls != 1 {
		t.Fatalf("flock calls = %d, wait calls = %d; want 3 and 1", flockCalls, waitCalls)
	}
}

func TestTargetRootTransactionLockReturnsCancellationDuringContention(t *testing.T) {
	lock, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	flockCalls := 0
	err = acquireTargetRootTransactionLockWith(
		ctx,
		lock,
		func(int, int) error {
			flockCalls++
			return syscall.EWOULDBLOCK
		},
		func(ctx context.Context) error {
			cancel()
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lock error = %v, want context canceled", err)
	}
	if flockCalls != 1 {
		t.Fatalf("flock calls = %d, want 1", flockCalls)
	}
}

func TestTargetRootTransactionLockReturnsDeadlineDuringContention(t *testing.T) {
	lock, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = acquireTargetRootTransactionLockWith(
		ctx,
		lock,
		func(int, int) error { return syscall.EWOULDBLOCK },
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock error = %v, want deadline exceeded", err)
	}
}

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
