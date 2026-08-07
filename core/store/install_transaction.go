package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"zsh-pro/core/model"
)

const (
	// ErrInvalidIngestAuthority rejects zero, forged, expired, token-swapped, and
	// cross-Store initialization or transaction authority.
	ErrInvalidIngestAuthority errStore = "zsh-pro: invalid ingest transaction authority"
	// ErrInitializerRollbackUnsafe means a transaction is still provisional,
	// active/finalizing, published, or has retained/uncertain cleanup evidence.
	ErrInitializerRollbackUnsafe errStore = "zsh-pro: profile store rollback is unsafe while ingest evidence is unresolved"
	// ErrStoreTransactionLockUnavailable is returned before private namespace
	// mutation when the authenticated per-root lock cannot be established.
	ErrStoreTransactionLockUnavailable errStore = "zsh-pro: profile store transaction lock is unavailable"
)

// InstallInitialization is the narrowly scoped compensation returned by an
// explicit install. It distinguishes a root created by this invocation from an
// existing initialized repository whose private-mode migration may need to be
// reversed after a later bootstrap failure.
type InstallInitialization struct {
	store *Store
	id    model.InstallInitializationID
	state *installInitializationState
}

// ID returns the opaque Store-issued authority for follow-on ingest work.
func (i InstallInitialization) ID() model.InstallInitializationID {
	return i.id
}

// Rollback restores exactly the state that InitForInstall changed. It is safe
// to call more than once; a newly created root is removed only when its complete
// filesystem tree still matches the tree this invocation created.
func (i InstallInitialization) Rollback() error {
	if i.state == nil {
		return nil
	}
	if i.store != nil {
		return i.store.rollbackInstallInitialization(i.id)
	}
	return i.state.rollback()
}

// Finalize expires this initialization authority without changing Store bytes.
// Successful install/ingest flows call it after their durable effects complete.
func (i InstallInitialization) Finalize() error {
	if i.state == nil || i.store == nil {
		return nil
	}
	return i.store.finalizeInstallInitialization(i.id)
}

// CreatedPath reports the store root only when this invocation created it. The
// installer uses that fact to order cleanup when an explicit ZSHPRO_HOME is also
// the cached-loader runtime directory.
func (i InstallInitialization) CreatedPath() string {
	if i.state == nil || i.state.preexisting {
		return ""
	}
	return i.state.dir
}

// InitForInstall initializes or safely migrates a profile store as one part of
// the install transaction. An existing root must already be a complete bare
// store with main: initializing an arbitrary pre-existing directory would make
// it impossible to prove that rollback removed only this invocation's state.
func (s *Store) InitForInstall(ctx context.Context) (InstallInitialization, error) {
	state, err := newInstallInitializationState(s.dir)
	if err != nil {
		return InstallInitialization{}, err
	}
	if state.preexisting && (!s.git.isBareRepo(ctx) || !s.git.catFileExists(ctx, "refs/heads/main")) {
		return InstallInitialization{}, errors.New("profile store root already exists but is not an initialized bare repository")
	}
	if err := s.Init(ctx); err != nil {
		return InstallInitialization{}, installInitializationError(err, state.rollback())
	}
	if err := state.sealCreatedTree(); err != nil {
		return InstallInitialization{}, installInitializationError(err, state.rollback())
	}
	id, err := model.NewInstallInitializationID()
	if err != nil {
		return InstallInitialization{}, installInitializationError(err, state.rollback())
	}
	root, err := filepath.Abs(state.dir)
	if err != nil {
		return InstallInitialization{}, installInitializationError(err, state.rollback())
	}
	rootInfo, err := os.Lstat(state.dir)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = ErrInvalidIngestAuthority
		}
		return InstallInitialization{}, installInitializationError(err, state.rollback())
	}

	s.transactionMu.Lock()
	if s.storeNonce.IsZero() {
		s.transactionMu.Unlock()
		return InstallInitialization{}, installInitializationError(ErrInvalidIngestAuthority, state.rollback())
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
	s.transactionMu.Unlock()

	return InstallInitialization{store: s, id: id, state: state}, nil
}

// BeginIngest starts a main-only ingest transaction.
func (s *Store) BeginIngest(ctx context.Context, id model.InstallInitializationID) (model.IngestBeginOutcome, error) {
	return s.beginIngestForRef(ctx, id, mainHeadRef())
}

