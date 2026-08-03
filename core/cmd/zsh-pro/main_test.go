package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"zsh-pro/core/cli"
	"zsh-pro/core/dto"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

func TestRuntimeCaptureUsesCompositionStoreAndVaultForEveryLocation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("descriptor-bound runtime capture is unsupported on this platform")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	zshPath, zshErr := exec.LookPath("zsh")
	binDir := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	locations := []struct {
		name      string
		configure func(*testing.T, string) string
	}{
		{
			name: "HOME fallback",
			configure: func(t *testing.T, home string) string {
				setCompositionEnvironment(t, map[string]string{"HOME": home})
				return filepath.Join(home, ".local", "share", "zsh-pro")
			},
		},
		{
			name: "absolute XDG data home",
			configure: func(t *testing.T, home string) string {
				dataHome := t.TempDir()
				setCompositionEnvironment(t, map[string]string{
					"HOME":          home,
					"XDG_DATA_HOME": dataHome,
				})
				return filepath.Join(dataHome, "zsh-pro")
			},
		},
		{
			name: "explicit ZSHPRO_HOME",
			configure: func(t *testing.T, home string) string {
				root := filepath.Join(t.TempDir(), "profiles")
				setCompositionEnvironment(t, map[string]string{
					"HOME":        home,
					"ZSHPRO_HOME": root,
				})
				return root
			},
		},
	}

	for _, location := range locations {
		t.Run(location.name, func(t *testing.T) {
			home := t.TempDir()
			root := location.configure(t, home)
			profile := "runtime-" + strings.ReplaceAll(location.name, " ", "-")
			identity := "composition-" + profile
			secret := "vault-" + profile
			seedCompositionStore(t, root, profile, identity, secret)

			program := newCLI()
			ordinaryList := runCompositionCLI(t, program, "list")
			if !strings.Contains(ordinaryList, profile+"\n") {
				t.Fatalf("ordinary list did not use %q:\n%s", root, ordinaryList)
			}
			runtimeList := runCompositionCLI(t, program, "runtime", "capture", "5", "--", "zsh-pro", "list")
			if runtimeList != ordinaryList {
				t.Fatalf("runtime list = %q, want ordinary list %q", runtimeList, ordinaryList)
			}

			ordinaryEmit := runCompositionCLI(t, program, "emit", "apply", profile)
			runtimeEmit := runCompositionCLI(t, program, "runtime", "capture", "5", "--", "zsh-pro", "emit", "apply", profile)
			for _, want := range []string{identity, secret} {
				if !strings.Contains(ordinaryEmit, want) {
					t.Fatalf("ordinary emit did not use store/vault value %q:\n%s", want, ordinaryEmit)
				}
				if !strings.Contains(runtimeEmit, want) {
					t.Fatalf("runtime emit did not use the same store/vault value %q:\n%s", want, runtimeEmit)
				}
			}
			if zshErr == nil {
				assertRuntimePayloadApplies(t, zshPath, runtimeEmit, identity, secret)
			}
		})
	}
}

type mainDeterministicKeychain struct {
	storeCalls    int
	retrieveCalls int
	deleteCalls   int
}

func (keychain *mainDeterministicKeychain) Store(string, string) error {
	keychain.storeCalls++
	return nil
}

func (keychain *mainDeterministicKeychain) Retrieve(string) (string, error) {
	keychain.retrieveCalls++
	return "", nil
}

func (keychain *mainDeterministicKeychain) Delete(string) error {
	keychain.deleteCalls++
	return nil
}

func (*mainDeterministicKeychain) Kind() model.SecretRefKind { return model.SecretRefFile }

