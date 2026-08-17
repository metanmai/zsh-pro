package activate

import (
	"fmt"
	"sort"
	"zsh-pro/core/model"
)

// BuildLivePatch derives one forward plan and its replacement reverse.
func BuildLivePatch(before, after []model.LiveIdentityState) (LivePatch, error) {
	beforeNormalized, err := model.NormalizeLiveStates(before)
	if err != nil {
		return LivePatch{}, err
	}
	afterNormalized, err := model.NormalizeLiveStates(after)
	if err != nil {
		return LivePatch{}, err
	}
	forwardChanges, err := diffLiveStates(beforeNormalized, afterNormalized)
	if err != nil {
		return LivePatch{}, err
	}
	reverseChanges, err := diffLiveStates(afterNormalized, beforeNormalized)
	if err != nil {
		return LivePatch{}, err
	}
	forward, err := liveChangesToOps(forwardChanges, beforeNormalized, afterNormalized)
	if err != nil {
		return LivePatch{}, err
	}
	replacementReverse, err := liveChangesToOps(reverseChanges, afterNormalized, beforeNormalized)
	if err != nil {
		return LivePatch{}, err
	}
	return LivePatch{Forward: forward, ReplacementReverse: replacementReverse}, nil
}

// diffLiveStates is intentionally a thin consumer of core/model's sole
// normalization/equality authority. core/activate cannot import core/worktree
// because shell.Provider already closes the worktree -> shell -> activate
// dependency path; external golden tests pin this traversal to DiffSnapshot.
func diffLiveStates(before, after []model.LiveIdentityState) ([]model.LiveChange, error) {
	beforeValues := liveStateValues(before)
	afterValues := liveStateValues(after)
	identities := make([]model.Identity, 0, len(beforeValues)+len(afterValues))
	seen := make(map[model.Identity]bool, len(beforeValues)+len(afterValues))
	for identity := range beforeValues {
		seen[identity] = true
		identities = append(identities, identity)
	}
	for identity := range afterValues {
		if !seen[identity] {
			identities = append(identities, identity)
		}
	}
	sort.Slice(identities, func(left, right int) bool {
		leftRank := liveIdentityRank(identities[left].Kind)
		rightRank := liveIdentityRank(identities[right].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return identities[left].Name < identities[right].Name
	})
	changes := make([]model.LiveChange, 0, len(identities))
	for _, identity := range identities {
		beforeValue, beforePresent := beforeValues[identity]
		afterValue, afterPresent := afterValues[identity]
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

func liveIdentityRank(kind model.LiveKind) int {
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

func liveChangesToOps(changes []model.LiveChange, before, after []model.LiveIdentityState) ([]Op, error) {
	beforeValues := liveStateValues(before)
	afterValues := liveStateValues(after)
	operations := make([]Op, 0, len(changes))
	for _, change := range changes {
		switch change.Identity.Kind {
		case model.LiveEnv, model.LiveAlias, model.LiveFunction:
			if change.Kind == model.LiveRemove {
				operations = append(operations, RemoveLiveScalar{Identity: change.Identity})
				continue
			}
			if change.Value.Scalar == nil {
				return nil, fmt.Errorf("live scalar %s/%s has no value", change.Identity.Kind, change.Identity.Name)
			}
			operations = append(operations, SetLiveScalar{Identity: change.Identity, Value: *change.Value.Scalar})
		case model.LivePath, model.LiveFPath:
			beforeValue, beforePresent := beforeValues[change.Identity]
			afterValue, afterPresent := afterValues[change.Identity]
			transition := TransitionLiveList{
				Identity:      change.Identity,
				BeforePresent: beforePresent,
				AfterPresent:  afterPresent,
			}
			if beforePresent {
				transition.Before = append([]string(nil), beforeValue.List...)
			}
			if afterPresent {
				transition.After = append([]string(nil), afterValue.List...)
			}
			operations = append(operations, transition)
		case model.LiveOption:
			if change.Kind == model.LiveRemove {
				operations = append(operations, RemoveLiveOptionState{Identity: change.Identity})
				continue
			}
			if change.Value.Option == nil {
				return nil, fmt.Errorf("live option %s has no value", change.Identity.Name)
			}
			operations = append(operations, SetLiveOptionState{Identity: change.Identity, Enabled: *change.Value.Option})
		default:
			return nil, fmt.Errorf("live identity kind %q is unsupported", change.Identity.Kind)
		}
	}
	return operations, nil
}

func liveStateValues(states []model.LiveIdentityState) map[model.Identity]model.LiveValue {
	values := make(map[model.Identity]model.LiveValue, len(states))
	for _, state := range states {
		if state.Value.Present {
			values[state.Identity] = model.CloneLiveValue(state.Value)
		}
	}
	return values
}

// Diff derives full deactivate operations from active, then full activate
// operations from target. Shadow restores derive from Added, not Shadowed.
func Diff(active, target *model.Manifest) (Plan, error) {
	if active != nil && active.Schema != model.SchemaV1 {
		return Plan{}, fmt.Errorf("active manifest schema %q unsupported", active.Schema)
	}
	if target != nil && target.Schema != model.SchemaV1 {
		return Plan{}, fmt.Errorf("target manifest schema %q unsupported", target.Schema)
	}
	if active != nil {
		normalized := normalizeManifest(*active)
		active = &normalized
	}
	if target != nil {
		normalized := normalizeManifest(*target)
		target = &normalized
	}
	if target != nil {
		for _, list := range target.Lists {
			if err := validateListDelta(list); err != nil {
				return Plan{}, err
			}
		}
		for _, name := range target.Functions.Added {
			if _, ok := target.Functions.Bodies[name]; !ok {
				return Plan{}, fmt.Errorf("target function %q has no activation body", name)
			}
		}
	}
	var p Plan
	if active != nil {
		p.Deactivate = deactivate(*active)
	}
	if target != nil {
		p.Activate = activate(*target)
	}
	return p, nil
}

func validateListDelta(list model.ListDelta) error {
	if list.BaseIndex == nil {
		if len(list.AdditionDynamic) != 0 {
			return fmt.Errorf("target list %q has dynamic metadata without base metadata", list.Name)
		}
		return nil
	}
	if *list.BaseIndex < 0 || *list.BaseIndex > len(list.Additions) {
		return fmt.Errorf("target list %q has invalid base index", list.Name)
	}
	if len(list.AdditionDynamic) != len(list.Additions) {
		return fmt.Errorf("target list %q has invalid dynamic metadata", list.Name)
	}
	return nil
}

// normalizeManifest preserves the final source occurrence of every ordered
// identity. It also protects Diff from legacy or hand-built manifests that did
// not pass through Build before generating ownership-changing operations.
func normalizeManifest(in model.Manifest) model.Manifest {
	out := in
	out.Env = normalizeScalars(in.Env)
	out.Functions.Added = normalizeNames(in.Functions.Added)
	out.Options = normalizeOptions(in.Options)
	return out
}

func normalizeScalars(in []model.Scalar) []model.Scalar {
	last := map[string]int{}
	exported := map[string]bool{}
	explicit := map[string]bool{}
	for i, scalar := range in {
		last[scalar.Name] = i
		if scalar.Exported != nil {
			explicit[scalar.Name] = true
			exported[scalar.Name] = exported[scalar.Name] || *scalar.Exported
		}
	}
	out := make([]model.Scalar, 0, len(last))
	for i, scalar := range in {
		if last[scalar.Name] != i {
			continue
		}
		if explicit[scalar.Name] {
			value := exported[scalar.Name]
			scalar.Exported = &value
		}
		out = append(out, scalar)
	}
	return out
}

func normalizeNames(in []string) []string {
	last := map[string]int{}
	for i, name := range in {
		last[name] = i
	}
	out := make([]string, 0, len(last))
	for i, name := range in {
		if last[name] == i {
			out = append(out, name)
		}
	}
	return out
}

func normalizeOptions(in []model.OptionSet) []model.OptionSet {
	last := map[string]int{}
	for i, option := range in {
		last[option.Name] = i
	}
	out := make([]model.OptionSet, 0, len(last))
	for i, option := range in {
		if last[option.Name] == i {
			out = append(out, option)
		}
	}
	return out
}
func deactivate(m model.Manifest) []Op {
	var out []Op
	for _, s := range m.Env {
		dynamic := scalarDynamic(s)
		if s.Original != nil {
			out = append(out, RestoreScalar{Name: s.Name, Applied: s.Applied, Dynamic: dynamic, Original: s.Original})
		} else {
			out = append(out, UnsetScalar{Name: s.Name, Applied: s.Applied, Dynamic: dynamic})
		}
	}
	for _, l := range m.Lists {
		out = append(out, RebuildListFromBase{Name: l.Name})
	}
	keys := make([]string, 0, len(m.Aliases.Added))
	for k := range m.Aliases.Added {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, Unalias{Name: k}, RestoreShadowedAlias{Name: k})
	}
	for _, k := range m.Functions.Added {
		out = append(out, UnsetFunc{Name: k}, RestoreShadowedFunc{Name: k})
	}
	for _, o := range m.Options {
		out = append(out, RestoreOption{Name: o.Name})
	}
	return out
}
func activate(m model.Manifest) []Op {
	var out []Op
	for _, s := range m.Env {
		dynamic := scalarDynamic(s)
		// A missing field is a legacy manifest, which historically used export
		// assignment for scalar activation.
		exported := true
		if s.Exported != nil {
			exported = *s.Exported
		}
		out = append(out, SetScalar{Name: s.Name, Applied: s.Applied, Dynamic: dynamic, Exported: exported})
	}
	for _, l := range m.Lists {
		out = append(out, ApplyListDelta{Name: l.Name, Additions: l.Additions, Deletions: l.Deletions, BaseIndex: l.BaseIndex, AdditionDynamic: l.AdditionDynamic})
	}
	keys := make([]string, 0, len(m.Aliases.Added))
	for k := range m.Aliases.Added {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m.Aliases.Added[k]
		dynamic, ok := m.Aliases.Dynamic[k]
		if !ok {
			dynamic = containsDynamic(v)
		}
		out = append(out, AddAlias{Name: k, Body: v, Dynamic: dynamic})
	}
	for _, k := range m.Functions.Added {
		out = append(out, AddFunc{Name: k, Body: m.Functions.Bodies[k]})
	}
	for _, o := range m.Options {
		out = append(out, SetOption{Name: o.Name, Enabled: o.Enabled})
	}
	return out
}

func scalarDynamic(s model.Scalar) bool {
	dynamic := containsDynamic(s.Applied)
	if s.Dynamic != nil {
		dynamic = *s.Dynamic
	}
	return dynamic
}

func containsDynamic(s string) bool {
	for _, x := range []string{"$", "`"} {
		for i := 0; i < len(s); i++ {
			if s[i] == x[0] {
				return true
			}
		}
	}
	return false
}
