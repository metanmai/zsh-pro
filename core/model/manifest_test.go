package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	empty := ""
	m := Manifest{Profile: "work", Schema: SchemaV1, Env: []Scalar{{Name: "A", Applied: "x", Original: func() *string { s := "old"; return &s }()}, {Name: "B", Applied: "y"}, {Name: "C", Applied: "z", Original: &empty}}, Lists: []ListDelta{{Name: "PATH", Additions: []string{"/x"}, Deletions: []string{}}, {Name: "FPATH", Additions: []string{"/f"}, Deletions: []string{"/old"}}}, Aliases: AliasSet{Added: map[string]string{"gs": "git status"}, Shadowed: map[string]string{"ll": "ls"}}, Functions: FuncSet{Added: []string{"work_deploy"}, Shadowed: map[string]string{"ff": "print hi"}}, Options: []OptionSet{{Name: "EXTENDED_GLOB", Enabled: true}}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Manifest
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, got) {
		t.Fatalf("round trip mismatch: %#v %#v", m, got)
	}
}

func TestManifestKeysAndTriState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		original *string
		want     bool
		value    string
	}{
		{"unset", nil, false, ""}, {"empty", func() *string { s := ""; return &s }(), true, ""}, {"value", func() *string { s := "vim"; return &s }(), true, "vim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(Scalar{Name: "X", Applied: "nvim", Original: tc.original})
			has := strings.Contains(string(b), "original")
			if has != tc.want {
				t.Fatalf("json=%s", b)
			}
			var s Scalar
			_ = json.Unmarshal(b, &s)
			if !reflect.DeepEqual(tc.original, s.Original) || (tc.want && s.Original != nil && *s.Original != tc.value) {
				t.Fatalf("got %#v", s.Original)
			}
		})
	}
	var m Manifest
	_ = json.Unmarshal([]byte(`{"profile":"work","schema":"v1","env":[],"lists":[],"aliases":{"added":{"gs":"git status"},"shadowed":{}},"functions":{"added":["work_deploy"],"shadowed":{}},"options":[]}`), &m)
	if m.Aliases.Added["gs"] != "git status" || len(m.Functions.Added) != 1 {
		t.Fatal("fixture did not decode")
	}
}

func TestManifestSemanticProvenanceRoundTrip(t *testing.T) {
	literal := false
	m := Manifest{
		Profile: "work",
		Schema:  SchemaV1,
		Env: []Scalar{{
			Name:    "LITERAL",
			Applied: "$HOME",
			Dynamic: &literal,
		}},
		Aliases: AliasSet{
			Added:   map[string]string{"literal": "echo $HOME"},
			Dynamic: map[string]bool{"literal": false},
		},
		Functions: FuncSet{
			Added:  []string{"empty", "multiline"},
			Bodies: map[string]string{"empty": "", "multiline": "print one\nprint two"},
		},
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Manifest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Env[0].Dynamic == nil || *got.Env[0].Dynamic {
		t.Fatalf("literal scalar provenance lost: %#v", got.Env[0].Dynamic)
	}
	if dynamic, ok := got.Aliases.Dynamic["literal"]; !ok || dynamic {
		t.Fatalf("literal alias provenance lost: %#v", got.Aliases.Dynamic)
	}
	if body, ok := got.Functions.Bodies["empty"]; !ok || body != "" {
		t.Fatalf("present-empty function body lost: %#v", got.Functions.Bodies)
	}
	if !reflect.DeepEqual(m, got) {
		t.Fatalf("round trip mismatch:\nwant %#v\n got %#v", m, got)
	}
}

func TestManifestAdditiveSemanticFieldsRemainCompatible(t *testing.T) {
	const legacy = `{"profile":"work","schema":"v1","env":[{"name":"HOME_COPY","applied":"$HOME"}],"lists":[],"aliases":{"added":{"home":"echo $HOME"},"shadowed":{}},"functions":{"added":["empty"],"shadowed":{}},"options":[]}`
	var got Manifest
	if err := json.Unmarshal([]byte(legacy), &got); err != nil {
		t.Fatal(err)
	}
	if got.Env[0].Dynamic != nil || got.Aliases.Dynamic != nil || got.Functions.Bodies != nil {
		t.Fatalf("legacy manifest inferred additive fields: %#v", got)
	}

	b, err := json.Marshal(Manifest{
		Profile:   "work",
		Schema:    SchemaV1,
		Aliases:   AliasSet{Added: map[string]string{}, Shadowed: map[string]string{}, Dynamic: map[string]bool{}},
		Functions: FuncSet{Added: []string{}, Shadowed: map[string]string{}, Bodies: map[string]string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"dynamic"`) || strings.Contains(string(b), `"bodies"`) {
		t.Fatalf("empty additive fields changed legacy shape: %s", b)
	}
}
