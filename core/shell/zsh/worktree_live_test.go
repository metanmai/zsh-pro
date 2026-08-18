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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
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

func (shell *retainedWorktreeShell) runWithin(t *testing.T, source string, limit time.Duration) (string, int) {
	t.Helper()
	type result struct {
		output string
		rc     int
	}
	done := make(chan result, 1)
	go func() {
		output, rc := shell.run(t, source)
		done <- result{output: output, rc: rc}
	}()
	select {
	case completed := <-done:
		return completed.output, completed.rc
	case <-time.After(limit):
		_ = shell.cmd.Process.Kill()
		_ = shell.stdin.Close()
		_ = shell.cmd.Wait()
		t.Fatalf("retained shell exceeded absolute-deadline allowance %s", limit)
		return "", -1
	}
}

func processPipeCount(t *testing.T, pid int) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "fd", entry.Name()))
		if err == nil && strings.HasPrefix(target, "pipe:[") {
			count++
		}
	}
	return count
}

func processChildren(t *testing.T, pid int) []string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "task", strconv.Itoa(pid), "children"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(payload))
}

func processAlive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

func TestWorktreePortableOwnedTransport(t *testing.T) {
	script := (Provider{}).HookScript()
	if strings.Contains(script, "/proc/") {
		t.Fatal("portable transport still has a procfs ownership gate")
	}
	for _, want := range []string{
		"writer_state", "helper_state", "closer_state",
		"writer_job", "helper_job", "closer_job",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("portable transport does not record %q", want)
		}
	}
	if t.Failed() {
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required for the portable retained-zsh contract")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Fatal("zsh is required for the portable retained-zsh contract")
	}

	testRoot, binary := buildFirstSyncBinary(t)
	fixture := newFirstSyncFixture(t, testRoot, binary, "portable-owned-transport")
	shell, callLog := fixture.shell(t, "timeout")
	attached := shell.runOK(t, firstSyncCredentialPrelude()+"\nalias zp15_first_sync='print -r -- "+firstSyncCanonicalValue+"'\nsource "+worktreeShellQuote(fixture.loader)+` || return 10
_zp_worktree_precmd || return 11
print -r -- "$ZP_WORKTREE_ATTACHED|$ZP_WORKTREE_APPLIED_REVISION|${aliases[zp15_first_sync]-unset}|${(q)ZP_WORKTREE_LAST_ERROR}"
`)
	if attached != "1|2|print -r -- "+firstSyncCanonicalValue+"|''\n" {
		calls, _ := os.ReadFile(callLog)
		t.Fatalf("portable retained shell did not attach exactly at head: %q; calls=%q stderr=%q", attached, calls, shell.stderr.String())
	}

	stateStore, err := worktree.OpenStateStore(fixture.runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	service, err := worktree.NewService(stateStore, worktree.NewRegistry(Provider{}))
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	capability, err := model.NewShellCapability(bytes.Repeat([]byte{'e'}, model.ShellCapabilityBytes))
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	credential := model.ShellCredential{ShellID: strings.Repeat("e", 64), Capability: capability}
	remote, err := service.Attach(context.Background(), model.AttachRequest{
		OperationID: strings.Repeat("f", 64), Credential: credential,
		Initial: model.LiveSnapshot{States: []model.LiveIdentityState{{
			Identity: model.Identity{Kind: model.LiveAlias, Name: "zp15_first_sync"},
			Value:    model.ScalarLiveValue("print -r -- " + firstSyncCanonicalValue),
		}}},
	})
	if err != nil || remote.ReconcileRequired || remote.Revision != 2 {
		_ = stateStore.Close()
		t.Fatalf("remote portable publisher attach = %#v, %v", remote, err)
	}
	published, err := service.Publish(context.Background(), model.PublishRequest{
		OperationID: strings.Repeat("1", 64), Credential: credential, AcknowledgedRevision: remote.Revision,
		Delta: []model.LiveChange{{
			Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveAlias, Name: "zp15_first_sync"},
			Value: model.ScalarLiveValue("print -r -- portable-next"),
		}},
	})
	if closeErr := stateStore.Close(); err == nil {
		err = closeErr
	}
	if err != nil || published.SharedRevision != 3 {
		t.Fatalf("remote portable publish = %#v, %v", published, err)
	}

	command := `
typeset -gi portable_jobs_before=${#jobstates}
zsh-pro sync
timeout_rc=$?
timeout_revision=$ZP_WORKTREE_APPLIED_REVISION
timeout_error=${#ZP_WORKTREE_LAST_ERROR}
PATH=` + worktreeShellQuote(filepath.Dir(binary)+":"+fixture.basePath) + `
rehash
zsh-pro sync
valid_rc=$?
print -r -- "result:$timeout_rc:$timeout_revision:$timeout_error:$valid_rc:$ZP_WORKTREE_APPLIED_REVISION:${#ZP_WORKTREE_LAST_ERROR}:$portable_jobs_before:${#jobstates}:${aliases[zp15_first_sync]-unset}:${(q)ZP_WORKTREE_LAST_ERROR}"
`
	started := time.Now()
	output, rc := shell.runWithin(t, command, 1500*time.Millisecond)
	if rc != 0 {
		t.Fatalf("portable timeout/retry flow returned %d after %s: %q", rc, time.Since(started), output)
	}
	fields := strings.Split(strings.TrimSpace(output), ":")
	if len(fields) != 11 || fields[0] != "result" || fields[1] == "0" || fields[2] != "2" || fields[3] == "0" ||
		fields[4] != "0" || fields[5] != "3" || fields[6] != "0" || fields[7] != fields[8] ||
		fields[9] != "print -r -- portable-next" {
		calls, _ := os.ReadFile(callLog)
		t.Fatalf("portable timeout/retry state = %q; calls=%q stderr=%q", output, calls, shell.stderr.String())
	}
	if elapsed := time.Since(started); elapsed >= 1500*time.Millisecond {
		t.Fatalf("portable timeout/retry exceeded allowance: %s", elapsed)
	}
	calls, err := os.ReadFile(callLog)
	if err != nil || !strings.Contains(string(calls), "attach\n") || !strings.Contains(string(calls), "prepare\n") {
		t.Fatalf("portable flow did not exercise attach and prepare: %q, %v", calls, err)
	}

	sentinel := filepath.Join(fixture.testRoot, "unrelated-signalled")
	ownership := shell.runOK(t, `
trap 'print -r -- signalled > `+worktreeShellQuote(sentinel)+`' TERM
sleep 5 & unrelated_pid=$!
typeset -gi writer_pid=$unrelated_pid helper_pid=0 closer_pid=0
typeset -gi writer_job=0 helper_job=0 closer_job=0
typeset writer_state='' helper_state='' closer_state=''
typeset -gi write_fd=-1 read_fd=-1 frame_rc=0 wait_rc=0
deadline=$(( EPOCHREALTIME + 0.050 ))
_zp_worktree_abort_transport "$deadline" "$(( deadline - 0.025 ))" || :
kill -0 "$unrelated_pid" 2>/dev/null || return 30
[[ ! -e `+worktreeShellQuote(sentinel)+` ]] || return 31
kill -TERM "$unrelated_pid" 2>/dev/null || :
wait "$unrelated_pid" 2>/dev/null || :
print -r -- unrelated-unsignalled
`)
	if ownership != "unrelated-unsignalled\n" {
		t.Fatalf("transport signalled an unrecorded or PID-reused process: %q", ownership)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("unrelated process observed a transport signal: %v", err)
	}
	if next := shell.runOK(t, "print -r -- next-command-sentinel"); next != "next-command-sentinel\n" {
		t.Fatalf("retained shell unusable after portable ownership probe: %q", next)
	}
}

