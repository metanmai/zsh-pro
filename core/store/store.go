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
	"compress/zlib"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

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

const (
	// ErrIngestNotCommitted is the stable legacy-compatible classification for
	// a transaction whose ref is known not to have published.
	ErrIngestNotCommitted errStore = "zsh-pro: profile ingest was not committed"
	// ErrIngestRecoveryRequired reports retained evidence or an effect whose
	// state cannot be safely compensated without operator recovery.
	ErrIngestRecoveryRequired errStore = "zsh-pro: profile ingest requires recovery"
)

type ingestRecoveryError struct {
	committed bool
}

func (err ingestRecoveryError) Error() string { return ErrIngestRecoveryRequired.Error() }

func (err ingestRecoveryError) Is(target error) bool { return target == ErrIngestRecoveryRequired }

// Committed reports publication truth independently from the recovery error.
func (err ingestRecoveryError) Committed() bool { return err.committed }

type ingestNotCommittedError struct {
	cause error
}

func (err ingestNotCommittedError) Error() string { return ErrIngestNotCommitted.Error() }

func (err ingestNotCommittedError) Is(target error) bool {
	return target == ErrIngestNotCommitted || errors.Is(err.cause, target)
}

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

	// CommitIngest terminal outcomes are immutable and replayable by token.
	ingestCommitOutcomes  map[model.IngestTransactionID]ingestCommitTerminal
	ingestTransactionRefs map[model.IngestTransactionID]validatedHeadRef
	legacyInitialization  model.InstallInitializationID

	// Per-Store test seams for deterministic publication/durability failures and
	// value-free event ordering. Production leaves these nil.
	commitPublishLink func(string, string) error
	commitSyncDir     func(string) error
	commitEvent       func(string)
	commitRefSession  func(*updateRefSession) error
}

type ingestCommitTerminal struct {
	outcome model.IngestCommitOutcome
	err     error
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
	return s.git.updateRefCAS(ctx, "refs/heads/"+name, mainSHA, strings.Repeat("0", len(mainSHA)))
}

type claimedIngestCommit struct {
	record     *ingestTransactionRecord
	baseline   model.IngestBaseline
	quarantine *ingestQuarantine
	ref        validatedHeadRef
}

func (s *Store) claimIngestCommit(
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
) (claimedIngestCommit, *ingestCommitTerminal, error) {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	initialization, validInitialization := s.validInstallInitializationLocked(initializationID)
	record, ok := s.ingestTransactions[transactionID]
	if !validInitialization || !ok || record == nil ||
		record.initializationID != initializationID || record.transactionID != transactionID ||
		record.storeNonce != s.storeNonce {
		return claimedIngestCommit{}, nil, ErrInvalidIngestAuthority
	}
	if terminal, terminalOK := s.ingestCommitOutcomes[transactionID]; terminalOK &&
		record.lifecycle == model.IngestLifecycleTerminal {
		copy := terminal
		return claimedIngestCommit{}, &copy, nil
	}
	if initialization.closing || initialization.terminal || record.lifecycle != model.IngestLifecycleActive ||
		record.quarantine == nil {
		return claimedIngestCommit{}, nil, ErrInvalidIngestAuthority
	}
	if s.ingestCommitOutcomes == nil {
		s.ingestCommitOutcomes = make(map[model.IngestTransactionID]ingestCommitTerminal)
	}
	record.lifecycle = model.IngestLifecycleCommitting
	baseline := record.baseline
	if baseline.ExpectedRevision != nil {
		expected := *baseline.ExpectedRevision
		baseline.ExpectedRevision = &expected
	}
	ref := mainHeadRef()
	if transactionRef, ok := s.ingestTransactionRefs[transactionID]; ok {
		ref = transactionRef
	}
	return claimedIngestCommit{
		record:     record,
		baseline:   baseline,
		quarantine: record.quarantine,
		ref:        ref,
	}, nil, nil
}

func invalidAuthorityCommitOutcome(
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
) model.IngestCommitOutcome {
	return model.IngestCommitOutcome{
		InitializationID: initializationID,
		TransactionID:    transactionID,
		Status:           model.IngestCommitNotCommitted,
		RefState:         model.IngestRefUnknown,
		Backend:          model.IngestBackendUnchanged,
		Objects:          model.IngestObjectsQuarantined,
		Cleanup:          model.QuarantineCleanupRetained,
		FailureCode:      model.IngestFailureInvalidAuthority,
		RecoveryRequired: false,
	}
}

