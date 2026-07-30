//go:build linux || darwin

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// secureRuntimeRoot walks every absolute-path component with descriptor-
// relative O_NOFOLLOW opens. A check on a pathname followed by a later reopen
// can be raced through a sticky-directory symlink; this function never follows
// such a link and does not reopen the root after closing the final descriptor.
func secureRuntimeRoot(root string) error {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || root == "/" {
		return fmt.Errorf("runtime root must name a private absolute directory")
	}

	euid := uint32(os.Geteuid())
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open runtime root anchor: %w", err)
	}
	defer func() { _ = syscall.Close(fd) }()
	var anchor syscall.Stat_t
	if err := syscall.Fstat(fd, &anchor); err != nil {
		return fmt.Errorf("stat runtime root anchor: %w", err)
	}
	if anchor.Uid != euid && anchor.Uid != 0 {
		return fmt.Errorf("runtime root anchor has an untrusted owner")
	}
	if anchor.Mode&0o022 != 0 && anchor.Mode&0o1000 == 0 {
		return fmt.Errorf("runtime root anchor is writable without sticky protection")
	}

	parts := strings.Split(strings.TrimPrefix(root, "/"), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("runtime root contains an invalid path component")
		}
		next, err := runtimeOpenat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return fmt.Errorf("open runtime root component: %w", err)
		}
		if err := syscall.Close(fd); err != nil {
			_ = syscall.Close(next)
			return fmt.Errorf("close runtime root parent: %w", err)
		}
		fd = next

		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			return fmt.Errorf("stat runtime root component: %w", err)
		}
		if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
			return fmt.Errorf("runtime root component is not a directory")
		}
		mode := stat.Mode
		if i == len(parts)-1 {
			if stat.Uid != euid || mode&0o077 != 0 {
				return fmt.Errorf("runtime root is not private")
			}
			continue
		}
		if stat.Uid != euid && stat.Uid != 0 {
			return fmt.Errorf("runtime root ancestor has an untrusted owner")
		}
		if mode&0o022 != 0 && mode&0o1000 == 0 {
			return fmt.Errorf("runtime root ancestor is writable without sticky protection")
		}
	}
	return nil
}
