package store

// Tests for the store-side secret exclusion (D-07–D-10). They are an INTERNAL
// (package store) test so they can reach the package-private excludeSecrets and the
// vault constructor (newVaultKeychain) and inspect the post-exclusion profile
// directly. They wire the concrete zsh.Provider{} + ir.Build only to BUILD a
// realistic fixture profile (so Entry.Category==CatSecrets and Entry.Dynamic are set
// by the SHIPPED classifier/parser, not hand-stamped) — the same sanctioned
// test-wiring exception used by roundtrip_test.go; production store.go stays
// shell-agnostic.
//
// They pin the four behaviours the plan requires:
//  1. A literal secret is excluded from the committed tree, captured into the backend,
//     reported, and the read-back SecretRef carries the ACTIVE backend's Kind (vault
//     => file, T-03-09).
//  2. An already-dynamic secret commits VERBATIM (D-08) and is not reported.
//  3. excludeSecrets never mutates the caller's Profile (defensive copy).
//  4. A nil backend with a literal secret is ErrSecretBackendUnavailable (no nil-panic);
//     a nil backend with a secret-free profile is a no-op with an empty report.

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

// buildSecretProfile parses src through the shipped provider + ir.Build so the
// returned Profile's Category/Dynamic/Value/Names/StartLine are set exactly as the
// real ingest pipeline would set them — the inputs excludeSecrets reads. It is the
// honest way to exercise the D-08 predicate without hand-stamping Entry fields.
func buildSecretProfile(t *testing.T, src string) model.Profile {
	t.Helper()
	p := zsh.Provider{}
	blocks, err := p.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return ir.Build(blocks, p)
}

// newSecretStore builds a Store over a fresh temp bare repo with a real VAULT backend
// (newVaultKeychain — no external binary, so the test runs everywhere) and a stub
// Regenerator. Skips when git is absent. Returns the store and its vault so a test can
// assert the captured value is recoverable from the very backend the SecretRef points
// at.
func newSecretStore(t *testing.T) (*Store, vaultKeychain) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping secret-exclusion store tests")
	}
	dir := t.TempDir()
	repoDir := dir + "/zsh-pro" // the bare repo dir; the vault is its sibling
	vault := newVaultKeychain(repoDir)
	s, err := New(repoDir, stubRegen{}, vault)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, vault
}

