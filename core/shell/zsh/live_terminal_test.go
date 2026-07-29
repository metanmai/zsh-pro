package zsh

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveTerminalLoaderSwitchesCurrentShellWithoutResidue(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	shim := filepath.Join(dir, "zsh-pro")
	const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:A) printf '%s\n' "__zp_deactivate_A() { unalias zp_test_alias 2>/dev/null; unset ZP_TEST_ENV; path=(\${(@s/:/)ZP_BASE_PATH}); }" "__zp_apply_A() { export ZP_TEST_ENV=A; alias zp_test_alias='print A'; path=(/zp-a/bin \$path); }" "_zp_run_payload __zp_apply_A __zp_deactivate_A" ;;
  emit:apply:B) printf '%s\n' "__zp_deactivate_B() { unalias zp_test_alias 2>/dev/null; unset ZP_TEST_ENV; path=(\${(@s/:/)ZP_BASE_PATH}); }" "__zp_apply_B() { export ZP_TEST_ENV=B; alias zp_test_alias='print B'; path=(/zp-b/bin \$path); }" "_zp_run_payload __zp_apply_B __zp_deactivate_B" ;;
  list) printf '%s\n' main A B ;;
  status) printf '%s\n' main ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	const body = `
source "$1"
before_path_count=$#path
checkout A || exit 10
[[ "$ZSHPRO_PROFILE" == A ]] || exit 11
[[ "$ZP_TEST_ENV" == A ]] || exit 12
alias zp_test_alias >/dev/null || exit 13
activate B || exit 14
[[ "$ZSHPRO_PROFILE" == B ]] || exit 15
deactivate || exit 16
[[ -z "${ZSHPRO_PROFILE+x}" ]] || exit 17
[[ -z "${ZP_TEST_ENV+x}" ]] || exit 18
alias zp_test_alias >/dev/null 2>&1 && exit 19
[[ "$(status)" == main ]] || exit 20
[[ $#path -eq $before_path_count ]] || exit 21

# An inherited profile without loader state is a fresh terminal, not a request
# to consume missing undo slots.
unset ZP_BASE_PATH
export ZSHPRO_PROFILE=A
activate A || exit 22
[[ -n "${ZP_BASE_PATH+x}" ]] || exit 23
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("loader did not mutate the current shell correctly: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestLiveTerminalActivationMarkerDistinguishesInheritedProfile(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	emitLog := filepath.Join(dir, "emit.log")
	shim := filepath.Join(dir, "zsh-pro")
	const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:A)
    printf '%s:%s\n' "$2" "$3" >> "$ZP_EMIT_LOG"
    printf '%s\n' "__zp_deactivate_marker() { unset ZP_MARKER_TEST; }" "__zp_apply_marker() { export ZP_MARKER_TEST=A; }" "_zp_run_payload __zp_apply_marker __zp_deactivate_marker"
    ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	const body = `
unset ZP_ACTIVE_PROFILE
export ZSHPRO_PROFILE=inherited
source "$1"
[[ -z "${ZP_ACTIVE_PROFILE+x}" ]] || exit 10
[[ "$(status)" == main ]] || exit 11

activate A
[[ "$ZP_ACTIVE_PROFILE" == A ]] || exit 12
[[ "$ZSHPRO_PROFILE" == A ]] || exit 13
[[ "$ZP_MARKER_TEST" == A ]] || exit 14
[[ "${parameters[ZP_ACTIVE_PROFILE]}" != *export* ]] || exit 15

activate A
[[ "$ZP_MARKER_TEST" == A ]] || exit 16

deactivate
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 17
[[ -z "${ZP_MARKER_TEST+x}" ]] || exit 18
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-marker-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"), "ZP_EMIT_LOG="+emitLog)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("loader did not distinguish inherited profile state: %v\n%s", err, out)
	}
	emissions, err := os.ReadFile(emitLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(emissions); got != "apply:A\n" {
		t.Fatalf("emissions = %q, want one target apply and no binary deactivate", got)
	}
}