func (s *Store) beginIngestForRef(
	ctx context.Context,
	id model.InstallInitializationID,
	ref validatedHeadRef,
) (model.IngestBeginOutcome, error) {
	if !ref.valid() {
		return model.IngestBeginOutcome{FailureCode: model.IngestFailureInvalidAuthority}, ErrInvalidIngestAuthority
	}
	record, outcome, err := s.reserveIngestTransaction(id)
	if err != nil {
		return outcome, err
	}
	if s.beginAfterReserve != nil {
		if err := s.beginAfterReserve(); err != nil {
			return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
		}
	}
	return s.beginIngestReserved(ctx, record, ref)
}

func (s *Store) rollbackInstallInitialization(id model.InstallInitializationID) error {
	s.transactionMu.Lock()
	record, ok := s.validInstallInitializationLocked(id)
	if !ok {
		s.transactionMu.Unlock()
		return ErrInvalidIngestAuthority
	}
	if record.terminal {
		err := record.rollbackErr
		s.transactionMu.Unlock()
		return err
	}
	if record.closing || !s.initializerRollbackSafeLocked(record) {
		s.transactionMu.Unlock()
		return ErrInitializerRollbackUnsafe
	}
	record.closing = true
	s.transactionMu.Unlock()

	err := record.state.rollback()

	s.transactionMu.Lock()
	record.terminal = true
	record.rollbackErr = err
	s.transactionMu.Unlock()
	return err
}

func (s *Store) installInitializationRoot(id model.InstallInitializationID) string {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record, ok := s.validInstallInitializationLocked(id)
	if !ok {
		return ""
	}
	return record.root
}

func (s *Store) installInitializationTransactionCount(id model.InstallInitializationID) int {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record, ok := s.validInstallInitializationLocked(id)
	if !ok {
		return 0
	}
	return len(record.transactions)
}

func (s *Store) initializerRollbackSafe(id model.InstallInitializationID) bool {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record, ok := s.validInstallInitializationLocked(id)
	return ok && !record.closing && !record.terminal && s.initializerRollbackSafeLocked(record)
}

type installInitializationRecord struct {
	id           model.InstallInitializationID
	storeNonce   model.InstallInitializationID
	root         string
	rootInfo     os.FileInfo
	state        *installInitializationState
	transactions map[model.IngestTransactionID]struct{}
	closing      bool
	terminal     bool
	rollbackErr  error
}

type ingestTransactionRecord struct {
	initializationID model.InstallInitializationID
	transactionID    model.IngestTransactionID
	storeNonce       model.InstallInitializationID
	lifecycle        model.IngestLifecycle
	commitStatus     model.IngestCommitStatus
	cleanup          model.QuarantineCleanupState
	recoveryRequired bool
	baseline         model.IngestBaseline
	quarantine       *ingestQuarantine
	beginOutcome     model.IngestBeginOutcome
	abortOutcome     model.IngestAbortOutcome
	abortOutcomeSet  bool
	abortErr         error
}

// ingestQuarantine retains the authenticated top-level identity and descriptor
// issued during Begin. Paths are internal locators only; callers can never
// supply one to Abort.
type ingestQuarantine struct {
	namespacePath string
	basename      string
	path          string
	indexPath     string
	objectsPath   string
	root          *os.Root
	info          os.FileInfo
}

type quarantineCleanupSeam struct {
	Parent    *os.Root
	Name      string
	Relative  string
	Directory bool
}

func (s *Store) setIngestBaseline(record *ingestTransactionRecord, baseline model.IngestBaseline) {
	s.transactionMu.Lock()
	record.baseline = baseline
	s.transactionMu.Unlock()
}

func (s *Store) ingestBaseline(record *ingestTransactionRecord) model.IngestBaseline {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	return record.baseline
}

func (s *Store) setIngestQuarantine(record *ingestTransactionRecord, quarantine *ingestQuarantine) {
	s.transactionMu.Lock()
	record.quarantine = quarantine
	s.transactionMu.Unlock()
}

func (s *Store) ingestQuarantine(record *ingestTransactionRecord) *ingestQuarantine {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	return record.quarantine
}

