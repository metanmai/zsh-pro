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
	"fmt"
	"os/exec"
	"reflect"
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
	if got != secretLiteral {
		t.Errorf("vault.Retrieve(API_KEY) = %q, want exact semantic value %q", got, secretLiteral)
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
	// The literal must be gone from both derived text fields and the decoded runtime
	// field. Unsupported mode prevents activation from treating the inert placeholder
	// as a legacy runtime value while SecretRef is authoritative.
	if strings.Contains(apiEntry.Value, secretLiteral) {
		t.Errorf("API_KEY entry Value = %q still contains the literal %q after exclusion", apiEntry.Value, secretLiteral)
	}
	if strings.Contains(apiEntry.Text, secretLiteral) {
		t.Errorf("API_KEY entry Text = %q still contains the literal %q after exclusion", apiEntry.Text, secretLiteral)
	}
	if apiEntry.RuntimeValue != nil {
		t.Errorf("API_KEY entry RuntimeValue = %q, want nil after secret exclusion", *apiEntry.RuntimeValue)
	}
	if apiEntry.ValueMode != model.ValueModeUnsupported {
		t.Errorf("API_KEY entry ValueMode = %q, want %q after secret exclusion", apiEntry.ValueMode, model.ValueModeUnsupported)
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

func TestExcludeSecretsStoresLiteralRuntimeValueExactly(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"single quoted", `export API_KEY='single value'`, "single value"},
		{"double quoted", `export API_KEY="double value"`, "double value"},
		{"escaped", `export API_KEY=escaped\ value`, "escaped value"},
		{"empty", `export API_KEY=''`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := buildSecretProfile(t, tc.src+"\n")
			vault := newVaultKeychain(t.TempDir())
			out, _, err := excludeSecrets(context.Background(), in, vault)
			if err != nil {
				t.Fatal(err)
			}
			got, err := vault.Retrieve("API_KEY")
			if err != nil || got != tc.want {
				t.Fatalf("stored=%q err=%v want=%q", got, err, tc.want)
			}
			if out.Entries[0].RuntimeValue != nil || out.Entries[0].ValueMode != model.ValueModeUnsupported {
				t.Fatalf("redaction=%#v", out.Entries[0])
			}
		})
	}
}

// transactionKeychain records side effects so transaction ordering and rollback can
// be exercised without relying on an OS keychain or exposing values in subprocesses.
type transactionKeychain struct {
	values   map[string]string
	stores   []string
	failKey  string
	failOnce bool
}

func (k *transactionKeychain) Store(key, value string) error {
	k.stores = append(k.stores, key)
	if key == k.failKey && k.failOnce {
		k.failOnce = false
		return ErrSecretBackendUnavailable
	}
	k.values[key] = value
	return nil
}

func (k *transactionKeychain) Retrieve(key string) (string, error) {
	value, ok := k.values[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (k *transactionKeychain) Delete(key string) error {
	delete(k.values, key)
	return nil
}

func (*transactionKeychain) Kind() model.SecretRefKind { return model.SecretRefFile }

func TestPrepareSecretsIsPureAndCoalescesLastWrite(t *testing.T) {
	kc := &transactionKeychain{values: map[string]string{}}
	in := buildSecretProfile(t, "export API_KEY=first\nexport API_KEY=second\n")

	prepared, err := prepareSecrets(in, kc)
	if err != nil {
		t.Fatal("prepareSecrets returned an error")
	}
	if len(kc.stores) != 0 {
		t.Fatal("prepareSecrets mutated the backend")
	}
	if len(prepared.pending) != 1 || prepared.pending[0].key != "API_KEY" || prepared.pending[0].value != "second" {
		t.Fatal("prepareSecrets did not retain one final mutation")
	}
	if len(prepared.report) != 2 {
		t.Fatal("prepareSecrets did not report every source occurrence")
	}
	if in.Entries[0].Secret != nil || in.Entries[1].Secret != nil {
		t.Fatal("prepareSecrets mutated the caller profile")
	}
}

func TestCommitRollsBackSecretWritesBeforeMovingRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping transaction test")
	}
	ctx := context.Background()
	kc := &transactionKeychain{
		values:   map[string]string{"API_KEY": "previous-a", "TOKEN": "previous-b"},
		failKey:  "TOKEN",
		failOnce: true,
	}
	s, err := New(t.TempDir(), stubRegen{}, kc)
	if err != nil {
		t.Fatal("New returned an error")
	}
	if err := s.Init(ctx); err != nil {
		t.Fatal("Init returned an error")
	}
	before, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal("could not read baseline ref")
	}
	p := buildSecretProfile(t, "export API_KEY=next_a\nexport TOKEN=next_b\n")
	if _, err := s.Commit(ctx, "main", p, "transaction test"); !errors.Is(err, ErrSecretBackendUnavailable) {
		t.Fatal("Commit did not return the typed backend failure")
	}
	after, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil || after != before {
		t.Fatal("failed secret write moved the profile ref")
	}
	if kc.values["API_KEY"] != "previous-a" || kc.values["TOKEN"] != "previous-b" {
		t.Fatal("failed secret write was not rolled back")
	}
}

func TestCommitCompensatesAmbiguousRefFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping ref transaction test")
	}
	for _, tc := range []struct {
		name      string
		committed bool
	}{
		{name: "expected observed"},
		{name: "candidate observed", committed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			kc := &transactionKeychain{values: map[string]string{}}
			s, err := New(t.TempDir(), stubRegen{}, kc)
			if err != nil {
				t.Fatal("New returned an error")
			}
			if err := s.Init(ctx); err != nil {
				t.Fatal("Init returned an error")
			}
			before, err := s.git.revParse(ctx, "refs/heads/main")
			if err != nil {
				t.Fatal("could not read baseline ref")
			}
			s.commitRefSession = func(session *updateRefSession) error {
				if tc.committed {
					if err := session.Commit(); err != nil {
						return err
					}
				} else if err := session.Abort(); err != nil {
					return err
				}
				return ErrGitCommand
			}
			p := buildSecretProfile(t, "export API_KEY=next_value\n")
			report, err := s.Commit(ctx, "main", p, "ref transaction test")
			if tc.committed {
				if err != nil || len(report) != 1 {
					t.Fatalf("observed candidate = (%#v, %v), want committed report", report, err)
				}
			} else if !errors.Is(err, ErrIngestNotCommitted) || len(report) != 0 {
				t.Fatalf("observed expected = (%#v, %v), want clean not-committed", report, err)
			}
			after, err := s.git.revParse(ctx, "refs/heads/main")
			if err != nil {
				t.Fatal(err)
			}
			if tc.committed {
				if after == before || kc.values["API_KEY"] != "next_value" {
					t.Fatal("candidate observation lost committed ref/backend effects")
				}
			} else if after != before {
				t.Fatal("expected observation moved the ref")
			} else if _, exists := kc.values["API_KEY"]; exists {
				t.Fatal("guarded expected observation left a backend write behind")
			}
		})
	}
}