func TestLiveTerminalFailedEvalLeavesTruthfulInactiveStateAndRecovers(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	shim := filepath.Join(dir, "zsh-pro")
	const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:good)
    printf '%s\n' \
      "__zp_deactivate_good() { unset ZP_A_ENV; unalias zp_a_alias 2>/dev/null; unset -f zp_a_function; unsetopt extendedglob; path=(\${(@s/:/)ZP_BASE_PATH}); }" \
      "__zp_apply_good() { export ZP_A_ENV=1; alias zp_a_alias='print -r -- A'; functions[zp_a_function]='print -r -- A'; setopt extendedglob; path=(/zp-a/bin \$path); }" \
      "_zp_run_payload __zp_apply_good __zp_deactivate_good"
    ;;
  emit:apply:bad)
    printf '%s\n' \
      "__zp_deactivate_bad() { unset ZP_B_ENV; unalias zp_b_alias 2>/dev/null; unset -f zp_b_function; unsetopt nomatch; path=(\${(@s/:/)ZP_BASE_PATH}); }" \
      "__zp_apply_bad() { export ZP_B_ENV=1; alias zp_b_alias='print -r -- B'; functions[zp_b_function]='print -r -- B'; setopt nomatch; path=(/zp-b/bin \$path); return 9; }" \
      "_zp_run_payload __zp_apply_bad __zp_deactivate_bad"
    ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	const body = `
unset ZSHPRO_PROFILE ZP_ACTIVE_PROFILE
source "$1"
before_path="$PATH"
unsetopt extendedglob nomatch
activate good
[[ "$ZP_ACTIVE_PROFILE" == good && "$ZSHPRO_PROFILE" == good ]] || exit 20
[[ "$ZP_A_ENV" == 1 ]] || exit 21
alias zp_a_alias >/dev/null || exit 22
(( ${+functions[zp_a_function]} )) || exit 23
[[ -o extendedglob ]] || exit 24
[[ "$PATH" == /zp-a/bin:* ]] || exit 25
activate bad
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 30
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 31
[[ "$(status)" == main ]] || exit 32
[[ -z "${ZP_A_ENV+x}" && -z "${ZP_B_ENV+x}" ]] || exit 33
alias zp_a_alias >/dev/null 2>&1 && exit 34
alias zp_b_alias >/dev/null 2>&1 && exit 35
(( ${+functions[zp_a_function]} || ${+functions[zp_b_function]} )) && exit 36
[[ ! -o extendedglob && ! -o nomatch ]] || exit 37
[[ "$PATH" == "$before_path" ]] || exit 38
(( ${+functions[__zp_apply_good]} || ${+functions[__zp_deactivate_good]} || ${+functions[__zp_apply_bad]} || ${+functions[__zp_deactivate_bad]} )) && exit 39

# The failed B transition left a truthful inactive state, so activating the
# previous profile must re-emit and restore it instead of taking a stale
# same-profile shortcut.
activate good
[[ "$ZP_ACTIVE_PROFILE" == good && "$ZSHPRO_PROFILE" == good ]] || exit 40
[[ "$ZP_A_ENV" == 1 && -z "${ZP_B_ENV+x}" ]] || exit 41
alias zp_a_alias >/dev/null || exit 42
(( ${+functions[zp_a_function]} )) || exit 43
[[ -o extendedglob && ! -o nomatch ]] || exit 44
[[ "$PATH" == /zp-a/bin:* ]] || exit 45
[[ ${+functions[__zp_apply_good]} == 0 && ${+functions[__zp_deactivate_good]} == 1 ]] || exit 46
deactivate
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 47
[[ -z "${ZP_A_ENV+x}" && -z "${ZP_B_ENV+x}" ]] || exit 48
alias zp_a_alias >/dev/null 2>&1 && exit 49
(( ${+functions[zp_a_function]} || ${+functions[zp_b_function]} )) && exit 50
(( ${+functions[__zp_apply_good]} || ${+functions[__zp_deactivate_good]} || ${+functions[__zp_apply_bad]} || ${+functions[__zp_deactivate_bad]} )) && exit 51
[[ ! -o extendedglob && ! -o nomatch && "$PATH" == "$before_path" ]] || exit 52
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-marker-failure-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed evaluation left an untruthful or unrecoverable shell: %v\n%s", err, out)
	}
}

func TestLiveTerminalRetainedSecretReverseSurvivesUnavailableBinary(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	secretSource := filepath.Join(dir, "secret-apply.zsh")
	if err := os.WriteFile(secretSource, []byte(`