// TestCommitExcludesLiteralSecret is the core T-03-03 pin: a literal secret is
// excluded from the committed tree, captured into the backend, reported, and the
// read-back entry's SecretRef.Kind is the vault's file kind (NOT keychain — T-03-09).
func TestCommitExcludesLiteralSecret(t *testing.T) {
	s, vault := newSecretStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// A literal secret (classifier sets CatSecrets via API_KEY; parser sets
	// Dynamic=false, Value=`"sk-abc123"`) plus a benign neighbour.
	const secretLiteral = "sk-abc123"
	p := buildSecretProfile(t, `export EDITOR=nvim
export API_KEY="`+secretLiteral+`"
`)

	report, err := s.Commit(ctx, "main", p, "with a literal secret")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// (c) The withheld report names API_KEY with its source line.
	if len(report) != 1 {
		t.Fatalf("WithheldReport = %v, want exactly one entry (API_KEY)", report)
	}
	if report[0].Name != "API_KEY" {
		t.Errorf("WithheldReport[0].Name = %q, want API_KEY", report[0].Name)
	}
	if report[0].StartLine != 2 {
		t.Errorf("WithheldReport[0].StartLine = %d, want 2 (the API_KEY line)", report[0].StartLine)
	}

	// (a) The committed profile.json must NOT contain the literal secret value.
	jsonBytes, err := s.git.show(ctx, "main:profile.json")
	if err != nil {
		t.Fatalf("git show main:profile.json: %v", err)
	}
	if strings.Contains(string(jsonBytes), secretLiteral) {
		t.Errorf("committed profile.json contains the literal secret %q — it must be excluded:\n%s", secretLiteral, jsonBytes)
	}
	// The reference (kind+key) IS in the tree — that is the whole point.
	if !strings.Contains(string(jsonBytes), `"key": "API_KEY"`) {
		t.Errorf("committed profile.json missing the SecretRef key for API_KEY:\n%s", jsonBytes)
	}

	// (b) The value is recoverable from the backend the SecretRef points at.
	got, err := vault.Retrieve("API_KEY")
	if err != nil {
		t.Fatalf("vault.Retrieve(API_KEY): %v", err)
	}
	if !strings.Contains(got, secretLiteral) {
		t.Errorf("vault.Retrieve(API_KEY) = %q, want it to contain the captured literal %q", got, secretLiteral)
	}

	// (d) The read-back entry's SecretRef.Kind is the vault's file kind (T-03-09):
	// a machine without security/secret-tool stamps file, so Ph4/5 derefs from the vault.
	back, err := s.Read(ctx, "main")
	if err != nil {
		t.Fatalf("Read(main): %v", err)
	}
	var apiEntry *model.Entry
	for i := range back.Entries {
		if len(back.Entries[i].Names) > 0 && back.Entries[i].Names[0] == "API_KEY" {
			apiEntry = &back.Entries[i]
		}
	}
	if apiEntry == nil {
		t.Fatalf("API_KEY entry missing from the read-back profile: %#v", back.Entries)
	}
	if apiEntry.Secret == nil {
		t.Fatalf("API_KEY entry has no SecretRef after exclusion: %#v", *apiEntry)
	}
	if apiEntry.Secret.Kind != model.SecretRefFile {
		t.Errorf("SecretRef.Kind = %q, want %q (vault fallback => file, not keychain)", apiEntry.Secret.Kind, model.SecretRefFile)
	}
	// The literal must be gone from BOTH derived text fields (Value feeds the
	// profile.zsh emit; Text is the JSON "text" field + the Regenerator fallback).
	if strings.Contains(apiEntry.Value, secretLiteral) {
		t.Errorf("API_KEY entry Value = %q still contains the literal %q after exclusion", apiEntry.Value, secretLiteral)
	}
	if strings.Contains(apiEntry.Text, secretLiteral) {
		t.Errorf("API_KEY entry Text = %q still contains the literal %q after exclusion", apiEntry.Text, secretLiteral)
	}

	// The committed profile.zsh (the derived human view, D-02) must ALSO be free of
	// the literal — it is regenerated from the post-exclusion profile, so a leak there
	// is the same T-03-03 violation as in profile.json.
	zshBytes, err := s.git.show(ctx, "main:profile.zsh")
	if err != nil {
		t.Fatalf("git show main:profile.zsh: %v", err)
	}
	if strings.Contains(string(zshBytes), secretLiteral) {
		t.Errorf("committed profile.zsh contains the literal secret %q — it must be excluded:\n%s", secretLiteral, zshBytes)
	}
	// The benign neighbour is untouched.
	var editor *model.Entry
	for i := range back.Entries {
		if len(back.Entries[i].Names) > 0 && back.Entries[i].Names[0] == "EDITOR" {
			editor = &back.Entries[i]
		}
	}
	if editor == nil || editor.Secret != nil || editor.Value != "nvim" {
		t.Errorf("EDITOR entry was altered by exclusion: %#v", editor)
	}
}