func TestExcludeSecretsRejectsMalformedLiteralBeforeBackendWrite(t *testing.T) {
	vault := newVaultKeychain(t.TempDir())
	in := model.Profile{Entries: []model.Entry{{
		Category: model.CatSecrets, Kind: model.KindAssignment, Names: []string{"API_KEY"},
		ValueMode: model.ValueModeLiteral,
	}}}
	if _, _, err := excludeSecrets(context.Background(), in, vault); !errors.Is(err, ErrUnsafeSecretShape) {
		t.Fatalf("excludeSecrets error=%v, want ErrUnsafeSecretShape", err)
	}
	if _, err := vault.Retrieve("API_KEY"); err == nil {
		t.Fatal("backend was written for malformed literal")
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
	originalRuntimeValue := in.Entries[0].RuntimeValue
	if originalRuntimeValue == nil {
		t.Fatalf("fixture has no decoded RuntimeValue: %#v", in.Entries[0])
	}

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
	if in.Entries[0].RuntimeValue == nil || in.Entries[0].RuntimeValue != originalRuntimeValue || *in.Entries[0].RuntimeValue != *originalRuntimeValue {
		t.Errorf("excludeSecrets mutated the caller's Entry.RuntimeValue: got %v, want original pointer/value %q", in.Entries[0].RuntimeValue, *originalRuntimeValue)
	}
	// The returned COPY is rewritten: literal gone from Text+Value, SecretRef stamped.
	if out.Entries[0].Secret == nil {
		t.Errorf("returned copy has no SecretRef: %#v", out.Entries[0])
	}
	if strings.Contains(out.Entries[0].Value, secretLiteral) || strings.Contains(out.Entries[0].Text, secretLiteral) {
		t.Errorf("returned copy still carries the literal %q: %#v", secretLiteral, out.Entries[0])
	}
	if out.Entries[0].RuntimeValue != nil || out.Entries[0].ValueMode != model.ValueModeUnsupported {
		t.Errorf("returned copy retained an activatable runtime value: %#v", out.Entries[0])
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

// assertCommitDidNotWrite proves a failed Commit was fully fail-closed against the
// `main` branch: the ref did not move off its pre-Commit tip and no profile.json blob
// was written, so NO committed blob can contain the leaked literal. It is the shared
// "nothing reached the tree" assertion for the CR-01/CR-02 adversarial tests.
//
// The tip-unmoved + no-profile.json checks ARE the "nothing was written" proof. Read
// is then expected to return the EMPTY profile, not ErrProfileNotFound: `main` exists
// from Init's baseline root commit, so per the WR-03 contract a present-but-uncommitted
// branch reads back as model.Profile{} (an empty profile has no entries, hence no
// literal — which is exactly the fail-closed outcome we want).
func assertCommitDidNotWrite(t *testing.T, s *Store, ctx context.Context, preTip string) {
	t.Helper()
	postTip, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatalf("revParse(main) after failed Commit: %v", err)
	}
	if postTip != preTip {
		t.Errorf("main tip moved on a fail-closed Commit: pre=%q post=%q (a commit was written)", preTip, postTip)
	}
	if s.git.catFileExists(ctx, "main:profile.json") {
		t.Errorf("main:profile.json exists after a fail-closed Commit — a blob was written when none should be")
	}
	got, err := s.Read(ctx, "main")
	if err != nil {
		t.Errorf("Read(main) after a fail-closed Commit = %v, want nil (main exists but nothing was committed; WR-03)", err)
	}
	if len(got.Entries) != 0 {
		t.Errorf("Read(main) after a fail-closed Commit returned %d entries, want 0 (nothing was committed)", len(got.Entries))
	}
}

// TestCommitFailsClosedMultiNameSecretWithDynamicSibling is the CR-01 adversarial pin
// (T-03-03). `export PATH=$HOME/bin API_KEY=sk-LEAKED-SECRET-123` collapses in the
// parser to ONE entry: Names=[PATH, API_KEY], Value="sk-LEAKED-SECRET-123" (last
// segment), Dynamic=true (because of $HOME). Before the fix, the !Dynamic guard made
// isLiteralSecret false, so the entry bypassed exclusion and the literal committed
// VERBATIM into both profile.json and profile.zsh. After the fix, the multi-name shape
// is un-excludable, so Commit FAILS CLOSED with ErrUnsafeSecretShape and writes nothing.
func TestCommitFailsClosedMultiNameSecretWithDynamicSibling(t *testing.T) {
	s, vault := newSecretStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	preTip, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatalf("revParse(main) baseline: %v", err)
	}

	const leaked = "sk-LEAKED-SECRET-123"
	p := buildSecretProfile(t, "export PATH=$HOME/bin API_KEY="+leaked+"\n")

	// Sanity: the fixture really is the CR-01 collapsed shape (guards against a parser
	// change that would silently make this test vacuous).
	if len(p.Entries) != 1 {
		t.Fatalf("fixture should parse to one collapsed entry, got %d: %#v", len(p.Entries), p.Entries)
	}
	e := p.Entries[0]
	if e.Category != model.CatSecrets || len(e.Names) != 2 || !e.Dynamic {
		t.Fatalf("fixture is not the CR-01 multi-name dynamic-sibling shape: %#v", e)
	}

	// FAIL CLOSED: Commit must refuse, not leak.
	report, err := s.Commit(ctx, "main", p, "multi-name secret with a dynamic sibling")
	if !errors.Is(err, ErrUnsafeSecretShape) {
		t.Fatalf("Commit = (%v, %v), want ErrUnsafeSecretShape (fail closed)", report, err)
	}
	if report != nil {
		t.Errorf("WithheldReport = %v, want nil on a fail-closed abort", report)
	}

	// Nothing reached the tree: ref unmoved, no profile.json, Read => not found.
	assertCommitDidNotWrite(t, s, ctx, preTip)

	// And the literal was never captured into the backend either.
	if got, err := vault.Retrieve("API_KEY"); err == nil {
		t.Errorf("vault captured API_KEY=%q on a fail-closed abort; nothing should have been stored", got)
	}
}

// TestCommitFailsClosedMultiNameSecretWrongKeyOrder is the CR-02 adversarial pin.
// `export OTHER=foo API_KEY=sk-REAL-SECRET` collapses to Names=[OTHER, API_KEY],
// Value="sk-REAL-SECRET" (last segment), Dynamic=false. Before the fix, isLiteralSecret
// was true, so the real secret was captured under the WRONG key OTHER (Names[0]), the
// benign `OTHER=foo` was destroyed, and API_KEY resolved to nothing. After the fix, the
// multi-name shape is un-excludable: Commit FAILS CLOSED, writes nothing, the benign
// sibling is not destroyed (the caller's profile is untouched), and the secret is NOT
// captured under any key.
func TestCommitFailsClosedMultiNameSecretWrongKeyOrder(t *testing.T) {
	s, vault := newSecretStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	preTip, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatalf("revParse(main) baseline: %v", err)
	}

	const real = "sk-REAL-SECRET"
	p := buildSecretProfile(t, "export OTHER=foo API_KEY="+real+"\n")

	// Sanity: the fixture is the CR-02 collapsed shape (Names[0] is NOT the secret).
	if len(p.Entries) != 1 {
		t.Fatalf("fixture should parse to one collapsed entry, got %d: %#v", len(p.Entries), p.Entries)
	}
	e := p.Entries[0]
	if e.Category != model.CatSecrets || len(e.Names) != 2 || e.Names[0] != "OTHER" || e.Dynamic {
		t.Fatalf("fixture is not the CR-02 wrong-key-order shape: %#v", e)
	}
	originalText := p.Entries[0].Text // capture to prove the benign sibling is not destroyed

	report, err := s.Commit(ctx, "main", p, "multi-name secret, benign sibling first")
	if !errors.Is(err, ErrUnsafeSecretShape) {
		t.Fatalf("Commit = (%v, %v), want ErrUnsafeSecretShape (fail closed)", report, err)
	}
	if report != nil {
		t.Errorf("WithheldReport = %v, want nil on a fail-closed abort", report)
	}

	// Nothing reached the tree.
	assertCommitDidNotWrite(t, s, ctx, preTip)

	// The benign `OTHER=foo` sibling is NOT destroyed: Commit operates on a copy and
	// aborted, so the caller's entry still carries its original verbatim Text.
	if p.Entries[0].Text != originalText {
		t.Errorf("caller's Entry.Text was mutated by a fail-closed Commit: got %q, want %q", p.Entries[0].Text, originalText)
	}
	if !strings.Contains(p.Entries[0].Text, "OTHER=foo") {
		t.Errorf("benign sibling OTHER=foo missing from caller's entry: %q", p.Entries[0].Text)
	}

	// The secret was captured under NEITHER the wrong key (OTHER) nor the right one
	// (API_KEY) — a fail-closed abort stores nothing.
	if got, err := vault.Retrieve("OTHER"); err == nil {
		t.Errorf("vault captured the secret under the WRONG key OTHER=%q (CR-02 regression)", got)
	}
	if got, err := vault.Retrieve("API_KEY"); err == nil {
		t.Errorf("vault captured API_KEY=%q on a fail-closed abort; nothing should have been stored", got)
	}
}

