//go:build linux && !phase6_test_unsupported_atomic

package cli

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	linuxRenameNoReplaceFlag uintptr = 1
	linuxRenameExchangeFlag  uintptr = 2
)

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

func atomicRenameAtWithSyscallPlatform(call syscall6Fn, dirFD int, from, to string, mode atomicRenameMode) error {
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
	_, _, errno := call(trap, uintptr(dirFD), uintptr(unsafe.Pointer(fromPointer)), uintptr(dirFD), uintptr(unsafe.Pointer(toPointer)), flag, 0)
	runtime.KeepAlive(fromPointer)
	runtime.KeepAlive(toPointer)
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
