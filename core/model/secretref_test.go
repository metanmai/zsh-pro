package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSecretRefMarshalShape pins the locked kind:key wire shape (D-09): a
// SecretRef serializes to exactly {"kind":...,"key":...} with lowercase keys.
func TestSecretRefMarshalShape(t *testing.T) {
	ref := SecretRef{Kind: SecretRefKeychain, Key: "API_KEY"}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal SecretRef: %v", err)
	}
	got := string(b)
	want := `{"kind":"keychain","key":"API_KEY"}`
	if got != want {
		t.Errorf("SecretRef wire shape mismatch:\n got=%s\nwant=%s", got, want)
	}
}

// TestSecretRefKinds pins the three resolver-backend constants (D-09): keychain
// (default), file (git-ignored vault fallback), cmd (future managers).
func TestSecretRefKinds(t *testing.T) {
	cases := map[SecretRefKind]string{
		SecretRefKeychain: "keychain",
		SecretRefFile:     "file",
		SecretRefCmd:      "cmd",
	}
	for kind, want := range cases {
		if string(kind) != want {
			t.Errorf("SecretRefKind %q != %q", string(kind), want)
		}
	}
}

// TestSecretRefRoundTrip proves a SecretRef survives marshal->unmarshal with
// Kind and Key reconstructed exactly (the Read/Commit payload contract).
func TestSecretRefRoundTrip(t *testing.T) {
	in := SecretRef{Kind: SecretRefFile, Key: "DEPLOY_TOKEN"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SecretRef
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("SecretRef round-trip mismatch: got %+v, want %+v", out, in)
	}
}

// TestEntryOmitsSecretWhenNil proves the Secret field is additive (Pitfall 5): a
// zero Entry (no Secret) marshals with NO "secret" key, so Phase 2 fixtures and
// the oracle round-trip unchanged. NOTE: model.Entry carries no json tags of its
// own (serialization lives in the store DTO, Task 2); this test tags an inline
// mirror to assert the *omitempty pointer* additive behavior of the field itself.
func TestEntryOmitsSecretWhenNil(t *testing.T) {
	// The store DTO (Task 2) owns the on-wire tags; here we assert the additive
	// contract directly on a *SecretRef omitempty field, which is what the DTO
	// and any future tagging will rely on.
	type entryShape struct {
		Names  []string   `json:"names"`
		Secret *SecretRef `json:"secret,omitempty"`
	}

	// nil Secret -> no "secret" key.
	bare := entryShape{Names: []string{"EDITOR"}}
	b, err := json.Marshal(bare)
	if err != nil {
		t.Fatalf("marshal bare: %v", err)
	}
	if strings.Contains(string(b), "secret") {
		t.Errorf("nil Secret must be omitted (omitempty), got: %s", b)
	}

	// set Secret -> nested {"secret":{"kind":...,"key":...}} present.
	set := entryShape{
		Names:  []string{"API_KEY"},
		Secret: &SecretRef{Kind: SecretRefKeychain, Key: "API_KEY"},
	}
	b2, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal set: %v", err)
	}
	if !strings.Contains(string(b2), `"secret":{"kind":"keychain","key":"API_KEY"}`) {
		t.Errorf("set Secret must marshal the nested object, got: %s", b2)
	}
}
