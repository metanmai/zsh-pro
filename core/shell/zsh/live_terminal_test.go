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
	writeLiveExecutable(t, shim, shimSource)
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
	writeLiveExecutable(t, shim, shimSource)
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
	writeLiveExecutable(t, shim, shimSource)
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

func TestLiveTerminalTargetPreflightFailuresPreserveActiveProfile(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name    string
		failure string
	}{
		{name: "emitter nonzero", failure: "nonzero"},
		{name: "emitter empty output", failure: "empty"},
		{name: "validator rejection", failure: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			shim := filepath.Join(dir, "zsh-pro")
			const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B)
    case "$ZP_PREFLIGHT_FAILURE" in
      nonzero) exit 7 ;;
      empty) exit 0 ;;
      invalid) printf '%s\n' 'if then' ;;
      valid) printf '%s\n' '__zp_deactivate_B() { unset ZP_B_ENV; }' '__zp_apply_B() { export ZP_B_ENV=1; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B' ;;
      *) exit 64 ;;
    esac
    ;;
  *) exit 64 ;;
esac
`
			writeLiveExecutable(t, shim, shimSource)

			runtimeRoot := liveRuntimeDir(dir)
			const body = `
source "$1"
unsetopt extendedglob
typeset -g ZP_BASE_PATH="$PATH"
__zp_reverse_A() {
  unset ZP_A_ENV
  unalias zp_a_alias 2>/dev/null
  unset -f zp_a_function
  unsetopt extendedglob
  path=(${(@s/:/)ZP_BASE_PATH})
}
__zp_apply_A() {
  export ZP_A_ENV=present
  alias zp_a_alias='print -r -- A'
  functions[zp_a_function]='print -r -- A'
  setopt extendedglob
  path=(/zp-a/bin $path)
}
_zp_run_payload __zp_apply_A __zp_reverse_A || exit 10
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
before_path="$PATH"
before_reverse="$ZP_ACTIVE_REVERSE_FN"
before_reverse_body="${functions[$before_reverse]}"
before_function_body="${functions[zp_a_function]}"

