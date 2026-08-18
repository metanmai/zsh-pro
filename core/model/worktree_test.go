package model

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func strptr(value string) *string { return &value }

func TestLiveValuePresenceCompatibilityAndEquality(t *testing.T) {
	tests := []struct {
		name    string
		kind    LiveKind
		empty   LiveValue
		changed LiveValue
	}{
		{name: "env", kind: LiveEnv, empty: ScalarLiveValue(""), changed: ScalarLiveValue("x")},
		{name: "alias", kind: LiveAlias, empty: ScalarLiveValue(""), changed: ScalarLiveValue("print hi")},
		{name: "function", kind: LiveFunction, empty: ScalarLiveValue(""), changed: ScalarLiveValue("print hi")},
		{name: "path", kind: LivePath, empty: ListLiveValue(nil), changed: ListLiveValue([]string{"", "/bin", "/bin"})},
		{name: "fpath", kind: LiveFPath, empty: ListLiveValue([]string{}), changed: ListLiveValue([]string{"/functions"})},
		{name: "option", kind: LiveOption, empty: OptionLiveValue(false), changed: OptionLiveValue(true)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := Identity{Kind: test.kind, Name: liveTestName(test.kind)}
			if err := ValidateLiveIdentityState(LiveIdentityState{Identity: identity, Value: test.empty}); err != nil {
				t.Fatalf("present-empty state rejected: %v", err)
			}
			removed := RemovedLiveValue()
			if err := ValidateLiveIdentityState(LiveIdentityState{Identity: identity, Value: removed}); err != nil {
				t.Fatalf("removed state rejected: %v", err)
			}
			if EqualLiveValue(test.kind, test.empty, removed) {
				t.Fatal("present-empty compared equal to removal")
			}
			if EqualLiveValue(test.kind, test.empty, test.changed) {
				t.Fatal("changed value compared equal to present-empty")
			}
			if !EqualLiveValue(test.kind, test.empty, CloneLiveValue(test.empty)) {
				t.Fatal("exact semantic clone did not compare equal")
			}
		})
	}

	ordered := ListLiveValue([]string{"/a", "", "/b", "/a"})
	reordered := ListLiveValue([]string{"/a", "/b", "", "/a"})
	if EqualLiveValue(LivePath, ordered, reordered) {
		t.Fatal("ordered list equality ignored element order")
	}
	if EqualLiveValue(LiveEnv, ScalarLiveValue("x\n"), ScalarLiveValue("x")) {
		t.Fatal("scalar equality was not exact")
	}
	if EqualLiveValue(LiveFunction, ScalarLiveValue("print x\n"), ScalarLiveValue("print x")) {
		t.Fatal("function equality was not exact")
	}
}

func TestShellCapabilityVerifierAndCredentialSerializationContract(t *testing.T) {
	raw := bytes.Repeat([]byte{0x5a}, ShellCapabilityBytes)
	capability, err := NewShellCapability(raw)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := DeriveShellCapabilityVerifier(capability)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyShellCapability(verifier, capability) {
		t.Fatal("derived verifier rejected its capability")
	}
	otherRaw := append([]byte(nil), raw...)
	otherRaw[len(otherRaw)-1] ^= 1
	other, err := NewShellCapability(otherRaw)
	if err != nil {
		t.Fatal(err)
	}
	if VerifyShellCapability(verifier, other) {
		t.Fatal("verifier accepted a mismatched capability")
	}
	if _, err := NewShellCapability(raw[:len(raw)-1]); err == nil {
		t.Fatal("short capability accepted")
	}

	credential := ShellCredential{ShellID: "shell-1", Capability: capability}
	if err := credential.Validate(); err != nil {
		t.Fatalf("credential rejected: %v", err)
	}
	if encoded, err := json.Marshal(credential); err == nil || bytes.Contains(encoded, raw) {
		t.Fatalf("credential serialized raw bearer: %q, %v", encoded, err)
	}
	if rendered := credential.String(); strings.Contains(rendered, "ZZZZ") {
		t.Fatalf("credential formatter exposed bearer bytes: %s", rendered)
	}
}

