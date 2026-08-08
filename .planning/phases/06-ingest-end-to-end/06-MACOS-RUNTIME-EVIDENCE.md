# Phase 6 Native macOS Runtime Evidence

status: PASS
implementation_sha: 2443a0efbff263c27e0dc2db1bd075af18879d3a
authority: ci:https://github.com/metanmai/zsh-pro/actions/runs/31277710208
observed_at_utc: 2026-08-08T20:46:59Z
native_os: macOS 14.8.7
native_arch: arm64
filesystem: apfs
go_toolchain: local
go_env_goversion: go1.25.0
exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh 2443a0efbff263c27e0dc2db1bd075af18879d3a
exit_status: 0
exchange: PASS
no_replace_absent: PASS
no_replace_late_target: PASS
cache_traversal_aliases: PASS
macos_keychain_round_trip: PASS
keychain_ingest_activation: PASS
cleanup_capability_supported: PASS
cleanup_top_level: PASS
cleanup_nested_file: PASS
cleanup_nested_directory: PASS
cleanup_symlink: PASS
cleanup_replacement_before: PASS
cleanup_replacement_after: PASS
cleanup_capability: supported

The CI workflow commit is `d07a770abe5ceac44ccde8c063b4bcda79b5f6f4`; it checks out the earlier implementation SHA above before executing the verifier. The [successful job](https://github.com/metanmai/zsh-pro/actions/runs/31277710208/job/93153909882) uploaded the raw output without transformation.

The committed [unedited stdout/stderr sidecar](06-MACOS-RUNTIME-EVIDENCE-OUTPUT.txt) has SHA-256 `6cffda072dbf8bbef80b1d5b3432c4d1d7c406511c5b2f44aa1b8c50fd7d9e9b`.