func (s *Store) terminalizeIngestCommit(
	claim claimedIngestCommit,
	outcome model.IngestCommitOutcome,
	commitErr error,
) (model.IngestCommitOutcome, error) {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record := claim.record
	current, ok := s.ingestTransactions[record.transactionID]
	if !ok || current != record || record.storeNonce != s.storeNonce {
		outcome.RecoveryRequired = true
		outcome.Status = model.IngestCommitRecoveryRequired
		outcome.FailureCode = model.IngestFailureInvalidAuthority
		return outcome, ErrIngestRecoveryRequired
	}
	record.lifecycle = model.IngestLifecycleTerminal
	if outcome.Status == model.IngestCommitCommitted {
		record.commitStatus = model.IngestCommitCommitted
	} else {
		// The initializer rollback predicate treats every clean non-publication
		// result alike. Conflict remains visible in the immutable public outcome.
		record.commitStatus = model.IngestCommitNotCommitted
	}
	record.cleanup = outcome.Cleanup
	record.recoveryRequired = outcome.RecoveryRequired
	initialization := s.installInitializations[record.initializationID]
	outcome.InitializerRollbackSafe = initialization != nil && s.initializerRollbackSafeLocked(initialization)
	terminal := ingestCommitTerminal{outcome: outcome, err: commitErr}
	if s.ingestCommitOutcomes == nil {
		s.ingestCommitOutcomes = make(map[model.IngestTransactionID]ingestCommitTerminal)
	}
	s.ingestCommitOutcomes[record.transactionID] = terminal
	return outcome, commitErr
}

func (s *Store) emitCommitEvent(event string) {
	if s.commitEvent != nil {
		s.commitEvent(event)
	}
}

func (s *Store) authenticateClaimedQuarantine(
	guard *storeRootTransactionGuard,
	claim claimedIngestCommit,
	fn func() error,
) error {
	return guard.withAuthenticatedMutation(func(namespace *os.Root) error {
		current, err := namespace.Lstat(claim.quarantine.basename)
		if err != nil || !sameQuarantineEntry(claim.quarantine.info, current) {
			return errQuarantineIdentityChanged
		}
		root, err := namespace.OpenRoot(claim.quarantine.basename)
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		opened, err := root.Stat(".")
		if err != nil || !sameQuarantineEntry(claim.quarantine.info, opened) {
			return errQuarantineIdentityChanged
		}
		for _, relative := range []string{"index", "objects"} {
			info, statErr := root.Lstat(relative)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				return errQuarantineIdentityChanged
			}
		}
		return fn()
	})
}

func (s *Store) prepareIngestCandidate(
	ctx context.Context,
	claim claimedIngestCommit,
	profile model.Profile,
	message string,
) (string, error) {
	jsonBytes, err := MarshalProfile(profile)
	if err != nil {
		return "", err
	}
	zshBytes := ir.Regenerate(profile, s.regen)
	candidate := s.git.candidate(claim.quarantine.indexPath, claim.quarantine.objectsPath)
	blobJSON, err := candidate.hashObject(ctx, jsonBytes)
	if err != nil {
		return "", err
	}
	blobZSH, err := candidate.hashObject(ctx, zshBytes)
	if err != nil {
		return "", err
	}
	if _, err := candidate.runCommit(ctx, claim.quarantine.indexPath, commitTS,
		"update-index", "--add", "--cacheinfo", "100644,"+blobJSON+",profile.json"); err != nil {
		return "", err
	}
	if _, err := candidate.runCommit(ctx, claim.quarantine.indexPath, commitTS,
		"update-index", "--add", "--cacheinfo", "100644,"+blobZSH+",profile.zsh"); err != nil {
		return "", err
	}
	treeOut, err := candidate.runCommit(ctx, claim.quarantine.indexPath, commitTS, "write-tree")
	if err != nil {
		return "", err
	}
	tree := strings.TrimSpace(string(treeOut))
	args := []string{"commit-tree", tree, "-m", message}
	if claim.baseline.ExpectedRevision != nil {
		args = []string{"commit-tree", tree, "-p", *claim.baseline.ExpectedRevision, "-m", message}
	}
	commitOut, err := candidate.runCommit(ctx, claim.quarantine.indexPath, commitTS, args...)
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(commitOut))
	if !validGitObjectID(commit) {
		return "", ErrGitCommand
	}
	return commit, nil
}

