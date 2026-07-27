package ir

import (
	"reflect"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

// stubClassifier is a minimal in-test shell.Classifier so core/ir stays free of
// core/shell/zsh. It returns whatever category it is told to, per CmdName/Kind.
type stubClassifier struct {
	cat model.Category
}

func (s stubClassifier) Classify(b model.Block) (model.Category, model.Confidence) {
	return s.cat, model.ConfHigh
}

func (s stubClassifier) Categories() []model.Category { return model.Categories() }

// TestRouteManaged pins the ING-02 declarative/imperative gate (D-04/D-06):
// only the 5 admitted reversible classes route managed.
func TestRouteManaged(t *testing.T) {
	cases := []struct {
		name string
		b    model.Block
		cat  model.Category
		want bool
	}{
		{"env assignment", model.Block{Kind: model.KindAssignment, Names: []string{"EDITOR"}}, model.CatEnvironment, true},
		{"path assignment", model.Block{Kind: model.KindAssignment, Names: []string{"PATH"}}, model.CatPath, true},
		{"secret assignment", model.Block{Kind: model.KindAssignment, Names: []string{"API_KEY"}}, model.CatSecrets, true},
		{"alias", model.Block{Kind: model.KindAlias, Names: []string{"gs"}}, model.CatAliases, true},
		{"function", model.Block{Kind: model.KindFuncDecl, Names: []string{"greet"}}, model.CatFunctions, true},
		{"setopt with name", model.Block{Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}}, model.CatOptions, true},
		{"unsetopt with name", model.Block{Kind: model.KindCommand, CmdName: "unsetopt", Names: []string{"X"}}, model.CatOptions, true},
		{"bare setopt no names", model.Block{Kind: model.KindCommand, CmdName: "setopt"}, model.CatOptions, false},
		{"bare unsetopt no names", model.Block{Kind: model.KindCommand, CmdName: "unsetopt"}, model.CatOptions, false},
		{"zstyle", model.Block{Kind: model.KindCommand, CmdName: "zstyle", Names: []string{"x"}}, model.CatOptions, false},
		{"autoload", model.Block{Kind: model.KindCommand, CmdName: "autoload"}, model.CatOptions, false},
		{"compinit", model.Block{Kind: model.KindCommand, CmdName: "compinit"}, model.CatOptions, false},
		{"compdef", model.Block{Kind: model.KindCommand, CmdName: "compdef"}, model.CatOptions, false},
		{"zmodload", model.Block{Kind: model.KindCommand, CmdName: "zmodload"}, model.CatOptions, false},
		{"opaque", model.Block{Opaque: true, Kind: model.KindAlias, Names: []string{"x"}}, model.CatAliases, false},
		{"misc compound", model.Block{Kind: model.KindCompound}, model.CatMisc, false},
		{"plugins source", model.Block{Kind: model.KindCommand, CmdName: "source"}, model.CatPlugins, false},
		{"keybinding", model.Block{Kind: model.KindCommand, CmdName: "bindkey"}, model.CatKeybindings, false},
		{"local override", model.Block{Kind: model.KindCompound}, model.CatLocal, false},
		// BL-01: empty-Names assignment/alias must NOT route managed (would panic
		// the templater's Names[0] read). These are bare/flag-only forms.
		{"bare export no names", model.Block{Kind: model.KindAssignment}, model.CatEnvironment, false},
		{"export -p no names", model.Block{Kind: model.KindAssignment}, model.CatEnvironment, false},
		{"bare alias no names", model.Block{Kind: model.KindAlias}, model.CatAliases, false},
		// BL-02: multi-name assignment/alias must route imperative (the model
		// stores a single Value, so templating would pair name[0] with the last
		// value and drop the rest). Verbatim Text round-trips faithfully instead.
		{"multi-name export", model.Block{Kind: model.KindAssignment, Names: []string{"FOO", "BAZ"}}, model.CatEnvironment, false},
		{"multi-name assignment", model.Block{Kind: model.KindAssignment, Names: []string{"A", "B"}}, model.CatPath, false},
		{"multi-name alias", model.Block{Kind: model.KindAlias, Names: []string{"a", "b"}}, model.CatAliases, false},
		// WR-01: a `+=` append assignment must route imperative (templating emits
		// `=`, silently turning an append into an overwrite).
		{"append assignment", model.Block{Kind: model.KindAssignment, Names: []string{"PATH"}, Append: true}, model.CatPath, false},
		// WR-02: a flagged alias (`alias -g`/`-s`) must route imperative (the flag
		// is not captured in structured fields, so templating downgrades it).
		{"flagged alias", model.Block{Kind: model.KindAlias, Names: []string{"G"}, Flagged: true}, model.CatAliases, false},
		// Single faithful shapes still route managed.
		{"single env assignment still managed", model.Block{Kind: model.KindAssignment, Names: []string{"EDITOR"}}, model.CatEnvironment, true},
		{"single alias still managed", model.Block{Kind: model.KindAlias, Names: []string{"gs"}}, model.CatAliases, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := routeManaged(c.b, c.cat); got != c.want {
				t.Errorf("routeManaged(%+v, %q) = %v, want %v", c.b, c.cat, got, c.want)
			}
		})
	}
}

