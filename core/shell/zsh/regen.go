package zsh

import (
	"fmt"
	"strings"

	"zsh-pro/core/model"
)

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
