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
	"strings"
	"sync"
	"testing"
	"time"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
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

func newWorktreeStore(t *testing.T, keychain KeychainDriver) *Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping committed worktree Store test")
	}
	store, err := New(t.TempDir(), zsh.Provider{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func exactWorktreeDocument() model.CommittedWorktree {
	empty := ""
	return model.NewCommittedWorktree(model.Profile{Entries: []model.Entry{
		{Text: "export EXPORTED=source", Kind: model.KindAssignment, Category: model.CatEnvironment, Names: []string{"EXPORTED"}, Exported: true, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeLiteral, RuntimeValue: &empty},
		{Text: "PLAIN=source", Kind: model.KindAssignment, Category: model.CatEnvironment, Names: []string{"PLAIN"}, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeLiteral, RuntimeValue: &empty},
	}}, model.LiveProjection{
		States: []model.LiveIdentityState{
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "EXPORTED"}, Value: model.ScalarLiveValue("it's\nexact")},
			{Identity: model.Identity{Kind: model.LiveEnv, Name: "PLAIN"}, Value: model.ScalarLiveValue("")},
			{Identity: model.Identity{Kind: model.LiveAlias, Name: "empty_alias"}, Value: model.ScalarLiveValue("")},
			{Identity: model.Identity{Kind: model.LiveFunction, Name: "multi_fn"}, Value: model.ScalarLiveValue("print -r -- one\nprint -r -- two")},
			{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/base", "", "/dup", "/dup"})},
			{Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.ListLiveValue([]string{})},
			{Identity: model.Identity{Kind: model.LiveOption, Name: "AUTO_CD"}, Value: model.OptionLiveValue(false)},
		},
		Tombstones: []model.Identity{
			{Kind: model.LiveEnv, Name: "REMOVED"},
			{Kind: model.LiveAlias, Name: "old_alias"},
			{Kind: model.LiveFunction, Name: "old_fn"},
			{Kind: model.LiveOption, Name: "BEEP"},
		},
	})
}

func TestCommitWorktreeReadWorktreeRevisionExactRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newWorktreeStore(t, nil)
	base, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	document := exactWorktreeDocument()
	result, err := store.CommitWorktree(ctx, "main", base, document, "exact worktree")
	if err != nil || !result.Committed || result.Conflict || result.RecoveryRequired || !validGitObjectID(result.OID) {
		t.Fatalf("CommitWorktree = %#v, %v", result, err)
	}
	parent, err := store.git.revParse(ctx, result.OID+"^")
	if err != nil || parent != base {
		t.Fatalf("published parent = %q, %v; want %q", parent, err, base)
	}
	got, err := store.ReadWorktreeRevision(ctx, result.OID)
	if err != nil || !reflect.DeepEqual(got, document) {
		t.Fatalf("ReadWorktreeRevision = %#v, %v; want %#v", got, err, document)
	}
	wantZSH, err := (zsh.Provider{}).RegenerateWorktree(document)
	if err != nil {
		t.Fatal(err)
	}
	gotZSH, err := store.git.show(ctx, result.OID+":profile.zsh")
	if err != nil || !reflect.DeepEqual(gotZSH, wantZSH) {
		t.Fatalf("profile.zsh = %q, %v; want %q", gotZSH, err, wantZSH)
	}
}

func TestCreateFromUsesExactCurrentBaseAndNeverOverwrites(t *testing.T) {
	ctx := context.Background()
	store := newWorktreeStore(t, nil)
	initial, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitWorktree(ctx, "main", initial, exactWorktreeDocument(), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CommitWorktree(ctx, "main", first.OID, model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{}), "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateFrom(ctx, "work", first.OID); err != nil {
		t.Fatal(err)
	}
	tip, err := store.git.revParse(ctx, "refs/heads/work")
	if err != nil || tip != first.OID || tip == second.OID {
		t.Fatalf("work tip = %q, %v; want exact current base %q", tip, err, first.OID)
	}
	if err := store.CreateFrom(ctx, "work", second.OID); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("duplicate CreateFrom = %v, want ErrProfileExists", err)
	}
}

func TestCommitWorktreeExpectedBaseRaceIsVisibleConflict(t *testing.T) {
	ctx := context.Background()
	store := newWorktreeStore(t, nil)
	base, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	winner, err := store.CommitWorktree(ctx, "main", base, exactWorktreeDocument(), "winner")
	if err != nil {
		t.Fatal(err)
	}
	loser, err := store.CommitWorktree(ctx, "main", base, model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{}), "loser")
	if !errors.Is(err, ErrSecretRefConflict) || loser.Committed || !loser.Conflict || loser.RecoveryRequired {
		t.Fatalf("losing CommitWorktree = %#v, %v", loser, err)
	}
	tip, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil || tip != winner.OID {
		t.Fatalf("race changed winner: tip=%q err=%v winner=%q", tip, err, winner.OID)
	}
}

func TestCommitWorktreeSecretCanariesAreRedactedOrRejected(t *testing.T) {
	ctx := context.Background()
	keychain := &transactionKeychain{values: map[string]string{}}
	store := newWorktreeStore(t, keychain)
	base, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	const canary = "pinned-runtime-secret-canary"
	document := model.NewCommittedWorktree(buildSecretProfile(t, "export API_KEY="+canary+"\n"), model.LiveProjection{})
	result, err := store.CommitWorktree(ctx, "main", base, document, "redact secret")
	if err != nil || !result.Committed {
		t.Fatalf("secret CommitWorktree = %#v, %v", result, err)
	}
	for _, path := range []string{"profile.json", "profile.zsh"} {
		blob, err := store.git.show(ctx, result.OID+":"+path)
		if err != nil || strings.Contains(string(blob), canary) {
			t.Fatalf("%s retained secret canary, err=%v", path, err)
		}
	}
	if got, err := keychain.Retrieve("API_KEY"); err != nil || got != canary {
		t.Fatalf("backend secret = %q, %v", got, err)
	}

	base = result.OID
	pinned := model.NewCommittedWorktree(buildSecretProfile(t, "export TOKEN=second-secret-canary\n"), model.LiveProjection{
		States: []model.LiveIdentityState{{Identity: model.Identity{Kind: model.LiveEnv, Name: "TOKEN"}, Value: model.ScalarLiveValue("second-secret-canary")}},
	})
	rejected, err := store.CommitWorktree(ctx, "main", base, pinned, "reject pinned")
	if err == nil || rejected.Committed || len(keychain.stores) != 1 {
		t.Fatalf("pinned projection = %#v, %v; stores=%v", rejected, err, keychain.stores)
	}
}

func TestReadWorktreeRevisionRejectsBlobTreeTagAndSHAshapedInputs(t *testing.T) {
	ctx := context.Background()
	store := newWorktreeStore(t, nil)
	blob, err := store.git.hashObject(ctx, []byte("not a commit"))
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(t.TempDir(), "index")
	if _, err := store.git.runCommit(ctx, index, commitTS, "read-tree", "--empty"); err != nil {
		t.Fatal(err)
	}
	treeOut, err := store.git.runCommit(ctx, index, commitTS, "write-tree")
	if err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(string(treeOut))
	base, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	tagInput := fmt.Sprintf("object %s\ntype commit\ntag exact\ntagger zsh-pro <zsh-pro@local> 1700000000 +0000\n\nexact\n", base)
	cmd := exec.Command("git", "hash-object", "-t", "tag", "-w", "--stdin")
	cmd.Env = store.git.ownedEnvironment()
	cmd.Stdin = strings.NewReader(tagInput)
	tagOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for name, revision := range map[string]string{
		"blob":           blob,
		"tree":           tree,
		"tag":            strings.TrimSpace(string(tagOut)),
		"missing sha1":   strings.Repeat("f", 40),
		"missing sha256": strings.Repeat("e", 64),
		"not shaped":     "main",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.ReadWorktreeRevision(ctx, revision); err == nil {
				t.Fatal("non-commit exact revision was accepted")
			}
		})
	}
}

