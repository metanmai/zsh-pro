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
)

// InstallInitialization is the narrowly scoped compensation returned by an
// explicit install. It distinguishes a root created by this invocation from an
// existing initialized repository whose private-mode migration may need to be
// reversed after a later bootstrap failure.
type InstallInitialization struct {
	state *installInitializationState
}

// Rollback restores exactly the state that InitForInstall changed. It is safe
// to call more than once; a newly created root is removed only when its complete
// filesystem tree still matches the tree this invocation created.
func (i InstallInitialization) Rollback() error {
	if i.state == nil {
		return nil
	}
	return i.state.rollback()
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
	return InstallInitialization{state: state}, nil
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
