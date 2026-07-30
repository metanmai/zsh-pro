//go:build darwin

package cli

import (
	"syscall"
	"unsafe"
)

const (
	cacheDarwinRenameat = 465
	cacheDarwinUnlinkat = 472
	cacheDarwinMkdirat  = 475
	cacheATRemovedir    = 0x80
)

func cacheMkdirat(dirfd int, name string, mode uint32) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall(cacheDarwinMkdirat, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(mode))
	if errno != 0 {
		return errno
	}
	return nil
}

func cacheRenameat(fromFD int, from string, toFD int, to string) error {
	fromPtr, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(cacheDarwinRenameat, uintptr(fromFD), uintptr(unsafe.Pointer(fromPtr)), uintptr(toFD), uintptr(unsafe.Pointer(toPtr)), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func cacheUnlinkat(dirfd int, name string) error {
	return cacheUnlinkatFlags(dirfd, name, 0)
}

func cacheRemoveDirectoryAt(dirfd int, name string) error {
	return cacheUnlinkatFlags(dirfd, name, cacheATRemovedir)
}

func cacheUnlinkatFlags(dirfd int, name string, flags int) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(cacheDarwinUnlinkat, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(flags), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
