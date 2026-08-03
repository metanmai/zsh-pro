// Package store is the git-backed persistence layer for the Phase 2 model.Profile
// IR, where each git branch is an environment profile. It drives the git binary as
// a subprocess (mirroring the zsh -f introspection pattern; no go-git, no new
// dependency) over a BARE repository, so there is no working tree and no
// checked-out HEAD to contend over: Commit writes to a target branch purely via
// plumbing (temp index -> write-tree -> commit-tree -> update-ref) and Read pulls
// from the object DB via `git show` — never a checkout (D-11/D-12, the conda
// concurrent-activation race avoided by construction).
//
// The package is shell-agnostic: it must NEVER import the concrete zsh provider
// package. That import is reserved for the composition root. Shell-specific .zsh
// regeneration is injected via the shell.Regenerator seam (D-03) and reached
// through the agnostic ir.Regenerate profile-level emitter.
package store

import (
	"context"
	"os"
	"strings"
	"sync"

	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// commitTS is the fixed commit timestamp ("<epoch> <tz>") the store stamps on every
// commit via runCommit's GIT_AUTHOR_DATE/GIT_COMMITTER_DATE (Pitfall 2). A constant
// date makes the `main` baseline and every profile snapshot byte-reproducible
// across machines and runs, so history-sensitive tests assert on content rather
// than racing wall-clock SHAs (T-03-05). Authorship identity is pinned in runCommit.
const commitTS = "1700000000 +0000"

// KeychainDriver is the keychain seam — Store/Retrieve/Delete move a name-scoped
// secret value to/from a backend; Kind() reports which backend this driver is (so
// Plan 03's literal-secret exclusion stamps the matching SecretRef.Kind on the
// committed pointer). Concrete drivers (the macOS `security` / Linux `secret-tool`
// subprocess backends and the 0600 git-ignored vault fallback) and the deref use
// of Retrieve land in Plan 03 / Ph4-5; this plan only depends on the contract.
//
// It is declared HERE, at first use, because the Store field and the New signature
// below reference it (define contracts no later than first use). It is the FINAL
// form: Plan 03 wires a concrete implementation behind it without changing this
// interface.
type KeychainDriver interface {
	Store(key, value string) error
	Retrieve(key string) (string, error)
	Delete(key string) error
	Kind() model.SecretRefKind // which backend this driver is -> SecretRef.Kind in Plan 03
}

// WithheldSecret names one literal secret excluded from a committed tree: the var
// name and its 1-based source line, so the CLI can tell the user exactly what was
// withheld. It is zero-value-usable. Plan 03 populates it as Commit excludes
// literal secrets; this plan only declares the shape.
type WithheldSecret = model.WithheldSecret

// WithheldReport is the producer/consumer contract for secret exclusion: Phase 3
// PRODUCES this report on Commit (Plan 03 populates it as it captures literal
// secrets into the keychain backend and replaces them with a SecretRef before the
// tree is written); the CLI CONSUMES it in Phase 5 to tell the user what was
// withheld (success-criterion #4, "told what was withheld"). A nil/empty report
// means nothing was withheld. It is declared HERE because Commit's return type
// references it (define contracts no later than first use); the type is FINAL —
// Plan 03 changes neither it nor Commit's signature, only the body that fills it.
type WithheldReport = model.WithheldReport

// Store is the git-backed profile store. It holds exactly four injected
// dependencies and no global state, mirroring the project's injected-driver
// constructor pattern (core/cli.CLI, core/analyze.Analyzer). The field set is
// FINAL: Plan 03 fills in the keychain backend behind the existing field, it does
// NOT add or remove a field here.
type Store struct {
	dir      string            // path to the bare git repo ($ZSHPRO_HOME / XDG default)
	git      gitRunner         // git-binary subprocess driver (Plan 01)
	regen    shell.Regenerator // injected per-entry zsh emitter (D-03; never the concrete zsh provider)
	keychain KeychainDriver    // secret backend seam (concrete impls land in Plan 03)

	// Per-instance test seams for ambiguous ref effects. Production leaves these nil
	// and calls gitRunner directly; a test can inject one Store without leaking state
	// to later tests or another concurrent Store.
	refRead   func(context.Context, string) (string, error)
	refUpdate func(context.Context, string, string, string) error
	refDelete func(context.Context, string, string) error

	transactionMu          sync.Mutex
	storeNonce             model.InstallInitializationID
	installInitializations map[model.InstallInitializationID]*installInitializationRecord
	ingestTransactions     map[model.IngestTransactionID]*ingestTransactionRecord

	// beginAfterReserve is a per-Store test seam used to prove that authority is
	// reserved before any baseline or filesystem I/O. Production leaves it nil.
	beginAfterReserve          func() error
	beginObserveMain           func(context.Context) (string, bool, error)
	beginAfterQuarantineCreate func(string) error
	beginAfterQuarantineOpen   func(string) error
	beginAfterQuarantineStat   func(string) error
	cleanupBeforeFinalCheck    func(quarantineCleanupSeam) error
	cleanupAfterFinalCheck     func(quarantineCleanupSeam) error
	cleanupDiscardLock         bool
}

// New constructs a Store after a one-time git-presence guard (an absent git binary
// degrades to ErrGitAbsent — never a crash, D-06). It takes the keychain driver in
// its FINAL form so Plan 03 wires the concrete backend without touching this
// signature or any caller; tests in this plan pass a nil KeychainDriver because
// literal-secret exclusion is wired in Plan 03 (Commit returns an empty report
// until then).
func New(dir string, regen shell.Regenerator, kc KeychainDriver) (*Store, error) {
	g, err := newGitRunner(dir) // LookPath("git") guard -> ErrGitAbsent on absence
	if err != nil {
		return nil, err
	}
	nonce, err := model.NewInstallInitializationID()
	if err != nil {
		return nil, err
	}
	return &Store{
		dir:                    dir,
		git:                    g,
		regen:                  regen,
		keychain:               kc,
		storeNonce:             nonce,
		installInitializations: make(map[model.InstallInitializationID]*installInitializationRecord),
		ingestTransactions:     make(map[model.IngestTransactionID]*ingestTransactionRecord),
	}, nil
}

// NewRuntime constructs the read path used by the sourced runtime helper. The
// root and vaultParent descriptors were authenticated by core/cli and remain
// owned by its RuntimeRoot for the duration of one emission. Git inherits root
// as a child descriptor; the fallback vault is opened relative to vaultParent
// with O_NOFOLLOW. No path spelling from $ZSHPRO_HOME is reused here.
func NewRuntime(root, vaultParent *os.File, regen shell.Regenerator) (*Store, error) {
	g, err := newRuntimeGitRunner(root)
	if err != nil {
		return nil, err
	}
	nonce, err := model.NewInstallInitializationID()
	if err != nil {
		return nil, err
	}
	return &Store{
		git:                    g,
		regen:                  regen,
		keychain:               newRuntimeKeychain(vaultParent),
		storeNonce:             nonce,
		installInitializations: make(map[model.InstallInitializationID]*installInitializationRecord),
		ingestTransactions:     make(map[model.IngestTransactionID]*ingestTransactionRecord),
	}, nil
}

// RuntimeSecretResolver exposes the already-bound runtime resolver to the
// composition root's narrow CLI seam. Store retains ownership of the driver.
func (s *Store) RuntimeSecretResolver() KeychainDriver {
	if s == nil {
		return nil
	}
	return s.keychain
}

// Init initializes the bare profile store and is idempotent (D-05/D-06): re-running
// it on an existing store is a safe no-op that never clobbers the repo. If the dir
// is already a bare repo, Init returns nil immediately. Otherwise it creates the
// directory and runs `git init --bare -b main`, then — if the `main` baseline ref
// is absent — seeds it with an empty-tree root commit so `main` exists as the clean
// common ancestor every profile forks from. It NEVER calls a working-tree verb
// (status/checkout/reset): a bare repo has no working tree (Pitfall 1).
func (s *Store) Init(ctx context.Context) error {
	// Runtime capture later accepts only a private, current-user store root.
	// Establish (or safely migrate) that invariant before asking git whether an
	// existing directory is bare; otherwise an old 0755 repository returns
	// early here and every sourced runtime command rejects it.
	if err := ensurePrivateStoreDir(s.dir); err != nil {
		return err
	}
	if s.git.isBareRepo(ctx) {
		return nil // already initialized — idempotent no-op, never clobber (D-06)
	}
	if _, err := s.git.run(ctx, "init", "--bare", "-b", "main"); err != nil {
		return err
	}
	// Seed the `main` baseline if it does not yet exist (a fresh --bare repo has no
	// refs). A root commit (no -p) over the empty tree makes `main` a valid branch
	// to fork from without staging any blob.
	if !s.git.catFileExists(ctx, "refs/heads/main") {
		commit, err := s.git.runCommit(ctx, "", commitTS, "commit-tree", emptyTreeSHA, "-m", "init")
		if err != nil {
			return err
		}
		sha := strings.TrimSpace(string(commit))
		if err := s.git.updateRef(ctx, "refs/heads/main", sha); err != nil {
			return err
		}
	}
	return nil
}

// Branches lists the profiles (= branches) as a sorted slice of short names
// (PROF-02 list). It reads `refs/heads/` via for-each-ref from the object DB; no
// checkout.
func (s *Store) Branches(ctx context.Context) ([]string, error) {
	out, err := s.git.forEachRef(ctx, "refs/heads/")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	sortStrings(names)
	return names, nil
}

// Current returns the active profile for THIS terminal, read from the
// ZSHPRO_PROFILE env var carrying the profile name only (D-13). An unset (or empty)
// var means the `main` baseline implicitly. The env var EXPORT on switch is Phase 5
// (the sourced loader); this plan only reads it.
func (s *Store) Current() string {
	if name := os.Getenv("ZSHPRO_PROFILE"); name != "" {
		return name
	}
	return "main"
}

// Checkout validates that a profile is switchable: it checks the name is well-formed
// and that the branch exists (else ErrProfileNotFound). It does NOT export
// ZSHPRO_PROFILE — the env-var export on switch is Phase 5 (the sourced loader
// honors the per-terminal contract); Checkout only validates the target is a real,
// switchable branch from the object DB (no checkout, no working tree).
func (s *Store) Checkout(ctx context.Context, name string) error {
	if err := validBranchName(name); err != nil {
		return err
	}
	if !s.git.catFileExists(ctx, "refs/heads/"+name) {
		return ErrProfileNotFound
	}
	return nil
}

// Create makes a new profile by forking the `main` baseline (D-05): every profile
// shares the clean common ancestor. It validates the name, fails with
// ErrProfileExists if the branch already exists, then points refs/heads/<name> at
// main's current tip via update-ref (PROF-02 create). No checkout.
func (s *Store) Create(ctx context.Context, name string) error {
	if err := validBranchName(name); err != nil {
		return err
	}
	if s.git.catFileExists(ctx, "refs/heads/"+name) {
		return ErrProfileExists
	}
	mainSHA, err := s.git.revParse(ctx, "refs/heads/main")
	if err != nil {
		return err
	}
	return s.git.updateRef(ctx, "refs/heads/"+name, mainSHA)
}

// Commit writes a profile to a target branch purely via plumbing — no checkout,
// no working-tree mutation anywhere (D-12, critical decision #4; the conda race
// avoided by construction). It stages exactly two blobs: profile.json (the
// authoritative lossless IR serialization, D-01) and profile.zsh (a derived
// source-ordered view generated through the injected regenerator seam, D-02/D-03).
// The recipe is the Plan-01-proven Pattern 1 (exercised end-to-end by
// TestGitCommitToBranch): hash-object each blob, seed a temp index from the branch
// tree (parented) or empty (root commit), update-index --cacheinfo both paths,
// write-tree, commit-tree, update-ref. Every git call goes through gitRunner so an
// error is zsh-pro-phrased, never raw (D-11).
//
// Commit has its FINAL two-value signature (WithheldReport, error). Secret
// exclusion is ACTIVE (D-07–D-10): as the FIRST step, excludeSecrets rewrites the
// profile so every literal secret (Category==CatSecrets && !Dynamic && Value!="")
// is captured into the injected keychain/vault backend and replaced by a SecretRef
// (Kind=kc.Kind()) with its literal cleared, while already-dynamic secrets commit
// verbatim (D-08). The post-exclusion profile is what gets marshaled, regenerated,
// and committed — the literal value never enters the tree — and the populated
// WithheldReport names what was withheld (success-criterion #4, surfaced by the CLI
// in Phase 5). The signature is unchanged from Plan 02.
func (s *Store) Commit(ctx context.Context, branch string, p model.Profile, msg string) (WithheldReport, error) {
	if err := validBranchName(branch); err != nil {
		return nil, err
	}

	// Exclude literal secrets BEFORE anything is serialized: capture each into the
	// backend, replace with a SecretRef, clear the literal. Everything downstream
	// (marshal, regenerate, hash, commit) operates on `excluded`, NOT the caller's `p`,
	// so the literal never reaches the committed tree (T-03-03). A backend/nil-driver
	// failure aborts the Commit before any ref moves.
	prepared, err := prepareSecrets(p, s.keychain)
	if err != nil {
		return nil, err
	}
	excluded, report := prepared.profile, prepared.report

	// profile.json is authoritative (D-01); profile.zsh is the derived view emitted
	// via the injected seam (D-02/D-03) — values pass through verbatim, never resolved.
	jsonBytes, err := MarshalProfile(excluded)
	if err != nil {
		return nil, err
	}
	zshBytes := ir.Regenerate(excluded, s.regen)

	blobJSON, err := s.git.hashObject(ctx, jsonBytes)
	if err != nil {
		return nil, err
	}
	blobZSH, err := s.git.hashObject(ctx, zshBytes)
	if err != nil {
		return nil, err
	}

	// Temp index file OUTSIDE any working tree (a bare repo has none anyway), cleaned
	// up after. GIT_INDEX_FILE is set by runCommit so all index ops target it.
	idxFile, err := os.CreateTemp("", "zshpro-index-*")
	if err != nil {
		return nil, err
	}
	idx := idxFile.Name()
	// Only the name is needed; git owns the file content via GIT_INDEX_FILE. The
	// deferred remove is best-effort — a leftover temp index is harmless.
	_ = idxFile.Close()
	defer func() { _ = os.Remove(idx) }()

	ref := "refs/heads/" + branch
	var parent string
	if s.git.catFileExists(ctx, ref) {
		// Seed the temp index from the branch's current tree and parent on its tip.
		if _, err := s.git.runCommit(ctx, idx, commitTS, "read-tree", branch); err != nil {
			return nil, err
		}
		parent, err = s.git.revParse(ctx, ref)
		if err != nil {
			return nil, err
		}
	} else if _, err := s.git.runCommit(ctx, idx, commitTS, "read-tree", "--empty"); err != nil {
		// Brand-new branch: start from an empty index and make a root commit (no -p).
		return nil, err
	}

	// Stage exactly profile.json + profile.zsh via cacheinfo (mode,sha,path) — never
	// any other path, so the git-ignored vault file can never enter the tree.
	if _, err := s.git.runCommit(ctx, idx, commitTS, "update-index", "--add", "--cacheinfo", "100644,"+blobJSON+",profile.json"); err != nil {
		return nil, err
	}
	if _, err := s.git.runCommit(ctx, idx, commitTS, "update-index", "--add", "--cacheinfo", "100644,"+blobZSH+",profile.zsh"); err != nil {
		return nil, err
	}

	treeOut, err := s.git.runCommit(ctx, idx, commitTS, "write-tree")
	if err != nil {
		return nil, err
	}
	tree := strings.TrimSpace(string(treeOut))

	// commit-tree: parented if the branch existed, else a root commit. The message is
	// passed as distinct argv (-m) — the Plan-01-proven form (TestGitCommitToBranch);
	// it is non-sensitive fixed/user text and is never shell-interpolated.
	commitArgs := []string{"commit-tree", tree, "-m", msg}
	if parent != "" {
		commitArgs = []string{"commit-tree", tree, "-p", parent, "-m", msg}
	}
	commitOut, err := s.git.runCommit(ctx, idx, commitTS, commitArgs...)
	if err != nil {
		return nil, err
	}
	commit := strings.TrimSpace(string(commitOut))

	type priorSecret struct {
		key, value string
		exists     bool
	}
	priors := make([]priorSecret, 0, len(prepared.pending))
	for _, mutation := range prepared.pending {
		value, retrieveErr := s.keychain.Retrieve(mutation.key)
		if retrieveErr != nil && retrieveErr != ErrSecretNotFound {
			return nil, retrieveErr
		}
		priors = append(priors, priorSecret{key: mutation.key, value: value, exists: retrieveErr == nil})
	}
	rollback := func() error {
		failed := false
		for i := len(priors) - 1; i >= 0; i-- {
			prior := priors[i]
			var restoreErr error
			if prior.exists {
				restoreErr = s.keychain.Store(prior.key, prior.value)
			} else {
				restoreErr = s.keychain.Delete(prior.key)
				if restoreErr == ErrSecretNotFound {
					restoreErr = nil
				}
			}
			if restoreErr != nil {
				failed = true
			}
		}
		if failed {
			return ErrSecretRollback
		}
		return nil
	}
	for _, mutation := range prepared.pending {
		if err := s.keychain.Store(mutation.key, mutation.value); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return nil, rollbackErr
			}
			return nil, err
		}
	}
	old := parent
	if old == "" {
		old = "0000000000000000000000000000000000000000"
	}
	if err := s.updateRefCAS(ctx, ref, commit, old); err != nil {
		// update-ref can be ambiguous from this process's perspective (for example,
		// a timeout after git has moved the ref). Re-read the ref before restoring
		// secrets: compensate only if it points to OUR commit, and use CAS again so
		// another writer can never be clobbered.
		result := err
		if current, readErr := s.readRef(ctx, ref); readErr == nil {
			switch {
			case current == commit:
				var compensateErr error
				if parent == "" {
					compensateErr = s.deleteRefCAS(ctx, ref, commit)
				} else {
					compensateErr = s.updateRefCAS(ctx, ref, parent, commit)
				}
				if compensateErr != nil {
					result = ErrSecretRollback
				}
			case current != parent:
				// The ref changed to a third value. Do not compensate it; callers
				// receive a typed conflict instead of an accidental overwrite.
				result = ErrSecretRefConflict
			}
		}
		if rollbackErr := rollback(); rollbackErr != nil {
			return nil, rollbackErr
		}
		return nil, result
	}
	return report, nil // names the literal secrets excluded above (nil/empty when none)
}

