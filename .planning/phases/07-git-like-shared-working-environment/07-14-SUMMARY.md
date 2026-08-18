---
phase: 07-git-like-shared-working-environment
plan: 14
subsystem: shell-runtime
tags: [zsh, live-patch, fail-fast, recovery, idempotent-deactivate]

requires:
  - phase: 07-13
    provides: absolute worktree lifecycle bounds and fail-open shell-operation cleanup
provides:
  - Per-operation status propagation for emitted live forward and replacement-reverse functions
  - Replacement reverse ownership before the first parent-shell mutation
  - Retained, retryable public recovery across apply and post-apply failures
affects: [07-15, phase-07-verification, shell-runtime-security]

tech-stack:
  added: []
  patterns:
    - Capture each emitted mutation status immediately and return before the next operation
    - Install recovery ownership before apply, promote before old-owner cleanup, and acknowledge last

key-files:
  created: []
  modified:
    - core/shell/zsh/emit.go
    - core/shell/zsh/emit_test.go
    - core/shell/zsh/hook.go
    - core/shell/zsh/hook_test.go
    - core/shell/zsh/worktree_live_test.go

key-decisions:
  - "Wrap each already-validated operation with an immediate local status capture so both forward and reverse execution preserve the exact first nonzero result."
  - "Point ZP_RECOVERY_REVERSE_FN at the replacement reverse before apply, promote it to ZP_ACTIVE_REVERSE_FN before old-owner cleanup, and advance the applied revision only after exact acknowledgement."
  - "Public deactivate services pending recovery before the active worktree reverse, preserving a failed reverse owner for repair and retry."

patterns-established:
  - "Continuous reverse ownership: every live mutation begins only after its exact replacement reverse is reachable through a loader-owned pointer."
  - "Truthful convergence: apply, deadline, capture, reply, and acknowledge failures leave the prior applied revision and a reachable reverse intact."

requirements-completed: [WORK-02, SYNC-01, SYNC-02]

coverage:
  - id: D1
    description: "Every live forward and replacement-reverse operation stops at its first failure and returns that operation's nonzero status."
    requirement: WORK-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestEmitLivePatchFailsFastPerOperation"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestEmitLivePatchOwnsExactApplyAndReplacementReverse"
        status: pass
    human_judgment: false
  - id: D2
    description: "Replacement recovery ownership precedes forward mutation and prior ownership remains until promotion succeeds."
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/hook_test.go#TestWorktreeRecoveryOwnerPrecedesMutation"
        status: pass
    human_judgment: false
  - id: D3
    description: "Real-zsh partial and post-apply failures remain exactly recoverable through idempotent public deactivation without advancing applied revision truth."
    requirement: SYNC-02
    verification:
      - kind: e2e
        ref: "core/shell/zsh/worktree_live_test.go#TestWorktreePartialPatchFailureRecovery"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=local go test -race -count=1 ./core/shell/zsh -timeout=180s"
        status: pass
    human_judgment: false

duration: 21min
completed: 2026-08-18
status: complete
---

# Phase 07 Plan 14: Fail-Fast Live Patch Recovery Summary

**Every parent-zsh live mutation now fails at the exact offending operation while continuous reverse ownership preserves truthful, retryable recovery through acknowledgement.**

## Performance

- **Duration:** 21 min
- **Started:** 2026-08-18T02:41:18Z
- **Completed:** 2026-08-18T03:02:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Emitted forward and replacement-reverse functions now capture each operation's status immediately, return the exact first failure, and never run later mutations that could mask it.
- The loader validates and owns the replacement reverse before apply, retains the prior active reverse through forward execution, and promotes new ownership before clearing recovery or removing the old owner.
- Apply, deadline, capture, malformed-reply, and acknowledgement failures preserve the old applied revision, visible error/behind truth, next-command usability, and an exact public recovery path.
- Real `zsh -f -i` coverage proves hostile readonly targets, scalar/PATH/FPATH/alias/function fidelity, failed compensation retry, public deactivation ordering, idempotency, and zero generated-function/control residue.

## Task Commits

Each task was committed atomically with its TDD gate:

1. **Task 1 RED: per-operation fail-fast contract** - `e4aba9c`
2. **Task 1 GREEN: checked live operation emission** - `7f5e623`
3. **Task 2 RED: continuous recovery ownership contract** - `8ddc8bf`
4. **Task 2 GREEN: pre-mutation ownership and public recovery** - `8f73dcc`

## Files Created/Modified

- `core/shell/zsh/emit.go` - Immediate status checking for every validated forward and reverse mutation.
- `core/shell/zsh/emit_test.go` - First/middle/final fail-fast, replacement-reverse, exact-fidelity, and malformed-transition coverage.
- `core/shell/zsh/hook.go` - Pre-mutation recovery installation, safe promotion, retained post-apply ownership, and recovery-first deactivation.
- `core/shell/zsh/hook_test.go` - Structural and real-zsh proof that recovery ownership precedes mutation and promotion ordering is safe.
- `core/shell/zsh/worktree_live_test.go` - Retained real-zsh partial mutation, compensation failure, post-apply failure, exact restoration, and idempotency evidence.

## Decisions Made

- Operation syntax remains centralized in `emitLiveOperation`; `emitCheckedLiveOperation` only adds an immediate, value-free status guard around already validated source.
- A replacement reverse enters recovery ownership before apply. It becomes the active reverse after complete forward success, before recovery is cleared and before the prior reverse is removed.
- The applied revision and capture baseline move only after exact acknowledgement; all earlier failures preserve truthful non-convergence and recovery authority.
- Public deactivation handles pending compensation first, then the active worktree reverse, so one retry can recover a failed new transition before deactivating the prior layer.

## Validation

- `GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -run '^(TestEmitLivePatchFailsFastPerOperation|TestEmitLivePatchOwnsExactApplyAndReplacementReverse|TestEmitRuntimeTransitionRejectsMalformedInput)$' -timeout=15s` - pass.
- `GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -run '^(TestWorktreeRecoveryOwnerPrecedesMutation|TestWorktreePartialPatchFailureRecovery)$' -timeout=25s` - pass.
- `GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -timeout=60s` - pass.
- `GOTOOLCHAIN=local go test -count=1 ./... -timeout=180s` - pass.
- `GOTOOLCHAIN=local go test -race -count=1 ./core/shell/zsh -timeout=180s` - pass.
- `GOTOOLCHAIN=local go vet ./...` - pass.
- `GOTOOLCHAIN=local golangci-lint run ./...` - pass with zero issues.
- `GOTOOLCHAIN=local go build ./...` - pass.
- `GOTOOLCHAIN=local make check` - pass, including vet, lint, and the full test suite.
- `git diff --check` - pass.

## TDD Gate Compliance

Both tasks have a failing `test(07-14)` commit before their corresponding passing `feat(07-14)` commit.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- One initial uncached shell-package pass observed the existing timing-sensitive two-shell behind-state assertion. Its isolated rerun and all subsequent package, repository, race, and `make check` runs passed without an implementation change.

## Known Stubs

None.

## Authentication Gates

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 07-15 and final Phase 07 verification can rely on fail-fast operation status and continuous exact recovery ownership.
- No blockers remain. Pre-existing unrelated untracked planning, graph, cache, and output paths were not touched.

## Self-Check: PASSED

- All five modified files and this summary exist.
- All four RED/GREEN task commits exist in order.
- Focused, package, full, race, vet, lint, build, `make check`, and diff-integrity gates pass.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-18*