func TestMainInitToBeginUsesExactCLIStore(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("authenticated ingest transactions are unsupported on this platform")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(source), "store.New("); count != 1 {
		t.Fatalf("composition root constructs Store %d times, want exactly once", count)
	}
	if strings.Contains(string(source), "GOTOOLCHAIN=auto") {
		t.Fatal("composition-root build helpers may not select or download a Go toolchain")
	}

	root := filepath.Join(t.TempDir(), "profiles.git")
	provider := zsh.Provider{}
	backend := &mainDeterministicKeychain{}
	cliStore, err := store.New(root, provider, backend)
	if err != nil {
		t.Fatal(err)
	}
	initialize := storeInitializerFor(cliStore, root, nil)
	initialization, err := initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if initialization.Transactions != cliStore {
		t.Fatal("initializer did not expose the exact concrete Store pointer")
	}
	if initialization.InitializationID.IsZero() || initialization.CanonicalRoot != root {
		t.Fatalf("initialization evidence = %#v, want nonzero ID and root %q", initialization, root)
	}

	begin, err := initialization.Transactions.BeginIngest(context.Background(), initialization.InitializationID)
	if err != nil || begin.TransactionID.IsZero() || begin.InitializationID != initialization.InitializationID {
		t.Fatalf("BeginIngest = (%#v, %v)", begin, err)
	}
	runtimeValue := "nvim"
	profile := model.Profile{Entries: []model.Entry{{
		Text:                    "export EDITOR=nvim",
		StartLine:               1,
		Category:                model.CatEnvironment,
		Kind:                    model.KindAssignment,
		Names:                   []string{"EDITOR"},
		Value:                   "nvim",
		Exported:                true,
		Managed:                 true,
		StructuralFidelityKnown: true,
		RuntimeValue:            &runtimeValue,
		ValueMode:               model.ValueModeLiteral,
	}}}
	committed, err := initialization.Transactions.CommitIngest(
		context.Background(),
		initialization.InitializationID,
		begin.TransactionID,
		profile,
		"composition identity proof",
	)
	if err != nil || committed.Status != model.IngestCommitCommitted || committed.RecoveryRequired {
		t.Fatalf("CommitIngest = (%#v, %v)", committed, err)
	}

	abortBegin, err := initialization.Transactions.BeginIngest(context.Background(), initialization.InitializationID)
	if err != nil {
		t.Fatalf("second BeginIngest: %v", err)
	}
	aborted, err := initialization.Transactions.AbortIngest(
		context.Background(),
		initialization.InitializationID,
		abortBegin.TransactionID,
	)
	if err != nil || aborted.Lifecycle != model.IngestLifecycleTerminal || aborted.RecoveryRequired {
		t.Fatalf("AbortIngest = (%#v, %v)", aborted, err)
	}

	beforeRef := gitRef(t, root, "refs/heads/main")
	otherBackend := &mainDeterministicKeychain{}
	other, err := store.New(root, provider, otherBackend)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, crossErr := other.BeginIngest(context.Background(), initialization.InitializationID); !errors.Is(crossErr, store.ErrInvalidIngestAuthority) {
		t.Fatalf("cross-Store BeginIngest = (%#v, %v), want invalid authority", outcome, crossErr)
	}
	if afterRef := gitRef(t, root, "refs/heads/main"); afterRef != beforeRef {
		t.Fatalf("cross-Store rejection changed main: got %s, want %s", afterRef, beforeRef)
	}
	if *otherBackend != (mainDeterministicKeychain{}) {
		t.Fatalf("cross-Store rejection touched backend: %#v", otherBackend)
	}

	if err := initialization.Finalize(); err != nil {
		t.Fatalf("Finalize aggregate terminal transactions: %v", err)
	}
	if outcome, expiredErr := cliStore.BeginIngest(context.Background(), initialization.InitializationID); !errors.Is(expiredErr, store.ErrInvalidIngestAuthority) {
		t.Fatalf("expired initialization BeginIngest = (%#v, %v), want invalid authority", outcome, expiredErr)
	}

	constructionErr := errors.New("original store construction failure")
	failedRoot := filepath.Join(t.TempDir(), "must-not-be-created")
	failedInitializer := storeInitializerFor(nil, failedRoot, constructionErr)
	if _, gotErr := failedInitializer(context.Background()); !errors.Is(gotErr, constructionErr) {
		t.Fatalf("failed initializer error = %v, want original %v", gotErr, constructionErr)
	}
	if _, statErr := os.Lstat(failedRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed initializer reconstructed or mutated Store root: %v", statErr)
	}

	analysisPath := filepath.Join(t.TempDir(), "analysis.zsh")
	if err := os.WriteFile(analysisPath, []byte("export EDITOR=nvim\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	program := cli.NewWithStoreInitializer(provider, nil, cli.NotReadyEmitter(), failedInitializer)
	var stdout, stderr bytes.Buffer
	if code := program.Run([]string{"analyze", analysisPath}, &stdout, &stderr); code != int(model.ExitClean) {
		t.Fatalf("analyze with failed deferred Store = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

// TestBuiltBinaryInstallMatchesDescriptorRuntimePlatformBoundary proves that the
// platforms allowed to initialize storage can also serve descriptor-bound
// runtime list/emission. Platforms without that boundary must reject install
// before either store or bootstrap state is written.
func TestBuiltBinaryInstallMatchesDescriptorRuntimePlatformBoundary(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	binary := buildInstalledBinary(t)
	supported := runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	if supported {
		if _, err := exec.LookPath("zsh"); err != nil {
			t.Skip("zsh not installed")
		}
	}

	for _, route := range builtInstallRoutes() {
		route := route
		t.Run(route.name, func(t *testing.T) {
			state := route.configure(t)
			root, env := state.storeRoot, state.env

			if out, err := runInstalledBinary(binary, env, "list"); err == nil {
				t.Fatalf("list before install unexpectedly succeeded: %s", out)
			}
			if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read-only list created profile storage at %s: %v", root, err)
			}
			if !supported {
				if out, err := runInstalledBinary(binary, env, "install"); err == nil {
					t.Fatalf("unsupported platform install unexpectedly succeeded: %s", out)
				}
				assertFreshInstallRouteRestored(t, state)
				return
			}

			if out, err := runInstalledBinary(binary, env, "install"); err != nil {
				t.Fatalf("fresh install failed: %v\n%s", err, out)
			}
			assertInstalledProfileStore(t, binary, env, root)

			if err := os.Chmod(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if out, err := runInstalledBinary(binary, env, "install"); err != nil {
				t.Fatalf("migration install failed: %v\n%s", err, out)
			}
			assertInstalledProfileStore(t, binary, env, root)
		})
	}
}

func TestBuiltBinaryInstallRollsBackFreshStoreAfterBootstrapFailures(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	binary := buildInstalledBinary(t)

	routes := builtInstallRoutes()
	t.Run("loader validation", func(t *testing.T) {
		binDir := t.TempDir()
		if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "zsh"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, route := range routes {
			route := route
			t.Run(route.name, func(t *testing.T) {
				state := route.configure(t)
				if out, err := runInstalledBinary(binary, withInstalledBinaryEnv(state.env, map[string]string{"PATH": binDir}), "install"); err == nil {
					t.Fatalf("install unexpectedly passed with failing loader validation: %s", out)
				} else {
					assertNoRollbackFailure(t, out)
				}
				assertFreshInstallRouteRestored(t, state)
			})
		}
	})

	t.Run("runtime directory", func(t *testing.T) {
		for _, route := range routes {
			route := route
			t.Run(route.name, func(t *testing.T) {
				state := route.configure(t)
				before := []byte("pre-install runtime file\n")
				if err := os.WriteFile(state.runtimeRoot, before, 0o600); err != nil {
					t.Fatal(err)
				}
				if out, err := runInstalledBinary(binary, state.env, "install"); err == nil {
					t.Fatalf("install unexpectedly passed with a non-directory runtime root: %s", out)
				} else {
					assertNoRollbackFailure(t, out)
				}
				if got, err := os.ReadFile(state.runtimeRoot); err != nil || !bytes.Equal(got, before) {
					t.Fatalf("runtime root changed after failed install: %q, err=%v", got, err)
				}
				if state.storeRoot != state.runtimeRoot {
					assertFreshProfileStoreAbsent(t, state)
					return
				}
				if _, err := os.Lstat(filepath.Join(state.home, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed install wrote .zshrc: %v", err)
				}
			})
		}
	})
}

func TestBuiltBinaryInstallStagingFailureRollsBackStoreState(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	binary := buildInstalledBinary(t)

	for _, route := range builtInstallRoutes()[:2] {
		route := route
		t.Run("fresh "+route.name, func(t *testing.T) {
			state := route.configure(t)
			if err := os.Mkdir(state.runtimeRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			loaderDir := filepath.Join(state.runtimeRoot, "loader.zsh")
			if err := os.Mkdir(loaderDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if out, err := runInstalledBinary(binary, state.env, "install"); err == nil {
				t.Fatalf("install unexpectedly accepted a non-regular loader target: %s", out)
			} else {
				assertNoRollbackFailure(t, out)
			}
			if _, err := os.Lstat(state.storeRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("staging failure left a new profile store at %s: %v", state.storeRoot, err)
			}
			if info, err := os.Stat(loaderDir); err != nil || !info.IsDir() {
				t.Fatalf("staging failure changed the pre-existing loader directory: %v, err=%v", info, err)
			}
			if _, err := os.Lstat(filepath.Join(state.home, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("staging failure wrote .zshrc: %v", err)
			}
		})
	}

	t.Run("explicit preexisting migration", func(t *testing.T) {
		state := builtInstallRoutes()[2].configure(t)
		provider := zsh.Provider{}
		s, err := store.New(state.storeRoot, provider, store.NewOSKeychainDriver(state.storeRoot))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		beforeMain := gitRef(t, state.storeRoot, "refs/heads/main")
		if err := os.Chmod(state.storeRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		loaderDir := filepath.Join(state.runtimeRoot, "loader.zsh")
		if err := os.Mkdir(loaderDir, 0o700); err != nil {
			t.Fatal(err)
		}

		if out, err := runInstalledBinary(binary, state.env, "install"); err == nil {
			t.Fatalf("install unexpectedly accepted a non-regular explicit loader target: %s", out)
		} else {
			assertNoRollbackFailure(t, out)
		}
		if info, err := os.Stat(state.storeRoot); err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("failed migration mode = %v, err=%v; want 0755", info.Mode().Perm(), err)
		}
		if got := gitRef(t, state.storeRoot, "refs/heads/main"); got != beforeMain {
			t.Fatalf("failed migration rewrote main: got %s, want %s", got, beforeMain)
		}
		if info, err := os.Stat(loaderDir); err != nil || !info.IsDir() {
			t.Fatalf("failed migration changed the pre-existing loader directory: %v, err=%v", info, err)
		}
	})
}

func TestBuiltBinaryInstallRollsBackStoreOnPromotionFailures(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("transactional initialization is unsupported on this platform")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	binary := buildInstalledBinary(t)

	t.Run("first loader promotion", func(t *testing.T) {
		binDir := writeValidationShim(t, gitPath, "#!/bin/sh\nexec /bin/rm \"$3\"\n")
		for _, route := range builtInstallRoutes() {
			route := route
			t.Run(route.name, func(t *testing.T) {
				state := route.configure(t)
				if out, err := runInstalledBinary(binary, withInstalledBinaryEnv(state.env, map[string]string{"PATH": binDir}), "install"); err == nil {
					t.Fatalf("install unexpectedly passed after the candidate disappeared before promotion: %s", out)
				} else {
					assertNoRollbackFailure(t, out)
				}
				assertFreshInstallRouteRestored(t, state)
			})
		}
	})

	t.Run("second zshrc promotion", func(t *testing.T) {
		binDir := writeValidationShim(t, gitPath, "#!/bin/sh\n/bin/chmod 500 \"$ZDOTDIR\"\nexit 0\n")
		for _, route := range builtInstallRoutes() {
			route := route
			t.Run(route.name, func(t *testing.T) {
				state := route.configure(t)
				zdotdir := filepath.Join(t.TempDir(), "zdotdir")
				if err := os.Mkdir(zdotdir, 0o700); err != nil {
					t.Fatal(err)
				}
				rc := filepath.Join(zdotdir, ".zshrc")
				before := []byte("export KEEP_PREINSTALL=1\n")
				if err := os.WriteFile(rc, before, 0o600); err != nil {
					t.Fatal(err)
				}
				env := withInstalledBinaryEnv(state.env, map[string]string{"PATH": binDir, "ZDOTDIR": zdotdir})
				if out, err := runInstalledBinary(binary, env, "install"); err == nil {
					t.Fatalf("install unexpectedly passed after the zshrc directory became unwritable: %s", out)
				} else {
					assertNoRollbackFailure(t, out)
				}
				if err := os.Chmod(zdotdir, 0o700); err != nil {
					t.Fatal(err)
				}
				if got, err := os.ReadFile(rc); err != nil || !bytes.Equal(got, before) {
					t.Fatalf("second-promotion failure changed .zshrc: %q, err=%v", got, err)
				}
				if _, err := os.Lstat(state.storeRoot); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("second-promotion failure left a new profile store at %s: %v", state.storeRoot, err)
				}
				if state.runtimeRoot != state.storeRoot {
					if _, err := os.Lstat(state.runtimeRoot); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("second-promotion failure left a new runtime directory at %s: %v", state.runtimeRoot, err)
					}
				}
			})
		}
	})
}

// TestBuiltBinaryInstallRejectsSymlinkedCacheTargetsWithoutMutation exercises
// the released command rather than its package seam. The generated cache has a
// stricter policy than a user-selected .zshrc: a symlinked root or loader must
// fail before it can redirect a chmod, cache write, store initialization, or
// bootstrap replacement to unrelated data.
func TestBuiltBinaryInstallRejectsSymlinkedCacheTargetsWithoutMutation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("descriptor-authenticated cached-loader installation is unsupported on this platform")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	binary := buildInstalledBinary(t)

	for _, route := range builtInstallRoutes() {
		route := route
		t.Run("symlinked cache root/"+route.name, func(t *testing.T) {
			state := route.configure(t)
			rc := filepath.Join(state.home, ".zshrc")
			beforeRC := []byte("export KEEP_ROOT_SYMLINK=1\n")
			if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
				t.Fatal(err)
			}

			victim := t.TempDir()
			if err := os.Chmod(victim, 0o755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(victim, "unrelated.txt")
			beforeSentinel := []byte("do not alter this unrelated directory\n")
			if err := os.WriteFile(sentinel, beforeSentinel, 0o644); err != nil {
				t.Fatal(err)
			}
			beforeVictimTree := snapshotBuiltTree(t, victim)
			if err := os.Symlink(victim, state.runtimeRoot); err != nil {
				t.Fatal(err)
			}

			if out, err := runInstalledBinary(binary, state.env, "install"); err == nil {
				t.Fatalf("install accepted a symlinked cache root: %s", out)
			} else {
				assertNoRollbackFailure(t, out)
			}
			assertBuiltSymlink(t, state.runtimeRoot, victim)
			assertBuiltBytesAndMode(t, sentinel, beforeSentinel, 0o644)
			assertBuiltTree(t, victim, beforeVictimTree)
			assertBuiltBytesAndMode(t, rc, beforeRC, 0o600)
			if state.storeRoot != state.runtimeRoot {
				assertFreshBuiltStoreAbsent(t, state)
			}
		})
	}

	for _, route := range builtInstallRoutes() {
		route := route
		t.Run("symlinked cached loader/"+route.name, func(t *testing.T) {
			state := route.configure(t)
			rc := filepath.Join(state.home, ".zshrc")
			beforeRC := []byte("export KEEP_LOADER_SYMLINK=1\n")
			if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
				t.Fatal(err)
			}

			sharedCacheAndStore := state.storeRoot == state.runtimeRoot
			var beforeMain string
			if sharedCacheAndStore {
				initializeBuiltInstallStore(t, state)
				beforeMain = gitRef(t, state.storeRoot, "refs/heads/main")
			} else {
				if err := os.Mkdir(state.runtimeRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				// A non-private pre-existing cache makes this test prove that a
				// rejected loader does not even transiently repair its mode.
				if err := os.Chmod(state.runtimeRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			victim := filepath.Join(t.TempDir(), "unrelated-loader.zsh")
			beforeVictim := []byte("DO NOT OVERWRITE THIS FILE\n")
			if err := os.WriteFile(victim, beforeVictim, 0o644); err != nil {
				t.Fatal(err)
			}
			loader := filepath.Join(state.runtimeRoot, "loader.zsh")
			if err := os.Symlink(victim, loader); err != nil {
				t.Fatal(err)
			}
			beforeCacheTree := snapshotBuiltTree(t, state.runtimeRoot)

			if out, err := runInstalledBinary(binary, state.env, "install"); err == nil {
				t.Fatalf("install accepted a symlinked cached loader: %s", out)
			} else {
				assertNoRollbackFailure(t, out)
			}
			assertBuiltSymlink(t, loader, victim)
			assertBuiltBytesAndMode(t, victim, beforeVictim, 0o644)
			assertBuiltBytesAndMode(t, rc, beforeRC, 0o600)
			assertBuiltTree(t, state.runtimeRoot, beforeCacheTree)
			if !sharedCacheAndStore {
				assertFreshBuiltStoreAbsent(t, state)
				return
			}
			if got := gitRef(t, state.storeRoot, "refs/heads/main"); got != beforeMain {
				t.Fatalf("symlink rejection changed explicit store main: got %s, want %s", got, beforeMain)
			}
		})
	}
}

type builtInstallRoute struct {
	name      string
	configure func(*testing.T) builtInstallState
}

type builtInstallState struct {
	home        string
	storeRoot   string
	runtimeRoot string
	env         []string
	newParents  []string
}

func builtInstallRoutes() []builtInstallRoute {
	return []builtInstallRoute{
		{
			name: "HOME fallback",
			configure: func(t *testing.T) builtInstallState {
				home := t.TempDir()
				storeRoot := filepath.Join(home, ".local", "share", "zsh-pro")
				return builtInstallState{
					home:        home,
					storeRoot:   storeRoot,
					runtimeRoot: filepath.Join(home, ".zsh-pro"),
					env:         installedBinaryEnv(map[string]string{"HOME": home}),
					newParents:  []string{filepath.Join(home, ".local", "share"), filepath.Join(home, ".local")},
				}
			},
		},
		{
			name: "XDG_DATA_HOME",
			configure: func(t *testing.T) builtInstallState {
				home := t.TempDir()
				dataHome := t.TempDir()
				return builtInstallState{
					home:        home,
					storeRoot:   filepath.Join(dataHome, "zsh-pro"),
					runtimeRoot: filepath.Join(home, ".zsh-pro"),
					env:         installedBinaryEnv(map[string]string{"HOME": home, "XDG_DATA_HOME": dataHome}),
				}
			},
		},
		{
			name: "explicit ZSHPRO_HOME",
			configure: func(t *testing.T) builtInstallState {
				home := t.TempDir()
				storeRoot := filepath.Join(t.TempDir(), "profiles")
				return builtInstallState{
					home:        home,
					storeRoot:   storeRoot,
					runtimeRoot: storeRoot,
					env:         installedBinaryEnv(map[string]string{"HOME": home, "ZSHPRO_HOME": storeRoot}),
				}
			},
		},
	}
}

func assertFreshInstallRouteRestored(t *testing.T, state builtInstallState) {
	t.Helper()
	paths := append([]string{state.storeRoot, state.runtimeRoot, filepath.Join(state.home, ".zshrc")}, state.newParents...)
	assertPathsAbsent(t, paths)
}

func assertFreshProfileStoreAbsent(t *testing.T, state builtInstallState) {
	t.Helper()
	paths := append([]string{state.storeRoot, filepath.Join(state.home, ".zshrc")}, state.newParents...)
	assertPathsAbsent(t, paths)
}

func assertFreshBuiltStoreAbsent(t *testing.T, state builtInstallState) {
	t.Helper()
	paths := append([]string{state.storeRoot}, state.newParents...)
	assertPathsAbsent(t, paths)
}

func assertPathsAbsent(t *testing.T, paths []string) {
	t.Helper()
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed install left new path %s: %v", path, err)
		}
	}
}

func assertNoRollbackFailure(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, "rollback failed") {
		t.Fatalf("install restored the filesystem but reported a rollback failure:\n%s", output)
	}
}

func initializeBuiltInstallStore(t *testing.T, state builtInstallState) {
	t.Helper()
	provider := zsh.Provider{}
	s, err := store.New(state.storeRoot, provider, store.NewOSKeychainDriver(state.storeRoot))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func assertBuiltSymlink(t *testing.T, path, wantTarget string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink: %v, err=%v", path, info, err)
	}
	if got, err := os.Readlink(path); err != nil || got != wantTarget {
		t.Fatalf("symlink %s target = %q, err=%v; want %q", path, got, err, wantTarget)
	}
}

func assertBuiltBytesAndMode(t *testing.T, path string, want []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s bytes = %q, err=%v; want %q", path, got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %v, want %v", path, info.Mode().Perm(), mode)
	}
}

type builtTreeEntry struct {
	mode   fs.FileMode
	digest [sha256.Size]byte
	link   string
}

func snapshotBuiltTree(t *testing.T, root string) map[string]builtTreeEntry {
	t.Helper()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatalf("stat snapshot root %s: %v", root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("snapshot root %s is not a real directory: %v", root, info.Mode())
	}
	tree := make(map[string]builtTreeEntry)
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
		recorded := builtTreeEntry{mode: info.Mode()}
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
			// Directories are represented by their exact mode; WalkDir does not
			// recurse through a symlink.
		default:
			return errors.New("snapshot encountered an unsupported filesystem entry")
		}
		tree[rel] = recorded
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree
}

func assertBuiltTree(t *testing.T, root string, want map[string]builtTreeEntry) {
	t.Helper()
	got := snapshotBuiltTree(t, root)
	if len(got) != len(want) {
		t.Fatalf("tree %s entry count = %d, want %d", root, len(got), len(want))
	}
	for path, expected := range want {
		if actual, ok := got[path]; !ok || actual != expected {
			t.Fatalf("tree %s entry %q = %#v, present=%t; want %#v", root, path, actual, ok, expected)
		}
	}
}

func withInstalledBinaryEnv(env []string, values map[string]string) []string {
	replaced := make(map[string]bool, len(values))
	for name := range values {
		replaced[name] = true
	}
	updated := make([]string, 0, len(env)+len(values))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !replaced[name] {
			updated = append(updated, entry)
		}
	}
	for name, value := range values {
		updated = append(updated, name+"="+value)
	}
	return updated
}

