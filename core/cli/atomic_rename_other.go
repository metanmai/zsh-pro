//go:build (!linux && !darwin) || phase6_test_unsupported_atomic

package cli

import (
	"context"
	"os"
)

func atomicRenamePlatformCapability(atomicRenameMode) error {
	return ErrAtomicRenameUnsupported
}

func atomicRenameAtWithSyscallPlatform(syscall6Fn, int, string, int, string, atomicRenameMode) error {
	return ErrAtomicRenameUnsupported
}

func acquireTargetRootTransactionLock(context.Context, *os.File) error {
	return ErrAtomicRenameUnsupported
}
