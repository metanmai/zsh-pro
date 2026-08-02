package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"zsh-pro/core/model"
)

const (
	// ErrInvalidIngestAuthority rejects zero, forged, expired, token-swapped, and
	// cross-Store initialization or transaction authority.
	ErrInvalidIngestAuthority errStore = "zsh-pro: invalid ingest transaction authority"
	// ErrInitializerRollbackUnsafe means a transaction is still provisional,
	// active/finalizing, published, or has retained/uncertain cleanup evidence.
	ErrInitializerRollbackUnsafe errStore = "zsh-pro: profile store rollback is unsafe while ingest evidence is unresolved"
	// ErrIngestUnavailable is the temporary fail-closed result until the baseline
	// and quarantine implementation activates a valid reserved transaction.
	ErrIngestUnavailable errStore = "zsh-pro: ingest transaction setup is unavailable"
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
	rootInfo, err := os.Stat(state.dir)
	if err != nil {
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
	record, outcome, err := s.reserveIngestTransaction(id)
	if err != nil {
		return outcome, err
	}
	if s.beginAfterReserve != nil {
		if err := s.beginAfterReserve(); err != nil {
			return s.terminalizeBeginFailure(record, model.IngestFailureBaselineRead, model.QuarantineCleanupRemoved, false), err
		}
	}
	return s.beginIngestReserved(ctx, record)
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
	beginOutcome     model.IngestBeginOutcome
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

func (s *Store) beginIngestReserved(_ context.Context, record *ingestTransactionRecord) (model.IngestBeginOutcome, error) {
	return s.terminalizeBeginFailure(record, model.IngestFailureQuarantine, model.QuarantineCleanupRemoved, false), ErrIngestUnavailable
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
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				s.rollbackErr = fmt.Errorf("remove newly created profile store parent %s: %w", path, err)
				return
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
