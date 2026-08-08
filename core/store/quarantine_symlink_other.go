//go:build !linux && !darwin

package store

import "os"

func openAuthenticatedQuarantineSymlink(_ *os.Root, _ string) (*os.File, error) {
	return nil, os.ErrInvalid
}
