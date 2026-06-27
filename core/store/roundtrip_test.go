package store_test

// This is the Phase 3 correctness gate: the Read(Commit(p)) round-trip pin,
// composed with the Phase 2 byte-identical oracle. It lives in the EXTERNAL test
// package (store_test) on purpose — it is the only place in core/store that wires
// the concrete zsh.Provider, so the production store package stays shell-agnostic
// (the same sanctioned end-to-end-wiring exception as core/ir/roundtrip_test.go
// and core/analyze/corpus_test.go).
//
// It pins three properties:
//  1. Read(Commit(p)) == p — a full profile (all 5 declarative classes + an
//     explicit override) survives Commit (plumbing-to-branch) and Read (git show)
//     byte-for-byte across every derived field (D-01), and the returned
//     WithheldReport is empty (no secrets in this fixture; exclusion is Plan 03).
//  2. The empty-profile edge: Read(Commit(model.Profile{})) == model.Profile{}.
//  3. Compose with the Phase 2 oracle: regenerating profile.zsh from the READ-BACK
//     profile through zsh.Provider{} sources (under `zsh -f`) to a byte-identical
//     model.IdentitySet vs the original source — proving a stored+read profile
//     still yields identical runtime behavior, with dynamic values late-bound.
//
// Skip-guarded on git for (1)/(2) and additionally on zsh for the (3) oracle step.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

// roundTripSrc is the representative fixture covering ALL 5 declarative classes
// (static env, dynamic env $HOME/go, dynamic PATH, alias, option, function) — the
// shapes Commit must round-trip faithfully and the oracle must re-source
// identically. It deliberately omits the Phase 2 adversarial/imperative shapes:
// the store round-trip pins the DECLARATIVE classes (those that pass through the
// Regenerator), and the Phase 2 oracle already pins the imperative shapes.
const roundTripSrc = "export EDITOR=nvim\n" +
	"export GOPATH=$HOME/go\n" +
	"export PATH=$HOME/bin:$PATH\n" +
	"alias gs='git status'\n" +
	"setopt EXTENDED_GLOB\n" +
	"greet() { echo hi }\n"

// buildProfile parses roundTripSrc through the concrete provider and the agnostic
// ir.Build, yielding a faithful model.Profile with real Managed verdicts. It then
// flips one entry's Override to OverrideManaged so the round-trip also pins that an
// explicit override (an orthogonal axis, D-07) survives serialization.
func buildProfile(t *testing.T) model.Profile {
	t.Helper()
	p := zsh.Provider{}
	blocks, err := p.Parse([]byte(roundTripSrc))
	if err != nil {
		t.Fatalf("Parse returned an error (regression): %v", err)
	}
	profile := ir.Build(blocks, p)
	// Force one declarative entry's override so the round-trip proves Override
	// (D-07) is a persisted field, not just the auto Managed verdict.
	for i := range profile.Entries {
		if len(profile.Entries[i].Names) > 0 && profile.Entries[i].Names[0] == "EDITOR" {
			profile.Entries[i].Override = model.OverrideManaged
		}
	}
	return profile
}

// newRoundTripStore builds a Store over a fresh temp bare repo with the concrete
// zsh.Provider{} as the Regenerator and a nil KeychainDriver (this plan's profiles
// are secret-free; exclusion is Plan 03). It is the FINAL 3-arg New form.
func newRoundTripStore(t *testing.T) *store.Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store round-trip pin")
	}
	s, err := store.New(t.TempDir(), zsh.Provider{}, nil)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s
}

// TestRoundTripReadCommit pins property (1): a full profile committed via plumbing
// reads back byte-identical across every derived field, and nothing is withheld.
func TestRoundTripReadCommit(t *testing.T) {
	s := newRoundTripStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	p := buildProfile(t)

	report, err := s.Commit(ctx, "main", p, "round-trip fixture")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(report) != 0 {
		t.Errorf("WithheldReport = %v, want empty (no secrets in this fixture; exclusion is Plan 03)", report)
	}

	got, err := s.Read(ctx, "main")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("Read(Commit(p)) != p\n got=%#v\nwant=%#v", got, p)
	}
}