func writeValidationShim(t *testing.T, gitPath, script string) string {
	t.Helper()
	binDir := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "zsh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binDir
}

func gitRef(t *testing.T, root, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "--git-dir="+root, "rev-parse", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v\n%s", ref, err, out)
	}
	return strings.TrimSpace(string(out))
}

func buildInstalledBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "zsh-pro")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build zsh-pro binary: %v\n%s", err, out)
	}
	return binary
}

func installedBinaryEnv(values map[string]string) []string {
	replaced := map[string]bool{"HOME": true, "XDG_DATA_HOME": true, "ZSHPRO_HOME": true}
	for name := range values {
		replaced[name] = true
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !replaced[name] {
			env = append(env, entry)
		}
	}
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	return env
}

func runInstalledBinary(binary string, env []string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertInstalledProfileStore(t *testing.T, binary string, env []string, root string) {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat profile store: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("profile store mode = %#o, want 0700", got)
	}

	git := exec.Command("git", "--git-dir="+root, "rev-parse", "--is-bare-repository")
	git.Env = env
	if out, err := git.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "true" {
		t.Fatalf("profile store is not a usable bare repository: %v\n%s", err, out)
	}

	list, err := runInstalledBinary(binary, env, "list")
	if err != nil || list != "main\n" {
		t.Fatalf("ordinary list = (%v, %q), want main", err, list)
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return
	}
	runtimeList, err := runInstalledBinary(binary, env, "runtime", "capture", "5", "--", "zsh-pro", "list")
	if err != nil || runtimeList != list {
		t.Fatalf("runtime list = (%v, %q), want %q", err, runtimeList, list)
	}
	ordinaryEmit, err := runInstalledBinary(binary, env, "emit", "apply", "main")
	if err != nil || !strings.Contains(ordinaryEmit, "_zp_run_payload") {
		t.Fatalf("ordinary emit = (%v, %q), want runtime payload", err, ordinaryEmit)
	}
	runtimeEmit, err := runInstalledBinary(binary, env, "runtime", "capture", "5", "--", "zsh-pro", "emit", "apply", "main")
	if err != nil || !strings.Contains(runtimeEmit, "_zp_run_payload") {
		t.Fatalf("runtime emission = (%v, %q), want runtime payload", err, runtimeEmit)
	}
}

