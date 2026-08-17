//go:build unix

package worktree

import (
	"context"
	"errors"
	"os"
)

var ErrStateStoreClosed = errors.New("worktree state store is closed")

const (
	stateFileName     = "worktree.json"
	stateLockFileName = "worktree.lock"
)

type stateRootMetadata struct {
	isDir bool
	mode  os.FileMode
	uid   uint32
}

func validateStateRootMetadata(stateRootMetadata) error {
	return errors.New("state root authentication not implemented")
}

type stateStoreFaults struct {
	write        func(int, []byte) (int, error)
	fileSync     func(int) error
	rename       func(int, string, int, string) error
	dirSync      func(int) error
	beforeRename func() error
	afterRename  func() error
	afterDirSync func() error
}

// StateStore owns one authenticated runtime-root descriptor.
type StateStore struct {
	faults stateStoreFaults
}

func OpenStateStore(string) (*StateStore, error) {
	return nil, errors.New("state store not implemented")
}

func NewStateStoreFromAuthenticatedRoot(*os.File) (*StateStore, error) {
	return nil, errors.New("state store not implemented")
}

func (s *StateStore) Close() error {
	return errors.New("state store not implemented")
}

func (s *StateStore) Read(context.Context) (State, error) {
	return State{}, errors.New("state store not implemented")
}

func (s *StateStore) WithTransaction(context.Context, func(*State) error) error {
	return errors.New("state store not implemented")
}
