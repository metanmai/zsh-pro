//go:build darwin

package store

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// Darwin PATH_MAX is 1024. F_GETPATH writes a NUL-terminated path into a
// caller-owned PATH_MAX buffer.
const darwinPathMax = 1024

func runtimeGitWorkingDirectory(root *os.File) (string, error) {
	if root == nil {
		return "", ErrGitCommand
	}
	buffer := make([]byte, darwinPathMax)
	_, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL,
		root.Fd(),
		uintptr(syscall.F_GETPATH),
		uintptr(unsafe.Pointer(&buffer[0])),
	)
	runtime.KeepAlive(root)
	if errno != 0 {
		return "", ErrGitCommand
	}
	end := bytes.IndexByte(buffer, 0)
	if end <= 0 {
		return "", ErrGitCommand
	}
	path := string(buffer[:end])
	if !filepath.IsAbs(path) {
		return "", ErrGitCommand
	}
	descriptorInfo, descriptorErr := root.Stat()
	pathInfo, pathErr := os.Stat(path)
	if descriptorErr != nil || pathErr != nil || !descriptorInfo.IsDir() || !os.SameFile(descriptorInfo, pathInfo) {
		return "", ErrGitCommand
	}
	return path, nil
}
