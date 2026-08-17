package zsh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorktreeLazyAttachPublishResolveAndCapabilityVisibility(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "zsh-pro")
	const capability = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const shellID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shimSource := `#!/bin/sh
op="$3"
count_file="$ZP_WORKTREE_TEST_DIR/count-$op"
count=0
if test -f "$count_file"; then read count < "$count_file"; fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
cat > "$ZP_WORKTREE_TEST_DIR/frame-$op-$count"
printf '%s|%s|%s\n' "$*" "${ZP_WORKTREE_CAPABILITY-}" "${ZSHPRO_SHELL_CAPABILITY-}" >> "$ZP_WORKTREE_TEST_DIR/visibility"
case "$op:$count" in
  attach:1) printf '%s\n%s\n%s\n' 'ZPWC 1' '` + shellID + `' '` + capability + `' ;;
  attach:2) printf '%s\n' 'ZPWA 1 1 1' ;;
  publish:1) printf '%s\n%s\n' 'ZPWP 1 2 1' 'overlap env ZP_CONFLICT durable-token' ;;
  publish:2) printf '%s\n' 'ZPWP 1 3 0' ;;
  resolve:1)
    printf '%s\n' \
      "typeset -g ZP_WORKTREE_REPLY_PROTOCOL='1'" \
      "typeset -g ZP_WORKTREE_REPLY_REVISION='3'" \
      "typeset -g ZP_WORKTREE_REPLY_TOKEN='9'" \
      "typeset -g ZP_WORKTREE_REPLY_FINGERPRINT='0000000000000000000000000000000000000000000000000000000000000000'" \
      "typeset -g ZP_WORKTREE_REPLY_COMPLETE='1'"
    ;;
  acknowledge:1) printf '%s\n' 'ZPWK 1 3 1' ;;
  prepare:*) : ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `
source "$1" || exit 10
[[ ! -e "$ZP_WORKTREE_TEST_DIR/count-attach" ]] || exit 11
_zp_worktree_precmd
[[ "$ZP_WORKTREE_ATTACHED" == 1 && "$ZP_WORKTREE_ATTACHED_NOW" == 1 ]] || { print -r -- "attach:$ZP_WORKTREE_ATTACHED:$ZP_WORKTREE_ATTACHED_NOW:$ZP_WORKTREE_LAST_ERROR:$ZP_WORKTREE_UNSUPPORTED"; exit 12; }
[[ ! -e "$ZP_WORKTREE_TEST_DIR/count-publish" ]] || exit 13
exec 9>&2 2>"$ZP_WORKTREE_TEST_DIR/xtrace"
setopt xtrace
_zp_worktree_precmd
unsetopt xtrace
[[ "$ZP_WORKTREE_CONFLICT_COUNT" == 1 && "$ZP_WORKTREE_CONFLICT_TOKEN" == durable-token ]] || { print -r -- "publish:$ZP_WORKTREE_CONFLICT_COUNT:${ZP_WORKTREE_CONFLICT_TOKEN-}:$ZP_WORKTREE_LAST_ERROR"; exit 14; }
setopt xtrace
zsh-pro sync --resolve shared || exit 15
unsetopt xtrace
exec 2>&9 9>&-
[[ "$ZP_WORKTREE_CONFLICT_COUNT" == 0 && "$ZP_WORKTREE_APPLIED_REVISION" == 3 ]] || exit 16
_zp_worktree_publish || exit 17
print -r -- survived
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-worktree-live", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_WORKTREE_TEST_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real-zsh worktree lifecycle: %v\n%s", err, out)
	}
	if string(out) != "survived\n" {
		t.Fatalf("worktree lifecycle output = %q", out)
	}
	visibility, err := os.ReadFile(filepath.Join(dir, "visibility"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(visibility, []byte(capability)) || bytes.Contains(visibility, []byte(shellID+" "+capability)) {
		t.Fatalf("credential appeared in helper argv/environment: %q", visibility)
	}
	trace, err := os.ReadFile(filepath.Join(dir, "xtrace"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(trace, []byte(capability)) || bytes.Contains(trace, []byte("durable-token")) || bytes.Contains(trace, []byte("ZP_WORKTREE_REPLY_FINGERPRINT='")) {
		t.Fatalf("xtrace disclosed private worktree transport: %q", trace)
	}
	for _, target := range []string{"frame-attach-2", "frame-publish-1", "frame-resolve-1", "frame-acknowledge-1"} {
		frame, err := os.ReadFile(filepath.Join(dir, target))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateWorktreeHookFrame(frame); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if !bytes.Contains(frame, []byte(capability)) {
			t.Fatalf("%s omitted stdin capability", target)
		}
	}
	resolveFrame, err := os.ReadFile(filepath.Join(dir, "frame-resolve-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(resolveFrame, []byte("durable-token")) || !bytes.Contains(resolveFrame, []byte("ZP_CONFLICT")) {
		t.Fatal("explicit resolve did not use durable value-free conflict metadata")
	}
}

func TestWorktreeCapabilityProcessVisibilityWhileHelperBlocked(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires /proc process inspection")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "zsh-pro")
	const capability = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	const shellID = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	shimSource := `#!/bin/sh
count_file="$ZP_WORKTREE_TEST_DIR/attach-count"
count=0
if test -f "$count_file"; then read count < "$count_file"; fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
cat > "$ZP_WORKTREE_TEST_DIR/blocked-frame-$count"
if test "$count" -eq 1; then
  printf '%s\n%s\n%s\n' 'ZPWC 1' '` + shellID + `' '` + capability + `'
  exit 0
fi
printf '%s\n' "$$" > "$ZP_WORKTREE_TEST_DIR/helper-pid"
: > "$ZP_WORKTREE_TEST_DIR/ready"
while test ! -e "$ZP_WORKTREE_TEST_DIR/release"; do sleep 0.01; done
printf '%s\n' 'ZPWA 1 1 1'
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("zsh", "-f", "-c", `source "$1"; _zp_worktree_ensure_attached`, "zsh-pro-proc", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_WORKTREE_TEST_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	finished := false
	t.Cleanup(func() {
		if finished {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "release"), nil, 0o600)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("blocked helper did not accept stdin")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pidBytes, err := os.ReadFile(filepath.Join(dir, "helper-pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(pidBytes))
	cmdline, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
	if err != nil {
		t.Fatal(err)
	}
	environ, err := os.ReadFile(filepath.Join("/proc", pid, "environ"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cmdline, []byte(capability)) || bytes.Contains(environ, []byte(capability)) {
		t.Fatalf("blocked helper disclosed capability in process metadata: cmdline=%q", cmdline)
	}
	frame, err := os.ReadFile(filepath.Join(dir, "blocked-frame-2"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(frame, []byte(capability)) {
		t.Fatal("blocked helper did not receive the capability solely over stdin")
	}
	if err := os.WriteFile(filepath.Join(dir, "release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	finished = true
}

func TestWorktreeTransitionFakeClockCumulative249And251(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	const body = `
source "$1" || exit 10
deadline=0.250
for now in 0.025 0.065 0.119 0.189 0.249; do
  _zp_worktree_budget_check "$deadline" "$now" || exit 11
done
_zp_worktree_budget_check "$deadline" 0.251 && exit 12
print -r -- survived
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-fake-clock", loader)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "survived\n" {
		t.Fatalf("fake cumulative deadline = (%v, %q)", err, out)
	}
}

func TestWorktreeTransitionRealTime25And500MillisecondMargins(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	for _, test := range []struct {
		name     string
		stall    string
		attached string
		max      time.Duration
	}{
		{name: "well-below", stall: "0.025", attached: "1", max: 400 * time.Millisecond},
		{name: "clearly-over", stall: "0.500", attached: "0", max: 450 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := filepath.Join(dir, "loader.zsh")
			if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
				t.Fatal(err)
			}
			shim := filepath.Join(dir, "zsh-pro")
			shimSource := `#!/bin/sh
count_file="$ZP_WORKTREE_TEST_DIR/count"
count=0
if test -f "$count_file"; then read count < "$count_file"; fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
cat >/dev/null
sleep "$ZP_WORKTREE_TEST_STALL"
if test "$count" -eq 1; then
  printf '%s\n%s\n%s\n' 'ZPWC 1' 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee' 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff'
else
  printf '%s\n' 'ZPWA 1 1 1'
fi
`
			if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
				t.Fatal(err)
			}
			body := `source "$1"; _zp_worktree_ensure_attached || :; print -r -- "survived:$ZP_WORKTREE_ATTACHED"`
			cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-real-clock", loader)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_WORKTREE_TEST_DIR="+dir, "ZP_WORKTREE_TEST_STALL="+test.stall)
			started := time.Now()
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(started)
			if err != nil || string(out) != "survived:"+test.attached+"\n" {
				t.Fatalf("real deadline = (%v, %q) after %s", err, out, elapsed)
			}
			if elapsed >= test.max {
				t.Fatalf("real deadline margin exceeded: %s >= %s", elapsed, test.max)
			}
		})
	}
}

