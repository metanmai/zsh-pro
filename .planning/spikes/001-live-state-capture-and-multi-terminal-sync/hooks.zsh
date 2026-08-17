# Spike 001 live-state capture and synchronization hooks.
#
# Required globals before sourcing:
#   ZP_SPIKE_ROOT, ZP_SPIKE_HELPER, ZP_SPIKE_SHELL

zmodload zsh/parameter 2>/dev/null || return 1

typeset -g ZP_SPIKE_AUTO_APPLY=${ZP_SPIKE_AUTO_APPLY:-1}
typeset -g ZP_SPIKE_TIMEOUT_SECONDS=${ZP_SPIKE_TIMEOUT_SECONDS:-0.25}
typeset -g ZP_SPIKE_LAST_ERROR=''
typeset -g ZP_SPIKE_LAST_HOOK_STATUS=0
typeset -ga ZP_SPIKE_SUPPORTED_OPTIONS=(
  autocd
  extendedglob
  interactivecomments
  nomatch
  nullglob
  pushdsilent
)

_zp_spike_exclusion_reason() {
  local name=$1 upper=${(U)1}

  if [[ $name == ZP_* || $name == _ZP_* ]]; then
    REPLY=bookkeeping
    return 0
  fi
  case $upper in
    (*TOKEN*|*SECRET*|*PASSWORD*|*PASSWD*|*CREDENTIAL*|*COOKIE*|*SESSION*|*PRIVATE_KEY*|*ACCESS_KEY*|*API_KEY*|*AUTH*)
      REPLY=secret-name
      return 0
      ;;
  esac
  case $name in
    (_|ARGC|COLUMNS|EPOCHREALTIME|EPOCHSECONDS|HISTCMD|LINENO|LINES|OLDPWD|PPID|PWD|RANDOM|SECONDS|SHLVL|TTY|TTYIDLE|pipestatus|status|ZSH_SUBSHELL))
      REPLY=volatile
      return 0
      ;;
  esac
  REPLY=''
  return 1
}

_zp_spike_snapshot() {
  local name descriptor value reason option
  local -a values

  # Only exported scalar-like parameters are in the environment category.
  # PATH/FPATH are emitted from their tied arrays so order and empty entries
  # remain semantic rather than colon-string accidents.
  for name in ${(ok)parameters}; do
    descriptor=${parameters[$name]}
    [[ $descriptor == *export* ]] || continue
    [[ $descriptor != *array* && $descriptor != *association* ]] || continue
    [[ $name != PATH && $name != FPATH ]] || continue
    if _zp_spike_exclusion_reason "$name"; then
      reason=$REPLY
      # Report only experiment/loader identities. Unmanaged environment names
      # are never admitted into the snapshot or its forensic metadata.
      if [[ $name == SPIKE_* || $name == ZP_* || $name == _ZP_* ]]; then
        builtin printf '%s\0%s\0%s\0%s\0' exclude env "$name" "$reason" || return 1
      fi
      continue
    fi
    # This prototype namespace stands in for the production materialized
    # profile's identity-admission policy. Arbitrary inherited environment is
    # deliberately not persisted merely because it exists in the process.
    [[ $name == SPIKE_* ]] || continue
    value=${(P)name}
    builtin printf '%s\0%s\0%s\0%s\0' entry env "$name" "$value" || return 1
  done

  for name in ${(ok)aliases}; do
    [[ $name == spike_* ]] || continue
    builtin printf '%s\0%s\0%s\0%s\0' entry alias "$name" "${aliases[$name]}" || return 1
  done

  for name in ${(ok)functions}; do
    [[ $name == spike_* ]] || continue
    builtin printf '%s\0%s\0%s\0%s\0' entry function "$name" "${functions[$name]}" || return 1
  done

  values=( "${path[@]}" )
  builtin printf '%s\0%s\0%s\0' array PATH "${#values}" || return 1
  for value in "${values[@]}"; do builtin printf '%s\0' "$value" || return 1; done

  values=( "${fpath[@]}" )
  builtin printf '%s\0%s\0%s\0' array FPATH "${#values}" || return 1
  for value in "${values[@]}"; do builtin printf '%s\0' "$value" || return 1; done

  for option in "${ZP_SPIKE_SUPPORTED_OPTIONS[@]}"; do
    builtin printf '%s\0%s\0%s\0%s\0' entry option "$option" "${options[$option]}" || return 1
  done
}

_zp_spike_helper() {
  command timeout "$ZP_SPIKE_TIMEOUT_SECONDS" "$ZP_SPIKE_HELPER" "$@"
}