func (s *Store) reserveIngestTransaction(id model.InstallInitializationID) (*ingestTransactionRecord, model.IngestBeginOutcome, error) {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	initialization, ok := s.validInstallInitializationLocked(id)
	if !ok || initialization.closing || initialization.terminal {
		return nil, model.IngestBeginOutcome{FailureCode: model.IngestFailureInvalidAuthority}, ErrInvalidIngestAuthority
	}
	token, err := model.NewIngestTransactionID()
	if err != nil {
		return nil, model.IngestBeginOutcome{FailureCode: model.IngestFailureInvalidAuthority}, err
	}
	record := &ingestTransactionRecord{
		initializationID: id,
		transactionID:    token,
		storeNonce:       s.storeNonce,
		lifecycle:        model.IngestLifecycleProvisional,
		commitStatus:     model.IngestCommitNotCommitted,
		cleanup:          model.QuarantineCleanupRemoved,
	}
	record.beginOutcome = model.IngestBeginOutcome{
		InitializationID: id,
		TransactionID:    token,
		Lifecycle:        model.IngestLifecycleProvisional,
		Cleanup:          model.QuarantineCleanupRemoved,
		FailureCode:      model.IngestFailureNone,
	}
	s.ingestTransactions[token] = record
	initialization.transactions[token] = struct{}{}
	return record, record.beginOutcome, nil
}

func (s *Store) beginIngestReserved(
	ctx context.Context,
	record *ingestTransactionRecord,
	ref validatedHeadRef,
) (model.IngestBeginOutcome, error) {
	observe := func(ctx context.Context) (string, bool, error) {
		return s.git.observeDirectRef(ctx, ref)
	}
	if ref == mainHeadRef() && s.beginObserveMain != nil {
		observe = s.beginObserveMain
	}
	revision, present, err := observe(ctx)
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
		if err != nil {
			return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
		}
		profile, profilePresent, err = s.git.profileAtRevision(ctx, revision)
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
	s.setIngestBaseline(record, baseline)

	cleanup, recoveryRequired, setupErr := s.createIngestQuarantine(ctx, record)
	if setupErr != nil {
		return s.terminalizeBeginFailure(record, model.IngestFailureQuarantine, cleanup, recoveryRequired), setupErr
	}

	s.transactionMu.Lock()
	current, ok := s.ingestTransactions[record.transactionID]
	if !ok || current != record || record.lifecycle != model.IngestLifecycleProvisional {
		s.transactionMu.Unlock()
		return s.terminalizeBeginFailure(record, model.IngestFailureInvalidAuthority, model.QuarantineCleanupRetained, true), ErrInvalidIngestAuthority
	}
	if ref != mainHeadRef() {
		if s.ingestTransactionRefs == nil {
			s.ingestTransactionRefs = make(map[model.IngestTransactionID]validatedHeadRef)
		}
		s.ingestTransactionRefs[record.transactionID] = ref
	}
	record.lifecycle = model.IngestLifecycleActive
	record.cleanup = model.QuarantineCleanupRetained
	record.beginOutcome = model.IngestBeginOutcome{
		InitializationID:        record.initializationID,
		TransactionID:           record.transactionID,
		Baseline:                baseline,
		Lifecycle:               model.IngestLifecycleActive,
		Cleanup:                 model.QuarantineCleanupRetained,
		FailureCode:             model.IngestFailureNone,
		RecoveryRequired:        false,
		InitializerRollbackSafe: false,
	}
	outcome := record.beginOutcome
	s.transactionMu.Unlock()
	return outcome, nil
}

// AbortIngest cleans one Store-owned transaction quarantine.
func (s *Store) AbortIngest(
	ctx context.Context,
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
) (model.IngestAbortOutcome, error) {
	s.transactionMu.Lock()
	initialization, validInitialization := s.validInstallInitializationLocked(initializationID)
	record, ok := s.ingestTransactions[transactionID]
	if !validInitialization || !ok || record == nil || record.initializationID != initializationID ||
		record.storeNonce != s.storeNonce {
		s.transactionMu.Unlock()
		return model.IngestAbortOutcome{}, ErrInvalidIngestAuthority
	}
	if record.lifecycle == model.IngestLifecycleTerminal {
		outcome, err := s.terminalAbortOutcomeLocked(initialization, record)
		s.transactionMu.Unlock()
		return outcome, err
	}
	if initialization.closing || initialization.terminal || record.lifecycle != model.IngestLifecycleActive {
		s.transactionMu.Unlock()
		return model.IngestAbortOutcome{}, ErrInvalidIngestAuthority
	}
	record.lifecycle = model.IngestLifecycleFinalizing
	s.transactionMu.Unlock()

	cleanup, recoveryRequired, cleanupErr := s.cleanupIngestQuarantine(ctx, record)

	s.transactionMu.Lock()
	record.lifecycle = model.IngestLifecycleTerminal
	record.cleanup = cleanup
	record.recoveryRequired = recoveryRequired
	rollbackSafe := s.initializerRollbackSafeLocked(initialization)
	failure := model.IngestFailureNone
	if cleanup != model.QuarantineCleanupRemoved || recoveryRequired {
		failure = model.IngestFailureCleanup
	}
	record.abortOutcome = model.IngestAbortOutcome{
		InitializationID:        record.initializationID,
		TransactionID:           record.transactionID,
		Lifecycle:               model.IngestLifecycleTerminal,
		Cleanup:                 cleanup,
		FailureCode:             failure,
		RecoveryRequired:        recoveryRequired,
		InitializerRollbackSafe: rollbackSafe,
	}
	record.abortOutcomeSet = true
	if errors.Is(cleanupErr, context.Canceled) || errors.Is(cleanupErr, context.DeadlineExceeded) {
		record.abortErr = cleanupErr
	}
	outcome := record.abortOutcome
	abortErr := record.abortErr
	s.transactionMu.Unlock()
	return outcome, abortErr
}

