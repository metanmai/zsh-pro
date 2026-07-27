package store

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

// mixedProfile is a representative profile covering every declarative class plus
// both override forms — the lossless-round-trip fixture (D-01, Pitfall 6).
func mixedProfile() model.Profile {
	return model.Profile{Entries: []model.Entry{
		{ // static env
			Text: "export EDITOR=nvim", StartLine: 1, Category: model.CatEnvironment,
			Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim",
			Exported: true, Managed: true, Override: model.OverrideAuto,
		},
		{ // dynamic env ($HOME) — must survive verbatim, never resolved
			Text: "export GOPATH=$HOME/go", StartLine: 2, Category: model.CatEnvironment,
			Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go",
			Exported: true, Managed: true, Override: model.OverrideAuto, Dynamic: true,
		},
		{ // alias
			Text: "alias gs='git status'", StartLine: 3, Category: model.CatAliases,
			Kind: model.KindAlias, CmdName: "alias", Names: []string{"gs"}, Value: "git status",
			Managed: true, Override: model.OverrideAuto,
		},
		{ // setopt option
			Text: "setopt EXTENDED_GLOB", StartLine: 4, Category: model.CatOptions,
			Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"},
			Managed: true, Override: model.OverrideAuto,
		},
		{ // function
			Text: "greet() { echo hi }", StartLine: 5, Category: model.CatFunctions,
			Kind: model.KindFuncDecl, Names: []string{"greet"}, Value: "{ echo hi }",
			Managed: true, Override: model.OverrideAuto,
		},
		{ // forced-managed override
			Text: "weird_thing", StartLine: 6, Category: model.CatMisc,
			Kind: model.KindCommand, CmdName: "weird_thing", Override: model.OverrideManaged,
		},
		{ // forced-unmanaged override
			Text: "alias x=y", StartLine: 7, Category: model.CatAliases,
			Kind: model.KindCommand, CmdName: "alias", Names: []string{"x"}, Value: "y",
			Managed: true, Override: model.OverrideUnmanaged,
		},
	}}
}

func TestRoundTripPersistedOverrideManagedDeclarations(t *testing.T) {
	cases := []struct {
		name string
		src  string
		cmd  string
	}{
		{name: "integer", src: "typeset -i COUNT=2\n", cmd: "typeset"},
		{name: "readonly", src: "readonly LOCKED=value\n", cmd: "readonly"},
		{name: "tied", src: "typeset -T PATH path\n", cmd: "typeset"},
		{name: "local", src: "local scoped=value\n", cmd: "local"},
	}
	provider := zsh.Provider{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := provider.Parse([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			profile := ir.Build(blocks, provider)
			if len(profile.Entries) != 1 {
				t.Fatalf("profile=%#v", profile)
			}
			profile.Entries[0].Override = model.OverrideManaged
			payload, err := MarshalProfile(profile)
			if err != nil {
				t.Fatal(err)
			}
			got, err := UnmarshalProfile(payload)
			if err != nil {
				t.Fatal(err)
			}
			entry := got.Entries[0]
			if entry.Text != strings.TrimSuffix(tc.src, "\n") || entry.CmdName != tc.cmd || entry.Override != model.OverrideManaged {
				t.Fatalf("round trip lost declaration state: %#v", entry)
			}
		})
	}
}

