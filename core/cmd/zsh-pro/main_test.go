package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"zsh-pro/core/cli"
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
		binDir := writeValidationShim(t, gitPath, "#!/bin/sh\nexec /bin/rm \"$2\"\n")
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
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
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
