package store

// Internal (package store) tests for the concrete KeychainDriver backends. They
// exercise the vault fallback with no external binary (so they run everywhere), the
// per-backend Kind() contract (the root fix for "SecretRef stamped with the wrong
// backend"), and — guarded on exec.LookPath("security") — a real macOS keychain
// round-trip. Raw keychain stderr must never leak into a returned error (D-11).

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

// TestKindPerBackend pins the concern-#2 contract: each backend reports its OWN
// kind so excludeSecrets can stamp the matching SecretRef.Kind. The vault fallback
// is file-kind (NOT keychain) — the distinction a machine without security/
// secret-tool relies on.
func TestKindPerBackend(t *testing.T) {
	if got := (macOSKeychain{}).Kind(); got != model.SecretRefKeychain {
		t.Errorf("macOSKeychain.Kind() = %q, want %q", got, model.SecretRefKeychain)
	}
	if got := (linuxKeychain{}).Kind(); got != model.SecretRefKeychain {
		t.Errorf("linuxKeychain.Kind() = %q, want %q", got, model.SecretRefKeychain)
	}
	if got := newVaultKeychain(t.TempDir()).Kind(); got != model.SecretRefFile {
		t.Errorf("vaultKeychain.Kind() = %q, want %q (file, not keychain)", got, model.SecretRefFile)
	}
}

// TestVaultRoundTrip proves the vault backend Store/Retrieve/Delete round-trips a
// value with no external binary, the on-disk file is mode 0600, and the vault path
// is a SIBLING of the bare repo dir (never inside its object store).
func TestVaultRoundTrip(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "zsh-pro") // the (would-be) bare repo dir
	v := newVaultKeychain(repoDir)

	// The vault must NOT live inside the bare repo dir (it would sit alongside
	// objects/ and refs/). It is a sibling under the same parent.
	if filepath.Dir(v.path) == repoDir {
		t.Fatalf("vault path %q is inside the bare repo dir %q; must be a sibling", v.path, repoDir)
	}
	if filepath.Dir(v.path) != filepath.Dir(repoDir) {
		t.Fatalf("vault path %q is not a sibling of the repo dir %q", v.path, repoDir)
	}

	if err := v.Store("API_KEY", "sk-xyz"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := v.Retrieve("API_KEY")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got != "sk-xyz" {
		t.Errorf("Retrieve = %q, want sk-xyz", got)
	}

	// 0600 perms (owner read/write only) — ASVS V12.
	info, err := os.Stat(v.path)
	if err != nil {
		t.Fatalf("Stat vault: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("vault file mode = %o, want 600", perm)
	}

	// A second secret coexists; Delete removes only the named one.
	if err := v.Store("TOKEN", "tok-1"); err != nil {
		t.Fatalf("Store(TOKEN): %v", err)
	}
	if err := v.Delete("API_KEY"); err != nil {
		t.Fatalf("Delete(API_KEY): %v", err)
	}
	if _, err := v.Retrieve("API_KEY"); err == nil {
		t.Error("Retrieve(API_KEY) after Delete = nil error, want not-found")
	}
	if got, err := v.Retrieve("TOKEN"); err != nil || got != "tok-1" {
		t.Errorf("Retrieve(TOKEN) after deleting API_KEY = (%q, %v), want (tok-1, nil)", got, err)
	}
}

// TestVaultRetrieveNotFoundIsPhrased proves a missing key returns the zsh-pro
// sentinel, never a raw filesystem/keychain message (D-11).
func TestVaultRetrieveNotFoundIsPhrased(t *testing.T) {
	v := newVaultKeychain(t.TempDir())
	_, err := v.Retrieve("MISSING")
	if !errors.Is(err, ErrSecretBackendUnavailable) {
		t.Errorf("Retrieve(missing) = %v, want ErrSecretBackendUnavailable", err)
	}
	if err != nil && !strings.HasPrefix(err.Error(), "zsh-pro:") {
		t.Errorf("error %q is not zsh-pro-phrased", err.Error())
	}
}

// TestNewOSKeychainDriverNonNil proves the selector never returns nil — it always
// at least falls back to the vault — so excludeSecrets and the composition root can
// rely on a non-nil driver.
func TestNewOSKeychainDriverNonNil(t *testing.T) {
	if kc := NewOSKeychainDriver(t.TempDir()); kc == nil {
		t.Fatal("NewOSKeychainDriver returned nil; it must always at least fall back to the vault")
	}
}

// TestMacOSKeychainRoundTrip exercises the real `security` backend when present
// (skipped otherwise). It Store/Retrieve/Delete round-trips a test-scoped key and
// cleans up via t.Cleanup so it never leaves an entry in the developer's keychain.
func TestMacOSKeychainRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security not installed; skipping macOS keychain round-trip")
	}
	m := macOSKeychain{}
	key := "ZSHPRO_TEST_" + t.Name()
	t.Cleanup(func() { _ = m.Delete(key) })

	if err := m.Store(key, "sk-roundtrip"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := m.Retrieve(key)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got != "sk-roundtrip" {
		t.Errorf("Retrieve = %q, want sk-roundtrip", got)
	}
	if err := m.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestMacOSKeychainNotFoundIsPhrased proves a not-found retrieve maps to the
// zsh-pro sentinel, never the raw `SecKeychain...: The specified item could not be
// found` stderr (D-11, T-03-01).
func TestMacOSKeychainNotFoundIsPhrased(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security not installed; skipping macOS keychain not-found check")
	}
	m := macOSKeychain{}
	key := "ZSHPRO_TEST_DEFINITELY_ABSENT_" + t.Name()
	_ = m.Delete(key) // ensure absent

	_, err := m.Retrieve(key)
	if err == nil {
		t.Fatal("Retrieve(absent) = nil error, want a not-found error")
	}
	if !errors.Is(err, ErrSecretBackendUnavailable) {
		t.Errorf("Retrieve(absent) = %v, want ErrSecretBackendUnavailable", err)
	}
	if strings.Contains(err.Error(), "SecKeychain") {
		t.Errorf("returned error leaked raw keychain stderr: %q", err.Error())
	}
}

