package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/shell"
)

const (
	installBegin = "# >>> zsh-pro >>>"
	installEnd   = "# <<< zsh-pro <<<"

	validationTimeout = 5 * time.Second
)

type installPaths struct {
	runtimeDir string
	zshrcPath  string
}

// runInstall prepares and fsyncs both final-target replacements before either
// one is promoted. Loader promotion precedes .zshrc promotion so the bootstrap
// never points at an unavailable loader; if the second promotion fails, the
// prepared original loader is atomically restored.
func runInstall(provider shell.Hooker) error {
	return runInstallWithStoreInitialization(provider, nil)
}

// runInstallWithStoreInitialization validates the user-owned bootstrap first,
// then initializes the profile store as part of the same transaction as the
// loader and .zshrc replacements. Every later failure invokes the initializer's
// narrowly scoped compensation before returning an installation error.
func runInstallWithStoreInitialization(provider shell.Hooker, initializeStore StoreInitializer) error {
	paths, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(paths.zshrcPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", paths.zshrcPath, err)
	}
	next, err := replaceManagedBlock(current, renderInstallBlock())
	if err != nil {
		return err
	}
	var storeInitialization StoreInitialization
	if initializeStore != nil {
		storeInitialization, err = initializeStore(context.Background())
		if err != nil {
			return installTransactionError("initialize profile store", err, rollbackStoreInitialization(storeInitialization))
		}
	}
	rcWrite, err := prepareAtomicWrite(paths.zshrcPath, next, 0o644)
	if err != nil {
		return installTransactionError("prepare "+paths.zshrcPath, err, rollbackStoreInitialization(storeInitialization))
	}
	rcRollback, err := prepareWriteRollback(rcWrite.target)
	if err != nil {
		rcWrite.discard()
		return installTransactionError("prepare rollback for "+paths.zshrcPath, err, rollbackStoreInitialization(storeInitialization))
	}

	runtimeState, err := secureRuntimeDirectory(paths.runtimeDir)
	if err != nil {
		rcWrite.discard()
		rcRollback.discard()
		return installTransactionError("create runtime directory", err, rollbackStoreInitialization(storeInitialization))
	}
	loader := filepath.Join(paths.runtimeDir, "loader.zsh")
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + provider.HookScript()
	loaderWrite, err := prepareValidatedLoader(loader, []byte(loaderScript))
	if err != nil {
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr := rollbackRuntimeAndStore(runtimeState, storeInitialization)
		return installTransactionError("install cached loader", err, rollbackErr)
	}
	loaderRollback, err := prepareWriteRollback(loaderWrite.target)
	if err != nil {
		loaderWrite.discard()
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr := rollbackRuntimeAndStore(runtimeState, storeInitialization)
		return installTransactionError("prepare loader rollback", err, rollbackErr)
	}

	if err := loaderWrite.promote(); err != nil {
		rollbackErr := rollbackPromoted(&loaderWrite, &loaderRollback)
		loaderWrite.discard()
		loaderRollback.discard()
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr = errors.Join(rollbackErr, rollbackRuntimeAndStore(runtimeState, storeInitialization))
		return installTransactionError("install cached loader", err, rollbackErr)
	}
	if err := rcWrite.promote(); err != nil {
		rollbackErr := errors.Join(
			rollbackPromoted(&rcWrite, &rcRollback),
			rollbackPromoted(&loaderWrite, &loaderRollback),
		)
		rcWrite.discard()
		rcRollback.discard()
		loaderWrite.discard()
		loaderRollback.discard()
		rollbackErr = errors.Join(rollbackErr, rollbackRuntimeAndStore(runtimeState, storeInitialization))
		return installTransactionError("write "+paths.zshrcPath, err, rollbackErr)
	}

	rcRollback.discard()
	loaderRollback.discard()
	return nil
}

func rollbackStoreInitialization(initialization StoreInitialization) error {
	if initialization.Rollback == nil {
		return nil
	}
	return initialization.Rollback()
}

// rollbackRuntimeAndStore restores the loader/cache state before a pre-existing
// store migration so its original mode is the final visible state. When an
// explicit ZSHPRO_HOME made the store root and runtime directory the same newly
// created path, remove the store first; a successful store rollback owns and
// removes the shared directory without ever deleting pre-existing data.
func rollbackRuntimeAndStore(runtime runtimeDirectoryState, initialization StoreInitialization) error {
	if initialization.CreatedPath != "" && filepath.Clean(initialization.CreatedPath) == filepath.Clean(runtime.path) {
		storeErr := rollbackStoreInitialization(initialization)
		if storeErr == nil {
			// The created store owned the shared root, so its successful rollback
			// already removed the runtime directory. Calling runtime.rollback here
			// would turn a complete restoration into a spurious ENOENT error.
			return nil
		}
		return errors.Join(storeErr, runtime.rollback())
	}
	return errors.Join(runtime.rollback(), rollbackStoreInitialization(initialization))
}

