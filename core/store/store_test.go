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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

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

func TestExpectedRevisionPresenceInvariant(t *testing.T) {
	initID, err := model.NewInstallInitializationID()
	if err != nil {
		t.Fatal(err)
	}
	token, err := model.NewIngestTransactionID()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := model.NewExpectedRevision("0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := model.NewIngestBaseline(initID, token, false, expected, model.Profile{}, false); err == nil {
		t.Fatal("absent ref accepted a present expected revision")
	}
	if _, err := model.NewIngestBaseline(initID, token, true, nil, model.Profile{}, false); err == nil {
		t.Fatal("present ref accepted a missing expected revision")
	}
	if _, err := model.NewIngestBaseline(initID, token, false, nil, model.Profile{}, false); err != nil {
		t.Fatalf("valid absent baseline: %v", err)
	}
	if _, err := model.NewIngestBaseline(initID, token, true, expected, model.Profile{}, true); err != nil {
		t.Fatalf("valid present baseline: %v", err)
	}
	for _, bad := range []string{"", "not-an-object", "0123456789abcdef0123456789abcdef0123456g"} {
		if _, err := model.NewExpectedRevision(bad); err == nil {
			t.Errorf("NewExpectedRevision(%q) accepted an invalid object ID", bad)
		}
	}
}

func TestIngestTransactionEvidenceComplete(t *testing.T) {
	assertDistinctStrings := func(name string, values ...string) {
		t.Helper()
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" {
				t.Fatalf("%s contains an empty evidence value", name)
			}
			if _, ok := seen[value]; ok {
				t.Fatalf("%s repeats evidence value %q", name, value)
			}
			seen[value] = struct{}{}
		}
	}

	assertDistinctStrings("lifecycle",
		string(model.IngestLifecycleProvisional), string(model.IngestLifecycleActive),
		string(model.IngestLifecycleCommitting), string(model.IngestLifecycleFinalizing),
		string(model.IngestLifecycleTerminal))
	assertDistinctStrings("commit status",
		string(model.IngestCommitNotCommitted), string(model.IngestCommitCommitted),
		string(model.IngestCommitConflict), string(model.IngestCommitRecoveryRequired))
	assertDistinctStrings("ref state",
		string(model.IngestRefExpected), string(model.IngestRefCandidate),
		string(model.IngestRefOther), string(model.IngestRefUnknown))
	assertDistinctStrings("backend state",
		string(model.IngestBackendUnchanged), string(model.IngestBackendApplied),
		string(model.IngestBackendRestored), string(model.IngestBackendUncertain))
	assertDistinctStrings("object state",
		string(model.IngestObjectsQuarantined), string(model.IngestObjectsRemoved),
		string(model.IngestObjectsPublished), string(model.IngestObjectsRetained),
		string(model.IngestObjectsUncertain))
	assertDistinctStrings("cleanup state",
		string(model.QuarantineCleanupRemoved), string(model.QuarantineCleanupRetained),
		string(model.QuarantineCleanupUncertain))
	assertDistinctStrings("failure code",
		string(model.IngestFailureNone), string(model.IngestFailureInvalidAuthority),
		string(model.IngestFailureBaselineRead), string(model.IngestFailureQuarantine),
		string(model.IngestFailureCleanup))

	report := model.WithheldReport{{Name: "API_KEY", StartLine: 7}}
	if report[0].Name != "API_KEY" || report[0].StartLine != 7 {
		t.Fatalf("value-free withheld report changed shape: %#v", report)
	}
}

func TestInstallInitializationIDBindsStoreAndRoot(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	root := filepath.Join(t.TempDir(), "store")
	s, err := New(root, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := s.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if initialization.ID().IsZero() {
		t.Fatal("InitForInstall returned a zero initialization ID")
	}
	if got := s.installInitializationRoot(initialization.ID()); got != root {
		t.Fatalf("initialization root = %q, want %q", got, root)
	}

	other, err := New(filepath.Join(t.TempDir(), "other"), stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.BeginIngest(context.Background(), initialization.ID()); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("cross-Store BeginIngest = %v, want ErrInvalidIngestAuthority", err)
	}
	if err := other.rollbackInstallInitialization(initialization.ID()); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("cross-Store rollback = %v, want ErrInvalidIngestAuthority", err)
	}
}

