//go:build linux || darwin

package store

import (
	"context"
	"errors"
	"os"
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
