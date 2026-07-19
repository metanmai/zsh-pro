package activate

import (
	"testing"
	"zsh-pro/core/model"
)

func TestBuildAdmittedAndGuards(t *testing.T) {
	body := "print ok"
	p := model.Profile{Entries: []model.Entry{{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim", Managed: true}, {Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"PATH"}, Value: "$HOME/bin:$PATH", Managed: true}, {Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"gs"}, Value: "git status", Managed: true}, {Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"foo"}, Managed: true, FunctionBody: &body}, {Category: model.CatOptions, Kind: model.KindCommand, CmdName: "setopt", Names: []string{"extendedglob"}, Managed: true}, {Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"BAD"}, Value: "x", Managed: true, Override: model.OverrideUnmanaged}}}
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

func TestBuildComposesSemanticListsInSourceOrder(t *testing.T) {
	list := func(segments ...model.ListSegment) *model.ListValue { return &model.ListValue{Segments: segments} }
	p := model.Profile{Entries: []model.Entry{
		{Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"PATH"}, Managed: true, ListValue: list(model.ListSegment{Value: "/a"}, model.ListSegment{Self: true})},
		{Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"PATH"}, Managed: true, ListValue: list(model.ListSegment{Self: true}, model.ListSegment{Value: "/b"})},
		{Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"FPATH"}, Managed: true, ListValue: list(model.ListSegment{Dynamic: true, Source: "$EXTRA"}, model.ListSegment{Self: true})},
	}}
	m := Build(p)
	if len(m.Lists) != 2 {
		t.Fatalf("lists=%#v", m.Lists)
	}
	path := m.Lists[0]
	if path.Name != "PATH" || len(path.Additions) != 2 || path.Additions[0] != "/a" || path.Additions[1] != "/b" || path.BaseIndex == nil || *path.BaseIndex != 1 {
		t.Fatalf("PATH=%#v", path)
	}
	fpath := m.Lists[1]
	if fpath.Name != "FPATH" || len(fpath.AdditionDynamic) != 1 || !fpath.AdditionDynamic[0] || fpath.BaseIndex == nil || *fpath.BaseIndex != 1 {
		t.Fatalf("FPATH=%#v", fpath)
	}
}

func TestBuildUsesExplicitRuntimeValueContract(t *testing.T) {
	literalDollar := "$HOME"
	empty := ""
	p := model.Profile{Entries: []model.Entry{
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"LITERAL"}, Value: "'$HOME'", Managed: true, ValueMode: model.ValueModeLiteral, RuntimeValue: &literalDollar},
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"DYNAMIC"}, Value: "$HOME", Managed: true, Dynamic: true, ValueMode: model.ValueModeDynamic},
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EMPTY"}, Value: "''", Managed: true, ValueMode: model.ValueModeLiteral, RuntimeValue: &empty},
		{Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"literal"}, Value: "'echo $HOME'", Managed: true, ValueMode: model.ValueModeLiteral, RuntimeValue: func() *string { s := "echo $HOME"; return &s }()},
		{Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"dynamic"}, Value: "echo $HOME", Managed: true, Dynamic: true, ValueMode: model.ValueModeDynamic},
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"UNSUPPORTED"}, Value: "${^spec}", Managed: true, ValueMode: model.ValueModeUnsupported},
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"BAD_LITERAL"}, Value: "'bad'", Managed: true, ValueMode: model.ValueModeLiteral},
		{Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"bad_dynamic"}, Value: "$HOME", Managed: true, ValueMode: model.ValueModeDynamic},
	}}

	m := Build(p)
	if len(m.Env) != 3 {
		t.Fatalf("malformed/unsupported values were not dropped: %#v", m.Env)
	}
	for i, tc := range []struct {
		name    string
		applied string
		dynamic bool
	}{{"LITERAL", "$HOME", false}, {"DYNAMIC", "$HOME", true}, {"EMPTY", "", false}} {
		got := m.Env[i]
		if got.Name != tc.name || got.Applied != tc.applied || got.Dynamic == nil || *got.Dynamic != tc.dynamic {
			t.Fatalf("env[%d]=%#v", i, got)
		}
	}
	if got := m.Aliases.Added["literal"]; got != "echo $HOME" {
		t.Fatalf("literal alias body=%q", got)
	}
	if dynamic, ok := m.Aliases.Dynamic["literal"]; !ok || dynamic {
		t.Fatalf("literal alias provenance=%#v", m.Aliases.Dynamic)
	}
	if dynamic, ok := m.Aliases.Dynamic["dynamic"]; !ok || !dynamic {
		t.Fatalf("dynamic alias provenance=%#v", m.Aliases.Dynamic)
	}
	if _, ok := m.Aliases.Added["bad_dynamic"]; ok {
		t.Fatal("malformed dynamic alias used legacy fallback")
	}
}

