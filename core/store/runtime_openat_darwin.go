//go:build darwin

package store

import _ "unsafe" // required for go:linkname

//go:linkname runtimeVaultOpenat syscall.openat
func runtimeVaultOpenat(fd int, path string, flags int, perm uint32) (int, error)