// TestBuildPreservesSourceOrderAndFields pins that Build emits one Entry per
// block in source order, copying the derived fields and setting Override=Auto.
func TestBuildPreservesSourceOrderAndFields(t *testing.T) {
	blocks := []model.Block{
		{Text: "export EDITOR=nvim", StartLine: 1, Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim", Exported: true},
		{Text: "alias gs='git status'", StartLine: 2, Kind: model.KindAlias, Names: []string{"gs"}, Value: "'git status'"},
	}
	p := Build(blocks, stubClassifier{cat: model.CatAliases})
	if len(p.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(p.Entries))
	}
	if p.Entries[0].Text != "export EDITOR=nvim" || p.Entries[1].Text != "alias gs='git status'" {
		t.Errorf("entries not in source order: %q, %q", p.Entries[0].Text, p.Entries[1].Text)
	}
	e := p.Entries[0]
	if e.Kind != model.KindAssignment || e.Value != "nvim" || !e.Exported || e.StartLine != 1 {
		t.Errorf("entry 0 derived fields not copied: %+v", e)
	}
	if e.Override != model.OverrideAuto {
		t.Errorf("entry 0 Override = %q, want OverrideAuto", e.Override)
	}
	if e.Category != model.CatAliases {
		t.Errorf("entry 0 Category = %q, want from classifier", e.Category)
	}
}

func TestBuildCopiesStructuralFidelity(t *testing.T) {
	blocks := []model.Block{
		{Kind: model.KindAssignment, Names: []string{"FOO"}, Append: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Kind: model.KindAssignment, Names: []string{"plugins"}, Array: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Kind: model.KindAlias, Names: []string{"G"}, Flagged: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Kind: model.KindAssignment, Names: []string{"FOO"}, Indexed: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Kind: model.KindAssignment, CmdName: "export", Names: []string{"COUNT"}, DeclarationFlags: []string{"-i"}, OptionFlags: []string{}},
		{Kind: model.KindAlias, Names: []string{"ll"}, AliasAssignment: false, DeclarationFlags: []string{}, OptionFlags: nil},
		{Kind: model.KindAlias, Names: []string{"ll"}, AliasAssignment: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}, DeclarationFlags: []string{}, OptionFlags: []string{"-o"}},
	}
	profile := Build(blocks, stubClassifier{cat: model.CatEnvironment})
	for i, want := range blocks {
		got := profile.Entries[i]
		if !got.StructuralFidelityKnown || got.Append != want.Append || got.Array != want.Array || got.Flagged != want.Flagged || got.Indexed != want.Indexed || got.AliasAssignment != want.AliasAssignment || !reflect.DeepEqual(got.DeclarationFlags, want.DeclarationFlags) || !reflect.DeepEqual(got.OptionFlags, want.OptionFlags) {
			t.Fatalf("entry %d lost structural fidelity: got=%#v want=%#v", i, got, want)
		}
	}
	profile.Entries[4].DeclarationFlags[0] = "-r"
	if blocks[4].DeclarationFlags[0] != "-i" {
		t.Fatal("Entry.DeclarationFlags shares Block backing storage")
	}
	blocks[4].DeclarationFlags[0] = "-x"
	if profile.Entries[4].DeclarationFlags[0] != "-r" {
		t.Fatal("Block.DeclarationFlags shares Entry backing storage")
	}
	profile.Entries[7].OptionFlags[0] = "--"
	if blocks[7].OptionFlags[0] != "-o" {
		t.Fatal("Entry.OptionFlags shares Block backing storage")
	}
	blocks[7].OptionFlags[0] = "+o"
	if profile.Entries[7].OptionFlags[0] != "--" {
		t.Fatal("Block.OptionFlags shares Entry backing storage")
	}
}

// TestBuildSetsManagedFromRouter pins that Build's Managed verdict is driven by
// routeManaged + the injected classifier's category.
func TestBuildSetsManagedFromRouter(t *testing.T) {
	// An alias classified as CatAliases routes managed.
	blocks := []model.Block{{Kind: model.KindAlias, Names: []string{"gs"}}}
	p := Build(blocks, stubClassifier{cat: model.CatAliases})
	if !p.Entries[0].Managed {
		t.Errorf("alias entry Managed = false, want true")
	}
	if !p.Entries[0].EffectiveManaged() {
		t.Errorf("alias entry EffectiveManaged = false, want true (Override defaults auto)")
	}
}

