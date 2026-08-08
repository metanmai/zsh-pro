# Phase 6 Native macOS Runtime Evidence

status: PASS
implementation_sha: 1a83446b5911e65137f1c4eba37d84914dee96d9
authority: ci:https://github.com/metanmai/zsh-pro/actions/runs/31274061615
observed_at_utc: 2026-08-08T19:17:48Z
native_os: macOS 14.8.7
native_arch: arm64
filesystem: apfs
go_toolchain: local
go_env_goversion: go1.25.0
exact_command: GOTOOLCHAIN=local scripts/verify-phase06-macos-runtime.sh 1a83446b5911e65137f1c4eba37d84914dee96d9
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

The CI workflow commit is `aa0f996bb16ca9211ba9aede72de94feae84f5bb`; it checks out the earlier implementation SHA above before executing the verifier. The [successful job](https://github.com/metanmai/zsh-pro/actions/runs/31274061615/job/93144688734) uploaded the raw output without transformation.

The committed [unedited stdout/stderr sidecar](06-MACOS-RUNTIME-EVIDENCE-OUTPUT.txt) has SHA-256 `b35aa4d620e07e417bc038262a3dbdece745f383e84bd48165d1b2d445cf9b90`.
