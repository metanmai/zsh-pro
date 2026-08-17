// Package shell defines the shell-agnostic seam. Each concern is its own small
// interface (interface segregation); Provider composes them for wiring at the
// composition root. The rest of the engine depends on the narrowest interface
// it needs.
package shell

import (
	"zsh-pro/core/activate"
	"zsh-pro/core/model"
)

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

// WorktreeRegenerator emits the complete generated source for one validated,
// materialized worktree document. It remains optional so legacy Provider
// consumers do not need to implement Phase 7 behavior.
type WorktreeRegenerator interface {
	RegenerateWorktree(model.CommittedWorktree) ([]byte, error)
}

// LiveSecretPolicy is the classifier-owned admission boundary for newly
// observed live identities.
type LiveSecretPolicy interface {
	IsLiveSecretIdentity(model.Identity) bool
}

// LiveCaptureSource provides the sourced-shell program that records current
// supported state. The concrete shell owns this source; consumers only run or
// transport it.
type LiveCaptureSource interface {
	LiveCaptureSource() string
}

// LiveSnapshotDecoder validates and decodes one bounded semantic capture.
type LiveSnapshotDecoder interface {
	DecodeLiveSnapshot([]byte) (model.LiveSnapshot, error)
}

// LivePatchEmitter renders validated shell-neutral operations into one exact
// caller-named apply/replacement-reverse payload. It remains separate from the
// broad Provider until the concrete live emitter exists.
type LivePatchEmitter interface {
	EmitLivePatch(forward, replacementReverse []activate.Op, applyName, reverseName string) ([]byte, error)
}

// Emitter is the sole reverse-zsh code-generation seam. Implementations turn
// the shell-agnostic activation plan into apply and deactivate loader blocks.
type Emitter interface {
	Emit(p activate.Plan) (apply, deactivate string, err error)
}

// RuntimeEmitter emits a loader-only payload whose apply and reverse function
// names are chosen by the caller. The loader uses this narrower transport to
// avoid replacing user-visible generic helper names in the live shell.
type RuntimeEmitter interface {
	Emitter
	EmitRuntime(p activate.Plan, applyName, deactivateName string) (apply, deactivate string, err error)
}

// Hooker provides the sourced runtime loader. Like Regenerator and Emitter,
// this is a boundary where zsh syntax originates, so implementations belong in
// the concrete shell package rather than a caller assembling shell text.
type Hooker interface {
	HookScript() string
}

// Provider composes the shell concerns for convenient wiring.
type Provider interface {
	Parser
	Classifier
	Introspector
	Regenerator
	Hooker
}
