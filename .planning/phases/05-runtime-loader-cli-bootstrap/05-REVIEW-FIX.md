---
phase: 05
fixed_at: 2026-07-30T01:32:26Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 3
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T01:32:26Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 3

**Summary:**

- Findings in scope: 2
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: The descriptor-authenticated runtime root is discarded before the emitter reopens it

**Status:** fixed: requires human verification
**Files modified:** `core/cli/emitter.go`, `core/cli/runtime.go`, `core/cli/runtime_root.go`, `core/cli/runtime_root_other.go`, `core/cli/runtime_root_unix.go`, `core/cli/runtime_test.go`, `core/cmd/zsh-pro/main.go`, `core/store/git.go`, `core/store/keychain.go`, `core/store/store.go`, `core/store/runtime_openat_darwin.go`, `core/store/runtime_openat_linux.go`, `core/store/runtime_vault_other.go`, `core/store/runtime_vault_unix.go`
**Commit:** 22763bd
**Applied fix:** Retains the authenticated repository and vault-parent descriptors, emits in the already-validated helper process, and binds Git plus fallback-vault reads to those descriptors. The deterministic sticky-directory swap regression pauses after validation, replaces the pathname with an attacker symlink, then proves the real store/emitter reads and evaluates only the victim profile and vault secret.

### CR-02: Successful activation persists resolved secret source in global `REPLY`

**Status:** fixed: requires human verification
**Files modified:** `core/activate/diff.go`, `core/activate/plan.go`, `core/cli/emitter_test.go`, `core/shell/zsh/emit.go`, `core/shell/zsh/emit_test.go`, `core/shell/zsh/hook.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** 5118507
**Applied fix:** Replaces secret-bearing `REPLY` transport with dynamically scoped caller locals and `always`-block scrubbing. Static runtime values no longer create duplicate global applied-value slots; dynamic values retain the necessary evaluated-value slot for correct reversal. Native zsh coverage scans global parameters and obsolete generated functions across success, emitter failure, validation failure, partial evaluation failure, switch, and deactivate.

## Verification

- `TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors` — passed
- `TestLiveTerminalRuntimeTransportScrubsResolvedSource`, `TestEmitRuntimeAvoidsStaticAppliedSecretCopiesButKeepsDynamicReversal`, and `TestRuntimeEmitterPreservesUserFunctionsAndScrubsResolvedSecrets` — passed
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed
- `GOTOOLCHAIN=auto go vet ./...` — passed
- `GOTOOLCHAIN=auto make check` — passed (`golangci-lint`: 0 issues)
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go test -c -o /tmp/zsh-pro-cli-final-test ./core/cli` — passed
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go test -c -o /tmp/zsh-pro-zsh-final-test ./core/shell/zsh` — passed

## Remaining Manual Verification

The review's startup benchmark is not part of these fixes and was not run here.

---

_Fixed: 2026-07-30T01:32:26Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 3_