func seedCompositionStore(t *testing.T, root, profile, identity, secret string) {
	t.Helper()
	provider := zsh.Provider{}
	keychain := store.NewOSKeychainDriver(root)
	if keychain.Kind() != model.SecretRefFile {
		t.Fatalf("test PATH did not select the file vault: %s", keychain.Kind())
	}
	s, err := store.New(root, provider, keychain)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("Store.Init root mode = %#o, want 0700", got)
	}
	if err := s.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, profile, compositionProfile(identity, secret), "runtime store root regression fixture"); err != nil {
		t.Fatal(err)
	}
}

func compositionProfile(identity, secret string) model.Profile {
	return model.Profile{Entries: []model.Entry{
		{
			Text:                    "export ZP_COMPOSITION_ID=" + identity,
			Category:                model.CatEnvironment,
			Kind:                    model.KindAssignment,
			Names:                   []string{"ZP_COMPOSITION_ID"},
			Value:                   identity,
			Exported:                true,
			Managed:                 true,
			StructuralFidelityKnown: true,
			RuntimeValue:            &identity,
			ValueMode:               model.ValueModeLiteral,
		},
		{
			Text:                    "export ZP_COMPOSITION_SECRET=" + secret,
			Category:                model.CatSecrets,
			Kind:                    model.KindAssignment,
			Names:                   []string{"ZP_COMPOSITION_SECRET"},
			Value:                   secret,
			Exported:                true,
			Managed:                 true,
			StructuralFidelityKnown: true,
			RuntimeValue:            &secret,
			ValueMode:               model.ValueModeLiteral,
		},
	}}
}

