package testgen

import (
	"math/rand"
	"testing"

	"zsh-pro/core/model"
)

func TestExpectedMatchesPlantedDefects(t *testing.T) {
	g := New(rand.New(rand.NewSource(7))).Build(sampleParams())
	g.RenderZsh() // sets Node.Line
	exp := g.Expected()

	kinds := map[model.IssueKind]int{}
	for _, is := range exp.Issues {
		kinds[is.Kind]++
	}
	for _, want := range []model.IssueKind{
		model.IssueDuplicateAlias, model.IssueReassignedEnv,
		model.IssueDuplicatePath, model.IssueShadowed,
	} {
		if kinds[want] != 1 {
			t.Errorf("expected exactly 1 %s issue, got %d", want, kinds[want])
		}
	}
	if !exp.HasSecrets {
		t.Error("HasSecrets=false; sampleParams plants 1 secret")
	}
	// Categories carry counts — one per node.
	total := 0
	for _, c := range exp.Categories {
		total += c.Count
	}
	if total != len(g.Nodes) {
		t.Errorf("category counts sum to %d; want %d (one per node)", total, len(g.Nodes))
	}
}