// TestRoundTripEmptyProfile pins property (2): a zero-entry profile commits and
// reads back as an empty profile (guards the no-entries edge in the plumbing and
// MarshalProfile).
func TestRoundTripEmptyProfile(t *testing.T) {
	s := newRoundTripStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	report, err := s.Commit(ctx, "main", model.Profile{}, "empty")
	if err != nil {
		t.Fatalf("Commit(empty): %v", err)
	}
	if len(report) != 0 {
		t.Errorf("WithheldReport = %v, want empty for an empty profile", report)
	}

	got, err := s.Read(ctx, "main")
	if err != nil {
		t.Fatalf("Read(empty): %v", err)
	}
	if !reflect.DeepEqual(got, model.Profile{}) {
		t.Errorf("Read(Commit(empty)) = %#v, want empty model.Profile{}", got)
	}
}

// TestRoundTripComposeOracle pins property (3): regenerating profile.zsh from the
// READ-BACK profile sources to a byte-identical IdentitySet vs the original source
// under `zsh -f`, across all 5 declarative classes — the stored+read profile still
// yields identical runtime behavior, dynamic values late-bound (never resolved).
// This is where the Phase 3 round-trip composes with the Phase 2 oracle.
func TestRoundTripComposeOracle(t *testing.T) {
	s := newRoundTripStore(t) // skips if git absent
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping compose-with-Phase-2-oracle step")
	}
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	p := buildProfile(t)
	if _, err := s.Commit(ctx, "main", p, "round-trip fixture"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, err := s.Read(ctx, "main")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// Regenerate profile.zsh from the read-back profile via the concrete provider
	// (the same agnostic emitter the store uses internally), then re-source it.
	provider := zsh.Provider{}
	regen := ir.Regenerate(got, provider)

	dir := t.TempDir()
	origPath := filepath.Join(dir, "orig.zsh")
	regenPath := filepath.Join(dir, "regen.zsh")
	if err := os.WriteFile(origPath, []byte(roundTripSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regenPath, regen, 0o644); err != nil {
		t.Fatal(err)
	}

	// Separate error variables so a second-call failure never surfaces ambiguously.
	origIDS, origErr := provider.Introspect(origPath)
	regenIDS, regenErr := provider.Introspect(regenPath)
	if origErr != nil || !origIDS.Available {
		t.Fatalf("introspect(original) failed: err=%v available=%v", origErr, origIDS.Available)
	}
	if regenErr != nil || !regenIDS.Available {
		t.Fatalf("introspect(read-back regen) failed: err=%v available=%v\nregen=%q", regenErr, regenIDS.Available, regen)
	}

	// Byte-identical state across all 5 declarative classes (the Phase 2 invariant,
	// now proven through a store Commit+Read round-trip).
	if !reflect.DeepEqual(origIDS.Aliases, regenIDS.Aliases) {
		t.Errorf("Aliases differ after store round-trip:\n orig=%v\n regen=%v", origIDS.Aliases, regenIDS.Aliases)
	}
	if !reflect.DeepEqual(origIDS.Functions, regenIDS.Functions) {
		t.Errorf("Functions differ after store round-trip:\n orig=%v\n regen=%v", origIDS.Functions, regenIDS.Functions)
	}
	if !reflect.DeepEqual(origIDS.Env, regenIDS.Env) {
		t.Errorf("Env differ after store round-trip:\n orig=%v\n regen=%v", origIDS.Env, regenIDS.Env)
	}
	if !reflect.DeepEqual(origIDS.Options, regenIDS.Options) {
		t.Errorf("Options differ after store round-trip:\n orig=%v\n regen=%v", origIDS.Options, regenIDS.Options)
	}
	if !reflect.DeepEqual(origIDS.Path, regenIDS.Path) {
		t.Errorf("Path differ after store round-trip:\n orig=%v\n regen=%v", origIDS.Path, regenIDS.Path)
	}

	// Class-presence sanity: prove these are two non-empty equal tables, not two
	// empty-but-equal ones (a vacuous pass).
	if !origIDS.Functions["greet"] || !regenIDS.Functions["greet"] {
		t.Errorf("greet absent from a snapshot (function regen not proven through the store): orig=%v regen=%v", origIDS.Functions, regenIDS.Functions)
	}
	if !origIDS.Options["extendedglob"] || !regenIDS.Options["extendedglob"] {
		t.Errorf("extendedglob absent from a snapshot (setopt regen not proven through the store): orig=%v regen=%v", origIDS.Options, regenIDS.Options)
	}
	if !origIDS.Env["GOPATH"] || !regenIDS.Env["GOPATH"] {
		t.Errorf("GOPATH absent from a snapshot (dynamic env round-trip not proven through the store): orig=%v regen=%v", origIDS.Env, regenIDS.Env)
	}
}
