#!/usr/bin/env bash
set -u

blocked() {
  reason=$1
  printf 'status: BLOCKED\n'
  printf 'reason: %s\n' "$reason"
  exit 1
}

if [ "$#" -ne 1 ]; then
  blocked 'usage: scripts/verify-phase06-macos-runtime.sh <exact-implementation-sha>'
fi

expected_sha=$1
case "$expected_sha" in
  *[!0-9a-f]*|'') blocked 'implementation SHA must be a full lowercase hexadecimal commit ID' ;;
esac
if [ "${#expected_sha}" -ne 40 ]; then
  blocked 'implementation SHA must contain exactly 40 hexadecimal characters'
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || blocked 'not running in the zsh-pro Git repository'
cd "$repo_root" || blocked 'cannot enter repository root'
actual_sha=$(git rev-parse HEAD 2>/dev/null) || blocked 'cannot resolve repository HEAD'
if [ "$actual_sha" != "$expected_sha" ]; then
  blocked "HEAD $actual_sha does not match requested implementation SHA $expected_sha"
fi
if ! git diff --quiet "$expected_sha" -- core scripts/verify-phase06-macos-runtime.sh; then
  blocked 'implementation or verifier differs from the requested SHA'
fi
if [ "$(uname -s)" != Darwin ]; then
  blocked 'native Darwin execution is required; cross-builds do not qualify'
fi
if [ "${GOTOOLCHAIN-}" != local ]; then
  blocked 'GOTOOLCHAIN=local must be set by the invoking operator'
fi
if [ "$(GOTOOLCHAIN=local go env GOTOOLCHAIN)" != local ]; then
  blocked 'Go did not retain the local-toolchain setting'
fi
goversion=$(GOTOOLCHAIN=local go env GOVERSION) || blocked 'cannot inspect local Go version'
case "$goversion" in
  go1.25.*) ;;
  *) blocked "local Go 1.25.x is required; found $goversion" ;;
esac
if [ "$(GOTOOLCHAIN=local go env GOHOSTOS)" != darwin ] || [ "$(GOTOOLCHAIN=local go env GOOS)" != darwin ]; then
  blocked 'native Darwin host and target are both required'
fi

filesystem=$(stat -f '%T' "$repo_root" 2>/dev/null || printf unknown)
printf 'implementation_sha: %s\n' "$actual_sha"
printf 'observed_at_utc: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
printf 'native_os: macOS %s\n' "$(sw_vers -productVersion 2>/dev/null || uname -r)"
printf 'native_arch: %s\n' "$(uname -m)"
printf 'filesystem: %s\n' "$filesystem"
printf 'go_toolchain: local\n'
printf 'go_env_goversion: %s\n' "$goversion"
printf 'exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh %s\n' "$actual_sha"

run_exact() {
  package=$1
  test_name=$2
  evidence_key=$3
  listed=$(GOTOOLCHAIN=local go test "$package" -list "^${test_name}$" 2>&1) || {
    printf '%s\n' "$listed"
    printf '%s: BLOCKED\n' "$evidence_key"
    blocked "exact-name preflight failed for $test_name"
  }
  count=$(printf '%s\n' "$listed" | awk -v name="$test_name" '$0 == name { n++ } END { print n + 0 }')
  if [ "$count" -ne 1 ]; then
    printf '%s: BLOCKED\n' "$evidence_key"
    blocked "exact-name preflight found $count copies of $test_name"
  fi
  if ! GOTOOLCHAIN=local go test "$package" -run "^${test_name}$" -count=1; then
    printf '%s: BLOCKED\n' "$evidence_key"
    blocked "native implementation row failed: $test_name"
  fi
  printf '%s: PASS\n' "$evidence_key"
}

run_exact ./core/cli TestAtomicRenameAtExchangePreservesBothInodes exchange
run_exact ./core/cli TestAtomicRenameAtNoReplaceCreatesAbsentTarget no_replace_absent
run_exact ./core/cli TestAtomicRenameAtNoReplacePreservesLateTarget no_replace_late_target
run_exact ./core/store TestDarwinQuarantineCleanupCapabilitySupported cleanup_capability_supported
run_exact ./core/store TestDarwinQuarantineCleanupRealTopLevel cleanup_top_level
run_exact ./core/store TestDarwinQuarantineCleanupRealNestedFile cleanup_nested_file
run_exact ./core/store TestDarwinQuarantineCleanupRealNestedDirectory cleanup_nested_directory
run_exact ./core/store TestDarwinQuarantineCleanupRealSymlink cleanup_symlink
run_exact ./core/store TestDarwinQuarantineCleanupRealReplacementBeforeFinalCheckMatrix cleanup_replacement_before
run_exact ./core/store TestDarwinQuarantineCleanupRealReplacementAfterFinalCheckMatrix cleanup_replacement_after

printf 'cleanup_capability: supported\n'
printf 'status: PASS\n'
