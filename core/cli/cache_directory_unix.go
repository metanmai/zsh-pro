//go:build linux || darwin

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// secureCacheDirectory walks every component by descriptor with O_NOFOLLOW.
// The loader-cache root is generated code sourced by every shell, so unlike a
// user-selected .zshrc it must never inherit pathname symlink compatibility.
// All later cache operations consume the retained descriptors rather than the
// mutable path spelling.
func secureCacheDirectory(path string) (*cacheDirectoryState, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || path == "/" {
		return nil, errors.New("cached loader directory must name a private absolute directory")
	}

	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open cached loader anchor: %w", err)
	}
	parentFD := -1
	keepFinal := false
	createdFinal := false
	finalName := ""
	defer func() {
		if !keepFinal {
			if createdFinal && parentFD >= 0 && fd >= 0 {
				_ = removeCacheDirectoryIfSame(parentFD, finalName, fd)
			}
		}
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
		if parentFD >= 0 {
			_ = syscall.Close(parentFD)
		}
	}()

	euid := uint32(os.Geteuid())
	var anchor syscall.Stat_t
	if err := syscall.Fstat(fd, &anchor); err != nil {
		return nil, fmt.Errorf("stat cached loader anchor: %w", err)
	}
	if err := validateCacheAncestor(anchor, euid); err != nil {
		return nil, fmt.Errorf("cached loader anchor: %w", err)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("cached loader directory contains an invalid path component")
		}
		if i == len(parts)-1 {
			parentFD, err = syscall.Dup(fd)
			if err != nil {
				return nil, fmt.Errorf("duplicate cached loader parent: %w", err)
			}
			finalName = part
		}

		next, err := runtimeOpenat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		created := false
		if errors.Is(err, syscall.ENOENT) {
			if mkdirErr := cacheMkdirat(fd, part, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, syscall.EEXIST) {
				return nil, fmt.Errorf("create cached loader component %q: %w", part, mkdirErr)
			} else if mkdirErr == nil {
				created = true
				if i == len(parts)-1 {
					createdFinal = true
				}
			}
			next, err = runtimeOpenat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		}
		if err != nil {
			return nil, fmt.Errorf("open cached loader component %q without following links: %w", part, err)
		}
		if err := syscall.Close(fd); err != nil {
			_ = syscall.Close(next)
			return nil, fmt.Errorf("close cached loader parent: %w", err)
		}
		fd = next

		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			return nil, fmt.Errorf("stat cached loader component %q: %w", part, err)
		}
		if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
			return nil, fmt.Errorf("cached loader component %q is not a directory", part)
		}
		if created {
			if err := syscall.Fchmod(fd, 0o700); err != nil {
				return nil, fmt.Errorf("secure created cached loader component %q: %w", part, err)
			}
		}
		if i != len(parts)-1 {
			if err := validateCacheAncestor(stat, euid); err != nil {
				return nil, err
			}
			continue
		}

		if stat.Uid != euid {
			return nil, errors.New("cached loader directory is not owned by the current user")
		}
		originalMode := os.FileMode(stat.Mode & 0o777)

		directory := os.NewFile(uintptr(fd), "zsh-pro cached loader directory")
		if directory == nil {
			return nil, errors.New("retain cached loader descriptors")
		}
		fd = -1
		parent := os.NewFile(uintptr(parentFD), "zsh-pro cached loader parent")
		if parent == nil {
			_ = directory.Close()
			return nil, errors.New("retain cached loader descriptors")
		}
		parentFD = -1
		keepFinal = true
		return &cacheDirectoryState{
			path:         path,
			directory:    directory,
			parent:       parent,
			name:         finalName,
			existed:      !createdFinal,
			created:      createdFinal,
			originalMode: originalMode,
		}, nil
	}
	return nil, errors.New("cached loader directory has no components")
}

func validateCacheAncestor(stat syscall.Stat_t, euid uint32) error {
	if stat.Uid != euid && stat.Uid != 0 {
		return errors.New("cached loader ancestor has an untrusted owner")
	}
	if stat.Mode&0o022 != 0 && stat.Mode&0o1000 == 0 {
		return errors.New("cached loader ancestor is writable without sticky protection")
	}
	return nil
}

