package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zsh-pro/core/model"
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

	beforeStart func([]string)

	// runtimeRoot is non-nil only for a sourced-loader capture. Every git
	// subprocess starts from this descriptor's /dev/fd spelling before exec and
	// inherits it as fd 3 with a relative GIT_DIR. The resulting working
	// directory preserves the authenticated object even if its old pathname is
	// replaced meanwhile.
	runtimeRoot *os.File

	// Candidate runners use a transaction-private index and object directory.
	// The alternate object directory is always the authenticated bare store's
	// object database, never an inherited GIT_* setting.
	privateIndexFile       string
	privateObjectDirectory string
	privateAlternateObject string
}

// ownedEnvironment is the complete Git environment owned by zsh-pro. It starts
// from the process environment only after removing every GIT_* variable: Git
// treats several of those variables as higher precedence than command-line
// repository selection, so carrying one through could redirect a supposedly
// bare-store operation into a caller-controlled worktree or object database.
func (g gitRunner) ownedEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "GIT_") {
			continue
		}
		environment = append(environment, entry)
	}
	gitDir := g.repoDir
	if g.runtimeRoot != nil {
		// command anchors the child in its authenticated descriptor and GIT_DIR
		// stays relative to that descriptor. No source path is re-opened.
		gitDir = "."
	}
	environment = append(environment,
		"GIT_DIR="+gitDir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
	)
	if g.privateIndexFile != "" {
		environment = append(environment, "GIT_INDEX_FILE="+g.privateIndexFile)
	}
	if g.privateObjectDirectory != "" {
		environment = append(environment, "GIT_OBJECT_DIRECTORY="+g.privateObjectDirectory)
	}
	if g.privateAlternateObject != "" {
		environment = append(environment, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+g.privateAlternateObject)
	}
	return environment
}

// withPrivateQuarantine returns a copy of g whose writes are isolated to a
// transaction-owned index and object directory. The source object directory is
// available read-only to Git through a controlled alternate, so candidates can
// read their baseline without mutating the real bare store.
func (g gitRunner) candidate(indexFile, objectDirectory string) gitRunner {
	candidate := g
	candidate.privateIndexFile = indexFile
	candidate.privateObjectDirectory = objectDirectory
	if g.runtimeRoot == nil {
		candidate.privateAlternateObject = filepath.Join(g.repoDir, "objects")
	} else {
		candidate.privateAlternateObject = "objects"
	}
	return candidate
}

func validateGitArgv(args []string) error {
	if len(args) == 0 {
		return ErrGitCommand
	}
	for _, arg := range args {
		// These are global options, not plumbing arguments. Reject them even
		// for an otherwise allow-listed subcommand, before a process can start.
		if arg == "-C" || arg == "-c" || arg == "--git-dir" || arg == "--work-tree" ||
			arg == "--config-env" || strings.HasPrefix(arg, "--git-dir=") ||
			strings.HasPrefix(arg, "--work-tree=") || strings.HasPrefix(arg, "--config-env=") {
			return ErrGitCommand
		}
	}

	validAtom := func(value string) bool {
		return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsRune(value, '\x00')
	}
	allAtoms := func(values []string) bool {
		for _, value := range values {
			if !validAtom(value) {
				return false
			}
		}
		return true
	}

	switch args[0] {
	case "init":
		return validInitArgv(args)
	case "rev-parse":
		if len(args) == 2 && (args[1] == "--is-bare-repository" || validAtom(args[1])) {
			return nil
		}
	case "cat-file":
		if len(args) == 3 && args[1] == "-e" && validAtom(args[2]) {
			return nil
		}
	case "show":
		if len(args) == 2 && validAtom(args[1]) {
			return nil
		}
	case "for-each-ref":
		if len(args) == 3 && strings.HasPrefix(args[1], "--format=") && validAtom(args[2]) {
			return nil
		}
	case "hash-object":
		if len(args) == 3 && args[1] == "-w" && args[2] == "--stdin" {
			return nil
		}
	case "update-ref":
		if len(args) == 4 && args[1] == "-d" && allAtoms(args[2:]) {
			return nil
		}
		if (len(args) == 3 || len(args) == 4) && allAtoms(args[1:]) {
			return nil
		}
	case "read-tree":
		if len(args) == 2 && (args[1] == "--empty" || validAtom(args[1])) {
			return nil
		}
	case "update-index":
		if len(args) == 4 && args[1] == "--add" && args[2] == "--cacheinfo" &&
			strings.HasPrefix(args[3], "100644,") && !strings.ContainsRune(args[3], '\x00') {
			return nil
		}
	case "write-tree":
		if len(args) == 1 {
			return nil
		}
	case "commit-tree":
		if len(args) >= 4 && validAtom(args[1]) {
			for index := 2; index < len(args); {
				switch args[index] {
				case "-m", "-p":
					if index+1 >= len(args) || !validAtom(args[index+1]) {
						return ErrGitCommand
					}
					index += 2
				default:
					return ErrGitCommand
				}
			}
			return nil
		}
	case "ls-tree":
		if len(args) == 5 && args[1] == "-z" && validAtom(args[2]) && args[3] == "--" && args[4] == "profile.json" {
			return nil
		}
	case "show-ref":
		if len(args) == 4 && args[1] == "--verify" && args[2] == "--hash" && validAtom(args[3]) {
			return nil
		}
	}
	return ErrGitCommand
}