func TestLegacyStoreAdaptersRemainIsolatedFromExactWorktreeAuthority(t *testing.T) {
	ctx := context.Background()
	store := newWorktreeStore(t, nil)
	base, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSHPRO_PROFILE", "process-local-only")
	if err := store.CreateFrom(ctx, "work", base); err != nil {
		t.Fatal(err)
	}
	tip, err := store.git.revParse(ctx, "refs/heads/work")
	if err != nil || tip != base || store.Current() != "process-local-only" {
		t.Fatalf("legacy process input affected exact authority: tip=%q current=%q err=%v", tip, store.Current(), err)
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

func TestInstallInitializationRejectsReplacedOrSymlinkedRoot(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	for _, kind := range []string{"replacement", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			store, err := New(root, stubRegen{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			initialization, err := store.InitForInstall(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			displaced := root + "-original"
			if err := os.Rename(root, displaced); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "replacement":
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(displaced, root); err != nil {
					t.Fatal(err)
				}
			}

			gitStarts := 0
			store.git.beforeStart = func([]string) { gitStarts++ }
			outcome, err := store.BeginIngest(context.Background(), initialization.ID())
			if !errors.Is(err, ErrInvalidIngestAuthority) || !outcome.TransactionID.IsZero() || gitStarts != 0 {
				t.Fatalf("changed root Begin: token_zero=%t git_starts=%d err=%v",
					outcome.TransactionID.IsZero(), gitStarts, err)
			}
			if err := initialization.Finalize(); !errors.Is(err, ErrInvalidIngestAuthority) {
				t.Fatalf("changed root Finalize = %v, want invalid authority", err)
			}
			if err := initialization.Rollback(); !errors.Is(err, ErrInvalidIngestAuthority) {
				t.Fatalf("changed root Rollback = %v, want invalid authority", err)
			}
		})
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
		err := withStoreRootTransactionLock(context.Background(), root, func(guard *storeRootTransactionGuard) error {
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
	if err := withStoreRootTransactionLock(context.Background(), installRoot, func(*storeRootTransactionGuard) error { return nil }); err != nil {
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

func holdStoreRootTransactionLockForTest(t *testing.T, root string) func() {
	t.Helper()
	acquired := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withStoreRootTransactionLock(context.Background(), root, func(*storeRootTransactionGuard) error {
			close(acquired)
			<-release
			return nil
		})
	}()
	select {
	case <-acquired:
	case err := <-done:
		t.Fatalf("lock holder exited before acquisition: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for lock holder")
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			close(release)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("release lock holder: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Error("timed out releasing lock holder")
			}
		})
	}
}

func TestCommitIngestLockContentionHonorsContextCancellation(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	release := holdStoreRootTransactionLockForTest(t, root)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := store.CommitIngest(
		ctx,
		initialization.ID(),
		transaction.TransactionID,
		phase6Profile("CANCELED", "one"),
		"context cancellation",
	)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrIngestRecoveryRequired) {
		t.Fatalf("CommitIngest contention error = %v, want cancellation plus recovery classification", err)
	}
	if outcome.Status != model.IngestCommitRecoveryRequired ||
		outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
		t.Fatalf("CommitIngest cancellation facts: status=%s cleanup=%s recovery=%t",
			outcome.Status, outcome.Cleanup, outcome.RecoveryRequired)
	}
	if _, statErr := os.Stat(store.transactionQuarantinePath(transaction.TransactionID)); statErr != nil {
		t.Fatalf("CommitIngest cancellation lost retained quarantine: %v", statErr)
	}
}

func TestAbortIngestLockContentionHonorsContextDeadline(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	release := holdStoreRootTransactionLockForTest(t, root)
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	outcome, err := store.AbortIngest(ctx, initialization.ID(), transaction.TransactionID)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrIngestRecoveryRequired) {
		t.Fatalf("AbortIngest contention error = %v, want deadline plus recovery classification", err)
	}
	if outcome.Lifecycle != model.IngestLifecycleTerminal ||
		outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
		t.Fatalf("AbortIngest deadline facts: lifecycle=%s cleanup=%s recovery=%t",
			outcome.Lifecycle, outcome.Cleanup, outcome.RecoveryRequired)
	}
	if _, statErr := os.Stat(store.transactionQuarantinePath(transaction.TransactionID)); statErr != nil {
		t.Fatalf("AbortIngest deadline lost retained quarantine: %v", statErr)
	}

	release()
	replayed, replayErr := store.AbortIngest(context.Background(), initialization.ID(), transaction.TransactionID)
	if !errors.Is(replayErr, context.DeadlineExceeded) || !errors.Is(replayErr, ErrIngestRecoveryRequired) ||
		!reflect.DeepEqual(replayed, outcome) {
		t.Fatalf("AbortIngest deadline replay: same_outcome=%t error=%v",
			reflect.DeepEqual(replayed, outcome), replayErr)
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
				if err := os.Chmod(namespace, 0o755); err != nil {
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
				if err := os.Chmod(filepath.Join(namespace, "lock"), 0o644); err != nil {
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
			if err := withStoreRootTransactionLock(context.Background(), root, func(*storeRootTransactionGuard) error {
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
	err := withStoreRootTransactionLock(context.Background(), root, func(guard *storeRootTransactionGuard) error {
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

func newPhase6IngestStore(t *testing.T) (*Store, InstallInitialization, string) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional ingest is unsupported on this platform")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping ingest transaction tests")
	}
	root := filepath.Join(t.TempDir(), "store")
	store, err := New(root, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return store, initialization, root
}

func beginPhase6Ingest(t *testing.T, store *Store, initialization InstallInitialization) model.IngestBeginOutcome {
	t.Helper()
	outcome, err := store.BeginIngest(context.Background(), initialization.ID())
	if err != nil {
		t.Fatalf("BeginIngest: %v (%#v)", err, outcome)
	}
	if outcome.Lifecycle != model.IngestLifecycleActive || outcome.TransactionID.IsZero() {
		t.Fatalf("BeginIngest outcome = %#v, want active nonzero token", outcome)
	}
	return outcome
}

func abortPhase6Ingest(t *testing.T, store *Store, initialization InstallInitialization, token model.IngestTransactionID) model.IngestAbortOutcome {
	t.Helper()
	outcome, err := store.AbortIngest(context.Background(), initialization.ID(), token)
	if err != nil {
		t.Fatalf("AbortIngest: %v (%#v)", err, outcome)
	}
	return outcome
}

func TestBeginIngestTargetsMainWithoutBranchInput(t *testing.T) {
	method := reflect.TypeOf((*Store).BeginIngest)
	if method.NumIn() != 3 {
		t.Fatalf("BeginIngest accepts %d inputs including receiver, want receiver+context+initialization ID", method.NumIn())
	}
	store, initialization, _ := newPhase6IngestStore(t)
	outcome := beginPhase6Ingest(t, store, initialization)
	mainTip, err := store.git.revParse(context.Background(), "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Baseline.ExpectedRevision == nil || *outcome.Baseline.ExpectedRevision != mainTip {
		t.Fatalf("baseline revision = %v, want main %s", outcome.Baseline.ExpectedRevision, mainTip)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestSharedBeginStateMachineRegistersOnlyBranchSpecificRefs(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	ctx := context.Background()
	main := beginPhase6Ingest(t, store, initialization)
	authority, err := store.legacyAuthority(ctx)
	if err != nil {
		t.Fatal(err)
	}
	featureRef, err := validatedHeadRefForBranch("feature/shared-begin")
	if err != nil {
		t.Fatal(err)
	}
	feature, err := store.beginTransactionForRef(ctx, featureRef, authority)
	if err != nil {
		t.Fatal(err)
	}

	store.transactionMu.Lock()
	_, mainRegistered := store.ingestTransactionRefs[main.TransactionID]
	registeredFeature, featureRegistered := store.ingestTransactionRefs[feature.TransactionID]
	store.transactionMu.Unlock()
	if mainRegistered || !featureRegistered || registeredFeature != featureRef {
		t.Fatalf("ref registration: main=%t feature=%t exact_feature=%t",
			mainRegistered, featureRegistered, registeredFeature == featureRef)
	}
	abortPhase6Ingest(t, store, initialization, main.TransactionID)
	if outcome, err := store.AbortIngest(ctx, authority.id, feature.TransactionID); err != nil ||
		outcome.Cleanup != model.QuarantineCleanupRemoved {
		t.Fatalf("legacy feature abort: cleanup=%s err=%v", outcome.Cleanup, err)
	}
}

func TestSharedBeginStateMachinePreservesMainProbeFailureEvidence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	authority, err := store.legacyAuthority(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store.beginObserveMain = func(context.Context) (string, bool, error) {
		return "", false, ErrGitCommand
	}
	outcome, err := store.beginTransactionForRef(ctx, mainHeadRef(), authority)
	if !errors.Is(err, ErrGitCommand) || outcome.Lifecycle != model.IngestLifecycleTerminal ||
		outcome.FailureCode != model.IngestFailureBaselineRead ||
		outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired ||
		!outcome.InitializerRollbackSafe {
		t.Fatalf("shared main probe failure: lifecycle=%s failure=%s cleanup=%s recovery=%t rollback_safe=%t err=%v",
			outcome.Lifecycle, outcome.FailureCode, outcome.Cleanup, outcome.RecoveryRequired,
			outcome.InitializerRollbackSafe, err)
	}
}

func TestBeginIngestPresentRefCarriesExpectedRevision(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	want, err := store.git.revParse(context.Background(), "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	outcome := beginPhase6Ingest(t, store, initialization)
	if !outcome.Baseline.RefPresent || outcome.Baseline.ExpectedRevision == nil || *outcome.Baseline.ExpectedRevision != want {
		t.Fatalf("present baseline = %#v, want exact revision %s", outcome.Baseline, want)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestBeginIngestAbsentRefUsesNilExpectedAndZeroRevisionReads(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	ctx := context.Background()
	tip, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.git.deleteRefCAS(ctx, "refs/heads/main", tip); err != nil {
		t.Fatal(err)
	}
	revisionReads := 0
	store.git.beforeStart = func(args []string) {
		if len(args) > 0 && (args[0] == "ls-tree" || args[0] == "cat-file" || args[0] == "show") {
			revisionReads++
		}
	}
	outcome := beginPhase6Ingest(t, store, initialization)
	if outcome.Baseline.RefPresent || outcome.Baseline.ExpectedRevision != nil || outcome.Baseline.ProfileObjectPresent {
		t.Fatalf("absent baseline = %#v", outcome.Baseline)
	}
	if revisionReads != 0 {
		t.Fatalf("absent ref performed %d exact-revision/object reads", revisionReads)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestBeginIngestExactMainObservationIgnoresPrefixRefs(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	ctx := context.Background()
	tip, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.git.deleteRefCAS(ctx, "refs/heads/main", tip); err != nil {
		t.Fatal(err)
	}
	if err := store.git.updateRef(ctx, "refs/heads/main/nested", tip); err != nil {
		t.Fatal(err)
	}
	outcome := beginPhase6Ingest(t, store, initialization)
	if outcome.Baseline.RefPresent || outcome.Baseline.ExpectedRevision != nil {
		t.Fatalf("main prefix ref was misclassified as exact main: %#v", outcome.Baseline)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestBeginIngestInitOnlyBaseline(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	outcome := beginPhase6Ingest(t, store, initialization)
	if !outcome.Baseline.RefPresent || outcome.Baseline.ProfileObjectPresent || len(outcome.Baseline.Profile.Entries) != 0 {
		t.Fatalf("Init-only baseline = %#v", outcome.Baseline)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestBeginIngestCommittedEmptyBaseline(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional ingest is unsupported on this platform")
	}
	root := filepath.Join(t.TempDir(), "store")
	store, err := New(root, stubRegen{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit(ctx, "main", model.Profile{}, "empty"); err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	outcome := beginPhase6Ingest(t, store, initialization)
	if !outcome.Baseline.RefPresent || !outcome.Baseline.ProfileObjectPresent || len(outcome.Baseline.Profile.Entries) != 0 {
		t.Fatalf("committed-empty baseline = %#v", outcome.Baseline)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestBeginIngestPresentProbeFailureIsNotAbsence(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	store.beginObserveMain = func(context.Context) (string, bool, error) { return "", false, ErrGitCommand }
	outcome, err := store.BeginIngest(context.Background(), initialization.ID())
	if !errors.Is(err, ErrGitCommand) {
		t.Fatalf("BeginIngest = (%#v, %v), want probe failure", outcome, err)
	}
	if outcome.TransactionID.IsZero() || outcome.Lifecycle != model.IngestLifecycleTerminal ||
		outcome.FailureCode != model.IngestFailureBaselineRead || outcome.Baseline.RefPresent {
		t.Fatalf("probe failure was misclassified as absence: %#v", outcome)
	}
}

func TestBeginIngestReservesTokenBeforeGitOrFilesystemIO(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	reached := make(chan struct{})
	release := make(chan struct{})
	store.beginAfterReserve = func() error {
		close(reached)
		<-release
		return nil
	}
	gitStarts := 0
	store.git.beforeStart = func([]string) { gitStarts++ }
	type result struct {
		outcome model.IngestBeginOutcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		outcome, err := store.BeginIngest(context.Background(), initialization.ID())
		done <- result{outcome: outcome, err: err}
	}()
	<-reached
	if got := store.installInitializationTransactionCount(initialization.ID()); got != 1 {
		t.Fatalf("provisional token count = %d, want 1", got)
	}
	if err := initialization.Rollback(); !errors.Is(err, ErrInitializerRollbackUnsafe) {
		t.Fatalf("rollback during provisional Begin = %v, want unsafe", err)
	}
	if gitStarts != 0 {
		t.Fatalf("provisional reservation started %d Git commands", gitStarts)
	}
	namespace, err := storeTransactionNamespacePath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(namespace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provisional reservation created namespace: %v", err)
	}
	close(release)
	resultValue := <-done
	if resultValue.err != nil {
		t.Fatalf("Begin after release: %v", resultValue.err)
	}
	abortPhase6Ingest(t, store, initialization, resultValue.outcome.TransactionID)
	if err := initialization.Rollback(); err != nil {
		t.Fatalf("rollback after terminal cleanup: %v", err)
	}
}

func TestBeginIngestProbeFailureRetainsTerminalEvidence(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	store.beginObserveMain = func(context.Context) (string, bool, error) { return "", false, ErrGitCommand }
	outcome, err := store.BeginIngest(context.Background(), initialization.ID())
	if !errors.Is(err, ErrGitCommand) {
		t.Fatal(err)
	}
	if outcome.TransactionID.IsZero() || store.installInitializationTransactionCount(initialization.ID()) != 1 {
		t.Fatal("failed probe lost its reserved token")
	}
	if outcome.Lifecycle != model.IngestLifecycleTerminal || outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("failed probe terminal evidence = %#v", outcome)
	}
}

func TestBeginIngestUncertainSetupRetainsLocatorAndTerminalEvidence(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	store.beginAfterQuarantineCreate = func(string) error { return errors.New("injected setup failure") }
	store.cleanupBeforeFinalCheck = replacementTopLevelQuarantineSeam(t, false)
	outcome, err := store.BeginIngest(context.Background(), initialization.ID())
	if err == nil {
		t.Fatal("injected setup failure returned nil")
	}
	path := store.transactionQuarantinePath(outcome.TransactionID)
	if path == "" {
		t.Fatal("uncertain setup lost its authenticated locator")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("retained quarantine missing: %v", err)
	}
	if outcome.Lifecycle != model.IngestLifecycleTerminal || outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
		t.Fatalf("uncertain setup evidence = %#v", outcome)
	}
}

func environmentByName(environment []string) map[string]string {
	result := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			result[name] = value
		}
	}
	return result
}

func TestGitRunnerUsesCanonicalBareEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	for name, value := range map[string]string{
		"GIT_DIR":                          filepath.Join(t.TempDir(), "hostile-git"),
		"GIT_WORK_TREE":                    filepath.Join(t.TempDir(), "hostile-worktree"),
		"GIT_CONFIG_NOSYSTEM":              "0",
		"GIT_CONFIG_GLOBAL":                filepath.Join(t.TempDir(), "config"),
		"GIT_INDEX_FILE":                   filepath.Join(t.TempDir(), "index"),
		"GIT_OBJECT_DIRECTORY":             filepath.Join(t.TempDir(), "objects"),
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": filepath.Join(t.TempDir(), "alternate"),
	} {
		t.Setenv(name, value)
	}
	runner, err := newGitRunner(root)
	if err != nil {
		t.Fatal(err)
	}
	environment := environmentByName(runner.ownedEnvironment())
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if environment["GIT_DIR"] != want || environment["GIT_CONFIG_NOSYSTEM"] != "1" || environment["GIT_CONFIG_GLOBAL"] != os.DevNull {
		t.Fatalf(
			"owned bare environment: GIT_DIR=%q GIT_CONFIG_NOSYSTEM=%q GIT_CONFIG_GLOBAL=%q",
			environment["GIT_DIR"],
			environment["GIT_CONFIG_NOSYSTEM"],
			environment["GIT_CONFIG_GLOBAL"],
		)
	}
	for _, name := range []string{"GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"} {
		if _, ok := environment[name]; ok {
			t.Fatalf("owned base environment retained %s", name)
		}
	}
}

func TestBeginIngestHostileGitEnvironmentCannotWriteWorktree(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	decoy := t.TempDir()
	sentinel := filepath.Join(decoy, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "hostile.git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(decoy, "index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(decoy, "objects"))
	outcome := beginPhase6Ingest(t, store, initialization)
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unchanged" {
		t.Fatalf("hostile worktree changed: %q, %v", got, err)
	}
	abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
}

func TestGitRunnerRejectsWorktreeMutatingArgv(t *testing.T) {
	rejected := [][]string{
		{"checkout", "main"}, {"reset", "--hard"}, {"switch", "main"},
		{"restore", "."}, {"clean", "-fd"}, {"read-tree", "-u", "main"},
		{"--work-tree=/tmp/decoy", "status"}, {"--git-dir=/tmp/other", "rev-parse", "HEAD"},
		{"-C", "/tmp", "status"}, {"-c", "core.worktree=/tmp", "status"},
	}
	for _, args := range rejected {
		if err := validateGitArgv(args); err == nil {
			t.Errorf("validateGitArgv(%q) accepted a worktree/global override", args)
		}
	}
	if err := validateGitArgv([]string{"read-tree", "--empty"}); err != nil {
		t.Fatalf("safe read-tree rejected: %v", err)
	}
}

func TestBeginIngestConcurrentIndexesIsolated(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	var wg sync.WaitGroup
	results := make(chan model.IngestBeginOutcome, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := store.BeginIngest(context.Background(), initialization.ID())
			results <- outcome
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var outcomes []model.IngestBeginOutcome
	for outcome := range results {
		outcomes = append(outcomes, outcome)
	}
	if len(outcomes) != 2 {
		t.Fatalf("Begin outcomes = %d, want 2", len(outcomes))
	}
	pathA := store.transactionQuarantinePath(outcomes[0].TransactionID)
	pathB := store.transactionQuarantinePath(outcomes[1].TransactionID)
	if pathA == "" || pathB == "" || pathA == pathB {
		t.Fatalf("quarantine paths are not isolated: %q, %q", pathA, pathB)
	}
	for _, outcome := range outcomes {
		abortPhase6Ingest(t, store, initialization, outcome.TransactionID)
	}
}

func TestBeginIngestRecordStatePublishesUnderTransactionLock(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	reserved := make(chan model.IngestTransactionID, 1)
	release := make(chan struct{})
	store.beginAfterReserve = func() error {
		store.transactionMu.Lock()
		initializationRecord := store.installInitializations[initialization.ID()]
		var transactionID model.IngestTransactionID
		if initializationRecord != nil {
			for candidate := range initializationRecord.transactions {
				transactionID = candidate
				break
			}
		}
		store.transactionMu.Unlock()
		if transactionID.IsZero() {
			return errors.New("reserved transaction was not registered")
		}
		reserved <- transactionID
		<-release
		return nil
	}

	type beginResult struct {
		outcome model.IngestBeginOutcome
		err     error
	}
	done := make(chan beginResult, 1)
	go func() {
		outcome, err := store.BeginIngest(context.Background(), initialization.ID())
		done <- beginResult{outcome: outcome, err: err}
	}()
	transactionID := <-reserved

	readerStarted := make(chan struct{})
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		first := true
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			store.transactionMu.Lock()
			record := store.ingestTransactions[transactionID]
			if record != nil {
				_ = record.baseline.RefPresent
				if record.quarantine != nil {
					_ = record.quarantine.path
				}
			}
			store.transactionMu.Unlock()
			if first {
				close(readerStarted)
				first = false
			}
			runtime.Gosched()
		}
	}()
	<-readerStarted
	close(release)
	result := <-done
	close(stopReader)
	<-readerDone
	if result.err != nil || result.outcome.Lifecycle != model.IngestLifecycleActive {
		t.Fatalf("BeginIngest with concurrent record reader: lifecycle=%s err=%v",
			result.outcome.Lifecycle, result.err)
	}
	abortPhase6Ingest(t, store, initialization, result.outcome.TransactionID)
}

func testBeginIngestSetupUnwind(t *testing.T, stage string) {
	t.Helper()
	store, initialization, _ := newPhase6IngestStore(t)
	injected := errors.New("injected setup failure")
	switch stage {
	case "create":
		store.beginAfterQuarantineCreate = func(string) error { return injected }
	case "open":
		store.beginAfterQuarantineOpen = func(string) error { return injected }
	case "stat":
		store.beginAfterQuarantineStat = func(string) error { return injected }
	default:
		t.Fatalf("unknown setup stage %q", stage)
	}
	outcome, err := store.BeginIngest(context.Background(), initialization.ID())
	if !errors.Is(err, injected) {
		t.Fatalf("BeginIngest = (%#v, %v), want injected failure", outcome, err)
	}
	if outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired || outcome.Lifecycle != model.IngestLifecycleTerminal {
		t.Fatalf("setup unwind evidence = %#v", outcome)
	}
	if path := store.transactionQuarantinePath(outcome.TransactionID); path == "" {
		t.Fatal("setup unwind lost recorded locator")
	} else if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("setup unwind retained %s: %v", path, err)
	}
}

func TestBeginIngestSetupUnwindAfterCreate(t *testing.T) {
	testBeginIngestSetupUnwind(t, "create")
}

func TestBeginIngestSetupUnwindAfterOpen(t *testing.T) {
	testBeginIngestSetupUnwind(t, "open")
}

func TestBeginIngestSetupUnwindAfterStat(t *testing.T) {
	testBeginIngestSetupUnwind(t, "stat")
}

func waitForPhase6File(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func TestQuarantineCleanupCrossProcessLockSerializesCooperatingStores(t *testing.T) {
	if os.Getenv("ZSHPRO_QUARANTINE_LOCK_HELPER") == "1" {
		root := os.Getenv("ZSHPRO_QUARANTINE_LOCK_ROOT")
		ready := os.Getenv("ZSHPRO_QUARANTINE_LOCK_READY")
		release := os.Getenv("ZSHPRO_QUARANTINE_LOCK_RELEASE")
		err := withStoreRootTransactionLock(context.Background(), root, func(*storeRootTransactionGuard) error {
			if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
				return err
			}
			return waitForPhase6File(release, 5*time.Second)
		})
		if err != nil {
			os.Exit(13)
		}
		return
	}
	store, initialization, root := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	base := t.TempDir()
	ready := filepath.Join(base, "ready")
	release := filepath.Join(base, "release")
	helper := exec.Command(os.Args[0], "-test.run=^TestQuarantineCleanupCrossProcessLockSerializesCooperatingStores$")
	helper.Env = append(os.Environ(),
		"ZSHPRO_QUARANTINE_LOCK_HELPER=1",
		"ZSHPRO_QUARANTINE_LOCK_ROOT="+root,
		"ZSHPRO_QUARANTINE_LOCK_READY="+ready,
		"ZSHPRO_QUARANTINE_LOCK_RELEASE="+release,
	)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	if err := waitForPhase6File(ready, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	type result struct {
		outcome model.IngestAbortOutcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		done <- result{outcome: outcome, err: err}
	}()
	select {
	case result := <-done:
		t.Fatalf("cleanup bypassed held cross-process lock: %#v, %v", result.outcome, result.err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err != nil {
		t.Fatal(err)
	}
	resultValue := <-done
	if resultValue.err != nil || resultValue.outcome.Cleanup != model.QuarantineCleanupRemoved {
		t.Fatalf("cleanup after lock release = (%#v, %v)", resultValue.outcome, resultValue.err)
	}
}

func TestAbortIngestLockUnavailableOrInvalidatedRetainsRecovery(t *testing.T) {
	t.Run("unsafe lock entry", func(t *testing.T) {
		store, initialization, root := newPhase6IngestStore(t)
		begin := beginPhase6Ingest(t, store, initialization)
		namespace, err := storeTransactionNamespacePath(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(namespace, "lock"), 0o644); err != nil {
			t.Fatal(err)
		}
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		if !errors.Is(err, ErrIngestRecoveryRequired) {
			t.Fatalf("unsafe-lock AbortIngest = %v, want recovery required", err)
		}
		if outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
			t.Fatalf("unsafe-lock cleanup = %#v", outcome)
		}
		if _, err := os.Stat(store.transactionQuarantinePath(begin.TransactionID)); err != nil {
			t.Fatalf("unsafe-lock cleanup mutated quarantine: %v", err)
		}
	})

	t.Run("discarded descriptor", func(t *testing.T) {
		store, initialization, _ := newPhase6IngestStore(t)
		begin := beginPhase6Ingest(t, store, initialization)
		store.cleanupDiscardLock = true
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		if !errors.Is(err, ErrIngestRecoveryRequired) {
			t.Fatalf("discarded-lock AbortIngest = %v, want recovery required", err)
		}
		if outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
			t.Fatalf("discarded-lock cleanup = %#v", outcome)
		}
		if _, err := os.Stat(store.transactionQuarantinePath(begin.TransactionID)); err != nil {
			t.Fatalf("discarded-lock cleanup mutated quarantine: %v", err)
		}
	})
}

func replacementEntrySeam(t *testing.T, relative, kind string, after bool) func(quarantineCleanupSeam) error {
	t.Helper()
	triggered := false
	return func(seam quarantineCleanupSeam) error {
		if triggered || seam.Relative != relative {
			return nil
		}
		triggered = true
		if err := seam.Parent.Remove(seam.Name); err != nil {
			return err
		}
		switch kind {
		case "file":
			return seam.Parent.WriteFile(seam.Name, []byte("replacement"), 0o600)
		case "symlink":
			return seam.Parent.Symlink("replacement-target", seam.Name)
		case "directory":
			if err := seam.Parent.Mkdir(seam.Name, 0o700); err != nil {
				return err
			}
			replacement, err := seam.Parent.OpenRoot(seam.Name)
			if err != nil {
				return err
			}
			defer func() { _ = replacement.Close() }()
			return replacement.WriteFile("preserve", []byte("replacement"), 0o600)
		default:
			return fmt.Errorf("unknown replacement kind %s after=%v", kind, after)
		}
	}
}

func testAbortIngestRetainsReplacedChild(t *testing.T, kind string, after bool) {
	t.Helper()
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	quarantine := store.transactionQuarantinePath(begin.TransactionID)
	relative := "victim"
	switch kind {
	case "file":
		if err := os.WriteFile(filepath.Join(quarantine, relative), []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	case "symlink":
		if err := os.Symlink("original-target", filepath.Join(quarantine, relative)); err != nil {
			t.Fatal(err)
		}
	case "directory":
		if err := os.Mkdir(filepath.Join(quarantine, relative), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(quarantine, relative, "child"), []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seam := replacementEntrySeam(t, relative, kind, after)
	if after {
		store.cleanupAfterFinalCheck = seam
	} else {
		store.cleanupBeforeFinalCheck = seam
	}
	outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
	if !errors.Is(err, ErrIngestRecoveryRequired) {
		t.Fatalf("replacement AbortIngest = %v, want recovery required", err)
	}
	if outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
		t.Fatalf("replacement cleanup = %#v", outcome)
	}
	switch kind {
	case "file":
		if got, err := os.ReadFile(filepath.Join(quarantine, relative)); err != nil || string(got) != "replacement" {
			t.Fatalf("replacement file = %q, %v", got, err)
		}
	case "symlink":
		if got, err := os.Readlink(filepath.Join(quarantine, relative)); err != nil || got != "replacement-target" {
			t.Fatalf("replacement symlink = %q, %v", got, err)
		}
	case "directory":
		if got, err := os.ReadFile(filepath.Join(quarantine, relative, "preserve")); err != nil || string(got) != "replacement" {
			t.Fatalf("replacement directory = %q, %v", got, err)
		}
	}
}

func TestAbortIngestRetainsReplacedFileChildBeforeFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "file", false)
}

func TestAbortIngestRetainsReplacedFileChildAfterFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "file", true)
}

func TestAbortIngestRetainsReplacedSymlinkChildBeforeFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "symlink", false)
}

func TestAbortIngestRetainsReplacedSymlinkChildAfterFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "symlink", true)
}

func TestAbortIngestRetainsReplacedNestedDirectoryChildBeforeFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "directory", false)
}

func TestAbortIngestRetainsReplacedNestedDirectoryChildAfterFinalCheck(t *testing.T) {
	testAbortIngestRetainsReplacedChild(t, "directory", true)
}

func TestAbortIngestTerminalResultReplay(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	first := abortPhase6Ingest(t, store, initialization, begin.TransactionID)
	cleanupCalls := 0
	store.cleanupBeforeFinalCheck = func(quarantineCleanupSeam) error {
		cleanupCalls++
		return nil
	}
	second := abortPhase6Ingest(t, store, initialization, begin.TransactionID)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("terminal replay changed: first=%#v second=%#v", first, second)
	}
	if cleanupCalls != 0 {
		t.Fatalf("terminal replay made %d cleanup calls", cleanupCalls)
	}
}

func TestAbortIngestReplaysEveryTerminalTransaction(t *testing.T) {
	t.Run("begin failure", func(t *testing.T) {
		store, initialization, _ := newPhase6IngestStore(t)
		store.beginObserveMain = func(context.Context) (string, bool, error) {
			return "", false, ErrGitCommand
		}
		begin, err := store.BeginIngest(context.Background(), initialization.ID())
		if !errors.Is(err, ErrGitCommand) || begin.Lifecycle != model.IngestLifecycleTerminal {
			t.Fatalf("terminal BeginIngest = (%#v, %v)", begin, err)
		}
		cleanupCalls := 0
		store.cleanupBeforeFinalCheck = func(quarantineCleanupSeam) error {
			cleanupCalls++
			return nil
		}
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		if err != nil || outcome.Lifecycle != model.IngestLifecycleTerminal ||
			outcome.Cleanup != begin.Cleanup || outcome.FailureCode != begin.FailureCode ||
			outcome.RecoveryRequired != begin.RecoveryRequired || cleanupCalls != 0 {
			t.Fatalf("terminal begin Abort: outcome=%#v err=%v cleanup_calls=%d", outcome, err, cleanupCalls)
		}
	})

	t.Run("committed", func(t *testing.T) {
		store, initialization, _ := newPhase6IngestStore(t)
		begin := beginPhase6Ingest(t, store, initialization)
		committed, err := commitPhase6Ingest(t, store, initialization, begin, phase6Profile("TERMINAL", "committed"))
		if err != nil || committed.Status != model.IngestCommitCommitted {
			t.Fatalf("CommitIngest = (%#v, %v)", committed, err)
		}
		cleanupCalls := 0
		store.cleanupBeforeFinalCheck = func(quarantineCleanupSeam) error {
			cleanupCalls++
			return nil
		}
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		if err != nil || outcome.Lifecycle != model.IngestLifecycleTerminal ||
			outcome.Cleanup != committed.Cleanup || outcome.FailureCode != committed.FailureCode ||
			outcome.RecoveryRequired != committed.RecoveryRequired || cleanupCalls != 0 {
			t.Fatalf("terminal commit Abort: outcome=%#v err=%v cleanup_calls=%d", outcome, err, cleanupCalls)
		}
	})

	t.Run("recovery required", func(t *testing.T) {
		store, initialization, _ := newPhase6IngestStore(t)
		store.beginAfterQuarantineCreate = func(string) error {
			return errors.New("injected setup failure")
		}
		store.cleanupBeforeFinalCheck = replacementTopLevelQuarantineSeam(t, false)
		begin, err := store.BeginIngest(context.Background(), initialization.ID())
		if err == nil || begin.Lifecycle != model.IngestLifecycleTerminal || !begin.RecoveryRequired {
			t.Fatalf("terminal recovery BeginIngest = (%#v, %v)", begin, err)
		}
		cleanupCalls := 0
		store.cleanupBeforeFinalCheck = func(quarantineCleanupSeam) error {
			cleanupCalls++
			return nil
		}
		outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
		if !errors.Is(err, ErrIngestRecoveryRequired) ||
			outcome.Lifecycle != model.IngestLifecycleTerminal || !outcome.RecoveryRequired ||
			outcome.Cleanup != begin.Cleanup || outcome.FailureCode != begin.FailureCode || cleanupCalls != 0 {
			t.Fatalf("terminal recovery Abort: outcome=%#v err=%v cleanup_calls=%d", outcome, err, cleanupCalls)
		}
	})
}

func TestAbortIngestRejectsUnknownCrossStoreAndFinalizing(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	unknown, err := model.NewIngestTransactionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AbortIngest(context.Background(), initialization.ID(), unknown); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("unknown Abort = %v", err)
	}
	other, otherInitialization, _ := newPhase6IngestStore(t)
	if _, err := other.AbortIngest(context.Background(), otherInitialization.ID(), begin.TransactionID); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("cross-Store Abort = %v", err)
	}
	store.transactionMu.Lock()
	store.ingestTransactions[begin.TransactionID].lifecycle = model.IngestLifecycleFinalizing
	store.transactionMu.Unlock()
	if _, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("finalizing Abort = %v", err)
	}
	store.transactionMu.Lock()
	store.ingestTransactions[begin.TransactionID].lifecycle = model.IngestLifecycleActive
	store.transactionMu.Unlock()
	abortPhase6Ingest(t, store, initialization, begin.TransactionID)
}

func replacementTopLevelQuarantineSeam(t *testing.T, after bool) func(quarantineCleanupSeam) error {
	t.Helper()
	triggered := false
	return func(seam quarantineCleanupSeam) error {
		if triggered || seam.Relative != "." {
			return nil
		}
		triggered = true
		preserved := seam.Name + "-preserved"
		if err := seam.Parent.Rename(seam.Name, preserved); err != nil {
			return err
		}
		if err := seam.Parent.Mkdir(seam.Name, 0o700); err != nil {
			return err
		}
		replacement, err := seam.Parent.OpenRoot(seam.Name)
		if err != nil {
			return err
		}
		defer func() { _ = replacement.Close() }()
		if err := replacement.WriteFile("replacement", []byte("preserve"), 0o600); err != nil {
			return err
		}
		_ = after
		return nil
	}
}

func testAbortIngestRetainsTopReplacement(t *testing.T, after bool) {
	t.Helper()
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	quarantine := store.transactionQuarantinePath(begin.TransactionID)
	seam := replacementTopLevelQuarantineSeam(t, after)
	if after {
		store.cleanupAfterFinalCheck = seam
	} else {
		store.cleanupBeforeFinalCheck = seam
	}
	outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
	if !errors.Is(err, ErrIngestRecoveryRequired) {
		t.Fatalf("top replacement AbortIngest = %v, want recovery required", err)
	}
	if outcome.Cleanup != model.QuarantineCleanupRetained || !outcome.RecoveryRequired {
		t.Fatalf("top replacement cleanup = %#v", outcome)
	}
	if got, err := os.ReadFile(filepath.Join(quarantine, "replacement")); err != nil || string(got) != "preserve" {
		t.Fatalf("top replacement = %q, %v", got, err)
	}
	if _, err := os.Stat(quarantine + "-preserved"); err != nil {
		t.Fatalf("displaced original quarantine was lost: %v", err)
	}
}

func TestAbortIngestRejectsQuarantineReplacementBeforeCleanup(t *testing.T) {
	testAbortIngestRetainsTopReplacement(t, false)
}

func TestAbortIngestRejectsSubstitutionAfterFinalCheck(t *testing.T) {
	testAbortIngestRetainsTopReplacement(t, true)
}

type phase6IngestCommitter interface {
	CommitIngest(
		context.Context,
		model.InstallInitializationID,
		model.IngestTransactionID,
		model.Profile,
		string,
	) (model.IngestCommitOutcome, error)
}

func commitPhase6Ingest(
	t *testing.T,
	store *Store,
	initialization InstallInitialization,
	transaction model.IngestBeginOutcome,
	profile model.Profile,
) (model.IngestCommitOutcome, error) {
	t.Helper()
	committer, ok := any(store).(phase6IngestCommitter)
	if !ok {
		t.Fatal("Store does not implement the typed CommitIngest contract")
	}
	return committer.CommitIngest(
		context.Background(),
		initialization.ID(),
		transaction.TransactionID,
		profile,
		"zsh-pro: ingest baseline",
	)
}

func phase6Profile(name, value string) model.Profile {
	return model.Profile{Entries: []model.Entry{{
		Text:      "export " + name + "=" + value,
		StartLine: 1,
		Category:  model.CatEnvironment,
		Kind:      model.KindAssignment,
		Names:     []string{name},
		Value:     value,
		Exported:  true,
		Managed:   true,
		ValueMode: model.ValueModeLegacy,
	}}}
}

func capturedUpdateRefTransactions(argv [][]string) [][]string {
	var transactions [][]string
	for _, args := range argv {
		if len(args) > 0 && args[0] == "update-ref" {
			transactions = append(transactions, args)
		}
	}
	return transactions
}

func TestCommitIngestRefAbsentUsesCreateWire(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	ctx := context.Background()
	tip, err := store.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.git.deleteRefCAS(ctx, "refs/heads/main", tip); err != nil {
		t.Fatal(err)
	}
	transaction := beginPhase6Ingest(t, store, initialization)
	var argv [][]string
	store.git.beforeStart = func(args []string) { argv = append(argv, append([]string(nil), args...)) }
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("ABSENT", "one"))
	if err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("CommitIngest absent ref = (%#v, %v)", outcome, err)
	}
	transactions := capturedUpdateRefTransactions(argv)
	if !reflect.DeepEqual(transactions, [][]string{{"update-ref", "--no-deref", "--stdin"}}) {
		t.Fatalf("update-ref argv = %#v", transactions)
	}
}

