package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
	"zsh-pro/core/worktree"
)

func TestRuntimeWorktreeContractConstantsAndPrivateSurface(t *testing.T) {
	source, err := os.ReadFile("runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"const DefaultWorktreeTransitionBudget = 250 * time.Millisecond",
		"WorktreeRuntimeFrameVersion = 1",
		"MaxRuntimeWorktreeFrameBytes = model.MaxSnapshotBytes + 4096",
		"MaxRuntimeWorktreeFrameRecords = model.MaxSnapshotRecords + 16",
		`case "attach", "publish", "prepare", "acknowledge", "resolve":`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("runtime worktree contract is missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"ZSHPRO_SHELL_CAPABILITY", "os.Getenv(\"ZSHPRO_SHELL", "runtime worktree <pull", "runtime worktree <query",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("runtime worktree source contains forbidden alternate surface %q", forbidden)
		}
	}
}

func TestRuntimeWorktreeAttachAllocationAndCredentialForwarding(t *testing.T) {
	root := runtimeWorktreeTestRoot(t)
	authority := &runtimeTestAuthority{runtime: &runtimeTestWorktree{}}
	program := NewWithWorktree(nil, nil, NotReadyEmitter(), authority, nil)

	allocation := runtimeTestFrame(t,
		runtimeTestRecord{tag: 1, payload: []byte("attach")},
		runtimeTestRecord{tag: 2, payload: []byte("allocate")},
	)
	code, stdout, stderr := runRuntimeWorktreeWithStdin(t, program, root, allocation, "attach")
	if code != 0 || stderr != "" {
		t.Fatalf("attach allocation = (%d, %q, %q)", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 3 || lines[0] != "ZPWC 1" {
		t.Fatalf("credential response = %q", stdout)
	}
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !hex64.MatchString(lines[1]) || !hex64.MatchString(lines[2]) || lines[1] == lines[2] {
		t.Fatalf("allocated credential is not two independent 256-bit values: %q", stdout)
	}
	if authority.bindCount != 1 || authority.closeCount != 1 {
		t.Fatalf("allocation binding lifecycle = bind %d close %d", authority.bindCount, authority.closeCount)
	}
	if authority.runtime.(*runtimeTestWorktree).attachCalls != 0 {
		t.Fatal("credential allocation mutated Service through Attach")
	}

	snapshot := []byte("ZP_LIVE_SNAPSHOT\x001\x00E\x00")
	commit := runtimeTestFrame(t,
		runtimeTestRecord{tag: 1, payload: []byte("attach")},
		runtimeTestRecord{tag: 2, payload: []byte("commit")},
		runtimeTestRecord{tag: 3, payload: []byte(lines[1])},
		runtimeTestRecord{tag: 4, payload: []byte(lines[2])},
		runtimeTestRecord{tag: 5, payload: []byte("attach-commit-1")},
		runtimeTestRecord{tag: 32, payload: snapshot},
	)
	code, stdout, stderr = runRuntimeWorktreeWithStdin(t, program, root, commit, "attach")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("attach commit = (%d, %q, %q)", code, stdout, stderr)
	}
	called := authority.runtime.(*runtimeTestWorktree)
	if called.attachCalls != 1 || called.operationID != "attach-commit-1" || !bytes.Equal(called.frame, snapshot) {
		t.Fatalf("Attach forwarding = calls %d operation %q frame %q", called.attachCalls, called.operationID, called.frame)
	}
	capability, ok := called.credential.Capability.Bytes()
	if !ok || fmt.Sprintf("%x", capability) != lines[2] || called.credential.ShellID != lines[1] {
		t.Fatal("runtime changed the stdin credential before forwarding")
	}
}

func TestRuntimeWorktreeFrameRejectsMalformedBeforeService(t *testing.T) {
	root := runtimeWorktreeTestRoot(t)
	capability := strings.Repeat("a", 64)
	valid := []runtimeTestRecord{
		{tag: 1, payload: []byte("attach")},
		{tag: 2, payload: []byte("commit")},
		{tag: 3, payload: []byte(strings.Repeat("b", 64))},
		{tag: 4, payload: []byte(capability)},
		{tag: 5, payload: []byte("attach-commit")},
		{tag: 32, payload: []byte("ZP_LIVE_SNAPSHOT\x001\x00E\x00")},
	}
	tests := map[string][]byte{
		"operation mismatch": runtimeTestFrame(t, append([]runtimeTestRecord(nil), valid...)...),
		"duplicate control":  runtimeTestFrame(t, append(append([]runtimeTestRecord(nil), valid[:3]...), append([]runtimeTestRecord{{tag: 3, payload: []byte(strings.Repeat("c", 64))}}, valid[3:]...)...)...),
		"unknown control":    runtimeTestFrame(t, append(append([]runtimeTestRecord(nil), valid[:5]...), runtimeTestRecord{tag: 30, payload: []byte("unknown")}, valid[5])...),
		"trailing bytes":     append(runtimeTestFrame(t, valid...), []byte("trailing")...),
		"partial payload":    runtimeTestFrame(t, valid...)[:len(runtimeTestFrame(t, valid...))-3],
		"oversized":          bytes.Repeat([]byte{'x'}, 2_101_249),
	}
	for name, frame := range tests {
		t.Run(name, func(t *testing.T) {
			authority := &runtimeTestAuthority{runtime: &runtimeTestWorktree{}}
			program := NewWithWorktree(nil, nil, NotReadyEmitter(), authority, nil)
			op := "attach"
			if name == "operation mismatch" {
				op = "publish"
			}
			code, stdout, stderr := runRuntimeWorktreeWithStdin(t, program, root, frame, op)
			if code == 0 || stdout != "" || stderr == "" {
				t.Fatalf("malformed frame = (%d, %q, %q)", code, stdout, stderr)
			}
			if strings.Contains(stderr, capability) || authority.runtime.(*runtimeTestWorktree).serviceCalls() != 0 {
				t.Fatalf("malformed frame disclosed a capability or called Service: %q", stderr)
			}
		})
	}
}

func TestRuntimeWorktreeReplyOutputIsWholeAndNoOpIsEmpty(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	root := runtimeWorktreeTestRoot(t)
	credentialRecords := []runtimeTestRecord{
		{tag: 1, payload: []byte("prepare")},
		{tag: 3, payload: []byte(strings.Repeat("b", 64))},
		{tag: 4, payload: []byte(strings.Repeat("a", 64))},
		{tag: 5, payload: []byte("prepare-1")},
		{tag: 6, payload: []byte("1")},
		{tag: 10, payload: []byte("__zp09_apply")},
		{tag: 11, payload: []byte("__zp09_reverse")},
		{tag: 32, payload: []byte("ZP_LIVE_SNAPSHOT\x001\x00E\x00")},
	}
	want := "typeset -g ZP_WORKTREE_REPLY_PROTOCOL=1\n" +
		"typeset -g ZP_WORKTREE_REPLY_REVISION=2\n" +
		"typeset -g ZP_WORKTREE_REPLY_TOKEN=3\n" +
		"typeset -g ZP_WORKTREE_REPLY_FINGERPRINT='" + strings.Repeat("0", 64) + "'\n" +
		"typeset -g ZP_WORKTREE_REPLY_COMPLETE=1\n"

	runtime := &runtimeTestWorktree{preparePayload: runtimePatchPayload{transition: true, source: []byte(want)}}
	authority := &runtimeTestAuthority{runtime: runtime}
	program := NewWithWorktree(nil, nil, NotReadyEmitter(), authority, nil)
	code, stdout, stderr := runRuntimeWorktreeWithStdin(t, program, root, runtimeTestFrame(t, credentialRecords...), "prepare")
	if code != 0 || stdout != want || stderr != "" || runtime.prepareCalls != 1 {
		t.Fatalf("prepare transition = (%d, %q, %q), calls %d", code, stdout, stderr, runtime.prepareCalls)
	}

	runtime.preparePayload = runtimePatchPayload{}
	credentialRecords[3].payload = []byte("prepare-2")
	code, stdout, stderr = runRuntimeWorktreeWithStdin(t, program, root, runtimeTestFrame(t, credentialRecords...), "prepare")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("prepare at-head no-op = (%d, %q, %q)", code, stdout, stderr)
	}
}

func TestRuntimeWorktreeBudgetSeamIsSealedAndCumulative(t *testing.T) {
	source, err := os.ReadFile("runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{"type transitionClock interface", "type transitionBudget struct", "newTransitionBudget", "budget.check"} {
		if !strings.Contains(text, want) {
			t.Errorf("runtime budget seam is missing %q", want)
		}
	}
	for _, forbidden := range []string{"ZSHPRO_WORKTREE_TIMEOUT", "WORKTREE_TRANSITION_BUDGET", "DefaultWorktreeTransitionBudget = 0"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("runtime exposes forbidden budget override %q", forbidden)
		}
	}
}

