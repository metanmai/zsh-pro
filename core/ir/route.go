package ir

import "zsh-pro/core/model"

// routeManaged is the ING-02 declarative/imperative gate: it returns true iff a
// block is one of the admitted reversible classes — and ONLY in the precise
// shape the templater can faithfully, byte-reversibly reproduce. Every other
// shape (including the not-quite-templatable variants of an admitted class)
// routes imperative and is emitted verbatim, never mis-templated (D-04 / D-06).
//
// The core hardening principle: the router may only MANAGE what the regenerator
// can reproduce without loss. A shape it cannot faithfully template MUST route
// imperative — a verbatim round-trip is always safe, a lossy template is not.
//
// Admitted (D-04 / D-06):
//   - KindAssignment in CatEnvironment / CatPath / CatSecrets, with EXACTLY one
//     name and a plain `=` assignment. Bare/flag-only forms (`export`,
//     `export -p`) have no name; multi-name forms (`export A=1 B=2`) collapse to
//     one Value in the model; `+=` append forms would be rewritten to `=`. All
//     three route imperative (BL-01 / BL-02 / WR-01).
//   - KindAlias, with EXACTLY one name and NO type flag. Bare `alias`, alias-print
//     queries, multi-name `alias a=1 b=2`, and flagged `alias -g`/`-s` route
//     imperative (BL-01 / BL-02 / WR-02).
//   - KindFuncDecl (emitted as its full verbatim span — always faithful).
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
		// Faithfully templatable only as `alias NAME=VALUE`: exactly one name and
		// no type flag (BL-01 empty-name guard; BL-02 multi-name; WR-02 flag).
		return len(b.Names) == 1 && !b.Flagged
	case model.KindFuncDecl:
		return true
	case model.KindAssignment:
		// Faithfully templatable only as `[export ]NAME=VALUE`: exactly one name
		// and a plain `=` (BL-01 empty-name guard; BL-02 multi-name; WR-01 +=).
		if len(b.Names) != 1 || b.Append {
			return false
		}
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