func (s *Store) readRef(ctx context.Context, ref string) (string, error) {
	if s.refRead != nil {
		return s.refRead(ctx, ref)
	}
	return s.git.revParse(ctx, ref)
}

func (s *Store) updateRefCAS(ctx context.Context, ref, sha, old string) error {
	if s.refUpdate != nil {
		return s.refUpdate(ctx, ref, sha, old)
	}
	return s.git.updateRefCAS(ctx, ref, sha, old)
}

func (s *Store) deleteRefCAS(ctx context.Context, ref, old string) error {
	if s.refDelete != nil {
		return s.refDelete(ctx, ref, old)
	}
	return s.git.deleteRefCAS(ctx, ref, old)
}

// Read reconstructs a model.Profile from a branch's profile.json, pulled straight
// from the object DB via `git show <branch>:profile.json` — no checkout, no working
// tree (D-12). It reads from the object DB so a read never disturbs another
// terminal's HEAD; the bytes are decoded by UnmarshalProfile — no re-parse, so the
// store stays shell-agnostic.
//
// CONTRACT (WR-03) — Read distinguishes "branch absent" from "branch present but not
// yet committed to", because Branches/Checkout treat a never-committed branch as a
// real, switchable profile and the Phase 5 verb caller must be able to read it:
//   - branch does NOT exist (no refs/heads/<branch>): ErrProfileNotFound. This is the
//     genuinely-absent case (e.g. `checkout typo`), the zsh-pro-phrased error the CLI
//     surfaces instead of a raw git `fatal: path ... does not exist` (T-03-01).
//   - branch EXISTS but has no profile.json at its tip: the EMPTY profile
//     model.Profile{} (no error). This is the fresh `main` baseline (an empty-tree
//     root commit from Init) and a freshly Create'd fork before its first Commit —
//     both are valid, readable profiles that simply hold no entries yet.
//   - branch exists WITH a profile.json: decode and return it (the committed case,
//     including a profile committed as empty, which serializes a profile.json blob).
func (s *Store) Read(ctx context.Context, branch string) (model.Profile, error) {
	if err := validBranchName(branch); err != nil {
		return model.Profile{}, err
	}
	// Probe the BRANCH first: a missing ref is a genuinely-absent profile.
	if !s.git.catFileExists(ctx, "refs/heads/"+branch) {
		return model.Profile{}, ErrProfileNotFound
	}
	// The branch exists. A missing profile.json at its tip means "present but never
	// committed to" — return the empty profile, NOT ErrProfileNotFound (WR-03).
	objRef := branch + ":profile.json"
	if !s.git.catFileExists(ctx, objRef) {
		return model.Profile{}, nil
	}
	b, err := s.git.show(ctx, objRef)
	if err != nil {
		return model.Profile{}, err
	}
	return UnmarshalProfile(b)
}

