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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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
	info, err := os.Stat(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("initialized store mode = %#o, want 0700", got)
	}

	// Existing stores from before the runtime descriptor boundary were commonly
	// initialized at 0755. Re-init must repair a current-user store rather than
	// returning early because the bare repository already exists.
	if err := os.Chmod(s.dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Second Init must be an idempotent no-op for repository content while it
	// repairs the store root's private mode.
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
	info, err = os.Stat(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("reinitialized store mode = %#o, want 0700", got)
	}
}

func TestInitRefusesUnsafeExistingStoreRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store orchestration tests")
	}

	t.Run("symlink", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(t.TempDir(), "store-link")
		if err := os.Symlink(target, root); err != nil {
			t.Fatal(err)
		}
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Init(context.Background()); err == nil {
			t.Fatal("Init accepted a symlinked store root")
		}
		info, err := os.Lstat(root)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("Init changed symlink root: info=%v err=%v", info, err)
		}
		info, err = os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o755 {
			t.Fatalf("Init changed symlink target mode = %#o, want 0755", got)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "store-file")
		if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Init(context.Background()); err == nil {
			t.Fatal("Init accepted a regular-file store root")
		}
		info, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("Init changed regular-file root into %v", info.Mode())
		}
	})
}

func TestInitForInstallRollsBackOnlyCreatedStoreState(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store orchestration tests")
	}

	t.Run("new root and parents", func(t *testing.T) {
		home := t.TempDir()
		root := filepath.Join(home, ".local", "share", "zsh-pro")
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		transaction, err := s.InitForInstall(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if transaction.CreatedPath() != root {
			t.Fatalf("CreatedPath() = %q, want %q", transaction.CreatedPath(), root)
		}
		if !s.git.isBareRepo(context.Background()) {
			t.Fatal("transactional initialization did not create a bare repository")
		}
		if err := transaction.Rollback(); err != nil {
			t.Fatalf("rollback new store: %v", err)
		}
		for _, path := range []string{root, filepath.Dir(root), filepath.Dir(filepath.Dir(root))} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rollback left newly created path %s: %v", path, err)
			}
		}
	})

	t.Run("preexisting initialized migration", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "profiles")
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := s.Init(ctx); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		before, exists, err := snapshotStoreTree(root)
		if err != nil || !exists {
			t.Fatalf("snapshot initialized store = (%v, %v)", exists, err)
		}

		transaction, err := s.InitForInstall(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if transaction.CreatedPath() != "" {
			t.Fatalf("migration CreatedPath() = %q, want empty", transaction.CreatedPath())
		}
		if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("migrated store mode = %v, err=%v; want 0700", info.Mode().Perm(), err)
		}
		if err := transaction.Rollback(); err != nil {
			t.Fatalf("rollback migrated store: %v", err)
		}
		if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("restored store mode = %v, err=%v; want 0755", info.Mode().Perm(), err)
		}
		after, exists, err := snapshotStoreTree(root)
		if err != nil || !exists || !sameStoreTree(before, after) {
			t.Fatalf("rollback rewrote preexisting store: exists=%v err=%v", exists, err)
		}
	})

	t.Run("preexisting uninitialized directory", func(t *testing.T) {
		root := t.TempDir()
		before, exists, err := snapshotStoreTree(root)
		if err != nil || !exists {
			t.Fatalf("snapshot uninitialized directory = (%v, %v)", exists, err)
		}
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.InitForInstall(context.Background()); err == nil {
			t.Fatal("transactional initialization accepted a preexisting uninitialized directory")
		}
		after, exists, err := snapshotStoreTree(root)
		if err != nil || !exists || !sameStoreTree(before, after) {
			t.Fatalf("rejected initialization changed preexisting directory: exists=%v err=%v", exists, err)
		}
	})

	t.Run("changed new root is retained", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "profiles")
		s, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		transaction, err := s.InitForInstall(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		foreign := filepath.Join(root, "concurrent-user-file")
		if err := os.WriteFile(foreign, []byte("preserve me"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := transaction.Rollback(); err == nil {
			t.Fatal("rollback removed a root that no longer matched its created state")
		}
		if got, err := os.ReadFile(foreign); err != nil || string(got) != "preserve me" {
			t.Fatalf("rollback removed or rewrote concurrent data: %q, err=%v", got, err)
		}
	})
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