type runtimeTestRecord struct {
	tag     int
	payload []byte
}

func runtimeTestFrame(t *testing.T, records ...runtimeTestRecord) []byte {
	t.Helper()
	var frame bytes.Buffer
	_, _ = fmt.Fprintf(&frame, "ZPWT 1 %d\n", len(records)+1)
	for _, record := range records {
		_, _ = fmt.Fprintf(&frame, "%d %d\n", record.tag, len(record.payload))
		_, _ = frame.Write(record.payload)
		_ = frame.WriteByte('\n')
	}
	_, _ = fmt.Fprint(&frame, "255 0\n\n")
	return frame.Bytes()
}

func runtimeWorktreeTestRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func runRuntimeWorktreeWithStdin(t *testing.T, program *CLI, root string, frame []byte, operation string) (int, string, string) {
	t.Helper()
	t.Setenv("ZSHPRO_HOME", root)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() {
		_, writeErr := writer.Write(frame)
		if closeErr := writer.Close(); writeErr == nil {
			writeErr = closeErr
		}
		written <- writeErr
	}()
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
	})
	var stdout, stderr bytes.Buffer
	code := program.Run([]string{"runtime", "worktree", operation, "5"}, &stdout, &stderr)
	os.Stdin = previous
	if err := reader.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	<-written // A pre-read rejection may close the pipe before all bytes arrive.
	return code, stdout.String(), stderr.String()
}

