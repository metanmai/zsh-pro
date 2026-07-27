package ir

import (
	"testing"

	"zsh-pro/core/model"
)

// TestRouteManagedArrayImperative pins the UAT array gap fix at the routing gate:
// an array assignment (Block.Array set) is NOT scalar-templatable (a.Array, no
// a.Value), so routeManaged must reject it (return false -> imperative) in EVERY
// admitted category. The scalar single-name plain-`=` path stays managed, and the
// existing multi-name (BL-02) / Append (WR-01) guards must remain intact — proving
// the array guard was added without weakening the others. Same remedy lineage as
// BL-02 / WR-01 / WR-02 (D-04 / D-06: manage only faithfully-reproducible shapes).
func TestRouteManagedArrayImperative(t *testing.T) {
	cases := []struct {
		name string
		b    model.Block
		cat  model.Category
		want bool
	}{
		// Array assignment is never managed, in any admitted category.
		{
			"array in CatEnvironment routes imperative",
			model.Block{Kind: model.KindAssignment, Names: []string{"plugins"}, Array: true},
			model.CatEnvironment, false,
		},
		{
			"array in CatPath routes imperative",
			model.Block{Kind: model.KindAssignment, Names: []string{"fpath"}, Array: true},
			model.CatPath, false,
		},
		{
			"array in CatSecrets routes imperative",
			model.Block{Kind: model.KindAssignment, Names: []string{"keys"}, Array: true},
			model.CatSecrets, false,
		},
		// Regression: the scalar managed path is unaffected.
		{
			"scalar single-name env still managed",
			model.Block{Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim"},
			model.CatEnvironment, true,
		},
		// Regression: the existing guards still reject their shapes.
		{
			"multi-name still imperative (BL-02)",
			model.Block{Kind: model.KindAssignment, Names: []string{"A", "B"}, Value: "x"},
			model.CatEnvironment, false,
		},
		{
			"append still imperative (WR-01)",
			model.Block{Kind: model.KindAssignment, Names: []string{"PATH"}, Value: "/x", Append: true},
			model.CatPath, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := routeManaged(c.b, c.cat); got != c.want {
				t.Errorf("routeManaged() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRouteManagedRejectsUnmodeledDeclarationCommands(t *testing.T) {
	for _, cmd := range []string{"typeset", "declare", "local", "readonly"} {
		t.Run(cmd, func(t *testing.T) {
			block := model.Block{Kind: model.KindAssignment, CmdName: cmd, Names: []string{"COUNT"}, Value: "2"}
			if routeManaged(block, model.CatEnvironment) {
				t.Fatalf("%s declaration was admitted as a managed scalar", cmd)
			}
		})
	}
}
