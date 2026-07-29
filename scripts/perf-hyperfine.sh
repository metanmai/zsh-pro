#!/usr/bin/env bash
set -euo pipefail

# CI backstop for the always-run structural Go assertion. This never reads or
# writes the developer's real HOME/.zshrc: both fixtures live under mktemp.
if ! command -v hyperfine >/dev/null 2>&1; then
  echo "SKIP: hyperfine absent; structural zero-subprocess test is the gate (EPOCHREALTIME proxy: ~0.01 ms)."
  exit 0
fi

bin=${ZSHPRO_BIN:?set ZSHPRO_BIN to an absolute zsh-pro binary}
case "$bin" in
  /*) ;;
  *) echo "ZSHPRO_BIN must be an absolute path" >&2; exit 1 ;;
esac
[[ -x "$bin" ]] || { echo "ZSHPRO_BIN is not executable: $bin" >&2; exit 1; }

repo_dir=$(cd "$(dirname "$0")/.." && pwd)
bin_dir=$(dirname "$bin")
with_dir=$(mktemp -d)
without_dir=$(mktemp -d)
result=$(mktemp)
trap 'rm -rf "$with_dir" "$without_dir" "$result"' EXIT

HOME="$with_dir" ZDOTDIR="$with_dir" PATH="$bin_dir:$PATH" "$bin" install >/dev/null
touch "$without_dir/.zshrc"
HOME="$with_dir" ZDOTDIR="$with_dir" PATH="$bin_dir:$PATH" zsh -i -c 'whence -w activate' | grep -qx 'activate: function'

with_command="HOME='$with_dir' ZDOTDIR='$with_dir' PATH='$bin_dir':\$PATH zsh -i -c exit"
without_command="HOME='$without_dir' ZDOTDIR='$without_dir' PATH='$bin_dir':\$PATH zsh -i -c exit"
hyperfine --warmup 3 --export-json "$result" \
  "$with_command" \
  "$without_command"
added_ms=$(go run "$repo_dir/core/perf/hyperfine.go" "$result" "$with_command" "$without_command")
awk -v added="$added_ms" 'BEGIN { printf "added startup mean: %.3f ms\n", added; exit !(added < 10) }'
