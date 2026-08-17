package store

import (
	"context"
	"errors"
	"os"
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
		ErrProfileExists, ErrInvalidProfileName, ErrSecretBackendUnavailable, ErrGitCommand,
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

func TestGitSupportsUpdateRefTransactions(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "before transaction verbs", output: "git version 2.26.3\n", want: false},
		{name: "minimum supported", output: "git version 2.27.0\n", want: true},
		{name: "vendor suffix", output: "git version 2.27.0.windows.1\n", want: true},
		{name: "vendor annotation", output: "git version 2.39.3 (Apple Git-146)\n", want: true},
		{name: "new major", output: "git version 3.0.0\n", want: true},
		{name: "missing patch", output: "git version 2.27\n", want: false},
		{name: "non-numeric minor", output: "git version 2.next.0\n", want: false},
		{name: "non-numeric patch", output: "git version 2.27.next\n", want: false},
		{name: "unexpected format", output: "version 2.43.0\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gitSupportsUpdateRefTransactions([]byte(test.output)); got != test.want {
				t.Fatalf("support decision = %t, want %t", got, test.want)
			}
		})
	}
}

func TestStartUpdateRefSessionRejectsUnsupportedGitBeforeProcess(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
	}{
		{name: "older version", output: "git version 2.26.3\n"},
		{name: "malformed output", output: "git version unknown\n"},
		{name: "version probe failure", err: errors.New("probe failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			starts := 0
			runner := gitRunner{
				repoDir: t.TempDir(),
				beforeStart: func([]string) {
					starts++
				},
				gitVersionOutput: func(context.Context) ([]byte, error) {
					return []byte(test.output), test.err
				},
			}
			session, err := runner.startUpdateRefSession(context.Background())
			if session != nil || !errors.Is(err, ErrGitCommand) {
				t.Fatalf("unsupported Git gate: session_present=%t err_is_git_command=%t",
					session != nil, errors.Is(err, ErrGitCommand))
			}
			if starts != 0 {
				t.Fatalf("unsupported Git started %d update-ref processes", starts)
			}
		})
	}
}

func TestValidateGitArgvAllowsOnlyExactVersionProbe(t *testing.T) {
	if err := validateGitArgv([]string{"version"}); err != nil {
		t.Fatalf("exact version probe rejected: %v", err)
	}
	if err := validateGitArgv([]string{"version", "--build-options"}); !errors.Is(err, ErrGitCommand) {
		t.Fatalf("version probe with extra authority accepted: %v", err)
	}
}

func TestCommitTreeAllowsHyphenPrefixedMessageOperand(t *testing.T) {
	for _, message := range []string{"-snapshot", "-C", "--git-dir=/tmp/not-authority"} {
		if err := validateGitArgv([]string{"commit-tree", emptyTreeSHA, "-m", message}); err != nil {
			t.Fatalf("safe hyphen-prefixed commit message %q rejected: %v", message, err)
		}
	}
	if err := validateGitArgv([]string{"commit-tree", emptyTreeSHA, "-p", "-parent", "-m", "snapshot"}); !errors.Is(err, ErrGitCommand) {
		t.Fatalf("hyphen-prefixed parent atom accepted: %v", err)
	}

	runner := newBareGitRunnerForTest(t, "message-store")
	commit := createGitCommitForTest(t, runner, "-snapshot")
	if !validGitObjectID(commit) {
		t.Fatalf("commit-tree returned invalid object ID %q", commit)
	}
}