func runCompositionCLI(t *testing.T, program *cli.CLI, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := program.Run(args, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("zsh-pro %s = (%d, %q, %q)", strings.Join(args, " "), code, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func assertRuntimePayloadApplies(t *testing.T, zshPath, payload, identity, secret string) {
	t.Helper()
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	payloadPath := filepath.Join(dir, "payload.zsh")
	if err := os.WriteFile(loader, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payloadPath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zshPath, "-f", "-c", `
source "$1"
typeset -g ZP_BASE_PATH="$PATH"
source "$2"
[[ "$ZP_COMPOSITION_ID" == "$3" && "$ZP_COMPOSITION_SECRET" == "$4" ]] || exit 10
`, "zsh-pro-composition-runtime-test", loader, payloadPath, identity, secret)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("captured source did not apply composition store values: %v\n%s", err, out)
	}
}

func setCompositionEnvironment(t *testing.T, values map[string]string) {
	t.Helper()
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "ZSHPRO_HOME"} {
		name := name
		old, present := os.LookupEnv(name)
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, old)
				return
			}
			_ = os.Unsetenv(name)
		})
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			t.Fatal(err)
		}
	}
}

const phase6SecretPlaceholder = "__PHASE6_LITERAL_SECRET__"

type phase6BuiltFixture struct {
	binary            string
	home              string
	dataHome          string
	storeRoot         string
	target            string
	zshPath           string
	secret            string
	sourceTemplate    []byte
	source            []byte
	expectedInstalled []byte
	env               []string
	executionCanary   string
	subprocessCounter string
}

func TestMainIngestHumanAndJSON(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	human, err := fixture.run("ingest")
	if err != nil {
		t.Fatalf("human ingest: %v\n%s", err, human)
	}
	lines := strings.Split(strings.TrimSuffix(human, "\n"), "\n")
	wantPrefixes := []string{
		"zsh-pro: ingest complete",
		"profile committed: yes",
		"startup installed: yes",
		"recovery required: no",
		"managed entries: ",
		"unmanaged statements: ",
		"unmanaged source lines: ",
		"source statements: ",
		"accounted statements: ",
		"withheld: PHASE6_API_TOKEN (line ",
	}
	if len(lines) != len(wantPrefixes) {
		t.Fatalf("human output lines = %d, want %d: %q", len(lines), len(wantPrefixes), human)
	}
	for index, prefix := range wantPrefixes {
		if !strings.HasPrefix(lines[index], prefix) {
			t.Fatalf("human output line %d = %q, want prefix %q", index+1, lines[index], prefix)
		}
	}

	encoded, err := fixture.run("ingest", "--json")
	if err != nil {
		t.Fatalf("JSON ingest: %v\n%s", err, encoded)
	}
	result := decodePhase6IngestResult(t, encoded)
	if !result.OK || result.ExitCode != int(model.ExitClean) || !result.ProfileCommitted ||
		!result.StartupInstalled || result.RecoveryRequired {
		t.Fatalf("JSON success evidence = %#v", result)
	}
	if result.ManagedEntries+result.UnmanagedStatements != result.AccountedStatements ||
		result.AccountedStatements != result.SourceStatements || result.SourceStatements == 0 {
		t.Fatalf("JSON statement accounting = %#v", result)
	}
	if len(result.Withheld) != 1 || result.Withheld[0].Name != "PHASE6_API_TOKEN" ||
		result.Withheld[0].StartLine <= 0 || len(result.Warnings) != 0 {
		t.Fatalf("JSON withheld/warning evidence = %#v", result)
	}
}

