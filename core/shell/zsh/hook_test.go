package zsh

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

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
		t.Fatal("runtime staging must stay beneath the zsh-pro private cache root, not TMPDIR")
	}
	if !strings.Contains(script, "_zp_run_bounded") {
		t.Fatal("loader must expose the shared bounded runtime boundary")
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
