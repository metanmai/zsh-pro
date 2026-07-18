package activate

import (
	"testing"
	"zsh-pro/core/model"
)

func TestDiffOrderingAndDistinctLists(t *testing.T) {
	a := &model.Manifest{Schema: model.SchemaV1, Lists: []model.ListDelta{{Name: "PATH", Additions: []string{"/a"}}}, Aliases: model.AliasSet{Added: map[string]string{"ll": "ls"}}, Functions: model.FuncSet{Added: []string{"f"}}}
	b := &model.Manifest{Schema: model.SchemaV1, Lists: []model.ListDelta{{Name: "PATH", Additions: []string{"/b"}}}}
	p, e := Diff(a, b)
	if e != nil || len(p.Deactivate) == 0 || len(p.Activate) == 0 {
		t.Fatal(e, p)
	}
	if _, ok := p.Deactivate[0].(RebuildListFromBase); !ok {
		t.Fatalf("%T", p.Deactivate[0])
	}
	if _, ok := p.Activate[0].(ApplyListDelta); !ok {
		t.Fatalf("%T", p.Activate[0])
	}
	for _, op := range p.Deactivate {
		if _, ok := op.(ApplyListDelta); ok {
			t.Fatal("activate op in deactivate")
		}
	}
	if len(p.Deactivate) != 7 {
		t.Logf("deactivate ops=%#v", p.Deactivate)
	}
}
func TestDiffSchemaGate(t *testing.T) {
	for _, which := range []int{0, 1} {
		a := &model.Manifest{Schema: model.SchemaV1}
		b := &model.Manifest{Schema: model.SchemaV1}
		if which == 0 {
			a.Schema = "v2"
		} else {
			b.Schema = "v2"
		}
		p, e := Diff(a, b)
		if e == nil || len(p.Deactivate) > 0 || len(p.Activate) > 0 {
			t.Fatal(which, p, e)
		}
	}
}
func TestDiffRestoreDerivedFromAdded(t *testing.T) {
	a := &model.Manifest{Schema: model.SchemaV1, Aliases: model.AliasSet{Added: map[string]string{"a": "x"}}, Functions: model.FuncSet{Added: []string{"f"}}}
	p, _ := Diff(a, nil)
	var ra, rf int
	for _, op := range p.Deactivate {
		switch op.(type) {
		case RestoreShadowedAlias:
			ra++
		case RestoreShadowedFunc:
			rf++
		}
	}
	if ra != 1 || rf != 1 {
		t.Fatal(p)
	}
}