func (s *Store) terminalAbortOutcomeLocked(
	initialization *installInitializationRecord,
	record *ingestTransactionRecord,
) (model.IngestAbortOutcome, error) {
	if record.abortOutcomeSet {
		return record.abortOutcome, record.abortErr
	}

	failure := record.beginOutcome.FailureCode
	cleanup := record.cleanup
	recoveryRequired := record.recoveryRequired
	if terminal, ok := s.ingestCommitOutcomes[record.transactionID]; ok {
		failure = terminal.outcome.FailureCode
		cleanup = terminal.outcome.Cleanup
		recoveryRequired = terminal.outcome.RecoveryRequired
	}
	record.abortOutcome = model.IngestAbortOutcome{
		InitializationID:        record.initializationID,
		TransactionID:           record.transactionID,
		Lifecycle:               model.IngestLifecycleTerminal,
		Cleanup:                 cleanup,
		FailureCode:             failure,
		RecoveryRequired:        recoveryRequired,
		InitializerRollbackSafe: s.initializerRollbackSafeLocked(initialization),
	}
	record.abortOutcomeSet = true
	return record.abortOutcome, record.abortErr
}

func (s *Store) transactionQuarantinePath(transactionID model.IngestTransactionID) string {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record := s.ingestTransactions[transactionID]
	if record == nil || record.storeNonce != s.storeNonce || record.quarantine == nil {
		return ""
	}
	return record.quarantine.path
}

