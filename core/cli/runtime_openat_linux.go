//go:build linux

package cli

import "syscall"

func runtimeOpenat(fd int, path string, flags int, perm uint32) (int, error) {
	return syscall.Openat(fd, path, flags, perm)
}
