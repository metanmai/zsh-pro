package ir

import "zsh-pro/core/model"

// routeManaged is the ING-02 declarative/imperative gate: it returns true iff a
// block is one of the 5 admitted reversible classes, so only those entries are
// switchable and the rest stay verbatim/imperative.
//
// Admitted (D-04 / D-06):
//   - KindAssignment in CatEnvironment / CatPath / CatSecrets
//   - KindAlias
//   - KindFuncDecl
//   - KindCommand setopt/unsetopt WITH at least one option name (a bare
//     setopt/unsetopt has no reversible state, so it routes imperative — #2)
//
// Everything else routes imperative, including the rest of the CatOptions
// grab-bag (zstyle/autoload/compinit/compdef/zmodload), Opaque blocks, and any
// unknown category (CatMisc/CatKeybindings/CatPlugins/CatLocal — the
// conservative default per D-06). The classifier's confidence value is NEVER
// consulted as a routing gate (D-06).
func routeManaged(b model.Block, cat model.Category) bool {
	if b.Opaque {
		return false
	}
	switch b.Kind {
	case model.KindAlias:
		return true
	case model.KindFuncDecl:
		return true
	case model.KindAssignment:
		return cat == model.CatEnvironment || cat == model.CatPath || cat == model.CatSecrets
	case model.KindCommand:
		// Only setopt/unsetopt are declarative within the CatOptions grab-bag,
		// and only when they carry at least one option name (#2). The rest
		// (zstyle/autoload/compinit/compdef/zmodload) route imperative.
		return cat == model.CatOptions &&
			(b.CmdName == "setopt" || b.CmdName == "unsetopt") &&
			len(b.Names) > 0
	default:
		// KindCompound / KindOther and anything else: imperative.
		return false
	}
}