// TestEffectiveManagedOverrideWins pins D-07: a set Override wins over the auto
// verdict, in both directions.
func TestEffectiveManagedOverrideWins(t *testing.T) {
	managed := model.Entry{Managed: true, Override: model.OverrideUnmanaged}
	if managed.EffectiveManaged() {
		t.Errorf("OverrideUnmanaged did not win over Managed=true")
	}
	imperative := model.Entry{Managed: false, Override: model.OverrideManaged}
	if !imperative.EffectiveManaged() {
		t.Errorf("OverrideManaged did not win over Managed=false")
	}
}

// TestBuildCopiesNamesSlice pins WR-03: the Entry must NOT share the source
// Block's Names backing array, so a later mutation of either side cannot
// silently corrupt the other.
func TestBuildCopiesNamesSlice(t *testing.T) {
	names := []string{"EDITOR"}
	blocks := []model.Block{{Kind: model.KindAssignment, Names: names, Value: "nvim"}}
	p := Build(blocks, stubClassifier{cat: model.CatEnvironment})

	got := p.Entries[0].Names
	if len(got) != 1 || got[0] != "EDITOR" {
		t.Fatalf("Names not copied through: %v", got)
	}
	// Mutate the Entry's slice; the source block's slice must be untouched.
	got[0] = "MUTATED"
	if names[0] != "EDITOR" {
		t.Errorf("Entry.Names shares backing array with source Block.Names: source mutated to %q", names[0])
	}
	names[0] = "SOURCE"
	if got[0] != "MUTATED" {
		t.Errorf("source Block.Names shares backing array with Entry.Names: entry mutated to %q", got[0])
	}
}