type secretPrior struct {
	key    string
	value  string
	exists bool
}

func snapshotSecretPriors(prepared preparedSecrets, keychain KeychainDriver) ([]secretPrior, error) {
	priors := make([]secretPrior, 0, len(prepared.pending))
	for _, mutation := range prepared.pending {
		value, err := keychain.Retrieve(mutation.key)
		if err != nil && !errors.Is(err, ErrSecretNotFound) {
			return nil, err
		}
		priors = append(priors, secretPrior{key: mutation.key, value: value, exists: err == nil})
	}
	return priors, nil
}

func restoreSecretPriors(keychain KeychainDriver, priors []secretPrior) error {
	failed := false
	for index := len(priors) - 1; index >= 0; index-- {
		prior := priors[index]
		var err error
		if prior.exists {
			err = keychain.Store(prior.key, prior.value)
		} else {
			err = keychain.Delete(prior.key)
			if errors.Is(err, ErrSecretNotFound) {
				err = nil
			}
		}
		if err != nil {
			failed = true
		}
	}
	if failed {
		return ErrSecretRollback
	}
	return nil
}

func applyPreparedSecrets(prepared preparedSecrets, keychain KeychainDriver) ([]secretPrior, error) {
	if len(prepared.pending) == 0 {
		return nil, nil
	}
	priors, err := snapshotSecretPriors(prepared, keychain)
	if err != nil {
		return nil, err
	}
	for _, mutation := range prepared.pending {
		if err := keychain.Store(mutation.key, mutation.value); err != nil {
			if rollbackErr := restoreSecretPriors(keychain, priors); rollbackErr != nil {
				return priors, rollbackErr
			}
			return priors, err
		}
	}
	return priors, nil
}

type objectPublication struct {
	created   int
	uncertain bool
	dirs      map[string]struct{}
}

func (s *Store) linkCandidateObject(source, destination string) error {
	if s.commitPublishLink != nil {
		return s.commitPublishLink(source, destination)
	}
	return os.Link(source, destination)
}

