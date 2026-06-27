package store

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMapGitErrorNeverLeaksStderr proves mapGitError maps a raw git failure to a
// zsh-pro-phrased sentinel and never embeds the raw stderr (D-11; T-03-01). A
// `fatal: ...` substring must NOT reach the returned error.
func TestMapGitErrorNeverLeaksStderr(t *testing.T) {
	raw := "fatal: not a git repository (or any of the parent directories): .git"
	got := mapGitError(errors.New("exit status 128"), raw)

	if got == nil {
		t.Fatal("mapGitError returned nil for a failure")
	}
	if strings.Contains(got.Error(), "fatal:") {
		t.Errorf("mapped error leaks raw git stderr: %q", got.Error())
	}
	if strings.Contains(got.Error(), "not a git repository") {
		t.Errorf("mapped error leaks raw git stderr content: %q", got.Error())
	}
	// It must be an errStore sentinel (the zsh-pro-phrased typed kind).
	var se errStore
	if !errors.As(got, &se) {
		t.Errorf("mapped error is not an errStore sentinel: %T", got)
	}
	if got != ErrGitCommand {
		t.Errorf("generic git failure should map to ErrGitCommand, got %v", got)
	}
}

// TestErrStoreSentinelsArePhrased proves every sentinel implements error and is
// zsh-pro-phrased (starts with "zsh-pro:") — no raw git/keychain vocabulary (D-11).
func TestErrStoreSentinelsArePhrased(t *testing.T) {
	var _ error = ErrGitAbsent // compile-time: errStore implements error

	sentinels := []errStore{
		ErrGitAbsent, ErrNotInitialized, ErrProfileNotFound,
		ErrProfileExists, ErrSecretBackendUnavailable, ErrGitCommand,
	}
	for _, s := range sentinels {
		if !strings.HasPrefix(s.Error(), "zsh-pro:") {
			t.Errorf("sentinel not zsh-pro-phrased: %q", s.Error())
		}
		for _, leak := range []string{"fatal:", "git:", "SecKeychain", "security:"} {
			if strings.Contains(s.Error(), leak) {
				t.Errorf("sentinel %q leaks raw vocabulary %q", s.Error(), leak)
			}
		}
	}
}

// TestEmptyTreeSHA pins git's well-known empty-tree object id (used to seed a root
// commit). A typo here would break Plan 02's Init.
func TestEmptyTreeSHA(t *testing.T) {
	const want = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	if emptyTreeSHA != want {
		t.Errorf("emptyTreeSHA = %q, want %q", emptyTreeSHA, want)
	}
}

// TestNewGitRunnerAbsenceGuard proves newGitRunner returns ErrGitAbsent (never a
// crash) when git is not on $PATH. Skipped when git IS present (cannot simulate
// absence without mutating $PATH), but always exercises the present-path success.
func TestNewGitRunner(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		// git absent: the guard must fire.
		if _, gerr := newGitRunner(t.TempDir()); gerr != ErrGitAbsent {
			t.Fatalf("git absent: newGitRunner should return ErrGitAbsent, got %v", gerr)
		}
		t.Skip("git not installed; absence guard verified")
	}
	g, err := newGitRunner(t.TempDir())
	if err != nil {
		t.Fatalf("git present: newGitRunner should succeed, got %v", err)
	}
	if g.repoDir == "" {
		t.Error("newGitRunner left repoDir empty")
	}
}

// TestGitPlumbingPrimitives exercises the real git plumbing against a temp bare
// repo: hashObject returns a 40-char SHA for arbitrary content, and catFileExists
// is true for that SHA but false for a missing ref. Skip-guarded for git-absent
// environments, exactly like core/ir/roundtrip_test.go.
func TestGitPlumbingPrimitives(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store git plumbing tests")
	}

	dir := t.TempDir()
	// Initialize a bare repo via the binary directly (Init orchestration is Plan 02).
	if out, err := exec.Command("git", "init", "--bare", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}

	g, err := newGitRunner(dir)
	if err != nil {
		t.Fatalf("newGitRunner: %v", err)
	}
	ctx := context.Background()

	// isBareRepo should be true for a freshly --bare repo.
	if !g.isBareRepo(ctx) {
		t.Error("isBareRepo returned false for a --bare repo")
	}

	// hashObject of arbitrary bytes returns a 40-char SHA...
	sha, err := g.hashObject(ctx, []byte("profile.json contents\n"))
	if err != nil {
		t.Fatalf("hashObject: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("hashObject returned a non-40-char SHA: %q (len %d)", sha, len(sha))
	}

	// ...and that object now exists in the DB.
	if !g.catFileExists(ctx, sha) {
		t.Errorf("catFileExists(%q) = false for a just-written blob", sha)
	}

	// A missing ref must probe false (no raw error surfaced).
	if g.catFileExists(ctx, "refs/heads/does-not-exist") {
		t.Error("catFileExists returned true for a missing ref")
	}

	// show should return the exact blob bytes.
	got, err := g.show(ctx, sha)
	if err != nil {
		t.Fatalf("show(%q): %v", sha, err)
	}
	if string(got) != "profile.json contents\n" {
		t.Errorf("show returned %q, want the original blob content", got)
	}
}