func (s *cacheDirectoryState) prepareLoaderRollback() (cacheWriteRollback, error) {
	original, err := s.openExistingRegular(cacheLoaderName)
	if errors.Is(err, syscall.ENOENT) {
		if err := s.secureForWrite(); err != nil {
			return cacheWriteRollback{}, err
		}
		return cacheWriteRollback{directory: s, remove: true}, nil
	}
	if err != nil {
		return cacheWriteRollback{}, fmt.Errorf("inspect existing cached loader: %w", err)
	}
	defer func() { _ = original.Close() }()

	info, err := original.Stat()
	if err != nil {
		return cacheWriteRollback{}, fmt.Errorf("stat existing cached loader: %w", err)
	}
	content, err := io.ReadAll(original)
	if err != nil {
		return cacheWriteRollback{}, fmt.Errorf("read existing cached loader: %w", err)
	}
	// Do not chmod the root until the existing loader has passed the no-follow
	// regular-file check above. Once the descriptor-bound directory is private,
	// staging the rollback copy cannot be redirected by a path replacement.
	if err := s.secureForWrite(); err != nil {
		return cacheWriteRollback{}, err
	}
	temp, file, err := s.writeTemp(content, info.Mode().Perm())
	if err != nil {
		return cacheWriteRollback{}, fmt.Errorf("stage cached loader rollback: %w", err)
	}
	if err := file.Close(); err != nil {
		s.discardTemp(temp)
		return cacheWriteRollback{}, fmt.Errorf("close cached loader rollback: %w", err)
	}
	return cacheWriteRollback{directory: s, temp: temp}, nil
}

func (s *cacheDirectoryState) prepareValidatedLoader(content []byte) (preparedCacheWrite, error) {
	temp, source, err := s.writeTemp(content, 0o600)
	if err != nil {
		return preparedCacheWrite{}, err
	}
	candidatePath := filepath.Join(s.path, temp)
	if err := validateZsh(source, candidatePath); err != nil {
		_ = source.Close()
		s.discardTemp(temp)
		return preparedCacheWrite{}, err
	}
	if err := source.Close(); err != nil {
		s.discardTemp(temp)
		return preparedCacheWrite{}, fmt.Errorf("close validated cached loader: %w", err)
	}
	return preparedCacheWrite{directory: s, temp: temp}, nil
}

// secureForWrite delays the final-root chmod until after prepareLoaderRollback
// has rejected an unsafe existing loader. A loader symlink must not cause even
// a transient mode mutation of its containing cache directory.
func (s *cacheDirectoryState) secureForWrite() error {
	if s == nil || s.directory == nil {
		return errors.New("cached loader directory descriptor is unavailable")
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(s.directoryFD(), &stat); err != nil {
		return fmt.Errorf("stat cached loader directory: %w", err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return errors.New("cached loader directory is no longer a directory")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return errors.New("cached loader directory is not owned by the current user")
	}
	if stat.Mode&0o077 == 0 {
		return nil
	}
	if err := s.directory.Chmod(0o700); err != nil {
		return fmt.Errorf("secure cached loader directory: %w", err)
	}
	s.modeChanged = true
	if err := s.directory.Sync(); err != nil {
		return fmt.Errorf("sync cached loader directory mode: %w", err)
	}
	if err := syscall.Fstat(s.directoryFD(), &stat); err != nil {
		return fmt.Errorf("restat cached loader directory: %w", err)
	}
	if stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return errors.New("cached loader directory is not private")
	}
	return nil
}

func (s *cacheDirectoryState) openExistingRegular(name string) (*os.File, error) {
	fd, err := runtimeOpenat(s.directoryFD(), name, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "zsh-pro cached loader")
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("retain cached loader descriptor")
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		_ = file.Close()
		return nil, errors.New("cached loader is not a regular file")
	}
	return file, nil
}

func (s *cacheDirectoryState) verifyLoaderTarget() error {
	file, err := s.openExistingRegular(cacheLoaderName)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("recheck cached loader before promotion: %w", err)
	}
	return file.Close()
}

func (s *cacheDirectoryState) writeTemp(content []byte, mode os.FileMode) (string, *os.File, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name, err := newCacheTempName()
		if err != nil {
			return "", nil, err
		}
		fd, err := runtimeOpenat(s.directoryFD(), name, syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		file := os.NewFile(uintptr(fd), "zsh-pro cached loader staging")
		if file == nil {
			_ = syscall.Close(fd)
			s.discardTemp(name)
			return "", nil, errors.New("retain cached loader staging descriptor")
		}
		cleanup := func() {
			_ = file.Close()
			s.discardTemp(name)
		}
		if err := file.Chmod(mode); err != nil {
			cleanup()
			return "", nil, err
		}
		if n, err := file.Write(content); err != nil {
			cleanup()
			return "", nil, err
		} else if n != len(content) {
			cleanup()
			return "", nil, io.ErrShortWrite
		}
		if err := file.Sync(); err != nil {
			cleanup()
			return "", nil, err
		}
		return name, file, nil
	}
	return "", nil, errors.New("create unique cached loader staging file")
}

