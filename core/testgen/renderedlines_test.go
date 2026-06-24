package testgen

import (
	"bytes"
	"math/rand"
	"testing"
)

// editorLines counts lines the way an editor (and the engine's countLines) does:
// empty -> 0; otherwise newline count plus 1 only when the last byte is not '\n'.
// It is derived purely from the rendered bytes, so using it to check
// RenderedLines proves the side-effect field is a faithful render total without
// re-using the engine's implementation.
func editorLines(src []byte) int {
	if len(src) == 0 {
		return 0
	}
	n := bytes.Count(src, []byte{'\n'})
	if src[len(src)-1] != '\n' {
		n++
	}
	return n
}

// TestRenderedLinesMatchesSource pins PIN-01's non-circular total: after
// RenderZsh, ConfigGraph.RenderedLines equals the editor-style line count of the
// rendered bytes, and Expected() copies that into Analysis.Lines.
func TestRenderedLinesMatchesSource(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 89} {
		g := New(rand.New(rand.NewSource(seed))).Build(sampleParams())
		src := g.RenderZsh()
		want := editorLines(src)
		if g.RenderedLines != want {
			t.Fatalf("seed %d: RenderedLines = %d; want %d (editor count of rendered source)", seed, g.RenderedLines, want)
		}
		exp := g.Expected()
		if exp.Lines != g.RenderedLines {
			t.Fatalf("seed %d: Expected().Lines = %d; want RenderedLines %d", seed, exp.Lines, g.RenderedLines)
		}
	}
}

// TestDupAliasAndDupEnvCarryLeadingComment pins PIN-01's LINE-02 coverage: the
// first node of every planted dupAlias and dupEnv pair must carry a leading
// comment so the rendered source places a `# comment` line above the first
// occurrence, exercising LINE-02 for duplicate_alias and reassigned_env (not
// only shadowed). The second node of each pair must stay comment-free.
func TestDupAliasAndDupEnvCarryLeadingComment(t *testing.T) {
	g := New(rand.New(rand.NewSource(7))).Build(sampleParams())

	firstSeen := map[NodeKind]map[string]bool{NodeAlias: {}, NodeEnvVar: {}}
	dupSeen := map[NodeKind]map[string]int{NodeAlias: {}, NodeEnvVar: {}}
	for _, n := range g.Nodes {
		if n.Kind == NodeAlias || n.Kind == NodeEnvVar {
			dupSeen[n.Kind][n.Name]++
		}
	}
	// A planted pair is a name that appears twice for that kind. Its first
	// occurrence must have a comment; its second must not.
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeAlias, NodeEnvVar:
			if dupSeen[n.Kind][n.Name] < 2 {
				continue // not a planted defect pair
			}
			if !firstSeen[n.Kind][n.Name] {
				firstSeen[n.Kind][n.Name] = true
				if n.Comment == "" {
					t.Errorf("first %v node %q has no leading comment; LINE-02 not exercised", n.Kind, n.Name)
				}
			} else if n.Comment != "" {
				t.Errorf("second %v node %q unexpectedly has comment %q", n.Kind, n.Name, n.Comment)
			}
		}
	}
	if len(firstSeen[NodeAlias]) != 1 {
		t.Errorf("dupAlias pairs seen = %d; want 1", len(firstSeen[NodeAlias]))
	}
	if len(firstSeen[NodeEnvVar]) != 1 {
		t.Errorf("dupEnv pairs seen = %d; want 1", len(firstSeen[NodeEnvVar]))
	}
}
