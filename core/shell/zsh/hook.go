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

_zp_eval_emit() {
  local mode="$1" name="$2" code
  _zp_prepare_eval_state
  code="$(command zsh-pro emit "$mode" "$name")" || return $?
  [[ -n "$code" ]] || return 1
  eval "$code" || return $?
  if [[ "$mode" == apply ]]; then
    zp_apply
  else
    zp_deactivate
  fi
}

activate() {
  local name="$1"
  [[ -n "$name" ]] || return 2
  _zp_eval_emit apply "$name" || return $?
  export ZSHPRO_PROFILE="$name"
}

checkout() {
  local name="$1" prior="${ZSHPRO_PROFILE-}" target priorBlock block
  [[ -n "$name" ]] || return 2
  _zp_prepare_eval_state
  # The emit-apply command validates the requested Store branch before returning source.
  target="$(command zsh-pro emit apply "$name")" || return $?
  [[ -n "$target" ]] || return 1
  if [[ -n "$prior" ]]; then
    priorBlock="$(command zsh-pro emit deactivate "$prior")" || return $?
    [[ -n "$priorBlock" ]] || return 1
  fi
  block="$priorBlock"$'\n'"$target"
  # Plan 05-02 adds one-block syntax validation and last-good handling here.
  eval "$block" || return $?
  if [[ -n "$prior" ]]; then zp_deactivate || return $?; fi
  zp_apply || return $?
  export ZSHPRO_PROFILE="$name"
}

deactivate() {
  local name="${ZSHPRO_PROFILE-}"
  [[ -n "$name" ]] || return 0
  _zp_eval_emit deactivate "$name" || return $?
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
