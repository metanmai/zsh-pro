//go:build linux || darwin

package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStoreRootTransactionLockRetriesNonblockingAndHandlesEINTR(t *testing.T) {
	lock, err := os.CreateTemp(t.TempDir(), "lock-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	responses := []error{syscall.EINTR, syscall.EWOULDBLOCK, nil}
	flockCalls := 0
	waitCalls := 0
	err = lockStoreRootTransactionWith(
		context.Background(),
		lock,
		func(_ int, operation int) error {
			if operation != syscall.LOCK_EX|syscall.LOCK_NB {
				t.Fatalf("flock operation = %d, want exclusive nonblocking", operation)
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
		t.Fatalf("retry counters: flock=%d wait=%d, want flock=3 wait=1", flockCalls, waitCalls)
	}
}

func TestStoreRootTransactionLockContentionHonorsCancellationAndDeadline(t *testing.T) {
	lock, err := os.CreateTemp(t.TempDir(), "lock-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	t.Run("cancellation after contention", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		flockCalls := 0
		err := lockStoreRootTransactionWith(
			ctx,
			lock,
			func(_ int, operation int) error {
				flockCalls++
				if operation != syscall.LOCK_EX|syscall.LOCK_NB {
					t.Fatalf("flock operation = %d, want exclusive nonblocking", operation)
				}
				return syscall.EWOULDBLOCK
			},
			func(ctx context.Context) error {
				cancel()
				<-ctx.Done()
				return ctx.Err()
			},
		)
		if !errors.Is(err, context.Canceled) || flockCalls != 1 {
			t.Fatalf("canceled contention = (%v, flock=%d), want context cancellation after one attempt", err, flockCalls)
		}
	})

	t.Run("deadline during contention", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := lockStoreRootTransactionWith(
			ctx,
			lock,
			func(_ int, operation int) error {
				if operation != syscall.LOCK_EX|syscall.LOCK_NB {
					t.Fatalf("flock operation = %d, want exclusive nonblocking", operation)
				}
				return syscall.EWOULDBLOCK
			},
			waitForStoreRootLockRetry,
		)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline contention = %v, want context deadline exceeded", err)
		}
	})
}

func TestStoreRootTransactionLockBoundsNonExpiringContext(t *testing.T) {
	if storeRootLockAcquisitionLimit <= gitTimeout {
		t.Fatalf("store root lock acquisition limit = %s, want longer than Git timeout %s", storeRootLockAcquisitionLimit, gitTimeout)
	}
	lock, err := os.CreateTemp(t.TempDir(), "lock-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	started := time.Now()
	err = lockStoreRootTransactionWithin(
		context.Background(),
		lock,
		20*time.Millisecond,
		func(_ int, operation int) error {
			if operation != syscall.LOCK_EX|syscall.LOCK_NB {
				t.Fatalf("flock operation = %d, want exclusive nonblocking", operation)
			}
			return syscall.EWOULDBLOCK
		},
		waitForStoreRootLockRetry,
	)
	if !errors.Is(err, ErrStoreTransactionLockUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded background lock = %v, want lock unavailable", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded background lock returned after %s", elapsed)
	}
}

func TestStoreTransactionCreationRepairsRestrictiveUmask(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	previousUmask := syscall.Umask(0o300)
	defer syscall.Umask(previousUmask)

	if err := withStoreRootTransactionLock(context.Background(), root, func(*storeRootTransactionGuard) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	namespace, err := storeTransactionNamespacePath(root)
	if err != nil {
		t.Fatal(err)
	}
	namespaceInfo, err := os.Lstat(namespace)
	if err != nil {
		t.Fatal(err)
	}
	if namespaceInfo.Mode().Perm() != 0o700 {
		t.Fatalf("created transaction namespace mode = %v, want 0700", namespaceInfo.Mode().Perm())
	}
	lockInfo, err := os.Lstat(filepath.Join(namespace, "lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lockInfo.Mode().Perm() != 0o600 {
		t.Fatalf("created transaction lock mode = %v, want 0600", lockInfo.Mode().Perm())
	}
}