// TestBuildDynamicIsOrthogonal pins D-05: Dynamic is copied through and is
// independent of Managed (a dynamic value can still be managed).
func TestBuildDynamicIsOrthogonal(t *testing.T) {
	blocks := []model.Block{{Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go", Dynamic: true}}
	p := Build(blocks, stubClassifier{cat: model.CatEnvironment})
	if !p.Entries[0].Dynamic {
		t.Errorf("Dynamic flag not propagated to Entry")
	}
	if !p.Entries[0].Managed {
		t.Errorf("a dynamic env assignment should still route managed (orthogonal axes)")
	}
}

// TestBuildCopiesRuntimeValueContract pins the additive parse-to-store semantic
// contract. In particular, a present empty RuntimeValue must remain distinct
// from a missing value, and Build must not alias the source pointer.
func TestBuildCopiesRuntimeValueContract(t *testing.T) {
	empty := ""
	literal := "decoded value"
	cases := []struct {
		name        string
		mode        model.ValueMode
		runtime     *string
		wantRuntime *string
	}{
		{name: "legacy missing", mode: model.ValueModeLegacy},
		{name: "literal empty", mode: model.ValueModeLiteral, runtime: &empty, wantRuntime: &empty},
		{name: "literal value", mode: model.ValueModeLiteral, runtime: &literal, wantRuntime: &literal},
		{name: "dynamic missing", mode: model.ValueModeDynamic},
		{name: "unsupported missing", mode: model.ValueModeUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := []model.Block{{
				Kind:         model.KindAssignment,
				Names:        []string{"VALUE"},
				ValueMode:    tc.mode,
				RuntimeValue: tc.runtime,
			}}
			p := Build(blocks, stubClassifier{cat: model.CatEnvironment})
			got := p.Entries[0]

			if got.ValueMode != tc.mode {
				t.Fatalf("ValueMode = %q, want %q", got.ValueMode, tc.mode)
			}
			if tc.wantRuntime == nil {
				if got.RuntimeValue != nil {
					t.Fatalf("RuntimeValue = %q, want nil", *got.RuntimeValue)
				}
				return
			}
			if got.RuntimeValue == nil || *got.RuntimeValue != *tc.wantRuntime {
				t.Fatalf("RuntimeValue = %v, want present %q", got.RuntimeValue, *tc.wantRuntime)
			}
			if got.RuntimeValue == tc.runtime {
				t.Fatal("RuntimeValue pointer aliases the source Block")
			}
			*got.RuntimeValue = "mutated"
			if *tc.runtime != *tc.wantRuntime {
				t.Fatalf("mutating Entry.RuntimeValue changed Block.RuntimeValue to %q", *tc.runtime)
			}
		})
	}
}

// TestBuildCopiesFunctionBodyContract pins nil-vs-present body semantics for
// empty and multiline functions without sharing pointers across the IR seam.
func TestBuildCopiesFunctionBodyContract(t *testing.T) {
	empty := ""
	multiline := "\n\tprint one\n\tprint two\n"
	cases := []struct {
		name string
		body *string
	}{
		{name: "missing"},
		{name: "empty", body: &empty},
		{name: "multiline", body: &multiline},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := []model.Block{{
				Kind:         model.KindFuncDecl,
				Names:        []string{"demo"},
				FunctionBody: tc.body,
			}}
			p := Build(blocks, stubClassifier{cat: model.CatFunctions})
			got := p.Entries[0].FunctionBody

			if tc.body == nil {
				if got != nil {
					t.Fatalf("FunctionBody = %q, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tc.body {
				t.Fatalf("FunctionBody = %v, want present %q", got, *tc.body)
			}
			if got == tc.body {
				t.Fatal("FunctionBody pointer aliases the source Block")
			}
			*got = "mutated"
			if *tc.body == "mutated" {
				t.Fatal("mutating Entry.FunctionBody changed Block.FunctionBody")
			}
		})
	}
}

func TestBuildCopiesListValueContract(t *testing.T) {
	list := &model.ListValue{Segments: []model.ListSegment{
		{Value: "/literal"},
		{Dynamic: true, Source: "${EXTRA}"},
		{Self: true},
	}}
	p := Build([]model.Block{{
		Kind:      model.KindAssignment,
		Names:     []string{"PATH"},
		ListValue: list,
	}}, stubClassifier{cat: model.CatPath})

	got := p.Entries[0].ListValue
	if got == nil || len(got.Segments) != 3 {
		t.Fatalf("ListValue = %#v, want three present segments", got)
	}
	if got == list || &got.Segments[0] == &list.Segments[0] {
		t.Fatal("ListValue shares source storage")
	}
	if got.Segments[1].Source != "${EXTRA}" || !got.Segments[1].Dynamic || !got.Segments[2].Self {
		t.Fatalf("ListValue = %#v", got)
	}
	got.Segments[0].Value = "mutated"
	if list.Segments[0].Value != "/literal" {
		t.Fatalf("source ListValue mutated to %#v", list)
	}

	selfOnly := Build([]model.Block{{
		Kind:      model.KindAssignment,
		Names:     []string{"PATH"},
		ListValue: &model.ListValue{Segments: []model.ListSegment{{Self: true}}},
	}}, stubClassifier{cat: model.CatPath})
	if selfOnly.Entries[0].ListValue == nil {
		t.Fatal("self-only ListValue was lost")
	}
}

func TestBuildRejectsMalformedListValue(t *testing.T) {
	for _, list := range []*model.ListValue{
		{Segments: []model.ListSegment{{Dynamic: true}, {Self: true}}},
		{Segments: []model.ListSegment{{Self: true, Value: "/bad"}}},
		{Segments: []model.ListSegment{{Value: "/a"}}},
		{Segments: []model.ListSegment{{Self: true}, {Self: true}}},
	} {
		p := Build([]model.Block{{
			Kind:      model.KindAssignment,
			Names:     []string{"PATH"},
			ListValue: list,
		}}, stubClassifier{cat: model.CatPath})
		if p.Entries[0].ListValue != nil {
			t.Fatalf("malformed ListValue survived: %#v", p.Entries[0].ListValue)
		}
	}
}

func TestBuildDeclarationFormsStayImperative(t *testing.T) {
	provider := zsh.Provider{}
	cases := []struct {
		name string
		src  string
	}{
		{name: "integer typeset", src: "typeset -i COUNT=2\n"},
		{name: "readonly", src: "readonly LOCKED=value\n"},
		{name: "tied", src: "typeset -T PATH path\n"},
		{name: "local", src: "local scoped=value\n"},
		{name: "plain assignment remains managed", src: "EDITOR=nvim\n"},
		{name: "export remains managed", src: "export EDITOR=nvim\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := provider.Parse([]byte(tc.src))
			if err != nil || len(blocks) != 1 {
				t.Fatalf("Parse() blocks=%#v err=%v", blocks, err)
			}
			profile := Build(blocks, stubClassifier{cat: model.CatEnvironment})
			entry := profile.Entries[0]
			if entry.Text != "" && entry.Text != tc.src[:len(tc.src)-1] {
				t.Fatalf("Text=%q, want source preserved %q", entry.Text, tc.src)
			}
			wantManaged := tc.name == "plain assignment remains managed" || tc.name == "export remains managed"
			if entry.Managed != wantManaged {
				t.Fatalf("Managed=%v, want %v for %#v", entry.Managed, wantManaged, entry)
			}
			if !wantManaged && entry.EffectiveManaged() {
				t.Fatalf("declaration unexpectedly effective-managed: %#v", entry)
			}
		})
	}
}
