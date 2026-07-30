//go:build !linux && !darwin && !freebsd

package store

import "fmt"

// ensurePrivateStoreDir fails closed on platforms without a supported
// descriptor-relative no-follow open, Fstat, and Fchmod sequence. A
// pathname check followed by chmod can be raced onto a substituted or foreign
// target, so Store.Init must not create or mutate profile storage here.
func ensurePrivateStoreDir(dir string) error {
	return fmt.Errorf("private profile store initialization is unsupported on this platform")
}
