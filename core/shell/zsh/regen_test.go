package zsh

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func TestRegenerateWorktreeBehaviorExactProjectionAndTombstones(t *testing.T) {
	pinned := "captured-secret-canary"
	source := model.Profile{Entries: []model.Entry{
		{Kind: model.KindAssignment, Category: model.CatEnvironment, Names: []string{"EXPORTED"}, Exported: true, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeLiteral, RuntimeValue: stringPointerZsh("source")},
		{Kind: model.KindAssignment, Category: model.CatEnvironment, Names: []string{"PLAIN"}, Exported: false, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeLiteral, RuntimeValue: stringPointerZsh("source")},
		{Kind: model.KindAssignment, Category: model.CatSecrets, Names: []string{"TOKEN"}, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeUnsupported, Value: pinned, Secret: &model.SecretRef{Kind: model.SecretRefFile, Key: "TOKEN"}},
	}}
	document := model.NewCommittedWorktree(source, model.LiveProjection{
		States: []model.LiveIdentityState{
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "EXPORTED"}, Value: model.ScalarLiveValue("it's\nexact")},
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "PLAIN"}, Value: model.ScalarLiveValue("")},
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "ADDED"}, Value: model.ScalarLiveValue("new")},
			{Identity: model.Identity{Kind: model.LiveAlias, Name: "empty_alias"}, Value: model.ScalarLiveValue("")},
			{Identity: model.Identity{Kind: model.LiveFunction, Name: "multi_fn"}, Value: model.ScalarLiveValue("print -r -- one\nprint -r -- two")},
			{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/base", "", "/dup", "/dup"})},
			{Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.ListLiveValue([]string{})},
			{Identity: model.Identity{Kind: model.LiveOption, Name: "NO_BEEP"}, Value: model.OptionLiveValue(false)},
		},
		Tombstones: []model.Identity{
			{Kind: model.LiveEnv, Name: "REMOVED"},
			{Kind: model.LiveAlias, Name: "old_alias"},
			{Kind: model.LiveFunction, Name: "old_fn"},
			{Kind: model.LiveOption, Name: "BEEP"},
		},
	})
	before := model.CloneProfile(document.Source)
	generated, err := (Provider{}).RegenerateWorktree(document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document.Source, before) {
		t.Fatal("RegenerateWorktree mutated the source profile")
	}
	if strings.Contains(string(generated), pinned) || strings.Contains(string(generated), "TOKEN") {
		t.Fatalf("SecretRef content crossed generated source: %s", generated)
	}
	check := exec.Command("zsh", "-n")
	check.Stdin = strings.NewReader(string(generated))
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("generated source is invalid: %v\n%s\n%s", err, output, generated)
	}
	script := strings.Join([]string{
		"export REMOVED=old",
		"alias old_alias='old'",
		"old_fn() { print old; }",
		"setopt BEEP",
		string(generated),
		"[[ $EXPORTED == $\"it's\\nexact\" && ${parameters[EXPORTED]} == *export* ]] || exit 11",
		"[[ ${+PLAIN} == 1 && $PLAIN == '' && ${parameters[PLAIN]} != *export* ]] || exit 12",
		"[[ $ADDED == new && ${parameters[ADDED]} == *export* ]] || exit 13",
		"[[ ${aliases[empty_alias]} == '' ]] || exit 14",
		"[[ \"$(multi_fn)\" == $'one\\ntwo' ]] || exit 15",
		"[[ ${#path} == 4 && $path[1] == /base && $path[2] == '' && $path[3] == /dup && $path[4] == /dup ]] || exit 16",
		"[[ ${#fpath} == 0 ]] || exit 17",
		"[[ -o NO_BEEP ]] && exit 18",
		"(( ${+REMOVED} == 0 && ${+aliases[old_alias]} == 0 && ${+functions[old_fn]} == 0 )) || exit 19",
		"[[ -o BEEP ]] && exit 20",
	}, "\n")
	if output, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("generated worktree behavior mismatch: %v\n%s\n%s", err, output, script)
	}
}

