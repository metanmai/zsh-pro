package testgen

import (
	"sort"

	"zsh-pro/core/model"
)

// Expected derives the known-correct analysis from the graph. It requires
// RenderZsh to have run (it reads Node.Line). The line numbers it records are
// the CORRECT statement lines; the engine may differ on a first occurrence that
// follows a comment (a known bug), so the property test compares issues by
// (kind, name) and gates the line comparison.
//
// Grouping mirrors the engine: reassigned_env spans env+secret nodes; a path
// duplicate keys on the rooted directory; a shadow is a name defined as both an
// alias and a function.
func (g *ConfigGraph) Expected() model.Analysis {
	var a model.Analysis
	counts := map[model.Category]int{}
	for _, n := range g.Nodes {
		counts[n.Cat]++
		if n.Kind == NodeSecret {
			a.HasSecrets = true
		}
	}
	for _, cat := range model.Categories() {
		if c := counts[cat]; c > 0 {
			a.Categories = append(a.Categories, model.CategorySummary{Category: cat, Count: c})
		}
	}

	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodeAlias}, model.IssueDuplicateAlias)...)
	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodeEnvVar, NodeSecret}, model.IssueReassignedEnv)...)
	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodePathEntry}, model.IssueDuplicatePath)...)
	a.Issues = append(a.Issues, g.shadowIssues()...)

	sort.SliceStable(a.Issues, func(i, j int) bool {
		if a.Issues[i].Kind != a.Issues[j].Kind {
			return a.Issues[i].Kind < a.Issues[j].Kind
		}
		return a.Issues[i].Name < a.Issues[j].Name
	})
	return a
}

// dupNameIssues reports a name appearing on more than one node of the given kinds.
func (g *ConfigGraph) dupNameIssues(kinds []NodeKind, kind model.IssueKind) []model.Issue {
	want := map[NodeKind]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	lines := map[string][]int{}
	order := []string{}
	for _, n := range g.Nodes {
		if !want[n.Kind] {
			continue
		}
		if _, seen := lines[n.Name]; !seen {
			order = append(order, n.Name)
		}
		lines[n.Name] = append(lines[n.Name], n.Line)
	}
	var out []model.Issue
	for _, name := range order {
		if ls := lines[name]; len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: kind, Name: name, Lines: ls})
		}
	}
	return out
}

// shadowIssues reports a name defined as BOTH an alias and a function.
func (g *ConfigGraph) shadowIssues() []model.Issue {
	aliasLine := map[string]int{}
	funcLine := map[string]int{}
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeAlias:
			if _, ok := aliasLine[n.Name]; !ok {
				aliasLine[n.Name] = n.Line
			}
		case NodeFunction:
			if _, ok := funcLine[n.Name]; !ok {
				funcLine[n.Name] = n.Line
			}
		}
	}
	var out []model.Issue
	for name, al := range aliasLine {
		if fl, ok := funcLine[name]; ok {
			ls := []int{al, fl}
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: model.IssueShadowed, Name: name, Lines: ls})
		}
	}
	return out
}
