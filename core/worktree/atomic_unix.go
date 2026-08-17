//go:build linux || darwin

package worktree

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	stateFileName             = "worktree.json"
	stateLockFileName         = "worktree.lock"
	stateForensicFileName     = "events.jsonl"
	stateTemporaryPrefix      = ".worktree-state-"
	stateLockRetryInterval    = 5 * time.Millisecond
	maxStateForensicFileBytes = 64 * 1024
)

var (
	ErrStateStoreClosed         = errors.New("worktree state store is closed")
	ErrStateStoreAuthentication = errors.New("worktree state store authentication failed")
	ErrStateStoreCorrupt        = errors.New("worktree state generation is invalid")
)

type stateRootMetadata struct {
	isDir bool
	mode  os.FileMode
	uid   uint32
	dev   uint64
	ino   uint64
}

type stateStoreFaults struct {
	write          func(int, []byte) (int, error)
	fileSync       func(int) error
	rename         func(int, string, int, string) error
	dirSync        func(int) error
	forensicAppend func(int, []byte) (int, error)
	beforeRename   func() error
	afterRename    func() error
	afterDirSync   func() error
}

// StateStore owns a duplicate of one authenticated private runtime-root
// descriptor. All mutable authority is addressed relative to that descriptor.
type StateStore struct {
	lifecycle sync.Mutex
	root      *os.File
	identity  stateRootMetadata
	closed    bool

	// A derived forensic append is not authoritative. If it fails, the next
	// successful transaction durably projects a value-free recovered marker.
	pendingRecoveredPersistence bool
	faults                      stateStoreFaults
}

// OpenStateStore walks an absolute path without following symlinks, authenticates
// the resulting directory descriptor, and transfers ownership to the Store.
func OpenStateStore(path string) (*StateStore, error) {
	root, err := openStateRootNoFollow(path)
	if err != nil {
		return nil, err
	}
	store, err := newStateStoreOwnedRoot(root)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return store, nil
}

// NewStateStoreFromAuthenticatedRoot duplicates and re-authenticates rootFD.
// It never performs pathname access and never takes ownership of rootFD.
func NewStateStoreFromAuthenticatedRoot(rootFD *os.File) (*StateStore, error) {
	if rootFD == nil {
		return nil, fmt.Errorf("%w: missing root descriptor", ErrStateStoreAuthentication)
	}
	duplicateFD, err := syscall.Dup(int(rootFD.Fd()))
	if err != nil {
		return nil, fmt.Errorf("duplicate state root: %w", err)
	}
	syscall.CloseOnExec(duplicateFD)
	duplicate := os.NewFile(uintptr(duplicateFD), "zsh-pro worktree state root")
	if duplicate == nil {
		_ = syscall.Close(duplicateFD)
		return nil, fmt.Errorf("%w: retain duplicate root descriptor", ErrStateStoreAuthentication)
	}
	store, err := newStateStoreOwnedRoot(duplicate)
	if err != nil {
		_ = duplicate.Close()
		return nil, err
	}
	return store, nil
}

func newStateStoreOwnedRoot(root *os.File) (*StateStore, error) {
	metadata, err := authenticateStateRoot(root)
	if err != nil {
		return nil, err
	}
	return &StateStore{root: root, identity: metadata}, nil
}

// Close releases the Store-owned descriptor exactly once. It never closes a
// descriptor supplied to NewStateStoreFromAuthenticatedRoot.
func (s *StateStore) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	root := s.root
	s.root = nil
	if root == nil {
		return nil
	}
	return root.Close()
}

func (s *StateStore) Read(ctx context.Context) (State, error) {
	var result State
	err := s.WithTransaction(ctx, func(state *State) error {
		result = cloneState(*state)
		return nil
	})
	return result, err
}