// TestCommitFailsClosedArraySecret pins the array-secret leak gap that the single-name
// shape gate alone would miss: `export SECRETS=(sk-arr-leak)` parses to a SINGLE-name
// assignment (Names=[SECRETS]) but with Value="" and Dynamic=false — the literal lives
// ONLY in Text. Committing it verbatim would leak `sk-arr-leak` into profile.json's text
// field and profile.zsh. The store cannot model an array value from Entry fields, so the
// empty-scalar-literal branch FAILS CLOSED with ErrUnsafeSecretShape.
func TestCommitFailsClosedArraySecret(t *testing.T) {
	s, vault := newSecretStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	preTip, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatalf("revParse(main) baseline: %v", err)
	}

	const leaked = "sk-arr-leak"
	p := buildSecretProfile(t, "export SECRETS=("+leaked+")\n")

	// Sanity: a single-name CatSecrets assignment with no scalar Value (the array shape).
	if len(p.Entries) != 1 {
		t.Fatalf("fixture should parse to one entry, got %d: %#v", len(p.Entries), p.Entries)
	}
	e := p.Entries[0]
	if e.Category != model.CatSecrets || len(e.Names) != 1 || e.Dynamic || e.Value != "" {
		t.Fatalf("fixture is not the single-name array-secret shape: %#v", e)
	}
	if !strings.Contains(e.Text, leaked) {
		t.Fatalf("array literal not in Text as expected (test would be vacuous): %q", e.Text)
	}

	report, err := s.Commit(ctx, "main", p, "array-valued secret")
	if !errors.Is(err, ErrUnsafeSecretShape) {
		t.Fatalf("Commit = (%v, %v), want ErrUnsafeSecretShape (fail closed on array secret)", report, err)
	}

	assertCommitDidNotWrite(t, s, ctx, preTip)
	if got, err := vault.Retrieve("SECRETS"); err == nil {
		t.Errorf("vault captured SECRETS=%q on a fail-closed abort; nothing should have been stored", got)
	}
}

