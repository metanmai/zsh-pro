package store

// This file provides the three CONCRETE implementations of the KeychainDriver
// interface declared (in its FINAL form) in store.go (Plan 02) — it does NOT
// redeclare that interface. Each backend moves a name-scoped secret value to/from
// a system secret store and reports which backend it is via Kind(), so Plan 03's
// literal-secret exclusion can stamp the matching SecretRef.Kind on the committed
// pointer (a machine without a system keychain produces a `file`-kind reference
// that Ph4/5 dereferences from the vault, not via `security`).
//
// Every subprocess backend mirrors the introspect.go / git.go shape verbatim:
// context.WithTimeout(5s) + defer cancel, exec.CommandContext, a SEPARATE stderr
// buffer, and a zsh-pro-phrased typed error on failure — raw `security`/secret-tool
// stderr is NEVER surfaced (D-11, Pitfall 4, ASVS V7). Secret values flow via
// cmd.Stdin, never argv, so they never appear in `ps` (Pitfall 3, ASVS V7).
//
// Deref naming contract (RESEARCH Open Question 2, critical decision #3 — the
// scheme Ph4/5 reads back): secrets are GLOBAL-BY-NAME. The OS keychain backends
// use service "zsh-pro", account "<KEY>" (exactly one stored entry per secret name,
// referenced per-profile via a SecretRef); the vault backend uses one
// `KEY=base64(value)` line per name. Two profiles that both reference API_KEY share
// the one stored value — D-07's "value store keyed by name in one place, referenced
// per-profile".

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zsh-pro/core/model"
)

// keychainTimeout is the per-call subprocess timeout, mirroring git.go's gitTimeout
// and the 5s zsh -f timeout in the zsh provider's introspect.go.
const keychainTimeout = 5 * time.Second

// keychainService is the namespace under which every secret value is stored
// (service "zsh-pro", account = the secret name). It is the global-by-name deref
// contract Ph4/5 reads.
const keychainService = "zsh-pro"

// keychainTransportPrefix identifies the ASCII-only, versioned representation
// stored in OS keychains. Both `security` and `secret-tool` communicate values
// through streams whose output framing differs by platform; base64 means the
// stored payload can never be confused with that framing, while the version
// makes an incompatible legacy value fail closed instead of being corrupted.
const keychainTransportPrefix = "zsh-pro:v1:"

// vaultFileName is the basename of the git-ignored fallback vault. It lives as a
// SIBLING of the bare repo dir (never inside it), see newVaultKeychain.
const vaultFileName = ".zsh-pro-vault"

// Compile-time assertions that all three concrete backends satisfy the four-method
// KeychainDriver interface (Store/Retrieve/Delete/Kind) declared in store.go.
var (
	_ KeychainDriver = macOSKeychain{}
	_ KeychainDriver = linuxKeychain{}
	_ KeychainDriver = vaultKeychain{}
	_ KeychainDriver = runtimeVaultKeychain{}
)

// NewOSKeychainDriver selects the secret backend at runtime via exec.LookPath,
// mirroring the git/zsh absence guards: `security` present (macOS) -> macOSKeychain;
// else `secret-tool` present (Linux) -> linuxKeychain; else the git-ignored 0600
// vault file fallback rooted at a sibling of the bare repo dir. It never returns
// nil — the vault fallback always succeeds — so callers (and excludeSecrets) can
// rely on a non-nil driver. Called at the composition root (main.go).
func NewOSKeychainDriver(dir string) KeychainDriver {
	if _, err := exec.LookPath("security"); err == nil {
		return macOSKeychain{}
	}
	if _, err := exec.LookPath("secret-tool"); err == nil {
		return linuxKeychain{}
	}
	return newVaultKeychain(dir)
}

// newRuntimeKeychain selects the ordinary OS backends when available. Its
// file-vault fallback is different: it opens the sibling through the
// authenticated parent descriptor, never through filepath.Dir(ZSHPRO_HOME).
func newRuntimeKeychain(parent *os.File) KeychainDriver {
	if _, err := exec.LookPath("security"); err == nil {
		return macOSKeychain{}
	}
	if _, err := exec.LookPath("secret-tool"); err == nil {
		return linuxKeychain{}
	}
	return runtimeVaultKeychain{parent: parent}
}

// macOSKeychain stores secrets in the macOS login keychain via the `security`
// binary. Kind() is SecretRefKeychain.
type macOSKeychain struct{}

// Kind reports this as a keychain-class backend so the SecretRef Plan 03 stamps
// resolves via the OS keychain at deref time.
func (macOSKeychain) Kind() model.SecretRefKind { return model.SecretRefKeychain }

// Store writes a versioned transport of value under account=key, service=zsh-pro,
// updating in place (-U). The transport is piped via cmd.Stdin using the VERIFIED
// doubled-stdin form (value\nvalue\n — `security -w` with no value arg prompts AND
// asks to confirm), so the plaintext secret NEVER appears on argv / in `ps`.
// The arg list ends in `-w` with no trailing value element.
func (macOSKeychain) Store(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	transport := encodeKeychainTransport(value)
	cmd := exec.CommandContext(ctx, "security", "add-generic-password",
		"-a", key, "-s", keychainService, "-U", "-w")
	cmd.Stdin = strings.NewReader(transport + "\n" + transport + "\n")
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return mapKeychainError()
	}
	return nil
}