func validateWorktreeHookFrame(frame []byte) error {
	lineEnd := bytes.IndexByte(frame, '\n')
	if lineEnd < 0 {
		return fmt.Errorf("missing header")
	}
	var version, count int
	if _, err := fmt.Sscanf(string(frame[:lineEnd]), "ZPWT %d %d", &version, &count); err != nil || version != 1 || count <= 0 || count > 10016 {
		return fmt.Errorf("invalid header")
	}
	offset := lineEnd + 1
	for index := 0; index < count; index++ {
		lineEnd = bytes.IndexByte(frame[offset:], '\n')
		if lineEnd < 0 {
			return fmt.Errorf("record %d missing header", index)
		}
		var tag, length int
		if _, err := fmt.Sscanf(string(frame[offset:offset+lineEnd]), "%d %d", &tag, &length); err != nil || length < 0 {
			return fmt.Errorf("record %d invalid header", index)
		}
		offset += lineEnd + 1
		if offset+length >= len(frame) || frame[offset+length] != '\n' {
			return fmt.Errorf("record %d truncated", index)
		}
		offset += length + 1
		if index+1 == count && (tag != 255 || length != 0) {
			return fmt.Errorf("missing final record")
		}
	}
	if offset != len(frame) {
		return fmt.Errorf("trailing bytes")
	}
	return nil
}

func TestWorktreeLoaderSourceZeroProcessAndLazySurface(t *testing.T) {
	script := (Provider{}).HookScript()
	for _, want := range []string{
		"_zp_worktree_ensure_attached()", "_zp_worktree_publish()", "_zp_worktree_pull()",
		"_zp_worktree_sync()", "_zp_worktree_resolve_shared()", "zsh-pro()",
		"_zp_worktree_precmd()", "_zp_worktree_line_finish()",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("worktree loader missing %q", want)
		}
	}
	top := loaderTopLevel(script)
	for _, forbidden := range []string{"$(", "`", "command zsh-pro", "git ", "_zp_worktree_ensure_attached", "_zp_worktree_publish"} {
		if strings.Contains(top, forbidden) {
			t.Fatalf("worktree loader source path contains %q:\n%s", forbidden, top)
		}
	}

	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	seen := filepath.Join(dir, "process-seen")
	shim := filepath.Join(dir, "zsh-pro")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\n: > \"$ZP_SOURCE_PROCESS_SEEN\"\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("zsh", "-f", "-c", `source "$1"; [[ ! -e "$ZP_SOURCE_PROCESS_SEEN" ]]`, "zsh-pro-zero-process", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_SOURCE_PROCESS_SEEN="+seen)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sourcing loader was not zero-process: %v\n%s", err, out)
	}
}

