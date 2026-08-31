package activate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Reverse shell syntax belongs exclusively to core/shell/zsh/emit.go. This
// narrow check scans non-test Go string literals and ignores Go identifiers.
func TestActivateSourceIsTokenFree(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	reverse := regexp.MustCompile(`\b(?:unalias|unset -f|unsetopt)\b`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote string literal in %s: %v", f, err)
			}
			if value != "unsetopt" && reverse.MatchString(value) {
				t.Fatalf("reverse token in %s: %s", f, lit.Value)
			}
			return true
		})
	}
}
