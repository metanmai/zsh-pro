package activate

import (
	"fmt"
	"sort"
	"zsh-pro/core/model"
)

// Diff derives full deactivate operations from active, then full activate
// operations from target. Shadow restores derive from Added, not Shadowed.
func Diff(active, target *model.Manifest) (Plan, error) {
	if active != nil && active.Schema != model.SchemaV1 {
		return Plan{}, fmt.Errorf("active manifest schema %q unsupported", active.Schema)
	}
	if target != nil && target.Schema != model.SchemaV1 {
		return Plan{}, fmt.Errorf("target manifest schema %q unsupported", target.Schema)
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
func deactivate(m model.Manifest) []Op {
	var out []Op
	for _, s := range m.Env {
		if s.Original != nil {
			out = append(out, RestoreScalar{Name: s.Name, Applied: s.Applied, Original: s.Original})
		} else {
			out = append(out, UnsetScalar{Name: s.Name, Applied: s.Applied})
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
		out = append(out, SetScalar{Name: s.Name, Applied: s.Applied, Dynamic: containsDynamic(s.Applied)})
	}
	for _, l := range m.Lists {
		out = append(out, ApplyListDelta{Name: l.Name, Additions: l.Additions, Deletions: l.Deletions})
	}
	keys := make([]string, 0, len(m.Aliases.Added))
	for k := range m.Aliases.Added {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m.Aliases.Added[k]
		out = append(out, AddAlias{Name: k, Body: v, Dynamic: containsDynamic(v)})
	}
	for _, k := range m.Functions.Added {
		out = append(out, AddFunc{Name: k})
	}
	for _, o := range m.Options {
		out = append(out, SetOption{Name: o.Name, Enabled: o.Enabled})
	}
	return out
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
