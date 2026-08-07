//go:build linux && !phase6_test_unsupported_atomic

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
	linuxRenameNoReplaceFlag uintptr = 1
	linuxRenameExchangeFlag  uintptr = 2
)

const targetRootLockRetryInterval = 10 * time.Millisecond

type targetRootFlockFunc func(int, int) error
type targetRootLockWaitFunc func(context.Context) error

var linuxRenameat2TrapByArch = map[string]uintptr{
	"386":      353,
	"amd64":    316,
	"arm":      382,
	"arm64":    276,
	"loong64":  276,
	"mips":     4351,
	"mips64":   5311,
	"mips64le": 5311,
	"mipsle":   4351,
	"ppc64":    357,
	"ppc64le":  357,
	"riscv64":  276,
	"s390x":    347,
}

func atomicRenamePlatformCapability(mode atomicRenameMode) error {
	if !validAtomicRenameMode(mode) {
		return errors.New("invalid atomic rename mode")
	}
	if _, ok := linuxRenameat2TrapByArch[runtime.GOARCH]; !ok {
		return ErrAtomicRenameUnsupported
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
	trap, ok := linuxRenameat2TrapByArch[runtime.GOARCH]
	if !ok {
		return ErrAtomicRenameUnsupported
	}
	fromPointer, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	toPointer, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	flag := linuxRenameExchangeFlag
	if mode == atomicRenameNoReplace {
		flag = linuxRenameNoReplaceFlag
	}
	_, _, errno := call(trap, uintptr(fromDirFD), uintptr(unsafe.Pointer(fromPointer)), uintptr(toDirFD), uintptr(unsafe.Pointer(toPointer)), flag, 0)
	runtime.KeepAlive(fromPointer)
	runtime.KeepAlive(toPointer)
	return classifyAtomicRenameErrno(errno)
}

func acquireTargetRootTransactionLock(ctx context.Context, lock *os.File) error {
	return acquireTargetRootTransactionLockWith(ctx, lock, syscall.Flock, waitForTargetRootLockRetry)
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