func TestWorktreeAbsoluteDeadlineAdversarialTransport(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux /proc process and descriptor evidence")
	}
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, test := range []struct {
		name    string
		payload int
	}{
		{name: "oversized-capture", payload: 2097153},
		{name: "blocked-reader", payload: 120000},
		{name: "term-ignoring-helper", payload: 120000},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := filepath.Join(dir, "loader.zsh")
			if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
				t.Fatal(err)
			}
			shim := filepath.Join(dir, "zsh-pro")
			shimSource := `#!/bin/sh
printf '%s\n' "$$" > "$ZP_WORKTREE_TEST_DIR/helper-pid"
case "$ZP_WORKTREE_ADVERSARY" in
  oversized-capture) exit 99 ;;
  blocked-reader) while :; do :; done ;;
  term-ignoring-helper)
    cat >/dev/null
    trap '' TERM
    while :; do :; done
    ;;
  *) exit 98 ;;
esac
`
			if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(dir, "home")
			if err := os.Mkdir(home, 0o700); err != nil {
				t.Fatal(err)
			}
			env := []string{
				"HOME=" + home,
				"ZDOTDIR=" + home,
				"ZSHPRO_HOME=" + filepath.Join(dir, "runtime"),
				"PATH=" + dir + ":/usr/bin:/bin",
				"TERM=dumb",
				"LC_ALL=C",
				"ZP_WORKTREE_TEST_DIR=" + dir,
				"ZP_WORKTREE_ADVERSARY=" + test.name,
			}
			shell := startRetainedWorktreeShell(t, zshPath, dir, env)
			pidFile := filepath.Join(dir, "helper-pid")
			t.Cleanup(func() {
				payload, readErr := os.ReadFile(pidFile)
				if readErr != nil {
					return
				}
				pid, _ := strconv.Atoi(strings.TrimSpace(string(payload)))
				if processAlive(pid) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			})

			beforePipes := processPipeCount(t, shell.cmd.Process.Pid)
			setup := fmt.Sprintf(`
source %s || return 10
typeset -g ZP_WORKTREE_ATTACHED=1 ZP_WORKTREE_ATTACHED_NOW=0 ZP_WORKTREE_APPLIED_REVISION=1
typeset -g ZP_WORKTREE_SHELL_ID=${(l:64::a:)} ZP_WORKTREE_CAPABILITY=${(l:64::b:)}
seed_deadline=$(( EPOCHREALTIME + 1 ))
_zp_worktree_capture "$seed_deadline" || return 11
ZP_WORKTREE_BASELINE_FIELDS=("${_ZP_WORKTREE_CAPTURE_FIELDS[@]}")
ZP_WORKTREE_BASELINE_COUNTS=("${_ZP_WORKTREE_CAPTURE_COUNTS[@]}")
_ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
export ZP_DEADLINE_CANARY=${(l:%d::q:)}
_zp_worktree_precmd
hook_rc=$?
zsh-pro sync
explicit_rc=$?
print -r -- "result:$hook_rc:$explicit_rc:$ZP_WORKTREE_APPLIED_REVISION:${#ZP_WORKTREE_LAST_ERROR}:${#_ZP_WORKTREE_CAPTURE_FIELDS}:${#_ZP_WORKTREE_CAPTURE_COUNTS}:${+ZP_WORKTREE_REPLY_COMPLETE}:${#jobstates}"
`, worktreeShellQuote(loader), test.payload)
			started := time.Now()
			output, rc := shell.runWithin(t, setup, 1200*time.Millisecond)
			elapsed := time.Since(started)
			if rc != 0 {
				t.Fatalf("adversarial operation returned %d after %s: %q", rc, elapsed, output)
			}
			fields := strings.Split(strings.TrimSpace(output), ":")
			if len(fields) != 9 || fields[0] != "result" || fields[1] != "0" || fields[2] == "0" || fields[3] != "1" || fields[4] == "0" || fields[5] != "0" || fields[6] != "0" || fields[7] != "0" || fields[8] != "0" {
				t.Fatalf("fail-open state after %s = %q", test.name, output)
			}
			if sentinel := shell.runOK(t, "print -r -- next-command-sentinel"); sentinel != "next-command-sentinel\n" {
				t.Fatalf("next command after %s = %q", test.name, sentinel)
			}
			if children := processChildren(t, shell.cmd.Process.Pid); len(children) != 0 {
				t.Fatalf("%s retained child processes: %v", test.name, children)
			}
			afterPipes := processPipeCount(t, shell.cmd.Process.Pid)
			if afterPipes != beforePipes {
				t.Fatalf("%s pipe descriptors = %d, want baseline %d", test.name, afterPipes, beforePipes)
			}
			if payload, readErr := os.ReadFile(pidFile); readErr == nil {
				pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
				if parseErr != nil || processAlive(pid) {
					t.Fatalf("%s helper was not reaped: pid=%q", test.name, payload)
				}
			} else if test.name != "oversized-capture" {
				t.Fatalf("%s helper did not start: %v", test.name, readErr)
			}
			if reportPath := os.Getenv("ZP_WORKTREE_ADVERSARIAL_REPORT"); reportPath != "" {
				report, openErr := os.OpenFile(reportPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if openErr != nil {
					t.Fatal(openErr)
				}
				_, writeErr := fmt.Fprintf(report, "mode=%s elapsed_ms=%d return_class=fail-open survivors=0 pipe_delta=%d applied_revision_unchanged=1 behind_error=1 continuation=1\n", test.name, elapsed.Milliseconds(), afterPipes-beforePipes)
				closeErr := report.Close()
				if writeErr != nil {
					t.Fatal(writeErr)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		})
	}

	script := (Provider{}).HookScript()
	for _, want := range []string{"_zp_worktree_abort_transport() {", "writer_pid", "kill -KILL", "termination_deadline"} {
		if !strings.Contains(script, want) {
			t.Errorf("bounded transport missing %q", want)
		}
	}
}

