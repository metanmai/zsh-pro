//go:build linux

package store

import (
	"fmt"
	"os"
)

func runtimeGitWorkingDirectory(root *os.File) (string, error) {
	if root == nil {
		return "", ErrGitCommand
	}
	path := fmt.Sprintf("/proc/self/fd/%d", root.Fd())
	descriptorInfo, descriptorErr := root.Stat()
	pathInfo, pathErr := os.Stat(path)
	if descriptorErr != nil || pathErr != nil || !descriptorInfo.IsDir() || !os.SameFile(descriptorInfo, pathInfo) {
		return "", ErrGitCommand
	}
	return path, nil
}
