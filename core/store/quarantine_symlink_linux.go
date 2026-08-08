//go:build linux

package store

import (
	"os"
	"syscall"
)

func openAuthenticatedQuarantineSymlink(parent *os.Root, name string) (*os.File, error) {
	// O_PATH keeps the link inode open without following its target.
	return parent.OpenFile(name, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|0x200000, 0)
}
