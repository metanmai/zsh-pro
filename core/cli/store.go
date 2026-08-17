package cli

import (
	"context"

	"zsh-pro/core/model"
	"zsh-pro/core/worktree"
)

// Store is the CLI's narrow profile-store dependency. The composition root
// supplies *store.Store; this package deliberately does not import it.
type Store interface {
	Branches(ctx context.Context) ([]string, error)
	Current() string
	Checkout(ctx context.Context, name string) error
	Read(ctx context.Context, branch string) (model.Profile, error)
}

// WorktreeReader is the complete value-safe public observation boundary. It
// deliberately carries no legacy per-process current-profile operations.
type WorktreeReader interface {
	WorkflowStatus(context.Context, string) (worktree.WorkflowStatus, error)
	Diff(context.Context) (model.CategorizedDiff, error)
	Branches(context.Context) ([]string, error)
}

// WorktreeWorkflow is the complete durable mutation boundary. Commit has no
// staging/partial input, and checkout's create intent is explicit.
type WorktreeWorkflow interface {
	Commit(context.Context, string) (model.WorktreeCommitResult, error)
	Branch(context.Context, string) error
	Checkout(context.Context, string, bool) error
	ResetHard(context.Context) error
	SetAutoApplyDefault(context.Context, bool) error
}

// IngestTransactionStore is the CLI-neutral authenticated ingest surface. It
// deliberately carries only model-owned authority and outcome values; core/cli
// never imports or reconstructs the concrete Store.
type IngestTransactionStore interface {
	BeginIngest(context.Context, model.InstallInitializationID) (model.IngestBeginOutcome, error)
	CommitIngest(context.Context, model.InstallInitializationID, model.IngestTransactionID, model.Profile, string) (model.IngestCommitOutcome, error)
	AbortIngest(context.Context, model.InstallInitializationID, model.IngestTransactionID) (model.IngestAbortOutcome, error)
}

// StoreInitialization records only the profile-store state that an explicit
// install is allowed to compensate if a later bootstrap step fails. Rollback
// must never delete or rewind a pre-existing repository; CreatedPath is set
// only when this invocation created the store root so installer rollback can
// order an overlapping runtime-directory cleanup safely.
type StoreInitialization struct {
	InitializationID model.InstallInitializationID
	Transactions     IngestTransactionStore
	CanonicalRoot    string
	Rollback         func() error
	Finalize         func() error

	// CreatedPath is retained for the landed install adapter until the Phase 6
	// composition-root plan supplies CanonicalRoot directly. It is ownership
	// evidence only when this invocation created the root.
	CreatedPath string
}

// StoreInitializer bootstraps or safely migrates the persistent profile store
// during an explicit install. It is deliberately separate from Store so that
// ordinary read-only CLI verbs cannot create or mutate profile storage. A
// successful initializer returns compensation for state it created or migrated
// during this invocation; the installer invokes it on every later failure.
type StoreInitializer func(ctx context.Context) (StoreInitialization, error)
