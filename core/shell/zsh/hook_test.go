package zsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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
