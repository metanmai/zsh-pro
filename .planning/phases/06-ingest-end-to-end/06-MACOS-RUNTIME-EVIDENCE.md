# Phase 6 Native macOS Runtime Evidence

status: PASS
implementation_sha: cdb82ef21f8f3f10034a0c07442e1c52a8ef7802
authority: ci:https://github.com/metanmai/zsh-pro/actions/runs/31278506938
observed_at_utc: 2026-08-08T21:06:30Z
native_os: macOS 14.8.7
native_arch: arm64
filesystem: apfs
go_toolchain: local
go_env_goversion: go1.25.0
exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh cdb82ef21f8f3f10034a0c07442e1c52a8ef7802
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

The CI workflow commit is `bec704ad7f292827c1e4c36b42451270700afe27`; it checks out the exact implementation SHA above before executing the verifier. The [successful job](https://github.com/metanmai/zsh-pro/actions/runs/31278506938/job/93155882666) uploaded the raw output without transformation.

The committed [unedited stdout/stderr sidecar](06-MACOS-RUNTIME-EVIDENCE-OUTPUT.txt) has SHA-256 `7cc6a9e26686feb765d453ea273cff28c3f629a0b6fe4964b453a688795839d7`.
