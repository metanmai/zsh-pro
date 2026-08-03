package cli

import (
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
	prepared, err := prepareIngestInstallAt(paths.zshrcPath, renderInstallBlock())
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
	if err := validateInstallRootRelationship(paths.runtimeDir, storeInitialization); err != nil {
		return installTransactionError("validate profile store and runtime roots", err, rollbackStoreInitialization(storeInitialization))
	}
	targetMode := os.FileMode(0o644)
	if prepared.originalSnapshot.exists {
		targetMode = prepared.originalSnapshot.mode
	}
	rcWrite, err := prepareWriteTarget(prepared.originalSnapshot.resolvedPath, prepared.candidate, targetMode)
	if err != nil {
		return installTransactionError("prepare "+paths.zshrcPath, err, rollbackStoreInitialization(storeInitialization))
	}
	rcRollback, err := prepareWriteRollback(rcWrite.target)
	if err != nil {
		rcWrite.discard()
		return installTransactionError("prepare rollback for "+paths.zshrcPath, err, rollbackStoreInitialization(storeInitialization))
	}

	cacheState, err := secureCacheDirectory(paths.runtimeDir)
	if err != nil {
		rcWrite.discard()
		rcRollback.discard()
		return installTransactionError("create runtime directory", err, rollbackStoreInitialization(storeInitialization))
	}
	defer func() { _ = cacheState.close() }()
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + provider.HookScript()
	loaderRollback, err := cacheState.prepareLoaderRollback()
	if err != nil {
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr := rollbackRuntimeAndStore(cacheState, storeInitialization)
		return installTransactionError("prepare loader rollback", err, rollbackErr)
	}
	loaderWrite, err := cacheState.prepareValidatedLoader([]byte(loaderScript))
	if err != nil {
		loaderRollback.discard()
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr := rollbackRuntimeAndStore(cacheState, storeInitialization)
		return installTransactionError("install cached loader", err, rollbackErr)
	}

	if err := loaderWrite.promote(); err != nil {
		rollbackErr := rollbackPromotedCache(&loaderWrite, &loaderRollback)
		loaderWrite.discard()
		loaderRollback.discard()
		rcWrite.discard()
		rcRollback.discard()
		rollbackErr = errors.Join(rollbackErr, rollbackRuntimeAndStore(cacheState, storeInitialization))
		return installTransactionError("install cached loader", err, rollbackErr)
	}
	if err := rcWrite.promote(); err != nil {
		rollbackErr := errors.Join(
			rollbackPromoted(&rcWrite, &rcRollback),
			rollbackPromotedCache(&loaderWrite, &loaderRollback),
		)
		rcWrite.discard()
		rcRollback.discard()
		loaderWrite.discard()
		loaderRollback.discard()
		rollbackErr = errors.Join(rollbackErr, rollbackRuntimeAndStore(cacheState, storeInitialization))
		return installTransactionError("write "+paths.zshrcPath, err, rollbackErr)
	}
	if err := finalizeStoreInitialization(storeInitialization); err != nil {
		rollbackErr := errors.Join(
			rollbackPromoted(&rcWrite, &rcRollback),
			rollbackPromotedCache(&loaderWrite, &loaderRollback),
		)
		rcWrite.discard()
		rcRollback.discard()
		loaderWrite.discard()
		loaderRollback.discard()
		rollbackErr = errors.Join(rollbackErr, rollbackRuntimeAndStore(cacheState, storeInitialization))
		return installTransactionError("finalize profile store initialization", err, rollbackErr)
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

func finalizeStoreInitialization(initialization StoreInitialization) error {
	if initialization.Finalize == nil {
		return nil
	}
	return initialization.Finalize()
}

// rollbackRuntimeAndStore restores the loader/cache state before a pre-existing
// store migration so its original mode is the final visible state. When an
// explicit ZSHPRO_HOME made the store root and runtime directory the same newly
// created path, remove the store first; a successful store rollback owns and
// removes the shared directory without ever deleting pre-existing data.
func rollbackRuntimeAndStore(cache *cacheDirectoryState, initialization StoreInitialization) error {
	if cache == nil {
		return rollbackStoreInitialization(initialization)
	}
	return errors.Join(cache.rollback(), rollbackStoreInitialization(initialization))
}

func validateInstallRootRelationship(runtimeRoot string, initialization StoreInitialization) error {
	storeRoot := initialization.CanonicalRoot
	if storeRoot == "" {
		storeRoot = initialization.CreatedPath
	}
	if storeRoot == "" {
		return nil
	}
	canonicalRuntime, err := canonicalInstallRoot(runtimeRoot)
	if err != nil {
		return fmt.Errorf("canonicalize runtime root: %w", err)
	}
	canonicalStore, err := canonicalInstallRoot(storeRoot)
	if err != nil {
		return fmt.Errorf("canonicalize profile store root: %w", err)
	}
	if canonicalRuntime == canonicalStore {
		return nil
	}
	if installRootContains(canonicalRuntime, canonicalStore) || installRootContains(canonicalStore, canonicalRuntime) {
		return errors.New("profile store and runtime roots have an unsupported ancestor overlap")
	}
	return nil
}

func canonicalInstallRoot(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("install root must be an absolute path")
	}
	abs := filepath.Clean(path)
	cursor := abs
	var suffix []string
	for {
		if _, err := os.Lstat(cursor); err == nil {
			resolved, err := filepath.EvalSymlinks(cursor)
			if err != nil {
				return "", err
			}
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", errors.New("install root has no existing ancestor")
		}
		suffix = append(suffix, filepath.Base(cursor))
		cursor = parent
	}
}

func installRootContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil || relative == "." || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
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
	layout, err := scanZshrcMarkerTopology(current)
	if err != nil {
		return nil, err
	}
	return renderManagedCandidate(current, block, layout), nil
}

type preparedWrite struct {
	target   string
	temp     string
	promoted bool
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

func installTransactionError(operation string, cause, rollbackErr error) error {
	if rollbackErr == nil {
		return fmt.Errorf("%s: %w", operation, cause)
	}
	return fmt.Errorf("%s: %w; rollback failed: %v", operation, cause, rollbackErr)
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

// validateZsh parses the exact staged loader descriptor through /dev/fd/3.
// The candidate spelling is only a positional argument to zsh; its parser
// never opens that path, so a pathname replacement cannot change the bytes
// validated before promotion.
func validateZsh(source *os.File, candidatePath string) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("find zsh for loader validation: %w", err)
	}
	if source == nil {
		return errors.New("validated loader descriptor is unavailable")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind loader validation source: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, zsh, "-n", "/dev/fd/3", candidatePath)
	cmd.ExtraFiles = []*os.File{source}
	if out, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("zsh -n validation timed out")
		}
		return fmt.Errorf("zsh -n: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
