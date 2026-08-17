package zsh

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"zsh-pro/core/worktree"
)

type retainedWorktreeShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
	seq    int
}

func startRetainedWorktreeShell(t *testing.T, zshPath, dir string, env []string) *retainedWorktreeShell {
	t.Helper()
	cmd := exec.Command(zshPath, "-f")
	cmd.Dir = dir
	cmd.Env = append([]string(nil), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	shell := &retainedWorktreeShell{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}
	cmd.Stderr = &shell.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shell.close() })
	return shell
}

func (shell *retainedWorktreeShell) run(t *testing.T, source string) (string, int) {
	t.Helper()
	shell.seq++
	begin := fmt.Sprintf("__ZP_TEST_BEGIN_%d__", shell.seq)
	end := fmt.Sprintf("__ZP_TEST_END_%d__", shell.seq)
	if _, err := fmt.Fprintf(shell.stdin, "print -r -- %q\n{\n%s\n}\n__zp_test_rc=$?\nprint -r -- %q\"$__zp_test_rc\"\n", begin, source, end+":"); err != nil {
		t.Fatalf("write retained shell command: %v", err)
	}
	var output strings.Builder
	seenBegin := false
	for {
		line, err := shell.stdout.ReadString('\n')
		if err != nil {
			t.Fatalf("retained shell ended before %s: %v; stderr=%q", end, err, shell.stderr.String())
		}
		trimmed := strings.TrimSuffix(line, "\n")
		if !seenBegin {
			if trimmed == begin {
				seenBegin = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, end+":") {
			rc, err := strconv.Atoi(strings.TrimPrefix(trimmed, end+":"))
			if err != nil {
				t.Fatalf("invalid retained-shell status %q", trimmed)
			}
			return output.String(), rc
		}
		output.WriteString(line)
	}
}

func (shell *retainedWorktreeShell) runOK(t *testing.T, source string) string {
	t.Helper()
	output, rc := shell.run(t, source)
	if rc != 0 {
		t.Fatalf("retained shell command failed with %d; output=%q stderr=%q", rc, output, shell.stderr.String())
	}
	return output
}

func (shell *retainedWorktreeShell) close() {
	if shell == nil || shell.cmd == nil || shell.cmd.ProcessState != nil {
		return
	}
	_, _ = io.WriteString(shell.stdin, "exit\n")
	_ = shell.stdin.Close()
	if err := shell.cmd.Wait(); err != nil && shell.cmd.ProcessState == nil {
		_ = shell.cmd.Process.Kill()
	}
}

func TestWorktreeTwoShellEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	repoRoot := liveTestRepositoryRoot(t)
	testRoot := t.TempDir()
	binDir := filepath.Join(testRoot, "bin")
	home := filepath.Join(testRoot, "home")
	runtimeRoot := filepath.Join(testRoot, "runtime")
	counterDir := filepath.Join(testRoot, "source-counter")
	for _, dir := range []string{binDir, home, counterDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(binDir, "zsh-pro")
	build := exec.Command("go", "build", "-o", binary, "./core/cmd/zsh-pro")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production binary: %v\n%s", err, output)
	}

	source := filepath.Join(home, "source.zsh")
	if err := os.WriteFile(source, []byte("export DEMO_SHARED_ENV=initial\nalias demo_seed='print -r -- seeded'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	basePath := "/usr/bin:/bin"
	ingest := exec.Command(binary, "ingest", source)
	ingest.Env = []string{"HOME=" + home, "ZDOTDIR=" + home, "ZSHPRO_HOME=" + runtimeRoot, "PATH=" + basePath, "LC_ALL=C"}
	if output, err := ingest.CombinedOutput(); err != nil {
		t.Fatalf("production ingest: %v\n%s", err, output)
	}
	loader := filepath.Join(runtimeRoot, "loader.zsh")
	if _, err := os.Stat(loader); err != nil {
		t.Fatalf("installed loader: %v", err)
	}

	counter := filepath.Join(testRoot, "source-process-count")
	shim := filepath.Join(counterDir, "zsh-pro")
	shimSource := "#!/bin/sh\nprintf invoked > \"$ZP_SOURCE_PROCESS_COUNT\"\nexit 99\n"
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	dirA := filepath.Join(testRoot, "shell-a")
	dirB := filepath.Join(testRoot, "shell-b")
	for _, dir := range []string{dirA, dirB} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
		"ZSHPRO_HOME=" + runtimeRoot,
		"PATH=" + counterDir + ":" + basePath,
		"TERM=dumb",
		"LC_ALL=C",
		"ZP_SOURCE_PROCESS_COUNT=" + counter,
		"INHERITED_ORDINARY=before",
	}
	shellA := startRetainedWorktreeShell(t, zshPath, dirA, env)
	shellB := startRetainedWorktreeShell(t, zshPath, dirB, env)
	for _, shell := range []*retainedWorktreeShell{shellA, shellB} {
		shell.runOK(t, "source "+worktreeShellQuote(loader))
		if _, err := os.Stat(counter); !os.IsNotExist(err) {
			t.Fatalf("loader source invoked a helper: %v", err)
		}
		shell.runOK(t, "PATH="+worktreeShellQuote(binDir+":"+basePath)+"; rehash; _zp_worktree_precmd; [[ $ZP_WORKTREE_ATTACHED == 1 && $ZP_WORKTREE_ATTACHED_NOW == 1 ]]")
		state := strings.TrimSpace(shell.runOK(t, "_zp_worktree_line_finish; print -r -- \"$ZP_WORKTREE_APPLIED_REVISION|${#ZP_WORKTREE_LAST_ERROR}|${DEMO_SHARED_ENV-unset}|${aliases[demo_seed]-unset}\""))
		if state != "1|0|initial|print -r -- seeded" {
			t.Fatalf("initial worktree convergence state = %q", state)
		}
	}

	identityA := strings.TrimSpace(shellA.runOK(t, "print -r -- \"$$|$PWD|$ZP_WORKTREE_SHELL_ID|$ZP_WORKTREE_CAPABILITY|${parameters[ZP_WORKTREE_CAPABILITY]}\""))
	identityB := strings.TrimSpace(shellB.runOK(t, "print -r -- \"$$|$PWD|$ZP_WORKTREE_SHELL_ID|$ZP_WORKTREE_CAPABILITY|${parameters[ZP_WORKTREE_CAPABILITY]}\""))
	partsA := strings.Split(identityA, "|")
	partsB := strings.Split(identityB, "|")
	if len(partsA) != 5 || len(partsB) != 5 {
		t.Fatal("retained shell identity output was malformed")
	}
	if partsA[0] == partsB[0] || partsA[1] == partsB[1] || partsA[2] == partsB[2] || partsA[3] == partsB[3] {
		t.Fatal("retained shells did not have independent PID/PWD/credential pairs")
	}
	for _, parts := range [][]string{partsA, partsB} {
		if !isLowerHex64(parts[2]) || !isLowerHex64(parts[3]) || strings.Contains(parts[4], "export") {
			t.Fatal("shell ID/capability pair was not stable private 256-bit state")
		}
	}

	for _, shell := range []*retainedWorktreeShell{shellA, shellB} {
		shell.runOK(t, "PATH="+worktreeShellQuote(counterDir+":"+basePath)+"; rehash; source "+worktreeShellQuote(loader)+"; PATH="+worktreeShellQuote(binDir+":"+basePath)+"; rehash")
		if _, err := os.Stat(counter); !os.IsNotExist(err) {
			t.Fatalf("loader re-source invoked a helper: %v", err)
		}
	}
	stableA := strings.TrimSpace(shellA.runOK(t, "print -r -- \"$ZP_WORKTREE_SHELL_ID|$ZP_WORKTREE_CAPABILITY\""))
	if stableA != partsA[2]+"|"+partsA[3] {
		t.Fatal("loader re-source replaced the authenticated pair")
	}

	shellA.runOK(t, "typeset -g BUFFER=buffer-a; export DEMO_LIVE_ENV=$'line one\\nline two'; export INHERITED_ORDINARY=changed; export CREATED_API_TOKEN=secret-canary; alias demo.live='print -r -- live-from-a'; functions[demo_live_fn]=$'print -r -- function-one\\nprint -r -- function-two'")
	shellA.runOK(t, "_zp_worktree_precmd; [[ $ZP_WORKTREE_CONFLICT_COUNT == 0 ]]")
	categoryState := strings.TrimSpace(shellB.runOK(t, "typeset -g BUFFER=buffer-b; _zp_worktree_line_finish; if [[ ${aliases[demo.live]-} == 'print -r -- live-from-a' ]]; then alias_ok=1; else alias_ok=0; fi; if [[ $DEMO_LIVE_ENV == $'line one\\nline two' && $INHERITED_ORDINARY == before && ${+CREATED_API_TOKEN} == 0 && ${+functions[demo_live_fn]} == 1 ]]; then category_ok=1; else category_ok=0; fi; print -r -- \"${#ZP_WORKTREE_LAST_ERROR}|$ZP_RUNTIME_TIMED_OUT|$BUFFER|$alias_ok|$category_ok|${ZP_ACTIVE_REVERSE_FN-unset}|${+ZP_WORKTREE_REPLY_COMPLETE}\""))
	if categoryState != "0|0|buffer-b|1|1|__expected_reverse__|0" {
		fields := strings.Split(categoryState, "|")
		if len(fields) != 7 || fields[0] != "0" || fields[1] != "0" || fields[2] != "buffer-b" || fields[3] != "1" || fields[4] != "1" || !strings.HasPrefix(fields[5], "__zp_worktree_reverse_") || fields[6] != "0" {
			t.Fatalf("cross-shell category convergence = %q; status=%q", categoryState, shellB.runOK(t, "zsh-pro status"))
		}
	}
	if output := shellB.runOK(t, "demo.live"); output != "live-from-a\n" {
		t.Fatalf("receiving shell's next alias command = %q", output)
	}
	if output := shellB.runOK(t, "demo_live_fn"); output != "function-one\nfunction-two\n" {
		t.Fatalf("receiving shell's multiline function = %q", output)
	}
	assertWorktreeStateExcludes(t, runtimeRoot, "secret-canary")

	shellA.runOK(t, "zsh-pro sync; [[ ${aliases[demo.live]} == 'print -r -- live-from-a' ]]")
	shellB.runOK(t, "zsh-pro config set auto-apply false >/dev/null")
	shellA.runOK(t, "alias demo.live='print -r -- manual-sync'; _zp_worktree_precmd")
	disabledState := strings.TrimSpace(shellB.runOK(t, "_zp_worktree_line_finish; print -r -- \"${aliases[demo.live]}|$ZP_WORKTREE_AUTO_APPLY_EFFECTIVE|${#ZP_WORKTREE_LAST_ERROR}\""))
	if disabledState != "print -r -- live-from-a|false|0" {
		t.Fatalf("disabled auto-apply shell state = %q", disabledState)
	}
	status := shellB.runOK(t, "zsh-pro status")
	if !strings.Contains(status, "behind: true\n") || !strings.Contains(status, "auto-apply default: false\n") {
		t.Fatalf("disabled auto-apply status = %q", status)
	}
	shellB.runOK(t, "zsh-pro sync; [[ ${aliases[demo.live]} == 'print -r -- manual-sync' ]]")
	shellB.runOK(t, "export ZSHPRO_AUTO_APPLY=invalid; _zp_worktree_line_finish; [[ $ZP_WORKTREE_LAST_ERROR == 'invalid ZSHPRO_AUTO_APPLY override; using persisted default' && $ZP_WORKTREE_AUTO_APPLY_EFFECTIVE == false ]]; unset ZSHPRO_AUTO_APPLY; zsh-pro config set auto-apply true >/dev/null")
	shellA.runOK(t, "zsh-pro sync; unset DEMO_LIVE_ENV CREATED_API_TOKEN; unfunction demo_live_fn; _zp_worktree_precmd")
	shellB.runOK(t, "_zp_worktree_line_finish; [[ ${+DEMO_LIVE_ENV} == 0 && ${+functions[demo_live_fn]} == 0 && $INHERITED_ORDINARY == before ]]")

	shellA.runOK(t, "zsh-pro sync; alias demo_disjoint_a='print -r -- a'")
	shellB.runOK(t, "zsh-pro sync; alias demo_disjoint_b='print -r -- b'")
	shellA.runOK(t, "_zp_worktree_precmd; [[ $ZP_WORKTREE_CONFLICT_COUNT == 0 ]]")
	shellB.runOK(t, "_zp_worktree_precmd; [[ $ZP_WORKTREE_CONFLICT_COUNT == 0 ]]")
	shellA.runOK(t, "_zp_worktree_line_finish; [[ ${+aliases[demo_disjoint_a]} == 1 && ${+aliases[demo_disjoint_b]} == 1 ]]")
	shellB.runOK(t, "_zp_worktree_line_finish; [[ ${+aliases[demo_disjoint_a]} == 1 && ${+aliases[demo_disjoint_b]} == 1 ]]")

	shellA.runOK(t, "zsh-pro sync; alias demo_race='print -r -- winner-a'")
	shellB.runOK(t, "zsh-pro sync; alias demo_race='print -r -- loser-b'")
	shellA.runOK(t, "_zp_worktree_precmd; [[ $ZP_WORKTREE_CONFLICT_COUNT == 0 ]]")
	shellB.runOK(t, "_zp_worktree_precmd; [[ $ZP_WORKTREE_CONFLICT_COUNT == 1 && -n $ZP_WORKTREE_CONFLICT_TOKEN ]]")
	resolutionState := strings.TrimSpace(shellB.runOK(t, "zsh-pro sync --resolve shared; rc=$?; print -r -- \"$rc|${aliases[demo_race]-unset}|$ZP_WORKTREE_CONFLICT_COUNT|${(q)ZP_WORKTREE_LAST_ERROR}\""))
	if resolutionState != "0|print -r -- winner-a|0|''" {
		t.Fatalf("shared conflict resolution = %q; status=%q", resolutionState, shellB.runOK(t, "zsh-pro status"))
	}
	shellA.runOK(t, "_zp_worktree_line_finish")

	diff := shellA.runOK(t, "zsh-pro diff")
	if !strings.Contains(diff, "aliases:\n") || !strings.Contains(diff, "demo_race") || strings.Contains(diff, "winner-a") {
		t.Fatalf("value-free shared diff = %q", diff)
	}
	if output := shellA.runOK(t, "zsh-pro commit -m 'two-shell workflow'"); output != "committed\n" {
		t.Fatalf("worktree commit output = %q", output)
	}
	branchBefore := shellA.runOK(t, "zsh-pro branch")
	for _, invalid := range []string{"zsh-pro checkout", "zsh-pro checkout -b", "zsh-pro config set auto-apply"} {
		if _, rc := shellA.run(t, invalid); rc != 2 {
			t.Fatalf("incomplete command %q exit = %d, want 2", invalid, rc)
		}
	}
	if branchAfter := shellA.runOK(t, "zsh-pro branch"); branchAfter != branchBefore {
		t.Fatal("incomplete worktree commands changed branch state")
	}
	shellA.runOK(t, "zsh-pro checkout -b feature >/dev/null; zsh-pro branch topic >/dev/null; zsh-pro checkout main >/dev/null")
	shellA.runOK(t, "alias demo_race='print -r -- dirty-before-checkout'")
	if _, rc := shellA.run(t, "zsh-pro checkout feature >/dev/null"); rc == 0 {
		t.Fatal("dirty checkout unexpectedly succeeded")
	}
	status = shellA.runOK(t, "zsh-pro status")
	if !strings.Contains(status, "branch: main\n") || !strings.Contains(status, "dirty identities:") {
		t.Fatalf("dirty checkout status = %q", status)
	}
	resetState := strings.TrimSpace(shellA.runOK(t, "zsh-pro reset --hard >/dev/null; rc=$?; print -r -- \"$rc|${aliases[demo_race]-unset}|${(q)ZP_WORKTREE_LAST_ERROR}\""))
	if resetState != "0|print -r -- winner-a|''" {
		t.Fatalf("parent-shell reset convergence = %q; status=%q; durable=%s", resetState, shellA.runOK(t, "zsh-pro status"), worktreeShellStateShape(t, runtimeRoot, partsA[2]))
	}
	shellA.runOK(t, "zsh-pro checkout feature >/dev/null")

	shellB.runOK(t, "deactivate; [[ ${+DEMO_SHARED_ENV} == 0 && ${+aliases[demo_seed]} == 0 && ${+aliases[demo.live]} == 0 && ${+ZP_ACTIVE_REVERSE_FN} == 0 && ${+ZP_WORKTREE_CAPABILITY} == 0 && $BUFFER == buffer-b ]]; deactivate")
	if output := shellB.runOK(t, "print -r -- usable-after-deactivate"); output != "usable-after-deactivate\n" {
		t.Fatalf("post-deactivate command = %q", output)
	}

	t.Run("zero-process source contract", TestWorktreeLoaderSourceZeroProcessAndLazySurface)
	t.Run("stdin-only capability and exact reply cleanup", TestWorktreeCapabilityStdinAndReplyCleanupContract)
}

func worktreeShellStateShape(t *testing.T, runtimeRoot, shellID string) string {
	t.Helper()
	stateStore, err := worktree.OpenStateStore(runtimeRoot)
	if err != nil {
		return "open-failed"
	}
	defer func() { _ = stateStore.Close() }()
	state, err := stateStore.Read(context.Background())
	if err != nil {
		return "read-failed"
	}
	shell, ok := state.Shells[shellID]
	if !ok {
		return "shell-missing"
	}
	return fmt.Sprintf("head=%d applied=%d transition=%s pending=%s/%d changes=%d conflict=%t unpublished=%d", state.HeadRevision, shell.AppliedRevision, shell.Transition, shell.Pending.Kind, shell.Pending.Revision, len(shell.Pending.Changes), shell.Conflict != nil, len(shell.UnpublishedDelta))
}

func assertWorktreeStateExcludes(t *testing.T, runtimeRoot, canary string) {
	t.Helper()
	stateStore, err := worktree.OpenStateStore(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stateStore.Close() }()
	state, err := stateStore.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := worktree.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(canary)) {
		t.Fatal("secret-classified post-attach canary entered durable state")
	}
}

func isLowerHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

func worktreeShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
