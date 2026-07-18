package activate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Reverse shell syntax belongs exclusively to core/shell/zsh/emit.go. This
// narrow check scans non-test Go string literals and ignores Go identifiers.
func TestActivateSourceIsTokenFree(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`"[^"\\]*(?:\\.[^"\\]*)*"`)
	reverse := regexp.MustCompile(`\b(?:unalias|unset -f|unsetopt)\b`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range re.FindAllString(string(b), -1) {
			if lit == `"unsetopt"` {
				continue
			} // command-name discriminator, not emitted syntax
			if reverse.MatchString(lit) {
				t.Fatalf("reverse token in %s: %s", f, lit)
			}
		}
	}
}
