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
