package testgen

import (
	"math/rand"
	"reflect"
	"testing"
)

func sampleParams() GenParams {
	return GenParams{
		EnvVars: 3, Aliases: 3, Functions: 2, PathEntries: 2, Commands: 2,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	g1 := New(rand.New(rand.NewSource(42))).Build(sampleParams())
	g2 := New(rand.New(rand.NewSource(42))).Build(sampleParams())
	if !reflect.DeepEqual(g1, g2) {
		t.Fatal("same seed produced different graphs")
	}
}

func TestBuildPlantsDefects(t *testing.T) {
	g := New(rand.New(rand.NewSource(7))).Build(sampleParams())

	aliasCount := map[string]int{}
	envCount := map[string]int{}
	pathCount := map[string]int{}
	isAlias := map[string]bool{}
	isFunc := map[string]bool{}
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeAlias:
			aliasCount[n.Name]++
			isAlias[n.Name] = true
		case NodeEnvVar, NodeSecret:
			envCount[n.Name]++
		case NodePathEntry:
			pathCount[n.Name]++
		case NodeFunction:
			isFunc[n.Name] = true
		}
	}
	dups := func(m map[string]int) int {
		c := 0
		for _, v := range m {
			if v > 1 {
				c++
			}
		}
		return c
	}
	if dups(aliasCount) != 1 {
		t.Errorf("duplicate-alias pairs = %d; want 1", dups(aliasCount))
	}
	if dups(envCount) != 1 {
		t.Errorf("reassigned-env pairs = %d; want 1", dups(envCount))
	}
	if dups(pathCount) != 1 {
		t.Errorf("duplicate-path pairs = %d; want 1", dups(pathCount))
	}
	shadows := 0
	for name := range isAlias {
		if isFunc[name] {
			shadows++
		}
	}
	if shadows != 1 {
		t.Errorf("shadows = %d; want 1", shadows)
	}
}
