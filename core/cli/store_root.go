package cli

import (
	"errors"
	"os"
	"path/filepath"
)

// StoreRoot resolves the profile-store directory used by ordinary CLI
// composition and descriptor-bound runtime capture. It is deliberately
// separate from the installer's loader-cache resolver: the cache defaults to
// $HOME/.zsh-pro, while the profile store defaults to
// $HOME/.local/share/zsh-pro.
func StoreRoot() (string, error) {
	if root, present := os.LookupEnv("ZSHPRO_HOME"); present {
		if root == "" || !filepath.IsAbs(root) {
			return "", errors.New("profile store requires ZSHPRO_HOME to be a non-empty absolute path when set")
		}
		return root, nil
	}
	if dataHome, present := os.LookupEnv("XDG_DATA_HOME"); present {
		if dataHome == "" || !filepath.IsAbs(dataHome) {
			return "", errors.New("profile store requires XDG_DATA_HOME to be a non-empty absolute path when set")
		}
		return filepath.Join(dataHome, "zsh-pro"), nil
	}
	home, present := os.LookupEnv("HOME")
	if !present || home == "" || !filepath.IsAbs(home) {
		return "", errors.New("profile store requires an absolute non-empty HOME")
	}
	return filepath.Join(home, ".local", "share", "zsh-pro"), nil
}