func TestCandidateAlternateQuotesColonInStorePath(t *testing.T) {
	runner := newBareGitRunnerForTest(t, "store:primary")
	objectID, err := runner.hashObject(context.Background(), []byte("alternate-visible\n"))
	if err != nil {
		t.Fatal(err)
	}
	privateObjects := filepath.Join(t.TempDir(), "objects")
	if err := os.Mkdir(privateObjects, 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := runner.candidate(filepath.Join(t.TempDir(), "index"), privateObjects)
	wantAlternate := quoteGitPath(filepath.Join(runner.repoDir, "objects"))
	if candidate.privateAlternateObject != wantAlternate || !strings.HasPrefix(wantAlternate, "\"") {
		t.Fatalf("candidate alternate = %q, want Git C-style path %q", candidate.privateAlternateObject, wantAlternate)
	}
	if !candidate.catFileExists(context.Background(), objectID) {
		t.Fatal("quoted colon-bearing alternate could not read the authenticated store object")
	}
}

func TestValidateDirectRefRejectsSymlinkedPackedRefsBeforeGit(t *testing.T) {
	runner := newBareGitRunnerForTest(t, "packed-store")
	packedRefs := filepath.Join(runner.repoDir, "packed-refs")
	if err := os.Symlink("HEAD", packedRefs); err != nil {
		t.Fatal(err)
	}
	starts := 0
	runner.beforeStart = func([]string) { starts++ }
	_, _, err := runner.observeDirectRef(context.Background(), mainHeadRef())
	if !errors.Is(err, ErrGitCommand) {
		t.Fatalf("symlinked packed-refs error = %v, want Git command rejection", err)
	}
	if starts != 0 {
		t.Fatalf("symlinked packed-refs started %d Git processes", starts)
	}
}

func TestUpdateRefPrepareClassifiesOnlyGitLockRejectionAsConflict(t *testing.T) {
	t.Run("Git lock rejection", func(t *testing.T) {
		runner := newBareGitRunnerForTest(t, "conflict-store")
		current := createGitCommitForTest(t, runner, "current")
		if err := runner.updateRef(context.Background(), mainHeadRef().name, current); err != nil {
			t.Fatal(err)
		}
		session, err := runner.startUpdateRefSession(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		mutation, err := encodeRefMutation(mainHeadRef(), current, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Prepare(mutation); !errors.Is(err, ErrSecretRefConflict) {
			t.Fatalf("prepare lock rejection = %v, want ref conflict", err)
		}
	})

	t.Run("process transport failure", func(t *testing.T) {
		runner := newBareGitRunnerForTest(t, "transport-store")
		candidate := createGitCommitForTest(t, runner, "candidate")
		session, err := runner.startUpdateRefSession(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		mutation, err := encodeRefMutation(mainHeadRef(), candidate, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		err = session.Prepare(mutation)
		if !errors.Is(err, ErrGitCommand) || errors.Is(err, ErrSecretRefConflict) {
			t.Fatalf("killed prepare process = %v, want infrastructure Git error", err)
		}
	})
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

func newBareGitRunnerForTest(t *testing.T, name string) gitRunner {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping store Git test")
	}
	directory := filepath.Join(t.TempDir(), name)
	if output, err := exec.Command("git", "init", "--bare", "-b", "main", directory).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, output)
	}
	runner, err := newGitRunner(directory)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func createGitCommitForTest(t *testing.T, runner gitRunner, message string) string {
	t.Helper()
	ctx := context.Background()
	index := filepath.Join(t.TempDir(), "index")
	const timestamp = "1700000000 +0000"
	if _, err := runner.runCommit(ctx, index, timestamp, "read-tree", "--empty"); err != nil {
		t.Fatal(err)
	}
	treeOutput, err := runner.runCommit(ctx, index, timestamp, "write-tree")
	if err != nil {
		t.Fatal(err)
	}
	commitOutput, err := runner.runCommit(
		ctx,
		index,
		timestamp,
		"commit-tree",
		strings.TrimSpace(string(treeOutput)),
		"-m",
		message,
	)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(commitOutput))
}

func TestGitArgsWorktreeExactReadAllowlist(t *testing.T) {
	oid := strings.Repeat("a", 40)
	for _, args := range [][]string{
		{"cat-file", "-t", oid},
		{"cat-file", "blob", oid},
		{"ls-tree", "-z", oid},
	} {
		if err := validateGitArgv(args); err != nil {
			t.Fatalf("worktree read argv %q rejected: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"cat-file", "-p", oid},
		{"cat-file", "blob", oid + ":profile.json"},
		{"ls-tree", "-r", oid},
		{"ls-tree", "-z", "--name-only", oid},
	} {
		if err := validateGitArgv(args); !errors.Is(err, ErrGitCommand) {
			t.Fatalf("hostile or over-broad argv %q accepted: %v", args, err)
		}
	}
}

func TestReadWorktreeRevisionStrictTreeRecords(t *testing.T) {
	a := strings.Repeat("a", 40)
	b := strings.Repeat("b", 40)
	valid := []byte("100644 blob " + a + "\tprofile.json\x00" +
		"100644 blob " + b + "\tprofile.zsh\x00")
	got, err := parseWorktreeTreeEntries(valid, 40)
	if err != nil || got.profileJSON != a || got.profileZSH != b {
		t.Fatalf("valid fixed tree = %#v, %v", got, err)
	}

	cases := map[string][]byte{
		"missing":         []byte("100644 blob " + a + "\tprofile.json\x00"),
		"duplicate":       append(append([]byte(nil), valid...), []byte("100644 blob "+a+"\tprofile.json\x00")...),
		"unexpected path": append(append([]byte(nil), valid...), []byte("100644 blob "+a+"\tother\x00")...),
		"wrong mode":      []byte("100755 blob " + a + "\tprofile.json\x00100644 blob " + b + "\tprofile.zsh\x00"),
		"wrong type":      []byte("100644 tree " + a + "\tprofile.json\x00100644 blob " + b + "\tprofile.zsh\x00"),
		"invalid oid":     []byte("100644 blob nope\tprofile.json\x00100644 blob " + b + "\tprofile.zsh\x00"),
		"malformed":       []byte("100644 blob " + a + " profile.json\x00100644 blob " + b + "\tprofile.zsh\x00"),
		"unterminated":    valid[:len(valid)-1],
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWorktreeTreeEntries(payload, 40); !errors.Is(err, ErrGitCommand) {
				t.Fatalf("malformed tree accepted: %v", err)
			}
		})
	}
}