func (c *CLI) runInstall(stdout, stderr io.Writer) int {
	provider, code := c.providerOrFail(stdout, stderr, false)
	if code != 0 {
		return code
	}
	if err := runInstallWithStoreInitialization(provider, c.storeInitializer); err != nil {
		return c.fail(stdout, stderr, false, err.Error())
	}
	_, _ = fmt.Fprintln(stdout, "zsh-pro: installed")
	return 0
}

func resolveInstallPaths() (installPaths, error) {
	home, present := os.LookupEnv("HOME")
	if !present || home == "" || !filepath.IsAbs(home) {
		return installPaths{}, errors.New("install requires a non-empty absolute HOME")
	}

	runtime := filepath.Join(home, ".zsh-pro")
	if configured, present := os.LookupEnv("ZSHPRO_HOME"); present {
		if configured == "" || !filepath.IsAbs(configured) {
			return installPaths{}, errors.New("install requires ZSHPRO_HOME to be a non-empty absolute path when set")
		}
		runtime = configured
	}

	zdotdir := home
	if configured, present := os.LookupEnv("ZDOTDIR"); present {
		if configured == "" || !filepath.IsAbs(configured) {
			return installPaths{}, errors.New("install requires ZDOTDIR to be a non-empty absolute path when set")
		}
		zdotdir = configured
	}

	return installPaths{
		runtimeDir: runtime,
		zshrcPath:  filepath.Join(zdotdir, ".zshrc"),
	}, nil
}

func renderInstallBlock() []byte {
	return []byte(installBegin + "\n" +
		"if [[ -z \"${ZSHPRO_DISABLE-}\" ]]; then\n" +
		"  if [[ -f \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\" && -r \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\" ]]; then\n" +
		"    if source \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\"; then\n" +
		"      :\n" +
		"    else\n" +
		"      :\n" +
		"    fi\n" +
		"  fi\n" +
		"fi\n" +
		"true\n" +
		installEnd)
}

// replaceManagedBlock is deliberately byte-oriented: all text outside complete
// marker regions is copied verbatim. Any malformed marker sequence is rejected
// before a caller can write, preventing accidental truncation to EOF.
func replaceManagedBlock(current, block []byte) ([]byte, error) {
	type region struct{ start, end int }
	var regions []region
	open := -1
	for lineStart := 0; lineStart < len(current); {
		lineEnd := len(current)
		terminatorEnd := len(current)
		if newline := bytes.IndexByte(current[lineStart:], '\n'); newline >= 0 {
			lineEnd = lineStart + newline
			terminatorEnd = lineEnd + 1
		}

		comparisonEnd := lineEnd
		if terminatorEnd > lineEnd && comparisonEnd > lineStart && current[comparisonEnd-1] == '\r' {
			comparisonEnd--
		}
		line := current[lineStart:comparisonEnd]

		switch {
		case bytes.Equal(line, []byte(installBegin)):
			if open >= 0 {
				return nil, errors.New("refusing to edit .zshrc: nested or interleaved BEGIN markers")
			}
			open = lineStart
		case bytes.Equal(line, []byte(installEnd)):
			if open < 0 {
				return nil, errors.New("refusing to edit .zshrc: END marker has no preceding BEGIN marker")
			}
			// Keep the END line's terminator outside the region so the rendered
			// block remains a complete physical line without normalizing it.
			regions = append(regions, region{start: open, end: comparisonEnd})
			open = -1
		}

		lineStart = terminatorEnd
	}
	if open >= 0 {
		return nil, errors.New("refusing to edit .zshrc: BEGIN marker has no END marker")
	}
	if len(regions) == 0 {
		if len(current) == 0 {
			return append(append([]byte(nil), block...), '\n'), nil
		}
		out := append([]byte(nil), current...)
		if !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		out = append(out, '\n')
		return append(out, block...), nil
	}

	var out bytes.Buffer
	last := 0
	for i, region := range regions {
		out.Write(current[last:region.start])
		if i == 0 {
			out.Write(block)
		}
		last = region.end
	}
	out.Write(current[last:])
	return out.Bytes(), nil
}

type preparedWrite struct {
	target   string
	temp     string
	promoted bool
}

