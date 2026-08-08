//go:build darwin && !phase6_test_unsupported_atomic

package cli

import (
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	darwinRenameatxNPTrap     uintptr = 488
	darwinRenameSwapFlag      uintptr = 2
	darwinRenameExclusiveFlag uintptr = 4
)

type targetRootFlockFunc func(int, int) error
type targetRootLockWaitFunc func(context.Context) error

func atomicRenamePlatformCapability(mode atomicRenameMode) error {
	if !validAtomicRenameMode(mode) {
		return errors.New("invalid atomic rename mode")
	}
	return nil
}

func atomicRenameAtWithSyscallPlatform(
	call syscall6Fn,
	fromDirFD int,
	from string,
	toDirFD int,
	to string,
	mode atomicRenameMode,
) error {
	fromPointer, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	toPointer, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	flag := darwinRenameSwapFlag
	if mode == atomicRenameNoReplace {
		flag = darwinRenameExclusiveFlag
	}
	_, _, errno := call(darwinRenameatxNPTrap, uintptr(fromDirFD), uintptr(unsafe.Pointer(fromPointer)), uintptr(toDirFD), uintptr(unsafe.Pointer(toPointer)), flag, 0)
	runtime.KeepAlive(fromPointer)
	runtime.KeepAlive(toPointer)
	return classifyAtomicRenameErrno(errno)
}

func acquireTargetRootTransactionLock(ctx context.Context, lock *os.File) error {
	bounded, cancel := boundedTargetRootLockContext(ctx)
	defer cancel()
	return acquireTargetRootTransactionLockWith(bounded, lock, syscall.Flock, waitForTargetRootLockRetry)
}

func acquireTargetRootTransactionLockWith(
	ctx context.Context,
	lock *os.File,
	flock targetRootFlockFunc,
	wait targetRootLockWaitFunc,
) error {
	if ctx == nil || lock == nil || flock == nil || wait == nil {
		return ErrAtomicRenameUnsupported
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		if err := wait(ctx); err != nil {
			return err
		}
	}
}

func waitForTargetRootLockRetry(ctx context.Context) error {
	timer := time.NewTimer(targetRootLockRetryInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