func TestWorktreePartialPatchFailureRecovery(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}

	aliasState := func(name, body string) model.LiveIdentityState {
		return model.LiveIdentityState{
			Identity: model.Identity{Kind: model.LiveAlias, Name: name},
			Value:    model.ScalarLiveValue(body),
		}
	}
	emitTransition := func(before, after []model.LiveIdentityState) string {
		t.Helper()
		patch, err := activate.BuildLivePatch(before, after)
		if err != nil {
			t.Fatal(err)
		}
		source, err := (Provider{}).EmitRuntimeTransition(
			patch.Forward, patch.ReplacementReverse,
			"__zp14_apply_template", "__zp14_reverse_template",
			6, 7, strings.Repeat("0", 64),
		)
		if err != nil {
			t.Fatal(err)
		}
		return string(source)
	}
	emitOperations := func(forward, reverse []activate.Op) string {
		t.Helper()
		source, err := (Provider{}).EmitRuntimeTransition(
			forward, reverse,
			"__zp14_apply_template", "__zp14_reverse_template",
			6, 7, strings.Repeat("0", 64),
		)
		if err != nil {
			t.Fatal(err)
		}
		return string(source)
	}
	runScript := func(test *testing.T, name, transition, assertions string) {
		test.Helper()
		common := `
source "$1" || exit 10
typeset -g ZP14_TRANSITION_SOURCE=` + worktreeShellQuote(transition) + `
typeset -g ZP_WORKTREE_ATTACHED=1 ZP_WORKTREE_ATTACHED_NOW=0 ZP_WORKTREE_APPLIED_REVISION=5
typeset -g ZP_WORKTREE_LAST_ERROR='' ZP_WORKTREE_OPERATION_SEQUENCE=0
typeset -ga ZP_WORKTREE_BASELINE_FIELDS=(ZP_LIVE_SNAPSHOT 1 E)
typeset -ga ZP_WORKTREE_BASELINE_COUNTS=(2 1)
typeset -gi ZP14_CAPTURE_COUNT=0
_zp_worktree_ensure_attached() { typeset -g ZP_WORKTREE_ATTACHED_NOW=0; return 0 }
_zp_worktree_capture() {
  (( ++ZP14_CAPTURE_COUNT ))
  if [[ "$ZP14_FAILURE_MODE" == capture && $ZP14_CAPTURE_COUNT -ge 2 ]]; then return 73; fi
  _ZP_WORKTREE_CAPTURE_FIELDS=(ZP_LIVE_SNAPSHOT 1 E)
  _ZP_WORKTREE_CAPTURE_COUNTS=(2 1)
  return 0
}
_zp_worktree_budget_check() {
  if [[ "$ZP14_FAILURE_MODE" == deadline && "${aliases[zp14_post]-}" == 'print -r -- after-post-secret' ]]; then return 1; fi
  return 0
}
_zp_worktree_invoke() {
  local operation="$1" result_name="$2" apply_name="$9" reverse_name="${10}" response=''
  case "$operation" in
    prepare)
      response="${ZP14_TRANSITION_SOURCE//__zp14_apply_template/$apply_name}"
      response="${response//__zp14_reverse_template/$reverse_name}"
      : ${(P)result_name::=$response}
      return 0
      ;;
    acknowledge)
      case "$ZP14_FAILURE_MODE" in
        acknowledgement) return 74 ;;
        malformed-reply) : ${(P)result_name::=malformed}; return 0 ;;
        *) : ${(P)result_name::='ZPWK 1 6 1'}; return 0 ;;
      esac
      ;;
    *) return 75 ;;
  esac
}
` + assertions
		cmd := exec.Command(zshPath, "-f", "-c", common, "zsh-pro-partial-"+name, loader)
		output, runErr := cmd.CombinedOutput()
		if bytes.Contains(output, []byte("secret-value")) || bytes.Contains(output, []byte("after-post-secret")) {
			test.Fatalf("%s leaked a live value in diagnostics: %q", name, output)
		}
		if runErr != nil {
			test.Fatalf("%s partial-patch recovery: %v\n%s", name, runErr, output)
		}
		if !bytes.Contains(output, []byte("next-command-usable\n")) {
			test.Fatalf("%s did not leave the next command usable: %q", name, output)
		}
	}

	pathIdentity := model.Identity{Kind: model.LivePath, Name: "PATH"}
	fpathIdentity := model.Identity{Kind: model.LiveFPath, Name: "FPATH"}
	forwardOnly := emitOperations(
		[]activate.Op{
			activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_first"}, Value: "print -r -- first-secret-value"},
			activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveFunction, Name: "zp14_hostile_fn"}, Value: "print -r -- 'hostile; $() *'\nprint -r -- second"},
			activate.TransitionLiveList{Identity: pathIdentity, BeforePresent: true, AfterPresent: true, Before: []string{"/base", "", "/dup", "/dup"}, After: []string{"/new", "", "/dup", "/dup"}},
			activate.TransitionLiveList{Identity: fpathIdentity, BeforePresent: true, AfterPresent: true, Before: []string{"/functions", "", "/functions"}, After: []string{"", "/new-functions", ""}},
			activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_final"}, Value: "print -r -- final-secret-value"},
		},
		[]activate.Op{
			activate.RemoveLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_first"}},
			activate.RemoveLiveScalar{Identity: model.Identity{Kind: model.LiveFunction, Name: "zp14_hostile_fn"}},
			activate.TransitionLiveList{Identity: pathIdentity, BeforePresent: true, AfterPresent: true, Before: []string{"/new", "", "/dup", "/dup"}, After: []string{"/base", "", "/dup", "/dup"}},
			activate.TransitionLiveList{Identity: fpathIdentity, BeforePresent: true, AfterPresent: true, Before: []string{"", "/new-functions", ""}, After: []string{"/functions", "", "/functions"}},
			activate.RemoveLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_final"}},
		},
	)
	for _, failure := range []struct {
		name   string
		target string
	}{
		{name: "early-operation", target: "zp14_first"},
		{name: "final-operation", target: "zp14_final"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			body := `
typeset -g ZP14_FAILURE_MODE=operation ZP14_FAIL_ALIAS=` + failure.target + `
alias() { if [[ "$1" == "$ZP14_FAIL_ALIAS"=* ]]; then return 61; fi; builtin alias "$@"; }
path=('/base' '' '/dup' '/dup')
fpath=('/functions' '' '/functions')
typeset ZP14_PATH_BEFORE="${(qqqq)path}" ZP14_FPATH_BEFORE="${(qqqq)fpath}"
__zp_worktree_reverse_old_14() { : }
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_worktree_reverse_old_14
_zp_worktree_pull_impl 999
pull_rc=$?
(( pull_rc != 0 && ZP_WORKTREE_APPLIED_REVISION == 5 && ${#ZP_WORKTREE_LAST_ERROR} > 0 )) || exit 20
for managed in zp14_first zp14_final; do (( ${+aliases[$managed]} == 0 )) || exit 21; done
(( ${+functions[zp14_hostile_fn]} == 0 )) || exit 21
[[ "${(qqqq)path}" == "$ZP14_PATH_BEFORE" && "${(qqqq)fpath}" == "$ZP14_FPATH_BEFORE" ]] || exit 21
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_worktree_reverse_old_14 && ${+functions[__zp_worktree_reverse_old_14]} == 1 && ${+ZP_RECOVERY_REVERSE_FN} == 0 ]] || exit 22
for generated in ${(k)functions}; do
  [[ "$generated" != __zp_worktree_apply_* ]] || exit 23
  [[ "$generated" != __zp_worktree_reverse_* || "$generated" == __zp_worktree_reverse_old_14 ]] || exit 24
done
(( ${+ZP_WORKTREE_REPLY_COMPLETE} == 0 )) || exit 25
print -r -- next-command-usable
`
			runScript(t, failure.name, forwardOnly, body)
		})
	}

	for _, failure := range []struct {
		name    string
		forward []activate.Op
		reverse []activate.Op
	}{
		{
			name: "readonly-early-operation",
			forward: []activate.Op{
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP14_READONLY_TARGET"}, Value: "readonly-secret-value"},
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_readonly_alias"}, Value: "print -r -- readonly-secret-value"},
			},
			reverse: []activate.Op{
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP14_READONLY_TARGET"}, Value: "before-readonly"},
				activate.RemoveLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_readonly_alias"}},
			},
		},
		{
			name: "readonly-final-operation",
			forward: []activate.Op{
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_readonly_alias"}, Value: "print -r -- readonly-secret-value"},
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP14_READONLY_TARGET"}, Value: "readonly-secret-value"},
			},
			reverse: []activate.Op{
				activate.RemoveLiveScalar{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp14_readonly_alias"}},
				activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP14_READONLY_TARGET"}, Value: "before-readonly"},
			},
		},
	} {
		t.Run(failure.name, func(t *testing.T) {
			transition := emitOperations(failure.forward, failure.reverse)
			body := `
PROMPT='' RPROMPT='' PS2=''
source ` + worktreeShellQuote(loader) + ` || exit 50
typeset -g ZP14_TRANSITION_SOURCE=` + worktreeShellQuote(transition) + `
typeset -g ZP_WORKTREE_ATTACHED=1 ZP_WORKTREE_ATTACHED_NOW=0 ZP_WORKTREE_APPLIED_REVISION=5 ZP_WORKTREE_LAST_ERROR=''
typeset -g ZP_WORKTREE_OPERATION_SEQUENCE=0
typeset -ga ZP_WORKTREE_BASELINE_FIELDS=(ZP_LIVE_SNAPSHOT 1 E) ZP_WORKTREE_BASELINE_COUNTS=(2 1)
_zp_worktree_ensure_attached() { typeset -g ZP_WORKTREE_ATTACHED_NOW=0; return 0 }
_zp_worktree_capture() { _ZP_WORKTREE_CAPTURE_FIELDS=(ZP_LIVE_SNAPSHOT 1 E); _ZP_WORKTREE_CAPTURE_COUNTS=(2 1); return 0 }
_zp_worktree_budget_check() { return 0 }
_zp_worktree_invoke() {
  local operation="$1" result_name="$2" apply_name="$9" reverse_name="${10}" response=''
  [[ "$operation" == prepare ]] || return 75
  response="${ZP14_TRANSITION_SOURCE//__zp14_apply_template/$apply_name}"
  response="${response//__zp14_reverse_template/$reverse_name}"
  : ${(P)result_name::=$response}
}
export ZP14_READONLY_TARGET=before-readonly
__zp_worktree_reverse_old_readonly() { : }
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_worktree_reverse_old_readonly
zp14_trigger_readonly() { local -r ZP14_READONLY_TARGET=before-readonly; _zp_worktree_pull_impl 999 }
zp14_trigger_readonly
[[ "$ZP_WORKTREE_APPLIED_REVISION" == 5 && -n "$ZP_WORKTREE_LAST_ERROR" ]] || exit 51
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_worktree_reverse_old_readonly && -n "$ZP_RECOVERY_REVERSE_FN" && ${+functions[$ZP_RECOVERY_REVERSE_FN]} == 1 ]] || exit 52
deactivate || exit 53
[[ "$ZP14_READONLY_TARGET" == before-readonly && ${+aliases[zp14_readonly_alias]} == 0 ]] || exit 54
(( ${+ZP_RECOVERY_REVERSE_FN} == 0 && ${+ZP_ACTIVE_REVERSE_FN} == 0 && ${+functions[__zp_worktree_reverse_old_readonly]} == 0 )) || exit 55
deactivate || exit 56
print -r -- next-command-usable
exit
`
			cmd := exec.Command(zshPath, "-f", "-i")
			cmd.Stdin = strings.NewReader(body)
			output, runErr := cmd.CombinedOutput()
			if bytes.Contains(output, []byte("readonly-secret-value")) {
				t.Fatalf("readonly failure leaked a live value: %q", output)
			}
			if runErr != nil || !bytes.Contains(output, []byte("next-command-usable")) {
				t.Fatalf("readonly recovery = (%v, %q)", runErr, output)
			}
		})
	}

	t.Run("failed-compensation-public-retry", func(t *testing.T) {
		transition := emitTransition(
			[]model.LiveIdentityState{aliasState("zp14_changed", "print -r -- before-secret-value")},
			[]model.LiveIdentityState{
				aliasState("zp14_changed", "print -r -- after-secret-value"),
				aliasState("zp14_stop", "print -r -- stop-secret-value"),
			},
		)
		body := `
typeset -g ZP14_FAILURE_MODE=operation ZP14_BLOCK_REVERSE=1
alias zp14_changed='print -r -- before-secret-value'
alias() {
  [[ "$1" == zp14_stop=* ]] && return 61
  [[ "$ZP14_BLOCK_REVERSE" == 1 && "$1" == zp14_changed=*before-secret-value* ]] && return 62
  builtin alias "$@"
}
__zp_worktree_reverse_old_14() { : }
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_worktree_reverse_old_14
_zp_worktree_pull_impl 999
pull_rc=$?
(( pull_rc != 0 && ZP_WORKTREE_APPLIED_REVISION == 5 && ${#ZP_WORKTREE_LAST_ERROR} > 0 )) || exit 30
[[ "${aliases[zp14_changed]}" == 'print -r -- after-secret-value' && ${+aliases[zp14_stop]} == 0 ]] || exit 31
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_worktree_reverse_old_14 && -n "$ZP_RECOVERY_REVERSE_FN" && ${+functions[$ZP_RECOVERY_REVERSE_FN]} == 1 ]] || exit 32
typeset -g ZP14_BLOCK_REVERSE=0
deactivate || exit 33
[[ "${aliases[zp14_changed]}" == 'print -r -- before-secret-value' && ${+aliases[zp14_stop]} == 0 ]] || exit 34
(( ${+ZP_RECOVERY_REVERSE_FN} == 0 && ${+ZP_ACTIVE_REVERSE_FN} == 0 && ${+functions[__zp_worktree_reverse_old_14]} == 0 )) || exit 35
for generated in ${(k)functions}; do
  [[ "$generated" != __zp_worktree_apply_* && "$generated" != __zp_worktree_reverse_* ]] || exit 36
done
for residue in ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN ZP_WORKTREE_REPLY_FINGERPRINT ZP_WORKTREE_REPLY_COMPLETE; do (( ${+parameters[$residue]} == 0 )) || exit 37; done
deactivate || exit 38
print -r -- next-command-usable
`
		runScript(t, "failed-compensation-public-retry", transition, body)
	})

	postTransition := emitTransition(
		[]model.LiveIdentityState{aliasState("zp14_post", "print -r -- before-post")},
		[]model.LiveIdentityState{aliasState("zp14_post", "print -r -- after-post-secret")},
	)
	for _, mode := range []string{"deadline", "capture", "malformed-reply", "acknowledgement"} {
		t.Run("post-apply-"+mode, func(t *testing.T) {
			body := `
typeset -g ZP14_FAILURE_MODE=` + mode + `
alias zp14_post='print -r -- before-post'
__zp_worktree_reverse_old_14() { : }
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_worktree_reverse_old_14
_zp_worktree_pull_impl 999
pull_rc=$?
(( pull_rc != 0 && ZP_WORKTREE_APPLIED_REVISION == 5 && ${#ZP_WORKTREE_LAST_ERROR} > 0 )) || exit 40
[[ "${aliases[zp14_post]}" == 'print -r -- after-post-secret' ]] || exit 41
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_worktree_reverse_* && "$ZP_ACTIVE_REVERSE_FN" != __zp_worktree_reverse_old_14 && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 42
(( ${+functions[__zp_worktree_reverse_old_14]} == 0 && ${+ZP_RECOVERY_REVERSE_FN} == 0 )) || exit 43
for reply in ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN ZP_WORKTREE_REPLY_FINGERPRINT ZP_WORKTREE_REPLY_COMPLETE; do (( ${+parameters[$reply]} == 0 )) || exit 44; done
print -r -- pre-deactivate-usable
deactivate || exit 45
[[ "${aliases[zp14_post]}" == 'print -r -- before-post' ]] || exit 46
(( ${+ZP_ACTIVE_REVERSE_FN} == 0 && ${+ZP_RECOVERY_REVERSE_FN} == 0 )) || exit 47
for generated in ${(k)functions}; do
  [[ "$generated" != __zp_worktree_apply_* && "$generated" != __zp_worktree_reverse_* ]] || exit 48
done
deactivate || exit 49
print -r -- next-command-usable
`
			runScript(t, "post-apply-"+mode, postTransition, body)
		})
	}
}

