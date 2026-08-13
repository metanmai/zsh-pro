#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: scripts/uat-phase06-user.sh <shell|secret|idempotence|append>" >&2
  exit 2
}

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "phase06 UAT: required tool not found: $1" >&2
    exit 1
  fi
}

run_shell_test() {
  require_tool go
  require_tool git
  require_tool zsh

  local repo_root uat_root real_git real_zsh
  repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
  uat_root="$(mktemp -d /tmp/zsh-pro-user-uat.XXXXXX)"
  real_git="$(command -v git)"
  real_zsh="$(command -v zsh)"

  mkdir -p "$uat_root/home" "$uat_root/bin"
  (
    cd "$repo_root"
    go build -o "$uat_root/bin/zsh-pro" ./core/cmd/zsh-pro
    cp core/cli/testdata/ingest/real.zshrc "$uat_root/home/.zshrc"
  )
  ln -s "$real_git" "$uat_root/bin/git"
  ln -s "$real_zsh" "$uat_root/bin/zsh"

  echo "uat sandbox: $uat_root"
  env HOME="$uat_root/home" \
    ZDOTDIR="$uat_root/home" \
    ZSHPRO_HOME="$uat_root/home/.zsh-pro" \
    PATH="$uat_root/bin:$PATH" \
    "$uat_root/bin/zsh-pro" ingest "$uat_root/home/.zshrc"

  env HOME="$uat_root/home" \
    ZDOTDIR="$uat_root/home" \
    ZSHPRO_HOME="$uat_root/home/.zsh-pro" \
    PHASE6_STUB_BIN="$uat_root/bin" \
    PATH="$uat_root/bin:$PATH" \
    zsh -f <<'ZSH'
source "$ZDOTDIR/.zshrc"
checkout main
status
phase6_alias
phase6_function
print -r -- "$PHASE6_FUNCTION_RESULT"
print -r -- "${(j: :)PHASE6_ORDER_LEDGER}"
print -r -- "$PHASE6_IMPERATIVE_RESULT"
whence -w activate
whence -w checkout
deactivate
print -r -- "runtime_status=$ZP_LAST_RUNTIME_STATUS"
ZSH

  ln -sfn "$uat_root" /tmp/zsh-pro-phase06-user-uat.latest
  echo "uat sandbox retained: $uat_root"
}

latest_sandbox() {
  local latest_link=/tmp/zsh-pro-phase06-user-uat.latest
  local uat_root
  if [[ -n "${ZSHPRO_UAT_ROOT-}" ]]; then
    if [[ ! -d "$ZSHPRO_UAT_ROOT" ]]; then
      echo "phase06 UAT: requested sandbox does not exist" >&2
      exit 1
    fi
    printf '%s\n' "$ZSHPRO_UAT_ROOT"
    return
  fi
  if [[ ! -L "$latest_link" ]]; then
    echo "phase06 UAT: no retained sandbox; run the shell test first" >&2
    exit 1
  fi
  uat_root="$(readlink -f "$latest_link")"
  if [[ ! -d "$uat_root" ]]; then
    echo "phase06 UAT: retained sandbox has expired; run the shell test again" >&2
    exit 1
  fi
  printf '%s\n' "$uat_root"
}

run_secret_test() {
  require_tool git

  local uat_root store_root profile
  uat_root="$(latest_sandbox)"
  store_root="$uat_root/home/.zsh-pro"
  profile="$(git --git-dir="$store_root" show main:profile.json)"

  if git --git-dir="$store_root" grep -Fq '__PHASE6_LITERAL_SECRET__' main --; then
    echo "secret boundary: FAIL - literal found in committed main" >&2
    exit 1
  fi
  if [[ "$profile" != *'"startLine": 25'* ]] ||
    [[ "$profile" != *'"key": "PHASE6_API_TOKEN"'* ]] ||
    [[ "$profile" != *'"kind": "file"'* ]]; then
    echo "secret boundary: FAIL - expected value-free SecretRef not found" >&2
    exit 1
  fi

  echo "secret report: PHASE6_API_TOKEN (line 25)"
  echo "committed main: SecretRef=file:PHASE6_API_TOKEN"
  echo "committed main: literal absent"
  echo "secret boundary: PASS"
}

