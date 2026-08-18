package zsh

// loaderScript is sourced into an interactive zsh. It deliberately avoids
// emulate -L/LOCAL_OPTIONS: state changes made by emitted apply/deactivate
// blocks must persist in the caller's terminal after these functions return.
const loaderScript = `
typeset -g ZP_LAST_RUNTIME_STATUS=0
typeset -g ZP_LAST_RUNTIME_ERROR=''
typeset -g ZP_RUNTIME_TIMED_OUT=0

# Worktree state is parent-shell-local and deliberately non-exported. Preserve
# an authenticated pair and semantic baseline across a loader refresh; clear
# only transient capture arrays at source time.
(( ${+ZP_WORKTREE_SHELL_ID} )) || typeset -g ZP_WORKTREE_SHELL_ID=''
(( ${+ZP_WORKTREE_CAPABILITY} )) || typeset -g ZP_WORKTREE_CAPABILITY=''
typeset -g +x ZP_WORKTREE_SHELL_ID ZP_WORKTREE_CAPABILITY
(( ${+ZP_WORKTREE_ATTACHED} )) || typeset -g ZP_WORKTREE_ATTACHED=0
(( ${+ZP_WORKTREE_APPLIED_REVISION} )) || typeset -g ZP_WORKTREE_APPLIED_REVISION=0
(( ${+ZP_WORKTREE_RECONCILE_REQUIRED} )) || typeset -g ZP_WORKTREE_RECONCILE_REQUIRED=0
typeset -g +x ZP_WORKTREE_RECONCILE_REQUIRED
(( ${+ZP_WORKTREE_OPERATION_SEQUENCE} )) || typeset -g ZP_WORKTREE_OPERATION_SEQUENCE=0
(( ${+ZP_WORKTREE_LAST_ERROR} )) || typeset -g ZP_WORKTREE_LAST_ERROR=''
(( ${+ZP_WORKTREE_CONFLICT_COUNT} )) || typeset -g ZP_WORKTREE_CONFLICT_COUNT=0
(( ${+ZP_WORKTREE_AUTO_APPLY_DEFAULT} )) || typeset -g ZP_WORKTREE_AUTO_APPLY_DEFAULT=true
(( ${+ZP_WORKTREE_AUTO_APPLY_EFFECTIVE} )) || typeset -g ZP_WORKTREE_AUTO_APPLY_EFFECTIVE=true
(( ${+ZP_WORKTREE_BASELINE_FIELDS} )) || typeset -ga ZP_WORKTREE_BASELINE_FIELDS=()
(( ${+ZP_WORKTREE_BASELINE_COUNTS} )) || typeset -ga ZP_WORKTREE_BASELINE_COUNTS=()
typeset -ga _ZP_WORKTREE_CAPTURE_FIELDS=()
typeset -ga _ZP_WORKTREE_CAPTURE_COUNTS=()
typeset -g ZP_WORKTREE_GUARD=0 ZP_WORKTREE_UNSUPPORTED=0

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

_zp_worktree_install_transition() {
  local apply="$1" reverse="$2" old_reverse="$3" apply_rc=1 cleanup_rc=1
  # The generated pair and the prior owner are all loader-selected. Validate
  # the complete ownership graph before the first forward mutation.
  if [[ -z "$apply" || -z "$reverse" || "$apply" == "$reverse" ||
        ${+functions[$apply]} != 1 || ${+functions[$reverse]} != 1 ]]; then
    unset -f "$apply" 2>/dev/null || :
    [[ "$reverse" == "${ZP_ACTIVE_REVERSE_FN-}" || "$reverse" == "${ZP_RECOVERY_REVERSE_FN-}" ]] || unset -f "$reverse" 2>/dev/null || :
    return 1
  fi
  if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x || "${ZP_ACTIVE_REVERSE_FN-}" != "$old_reverse" ||
        ( -n "$old_reverse" && ( "$old_reverse" == "$reverse" || ${+functions[$old_reverse]} != 1 ) ) ]]; then
    unset -f "$apply" 2>/dev/null || :
    [[ "$reverse" == "${ZP_ACTIVE_REVERSE_FN-}" || "$reverse" == "${ZP_RECOVERY_REVERSE_FN-}" ]] || unset -f "$reverse" 2>/dev/null || :
    return 1
  fi
  if ! _zp_preflight_undo_slots ZP_ACTIVE_REVERSE_FN ZP_RECOVERY_REVERSE_FN; then
    unset -f "$apply" 2>/dev/null || :
    unset -f "$reverse" 2>/dev/null || :
    return 1
  fi

  # Replacement recovery is reachable before forward operation one. Keep the
  # previous active owner intact until the replacement has fully applied and
  # its active-pointer promotion succeeds.
  if ! typeset -g ZP_RECOVERY_REVERSE_FN="$reverse"; then
    unset -f "$apply" 2>/dev/null || :
    unset -f "$reverse" 2>/dev/null || :
    return 1
  fi
  if "$apply"; then
    apply_rc=0
  else
    apply_rc=$?
  fi
  if (( apply_rc != 0 )); then
    if _zp_recover_failed_target; then cleanup_rc=0; else cleanup_rc=$?; fi
    unset -f "$apply" 2>/dev/null || :
    if (( cleanup_rc != 0 )); then return "$cleanup_rc"; fi
    return "$apply_rc"
  fi

  # Promotion never creates an ownership gap: recovery remains set until the
  # active pointer names the replacement. Old cleanup happens last.
  if ! typeset -g ZP_ACTIVE_REVERSE_FN="$reverse"; then
    unset -f "$apply" 2>/dev/null || :
    return 1
  fi
  if ! unset ZP_RECOVERY_REVERSE_FN; then
    unset -f "$apply" 2>/dev/null || :
    return 1
  fi
  if [[ -n "$old_reverse" && "$old_reverse" != "$reverse" ]]; then
    if ! unset -f "$old_reverse" 2>/dev/null; then
      unset -f "$apply" 2>/dev/null || :
      return 1
    fi
  fi
  if ! unset -f "$apply" 2>/dev/null; then return 1; fi
  return 0
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

_zp_worktree_error() {
  local message="$1"
  typeset -g ZP_WORKTREE_LAST_ERROR="$message" ZP_LAST_RUNTIME_STATUS=1 ZP_LAST_RUNTIME_ERROR="$message"
  return 1
}

_zp_worktree_protected_call() {
	local target="$1" rc=1 xtrace_was_on=0 history_pushed=0
	shift
	{
		[[ -o xtrace ]] && xtrace_was_on=1
		setopt NOXTRACE 2>/dev/null || return 1
		if fc -p 2>/dev/null; then history_pushed=1; else return 1; fi
		"$target" "$@"
		rc=$?
	} always {
		if (( history_pushed )); then fc -P 2>/dev/null || :; fi
		if (( xtrace_was_on )); then setopt XTRACE 2>/dev/null || :; else setopt NOXTRACE 2>/dev/null || :; fi
		unset REPLY
	}
	return "$rc"
}

_zp_worktree_next_operation() {
  local result_name="$1" value
  (( ++ZP_WORKTREE_OPERATION_SEQUENCE ))
  value="zp:${$}:${ZP_WORKTREE_OPERATION_SEQUENCE}:${EPOCHREALTIME//./}"
  : ${(P)result_name::=$value}
}

_zp_worktree_budget_begin() {
  local result_name="$1"
  zmodload zsh/datetime 2>/dev/null || return 1
  : ${(P)result_name::=$(( EPOCHREALTIME + 0.250 ))}
}

_zp_worktree_budget_check() {
  (( ${2-${EPOCHREALTIME-0}} <= $1 ))
}

_zp_worktree_capture_clear() {
  _ZP_WORKTREE_CAPTURE_FIELDS=()
  _ZP_WORKTREE_CAPTURE_COUNTS=()
}

_zp_worktree_capture_append_record() {
  local deadline="$1" marker="$2" kind="$3" name="$4" attribute="$5" count="$6" value
  local -i encoded_bytes=0 encoded_fields=0 field_count=$(( $# - 1 )) value_count=$(( $# - 6 ))
  shift
  [[ "$marker" == R && "$count" == <-> && ( ${#count} == 1 || "$count" != 0* ) ]] && (( count == value_count )) || {
    _zp_worktree_capture_clear
    return 1
  }
  case "$kind" in
    env)
      [[ "$attribute" == exported && "$name" == [ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* ]] && (( count == 1 )) || { _zp_worktree_capture_clear; return 1; }
      [[ "$name" != PATH && "$name" != FPATH && "$name" != PWD && "$name" != OLDPWD && "$name" != SHLVL && "$name" != _ ]] || { _zp_worktree_capture_clear; return 1; }
      ;;
    alias|function)
      [[ "$attribute" == body && "${name:l}" != _zp_* && "${name:l}" != __zp_* ]] && (( count == 1 )) || { _zp_worktree_capture_clear; return 1; }
      [[ "$name" == [0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_.-]* ]] || { _zp_worktree_capture_clear; return 1; }
      ;;
    path) [[ "$name" == PATH && "$attribute" == ordered ]] || { _zp_worktree_capture_clear; return 1; } ;;
    fpath) [[ "$name" == FPATH && "$attribute" == ordered ]] || { _zp_worktree_capture_clear; return 1; } ;;
    option)
      [[ "$attribute" == boolean && "$name" == [ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && ( "$6" == on || "$6" == off ) ]] && (( count == 1 )) || { _zp_worktree_capture_clear; return 1; }
      ;;
    *) _zp_worktree_capture_clear; return 1 ;;
  esac
  (( snapshot_records < 10000 )) || { _zp_worktree_capture_clear; return 1; }
  for value in "$@"; do
    (( encoded_bytes <= 2097152 - ${#value} - 1 )) || { _zp_worktree_capture_clear; return 1; }
    (( encoded_bytes += ${#value} + 1 ))
    (( ++encoded_fields % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || { _zp_worktree_capture_clear; return 124; }
  done
  _zp_worktree_budget_check "$deadline" || { _zp_worktree_capture_clear; return 124; }
  (( snapshot_bytes <= 2097152 - encoded_bytes )) || { _zp_worktree_capture_clear; return 1; }
  _ZP_WORKTREE_CAPTURE_FIELDS+=("$@")
  _ZP_WORKTREE_CAPTURE_COUNTS+=("$field_count")
  (( ++snapshot_records, snapshot_bytes += encoded_bytes ))
  return 0
}

_zp_worktree_capture() {
  local deadline="$1" name descriptor value option_state rc=1
  local -a elements
  local -i snapshot_records=0 snapshot_bytes=21 scan_count=0
  _ZP_WORKTREE_CAPTURE_FIELDS=(ZP_LIVE_SNAPSHOT 1)
  _ZP_WORKTREE_CAPTURE_COUNTS=(2)
  {
    _zp_worktree_budget_check "$deadline" || return 124
    zmodload zsh/parameter 2>/dev/null || return 1
    for name in "${(@ok)parameters}"; do
      (( ++scan_count % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
      [[ "$name" == PATH || "$name" == FPATH || "$name" == PWD || "$name" == OLDPWD || "$name" == SHLVL || "$name" == _ ]] && continue
      descriptor="${parameters[$name]}"
      [[ "$descriptor" == *scalar* && "$descriptor" == *export* ]] || continue
      value="${(P)name}"
      _zp_worktree_capture_append_record "$deadline" R env "$name" exported 1 "$value" || return $?
      value=''
    done
    _zp_worktree_budget_check "$deadline" || return 124
    scan_count=0
    for name in "${(@ok)aliases}"; do
      (( ++scan_count % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
      [[ "${name:l}" != _zp_* && "${name:l}" != __zp_* ]] || continue
      [[ "$name" == [0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_.-]* ]] || continue
      value="${aliases[$name]}"
      _zp_worktree_capture_append_record "$deadline" R alias "$name" body 1 "$value" || return $?
      value=''
    done
    _zp_worktree_budget_check "$deadline" || return 124
    scan_count=0
    for name in "${(@ok)functions}"; do
      (( ++scan_count % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
      [[ "${name:l}" != _zp_* && "${name:l}" != __zp_* ]] || continue
      [[ "$name" == [0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_.-]* ]] || continue
      value="${functions[$name]}"
      _zp_worktree_capture_append_record "$deadline" R function "$name" body 1 "$value" || return $?
      value=''
    done
    _zp_worktree_budget_check "$deadline" || return 124
    elements=("${path[@]}")
    _zp_worktree_capture_append_record "$deadline" R path PATH ordered "${#elements}" "${elements[@]}" || return $?
    elements=()
    _zp_worktree_budget_check "$deadline" || return 124
    elements=("${fpath[@]}")
    _zp_worktree_capture_append_record "$deadline" R fpath FPATH ordered "${#elements}" "${elements[@]}" || return $?
    elements=()
    _zp_worktree_budget_check "$deadline" || return 124
    scan_count=0
    for name in "${(@ok)options}"; do
      (( ++scan_count % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
      option_state="${options[$name]}"
      _zp_worktree_capture_append_record "$deadline" R option "$name" boolean 1 "$option_state" || return $?
    done
    _zp_worktree_budget_check "$deadline" || return 124
    _ZP_WORKTREE_CAPTURE_FIELDS+=(E)
    _ZP_WORKTREE_CAPTURE_COUNTS+=(1)
    rc=0
  } always {
    value='' option_state='' elements=()
    (( rc == 0 )) || _zp_worktree_capture_clear
  }
  return "$rc"
}

_zp_worktree_write_scalar_record() {
  local tag="$1" value="$2" deadline="$3" length LC_ALL=C
  local -i addition
  length="${#value}"
  _zp_worktree_budget_check "$deadline" || return 124
  addition=$(( ${#tag} + 1 + ${#length} + 1 + length + 1 ))
  (( ZP_WORKTREE_FRAME_BYTES <= 2101248 - addition )) || return 1
  (( ZP_WORKTREE_FRAME_BYTES += addition, ++ZP_WORKTREE_FRAME_RECORDS_WRITTEN ))
  builtin printf '%s %s\n' "$tag" "$length"
  builtin printf '%s\n' "$value"
}

_zp_worktree_write_snapshot_records() {
  local tag="$1" fields_name="$2" counts_name="$3" deadline="$4" value marker LC_ALL=C
  local -i offset=0 count length index end addition snapshot_bytes=0 snapshot_records=0
  local -a fields counts
  _zp_worktree_budget_check "$deadline" || return 124
  fields=("${(@P)fields_name}")
  counts=("${(@P)counts_name}")
  _zp_worktree_budget_check "$deadline" || return 124
  (( ${#counts} <= 10002 )) || return 1
  for count in "${counts[@]}"; do
    _zp_worktree_budget_check "$deadline" || return 124
    [[ "$count" == <-> && "$count" != 0 ]] || return 1
    end=$(( offset + count ))
    (( end <= ${#fields} )) || return 1
    marker="${fields[$(( offset + 1 ))]}"
    case "$marker" in
      R) (( ++snapshot_records <= 10000 )) || return 1 ;;
      ZP_LIVE_SNAPSHOT) (( offset == 0 && count == 2 )) || return 1 ;;
      E) (( end == ${#fields} && count == 1 )) || return 1 ;;
      *) return 1 ;;
    esac
    length=0
    for (( index=offset + 1; index <= end; ++index )); do
      value="${fields[$index]}"
      (( length <= 2097152 - ${#value} - 1 )) || return 1
      (( length += ${#value} + 1 ))
    done
    _zp_worktree_budget_check "$deadline" || return 124
    (( snapshot_bytes <= 2097152 - length )) || return 1
    (( snapshot_bytes += length ))
    addition=$(( ${#tag} + 1 + ${#length} + 1 + length + 1 ))
    (( ZP_WORKTREE_FRAME_BYTES <= 2101248 - addition )) || return 1
    (( ZP_WORKTREE_FRAME_BYTES += addition, ++ZP_WORKTREE_FRAME_RECORDS_WRITTEN ))
    builtin printf '%s %s\n' "$tag" "$length"
    for (( index=offset + 1; index <= end; ++index )); do
      builtin printf '%s\0' "${fields[$index]}"
    done
    builtin printf '\n'
    offset=$end
  done
  (( offset == ${#fields} && snapshot_records <= 10000 && snapshot_bytes <= 2097152 ))
}

_zp_worktree_write_frame() {
  local operation="$1" mode="$2" operation_id="$3" revision="$4" token="$5"
  local identity_kind="$6" identity_name="$7" apply_name="$8" reverse_name="$9"
  local baseline_fields="${10}" baseline_counts="${11}" current_fields="${12}" current_counts="${13}"
  local deadline="${14}" header
  local -i record_count baseline_records=0 current_records=0 header_bytes
  local -i ZP_WORKTREE_FRAME_BYTES=0 ZP_WORKTREE_FRAME_RECORDS_WRITTEN=0
  [[ -z "$baseline_counts" ]] || baseline_records=${#${(P)baseline_counts}}
  [[ -z "$current_counts" ]] || current_records=${#${(P)current_counts}}
  case "$operation:$mode" in
    attach:allocate) record_count=3 ;;
    attach:commit) record_count=$(( 6 + current_records )) ;;
    publish:*) record_count=$(( 6 + baseline_records + current_records )) ;;
    prepare:*) record_count=$(( 8 + current_records )) ;;
    acknowledge:*) record_count=$(( 7 + current_records )) ;;
    resolve:*) record_count=$(( 10 + current_records )) ;;
    *) return 1 ;;
  esac
  (( record_count <= 10016 )) || return 1
  _zp_worktree_budget_check "$deadline" || return 124
  header="ZPWT 1 ${record_count}"
  header_bytes=$(( ${#header} + 1 ))
  (( header_bytes <= 2101248 )) || return 1
  ZP_WORKTREE_FRAME_BYTES=$header_bytes
  builtin printf '%s\n' "$header"
  _zp_worktree_write_scalar_record 1 "$operation" "$deadline" || return $?
  if [[ "$operation" == attach ]]; then _zp_worktree_write_scalar_record 2 "$mode" "$deadline" || return $?; fi
  if [[ "$mode" != allocate ]]; then
    _zp_worktree_write_scalar_record 3 "$ZP_WORKTREE_SHELL_ID" "$deadline" || return $?
    _zp_worktree_write_scalar_record 4 "$ZP_WORKTREE_CALL_CAPABILITY" "$deadline" || return $?
    _zp_worktree_write_scalar_record 5 "$operation_id" "$deadline" || return $?
  fi
  case "$operation" in
    publish|prepare|acknowledge) _zp_worktree_write_scalar_record 6 "$revision" "$deadline" || return $? ;;
  esac
  case "$operation" in
    acknowledge|resolve) _zp_worktree_write_scalar_record 7 "$token" "$deadline" || return $? ;;
  esac
  if [[ "$operation" == resolve ]]; then
    _zp_worktree_write_scalar_record 8 "$identity_kind" "$deadline" || return $?
    _zp_worktree_write_scalar_record 9 "$identity_name" "$deadline" || return $?
  fi
  if [[ "$operation" == prepare || "$operation" == resolve ]]; then
    _zp_worktree_write_scalar_record 10 "$apply_name" "$deadline" || return $?
    _zp_worktree_write_scalar_record 11 "$reverse_name" "$deadline" || return $?
  fi
  if [[ "$operation" == publish ]]; then
    _zp_worktree_write_snapshot_records 31 "$baseline_fields" "$baseline_counts" "$deadline" || return $?
  fi
  if [[ "$mode" != allocate ]]; then
    _zp_worktree_write_snapshot_records 32 "$current_fields" "$current_counts" "$deadline" || return $?
  fi
  # The exact final wire record remains 255 0 plus its empty payload line.
  _zp_worktree_write_scalar_record 255 '' "$deadline" || return $?
  (( ZP_WORKTREE_FRAME_RECORDS_WRITTEN == record_count ))
}

_zp_worktree_close_transport_fd() {
	local name="$1" fd="${(P)1}"
	: ${(P)name::=-1}
	if [[ "$fd" == <-> ]] && (( fd >= 0 )); then
		exec {fd}>&- || :
	fi
}

_zp_worktree_close_coproc_endpoints() {
	local baseline_name="$1" read_fd="$2" write_fd="$3" fd
	local -a baseline current
	[[ -d /proc/$$/fd ]] || return 1
	baseline=("${(@P)baseline_name}")
	current=(/proc/$$/fd/*(N:t))
	for fd in "${current[@]}"; do
		[[ "$fd" == <-> ]] || continue
		(( ${baseline[(Ie)$fd]} == 0 )) || continue
		[[ "$fd" != "$read_fd" && "$fd" != "$write_fd" ]] || continue
		[[ -p "/proc/$$/fd/$fd" ]] || continue
		coproc_fds+=("$fd")
	done
}

_zp_worktree_close_owned_fd() {
	local fd="$1"
	[[ "$fd" == <-> ]] && (( fd >= 0 )) || return 0
	exec {fd}>&- || :
}

_zp_worktree_pid_owned() {
	local pid="$1" children=''
	[[ "$pid" == <-> ]] && (( pid > 1 )) || return 1
	[[ -r "/proc/$$/task/$$/children" ]] || return 1
	IFS= read -r children < "/proc/$$/task/$$/children" || :
	[[ " $children " == *" $pid "* ]]
}

_zp_worktree_reap_exited_pid() {
	local pid_name="$1" status_name="$2" pid="${(P)1}" child_status=0
	[[ "$pid" == <-> ]] && (( pid > 0 )) || { : ${(P)pid_name::=0}; return 0; }
	kill -0 "$pid" 2>/dev/null && return 1
	if wait "$pid" 2>/dev/null; then child_status=0; else child_status=$?; fi
	[[ -z "$status_name" ]] || : ${(P)status_name::=$child_status}
	: ${(P)pid_name::=0}
	return 0
}

_zp_worktree_abort_transport() {
	local deadline="$1" termination_deadline="$2" name pid now
	local kill_deadline=$(( deadline - 0.010 ))
	local -a pid_names=(writer_pid helper_pid closer_pid)
	_zp_worktree_close_transport_fd write_fd
	_zp_worktree_close_transport_fd read_fd
	_zp_worktree_reap_exited_pid writer_pid frame_rc || :
	_zp_worktree_reap_exited_pid helper_pid wait_rc || :
	_zp_worktree_reap_exited_pid closer_pid '' || :
	for name in "${pid_names[@]}"; do
		pid="${(P)name}"
		(( pid > 0 )) || continue
		_zp_worktree_pid_owned "$pid" && kill -TERM "$pid" 2>/dev/null || :
	done
	while true; do
		_zp_worktree_reap_exited_pid writer_pid frame_rc || :
		_zp_worktree_reap_exited_pid helper_pid wait_rc || :
		_zp_worktree_reap_exited_pid closer_pid '' || :
		(( writer_pid == 0 && helper_pid == 0 && closer_pid == 0 )) && return 124
		now="${EPOCHREALTIME-0}"
		(( now < kill_deadline )) || break
	done
	for name in "${pid_names[@]}"; do
		pid="${(P)name}"
		(( pid > 0 )) || continue
		_zp_worktree_pid_owned "$pid" && kill -KILL "$pid" 2>/dev/null || :
	done
	while true; do
		_zp_worktree_reap_exited_pid writer_pid frame_rc || :
		_zp_worktree_reap_exited_pid helper_pid wait_rc || :
		_zp_worktree_reap_exited_pid closer_pid '' || :
		(( writer_pid == 0 && helper_pid == 0 && closer_pid == 0 )) && return 124
		now="${EPOCHREALTIME-0}"
		(( now < deadline )) || break
	done
	_zp_worktree_reap_exited_pid writer_pid frame_rc || :
	_zp_worktree_reap_exited_pid helper_pid wait_rc || :
	_zp_worktree_reap_exited_pid closer_pid '' || :
	return 124
}

_zp_worktree_invoke() {
  local operation="$1" result_name="$2" mode="$3" operation_id="$4" revision="$5" token="$6"
  local identity_kind="$7" identity_name="$8" apply_name="$9" reverse_name="${10}"
  local baseline_fields="${11}" baseline_counts="${12}" current_fields="${13}" current_counts="${14}"
	local deadline="${15}"
	local termination_deadline=$(( deadline - 0.025 ))
	local captured='' line='' rc=1 xtrace_was_on=0 history_pushed=0 helper_pid=0 writer_pid=0 closer_pid=0
	local read_fd=-1 write_fd=-1 remaining wait_rc=0 frame_rc=0
	local -a transport_base_fds coproc_fds
	local ZP_WORKTREE_CALL_CAPABILITY=''
	: ${(P)result_name::=}
	{
		[[ -n "$deadline" ]] || return 1
		_zp_worktree_budget_check "$termination_deadline" || return 124
		[[ -o xtrace ]] && xtrace_was_on=1
		setopt NOXTRACE 2>/dev/null || return 1
		ZP_WORKTREE_CALL_CAPABILITY="$ZP_WORKTREE_CAPABILITY"
		if fc -p 2>/dev/null; then history_pushed=1; else return 1; fi
		[[ -d /proc/$$/fd ]] || return 1
		transport_base_fds=(/proc/$$/fd/*(N:t))
		case "$operation" in
			attach)
				coproc command zsh-pro runtime worktree attach 1 2>/dev/null
				;;
			publish)
				coproc command zsh-pro runtime worktree publish 1 2>/dev/null
				;;
			prepare)
				coproc command zsh-pro runtime worktree prepare 1 2>/dev/null
				;;
			acknowledge)
				coproc command zsh-pro runtime worktree acknowledge 1 2>/dev/null
				;;
			resolve)
				coproc command zsh-pro runtime worktree resolve 1 2>/dev/null
				;;
			*) return 2 ;;
		esac
		helper_pid=$!
		exec {write_fd}>&p || return 1
		exec {read_fd}<&p || return 1
		# Starting an inert replacement closes zsh's special coprocess endpoints;
		# the two private duplicates remain owned by this call.
		coproc :
		closer_pid=$!
		_zp_worktree_close_coproc_endpoints transport_base_fds "$read_fd" "$write_fd" || return 1
		for fd in "${coproc_fds[@]}"; do _zp_worktree_close_owned_fd "$fd"; done
		coproc_fds=()
		( _zp_worktree_write_frame "$operation" "$mode" "$operation_id" "$revision" "$token" "$identity_kind" "$identity_name" "$apply_name" "$reverse_name" "$baseline_fields" "$baseline_counts" "$current_fields" "$current_counts" "$deadline" ) >&$write_fd &
		writer_pid=$!
		_zp_worktree_close_transport_fd write_fd
		while true; do
			remaining=$(( termination_deadline - EPOCHREALTIME ))
			if (( remaining <= 0 )); then rc=124; break; fi
			if IFS= read -r -t "$remaining" line <&$read_fd; then
				captured+="$line"$'\n'
				(( ${#captured} <= 2101248 )) || { rc=1; break; }
				continue
			fi
			if _zp_worktree_budget_check "$termination_deadline"; then rc=0; else rc=124; fi
			break
		done
		_zp_worktree_close_transport_fd read_fd
		if (( rc != 0 )); then
			_zp_worktree_abort_transport "$deadline" "$termination_deadline"
			return 124
		fi
		while (( writer_pid > 0 || helper_pid > 0 || closer_pid > 0 )); do
			_zp_worktree_reap_exited_pid writer_pid frame_rc || :
			_zp_worktree_reap_exited_pid helper_pid wait_rc || :
			_zp_worktree_reap_exited_pid closer_pid '' || :
			(( writer_pid == 0 && helper_pid == 0 && closer_pid == 0 )) && break
			_zp_worktree_budget_check "$termination_deadline" || { _zp_worktree_abort_transport "$deadline" "$termination_deadline"; return 124; }
		done
		if (( wait_rc != 0 )); then rc=$wait_rc; elif (( frame_rc != 0 )); then rc=$frame_rc; fi
		if (( rc == 0 )); then
			captured="${captured%$'\n'}"
			: ${(P)result_name::=$captured}
		fi
	} always {
		if (( write_fd >= 0 || read_fd >= 0 || helper_pid > 0 || writer_pid > 0 || closer_pid > 0 )); then
			_zp_worktree_abort_transport "$deadline" "$termination_deadline" || :
		fi
		captured='' line='' ZP_WORKTREE_CALL_CAPABILITY='' operation_id='' token='' deadline='' termination_deadline='' remaining=''
		transport_base_fds=() coproc_fds=()
		if (( history_pushed )); then fc -P 2>/dev/null || :; fi
    if (( xtrace_was_on )); then setopt XTRACE 2>/dev/null || :; else setopt NOXTRACE 2>/dev/null || :; fi
    unset REPLY
  }
  return "$rc"
}

_zp_worktree_valid_hex64() {
  local value="$1"
  [[ ${#value} == 64 && "$value" != *[^0-9a-f]* ]]
}

_zp_worktree_valid_uint() {
  local value="$1"
  [[ "$value" == <-> && "$value" != 0 && ( ${#value} == 1 || "$value" != 0* ) ]]
}

_zp_worktree_refresh_auto_apply() {
	local status_output="$1" line='' persisted=''
	for line in "${(@f)status_output}"; do
		case "$line" in
			'auto-apply default: true') persisted=true ;;
			'auto-apply default: false') persisted=false ;;
		esac
	done
	[[ -z "$persisted" ]] || typeset -g ZP_WORKTREE_AUTO_APPLY_DEFAULT="$persisted"
  status_output='' line='' persisted=''
  return 0
}

_zp_worktree_ensure_attached_impl() {
  local deadline="$1" response='' operation_id='' rc=1
  local -a lines fields
  typeset -g ZP_WORKTREE_ATTACHED_NOW=0
  (( ZP_WORKTREE_UNSUPPORTED == 0 )) || return 64
  if (( ZP_WORKTREE_ATTACHED )) && _zp_worktree_valid_hex64 "$ZP_WORKTREE_SHELL_ID" && _zp_worktree_valid_hex64 "$ZP_WORKTREE_CAPABILITY"; then
    return 0
  fi
  if [[ -n "$ZP_WORKTREE_SHELL_ID" || -n "$ZP_WORKTREE_CAPABILITY" ]]; then
    if ! _zp_worktree_valid_hex64 "$ZP_WORKTREE_SHELL_ID" || ! _zp_worktree_valid_hex64 "$ZP_WORKTREE_CAPABILITY"; then
      typeset -g ZP_WORKTREE_SHELL_ID='' ZP_WORKTREE_CAPABILITY='' ZP_WORKTREE_ATTACHED=0
      return 1
    fi
  fi
  if (( ${#ZP_WORKTREE_BASELINE_COUNTS} == 0 )); then
    _zp_worktree_capture "$deadline" || return $?
    ZP_WORKTREE_BASELINE_FIELDS=("${_ZP_WORKTREE_CAPTURE_FIELDS[@]}")
    ZP_WORKTREE_BASELINE_COUNTS=("${_ZP_WORKTREE_CAPTURE_COUNTS[@]}")
    _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
  fi
  if [[ -z "$ZP_WORKTREE_SHELL_ID" ]]; then
    if _zp_worktree_invoke attach response allocate '' '' '' '' '' '' '' '' '' '' '' "$deadline"; then :; else
      rc=$?
      (( rc == 64 )) && typeset -g ZP_WORKTREE_UNSUPPORTED=1
      _zp_worktree_error "worktree attachment allocation failed"
      return 1
    fi
    lines=("${(@f)response}")
    if (( ${#lines} != 3 )) || [[ "${lines[1]}" != 'ZPWC 1' ]] || ! _zp_worktree_valid_hex64 "${lines[2]}" || ! _zp_worktree_valid_hex64 "${lines[3]}" || [[ "${lines[2]}" == "${lines[3]}" ]]; then
      response='' lines=()
      _zp_worktree_error "worktree attachment response was invalid"
      return 1
    fi
    typeset -g +x ZP_WORKTREE_SHELL_ID="${lines[2]}" ZP_WORKTREE_CAPABILITY="${lines[3]}"
    response='' lines=()
  fi
  [[ -n "$ZP_WORKTREE_ATTACH_OPERATION_ID" ]] || _zp_worktree_next_operation operation_id
  [[ -n "$ZP_WORKTREE_ATTACH_OPERATION_ID" ]] || typeset -g ZP_WORKTREE_ATTACH_OPERATION_ID="$operation_id"
  operation_id="$ZP_WORKTREE_ATTACH_OPERATION_ID"
  if ! _zp_worktree_invoke attach response commit "$operation_id" '' '' '' '' '' '' '' '' ZP_WORKTREE_BASELINE_FIELDS ZP_WORKTREE_BASELINE_COUNTS "$deadline"; then
    _zp_worktree_error "worktree attachment failed"
    return 1
  fi
  fields=(${(z)response})
  if (( ${#fields} != 5 )) || [[ "${fields[1]}" != ZPWA || "${fields[2]}" != 1 || "${fields[4]}" != 1 || ( "${fields[5]}" != 0 && "${fields[5]}" != 1 ) ]] || ! _zp_worktree_valid_uint "${fields[3]}"; then
    _zp_worktree_error "worktree attachment acknowledgement was invalid"
    return 1
  fi
	typeset -g ZP_WORKTREE_ATTACHED=1 ZP_WORKTREE_ATTACHED_NOW=1 ZP_WORKTREE_RECONCILE_REQUIRED="${fields[5]}" ZP_WORKTREE_LAST_ERROR=''
	if (( ZP_WORKTREE_RECONCILE_REQUIRED )); then
		typeset -g ZP_WORKTREE_APPLIED_REVISION=0
	else
		typeset -g ZP_WORKTREE_APPLIED_REVISION="${fields[3]}"
	fi
	unset ZP_WORKTREE_ATTACH_OPERATION_ID
  response='' fields=() operation_id=''
  return 0
}

_zp_worktree_parse_publish_response() {
  local response="$1" header line count index
  local -a lines fields
  lines=("${(@f)response}")
  (( ${#lines} >= 1 )) || return 1
  fields=(${(z)lines[1]})
  (( ${#fields} == 4 )) || return 1
  [[ "${fields[1]}" == ZPWP && "${fields[2]}" == 1 ]] || return 1
  _zp_worktree_valid_uint "${fields[3]}" || return 1
  count="${fields[4]}"
  [[ "$count" == <-> && ( ${#count} == 1 || "$count" != 0* ) && "$count" -le 10000 ]] || return 1
  (( ${#lines} == count + 1 )) || return 1
  typeset -g ZP_WORKTREE_CONFLICT_COUNT="$count"
  unset ZP_WORKTREE_CONFLICT_KIND ZP_WORKTREE_CONFLICT_IDENTITY_KIND ZP_WORKTREE_CONFLICT_IDENTITY_NAME ZP_WORKTREE_CONFLICT_TOKEN
  for (( index=2; index <= ${#lines}; ++index )); do
    fields=(${(z)lines[$index]})
    (( ${#fields} == 4 )) || return 1
    [[ "${fields[1]}" == overlap || "${fields[1]}" == history-gap ]] || return 1
    [[ "${fields[2]}" == env || "${fields[2]}" == alias || "${fields[2]}" == function || "${fields[2]}" == path || "${fields[2]}" == fpath || "${fields[2]}" == option ]] || return 1
    [[ -n "${fields[3]}" && -n "${fields[4]}" ]] || return 1
    if (( index == 2 )); then
      typeset -g ZP_WORKTREE_CONFLICT_KIND="${fields[1]}" ZP_WORKTREE_CONFLICT_IDENTITY_KIND="${fields[2]}"
      typeset -g ZP_WORKTREE_CONFLICT_IDENTITY_NAME="${fields[3]}" ZP_WORKTREE_CONFLICT_TOKEN="${fields[4]}"
    fi
  done
  return 0
}

_zp_worktree_capture_matches_baseline() {
  local deadline="$1" value
  local -i index
  (( ${#_ZP_WORKTREE_CAPTURE_FIELDS} == ${#ZP_WORKTREE_BASELINE_FIELDS} && ${#_ZP_WORKTREE_CAPTURE_COUNTS} == ${#ZP_WORKTREE_BASELINE_COUNTS} )) || return 1
  for (( index=1; index <= ${#_ZP_WORKTREE_CAPTURE_COUNTS}; ++index )); do
    (( index % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
    [[ "${_ZP_WORKTREE_CAPTURE_COUNTS[$index]}" == "${ZP_WORKTREE_BASELINE_COUNTS[$index]}" ]] || return 1
  done
  _zp_worktree_budget_check "$deadline" || return 124
  for (( index=1; index <= ${#_ZP_WORKTREE_CAPTURE_FIELDS}; ++index )); do
    (( index % 64 != 0 )) || _zp_worktree_budget_check "$deadline" || return 124
    [[ "${_ZP_WORKTREE_CAPTURE_FIELDS[$index]}" == "${ZP_WORKTREE_BASELINE_FIELDS[$index]}" ]] || return 1
  done
  _zp_worktree_budget_check "$deadline"
}

_zp_worktree_publish_impl() {
  local deadline="$1" response='' operation_id=''
  local -i match_rc=1 capture_rc=0
  _zp_worktree_ensure_attached "$deadline" || return 1
  (( ZP_WORKTREE_ATTACHED_NOW )) && return 0
  _zp_worktree_capture "$deadline"
  capture_rc=$?
  if (( capture_rc != 0 )); then
    _zp_worktree_error "worktree capture failed"
    return "$capture_rc"
  fi
  if (( ${ZP_WORKTREE_SKIP_UNCHANGED_PUBLISH-0} )); then
    _zp_worktree_capture_matches_baseline "$deadline" && match_rc=0 || match_rc=$?
    if (( match_rc == 0 )); then
      _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
      typeset -g ZP_WORKTREE_LAST_ERROR=''
      return 0
    fi
    if (( match_rc == 124 )); then
      _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
      return 124
    fi
  fi
  _zp_worktree_next_operation operation_id
  if ! _zp_worktree_invoke publish response '' "$operation_id" "$ZP_WORKTREE_APPLIED_REVISION" '' '' '' '' '' ZP_WORKTREE_BASELINE_FIELDS ZP_WORKTREE_BASELINE_COUNTS _ZP_WORKTREE_CAPTURE_FIELDS _ZP_WORKTREE_CAPTURE_COUNTS "$deadline"; then
    _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
    _zp_worktree_error "worktree publication failed"
    return 1
  fi
  if ! _zp_worktree_parse_publish_response "$response"; then
    _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
    _zp_worktree_error "worktree publication response was invalid"
    return 1
  fi
  ZP_WORKTREE_BASELINE_FIELDS=("${_ZP_WORKTREE_CAPTURE_FIELDS[@]}")
  ZP_WORKTREE_BASELINE_COUNTS=("${_ZP_WORKTREE_CAPTURE_COUNTS[@]}")
  _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
  typeset -g ZP_WORKTREE_LAST_ERROR=''
  (( ZP_WORKTREE_CONFLICT_COUNT == 0 ))
}

_zp_worktree_publish() {
	local deadline="${ZP_WORKTREE_TRANSITION_DEADLINE-}"
	[[ -n "$deadline" ]] || _zp_worktree_budget_begin deadline || return 1
	_zp_worktree_protected_call _zp_worktree_publish_impl "$deadline"
}

_zp_worktree_apply_transition() {
  local operation="$1" deadline="$2" source='' operation_id='' apply_name reverse_name ack_id='' ack_response=''
  local revision token fingerprint old_reverse="${ZP_ACTIVE_REVERSE_FN-}" rc=1
	[[ "${ZP_RECOVERY_REVERSE_FN+x}" != x ]] || return 1
	unset ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN ZP_WORKTREE_REPLY_FINGERPRINT ZP_WORKTREE_REPLY_COMPLETE
	_zp_worktree_budget_check "$deadline" || return 124
  apply_name="__zp_worktree_apply_${$}_${ZP_WORKTREE_OPERATION_SEQUENCE}"
  reverse_name="__zp_worktree_reverse_${$}_${ZP_WORKTREE_OPERATION_SEQUENCE}"
  if [[ "$operation" == resolve ]]; then
    [[ -n "$ZP_WORKTREE_CONFLICT_TOKEN" && -n "$ZP_WORKTREE_CONFLICT_IDENTITY_KIND" && -n "$ZP_WORKTREE_CONFLICT_IDENTITY_NAME" ]] || return 1
    [[ -n "$ZP_WORKTREE_RESOLVE_OPERATION_ID" ]] || { _zp_worktree_next_operation operation_id; typeset -g ZP_WORKTREE_RESOLVE_OPERATION_ID="$operation_id"; }
    operation_id="$ZP_WORKTREE_RESOLVE_OPERATION_ID"
    _zp_worktree_invoke resolve source '' "$operation_id" '' "$ZP_WORKTREE_CONFLICT_TOKEN" "$ZP_WORKTREE_CONFLICT_IDENTITY_KIND" "$ZP_WORKTREE_CONFLICT_IDENTITY_NAME" "$apply_name" "$reverse_name" '' '' _ZP_WORKTREE_CAPTURE_FIELDS _ZP_WORKTREE_CAPTURE_COUNTS "$deadline" || return 1
  else
    [[ -n "$ZP_WORKTREE_PREPARE_OPERATION_ID" ]] || { _zp_worktree_next_operation operation_id; typeset -g ZP_WORKTREE_PREPARE_OPERATION_ID="$operation_id"; }
    operation_id="$ZP_WORKTREE_PREPARE_OPERATION_ID"
    _zp_worktree_invoke prepare source '' "$operation_id" "$ZP_WORKTREE_APPLIED_REVISION" '' '' '' "$apply_name" "$reverse_name" '' '' _ZP_WORKTREE_CAPTURE_FIELDS _ZP_WORKTREE_CAPTURE_COUNTS "$deadline" || return 1
  fi
	if [[ -z "$source" ]]; then
    unset ZP_WORKTREE_PREPARE_OPERATION_ID ZP_WORKTREE_RESOLVE_OPERATION_ID
    return 0
  fi
	typeset -g ZP_WORKTREE_RECONCILE_REQUIRED=1
	source+=$'\n'
	{
		_zp_worktree_budget_check "$deadline" || return 124
		_zp_eval_block "$source" || return 1
		_zp_worktree_budget_check "$deadline" || return 124
    [[ "${ZP_WORKTREE_REPLY_PROTOCOL-}" == 1 && "${ZP_WORKTREE_REPLY_COMPLETE-}" == 1 ]] || return 1
    revision="${ZP_WORKTREE_REPLY_REVISION-}" token="${ZP_WORKTREE_REPLY_TOKEN-}" fingerprint="${ZP_WORKTREE_REPLY_FINGERPRINT-}"
    _zp_worktree_valid_uint "$revision" && _zp_worktree_valid_uint "$token" && _zp_worktree_valid_hex64 "$fingerprint" || return 1
		if (( ${+functions[$apply_name]} || ${+functions[$reverse_name]} )); then
      (( ${+functions[$apply_name]} && ${+functions[$reverse_name]} )) || return 1
			_zp_worktree_install_transition "$apply_name" "$reverse_name" "$old_reverse" || return $?
		fi
		_zp_worktree_budget_check "$deadline" || return 124
		_zp_worktree_capture "$deadline" || return $?
		_zp_worktree_budget_check "$deadline" || return 124
    [[ -n "$ZP_WORKTREE_ACK_OPERATION_ID" ]] || { _zp_worktree_next_operation ack_id; typeset -g ZP_WORKTREE_ACK_OPERATION_ID="$ack_id"; }
    ack_id="$ZP_WORKTREE_ACK_OPERATION_ID"
		_zp_worktree_invoke acknowledge ack_response '' "$ack_id" "$revision" "$token" '' '' '' '' '' '' _ZP_WORKTREE_CAPTURE_FIELDS _ZP_WORKTREE_CAPTURE_COUNTS "$deadline" || return 1
		_zp_worktree_budget_check "$deadline" || return 124
    local -a ack_fields
    ack_fields=(${(z)ack_response})
    (( ${#ack_fields} == 4 )) && [[ "${ack_fields[1]}" == ZPWK && "${ack_fields[2]}" == 1 && "${ack_fields[3]}" == "$revision" && "${ack_fields[4]}" == 1 ]] || return 1
    typeset -g ZP_WORKTREE_APPLIED_REVISION="$revision" ZP_WORKTREE_RECONCILE_REQUIRED=0 ZP_WORKTREE_CONFLICT_COUNT=0 ZP_WORKTREE_LAST_ERROR=''
    ZP_WORKTREE_BASELINE_FIELDS=("${_ZP_WORKTREE_CAPTURE_FIELDS[@]}")
    ZP_WORKTREE_BASELINE_COUNTS=("${_ZP_WORKTREE_CAPTURE_COUNTS[@]}")
    unset ZP_WORKTREE_ACK_OPERATION_ID ZP_WORKTREE_PREPARE_OPERATION_ID ZP_WORKTREE_RESOLVE_OPERATION_ID
    unset ZP_WORKTREE_CONFLICT_KIND ZP_WORKTREE_CONFLICT_IDENTITY_KIND ZP_WORKTREE_CONFLICT_IDENTITY_NAME ZP_WORKTREE_CONFLICT_TOKEN
    rc=0
  } always {
    source='' operation_id='' ack_id='' ack_response='' token='' fingerprint=''
    _ZP_WORKTREE_CAPTURE_FIELDS=() _ZP_WORKTREE_CAPTURE_COUNTS=()
		if [[ -n "$apply_name" && ${+functions[$apply_name]} == 1 ]]; then unset -f "$apply_name" 2>/dev/null || :; fi
		if [[ -n "$reverse_name" && ${+functions[$reverse_name]} == 1 &&
		      "$reverse_name" != "${ZP_ACTIVE_REVERSE_FN-}" && "$reverse_name" != "${ZP_RECOVERY_REVERSE_FN-}" ]]; then
			unset -f "$reverse_name" 2>/dev/null || :
		fi
    unset ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN ZP_WORKTREE_REPLY_FINGERPRINT ZP_WORKTREE_REPLY_COMPLETE
  }
  return "$rc"
}

_zp_worktree_pull_impl() {
	local deadline="$1"
	_zp_worktree_ensure_attached "$deadline" || return 1
	(( ZP_WORKTREE_ATTACHED_NOW && ! ZP_WORKTREE_RECONCILE_REQUIRED )) && return 0
  _zp_worktree_capture "$deadline" || return $?
  _zp_worktree_apply_transition prepare "$deadline" || { _zp_worktree_error "worktree pull failed"; return 1; }
}

_zp_worktree_pull() {
	local deadline="${ZP_WORKTREE_TRANSITION_DEADLINE-}"
	[[ -n "$deadline" ]] || _zp_worktree_budget_begin deadline || return 1
	_zp_worktree_protected_call _zp_worktree_pull_impl "$deadline"
}

_zp_worktree_resolve_shared_impl() {
	local deadline="$1"
	_zp_worktree_ensure_attached "$deadline" || return 1
	(( ZP_WORKTREE_ATTACHED_NOW )) && return 1
  _zp_worktree_capture "$deadline" || return $?
  _zp_worktree_apply_transition resolve "$deadline" || { _zp_worktree_error "worktree shared resolution failed"; return 1; }
}

_zp_worktree_resolve_shared() {
	local deadline="${ZP_WORKTREE_TRANSITION_DEADLINE-}"
	[[ -n "$deadline" ]] || _zp_worktree_budget_begin deadline || return 1
	_zp_worktree_protected_call _zp_worktree_resolve_shared_impl "$deadline"
}

_zp_worktree_disable() {
	# Deactivation is terminal-local: remove the cooperative safe-boundary hooks
	# before erasing the credential pair so the next prompt cannot immediately
	# attach and reapply the shared projection.
	autoload -Uz add-zsh-hook
	add-zsh-hook -d precmd _zp_worktree_precmd 2>/dev/null || :
	if (( ${+widgets} )); then
		autoload -Uz add-zle-hook-widget
		add-zle-hook-widget -d line-finish _zp_worktree_line_finish 2>/dev/null || :
	fi
	unset ZP_WORKTREE_SHELL_ID ZP_WORKTREE_CAPABILITY ZP_WORKTREE_ATTACHED ZP_WORKTREE_ATTACHED_NOW
	unset ZP_WORKTREE_APPLIED_REVISION ZP_WORKTREE_RECONCILE_REQUIRED ZP_WORKTREE_OPERATION_SEQUENCE ZP_WORKTREE_LAST_ERROR
	unset ZP_WORKTREE_CONFLICT_COUNT ZP_WORKTREE_CONFLICT_KIND ZP_WORKTREE_CONFLICT_IDENTITY_KIND
	unset ZP_WORKTREE_CONFLICT_IDENTITY_NAME ZP_WORKTREE_CONFLICT_TOKEN
	unset ZP_WORKTREE_AUTO_APPLY_DEFAULT ZP_WORKTREE_AUTO_APPLY_EFFECTIVE
	unset ZP_WORKTREE_ATTACH_OPERATION_ID ZP_WORKTREE_PREPARE_OPERATION_ID
	unset ZP_WORKTREE_ACK_OPERATION_ID ZP_WORKTREE_RESOLVE_OPERATION_ID
	unset ZP_WORKTREE_REPLY_PROTOCOL ZP_WORKTREE_REPLY_REVISION ZP_WORKTREE_REPLY_TOKEN
	unset ZP_WORKTREE_REPLY_FINGERPRINT ZP_WORKTREE_REPLY_COMPLETE
	unset ZP_WORKTREE_TRANSITION_DEADLINE ZP_WORKTREE_GUARD ZP_WORKTREE_UNSUPPORTED
	unset ZP_WORKTREE_BASELINE_FIELDS ZP_WORKTREE_BASELINE_COUNTS
	unset _ZP_WORKTREE_CAPTURE_FIELDS _ZP_WORKTREE_CAPTURE_COUNTS
	return 0
}

_zp_worktree_effective_auto_apply() {
  case "${ZSHPRO_AUTO_APPLY-}" in
    '') typeset -g ZP_WORKTREE_AUTO_APPLY_EFFECTIVE="$ZP_WORKTREE_AUTO_APPLY_DEFAULT" ;;
    true|false) typeset -g ZP_WORKTREE_AUTO_APPLY_EFFECTIVE="$ZSHPRO_AUTO_APPLY" ;;
    *)
      typeset -g ZP_WORKTREE_AUTO_APPLY_EFFECTIVE="$ZP_WORKTREE_AUTO_APPLY_DEFAULT"
      _zp_worktree_error "invalid ZSHPRO_AUTO_APPLY override; using persisted default" || :
      ;;
  esac
  [[ "$ZP_WORKTREE_AUTO_APPLY_EFFECTIVE" == true ]]
}

_zp_worktree_sync() {
	local mode="${1-}" ZP_WORKTREE_TRANSITION_DEADLINE='' ZP_WORKTREE_SKIP_UNCHANGED_PUBLISH=1
	(( ZP_WORKTREE_GUARD == 0 )) || return 0
	_zp_worktree_budget_begin ZP_WORKTREE_TRANSITION_DEADLINE || return 1
  typeset -g ZP_WORKTREE_GUARD=1
	{
		_zp_worktree_ensure_attached "$ZP_WORKTREE_TRANSITION_DEADLINE" || return 1
		if (( ZP_WORKTREE_ATTACHED_NOW )); then
			(( ZP_WORKTREE_RECONCILE_REQUIRED )) || return 0
			[[ -z "$mode" ]] || return 1
			_zp_worktree_pull || return 1
			return 0
		fi
		if [[ "$mode" == resolve ]]; then
      _zp_worktree_resolve_shared || return 1
    elif [[ -z "$mode" ]]; then
      _zp_worktree_publish || return 1
      _zp_worktree_pull || return 1
    else
      return 2
    fi
  } always {
    typeset -g ZP_WORKTREE_GUARD=0
  }
}

_zp_worktree_precmd() {
	local ZP_WORKTREE_TRANSITION_DEADLINE=''
	(( ZP_WORKTREE_GUARD == 0 )) || return 0
	_zp_worktree_budget_begin ZP_WORKTREE_TRANSITION_DEADLINE || return 0
  _zp_worktree_publish || :
  return 0
}

_zp_worktree_line_finish() {
	local ZP_WORKTREE_TRANSITION_DEADLINE=''
	(( ZP_WORKTREE_GUARD == 0 )) || return 0
	_zp_worktree_budget_begin ZP_WORKTREE_TRANSITION_DEADLINE || return 0
  if _zp_worktree_effective_auto_apply; then _zp_worktree_pull || :; fi
  return 0
}

zsh-pro() {
	# Exact explicit resolution form: zsh-pro sync --resolve shared
	local verb="${1-}" rc=0 status_output='' ZP_WORKTREE_TRANSITION_DEADLINE='' ZP_WORKTREE_SKIP_UNCHANGED_PUBLISH=1 _zp_worktree_dispatcher_marker=1
  case "$verb:$#" in
    sync:1) _zp_worktree_sync ; return $? ;;
    sync:3)
      if [[ "$2" == --resolve && "$3" == shared ]]; then _zp_worktree_sync resolve; return $?; fi
      _zp_worktree_error "usage: zsh-pro sync [--resolve shared]"
      return 2
      ;;
		sync:*) _zp_worktree_error "usage: zsh-pro sync [--resolve shared]"; return 2 ;;
		status:1)
			_zp_worktree_ensure_attached || :
			if _zp_worktree_valid_hex64 "$ZP_WORKTREE_SHELL_ID"; then
				status_output="$(ZSHPRO_SHELL_ID="$ZP_WORKTREE_SHELL_ID" command zsh-pro status)"
			else
				status_output="$(command zsh-pro status)"
			fi
			rc=$?
			if (( rc == 0 )); then _zp_worktree_refresh_auto_apply "$status_output"; print -r -- "$status_output"; fi
			status_output=''
			return "$rc"
			;;
		config:4)
			_zp_worktree_ensure_attached || :
			if _zp_worktree_valid_hex64 "$ZP_WORKTREE_SHELL_ID"; then
				ZSHPRO_SHELL_ID="$ZP_WORKTREE_SHELL_ID" command zsh-pro "$@"
			else
				command zsh-pro "$@"
			fi
			rc=$?
			if (( rc == 0 )) && [[ "$2" == set && "$3" == auto-apply && ( "$4" == true || "$4" == false ) ]]; then
				typeset -g ZP_WORKTREE_AUTO_APPLY_DEFAULT="$4"
			fi
			return "$rc"
			;;
		checkout:*|reset:*)
			case "$verb:$#" in
				checkout:2) [[ -n "$2" && "$2" != -b ]] || { _zp_worktree_error "usage: zsh-pro checkout [-b] <branch>"; return 2; } ;;
				checkout:3) [[ "$2" == -b && -n "$3" ]] || { _zp_worktree_error "usage: zsh-pro checkout [-b] <branch>"; return 2; } ;;
				reset:2) [[ "$2" == --hard ]] || { _zp_worktree_error "usage: zsh-pro reset --hard"; return 2; } ;;
				*) _zp_worktree_error "invalid worktree mutation syntax"; return 2 ;;
			esac
			_zp_worktree_budget_begin ZP_WORKTREE_TRANSITION_DEADLINE || return 1
			_zp_worktree_ensure_attached || return $?
			if (( ! ZP_WORKTREE_ATTACHED_NOW )); then _zp_worktree_publish || return $?; fi
      (( ZP_WORKTREE_CONFLICT_COUNT == 0 )) || return 1
			ZSHPRO_SHELL_ID="$ZP_WORKTREE_SHELL_ID" command zsh-pro "$@"
			rc=$?
			(( rc == 0 )) || return "$rc"
			_zp_worktree_pull || return $?
			return 0
      ;;
    *)
      _zp_worktree_ensure_attached || :
      if _zp_worktree_valid_hex64 "$ZP_WORKTREE_SHELL_ID"; then
        ZSHPRO_SHELL_ID="$ZP_WORKTREE_SHELL_ID" command zsh-pro "$@"
      else
        command zsh-pro "$@"
      fi
      return $?
      ;;
  esac
}

_zp_worktree_dispatcher_installed() {
	[[ "${functions[zsh-pro]-}" == *'_zp_worktree_dispatcher_marker=1'* ]]
}

_zp_worktree_needs_legacy_switch() {
	! _zp_worktree_dispatcher_installed || (( ZP_WORKTREE_UNSUPPORTED )) ||
		[[ "${ZP_ACTIVE_PROFILE+x}" == x || "${ZP_LAST_GOOD_PROFILE+x}" == x ]]
}

activate() {
	if (( $# != 1 )) || [[ -z "$1" ]]; then
		_zp_runtime_error 2 "usage: activate <profile>"
		return 0
	fi
	local name="$1" rc=1
	{
		if _zp_worktree_needs_legacy_switch; then
			if _zp_switch "$name"; then _zp_runtime_ok; fi
			return 0
		fi
		if zsh-pro checkout "$@"; then _zp_runtime_ok; return 0; fi
		rc=$?
		# Compatibility is limited to pre-worktree binaries (the Phase 5 test
		# shims return 64). A production shared-worktree failure never falls
		# back to the retired per-profile authority.
		if (( ZP_WORKTREE_UNSUPPORTED )); then
			if _zp_switch "$name"; then _zp_runtime_ok; fi
			return 0
		fi
		_zp_runtime_error "$rc" "shared checkout failed; shell state unchanged"
		return 0
	} always {
		unset REPLY
	}
}

checkout() {
	if (( $# != 1 )) || [[ -z "$1" ]]; then
		_zp_runtime_error 2 "usage: checkout <profile>"
		return 0
	fi
	local name="$1" rc=1
	{
		if _zp_worktree_needs_legacy_switch; then
			if _zp_switch "$name"; then _zp_runtime_ok; fi
			return 0
		fi
		if zsh-pro checkout "$@"; then _zp_runtime_ok; return 0; fi
		rc=$?
		if (( ZP_WORKTREE_UNSUPPORTED )); then
			if _zp_switch "$name"; then _zp_runtime_ok; fi
			return 0
		fi
		_zp_runtime_error "$rc" "shared checkout failed; shell state unchanged"
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
	local rc=1 recovered=0
	{
		if [[ "${ZP_RECOVERY_REVERSE_FN+x}" == x ]]; then
			if _zp_worktree_protected_call _zp_recover_failed_target; then recovered=1; else
				rc=$?
				_zp_runtime_error "$rc" "target apply cleanup failed; recovery is retained; repair the terminal state and run deactivate again"
				return 0
			fi
		fi
		if _zp_worktree_dispatcher_installed && [[ "${ZP_ACTIVE_REVERSE_FN-}" == __zp_worktree_reverse_* ]]; then
			if _zp_worktree_protected_call _zp_run_retained_reverse "$ZP_ACTIVE_REVERSE_FN"; then
				_zp_worktree_disable
				_zp_runtime_ok
				return 0
			fi
			rc=$?
			_zp_runtime_error "$rc" "worktree deactivation failed; recovery is retained"
			return 0
		fi
		if (( recovered )) && _zp_worktree_dispatcher_installed && [[ "${ZP_ACTIVE_PROFILE+x}" != x ]]; then
			_zp_worktree_disable
			_zp_runtime_ok
			return 0
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
    if _zp_run_bounded "$timeout" listing zsh-pro list "$@"; then
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

_zp_worktree_ensure_attached() {
	local deadline="${1-${ZP_WORKTREE_TRANSITION_DEADLINE-}}" rc=1 xtrace_was_on=0 history_pushed=0
	[[ -n "$deadline" ]] || _zp_worktree_budget_begin deadline || return 1
	{
		[[ -o xtrace ]] && xtrace_was_on=1
		setopt NOXTRACE 2>/dev/null || return 1
		if fc -p 2>/dev/null; then history_pushed=1; else return 1; fi
		_zp_worktree_ensure_attached_impl "$deadline"
		rc=$?
	} always {
		if (( history_pushed )); then fc -P 2>/dev/null || :; fi
		if (( xtrace_was_on )); then setopt XTRACE 2>/dev/null || :; else setopt NOXTRACE 2>/dev/null || :; fi
		unset REPLY
	}
	return "$rc"
}

status() {
	if (( $# != 0 )); then
		_zp_runtime_error 2 "usage: status"
		return 0
	fi
	if (( ! ZP_WORKTREE_ATTACHED )) || ! _zp_worktree_dispatcher_installed; then
		if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then print -r -- "$ZP_ACTIVE_PROFILE"; else print -r -- main; fi
		return 0
	fi
	if zsh-pro status "$@"; then return 0; fi
	if (( ZP_WORKTREE_UNSUPPORTED )); then
		if [[ "${ZP_ACTIVE_PROFILE+x}" == x ]]; then print -r -- "$ZP_ACTIVE_PROFILE"; else print -r -- main; fi
	else
		_zp_runtime_error $? "status failed"
	fi
	return 0
}

# Cooperative registration performs no helper invocation and starts no
# process. The handlers themselves run only at safe foreground boundaries.
autoload -Uz add-zsh-hook
add-zsh-hook -d precmd _zp_worktree_precmd 2>/dev/null || :
add-zsh-hook precmd _zp_worktree_precmd
if (( ${+widgets} )); then
  autoload -Uz add-zle-hook-widget
  add-zle-hook-widget -d line-finish _zp_worktree_line_finish 2>/dev/null || :
  add-zle-hook-widget line-finish _zp_worktree_line_finish
fi
`

// HookScript returns the complete sourced runtime loader.
func (Provider) HookScript() string { return loaderScript }