// TestCommitDynamicSecretVerbatim pins D-08: an already-dynamic secret
// (export TOKEN=$(op read ...)) is committed VERBATIM (it is already a late-bound
// pointer) and is NOT named in the withheld report.
func TestCommitDynamicSecretVerbatim(t *testing.T) {
	s, _ := newSecretStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// TOKEN matches secretRe (CatSecrets) but its value is a $(...) command
	// substitution, so the parser sets Dynamic=true — a late-bound pointer, D-08.
	p := buildSecretProfile(t, "export TOKEN=$(op read op://vault/item/token)\n")

	// Sanity: the fixture really is a dynamic secret (guards against a parser change
	// silently turning this into a literal and making the test vacuous).
	if len(p.Entries) != 1 || p.Entries[0].Category != model.CatSecrets || !p.Entries[0].Dynamic {
		t.Fatalf("fixture is not a dynamic secret as expected: %#v", p.Entries)
	}

	report, err := s.Commit(ctx, "main", p, "with a dynamic secret")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The report does NOT name TOKEN — nothing was withheld (D-08).
	if len(report) != 0 {
		t.Errorf("WithheldReport = %v, want empty (a dynamic secret is committed verbatim, not withheld)", report)
	}

	// The committed profile.json contains the verbatim $(op read ...) value.
	jsonBytes, err := s.git.show(ctx, "main:profile.json")
	if err != nil {
		t.Fatalf("git show main:profile.json: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "$(op read") {
		t.Errorf("committed profile.json lost the verbatim dynamic secret value:\n%s", jsonBytes)
	}

	// Read-back: the entry keeps its literal Value and has NO SecretRef.
	back, err := s.Read(ctx, "main")
	if err != nil {
		t.Fatalf("Read(main): %v", err)
	}
	if len(back.Entries) != 1 {
		t.Fatalf("read-back has %d entries, want 1", len(back.Entries))
	}
	if back.Entries[0].Secret != nil {
		t.Errorf("dynamic secret got a SecretRef (should commit verbatim): %#v", back.Entries[0])
	}
	if !strings.Contains(back.Entries[0].Value, "$(op read") {
		t.Errorf("dynamic secret Value = %q, want the verbatim $(op read ...) span", back.Entries[0].Value)
	}
}

// TestExcludeSecretsDefensiveCopy proves excludeSecrets never mutates the caller's
// Profile: the input entry's literal Value is still present after the call (the
// rewrite happens on a copy, so the Phase 2 round-trip comparison stays valid).
func TestExcludeSecretsDefensiveCopy(t *testing.T) {
	const secretLiteral = "sk-defensive"
	in := buildSecretProfile(t, `export API_KEY="`+secretLiteral+`"`+"\n")
	// Capture the input's pre-call value so we can prove it is unchanged.
	if len(in.Entries) != 1 || in.Entries[0].Value == "" {
		t.Fatalf("fixture not a single literal secret: %#v", in.Entries)
	}
	originalValue := in.Entries[0].Value

	vault := newVaultKeychain(t.TempDir())
	out, report, err := excludeSecrets(context.Background(), in, vault)
	if err != nil {
		t.Fatalf("excludeSecrets: %v", err)
	}

	// The CALLER's profile is untouched: its literal Value still present, Secret nil.
	if in.Entries[0].Value != originalValue {
		t.Errorf("excludeSecrets mutated the caller's Entry.Value: got %q, want %q (unchanged)", in.Entries[0].Value, originalValue)
	}
	if in.Entries[0].Secret != nil {
		t.Errorf("excludeSecrets stamped a SecretRef on the caller's Entry: %#v", in.Entries[0])
	}
	// The returned COPY is rewritten: literal gone from Text+Value, SecretRef stamped.
	if out.Entries[0].Secret == nil {
		t.Errorf("returned copy has no SecretRef: %#v", out.Entries[0])
	}
	if strings.Contains(out.Entries[0].Value, secretLiteral) || strings.Contains(out.Entries[0].Text, secretLiteral) {
		t.Errorf("returned copy still carries the literal %q: %#v", secretLiteral, out.Entries[0])
	}
	if len(report) != 1 || report[0].Name != "API_KEY" {
		t.Errorf("report = %v, want one API_KEY entry", report)
	}
}

// TestExcludeSecretsNilKeychain proves the nil-backend guard: a literal secret with a
// nil driver is ErrSecretBackendUnavailable (NOT a nil-panic), while a secret-free
// profile with a nil driver is a no-op returning an empty report.
func TestExcludeSecretsNilKeychain(t *testing.T) {
	ctx := context.Background()

	// (1) literal secret + nil backend => ErrSecretBackendUnavailable, no panic.
	withSecret := buildSecretProfile(t, `export API_KEY="sk-nilguard"`+"\n")
	if _, _, err := excludeSecrets(ctx, withSecret, nil); !errors.Is(err, ErrSecretBackendUnavailable) {
		t.Errorf("excludeSecrets(literal secret, nil kc) = %v, want ErrSecretBackendUnavailable", err)
	}

	// (2) secret-free profile + nil backend => no-op, empty report, no error.
	secretFree := buildSecretProfile(t, "export EDITOR=nvim\nalias gs='git status'\n")
	out, report, err := excludeSecrets(ctx, secretFree, nil)
	if err != nil {
		t.Fatalf("excludeSecrets(secret-free, nil kc) = %v, want nil (no-op)", err)
	}
	if len(report) != 0 {
		t.Errorf("secret-free report = %v, want empty", report)
	}
	if len(out.Entries) != len(secretFree.Entries) {
		t.Errorf("secret-free profile changed shape: got %d entries, want %d", len(out.Entries), len(secretFree.Entries))
	}
}
