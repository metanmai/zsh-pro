// Package worktree owns shell-neutral shared-worktree policy and transforms.
package worktree

import (
	"errors"
	"fmt"
	"sort"
	"zsh-pro/core/model"
)

// ValidateSnapshot validates the complete bounded semantic input before any
// caller can observe a partial diff or projection.
func ValidateSnapshot(snapshot model.LiveSnapshot) error {
	return model.ValidateLiveSnapshot(snapshot)
}

// DiffSnapshot compares final semantic values per identity. It never compares
// framing, serialized JSON, or source command text.
func DiffSnapshot(before, after model.LiveSnapshot) ([]model.LiveChange, error) {
	if err := ValidateSnapshot(before); err != nil {
		return nil, err
	}
	if err := ValidateSnapshot(after); err != nil {
		return nil, err
	}
	beforeStates, err := model.NormalizeLiveStates(before.States)
	if err != nil {
		return nil, err
	}
	afterStates, err := model.NormalizeLiveStates(after.States)
	if err != nil {
		return nil, err
	}

	beforeByIdentity := presentStateMap(beforeStates)
	afterByIdentity := presentStateMap(afterStates)
	identities := make([]model.Identity, 0, len(beforeByIdentity)+len(afterByIdentity))
	seen := make(map[model.Identity]bool, len(beforeByIdentity)+len(afterByIdentity))
	for identity := range beforeByIdentity {
		seen[identity] = true
		identities = append(identities, identity)
	}
	for identity := range afterByIdentity {
		if !seen[identity] {
			identities = append(identities, identity)
		}
	}
	sortIdentities(identities)

	var changes []model.LiveChange
	for _, identity := range identities {
		beforeValue, beforePresent := beforeByIdentity[identity]
		afterValue, afterPresent := afterByIdentity[identity]
		switch {
		case !beforePresent && afterPresent:
			changes = append(changes, model.LiveChange{Kind: model.LiveAdd, Identity: identity, Value: model.CloneLiveValue(afterValue)})
		case beforePresent && !afterPresent:
			changes = append(changes, model.LiveChange{Kind: model.LiveRemove, Identity: identity, Value: model.RemovedLiveValue()})
		case beforePresent && afterPresent && !model.EqualLiveValue(identity.Kind, beforeValue, afterValue):
			changes = append(changes, model.LiveChange{Kind: model.LiveUpdate, Identity: identity, Value: model.CloneLiveValue(afterValue)})
		}
	}
	return changes, nil
}

func presentStateMap(states []model.LiveIdentityState) map[model.Identity]model.LiveValue {
	out := make(map[model.Identity]model.LiveValue, len(states))
	for _, state := range states {
		if state.Value.Present {
			out[state.Identity] = state.Value
		}
	}
	return out
}

// ApplyOverlay preserves the final source-order positions of existing
// identities, appends newly admitted identities in overlay order, and makes a
// final tombstone authoritative over every shadowed source occurrence.
func ApplyOverlay(base []model.LiveIdentityState, overlay []model.OverlayEntry) ([]model.LiveIdentityState, error) {
	normalizedBase, err := model.NormalizeLiveStates(base)
	if err != nil {
		return nil, err
	}
	normalizedOverlay, err := model.NormalizeOverlay(overlay)
	if err != nil {
		return nil, err
	}
	return applyNormalizedOverlay(normalizedBase, normalizedOverlay), nil
}

type overlaySlot struct {
	state  model.LiveIdentityState
	active bool
}

func applyNormalizedOverlay(base []model.LiveIdentityState, overlay []model.OverlayEntry) []model.LiveIdentityState {
	slots := make([]overlaySlot, 0, len(base)+len(overlay))
	indexes := make(map[model.Identity]int, len(base)+len(overlay))
	for _, state := range base {
		if !state.Value.Present {
			continue
		}
		indexes[state.Identity] = len(slots)
		slots = append(slots, overlaySlot{
			state:  model.LiveIdentityState{Identity: state.Identity, Value: model.CloneLiveValue(state.Value)},
			active: true,
		})
	}
	for _, entry := range overlay {
		index, exists := indexes[entry.Identity]
		if entry.Tombstone {
			if exists {
				slots[index].active = false
			}
			continue
		}
		state := model.LiveIdentityState{Identity: entry.Identity, Value: model.CloneLiveValue(entry.Value)}
		if exists {
			slots[index] = overlaySlot{state: state, active: true}
			continue
		}
		indexes[entry.Identity] = len(slots)
		slots = append(slots, overlaySlot{state: state, active: true})
	}
	out := make([]model.LiveIdentityState, 0, len(slots))
	for _, slot := range slots {
		if slot.active {
			out = append(out, slot.state)
		}
	}
	return out
}

