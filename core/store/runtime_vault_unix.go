//go:build linux || darwin

package store

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// readRuntimeVault opens the fallback vault through the authenticated parent
// descriptor. O_NOFOLLOW plus the ownership/mode check prevents a replacement
// symlink or attacker-owned sibling in a sticky ancestor from becoming a secret
// source after the runtime root was validated.
func readRuntimeVault(parent *os.File) ([]byte, error) {
	if parent == nil {
		return nil, ErrSecretBackendUnavailable
	}
	fd, err := runtimeVaultOpenat(
		int(parent.Fd()),
		vaultFileName,
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, ErrSecretBackendUnavailable
	}
	vault := os.NewFile(uintptr(fd), "zsh-pro runtime vault")
	if vault == nil {
		_ = syscall.Close(fd)
		return nil, ErrSecretBackendUnavailable
	}
	defer func() { _ = vault.Close() }()

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return nil, ErrSecretBackendUnavailable
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return nil, ErrSecretBackendUnavailable
	}
	b, err := io.ReadAll(vault)
	if err != nil {
		return nil, ErrSecretBackendUnavailable
	}
	return b, nil
}