func TestMainIngestUsageExitTwo(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	before := append([]byte(nil), fixture.source...)

	human, err := fixture.run("ingest", "one", "two")
	if got := phase6ExitCode(err); got != int(model.ExitUsageErr) {
		t.Fatalf("human usage exit = %d, want %d; output=%q", got, model.ExitUsageErr, human)
	}
	if human != "usage: zsh-pro ingest [path] [--json]\n" {
		t.Fatalf("human usage output = %q", human)
	}

	encoded, err := fixture.run("ingest", "one", "two", "--json")
	if got := phase6ExitCode(err); got != int(model.ExitUsageErr) {
		t.Fatalf("JSON usage exit = %d, want %d; output=%q", got, model.ExitUsageErr, encoded)
	}
	result := decodePhase6IngestResult(t, encoded)
	if result.ExitCode != int(model.ExitUsageErr) || result.OK || result.ProfileCommitted ||
		result.StartupInstalled || result.RecoveryRequired {
		t.Fatalf("JSON usage result = %#v", result)
	}
	after, readErr := os.ReadFile(fixture.target)
	if readErr != nil || !bytes.Equal(after, before) {
		t.Fatal("usage rejection changed the startup target")
	}
	for _, path := range []string{fixture.storeRoot, filepath.Join(fixture.home, ".zsh-pro")} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("usage rejection created %s: %v", path, statErr)
		}
	}
}

func TestMainIngestExplicitSharedRoot(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	shared := filepath.Join(t.TempDir(), "shared-zsh-pro")
	fixture.env = phase6ReplaceEnv(fixture.env, map[string]string{"ZSHPRO_HOME": shared}, "XDG_DATA_HOME")
	fixture.storeRoot = shared

	encoded, err := fixture.run("ingest", fixture.target, "--json")
	if err != nil {
		t.Fatalf("shared-root ingest: %v\n%s", err, encoded)
	}
	result := decodePhase6IngestResult(t, encoded)
	if !result.OK || !result.ProfileCommitted || !result.StartupInstalled || result.RecoveryRequired {
		t.Fatalf("shared-root result = %#v", result)
	}
	if info, statErr := os.Stat(shared); statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("shared root mode = %v, err=%v; want 0700", info, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(shared, "loader.zsh")); statErr != nil {
		t.Fatalf("shared root lacks installed loader: %v", statErr)
	}
	if got := gitRef(t, shared, "refs/heads/main"); got == "" {
		t.Fatal("shared root main ref is empty")
	}
}

func TestMainIngestPromotesTargetExactlyOnce(t *testing.T) {
	root := phase6RepositoryRoot(t)
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, filepath.Join(root, "core/cli/ingest.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var targetPromotions int
	var targetPromotion, storeCommit, targetFinalize token.Pos
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "runIngestWithSeams" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, _ := selector.X.(*ast.Ident)
			switch {
			case receiver != nil && receiver.Name == "targetTransaction" && selector.Sel.Name == "promote":
				targetPromotions++
				targetPromotion = call.Pos()
			case selector.Sel.Name == "CommitIngest":
				storeCommit = call.Pos()
			case receiver != nil && receiver.Name == "targetOutcome" && selector.Sel.Name == "Finalize":
				targetFinalize = call.Pos()
			}
			return true
		})
	}
	if targetPromotions != 1 || targetPromotion == token.NoPos || storeCommit == token.NoPos || targetFinalize == token.NoPos ||
		targetPromotion >= storeCommit || storeCommit >= targetFinalize {
		t.Fatalf("ingest target transition = promotions %d positions %d/%d/%d", targetPromotions, targetPromotion, storeCommit, targetFinalize)
	}

	fixture := newPhase6BuiltFixture(t)
	if output, runErr := fixture.run("ingest", "--json"); runErr != nil {
		t.Fatalf("built ingest: %v\n%s", runErr, output)
	}
	entries, err := os.ReadDir(filepath.Join(fixture.home, ".zsh-pro-transactions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "lock" {
		t.Fatalf("successful ingest retained target transaction state: %v", entries)
	}
}

func TestMainIngestExpectedInstalledAndOutsideBytes(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("ingest: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, fixture.expectedInstalled) {
		t.Fatal("actual installed bytes differ from the independently authored expected template")
	}
	if len(installed) <= len(fixture.source) || !bytes.Equal(installed[:len(fixture.source)], fixture.source) {
		t.Fatal("first adoption changed an outside-marker byte")
	}
	if bytes.Count(installed, []byte("# >>> zsh-pro >>>")) != 1 ||
		bytes.Count(installed, []byte("# <<< zsh-pro <<<")) != 1 {
		t.Fatal("first adoption did not install exactly one marker pair")
	}
}

func TestMainIngestIdempotent(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("first ingest: %v\n%s", err, output)
	}
	first, err := os.ReadFile(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("second ingest: %v\n%s", err, output)
	}
	second, err := os.ReadFile(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || bytes.Count(second, []byte("# >>> zsh-pro >>>")) != 1 ||
		bytes.Count(second, []byte("# <<< zsh-pro <<<")) != 1 {
		t.Fatal("unchanged second ingest was not byte-identical with one marker pair")
	}
}

func TestMainIngestAppendWarningPreservesAndPersists(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	fixture.replaceTargetFromFixture(t, "installed-with-appends.zshrc")
	original := append([]byte(nil), fixture.source...)

	encoded, err := fixture.run("ingest", fixture.target, "--json")
	if err != nil {
		t.Fatalf("post-END ingest: %v\n%s", err, encoded)
	}
	result := decodePhase6IngestResult(t, encoded)
	if len(result.Warnings) != 1 || result.Warnings[0] != "zsh-pro: ordinary startup content remains after the managed loader block" {
		t.Fatalf("post-END warnings = %#v", result.Warnings)
	}
	installed, err := os.ReadFile(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	assertPhase6OutsideMarkerBytes(t, original, installed)
	profile := fixture.readMainProfile(t)
	if !phase6ProfileContainsName(profile, "PHASE6_APPEND_AFTER") {
		t.Fatal("eligible post-END declaration was not persisted in the complete Profile")
	}
}

func TestMainIngestActualInstalledStartupEquivalent(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	pristine := fixture.startupSnapshot(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("ingest: %v\n%s", err, output)
	}
	installed := fixture.startupSnapshot(t)
	if installed != pristine {
		t.Fatalf("actual installed startup differs from pristine startup:\npristine:\n%s\ninstalled:\n%s", pristine, installed)
	}
}

func TestMainIngestOrderSensitiveDefinitionBeforeUse(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("ingest: %v\n%s", err, output)
	}
	snapshot := fixture.startupSnapshot(t)
	for _, want := range []string{
		"order=definition,use\n",
		"imperative=definition-before-use\n",
		"function_result=function-ok\n",
	} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("installed startup lacks order-sensitive evidence %q:\n%s", strings.TrimSpace(want), snapshot)
		}
	}
}