func validInitArgv(args []string) error {
	if len(args) != 4 || args[1] != "--bare" || args[2] != "-b" || args[3] != "main" {
		return ErrGitCommand
	}
	return nil
}

// newGitRunner constructs a gitRunner after a one-time exec.LookPath("git") guard,
// mirroring the introspect.go absence check. An absent git binary degrades to
// ErrGitAbsent — never a crash (D-06).
func newGitRunner(dir string) (gitRunner, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return gitRunner{}, ErrGitAbsent
	}
	canonical, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return gitRunner{}, ErrGitCommand
	}
	return gitRunner{repoDir: canonical}, nil
}

// newRuntimeGitRunner anchors all git operations to root rather than a path.
// /dev/fd is available on both supported Unix targets. command starts each git
// child in the current-process descriptor path before exec, then passes the
// same directory through ExtraFiles as fd 3 for the child lifetime.
func newRuntimeGitRunner(root *os.File) (gitRunner, error) {
	if root == nil {
		return gitRunner{}, ErrGitCommand
	}
	if _, err := exec.LookPath("git"); err != nil {
		return gitRunner{}, ErrGitAbsent
	}
	return gitRunner{repoDir: ".", runtimeRoot: root}, nil
}

func (g gitRunner) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if err := validateGitArgv(args); err != nil {
		return nil, err
	}
	if g.runtimeRoot == nil {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = g.ownedEnvironment()
		return cmd, nil
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	// os/exec changes directory before the ExtraFiles descriptors are installed
	// at their child numbers. Use the already-authenticated parent descriptor for
	// that pre-exec chdir, then retain the same object as child fd 3 for Git's
	// lifetime. No mutable source pathname is reopened.
	cmd.Dir = fmt.Sprintf("/dev/fd/%d", g.runtimeRoot.Fd())
	cmd.ExtraFiles = []*os.File{g.runtimeRoot}
	cmd.Env = g.ownedEnvironment()
	return cmd, nil
}

