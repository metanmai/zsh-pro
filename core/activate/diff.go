package activate

import (
	"errors"
	"fmt"
	"sort"
	"zsh-pro/core/model"
)

// BuildLivePatch derives one forward plan and its replacement reverse.
func BuildLivePatch(_, _ []model.LiveIdentityState) (LivePatch, error) {
	return LivePatch{}, errors.New("live patch construction is not implemented")
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
