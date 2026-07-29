package zsh

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReverseSyntaxHasSingleEmitHome(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	forbidden := []string{
		"unalias ", "unset -f ", "unsetopt ",
		"zp_capture_scalar", "zp_restore_scalar",
		"ZP_ORIGINAL_SCALAR_", "ZP_PRESENT_SCALAR_", "ZP_EXPORTED_SCALAR_", "ZP_APPLIED_SCALAR_",
	}
	loaderPath := filepath.Join(root, "core", "shell", "zsh", "hook.go")
	for _, rel := range []string{"core/activate", "core/model", "core/store", "core/shell"} {
		err := filepath.WalkDir(filepath.Join(root, rel), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Clean(path) == filepath.Join(root, "core/shell/zsh/emit.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Errorf("unquote %s: %v", path, err)
					return true
				}
				for _, fragment := range forbidden {
					// The loader owns private scalar support and transient generated
					// function cleanup. Declarative profile reversal remains emitted
					// only by emit.go, so the loader must not gain the other tokens.
					if filepath.Clean(path) == loaderPath && (fragment == "unset -f " || fragment == "zp_capture_scalar" || fragment == "zp_restore_scalar") {
						continue
					}
					if strings.Contains(value, fragment) {
						relPath, _ := filepath.Rel(root, path)
						t.Errorf("reverse token %q found outside emit.go in %s", fragment, relPath)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