func (s *Store) createIngestQuarantine(
	ctx context.Context,
	record *ingestTransactionRecord,
) (model.QuarantineCleanupState, bool, error) {
	var (
		setupErr         error
		cleanup          = model.QuarantineCleanupRemoved
		recoveryRequired bool
	)
	err := withStoreRootTransactionLock(ctx, s.dir, func(guard *storeRootTransactionGuard) error {
		return guard.withAuthenticatedMutation(func(namespace *os.Root) error {
			name, err := newQuarantineBasename()
			if err != nil {
				setupErr = err
				return err
			}
			namespacePath, err := storeTransactionNamespacePath(s.dir)
			if err != nil {
				setupErr = err
				return err
			}
			if err := namespace.Mkdir(name, 0o700); err != nil {
				setupErr = err
				return err
			}
			info, err := namespace.Lstat(name)
			if err != nil || !validTransactionNamespaceInfo(info) {
				if err == nil {
					err = errors.New("transaction quarantine is not private")
				}
				setupErr = err
				return err
			}
			quarantine := &ingestQuarantine{
				namespacePath: namespacePath,
				basename:      name,
				path:          filepath.Join(namespacePath, name),
				indexPath:     filepath.Join(namespacePath, name, "index"),
				objectsPath:   filepath.Join(namespacePath, name, "objects"),
				info:          info,
			}
			s.setIngestQuarantine(record, quarantine)

			failAfterCreation := func(cause error) error {
				setupErr = cause
				cleanup, recoveryRequired = s.cleanupIngestQuarantineWithGuard(guard, record)
				return cause
			}
			if s.beginAfterQuarantineCreate != nil {
				if err := s.beginAfterQuarantineCreate(quarantine.path); err != nil {
					return failAfterCreation(err)
				}
			}

			root, err := namespace.OpenRoot(name)
			if err != nil {
				return failAfterCreation(err)
			}
			quarantine.root = root
			if s.beginAfterQuarantineOpen != nil {
				if err := s.beginAfterQuarantineOpen(quarantine.path); err != nil {
					return failAfterCreation(err)
				}
			}
			openedInfo, err := root.Stat(".")
			if err != nil || !sameQuarantineEntry(info, openedInfo) || !validTransactionNamespaceInfo(openedInfo) {
				if err == nil {
					err = errors.New("transaction quarantine identity changed")
				}
				return failAfterCreation(err)
			}
			if s.beginAfterQuarantineStat != nil {
				if err := s.beginAfterQuarantineStat(quarantine.path); err != nil {
					return failAfterCreation(err)
				}
			}

			if err := root.Mkdir("objects", 0o700); err != nil {
				return failAfterCreation(err)
			}
			objectsInfo, err := root.Lstat("objects")
			if err != nil || !validTransactionNamespaceInfo(objectsInfo) {
				if err == nil {
					err = errors.New("candidate object directory is not private")
				}
				return failAfterCreation(err)
			}

			candidate := s.git.candidate(quarantine.indexPath, quarantine.objectsPath)
			baseline := s.ingestBaseline(record)
			if baseline.ExpectedRevision == nil {
				_, err = candidate.run(ctx, "read-tree", "--empty")
			} else {
				_, err = candidate.run(ctx, "read-tree", *baseline.ExpectedRevision)
			}
			if err != nil {
				return failAfterCreation(err)
			}
			index, err := root.OpenFile("index", os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return failAfterCreation(err)
			}
			if err := index.Chmod(0o600); err != nil {
				_ = index.Close()
				return failAfterCreation(err)
			}
			indexInfo, statErr := index.Stat()
			currentIndexInfo, lstatErr := root.Lstat("index")
			if statErr != nil || lstatErr != nil || !validTransactionLockInfo(indexInfo) ||
				!sameQuarantineEntry(indexInfo, currentIndexInfo) {
				_ = index.Close()
				if statErr != nil {
					return failAfterCreation(statErr)
				}
				if lstatErr != nil {
					return failAfterCreation(lstatErr)
				}
				return failAfterCreation(errors.New("candidate index is not private"))
			}
			if err := index.Sync(); err != nil {
				_ = index.Close()
				return failAfterCreation(err)
			}
			if err := index.Close(); err != nil {
				return failAfterCreation(err)
			}
			objects, err := root.OpenRoot("objects")
			if err != nil {
				return failAfterCreation(err)
			}
			objectsSyncErr := syncTransactionRoot(objects)
			objectsCloseErr := objects.Close()
			if objectsSyncErr != nil {
				return failAfterCreation(objectsSyncErr)
			}
			if objectsCloseErr != nil {
				return failAfterCreation(objectsCloseErr)
			}
			if err := syncTransactionRoot(root); err != nil {
				return failAfterCreation(err)
			}
			if err := syncTransactionRoot(namespace); err != nil {
				return failAfterCreation(err)
			}
			return nil
		})
	})
	if err != nil {
		if setupErr == nil {
			setupErr = err
			if s.ingestQuarantine(record) != nil {
				cleanup = model.QuarantineCleanupRetained
				recoveryRequired = true
			} else if errors.Is(err, ErrStoreTransactionLockUnavailable) {
				cleanup = model.QuarantineCleanupRetained
				recoveryRequired = true
			}
		}
		return cleanup, recoveryRequired, setupErr
	}
	return model.QuarantineCleanupRetained, false, nil
}

func newQuarantineBasename() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "ingest-" + hex.EncodeToString(random[:]), nil
}

func (s *Store) cleanupIngestQuarantine(
	ctx context.Context,
	record *ingestTransactionRecord,
) (model.QuarantineCleanupState, bool, error) {
	if record == nil || s.ingestQuarantine(record) == nil {
		return model.QuarantineCleanupRemoved, false, nil
	}
	cleanup := model.QuarantineCleanupRetained
	recoveryRequired := true
	err := withStoreRootTransactionLock(ctx, s.dir, func(guard *storeRootTransactionGuard) error {
		if s.cleanupDiscardLock {
			guard.discardForTest()
		}
		cleanup, recoveryRequired = s.cleanupIngestQuarantineWithGuard(guard, record)
		if cleanup != model.QuarantineCleanupRemoved || recoveryRequired {
			return errors.New("transaction quarantine cleanup retained state")
		}
		return nil
	})
	if err != nil && cleanup == model.QuarantineCleanupRemoved {
		return model.QuarantineCleanupRetained, true, err
	}
	return cleanup, recoveryRequired, err
}

