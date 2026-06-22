// Package analyze is the shell-agnostic reconciler: it drives a Provider and
// merges the static (parse+classify) and dynamic (introspect) views into one
// Analysis.
//
// This package depends ONLY on core/shell (the interfaces) and core/model. It
// must never import a concrete shell implementation such as core/shell/zsh —
// that decoupling is what lets a new shell be supported by adding a Provider,
// not by touching the reconciler.
package analyze

import (
	"sort"
	"strings"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// Analyze produces a read-only Analysis. src is the raw config; path is used
// for introspection and reporting. It never panics: a failed or unavailable
// introspection degrades to a static-only analysis with an explanatory note.
func Analyze(p shell.Provider, src []byte, path string) model.Analysis {
	blocks, _ := p.Parse(src)
	a := model.Analysis{
		Path:       path,
		Lines:      strings.Count(string(src), "\n") + 1,
		BlockCount: len(blocks),
	}

	// Classify each block and bucket it by category. We mutate a local copy of
	// the slice the parser returned; nothing here is global state.
	buckets := map[model.Category][]model.Block{}
	for i := range blocks {
		cat, conf := p.Classify(blocks[i])
		blocks[i].Category = cat
		blocks[i].Conf = conf
		if blocks[i].Opaque {
			a.OpaqueBlocks++
		}
		buckets[cat] = append(buckets[cat], blocks[i])
	}

	// Per-category rollups, emitted in taxonomy (load) order; skip empties.
	for _, cat := range p.Categories() {
		bs := buckets[cat]
		if len(bs) == 0 {
			continue
		}
		items := make([]string, 0, len(bs))
		for _, b := range bs {
			items = append(items, primaryName(b))
		}
		a.Categories = append(a.Categories, model.CategorySummary{
			Category: cat, Count: len(bs), Items: items,
		})
	}
	a.HasSecrets = len(buckets[model.CatSecrets]) > 0

	// Static issue detection — every definition is visible in the parsed
	// blocks, so all four issue kinds are derived from the file itself. Crucially
	// for cross-type shadow, this means we report a name the FILE defines as both
	// an alias and a function; we never iterate the resolved IdentitySet, whose
	// inherited/default identities (e.g. `zsh -f`'s own `run-help`, inherited
	// exported env) would otherwise produce false positives.
	a.Issues = append(a.Issues, dupNameIssues(buckets[model.CatAliases], model.IssueDuplicateAlias)...)
	envBlocks := append(append([]model.Block{}, buckets[model.CatEnvironment]...), buckets[model.CatSecrets]...)
	a.Issues = append(a.Issues, dupNameIssues(envBlocks, model.IssueReassignedEnv)...)
	a.Issues = append(a.Issues, dupPathIssues(buckets[model.CatPath])...)
	a.Issues = append(a.Issues, shadowIssues(blocks)...)

	// Dynamic reconciliation (resolved end-state). The resolved view can only
	// confirm — never invent — a file-defined shadow, so it does not add issues
	// here; its role is to set Introspected. On any failure (broken/missing
	// zsh, timeout, unavailable set) we degrade to a static-only analysis with
	// an explanatory note rather than crashing.
	// v1 scope: only ids.Available is consumed here — static blocks drive all
	// issue detection so we never raise inherited-env false positives. The
	// resolved IdentitySet (aliases/functions/env/path/options) is intentionally
	// not read yet; consuming it (env-isolated introspection + opaque-init
	// identity detection) is a tracked follow-up.
	ids, err := p.Introspect(path)
	if err != nil || !ids.Available {
		a.Introspected = false
		a.Notes = append(a.Notes, "introspection unavailable — showing static analysis only")
	} else {
		a.Introspected = true
	}

	sort.SliceStable(a.Issues, func(i, j int) bool {
		if a.Issues[i].Kind != a.Issues[j].Kind {
			return a.Issues[i].Kind < a.Issues[j].Kind
		}
		return a.Issues[i].Name < a.Issues[j].Name
	})
	return a
}
