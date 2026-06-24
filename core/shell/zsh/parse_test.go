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