__zp_deactivate_secret() { unset ZP_RUNTIME_SECRET; }
__zp_apply_secret() { export ZP_RUNTIME_SECRET=phase5-runtime-fixture; }
_zp_run_payload __zp_apply_secret __zp_deactivate_secret
`), 0o600); err != nil {
		t.Fatal(err)
	}
	bSource := filepath.Join(dir, "b-apply.zsh")
	if err := os.WriteFile(bSource, []byte(`
__zp_deactivate_b() { unset ZP_B_ONLY; }
__zp_apply_b() {
  [[ -z "${ZP_RUNTIME_SECRET+x}" ]] || return 91
  export ZP_B_ONLY=from-b
}
_zp_run_payload __zp_apply_b __zp_deactivate_b
`), 0o600); err != nil {
		t.Fatal(err)
	}
	emitLog := filepath.Join(dir, "emit.log")
	shim := filepath.Join(dir, "zsh-pro")
	const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B)
    printf '%s:%s\n' "$2" "$3" >> "$ZP_EMIT_LOG"
    cat "$ZP_APPLY_B"
    ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	const body = `
source "$1"

# This payload was generated and sourced while its resolver was available.
# Afterwards the binary has no reverse route: only the retained function may
# remove the old secret state.
source "$2"
typeset -g +x ZP_ACTIVE_PROFILE=secret
export ZSHPRO_PROFILE=secret
deactivate
[[ -z "${ZP_RUNTIME_SECRET+x}" ]] || exit 30
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 31

# Re-establish the already-emitted payload, then make a distinct target apply.
# Its payload refuses to apply until the retained secret reverse has run.
source "$2"
typeset -g +x ZP_ACTIVE_PROFILE=secret
export ZSHPRO_PROFILE=secret
activate B
[[ -z "${ZP_RUNTIME_SECRET+x}" ]] || exit 32
[[ "$ZP_B_ONLY" == from-b ]] || exit 33
[[ "$ZP_ACTIVE_PROFILE" == B && "$ZSHPRO_PROFILE" == B ]] || exit 34
deactivate
[[ -z "${ZP_B_ONLY+x}" ]] || exit 35
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 36
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-secret-reverse-test", loader, secretSource)
	cmd.Env = liveEnv(dir,
		"PATH="+dir+":"+os.Getenv("PATH"),
		"ZP_APPLY_B="+bSource,
		"ZP_EMIT_LOG="+emitLog,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("retained reverse depended on an unavailable binary: %v\n%s", err, out)
	}
	emissions, err := os.ReadFile(emitLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(emissions); got != "apply:B\n" {
		t.Fatalf("binary calls = %q, want only the new target apply", got)
	}
}

func TestLiveTerminalLoaderRejectsInvalidEmitWithoutChangingLastGood(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	shim := filepath.Join(dir, "zsh-pro")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nprintf '%s\\n' 'this is ( invalid zsh'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	validationMarker := filepath.Join(dir, "validated")
	validator := filepath.Join(dir, "zsh")
	validatorSource := "#!/bin/sh\ntouch \"$ZP_VALIDATION_MARKER\"\nexec \"$ZP_REAL_ZSH\" \"$@\"\n"
	if err := os.WriteFile(validator, []byte(validatorSource), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
checkout bad
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ "$ZSHPRO_PROFILE" == good ]] || exit 11
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 12
[[ -z "${ZP_TEST_ENV+x}" ]] || exit 13
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"), "ZP_VALIDATION_MARKER="+validationMarker, "ZP_REAL_ZSH="+realZsh)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid emit changed live state or escaped the validation gate: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(validationMarker); err != nil {
		t.Fatal("invalid emit never ran zsh -n validation")
	}
}