func TestRegenerateWorktreeRejectsMalformedOrHostileProjectionWithoutBytes(t *testing.T) {
	valid := model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{
		States: []model.LiveIdentityState{{Identity: model.Identity{Kind: model.LiveEnv, Name: "SAFE"}, Value: model.ScalarLiveValue("ok")}},
	})
	cases := []model.CommittedWorktree{
		{Schema: "v2", Projection: valid.Projection},
		{Schema: model.WorktreeSchemaV1, Projection: model.LiveProjection{Schema: "v2"}},
		model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{States: []model.LiveIdentityState{
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "DUP"}, Value: model.ScalarLiveValue("one")},
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "DUP"}, Value: model.ScalarLiveValue("two")},
		}}),
		model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{
			States:     valid.Projection.States,
			Tombstones: []model.Identity{{Kind: model.LiveEnv, Name: "SAFE"}},
		}),
		model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{
			States: []model.LiveIdentityState{{Identity: model.Identity{Kind: model.LiveAlias, Name: "x;touch"}, Value: model.ScalarLiveValue("bad")}},
		}),
	}
	for index, document := range cases {
		generated, err := (Provider{}).RegenerateWorktree(document)
		if err == nil || len(generated) != 0 {
			t.Fatalf("case %d returned partial source %q, %v", index, generated, err)
		}
	}
}

func TestWorktreeSyntaxOwnedByEmit(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "regen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var method *ast.FuncDecl
	for _, declaration := range file.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if ok && candidate.Name.Name == "RegenerateWorktree" {
			method = candidate
			break
		}
	}
	if method == nil {
		t.Fatal("RegenerateWorktree method not found")
	}
	emitCalls := map[string]bool{}
	ast.Inspect(method.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.SelectorExpr:
			if owner, ok := function.X.(*ast.Ident); ok && (owner.Name == "fmt" || owner.Name == "strings" || owner.Name == "bytes") {
				t.Errorf("RegenerateWorktree constructs source via %s.%s", owner.Name, function.Sel.Name)
			}
		case *ast.Ident:
			if function.Name == "emitLiveIdentityState" || function.Name == "emitLiveIdentityTombstone" {
				emitCalls[function.Name] = true
			}
		}
		return true
	})
	if !emitCalls["emitLiveIdentityState"] || !emitCalls["emitLiveIdentityTombstone"] {
		t.Fatalf("RegenerateWorktree does not delegate both typed state/removal paths: %#v", emitCalls)
	}
}

func stringPointerZsh(value string) *string { return &value }

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

// TestRegenerateEmptyValueFallsThroughToText pins the defensive empty-Value
// guard (UAT array gap, belt-and-suspenders): a KindAssignment Entry that has a
// name but NO captured scalar Value — e.g. an array assignment forced managed via
// OverrideManaged — must fall through to verbatim Text, never emit a bare `name=`
// that drops the array. Mirrors the existing empty-Names guard (BL-01).
func TestRegenerateEmptyValueFallsThroughToText(t *testing.T) {
	p := Provider{}
	entry := model.Entry{
		Kind:  model.KindAssignment,
		Names: []string{"plugins"},
		Value: "",
		Text:  "plugins=(git zsh-autosuggestions zsh-syntax-highlighting)",
	}
	got := p.Regenerate(entry)
	if got != entry.Text {
		t.Fatalf("Regenerate() = %q, want verbatim Text %q (empty-Value must not emit a bare name=)", got, entry.Text)
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

// TestRegenerateRejectsIncompleteSourceShapes is the provider-side half of the
// persisted fidelity contract. ir.Regenerate already checks Representable, but
// Provider.Regenerate is a public lowering seam and must not reinterpret an
// incomplete forced-managed entry when called directly.
func TestRegenerateRejectsIncompleteSourceShapes(t *testing.T) {
	p := Provider{}
	cases := []model.Entry{
		{Kind: model.KindAssignment, Text: "A=one B=two", Names: []string{"A", "B"}, Value: "two", StructuralFidelityKnown: true},
		{Kind: model.KindAlias, Text: "alias ll", Names: []string{"ll"}, StructuralFidelityKnown: true},
		{Kind: model.KindAlias, Text: "alias a=one b=two", Names: []string{"a", "b"}, Value: "two", AliasAssignment: true, StructuralFidelityKnown: true},
		{Kind: model.KindCommand, Text: "setopt +o extendedglob", CmdName: "setopt", Names: []string{"extendedglob"}, OptionFlags: []string{"+o"}, StructuralFidelityKnown: true},
		{Kind: model.KindCommand, Text: "unsetopt -m extendedglob", CmdName: "unsetopt", Names: []string{"extendedglob"}, OptionFlags: []string{"-m"}, StructuralFidelityKnown: true},
	}
	for _, entry := range cases {
		if got := p.Regenerate(entry); got != entry.Text {
			t.Fatalf("Regenerate(%q) = %q, want verbatim Text", entry.Text, got)
		}
	}
}