func TestRoundTripStructuralFidelity(t *testing.T) {
	profile := model.Profile{Entries: []model.Entry{
		{Text: "FOO=bar", Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "bar", StructuralFidelityKnown: true},
		{Text: "FOO+=baz", Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "baz", StructuralFidelityKnown: true, Append: true},
		{Text: "plugins=(git zsh-autosuggestions)", Kind: model.KindAssignment, Names: []string{"plugins"}, StructuralFidelityKnown: true, Array: true},
		{Text: "alias -g G='| grep'", Kind: model.KindAlias, CmdName: "alias", Names: []string{"G"}, Value: "| grep", StructuralFidelityKnown: true, AliasAssignment: true, Flagged: true},
		{Text: "FOO[2]=bar", Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "bar", StructuralFidelityKnown: true, Indexed: true},
		{Text: "export -i COUNT=1", Kind: model.KindAssignment, CmdName: "export", Names: []string{"COUNT"}, Value: "1", Exported: true, StructuralFidelityKnown: true, DeclarationFlags: []string{"-i"}},
		{Text: "export -- FOO=1", Kind: model.KindAssignment, CmdName: "export", Names: []string{"FOO"}, Value: "1", Exported: true, StructuralFidelityKnown: true, DeclarationFlags: []string{}, OptionFlags: []string{}},
		{Text: "setopt -o EXTENDED_GLOB", Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}, StructuralFidelityKnown: true, OptionFlags: []string{"-o"}},
	}}

	payload, err := MarshalProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(payload, []byte(`"structuralFidelity"`)); got != len(profile.Entries) {
		t.Fatalf("structural fidelity objects=%d, want %d:\n%s", got, len(profile.Entries), payload)
	}
	for _, marker := range []string{`"version"`, `"append"`, `"array"`, `"flagged"`, `"indexed"`, `"aliasAssignment"`, `"declarationFlags"`, `"optionFlags"`} {
		if !bytes.Contains(payload, []byte(marker)) {
			t.Fatalf("payload missing %s:\n%s", marker, payload)
		}
	}
	got, err := UnmarshalProfile(payload)
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range got.Entries {
		if !entry.StructuralFidelityKnown {
			t.Fatalf("entry %d lost known structural fidelity: %#v", i, entry)
		}
	}
	if !got.Entries[3].AliasAssignment || !got.Entries[4].Indexed || !reflect.DeepEqual(got.Entries[5].DeclarationFlags, []string{"-i"}) || !reflect.DeepEqual(got.Entries[6].DeclarationFlags, []string{}) || !reflect.DeepEqual(got.Entries[7].OptionFlags, []string{"-o"}) {
		t.Fatalf("new structural metadata did not round-trip: %#v", got.Entries[4:])
	}
}

func TestStructuralFidelityDTOCompatibilityMatrix(t *testing.T) {
	entry := func(fidelity string) string {
		return `{"entries":[{"text":"export FOO=bar","startLine":1,"category":"environment","kind":"assignment","cmdName":"export","names":["FOO"],"value":"bar","exported":true,"managed":false,"override":"forced-managed","dynamic":false` + fidelity + `}]}`
	}
	rows := []struct {
		name string
		raw  string
	}{
		{name: "absent", raw: entry("")},
		{name: "complete v1", raw: entry(`,"structuralFidelity":{"version":1,"append":false,"array":false,"flagged":false}`)},
		{name: "complete v2", raw: entry(`,"structuralFidelity":{"version":2,"append":false,"array":false,"flagged":false,"indexed":false,"declarationFlags":[]}`)},
		{name: "v3 append omitted", raw: entry(`,"structuralFidelity":{"version":3,"array":false,"flagged":false,"indexed":false,"aliasAssignment":false,"declarationFlags":[],"optionFlags":{"values":null}}`)},
		{name: "v3 alias assignment omitted", raw: entry(`,"structuralFidelity":{"version":3,"append":false,"array":false,"flagged":false,"indexed":false,"declarationFlags":[],"optionFlags":{"values":null}}`)},
		{name: "v3 option flags omitted", raw: entry(`,"structuralFidelity":{"version":3,"append":false,"array":false,"flagged":false,"indexed":false,"aliasAssignment":false,"declarationFlags":[]}`)},
		{name: "unsupported version", raw: entry(`,"structuralFidelity":{"version":99,"append":false,"array":false,"flagged":false,"indexed":false,"aliasAssignment":false,"declarationFlags":[],"optionFlags":{"values":null}}`)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			first, err := UnmarshalProfile([]byte(row.raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Entries) != 1 || first.Entries[0].StructuralFidelityKnown || first.Entries[0].Representable() {
				t.Fatalf("first decode inferred structural state: %#v", first)
			}
			saved, err := MarshalProfile(first)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(saved, []byte(`"structuralFidelity"`)) {
				t.Fatalf("historical fidelity was inferred on re-save:\n%s", saved)
			}
			second, err := UnmarshalProfile(saved)
			if err != nil {
				t.Fatal(err)
			}
			if len(second.Entries) != 1 || second.Entries[0].StructuralFidelityKnown || second.Entries[0].Representable() {
				t.Fatalf("re-saved decode inferred structural state: %#v", second)
			}
		})
	}
}

// TestRoundTripLossless proves UnmarshalProfile(MarshalProfile(p)) == p for a
// profile mixing all declarative classes + both overrides: every derived field
// (ManagedOverride, Dynamic, Category, Managed, Names, Value, Text) survives (D-01).
func TestRoundTripLossless(t *testing.T) {
	in := mixedProfile()
	b, err := MarshalProfile(in)
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	out, err := UnmarshalProfile(b)
	if err != nil {
		t.Fatalf("UnmarshalProfile: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round-trip not lossless:\n in=%+v\nout=%+v", in, out)
	}
}

