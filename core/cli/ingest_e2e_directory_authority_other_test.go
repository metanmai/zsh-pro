//go:build !linux && !darwin

package cli

import "testing"

func requirePrivateCurrentUserDirectory(t *testing.T, _ string) {
	t.Helper()
	t.Fatal("directory ownership assertion is unavailable on this platform")
}
