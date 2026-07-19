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

func TestDiffRejectsMalformedListMetadata(t *testing.T) {
	index := 2
	for _, list := range []model.ListDelta{
		{Name: "PATH", Additions: []string{"/a"}, BaseIndex: &index, AdditionDynamic: []bool{false}},
		{Name: "PATH", Additions: []string{"/a"}, AdditionDynamic: []bool{false}},
		{Name: "PATH", Additions: []string{"/a"}, BaseIndex: func() *int { i := 0; return &i }(), AdditionDynamic: nil},
	} {
		plan, err := Diff(nil, &model.Manifest{Schema: model.SchemaV1, Lists: []model.ListDelta{list}})
		if err == nil || len(plan.Activate) != 0 {
			t.Fatalf("metadata accepted: %#v %#v", list, plan)
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

func TestDiffUsesExplicitDynamicProvenance(t *testing.T) {
	literal := false
	dynamic := true
	target := &model.Manifest{
		Schema: model.SchemaV1,
		Env: []model.Scalar{
			{Name: "LITERAL", Applied: "$HOME", Dynamic: &literal},
			{Name: "DYNAMIC", Applied: "plain", Dynamic: &dynamic},
			{Name: "LEGACY", Applied: "$HOME"},
		},
		Aliases: model.AliasSet{
			Added:   map[string]string{"literal": "echo $HOME", "dynamic": "echo plain", "legacy": "echo $HOME"},
			Dynamic: map[string]bool{"literal": false, "dynamic": true},
		},
	}
	p, err := Diff(nil, target)
	if err != nil {
		t.Fatal(err)
	}
	scalars := map[string]bool{}
	aliases := map[string]bool{}
	for _, op := range p.Activate {
		switch op := op.(type) {
		case SetScalar:
			scalars[op.Name] = op.Dynamic
		case AddAlias:
			aliases[op.Name] = op.Dynamic
		}
	}
	if scalars["LITERAL"] || !scalars["DYNAMIC"] || !scalars["LEGACY"] {
		t.Fatalf("scalar provenance=%#v", scalars)
	}
	if aliases["literal"] || !aliases["dynamic"] || !aliases["legacy"] {
		t.Fatalf("alias provenance=%#v", aliases)
	}
}

func TestDiffFunctionBodyPresence(t *testing.T) {
	target := &model.Manifest{
		Schema:    model.SchemaV1,
		Functions: model.FuncSet{Added: []string{"empty", "multiline"}, Bodies: map[string]string{"empty": "", "multiline": "print one\nprint two"}},
	}
	p, err := Diff(nil, target)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, op := range p.Activate {
		if fn, ok := op.(AddFunc); ok {
			got[fn.Name] = fn.Body
		}
	}
	if body, ok := got["empty"]; !ok || body != "" || got["multiline"] != "print one\nprint two" {
		t.Fatalf("function bodies=%#v", got)
	}

	active := &model.Manifest{Schema: model.SchemaV1, Env: []model.Scalar{{Name: "OLD", Applied: "x"}}}
	bad := &model.Manifest{Schema: model.SchemaV1, Functions: model.FuncSet{Added: []string{"missing"}}}
	p, err = Diff(active, bad)
	if err == nil || len(p.Deactivate) != 0 || len(p.Activate) != 0 {
		t.Fatalf("missing target body did not reject whole plan: %#v %v", p, err)
	}

	legacyActive := &model.Manifest{Schema: model.SchemaV1, Functions: model.FuncSet{Added: []string{"old"}}}
	p, err = Diff(legacyActive, nil)
	if err != nil || len(p.Deactivate) != 2 || len(p.Activate) != 0 {
		t.Fatalf("legacy deactivation blocked: %#v %v", p, err)
	}
}

func TestDiffNormalizesDuplicateFunctionIdentitiesAndExportProvenance(t *testing.T) {
	exported := false
	target := &model.Manifest{
		Schema: model.SchemaV1,
		Env: []model.Scalar{
			{Name: "PLAIN", Applied: "one", Exported: &exported},
			{Name: "PLAIN", Applied: "two", Exported: &exported},
		},
		Functions: model.FuncSet{Added: []string{"dup", "dup"}, Bodies: map[string]string{"dup": "print final"}},
	}
	active := &model.Manifest{Schema: model.SchemaV1, Functions: model.FuncSet{Added: []string{"dup", "dup"}, Bodies: map[string]string{"dup": "print old"}}}
	p, err := Diff(active, target)
	if err != nil {
		t.Fatal(err)
	}
	var unset, restore, set int
	for _, op := range p.Deactivate {
		switch op.(type) {
		case UnsetFunc:
			unset++
		case RestoreShadowedFunc:
			restore++
		}
	}
	for _, op := range p.Activate {
		if scalar, ok := op.(SetScalar); ok {
			set++
			if scalar.Name != "PLAIN" || scalar.Applied != "two" || scalar.Exported {
				t.Fatalf("scalar provenance=%#v", scalar)
			}
		}
	}
	if unset != 1 || restore != 1 || set != 1 {
		t.Fatalf("duplicate identities produced repeated operations: %#v", p)
	}
}