// Retrieve decodes the versioned value stored under account=key, service=zsh-pro.
// A missing macOS item exits with errSecItemNotFound's shell status (44); raw
// `SecKeychain...` stderr is never surfaced (D-11).
func (macOSKeychain) Retrieve(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", key, "-s", keychainService, "-w")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", mapMacOSKeychainRetrieveError(err)
	}
	return decodeKeychainTransport(out.Bytes())
}

// Delete removes the account=key, service=zsh-pro entry. A not-found delete is
// mapped to a zsh-pro-phrased error (raw stderr suppressed).
func (macOSKeychain) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "delete-generic-password",
		"-a", key, "-s", keychainService)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return mapKeychainError()
	}
	return nil
}

// linuxKeychain stores secrets in the Linux Secret Service (GNOME Keyring / KWallet)
// via libsecret's `secret-tool`. [CITED: man secret-tool] — this recipe is
// documented-only and was NOT exercised on the macOS development machine; it is
// guarded by NewOSKeychainDriver's LookPath probe and selected only when
// `secret-tool` is present. Kind() is SecretRefKeychain: secret-tool IS a
// keychain-class backend (the system secret service), so its references resolve via
// the keychain path at deref time.
type linuxKeychain struct{}

// Kind reports this as a keychain-class backend (the Linux system secret service).
func (linuxKeychain) Kind() model.SecretRefKind { return model.SecretRefKeychain }

// Store pipes a versioned transport of value via stdin (secret-tool reads the
// value from stdin natively, so plaintext never reaches argv) under the attribute
// schema `service zsh-pro key <key>`, which Retrieve/Delete look up identically.
func (linuxKeychain) Store(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secret-tool", "store",
		"--label=zsh-pro "+key, "service", keychainService, "key", key)
	cmd.Stdin = strings.NewReader(encodeKeychainTransport(value))
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return mapKeychainError()
	}
	return nil
}

// Retrieve decodes the versioned value looked up by the same attribute schema.
// libsecret returns exit status 1 with no stderr when there is no matching item;
// a status-1 error with diagnostics remains an unavailable backend.
func (linuxKeychain) Retrieve(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secret-tool", "lookup",
		"service", keychainService, "key", key)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", mapLinuxKeychainRetrieveError(err, errb.Len())
	}
	return decodeKeychainTransport(out.Bytes())
}

func encodeKeychainTransport(value string) string {
	return keychainTransportPrefix + base64.StdEncoding.EncodeToString([]byte(value))
}

// decodeKeychainTransport removes only a command-output record terminator. The
// encoded transport itself never contains CR or LF, so empty, trailing-newline,
// and multi-line plaintext values all decode byte-for-byte without broad trimming.
func decodeKeychainTransport(output []byte) (string, error) {
	transport := strings.TrimSuffix(string(output), "\n")
	transport = strings.TrimSuffix(transport, "\r")
	if !strings.HasPrefix(transport, keychainTransportPrefix) {
		return "", ErrSecretBackendUnavailable
	}
	value, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(transport, keychainTransportPrefix))
	if err != nil {
		return "", ErrSecretBackendUnavailable
	}
	return string(value), nil
}

func mapMacOSKeychainRetrieveError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 { // errSecItemNotFound modulo 256
		return ErrSecretNotFound
	}
	return ErrSecretBackendUnavailable
}

func mapLinuxKeychainRetrieveError(err error, stderrLen int) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stderrLen == 0 {
		return ErrSecretNotFound
	}
	return ErrSecretBackendUnavailable
}

// Delete clears the entry matching the attribute schema. [CITED]
func (linuxKeychain) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secret-tool", "clear",
		"service", keychainService, "key", key)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return mapKeychainError()
	}
	return nil
}

// vaultKeychain is the portable fallback used when no OS keychain CLI is present.
// It stores entries in a plain file, one `key=base64(value)` line per secret name,
// written with 0o600 perms (owner-only — ASVS V12, Security V12). The base64 value
// half keeps the line-per-secret format while preserving arbitrary bytes, including
// multi-line values (WR-01). Kind() is SecretRefFile
// (NOT keychain — this is exactly the distinction the SecretRef.Kind = kc.Kind()
// stamp relies on: a machine without `security`/`secret-tool` produces a file-kind
// reference that Ph4/5 dereferences from the vault, not via a missing `security`).
//
// There is NO encryption (forbidden, D-09): the protection is "not in git history",
// not at-rest crypto. That guarantee is enforced three ways — the file is
// .gitignore'd, it lives OUTSIDE the bare repo (see newVaultKeychain), and the
// commit plumbing recipe (Plan 02) stages ONLY profile.json + profile.zsh, so the
// vault path can never enter a committed tree.
type vaultKeychain struct {
	path string // absolute path to the vault file (a sibling of the bare repo dir)
}