// cleanupIngestQuarantineWithGuard implements the cooperating-process boundary:
// every zsh-pro process holds the per-root advisory lock for the complete
// authenticate, remove, and parent-sync interval. A non-cooperating same-UID or
// privileged process can still mutate this private namespace; every observed
// identity substitution is therefore retained for explicit recovery.
func (s *Store) cleanupIngestQuarantineWithGuard(
	guard *storeRootTransactionGuard,
	record *ingestTransactionRecord,
) (model.QuarantineCleanupState, bool) {
	quarantine := s.ingestQuarantine(record)
	if quarantine == nil {
		return model.QuarantineCleanupRemoved, false
	}
	err := guard.withAuthenticatedMutation(func(namespace *os.Root) error {
		current, err := namespace.Lstat(quarantine.basename)
		if err != nil || !sameQuarantineEntry(quarantine.info, current) {
			if err != nil {
				return err
			}
			return errQuarantineIdentityChanged
		}
		root := quarantine.root
		if root == nil {
			root, err = namespace.OpenRoot(quarantine.basename)
			if err != nil {
				return err
			}
			quarantine.root = root
		}
		opened, err := root.Stat(".")
		if err != nil || !sameQuarantineEntry(quarantine.info, opened) {
			if err != nil {
				return err
			}
			return errQuarantineIdentityChanged
		}

		entries, err := fs.ReadDir(root.FS(), ".")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := s.removeAuthenticatedQuarantineEntry(root, entry.Name(), entry.Name()); err != nil {
				return err
			}
		}
		if err := syncTransactionRoot(root); err != nil {
			return err
		}
		seam := quarantineCleanupSeam{Parent: namespace, Name: quarantine.basename, Relative: ".", Directory: true}
		if s.cleanupBeforeFinalCheck != nil {
			if err := s.cleanupBeforeFinalCheck(seam); err != nil {
				return err
			}
		}
		current, err = namespace.Lstat(quarantine.basename)
		if err != nil || !sameQuarantineEntry(quarantine.info, current) {
			if err != nil {
				return err
			}
			return errQuarantineIdentityChanged
		}
		if s.cleanupAfterFinalCheck != nil {
			if err := s.cleanupAfterFinalCheck(seam); err != nil {
				return err
			}
		}
		current, err = namespace.Lstat(quarantine.basename)
		if err != nil || !sameQuarantineEntry(quarantine.info, current) {
			if err != nil {
				return err
			}
			return errQuarantineIdentityChanged
		}
		if err := namespace.Remove(quarantine.basename); err != nil {
			return err
		}
		if err := syncTransactionRoot(namespace); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return model.QuarantineCleanupRetained, true
	}
	if quarantine.root != nil {
		_ = quarantine.root.Close()
		quarantine.root = nil
	}
	return model.QuarantineCleanupRemoved, false
}

var errQuarantineIdentityChanged = errors.New("transaction quarantine identity changed")

func (s *Store) removeAuthenticatedQuarantineEntry(parent *os.Root, name, relative string) error {
	original, err := parent.Lstat(name)
	if err != nil || !validQuarantineChildInfo(original) {
		if err != nil {
			return err
		}
		return errQuarantineIdentityChanged
	}
	directory := original.IsDir()
	if directory {
		child, err := parent.OpenRoot(name)
		if err != nil {
			return err
		}
		// Keep the descriptor open through the final seams and unlink. Besides
		// anchoring recursion, this prevents a removed replacement from reusing
		// the original directory inode before the final identity comparison.
		defer func() { _ = child.Close() }()
		opened, statErr := child.Stat(".")
		if statErr != nil || !sameQuarantineEntry(original, opened) {
			if statErr != nil {
				return statErr
			}
			return errQuarantineIdentityChanged
		}
		entries, readErr := fs.ReadDir(child.FS(), ".")
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			childRelative := filepath.Join(relative, entry.Name())
			if err := s.removeAuthenticatedQuarantineEntry(child, entry.Name(), childRelative); err != nil {
				return err
			}
		}
		if err := syncTransactionRoot(child); err != nil {
			return err
		}
	} else if original.Mode().IsRegular() {
		file, err := parent.OpenFile(name, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		opened, statErr := file.Stat()
		if statErr != nil || !sameQuarantineEntry(original, opened) {
			if statErr != nil {
				return statErr
			}
			return errQuarantineIdentityChanged
		}
	} else if original.Mode()&os.ModeSymlink != 0 {
		// 0x200000 is O_PATH on Linux and O_SYMLINK on Darwin. Combined with
		// O_NOFOLLOW it retains the symlink object itself across the final-check
		// seams, preventing same-inode reuse from defeating replacement detection.
		link, err := parent.OpenFile(name, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|0x200000, 0)
		if err != nil {
			return err
		}
		defer func() { _ = link.Close() }()
		opened, statErr := link.Stat()
		if statErr != nil || !sameQuarantineEntry(original, opened) {
			if statErr != nil {
				return statErr
			}
			return errQuarantineIdentityChanged
		}
	}

	seam := quarantineCleanupSeam{Parent: parent, Name: name, Relative: relative, Directory: directory}
	if s.cleanupBeforeFinalCheck != nil {
		if err := s.cleanupBeforeFinalCheck(seam); err != nil {
			return err
		}
	}
	current, err := parent.Lstat(name)
	if err != nil || !sameQuarantineEntry(original, current) {
		if err != nil {
			return err
		}
		return errQuarantineIdentityChanged
	}
	if s.cleanupAfterFinalCheck != nil {
		if err := s.cleanupAfterFinalCheck(seam); err != nil {
			return err
		}
	}
	current, err = parent.Lstat(name)
	if err != nil || !sameQuarantineEntry(original, current) {
		if err != nil {
			return err
		}
		return errQuarantineIdentityChanged
	}
	if err := parent.Remove(name); err != nil {
		return err
	}
	return syncTransactionRoot(parent)
}

