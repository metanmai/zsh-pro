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

// Regenerator emits behavior-equivalent forward zsh source for a declarative
// entry. It is the single place forward zsh syntax is generated this phase
// (the milestone invariant pins zsh-syntax codegen to the zsh package), so the
// agnostic core/ir delegates all syntax generation here. It rebuilds the entry
// from structured fields, emitting the captured value verbatim (dynamic values
// stay late-bound, never resolved); its default case returns the entry's
// verbatim Text so it is total — no Kind ever yields undefined/empty output.
type Regenerator interface {
	Regenerate(e model.Entry) string
}

// Provider composes the four concerns for convenient wiring.
type Provider interface {
	Parser
	Classifier
	Introspector
	Regenerator
}
