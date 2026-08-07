#!/usr/bin/env bash
set -eu

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

test_root=$(mktemp -d "${TMPDIR:-/tmp}/zsh-pro-phase06-verifier-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

mkdir -p "$test_root/core" "$test_root/scripts"
cp "$(dirname "$0")/verify-phase06-macos-runtime.sh" "$test_root/scripts/verify-phase06-macos-runtime.sh"
printf 'package tracked\n' >"$test_root/core/tracked.go"
printf 'core/generated.go\n' >"$test_root/.gitignore"

git -C "$test_root" init -q
git -C "$test_root" add .gitignore core/tracked.go scripts/verify-phase06-macos-runtime.sh
git -C "$test_root" -c user.name='Phase 6 Verifier Test' -c user.email='phase6-verifier@example.invalid' commit -qm 'test fixture'
expected_sha=$(git -C "$test_root" rev-parse HEAD)

# Ignored files are still untracked implementation inputs: a Go source file
# below core can affect the tested package even when Git status hides it.
printf 'package generated\n' >"$test_root/core/generated.go"
if output=$(cd "$test_root" && GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh "$expected_sha" 2>&1); then
  fail 'verifier accepted an untracked implementation file'
fi
printf '%s\n' "$output" | grep -qx 'status: BLOCKED' || fail 'verifier did not emit BLOCKED status'
printf '%s\n' "$output" | grep -qx 'reason: untracked implementation or verifier files are present' || {
  fail 'verifier did not report the closed untracked-input reason'
}
if printf '%s\n' "$output" | grep -q 'generated.go'; then
  fail 'verifier disclosed the untracked path'
fi

printf 'PASS: phase 6 verifier rejects untracked implementation inputs\n'