func validQuarantineChildInfo(info os.FileInfo) bool {
	if info == nil || !fileInfoOwnedByCurrentEUID(info) {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0
}

func sameQuarantineEntry(expected, actual os.FileInfo) bool {
	return expected != nil && actual != nil && os.SameFile(expected, actual) &&
		expected.Mode() == actual.Mode() && fileInfoOwnedByCurrentEUID(actual)
}

func (s *Store) terminalizeBeginFailure(
	record *ingestTransactionRecord,
	failure model.IngestFailureCode,
	cleanup model.QuarantineCleanupState,
	recoveryRequired bool,
) model.IngestBeginOutcome {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	current, ok := s.ingestTransactions[record.transactionID]
	if !ok || current != record || current.lifecycle == model.IngestLifecycleTerminal {
		return record.beginOutcome
	}
	record.lifecycle = model.IngestLifecycleTerminal
	record.commitStatus = model.IngestCommitNotCommitted
	record.cleanup = cleanup
	record.recoveryRequired = recoveryRequired
	initialization := s.installInitializations[record.initializationID]
	safe := initialization != nil && s.initializerRollbackSafeLocked(initialization)
	record.beginOutcome = model.IngestBeginOutcome{
		InitializationID:        record.initializationID,
		TransactionID:           record.transactionID,
		Baseline:                record.baseline,
		Lifecycle:               model.IngestLifecycleTerminal,
		Cleanup:                 cleanup,
		FailureCode:             failure,
		RecoveryRequired:        recoveryRequired,
		InitializerRollbackSafe: safe,
	}
	return record.beginOutcome
}

func (s *Store) validInstallInitializationLocked(id model.InstallInitializationID) (*installInitializationRecord, bool) {
	if id.IsZero() || s.storeNonce.IsZero() {
		return nil, false
	}
	record, ok := s.installInitializations[id]
	if !ok || record == nil || record.id != id || record.storeNonce != s.storeNonce || record.state == nil {
		return nil, false
	}
	if !record.closing && !record.terminal {
		current, err := os.Lstat(record.root)
		if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 ||
			record.rootInfo == nil || !os.SameFile(record.rootInfo, current) {
			return nil, false
		}
	}
	return record, true
}

func (s *Store) initializerRollbackSafeLocked(initialization *installInitializationRecord) bool {
	for token := range initialization.transactions {
		record, ok := s.ingestTransactions[token]
		if !ok || record == nil || record.initializationID != initialization.id || record.storeNonce != s.storeNonce {
			return false
		}
		if record.lifecycle != model.IngestLifecycleTerminal || record.commitStatus != model.IngestCommitNotCommitted ||
			record.cleanup != model.QuarantineCleanupRemoved || record.recoveryRequired {
			return false
		}
	}
	return true
}

func (s *Store) finalizeInstallInitialization(id model.InstallInitializationID) error {
	s.transactionMu.Lock()
	defer s.transactionMu.Unlock()
	record, ok := s.validInstallInitializationLocked(id)
	if !ok {
		return ErrInvalidIngestAuthority
	}
	if record.terminal {
		return record.rollbackErr
	}
	for token := range record.transactions {
		transaction := s.ingestTransactions[token]
		if transaction == nil || transaction.lifecycle != model.IngestLifecycleTerminal {
			return ErrInitializerRollbackUnsafe
		}
	}
	record.closing = true
	record.terminal = true
	return nil
}

type installInitializationState struct {
	dir                string
	preexisting        bool
	originalInfo       os.FileInfo
	restoreMode        bool
	missingDirectories []string
	createdTree        map[string]storeTreeEntry
	createdRoot        bool

	once        sync.Once
	rollbackErr error
}

func newInstallInitializationState(dir string) (*installInitializationState, error) {
	dir = filepath.Clean(dir)
	info, err := os.Lstat(dir)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("profile store root must not be a symlink")
		}
		if !info.IsDir() {
			return nil, errors.New("profile store root must be a directory")
		}
		return &installInitializationState{
			dir:          dir,
			preexisting:  true,
			originalInfo: info,
			restoreMode:  info.Mode().Perm()&0o077 != 0,
		}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect profile store root: %w", err)
	}
	missing, err := missingStoreDirectories(dir)
	if err != nil {
		return nil, err
	}
	return &installInitializationState{dir: dir, missingDirectories: missing}, nil
}

