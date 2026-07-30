//go:build linux

package cli

import (
	"syscall"
	"unsafe"
)

const cacheATRemovedir = 0x200

func cacheMkdirat(dirfd int, name string, mode uint32) error {
	return syscall.Mkdirat(dirfd, name, mode)
}

func cacheRenameat(fromFD int, from string, toFD int, to string) error {
	return syscall.Renameat(fromFD, from, toFD, to)
}

func cacheUnlinkat(dirfd int, name string) error {
	return syscall.Unlinkat(dirfd, name)
}

func cacheRemoveDirectoryAt(dirfd int, name string) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_UNLINKAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(cacheATRemovedir), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
