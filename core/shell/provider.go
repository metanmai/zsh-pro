// Package shell defines the shell-agnostic seam. Each concern is its own small
// interface (interface segregation); Provider composes them for wiring at the
// composition root. The rest of the engine depends on the narrowest interface
// it needs.
package shell

import "zsh-pro/core/model"

// Parser turns source into ordered, structurally-described blocks.
type Parser interface {
	Parse(src []byte) ([]model.Block, error)
}

// Classifier owns the category space and assigns a block to it.
type Classifier interface {
	Classify(b model.Block) (model.Category, model.Confidence)
	Categories() []model.Category
}

// Introspector runs the config in a sandbox and returns the resolved identity
// set. On failure it returns IdentitySet{Available: false}.
type Introspector interface {
	Introspect(path string) (model.IdentitySet, error)
}

// Provider composes the three concerns for convenient wiring.
type Provider interface {
	Parser
	Classifier
	Introspector
}
