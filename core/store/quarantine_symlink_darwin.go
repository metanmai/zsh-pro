//go:build darwin

package store

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
)

const darwinOSymlink = 0x200000

func openAuthenticatedQuarantineSymlink(parent *os.Root, name string) (*os.File, error) {
	if name == "" || strings.ContainsRune(name, '/') {
		return nil, os.ErrInvalid
	}
	// os.Root always adds O_NOFOLLOW, which makes Darwin report ENOENT for a
	// dangling symlink even with O_SYMLINK. A duplicate directory descriptor
	// anchors /dev/fd/N/name while O_SYMLINK opens the link object itself rather
	// than following its target.
	directory, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	path := fmt.Sprintf("/dev/fd/%d/%s", directory.Fd(), name)
	link, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|darwinOSymlink, 0)
	runtime.KeepAlive(directory)
	return link, err
}
