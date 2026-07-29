package zsh

// loaderScript is sourced into an interactive zsh. It deliberately avoids
// emulate -L/LOCAL_OPTIONS: state changes made by emitted apply/deactivate
// blocks must persist in the caller's terminal after these functions return.
const loaderScript = `
typeset -g ZP_LAST_RUNTIME_STATUS=0
typeset -g ZP_LAST_RUNTIME_ERROR=''
typeset -g ZP_RUNTIME_TIMED_OUT=0

zp_capture_env() {
  local name="$1" safe slot present_slot
  safe="${name//[^A-Za-z0-9_]/_}"
  slot="__ZP_ORIG_${safe}"
  present_slot="__ZP_ORIG_${safe}_PRESENT"
  if (( ${+parameters[$slot]} )); then
    # A loader refreshed during an old activation lacks presence metadata.
    # Preserve that retained value as data rather than treating any string as
    # an encoded unset value.
    (( ${+parameters[$present_slot]} )) || typeset -g "$present_slot=1"
    return 0
  fi
  if [[ "${(P)+name}" == "1" ]]; then
    typeset -g "$slot=${(P)name}" "$present_slot=1"
  else
    typeset -g "$slot=" "$present_slot=0"
  fi
}

zp_restore_env() {
  local name="$1" applied="$2" safe slot present_slot prior was_set
  safe="${name//[^A-Za-z0-9_]/_}"
  slot="__ZP_ORIG_${safe}"
  present_slot="__ZP_ORIG_${safe}_PRESENT"
  (( ${+parameters[$slot]} && ${+parameters[$present_slot]} )) || return 0
  prior="${(P)slot}"
  was_set="${(P)present_slot}"
  if [[ "${(P)+name}" == "1" && "${(P)name}" == "$applied" ]]; then
    case "$was_set" in
      1) export "$name=$prior" ;;
      0) unset "$name" ;;
    esac
  fi
  unset "$slot" "$present_slot"
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

# _zp_private_root resolves the installer-managed cache only when it is an
# absolute, owner-controlled directory. Explicit runtime verbs may repair an
# owner-owned mode, but sourcing this loader never touches the filesystem.
_zp_private_root() {
  local root=''
  if [[ "${ZSHPRO_HOME+x}" == x ]]; then
    [[ -n "$ZSHPRO_HOME" ]] || return 1
    root="$ZSHPRO_HOME"
  else
    [[ -n "${HOME-}" ]] || return 1
    root="$HOME/.zsh-pro"
  fi
  [[ "$root" == /* && -d "$root" && ! -L "$root" && -O "$root" ]] || return 1
  if command chmod 700 -- "$root" >/dev/null 2>&1; then :; else return 1; fi
  [[ -d "$root" && ! -L "$root" && -O "$root" ]] || return 1
  REPLY="$root"
  return 0
}

# _zp_private_temp creates one owner-controlled mode-0700 child for a single
# staging file. The child is removed with the file, so later command output or
# emitted source is never reopened through an untrusted directory.
_zp_private_temp() {
  local stem="$1" root='' stage='' candidate='' attempt=0
  stem="${stem//[^A-Za-z0-9_-]/_}"
  [[ -n "$stem" ]] || stem=zsh-pro-runtime
  if ! _zp_private_root; then return 1; fi
  root="$REPLY"
  while (( attempt < 32 )); do
    stage="$root/.runtime-${$}-${RANDOM}"
    if ( umask 077; command mkdir -m 700 -- "$stage" ) 2>/dev/null; then
      candidate="$stage/$stem"
      if ( umask 077; set -C; : > "$candidate" ) 2>/dev/null &&
        [[ -f "$candidate" && ! -L "$candidate" && -O "$candidate" ]] &&
        command chmod 600 -- "$candidate" >/dev/null 2>&1; then
        REPLY="$candidate"
        return 0
      fi
      if command rm -f -- "$candidate" >/dev/null 2>&1; then :; fi
      if command rmdir -- "$stage" >/dev/null 2>&1; then :; fi
    fi
    (( attempt += 1 ))
  done
  return 1
}

_zp_cleanup_private_temp() {
  local tmp="$1" stage=''
  [[ -n "$tmp" ]] || return 0
  stage="${tmp:h}"
  if [[ -e "$tmp" || -L "$tmp" ]]; then
    if command rm -f -- "$tmp" >/dev/null 2>&1; then :; fi
  fi
  if [[ -d "$stage" && ! -L "$stage" && -O "$stage" ]]; then
    if command rmdir -- "$stage" >/dev/null 2>&1; then :; fi
  fi
  return 0
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

      if [[ -r "$output" ]]; then
        REPLY="$(<"$output"; print -rn -- $'\001')"
        REPLY="${REPLY%$'\001'}"
      else
        REPLY=''
      fi
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
      _zp_cleanup_private_temp "$output"
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
      _zp_cleanup_private_temp "$tmp"
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
  if [[ "${ZP_ACTIVE_PROFILE+x}" == x && "$ZP_ACTIVE_PROFILE" == "$name" ]]; then
    if export ZSHPRO_PROFILE="$name"; then return 0; fi
    _zp_runtime_error 1 "unable to record active profile"
    return 1
  fi
  if ! _zp_prepare_eval_state; then return 1; fi
  # The apply emitter returns the requested target only: its retained reverse
  # is defined before zp_apply runs so the caller can later unwind this target.
  if ! _zp_emit apply "$name"; then return 1; fi
  block="$REPLY"
  if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then
    # Execute the active target's retained reverse before this new payload can
    # redefine zp_deactivate or apply a new target's state.
    block=$'zp_deactivate\n'"$block"
  fi
  if ! _zp_eval_block "$block" "$name"; then return 1; fi
  if ! typeset -g +x ZP_ACTIVE_PROFILE="$name"; then
    _zp_runtime_error 1 "unable to record active profile"
    return 1
  fi
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
	local block
	if [[ "${ZP_ACTIVE_PROFILE+x}" != x ]]; then _zp_runtime_ok; return 0; fi
	if ! _zp_prepare_eval_state; then return 0; fi
	block='zp_deactivate'
	if ! _zp_eval_block "$block" ""; then return 0; fi
	if unset ZP_ACTIVE_PROFILE && unset ZSHPRO_PROFILE; then _zp_runtime_ok; else _zp_runtime_error 1 "unable to clear active profile"; fi
	return 0
}

list() {
  local rc timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}"
  if _zp_run_bounded "$timeout" zsh-pro list; then
    print -rn -- "$REPLY"
    _zp_runtime_ok
  else
    rc=$?
    if (( ZP_RUNTIME_TIMED_OUT )); then
      _zp_runtime_error "$rc" "list timed out after ${timeout}s"
    else
      _zp_runtime_error "$rc" "list failed"
    fi
  fi
  return 0
}

status() {
  if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then
    print -r -- "$ZP_ACTIVE_PROFILE"
  else
    print -r -- main
  fi
}
`

// HookScript returns the complete sourced runtime loader.
func (Provider) HookScript() string { return loaderScript }
