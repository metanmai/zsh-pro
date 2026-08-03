# Phase 6 authored real-world startup fixture.

typeset -ga PHASE6_ORDER_LEDGER=()
export PHASE6_FIRST='ready'
PHASE6_ORDER_LEDGER+=('definition')

alias phase6_alias='print -r -- alias-ok'

phase6_function() {
  typeset -g PHASE6_FUNCTION_RESULT='function-ok'
}
phase6_function

export PATH="${PHASE6_STUB_BIN}:/phase6/first:/phase6/second"
setopt HIST_IGNORE_DUPS

typeset -gA PHASE6_OPAQUE=(alpha one beta two)
typeset -g PHASE6_OPAQUE_RESULT='one-two'

if [[ "$PHASE6_FIRST" == 'ready' ]]; then
  PHASE6_ORDER_LEDGER+=('use')
  typeset -g PHASE6_IMPERATIVE_RESULT='definition-before-use'
fi

export PHASE6_API_TOKEN='__PHASE6_LITERAL_SECRET__'
export PHASE6_DYNAMIC_TOKEN="${PHASE6_DYNAMIC_SOURCE:-dynamic-fallback}"

# Ingest must not execute this source. The explicit zsh -f oracle does.
if [[ -n "${PHASE6_SOURCE_EXECUTION_CANARY-}" ]]; then
  print -rn -- sourced >| "$PHASE6_SOURCE_EXECUTION_CANARY"
fi

# This unmanaged execution canary is deliberately false in every test run.
if [[ "${PHASE6_RUN_SUBPROCESS_CANARY-0}" == 1 ]]; then
  zsh-pro phase6-subprocess-canary
fi

# >>> zsh-pro >>>
if [[ -z "${ZSHPRO_DISABLE-}" ]]; then
  if [[ -f "${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh" && -r "${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh" ]]; then
    if source "${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh"; then
      :
    else
      :
    fi
  fi
fi
true
# <<< zsh-pro <<<