// TestMarshalDeterministic proves the same Profile marshals to byte-identical
// output across calls (D-01 stable-diff requirement; encoding/json declaration
// order + sorted map keys make this hold).
func TestMarshalDeterministic(t *testing.T) {
	p := mixedProfile()
	a, err := MarshalProfile(p)
	if err != nil {
		t.Fatalf("MarshalProfile #1: %v", err)
	}
	b, err := MarshalProfile(p)
	if err != nil {
		t.Fatalf("MarshalProfile #2: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("MarshalProfile not deterministic:\n#1=%s\n#2=%s", a, b)
	}
}

// TestMarshalTrailingNewline proves the payload ends in exactly one '\n' (D-01
// POSIX-friendly stable-diff convention).
func TestMarshalTrailingNewline(t *testing.T) {
	b, err := MarshalProfile(mixedProfile())
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		t.Fatalf("payload must end in a newline; tail=%q", tail(b))
	}
	if len(b) >= 2 && b[len(b)-2] == '\n' {
		t.Errorf("payload must end in exactly ONE newline; tail=%q", tail(b))
	}
}

// TestMarshalNoHomeResolution proves a dynamic Value passes through verbatim:
// the bytes carry the literal "$HOME/go", never a resolved absolute path
// (T-03-02 — no util.ExpandHome in the store).
func TestMarshalNoHomeResolution(t *testing.T) {
	p := model.Profile{Entries: []model.Entry{{
		Text: "export GOPATH=$HOME/go", Category: model.CatEnvironment,
		Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go",
		Exported: true, Dynamic: true, Override: model.OverrideAuto,
	}}}
	b, err := MarshalProfile(p)
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	if !bytes.Contains(b, []byte(`$HOME/go`)) {
		t.Errorf("dynamic value resolved away: bytes lack literal $HOME/go: %s", b)
	}
	if bytes.Contains(b, []byte("/Users/")) || bytes.Contains(b, []byte("/home/")) {
		t.Errorf("dynamic value resolved to an absolute home path: %s", b)
	}
}