_zp_spike_apply_pending() {
  local patch="$ZP_SPIKE_ROOT/shells/$ZP_SPIKE_SHELL.patch.zsh"
  local validate_status source_status helper_status
  local _ZP_SPIKE_PATCH_NAME
  [[ -s $patch ]] || return 0

  command timeout "$ZP_SPIKE_TIMEOUT_SECONDS" zsh -n < "$patch" >/dev/null 2>&1
  validate_status=$?
  if (( validate_status != 0 )); then
    ZP_SPIKE_LAST_ERROR="generated patch failed syntax validation (status $validate_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$validate_status
    return 0
  fi

  builtin source "$patch" >/dev/null 2>&1
  source_status=$?
  if (( source_status != 0 )); then
    ZP_SPIKE_LAST_ERROR="generated patch failed while sourcing (status $source_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$source_status
    return 0
  fi

  _zp_spike_snapshot | _zp_spike_helper ack --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" >/dev/null 2>&1
  helper_status=${pipestatus[-1]}
  if (( helper_status != 0 )); then
    ZP_SPIKE_LAST_ERROR="patch applied but acknowledgement failed (status $helper_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
    return 0
  fi

  ZP_SPIKE_LAST_ERROR=''
  ZP_SPIKE_LAST_HOOK_STATUS=0
  return 0
}

_zp_spike_attach() {
  local helper_status
  if [[ -z $ZP_SPIKE_ROOT || -z $ZP_SPIKE_HELPER || -z $ZP_SPIKE_SHELL ]]; then
    ZP_SPIKE_LAST_ERROR='ZP_SPIKE_ROOT, ZP_SPIKE_HELPER, and ZP_SPIKE_SHELL are required'
    ZP_SPIKE_LAST_HOOK_STATUS=2
    return 0
  fi
  _zp_spike_snapshot | _zp_spike_helper attach --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" >/dev/null 2>&1
  helper_status=${pipestatus[-1]}
  if (( helper_status != 0 )); then
    ZP_SPIKE_LAST_ERROR="attach failed (status $helper_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
    return 0
  fi
  ZP_SPIKE_LAST_ERROR=''
  ZP_SPIKE_LAST_HOOK_STATUS=0
  return 0
}

_zp_spike_cycle() {
  local apply=false helper_status
  [[ $ZP_SPIKE_AUTO_APPLY == 1 ]] && apply=true

  _zp_spike_snapshot | _zp_spike_helper cycle --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" --apply="$apply" --previous-error "$ZP_SPIKE_LAST_ERROR" >/dev/null 2>&1
  helper_status=${pipestatus[-1]}
  if (( helper_status != 0 )); then
    ZP_SPIKE_LAST_ERROR="sync cycle failed open (status $helper_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
    return 0
  fi

  ZP_SPIKE_LAST_ERROR=''
  ZP_SPIKE_LAST_HOOK_STATUS=0
  if [[ $apply == true ]]; then
    _zp_spike_apply_pending
  fi
  return 0
}

_zp_spike_precmd() {
  _zp_spike_cycle
  return 0
}

_zp_spike_line_finish() {
  # This runs after ZLE accepts the line but before zsh parses/expands it.
  # It pulls only; precmd owns publication of the previous command's result.
  if [[ $ZP_SPIKE_AUTO_APPLY == 1 ]]; then
    local helper_status
    _zp_spike_snapshot | _zp_spike_helper cycle --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" --apply=true --previous-error "$ZP_SPIKE_LAST_ERROR" >/dev/null 2>&1
    helper_status=${pipestatus[-1]}
    if (( helper_status == 0 )); then
      ZP_SPIKE_LAST_ERROR=''
      ZP_SPIKE_LAST_HOOK_STATUS=0
      _zp_spike_apply_pending
    else
      ZP_SPIKE_LAST_ERROR="line-finish sync failed open (status $helper_status)"
      ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
    fi
  fi
  return 0
}

_zp_spike_sync() {
  local helper_status
  _zp_spike_snapshot | _zp_spike_helper cycle --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" --apply=true --previous-error "$ZP_SPIKE_LAST_ERROR" >/dev/null 2>&1
  helper_status=${pipestatus[-1]}
  if (( helper_status == 0 )); then
    ZP_SPIKE_LAST_ERROR=''
    ZP_SPIKE_LAST_HOOK_STATUS=0
    _zp_spike_apply_pending
  else
    ZP_SPIKE_LAST_ERROR="explicit sync failed open (status $helper_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
  fi
  return 0
}

_zp_spike_resolve_shared() {
  local helper_status
  _zp_spike_snapshot | _zp_spike_helper resolve-shared --root "$ZP_SPIKE_ROOT" --shell "$ZP_SPIKE_SHELL" >/dev/null 2>&1
  helper_status=${pipestatus[-1]}
  if (( helper_status == 0 )); then
    _zp_spike_apply_pending
  else
    ZP_SPIKE_LAST_ERROR="shared conflict resolution failed open (status $helper_status)"
    ZP_SPIKE_LAST_HOOK_STATUS=$helper_status
  fi
  return 0
}

_zp_spike_set_auto_apply() {
  case $1 in
    (on|1|true) ZP_SPIKE_AUTO_APPLY=1 ;;
    (off|0|false) ZP_SPIKE_AUTO_APPLY=0 ;;
    (*) builtin print -u2 -r -- 'usage: _zp_spike_set_auto_apply <on|off>'; return 2 ;;
  esac
  return 0
}

_zp_spike_status() {
  "$ZP_SPIKE_HELPER" status --root "$ZP_SPIKE_ROOT"
}

autoload -Uz add-zsh-hook add-zle-hook-widget
add-zsh-hook precmd _zp_spike_precmd
add-zle-hook-widget line-finish _zp_spike_line_finish
