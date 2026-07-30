//go:build darwin

package cli

import _ "unsafe" // required for go:linkname

// syscall exposes openat only as an internal Darwin wrapper. Keep the
// descriptor-relative primitive here rather than falling back to a pathname
// reopen; the latter would recreate the symlink replacement race this helper
// is responsible for closing.
//
//go:linkname runtimeOpenat syscall.openat
func runtimeOpenat(fd int, path string, flags int, perm uint32) (int, error)
