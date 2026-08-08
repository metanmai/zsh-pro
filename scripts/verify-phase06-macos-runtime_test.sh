#!/usr/bin/env bash
set -eu

if [ "${PHASE6_VERIFIER_STUB_MODE-}" = 1 ]; then
  case "${0##*/}" in
    df)
      test "$#" -eq 2
      test "$1" = -P
      test "$2" = "$PHASE6_EXPECTED_REPO"
      printf 'Filesystem 512-blocks Used Available Capacity Mounted on\n'
      printf '/dev/disk9s1 1000 100 900 10%% /Volumes/Phase Six\n'
      ;;
    diskutil)
      test "$#" -eq 3
      test "$1" = info
      test "$2" = -plist
      test "$3" = '/Volumes/Phase Six'
      printf 'diskutil:%s\n' "$*" >>"$PHASE6_STUB_LOG"
      printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' '<plist><dict><key>FilesystemType</key><string>apfs</string></dict></plist>'
      ;;
    plutil)
      test "$#" -eq 6
      test "$1" = -extract
      test "$2" = FilesystemType
      test "$3" = raw
      test "$4" = -o
      test "$5" = -
      test "$6" = -
      printf 'plutil:%s\n' "$*" >>"$PHASE6_STUB_LOG"
      while IFS= read -r _line; do :; done
      if [ "${PHASE6_PLUTIL_FAIL-}" = 1 ]; then
        exit 91
      fi
      printf 'apfs\n'
      ;;
    uname)
      case "${1-}" in
        -s) printf 'Darwin\n' ;;
        -m) printf 'arm64\n' ;;
        -r) printf '23.6.0\n' ;;
        *) exit 125 ;;
      esac
      ;;
    sw_vers)
      test "$#" -eq 1
      test "$1" = -productVersion
      printf '14.7\n'
      ;;
    go)
      case "${1-}:${2-}" in
        env:GOTOOLCHAIN) printf 'local\n' ;;
        env:GOVERSION) printf 'go1.25.7\n' ;;
        env:GOHOSTOS|env:GOOS) printf 'darwin\n' ;;
        test:*)
          for argument in "$@"; do
            case "$argument" in
              ^Test*\$)
                test_name=${argument#?}
                test_name=${test_name%?}
                printf '%s\n' "$test_name"
                break
                ;;
            esac
          done
          ;;
        *) exit 125 ;;
      esac
      ;;
    *) exit 125 ;;
  esac
  exit 0
fi

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

test_root=$(mktemp -d "${TMPDIR:-/tmp}/zsh-pro-phase06-verifier-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

script_dir=$(cd "$(dirname "$0")" && pwd)
script_path="$script_dir/${0##*/}"

new_fixture() {
  fixture_root=$1
  mkdir -p "$fixture_root/core" "$fixture_root/scripts"
  cp "$script_dir/verify-phase06-macos-runtime.sh" "$fixture_root/scripts/verify-phase06-macos-runtime.sh"
  printf 'package tracked\n' >"$fixture_root/core/tracked.go"
  printf 'core/generated.go\n' >"$fixture_root/.gitignore"
  git -C "$fixture_root" init -q
  git -C "$fixture_root" add .gitignore core/tracked.go scripts/verify-phase06-macos-runtime.sh
  git -C "$fixture_root" -c user.name='Phase 6 Verifier Test' -c user.email='phase6-verifier@example.invalid' commit -qm 'test fixture'
  git -C "$fixture_root" rev-parse HEAD
}

untracked_root="$test_root/untracked"
expected_sha=$(new_fixture "$untracked_root")

# Ignored files are still untracked implementation inputs: a Go source file
# below core can affect the tested package even when Git status hides it.
printf 'package generated\n' >"$untracked_root/core/generated.go"
if output=$(cd "$untracked_root" && GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh "$expected_sha" 2>&1); then
  fail 'verifier accepted an untracked implementation file'
fi
printf '%s\n' "$output" | grep -qx 'status: BLOCKED' || fail 'verifier did not emit BLOCKED status'
printf '%s\n' "$output" | grep -qx 'reason: untracked repository files are present' || {
  fail 'verifier did not report the closed untracked-input reason'
}
if printf '%s\n' "$output" | grep -q 'generated.go'; then
  fail 'verifier disclosed the untracked path'
fi

workspace_root="$test_root/workspace"
workspace_sha=$(new_fixture "$workspace_root")
printf 'go 1.25.0\n' >"$workspace_root/go.work"
if workspace_output=$(cd "$workspace_root" && GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh "$workspace_sha" 2>&1); then
  fail 'verifier accepted an untracked root go.work file'
fi
printf '%s\n' "$workspace_output" | grep -qx 'status: BLOCKED' || fail 'workspace input did not emit BLOCKED status'
printf '%s\n' "$workspace_output" | grep -qx 'reason: untracked repository files are present' || {
  fail 'workspace input did not report the closed untracked-input reason'
}
if printf '%s\n' "$workspace_output" | grep -q 'go.work'; then
  fail 'verifier disclosed the untracked workspace path'
fi

native_root="$test_root/native"
native_sha=$(new_fixture "$native_root")
stub_dir="$test_root/native-tools"
stub_log="$test_root/native-tools.log"
mkdir -p "$stub_dir"
for command_name in df diskutil plutil uname sw_vers go; do
  ln -s "$script_path" "$stub_dir/$command_name"
done

if ! native_output=$(cd "$native_root" && PATH="$stub_dir:$PATH" PHASE6_VERIFIER_STUB_MODE=1 PHASE6_EXPECTED_REPO="$native_root" PHASE6_STUB_LOG="$stub_log" GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh "$native_sha" 2>&1); then
  printf '%s\n' "$native_output" >&2
  fail 'verifier rejected the deterministic native filesystem probe'
fi
printf '%s\n' "$native_output" | grep -qx 'filesystem: apfs' || fail 'verifier did not report the mounted filesystem type'
printf '%s\n' "$native_output" | grep -qx 'macos_keychain_round_trip: PASS' || fail 'verifier did not run the native macOS keychain row'
printf '%s\n' "$native_output" | grep -qx 'keychain_ingest_activation: PASS' || fail 'verifier did not run the keychain ingest/activation row'
grep -Fqx 'diskutil:info -plist /Volumes/Phase Six' "$stub_log" || fail 'verifier did not inspect the resolved mount point with diskutil plist output'
grep -Fqx 'plutil:-extract FilesystemType raw -o - -' "$stub_log" || fail 'verifier did not extract FilesystemType through plutil'

if ! fallback_output=$(cd "$native_root" && PATH="$stub_dir:$PATH" PHASE6_VERIFIER_STUB_MODE=1 PHASE6_EXPECTED_REPO="$native_root" PHASE6_STUB_LOG="$stub_log" PHASE6_PLUTIL_FAIL=1 GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh "$native_sha" 2>&1); then
  printf '%s\n' "$fallback_output" >&2
  fail 'verifier rejected the deterministic filesystem fallback'
fi
printf '%s\n' "$fallback_output" | grep -qx 'filesystem: unknown' || fail 'verifier did not fail closed to unknown filesystem metadata'

printf 'PASS: phase 6 verifier rejects untracked inputs including root go.work and reports mounted filesystem metadata\n'
