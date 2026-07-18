package activate

import (
	"testing"
	"zsh-pro/core/model"
)

func TestBuildAdmittedAndGuards(t *testing.T) {
	p := model.Profile{Entries: []model.Entry{{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim", Managed: true}, {Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"PATH"}, Value: "$HOME/bin:$PATH", Managed: true}, {Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"gs"}, Value: "git status", Managed: true}, {Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"foo"}, Managed: true}, {Category: model.CatOptions, Kind: model.KindCommand, CmdName: "setopt", Names: []string{"extendedglob"}, Managed: true}, {Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"BAD"}, Value: "x", Managed: true, Override: model.OverrideUnmanaged}}}
	m := Build(p)
	if len(m.Env) != 1 || len(m.Lists) != 1 || len(m.Aliases.Added) != 1 || len(m.Functions.Added) != 1 || len(m.Options) != 1 {
		t.Fatalf("manifest=%#v", m)
	}
	if m.Lists[0].Additions[0] != "$HOME/bin" {
		t.Fatal(m.Lists)
	}
	if m.Options[0].WasOn {
		t.Fatal("builder authored WasOn")
	}
}
func TestBuildRejectsHostile(t *testing.T) {
	for _, e := range []model.Entry{{Kind: model.KindAlias, Category: model.CatAliases, Names: []string{"gs;touch"}, Managed: true}, {Kind: model.KindFuncDecl, Category: model.CatFunctions, Names: []string{"x$(...)y"}, Managed: true}, {Kind: model.KindCommand, Category: model.CatOptions, CmdName: "setopt", Names: []string{"foo;bar"}, Managed: true}, {Kind: model.KindAssignment, Category: model.CatPath, Names: []string{"PATH"}, Value: "/x$(touch):$PATH", Managed: true}} {
		m := Build(model.Profile{Entries: []model.Entry{e}})
		if len(m.Aliases.Added) > 0 || len(m.Functions.Added) > 0 || len(m.Options) > 0 || len(m.Lists) > 0 {
			t.Fatalf("hostile admitted: %#v", m)
		}
	}
}
func TestBuildPathShapes(t *testing.T) {
	for _, tc := range []struct {
		v  string
		ok bool
	}{{"$PATH:$HOME/bin", true}, {"$HOME/bin:$PATH", true}, {"$HOME/bin:$PATH:$HOME/go", false}, {"/usr/bin", false}, {"$PATH:/opt/tool*", true}, {"/opt/x;bad:$PATH", false}} {
		m := Build(model.Profile{Entries: []model.Entry{{Kind: model.KindAssignment, Category: model.CatPath, Names: []string{"PATH"}, Value: tc.v, Managed: true}}})
		if (len(m.Lists) > 0) != tc.ok {
			t.Fatalf("%q got %#v", tc.v, m.Lists)
		}
	}
}
