package analyze

import (
	"regexp"
	"sort"

	"zsh-pro/core/model"
)

// dupNameIssues reports any name defined on more than one line within the given
// blocks. Used for duplicate aliases and reassigned env vars; the Lines slice
// is sorted so the report is deterministic.
func dupNameIssues(blocks []model.Block, kind model.IssueKind) []model.Issue {
	lines := map[string][]int{}
	order := []string{}
	for _, b := range blocks {
		for _, n := range b.Names {
			if _, seen := lines[n]; !seen {
				order = append(order, n)
			}
			lines[n] = append(lines[n], b.StartLine)
		}
	}
	var out []model.Issue
	for _, name := range order {
		ls := lines[name]
		if len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: kind, Name: name, Lines: ls, Note: "last definition wins"})
		}
	}
	return out
}

// pathSegRe matches absolute-, $HOME- or ~-rooted path segments inside a PATH
// manipulation so duplicate entries can be spotted across lines.
var pathSegRe = regexp.MustCompile(`(?:\$HOME|~|/)[^:"'\s)]+`)

// dupPathIssues reports a path segment that is added on more than one line.
func dupPathIssues(blocks []model.Block) []model.Issue {
	lines := map[string][]int{}
	order := []string{}
	for _, b := range blocks {
		for _, seg := range pathSegRe.FindAllString(b.Text, -1) {
			if _, seen := lines[seg]; !seen {
				order = append(order, seg)
			}
			lines[seg] = append(lines[seg], b.StartLine)
		}
	}
	var out []model.Issue
	for _, seg := range order {
		ls := lines[seg]
		if len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: model.IssueDuplicatePath, Name: seg, Lines: ls})
		}
	}
	return out
}

// shadowIssues reports a cross-type shadow: a name the FILE defines as BOTH an
// alias and a function.
//
// This intentionally operates on the classified static blocks rather than on
// the resolved IdentitySet. The IdentitySet from Task 5's introspection
// reflects the END-STATE under `zsh -f` and therefore contains inherited
// exported env vars and zsh's own default aliases/functions (e.g. run-help)
// that the user's file never defined. Iterating it — as an earlier draft did —
// would emit shadow issues for those inherited identities, which are false
// positives. Scoping to file-defined names guarantees we only flag a shadow the
// user actually wrote. The resolved set, when available, can confirm but never
// invent such a shadow, so it is not consulted here.
func shadowIssues(blocks []model.Block) []model.Issue {
	aliasLines := map[string]int{}
	funcLines := map[string]int{}
	for _, b := range blocks {
		switch b.Kind {
		case model.KindAlias:
			for _, n := range b.Names {
				if _, seen := aliasLines[n]; !seen {
					aliasLines[n] = b.StartLine
				}
			}
		case model.KindFuncDecl:
			for _, n := range b.Names {
				if _, seen := funcLines[n]; !seen {
					funcLines[n] = b.StartLine
				}
			}
		}
	}

	// Iterate the file's block order (via a stable name list) so output is
	// deterministic regardless of map iteration order.
	var names []string
	seen := map[string]bool{}
	for _, b := range blocks {
		for _, n := range b.Names {
			if _, isAlias := aliasLines[n]; isAlias {
				if _, isFunc := funcLines[n]; isFunc && !seen[n] {
					seen[n] = true
					names = append(names, n)
				}
			}
		}
	}

	var out []model.Issue
	for _, n := range names {
		ls := []int{aliasLines[n], funcLines[n]}
		sort.Ints(ls)
		out = append(out, model.Issue{
			Kind:  model.IssueShadowed,
			Name:  n,
			Lines: ls,
			Note:  "defined as both an alias and a function",
		})
	}
	return out
}
