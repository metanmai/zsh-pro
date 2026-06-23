// Command zsh-gen emits random generated zsh configs plus a corpus-compatible
// manifests.json, for inspection or promotion into the golden corpus.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"zsh-pro/core/testgen"
)

// manifestEntry matches the golden-corpus schema (see core/analyze/corpus_test.go).
type manifestEntry struct {
	MinBlocks        int      `json:"min_blocks"`
	IssueKinds       []string `json:"issue_kinds"`
	HasSecrets       bool     `json:"has_secrets"`
	ExpectCategories []string `json:"expect_categories"`
}

// genParams is the per-case generation profile: a few of each node type plus one
// of each defect kind, so every emitted case exercises all four issue kinds.
func genParams() testgen.GenParams {
	return testgen.GenParams{
		EnvVars: 4, Aliases: 4, Functions: 3, PathEntries: 3, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

// generate writes n cases (gen_<i>.zsh) plus manifests.json into outDir, each
// case seeded from seed+i so the output is reproducible.
func generate(n int, seed int64, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	manifests := map[string]manifestEntry{}
	for i := range n {
		g := testgen.New(rand.New(rand.NewSource(seed + int64(i)))).Build(genParams())
		src := g.RenderZsh()
		exp := g.Expected()

		name := fmt.Sprintf("gen_%d.zsh", i)
		if err := os.WriteFile(filepath.Join(outDir, name), src, 0o644); err != nil {
			return err
		}

		kinds := []string{}
		seen := map[string]bool{}
		for _, is := range exp.Issues {
			if k := string(is.Kind); !seen[k] {
				seen[k] = true
				kinds = append(kinds, k)
			}
		}
		cats := []string{}
		for _, c := range exp.Categories {
			cats = append(cats, string(c.Category))
		}
		manifests[name] = manifestEntry{
			MinBlocks:        len(g.Nodes),
			IssueKinds:       kinds,
			HasSecrets:       exp.HasSecrets,
			ExpectCategories: cats,
		}
	}
	raw, err := json.MarshalIndent(manifests, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "manifests.json"), raw, 0o644)
}

func main() {
	n := flag.Int("n", 5, "number of cases to generate")
	seed := flag.Int64("seed", 1, "base PRNG seed")
	out := flag.String("out", "", "output directory (required)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: zsh-gen -out DIR [-n N] [-seed S]")
		os.Exit(2)
	}
	if err := generate(*n, *seed, *out); err != nil {
		fmt.Fprintf(os.Stderr, "zsh-gen: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote %d cases + manifests.json to %s\n", *n, *out)
}