// BuildProjection constructs a complete versioned final-state projection from
// source-ordered semantic states plus an explicit overlay.
func BuildProjection(base []model.LiveIdentityState, overlay []model.OverlayEntry) (model.LiveProjection, error) {
	normalizedBase, err := model.NormalizeLiveStates(base)
	if err != nil {
		return model.LiveProjection{}, err
	}
	normalizedOverlay, err := model.NormalizeOverlay(overlay)
	if err != nil {
		return model.LiveProjection{}, err
	}
	states := applyNormalizedOverlay(normalizedBase, normalizedOverlay)
	tombstones := make([]model.Identity, 0, len(normalizedOverlay))
	for _, entry := range normalizedOverlay {
		if entry.Tombstone {
			tombstones = append(tombstones, entry.Identity)
		}
	}
	return model.LiveProjection{
		Schema:     model.WorktreeSchemaV1,
		States:     model.CloneLiveStates(states),
		Tombstones: append([]model.Identity(nil), tombstones...),
	}, nil
}

// CategorizeDiff drops value payloads and returns stable kind/name groups for
// public status/diff rendering.
func CategorizeDiff(changes []model.LiveChange) (model.CategorizedDiff, error) {
	seen := make(map[model.Identity]bool, len(changes))
	entries := make([]model.DiffEntry, len(changes))
	for index, change := range changes {
		if err := validateLiveChange(change); err != nil {
			return model.CategorizedDiff{}, err
		}
		if seen[change.Identity] {
			return model.CategorizedDiff{}, fmt.Errorf("duplicate live change for %s/%s", change.Identity.Kind, change.Identity.Name)
		}
		seen[change.Identity] = true
		entries[index] = model.DiffEntry{Kind: change.Kind, Identity: change.Identity}
	}
	sort.Slice(entries, func(left, right int) bool {
		return identityLess(entries[left].Identity, entries[right].Identity)
	})
	var out model.CategorizedDiff
	for _, entry := range entries {
		switch entry.Identity.Kind {
		case model.LiveEnv:
			out.Environment = append(out.Environment, entry)
		case model.LiveAlias:
			out.Aliases = append(out.Aliases, entry)
		case model.LiveFunction:
			out.Functions = append(out.Functions, entry)
		case model.LivePath:
			out.Path = append(out.Path, entry)
		case model.LiveFPath:
			out.FPath = append(out.FPath, entry)
		case model.LiveOption:
			out.Options = append(out.Options, entry)
		}
	}
	return out, nil
}

func validateLiveChange(change model.LiveChange) error {
	if err := model.ValidateIdentity(change.Identity); err != nil {
		return err
	}
	switch change.Kind {
	case model.LiveAdd, model.LiveUpdate:
		if err := model.ValidateLiveValue(change.Identity.Kind, change.Value); err != nil {
			return err
		}
		if !change.Value.Present {
			return errors.New("add/update live change has no value")
		}
	case model.LiveRemove:
		if err := model.ValidateLiveValue(change.Identity.Kind, change.Value); err != nil {
			return err
		}
		if change.Value.Present {
			return errors.New("remove live change carries a value")
		}
	default:
		return fmt.Errorf("live change kind %q is unsupported", change.Kind)
	}
	return nil
}

func sortIdentities(identities []model.Identity) {
	sort.Slice(identities, func(left, right int) bool {
		return identityLess(identities[left], identities[right])
	})
}

func identityLess(left, right model.Identity) bool {
	leftRank := liveKindRank(left.Kind)
	rightRank := liveKindRank(right.Kind)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return left.Name < right.Name
}

func liveKindRank(kind model.LiveKind) int {
	switch kind {
	case model.LiveEnv:
		return 0
	case model.LiveAlias:
		return 1
	case model.LiveFunction:
		return 2
	case model.LivePath:
		return 3
	case model.LiveFPath:
		return 4
	case model.LiveOption:
		return 5
	default:
		return 6
	}
}