run_idempotence_test() {
  require_tool git

  local uat_root target binary before_hash after_hash output begin_count end_count
  uat_root="$(latest_sandbox)"
  target="$uat_root/home/.zshrc"
  binary="$uat_root/bin/zsh-pro"
  before_hash="$(git hash-object "$target")"

  output="$(env HOME="$uat_root/home" \
    ZDOTDIR="$uat_root/home" \
    ZSHPRO_HOME="$uat_root/home/.zsh-pro" \
    PATH="$uat_root/bin:$PATH" \
    "$binary" ingest "$target")"
  after_hash="$(git hash-object "$target")"
  begin_count="$(grep -Fxc '# >>> zsh-pro >>>' "$target")"
  end_count="$(grep -Fxc '# <<< zsh-pro <<<' "$target")"

  if [[ "$output" != *'profile committed: yes'* ]] ||
    [[ "$output" != *'startup installed: yes'* ]]; then
    echo "idempotence: FAIL - re-ingest did not report success" >&2
    exit 1
  fi
  if [[ "$before_hash" != "$after_hash" ]]; then
    echo "idempotence: FAIL - installed .zshrc changed" >&2
    exit 1
  fi
  if [[ "$begin_count" != 1 || "$end_count" != 1 ]]; then
    echo "idempotence: FAIL - managed marker count changed" >&2
    exit 1
  fi

  echo "re-ingest: profile committed and startup installed"
  echo "installed .zshrc hash unchanged: PASS"
  echo "managed marker pairs: 1"
  echo "idempotence: PASS"
}

run_append_test() {
  require_tool git
  require_tool zsh

  local uat_root target binary before_suffix after_suffix output observed
  local begin_count end_count
  uat_root="$(latest_sandbox)"
  target="$uat_root/home/.zshrc"
  binary="$uat_root/bin/zsh-pro"

  if ! grep -Fq '# phase06 user append' "$target"; then
    printf '\n%s\n%s\n' \
      '# phase06 user append' \
      "export PHASE6_USER_APPEND='preserved'" >>"$target"
  fi
  before_suffix="$(tail -n 2 "$target" | git hash-object --stdin)"

  output="$(env HOME="$uat_root/home" \
    ZDOTDIR="$uat_root/home" \
    ZSHPRO_HOME="$uat_root/home/.zsh-pro" \
    PATH="$uat_root/bin:$PATH" \
    "$binary" ingest "$target")"
  after_suffix="$(tail -n 2 "$target" | git hash-object --stdin)"
  begin_count="$(grep -Fxc '# >>> zsh-pro >>>' "$target")"
  end_count="$(grep -Fxc '# <<< zsh-pro <<<' "$target")"
  observed="$(env HOME="$uat_root/home" \
    ZDOTDIR="$uat_root/home" \
    ZSHPRO_HOME="$uat_root/home/.zsh-pro" \
    PHASE6_STUB_BIN="$uat_root/bin" \
    PATH="$uat_root/bin:$PATH" \
    zsh -f -c 'source "$ZDOTDIR/.zshrc"; print -r -- "$PHASE6_USER_APPEND"')"

  if [[ "$output" != *'warning: zsh-pro: ordinary startup content remains after the managed loader block'* ]]; then
    echo "append preservation: FAIL - warning not reported" >&2
    exit 1
  fi
  if [[ "$before_suffix" != "$after_suffix" ]]; then
    echo "append preservation: FAIL - post-END bytes changed" >&2
    exit 1
  fi
  if [[ "$observed" != preserved ]]; then
    echo "append preservation: FAIL - appended startup content did not run" >&2
    exit 1
  fi
  if [[ "$begin_count" != 1 || "$end_count" != 1 ]]; then
    echo "append preservation: FAIL - managed marker count changed" >&2
    exit 1
  fi

  echo "post-END warning: PASS"
  echo "post-END bytes unchanged: PASS"
  echo "post-END value after startup: preserved"
  echo "managed marker pairs: 1"
  echo "append preservation: PASS"
}

case "${1:-}" in
  shell)
    run_shell_test
    ;;
  secret)
    run_secret_test
    ;;
  idempotence)
    run_idempotence_test
    ;;
  append)
    run_append_test
    ;;
  *)
    usage
    ;;
esac