activate B
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 20
[[ "$ZP_LAST_RUNTIME_ERROR" == *'shell state unchanged'* ]] || exit 21
[[ "$ZP_A_ENV" == present && -z "${ZP_B_ENV+x}" ]] || exit 22
alias zp_a_alias >/dev/null || exit 23
[[ "${functions[zp_a_function]}" == "$before_function_body" ]] || exit 24
[[ -o extendedglob && "$PATH" == "$before_path" ]] || exit 25
[[ "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 26
[[ "$ZP_ACTIVE_REVERSE_FN" == "$before_reverse" ]] || exit 27
[[ ${+functions[$before_reverse]} == 1 && "${functions[$before_reverse]}" == "$before_reverse_body" ]] || exit 28
`
			cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-preflight-test", loader)
			extra := []string{
				"PATH=" + dir + ":" + os.Getenv("PATH"),
				"ZP_PREFLIGHT_FAILURE=" + tc.failure,
			}
			cmd.Env = liveEnvAt(runtimeRoot, extra...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("target preflight changed active A: %v\n%s", err, out)
			}
		})
	}
}

// This exercises the production runtime helper, rather than the ordinary
// emit-fixture shim. An invalid configured root makes capture fail before the
// helper can start the target emitter, which is the staging/capture preflight
// boundary that must leave a live A entirely intact.
func TestLiveTerminalTargetStagingFailurePreservesActiveProfile(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	runtimeHelper := buildLiveRuntimeHelper(t, dir)
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	emitSeen := filepath.Join(dir, "emit-seen")
	emitter := filepath.Join(binDir, "zsh-pro")
	if err := os.WriteFile(emitter, []byte(`#!/bin/sh
: > "$ZP_EMIT_SEEN"
printf '%s\n' '__zp_deactivate_B() { unset ZP_B_ENV; }' '__zp_apply_B() { export ZP_B_ENV=1; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B'
`), 0o700); err != nil {
		t.Fatal(err)
	}
	unsafeRoot := filepath.Join(dir, "not-a-runtime-directory")
	if err := os.WriteFile(unsafeRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	const body = `
source "$1"
zsh-pro() {
  if [[ "$1" == runtime ]]; then
    command "$ZP_RUNTIME_HELPER" "$@"
  else
    command "$ZP_EMITTER_SHIM" "$@"
  fi
}
unsetopt extendedglob
typeset -g ZP_BASE_PATH="$PATH"
__zp_reverse_A() {
  unset ZP_A_ENV
  unalias zp_a_alias 2>/dev/null
  unset -f zp_a_function
  unsetopt extendedglob
  path=(${(@s/:/)ZP_BASE_PATH})
}
__zp_apply_A() {
  export ZP_A_ENV=present
  alias zp_a_alias='print -r -- A'
  functions[zp_a_function]='print -r -- A'
  setopt extendedglob
  path=(/zp-a/bin $path)
}
_zp_run_payload __zp_apply_A __zp_reverse_A || exit 10
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
before_path="$PATH"
before_reverse="$ZP_ACTIVE_REVERSE_FN"
before_reverse_body="${functions[$before_reverse]}"
before_function_body="${functions[zp_a_function]}"

activate B
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 20
[[ "$ZP_LAST_RUNTIME_ERROR" == *'shell state unchanged'* ]] || exit 21
[[ ! -e "$ZP_EMIT_SEEN" ]] || exit 22
[[ "$ZP_A_ENV" == present && -z "${ZP_B_ENV+x}" ]] || exit 23
alias zp_a_alias >/dev/null || exit 24
[[ "${functions[zp_a_function]}" == "$before_function_body" ]] || exit 25
[[ -o extendedglob && "$PATH" == "$before_path" ]] || exit 26
[[ "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 27
[[ "$ZP_ACTIVE_REVERSE_FN" == "$before_reverse" ]] || exit 28
[[ ${+functions[$before_reverse]} == 1 && "${functions[$before_reverse]}" == "$before_reverse_body" ]] || exit 29
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-staging-preflight-test", loader)
	cmd.Env = liveEnvAt(unsafeRoot,
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"ZP_RUNTIME_HELPER="+runtimeHelper,
		"ZP_EMITTER_SHIM="+emitter,
		"ZP_EMIT_SEEN="+emitSeen,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("target staging preflight changed active A: %v\n%s", err, out)
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
	writeLiveExecutable(t, shim, shimSource)
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
	writeLiveExecutable(t, shim, "#!/bin/sh\nprintf '%s\\n' 'this is ( invalid zsh'\n")
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
			writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.emit)
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

func TestLiveTerminalNoArgumentVerbsFailOpenUnderNoUnset(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, verb := range []string{"activate", "checkout"} {
		for _, option := range []struct {
			name  string
			setup string
		}{
			{name: "NO_UNSET", setup: "setopt NO_UNSET"},
			{name: "NO_UNSET_ERR_EXIT", setup: "setopt NO_UNSET ERR_EXIT"},
			{name: "NO_UNSET_ERR_RETURN", setup: "setopt NO_UNSET ERR_RETURN"},
		} {
			t.Run(verb+"/"+option.name, func(t *testing.T) {
				dir := t.TempDir()
				loader := writeLiveLoader(t, dir)
				emitSeen := filepath.Join(dir, "no-argument-emit-seen")
				writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
: > "$ZP_NO_ARGUMENT_EMIT_SEEN"
printf '%s\n' \
  '__zp_no_argument_reverse() { unset ZP_NO_ARGUMENT_SECRET; }' \
  '__zp_no_argument_apply() { export ZP_NO_ARGUMENT_SECRET=phase5-no-argument-secret; }' \
  '_zp_run_payload __zp_no_argument_apply __zp_no_argument_reverse'
`)

				body := `
source "$1"
unset ZP_ACTIVE_PROFILE ZSHPRO_PROFILE ZP_ACTIVE_REVERSE_FN ZP_RECOVERY_REVERSE_FN ZP_LAST_GOOD_PROFILE ZP_BASE_PATH ZP_NO_ARGUMENT_SECRET REPLY
` + option.setup + `
` + verb + `
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -eq 2 ]] || exit 10
[[ "$ZP_LAST_RUNTIME_ERROR" == "usage: ` + verb + ` <profile>" ]] || exit 11
[[ -z "${ZP_ACTIVE_PROFILE+x}" ]] || exit 12
[[ -z "${ZSHPRO_PROFILE+x}" ]] || exit 13
[[ -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 14
[[ -z "${ZP_RECOVERY_REVERSE_FN+x}" ]] || exit 15
[[ -z "${ZP_LAST_GOOD_PROFILE+x}" ]] || exit 16
[[ -z "${ZP_BASE_PATH+x}" ]] || exit 17
[[ "$ZP_RUNTIME_TIMED_OUT" -eq 0 ]] || exit 18
[[ -z "${ZP_NO_ARGUMENT_SECRET+x}" ]] || exit 19
[[ -z "${REPLY+x}" ]] || exit 20
for function_name in ${(k)functions}; do
  [[ "${functions[$function_name]}" != *phase5-no-argument-secret* ]] || exit 21
done
`
				cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-no-argument-test", loader)
				cmd.Env = liveEnv(dir,
					"PATH="+dir+":"+os.Getenv("PATH"),
					"ZP_NO_ARGUMENT_EMIT_SEEN="+emitSeen,
				)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s without an argument under %s escaped its fail-open boundary: %v\n%s", verb, option.name, err, out)
				}
				if !strings.Contains(string(out), "SURVIVED") {
					t.Fatalf("%s without an argument under %s did not reach the next command:\n%s", verb, option.name, out)
				}
				if !strings.Contains(string(out), "zsh-pro: usage: "+verb+" <profile>") {
					t.Fatalf("%s without an argument under %s omitted its usage diagnostic:\n%s", verb, option.name, out)
				}
				if strings.Contains(string(out), "phase5-no-argument-secret") {
					t.Fatalf("%s without an argument under %s disclosed a secret:\n%s", verb, option.name, out)
				}
				if _, err := os.Stat(emitSeen); err == nil {
					t.Fatalf("%s without an argument under %s invoked the emitter", verb, option.name)
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat no-argument emitter marker: %v", err)
				}
			})
		}
	}
}

// TestLiveTerminalPublicVerbArityFailsOpenBeforeRuntimeWork covers every
// public sourced verb at its boundary. Invalid calls must never emit, capture,
// reverse, or clear retained profile state, even when caller shell options make
// ordinary nonzero returns fatal.
func TestLiveTerminalPublicVerbArityFailsOpenBeforeRuntimeWork(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name       string
		invocation string
		usage      string
	}{
		{name: "activate missing", invocation: "activate", usage: "usage: activate <profile>"},
		{name: "activate surplus", invocation: "activate target accidental", usage: "usage: activate <profile>"},
		{name: "checkout missing", invocation: "checkout", usage: "usage: checkout <profile>"},
		{name: "checkout surplus", invocation: "checkout target accidental", usage: "usage: checkout <profile>"},
		{name: "deactivate surplus", invocation: "deactivate accidental", usage: "usage: deactivate"},
		{name: "list surplus", invocation: "list accidental", usage: "usage: list"},
		{name: "status surplus", invocation: "status accidental", usage: "usage: status"},
	} {
		for _, option := range []struct {
			name  string
			setup string
		}{
			{name: "normal"},
			{name: "NO_UNSET", setup: "setopt NO_UNSET"},
			{name: "ERR_EXIT", setup: "setopt ERR_EXIT"},
			{name: "ERR_RETURN", setup: "setopt ERR_RETURN"},
			{name: "NO_UNSET_ERR_EXIT", setup: "setopt NO_UNSET ERR_EXIT"},
			{name: "NO_UNSET_ERR_RETURN", setup: "setopt NO_UNSET ERR_RETURN"},
		} {
			t.Run(tc.name+"/"+option.name, func(t *testing.T) {
				dir := t.TempDir()
				loader := writeLiveLoader(t, dir)
				emitSeen := filepath.Join(dir, "arity-emit-seen")
				reverseSeen := filepath.Join(dir, "arity-reverse-seen")
				writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:*)
    : > "$ZP_ARGUMENT_EMIT_SEEN"
    printf '%s\n' \
      '__zp_argument_target_reverse() { unset ZP_ARGUMENT_TARGET; }' \
      '__zp_argument_target_apply() { export ZP_ARGUMENT_TARGET=changed; }' \
      '_zp_run_payload __zp_argument_target_apply __zp_argument_target_reverse'
    ;;
  list)
    : > "$ZP_ARGUMENT_EMIT_SEEN"
    printf '%s\n' main target
    ;;
  *) exit 64 ;;
esac
`)

				body := `
source "$1"
typeset -g +x ZP_ACTIVE_PROFILE=keep
export ZSHPRO_PROFILE=keep
typeset -g ZP_LAST_GOOD_PROFILE=keep
typeset -g ZP_BASE_PATH="$PATH"
typeset -g ZP_ARGUMENT_SECRET=phase5-arity-secret
typeset -g ZP_RUNTIME_TIMED_OUT=1
typeset -g REPLY=phase5-arity-reply
__zp_argument_reverse() {
  print -r -- invoked > "$ZP_ARGUMENT_REVERSE_SEEN"
  unset ZP_ARGUMENT_SECRET
}
typeset -g ZP_ACTIVE_REVERSE_FN=__zp_argument_reverse
active_reverse_body="${functions[$ZP_ACTIVE_REVERSE_FN]}"
` + option.setup + `
` + tc.invocation + `
call_rc=$?
print -r -- SURVIVED
[[ "$call_rc" -eq 0 ]] || exit 10
[[ "$ZP_LAST_RUNTIME_STATUS" -eq 2 ]] || exit 11
[[ "$ZP_LAST_RUNTIME_ERROR" == "` + tc.usage + `" ]] || exit 12
[[ "$ZP_ACTIVE_PROFILE" == keep && "$ZSHPRO_PROFILE" == keep ]] || exit 13
[[ "$ZP_LAST_GOOD_PROFILE" == keep && "$ZP_BASE_PATH" == "$PATH" ]] || exit 14
[[ "$ZP_ACTIVE_REVERSE_FN" == __zp_argument_reverse ]] || exit 15
[[ ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 && "${functions[$ZP_ACTIVE_REVERSE_FN]}" == "$active_reverse_body" ]] || exit 16
[[ "$ZP_ARGUMENT_SECRET" == phase5-arity-secret && -z "${ZP_ARGUMENT_TARGET+x}" ]] || exit 17
[[ "$REPLY" == phase5-arity-reply && "$ZP_RUNTIME_TIMED_OUT" -eq 1 ]] || exit 18
[[ ! -e "$ZP_ARGUMENT_EMIT_SEEN" && ! -e "$ZP_ARGUMENT_REVERSE_SEEN" ]] || exit 19
`
				cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-arity-test", loader)
				cmd.Env = liveEnv(dir,
					"PATH="+dir+":"+os.Getenv("PATH"),
					"ZP_ARGUMENT_EMIT_SEEN="+emitSeen,
					"ZP_ARGUMENT_REVERSE_SEEN="+reverseSeen,
				)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s under %s escaped its fail-open boundary: %v\n%s", tc.name, option.name, err, out)
				}
				wantOutput := "zsh-pro: " + tc.usage + "\nSURVIVED\n"
				if got := string(out); got != wantOutput {
					t.Fatalf("%s under %s output = %q, want %q", tc.name, option.name, got, wantOutput)
				}
				for _, path := range []string{emitSeen, reverseSeen} {
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Fatalf("%s under %s mutated runtime state through %s: %v", tc.name, option.name, path, statErr)
					}
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
		captureFailure bool
	}{
		{name: "emitter failure", emitter: "#!/bin/sh\nexit 9\n"},
		{name: "empty emitter output", emitter: "#!/bin/sh\nexit 0\n"},
		{name: "staging failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }' 'zp_apply'\n", captureFailure: true},
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
			extra := []string{"PATH=" + dir + ":" + os.Getenv("PATH")}
			if tc.captureFailure {
				extra = append(extra, "ZP_RUNTIME_SHIM_CAPTURE_FAILURE=1")
			}
			cmd.Env = liveEnvAt(runtimeRoot, extra...)
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

func TestLiveTerminalRuntimeTransportDoesNotTouchTMPDIR(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B) printf '%s\n' '__zp_deactivate_B() { unset ZP_TRANSPORT; }' '__zp_apply_B() { export ZP_TRANSPORT=ok; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B' ;;
  *) exit 64 ;;
esac
`)
	body := `
source "$1"
RANDOM=1
collision="$TMPDIR/zsh-pro-eval-17767-$$"
print -r -- sentinel > "$collision"
activate B
[[ "$ZP_TRANSPORT" == ok ]] || exit 50
[[ "$(<"$collision")" == sentinel ]] || exit 51
rm -f -- "$collision"
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir, "TMPDIR="+dir, "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime transport touched TMPDIR or left staged state: %v\n%s", err, out)
	}
	assertNoStagedSource(t, dir)
}

// TestLiveTerminalRuntimeTransportScrubsResolvedSource exercises the loader's
// real dynamic-local transport boundary. Each payload includes the same secret
// fixture, so a leaked REPLY, scalar bookkeeping slot, or obsolete generated
// function is observable even after a failure path returns control to the
// interactive shell. The one retained active reverse is deliberately allowed
// while a profile is active, then verified removed by switch/deactivate.
func TestLiveTerminalRuntimeTransportScrubsResolvedSource(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:secret)
    printf '%s\n' \
      "__zp_deactivate_secret() { unset ZP_TRANSPORT_SECRET; }" \
      "__zp_apply_secret() { export ZP_TRANSPORT_SECRET='phase5-runtime-transport-secret'; }" \
      "_zp_run_payload __zp_apply_secret __zp_deactivate_secret"
    ;;
  emit:apply:next)
    printf '%s\n' \
      "__zp_deactivate_next() { unset ZP_TRANSPORT_NEXT; }" \
      "__zp_apply_next() { export ZP_TRANSPORT_NEXT=ok; }" \
      "_zp_run_payload __zp_apply_next __zp_deactivate_next"
    ;;
  emit:apply:partial)
    printf '%s\n' \
      "__zp_deactivate_partial() { unset ZP_TRANSPORT_SECRET; }" \
      "__zp_apply_partial() { export ZP_TRANSPORT_SECRET='phase5-runtime-transport-secret'; return 9; }" \
      "_zp_run_payload __zp_apply_partial __zp_deactivate_partial"
    ;;
  emit:apply:invalid)
    printf '%s\n' 'if then # phase5-runtime-transport-secret'
    ;;
  emit:apply:failed)
    exit 7
    ;;
  *) exit 64 ;;