func TestWorktreeAdversarialDriverContract(t *testing.T) {
	scriptPath := filepath.Join(liveTestRepositoryRoot(t), "scripts", "perf-worktree.sh")
	payload, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, mode := range []string{"oversized-capture", "blocked-reader", "term-ignoring-helper"} {
		if !strings.Contains(script, "mode="+mode) {
			t.Errorf("adversarial driver missing %s mode", mode)
		}
	}
	for _, field := range []string{
		"elapsed_ms=", "return_class=", "survivors=", "pipe_delta=",
		"applied_revision_unchanged=", "behind_error=", "continuation=",
	} {
		if !strings.Contains(script, field) {
			t.Errorf("adversarial driver missing value-free %s field", field)
		}
	}
	for _, contract := range []string{
		"TestWorktreeAbsoluteDeadlineAdversarialTransport",
		"ZP_WORKTREE_ADVERSARIAL_REPORT",
		"-count=2",
	} {
		if !strings.Contains(script, contract) {
			t.Errorf("adversarial driver missing repeated cleanup contract %q", contract)
		}
	}
}

const firstSyncCanonicalValue = "value-canary"

func firstSyncCredentialPrelude() string {
	return "typeset -g ZP_WORKTREE_SHELL_ID=" + strings.Repeat("d", 64) +
		" ZP_WORKTREE_CAPABILITY=" + strings.Repeat("64", model.ShellCapabilityBytes)
}