func TestLiveTerminalLoaderGatesEmptyEmitAndReportsRuntimeFailure(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	for _, tc := range []struct {
		name, emit, assertion string
	}{
		{"empty emit", "#!/bin/sh\nexit 0\n", `[[ "$ZSHPRO_PROFILE" == good ]] || exit 11; [[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 12`},
		{"runtime failure", "#!/bin/sh\nprintf '%s\\n' '__zp_deactivate_bad() { unset ZP_PARTIAL; }' '__zp_apply_bad() { export ZP_PARTIAL=1; return 9; }' '_zp_run_payload __zp_apply_bad __zp_deactivate_bad'\n", `[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 21; [[ -z "${ZP_PARTIAL+x}" ]] || exit 22`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			if err := os.WriteFile(filepath.Join(dir, "zsh-pro"), []byte(tc.emit), 0o700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dir, "validated")
			validator := "#!/bin/sh\ntouch \"$ZP_VALIDATION_MARKER\"\nexec \"$ZP_REAL_ZSH\" \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "zsh"), []byte(validator), 0o700); err != nil {
				t.Fatal(err)
			}
			body := "source \"$1\"; export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good; activate bad; [[ \"$ZP_LAST_RUNTIME_STATUS\" -ne 0 ]] || exit 10; " + tc.assertion
			cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"), "ZP_VALIDATION_MARKER="+marker, "ZP_REAL_ZSH="+realZsh)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("loader gate behavior failed: %v\n%s", err, out)
			}
			if tc.name == "empty emit" {
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("empty emit reached zsh -n instead of aborting before validation")
				}
			} else if !strings.Contains(string(out), "switch failed at runtime") {
				t.Fatalf("runtime failure had no recovery/report path: %s", out)
			}
		})
	}
}

