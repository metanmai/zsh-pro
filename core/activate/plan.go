// Package activate contains shell-agnostic manifest and activation planning.
package activate

// Plan contains all reverse operations followed by all forward operations.
type Plan struct {
	Deactivate []Op
	Activate   []Op
}

// Op is implemented by each agnostic activation operation.
type Op interface{ activationOp() }

type RestoreScalar struct {
	Name, Applied string
	Original      *string
}

func (RestoreScalar) activationOp() {}

type UnsetScalar struct{ Name, Applied string }

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
