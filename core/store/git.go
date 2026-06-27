package store

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// emptyTreeSHA is git's well-known empty-tree object id (`git hash-object -t tree
// /dev/null`). It seeds a root commit for a brand-new baseline branch without
// staging any blob — used by Plan 02's Init.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// gitTimeout is the per-call subprocess timeout, mirroring the 5s zsh -f timeout
// in the zsh provider's introspect.go.
const gitTimeout = 5 * time.Second

// gitRunner drives the git binary as a subprocess against a single repo directory.
// It mirrors the introspect.go shape verbatim: context.WithTimeout + defer cancel
// + exec.CommandContext, separate stdout/stderr buffers, and a mapped (never raw)
// error on failure (D-11). The store never imports a git library (go-git rejected).
type gitRunner struct {
	repoDir string // path to the bare git repo ($ZSHPRO_HOME)
}

// newGitRunner constructs a gitRunner after a one-time exec.LookPath("git") guard,
// mirroring the introspect.go absence check. An absent git binary degrades to
// ErrGitAbsent — never a crash (D-06).
func newGitRunner(dir string) (gitRunner, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return gitRunner{}, ErrGitAbsent
	}
	return gitRunner{repoDir: dir}, nil
}

// run executes `git -C <repoDir> <args...>` with a timeout, returning stdout on
// success and a zsh-pro-phrased mapped error on failure (raw stderr is never
// surfaced — D-11, Pitfall 4). This is the canonical shape lifted from the zsh
// provider's introspect.go subprocess runner.
func (g gitRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.repoDir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, mapGitError(err, errb.String())
	}
	return out.Bytes(), nil
}

// runStdin is run with content piped to git's stdin (needed for
// `hash-object -w --stdin`, which writes a blob from stdin content).
func (g gitRunner) runStdin(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.repoDir}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, mapGitError(err, errb.String())
	}
	return out.Bytes(), nil
}

// runCommit runs git with a deterministic commit environment so commit SHAs are
// byte-stable across runs (Pitfall 2): it pins GIT_DIR, an isolated temp
// GIT_INDEX_FILE (outside any working tree), and fixed author/committer
// name/email/date. ts is the commit timestamp (e.g. "0 +0000" or RFC2822). It is
// used by Plan 02's Commit for the temp-index write-tree -> commit-tree path.
func (g gitRunner) runCommit(ctx context.Context, tmpIndex, ts string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.repoDir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_DIR="+g.repoDir,
		"GIT_INDEX_FILE="+tmpIndex,
		"GIT_AUTHOR_NAME=zsh-pro",
		"GIT_AUTHOR_EMAIL=zsh-pro@local",
		"GIT_AUTHOR_DATE="+ts,
		"GIT_COMMITTER_NAME=zsh-pro",
		"GIT_COMMITTER_EMAIL=zsh-pro@local",
		"GIT_COMMITTER_DATE="+ts,
	)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, mapGitError(err, errb.String())
	}
	return out.Bytes(), nil
}

// mapGitError translates a git subprocess failure into a zsh-pro-phrased typed
// error. It MUST NOT embed the raw stderr (D-11; T-03-01): the user never sees
// `fatal: ...`. The stderr argument is accepted for symmetry / future
// fine-grained mapping but is deliberately not interpolated into the message.
func mapGitError(err error, stderr string) error {
	_ = err    // exit state could refine the mapping later; never surfaced raw
	_ = stderr // raw git stderr is intentionally discarded (D-11)
	return ErrGitCommand
}

// --- Plumbing primitives (thin argv wrappers Plan 02 composes) -----------------
// None of these implement Init/Read/Commit orchestration (that is Plan 02); each
// is just the right git plumbing verb behind the timeout+degrade subprocess shape.

// hashObject writes content to the object DB as a blob and returns its 40-char SHA
// (`git hash-object -w --stdin`). Content flows via stdin, never argv or disk.
func (g gitRunner) hashObject(ctx context.Context, content []byte) (string, error) {
	out, err := g.runStdin(ctx, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

// catFileExists reports whether ref resolves to an existing object
// (`git cat-file -e <ref>`: exit 0 => true). A clean existence probe with no raw
// error surfaced.
func (g gitRunner) catFileExists(ctx context.Context, ref string) bool {
	_, err := g.run(ctx, "cat-file", "-e", ref)
	return err == nil
}

// show returns the bytes of ref from the object DB (`git show <ref>`), e.g.
// `<branch>:profile.json`. Used for reads — never a working-tree checkout (D-12).
func (g gitRunner) show(ctx context.Context, ref string) ([]byte, error) {
	return g.run(ctx, "show", ref)
}

// forEachRef lists refs matching pattern as short names, one per line
// (`git for-each-ref --format=%(refname:short) <pattern>`). Used to enumerate
// profile branches.
func (g gitRunner) forEachRef(ctx context.Context, pattern string) ([]byte, error) {
	return g.run(ctx, "for-each-ref", "--format=%(refname:short)", pattern)
}

// revParse resolves ref to its object SHA (`git rev-parse <ref>`).
func (g gitRunner) revParse(ctx context.Context, ref string) (string, error) {
	out, err := g.run(ctx, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

// updateRef atomically moves ref to point at sha (`git update-ref <ref> <sha>`).
func (g gitRunner) updateRef(ctx context.Context, ref, sha string) error {
	_, err := g.run(ctx, "update-ref", ref, sha)
	return err
}

// isBareRepo reports whether repoDir is a bare git repository
// (`git rev-parse --is-bare-repository` => "true"). Used by Plan 02's idempotent
// Init to detect an already-initialized store.
func (g gitRunner) isBareRepo(ctx context.Context) bool {
	out, err := g.run(ctx, "rev-parse", "--is-bare-repository")
	if err != nil {
		return false
	}
	return string(bytes.TrimSpace(out)) == "true"
}
