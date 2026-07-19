package zsh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReverseSyntaxHasSingleEmitHome(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, rel := range []string{"core/activate", "core/model", "core/store", "core/shell"} {
		err := filepath.WalkDir(filepath.Join(root, rel), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Clean(path) == filepath.Join(root, "core/shell/zsh/emit.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, token := range []string{"unalias ", "unset -f ", "unsetopt "} {
				if strings.Contains(string(b), token) {
					t.Errorf("reverse token %q found outside emit.go in %s", token, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
