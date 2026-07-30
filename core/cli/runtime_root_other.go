//go:build !linux && !darwin

package cli

import "errors"

// Fail closed on platforms without the descriptor-relative no-follow primitive
// required by the sourced runtime security boundary.
func secureRuntimeRoot(string) (*RuntimeRoot, error) {
	return nil, errors.New("runtime staging requires no-follow descriptor traversal on this platform")
}