type runtimeTestAuthority struct {
	runtime    RuntimeWorktree
	bindCount  int
	closeCount int
}

func (*runtimeTestAuthority) WorkflowStatus(context.Context, string) (worktree.WorkflowStatus, error) {
	return worktree.WorkflowStatus{}, nil
}
func (*runtimeTestAuthority) Diff(context.Context) (model.CategorizedDiff, error) {
	return model.CategorizedDiff{}, nil
}
func (*runtimeTestAuthority) Branches(context.Context) ([]string, error) { return nil, nil }
func (authority *runtimeTestAuthority) BindRuntimeWorktree(*RuntimeRoot) (RuntimeWorktree, io.Closer, error) {
	authority.bindCount++
	return authority.runtime, runtimeTestCloser{close: func() { authority.closeCount++ }}, nil
}

type runtimeTestCloser struct{ close func() }

func (closer runtimeTestCloser) Close() error {
	closer.close()
	return nil
}

type runtimeTestWorktree struct {
	attachCalls    int
	publishCalls   int
	prepareCalls   int
	ackCalls       int
	resolveCalls   int
	credential     model.ShellCredential
	operationID    string
	frame          []byte
	preparePayload runtimePatchPayload
}

func (runtime *runtimeTestWorktree) Attach(_ context.Context, credential model.ShellCredential, operationID string, frame []byte) (runtimeAttachCredential, error) {
	runtime.attachCalls++
	runtime.credential, runtime.operationID, runtime.frame = credential, operationID, append([]byte(nil), frame...)
	return runtimeAttachCredential{}, nil
}
func (runtime *runtimeTestWorktree) Publish(context.Context, model.ShellCredential, string, uint64, model.LiveSnapshot, []byte) (model.PublishResult, error) {
	runtime.publishCalls++
	return model.PublishResult{}, nil
}
func (runtime *runtimeTestWorktree) Prepare(context.Context, model.ShellCredential, string, uint64, []byte, string, string) (runtimePatchPayload, error) {
	runtime.prepareCalls++
	return runtime.preparePayload, nil
}
func (runtime *runtimeTestWorktree) Acknowledge(context.Context, model.ShellCredential, string, uint64, uint64, []byte) (model.AcknowledgeResult, error) {
	runtime.ackCalls++
	return model.AcknowledgeResult{}, nil
}
func (runtime *runtimeTestWorktree) Resolve(context.Context, model.ShellCredential, string, model.Identity, model.ResolutionToken, []byte, string, string) (runtimePatchPayload, error) {
	runtime.resolveCalls++
	return runtimePatchPayload{}, nil
}
func (runtime *runtimeTestWorktree) serviceCalls() int {
	return runtime.attachCalls + runtime.publishCalls + runtime.prepareCalls + runtime.ackCalls + runtime.resolveCalls
}

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
