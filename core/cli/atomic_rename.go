package cli

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

type atomicRenameMode uint8

const (
	atomicRenameExchange atomicRenameMode = iota + 1
	atomicRenameNoReplace
)

// ErrAtomicRenameUnsupported means the exact exchange/no-replace primitive is
// unavailable. Callers must fail closed; it is never permission to retry with
// ordinary rename, links, or unlink-based replacement.
var ErrAtomicRenameUnsupported = errors.New("zsh-pro: atomic rename operation is unsupported")

type syscall6Fn func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno)

func atomicRenameCapabilityCheck(mode atomicRenameMode) error {
	if !validAtomicRenameMode(mode) {
		return errors.New("invalid atomic rename mode")
	}
	return atomicRenamePlatformCapability(mode)
}

func atomicRenameAt(dirFD int, from, to string, mode atomicRenameMode) error {
	return atomicRenameAtWithSyscall(syscall.Syscall6, dirFD, from, to, mode)
}

func atomicRenameAtWithSyscall(call syscall6Fn, dirFD int, from, to string, mode atomicRenameMode) error {
	return atomicRenameBetweenAtWithSyscall(call, dirFD, from, dirFD, to, mode)
}

// atomicRenameBetweenAt performs the same audited operation across two
// retained directory descriptors. atomicRenameAt remains the sibling wrapper
// used by direct adapter tests; guarded install promotion needs this form
// because its secret-bearing exchange peer lives in a private transaction
// directory rather than beside the user-visible target.
func atomicRenameBetweenAt(fromDirFD int, from string, toDirFD int, to string, mode atomicRenameMode) error {
	return atomicRenameBetweenAtWithSyscall(syscall.Syscall6, fromDirFD, from, toDirFD, to, mode)
}

func atomicRenameBetweenAtWithSyscall(
	call syscall6Fn,
	fromDirFD int,
	from string,
	toDirFD int,
	to string,
	mode atomicRenameMode,
) error {
	if call == nil {
		return errors.New("atomic rename syscall is unavailable")
	}
	if fromDirFD < 0 || toDirFD < 0 {
		return errors.New("atomic rename directory descriptor is invalid")
	}
	if err := validateAtomicRenameBasename(from); err != nil {
		return err
	}
	if err := validateAtomicRenameBasename(to); err != nil {
		return err
	}
	if fromDirFD == toDirFD && from == to {
		return errors.New("atomic rename names must be distinct")
	}
	if err := atomicRenameCapabilityCheck(mode); err != nil {
		return err
	}
	return atomicRenameAtWithSyscallPlatform(call, fromDirFD, from, toDirFD, to, mode)
}

func validAtomicRenameMode(mode atomicRenameMode) bool {
	return mode == atomicRenameExchange || mode == atomicRenameNoReplace
}

func validateAtomicRenameBasename(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("atomic rename requires a safe sibling basename")
	}
	return nil
}

func classifyAtomicRenameErrno(errno syscall.Errno) error {
	if errno == 0 {
		return nil
	}
	if errno == syscall.ENOSYS || errno == syscall.EINVAL || errno == syscall.EOPNOTSUPP || errno == syscall.EXDEV {
		return fmt.Errorf("%w: %v", ErrAtomicRenameUnsupported, errno)
	}
	return errno
}
