package zsh

// loaderScript is sourced into an interactive zsh. It deliberately avoids
// emulate -L/LOCAL_OPTIONS: state changes made by emitted apply/deactivate
// blocks must persist in the caller's terminal after these functions return.
const loaderScript = `
typeset -g ZP_UNSET_SENTINEL='__zsh_pro_unset_7c5a0a15__'

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
  [[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"
}

_zp_emit() {
  local mode="$1" name="$2" code rc
  code="$(command zsh-pro emit "$mode" "$name")"
  rc=$?
  if (( rc != 0 )) || [[ -z "$code" ]]; then
    print -u2 -- "zsh-pro: emit $mode failed; shell state unchanged"
    return 1
  fi
  REPLY="$code"
}

_zp_eval_block() {
  local block="$1" applied="$2" tmp rc
  [[ -n "$block" ]] || {
    print -u2 -- "zsh-pro: emitted empty shell source; shell state unchanged"
    return 1
  }
  tmp="${TMPDIR:-/tmp}/zsh-pro-eval-${RANDOM}-$$"
  (umask 077; print -rn -- "$block" > "$tmp") || {
    print -u2 -- "zsh-pro: unable to stage emitted shell source"
    return 1
  }
  command zsh -n "$tmp"
  rc=$?
  rm -f -- "$tmp"
  if (( rc != 0 )); then
    print -u2 -- "zsh-pro: emitted shell source failed validation; shell state unchanged"
    return 1
  fi
  setopt NOXTRACE 2>/dev/null
  fc -p 2>/dev/null
  eval "$block"
  rc=$?
  fc -P 2>/dev/null
  if (( rc != 0 )); then
    print -u2 -- "zsh-pro: switch failed at runtime; shell may be partially changed; run checkout ${ZP_LAST_GOOD_PROFILE:-main}"
    return "$rc"
  fi
  [[ -z "$applied" ]] || typeset -g ZP_LAST_GOOD_PROFILE="$applied"
}

activate() {
	local name="$1" block
	[[ -n "$name" ]] || return 2
	_zp_prepare_eval_state
	_zp_emit apply "$name" || return $?
	block="$REPLY"$'\n'"zp_apply"
	_zp_eval_block "$block" "$name" || return $?
	export ZSHPRO_PROFILE="$name"
}

checkout() {
  local name="$1" prior="${ZSHPRO_PROFILE-}" target priorBlock block
  [[ -n "$name" ]] || return 2
  _zp_prepare_eval_state
  # The emit-apply command validates the requested Store branch before returning source.
	_zp_emit apply "$name" || return $?
	target="$REPLY"
	if [[ -n "$prior" ]]; then
		_zp_emit deactivate "$prior" || return $?
		priorBlock="$REPLY"
	fi
	block="$priorBlock"$'\n'
	if [[ -n "$prior" ]]; then block+="zp_deactivate"$'\n'; fi
	block+="$target"$'\n'"zp_apply"
	_zp_eval_block "$block" "$name" || return $?
	export ZSHPRO_PROFILE="$name"
}

deactivate() {
	local name="${ZSHPRO_PROFILE-}" block
	[[ -n "$name" ]] || return 0
	_zp_prepare_eval_state
	_zp_emit deactivate "$name" || return $?
	block="$REPLY"$'\n'"zp_deactivate"
	_zp_eval_block "$block" "" || return $?
	unset ZSHPRO_PROFILE
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
