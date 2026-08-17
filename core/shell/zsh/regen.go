package zsh

import (
	"fmt"
	"strings"

	"zsh-pro/core/model"
)

// RegenerateWorktree validates and lowers one committed final-state document.
// It deliberately performs no quoting or executable command construction: all
// such source originates in emit.go through the two typed helpers below.
func (Provider) RegenerateWorktree(document model.CommittedWorktree) ([]byte, error) {
	if err := validateCommittedWorktree(document); err != nil {
		return nil, err
	}
	exported, knownExport := committedExportState(document.Source)
	var generated []byte
	for _, state := range document.Projection.States {
		isExported := true
		if state.Identity.Kind == model.LiveEnv && knownExport[state.Identity.Name] {
			isExported = exported[state.Identity.Name]
		}
		line, err := emitLiveIdentityState(state, isExported)
		if err != nil {
			return nil, err
		}
		generated = append(generated, line...)
	}
	for _, identity := range document.Projection.Tombstones {
		line, err := emitLiveIdentityTombstone(identity)
		if err != nil {
			return nil, err
		}
		generated = append(generated, line...)
	}
	return generated, nil
}

func validateCommittedWorktree(document model.CommittedWorktree) error {
	if document.Schema != model.WorktreeSchemaV1 {
		return fmt.Errorf("committed worktree schema %q unsupported", document.Schema)
	}
	if document.Projection.Schema != model.WorktreeSchemaV1 {
		return fmt.Errorf("live projection schema %q unsupported", document.Projection.Schema)
	}
	normalized, err := model.NormalizeLiveStates(document.Projection.States)
	if err != nil {
		return err
	}
	if len(normalized) != len(document.Projection.States) {
		return fmt.Errorf("live projection contains duplicate identities")
	}
	seen := make(map[model.Identity]bool, len(normalized)+len(document.Projection.Tombstones))
	pinned := committedPinnedIdentities(document.Source)
	for _, state := range normalized {
		if !state.Value.Present {
			return fmt.Errorf("live projection state %s/%s is not present", state.Identity.Kind, state.Identity.Name)
		}
		if pinned[state.Identity] {
			return fmt.Errorf("live projection contains pinned identity %s/%s", state.Identity.Kind, state.Identity.Name)
		}
		seen[state.Identity] = true
	}
	for _, identity := range document.Projection.Tombstones {
		if err := model.ValidateIdentity(identity); err != nil {
			return err
		}
		if seen[identity] {
			return fmt.Errorf("live projection identity %s/%s is both present and removed", identity.Kind, identity.Name)
		}
		if pinned[identity] {
			return fmt.Errorf("live projection contains pinned identity %s/%s", identity.Kind, identity.Name)
		}
		seen[identity] = true
	}
	return nil
}

func committedPinnedIdentities(profile model.Profile) map[model.Identity]bool {
	pinned := make(map[model.Identity]bool)
	for _, entry := range profile.Entries {
		if entry.Secret == nil || len(entry.Names) != 1 || entry.Kind != model.KindAssignment {
			continue
		}
		pinned[model.Identity{Kind: model.LiveEnv, Name: entry.Names[0]}] = true
	}
	return pinned
}

func committedExportState(profile model.Profile) (map[string]bool, map[string]bool) {
	exported := make(map[string]bool)
	known := make(map[string]bool)
	for _, entry := range profile.Entries {
		if entry.Secret != nil || !entry.EffectiveManaged() || !entry.Representable() || entry.Kind != model.KindAssignment || len(entry.Names) != 1 {
			continue
		}
		if entry.Category != model.CatEnvironment && entry.Category != model.CatSecrets {
			continue
		}
		name := entry.Names[0]
		known[name] = true
		exported[name] = exported[name] || entry.Exported
	}
	return exported, known
}

// Regenerate rebuilds a declarative entry into behavior-equivalent zsh source
// from its structured fields. It is the only place this phase that generates
// forward zsh syntax (the shell.Regenerator seam).
//
// The captured Value is emitted VERBATIM — never %q-quoted and never wrapped in
// hard single quotes — so a dynamic value ($HOME/go, $(...), `...`) stays
// late-bound and survives the round-trip intact (D-05 / EVAL-01 / Pitfall 3).
// This diverges deliberately from core/testgen/render.go, which uses %q / hard
// quotes for STATIC literals (re-quoting would freeze a dynamic value).
//
// The default case returns the entry's verbatim Text, so Regenerate is total:
// any Kind it does not explicitly template — including a forced-managed entry of
// an unhandled Kind — falls through to verbatim and can never produce empty or
// undefined output (#3).
func (Provider) Regenerate(e model.Entry) string {
	// A persisted v3 entry can reach this lowering seam independently of
	// ir.Regenerate. Never let a forced override use partial source structure to
	// synthesize a different command: known-but-unrepresentable shapes remain
	// verbatim exactly as the shared admission gate requires. Legacy callers
	// without fidelity provenance retain the historical direct-regeneration API.
	if e.StructuralFidelityKnown && !e.Representable() {
		return e.Text
	}
	switch e.Kind {
	case model.KindAssignment:
		// Belt-and-suspenders length guard (BL-01): the router already keeps
		// empty-Names assignments out of the managed path, but a forced-managed
		// (OverrideManaged) entry could still reach here. Templating needs a name;
		// without one, fall through to verbatim Text rather than panicking on
		// Names[0] (the "never panics in production" invariant).
		if len(e.Names) == 0 {
			return e.Text
		}
		// Belt-and-suspenders empty-Value guard (UAT array gap): a KindAssignment
		// with a name but no captured scalar Value — e.g. an array assignment
		// (`name=(...)`, where mvdan/sh leaves a.Value nil) forced managed via
		// OverrideManaged — must NOT emit a bare `name=` that drops the value. Fall
		// through to verbatim Text, same spirit as the empty-Names guard above. The
		// router already routes arrays imperative; this defends against an override.
		if e.Value == "" {
			return e.Text
		}
		if e.Exported {
			return fmt.Sprintf("export %s=%s", e.Names[0], e.Value)
		}
		return fmt.Sprintf("%s=%s", e.Names[0], e.Value)
	case model.KindAlias:
		if len(e.Names) == 0 { // BL-01 guard, same rationale as KindAssignment
			return e.Text
		}
		return fmt.Sprintf("alias %s=%s", e.Names[0], e.Value)
	case model.KindFuncDecl:
		// Value is the full `name() { ... }` span (pinned by Plan 02-01), so
		// emitting it verbatim re-declares the same function — no wrapping.
		return e.Value
	case model.KindCommand:
		if e.CmdName == "setopt" || e.CmdName == "unsetopt" {
			// IN-01: a forced-managed bare setopt has no option name to template;
			// emit verbatim Text rather than "setopt " (dangling trailing space).
			if len(e.Names) == 0 {
				return e.Text
			}
			return fmt.Sprintf("%s %s", e.CmdName, strings.Join(e.Names, " "))
		}
		// Non-setopt commands are imperative: fall through to verbatim Text.
		return e.Text
	default:
		// Any Kind not explicitly templated — emit verbatim so the seam is total.
		return e.Text
	}
}