// TestRoundTripSecretRef proves an entry carrying a SecretRef with a cleared
// Value round-trips: Kind/Key survive and Value stays "" (T-03-03 — the literal
// never round-trips; the reference does).
func TestRoundTripSecretRef(t *testing.T) {
	in := model.Profile{Entries: []model.Entry{{
		Text:     "export API_KEY=keychain:API_KEY", // post-exclusion placeholder text
		Category: model.CatSecrets, Kind: model.KindAssignment,
		Names: []string{"API_KEY"}, Value: "", Exported: true, Override: model.OverrideAuto,
		Secret: &model.SecretRef{Kind: model.SecretRefKeychain, Key: "API_KEY"},
	}}}
	b, err := MarshalProfile(in)
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	out, err := UnmarshalProfile(b)
	if err != nil {
		t.Fatalf("UnmarshalProfile: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("SecretRef round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	got := out.Entries[0]
	if got.Secret == nil {
		t.Fatal("SecretRef dropped on round-trip")
	}
	if got.Secret.Kind != model.SecretRefKeychain || got.Secret.Key != "API_KEY" {
		t.Errorf("SecretRef kind/key not preserved: %+v", got.Secret)
	}
	if got.Value != "" {
		t.Errorf("secret literal Value must stay empty, got %q", got.Value)
	}
}

// TestMarshalStableLowercaseKeys proves every Entry field is present in the JSON
// under a stable lowercase key (the DTO carries the tags, not model.Entry).
func TestMarshalStableLowercaseKeys(t *testing.T) {
	b, err := MarshalProfile(mixedProfile())
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	for _, key := range []string{
		`"entries"`, `"text"`, `"startLine"`, `"category"`, `"kind"`,
		`"cmdName"`, `"names"`, `"value"`, `"exported"`, `"managed"`,
		`"override"`, `"dynamic"`,
	} {
		if !bytes.Contains(b, []byte(key)) {
			t.Errorf("expected JSON key %s missing from payload:\n%s", key, b)
		}
	}
}

// TestUnmarshalNamesNotAliased proves Names is reconstructed as a fresh slice on
// decode — mutating the result must not corrupt any shared backing array.
func TestUnmarshalNamesNotAliased(t *testing.T) {
	in := model.Profile{Entries: []model.Entry{{
		Text: "export MULTA=one MULTB=two", Category: model.CatEnvironment,
		Kind: model.KindAssignment, Names: []string{"MULTA", "MULTB"},
		Value: "one MULTB=two", Exported: true, Override: model.OverrideAuto,
	}}}
	b, err := MarshalProfile(in)
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	out, err := UnmarshalProfile(b)
	if err != nil {
		t.Fatalf("UnmarshalProfile: %v", err)
	}
	// Mutating the decoded slice must not touch the input's backing array.
	out.Entries[0].Names[0] = "MUTATED"
	if in.Entries[0].Names[0] != "MULTA" {
		t.Errorf("Names slice aliased across round-trip: input corrupted to %q", in.Entries[0].Names[0])
	}
}

// TestRoundTripSemanticFields pins the additive parser contract through the
// store, including pointer-present empty strings and modes without values.
func TestRoundTripSemanticFields(t *testing.T) {
	empty := ""
	literal := "decoded"
	multiline := "\n\tprint one\n\tprint two\n"
	in := model.Profile{Entries: []model.Entry{
		{Text: "FOO=''", Value: "''", ValueMode: model.ValueModeLiteral, RuntimeValue: &empty},
		{Text: "FOO='decoded'", Value: "'decoded'", ValueMode: model.ValueModeLiteral, RuntimeValue: &literal},
		{Text: "FOO=$HOME", Value: "$HOME", Dynamic: true, ValueMode: model.ValueModeDynamic},
		{Text: "FOO=$'\\n'", Value: "$'\\n'", ValueMode: model.ValueModeUnsupported},
		{Text: "empty(){}", Value: "empty(){}", FunctionBody: &empty},
		{Text: "multi() {\n\tprint one\n\tprint two\n}", Value: "multi() {\n\tprint one\n\tprint two\n}", FunctionBody: &multiline},
	}}

	b, err := MarshalProfile(in)
	if err != nil {
		t.Fatalf("MarshalProfile: %v", err)
	}
	for _, key := range []string{`"valueMode"`, `"runtimeValue"`, `"functionBody"`} {
		if !bytes.Contains(b, []byte(key)) {
			t.Errorf("semantic JSON key %s missing from payload:\n%s", key, b)
		}
	}
	if !bytes.Contains(b, []byte(`"runtimeValue": ""`)) {
		t.Errorf("present empty RuntimeValue disappeared from JSON:\n%s", b)
	}
	if !bytes.Contains(b, []byte(`"functionBody": ""`)) {
		t.Errorf("present empty FunctionBody disappeared from JSON:\n%s", b)
	}

	out, err := UnmarshalProfile(b)
	if err != nil {
		t.Fatalf("UnmarshalProfile: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("semantic round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	a, err := MarshalProfile(out)
	if err != nil {
		t.Fatalf("MarshalProfile repeat: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("semantic profile marshal not deterministic:\n#1=%s\n#2=%s", b, a)
	}
}

// TestUnmarshalLegacySemanticFields proves old profile.json bytes acquire only
// the explicit zero-value legacy mode; no runtime/body presence is invented.
func TestUnmarshalLegacySemanticFields(t *testing.T) {
	legacy := []byte(`{
  "entries": [
    {
      "text": "export EDITOR=nvim",
      "startLine": 1,
      "category": "environment",
      "kind": "assignment",
      "cmdName": "export",
      "names": ["EDITOR"],
      "value": "nvim",
      "exported": true,
      "managed": true,
      "override": "auto",
      "dynamic": false
    }
  ]
}
`)

	out, err := UnmarshalProfile(legacy)
	if err != nil {
		t.Fatalf("UnmarshalProfile legacy: %v", err)
	}
	if len(out.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(out.Entries))
	}
	got := out.Entries[0]
	if got.ValueMode != model.ValueModeLegacy {
		t.Errorf("legacy ValueMode = %q, want zero legacy", got.ValueMode)
	}
	if got.RuntimeValue != nil || got.FunctionBody != nil {
		t.Errorf("legacy entry invented semantic presence: RuntimeValue=%v FunctionBody=%v", got.RuntimeValue, got.FunctionBody)
	}
	if got.Value != "nvim" || got.Dynamic {
		t.Errorf("legacy Value/Dynamic changed: Value=%q Dynamic=%v", got.Value, got.Dynamic)
	}
}

// TestSemanticPointersNotAliased pins defensive copies in both DTO directions.
func TestSemanticPointersNotAliased(t *testing.T) {
	runtimeValue := "runtime"
	functionBody := "\n\tprint body\n"
	in := model.Entry{RuntimeValue: &runtimeValue, FunctionBody: &functionBody}
	dto := toEntryDTO(in)
	if dto.RuntimeValue == in.RuntimeValue || dto.FunctionBody == in.FunctionBody {
		t.Fatal("toEntryDTO aliased semantic pointers")
	}
	*dto.RuntimeValue = "dto runtime"
	*dto.FunctionBody = "dto body"
	if *in.RuntimeValue != "runtime" || *in.FunctionBody != "\n\tprint body\n" {
		t.Fatal("mutating DTO semantic pointers changed model Entry")
	}

	out := fromEntryDTO(dto)
	if out.RuntimeValue == dto.RuntimeValue || out.FunctionBody == dto.FunctionBody {
		t.Fatal("fromEntryDTO aliased semantic pointers")
	}
	*out.RuntimeValue = "entry runtime"
	*out.FunctionBody = "entry body"
	if *dto.RuntimeValue != "dto runtime" || *dto.FunctionBody != "dto body" {
		t.Fatal("mutating decoded Entry semantic pointers changed DTO")
	}
}

func TestRoundTripListValueContract(t *testing.T) {
	in := model.Profile{Entries: []model.Entry{{
		Text:  `PATH="$EXTRA:/literal:$PATH"`,
		Value: `"$EXTRA:/literal:$PATH"`,
		ListValue: &model.ListValue{Segments: []model.ListSegment{
			{Dynamic: true, Source: "$EXTRA"},
			{Value: "/literal"},
			{Self: true},
		}},
	}}}
	b, err := MarshalProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"listValue"`, `"dynamic": true`, `"dynamic": false`, `"self": true`, `"self": false`, `"source": "$EXTRA"`} {
		if !bytes.Contains(b, []byte(fragment)) {
			t.Fatalf("JSON missing %s:\n%s", fragment, b)
		}
	}
	out, err := UnmarshalProfile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip mismatch:\n in=%#v\nout=%#v", in, out)
	}
	again, err := MarshalProfile(out)
	if err != nil || !bytes.Equal(again, b) {
		t.Fatalf("repeat marshal=%s err=%v, want %s", again, err, b)
	}

	dto := toEntryDTO(in.Entries[0])
	if dto.ListValue == nil {
		t.Fatal("toEntryDTO dropped ListValue")
	}
	dto.ListValue.Segments[0].Source = "$MUTATED"
	if in.Entries[0].ListValue.Segments[0].Source != "$EXTRA" {
		t.Fatal("mutating DTO ListValue changed Entry")
	}
	decoded := fromEntryDTO(dto)
	decoded.ListValue.Segments[0].Source = "$ENTRY"
	if dto.ListValue.Segments[0].Source != "$MUTATED" {
		t.Fatal("mutating Entry ListValue changed DTO")
	}
}

func TestListValueDTOOmissionAndMalformedDynamicFailClosed(t *testing.T) {
	legacy := []byte(`{"entries":[{"text":"PATH=$PATH","startLine":1,"category":"path","kind":"assignment","cmdName":"","names":["PATH"],"value":"$PATH","exported":false,"managed":true,"override":"auto","dynamic":true}]}`)
	out, err := UnmarshalProfile(legacy)
	if err != nil || out.Entries[0].ListValue != nil {
		t.Fatalf("legacy list contract=%#v err=%v", out.Entries[0].ListValue, err)
	}

	malformed := []byte(`{"entries":[{"text":"PATH=$EXTRA:$PATH","startLine":1,"category":"path","kind":"assignment","cmdName":"","names":["PATH"],"value":"$EXTRA:$PATH","exported":false,"managed":true,"override":"auto","dynamic":true,"listValue":{"segments":[{"value":"","dynamic":true,"self":false},{"value":"","dynamic":false,"self":true}]}}]}`)
	out, err = UnmarshalProfile(malformed)
	if err != nil || out.Entries[0].ListValue != nil {
		t.Fatalf("malformed list contract=%#v err=%v", out.Entries[0].ListValue, err)
	}

	selfOnly := model.Profile{Entries: []model.Entry{{ListValue: &model.ListValue{Segments: []model.ListSegment{{Self: true}}}}}}
	b, err := MarshalProfile(selfOnly)
	if err != nil {
		t.Fatal(err)
	}
	out, err = UnmarshalProfile(b)
	if err != nil || out.Entries[0].ListValue == nil || len(out.Entries[0].ListValue.Segments) != 1 || !out.Entries[0].ListValue.Segments[0].Self {
		t.Fatalf("self-only contract=%#v err=%v", out.Entries[0].ListValue, err)
	}
}

func tail(b []byte) []byte {
	if len(b) <= 12 {
		return b
	}
	return b[len(b)-12:]
}
