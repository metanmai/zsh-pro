#!/usr/bin/env bash
set -euo pipefail

# CI backstop for the always-run structural Go assertion. This never reads or
# writes the developer's real HOME/.zshrc: both fixtures live under mktemp.
if ! command -v hyperfine >/dev/null 2>&1; then
  echo "SKIP: hyperfine absent; structural zero-subprocess test is the gate (EPOCHREALTIME proxy: ~0.01 ms)."
  exit 0
fi

bin=${ZSHPRO_BIN:?set ZSHPRO_BIN to an absolute zsh-pro binary}
bin_dir=$(dirname "$bin")
with_dir=$(mktemp -d)
without_dir=$(mktemp -d)
result=$(mktemp)
trap 'rm -rf "$with_dir" "$without_dir" "$result"' EXIT

HOME="$with_dir" ZDOTDIR="$with_dir" PATH="$bin_dir:$PATH" "$bin" install >/dev/null
touch "$without_dir/.zshrc"
HOME="$with_dir" ZDOTDIR="$with_dir" PATH="$bin_dir:$PATH" zsh -i -c 'whence -w activate' | grep -qx 'activate: function'

hyperfine --warmup 3 --export-json "$result" \
  "HOME='$with_dir' ZDOTDIR='$with_dir' PATH='$bin_dir':\$PATH zsh -i -c exit" \
  "HOME='$without_dir' ZDOTDIR='$without_dir' PATH='$bin_dir':\$PATH zsh -i -c exit"
mapfile -t means < <(grep -oE '"mean":[0-9.]+' "$result" | sed 's/.*://')
(( ${#means[@]} == 2 )) || { echo "unable to read hyperfine means" >&2; exit 1; }
awk -v with="${means[0]}" -v without="${means[1]}" 'BEGIN { added=(with-without)*1000; printf "added startup mean: %.3f ms\n", added; exit !(added < 10) }'