func TestLiveTerminalPublicVerbsFailOpenUnderErrExitAndErrReturn(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		verb        string
		wantFailure bool
	}{
		{verb: "activate bad", wantFailure: true},
		{verb: "checkout bad", wantFailure: true},
		{verb: "deactivate"},
	} {
		for _, option := range []string{"ERR_EXIT", "ERR_RETURN"} {
			t.Run(tc.verb+"/"+option, func(t *testing.T) {
				dir := t.TempDir()
				loader := writeLiveLoader(t, dir)
				writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), "#!/bin/sh\nexit 9\n")
				statusAssertion := `[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 20`
				if !tc.wantFailure {
					// An exported name without the non-exported marker belongs to a
					// nested/fresh shell, so deactivation must not invoke the binary.
					statusAssertion = `[[ "$ZP_LAST_RUNTIME_STATUS" -eq 0 ]] || exit 20`
				}

				body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ` + option + `
` + tc.verb + `
print -r -- SURVIVED
` + statusAssertion + `
[[ "$ZSHPRO_PROFILE" == good ]] || exit 21
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 22
`
				cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
				cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s under %s escaped its fail-open boundary: %v\n%s", tc.verb, option, err, out)
				}
				if !strings.Contains(string(out), "SURVIVED") {
					t.Fatalf("%s under %s did not reach the next command:\n%s", tc.verb, option, out)
				}
			})
		}
	}
}

func TestLiveTerminalListUsesBoundedFailOpenBoundary(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name       string
		binary     string
		wantOutput string
		timedOut   bool
	}{
		{
			name:       "success",
			binary:     "#!/bin/sh\nprintf '%s\\n' main A B\n",
			wantOutput: "main\nA\nB\nSURVIVED\n",
		},
		{
			name: "missing binary",
		},
		{
			name:   "failing binary",
			binary: "#!/bin/sh\nprintf '%s\\n' STALE-LIST-OUTPUT\nexit 9\n",
		},
		{
			name:     "sleeping binary",
			binary:   "#!/bin/sh\nprintf '%s\\n' STALE-LIST-OUTPUT\nexec /bin/sleep 5\n",
			timedOut: true,
		},
	} {
		for _, option := range []string{"ERR_EXIT", "ERR_RETURN"} {
			t.Run(tc.name+"/"+option, func(t *testing.T) {
				dir := t.TempDir()
				loader := writeLiveLoader(t, dir)
				if tc.binary != "" {
					writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.binary)
				}

				body := `
source "$1"
setopt ` + option + `
list
list_rc=$?
print -r -- SURVIVED
[[ "$list_rc" -eq 0 ]] || exit 10
`
				if tc.wantOutput == "" {
					body += `
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 11
[[ -n "$ZP_LAST_RUNTIME_ERROR" ]] || exit 12
`
					if tc.timedOut {
						body += `
[[ "$ZP_RUNTIME_TIMED_OUT" -eq 1 ]] || exit 13
`
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, realZsh, "-f", "-c", body, "zsh-pro-test", loader)
				extra := []string{"PATH=" + dir + ":" + os.Getenv("PATH")}
				if tc.timedOut {
					extra = append(extra, "ZP_RUNTIME_TIMEOUT_SECONDS=1")
				}
				cmd.Env = liveEnv(dir, extra...)
				out, err := cmd.CombinedOutput()
				if ctx.Err() == context.DeadlineExceeded {
					t.Fatalf("%s under %s needed the external Go watchdog:\n%s", tc.name, option, out)
				}
				if err != nil {
					t.Fatalf("%s under %s escaped the public fail-open boundary: %v\n%s", tc.name, option, err, out)
				}
				if tc.wantOutput != "" {
					if got := string(out); got != tc.wantOutput {
						t.Fatalf("successful list output = %q, want %q", got, tc.wantOutput)
					}
					return
				}
				if !strings.Contains(string(out), "SURVIVED") {
					t.Fatalf("%s under %s did not reach the next command:\n%s", tc.name, option, out)
				}
				if strings.Contains(string(out), "STALE-LIST-OUTPUT") {
					t.Fatalf("%s under %s printed failed command output:\n%s", tc.name, option, out)
				}
			})
		}
	}
}

func TestLiveTerminalEnvRestorePreservesPresenceAndLegacyMarkerData(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	const body = `
source "$1"
legacy='__zsh_pro_unset_7c5a0a15__'

export ZP_LEGACY_VALUE="$legacy"
zp_capture_env ZP_LEGACY_VALUE
export ZP_LEGACY_VALUE=applied
zp_restore_env ZP_LEGACY_VALUE applied
[[ "${ZP_LEGACY_VALUE+x}" == x && "$ZP_LEGACY_VALUE" == "$legacy" ]] || exit 10
[[ -z "${__ZP_ORIG_ZP_LEGACY_VALUE+x}" && -z "${__ZP_ORIG_ZP_LEGACY_VALUE_PRESENT+x}" ]] || exit 11

export ZP_EMPTY_VALUE=''
zp_capture_env ZP_EMPTY_VALUE
export ZP_EMPTY_VALUE=applied
zp_restore_env ZP_EMPTY_VALUE applied
[[ "${ZP_EMPTY_VALUE+x}" == x && -z "$ZP_EMPTY_VALUE" ]] || exit 20
[[ -z "${__ZP_ORIG_ZP_EMPTY_VALUE+x}" && -z "${__ZP_ORIG_ZP_EMPTY_VALUE_PRESENT+x}" ]] || exit 21

unset ZP_ABSENT_VALUE
zp_capture_env ZP_ABSENT_VALUE
export ZP_ABSENT_VALUE=applied
zp_restore_env ZP_ABSENT_VALUE applied
[[ -z "${ZP_ABSENT_VALUE+x}" ]] || exit 30
[[ -z "${__ZP_ORIG_ZP_ABSENT_VALUE+x}" && -z "${__ZP_ORIG_ZP_ABSENT_VALUE_PRESENT+x}" ]] || exit 31

export ZP_DRIFT_VALUE=original
zp_capture_env ZP_DRIFT_VALUE
export ZP_DRIFT_VALUE=applied
export ZP_DRIFT_VALUE=manual
zp_restore_env ZP_DRIFT_VALUE applied
[[ "$ZP_DRIFT_VALUE" == manual ]] || exit 40
[[ -z "${__ZP_ORIG_ZP_DRIFT_VALUE+x}" && -z "${__ZP_ORIG_ZP_DRIFT_VALUE_PRESENT+x}" ]] || exit 41
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("environment restoration lost presence or literal data: %v\n%s", err, out)
	}
}

