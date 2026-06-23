package testgen_test

import (
	"math/rand"
	"strings"
	"testing"

	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

func TestRenderParsesCleanly(t *testing.T) {
	g := testgen.New(rand.New(rand.NewSource(99))).Build(testgen.GenParams{
		EnvVars: 3, Aliases: 3, Functions: 2, PathEntries: 2, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	})
	src := g.RenderZsh()

	// Every node got a line, and that line in the output starts the statement.
	lines := strings.Split(string(src), "\n")
	for _, n := range g.Nodes {
		if n.Line < 1 || n.Line > len(lines) {
			t.Fatalf("node %q has bad Line %d", n.Name, n.Line)
		}
	}

	// The real parser must understand every block — no opaque fallback.
	blocks, _ := zsh.Provider{}.Parse(src)
	opaque := 0
	for _, b := range blocks {
		if b.Opaque {
			opaque++
		}
	}
	if opaque != 0 {
		t.Errorf("rendered config produced %d opaque blocks; generator must emit valid zsh\n%s", opaque, src)
	}
}
