package zsh

import (
	"strings"
	"testing"

	"zsh-pro/core/model"
)

// TestParseDynamicFlag pins EVAL-01: the parser tags a value dynamic iff its
// word contains a non-literal AST part ($HOME / $(...) / `...` / $((...))),
// performing NO execution — a $HOME value is stored verbatim, never resolved.
func TestParseDynamicFlag(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantDynamic bool
	}{
		{"param-expansion", `export GOPATH=$HOME/go`, true},
		{"command-subst", `export BREW=$(brew --prefix)`, true},
		{"backtick-subst", "export HOST=`hostname`", true},
		{"arith-expansion", `export Y=$((1+2))`, true},
		{"plain-literal", `export EDITOR=nvim`, false},
		{"single-quoted-literal", `alias gs='git status'`, false},
		{"single-quoted-with-dollar", `alias h='echo $HOME'`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(c.src))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			if blocks[0].Dynamic != c.wantDynamic {
				t.Errorf("Dynamic = %v, want %v (src=%q value=%q)",
					blocks[0].Dynamic, c.wantDynamic, c.src, blocks[0].Value)
			}
		})
	}
}

// TestParseCompoundIsDynamic pins that conditionals/loops are dynamic by
// construction.
func TestParseCompoundIsDynamic(t *testing.T) {
	src := `if [[ "$OSTYPE" == darwin* ]]; then
  alias ls='ls -G'
fi`
	blocks, err := (Provider{}).Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if blocks[0].Kind != model.KindCompound {
		t.Fatalf("block 0 kind = %q, want compound", blocks[0].Kind)
	}
	if !blocks[0].Dynamic {
		t.Errorf("compound block Dynamic = false, want true (conditional)")
	}
}

// TestParseValueVerbatim proves $HOME/go is stored verbatim in Block.Value,
// NOT resolved to a path — the no-execution invariant (EVAL-01).
func TestParseValueVerbatim(t *testing.T) {
	blocks, err := (Provider{}).Parse([]byte(`export GOPATH=$HOME/go`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if blocks[0].Value != "$HOME/go" {
		t.Errorf("Value = %q, want %q (verbatim, not resolved)", blocks[0].Value, "$HOME/go")
	}
}

// TestParseAliasValueVerbatim pins #4: an alias value is the text AFTER the '='
// only, with quotes preserved verbatim (no re-quoting).
func TestParseAliasValueVerbatim(t *testing.T) {
	blocks, err := (Provider{}).Parse([]byte(`alias gs='git status'`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if blocks[0].Value != "'git status'" {
		t.Errorf("alias Value = %q, want %q (single quotes preserved, value-after-= only)",
			blocks[0].Value, "'git status'")
	}
}

// TestParseFuncValueFullSpan pins the templater convention (02-02): a function's
// captured Value is the FULL `name() { ... }` span, not just the brace-body.
func TestParseFuncValueFullSpan(t *testing.T) {
	blocks, err := (Provider{}).Parse([]byte("greet() { echo hi }"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if blocks[0].Kind != model.KindFuncDecl {
		t.Fatalf("kind = %q, want func", blocks[0].Kind)
	}
	if !strings.Contains(blocks[0].Value, "greet() {") {
		t.Errorf("func Value = %q, want it to contain the full %q span", blocks[0].Value, "greet() {")
	}
}

// TestParseOpaqueStaysReversibleSafe pins threat T-02-01: a malformed config
// degrades to one opaque block without panic, and that opaque block leaves
// Dynamic=false by construction.
func TestParseOpaqueStaysReversibleSafe(t *testing.T) {
	blocks, err := (Provider{}).Parse([]byte("if then fi fi ;;"))
	if err != nil {
		t.Fatalf("Parse should not error on bad input: %v", err)
	}
	if len(blocks) != 1 || !blocks[0].Opaque {
		t.Fatalf("want one opaque block, got %+v", blocks)
	}
	if blocks[0].Dynamic {
		t.Errorf("opaque block Dynamic = true, want false by construction")
	}
}
