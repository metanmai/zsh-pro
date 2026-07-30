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

# Scalar capture/restore are loader-owned support functions. Runtime payloads
# use these private names so emitted secret-bearing source does not define or
# replace user-visible generic helpers in the current terminal.
_zp_capture_scalar() {
  local var="$1" original_slot="$2" presence_slot="$3" export_slot="$4"
  if [[ "${(P)+presence_slot}" == 1 ]]; then return 0; fi
  if [[ "${(P)+var}" == 1 ]]; then
    typeset -g "$presence_slot=1"
    typeset -g "$original_slot=${(P)var}"
    if [[ "${parameters[$var]}" == *export* ]]; then typeset -g "$export_slot=1"
    else typeset -g "$export_slot=0"; fi
  else
    typeset -g "$presence_slot=0"
    typeset -g "$export_slot=0"
  fi
}

_zp_preflight_undo_slots() {
  local slot
  for slot in "$@"; do
    if [[ -n "$slot" && "${(P)+slot}" == 1 && "${parameters[$slot]}" == *readonly* ]]; then
      return 1
    fi
  done
  return 0
}

_zp_preflight_restore_scalar() {
  local var="$1" applied="$2" original_slot="$3" presence_slot="$4" export_slot="$5" applied_slot="${6-}"
  # Static runtime payloads keep their expected value in the retained reverse;
  # dynamic payloads pass the evaluated value through a short-lived slot.
  if [[ -n "$applied_slot" && "${(P)+applied_slot}" == 1 ]]; then applied="${(P)applied_slot}"; fi
  # Commit clears these slots even when a user has changed the target since
  # activation, so validate them before returning for drift.
  _zp_preflight_undo_slots "$original_slot" "$presence_slot" "$export_slot" "$applied_slot" || return $?
  if [[ "${(P)+var}" != 1 || "${(P)var}" != "$applied" ]]; then return 0; fi
  if [[ "${parameters[$var]}" == *readonly* ]]; then return 1; fi
  if [[ "${(P)+presence_slot}" != 1 ]]; then return 1; fi
  if [[ "${(P)presence_slot}" == 1 && ( "${(P)+original_slot}" != 1 || "${(P)+export_slot}" != 1 ) ]]; then
    return 1
  fi
  return 0
}

_zp_restore_scalar() {
  local var="$1" applied="$2" original_slot="$3" presence_slot="$4" export_slot="$5" applied_slot="${6-}" rc=0
  if [[ -n "$applied_slot" && "${(P)+applied_slot}" == 1 ]]; then applied="${(P)applied_slot}"; fi
  if [[ "${(P)+var}" != 1 || "${(P)var}" != "$applied" ]]; then return 0; fi
  if [[ "${(P)presence_slot}" == 1 ]]; then
    if typeset -g "$var=${(P)original_slot}"; then :; else return $?; fi
    if [[ "${(P)export_slot}" == 1 ]]; then export "$var"; else typeset +x "$var"; fi
    rc=$?
    if (( rc != 0 )); then return "$rc"; fi
  elif unset "$var"; then :; else return $?; fi
  return 0
}

_zp_commit_restore_scalar() {
  local original_slot="$1" presence_slot="$2" export_slot="$3" applied_slot="${4-}"
  if [[ -n "$applied_slot" ]]; then
    if unset "$original_slot" "$presence_slot" "$export_slot" "$applied_slot"; then return 0; else return $?; fi
  fi
  if unset "$original_slot" "$presence_slot" "$export_slot"; then return 0; else return $?; fi
}

_zp_commit_undo_slots() {
  if unset "$@"; then return 0; else return $?; fi
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

# _zp_run_bounded delegates command capture to the binary helper. The helper
# keeps stdout on process-owned pipes and, before an emit, traverses the
# configured runtime root by descriptor with O_NOFOLLOW. This loader therefore
# never creates, reads, writes, chmods, or reopens a staging pathname itself.
_zp_run_bounded() {
  local timeout="$1" result_name="$2" captured='' rc=1
  shift 2
  # zsh local parameters are dynamically scoped. Assign through the caller's
  # local result slot instead of the process-global REPLY, which would retain
  # emitted source (including a resolved secret) after this function returns.
  : ${(P)result_name::=}
  {
    case "$timeout" in
      [1-9]|[1-9][0-9]) ;;
      *) timeout=5 ;;
    esac
    if typeset -g ZP_RUNTIME_TIMED_OUT=0; then :; fi
    if captured="$(zsh-pro runtime capture "$timeout" -- "$@")"; then
      # Command substitution removes terminal newlines. Emit blocks remain
      # parseable without one; list retains its ordinary line-oriented output.
      [[ -z "$captured" ]] || captured+=$'\n'
      if : ${(P)result_name::=$captured}; then rc=0; else rc=1; fi
    else
      rc=$?
    fi
    if (( rc == 124 )); then
      if typeset -g ZP_RUNTIME_TIMED_OUT=1; then :; fi
    fi
  } always {
    captured=''
    unset REPLY
  }
  return "$rc"
}

