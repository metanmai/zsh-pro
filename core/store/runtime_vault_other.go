//go:build !linux && !darwin

package store

import "os"

// readRuntimeVault fails closed where descriptor-relative O_NOFOLLOW opening
// is unavailable. The CLI runtime helper is already unavailable there.
func readRuntimeVault(*os.File) ([]byte, error) { return nil, ErrSecretBackendUnavailable }