type countingSecretRefKeychain struct {
	kind          model.SecretRefKind
	kindCalls     int
	retrieveCalls int
	storeCalls    int
	deleteCalls   int
	failStore     bool
	values        map[string]string
}

func (k *countingSecretRefKeychain) Store(key, value string) error {
	k.storeCalls++
	if k.failStore {
		return ErrSecretBackendUnavailable
	}
	if k.values == nil {
		k.values = make(map[string]string)
	}
	k.values[key] = value
	return nil
}

func (k *countingSecretRefKeychain) Retrieve(key string) (string, error) {
	k.retrieveCalls++
	value, ok := k.values[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (k *countingSecretRefKeychain) Delete(key string) error {
	k.deleteCalls++
	delete(k.values, key)
	return nil
}

func (k *countingSecretRefKeychain) Kind() model.SecretRefKind {
	k.kindCalls++
	return k.kind
}

func (k *countingSecretRefKeychain) resetCalls() {
	k.kindCalls = 0
	k.retrieveCalls = 0
	k.storeCalls = 0
	k.deleteCalls = 0
}

func persistedSecretRefProfile(kind model.SecretRefKind) model.Profile {
	ref := &model.SecretRef{Kind: kind, Key: "API_KEY"}
	placeholder := secretRefValue(*ref)
	return model.Profile{Entries: []model.Entry{{
		Text:                    "export API_KEY=" + placeholder,
		StartLine:               7,
		Category:                model.CatSecrets,
		Kind:                    model.KindAssignment,
		Names:                   []string{"API_KEY"},
		Value:                   placeholder,
		Exported:                true,
		Managed:                 true,
		StructuralFidelityKnown: true,
		DeclarationFlags:        []string{},
		OptionFlags:             []string{},
		Secret:                  ref,
		ValueMode:               model.ValueModeUnsupported,
	}}}
}

func newCountingIngestStore(
	t *testing.T,
	keychain *countingSecretRefKeychain,
) (*Store, InstallInitialization, model.IngestBeginOutcome) {
	t.Helper()
	root := t.TempDir() + "/store"
	store, err := New(root, stubRegen{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transaction := beginPhase6Ingest(t, store, initialization)
	keychain.resetCalls()
	return store, initialization, transaction
}

func TestCommitIngestExistingSecretRefPassThrough(t *testing.T) {
	keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, values: map[string]string{"API_KEY": "existing-value"}}
	store, initialization, transaction := newCountingIngestStore(t, keychain)
	profile := persistedSecretRefProfile(model.SecretRefFile)
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, profile)
	if err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("CommitIngest persisted SecretRef = (%#v, %v)", outcome, err)
	}
	if keychain.kindCalls != 1 || keychain.retrieveCalls != 0 || keychain.storeCalls != 0 || keychain.deleteCalls != 0 {
		t.Fatalf("backend calls = Kind:%d Retrieve:%d Store:%d Delete:%d",
			keychain.kindCalls, keychain.retrieveCalls, keychain.storeCalls, keychain.deleteCalls)
	}
	if len(outcome.Withheld) != 0 {
		t.Fatalf("persisted SecretRef produced withheld metadata: %#v", outcome.Withheld)
	}
	back, err := store.Read(context.Background(), "main")
	if err != nil || !reflect.DeepEqual(back, profile) {
		t.Fatalf("persisted SecretRef changed: got (%#v, %v), want %#v", back, err, profile)
	}
}

func TestCommitIngestMalformedSecretRefCallAndEffectMatrix(t *testing.T) {
	runtimeValue := "forbidden"
	rows := []struct {
		name     string
		mutate   func(*model.Entry)
		kind     model.SecretRefKind
		wantKind int
		valid    bool
	}{
		{name: "wrong category", mutate: func(entry *model.Entry) { entry.Category = model.CatEnvironment }},
		{name: "missing name", mutate: func(entry *model.Entry) { entry.Names = nil }},
		{name: "multiple names", mutate: func(entry *model.Entry) { entry.Names = []string{"API_KEY", "TOKEN"} }},
		{name: "unsupported reference kind", mutate: func(entry *model.Entry) { entry.Secret.Kind = model.SecretRefCmd }},
		{name: "blank key", mutate: func(entry *model.Entry) { entry.Secret.Key = "" }},
		{name: "key name mismatch", mutate: func(entry *model.Entry) { entry.Secret.Key = "TOKEN" }},
		{name: "wrong text", mutate: func(entry *model.Entry) { entry.Text = "export API_KEY=wrong" }},
		{name: "wrong value", mutate: func(entry *model.Entry) { entry.Value = "wrong" }},
		{name: "wrong value mode", mutate: func(entry *model.Entry) { entry.ValueMode = model.ValueModeLiteral }},
		{name: "runtime value", mutate: func(entry *model.Entry) { entry.RuntimeValue = &runtimeValue }},
		{name: "dynamic", mutate: func(entry *model.Entry) { entry.Dynamic = true }},
		{
			name: "backend kind mismatch",
			mutate: func(entry *model.Entry) {
				entry.Secret.Kind = model.SecretRefKeychain
				placeholder := secretRefValue(*entry.Secret)
				entry.Text = "export API_KEY=" + placeholder
				entry.Value = placeholder
			},
			kind:     model.SecretRefFile,
			wantKind: 1,
		},
		{name: "valid", kind: model.SecretRefFile, wantKind: 1, valid: true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			backendKind := row.kind
			if backendKind == "" {
				backendKind = model.SecretRefFile
			}
			keychain := &countingSecretRefKeychain{kind: backendKind, values: map[string]string{"API_KEY": "existing-value"}}
			store, initialization, transaction := newCountingIngestStore(t, keychain)
			profile := persistedSecretRefProfile(model.SecretRefFile)
			if row.mutate != nil {
				row.mutate(&profile.Entries[0])
			}
			gitStarts := 0
			store.git.beforeStart = func([]string) { gitStarts++ }
			before := *transaction.Baseline.ExpectedRevision
			outcome, err := commitPhase6Ingest(t, store, initialization, transaction, profile)
			if row.valid {
				if err != nil || outcome.Status != model.IngestCommitCommitted {
					t.Fatalf("valid pass-through = (%#v, %v)", outcome, err)
				}
				if gitStarts == 0 {
					t.Fatal("valid pass-through wrote no candidate")
				}
			} else {
				if !errors.Is(err, ErrUnsafeSecretShape) || outcome.Status == model.IngestCommitCommitted {
					t.Fatalf("malformed reference = (%#v, %v)", outcome, err)
				}
				if gitStarts != 0 {
					t.Fatalf("malformed reference started %d Git processes", gitStarts)
				}
				after, readErr := store.git.revParse(context.Background(), "refs/heads/main")
				if readErr != nil || after != before {
					t.Fatalf("malformed reference moved ref: %q, %v; want %q", after, readErr, before)
				}
			}
			if keychain.kindCalls != row.wantKind || keychain.retrieveCalls != 0 ||
				keychain.storeCalls != 0 || keychain.deleteCalls != 0 {
				t.Fatalf("calls = Kind:%d Retrieve:%d Store:%d Delete:%d; want Kind:%d and no data methods",
					keychain.kindCalls, keychain.retrieveCalls, keychain.storeCalls, keychain.deleteCalls, row.wantKind)
			}
		})
	}
}