// validBranchName rejects names that are unsafe to interpolate into git argv or a
// ref path (ASVS V5; threat T-03-02). It rejects: the empty name; any name starting
// with '-' (argument-injection guard — a leading dash could be read as a git flag);
// any rune outside the charset [A-Za-z0-9._/-]; AND — because that charset alone
// still admits `foo/../bar` — any name with a '/'-delimited component equal to ".."
// or "." (the precise path-traversal guard). A blanket strings.Contains(name, "..")
// is deliberately NOT used: `v1.0` is a legitimate name, so the check is per
// '/'-delimited segment. Refs are constructed only as refs/heads/<validated> and
// names are always passed as distinct argv (never a shell string).
func validBranchName(name string) error {
	if name == "" {
		return ErrInvalidProfileName
	}
	if strings.HasPrefix(name, "-") {
		return ErrInvalidProfileName
	}
	for _, r := range name {
		if !isBranchRune(r) {
			return ErrInvalidProfileName
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." || seg == "." {
			return ErrInvalidProfileName
		}
	}
	return nil
}

// isBranchRune reports whether r is in the safe branch-name charset [A-Za-z0-9._/-].
func isBranchRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '/' || r == '-':
		return true
	default:
		return false
	}
}

// sortStrings sorts in place (insertion sort — the branch list is tiny and this
// avoids pulling sort into the package's import set for one call site).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
