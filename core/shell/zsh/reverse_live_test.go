package zsh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/activate"
)

func TestLiveTerminalReadonlyRetainedReverseFailsOpenAndRetries(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	aPayload := emittedRetainedReversePayload(t, "readonly_a", activate.Plan{
		Activate: []activate.Op{
			activate.SetScalar{Name: "ZP_REVERSE_FIRST", Applied: "first-applied", Exported: true},
			activate.SetScalar{Name: "ZP_REVERSE_LOCKED", Applied: "locked-applied", Exported: true},
		},
		Deactivate: []activate.Op{
			activate.RestoreScalar{Name: "ZP_REVERSE_FIRST", Applied: "first-applied"},
			activate.RestoreScalar{Name: "ZP_REVERSE_LOCKED", Applied: "locked-applied"},
		},
	})
	bPayload := emittedRetainedReversePayload(t, "readonly_b", activate.Plan{
		Activate: []activate.Op{
			activate.SetScalar{Name: "ZP_REVERSE_B", Applied: "b-applied", Exported: true},
		},
		Deactivate: []activate.Op{
			activate.RestoreScalar{Name: "ZP_REVERSE_B", Applied: "b-applied"},
		},
	})
	firstOriginal := encodeSlot("ORIGINAL_SCALAR", "ZP_REVERSE_FIRST")
	firstPresence := encodeSlot("PRESENT_SCALAR", "ZP_REVERSE_FIRST")
	lockedOriginal := encodeSlot("ORIGINAL_SCALAR", "ZP_REVERSE_LOCKED")
	lockedPresence := encodeSlot("PRESENT_SCALAR", "ZP_REVERSE_LOCKED")

	for _, verb := range []struct {
		name   string
		invoke string
	}{
		{name: "deactivate", invoke: "deactivate"},
		{name: "switch", invoke: "activate B"},
	} {
		for _, locked := range []struct {
			name string
			var_ string
		}{
			{name: "applied-scalar", var_: "ZP_REVERSE_LOCKED"},
			{name: "undo-slot", var_: lockedOriginal},
		} {
			for _, option := range []string{"ERR_EXIT", "ERR_RETURN"} {
				t.Run(verb.name+"/"+locked.name+"/"+option, func(t *testing.T) {
					dir := t.TempDir()
					loader := writeLiveLoader(t, dir)
					aPath := filepath.Join(dir, "a.zsh")
					bPath := filepath.Join(dir, "b.zsh")
					if err := os.WriteFile(aPath, []byte(aPayload), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(bPath, []byte(bPayload), 0o600); err != nil {
						t.Fatal(err)
					}
					if verb.name == "switch" {
						writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B) cat "$ZP_REVERSE_B_PAYLOAD" ;;
  *) exit 64 ;;
esac
`)
					}

					body := fmt.Sprintf(`
source "$1"
export ZP_REVERSE_FIRST=first-before ZP_REVERSE_LOCKED=locked-before
source "$2"
typeset -g ZP_BASE_PATH="$PATH"
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
before_reverse="$ZP_ACTIVE_REVERSE_FN"
typeset -gr %s
setopt %s
%s
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 11
[[ "$ZP_ACTIVE_REVERSE_FN" == "$before_reverse" && ${+functions[$before_reverse]} == 1 ]] || exit 12
[[ "$ZP_REVERSE_FIRST" == first-applied && "$ZP_REVERSE_LOCKED" == locked-applied ]] || exit 13
[[ ${+%s} == 1 && ${+%s} == 1 ]] || exit 14
[[ ${+%s} == 1 && ${+%s} == 1 ]] || exit 15
[[ -z "${ZP_REVERSE_B+x}" ]] || exit 16
[[ "$ZP_LAST_RUNTIME_ERROR" == *'active profile reversal failed'* ]] || exit 17
unsetopt ERR_EXIT ERR_RETURN
typeset +r %s
deactivate
print -r -- RETRIED
[[ "$ZP_LAST_RUNTIME_STATUS" -eq 0 ]] || exit 20
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" && -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 21
[[ "$ZP_REVERSE_FIRST" == first-before && "$ZP_REVERSE_LOCKED" == locked-before ]] || exit 22
[[ ${+%s} == 0 && ${+%s} == 0 ]] || exit 23
[[ ${+%s} == 0 && ${+%s} == 0 ]] || exit 24
[[ ${+functions[$before_reverse]} == 0 ]] || exit 25
				`, locked.var_, option, verb.invoke, firstOriginal, firstPresence, lockedOriginal, lockedPresence, locked.var_, firstOriginal, firstPresence, lockedOriginal, lockedPresence)
					cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-readonly-reverse-test", loader, aPath)
					cmd.Env = liveEnv(dir,
						"PATH="+dir+":"+os.Getenv("PATH"),
						"ZP_REVERSE_B_PAYLOAD="+bPath,
					)
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("%s under %s lost retained reverse recovery: %v\n%s", verb.name, option, err, out)
					}
					for _, want := range []string{"SURVIVED", "RETRIED"} {
						if !strings.Contains(string(out), want) {
							t.Fatalf("%s under %s did not reach %q:\n%s", verb.name, option, want, out)
						}
					}
				})
			}
		}
	}
}

func TestLiveTerminalPartialRetainedReverseKeepsRecoveryState(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	firstOriginal := encodeSlot("ORIGINAL_SCALAR", "ZP_PARTIAL_FIRST")
	firstPresence := encodeSlot("PRESENT_SCALAR", "ZP_PARTIAL_FIRST")
	firstExported := encodeSlot("EXPORTED_SCALAR", "ZP_PARTIAL_FIRST")
	secondOriginal := encodeSlot("ORIGINAL_SCALAR", "ZP_PARTIAL_SECOND")
	secondPresence := encodeSlot("PRESENT_SCALAR", "ZP_PARTIAL_SECOND")
	secondExported := encodeSlot("EXPORTED_SCALAR", "ZP_PARTIAL_SECOND")

	body := fmt.Sprintf(`
source "$1"
export ZP_PARTIAL_FIRST=first-applied ZP_PARTIAL_SECOND=second-applied
typeset -g %s=first-before %s=1 %s=1
typeset -g %s=second-before %s=1 %s=1
__zp_partial_reverse() {
  _zp_restore_scalar ZP_PARTIAL_FIRST first-applied %s %s %s || return $?
  return 73
  _zp_restore_scalar ZP_PARTIAL_SECOND second-applied %s %s %s || return $?
}
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_partial_reverse
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
deactivate
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ "$ZP_PARTIAL_FIRST" == first-before && "$ZP_PARTIAL_SECOND" == second-applied ]] || exit 11
[[ "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 12
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_partial_reverse && ${+functions[__zp_partial_reverse]} == 1 ]] || exit 13
[[ ${+%s} == 1 && ${+%s} == 1 && ${+%s} == 1 ]] || exit 14
[[ ${+%s} == 1 && ${+%s} == 1 && ${+%s} == 1 ]] || exit 15
`, firstOriginal, firstPresence, firstExported, secondOriginal, secondPresence, secondExported, firstOriginal, firstPresence, firstExported, secondOriginal, secondPresence, secondExported, firstOriginal, firstPresence, firstExported, secondOriginal, secondPresence, secondExported)
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-partial-reverse-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("partial retained reverse lost recovery state: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Fatalf("partial retained reverse did not return through the public boundary:\n%s", out)
	}
}

func emittedRetainedReversePayload(t *testing.T, prefix string, plan activate.Plan) string {
	t.Helper()
	applyName := "__zp_" + prefix + "_apply"
	reverseName := "__zp_" + prefix + "_reverse"
	apply, reverse, err := (Provider{}).EmitRuntime(plan, applyName, reverseName)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join([]string{reverse, apply, "_zp_run_payload " + applyName + " " + reverseName}, "\n") + "\n"
}
