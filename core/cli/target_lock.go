package cli

import (
	"context"
	"time"
)

// A second install or ingest must not wait forever for a transaction that may
// have been abandoned. Keep the bound long enough for the normal validation
// and Git operations that run while the target lock is held, while preserving
// a caller's explicit, shorter cancellation or deadline.
const (
	targetRootLockRetryInterval    = 10 * time.Millisecond
	targetRootLockAcquisitionLimit = validationTimeout + targetRootLockRetryInterval
)

func boundedTargetRootLockContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return nil, func() {}
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, targetRootLockAcquisitionLimit)
}
