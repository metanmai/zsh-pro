package store

// Internal (package store) tests for the concrete KeychainDriver backends. They
// exercise the vault fallback with no external binary (so they run everywhere), the
// per-backend Kind() contract (the root fix for "SecretRef stamped with the wrong
// backend"), and — guarded on exec.LookPath("security") — a real macOS keychain
// round-trip. Raw keychain stderr must never leak into a returned error (D-11).

import (
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