func TestLiveIdentityValidationRejectsUnknownUnsafeAndMismatchedValues(t *testing.T) {
	valid := []Identity{
		{Kind: LiveEnv, Name: "EDITOR"},
		{Kind: LiveAlias, Name: "g.co"},
		{Kind: LiveFunction, Name: "deploy-prod"},
		{Kind: LivePath, Name: "PATH"},
		{Kind: LiveFPath, Name: "FPATH"},
		{Kind: LiveOption, Name: "EXTENDED_GLOB"},
	}
	for _, identity := range valid {
		if err := ValidateIdentity(identity); err != nil {
			t.Errorf("ValidateIdentity(%#v): %v", identity, err)
		}
	}

	invalid := []Identity{
		{},
		{Kind: LiveKind("pwd"), Name: "PWD"},
		{Kind: LiveEnv, Name: "1BAD"},
		{Kind: LiveAlias, Name: "bad name"},
		{Kind: LiveFunction, Name: "bad\x00name"},
		{Kind: LivePath, Name: "path"},
		{Kind: LiveFPath, Name: "PATH"},
		{Kind: LiveOption, Name: "bad-option"},
	}
	for _, identity := range invalid {
		if err := ValidateIdentity(identity); err == nil {
			t.Errorf("ValidateIdentity(%#v) unexpectedly succeeded", identity)
		}
	}

	bad := []LiveIdentityState{
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ListLiveValue([]string{"x"})},
		{Identity: Identity{Kind: LivePath, Name: "PATH"}, Value: ScalarLiveValue("/bin")},
		{Identity: Identity{Kind: LiveOption, Name: "NO_BEEP"}, Value: ScalarLiveValue("true")},
		{Identity: Identity{Kind: LiveAlias, Name: "ll"}, Value: ScalarLiveValue("a\x00b")},
		{Identity: Identity{Kind: LivePath, Name: "PATH"}, Value: ListLiveValue([]string{"/bin", "bad\x00path"})},
	}
	for _, state := range bad {
		if err := ValidateLiveIdentityState(state); err == nil {
			t.Errorf("ValidateLiveIdentityState(%#v) unexpectedly succeeded", state)
		}
	}
}

func TestLiveNormalizationFinalOccurrenceTombstoneAndDefensiveCopies(t *testing.T) {
	path := []string{"/one", "", "/one"}
	states := []LiveIdentityState{
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("old")},
		{Identity: Identity{Kind: LiveAlias, Name: "ll"}, Value: ScalarLiveValue("ls")},
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("new")},
		{Identity: Identity{Kind: LivePath, Name: "PATH"}, Value: ListLiveValue(path)},
	}
	normalized, err := NormalizeLiveStates(states)
	if err != nil {
		t.Fatal(err)
	}
	want := []LiveIdentityState{
		{Identity: Identity{Kind: LiveAlias, Name: "ll"}, Value: ScalarLiveValue("ls")},
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("new")},
		{Identity: Identity{Kind: LivePath, Name: "PATH"}, Value: ListLiveValue([]string{"/one", "", "/one"})},
	}
	if !reflect.DeepEqual(normalized, want) {
		t.Fatalf("NormalizeLiveStates() = %#v, want %#v", normalized, want)
	}
	path[0] = "/mutated"
	states[3].Value.List[1] = "/also-mutated"
	if !reflect.DeepEqual(normalized[2].Value.List, []string{"/one", "", "/one"}) {
		t.Fatalf("normalized list aliases caller memory: %#v", normalized[2].Value.List)
	}

	overlay, err := NormalizeOverlay([]OverlayEntry{
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("shadowed")},
		{Identity: Identity{Kind: LiveAlias, Name: "ll"}, Value: ScalarLiveValue("ls -l")},
		{Identity: Identity{Kind: LiveEnv, Name: "A"}, Tombstone: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay) != 2 || overlay[1].Identity.Name != "A" || !overlay[1].Tombstone || overlay[1].Value.Present {
		t.Fatalf("final tombstone did not win: %#v", overlay)
	}
}

func TestSnapshotExactBoundsAndRevisionOverflow(t *testing.T) {
	for _, bytes := range []uint64{0, 1, MaxSnapshotBytes} {
		if err := ValidateLiveSnapshot(LiveSnapshot{ByteSize: bytes}); err != nil {
			t.Fatalf("ByteSize %d rejected: %v", bytes, err)
		}
	}
	if err := ValidateLiveSnapshot(LiveSnapshot{ByteSize: MaxSnapshotBytes + 1}); err == nil {
		t.Fatal("snapshot byte cap plus one accepted")
	}

	state := LiveIdentityState{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("x")}
	atCap := make([]LiveIdentityState, MaxSnapshotRecords)
	for i := range atCap {
		atCap[i] = state
	}
	if err := ValidateLiveSnapshot(LiveSnapshot{ByteSize: MaxSnapshotBytes, States: atCap}); err != nil {
		t.Fatalf("record cap rejected: %v", err)
	}
	if err := ValidateLiveSnapshot(LiveSnapshot{States: append(atCap, state)}); err == nil {
		t.Fatal("record cap plus one accepted")
	}

	if got, err := NextRevision(math.MaxUint64 - 1); err != nil || got != math.MaxUint64 {
		t.Fatalf("NextRevision(MaxUint64-1) = %d, %v", got, err)
	}
	if got, err := NextRevision(math.MaxUint64); err == nil || got != math.MaxUint64 {
		t.Fatalf("NextRevision(MaxUint64) = %d, %v", got, err)
	}
}

