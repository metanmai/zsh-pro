package zsh

import (
	"strings"
	"testing"

	"zsh-pro/core/model"
)

// TestRegenerate pins the declarative templater: values are emitted VERBATIM
// (dynamic values never resolved, quotes preserved — EVAL-01 / D-05 / Pitfall 3),
// each Kind rebuilds from structured fields, and any unhandled Kind falls through
// to verbatim Text (the total #3 default case).
func TestRegenerate(t *testing.T) {
	p := Provider{}
	tests := []struct {
		name  string
		entry model.Entry
		want  string
	}{
		{
			name:  "exported dynamic env emitted verbatim",
			entry: model.Entry{Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go", Exported: true, Dynamic: true},
			want:  "export GOPATH=$HOME/go",
		},
		{
			name:  "non-exported assignment omits export",
			entry: model.Entry{Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "bar"},
			want:  "FOO=bar",
		},
		{
			name:  "alias preserves single-quoted body",
			entry: model.Entry{Kind: model.KindAlias, Names: []string{"gs"}, Value: "'git status'"},
			want:  "alias gs='git status'",
		},
		{
			name:  "setopt rebuilt from CmdName + Names",
			entry: model.Entry{Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}},
			want:  "setopt EXTENDED_GLOB",
		},
		{
			name:  "unsetopt rebuilt from CmdName + Names",
			entry: model.Entry{Kind: model.KindCommand, CmdName: "unsetopt", Names: []string{"NOMATCH"}},
			want:  "unsetopt NOMATCH",
		},
		{
			name:  "function emitted as full span verbatim",
			entry: model.Entry{Kind: model.KindFuncDecl, Names: []string{"greet"}, Value: "greet() { echo hi }"},
			want:  "greet() { echo hi }",
		},
		{
			name:  "non-setopt command falls through to verbatim Text",
			entry: model.Entry{Kind: model.KindCommand, CmdName: "bindkey", Text: "bindkey -e"},
			want:  "bindkey -e",
		},
		{
			name:  "unhandled Kind returns Text verbatim (default case #3)",
			entry: model.Entry{Kind: model.KindOther, Text: "# some opaque construct"},
			want:  "# some opaque construct",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Regenerate(tc.entry)
			if got != tc.want {
				t.Fatalf("Regenerate() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRegenerateNeverPanicsOnEmptyNames pins BL-01: a forced-managed entry of
// KindAssignment/KindAlias with empty Names must NOT panic (the "never panics in
// production" invariant). It falls through to verbatim Text instead of reading
// Names[0].
func TestRegenerateNeverPanicsOnEmptyNames(t *testing.T) {
	p := Provider{}
	cases := []struct {
		name  string
		entry model.Entry
		want  string
	}{
		{"empty-names assignment falls through to Text", model.Entry{Kind: model.KindAssignment, Text: "export -p"}, "export -p"},
		{"empty-names exported assignment falls through to Text", model.Entry{Kind: model.KindAssignment, Exported: true, Text: "export"}, "export"},
		{"empty-names alias falls through to Text", model.Entry{Kind: model.KindAlias, Text: "alias"}, "alias"},
		{"empty-names setopt falls through to Text", model.Entry{Kind: model.KindCommand, CmdName: "setopt", Text: "setopt"}, "setopt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Regenerate panicked on empty Names: %v", r)
				}
			}()
			if got := p.Regenerate(tc.entry); got != tc.want {
				t.Fatalf("Regenerate() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRegenerateDynamicValueNeverResolved is an explicit EVAL-01 pin: a $HOME
// value must round-trip with the literal $HOME intact, never the resolved path.
func TestRegenerateDynamicValueNeverResolved(t *testing.T) {
	p := Provider{}
	out := p.Regenerate(model.Entry{Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go", Exported: true})
	if !strings.Contains(out, "$HOME/go") {
		t.Fatalf("dynamic value resolved or lost: %q does not contain $HOME/go", out)
	}
}