// prepareAtomicWrite resolves the real target and fsyncs its sibling temporary
// file without changing the target. The caller decides when to promote it.
func prepareAtomicWrite(path string, content []byte, defaultMode os.FileMode) (preparedWrite, error) {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return preparedWrite{}, err
	}
	mode := defaultMode
	info, statErr := os.Stat(target)
	if statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return preparedWrite{}, statErr
	}
	return prepareWriteTarget(target, content, mode)
}

// prepareValidatedLoader prepares a mode-0600 loader replacement only after
// validating the exact temporary bytes that would be promoted.
func prepareValidatedLoader(path string, content []byte) (preparedWrite, error) {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return preparedWrite{}, err
	}
	prepared, err := prepareWriteTarget(target, content, 0o600)
	if err != nil {
		return preparedWrite{}, err
	}
	if err := validateZsh(prepared.temp); err != nil {
		prepared.discard()
		return preparedWrite{}, err
	}
	return prepared, nil
}

func prepareWriteTarget(target string, content []byte, mode os.FileMode) (preparedWrite, error) {
	temp, err := writeTemp(target, content, mode)
	if err != nil {
		return preparedWrite{}, err
	}
	return preparedWrite{target: target, temp: temp}, nil
}

func (w *preparedWrite) promote() error {
	if w.temp == "" {
		return errors.New("write target is not prepared")
	}
	if err := os.Rename(w.temp, w.target); err != nil {
		return err
	}
	w.temp = ""
	w.promoted = true
	return syncDirectory(filepath.Dir(w.target))
}

func (w *preparedWrite) discard() {
	if w == nil || w.temp == "" {
		return
	}
	_ = os.Remove(w.temp)
	w.temp = ""
}

type writeRollback struct {
	target string
	temp   string
	remove bool
}

// prepareWriteRollback captures the exact original target in a sibling temp
// before a transaction can replace it. A missing original is restored by
// removing the newly promoted target instead.
func prepareWriteRollback(target string) (writeRollback, error) {
	info, err := os.Stat(target)
	if errors.Is(err, os.ErrNotExist) {
		return writeRollback{target: target, remove: true}, nil
	}
	if err != nil {
		return writeRollback{}, err
	}
	if !info.Mode().IsRegular() {
		return writeRollback{}, fmt.Errorf("refusing to transactionally replace non-regular file %s", target)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return writeRollback{}, err
	}
	temp, err := writeTemp(target, content, info.Mode().Perm())
	if err != nil {
		return writeRollback{}, err
	}
	return writeRollback{target: target, temp: temp}, nil
}

func (r *writeRollback) restore() error {
	if r.remove {
		if err := os.Remove(r.target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return syncDirectory(filepath.Dir(r.target))
	}
	if r.temp == "" {
		return errors.New("rollback target is not prepared")
	}
	if err := os.Rename(r.temp, r.target); err != nil {
		return err
	}
	r.temp = ""
	return syncDirectory(filepath.Dir(r.target))
}

func (r *writeRollback) discard() {
	if r == nil || r.temp == "" {
		return
	}
	_ = os.Remove(r.temp)
	r.temp = ""
}

func rollbackPromoted(write *preparedWrite, rollback *writeRollback) error {
	if write == nil || !write.promoted {
		return nil
	}
	return rollback.restore()
}

type runtimeDirectoryState struct {
	path    string
	existed bool
	mode    os.FileMode
}

func secureRuntimeDirectory(path string) (runtimeDirectoryState, error) {
	state := runtimeDirectoryState{path: path}
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return state, fmt.Errorf("%s is not a directory", path)
		}
		state.existed = true
		state.mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return state, err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		if state.existed {
			_ = os.Chmod(path, state.mode)
		} else {
			_ = os.Remove(path)
		}
		return state, err
	}
	return state, nil
}

func (s runtimeDirectoryState) rollback() error {
	if s.existed {
		return os.Chmod(s.path, s.mode)
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func installTransactionError(operation string, cause, rollbackErr error) error {
	if rollbackErr == nil {
		return fmt.Errorf("%s: %w", operation, cause)
	}
	return fmt.Errorf("%s: %w; rollback failed: %v", operation, cause, rollbackErr)
}

func resolveWriteTarget(path string) (string, error) {
	target := path
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		return target, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return target, nil
}

func writeTemp(target string, content []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".zsh-pro-loader-*")
	if err != nil {
		return "", err
	}
	temp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(temp)
	}
	if err := f.Chmod(mode); err != nil {
		cleanup()
		return "", err
	}
	if n, err := f.Write(content); err != nil {
		cleanup()
		return "", err
	} else if n != len(content) {
		cleanup()
		return "", io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return temp, nil
}

func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func validateZsh(path string) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("find zsh for loader validation: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, zsh, "-n", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("zsh -n validation timed out")
		}
		return fmt.Errorf("zsh -n: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