func TestLiveTerminalConsumesAllExpectedRuntimeFailures(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name           string
		emitter        string
		validator      string
		badRuntimeRoot bool
	}{
		{name: "emitter failure", emitter: "#!/bin/sh\nexit 9\n"},
		{name: "empty emitter output", emitter: "#!/bin/sh\nexit 0\n"},
		{name: "staging failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }' 'zp_apply'\n", badRuntimeRoot: true},
		{name: "validation failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }' 'zp_apply'\n", validator: "#!/bin/sh\nexit 9\n"},
		{name: "evaluation failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { return 9; }' 'zp_apply'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.emitter)
			if tc.validator != "" {
				writeLiveExecutable(t, filepath.Join(dir, "zsh"), tc.validator)
			}
			runtimeRoot := liveRuntimeDir(dir)
			if tc.badRuntimeRoot {
				runtimeRoot = filepath.Join(dir, "not-a-directory")
				if err := os.WriteFile(runtimeRoot, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ERR_EXIT
setopt XTRACE
activate bad
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 30
[[ "$ZSHPRO_PROFILE" == good ]] || exit 31
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 32
[[ -o xtrace ]] || exit 33
`
			cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = liveEnvAt(runtimeRoot, "PATH="+dir+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s escaped its runtime failure boundary: %v\n%s", tc.name, err, out)
			}
			if !strings.Contains(string(out), "SURVIVED") {
				t.Fatalf("%s did not reach the next command:\n%s", tc.name, out)
			}
			assertNoStagedSource(t, dir)
		})
	}
}

func TestLiveTerminalTimeoutsAreBoundedAndCleanedUp(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name      string
		emitter   string
		validator string
	}{
		{name: "emitter", emitter: "#!/bin/sh\nexec sleep 5\n"},
		{name: "validator", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }' 'zp_apply'\n", validator: "#!/bin/sh\nexec sleep 5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.emitter)
			if tc.validator != "" {
				writeLiveExecutable(t, filepath.Join(dir, "zsh"), tc.validator)
			}

			body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ERR_EXIT
activate bad
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 40
[[ "$ZSHPRO_PROFILE" == good ]] || exit 41
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 42
`
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			started := time.Now()
			cmd := exec.CommandContext(ctx, realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"), "TMPDIR="+dir, "ZP_RUNTIME_TIMEOUT_SECONDS=1")
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(started)
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("%s timeout needed the external Go watchdog after %s:\n%s", tc.name, elapsed, out)
			}
			if err != nil {
				t.Fatalf("%s timeout escaped its runtime failure boundary: %v\n%s", tc.name, err, out)
			}
			if elapsed > 2500*time.Millisecond {
				t.Fatalf("%s timeout took %s, want less than 2.5s", tc.name, elapsed)
			}
			if !strings.Contains(string(out), "SURVIVED") {
				t.Fatalf("%s timeout did not reach the next command:\n%s", tc.name, out)
			}
			assertNoStagedSource(t, dir)
		})
	}
}

