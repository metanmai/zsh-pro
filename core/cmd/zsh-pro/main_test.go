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

func TestBuiltBinaryInstallInitializesAndMigratesProfileStores(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	binary := buildInstalledBinary(t)

	locations := []struct {
		name      string
		configure func(*testing.T) (string, []string)
	}{
		{
			name: "HOME fallback",
			configure: func(t *testing.T) (string, []string) {
				home := t.TempDir()
				return filepath.Join(home, ".local", "share", "zsh-pro"), installedBinaryEnv(map[string]string{"HOME": home})
			},
		},
		{
			name: "XDG_DATA_HOME",
			configure: func(t *testing.T) (string, []string) {
				home := t.TempDir()
				dataHome := t.TempDir()
				return filepath.Join(dataHome, "zsh-pro"), installedBinaryEnv(map[string]string{"HOME": home, "XDG_DATA_HOME": dataHome})
			},
		},
		{
			name: "explicit ZSHPRO_HOME",
			configure: func(t *testing.T) (string, []string) {
				home := t.TempDir()
				root := filepath.Join(t.TempDir(), "profiles")
				return root, installedBinaryEnv(map[string]string{"HOME": home, "ZSHPRO_HOME": root})
			},
		},
	}

	for _, location := range locations {
		t.Run(location.name, func(t *testing.T) {
			root, env := location.configure(t)

			if out, err := runInstalledBinary(binary, env, "list"); err == nil {
				t.Fatalf("list before install unexpectedly succeeded: %s", out)
			}
			if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read-only list created profile storage at %s: %v", root, err)
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