func TestCommitIngestRefPresentUsesExpectedUpdateWire(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	var argv [][]string
	store.git.beforeStart = func(args []string) { argv = append(argv, append([]string(nil), args...)) }
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("PRESENT", "one"))
	if err != nil || outcome.Status != model.IngestCommitCommitted || outcome.RefState != model.IngestRefCandidate {
		t.Fatalf("CommitIngest present ref = (%#v, %v)", outcome, err)
	}
	transactions := capturedUpdateRefTransactions(argv)
	if !reflect.DeepEqual(transactions, [][]string{{"update-ref", "--no-deref", "--stdin"}}) {
		t.Fatalf("update-ref argv = %#v", transactions)
	}
}

func TestCommitIngestRejectsCallerSelectedRefVerbBeforeProcess(t *testing.T) {
	method, ok := reflect.TypeOf((*Store)(nil)).MethodByName("CommitIngest")
	if !ok {
		t.Fatal("CommitIngest is missing")
	}
	if method.Type.NumIn() != 6 {
		t.Fatalf("CommitIngest accepts %d inputs including receiver; caller-selectable ref/verb authority is forbidden", method.Type.NumIn())
	}
}

func TestUpdateRefSessionLocksOnlyAfterPrepareOK(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, store, initialization)
	second := beginPhase6Ingest(t, store, initialization)
	firstOutcome, err := commitPhase6Ingest(t, store, initialization, first, phase6Profile("LOCK", "winner"))
	if err != nil || firstOutcome.Status != model.IngestCommitCommitted {
		t.Fatalf("winner = (%#v, %v)", firstOutcome, err)
	}
	secondOutcome, err := commitPhase6Ingest(t, store, initialization, second, phase6Profile("LOCK", "loser"))
	if !errors.Is(err, ErrSecretRefConflict) || secondOutcome.Status != model.IngestCommitConflict ||
		secondOutcome.Backend != model.IngestBackendUnchanged {
		t.Fatalf("loser = (%#v, %v), want prepare conflict before effects", secondOutcome, err)
	}
}

