package testgen_test

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"zsh-pro/core/analyze"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

// checkLines gates the issue-line / total-Lines comparison. It is true now that
// LINE-01 (total Lines off-by-one) and LINE-02 (comment line mis-attribution)
// are fixed: the gated block asserts the engine's total Lines and each issue's
// line slice against the oracle across every seed, pinning both fixes.
const checkLines = true

func propParams() testgen.GenParams {
	return testgen.GenParams{
		EnvVars: 4, Aliases: 4, Functions: 3, PathEntries: 3, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

// TestOracleProperty generates a config from a seeded graph, renders it, runs
// the real engine, and asserts the engine's output matches the oracle on the
// strict subset (category counts, issue (kind,name) set, has_secrets, no opaque
// blocks). A failure here is a real analyzer bug — capture the logged source.
func TestOracleProperty(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 5, 8, 13, 21, 34, 55, 89} {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			g := testgen.New(rand.New(rand.NewSource(seed))).Build(propParams())
			src := g.RenderZsh()
			want := g.Expected()
			got := analyze.New(zsh.Provider{}).Analyze(src, fmt.Sprintf("gen-%d.zsh", seed))
			assertStrict(t, seed, want, got, src)
		})
	}
}

func assertStrict(t *testing.T, seed int64, want, got model.Analysis, src []byte) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Errorf(format+"\n--- seed %d, source ---\n%s", append(args, seed, src)...)
	}

	if got.OpaqueBlocks != 0 {
		fail("opaque_blocks = %d; want 0", got.OpaqueBlocks)
	}
	if got.HasSecrets != want.HasSecrets {
		fail("has_secrets = %v; want %v", got.HasSecrets, want.HasSecrets)
	}

	wantCat := map[model.Category]int{}
	for _, c := range want.Categories {
		wantCat[c.Category] = c.Count
	}
	gotCat := map[model.Category]int{}
	for _, c := range got.Categories {
		gotCat[c.Category] = c.Count
	}
	for cat, n := range wantCat {
		if gotCat[cat] != n {
			fail("category %q count = %d; want %d", cat, gotCat[cat], n)
		}
	}
	for cat, n := range gotCat {
		if _, ok := wantCat[cat]; !ok {
			fail("unexpected category %q (count %d)", cat, n)
		}
	}

	key := func(is model.Issue) string { return string(is.Kind) + "/" + is.Name }
	var wantKeys, gotKeys []string
	for _, is := range want.Issues {
		wantKeys = append(wantKeys, key(is))
	}
	for _, is := range got.Issues {
		gotKeys = append(gotKeys, key(is))
	}
	sort.Strings(wantKeys)
	sort.Strings(gotKeys)
	if fmt.Sprint(wantKeys) != fmt.Sprint(gotKeys) {
		fail("issue set = %v; want %v", gotKeys, wantKeys)
	}

	if checkLines {
		// Total line count (LINE-01) and per-issue statement lines (LINE-02) must
		// match the oracle. Per-issue lines are compared only for issues present
		// in both sets (the (kind,name) set equality is asserted above).
		if got.Lines != want.Lines {
			fail("Lines = %d; want %d", got.Lines, want.Lines)
		}
		wantLines := map[string][]int{}
		for _, is := range want.Issues {
			wantLines[key(is)] = is.Lines
		}
		for _, is := range got.Issues {
			k := key(is)
			if wl, ok := wantLines[k]; ok && fmt.Sprint(is.Lines) != fmt.Sprint(wl) {
				fail("issue %s lines = %v; want %v", k, is.Lines, wl)
			}
		}
	}
}
