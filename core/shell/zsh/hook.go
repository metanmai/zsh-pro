package zsh

// loaderScript is sourced into an interactive zsh. It deliberately avoids
// emulate -L/LOCAL_OPTIONS: state changes made by emitted apply/deactivate
// blocks must persist in the caller's terminal after these functions return.
const loaderScript = `
typeset -g ZP_UNSET_SENTINEL='__zsh_pro_unset_7c5a0a15__'
typeset -g ZP_LAST_RUNTIME_STATUS=0
typeset -g ZP_LAST_RUNTIME_ERROR=''
typeset -g ZP_RUNTIME_TIMED_OUT=0

zp_capture_env() {
  local name="$1" safe slot
  safe="${name//[^A-Za-z0-9_]/_}"
  slot="__ZP_ORIG_${safe}"
  (( ${+parameters[$slot]} )) && return 0
  if [[ "${(P)+name}" == "1" ]]; then
    typeset -g "$slot=${(P)name}"
  else
    typeset -g "$slot=$ZP_UNSET_SENTINEL"
  fi
}

zp_restore_env() {
  local name="$1" applied="$2" safe slot prior
  safe="${name//[^A-Za-z0-9_]/_}"
  slot="__ZP_ORIG_${safe}"
  (( ${+parameters[$slot]} )) || return 0
  prior="${(P)slot}"
  if [[ "${(P)+name}" == "1" && "${(P)name}" == "$applied" ]]; then
    if [[ "$prior" == "$ZP_UNSET_SENTINEL" ]]; then
      unset "$name"
    else
      export "$name=$prior"
    fi
  fi
  unset "$slot"
}

_zp_prepare_eval_state() {
  # A child shell can inherit the active profile name but not our runtime undo
  # slots. Treat that asymmetric state as a fresh terminal.
  if [[ "${ZP_BASE_PATH+x}" == "x" ]]; then
    return 0
  fi
  if typeset -g ZP_BASE_PATH="$PATH"; then
    return 0
  fi
  _zp_runtime_error 1 "unable to capture the terminal base PATH; shell state unchanged"
  return 1
}

_zp_runtime_error() {
  local exit_status="$1" message="$2"
  [[ -n "$exit_status" && "$exit_status" != 0 ]] || exit_status=1
  if typeset -g ZP_LAST_RUNTIME_STATUS="$exit_status" ZP_LAST_RUNTIME_ERROR="$message"; then :; fi
  if print -u2 -- "zsh-pro: $message"; then :; fi
  return 0
}

_zp_runtime_ok() {
  if typeset -g ZP_LAST_RUNTIME_STATUS=0 ZP_LAST_RUNTIME_ERROR='' ZP_RUNTIME_TIMED_OUT=0; then :; fi
  return 0
}

# _zp_private_temp creates a 0600 regular file with noclobber enabled in a
# subshell. That gives staging and captured stdout exclusive ownership without
# requiring GNU timeout or a platform-specific mktemp flag.
_zp_private_temp() {
  local stem="$1" base candidate attempt=0
  base="${TMPDIR:-/tmp}"
  if [[ ! -d "$base" || ! -w "$base" ]]; then
    return 1
  fi
  base="${base%/}"
  while (( attempt < 32 )); do
    candidate="$base/${stem}-${RANDOM}-$$"
    if ( umask 077; set -C; : > "$candidate" ) 2>/dev/null; then
      REPLY="$candidate"
      return 0
    fi
    (( attempt += 1 ))
  done
  return 1
}

# _zp_run_bounded captures a command's stdout while a paired watchdog owns its
# finite deadline. It always reaps the command and watchdog and removes its
# private capture file. The caller owns the user-facing diagnostic because it
# knows whether the command was an emitter or a syntax validator.
_zp_run_bounded() {
  local timeout="$1" output='' child='' watchdog='' child_rc=1 watchdog_rc=0 result=1
  shift
  case "$timeout" in
    [1-9]|[1-9][0-9]) ;;
    *) timeout=5 ;;
  esac
  REPLY=''
  if typeset -g ZP_RUNTIME_TIMED_OUT=0; then :; fi

  {
    if ! _zp_private_temp zsh-pro-run; then
      result=1
    else
      output="$REPLY"
      ( exec "$@" > "$output" ) &
      child="$!"
      (
        local timer=''
        trap 'if [[ -n "$timer" ]]; then command kill -TERM "$timer" 2>/dev/null || :; if wait "$timer"; then :; else :; fi; fi; exit 0' TERM
        command sleep "$timeout" &
        timer="$!"
        if ! wait "$timer"; then exit 0; fi
        if command kill -0 "$child" 2>/dev/null; then
          command kill -TERM "$child" 2>/dev/null || :
          command kill -KILL "$child" 2>/dev/null || :
          exit 124
        fi
        exit 0
      ) &
      watchdog="$!"

      if wait "$child"; then child_rc=0; else child_rc=$?; fi
      child=''
      if command kill -0 "$watchdog" 2>/dev/null; then
        command kill -TERM "$watchdog" 2>/dev/null || :
      fi
      if wait "$watchdog"; then watchdog_rc=0; else watchdog_rc=$?; fi
      watchdog=''

      if [[ -r "$output" ]]; then REPLY="$(<"$output")"; else REPLY=''; fi
      if (( watchdog_rc == 124 )); then
        if typeset -g ZP_RUNTIME_TIMED_OUT=1; then :; fi
        result=124
      else
        result="$child_rc"
      fi
    fi
  } always {
    if [[ -n "$watchdog" ]]; then
      if command kill -TERM "$watchdog" 2>/dev/null; then :; fi
      if wait "$watchdog"; then :; else :; fi
    fi
    if [[ -n "$child" ]]; then
      if command kill -TERM "$child" 2>/dev/null; then :; fi
      if command kill -KILL "$child" 2>/dev/null; then :; fi
      if wait "$child"; then :; else :; fi
    fi
    if [[ -n "$output" ]]; then
      if command rm -f -- "$output" >/dev/null 2>&1; then :; fi
    fi
  }
  return "$result"
}

_zp_emit() {
  local mode="$1" name="$2" code rc timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}"
  if _zp_run_bounded "$timeout" zsh-pro emit "$mode" "$name"; then
    code="$REPLY"
  else
    rc=$?
    if (( ZP_RUNTIME_TIMED_OUT )); then
      _zp_runtime_error "$rc" "emit $mode timed out after ${timeout}s; shell state unchanged"
    else
      _zp_runtime_error "$rc" "emit $mode failed; shell state unchanged"
    fi
    return 1
  fi
  if [[ -z "$code" ]]; then
    _zp_runtime_error 1 "emit $mode produced empty source; shell state unchanged"
    return 1
  fi
  REPLY="$code"
  return 0
}

_zp_eval_block() {
  local block="$1" applied="$2" tmp='' rc=0 validator_rc=1 timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}"
  local history_pushed=0 xtrace_was_on=0
  if [[ -z "$block" ]]; then
    _zp_runtime_error 1 "emitted empty shell source; shell state unchanged"
    return 1
  fi

  {
    if [[ -o xtrace ]]; then xtrace_was_on=1; fi
    if setopt NOXTRACE 2>/dev/null; then :; else
      _zp_runtime_error 1 "unable to protect emitted source from xtrace; shell state unchanged"
      rc=1
    fi
    if (( rc == 0 )); then
      if _zp_private_temp zsh-pro-eval; then tmp="$REPLY"; else
        _zp_runtime_error 1 "unable to stage emitted shell source; shell state unchanged"
        rc=1
      fi
    fi
    if (( rc == 0 )); then
      if ( umask 077; print -rn -- "$block" > "$tmp" ); then :; else
        _zp_runtime_error 1 "unable to stage emitted shell source; shell state unchanged"
        rc=1
      fi
    fi
    if (( rc == 0 )); then
      if _zp_run_bounded "$timeout" zsh -n "$tmp"; then :; else
        validator_rc=$?
        if (( ZP_RUNTIME_TIMED_OUT )); then
          _zp_runtime_error "$validator_rc" "emitted shell source validation timed out after ${timeout}s; shell state unchanged"
        else
          _zp_runtime_error "$validator_rc" "emitted shell source failed validation; shell state unchanged"
        fi
        rc=1
      fi
    fi
    if (( rc == 0 )); then
      if fc -p 2>/dev/null; then history_pushed=1; else
        _zp_runtime_error 1 "unable to protect shell history during switch; shell state unchanged"
        rc=1
      fi
    fi
    if (( rc == 0 )); then
      if eval "$block"; then :; else
        rc=$?
        _zp_runtime_error "$rc" "switch failed at runtime; shell may be partially changed; run checkout ${ZP_LAST_GOOD_PROFILE:-main}"
      fi
    fi
    if (( rc == 0 )) && [[ -n "$applied" ]]; then
      if typeset -g ZP_LAST_GOOD_PROFILE="$applied"; then :; else
        _zp_runtime_error 1 "unable to record last good profile; shell state may be partially changed"
        rc=1
      fi
    fi
  } always {
    if [[ -n "$tmp" ]]; then
      if command rm -f -- "$tmp" >/dev/null 2>&1; then :; fi
    fi
    if (( history_pushed )); then
      if fc -P 2>/dev/null; then :; fi
    fi
    if (( xtrace_was_on )); then
      if setopt XTRACE 2>/dev/null; then :; fi
    else
      if setopt NOXTRACE 2>/dev/null; then :; fi
    fi
  }
  return "$rc"
}

_zp_switch() {
  local name="$1" block
  if ! _zp_prepare_eval_state; then return 1; fi
  # The apply emitter validates the target branch and returns one complete,
  # executable transaction: deactivate-prior (when needed), then apply-target.
  if ! _zp_emit apply "$name"; then return 1; fi
  block="$REPLY"
  if ! _zp_eval_block "$block" "$name"; then return 1; fi
  if export ZSHPRO_PROFILE="$name"; then return 0; fi
  _zp_runtime_error 1 "unable to record active profile"
  return 1
}

activate() {
	local name="$1"
	if [[ -z "$name" ]]; then
		_zp_runtime_error 2 "usage: activate <profile>"
		return 0
	fi
	if _zp_switch "$name"; then _zp_runtime_ok; fi
	return 0
}

checkout() {
  local name="$1"
  if [[ -z "$name" ]]; then
    _zp_runtime_error 2 "usage: checkout <profile>"
    return 0
  fi
  if _zp_switch "$name"; then _zp_runtime_ok; fi
  return 0
}

deactivate() {
	local name="${ZSHPRO_PROFILE-}" block
	if [[ -z "$name" ]]; then _zp_runtime_ok; return 0; fi
	if ! _zp_prepare_eval_state; then return 0; fi
	if ! _zp_emit deactivate "$name"; then return 0; fi
	block="$REPLY"
	if ! _zp_eval_block "$block" ""; then return 0; fi
	if unset ZSHPRO_PROFILE; then _zp_runtime_ok; else _zp_runtime_error 1 "unable to clear active profile"; fi
	return 0
}

list() {
  command zsh-pro list
}

status() {
  if [[ -n "${ZSHPRO_PROFILE-}" ]]; then
    print -r -- "$ZSHPRO_PROFILE"
  else
    print -r -- main
  fi
}
`

// HookScript returns the complete sourced runtime loader.
func (Provider) HookScript() string { return loaderScript }