func TestBeginIngestRejectsWrongOrCrossStoreInitialization(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	newInstallStore := func(t *testing.T) *Store {
		t.Helper()
		root := filepath.Join(t.TempDir(), "store")
		store, err := New(root, stubRegen{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	s := newInstallStore(t)
	initialization, err := s.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	other := newInstallStore(t)
	otherInitialization, err := other.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]model.InstallInitializationID{
		"zero":        {},
		"cross-store": otherInitialization.ID(),
	} {
		t.Run(name, func(t *testing.T) {
			outcome, err := s.BeginIngest(context.Background(), id)
			if !errors.Is(err, ErrInvalidIngestAuthority) {
				t.Fatalf("BeginIngest = (%#v, %v), want invalid authority", outcome, err)
			}
			if !outcome.TransactionID.IsZero() {
				t.Fatalf("invalid authority reserved token %#v", outcome.TransactionID)
			}
		})
	}
	if s.installInitializationTransactionCount(initialization.ID()) != 0 {
		t.Fatal("invalid Begin attempts were registered under the valid initialization")
	}
}

func TestInstallInitializationAggregateIncludesFailedBegin(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	root := filepath.Join(t.TempDir(), "store")
	s, err := New(root, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := s.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.beginAfterReserve = func() error { return ErrGitCommand }
	outcome, err := s.BeginIngest(context.Background(), initialization.ID())
	if !errors.Is(err, ErrGitCommand) {
		t.Fatalf("BeginIngest = (%#v, %v), want injected failure", outcome, err)
	}
	if outcome.TransactionID.IsZero() {
		t.Fatal("failed Begin did not return its pre-I/O reserved token")
	}
	if outcome.Lifecycle != model.IngestLifecycleTerminal || outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("failed Begin evidence = %#v, want clean immutable terminal", outcome)
	}
	if got := s.installInitializationTransactionCount(initialization.ID()); got != 1 {
		t.Fatalf("registered token count = %d, want 1", got)
	}
	if !s.initializerRollbackSafe(initialization.ID()) {
		t.Fatal("clean failed Begin should keep initializer rollback safe")
	}
	if err := initialization.Rollback(); err != nil {
		t.Fatalf("rollback after clean failed Begin: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback left created store: %v", err)
	}
}

func TestInitForInstallRejectsChangedOrMismatchedOwnership(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	root := filepath.Join(t.TempDir(), "store")
	s, err := New(root, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := s.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	forged, err := model.NewInstallInitializationID()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.rollbackInstallInitialization(forged); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("forged rollback = %v, want ErrInvalidIngestAuthority", err)
	}
	foreign := filepath.Join(root, "foreign")
	if err := os.WriteFile(foreign, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := initialization.Rollback(); err == nil {
		t.Fatal("rollback accepted a changed sealed Store tree")
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "preserve" {
		t.Fatalf("changed-tree refusal lost foreign bytes: %q, %v", got, err)
	}
	if err := initialization.Rollback(); err == nil {
		t.Fatal("repeated changed-tree rollback did not replay its failure")
	}
}

func TestStoreTransactionRootLockProcessMatrix(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("cross-process Store transaction locks are unsupported on this platform")
	}
	if os.Getenv("ZSHPRO_STORE_LOCK_HELPER") == "1" {
		root := os.Getenv("ZSHPRO_STORE_LOCK_ROOT")
		ready := os.Getenv("ZSHPRO_STORE_LOCK_READY")
		release := os.Getenv("ZSHPRO_STORE_LOCK_RELEASE")
		err := withStoreRootTransactionLock(root, func(guard *storeRootTransactionGuard) error {
			if err := guard.withAuthenticatedMutation(func(*os.Root) error { return nil }); err != nil {
				return err
			}
			if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
				return err
			}
			if release == "" {
				return nil
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(release); err == nil {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return errors.New("timed out waiting for lock release signal")
		})
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(12)
		}
		return
	}

	waitForFile := func(path string, timeout time.Duration) error {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); err == nil {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
		return fmt.Errorf("timed out waiting for %s", path)
	}
	startHelper := func(root, ready, release string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStoreTransactionRootLockProcessMatrix$")
		cmd.Env = append(os.Environ(),
			"ZSHPRO_STORE_LOCK_HELPER=1",
			"ZSHPRO_STORE_LOCK_ROOT="+root,
			"ZSHPRO_STORE_LOCK_READY="+ready,
			"ZSHPRO_STORE_LOCK_RELEASE="+release,
		)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	signal := func(path string) {
		if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	base := t.TempDir()
	rootA := filepath.Join(base, "a", "store")
	rootB := filepath.Join(base, "b", "store")
	for _, root := range []string{rootA, rootB} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	readyA := filepath.Join(base, "ready-a")
	releaseA := filepath.Join(base, "release-a")
	holder := startHelper(rootA, readyA, releaseA)
	if err := waitForFile(readyA, 3*time.Second); err != nil {
		t.Fatal(err)
	}

	readySame := filepath.Join(base, "ready-same")
	releaseSame := filepath.Join(base, "release-same")
	contender := startHelper(rootA, readySame, releaseSame)
	if err := waitForFile(readySame, 200*time.Millisecond); err == nil {
		t.Fatal("same-root contender acquired while the first process held the lock")
	}

	readyDifferent := filepath.Join(base, "ready-different")
	releaseDifferent := filepath.Join(base, "release-different")
	different := startHelper(rootB, readyDifferent, releaseDifferent)
	if err := waitForFile(readyDifferent, 3*time.Second); err != nil {
		t.Fatal("different-root helper was unnecessarily blocked")
	}
	signal(releaseDifferent)
	if err := different.Wait(); err != nil {
		t.Fatalf("different-root helper: %v", err)
	}

	signal(releaseA)
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder: %v", err)
	}
	if err := waitForFile(readySame, 3*time.Second); err != nil {
		t.Fatal("same-root contender did not acquire after release")
	}
	signal(releaseSame)
	if err := contender.Wait(); err != nil {
		t.Fatalf("same-root contender: %v", err)
	}

	readyExit := filepath.Join(base, "ready-exit")
	exiting := startHelper(rootA, readyExit, "")
	if err := waitForFile(readyExit, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := exiting.Wait(); err != nil {
		t.Fatalf("exiting holder: %v", err)
	}
	readyAfterExit := filepath.Join(base, "ready-after-exit")
	releaseAfterExit := filepath.Join(base, "release-after-exit")
	afterExit := startHelper(rootA, readyAfterExit, releaseAfterExit)
	if err := waitForFile(readyAfterExit, 3*time.Second); err != nil {
		t.Fatal("process exit did not release the kernel lock")
	}
	signal(releaseAfterExit)
	if err := afterExit.Wait(); err != nil {
		t.Fatalf("post-exit holder: %v", err)
	}

	installRoot := filepath.Join(base, "install", "store")
	store, err := New(installRoot, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := withStoreRootTransactionLock(installRoot, func(*storeRootTransactionGuard) error { return nil }); err != nil {
		t.Fatal(err)
	}
	namespace, err := storeTransactionNamespacePath(installRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialization.Rollback(); err != nil {
		t.Fatalf("rollback with persistent transaction sibling: %v", err)
	}
	if _, err := os.Lstat(installRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh Store root survived rollback: %v", err)
	}
	if info, err := os.Stat(namespace); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("persistent transaction namespace = (%v, %v)", info, err)
	}
	if info, err := os.Stat(filepath.Join(namespace, "lock")); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("persistent transaction lock = (%v, %v)", info, err)
	}
}

func TestStoreTransactionRootLockRejectsUnsafeEntry(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("cross-process Store transaction locks are unsupported on this platform")
	}

	for _, test := range []struct {
		name  string
		setup func(t *testing.T, namespace string)
	}{
		{
			name: "namespace wrong mode",
			setup: func(t *testing.T, namespace string) {
				t.Helper()
				if err := os.Mkdir(namespace, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink lock",
			setup: func(t *testing.T, namespace string) {
				t.Helper()
				if err := os.Mkdir(namespace, 0o700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(filepath.Dir(namespace), "target")
				if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(namespace, "lock")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory lock",
			setup: func(t *testing.T, namespace string) {
				t.Helper()
				if err := os.Mkdir(namespace, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(namespace, "lock"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong lock mode",
			setup: func(t *testing.T, namespace string) {
				t.Helper()
				if err := os.Mkdir(namespace, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(namespace, "lock"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			namespace, err := storeTransactionNamespacePath(root)
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, namespace)
			called := false
			if err := withStoreRootTransactionLock(root, func(*storeRootTransactionGuard) error {
				called = true
				return nil
			}); err == nil {
				t.Fatal("unsafe transaction lock entry was accepted")
			}
			if called {
				t.Fatal("callback ran after unsafe lock authentication")
			}
		})
	}
}

func TestStoreTransactionRootLockUnavailableOrDiscarded(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("cross-process Store transaction locks are unsupported on this platform")
	}
	root := filepath.Join(t.TempDir(), "store")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	err := withStoreRootTransactionLock(root, func(guard *storeRootTransactionGuard) error {
		guard.discardForTest()
		return guard.withAuthenticatedMutation(func(*os.Root) error {
			mutations++
			return nil
		})
	})
	if err == nil {
		t.Fatal("discarded lock descriptor retained mutation authority")
	}
	if mutations != 0 {
		t.Fatalf("discarded lock performed %d namespace mutations", mutations)
	}
}