esac
`)

	const body = `
source "$1"

assert_transport_scrubbed() {
  local secret="$1" allow_secret_value="$2" parameter_name function_name
  [[ -z "${REPLY+x}" ]] || return 70
  for parameter_name in ${(k)parameters}; do
    # The scan runs inside this helper, so do not mistake its own local
    # arguments for leaked process-global state.
    [[ "${parameters[$parameter_name]}" == *-local* ]] && continue
    case "$parameter_name" in
      [A-Za-z_]*) ;;
      *) continue ;;
    esac
    # zsh retains the complete -c script in this intrinsic diagnostic
    # parameter; it is not loader-owned state and necessarily contains the
    # fixture embedded by this regression probe.
    case "$parameter_name" in
      ZSH_EXECUTION_STRING|argv) continue ;;
    esac
    case "$parameter_name" in
      ZP_APPLIED_SCALAR_*) return 71 ;;
      ZP_TRANSPORT_SECRET) [[ "$allow_secret_value" == 1 ]] && continue ;;
    esac
    if [[ "${(P)parameter_name}" == *"$secret"* ]]; then
      print -u2 -- "transport residue parameter: $parameter_name"
      return 72
    fi
  done
  for function_name in ${(k)functions}; do
    if [[ "$function_name" == "${ZP_ACTIVE_REVERSE_FN-}" ]]; then continue; fi
    case "$function_name" in
      __zp_apply_*|__zp_deactivate_*) return 73 ;;
    esac
    if [[ "${functions[$function_name]}" == *"$secret"* ]]; then
      print -u2 -- "transport residue function: $function_name"
      return 74
    fi
  done
}

