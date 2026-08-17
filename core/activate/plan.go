// Package activate contains shell-agnostic manifest and activation planning.
package activate

import "zsh-pro/core/model"

// Plan contains all reverse operations followed by all forward operations.
type Plan struct {
	Deactivate []Op
	Activate   []Op
}

// Op is implemented by each agnostic activation operation.
type Op interface{ activationOp() }

// LivePatch carries the exact forward mutation and a complete replacement
// reverse plan for the shell's newly acknowledged live state.
type LivePatch struct {
	Forward            []Op
	ReplacementReverse []Op
}

// SetLiveScalar sets one scalar-valued live identity. Identity.Kind is limited
// to environment, alias, and function by BuildLivePatch and the concrete
// emitter validates it again before producing source.
type SetLiveScalar struct {
	Identity model.Identity
	Value    string
}

func (SetLiveScalar) activationOp() {}

// RemoveLiveScalar removes one scalar-valued live identity.
type RemoveLiveScalar struct{ Identity model.Identity }

func (RemoveLiveScalar) activationOp() {}

// TransitionLiveList transforms one PATH/FPATH value while retaining both
// sides so the concrete emitter can preserve occurrence ownership.
type TransitionLiveList struct {
	Identity                    model.Identity
	BeforePresent, AfterPresent bool
	Before, After               []string
}

func (TransitionLiveList) activationOp() {}

// SetLiveOptionState and RemoveLiveOptionState model exact option state and an
// explicit option tombstone without carrying shell syntax across the boundary.
type SetLiveOptionState struct {
	Identity model.Identity
	Enabled  bool
}

func (SetLiveOptionState) activationOp() {}

type RemoveLiveOptionState struct{ Identity model.Identity }

func (RemoveLiveOptionState) activationOp() {}

type RestoreScalar struct {
	Name, Applied string
	// Dynamic preserves whether Applied is evaluated at activation time. The
	// runtime renderer needs this to retain an actual applied-value slot only
	// when a literal comparison cannot safely represent that evaluated value.
	Dynamic  bool
	Original *string
}

func (RestoreScalar) activationOp() {}

type UnsetScalar struct {
	Name, Applied string
	Dynamic       bool
}

func (UnsetScalar) activationOp() {}

// RebuildListFromBase restores the captured base list and drops this profile's
// additions; multi-profile co-ownership is intentionally out of scope.
type RebuildListFromBase struct{ Name string }

func (RebuildListFromBase) activationOp() {}

type Unalias struct{ Name string }

func (Unalias) activationOp() {}

type RestoreShadowedAlias struct{ Name string }

func (RestoreShadowedAlias) activationOp() {}

type UnsetFunc struct{ Name string }

func (UnsetFunc) activationOp() {}

type RestoreShadowedFunc struct{ Name string }

func (RestoreShadowedFunc) activationOp() {}

// RestoreOption restores the runtime-captured prior state; WasOn is deliberately
// not carried because the builder cannot author that live fact.
type RestoreOption struct{ Name string }

func (RestoreOption) activationOp() {}

type SetScalar struct {
	Name, Applied string
	Dynamic       bool
	Exported      bool
}

func (SetScalar) activationOp() {}

type ApplyListDelta struct {
	Name                 string
	Additions, Deletions []string
	BaseIndex            *int
	AdditionDynamic      []bool
}

func (ApplyListDelta) activationOp() {}

type AddAlias struct {
	Name, Body string
	Dynamic    bool
}

func (AddAlias) activationOp() {}

type AddFunc struct{ Name, Body string }

func (AddFunc) activationOp() {}

type SetOption struct {
	Name    string
	Enabled bool
}

func (SetOption) activationOp() {}
