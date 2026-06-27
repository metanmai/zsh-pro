package store

// Internal (package store) test so it can exercise the unexported validBranchName
// path-traversal/argument-injection guard directly (T-03-02). The git-driven
// orchestration tests are skip-guarded on exec.LookPath("git") exactly like
// core/ir/roundtrip_test.go, and construct the Store with a nil KeychainDriver
// because literal-secret exclusion is Plan 03 (this plan's New takes the keychain
// in its FINAL form but does not yet use it). A tiny stub Regenerator stands in for
// the real zsh.Provider here — the byte-identical oracle compose lives in the
// external roundtrip_test.go where wiring zsh.Provider{} is sanctioned.

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"zsh-pro/core/model"
)

// stubRegen is a no-op shell.Regenerator: Task 1 exercises Init/Branches/Current/
// Create/Checkout, none of which regenerate .zsh, so the body is never invoked.
type stubRegen struct{}

func (stubRegen) Regenerate(e model.Entry) string { return e.Text }

// newTestStore builds a Store over a fresh temp dir with a nil KeychainDriver,
// skipping the whole test when git is absent (mirrors roundtrip_test.go).
func newTestStore(t *testing.T) *Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store orchestration tests")
	}
	s, err := New(t.TempDir(), stubRegen{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// TestInitIdempotent proves Init creates a bare repo with a `main` baseline and is a
// safe no-op on re-run (D-05/D-06): isBareRepo is true after both calls and Branches
// returns exactly ["main"].
func TestInitIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init (first): %v", err)
	}
	if !s.git.isBareRepo(ctx) {
		t.Fatal("after first Init the repo is not bare")
	}
	branches, err := s.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if !reflect.DeepEqual(branches, []string{"main"}) {
		t.Errorf("after Init, Branches = %v, want [main]", branches)
	}

	// Second Init must be an idempotent no-op (never clobber).
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init (second): %v", err)
	}
	if !s.git.isBareRepo(ctx) {
		t.Fatal("after second Init the repo is not bare")
	}
	branches, err = s.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches (after re-Init): %v", err)
	}
	if !reflect.DeepEqual(branches, []string{"main"}) {
		t.Errorf("after re-Init, Branches = %v, want [main] (no clobber)", branches)
	}
}

// TestCurrent pins D-13: ZSHPRO_PROFILE carries the profile name only; unset => main.
func TestCurrent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store orchestration tests")
	}
	s, err := New(t.TempDir(), stubRegen{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Unset (t.Setenv to "" then the loader treats empty as unset) => main.
	t.Setenv("ZSHPRO_PROFILE", "")
	if got := s.Current(); got != "main" {
		t.Errorf("Current with ZSHPRO_PROFILE unset = %q, want main", got)
	}

	t.Setenv("ZSHPRO_PROFILE", "work")
	if got := s.Current(); got != "work" {
		t.Errorf("Current with ZSHPRO_PROFILE=work = %q, want work", got)
	}
}

// TestCreate proves Create forks main (PROF-02), Branches stays sorted, and a
// duplicate Create returns ErrProfileExists.
func TestCreate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := s.Create(ctx, "work"); err != nil {
		t.Fatalf("Create(work): %v", err)
	}
	branches, err := s.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if !reflect.DeepEqual(branches, []string{"main", "work"}) {
		t.Errorf("after Create(work), Branches = %v, want [main work] (sorted)", branches)
	}

	// The new profile forks main: its tip equals main's tip (shared common ancestor).
	mainTip, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatalf("revParse(main): %v", err)
	}
	workTip, err := s.git.revParse(ctx, "refs/heads/work")
	if err != nil {
		t.Fatalf("revParse(work): %v", err)
	}
	if mainTip != workTip {
		t.Errorf("Create did not fork main: work tip %q != main tip %q", workTip, mainTip)
	}

	// A duplicate Create must fail with ErrProfileExists.
	if err := s.Create(ctx, "work"); !errors.Is(err, ErrProfileExists) {
		t.Errorf("Create(work) twice = %v, want ErrProfileExists", err)
	}
}

// TestCheckout proves Checkout validates branch existence (D-13 read-half):
// a missing profile returns ErrProfileNotFound, an existing one returns nil.
func TestCheckout(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := s.Checkout(ctx, "nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("Checkout(nope) = %v, want ErrProfileNotFound", err)
	}
	if err := s.Create(ctx, "work"); err != nil {
		t.Fatalf("Create(work): %v", err)
	}
	if err := s.Checkout(ctx, "work"); err != nil {
		t.Errorf("Checkout(work) after Create = %v, want nil", err)
	}
}

// TestReadDistinguishesAbsentFromUncommitted pins the WR-03 contract: Read returns
// the EMPTY profile for a branch that exists but was never committed to (fresh `main`
// after Init, and a freshly Create'd fork), and reserves ErrProfileNotFound for a
// genuinely-absent branch. Before the fix, Read conflated the two: a fresh `main`
// (whose baseline is an empty-tree root commit with no profile.json) errored with
// ErrProfileNotFound even though Branches/Checkout treat it as a real profile.
func TestReadDistinguishesAbsentFromUncommitted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// `main` exists (Branches/Checkout agree) but has no profile.json yet => empty
	// profile, NOT an error.
	got, err := s.Read(ctx, "main")
	if err != nil {
		t.Errorf("Read(main) on fresh init = %v, want nil (main exists but is uncommitted)", err)
	}
	if !reflect.DeepEqual(got, model.Profile{}) {
		t.Errorf("Read(main) on fresh init = %#v, want empty model.Profile{}", got)
	}

	// A freshly Create'd fork is likewise present-but-uncommitted (it shares main's
	// empty baseline tip) => empty profile, not ErrProfileNotFound.
	if err := s.Create(ctx, "work"); err != nil {
		t.Fatalf("Create(work): %v", err)
	}
	got, err = s.Read(ctx, "work")
	if err != nil {
		t.Errorf("Read(work) on a fresh fork = %v, want nil (work exists but is uncommitted)", err)
	}
	if !reflect.DeepEqual(got, model.Profile{}) {
		t.Errorf("Read(work) on a fresh fork = %#v, want empty model.Profile{}", got)
	}

	// A genuinely-absent branch still surfaces ErrProfileNotFound (the reserved case).
	if _, err := s.Read(ctx, "does-not-exist"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("Read(does-not-exist) = %v, want ErrProfileNotFound (genuinely absent branch)", err)
	}
}

// TestValidBranchName pins the V5 + path-traversal guard (T-03-02): argument
// injection (leading '-'), path traversal ('../etc' and embedded 'foo/../bar'), and
// the empty name are all rejected, while legitimate names ('work-1', 'v1.0') pass.
// This is the test that gives the unexported guard a real caller.
func TestValidBranchName(t *testing.T) {
	reject := []string{
		"",           // empty
		"-rf",        // leading dash — argument injection
		"../etc",     // path traversal
		"foo/../bar", // embedded path traversal (charset alone would admit this)
		"a/./b",      // embedded "." component
		"bad name",   // space — outside the charset
		"semi;colon", // shell metacharacter — outside the charset
		"$(inject)",  // command-substitution chars — outside the charset
	}
	for _, name := range reject {
		if err := validBranchName(name); err == nil {
			t.Errorf("validBranchName(%q) = nil, want a rejection error", name)
		}
	}

	accept := []string{"work-1", "v1.0", "main", "feature/x", "a.b_c-d"}
	for _, name := range accept {
		if err := validBranchName(name); err != nil {
			t.Errorf("validBranchName(%q) = %v, want nil", name, err)
		}
	}
}
