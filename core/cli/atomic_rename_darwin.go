//go:build darwin && !phase6_test_unsupported_atomic

package cli

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	darwinRenameatxNPTrap     uintptr = 488
	darwinRenameSwapFlag      uintptr = 2
	darwinRenameExclusiveFlag uintptr = 4
)

func atomicRenamePlatformCapability(mode atomicRenameMode) error {
	if !validAtomicRenameMode(mode) {
		return errors.New("invalid atomic rename mode")
	}
	return nil
}

func atomicRenameAtWithSyscallPlatform(call syscall6Fn, dirFD int, from, to string, mode atomicRenameMode) error {
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
	_, _, errno := call(darwinRenameatxNPTrap, uintptr(dirFD), uintptr(unsafe.Pointer(fromPointer)), uintptr(dirFD), uintptr(unsafe.Pointer(toPointer)), flag, 0)
	return classifyAtomicRenameErrno(errno)
}

func acquireTargetRootTransactionLock(lock *os.File) error {
	if lock == nil {
		return ErrAtomicRenameUnsupported
	}
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return err
	}
}