func TestCommittedWorktreeKeepsCompleteSourceSeparateFromProjection(t *testing.T) {
	before := "old"
	source := Profile{Entries: []Entry{
		{Kind: KindAssignment, Names: []string{"A"}, ValueMode: ValueModeLiteral, RuntimeValue: &before},
		{Kind: KindAssignment, Names: []string{"A"}, ValueMode: ValueModeLiteral, RuntimeValue: strptr("new")},
	}}
	doc := NewCommittedWorktree(source, LiveProjection{
		Schema:     WorktreeSchemaV1,
		States:     []LiveIdentityState{{Identity: Identity{Kind: LiveEnv, Name: "A"}, Value: ScalarLiveValue("new")}},
		Tombstones: []Identity{{Kind: LiveAlias, Name: "gone"}},
	})
	if doc.Schema != WorktreeSchemaV1 || len(doc.Source.Entries) != 2 || len(doc.Projection.States) != 1 || len(doc.Projection.Tombstones) != 1 {
		t.Fatalf("committed source/projection contract collapsed: %#v", doc)
	}
	source.Entries[0].Names[0] = "MUTATED"
	*source.Entries[0].RuntimeValue = "mutated"
	if doc.Source.Entries[0].Names[0] != "A" || *doc.Source.Entries[0].RuntimeValue != "old" {
		t.Fatalf("committed source aliases caller memory: %#v", doc.Source.Entries[0])
	}
}

func TestAttachResolveAndValueFreeMetadataContracts(t *testing.T) {
	attachType := reflect.TypeOf(AttachRequest{})
	if _, ok := attachType.FieldByName("Credential"); !ok {
		t.Fatal("AttachRequest has no ShellCredential")
	}
	if _, ok := attachType.FieldByName("ShellID"); ok {
		t.Fatal("AttachRequest has an unauthenticated ShellID field")
	}
	if _, ok := attachType.FieldByName("Initial"); !ok {
		t.Fatal("AttachRequest has no initial snapshot")
	}
	if _, ok := attachType.FieldByName("Delta"); ok {
		t.Fatal("AttachRequest can publish a delta")
	}
	attachResultType := reflect.TypeOf(AttachResult{})
	reconcileField, ok := attachResultType.FieldByName("ReconcileRequired")
	if !ok || reconcileField.Type.Kind() != reflect.Bool {
		t.Fatal("AttachResult must carry one boolean ReconcileRequired decision")
	}

	resolveType := reflect.TypeOf(ResolveSharedRequest{})
	for _, field := range []string{"Conflict", "Token", "Snapshot"} {
		if _, ok := resolveType.FieldByName(field); !ok {
			t.Errorf("ResolveSharedRequest missing %s", field)
		}
	}
	resultType := reflect.TypeOf(ResolveSharedResult{})
	for _, field := range []string{"PendingRevision", "Token", "Changes"} {
		if _, ok := resultType.FieldByName(field); !ok {
			t.Errorf("ResolveSharedResult missing %s", field)
		}
	}
	for _, request := range []any{AttachRequest{}, PublishRequest{}, PreparePullRequest{}, AcknowledgeRequest{}, ResolveSharedRequest{}} {
		typ := reflect.TypeOf(request)
		field, ok := typ.FieldByName("Credential")
		if !ok || field.Type != reflect.TypeOf(ShellCredential{}) {
			t.Errorf("%T does not require ShellCredential", request)
		}
	}

	for _, value := range []any{AttachResult{}, Conflict{}, Exclusion{}, RevisionEvent{}, WorktreeStatus{}, CategorizedDiff{}, WorktreeCommitResult{}} {
		if path, found := forbiddenValueField(reflect.TypeOf(value), map[reflect.Type]bool{}); found {
			t.Errorf("%T contains captured value-bearing field at %s", value, path)
		}
	}

	token := ResolutionToken("resolve-1")
	if err := token.Validate(); err != nil {
		t.Fatalf("valid resolution token rejected: %v", err)
	}
	if err := ResolutionToken(strings.Repeat("x", MaxResolutionTokenBytes+1)).Validate(); err == nil {
		t.Fatal("oversized resolution token accepted")
	}
}

func liveTestName(kind LiveKind) string {
	switch kind {
	case LivePath:
		return "PATH"
	case LiveFPath:
		return "FPATH"
	case LiveOption:
		return "NO_BEEP"
	default:
		return "name"
	}
}

func forbiddenValueField(typ reflect.Type, seen map[reflect.Type]bool) (string, bool) {
	if typ == reflect.TypeOf(LiveValue{}) || typ == reflect.TypeOf(LiveIdentityState{}) || typ == reflect.TypeOf(LiveSnapshot{}) || typ == reflect.TypeOf(LiveChange{}) || typ == reflect.TypeOf(OverlayEntry{}) || typ == reflect.TypeOf(LiveProjection{}) {
		return typ.String(), true
	}
	if typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		return forbiddenValueField(typ.Elem(), seen)
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return "", false
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if path, found := forbiddenValueField(field.Type, seen); found {
			return field.Name + "." + path, true
		}
	}
	return "", false
}