// newVaultKeychain roots the vault file at a SIBLING of the bare repo dir, never a
// file directly inside it. D-04 makes the store dir ($ZSHPRO_HOME / XDG) the bare
// repo itself, so a file inside it would sit alongside objects/ and refs/; placing
// it at filepath.Join(filepath.Dir(repoDir), vaultFileName) keeps it under the same
// parent but out of the object store — defense-in-depth on top of the plumbing
// recipe (which already stages only profile.json/profile.zsh).
func newVaultKeychain(repoDir string) vaultKeychain {
	return vaultKeychain{path: filepath.Join(filepath.Dir(repoDir), vaultFileName)}
}

// Kind reports this as the file backend (the vault fallback) — the per-backend kind
// that lets exclusion stamp a file-kind SecretRef when no OS keychain is present.
func (vaultKeychain) Kind() model.SecretRefKind { return model.SecretRefFile }

// Store upserts key=value in the vault file, rewriting it 0o600. The value is
// base64-encoded (see save) so an arbitrary byte sequence — including a multi-line
// PEM key, which PRIVATE_KEY matches in secretRe — round-trips intact through the
// line-per-secret format (WR-01). The in-memory map holds the DECODED value; save
// encodes it. The value is never logged.
func (v vaultKeychain) Store(key, value string) error {
	entries, err := v.load()
	if err != nil {
		return err
	}
	entries[key] = value
	return v.save(entries)
}

// Retrieve returns the value stored under key, or a zsh-pro-phrased not-found error
// (never a raw filesystem error message).
func (v vaultKeychain) Retrieve(key string) (string, error) {
	entries, err := v.load()
	if err != nil {
		return "", err
	}
	val, ok := entries[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}

// Delete removes key from the vault file (a no-op if absent), rewriting it 0o600.
func (v vaultKeychain) Delete(key string) error {
	entries, err := v.load()
	if err != nil {
		return err
	}
	delete(entries, key)
	return v.save(entries)
}

// load reads the vault file into a name->value map of DECODED values. A missing
// file is an empty vault (not an error) so the first Store creates it. Each line is
// `key=base64(value)`; the value half is base64-decoded so multi-line / arbitrary
// bytes round-trip (WR-01). A line with no '=' or an undecodable value half is
// skipped (forward/back compatible: a hand-edited or legacy plaintext line that is
// not valid base64 is ignored rather than surfacing a corrupt value).
func (v vaultKeychain) load() (map[string]string, error) {
	b, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, ErrSecretBackendUnavailable
	}
	return decodeVault(b), nil
}

func decodeVault(b []byte) map[string]string {
	entries := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		k, enc, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		raw, decErr := base64.StdEncoding.DecodeString(enc)
		if decErr != nil {
			continue // skip a malformed / non-base64 value half
		}
		entries[k] = string(raw)
	}
	return entries
}

// runtimeVaultKeychain is the read-only file fallback for a descriptor-bound
// runtime emission. Store/Delete are intentionally unavailable: activating a
// profile must never mutate a vault while the helper holds a transient root.
type runtimeVaultKeychain struct {
	parent *os.File
}

func (runtimeVaultKeychain) Kind() model.SecretRefKind { return model.SecretRefFile }

func (runtimeVaultKeychain) Store(string, string) error { return ErrSecretBackendUnavailable }

func (runtimeVaultKeychain) Delete(string) error { return ErrSecretBackendUnavailable }

func (v runtimeVaultKeychain) Retrieve(key string) (string, error) {
	b, err := readRuntimeVault(v.parent)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrSecretNotFound
		}
		return "", ErrSecretBackendUnavailable
	}
	value, ok := decodeVault(b)[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

// save writes the name->value map back to the vault file with 0o600 perms (owner
// read/write only). os.WriteFile truncates+rewrites, so a Delete shrinks the file. A
// write failure is mapped to a zsh-pro-phrased error.
//
// Two determinism/safety properties (WR-01/WR-02): values are base64-encoded so an
// arbitrary byte sequence (including embedded newlines, e.g. a PEM key) round-trips
// through the line-per-secret format with no corruption; and keys are written in
// SORTED order (reusing the package-local sortStrings — no new import) so an
// otherwise-identical vault always serializes byte-identically instead of churning
// across Go's randomized map-iteration order.
func (v vaultKeychain) save(entries map[string]string) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sortStrings(keys)

	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(entries[k])))
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(v.path, buf.Bytes(), 0o600); err != nil {
		return ErrSecretBackendUnavailable
	}
	return nil
}

// mapKeychainError translates any keychain subprocess failure into a zsh-pro-phrased
// typed error. Like mapGitError it deliberately does NOT embed the raw stderr
// (D-11; T-03-01): the user never sees `SecKeychainSearchCopyNext: ...`. A
// not-found, a timeout, and a backend error all collapse to the same surfaced
// sentinel — the store treats "no secret backend / lookup failed" uniformly.
func mapKeychainError() error {
	return ErrSecretBackendUnavailable
}