func TestPrepareSecretsLiteralRedactedBeforeObjects(t *testing.T) {
	keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, values: map[string]string{}}
	const literal = "literal-must-not-reach-objects"
	prepared, err := prepareSecrets(buildSecretProfile(t, "export API_KEY="+literal+"\n"), keychain)
	if err != nil {
		t.Fatal(err)
	}
	textRedacted := !strings.Contains(prepared.profile.Entries[0].Text, literal)
	valueRedacted := !strings.Contains(prepared.profile.Entries[0].Value, literal)
	if len(prepared.pending) != 1 || len(prepared.report) != 1 || !textRedacted || !valueRedacted {
		t.Fatalf("literal redaction facts: pending=%d report=%d text_redacted=%t value_redacted=%t",
			len(prepared.pending), len(prepared.report), textRedacted, valueRedacted)
	}
	if keychain.retrieveCalls != 0 || keychain.storeCalls != 0 || keychain.deleteCalls != 0 {
		t.Fatalf("prepareSecrets backend calls: Retrieve=%d Store=%d Delete=%d",
			keychain.retrieveCalls, keychain.storeCalls, keychain.deleteCalls)
	}
}

func TestPrepareSecretsDynamicVerbatim(t *testing.T) {
	keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, values: map[string]string{}}
	profile := buildSecretProfile(t, "export API_KEY=$(op read reference)\n")
	prepared, err := prepareSecrets(profile, keychain)
	if err != nil {
		t.Fatal(err)
	}
	profileUnchanged := reflect.DeepEqual(prepared.profile, profile)
	if !profileUnchanged || len(prepared.pending) != 0 || len(prepared.report) != 0 {
		t.Fatalf("dynamic secret facts: profile_unchanged=%t pending=%d report=%d",
			profileUnchanged, len(prepared.pending), len(prepared.report))
	}
	if keychain.kindCalls != 0 || keychain.retrieveCalls != 0 || keychain.storeCalls != 0 || keychain.deleteCalls != 0 {
		t.Fatalf("dynamic secret backend calls: Kind=%d Retrieve=%d Store=%d Delete=%d",
			keychain.kindCalls, keychain.retrieveCalls, keychain.storeCalls, keychain.deleteCalls)
	}
}