func TestMainIngestSecretBehaviorWithoutDisclosure(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	encoded, err := fixture.run("ingest", "--json")
	if err != nil {
		t.Fatalf("ingest: %v\n%s", err, encoded)
	}
	if strings.Contains(encoded, fixture.secret) {
		t.Fatal("ingest output disclosed the runtime literal")
	}
	snapshot := fixture.startupSnapshot(t)
	if !strings.Contains(snapshot, "secret_equal=1\n") || strings.Contains(snapshot, fixture.secret) {
		t.Fatal("startup did not prove secret equality without disclosure")
	}
	matches := phase6LiteralBearingPaths(t, fixture.secret, fixture.home, fixture.dataHome)
	want := []string{fixture.target}
	if fmt.Sprint(matches) != fmt.Sprint(want) {
		t.Fatalf("literal-bearing paths = %v, want only installed target", matches)
	}
	for _, relative := range []string{
		"core/cli/testdata/ingest/real.zshrc",
		"core/cli/testdata/ingest/expected-installed.zshrc",
	} {
		content, readErr := os.ReadFile(filepath.Join(phase6RepositoryRoot(t), relative))
		if readErr != nil || bytes.Contains(content, []byte(fixture.secret)) {
			t.Fatalf("authored fixture %s contains the runtime literal or is unreadable: %v", relative, readErr)
		}
	}
}

func TestMainIngestAllowsOnlyExactLoaderSymbols(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	pristineParameters, pristineFunctions := fixture.symbolSnapshot(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("ingest: %v\n%s", err, output)
	}
	installedParameters, installedFunctions := fixture.symbolSnapshot(t)

	wantParameters := []string{"ZP_LAST_RUNTIME_ERROR", "ZP_LAST_RUNTIME_STATUS", "ZP_RUNTIME_TIMED_OUT"}
	wantFunctions := []string{
		"_zp_capture_scalar", "_zp_commit_restore_scalar", "_zp_commit_undo_slots", "_zp_emit",
		"_zp_eval_block", "_zp_has_known_active_profile", "_zp_preflight_restore_scalar",
		"_zp_preflight_undo_slots", "_zp_prepare_eval_state", "_zp_recover_failed_target",
		"_zp_restore_scalar", "_zp_reverse_active_profile", "_zp_run_bounded", "_zp_run_payload",
		"_zp_run_retained_reverse", "_zp_run_transient_reverse_payload", "_zp_runtime_error",
		"_zp_runtime_ok", "_zp_switch", "_zp_validate_block", "activate", "checkout", "deactivate",
		"list", "status", "zp_capture_env", "zp_restore_env",
	}
	sort.Strings(wantParameters)
	sort.Strings(wantFunctions)
	if got := phase6SetDifference(installedParameters, pristineParameters); fmt.Sprint(got) != fmt.Sprint(wantParameters) {
		t.Fatalf("loader parameter delta = %v, want exact allowlist %v", got, wantParameters)
	}
	if got := phase6SetDifference(installedFunctions, pristineFunctions); fmt.Sprint(got) != fmt.Sprint(wantFunctions) {
		t.Fatalf("loader function delta = %v, want exact allowlist %v", got, wantFunctions)
	}
}