// TestVaultMultiLineRoundTrip is the WR-01 pin: a multi-line value (a PEM private
// key — PRIVATE_KEY matches secretRe) round-trips through the vault intact. Before
// the base64 encoding, Store stripped every '\n', concatenating the lines into an
// unrecoverable blob; now Store/Retrieve preserve arbitrary bytes byte-for-byte.
func TestVaultMultiLineRoundTrip(t *testing.T) {
	v := newVaultKeychain(t.TempDir())

	const pem = "-----BEGIN KEY-----\nLINE1\nLINE2\n-----END KEY-----\n"
	if err := v.Store("PRIVATE_KEY", pem); err != nil {
		t.Fatalf("Store(multi-line): %v", err)
	}

	got, err := v.Retrieve("PRIVATE_KEY")
	if err != nil {
		t.Fatalf("Retrieve(PRIVATE_KEY): %v", err)
	}
	if got != pem {
		t.Errorf("multi-line value corrupted by the vault:\n got=%q\nwant=%q", got, pem)
	}
	// Belt-and-suspenders: the value's newlines really survived (the old code
	// concatenated them away, which this exact-equality check above already catches,
	// but assert the line count explicitly so a regression names the symptom).
	if n := strings.Count(got, "\n"); n != strings.Count(pem, "\n") {
		t.Errorf("retrieved value has %d newlines, want %d (newline stripping regression)", n, strings.Count(pem, "\n"))
	}

	// The on-disk vault must still be a single physical line per secret (the embedded
	// newlines live INSIDE the base64, so one secret never spills across lines).
	raw, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatalf("ReadFile vault: %v", err)
	}
	if lines := strings.Count(strings.TrimRight(string(raw), "\n"), "\n"); lines != 0 {
		t.Errorf("vault holds %d line breaks for one secret, want 0 (multi-line value leaked into the line format):\n%s", lines, raw)
	}
	// And the raw plaintext must NOT appear on disk verbatim (it is base64-encoded).
	if strings.Contains(string(raw), "BEGIN KEY") {
		t.Errorf("vault stored the value as plaintext, not base64:\n%s", raw)
	}
}

// TestVaultSaveDeterministicOrder is the WR-02 pin: the vault file serializes its
// keys in a stable (sorted) order, so an otherwise-identical vault is byte-identical
// across rewrites instead of churning with Go's randomized map iteration. It writes a
// 5-key vault, then re-saves the SAME entries many times and asserts the on-disk bytes
// never change.
func TestVaultSaveDeterministicOrder(t *testing.T) {
	v := newVaultKeychain(t.TempDir())

	// Insert in a deliberately non-sorted order; the file must come out sorted.
	for _, k := range []string{"ZULU", "ALPHA", "MIKE", "BRAVO", "OSCAR"} {
		if err := v.Store(k, "val-"+k); err != nil {
			t.Fatalf("Store(%s): %v", k, err)
		}
	}

	first, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatalf("ReadFile vault: %v", err)
	}

	// Keys must be sorted on disk (ALPHA before BRAVO before MIKE ...).
	wantOrder := "ALPHA=\nBRAVO=\nMIKE=\nOSCAR=\nZULU="
	gotOrder := strings.Join(lineKeysPrefix(string(first)), "\n")
	if gotOrder != wantOrder {
		t.Errorf("vault key order = %q, want sorted %q", gotOrder, wantOrder)
	}

	// Re-saving the identical entry set must produce byte-identical output every time
	// (the WR-02 churn the map-iteration order used to cause). 20 passes: a randomized
	// 5-key map reorders with overwhelming probability across that many rewrites.
	entries, err := v.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := 0; i < 20; i++ {
		if err := v.save(entries); err != nil {
			t.Fatalf("save (pass %d): %v", i, err)
		}
		again, err := os.ReadFile(v.path)
		if err != nil {
			t.Fatalf("ReadFile (pass %d): %v", i, err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("vault bytes changed across an identical re-save (pass %d) — non-deterministic order:\nfirst=%q\nagain=%q", i, first, again)
		}
	}
}

// lineKeysPrefix returns the `KEY=` prefix of each non-empty vault line (the key plus
// the '=' separator, with the base64 value half stripped) so a test can assert key
// ORDER without depending on the encoded value bytes.
func lineKeysPrefix(content string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if line == "" {
			continue
		}
		if i := strings.IndexByte(line, '='); i >= 0 {
			out = append(out, line[:i+1])
		}
	}
	return out
}
