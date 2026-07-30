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
// relative O_NOFOLLOW opens and retains the final directory plus its parent.
// A check on a pathname followed by a later reopen can be raced through a
// sticky-directory symlink; callers must use the returned descriptors rather
// than root after this function succeeds.
func secureRuntimeRoot(root string) (*RuntimeRoot, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || root == "/" {
		return nil, fmt.Errorf("runtime root must name a private absolute directory")
	}

	euid := uint32(os.Geteuid())
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open runtime root anchor: %w", err)
	}
	parentFD := -1
	defer func() {
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
		if parentFD >= 0 {
			_ = syscall.Close(parentFD)
		}
	}()
	var anchor syscall.Stat_t
	if err := syscall.Fstat(fd, &anchor); err != nil {
		return nil, fmt.Errorf("stat runtime root anchor: %w", err)
	}
	if anchor.Uid != euid && anchor.Uid != 0 {
		return nil, fmt.Errorf("runtime root anchor has an untrusted owner")
	}
	if anchor.Mode&0o022 != 0 && anchor.Mode&0o1000 == 0 {
		return nil, fmt.Errorf("runtime root anchor is writable without sticky protection")
	}

	parts := strings.Split(strings.TrimPrefix(root, "/"), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("runtime root contains an invalid path component")
		}
		if i == len(parts)-1 {
			parentFD, err = syscall.Dup(fd)
			if err != nil {
				return nil, fmt.Errorf("duplicate runtime root parent: %w", err)
			}
		}
		next, err := runtimeOpenat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open runtime root component: %w", err)
		}
		if err := syscall.Close(fd); err != nil {
			_ = syscall.Close(next)
			return nil, fmt.Errorf("close runtime root parent: %w", err)
		}
		fd = next

		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			return nil, fmt.Errorf("stat runtime root component: %w", err)
		}
		if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
			return nil, fmt.Errorf("runtime root component is not a directory")
		}
		mode := stat.Mode
		if i == len(parts)-1 {
			if stat.Uid != euid || mode&0o077 != 0 {
				return nil, fmt.Errorf("runtime root is not private")
			}
			continue
		}
		if stat.Uid != euid && stat.Uid != 0 {
			return nil, fmt.Errorf("runtime root ancestor has an untrusted owner")
		}
		if mode&0o022 != 0 && mode&0o1000 == 0 {
			return nil, fmt.Errorf("runtime root ancestor is writable without sticky protection")
		}
	}
	repository := os.NewFile(uintptr(fd), "zsh-pro runtime repository")
	vaultParent := os.NewFile(uintptr(parentFD), "zsh-pro runtime vault parent")
	fd = -1
	parentFD = -1
	if repository == nil || vaultParent == nil {
		if repository != nil {
			_ = repository.Close()
		}
		if vaultParent != nil {
			_ = vaultParent.Close()
		}
		return nil, fmt.Errorf("retain runtime root descriptors")
	}
	return &RuntimeRoot{repository: repository, vaultParent: vaultParent}, nil
}
