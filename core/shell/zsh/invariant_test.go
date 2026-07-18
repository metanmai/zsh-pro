package zsh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReverseSyntaxHasSingleEmitHome(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	files := []string{"core/activate", "core/model", "core/shell/zsh"}
	for _, rel := range files {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if rel == "core/shell/zsh" && e.Name() != "regen.go" {
				continue
			}
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, rel, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			text := string(b)
			for _, token := range []string{"unalias ", "unset -f ", "unsetopt "} {
				if strings.Contains(text, token) {
					t.Fatalf("reverse token %q found outside emit.go in %s", token, filepath.Join(rel, e.Name()))
				}
			}
		}
	}
}
