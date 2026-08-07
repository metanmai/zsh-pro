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

// runInstall performs the same guarded one-promotion transaction used by
// ingest. Loader promotion precedes .zshrc promotion so startup never points
// at an unavailable loader.
func runInstall(provider shell.Hooker) error {
	return runInstallWithStoreInitialization(provider, nil)
}

// runInstallWithStoreInitialization validates the user-owned bootstrap first,
// then initializes the profile store as part of the same transaction as the
// loader and .zshrc replacements. Every later failure invokes the initializer's
// narrowly scoped compensation before returning an installation error.
func runInstallWithStoreInitialization(provider shell.Hooker, initializeStore StoreInitializer) error {
	return runInstallWithStoreInitializationAndSeams(provider, initializeStore, nil)
}

func runInstallWithStoreInitializationAndSeams(
	provider shell.Hooker,
	initializeStore StoreInitializer,
	transactionSeams *installTransactionSeams,
) error {
	paths, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	prepared, err := prepareIngestInstallAt(paths.zshrcPath, renderInstallBlock())
	if err != nil {
		return err
	}
	if err := preflightAtomicRenameTarget(prepared.originalSnapshot.resolvedPath, transactionSeams); err != nil {
		return installTransactionError("verify atomic startup transaction", err, nil)
	}
	var storeInitialization StoreInitialization
	if initializeStore != nil {
		normalizedInstallTransactionSeams(transactionSeams).event("initializer")
		storeInitialization, err = initializeStore(context.Background())
		if err != nil {
			return installTransactionError("initialize profile store", err, rollbackStoreInitialization(storeInitialization))
		}
	}
	if err := validateInstallRootRelationship(paths.runtimeDir, storeInitialization); err != nil {
		return installTransactionError("validate profile store and runtime roots", err, rollbackStoreInitialization(storeInitialization))
	}
	targetTransaction, err := prepareGuardedInstallTransaction(prepared, transactionSeams)
	if err != nil {
		if errors.Is(err, ErrInstallRecoveryRequired) {
			return installTransactionError("prepare "+paths.zshrcPath, err, nil)
		}
		return installTransactionError("prepare "+paths.zshrcPath, err, rollbackStoreInitialization(storeInitialization))
	}
	defer targetTransaction.closeHandles()

	normalizedInstallTransactionSeams(transactionSeams).event("cache")
	cacheState, err := secureCacheDirectory(paths.runtimeDir)
	if err != nil {
		targetRollback := targetTransaction.rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			return installTransactionError("create runtime directory", err, targetRollback)
		}
		return installTransactionError("create runtime directory", err, errors.Join(targetRollback, rollbackStoreInitialization(storeInitialization)))
	}
	defer func() { _ = cacheState.close() }()
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + provider.HookScript()
	loaderRollback, err := cacheState.prepareLoaderRollback()
	if err != nil {
		targetRollback := targetTransaction.rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			return installTransactionError("prepare loader rollback", err, targetRollback)
		}
		rollbackErr := errors.Join(targetRollback, rollbackRuntimeAndStore(cacheState, storeInitialization))
		return installTransactionError("prepare loader rollback", err, rollbackErr)
	}
	loaderWrite, err := cacheState.prepareValidatedLoader([]byte(loaderScript))
	if err != nil {
		loaderRollback.discard()
		targetRollback := targetTransaction.rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			return installTransactionError("install cached loader", err, targetRollback)
		}
		rollbackErr := errors.Join(targetRollback, rollbackRuntimeAndStore(cacheState, storeInitialization))
		return installTransactionError("install cached loader", err, rollbackErr)
	}

	normalizedInstallTransactionSeams(transactionSeams).event("loader")
	if err := loaderWrite.promote(); err != nil {
		targetRollback := targetTransaction.rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			loaderWrite.discard()
			loaderRollback.discard()
			return installTransactionError("install cached loader", err, targetRollback)
		}
		rollbackErr := compensateAfterLoaderPromotion(
			targetRollback, &loaderWrite, &loaderRollback, cacheState, storeInitialization,
		)
		return installTransactionError("install cached loader", err, rollbackErr)
	}
	targetOutcome, err := targetTransaction.promote()
	if err != nil {
		if targetOutcome.RecoveryRequired || errors.Is(err, ErrInstallRecoveryRequired) {
			if !targetOutcome.Promoted {
				rollbackErr := errors.Join(
					rollbackPromotedCache(&loaderWrite, &loaderRollback),
					rollbackRuntimeAndStore(cacheState, storeInitialization),
				)
				loaderWrite.discard()
				loaderRollback.discard()
				return installTransactionError("write "+paths.zshrcPath, err, rollbackErr)
			}
			return installTransactionError("write "+paths.zshrcPath, err, nil)
		}
		targetRollback := targetOutcome.Rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			return installTransactionError("write "+paths.zshrcPath, err, targetRollback)
		}
		rollbackErr := compensateAfterLoaderPromotion(
			targetRollback, &loaderWrite, &loaderRollback, cacheState, storeInitialization,
		)
		return installTransactionError("write "+paths.zshrcPath, err, rollbackErr)
	}
	if err := finalizeStoreInitialization(storeInitialization); err != nil {
		targetRollback := targetOutcome.Rollback()
		if errors.Is(targetRollback, ErrInstallRecoveryRequired) {
			return installTransactionError("finalize profile store initialization", err, targetRollback)
		}
		rollbackErr := compensateAfterLoaderPromotion(
			targetRollback, &loaderWrite, &loaderRollback, cacheState, storeInitialization,
		)
		return installTransactionError("finalize profile store initialization", err, rollbackErr)
	}
	if err := targetOutcome.Finalize(); err != nil {
		return installTransactionError("finalize startup transaction", err, nil)
	}

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

// rollbackRuntimeAndStore restores the loader/cache state before rolling back
// Store initialization. Recovery-required target outcomes never call it.
func rollbackRuntimeAndStore(cache *cacheDirectoryState, initialization StoreInitialization) error {
	if cache == nil {
		return rollbackStoreInitialization(initialization)
	}
	return errors.Join(cache.rollback(), rollbackStoreInitialization(initialization))
}

// compensateAfterLoaderPromotion is shared only by branches where target
// rollback is known not to require retained recovery. Preserve the effect
// order: target rollback has already run, then restore the promoted loader,
// discard its staging handles, roll back the runtime, and finally the Store.
func compensateAfterLoaderPromotion(
	targetRollback error,
	loaderWrite *preparedCacheWrite,
	loaderRollback *cacheWriteRollback,
	cache *cacheDirectoryState,
	initialization StoreInitialization,
) error {
	rollbackErr := errors.Join(targetRollback, rollbackPromotedCache(loaderWrite, loaderRollback))
	loaderWrite.discard()
	loaderRollback.discard()
	return errors.Join(rollbackErr, rollbackRuntimeAndStore(cache, initialization))
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

func installTransactionError(operation string, cause, rollbackErr error) error {
	if rollbackErr == nil {
		return fmt.Errorf("%s: %w", operation, cause)
	}
	return fmt.Errorf("%s: %w; rollback failed: %v", operation, cause, rollbackErr)
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