func TestLiveTerminalStagingUsesExclusiveCreateAndCleansOnlyItsFile(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	body := `
source "$1"
RANDOM=1
collision="$TMPDIR/zsh-pro-eval-17767-$$"
print -r -- sentinel > "$collision"
_zp_eval_block $'zp_apply() { :; }\nzp_apply' good || exit 50
[[ "$(<"$collision")" == sentinel ]] || exit 51
rm -f -- "$collision"
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir, "TMPDIR="+dir, "PATH="+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("staging collision changed an existing file or left temp state: %v\n%s", err, out)
	}
	assertNoStagedSource(t, dir)
}

func TestLiveTerminalStagingRejectsSharedTMPDIRReplacement(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	shared := filepath.Join(dir, "shared")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	leakTarget := filepath.Join(dir, "attacker-target")
	if err := os.WriteFile(leakTarget, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	replacementTarget := filepath.Join(dir, "attacker-replacement")
	const replacementSource = "zp_apply() { export ZP_ATTACKED=1; }\nzp_apply\n"
	if err := os.WriteFile(replacementTarget, []byte(replacementSource), 0o600); err != nil {
		t.Fatal(err)
	}
	attackerSeen := filepath.Join(dir, "attacker-seen")
	attackerStop := filepath.Join(dir, "attacker-stop")

	const body = `
source "$1"
functions[_zp_private_temp_original]="${functions[_zp_private_temp]}"
_zp_private_temp() {
  _zp_private_temp_original "$@" || return $?
  command sleep 0.2
}
functions[_zp_run_bounded_original]="${functions[_zp_run_bounded]}"
_zp_run_bounded() {
  if [[ "$2" == zsh && "$3" == -n ]]; then command sleep 0.2; fi
  _zp_run_bounded_original "$@"
}
attacker() {
  local phase=0 candidate
  while [[ ! -e "$ZP_ATTACKER_STOP" ]]; do
    for candidate in "$TMPDIR"/zsh-pro-eval-*(N); do
      [[ -e "$candidate" || -L "$candidate" ]] || continue
      if (( phase == 0 )); then
        command rm -f -- "$candidate"
        if command ln -s -- "$ZP_ATTACK_TARGET" "$candidate"; then
          builtin print -r -- seen > "$ZP_ATTACKER_SEEN"
          phase=1
        fi
      elif (( phase == 1 )) && [[ -s "$ZP_ATTACK_TARGET" ]]; then
        command rm -f -- "$candidate"
        if command ln -s -- "$ZP_REPLACEMENT_TARGET" "$candidate"; then
          phase=2
        fi
      fi
    done
    command sleep 0.01
  done
}
attacker &
attacker_pid=$!
_zp_eval_block $'zp_apply() { export ZP_SECRET_LIKE="emitted-secret-like-payload"; }\nzp_apply' good
eval_rc=$?
: > "$ZP_ATTACKER_STOP"
if wait "$attacker_pid"; then :; else :; fi
(( eval_rc == 0 )) || exit 50
[[ -z "${ZP_ATTACKED+x}" ]] || exit 51
[[ ! -s "$ZP_ATTACK_TARGET" ]] || exit 52
[[ ! -e "$ZP_ATTACKER_SEEN" ]] || exit 53
[[ "$ZP_SECRET_LIKE" == emitted-secret-like-payload ]] || exit 54
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir,
		"PATH="+os.Getenv("PATH"),
		"TMPDIR="+shared,
		"ZP_ATTACK_TARGET="+leakTarget,
		"ZP_REPLACEMENT_TARGET="+replacementTarget,
		"ZP_ATTACKER_SEEN="+attackerSeen,
		"ZP_ATTACKER_STOP="+attackerStop,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shared TMPDIR replacement altered private staging: %v\n%s", err, out)
	}
	assertEmptyDir(t, shared)
	assertPrivateRuntimeClean(t, liveRuntimeDir(dir))
}

