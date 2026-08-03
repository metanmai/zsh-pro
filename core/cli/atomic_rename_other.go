//go:build (!linux && !darwin) || phase6_test_unsupported_atomic

package cli

import "os"

func atomicRenamePlatformCapability(atomicRenameMode) error {
	return ErrAtomicRenameUnsupported
}

func atomicRenameAtWithSyscallPlatform(syscall6Fn, int, string, string, atomicRenameMode) error {
	return ErrAtomicRenameUnsupported
}

func acquireTargetRootTransactionLock(*os.File) error {
	return ErrAtomicRenameUnsupported
}
