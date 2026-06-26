package ir

import (
	"testing"

	"zsh-pro/core/model"
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