func TestLiveTerminalStagingRejectsNonStickyWritableAncestorReplacement(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	unsafeParent := filepath.Join(dir, "unsafe-parent")
	if err := os.Mkdir(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(unsafeParent, "victim-runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	attackerRoot := filepath.Join(dir, "attacker-runtime")
	if err := os.Mkdir(attackerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	attackerSource := filepath.Join(dir, "attacker-source.zsh")
	if err := os.WriteFile(attackerSource, []byte("__zp_deactivate_attacker() { unset ZP_ATTACKED; }\n__zp_apply_attacker() { export ZP_ATTACKED=1; }\n_zp_run_payload __zp_apply_attacker __zp_deactivate_attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	attackerSeen := filepath.Join(dir, "attacker-seen")
	stageSeen := filepath.Join(dir, "stage-seen")
	attackerStop := filepath.Join(dir, "attacker-stop")
	parkedRoot := filepath.Join(unsafeParent, "victim-runtime-parked")

	const body = `
source "$1"
restore_root() {
  if [[ -L "$ZSHPRO_HOME" ]]; then command rm -f -- "$ZSHPRO_HOME"; fi
  if [[ ! -e "$ZSHPRO_HOME" && -d "$ZP_PARKED_ROOT" ]]; then
    command mv -- "$ZP_PARKED_ROOT" "$ZSHPRO_HOME" 2>/dev/null || :
  fi
}
attacker() {
  local candidate
  while [[ ! -e "$ZP_ATTACKER_STOP" ]]; do
    if [[ -d "$ZSHPRO_HOME" && ! -L "$ZSHPRO_HOME" ]]; then
      if command mv -- "$ZSHPRO_HOME" "$ZP_PARKED_ROOT" 2>/dev/null; then
        if command ln -s -- "$ZP_ATTACK_ROOT" "$ZSHPRO_HOME"; then
          builtin print -r -- root > "$ZP_ATTACKER_SEEN"
        fi
      fi
    else
      restore_root
    fi
    for candidate in "$ZSHPRO_HOME"/.runtime-*/zsh-pro-eval(N) "$ZP_PARKED_ROOT"/.runtime-*/zsh-pro-eval(N); do
      [[ -e "$candidate" || -L "$candidate" ]] || continue
      command rm -f -- "$candidate"
      if command ln -s -- "$ZP_ATTACK_SOURCE" "$candidate"; then
        builtin print -r -- stage > "$ZP_STAGE_SEEN"
      fi
    done
    command sleep 0.001
  done
  restore_root
}
attacker &
attacker_pid=$!
typeset -i attempts=0
while [[ ! -e "$ZP_ATTACKER_SEEN" && attempts -lt 100 ]]; do
  command sleep 0.01
  (( attempts += 1 ))
done
_zp_eval_block $'export ZP_SECRET_LIKE="emitted-secret-like-payload"\n:'
eval_rc=$?
: > "$ZP_ATTACKER_STOP"
if wait "$attacker_pid"; then :; else :; fi
restore_root
[[ -e "$ZP_ATTACKER_SEEN" ]] || exit 50
(( eval_rc != 0 )) || exit 51
[[ -z "${ZP_ATTACKED+x}" && -z "${ZP_SECRET_LIKE+x}" ]] || exit 52
[[ ! -e "$ZP_STAGE_SEEN" ]] || exit 53
[[ -d "$ZSHPRO_HOME" && ! -L "$ZSHPRO_HOME" ]] || exit 54
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-unsafe-ancestor-test", loader)
	cmd.Env = liveEnvAt(runtimeRoot,
		"PATH="+os.Getenv("PATH"),
		"ZP_ATTACK_ROOT="+attackerRoot,
		"ZP_ATTACK_SOURCE="+attackerSource,
		"ZP_ATTACKER_SEEN="+attackerSeen,
		"ZP_STAGE_SEEN="+stageSeen,
		"ZP_ATTACKER_STOP="+attackerStop,
		"ZP_PARKED_ROOT="+parkedRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("non-sticky writable ancestor allowed root/stage replacement: %v\n%s", err, out)
	}
	assertPrivateRuntimeClean(t, runtimeRoot)
}

func writeLiveLoader(t *testing.T, dir string) string {
	t.Helper()
	ensureLiveRuntimeDir(t, dir)
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	return loader
}

func ensureLiveRuntimeDir(t *testing.T, dir string) {
	t.Helper()
	runtimeDir := liveRuntimeDir(dir)
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func liveRuntimeDir(dir string) string {
	return filepath.Join(dir, "runtime")
}

func liveEnv(dir string, extras ...string) []string {
	return liveEnvAt(liveRuntimeDir(dir), extras...)
}

func liveEnvAt(runtimeDir string, extras ...string) []string {
	replaced := map[string]bool{"ZSHPRO_HOME": true}
	for _, extra := range extras {
		key, _, _ := strings.Cut(extra, "=")
		replaced[key] = true
	}
	env := make([]string, 0, len(os.Environ())+len(extras)+1)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !replaced[key] {
			env = append(env, entry)
		}
	}
	env = append(env, "ZSHPRO_HOME="+runtimeDir)
	return append(env, extras...)
}

func writeLiveExecutable(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertNoStagedSource(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "loader.zsh", "zsh-pro", "zsh", "not-a-directory":
			continue
		case "runtime":
			assertPrivateRuntimeClean(t, filepath.Join(dir, entry.Name()))
			continue
		default:
			t.Fatalf("unexpected staged runtime file %q remains in %s", entry.Name(), dir)
		}
	}
}

func assertPrivateRuntimeClean(t *testing.T, runtimeDir string) {
	t.Helper()
	assertEmptyDir(t, runtimeDir)
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("private runtime directory %s still contains %q", dir, entries[0].Name())
	}
}
