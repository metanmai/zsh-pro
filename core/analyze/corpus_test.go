package analyze_test

// This is the Tier-1 golden corpus runner. It lives in the EXTERNAL test
// package (analyze_test) on purpose: it is the only place that wires the
// concrete zsh.Provider to the agnostic analyze.Analyze reconciler, so the
// production analyze package stays shell-agnostic. The harness is data-driven —
// adding a fixture is "drop a .zsh file in core/testdata/fixtures + add one
// entry to manifests.json"; no test code changes.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"zsh-pro/core/analyze"
	"zsh-pro/core/shell/zsh"
)

// manifest is the ground-truth expectation for one fixture.
type manifest struct {
	MinBlocks        int      `json:"min_blocks"`
	IssueKinds       []string `json:"issue_kinds"`
	HasSecrets       bool     `json:"has_secrets"`
	ExpectCategories []string `json:"expect_categories"`
}

func TestCorpusGolden(t *testing.T) {
	root := filepath.Join("..", "testdata", "fixtures")
	raw, err := os.ReadFile(filepath.Join(root, "manifests.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifests map[string]manifest
	if err := json.Unmarshal(raw, &manifests); err != nil {
		t.Fatal(err)
	}

	for name, m := range manifests {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			a := analyze.Analyze(zsh.Provider{}, src, filepath.Join(root, name))

			if a.BlockCount < m.MinBlocks {
				t.Errorf("blocks = %d, want >= %d", a.BlockCount, m.MinBlocks)
			}
			if a.HasSecrets != m.HasSecrets {
				t.Errorf("has_secrets = %v, want %v", a.HasSecrets, m.HasSecrets)
			}
			if a.OpaqueBlocks > 0 {
				t.Errorf("unexpected opaque blocks: %d (parser failed to understand real-world input)", a.OpaqueBlocks)
			}

			gotKinds := map[string]bool{}
			for _, is := range a.Issues {
				gotKinds[string(is.Kind)] = true
			}
			for _, want := range m.IssueKinds {
				if !gotKinds[want] {
					t.Errorf("missing expected issue kind %q (got issues %+v)", want, a.Issues)
				}
			}
			// No issue kinds beyond those declared in the manifest.
			declared := map[string]bool{}
			for _, k := range m.IssueKinds {
				declared[k] = true
			}
			for k := range gotKinds {
				if !declared[k] {
					t.Errorf("unexpected issue kind %q", k)
				}
			}

			gotCats := map[string]bool{}
			for _, c := range a.Categories {
				gotCats[string(c.Category)] = true
			}
			for _, want := range m.ExpectCategories {
				if !gotCats[want] {
					t.Errorf("missing expected category %q (got %v)", want, gotCats)
				}
			}
		})
	}
}