type firstSyncFixture struct {
	t           *testing.T
	testRoot    string
	binDir      string
	binary      string
	home        string
	runtimeRoot string
	loader      string
	zshPath     string
	basePath    string
}

func buildFirstSyncBinary(t *testing.T) (string, string) {
	t.Helper()
	testRoot := t.TempDir()
	binDir := filepath.Join(testRoot, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, "zsh-pro")
	build := exec.Command("go", "build", "-o", binary, "./core/cmd/zsh-pro")
	build.Dir = liveTestRepositoryRoot(t)
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production binary: %v\n%s", err, output)
	}
	return testRoot, binary
}

func newFirstSyncFixture(t *testing.T, testRoot, binary, name string) *firstSyncFixture {
	t.Helper()
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	root := filepath.Join(testRoot, name)
	binDir := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	runtimeRoot := filepath.Join(root, "runtime")
	for _, dir := range []string{binDir, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(home, "source.zsh")
	if err := os.WriteFile(source, []byte("alias zp15_first_sync='print -r -- initial'\n"), 0o600); err != nil {
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

	stateStore, err := worktree.OpenStateStore(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	service, err := worktree.NewService(stateStore, worktree.NewRegistry(Provider{}))
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	state, err := stateStore.Read(context.Background())
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	capability, err := model.NewShellCapability(bytes.Repeat([]byte{'a'}, model.ShellCapabilityBytes))
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	credential := model.ShellCredential{ShellID: strings.Repeat("a", 64), Capability: capability}
	attached, err := service.Attach(context.Background(), model.AttachRequest{
		OperationID: strings.Repeat("b", 64), Credential: credential,
		Initial: model.LiveSnapshot{States: model.CloneLiveStates(state.Shared)},
	})
	if err != nil || !attached.Attached || attached.ReconcileRequired || attached.Revision != 1 {
		_ = stateStore.Close()
		t.Fatalf("seed shell attachment = (%+v, %v)", attached, err)
	}
	published, err := service.Publish(context.Background(), model.PublishRequest{
		OperationID: strings.Repeat("c", 64), Credential: credential, AcknowledgedRevision: attached.Revision,
		Delta: []model.LiveChange{{
			Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveAlias, Name: "zp15_first_sync"},
			Value: model.ScalarLiveValue("print -r -- " + firstSyncCanonicalValue),
		}},
	})
	if err != nil || published.SharedRevision != 2 {
		_ = stateStore.Close()
		t.Fatalf("seed canonical mutation = (%+v, %v)", published, err)
	}
	if err := stateStore.Close(); err != nil {
		t.Fatal(err)
	}

	shim := filepath.Join(binDir, "zsh-pro")
	shimSource := `#!/bin/sh
operation="${3-}"
printf '%s\n' "$operation" >> "$ZP15_CALL_LOG"
case "$ZP15_FAILURE:$operation" in
  prepare:prepare) cat >/dev/null; exit 71 ;;
  acknowledge:acknowledge) cat >/dev/null; exit 72 ;;
  timeout:prepare) cat >/dev/null; sleep 1; exit 73 ;;
esac
exec "$ZP15_REAL_BINARY" "$@"
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	return &firstSyncFixture{
		t: t, testRoot: root, binDir: binDir, binary: binary, home: home, runtimeRoot: runtimeRoot,
		loader: loader, zshPath: zshPath, basePath: basePath,
	}
}

func (fixture *firstSyncFixture) shell(t *testing.T, failure string) (*retainedWorktreeShell, string) {
	t.Helper()
	dir := filepath.Join(fixture.testRoot, "shell")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	callLog := filepath.Join(fixture.testRoot, "calls")
	commandPath := fixture.binDir
	if failure == "" {
		commandPath = filepath.Dir(fixture.binary)
	}
	env := []string{
		"HOME=" + fixture.home,
		"ZDOTDIR=" + fixture.home,
		"ZSHPRO_HOME=" + fixture.runtimeRoot,
		"PATH=" + commandPath + ":" + fixture.basePath,
		"TERM=dumb", "LC_ALL=C",
		"ZP15_CALL_LOG=" + callLog,
		"ZP15_FAILURE=" + failure,
		"ZP15_REAL_BINARY=" + fixture.binary,
	}
	return startRetainedWorktreeShell(t, fixture.zshPath, dir, env), callLog
}

func firstSyncHead(t *testing.T, runtimeRoot string) uint64 {
	t.Helper()
	store, err := worktree.OpenStateStore(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	state, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state.HeadRevision
}

func TestWorktreeFirstExplicitSyncReconciles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	testRoot, binary := buildFirstSyncBinary(t)
	for _, test := range []struct {
		name        string
		prelude     string
		wantInitial string
	}{
		{name: "mismatch-auto-apply-default", wantInitial: "unset"},
		{name: "mismatch-auto-apply-disabled", prelude: "export ZSHPRO_AUTO_APPLY=false", wantInitial: "unset"},
		{name: "exact-attach", prelude: "alias zp15_first_sync='print -r -- " + firstSyncCanonicalValue + "'", wantInitial: "print -r -- " + firstSyncCanonicalValue},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFirstSyncFixture(t, testRoot, binary, test.name)
			shell, _ := fixture.shell(t, "")
			command := firstSyncCredentialPrelude() + "\n" + test.prelude + "\nsource " + worktreeShellQuote(fixture.loader) + ` || return 10
print -r -- "initial:${aliases[zp15_first_sync]-unset}"
zsh-pro sync
sync_rc=$?
print -r -- "result:$sync_rc:$ZP_WORKTREE_APPLIED_REVISION:$ZP_WORKTREE_RECONCILE_REQUIRED:${#ZP_WORKTREE_LAST_ERROR}:${aliases[zp15_first_sync]-unset}"
zp15_first_sync
zsh-pro status
`
			output := shell.runOK(t, command)
			if !strings.Contains(output, "initial:"+test.wantInitial+"\n") ||
				!strings.Contains(output, "result:0:2:0:0:print -r -- "+firstSyncCanonicalValue+"\n") ||
				!strings.Contains(output, firstSyncCanonicalValue+"\n") ||
				!strings.Contains(output, "behind: false\n") {
				t.Fatalf("first explicit sync did not converge: %q", output)
			}
			if output := shell.runOK(t, `zsh-pro sync
print -r -- "$ZP_WORKTREE_APPLIED_REVISION|$ZP_WORKTREE_RECONCILE_REQUIRED|${#ZP_WORKTREE_LAST_ERROR}|${aliases[zp15_first_sync]-unset}"
zp15_first_sync
`); output != "2|0|0|print -r -- "+firstSyncCanonicalValue+"\n"+firstSyncCanonicalValue+"\n" {
				t.Fatalf("repeated explicit sync = %q", output)
			}
			if head := firstSyncHead(t, fixture.runtimeRoot); head != 2 {
				t.Fatalf("repeated explicit sync advanced canonical head to %d", head)
			}
		})
	}
}

func TestWorktreeFirstExplicitSyncFailureStaysBehind(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	testRoot, binary := buildFirstSyncBinary(t)
	for _, test := range []struct {
		name     string
		failure  string
		override string
		mutates  bool
	}{
		{name: "prepare", failure: "prepare"},
		{name: "timeout", failure: "timeout"},
		{name: "apply", override: `alias() { [[ "$1" == zp15_first_sync=* ]] && return 61; builtin alias "$@" }
`},
		{name: "capture", override: `functions[_zp15_capture_original]=${functions[_zp_worktree_capture]}
typeset -gi ZP15_CAPTURE_COUNT=0
_zp_worktree_capture() { (( ++ZP15_CAPTURE_COUNT )); (( ZP15_CAPTURE_COUNT < 3 )) || return 73; _zp15_capture_original "$@" }
`, mutates: true},
		{name: "acknowledge", failure: "acknowledge", mutates: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFirstSyncFixture(t, testRoot, binary, test.name)
			shell, _ := fixture.shell(t, test.failure)
			command := firstSyncCredentialPrelude() + "\nsource " + worktreeShellQuote(fixture.loader) + " || return 10\n" + test.override + `
zsh-pro sync
sync_rc=$?
print -r -- "result:$sync_rc:$ZP_WORKTREE_APPLIED_REVISION:$ZP_WORKTREE_RECONCILE_REQUIRED:${#ZP_WORKTREE_LAST_ERROR}:${+ZP_ACTIVE_REVERSE_FN}:${+ZP_RECOVERY_REVERSE_FN}"
zsh-pro status
`
			started := time.Now()
			output := shell.runOK(t, command)
			elapsed := time.Since(started)
			if test.name == "timeout" && elapsed > 1200*time.Millisecond {
				t.Fatalf("first-sync timeout exceeded absolute allowance: %s", elapsed)
			}
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) == 0 || !strings.HasPrefix(lines[0], "result:") {
				t.Fatalf("first-sync failure result missing: %q", output)
			}
			fields := strings.Split(lines[0], ":")
			if len(fields) != 7 || fields[1] == "0" || fields[2] != "0" || fields[3] != "1" || fields[4] == "0" {
				t.Fatalf("first-sync failure published false convergence: %q", output)
			}
			if test.mutates && fields[5] == "0" && fields[6] == "0" {
				t.Fatalf("mutating failure retained no reverse owner: %q", output)
			}
			if !strings.Contains(output, "behind: true\n") {
				t.Fatalf("first-sync failure status is not behind: %q", output)
			}
			if strings.Contains(output, firstSyncCanonicalValue) {
				t.Fatalf("first-sync failure leaked a live value: output=%q", output)
			}
			if next := shell.runOK(t, "print -r -- next-command-sentinel"); next != "next-command-sentinel\n" {
				t.Fatalf("retained shell unusable after %s: %q", test.name, next)
			}
			shell.close()
			if strings.Contains(shell.stderr.String(), firstSyncCanonicalValue) {
				t.Fatalf("first-sync failure leaked a live value to stderr: %q", shell.stderr.String())
			}
		})
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