func missingStoreDirectories(dir string) ([]string, error) {
	var missing []string
	for path := dir; ; path = filepath.Dir(path) {
		_, err := os.Lstat(path)
		if err == nil {
			return missing, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect profile store parent: %w", err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil, errors.New("profile store root has no existing parent")
		}
		missing = append(missing, path)
	}
}

func (s *installInitializationState) sealCreatedTree() error {
	if s.preexisting || s.createdRoot {
		return nil
	}
	tree, exists, err := snapshotStoreTree(s.dir)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	s.createdTree = tree
	s.createdRoot = true
	return nil
}

func (s *installInitializationState) rollback() error {
	s.once.Do(func() {
		if s.preexisting {
			if s.restoreMode {
				s.rollbackErr = restorePrivateStoreDirMode(s.dir, s.originalInfo)
			}
			return
		}
		if err := s.sealCreatedTree(); err != nil {
			s.rollbackErr = err
			return
		}
		if !s.createdRoot {
			return
		}
		current, exists, err := snapshotStoreTree(s.dir)
		if err != nil {
			s.rollbackErr = err
			return
		}
		if !exists {
			return
		}
		if !sameStoreTree(s.createdTree, current) {
			s.rollbackErr = errors.New("refusing to remove profile store root changed during installation")
			return
		}
		if err := os.RemoveAll(s.dir); err != nil {
			s.rollbackErr = fmt.Errorf("remove newly created profile store root: %w", err)
			return
		}
		for _, path := range s.missingDirectories {
			if err := os.Remove(path); err != nil {
				switch {
				case errors.Is(err, os.ErrNotExist):
					continue
				case errors.Is(err, syscall.ENOTEMPTY), errors.Is(err, syscall.EEXIST):
					// A stable per-root transaction sibling deliberately survives Store
					// rollback, so its newly-created parent may no longer be removable.
					return
				default:
					s.rollbackErr = fmt.Errorf("remove newly created profile store parent %s: %w", path, err)
					return
				}
			}
		}
	})
	return s.rollbackErr
}

func installInitializationError(cause, rollbackErr error) error {
	if rollbackErr == nil {
		return cause
	}
	return fmt.Errorf("%w; profile-store rollback failed: %v", cause, rollbackErr)
}

type storeTreeEntry struct {
	mode   fs.FileMode
	digest [sha256.Size]byte
	link   string
}

func snapshotStoreTree(root string) (map[string]storeTreeEntry, bool, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect profile store rollback root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, errors.New("profile store rollback root is no longer a directory")
	}

	tree := make(map[string]storeTreeEntry)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		recorded := storeTreeEntry{mode: info.Mode()}
		switch {
		case info.Mode().IsRegular():
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			recorded.digest = sha256.Sum256(content)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			recorded.link = link
		case info.IsDir():
			// Directories carry only their mode; WalkDir never follows symlinks.
		default:
			return fmt.Errorf("profile store contains unsupported rollback entry %s", rel)
		}
		tree[rel] = recorded
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("snapshot profile store rollback state: %w", err)
	}
	return tree, true, nil
}

func sameStoreTree(want, got map[string]storeTreeEntry) bool {
	if len(want) != len(got) {
		return false
	}
	for path, expected := range want {
		if actual, ok := got[path]; !ok || actual != expected {
			return false
		}
	}
	return true
}