_zp_emit() {
  local mode="$1" name="$2" result_name="$3" emitted='' rc=1 timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}"
  : ${(P)result_name::=}
  {
    if _zp_run_bounded "$timeout" emitted zsh-pro emit "$mode" "$name"; then
      if [[ -z "$emitted" ]]; then
        _zp_runtime_error 1 "emit $mode produced empty source; shell state unchanged"
      elif : ${(P)result_name::=$emitted}; then
        rc=0
      else
        _zp_runtime_error 1 "unable to retain emitted source; shell state unchanged"
      fi
    else
      rc=$?
      if (( ZP_RUNTIME_TIMED_OUT )); then
        _zp_runtime_error "$rc" "emit $mode timed out after ${timeout}s; shell state unchanged"
      else
        _zp_runtime_error "$rc" "emit $mode failed; shell state unchanged"
      fi
    fi
  } always {
    emitted=''
    unset REPLY
  }
  return "$rc"
}

_zp_validate_block() {
  local block="$1" rc=0 validator_rc=1 timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}"
  local xtrace_was_on=0
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
      # The validator receives the exact in-memory payload via stdin. It does
      # not reopen a file below ZSHPRO_HOME, so a root replacement after the
      # helper's descriptor check cannot substitute or disclose source.
      if print -rn -- "$block" | zsh-pro runtime validate "$timeout"; then :; else
        validator_rc=$?
        if (( validator_rc == 124 )); then
          if typeset -g ZP_RUNTIME_TIMED_OUT=1; then :; fi
          _zp_runtime_error "$validator_rc" "emitted shell source validation timed out after ${timeout}s; shell state unchanged"
        else
          _zp_runtime_error "$validator_rc" "emitted shell source failed validation; shell state unchanged"
        fi
        rc=1
      fi
    fi
  } always {
    if (( xtrace_was_on )); then
      if setopt XTRACE 2>/dev/null; then :; fi
    else
      if setopt NOXTRACE 2>/dev/null; then :; fi
    fi
  }
  return "$rc"
}