func TestMainIngestStartupHasNoSubprocess(t *testing.T) {
	fixture := newPhase6BuiltFixture(t)
	if output, err := fixture.run("ingest", "--json"); err != nil {
		t.Fatalf("ingest: %v\n%s", err, output)
	}
	_ = fixture.startupSnapshot(t)
	if content, err := os.ReadFile(fixture.subprocessCounter); err == nil && len(content) != 0 {
		t.Fatalf("actual installed startup invoked a subprocess: %q", content)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func newPhase6BuiltFixture(t *testing.T) *phase6BuiltFixture {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("guarded ingest requires Linux or Darwin")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	root := phase6RepositoryRoot(t)
	sourceTemplate := phase6ReadFixture(t, root, "real.zshrc")
	expectedTemplate := phase6ReadFixture(t, root, "expected-installed.zshrc")
	if bytes.Count(sourceTemplate, []byte(phase6SecretPlaceholder)) != 1 ||
		bytes.Count(expectedTemplate, []byte(phase6SecretPlaceholder)) != 1 {
		t.Fatal("authored source and expected-installed fixtures must each contain exactly one reviewed placeholder")
	}
	secretBytes := make([]byte, 24)
	if _, err := rand.Read(secretBytes); err != nil {
		t.Fatal(err)
	}
	secret := "phase6_token_" + hex.EncodeToString(secretBytes)
	source := bytes.Replace(sourceTemplate, []byte(phase6SecretPlaceholder), []byte(secret), 1)
	expected := bytes.Replace(expectedTemplate, []byte(phase6SecretPlaceholder), []byte(secret), 1)

	home := t.TempDir()
	dataHome := t.TempDir()
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, source, 0o600); err != nil {
		t.Fatal(err)
	}
	toolDir := t.TempDir()
	for name, path := range map[string]string{"git": gitPath, "zsh": zshPath} {
		if err := os.Symlink(path, filepath.Join(toolDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	secretTool := "#!/bin/sh\ncase \"${1-}\" in\n  store) IFS= read -r phase6_value || :; phase6_value=; exit 0 ;;\n  clear) exit 0 ;;\n  lookup) exit 1 ;;\n  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(toolDir, "secret-tool"), []byte(secretTool), 0o700); err != nil {
		t.Fatal(err)
	}
	stubDir := t.TempDir()
	executionCanary := filepath.Join(t.TempDir(), "source-executed")
	subprocessCounter := filepath.Join(t.TempDir(), "subprocess-count")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$0\" >>\"$PHASE6_SUBPROCESS_COUNTER\"\nexit 97\n"
	for _, name := range []string{"awk", "bash", "cat", "curl", "date", "git", "grep", "hostname", "node", "python", "python3", "security", "sed", "secret-tool", "sh", "touch", "uname", "wget", "zsh-pro"} {
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(stub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + dataHome,
		"PATH=" + toolDir,
		"LC_ALL=C",
		"LANG=C",
		"PHASE6_STUB_BIN=" + stubDir,
		"PHASE6_SOURCE_EXECUTION_CANARY=" + executionCanary,
		"PHASE6_SUBPROCESS_COUNTER=" + subprocessCounter,
		"PHASE6_EXPECTED_SECRET=" + secret,
		"PHASE6_DYNAMIC_SOURCE=dynamic-runtime-value",
	}
	return &phase6BuiltFixture{
		binary: buildInstalledBinary(t), home: home, dataHome: dataHome,
		storeRoot: filepath.Join(dataHome, "zsh-pro"), target: target, zshPath: zshPath,
		secret: secret, sourceTemplate: sourceTemplate, source: source, expectedInstalled: expected,
		env: env, executionCanary: executionCanary, subprocessCounter: subprocessCounter,
	}
}

func phase6RepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func phase6ReadFixture(t *testing.T, root, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "core/cli/testdata/ingest", name))
	if err != nil {
		t.Fatalf("read Phase 6 fixture %s: %v", name, err)
	}
	return content
}

func (fixture *phase6BuiltFixture) replaceTargetFromFixture(t *testing.T, name string) {
	t.Helper()
	fixture.sourceTemplate = phase6ReadFixture(t, phase6RepositoryRoot(t), name)
	fixture.source = append([]byte(nil), fixture.sourceTemplate...)
	if err := os.WriteFile(fixture.target, fixture.source, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (fixture *phase6BuiltFixture) run(args ...string) (string, error) {
	cmd := exec.Command(fixture.binary, args...)
	cmd.Env = fixture.env
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte(fixture.secret)) {
		return "", errors.New("command output disclosed the runtime literal")
	}
	return string(out), err
}

func decodePhase6IngestResult(t *testing.T, encoded string) dto.IngestResult {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(encoded))
	var result dto.IngestResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode ingest result: %v; output=%q", err, encoded)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("ingest JSON has trailing content: %q", encoded)
	}
	return result
}

func phase6ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func phase6ReplaceEnv(env []string, values map[string]string, remove ...string) []string {
	blocked := make(map[string]bool, len(values)+len(remove))
	for name := range values {
		blocked[name] = true
	}
	for _, name := range remove {
		blocked[name] = true
	}
	result := make([]string, 0, len(env)+len(values))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			result = append(result, entry)
		}
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

func assertPhase6OutsideMarkerBytes(t *testing.T, before, after []byte) {
	t.Helper()
	begin := []byte("# >>> zsh-pro >>>")
	end := []byte("# <<< zsh-pro <<<")
	beforeStart, afterStart := bytes.Index(before, begin), bytes.Index(after, begin)
	beforeEnd, afterEnd := bytes.Index(before, end), bytes.Index(after, end)
	if beforeStart < 0 || afterStart < 0 || beforeEnd < beforeStart || afterEnd < afterStart {
		t.Fatal("marker fixture or installed target lacks one ordered marker pair")
	}
	if !bytes.Equal(before[:beforeStart], after[:afterStart]) ||
		!bytes.Equal(before[beforeEnd+len(end):], after[afterEnd+len(end):]) {
		t.Fatal("ingest changed a byte outside the exact marker region")
	}
}

func (fixture *phase6BuiltFixture) readMainProfile(t *testing.T) model.Profile {
	t.Helper()
	profileStore, err := store.New(fixture.storeRoot, zsh.Provider{}, &mainDeterministicKeychain{})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profileStore.Read(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func phase6ProfileContainsName(profile model.Profile, name string) bool {
	for _, entry := range profile.Entries {
		for _, candidate := range entry.Names {
			if candidate == name {
				return true
			}
		}
	}
	return false
}

func (fixture *phase6BuiltFixture) startupSnapshot(t *testing.T) string {
	t.Helper()
	_ = os.Remove(fixture.executionCanary)
	_ = os.Remove(fixture.subprocessCounter)
	script := `
source "$1" || exit 41
phase6_function || exit 42
typeset phase6_secret_equal=0
[[ "$PHASE6_API_TOKEN" == "$PHASE6_EXPECTED_SECRET" ]] && phase6_secret_equal=1
print -r -- "first=${(qqqq)PHASE6_FIRST}"
print -r -- "order=${(j:,:)PHASE6_ORDER_LEDGER}"
print -r -- "imperative=${(qqqq)PHASE6_IMPERATIVE_RESULT}"
print -r -- "opaque=${(qqqq)PHASE6_OPAQUE_RESULT}"
print -r -- "dynamic=${(qqqq)PHASE6_DYNAMIC_TOKEN}"
print -r -- "alias_body=${(qqqq)aliases[phase6_alias]}"
print -r -- "function_body=${(qqqq)functions[phase6_function]}"
print -r -- "function_result=${(qqqq)PHASE6_FUNCTION_RESULT}"
if [[ -o hist_ignore_dups ]]; then print -r -- 'hist_ignore_dups=1'; else print -r -- 'hist_ignore_dups=0'; fi
print -r -- "path=${(j:,:)path}"
print -r -- "secret_equal=$phase6_secret_equal"
`
	cmd := exec.Command(fixture.zshPath, "-f", "-c", script, "phase6-startup", fixture.target)
	cmd.Env = fixture.env
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte(fixture.secret)) {
		t.Fatal("startup output disclosed the runtime literal")
	}
	if err != nil {
		t.Fatalf("zsh -f startup oracle: %v\n%s", err, out)
	}
	if _, err := os.Stat(fixture.executionCanary); err != nil {
		t.Fatalf("explicit startup oracle did not source the actual target: %v", err)
	}
	if content, err := os.ReadFile(fixture.subprocessCounter); err == nil && len(content) != 0 {
		t.Fatalf("startup subprocess counter is nonzero: %q", content)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(out)
}

func (fixture *phase6BuiltFixture) symbolSnapshot(t *testing.T) ([]string, []string) {
	t.Helper()
	_ = os.Remove(fixture.executionCanary)
	_ = os.Remove(fixture.subprocessCounter)
	script := `
typeset -ga __phase6_before_parameters __phase6_before_functions
phase6_emit_symbol_delta() {
  local name
  local -a added_parameters added_functions
  for name in ${(k)parameters}; do
    (( ${__phase6_before_parameters[(Ie)$name]} )) || added_parameters+=("$name")
  done
  for name in ${(k)functions}; do
    (( ${__phase6_before_functions[(Ie)$name]} )) || added_functions+=("$name")
  done
  for name in ${(on)added_parameters}; do print -r -- "P:$name"; done
  for name in ${(on)added_functions}; do print -r -- "F:$name"; done
}
__phase6_before_parameters=( ${(k)parameters} )
__phase6_before_functions=( ${(k)functions} )
source "$1" || exit 51
phase6_emit_symbol_delta
`
	cmd := exec.Command(fixture.zshPath, "-f", "-c", script, "phase6-symbols", fixture.target)
	cmd.Env = fixture.env
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte(fixture.secret)) {
		t.Fatal("symbol oracle disclosed the runtime literal")
	}
	if err != nil {
		t.Fatalf("zsh -f symbol oracle: %v\n%s", err, out)
	}
	var parameters, functions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		switch {
		case strings.HasPrefix(line, "P:"):
			parameters = append(parameters, strings.TrimPrefix(line, "P:"))
		case strings.HasPrefix(line, "F:"):
			functions = append(functions, strings.TrimPrefix(line, "F:"))
		case line != "":
			t.Fatalf("unexpected symbol-oracle output %q", line)
		}
	}
	sort.Strings(parameters)
	sort.Strings(functions)
	return parameters, functions
}

func phase6SetDifference(left, right []string) []string {
	rightSet := make(map[string]bool, len(right))
	for _, value := range right {
		rightSet[value] = true
	}
	var difference []string
	for _, value := range left {
		if !rightSet[value] {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}

func phase6LiteralBearingPaths(t *testing.T, secret string, roots ...string) []string {
	t.Helper()
	var matches []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(content, []byte(secret)) {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(matches)
	return matches
}
