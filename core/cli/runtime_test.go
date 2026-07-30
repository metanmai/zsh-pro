package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

func TestRuntimeCaptureUsesPrivatePipeAndRejectsUnsafeRoots(t *testing.T) {
	emitter := NewRuntimeEmitterWithRuntimeStore(nil, zsh.Provider{}, nil, func(*RuntimeRoot) (Store, SecretResolver, error) {
		return fakeStore{}, nil, nil
	})
	c := New(nil, nil, emitter)

	run := func(t *testing.T, root string) (int, string, string) {
		t.Helper()
		t.Setenv("ZSHPRO_HOME", root)
		var stdout, stderr bytes.Buffer
		code := c.Run(
			[]string{"runtime", "capture", "1", "--", "zsh-pro", "emit", "apply", "B"},
			&stdout,
			&stderr,
		)
		return code, stdout.String(), stderr.String()
	}

	safeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(safeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run(t, safeRoot); code != 0 || !strings.Contains(stdout, "_zp_run_payload") || stderr != "" {
		t.Fatalf("safe capture = (%d, %q, %q)", code, stdout, stderr)
	}
	entries, err := os.ReadDir(safeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runtime capture staged a file under the configured root: %v", entries)
	}
	if info, err := os.Stat(os.TempDir()); err == nil && info.Mode()&os.ModeSticky != 0 {
		stickyRoot, err := os.MkdirTemp(os.TempDir(), "zsh-pro-sticky-safe-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(stickyRoot) })
		if err := os.Chmod(stickyRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if code, stdout, stderr := run(t, stickyRoot); code != 0 || !strings.Contains(stdout, "_zp_run_payload") || stderr != "" {
			t.Fatalf("sticky-ancestor capture = (%d, %q, %q)", code, stdout, stderr)
		}
	}

	link := filepath.Join(t.TempDir(), "attacker-owned-runtime-link")
	if err := os.Symlink(safeRoot, link); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, link); code == 0 || stdout != "" {
		t.Fatalf("symlink runtime root was accepted: (%d, %q)", code, stdout)
	}

	componentTarget := filepath.Join(t.TempDir(), "component-target")
	if err := os.Mkdir(componentTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	componentRoot := filepath.Join(componentTarget, "runtime")
	if err := os.Mkdir(componentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	componentLink := filepath.Join(t.TempDir(), "attacker-owned-component-link")
	if err := os.Symlink(componentTarget, componentLink); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, filepath.Join(componentLink, "runtime")); code == 0 || stdout != "" {
		t.Fatalf("symlink runtime component was accepted: (%d, %q)", code, stdout)
	}

	unsafeParent := filepath.Join(t.TempDir(), "unsafe-parent")
	if err := os.Mkdir(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	unsafeRoot := filepath.Join(unsafeParent, "runtime")
	if err := os.Mkdir(unsafeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, unsafeRoot); code == 0 || stdout != "" {
		t.Fatalf("non-sticky writable ancestor was accepted: (%d, %q)", code, stdout)
	}
}

func TestRuntimeCaptureBoundsEmitter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSHPRO_HOME", root)
	emitter := NewRuntimeEmitterWithRuntimeStore(nil, zsh.Provider{}, nil, func(*RuntimeRoot) (Store, SecretResolver, error) {
		return runtimeTimeoutStore{}, nil, nil
	})

	started := time.Now()
	var stdout, stderr bytes.Buffer
	code := New(nil, nil, emitter).Run(
		[]string{"runtime", "capture", "1", "--", "zsh-pro", "emit", "apply", "B"},
		&stdout,
		&stderr,
	)
	if code != 124 {
		t.Fatalf("timeout code = %d, want 124 (stderr %q)", code, stderr.String())
	}
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("runtime capture exceeded its deadline: %s", elapsed)
	}
	if stdout.Len() != 0 {
		t.Fatalf("timed-out capture returned output: %q", stdout.String())
	}
}

type runtimeTimeoutStore struct{}

func (runtimeTimeoutStore) Branches(context.Context) ([]string, error) { return nil, nil }
func (runtimeTimeoutStore) Current() string                            { return "" }
func (runtimeTimeoutStore) Checkout(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}
func (runtimeTimeoutStore) Read(context.Context, string) (model.Profile, error) {
	return model.Profile{}, nil
}

// TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors exercises the
// production runtime path without an emitter shim. It stops immediately after
// secureRuntimeRoot has retained its descriptors, replaces an ancestor in
// sticky /tmp with an attacker symlink, and then verifies the real Store plus
// zsh runtime emitter read and evaluate only the original profile and vault.
func TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	stickyParent := os.TempDir()
	info, err := os.Stat(stickyParent)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSticky == 0 {
		t.Skipf("%s is not sticky", stickyParent)
	}

	victimParent, err := os.MkdirTemp(stickyParent, "zsh-pro-runtime-bound-")
	if err != nil {
		t.Fatal(err)
	}
	victimHeld := victimParent + "-held"
	t.Cleanup(func() {
		_ = os.Remove(victimParent)
		_ = os.RemoveAll(victimHeld)
	})
	victimRoot := filepath.Join(victimParent, "zsh-pro")
	if err := os.Mkdir(victimRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	attackerParent := t.TempDir()
	attackerRoot := filepath.Join(attackerParent, "zsh-pro")
	if err := os.Mkdir(attackerRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	const (
		victimProfile   = "victim-profile"
		attackerProfile = "attacker-profile"
		victimSecret    = "victim-runtime-vault-secret"
		attackerSecret  = "attacker-runtime-vault-secret"
	)
	seedRuntimeRaceStore(t, victimRoot, runtimeRaceProfile(t, victimProfile))
	seedRuntimeRaceStore(t, attackerRoot, runtimeRaceProfile(t, attackerProfile))
	writeRuntimeRaceVault(t, victimParent, victimSecret)
	writeRuntimeRaceVault(t, attackerParent, attackerSecret)

	// Force the actual runtime factory to select the file vault. The only
	// executable it may discover is git, carried in this private test PATH.
	binDir := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ZSHPRO_HOME", victimRoot)

	boundErrors := make(chan error, 2)
	emitter := NewRuntimeEmitterWithRuntimeStore(nil, zsh.Provider{}, nil, func(root *RuntimeRoot) (Store, SecretResolver, error) {
		repository, vaultParent := root.Files()
		bound, err := store.NewRuntime(repository, vaultParent, zsh.Provider{})
		if err != nil {
			boundErrors <- err
			return nil, nil, err
		}
		return runtimeRaceStoreObserver{Store: bound, errors: boundErrors}, bound.RuntimeSecretResolver(), nil
	})
	c := New(nil, nil, emitter)

	previousHook := runtimeRootValidated
	validated := make(chan struct{})
	resume := make(chan struct{})
	runtimeRootValidated = func(*RuntimeRoot) {
		close(validated)
		<-resume
	}
	defer func() { runtimeRootValidated = previousHook }()

	type captureResult struct {
		code   int
		stdout string
		stderr string
	}
	result := make(chan captureResult, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		code := c.Run(
			[]string{"runtime", "capture", "5", "--", "zsh-pro", "emit", "apply", "B"},
			&stdout,
			&stderr,
		)
		result <- captureResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}()
	<-validated
	if err := os.Rename(victimParent, victimHeld); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(attackerParent, victimParent); err != nil {
		t.Fatal(err)
	}
	close(resume)

	captured := <-result
	if captured.code != 0 || captured.stderr != "" {
		select {
		case err := <-boundErrors:
			t.Fatalf("descriptor-bound capture = (%d, %q): %v", captured.code, captured.stderr, err)
		default:
		}
		t.Fatalf("descriptor-bound capture = (%d, %q)", captured.code, captured.stderr)
	}
	for _, want := range []string{victimProfile, victimSecret} {
		if !strings.Contains(captured.stdout, want) {
			t.Fatalf("captured source did not use authenticated victim data %q:\n%s", want, captured.stdout)
		}
	}
	for _, forbidden := range []string{attackerProfile, attackerSecret} {
		if strings.Contains(captured.stdout, forbidden) {
			t.Fatalf("captured source followed swapped attacker path %q:\n%s", forbidden, captured.stdout)
		}
	}

	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	payload := filepath.Join(dir, "payload.zsh")
	if err := os.WriteFile(loader, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte(captured.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(realZsh, "-f", "-c", `
source "$1"
typeset -g ZP_BASE_PATH="$PATH"
source "$2"
[[ "$ZP_RACE_PROFILE" == "$3" && "$ZP_RACE_SECRET" == "$4" ]] || exit 10
[[ "$ZP_RACE_PROFILE" != "$5" && "$ZP_RACE_SECRET" != "$6" ]] || exit 11
[[ -n "${ZP_ACTIVE_REVERSE_FN-}" && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 12
typeset -g +x ZP_ACTIVE_PROFILE=B
export ZSHPRO_PROFILE=B
deactivate
[[ -z "${ZP_RACE_PROFILE+x}" && -z "${ZP_RACE_SECRET+x}" ]] || exit 13
`, "zsh-pro-runtime-bound-test", loader, payload, victimProfile, victimSecret, attackerProfile, attackerSecret)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("captured source evaluated attacker data or failed victim reversal: %v\n%s", err, out)
	}
}

func seedRuntimeRaceStore(t *testing.T, root string, profile model.Profile) {
	t.Helper()
	s, err := store.New(root, zsh.Provider{}, runtimeRaceVaultSeeder{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, "B"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, "B", profile, "runtime race fixture"); err != nil {
		t.Fatal(err)
	}
}

func runtimeRaceProfile(t *testing.T, identity string) model.Profile {
	t.Helper()
	profile := transitionProfile(t, "export ZP_RACE_PROFILE="+identity+"\n")
	literal := "runtime-race-seed"
	profile.Entries = append(profile.Entries, model.Entry{
		Text:                    "export ZP_RACE_SECRET='runtime-race-seed'",
		Value:                   "'runtime-race-seed'",
		Category:                model.CatSecrets,
		Kind:                    model.KindAssignment,
		Names:                   []string{"ZP_RACE_SECRET"},
		Exported:                true,
		Managed:                 true,
		StructuralFidelityKnown: true,
		RuntimeValue:            &literal,
		ValueMode:               model.ValueModeLiteral,
	})
	return profile
}

type runtimeRaceVaultSeeder struct{}

func (runtimeRaceVaultSeeder) Store(string, string) error      { return nil }
func (runtimeRaceVaultSeeder) Retrieve(string) (string, error) { return "", nil }
func (runtimeRaceVaultSeeder) Delete(string) error             { return nil }
func (runtimeRaceVaultSeeder) Kind() model.SecretRefKind       { return model.SecretRefFile }

type runtimeRaceStoreObserver struct {
	Store
	errors chan<- error
}

func (s runtimeRaceStoreObserver) Checkout(ctx context.Context, name string) error {
	err := s.Store.Checkout(ctx, name)
	if err != nil {
		s.errors <- err
	}
	return err
}

func (s runtimeRaceStoreObserver) Read(ctx context.Context, name string) (model.Profile, error) {
	profile, err := s.Store.Read(ctx, name)
	if err != nil {
		s.errors <- err
	}
	return profile, err
}

func writeRuntimeRaceVault(t *testing.T, parent, value string) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	if err := os.WriteFile(filepath.Join(parent, ".zsh-pro-vault"), []byte("ZP_RACE_SECRET="+encoded+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRuntimeSourceUsesPipeAndSuppressesSourceDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	if err := validateRuntimeSource(context.Background(), strings.NewReader("export ZP_VALID=1\n")); err != nil {
		t.Fatalf("valid pipe source: %v", err)
	}
	secretLike := "phase5-runtime-fixture"
	err := validateRuntimeSource(context.Background(), strings.NewReader("if then "+secretLike+"\n"))
	if err == nil {
		t.Fatal("invalid pipe source was accepted")
	}
	if strings.Contains(err.Error(), secretLike) {
		t.Fatalf("validator error leaked source data: %v", err)
	}
}
