# Phase 6 Native macOS Runtime Evidence

status: PASS
implementation_sha: 5eef81c47edac79ea40d7659db9f5dab58885071
authority: ci:https://github.com/metanmai/zsh-pro/actions/runs/31277410602
observed_at_utc: 2026-08-08T20:39:30Z
native_os: macOS 14.8.7
native_arch: arm64
filesystem: apfs
go_toolchain: local
go_env_goversion: go1.25.0
exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh 5eef81c47edac79ea40d7659db9f5dab58885071
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

The CI workflow commit is `c35334e7bb39d55e63f6ee76d9c753b93fbd190f`; it checks out the earlier implementation SHA above before executing the verifier. The [successful job](https://github.com/metanmai/zsh-pro/actions/runs/31277410602/job/93153153814) uploaded the raw output without transformation.

The committed [unedited stdout/stderr sidecar](06-MACOS-RUNTIME-EVIDENCE-OUTPUT.txt) has SHA-256 `cb043a0b045875a3be5f21a95a845dba68e9bf85c79dda8e1c744a4c85827b1b`.