func newCacheTempName() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return ".zsh-pro-loader-" + hex.EncodeToString(bytes), nil
}

func (w *preparedCacheWrite) promote() error {
	if w == nil || w.directory == nil || w.temp == "" {
		return errors.New("cached loader is not prepared")
	}
	if err := w.directory.verifyLoaderTarget(); err != nil {
		return err
	}
	if err := cacheRenameat(w.directory.directoryFD(), w.temp, w.directory.directoryFD(), cacheLoaderName); err != nil {
		return err
	}
	w.temp = ""
	w.promoted = true
	return w.directory.directory.Sync()
}

func (w *preparedCacheWrite) discard() {
	if w == nil || w.directory == nil || w.temp == "" {
		return
	}
	w.directory.discardTemp(w.temp)
	w.temp = ""
}

func (r *cacheWriteRollback) restore() error {
	if r == nil || r.directory == nil {
		return errors.New("cached loader rollback is unavailable")
	}
	if r.remove {
		if err := cacheUnlinkat(r.directory.directoryFD(), cacheLoaderName); err != nil && !errors.Is(err, syscall.ENOENT) {
			return err
		}
		return r.directory.directory.Sync()
	}
	if r.temp == "" {
		return errors.New("cached loader rollback is not prepared")
	}
	if err := cacheRenameat(r.directory.directoryFD(), r.temp, r.directory.directoryFD(), cacheLoaderName); err != nil {
		return err
	}
	r.temp = ""
	return r.directory.directory.Sync()
}

func (r *cacheWriteRollback) discard() {
	if r == nil || r.directory == nil || r.temp == "" {
		return
	}
	r.directory.discardTemp(r.temp)
	r.temp = ""
}

func rollbackPromotedCache(write *preparedCacheWrite, rollback *cacheWriteRollback) error {
	if write == nil || !write.promoted {
		return nil
	}
	return rollback.restore()
}

func (s *cacheDirectoryState) rollback() error {
	if s == nil {
		return nil
	}
	if s.existed && s.modeChanged {
		if err := s.directory.Chmod(s.originalMode); err != nil {
			return err
		}
		return s.directory.Sync()
	}
	if !s.created {
		return nil
	}
	if err := removeCacheDirectoryIfSame(s.parentFD(), s.name, s.directoryFD()); err != nil {
		return err
	}
	return s.parent.Sync()
}

// removeCacheDirectoryIfSame refuses to remove a replacement that appeared at
// the final path after we retained the original descriptor. Descriptor-relative
// unlink alone would avoid following a symlink but could still delete an empty
// substituted directory; compare inode/device first.
func removeCacheDirectoryIfSame(parentFD int, name string, expectedFD int) error {
	currentFD, err := runtimeOpenat(parentFD, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reopen cached loader directory for rollback: %w", err)
	}
	defer func() { _ = syscall.Close(currentFD) }()

	var expected, current syscall.Stat_t
	if err := syscall.Fstat(expectedFD, &expected); err != nil {
		return fmt.Errorf("stat retained cached loader directory: %w", err)
	}
	if err := syscall.Fstat(currentFD, &current); err != nil {
		return fmt.Errorf("stat current cached loader directory: %w", err)
	}
	if expected.Dev != current.Dev || expected.Ino != current.Ino {
		return errors.New("cached loader directory changed during installation")
	}
	if err := cacheRemoveDirectoryAt(parentFD, name); err != nil && !errors.Is(err, syscall.ENOENT) {
		return err
	}
	return nil
}

func (s *cacheDirectoryState) close() error {
	if s == nil {
		return nil
	}
	var first error
	if s.directory != nil {
		if err := s.directory.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			first = err
		}
		s.directory = nil
	}
	if s.parent != nil {
		if err := s.parent.Close(); err != nil && !errors.Is(err, os.ErrClosed) && first == nil {
			first = err
		}
		s.parent = nil
	}
	return first
}

func (s *cacheDirectoryState) directoryFD() int {
	return int(s.directory.Fd())
}

func (s *cacheDirectoryState) parentFD() int {
	return int(s.parent.Fd())
}

func (s *cacheDirectoryState) discardTemp(name string) {
	if name == "" || s == nil || s.directory == nil {
		return
	}
	_ = cacheUnlinkat(s.directoryFD(), name)
}
