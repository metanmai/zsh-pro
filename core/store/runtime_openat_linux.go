//go:build linux

package store

import "syscall"

func runtimeVaultOpenat(fd int, path string, flags int, perm uint32) (int, error) {
	return syscall.Openat(fd, path, flags, perm)
}
