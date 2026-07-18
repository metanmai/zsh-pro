package zsh

import (
	"os"
	"path/filepath"
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

// TestParseRuntimeValueModes pins the fail-closed source-to-runtime contract:
// Value remains verbatim source text while RuntimeValue exists only for AST
// forms whose data semantics are explicitly modeled.
func TestParseRuntimeValueModes(t *testing.T) {
	empty := ""
	cases := []struct {
		name        string
		src         string
		wantValue   string
		wantMode    model.ValueMode
		wantRuntime *string
		wantDynamic bool
	}{
		{name: "plain literal", src: `FOO=plain`, wantValue: "plain", wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("plain")},
		{name: "single quoted", src: `FOO='single quoted'`, wantValue: "'single quoted'", wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("single quoted")},
		{name: "double quoted", src: `FOO="double quoted"`, wantValue: `"double quoted"`, wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("double quoted")},
		{name: "concatenated parts", src: `FOO=a' b'"c"`, wantValue: `a' b'"c"`, wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("a bc")},
		{name: "escaped space", src: `FOO=hello\ world`, wantValue: `hello\ world`, wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("hello world")},
		{name: "escaped quote", src: `FOO=it\'s`, wantValue: `it\'s`, wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("it's")},
		{name: "empty literal", src: `FOO=''`, wantValue: "''", wantMode: model.ValueModeLiteral, wantRuntime: &empty},
		{name: "quoted dollar literal", src: `alias x='$HOME'`, wantValue: "'$HOME'", wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("$HOME")},
		{name: "alias equals body", src: `alias x='a=b=c'`, wantValue: "'a=b=c'", wantMode: model.ValueModeLiteral, wantRuntime: stringPtr("a=b=c")},
		{name: "unquoted parameter dynamic", src: `alias x=$HOME`, wantValue: "$HOME", wantMode: model.ValueModeDynamic, wantDynamic: true},
		{name: "command substitution dynamic", src: `FOO=$(printf nope)`, wantValue: "$(printf nope)", wantMode: model.ValueModeDynamic, wantDynamic: true},
		{name: "ansi c quote unsupported", src: `FOO=$'\n'`, wantValue: `$'\n'`, wantMode: model.ValueModeUnsupported},
		{name: "leading tilde unsupported", src: `FOO=~`, wantValue: "~", wantMode: model.ValueModeUnsupported},
		{name: "brace expansion unsupported", src: `FOO={a,b}`, wantValue: "{a,b}", wantMode: model.ValueModeUnsupported},
		{name: "extglob unsupported", src: `FOO=@(a|b)`, wantValue: "@(a|b)", wantMode: model.ValueModeUnsupported},
		{name: "naked export unsupported", src: `export FOO`, wantValue: "", wantMode: model.ValueModeUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			got := blocks[0]
			if got.Opaque {
				t.Fatalf("source parsed opaque: %q", tc.src)
			}
			if got.Value != tc.wantValue {
				t.Errorf("Value = %q, want verbatim %q", got.Value, tc.wantValue)
			}
			if got.ValueMode != tc.wantMode {
				t.Errorf("ValueMode = %q, want %q", got.ValueMode, tc.wantMode)
			}
			if got.Dynamic != tc.wantDynamic {
				t.Errorf("Dynamic = %v, want %v", got.Dynamic, tc.wantDynamic)
			}
			if tc.wantRuntime == nil {
				if got.RuntimeValue != nil {
					t.Errorf("RuntimeValue = %q, want nil", *got.RuntimeValue)
				}
				return
			}
			if got.RuntimeValue == nil || *got.RuntimeValue != *tc.wantRuntime {
				t.Errorf("RuntimeValue = %v, want present %q", got.RuntimeValue, *tc.wantRuntime)
			}
		})
	}
}

func TestParseRuntimeValueDecoderDoesNotExecute(t *testing.T) {
	canary := filepath.Join(t.TempDir(), "parser-executed")
	src := "FOO=$(touch " + canary + ")"
	blocks, err := (Provider{}).Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if blocks[0].ValueMode != model.ValueModeDynamic || !blocks[0].Dynamic {
		t.Fatalf("command substitution contract = mode %q dynamic %v, want dynamic", blocks[0].ValueMode, blocks[0].Dynamic)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("parser executed command substitution; canary stat error = %v", err)
	}
}

func stringPtr(s string) *string { return &s }