func TestCommitIngestEveryRefOperationIsNoDeref(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	var argv [][]string
	store.git.beforeStart = func(args []string) { argv = append(argv, append([]string(nil), args...)) }
	if outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("DIRECT", "one")); err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("CommitIngest = (%#v, %v)", outcome, err)
	}
	transactions := capturedUpdateRefTransactions(argv)
	if len(transactions) == 0 {
		t.Fatal("CommitIngest started no update-ref transaction")
	}
	for _, args := range transactions {
		if !reflect.DeepEqual(args, []string{"update-ref", "--no-deref", "--stdin"}) {
			t.Fatalf("unsafe update-ref argv = %#v", args)
		}
	}
}

func TestBeginAndCommitIngestRejectSymbolicOrSymlinkedMain(t *testing.T) {
	for _, kind := range []string{"symbolic", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			store, initialization, root := newPhase6IngestStore(t)
			ctx := context.Background()
			tip, err := store.git.revParse(ctx, "refs/heads/main")
			if err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(root, "refs", "heads", "other")
			if err := os.WriteFile(other, []byte(tip+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			mainRef := filepath.Join(root, "refs", "heads", "main")
			switch kind {
			case "symbolic":
				if err := os.WriteFile(mainRef, []byte("ref: refs/heads/other\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(mainRef); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("other", mainRef); err != nil {
					t.Fatal(err)
				}
			}
			starts := 0
			store.git.beforeStart = func([]string) { starts++ }
			if outcome, err := store.BeginIngest(ctx, initialization.ID()); err == nil ||
				outcome.FailureCode != model.IngestFailureBaselineRead {
				t.Fatalf("BeginIngest accepted %s main: (%#v, %v)", kind, outcome, err)
			}
			if starts != 0 {
				t.Fatalf("%s main started %d Git processes", kind, starts)
			}
		})
	}
}

func TestCommitIngestRecoveryGuardRejectsRefRedirection(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	mainRef := filepath.Join(root, "refs", "heads", "main")
	if err := os.WriteFile(mainRef, []byte("ref: refs/heads/other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("REDIRECT", "one"))
	if err == nil || outcome.Status == model.IngestCommitCommitted || !outcome.RecoveryRequired {
		t.Fatalf("redirected ref recovery = (%#v, %v)", outcome, err)
	}
}

type blockingPhase6Keychain struct {
	entered chan struct{}
	release chan struct{}
	values  map[string]string
	once    sync.Once
}

func (k *blockingPhase6Keychain) Store(key, value string) error {
	k.once.Do(func() { close(k.entered) })
	<-k.release
	k.values[key] = value
	return nil
}

func (k *blockingPhase6Keychain) Retrieve(key string) (string, error) {
	value, ok := k.values[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (k *blockingPhase6Keychain) Delete(key string) error {
	delete(k.values, key)
	return nil
}

func (*blockingPhase6Keychain) Kind() model.SecretRefKind { return model.SecretRefFile }

func TestCommitIngestRefCommitWaitsForDurableEffects(t *testing.T) {
	keychain := &blockingPhase6Keychain{entered: make(chan struct{}), release: make(chan struct{}), values: map[string]string{}}
	root := filepath.Join(t.TempDir(), "store")
	store, err := New(root, stubRegen{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transaction := beginPhase6Ingest(t, store, initialization)
	want := *transaction.Baseline.ExpectedRevision
	done := make(chan struct {
		outcome model.IngestCommitOutcome
		err     error
	}, 1)
	go func() {
		outcome, commitErr := commitPhase6Ingest(t, store, initialization, transaction, buildSecretProfile(t, "export API_KEY=blocked\n"))
		done <- struct {
			outcome model.IngestCommitOutcome
			err     error
		}{outcome: outcome, err: commitErr}
	}()
	select {
	case <-keychain.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("backend effect was not reached")
	}
	current, err := store.git.revParse(context.Background(), "refs/heads/main")
	if err != nil || current != want {
		t.Fatalf("ref moved before backend completed: %q, %v; want %q", current, err, want)
	}
	close(keychain.release)
	result := <-done
	if result.err != nil || result.outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("CommitIngest = (%#v, %v)", result.outcome, result.err)
	}
}

func TestCommitIngestFsyncsEveryLinkedObjectBeforeDirectoriesAndRef(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	linkedObjects := 0
	store.commitPublishLink = func(source, destination string) error {
		err := os.Link(source, destination)
		if err == nil {
			linkedObjects++
		}
		return err
	}
	var events []string
	store.commitEvent = func(event string) {
		events = append(events, event)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("FSYNC", "ordered"))
	if err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("ordered durability commit = (%#v, %v)", outcome, err)
	}

	objectSyncs := 0
	lastObjectSync := -1
	firstDirectorySync := -1
	rootSync := -1
	refCommit := -1
	for index, event := range events {
		switch event {
		case "object-fsync":
			objectSyncs++
			lastObjectSync = index
		case "fanout-fsync":
			if firstDirectorySync == -1 {
				firstDirectorySync = index
			}
		case "object-root-fsync":
			rootSync = index
		case "commit-write":
			refCommit = index
		}
	}
	if linkedObjects == 0 || objectSyncs != linkedObjects || lastObjectSync < 0 ||
		firstDirectorySync <= lastObjectSync || rootSync <= firstDirectorySync || refCommit <= rootSync {
		t.Fatalf("durability ordering: linked=%d object_syncs=%d last_object=%d first_dir=%d root=%d ref=%d",
			linkedObjects, objectSyncs, lastObjectSync, firstDirectorySync, rootSync, refCommit)
	}
}

func TestCommitIngestFsyncsReusedLooseObjectsBeforeRef(t *testing.T) {
	setup := func(t *testing.T) (*Store, InstallInitialization, model.IngestBeginOutcome, string) {
		t.Helper()
		store, initialization, _ := newPhase6IngestStore(t)
		transaction := beginPhase6Ingest(t, store, initialization)
		return store, initialization, transaction, *transaction.Baseline.ExpectedRevision
	}
	simulateExisting := func(reused map[string]struct{}) func(string, string) error {
		return func(source, destination string) error {
			err := os.Link(source, destination)
			if err == nil {
				reused[destination] = struct{}{}
				return os.ErrExist
			}
			if errors.Is(err, os.ErrExist) {
				reused[destination] = struct{}{}
			}
			return err
		}
	}

	t.Run("success", func(t *testing.T) {
		store, initialization, transaction, _ := setup(t)
		reused := make(map[string]struct{})
		store.commitPublishLink = simulateExisting(reused)
		synced := 0
		store.commitSyncObject = func(path string) error {
			if _, ok := reused[path]; !ok {
				t.Fatalf("sync called for object that was not reused: %s", path)
			}
			synced++
			return nil
		}
		outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("REUSED", "object"))
		if err != nil || outcome.Status != model.IngestCommitCommitted || len(reused) == 0 || synced != len(reused) {
			t.Fatalf("reused publication: status=%s reused=%d synced=%d err=%v",
				outcome.Status, len(reused), synced, err)
		}
	})

	t.Run("sync failure", func(t *testing.T) {
		store, initialization, transaction, baseline := setup(t)
		injected := errors.New("injected reused object fsync failure")
		reused := make(map[string]struct{})
		store.commitPublishLink = simulateExisting(reused)
		store.commitSyncObject = func(path string) error {
			if _, ok := reused[path]; ok {
				return injected
			}
			return nil
		}
		refCommitWritten := false
		store.commitEvent = func(event string) {
			if event == "commit-write" {
				refCommitWritten = true
			}
		}
		outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("REUSED", "object"))
		if !errors.Is(err, injected) || len(reused) == 0 || refCommitWritten ||
			outcome.Status != model.IngestCommitNotCommitted || outcome.Objects != model.IngestObjectsUncertain ||
			outcome.Cleanup != model.QuarantineCleanupRetained ||
			outcome.FailureCode != model.IngestFailureObjectPublish || !outcome.RecoveryRequired {
			t.Fatalf("reused sync failure: reused=%d ref_commit=%t status=%s objects=%s cleanup=%s failure=%s recovery=%t err=%v",
				len(reused), refCommitWritten, outcome.Status, outcome.Objects, outcome.Cleanup,
				outcome.FailureCode, outcome.RecoveryRequired, err)
		}
		current, readErr := store.git.revParse(context.Background(), "refs/heads/main")
		if readErr != nil || current != baseline {
			t.Fatalf("reused sync failure moved ref: current=%q err=%v want=%q", current, readErr, baseline)
		}
	})
}

func TestCommitIngestPostPublicationLockFailurePreservesCommittedStatus(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	namespace, err := storeTransactionNamespacePath(root)
	if err != nil {
		t.Fatal(err)
	}
	invalidated := false
	store.commitEvent = func(event string) {
		if event != "commit-write" || invalidated {
			return
		}
		invalidated = true
		if chmodErr := os.Chmod(namespace, 0o755); chmodErr != nil {
			t.Errorf("invalidate transaction namespace: %v", chmodErr)
		}
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("LOCK", "published"))
	if !invalidated || !errors.Is(err, ErrIngestRecoveryRequired) ||
		outcome.Status != model.IngestCommitCommitted || outcome.RefState != model.IngestRefCandidate ||
		outcome.Objects != model.IngestObjectsPublished || outcome.Cleanup != model.QuarantineCleanupRetained ||
		outcome.FailureCode != model.IngestFailureQuarantine || !outcome.RecoveryRequired {
		t.Fatalf("post-publication lock failure: invalidated=%t status=%s ref=%s objects=%s cleanup=%s failure=%s recovery=%t err=%v",
			invalidated, outcome.Status, outcome.RefState, outcome.Objects, outcome.Cleanup,
			outcome.FailureCode, outcome.RecoveryRequired, err)
	}
	back, readErr := store.Read(context.Background(), "main")
	if readErr != nil || !reflect.DeepEqual(back, phase6Profile("LOCK", "published")) {
		t.Fatalf("committed ref was not published: profile=%#v err=%v", back, readErr)
	}
}

func TestCommitIngestLinkedObjectFsyncFailureRequiresRecoveryBeforeRef(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	wantRef := *transaction.Baseline.ExpectedRevision
	injected := errors.New("injected object fsync failure")
	syncAttempts := 0
	store.commitSyncObject = func(string) error {
		syncAttempts++
		return injected
	}
	var refCommitWritten bool
	store.commitEvent = func(event string) {
		if event == "commit-write" {
			refCommitWritten = true
		}
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("FSYNC", "failure"))
	if !errors.Is(err, injected) || syncAttempts != 1 || refCommitWritten ||
		outcome.Status != model.IngestCommitNotCommitted || outcome.Objects != model.IngestObjectsUncertain ||
		outcome.Cleanup != model.QuarantineCleanupRetained ||
		outcome.FailureCode != model.IngestFailureObjectPublish || !outcome.RecoveryRequired {
		t.Fatalf("object fsync failure: attempts=%d ref_commit=%t status=%s objects=%s cleanup=%s failure=%s recovery=%t err=%v",
			syncAttempts, refCommitWritten, outcome.Status, outcome.Objects, outcome.Cleanup,
			outcome.FailureCode, outcome.RecoveryRequired, err)
	}
	if current, readErr := store.git.revParse(context.Background(), "refs/heads/main"); readErr != nil || current != wantRef {
		t.Fatalf("object fsync failure moved ref: current=%q err=%v want=%q", current, readErr, wantRef)
	}
}

func TestCommitIngestPreCommitFailureNeverWritesRefCommit(t *testing.T) {
	keychain := &transactionKeychain{values: map[string]string{}, failKey: "API_KEY", failOnce: true}
	root := filepath.Join(t.TempDir(), "store")
	store, err := New(root, stubRegen{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transaction := beginPhase6Ingest(t, store, initialization)
	want := *transaction.Baseline.ExpectedRevision
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, buildSecretProfile(t, "export API_KEY=blocked\n"))
	if !errors.Is(err, ErrSecretBackendUnavailable) || outcome.Status == model.IngestCommitCommitted {
		t.Fatalf("backend failure = (%#v, %v)", outcome, err)
	}
	current, readErr := store.git.revParse(context.Background(), "refs/heads/main")
	if readErr != nil || current != want {
		t.Fatalf("backend failure moved ref: %q, %v; want %q", current, readErr, want)
	}
}

func TestCommitIngestRejectsRefPresenceExpectedMismatch(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	*transaction.Baseline.ExpectedRevision = "invalid"
	starts := 0
	store.git.beforeStart = func([]string) { starts++ }
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("INVALID", "one"))
	if err == nil || outcome.Status == model.IngestCommitCommitted || starts != 0 {
		t.Fatalf("invalid expected evidence = (%#v, %v), starts=%d", outcome, err, starts)
	}
}

func TestCommitIngestRefMismatchCleansWithZeroPublication(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, store, initialization)
	second := beginPhase6Ingest(t, store, initialization)
	if outcome, err := commitPhase6Ingest(t, store, initialization, first, phase6Profile("WINNER", "one")); err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("winner = (%#v, %v)", outcome, err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, second, phase6Profile("LOSER", "two"))
	if !errors.Is(err, ErrSecretRefConflict) || outcome.Status != model.IngestCommitConflict ||
		outcome.Backend != model.IngestBackendUnchanged || outcome.Objects != model.IngestObjectsRemoved ||
		outcome.Cleanup != model.QuarantineCleanupRemoved {
		t.Fatalf("loser = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestUpdateRefUsesSanitizedBareEnvironment(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	decoy := newTestStore(t)
	if err := decoy.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	decoyTip, err := decoy.git.revParse(context.Background(), "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", decoy.dir)
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "hostile.gitconfig"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "hostile-index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(t.TempDir(), "hostile-objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(t.TempDir(), "hostile-alternates"))
	transaction := beginPhase6Ingest(t, store, initialization)
	if outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("SAFE", "one")); err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("CommitIngest = (%#v, %v)", outcome, err)
	}
	if current, err := decoy.git.revParse(context.Background(), "refs/heads/main"); err != nil || current != decoyTip {
		t.Fatalf("hostile environment changed decoy ref: %q, %v", current, err)
	}
}

func TestCommitIngestClaimsRegisteredTransaction(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	committer, ok := any(store).(phase6IngestCommitter)
	if !ok {
		t.Fatal("Store does not implement CommitIngest")
	}
	outcome, err := committer.CommitIngest(context.Background(), initialization.ID(), model.IngestTransactionID{}, model.Profile{}, "claim")
	if !errors.Is(err, ErrInvalidIngestAuthority) || outcome.FailureCode != model.IngestFailureInvalidAuthority {
		t.Fatalf("unknown token = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestRejectsForgedOrSwappedToken(t *testing.T) {
	firstStore, firstInitialization, _ := newPhase6IngestStore(t)
	secondStore, secondInitialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, firstStore, firstInitialization)
	second := beginPhase6Ingest(t, secondStore, secondInitialization)
	committer := any(firstStore).(phase6IngestCommitter)
	for _, authority := range []struct {
		initialization model.InstallInitializationID
		transaction    model.IngestTransactionID
	}{
		{initialization: firstInitialization.ID(), transaction: second.TransactionID},
		{initialization: secondInitialization.ID(), transaction: first.TransactionID},
	} {
		outcome, err := committer.CommitIngest(context.Background(), authority.initialization, authority.transaction, model.Profile{}, "forged")
		if !errors.Is(err, ErrInvalidIngestAuthority) || outcome.Status == model.IngestCommitCommitted {
			t.Fatalf("forged authority = (%#v, %v)", outcome, err)
		}
	}
}

func TestCommitAbortAndCleanupSerialize(t *testing.T) {
	keychain := &blockingPhase6Keychain{entered: make(chan struct{}), release: make(chan struct{}), values: map[string]string{}}
	root := filepath.Join(t.TempDir(), "store")
	store, err := New(root, stubRegen{}, keychain)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := store.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transaction := beginPhase6Ingest(t, store, initialization)
	done := make(chan error, 1)
	go func() {
		_, commitErr := commitPhase6Ingest(t, store, initialization, transaction, buildSecretProfile(t, "export API_KEY=blocked\n"))
		done <- commitErr
	}()
	select {
	case <-keychain.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("commit did not reach backend")
	}
	if _, err := store.AbortIngest(context.Background(), initialization.ID(), transaction.TransactionID); !errors.Is(err, ErrInvalidIngestAuthority) {
		t.Fatalf("AbortIngest during Commit = %v, want invalid authority", err)
	}
	close(keychain.release)
	if err := <-done; err != nil {
		t.Fatalf("CommitIngest after release: %v", err)
	}
}

func TestCommitIngestCleanupRequiresCrossProcessRootLock(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	store.cleanupDiscardLock = true
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("LOCKED", "one"))
	if err == nil || outcome.Status != model.IngestCommitCommitted || outcome.Cleanup != model.QuarantineCleanupRetained ||
		!outcome.RecoveryRequired {
		t.Fatalf("discarded cleanup lock = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestCommittedCleanupAxis(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("CLEAN", "one"))
	if err != nil || outcome.Status != model.IngestCommitCommitted || outcome.Objects != model.IngestObjectsPublished ||
		outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("committed cleanup = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestLostResponseExpectedPreservesRetainedObjects(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	wantRef := *transaction.Baseline.ExpectedRevision
	linkedObjects := 0
	store.commitPublishLink = func(source, destination string) error {
		if err := os.Link(source, destination); err != nil {
			return err
		}
		linkedObjects++
		return nil
	}
	store.commitRefSession = func(session *updateRefSession) error {
		if err := session.Abort(); err != nil {
			return err
		}
		return ErrGitCommand
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("RETAINED", "one"))
	if linkedObjects == 0 || !errors.Is(err, ErrIngestNotCommitted) || outcome.Status != model.IngestCommitNotCommitted ||
		outcome.RefState != model.IngestRefExpected || outcome.Objects != model.IngestObjectsRetained ||
		outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("lost response at expected ref = (%#v, %v)", outcome, err)
	}
	if current, readErr := store.git.revParse(context.Background(), "refs/heads/main"); readErr != nil || current != wantRef {
		t.Fatalf("lost response moved ref: current=%q err=%v want=%q", current, readErr, wantRef)
	}
}

func TestCommitIngestNotCommittedCleanupAxis(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, store, initialization)
	second := beginPhase6Ingest(t, store, initialization)
	if _, err := commitPhase6Ingest(t, store, initialization, first, phase6Profile("FIRST", "one")); err != nil {
		t.Fatal(err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, second, phase6Profile("SECOND", "two"))
	if !errors.Is(err, ErrSecretRefConflict) || outcome.Status != model.IngestCommitConflict ||
		outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("conflict cleanup = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestCleanupFailurePreservesPublication(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	store.cleanupBeforeFinalCheck = func(quarantineCleanupSeam) error { return errors.New("injected cleanup failure") }
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("PUBLISHED", "one"))
	if err == nil || outcome.Status != model.IngestCommitCommitted || outcome.Cleanup != model.QuarantineCleanupRetained ||
		!outcome.RecoveryRequired {
		t.Fatalf("cleanup failure rewrote publication = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestCleanupRejectsNestedReplacement(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	store.cleanupBeforeFinalCheck = func(seam quarantineCleanupSeam) error {
		if seam.Relative != "." {
			return errors.New("injected nested replacement")
		}
		return nil
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("NESTED", "one"))
	if err == nil || outcome.Status != model.IngestCommitCommitted || outcome.Cleanup != model.QuarantineCleanupRetained {
		t.Fatalf("nested replacement = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestObservedCandidateIsCommitted(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("OBSERVED", "candidate"))
	if err != nil || outcome.Status != model.IngestCommitCommitted || outcome.RefState != model.IngestRefCandidate {
		t.Fatalf("candidate observation = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestExpectedRefRecoveryGuard(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, store, initialization)
	second := beginPhase6Ingest(t, store, initialization)
	if _, err := commitPhase6Ingest(t, store, initialization, first, phase6Profile("GUARD", "winner")); err != nil {
		t.Fatal(err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, second, phase6Profile("GUARD", "loser"))
	if !errors.Is(err, ErrSecretRefConflict) || outcome.Status != model.IngestCommitConflict || outcome.Backend != model.IngestBackendUnchanged {
		t.Fatalf("expected ref guard = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestUnknownRefRequiresRecovery(t *testing.T) {
	store, initialization, root := newPhase6IngestStore(t)
	transaction := beginPhase6Ingest(t, store, initialization)
	mainRef := filepath.Join(root, "refs", "heads", "main")
	if err := os.WriteFile(mainRef, []byte("not-an-object\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, transaction, phase6Profile("UNKNOWN", "one"))
	if err == nil || outcome.Status == model.IngestCommitCommitted || !outcome.RecoveryRequired {
		t.Fatalf("unknown ref = (%#v, %v)", outcome, err)
	}
}

func TestCommitIngestDifferentProfilesConflict(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	first := beginPhase6Ingest(t, store, initialization)
	second := beginPhase6Ingest(t, store, initialization)
	if outcome, err := commitPhase6Ingest(t, store, initialization, first, phase6Profile("PROFILE", "winner")); err != nil || outcome.Status != model.IngestCommitCommitted {
		t.Fatalf("winner = (%#v, %v)", outcome, err)
	}
	outcome, err := commitPhase6Ingest(t, store, initialization, second, phase6Profile("PROFILE", "loser"))
	if !errors.Is(err, ErrSecretRefConflict) || outcome.Status != model.IngestCommitConflict || outcome.Backend != model.IngestBackendUnchanged {
		t.Fatalf("loser = (%#v, %v)", outcome, err)
	}
	profile, readErr := store.Read(context.Background(), "main")
	if readErr != nil || !reflect.DeepEqual(profile, phase6Profile("PROFILE", "winner")) {
		t.Fatalf("winner profile = (%#v, %v)", profile, readErr)
	}
}