func TestLegacyCommitAdapterPreservesRequestedBranch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	mainBefore, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	var argv [][]string
	store.git.beforeStart = func(args []string) { argv = append(argv, append([]string(nil), args...)) }
	if report, err := store.Commit(ctx, "feature/exact", phase6Profile("BRANCH", "exact"), "legacy adapter"); err != nil || len(report) != 0 {
		t.Fatalf("Commit(feature/exact) = (%#v, %v)", report, err)
	}
	branch, err := store.Read(ctx, "feature/exact")
	if err != nil || !reflect.DeepEqual(branch, phase6Profile("BRANCH", "exact")) {
		t.Fatalf("feature/exact = (%#v, %v)", branch, err)
	}
	mainAfter, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil || mainAfter != mainBefore {
		t.Fatalf("legacy branch commit changed main: %q, %v; want %q", mainAfter, err, mainBefore)
	}
	if got := capturedUpdateRefTransactions(argv); !reflect.DeepEqual(got, [][]string{{"update-ref", "--no-deref", "--stdin"}}) {
		t.Fatalf("legacy update-ref argv = %#v", got)
	}
}

type committedStateError interface {
	error
	Committed() bool
}

func TestLegacyCommitAdapterOutcomeMatrix(t *testing.T) {
	t.Run("committed", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Commit(context.Background(), "main", phase6Profile("MATRIX", "committed"), "matrix"); err != nil {
			t.Fatalf("committed adapter: %v", err)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()
		if err := store.Init(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Commit(ctx, "main", phase6Profile("MATRIX", "older"), "older"); err != nil {
			t.Fatal(err)
		}
		older, err := store.git.revParse(ctx, "refs/heads/main")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Commit(ctx, "main", phase6Profile("MATRIX", "current"), "current"); err != nil {
			t.Fatal(err)
		}
		injected := false
		store.git.beforeStart = func(args []string) {
			if injected || !reflect.DeepEqual(args, []string{"update-ref", "--no-deref", "--stdin"}) {
				return
			}
			injected = true
			store.git.beforeStart = nil
			if err := store.git.updateRef(ctx, "refs/heads/main", older); err != nil {
				t.Errorf("inject concurrent ref: %v", err)
			}
		}
		if report, err := store.Commit(ctx, "main", phase6Profile("MATRIX", "loser"), "loser"); !errors.Is(err, ErrSecretRefConflict) || len(report) != 0 {
			t.Fatalf("conflict adapter = (%#v, %v)", report, err)
		}
	})

	t.Run("clean not committed", func(t *testing.T) {
		keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, failStore: true, values: map[string]string{}}
		store, err := New(t.TempDir()+"/store", stubRegen{}, keychain)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		if report, err := store.Commit(context.Background(), "main", buildSecretProfile(t, "export API_KEY=blocked\n"), "blocked"); !errors.Is(err, ErrIngestNotCommitted) || len(report) != 0 {
			t.Fatalf("not-committed adapter = (%#v, %v)", report, err)
		}
	})

	t.Run("recovery committed", func(t *testing.T) {
		keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, values: map[string]string{}}
		store, err := New(t.TempDir()+"/store", stubRegen{}, keychain)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		store.cleanupDiscardLock = true
		report, err := store.Commit(
			context.Background(),
			"main",
			buildSecretProfile(t, "export API_KEY=recovery-value\n"),
			"recovery",
		)
		state, ok := err.(committedStateError)
		if !errors.Is(err, ErrIngestRecoveryRequired) || !ok || !state.Committed() ||
			len(report) != 1 || report[0].Name != "API_KEY" || report[0].StartLine != 1 {
			t.Fatalf("committed recovery adapter = (%#v, %v, committed=%v)", report, err, ok && state.Committed())
		}
	})

	t.Run("recovery not committed", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		store.commitSyncDir = func(string) error { return errors.New("injected durability uncertainty") }
		report, err := store.Commit(context.Background(), "main", phase6Profile("MATRIX", "uncertain"), "uncertain")
		state, ok := err.(committedStateError)
		if !errors.Is(err, ErrIngestRecoveryRequired) || !ok || state.Committed() || len(report) != 0 {
			t.Fatalf("not-committed recovery adapter = (%#v, %v, committed=%v)", report, err, ok && state.Committed())
		}
	})
}