_zp_eval_block() {
  local block="$1" rc=0 history_pushed=0 xtrace_was_on=0
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
      if fc -p 2>/dev/null; then history_pushed=1; else
        _zp_runtime_error 1 "unable to protect shell history during switch; shell state unchanged"
        rc=1
      fi
    fi
    if (( rc == 0 )); then
      if eval "$block"; then :; else
        rc=$?
        if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x ]]; then
          _zp_runtime_error "$rc" "target apply cleanup failed; recovery is retained; run deactivate before another activation"
        else
          _zp_runtime_error "$rc" "switch failed at runtime; shell may be partially changed; run checkout ${ZP_LAST_GOOD_PROFILE:-main}"
        fi
      fi
    fi
  } always {
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

_zp_run_transient_reverse_payload() {
  local reverse="$1" rc=1
  if [[ -z "$reverse" || ${+functions[$reverse]} != 1 ]]; then return 1; fi
  if "$reverse"; then
    if unset -f "$reverse" 2>/dev/null; then return 0; else return $?; fi
  else
    rc=$?
  fi
  return "$rc"
}

_zp_run_retained_reverse() {
  local reverse="$1" rc=1
  if _zp_run_transient_reverse_payload "$reverse"; then
    if [[ "${ZP_ACTIVE_REVERSE_FN-}" == "$reverse" ]]; then
      if unset ZP_ACTIVE_REVERSE_FN; then return 0; else return $?; fi
    fi
    return 0
  else
    rc=$?
  fi
  return "$rc"
}

_zp_recover_failed_target() {
  local reverse="${ZP_RECOVERY_REVERSE_FN-}" rc=1
  if [[ -z "$reverse" || ${+functions[$reverse]} != 1 ]]; then return 1; fi
  # Do not run a successful reverse unless its recovery marker can be removed
  # immediately afterwards. Otherwise a consumed function could leave a stale
  # pointer that prevents a future cleanup retry.
  if [[ "${ZP_ACTIVE_REVERSE_FN-}" == "$reverse" ]]; then
    if ! _zp_preflight_undo_slots ZP_ACTIVE_REVERSE_FN ZP_RECOVERY_REVERSE_FN; then return 1; fi
  elif ! _zp_preflight_undo_slots ZP_RECOVERY_REVERSE_FN; then
    return 1
  fi
  if _zp_run_transient_reverse_payload "$reverse"; then
    if [[ "${ZP_ACTIVE_REVERSE_FN-}" == "$reverse" ]]; then
      if unset ZP_ACTIVE_REVERSE_FN; then :; else return $?; fi
    fi
    if [[ "${ZP_RECOVERY_REVERSE_FN-}" == "$reverse" ]]; then
      if unset ZP_RECOVERY_REVERSE_FN; then return 0; else return $?; fi
    fi
    return 1
  else
    rc=$?
  fi
  return "$rc"
}

_zp_run_payload() {
  local apply="$1" reverse="$2" apply_rc=1 cleanup_rc=1
  if [[ -z "$apply" || -z "$reverse" || ${+functions[$apply]} != 1 || ${+functions[$reverse]} != 1 ]]; then
    unset -f "$apply" "$reverse" 2>/dev/null || :
    return 1
  fi
  # A failed target can leave a partially applied shell plus its secret-bearing
  # reverse. Refuse all new payloads until deactivate has successfully retried
  # that compensation; do not let a later profile layer on top of it.
  if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x ]]; then
    unset -f "$apply" "$reverse" 2>/dev/null || :
    return 1
  fi
  # Both retained pointers are loader-owned state. Validate their mutability
  # before apply changes the current terminal, so a readonly marker cannot
  # turn a failed compensation into an untracked reverse function.
  if ! _zp_preflight_undo_slots ZP_ACTIVE_REVERSE_FN ZP_RECOVERY_REVERSE_FN; then
    unset -f "$apply" "$reverse" 2>/dev/null || :
    return 1
  fi
  # Install the recovery pointer before invoking apply. Any later failure can
  # therefore retain the reverse even if the target changed the terminal first.
  if ! typeset -g ZP_RECOVERY_REVERSE_FN="$reverse"; then
    unset -f "$apply" "$reverse" 2>/dev/null || :
    return 1
  fi
  if "$apply"; then
    if typeset -g ZP_ACTIVE_REVERSE_FN="$reverse"; then
      if unset ZP_RECOVERY_REVERSE_FN; then
        unset -f "$apply" 2>/dev/null || :
        return 0
      fi
    fi
    apply_rc=1
  else
    apply_rc=$?
  fi
  # Apply may have changed part of the target before returning non-zero. Keep
  # its reverse under a dedicated recovery marker until compensation succeeds.
  # If compensation fails, the function (and any resolved secret in its source)
  # remains reachable for a later public deactivate retry.
  if _zp_recover_failed_target; then
    cleanup_rc=0
  else
    cleanup_rc=$?
  fi
  unset -f "$apply" 2>/dev/null || :
  if (( cleanup_rc != 0 )); then return "$cleanup_rc"; fi
  return "$apply_rc"
}

_zp_has_known_active_profile() {
  local name="$1" reverse="${ZP_ACTIVE_REVERSE_FN-}"
  [[ "${ZP_ACTIVE_PROFILE+x}" == x && "$ZP_ACTIVE_PROFILE" == "$name" &&
    "${ZSHPRO_PROFILE-}" == "$name" && -n "$reverse" && ${+functions[$reverse]} == 1 ]]
}

_zp_reverse_active_profile() {
  local reverse="${ZP_ACTIVE_REVERSE_FN-}" rc=1
  if [[ -z "$reverse" || ${+functions[$reverse]} != 1 ]]; then
    _zp_runtime_error 1 "active profile reverse is unavailable; shell state may be partially changed"
    return 1
  fi
  # A retained reverse only commits its own undo cleanup after every generated
  # operation succeeds. Check the three loader-owned commit markers before
  # invoking it as well, so a readonly marker cannot turn a successful reverse
  # into an unrecoverable half-commit.
  if ! _zp_preflight_undo_slots ZP_ACTIVE_PROFILE ZSHPRO_PROFILE ZP_ACTIVE_REVERSE_FN; then
    _zp_runtime_error 1 "active profile markers cannot be cleared; shell state unchanged"
    return 1
  fi
  if _zp_run_retained_reverse "$reverse"; then
    if unset ZP_ACTIVE_PROFILE ZSHPRO_PROFILE; then return 0; else
      _zp_runtime_error 1 "unable to clear active profile after reversal"
      return 1
    fi
  else
    rc=$?
  fi
  _zp_runtime_error "$rc" "active profile reversal failed; shell state may be partially changed"
  return "$rc"
}

