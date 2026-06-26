package zsh

import (
	"testing"

	"zsh-pro/core/model"
)

func TestParseClassifiesKinds(t *testing.T) {
	src := []byte(`# my aliases
alias gs='git status'
export EDITOR=nvim
PATH="$HOME/bin:$PATH"
greet() { echo hi }
setopt AUTO_CD
if [[ "$OSTYPE" == darwin* ]]; then
  alias ls='ls -G'
fi
`)
	blocks, err := (Provider{}).Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(blocks) != 6 {
		t.Fatalf("got %d blocks, want 6", len(blocks))
	}

	cases := []struct {
		idx     int
		kind    model.BlockKind
		cmdName string
		name    string
	}{
		{0, model.KindAlias, "alias", "gs"},
		{1, model.KindAssignment, "export", "EDITOR"},
		{2, model.KindAssignment, "", "PATH"},
		{3, model.KindFuncDecl, "", "greet"},
		{4, model.KindCommand, "setopt", ""},
		{5, model.KindCompound, "", ""},
	}
	for _, c := range cases {
		b := blocks[c.idx]
		if b.Kind != c.kind {
			t.Errorf("block %d: kind %q, want %q", c.idx, b.Kind, c.kind)
		}
		if c.cmdName != "" && b.CmdName != c.cmdName {
			t.Errorf("block %d: cmdName %q, want %q", c.idx, b.CmdName, c.cmdName)
		}
		if c.name != "" && (len(b.Names) == 0 || b.Names[0] != c.name) {
			t.Errorf("block %d: names %v, want first=%q", c.idx, b.Names, c.name)
		}
	}
	// The leading comment is pulled into the alias block's Text for context, but
	// StartLine is the statement's own line (line 2), not the comment line (1).
	if blocks[0].StartLine != 2 {
		t.Errorf("alias block StartLine = %d, want 2 (statement line, not leading-comment line)", blocks[0].StartLine)
	}
}

// TestParseCapturesAppendAndFlags pins the faithfulness signals the router uses
// to keep non-byte-reversible shapes out of the managed/templated path:
//   - `+=` append assignments set Block.Append (WR-01) so they are not rewritten
//     to a plain `=`.
//   - `alias -g`/`-s` flagged aliases set Block.Flagged (WR-02) so the flag is
//     never dropped by the regenerator.
//   - multi-name assignment/alias statements keep every name in Names (BL-02) so
//     the router can detect len(Names) != 1 and route imperative rather than
//     pairing the first name with the last value.
func TestParseCapturesAppendAndFlags(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantKind  model.BlockKind
		append    bool
		flagged   bool
		wantNames []string
	}{
		{"plain append", "FOO+=bar\n", model.KindAssignment, true, false, []string{"FOO"}},
		{"export append", "export PATH+=:/x\n", model.KindAssignment, true, false, []string{"PATH"}},
		{"global alias flagged", "alias -g G='| grep'\n", model.KindAlias, false, true, nil},
		{"suffix alias flagged", "alias -s txt=cat\n", model.KindAlias, false, true, nil},
		{"multi-name export keeps all names", "export FOO=bar BAZ=qux\n", model.KindAssignment, false, false, []string{"FOO", "BAZ"}},
		{"multi-name assignment keeps all names", "FOO=bar BAZ=qux\n", model.KindAssignment, false, false, []string{"FOO", "BAZ"}},
		{"multi-name alias keeps all names", "alias a=1 b=2\n", model.KindAlias, false, false, []string{"a", "b"}},
		{"plain assignment not append", "FOO=bar\n", model.KindAssignment, false, false, []string{"FOO"}},
		{"plain alias not flagged", "alias gs='git status'\n", model.KindAlias, false, false, []string{"gs"}},
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
			b := blocks[0]
			if b.Kind != c.wantKind {
				t.Errorf("kind = %q, want %q", b.Kind, c.wantKind)
			}
			if b.Append != c.append {
				t.Errorf("Append = %v, want %v", b.Append, c.append)
			}
			if b.Flagged != c.flagged {
				t.Errorf("Flagged = %v, want %v", b.Flagged, c.flagged)
			}
			if c.wantNames != nil {
				if len(b.Names) != len(c.wantNames) {
					t.Fatalf("Names = %v, want %v", b.Names, c.wantNames)
				}
				for i, n := range c.wantNames {
					if b.Names[i] != n {
						t.Errorf("Names[%d] = %q, want %q", i, b.Names[i], n)
					}
				}
			}
		})
	}
}

func TestParseOpaqueOnUnparseable(t *testing.T) {
	// A hard syntax error must not crash; it yields one opaque block.
	blocks, err := (Provider{}).Parse([]byte("if then fi fi ;;"))
	if err != nil {
		t.Fatalf("Parse should not error on bad input, got %v", err)
	}
	if len(blocks) != 1 || !blocks[0].Opaque {
		t.Fatalf("want one opaque block, got %+v", blocks)
	}
}