// WithTransaction serializes one load/validate/mutate/atomic-replace cycle
// under a descriptor-relative advisory lock. The caller controls all waiting
// through ctx; the Store never extends or substitutes the deadline.
func (s *StateStore) WithTransaction(ctx context.Context, mutate func(*State) error) error {
	if s == nil {
		return ErrStateStoreClosed
	}
	if ctx == nil {
		return errors.New("state transaction requires a context")
	}
	if mutate == nil {
		return errors.New("state transaction requires a callback")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed || s.root == nil {
		return ErrStateStoreClosed
	}
	if err := s.authenticateRoot(); err != nil {
		return err
	}
	lock, err := s.openLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := acquireStateLock(ctx, int(lock.Fd())); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err := s.authenticateLockBinding(lock); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.authenticateRoot(); err != nil {
		return err
	}

	state, previous, err := s.readCanonical()
	if err != nil {
		return err
	}
	if s.pendingRecoveredPersistence {
		for shellID, shell := range state.Shells {
			shell.LastRecoveredError = RecoveredPersistence
			state.Shells[shellID] = shell
		}
	}
	if err := mutate(&state); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.authenticateRoot(); err != nil {
		return err
	}
	if err := s.authenticateLockBinding(lock); err != nil {
		return err
	}
	next, err := MarshalState(state)
	if err != nil {
		return err
	}
	if bytes.Equal(previous, next) {
		if err := s.authenticateLockBinding(lock); err != nil {
			return err
		}
		return s.authenticateRoot()
	}
	if err := s.replaceCanonical(next); err != nil {
		return err
	}
	if err := s.authenticateRoot(); err != nil {
		return err
	}
	s.pendingRecoveredPersistence = false
	if err := s.appendForensic(state); err != nil {
		s.pendingRecoveredPersistence = true
	}
	return nil
}

func openStateRootNoFollow(path string) (*os.File, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: root must be a non-root absolute path", ErrStateStoreAuthentication)
	}
	fd, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open state root anchor: %w", err)
	}
	for _, part := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			_ = syscall.Close(fd)
			return nil, fmt.Errorf("%w: invalid root path component", ErrStateStoreAuthentication)
		}
		next, openErr := stateOpenat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		closeErr := syscall.Close(fd)
		if openErr != nil {
			return nil, fmt.Errorf("%w: open root component: %v", ErrStateStoreAuthentication, openErr)
		}
		if closeErr != nil {
			_ = syscall.Close(next)
			return nil, fmt.Errorf("close state root parent: %w", closeErr)
		}
		fd = next
	}
	root := os.NewFile(uintptr(fd), "zsh-pro worktree state root")
	if root == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: retain root descriptor", ErrStateStoreAuthentication)
	}
	return root, nil
}

func authenticateStateRoot(root *os.File) (stateRootMetadata, error) {
	if root == nil {
		return stateRootMetadata{}, fmt.Errorf("%w: missing descriptor", ErrStateStoreAuthentication)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(root.Fd()), &stat); err != nil {
		return stateRootMetadata{}, fmt.Errorf("%w: stat root: %v", ErrStateStoreAuthentication, err)
	}
	metadata := stateRootMetadata{
		isDir: stat.Mode&syscall.S_IFMT == syscall.S_IFDIR,
		mode:  os.FileMode(stat.Mode & 0o777),
		uid:   stat.Uid,
		dev:   uint64(stat.Dev),
		ino:   stat.Ino,
	}
	if err := validateStateRootMetadata(metadata); err != nil {
		return stateRootMetadata{}, err
	}
	return metadata, nil
}

func validateStateRootMetadata(metadata stateRootMetadata) error {
	if !metadata.isDir || metadata.mode.Perm() != 0o700 || metadata.uid != uint32(os.Geteuid()) {
		return fmt.Errorf("%w: root must be an owner-only directory", ErrStateStoreAuthentication)
	}
	return nil
}

func (s *StateStore) authenticateRoot() error {
	current, err := authenticateStateRoot(s.root)
	if err != nil {
		return err
	}
	if current.dev != s.identity.dev || current.ino != s.identity.ino {
		return fmt.Errorf("%w: root descriptor identity changed", ErrStateStoreAuthentication)
	}
	return nil
}