func (s *Store) syncObjectDirectory(path string) error {
	if s.commitSyncDir != nil {
		return s.commitSyncDir(path)
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func verifyLooseObject(path, objectID string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrGitCommand
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return ErrGitCommand
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrGitCommand
	}
	reader, err := zlib.NewReader(file)
	if err != nil {
		return ErrGitCommand
	}
	defer func() { _ = reader.Close() }()
	var digest []byte
	switch len(objectID) {
	case 40:
		hash := sha1.New() //nolint:gosec // SHA-1 is Git's repository object format, not a security choice.
		if _, err := io.Copy(hash, reader); err != nil {
			return ErrGitCommand
		}
		digest = hash.Sum(nil)
	case 64:
		hash := sha256.New()
		if _, err := io.Copy(hash, reader); err != nil {
			return ErrGitCommand
		}
		digest = hash.Sum(nil)
	default:
		return ErrGitCommand
	}
	if !strings.EqualFold(hex.EncodeToString(digest), objectID) {
		return ErrGitCommand
	}
	return nil
}

func (s *Store) publishCandidateObjects(claim claimedIngestCommit) (objectPublication, error) {
	publication := objectPublication{dirs: make(map[string]struct{})}
	objectRoot := filepath.Join(s.git.repoDir, "objects")
	err := filepath.WalkDir(claim.quarantine.objectsPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == claim.quarantine.objectsPath || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(claim.quarantine.objectsPath, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 || len(parts[0]) != 2 || (len(parts[1]) != 38 && len(parts[1]) != 62) {
			return ErrGitCommand
		}
		objectID := parts[0] + parts[1]
		if !validGitObjectID(objectID) {
			return ErrGitCommand
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return ErrGitCommand
		}
		if err := verifyLooseObject(path, objectID); err != nil {
			return err
		}
		fanout := filepath.Join(objectRoot, parts[0])
		if err := os.Mkdir(fanout, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		fanoutInfo, err := os.Lstat(fanout)
		if err != nil || !fanoutInfo.IsDir() || fanoutInfo.Mode()&os.ModeSymlink != 0 {
			return ErrGitCommand
		}
		destination := filepath.Join(fanout, parts[1])
		if err := s.linkCandidateObject(path, destination); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			return verifyLooseObject(destination, objectID)
		}
		publication.created++
		publication.dirs[fanout] = struct{}{}
		return nil
	})
	if err != nil {
		return publication, err
	}
	s.emitCommitEvent("object-publication")
	directories := make([]string, 0, len(publication.dirs))
	for directory := range publication.dirs {
		directories = append(directories, directory)
	}
	sortStrings(directories)
	for _, directory := range directories {
		if err := s.syncObjectDirectory(directory); err != nil {
			publication.uncertain = true
			return publication, err
		}
		s.emitCommitEvent("fanout-fsync")
	}
	if err := s.syncObjectDirectory(objectRoot); err != nil {
		publication.uncertain = true
		return publication, err
	}
	s.emitCommitEvent("object-root-fsync")
	return publication, nil
}

func defaultCommitOutcome(claim claimedIngestCommit) model.IngestCommitOutcome {
	return model.IngestCommitOutcome{
		InitializationID: claim.record.initializationID,
		TransactionID:    claim.record.transactionID,
		Status:           model.IngestCommitNotCommitted,
		RefState:         model.IngestRefExpected,
		Backend:          model.IngestBackendUnchanged,
		Objects:          model.IngestObjectsQuarantined,
		Cleanup:          model.QuarantineCleanupRetained,
		FailureCode:      model.IngestFailureNone,
	}
}

func (s *Store) cleanupCommittedTransaction(
	claim claimedIngestCommit,
	outcome *model.IngestCommitOutcome,
	commitErr error,
) error {
	if outcome.RecoveryRequired || outcome.Status == model.IngestCommitRecoveryRequired {
		return ErrIngestRecoveryRequired
	}
	s.transactionMu.Lock()
	if claim.record.lifecycle == model.IngestLifecycleCommitting {
		claim.record.lifecycle = model.IngestLifecycleFinalizing
	}
	s.transactionMu.Unlock()
	cleanup, recovery := s.cleanupIngestQuarantine(claim.record)
	outcome.Cleanup = cleanup
	if cleanup == model.QuarantineCleanupRemoved {
		if outcome.Status != model.IngestCommitCommitted {
			outcome.Objects = model.IngestObjectsRemoved
		}
		return commitErr
	}
	outcome.RecoveryRequired = recovery || cleanup != model.QuarantineCleanupRemoved
	if outcome.FailureCode == model.IngestFailureNone {
		outcome.FailureCode = model.IngestFailureCleanup
	}
	return ErrIngestRecoveryRequired
}

// CommitIngest publishes one Store-issued main transaction. The caller supplies
// no branch, ref, path, Git verb, or cleanup locator: all authority is claimed
// atomically from the Store registry before any candidate/backend/ref effect.
func (s *Store) CommitIngest(
	ctx context.Context,
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
	profile model.Profile,
	message string,
) (model.IngestCommitOutcome, error) {
	claim, terminal, err := s.claimIngestCommit(initializationID, transactionID)
	if err != nil {
		return invalidAuthorityCommitOutcome(initializationID, transactionID), err
	}
	if terminal != nil {
		return terminal.outcome, terminal.err
	}
	outcome := defaultCommitOutcome(claim)
	if err := claim.baseline.Validate(); err != nil {
		outcome.FailureCode = model.IngestFailureInvalidBaseline
		commitErr := s.cleanupCommittedTransaction(claim, &outcome, ErrIngestNotCommitted)
		return s.terminalizeIngestCommit(claim, outcome, commitErr)
	}
	if err := s.git.validateDirectRef(claim.ref); err != nil {
		outcome.Status = model.IngestCommitRecoveryRequired
		outcome.RefState = model.IngestRefUnknown
		outcome.FailureCode = model.IngestFailureRefPrepare
		outcome.RecoveryRequired = true
		return s.terminalizeIngestCommit(claim, outcome, ErrIngestRecoveryRequired)
	}
	prepared, err := prepareSecrets(profile, s.keychain)
	if err != nil {
		outcome.FailureCode = model.IngestFailureBackend
		commitErr := s.cleanupCommittedTransaction(claim, &outcome, err)
		return s.terminalizeIngestCommit(claim, outcome, commitErr)
	}
	outcome.Withheld = append(model.WithheldReport(nil), prepared.report...)

	var commitErr error
	err = withStoreRootTransactionLock(s.dir, func(guard *storeRootTransactionGuard) error {
		return s.authenticateClaimedQuarantine(guard, claim, func() error {
			candidateOID, candidateErr := s.prepareIngestCandidate(ctx, claim, prepared.profile, message)
			if candidateErr != nil {
				outcome.FailureCode = model.IngestFailureCandidate
				commitErr = candidateErr
				return nil
			}
			mutation, mutationErr := encodeRefMutation(claim.ref, candidateOID, claim.baseline.RefPresent, claim.baseline.ExpectedRevision)
			if mutationErr != nil {
				outcome.FailureCode = model.IngestFailureInvalidBaseline
				commitErr = mutationErr
				return nil
			}
			if directErr := s.git.validateDirectRef(claim.ref); directErr != nil {
				outcome.Status = model.IngestCommitRecoveryRequired
				outcome.RefState = model.IngestRefUnknown
				outcome.FailureCode = model.IngestFailureRefPrepare
				outcome.RecoveryRequired = true
				commitErr = ErrIngestRecoveryRequired
				return nil
			}
			candidateRunner := s.git.candidate(claim.quarantine.indexPath, claim.quarantine.objectsPath)
			candidateRunner.refEvent = s.commitEvent
			session, sessionErr := candidateRunner.startUpdateRefSession(ctx)
			if sessionErr != nil {
				outcome.FailureCode = model.IngestFailureRefPrepare
				commitErr = sessionErr
				return nil
			}
			if prepareErr := session.Prepare(mutation); prepareErr != nil {
				outcome.Status = model.IngestCommitConflict
				outcome.RefState = model.IngestRefExpected
				outcome.FailureCode = model.IngestFailureRefPrepare
				commitErr = ErrSecretRefConflict
				return nil
			}

			priors, backendErr := applyPreparedSecrets(prepared, s.keychain)
			if backendErr != nil {
				outcome.FailureCode = model.IngestFailureBackend
				outcome.Backend = model.IngestBackendRestored
				if errors.Is(backendErr, ErrSecretRollback) {
					outcome.Backend = model.IngestBackendUncertain
					outcome.RecoveryRequired = true
				}
				if abortErr := session.Abort(); abortErr != nil {
					outcome.RecoveryRequired = true
				}
				commitErr = backendErr
				return nil
			}
			if len(prepared.pending) > 0 {
				outcome.Backend = model.IngestBackendApplied
				s.emitCommitEvent("backend-effects")
			}

			publication, publishErr := s.publishCandidateObjects(claim)
			if publishErr != nil {
				outcome.FailureCode = model.IngestFailureObjectPublish
				if publication.created > 0 || publication.uncertain {
					outcome.Objects = model.IngestObjectsUncertain
					outcome.RecoveryRequired = true
				}
				if restoreErr := restoreSecretPriors(s.keychain, priors); restoreErr != nil {
					outcome.Backend = model.IngestBackendUncertain
					outcome.RecoveryRequired = true
				} else if len(prepared.pending) > 0 {
					outcome.Backend = model.IngestBackendRestored
				}
				if abortErr := session.Abort(); abortErr != nil {
					outcome.RecoveryRequired = true
				}
				commitErr = publishErr
				return nil
			}
			outcome.Objects = model.IngestObjectsPublished

			commitSessionErr := error(nil)
			if s.commitRefSession != nil {
				commitSessionErr = s.commitRefSession(session)
			} else {
				commitSessionErr = session.Commit()
			}
			if commitSessionErr == nil {
				outcome.Status = model.IngestCommitCommitted
				outcome.RefState = model.IngestRefCandidate
				outcome.FailureCode = model.IngestFailureNone
				commitErr = nil
				return nil
			}

			// Only a lost response after the standalone commit write reaches this
			// observation boundary. Every deterministic pre-commit error returned
			// above after writing abort instead.
			observed, present, observeErr := s.git.observeDirectRef(ctx, claim.ref)
			switch {
			case observeErr == nil && present && observed == candidateOID:
				outcome.Status = model.IngestCommitCommitted
				outcome.RefState = model.IngestRefCandidate
				outcome.FailureCode = model.IngestFailureNone
				commitErr = nil
				return nil
			case observeErr == nil && ((claim.baseline.RefPresent && present &&
				claim.baseline.ExpectedRevision != nil && observed == *claim.baseline.ExpectedRevision) ||
				(!claim.baseline.RefPresent && !present)):
				outcome.RefState = model.IngestRefExpected
				guardMutation, guardErr := encodeRefGuard(claim.ref, claim.baseline.ExpectedRevision, len(candidateOID))
				if guardErr == nil {
					var guardSession *updateRefSession
					guardSession, guardErr = s.git.startUpdateRefSession(ctx)
					if guardErr == nil {
						guardErr = guardSession.Prepare(guardMutation)
					}
					if guardErr == nil {
						guardErr = restoreSecretPriors(s.keychain, priors)
						if guardErr == nil && len(prepared.pending) > 0 {
							outcome.Backend = model.IngestBackendRestored
						}
					}
					if guardSession != nil && guardSession.locked {
						if abortErr := guardSession.Abort(); guardErr == nil {
							guardErr = abortErr
						}
					}
				}
				if guardErr == nil {
					outcome.Status = model.IngestCommitNotCommitted
					outcome.Objects = model.IngestObjectsRetained
					outcome.FailureCode = model.IngestFailureRefCommit
					commitErr = ErrIngestNotCommitted
					return nil
				}
				outcome.Backend = model.IngestBackendUncertain
			default:
				if observeErr != nil || !present {
					outcome.RefState = model.IngestRefUnknown
				} else {
					outcome.RefState = model.IngestRefOther
				}
			}
			outcome.Status = model.IngestCommitRecoveryRequired
			outcome.FailureCode = model.IngestFailureRefCommit
			outcome.RecoveryRequired = true
			commitErr = ErrIngestRecoveryRequired
			return nil
		})
	})
	if err != nil {
		outcome.Status = model.IngestCommitRecoveryRequired
		outcome.FailureCode = model.IngestFailureQuarantine
		outcome.RecoveryRequired = true
		commitErr = ErrIngestRecoveryRequired
	}
	if !outcome.RecoveryRequired {
		commitErr = s.cleanupCommittedTransaction(claim, &outcome, commitErr)
	}
	return s.terminalizeIngestCommit(claim, outcome, commitErr)
}

type legacyInitializationAuthority struct {
	id model.InstallInitializationID
}

func (s *Store) legacyAuthority(ctx context.Context) (legacyInitializationAuthority, error) {
	s.transactionMu.Lock()
	if !s.legacyInitialization.IsZero() {
		if _, ok := s.validInstallInitializationLocked(s.legacyInitialization); ok {
			authority := legacyInitializationAuthority{id: s.legacyInitialization}
			s.transactionMu.Unlock()
			return authority, nil
		}
	}
	s.transactionMu.Unlock()

	if s.dir == "" {
		return legacyInitializationAuthority{}, ErrNotInitialized
	}
	preflightInfo, err := os.Lstat(s.dir)
	if err != nil || !preflightInfo.IsDir() || preflightInfo.Mode()&os.ModeSymlink != 0 || !s.git.isBareRepo(ctx) {
		return legacyInitializationAuthority{}, ErrNotInitialized
	}
	id, err := model.NewInstallInitializationID()
	if err != nil {
		return legacyInitializationAuthority{}, err
	}
	root, err := filepath.Abs(s.dir)
	if err != nil {
		return legacyInitializationAuthority{}, ErrInvalidIngestAuthority
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return legacyInitializationAuthority{}, ErrInvalidIngestAuthority
	}
	state := &installInitializationState{dir: root, preexisting: true, originalInfo: rootInfo}

	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	if !s.legacyInitialization.IsZero() {
		if _, ok := s.validInstallInitializationLocked(s.legacyInitialization); ok {
			return legacyInitializationAuthority{id: s.legacyInitialization}, nil
		}
	}
	if s.storeNonce.IsZero() {
		return legacyInitializationAuthority{}, ErrInvalidIngestAuthority
	}
	if s.installInitializations == nil {
		s.installInitializations = make(map[model.InstallInitializationID]*installInitializationRecord)
	}
	if s.ingestTransactions == nil {
		s.ingestTransactions = make(map[model.IngestTransactionID]*ingestTransactionRecord)
	}
	s.installInitializations[id] = &installInitializationRecord{
		id:           id,
		storeNonce:   s.storeNonce,
		root:         filepath.Clean(root),
		rootInfo:     rootInfo,
		state:        state,
		transactions: make(map[model.IngestTransactionID]struct{}),
	}
	s.legacyInitialization = id
	return legacyInitializationAuthority{id: id}, nil
}

func (s *Store) beginTransactionForRef(
	ctx context.Context,
	ref validatedHeadRef,
	authority legacyInitializationAuthority,
) (model.IngestBeginOutcome, error) {
	if !ref.valid() || authority.id.IsZero() {
		return model.IngestBeginOutcome{FailureCode: model.IngestFailureInvalidAuthority}, ErrInvalidIngestAuthority
	}
	record, outcome, err := s.reserveIngestTransaction(authority.id)
	if err != nil {
		return outcome, err
	}
	revision, present, err := s.git.observeDirectRef(ctx, ref)
	if err != nil {
		return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
	}
	var (
		expected       *string
		profile        model.Profile
		profilePresent bool
	)
	if present {
		expected, err = model.NewExpectedRevision(revision)
		if err == nil {
			profile, profilePresent, err = s.git.profileAtRevision(ctx, revision)
		}
		if err != nil {
			return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
		}
	}
	baseline, err := model.NewIngestBaseline(
		record.initializationID,
		record.transactionID,
		present,
		expected,
		profile,
		profilePresent,
	)
	if err != nil {
		return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
	}
	record.baseline = baseline
	cleanup, recoveryRequired, err := s.createIngestQuarantine(ctx, record)
	if err != nil {
		return s.terminalizeBeginFailure(record, model.IngestFailureQuarantine, cleanup, recoveryRequired), err
	}

	s.transactionMu.Lock()
	current, ok := s.ingestTransactions[record.transactionID]
	if !ok || current != record || record.lifecycle != model.IngestLifecycleProvisional {
		s.transactionMu.Unlock()
		return s.terminalizeBeginFailure(record, model.IngestFailureInvalidAuthority, model.QuarantineCleanupRetained, true), ErrInvalidIngestAuthority
	}
	if s.ingestTransactionRefs == nil {
		s.ingestTransactionRefs = make(map[model.IngestTransactionID]validatedHeadRef)
	}
	s.ingestTransactionRefs[record.transactionID] = ref
	record.lifecycle = model.IngestLifecycleActive
	record.cleanup = model.QuarantineCleanupRetained
	record.beginOutcome = model.IngestBeginOutcome{
		InitializationID: record.initializationID,
		TransactionID:    record.transactionID,
		Baseline:         baseline,
		Lifecycle:        model.IngestLifecycleActive,
		Cleanup:          model.QuarantineCleanupRetained,
		FailureCode:      model.IngestFailureNone,
	}
	outcome = record.beginOutcome
	s.transactionMu.Unlock()
	return outcome, nil
}

func adaptLegacyCommitOutcome(outcome model.IngestCommitOutcome, cause error) (WithheldReport, error) {
	report := append(WithheldReport(nil), outcome.Withheld...)
	if outcome.RecoveryRequired || outcome.Status == model.IngestCommitRecoveryRequired {
		if outcome.Status == model.IngestCommitCommitted {
			return report, ingestRecoveryError{committed: true}
		}
		return nil, ingestRecoveryError{committed: false}
	}
	switch outcome.Status {
	case model.IngestCommitCommitted:
		return report, nil
	case model.IngestCommitConflict:
		return nil, ErrSecretRefConflict
	default:
		return nil, ingestNotCommittedError{cause: cause}
	}
}

// Commit is the source-compatible branch-aware adapter over the typed ingest
// transaction. It validates the requested branch, creates a private Store-issued
// initialization authority, and routes publication through the same prepared
// no-deref ref lock, secret-redaction, durable-object, and cleanup state machine as
// public main-only ingest. The adapter returns a report only when publication is
// known committed and maps conflict, clean non-commit, and recovery-required
// outcomes to stable sentinel-compatible errors.
func (s *Store) Commit(ctx context.Context, branch string, p model.Profile, msg string) (WithheldReport, error) {
	headRef, err := validatedHeadRefForBranch(branch)
	if err != nil {
		return nil, err
	}
	authority, err := s.legacyAuthority(ctx)
	if err != nil {
		return nil, err
	}
	transaction, err := s.beginTransactionForRef(ctx, headRef, authority)
	if err != nil {
		return nil, err
	}
	outcome, commitErr := s.CommitIngest(ctx, authority.id, transaction.TransactionID, p, msg)
	return adaptLegacyCommitOutcome(outcome, commitErr)
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