func TestLegacyCommitAdapterReportContainsNoValues(t *testing.T) {
	keychain := &countingSecretRefKeychain{kind: model.SecretRefFile, values: map[string]string{}}
	store, err := New(t.TempDir()+"/store", stubRegen{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	const literal = "report-must-not-contain-this-value"
	report, err := store.Commit(context.Background(), "main", buildSecretProfile(t, "export API_KEY="+literal+"\n"), "report")
	if err != nil || len(report) != 1 || report[0].Name != "API_KEY" || report[0].StartLine != 1 {
		t.Fatalf("legacy report = (%#v, %v)", report, err)
	}
	if strings.Contains(fmt.Sprint(report), literal) || strings.Contains(fmt.Sprint(err), literal) {
		t.Fatal("legacy adapter exposed a literal secret")
	}
}

func TestWithheldReportContainsNoValues(t *testing.T) {
	secretType := reflect.TypeOf(model.WithheldSecret{})
	if secretType.NumField() != 2 || secretType.Field(0).Name != "Name" || secretType.Field(1).Name != "StartLine" {
		t.Fatalf("WithheldSecret shape can carry more than name/line: %v", secretType)
	}
	report := WithheldReport{{Name: "API_KEY", StartLine: 3}}
	if fmt.Sprint(report) != "[{API_KEY 3}]" {
		t.Fatalf("value-free report changed: %v", report)
	}
}