_zp_switch() {
  local name="$1" block='' active=0
  {
    if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x ]]; then
      _zp_runtime_error 1 "target cleanup is pending; run deactivate before activating another profile"
      return 1
    fi
    if _zp_has_known_active_profile "$name"; then
      if export ZSHPRO_PROFILE="$name"; then return 0; fi
      _zp_runtime_error 1 "unable to record active profile"
      return 1
    fi
    if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then
      active=1
      # Every successfully applied profile captures this before its apply
      # payload runs. Refuse an inconsistent active state before preflight so a
      # later transition can never consume A and then discover it cannot record
      # its base PATH.
      if [[ "${ZP_BASE_PATH+x}" != x ]]; then
        _zp_runtime_error 1 "active profile state is incomplete; shell state unchanged"
        return 1
      fi
    fi

    # Target-side work is a pure preflight while the current profile is still
    # intact. In particular, do not clear A's markers or consume its retained
    # reverse until B was captured through the helper's private pipe and
    # syntax-validated.
    if ! _zp_emit apply "$name" block; then return 1; fi
    if ! _zp_validate_block "$block"; then return 1; fi

    if (( active )); then
      if ! _zp_reverse_active_profile; then return 1; fi
    elif [[ -n "${ZP_ACTIVE_REVERSE_FN-}" ]]; then
      # A stale retained reverse is not an active profile marker. Consume it
      # before a new payload so resolved values cannot survive an interrupted
      # marker update.
      if ! _zp_run_retained_reverse "$ZP_ACTIVE_REVERSE_FN"; then
        _zp_runtime_error 1 "stale profile reverse failed; shell state may be partially changed"
        return 1
      fi
    fi
    if ! _zp_prepare_eval_state; then return 1; fi
    if ! _zp_eval_block "$block"; then return 1; fi
    if ! typeset -g +x ZP_ACTIVE_PROFILE="$name"; then
      _zp_runtime_error 1 "unable to record active profile"
      if _zp_run_retained_reverse "${ZP_ACTIVE_REVERSE_FN-}"; then :; else :; fi
      return 1
    fi
    if ! export ZSHPRO_PROFILE="$name"; then
      _zp_runtime_error 1 "unable to record active profile"
      if _zp_reverse_active_profile; then :; else :; fi
      return 1
    fi
    if typeset -g ZP_LAST_GOOD_PROFILE="$name"; then return 0; fi
    _zp_runtime_error 1 "unable to record last good profile"
    if _zp_reverse_active_profile; then :; else :; fi
    return 1
  } always {
    block=''
    unset REPLY
  }
}

activate() {
	if (( $# != 1 )); then
		_zp_runtime_error 2 "usage: activate <profile>"
		return 0
	fi
	local name="$1"
	{
		if _zp_switch "$name"; then _zp_runtime_ok; fi
		return 0
	} always {
		unset REPLY
	}
}

checkout() {
	if (( $# != 1 )); then
		_zp_runtime_error 2 "usage: checkout <profile>"
		return 0
	fi
	local name="$1"
	{
		if _zp_switch "$name"; then _zp_runtime_ok; fi
		return 0
	} always {
    unset REPLY
  }
}

deactivate() {
	if (( $# != 0 )); then
		_zp_runtime_error 2 "usage: deactivate"
		return 0
	fi
	local rc=1
	{
		if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x ]]; then
			if _zp_recover_failed_target; then :; else
				rc=$?
				_zp_runtime_error "$rc" "target apply cleanup failed; recovery is retained; repair the terminal state and run deactivate again"
				return 0
			fi
		fi
		if [[ "${ZP_ACTIVE_PROFILE+x}" != x ]]; then _zp_runtime_ok; return 0; fi
		if ! _zp_prepare_eval_state; then return 0; fi
		if _zp_reverse_active_profile; then _zp_runtime_ok; fi
		return 0
	} always {
		unset REPLY
	}
}

list() {
	if (( $# != 0 )); then
		_zp_runtime_error 2 "usage: list"
		return 0
	fi
	local rc timeout="${ZP_RUNTIME_TIMEOUT_SECONDS:-5}" listing=''
  {
    if _zp_run_bounded "$timeout" listing zsh-pro list; then
      print -rn -- "$listing"
      _zp_runtime_ok
    else
      rc=$?
      if (( ZP_RUNTIME_TIMED_OUT )); then
        _zp_runtime_error "$rc" "list timed out after ${timeout}s"
      else
        _zp_runtime_error "$rc" "list failed"
      fi
    fi
  } always {
    listing=''
    unset REPLY
  }
  return 0
}

status() {
	if (( $# != 0 )); then
		_zp_runtime_error 2 "usage: status"
		return 0
	fi
	if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then
    print -r -- "$ZP_ACTIVE_PROFILE"
  else
    print -r -- main
  fi
}
`

// HookScript returns the complete sourced runtime loader.
func (Provider) HookScript() string { return loaderScript }