func (g gitRunner) runRaw(ctx context.Context, stdin []byte, args ...string) ([]byte, string, error) {
	cmd, err := g.command(ctx, args...)
	if err != nil {
		return nil, "", err
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if g.beforeStart != nil {
		g.beforeStart(args)
	}
	if err := cmd.Run(); err != nil {
		return nil, errb.String(), err
	}
	return out.Bytes(), errb.String(), nil
}

// run executes `git -C <repoDir> <args...>` with a timeout, returning stdout on
// success and a zsh-pro-phrased mapped error on failure (raw stderr is never
// surfaced — D-11, Pitfall 4). This is the canonical shape lifted from the zsh
// provider's introspect.go subprocess runner.
func (g gitRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	out, errb, err := g.runRaw(ctx, nil, args...)
	if err != nil {
		return nil, mapGitError(err, errb)
	}
	return out, nil
}

// runStdin is run with content piped to git's stdin (needed for
// `hash-object -w --stdin`, which writes a blob from stdin content).
func (g gitRunner) runStdin(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	out, errb, err := g.runRaw(ctx, stdin, args...)
	if err != nil {
		return nil, mapGitError(err, errb)
	}
	return out, nil
}

// runCommit runs git with a deterministic commit environment so commit SHAs are
// byte-stable across runs (Pitfall 2): it pins GIT_DIR, an isolated temp
// GIT_INDEX_FILE (outside any working tree), and fixed author/committer
// name/email/date. ts is the commit timestamp (e.g. "0 +0000" or RFC2822). It is
// used by Plan 02's Commit for the temp-index write-tree -> commit-tree path.
func (g gitRunner) runCommit(ctx context.Context, tmpIndex, ts string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd, err := g.command(ctx, args...)
	if err != nil {
		return nil, mapGitError(err, "")
	}
	cmd.Env = replaceGitEnvironment(cmd.Env,
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
	if g.beforeStart != nil {
		g.beforeStart(args)
	}
	if err := cmd.Run(); err != nil {
		return nil, mapGitError(err, errb.String())
	}
	return out.Bytes(), nil
}

// replaceGitEnvironment replaces owned GIT values rather than appending a
// duplicate assignment. exec accepts duplicates but their resolution varies by
// consumer, which would undermine the transaction-private index guarantee.
func replaceGitEnvironment(environment []string, replacements ...string) []string {
	result := make([]string, 0, len(environment)+len(replacements))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		replaced := false
		for _, replacement := range replacements {
			replacementName, _, _ := strings.Cut(replacement, "=")
			if name == replacementName {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	return append(result, replacements...)
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

// updateRefCAS moves ref only if it still equals old. An all-zero old value
// asserts that the ref remains absent.
func (g gitRunner) updateRefCAS(ctx context.Context, ref, sha, old string) error {
	_, err := g.run(ctx, "update-ref", ref, sha, old)
	return err
}

// deleteRefCAS deletes ref only if it still equals old. It is used to compensate a
// ref update that may have succeeded even when the caller observed an error.
func (g gitRunner) deleteRefCAS(ctx context.Context, ref, old string) error {
	_, err := g.run(ctx, "update-ref", "-d", ref, old)
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

// observeRef distinguishes a genuinely absent ref from a failed Git probe.
// for-each-ref exits successfully with empty output for absence, while repository
// or subprocess failures stay operational errors; this avoids conflating the
// implementation-dependent show-ref missing-ref exit with a corrupt repository.
func (g gitRunner) observeRef(ctx context.Context, ref string) (string, bool, error) {
	out, err := g.run(ctx, "for-each-ref", "--format=%(refname)%00%(objectname)", ref)
	if err != nil {
		return "", false, err
	}
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte{'\n'}) {
		name, value, ok := bytes.Cut(line, []byte{0})
		if !ok || string(name) != ref {
			continue
		}
		objectID := string(bytes.TrimSpace(value))
		if objectID == "" || strings.ContainsAny(objectID, "\r\n") {
			return "", false, ErrGitCommand
		}
		return objectID, true, nil
	}
	return "", false, nil
}

// profileAtRevision reads profile.json from one exact commit. ls-tree establishes
// whether the object is present before show reads it, so an initialized empty
// baseline remains distinguishable from a committed empty profile.
func (g gitRunner) profileAtRevision(ctx context.Context, revision string) (model.Profile, bool, error) {
	tree, err := g.run(ctx, "ls-tree", "-z", revision, "--", "profile.json")
	if err != nil {
		return model.Profile{}, false, err
	}
	if len(tree) == 0 {
		return model.Profile{}, false, nil
	}
	// Git's -z form is `<mode> <type> <oid>\tpath\x00`; do not accept an
	// unexpected path merely because a future call-site changed its argv.
	if !bytes.HasSuffix(tree, []byte("\tprofile.json\x00")) {
		return model.Profile{}, false, ErrGitCommand
	}
	payload, err := g.show(ctx, revision+":profile.json")
	if err != nil {
		return model.Profile{}, false, err
	}
	profile, err := UnmarshalProfile(payload)
	if err != nil {
		return model.Profile{}, false, err
	}
	return profile, true, nil
}
