package testgen

import (
	"testing"

	"zsh-pro/core/model"
)

func TestGraphAddAndDepend(t *testing.T) {
	var g ConfigGraph
	a := g.Add(&Node{Kind: NodeEnvVar, Name: "EDITOR", Value: "vim", Cat: model.CatEnvironment})
	b := g.Add(&Node{Kind: NodeAlias, Name: "ll", Value: "ls -la", Cat: model.CatAliases})
	if a != 0 || b != 1 {
		t.Fatalf("Add returned indices %d,%d; want 0,1", a, b)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("len(Nodes)=%d; want 2", len(g.Nodes))
	}
	g.DependOn(b, a) // alias depends on the env var (must follow it)
	if got := g.Nodes[b].DependsOn; len(got) != 1 || got[0] != a {
		t.Fatalf("DependsOn=%v; want [%d]", got, a)
	}
}
