# Phase 6 Native macOS Runtime Evidence

status: PASS
implementation_sha: c2df26dfc695227c39f6620cd76f47efac185244
authority: ci:https://github.com/metanmai/zsh-pro/actions/runs/31273678559
observed_at_utc: 2026-08-08T19:08:33Z
native_os: macOS 14.8.7
native_arch: arm64
filesystem: apfs
go_toolchain: local
go_env_goversion: go1.25.0
exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh c2df26dfc695227c39f6620cd76f47efac185244
exit_status: 0
exchange: PASS
no_replace_absent: PASS
no_replace_late_target: PASS
cleanup_capability_supported: PASS
cleanup_top_level: PASS
cleanup_nested_file: PASS
cleanup_nested_directory: PASS
cleanup_symlink: PASS
cleanup_replacement_before: PASS
cleanup_replacement_after: PASS
cleanup_capability: supported

The CI workflow commit is `d40f9660c5485beafb4bcb49e85f8df94b685b98`; it checks out the earlier implementation SHA above before executing the verifier. The [successful job](https://github.com/metanmai/zsh-pro/actions/runs/31273678559/job/93143702318) uploaded the raw output without transformation.

The committed [unedited stdout/stderr sidecar](06-MACOS-RUNTIME-EVIDENCE-OUTPUT.txt) has SHA-256 `8124d04c11c03a03ba164943f698e15d8800279bec371e8f925376b6c2c72b5f`.
