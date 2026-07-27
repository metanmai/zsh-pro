package ir

import (
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// Regenerate emits a Profile back to .zsh source in stored (source) order
// (D-09) — never regrouped by category. For each entry: a managed entry
// (EffectiveManaged, after any Override — D-07) is rebuilt via the injected
// shell.Regenerator; everything else (imperative / Opaque / forced-unmanaged)
// is emitted verbatim from its Text (D-01/D-10).
//
// This file generates NO zsh syntax itself — all forward codegen is delegated
// to r (the milestone invariant pins zsh syntax to core/shell/zsh; Pitfall 5).
// The Regenerator is total (its default case returns verbatim Text), so the
// gate here also rejects declaration forms whose shell attributes or scope are
// not represented by Entry, preserving their original source verbatim.
//
// Dynamic declarative values survive verbatim because the Regenerator emits
// the captured Value unchanged (D-05) — no resolution happens anywhere here.
func Regenerate(p model.Profile, r shell.Regenerator) []byte {
	var out []byte
	for i := range p.Entries {
		e := p.Entries[i]
		var line string
		if e.EffectiveManaged() && e.DeclarationRepresentable() {
			line = r.Regenerate(e)
		} else {
			line = e.Text
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}