func TestWorktreePublicDispatcherAndLegacyDelegates(t *testing.T) {
	script := (Provider{}).HookScript()
	for _, want := range []string{
		`zsh-pro() {`, `sync --resolve shared`, `ZSHPRO_SHELL_ID="$ZP_WORKTREE_SHELL_ID" command zsh-pro`,
		`checkout() {`, `zsh-pro checkout "$@"`, `activate() {`, `status() {`, `zsh-pro status "$@"`,
		`list() {`, `zsh-pro list "$@"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("installed dispatcher missing %q", want)
		}
	}
	for _, forbidden := range []string{"export ZP_WORKTREE_CAPABILITY", "ZSHPRO_SHELL_CAPABILITY", "runtime worktree pull", "runtime worktree query"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("installed dispatcher contains forbidden transport %q", forbidden)
		}
	}
}

func TestWorktreeCapabilityStdinAndReplyCleanupContract(t *testing.T) {
	script := (Provider{}).HookScript()
	for _, operation := range []string{"attach", "publish", "prepare", "acknowledge", "resolve"} {
		if !strings.Contains(script, `runtime worktree `+operation+` 1`) {
			t.Errorf("loader missing private %s route", operation)
		}
	}
	for _, want := range []string{
		"ZPWT 1", "255 0", "ZP_WORKTREE_REPLY_PROTOCOL", "ZP_WORKTREE_REPLY_REVISION",
		"ZP_WORKTREE_REPLY_TOKEN", "ZP_WORKTREE_REPLY_FINGERPRINT", "ZP_WORKTREE_REPLY_COMPLETE",
		"unset ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN",
		"setopt NOXTRACE", "fc -p", "always {",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("private transport/cleanup missing %q", want)
		}
	}
}

func TestWorktreeHookBoundariesAndAutoApplyContract(t *testing.T) {
	script := (Provider{}).HookScript()
	precmd := functionBody(script, "_zp_worktree_precmd")
	lineFinish := functionBody(script, "_zp_worktree_line_finish")
	if !strings.Contains(precmd, "_zp_worktree_publish") || strings.Contains(precmd, "_zp_worktree_pull") {
		t.Fatalf("precmd is not publish-only:\n%s", precmd)
	}
	if !strings.Contains(lineFinish, "_zp_worktree_pull") || strings.Contains(lineFinish, "_zp_worktree_publish") {
		t.Fatalf("line-finish is not pull-only:\n%s", lineFinish)
	}
	for _, want := range []string{"ZSHPRO_AUTO_APPLY", "true|false", "ZP_WORKTREE_AUTO_APPLY_EFFECTIVE", "ZP_WORKTREE_LAST_ERROR"} {
		if !strings.Contains(script, want) {
			t.Errorf("auto-apply contract missing %q", want)
		}
	}
}

func TestWorktreeConfigDispatcherRefreshesOnlyExactSuccessfulAutoApplyDefault(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "zsh-pro")
	shimSource := `#!/bin/sh
printf '%s\n' "$*" >> "$ZP_CONFIG_CALLS"
if test "${ZP_CONFIG_FAIL-}" = 1; then exit 9; fi
exit 0
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	const body = `
source "$1" || exit 10
typeset -g ZP_WORKTREE_ATTACHED=1
typeset -g ZP_WORKTREE_SHELL_ID=${(l:64::a:)}
typeset -g ZP_WORKTREE_CAPABILITY=${(l:64::b:)}
typeset -g ZP_WORKTREE_AUTO_APPLY_DEFAULT=true
zsh-pro config set auto-apply false >/dev/null || exit 11
[[ $ZP_WORKTREE_AUTO_APPLY_DEFAULT == false ]] || exit 12
ZSHPRO_AUTO_APPLY=true
_zp_worktree_effective_auto_apply || exit 13
[[ $ZP_WORKTREE_AUTO_APPLY_EFFECTIVE == true ]] || exit 14
unset ZSHPRO_AUTO_APPLY
ZP_CONFIG_FAIL=1 zsh-pro config set auto-apply true >/dev/null 2>&1 && exit 15
[[ $ZP_WORKTREE_AUTO_APPLY_DEFAULT == false ]] || exit 16
zsh-pro config set auto-apply TRUE >/dev/null || exit 17
zsh-pro config set auto-apply false extra >/dev/null || exit 18
zsh-pro config set auto-apply >/dev/null || exit 19
[[ $ZP_WORKTREE_AUTO_APPLY_DEFAULT == false ]] || exit 20
zsh-pro config set auto-apply true >/dev/null || exit 21
[[ $ZP_WORKTREE_AUTO_APPLY_DEFAULT == true ]] || exit 22
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-config-refresh", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_CONFIG_CALLS="+calls)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("config dispatcher: %v\n%s", err, output)
	}
	payload, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	want := "config set auto-apply false\nconfig set auto-apply true\nconfig set auto-apply TRUE\nconfig set auto-apply false extra\nconfig set auto-apply\nconfig set auto-apply true\n"
	if string(payload) != want {
		t.Fatalf("delegated config calls = %q", payload)
	}
}

func TestWorktreeDeactivateConsumesReverseAndDisablesReattach(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	const body = `
source "$1" || exit 10
export WORKTREE_DEACTIVATE_ENV=applied
alias worktree-deactivate-alias='print applied'
functions[_ordinary_user_function]='print ordinary'
__zp_worktree_reverse_1_1() {
  unset WORKTREE_DEACTIVATE_ENV
  unalias worktree-deactivate-alias
}
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_worktree_reverse_1_1
typeset -g ZP_WORKTREE_ATTACHED=1 ZP_WORKTREE_ATTACHED_NOW=1 ZP_WORKTREE_APPLIED_REVISION=7
typeset -g ZP_WORKTREE_SHELL_ID=${(l:64::a:)} ZP_WORKTREE_CAPABILITY=${(l:64::b:)}
typeset -g ZP_WORKTREE_REPLY_PROTOCOL=1 ZP_WORKTREE_REPLY_COMPLETE=1 ZP_WORKTREE_CONFLICT_TOKEN=private-token
deactivate || exit 11
[[ ${+WORKTREE_DEACTIVATE_ENV} == 0 && ${+aliases[worktree-deactivate-alias]} == 0 ]] || exit 12
[[ ${+functions[__zp_worktree_reverse_1_1]} == 0 && ${+ZP_ACTIVE_REVERSE_FN} == 0 ]] || exit 13
[[ ${+functions[_ordinary_user_function]} == 1 ]] || exit 14
[[ "${precmd_functions[(r)_zp_worktree_precmd]-}" != _zp_worktree_precmd ]] || exit 15
for name in ZP_WORKTREE_SHELL_ID ZP_WORKTREE_CAPABILITY ZP_WORKTREE_ATTACHED ZP_WORKTREE_APPLIED_REVISION ZP_WORKTREE_BASELINE_FIELDS ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_COMPLETE ZP_WORKTREE_CONFLICT_TOKEN; do
  (( ${+parameters[$name]} == 0 )) || exit 16
done
deactivate || exit 17
print -r -- usable
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-worktree-deactivate", loader)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "usable\n" {
		t.Fatalf("worktree deactivate = (%v, %q)", err, out)
	}
}