func TestBuildLegacyValueFallback(t *testing.T) {
	m := Build(model.Profile{Entries: []model.Entry{
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"LEGACY"}, Value: "$HOME", Managed: true, Dynamic: true},
		{Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"legacy"}, Value: "echo $HOME", Managed: true, Dynamic: true},
	}})
	if len(m.Env) != 1 || m.Env[0].Applied != "$HOME" || m.Env[0].Dynamic == nil || !*m.Env[0].Dynamic {
		t.Fatalf("legacy scalar fallback changed: %#v", m.Env)
	}
	if m.Aliases.Added["legacy"] != "echo $HOME" || !m.Aliases.Dynamic["legacy"] {
		t.Fatalf("legacy alias fallback changed: %#v", m.Aliases)
	}
}

func TestBuildRequiresAndPreservesFunctionBodies(t *testing.T) {
	empty := ""
	multiline := "print one\nprint two"
	m := Build(model.Profile{Entries: []model.Entry{
		{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"empty"}, Managed: true, FunctionBody: &empty},
		{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"multiline"}, Managed: true, FunctionBody: &multiline},
		{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"missing"}, Managed: true},
	}})
	if len(m.Functions.Added) != 2 {
		t.Fatalf("missing function body did not fail closed: %#v", m.Functions)
	}
	if body, ok := m.Functions.Bodies["empty"]; !ok || body != "" {
		t.Fatalf("empty body lost: %#v", m.Functions.Bodies)
	}
	if m.Functions.Bodies["multiline"] != multiline {
		t.Fatalf("multiline body changed: %q", m.Functions.Bodies["multiline"])
	}
}

func TestBuildReducesRepeatedEffectiveIdentities(t *testing.T) {
	firstBody := "print first"
	lastBody := "print last"
	p := model.Profile{Entries: []model.Entry{
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "one", Managed: true},
		{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"work"}, Managed: true, FunctionBody: &firstBody},
		{Category: model.CatOptions, Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}, Managed: true},
		{Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "two", Managed: true, Exported: true},
		{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"work"}, Managed: true, FunctionBody: &lastBody},
		{Category: model.CatOptions, Kind: model.KindCommand, CmdName: "unsetopt", Names: []string{"EXTENDED_GLOB"}, Managed: true},
	}}

	m := Build(p)
	if len(m.Env) != 1 || m.Env[0].Name != "EDITOR" || m.Env[0].Applied != "two" || m.Env[0].Exported == nil || !*m.Env[0].Exported {
		t.Fatalf("scalar was not reduced with additive export intent: %#v", m.Env)
	}
	if len(m.Functions.Added) != 1 || m.Functions.Added[0] != "work" || m.Functions.Bodies["work"] != lastBody {
		t.Fatalf("function was not reduced to final body: %#v", m.Functions)
	}
	if len(m.Options) != 1 || m.Options[0] != (model.OptionSet{Name: "EXTENDED_GLOB", Enabled: false}) {
		t.Fatalf("option was not reduced to final state: %#v", m.Options)
	}
}

func TestBuildPreservesDistinctPunctuationIdentities(t *testing.T) {
	names := []string{"foo-bar", "foo.bar", "foo_bar"}
	p := model.Profile{}
	for _, name := range names {
		body := "print " + name
		p.Entries = append(p.Entries,
			model.Entry{Category: model.CatAliases, Kind: model.KindAlias, Names: []string{name}, Value: name, Managed: true},
			model.Entry{Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{name}, Managed: true, FunctionBody: &body},
		)
	}
	m := Build(p)
	if len(m.Aliases.Added) != len(names) || len(m.Functions.Added) != len(names) {
		t.Fatalf("punctuation-distinct names merged: %#v %#v", m.Aliases, m.Functions)
	}
}