// TestGitCommitToBranch exercises the remaining primitives end-to-end against a
// bare repo — the exact commit-to-a-non-checked-out-branch path Plan 02 composes
// (Pattern 1 / D-12): stage a blob into a temp index, write-tree, deterministic
// commit-tree (runCommit), update-ref, then prove the branch resolves (revParse),
// reads back (show), and lists (forEachRef). It also pins that runCommit produces a
// byte-stable commit SHA for a fixed timestamp (Pitfall 2). Skip-guarded.
func TestGitCommitToBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store commit-plumbing test")
	}

	commitOnce := func(t *testing.T) (sha, branchTip string) {
		t.Helper()
		dir := t.TempDir()
		if out, err := exec.Command("git", "init", "--bare", "-b", "main", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init --bare failed: %v\n%s", err, out)
		}
		g, err := newGitRunner(dir)
		if err != nil {
			t.Fatalf("newGitRunner: %v", err)
		}
		ctx := context.Background()

		// 1. blob for profile.json content.
		blob, err := g.hashObject(ctx, []byte(`{"entries":[]}`+"\n"))
		if err != nil {
			t.Fatalf("hashObject: %v", err)
		}

		// 2. temp index OUTSIDE any working tree (a bare repo has none anyway).
		idx := filepath.Join(t.TempDir(), "index")
		const ts = "1700000000 +0000" // fixed => byte-stable commit SHA (Pitfall 2)

		// 3. start empty, stage the blob via cacheinfo, snapshot to a tree.
		if _, err := g.runCommit(ctx, idx, ts, "read-tree", "--empty"); err != nil {
			t.Fatalf("read-tree --empty: %v", err)
		}
		if _, err := g.runCommit(ctx, idx, ts, "update-index", "--add", "--cacheinfo", "100644,"+blob+",profile.json"); err != nil {
			t.Fatalf("update-index: %v", err)
		}
		treeOut, err := g.runCommit(ctx, idx, ts, "write-tree")
		if err != nil {
			t.Fatalf("write-tree: %v", err)
		}
		tree := strings.TrimSpace(string(treeOut))

		// 4. deterministic root commit (no -p) via runCommit (fixed author/committer env).
		commitOut, err := g.runCommit(ctx, idx, ts, "commit-tree", tree, "-m", "snapshot")
		if err != nil {
			t.Fatalf("commit-tree: %v", err)
		}
		commit := strings.TrimSpace(string(commitOut))

		// 5. move the branch ref.
		if err := g.updateRef(ctx, "refs/heads/main", commit); err != nil {
			t.Fatalf("updateRef: %v", err)
		}

		// revParse(main) must resolve to the commit we just made.
		tip, err := g.revParse(ctx, "main")
		if err != nil {
			t.Fatalf("revParse(main): %v", err)
		}
		if tip != commit {
			t.Errorf("revParse(main)=%q, want the committed SHA %q", tip, commit)
		}

		// show <branch>:<path> reads the blob back from the object DB (D-12, no checkout).
		back, err := g.show(ctx, "main:profile.json")
		if err != nil {
			t.Fatalf("show(main:profile.json): %v", err)
		}
		if string(back) != `{"entries":[]}`+"\n" {
			t.Errorf("show read back %q, want the committed JSON", back)
		}

		// forEachRef lists the branch by short name.
		refs, err := g.forEachRef(ctx, "refs/heads/")
		if err != nil {
			t.Fatalf("forEachRef: %v", err)
		}
		if !strings.Contains(string(refs), "main") {
			t.Errorf("forEachRef did not list 'main': %q", refs)
		}
		return commit, tip
	}

	// Determinism (Pitfall 2): the same content + fixed timestamp/author yields the
	// same commit SHA across two independent bare repos.
	sha1, _ := commitOnce(t)
	sha2, _ := commitOnce(t)
	if sha1 != sha2 {
		t.Errorf("runCommit is not byte-stable for a fixed timestamp: %q vs %q", sha1, sha2)
	}
}