func functionBody(script, name string) string {
	start := strings.Index(script, name+"() {")
	if start < 0 {
		return ""
	}
	depth := 0
	for offset, line := range strings.Split(script[start:], "\n") {
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if offset > 0 && depth == 0 {
			lines := strings.Split(script[start:], "\n")
			return strings.Join(lines[:offset+1], "\n")
		}
	}
	return script[start:]
}

func TestHookScriptIsParseableAndDefinesRuntimeSurface(t *testing.T) {
	script := (Provider{}).HookScript()
	for _, name := range []string{"checkout", "activate", "deactivate", "list", "status"} {
		definition := regexp.MustCompile(`(?m)^` + name + `\(\) \{`)
		if got := len(definition.FindAllString(script, -1)); got != 1 {
			t.Errorf("%s definitions = %d, want 1", name, got)
		}
	}
	for _, surface := range []string{"zp_capture_env()", "zp_restore_env()", "_zp_capture_scalar()", "_zp_restore_scalar()", "_zp_run_payload()"} {
		if !strings.Contains(script, surface) {
			t.Errorf("loader missing %q", surface)
		}
	}
	if strings.Contains(script, "emulate -L") || strings.Contains(script, "LOCAL_OPTIONS") {
		t.Fatal("runtime loader must stay plain so option changes persist")
	}
	if got := loaderTopLevel(script); strings.Contains(got, "$(") || strings.Contains(got, "`") || strings.Contains(got, "git ") {
		t.Fatalf("loader top level starts a subprocess:\n%s", got)
	}

	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	cmd := exec.Command("zsh", "-n")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("HookScript is not zsh -n parseable: %v\n%s", err, out)
	}
}

func TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative(t *testing.T) {
	top := loaderTopLevel((Provider{}).HookScript())
	for _, forbidden := range []string{"$(", "`", "git ", "zsh-pro", "zp_rebuild_path", "zp_shadow"} {
		if strings.Contains(top, forbidden) {
			t.Fatalf("loader start path contains %q:\n%s", forbidden, top)
		}
	}
}

func TestHookScriptRuntimeStagingNeverUsesTMPDIR(t *testing.T) {
	script := (Provider{}).HookScript()
	if strings.Contains(script, "TMPDIR") {
		t.Fatal("runtime capture must not depend on a shared temporary directory")
	}
	if !strings.Contains(script, "_zp_run_bounded") {
		t.Fatal("loader must expose the shared bounded runtime boundary")
	}
	if strings.Contains(script, "zstat -L") || strings.Contains(script, "_zp_private_temp") || strings.Contains(script, "_zp_private_chain_safe") {
		t.Fatal("loader must not use shell pathname staging or a symlink-following proof")
	}
	for _, want := range []string{"zsh-pro runtime capture", "zsh-pro runtime validate"} {
		if !strings.Contains(script, want) {
			t.Fatalf("loader missing descriptor-backed runtime helper %q", want)
		}
	}
}

func TestHookScriptTracksEnvPresenceSeparatelyFromData(t *testing.T) {
	script := (Provider{}).HookScript()
	if strings.Contains(script, "ZP_UNSET_SENTINEL") {
		t.Fatal("loader must not encode an unset original value in a data sentinel")
	}
	if !strings.Contains(script, "__ZP_ORIG_${safe}_PRESENT") {
		t.Fatal("loader must store original environment presence in a separate sanitized slot")
	}
}

func TestHookScriptUsesNonExportedActivationMarker(t *testing.T) {
	script := (Provider{}).HookScript()
	if !strings.Contains(script, "ZP_ACTIVE_PROFILE") {
		t.Fatal("loader must track successful activation separately from inherited ZSHPRO_PROFILE")
	}
	if strings.Contains(script, "export ZP_ACTIVE_PROFILE") {
		t.Fatal("activation marker must stay in the sourced shell rather than being exported")
	}
}

// loaderTopLevel extracts lines outside function bodies. The loader may shell
// out only when a user explicitly invokes a verb, never while it is sourced.
func loaderTopLevel(script string) string {
	var lines []string
	depth := 0
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "() {") {
			depth = strings.Count(line, "{") - strings.Count(line, "}")
			continue
		}
		if depth == 0 {
			lines = append(lines, line)
			continue
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
	}
	return strings.Join(lines, "\n")
}