func (s *StateStore) openLock() (*os.File, error) {
	rootFD := int(s.root.Fd())
	fd, err := stateOpenat(rootFD, stateLockFileName, syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	created := err == nil
	if errors.Is(err, syscall.EEXIST) {
		fd, err = stateOpenat(rootFD, stateLockFileName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	if created {
		if err := syscall.Fchmod(fd, 0o600); err != nil {
			_ = syscall.Close(fd)
			return nil, fmt.Errorf("chmod state lock: %w", err)
		}
	}
	if err := authenticateStateFile(fd, 0o600); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("authenticate state lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), stateLockFileName)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("retain state lock descriptor")
	}
	return file, nil
}

func (s *StateStore) authenticateLockBinding(lock *os.File) error {
	if lock == nil {
		return fmt.Errorf("%w: missing state lock", ErrStateStoreAuthentication)
	}
	currentFD, err := stateOpenat(int(s.root.Fd()), stateLockFileName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("%w: reopen state lock binding", ErrStateStoreAuthentication)
	}
	defer func() { _ = syscall.Close(currentFD) }()
	if err := authenticateStateFile(currentFD, 0o600); err != nil {
		return fmt.Errorf("%w: current state lock", ErrStateStoreAuthentication)
	}
	var held syscall.Stat_t
	var current syscall.Stat_t
	if err := syscall.Fstat(int(lock.Fd()), &held); err != nil {
		return fmt.Errorf("%w: stat held state lock", ErrStateStoreAuthentication)
	}
	if err := syscall.Fstat(currentFD, &current); err != nil {
		return fmt.Errorf("%w: stat current state lock", ErrStateStoreAuthentication)
	}
	if held.Dev != current.Dev || held.Ino != current.Ino {
		return fmt.Errorf("%w: state lock binding changed", ErrStateStoreAuthentication)
	}
	return nil
}

func acquireStateLock(ctx context.Context, fd int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EINTR) {
			return fmt.Errorf("lock worktree state: %w", err)
		}
		timer := time.NewTimer(stateLockRetryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *StateStore) readCanonical() (State, []byte, error) {
	fd, err := stateOpenat(int(s.root.Fd()), stateFileName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ENOENT) {
		state := State{SchemaVersion: StateSchemaVersion}
		canonical, marshalErr := MarshalState(state)
		return state, canonical, marshalErr
	}
	if err != nil {
		return State{}, nil, fmt.Errorf("open canonical state: %w", err)
	}
	file := os.NewFile(uintptr(fd), stateFileName)
	if file == nil {
		_ = syscall.Close(fd)
		return State{}, nil, errors.New("retain canonical state descriptor")
	}
	defer func() { _ = file.Close() }()
	if err := authenticateStateFile(fd, 0o600); err != nil {
		return State{}, nil, fmt.Errorf("%w: %v", ErrStateStoreCorrupt, err)
	}
	payload, err := io.ReadAll(io.LimitReader(file, MaxStateBytes+1))
	if err != nil {
		return State{}, nil, fmt.Errorf("read canonical state: %w", err)
	}
	if len(payload) > MaxStateBytes {
		return State{}, nil, fmt.Errorf("%w: canonical state exceeds limit", ErrStateStoreCorrupt)
	}
	state, err := UnmarshalState(payload)
	if err != nil {
		return State{}, nil, fmt.Errorf("%w: %v", ErrStateStoreCorrupt, err)
	}
	return state, payload, nil
}

func authenticateStateFile(fd int, mode os.FileMode) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || os.FileMode(stat.Mode&0o777).Perm() != mode.Perm() || stat.Uid != uint32(os.Geteuid()) {
		return ErrStateStoreAuthentication
	}
	return nil
}

func (s *StateStore) replaceCanonical(payload []byte) (returnErr error) {
	name, err := randomStateTemporaryName()
	if err != nil {
		return err
	}
	rootFD := int(s.root.Fd())
	fd, err := stateOpenat(rootFD, name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create state temporary: %w", err)
	}
	temporaryExists := true
	defer func() {
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
		if temporaryExists {
			_ = stateUnlinkat(rootFD, name, 0)
		}
	}()
	if err := syscall.Fchmod(fd, 0o600); err != nil {
		return fmt.Errorf("chmod state temporary: %w", err)
	}
	if err := authenticateStateFile(fd, 0o600); err != nil {
		return fmt.Errorf("authenticate state temporary: %w", err)
	}
	if err := s.writeAll(fd, payload); err != nil {
		return fmt.Errorf("write state temporary: %w", err)
	}
	if err := s.syncFile(fd); err != nil {
		return fmt.Errorf("sync state temporary: %w", err)
	}
	if err := syscall.Close(fd); err != nil {
		fd = -1
		return fmt.Errorf("close state temporary: %w", err)
	}
	fd = -1
	if s.faults.beforeRename != nil {
		if err := s.faults.beforeRename(); err != nil {
			return err
		}
	}
	if err := s.rename(rootFD, name, rootFD, stateFileName); err != nil {
		return fmt.Errorf("replace canonical state: %w", err)
	}
	temporaryExists = false
	if s.faults.afterRename != nil {
		if err := s.faults.afterRename(); err != nil {
			return err
		}
	}
	if err := s.syncDirectory(rootFD); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	if s.faults.afterDirSync != nil {
		if err := s.faults.afterDirSync(); err != nil {
			return err
		}
	}
	return nil
}

func randomStateTemporaryName() (string, error) {
	var nonce [16]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("generate state temporary name: %w", err)
	}
	return stateTemporaryPrefix + hex.EncodeToString(nonce[:]), nil
}

func (s *StateStore) writeAll(fd int, payload []byte) error {
	write := syscall.Write
	if s.faults.write != nil {
		write = s.faults.write
	}
	for len(payload) != 0 {
		written, err := write(fd, payload)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if written <= 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func (s *StateStore) syncFile(fd int) error {
	if s.faults.fileSync != nil {
		return s.faults.fileSync(fd)
	}
	return syscall.Fsync(fd)
}

func (s *StateStore) rename(oldRoot int, oldName string, newRoot int, newName string) error {
	if s.faults.rename != nil {
		return s.faults.rename(oldRoot, oldName, newRoot, newName)
	}
	return stateRenameat(oldRoot, oldName, newRoot, newName)
}

func (s *StateStore) syncDirectory(fd int) error {
	if s.faults.dirSync != nil {
		return s.faults.dirSync(fd)
	}
	return syscall.Fsync(fd)
}

// appendForensic emits only causal counts. It is deliberately not required to
// reconstruct state and never includes captured values, conflicts, or errors.
func (s *StateStore) appendForensic(state State) error {
	payload := []byte(fmt.Sprintf("revision=%d events=%d shells=%d receipts=%d\n", state.HeadRevision, len(state.Events), len(state.Shells), len(state.OperationReceipts)))
	rootFD := int(s.root.Fd())
	fd, err := stateOpenat(rootFD, stateForensicFileName, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	created := err == nil
	if errors.Is(err, syscall.EEXIST) {
		fd, err = stateOpenat(rootFD, stateForensicFileName, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	}
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(fd) }()
	if created {
		if err := syscall.Fchmod(fd, 0o600); err != nil {
			return err
		}
	}
	if err := authenticateStateFile(fd, 0o600); err != nil {
		return err
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Size+int64(len(payload)) > maxStateForensicFileBytes {
		if err := syscall.Ftruncate(fd, 0); err != nil {
			return err
		}
		if _, err := syscall.Seek(fd, 0, io.SeekStart); err != nil {
			return err
		}
	}
	write := syscall.Write
	if s.faults.forensicAppend != nil {
		write = s.faults.forensicAppend
	}
	for len(payload) != 0 {
		written, err := write(fd, payload)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if written <= 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return syscall.Fsync(fd)
}

// The standard syscall package exposes these descriptor-relative operations
// unevenly across Linux and Darwin. Keep the wrappers here so neither target
// falls back to reopening an authenticated path.
func stateOpenat(fd int, path string, flags int, perm uint32) (int, error) {
	pointer, err := syscall.BytePtrFromString(path)
	if err != nil {
		return -1, err
	}
	number, _, _, supported := stateAtSyscallNumbers()
	if !supported {
		return -1, syscall.ENOSYS
	}
	opened, _, callErr := syscall.Syscall6(number, uintptr(fd), uintptr(unsafe.Pointer(pointer)), uintptr(flags), uintptr(perm), 0, 0)
	if callErr != 0 {
		return -1, callErr
	}
	return int(opened), nil
}

func stateUnlinkat(fd int, path string, flags int) error {
	pointer, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, number, _, supported := stateAtSyscallNumbers()
	if !supported {
		return syscall.ENOSYS
	}
	_, _, callErr := syscall.Syscall6(number, uintptr(fd), uintptr(unsafe.Pointer(pointer)), uintptr(flags), 0, 0, 0)
	if callErr != 0 {
		return callErr
	}
	return nil
}

func stateRenameat(oldFD int, oldPath string, newFD int, newPath string) error {
	oldPointer, err := syscall.BytePtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPointer, err := syscall.BytePtrFromString(newPath)
	if err != nil {
		return err
	}
	_, _, number, supported := stateAtSyscallNumbers()
	if !supported {
		return syscall.ENOSYS
	}
	_, _, callErr := syscall.Syscall6(number, uintptr(oldFD), uintptr(unsafe.Pointer(oldPointer)), uintptr(newFD), uintptr(unsafe.Pointer(newPointer)), 0, 0)
	if callErr != 0 {
		return callErr
	}
	return nil
}

func stateAtSyscallNumbers() (openat, unlinkat, renameat uintptr, supported bool) {
	if runtime.GOOS == "darwin" {
		return 463, 472, 465, true
	}
	switch runtime.GOARCH {
	case "amd64":
		return 257, 263, 264, true
	case "arm64":
		return 56, 35, 38, true
	case "386":
		return 295, 301, 302, true
	case "arm":
		return 322, 328, 329, true
	case "ppc64", "ppc64le":
		return 286, 292, 293, true
	case "s390x":
		return 288, 294, 295, true
	case "mips", "mipsle":
		return 4288, 4294, 4295, true
	case "mips64", "mips64le":
		return 5247, 5253, 5254, true
	case "riscv64", "loong64":
		// These architectures expose renameat2; the fifth argument is the
		// zero flags value already supplied by stateRenameat.
		return 56, 35, 276, true
	default:
		return 0, 0, 0, false
	}
}
