package cli

import (
	"context"

	"zsh-pro/core/model"
)

// Store is the CLI's narrow profile-store dependency. The composition root
// supplies *store.Store; this package deliberately does not import it.
type Store interface {
	Branches(ctx context.Context) ([]string, error)
	Current() string
	Checkout(ctx context.Context, name string) error
	Read(ctx context.Context, branch string) (model.Profile, error)
}

// StoreInitializer bootstraps or safely migrates the persistent profile store
// during an explicit install. It is deliberately separate from Store so that
// ordinary read-only CLI verbs cannot create or mutate profile storage.
type StoreInitializer func(ctx context.Context) error