# Partial evaluation applies the secret before returning nonzero. Its reverse
# must run, and the local payload must disappear once activation returns.
REPLY='phase5-runtime-transport-secret'
activate partial
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ -z "${ZP_TRANSPORT_SECRET+x}" && -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 11
assert_transport_scrubbed 'phase5-runtime-transport-secret' 0 || exit $?

# A successful activation intentionally leaves the target value plus exactly
# one active reverse. Neither REPLY nor duplicate global applied-value slots
# may retain another copy of the resolved secret.
REPLY='phase5-runtime-transport-secret'
activate secret
[[ "$ZP_TRANSPORT_SECRET" == 'phase5-runtime-transport-secret' ]] || exit 20
[[ "$ZP_ACTIVE_PROFILE" == secret && "$ZSHPRO_PROFILE" == secret ]] || exit 21
[[ -n "${ZP_ACTIVE_REVERSE_FN-}" && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 22
[[ "${functions[$ZP_ACTIVE_REVERSE_FN]}" == *ZP_TRANSPORT_SECRET* ]] || exit 23
assert_transport_scrubbed 'phase5-runtime-transport-secret' 1 || exit $?

# Switching consumes secret's retained reverse and installs a different active
# reverse. The old resolved payload must no longer be reachable anywhere.
REPLY='phase5-runtime-transport-secret'
activate next
[[ -z "${ZP_TRANSPORT_SECRET+x}" && "$ZP_TRANSPORT_NEXT" == ok ]] || exit 30
[[ "$ZP_ACTIVE_PROFILE" == next && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 31
[[ "${functions[$ZP_ACTIVE_REVERSE_FN]}" != *phase5-runtime-transport-secret* ]] || exit 32
assert_transport_scrubbed 'phase5-runtime-transport-secret' 0 || exit $?

REPLY='phase5-runtime-transport-secret'
deactivate
[[ -z "${ZP_TRANSPORT_NEXT+x}" && -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 40
assert_transport_scrubbed 'phase5-runtime-transport-secret' 0 || exit $?

# Preflight failures must scrub stale REPLY too. Invalid source contains the
# fixture and therefore covers the parser boundary separately from a nonzero
# emitter result.
REPLY='phase5-runtime-transport-secret'
activate failed
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 50
assert_transport_scrubbed 'phase5-runtime-transport-secret' 0 || exit $?

REPLY='phase5-runtime-transport-secret'
activate invalid
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 60
assert_transport_scrubbed 'phase5-runtime-transport-secret' 0 || exit $?
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-runtime-transport-scrub-test", loader)
	cmd.Env = liveEnv(dir, "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime transport retained resolved source outside active state: %v\n%s", err, out)
	}
}

func TestLiveTerminalRuntimeTransportIgnoresSharedTMPDIRRace(t *testing.T) {
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
	attackerSeen := filepath.Join(dir, "attacker-seen")
	attackerStop := filepath.Join(dir, "attacker-stop")
	writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B) printf '%s\n' '__zp_deactivate_B() { unset ZP_SHARED_TMP; }' '__zp_apply_B() { export ZP_SHARED_TMP=ok; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B' ;;
  *) exit 64 ;;
esac
`)

	const body = `
source "$1"
attacker() {
  local candidate
  while [[ ! -e "$ZP_ATTACKER_STOP" ]]; do
    for candidate in "$TMPDIR"/zsh-pro-eval-*(N) "$TMPDIR"/.runtime-*(N); do
      [[ -e "$candidate" || -L "$candidate" ]] || continue
      builtin print -r -- seen > "$ZP_ATTACKER_SEEN"
    done
    command sleep 0.01
  done
}
attacker &
attacker_pid=$!
activate B
: > "$ZP_ATTACKER_STOP"
if wait "$attacker_pid"; then :; else :; fi
[[ "$ZP_SHARED_TMP" == ok ]] || exit 50
[[ ! -e "$ZP_ATTACKER_SEEN" ]] || exit 51
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = liveEnv(dir,
		"PATH="+dir+":"+os.Getenv("PATH"),
		"TMPDIR="+shared,
		"ZP_ATTACKER_SEEN="+attackerSeen,
		"ZP_ATTACKER_STOP="+attackerStop,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shared TMPDIR race reached runtime transport: %v\n%s", err, out)
	}
	assertEmptyDir(t, shared)
	assertPrivateRuntimeClean(t, liveRuntimeDir(dir))
}

func TestLiveTerminalRuntimeHelperRejectsNonStickyWritableAncestor(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	runtimeHelper := buildLiveRuntimeHelper(t, dir)
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
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	emitSeen := filepath.Join(dir, "emit-seen")
	emitter := filepath.Join(binDir, "zsh-pro")
	if err := os.WriteFile(emitter, []byte(`#!/bin/sh
: > "$ZP_EMIT_SEEN"
printf '%s\n' '__zp_deactivate_B() { unset ZP_B_ENV; }' '__zp_apply_B() { export ZP_B_ENV=1; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B'
`), 0o700); err != nil {
		t.Fatal(err)
	}

	const body = `
source "$1"
zsh-pro() {
  if [[ "$1" == runtime ]]; then command "$ZP_RUNTIME_HELPER" "$@"
  else command "$ZP_EMITTER_SHIM" "$@"; fi
}
typeset -g ZP_BASE_PATH="$PATH"
__zp_reverse_A() { unset ZP_A_ENV; }
__zp_apply_A() { export ZP_A_ENV=present; }
_zp_run_payload __zp_apply_A __zp_reverse_A || exit 10
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
before_reverse="$ZP_ACTIVE_REVERSE_FN"
activate B
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 20
[[ ! -e "$ZP_EMIT_SEEN" ]] || exit 21
[[ "$ZP_A_ENV" == present && "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 22
[[ "$ZP_ACTIVE_REVERSE_FN" == "$before_reverse" && ${+functions[$before_reverse]} == 1 ]] || exit 23
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-unsafe-ancestor-test", loader)
	cmd.Env = liveEnvAt(runtimeRoot,
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"ZP_RUNTIME_HELPER="+runtimeHelper,
		"ZP_EMITTER_SHIM="+emitter,
		"ZP_EMIT_SEEN="+emitSeen,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("non-sticky writable ancestor reached the emitter or changed A: %v\n%s", err, out)
	}
	assertEmptyDir(t, runtimeRoot)
}

// The link sits directly in sticky /tmp and is owned by this test process,
// which models an attacker who may replace their own link even though the
// directory itself is sticky. The concurrent loop swaps it continuously while
// the real helper walks the root. A pathname check followed by a later reopen
// can be redirected here; descriptor-relative O_NOFOLLOW traversal cannot.
func TestLiveTerminalRuntimeHelperRejectsStickySymlinkReplacementRace(t *testing.T) {
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
	placeholder, err := os.CreateTemp(stickyParent, "zsh-pro-sticky-race-")
	if err != nil {
		t.Fatal(err)
	}
	runtimeLink := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(runtimeLink); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(runtimeLink) })

	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	runtimeHelper := buildLiveRuntimeHelper(t, dir)
	victimRoot := filepath.Join(dir, "victim-runtime")
	if err := os.Mkdir(victimRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	attackerRoot := filepath.Join(dir, "attacker-runtime")
	if err := os.Mkdir(attackerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victimRoot, runtimeLink); err != nil {
		t.Fatal(err)
	}
	attackerSource := filepath.Join(dir, "attacker-source.zsh")
	if err := os.WriteFile(attackerSource, []byte(`__zp_deactivate_attacker() { unset ZP_ATTACKED; }
__zp_apply_attacker() { export ZP_ATTACKED=1; }
_zp_run_payload __zp_apply_attacker __zp_deactivate_attacker
`), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	emitSeen := filepath.Join(dir, "emit-seen")
	emitter := filepath.Join(binDir, "zsh-pro")
	if err := os.WriteFile(emitter, []byte(`#!/bin/sh
: > "$ZP_EMIT_SEEN"
printf '%s\n' '__zp_deactivate_B() { unset ZP_SECRET_LIKE; }' '__zp_apply_B() { export ZP_SECRET_LIKE=emitted-secret-like-payload; }' '_zp_run_payload __zp_apply_B __zp_deactivate_B'
`), 0o700); err != nil {
		t.Fatal(err)
	}
	attackerSink := filepath.Join(dir, "attacker-sink")
	attackerSwapped := filepath.Join(dir, "attacker-swapped")
	attackerStop := filepath.Join(dir, "attacker-stop")

	const body = `
source "$1"
zsh-pro() {
  if [[ "$1" == runtime ]]; then command "$ZP_RUNTIME_HELPER" "$@"
  else command "$ZP_EMITTER_SHIM" "$@"; fi
}
typeset -g ZP_BASE_PATH="$PATH"
__zp_reverse_A() { unset ZP_A_ENV; unalias zp_a_alias 2>/dev/null; unset -f zp_a_function; path=(${(@s/:/)ZP_BASE_PATH}); }
__zp_apply_A() { export ZP_A_ENV=present; alias zp_a_alias='print -r -- A'; functions[zp_a_function]='print -r -- A'; path=(/zp-a/bin $path); }
_zp_run_payload __zp_apply_A __zp_reverse_A || exit 10
typeset -g +x ZP_ACTIVE_PROFILE=A
export ZSHPRO_PROFILE=A
before_path="$PATH"
before_reverse="$ZP_ACTIVE_REVERSE_FN"
before_reverse_body="${functions[$before_reverse]}"
before_function_body="${functions[zp_a_function]}"
attacker() {
  local candidate
  while [[ ! -e "$ZP_ATTACKER_STOP" ]]; do
    command rm -f -- "$ZSHPRO_HOME"
    if command ln -s -- "$ZP_ATTACK_ROOT" "$ZSHPRO_HOME"; then
      builtin print -r -- swapped > "$ZP_ATTACKER_SWAPPED"
    fi
    for candidate in "$ZP_ATTACK_ROOT"/.runtime-*/zsh-pro-eval(N); do
      [[ -s "$candidate" ]] || continue
      command cp -- "$candidate" "$ZP_ATTACKER_SINK" 2>/dev/null || :
      command rm -f -- "$candidate"
      command ln -s -- "$ZP_ATTACK_SOURCE" "$candidate" 2>/dev/null || :
    done
    command rm -f -- "$ZSHPRO_HOME"
    command ln -s -- "$ZP_VICTIM_ROOT" "$ZSHPRO_HOME" 2>/dev/null || :
    command sleep 0.001
  done
  command rm -f -- "$ZSHPRO_HOME"
  command ln -s -- "$ZP_VICTIM_ROOT" "$ZSHPRO_HOME" 2>/dev/null || :
}
attacker &
attacker_pid=$!
typeset -i attempts=0
while [[ ! -e "$ZP_ATTACKER_SWAPPED" && attempts -lt 100 ]]; do
  command sleep 0.01
  (( attempts += 1 ))
done
[[ -e "$ZP_ATTACKER_SWAPPED" ]] || exit 20
attempts=0
while (( attempts < 32 )); do
  activate B
  [[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 21
  (( attempts += 1 ))
done
: > "$ZP_ATTACKER_STOP"
if wait "$attacker_pid"; then :; else :; fi
[[ ! -e "$ZP_EMIT_SEEN" ]] || exit 22
[[ ! -s "$ZP_ATTACKER_SINK" ]] || exit 23
[[ -z "${ZP_ATTACKED+x}" && -z "${ZP_SECRET_LIKE+x}" ]] || exit 24
[[ "$ZP_A_ENV" == present && "$PATH" == "$before_path" ]] || exit 25
alias zp_a_alias >/dev/null || exit 26
[[ "${functions[zp_a_function]}" == "$before_function_body" ]] || exit 27
[[ "$ZP_ACTIVE_PROFILE" == A && "$ZSHPRO_PROFILE" == A ]] || exit 28
[[ "$ZP_ACTIVE_REVERSE_FN" == "$before_reverse" ]] || exit 29
[[ ${+functions[$before_reverse]} == 1 && "${functions[$before_reverse]}" == "$before_reverse_body" ]] || exit 30
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-sticky-symlink-race-test", loader)
	cmd.Env = liveEnvAt(runtimeLink,
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"ZP_RUNTIME_HELPER="+runtimeHelper,
		"ZP_EMITTER_SHIM="+emitter,
		"ZP_EMIT_SEEN="+emitSeen,
		"ZP_ATTACK_ROOT="+attackerRoot,
		"ZP_ATTACK_SOURCE="+attackerSource,
		"ZP_ATTACKER_SINK="+attackerSink,
		"ZP_ATTACKER_SWAPPED="+attackerSwapped,
		"ZP_ATTACKER_STOP="+attackerStop,
		"ZP_VICTIM_ROOT="+victimRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sticky symlink race reached emitted source or changed active A: %v\n%s", err, out)
	}
	assertEmptyDir(t, attackerRoot)
	assertEmptyDir(t, victimRoot)
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

// buildLiveRuntimeHelper gives the native-zsh security probes the actual
// descriptor-walking transport binary. The zsh test defines a function that
// routes only `runtime` calls here; the helper's child `zsh-pro emit` lookup
// still resolves the fixture executable on PATH.
func buildLiveRuntimeHelper(t *testing.T, dir string) string {
	t.Helper()
	repoRoot := liveTestRepositoryRoot(t)
	helper := filepath.Join(dir, "zsh-pro-runtime-helper")
	cmd := exec.Command("go", "build", "-o", helper, "./core/cmd/zsh-pro")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build production runtime helper: %v\n%s", err, out)
	}
	return helper
}

func liveTestRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository go.mod")
		}
		dir = parent
	}
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
	if filepath.Base(path) == "zsh-pro" {
		source = wrapRuntimeShim(source)
	}
	if err := os.WriteFile(path, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
}

// wrapRuntimeShim makes existing emit/list fixtures exercise the same loader
// transport shape as the production binary. The production helper owns the
// timeout; the fallback below keeps the fixtures bounded on platforms without
// GNU timeout as well.
func wrapRuntimeShim(source string) string {
	return `#!/bin/sh
run_bounded() {
  runtime_timeout="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$runtime_timeout" "$@"
    return $?
  fi
  "$@" &
  runtime_child=$!
  (
    sleep "$runtime_timeout"
    if kill -0 "$runtime_child" 2>/dev/null; then
      kill -TERM "$runtime_child" 2>/dev/null || :
      exit 124
    fi
  ) &
  runtime_watchdog=$!
  wait "$runtime_child"
  runtime_child_rc=$?
  if kill -0 "$runtime_watchdog" 2>/dev/null; then
    kill -TERM "$runtime_watchdog" 2>/dev/null || :
  fi
  wait "$runtime_watchdog"
  runtime_watchdog_rc=$?
  if [ "$runtime_watchdog_rc" -eq 124 ]; then return 124; fi
  return "$runtime_child_rc"
}
if [ "$1" = runtime ] && [ "$2" = capture ]; then
  timeout="$3"
  if [ -n "${ZP_RUNTIME_SHIM_CAPTURE_FAILURE-}" ]; then
    exit "$ZP_RUNTIME_SHIM_CAPTURE_FAILURE"
  fi
  shift 4
  run_bounded "$timeout" "$@"
  exit $?
fi
if [ "$1" = runtime ] && [ "$2" = validate ]; then
  timeout="$3"
  run_bounded "$timeout" zsh -n
  exit $?
fi
` + source
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
