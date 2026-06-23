package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateWritesCorpus(t *testing.T) {
	dir := t.TempDir()
	if err := generate(3, 1, dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifests.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifests map[string]manifestEntry
	if err := json.Unmarshal(raw, &manifests); err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 3 {
		t.Fatalf("manifest has %d entries; want 3", len(manifests))
	}
	for name, m := range manifests {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("manifest names %q but file missing: %v", name, err)
		}
		if m.MinBlocks <= 0 {
			t.Errorf("%s: min_blocks = %d; want > 0", name, m.MinBlocks)
		}
	}
}
