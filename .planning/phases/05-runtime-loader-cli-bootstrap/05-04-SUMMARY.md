---
phase: 05-runtime-loader-cli-bootstrap
plan: 04
subsystem: shell-runtime-transition
tags: [zsh, runtime-loader, watchdog, err-exit, err-return, profile-transition, store]
requires:
  - phase: 05-02
    provides: sourced loader, validated evaluation boundary, per-terminal last-good state, and runtime emitter wiring
  - phase: 05-03
    provides: installer/cache reliability fixes preserved by this runtime work
provides:
  - portable deadline-bound emitter and validator execution with private staging cleanup
  - fail-open activate, checkout, and deactivate calls under ERR_EXIT and ERR_RETURN
  - one self-executing deactivate-prior plus apply-target source for active profile switches
affects: [05-05, phase-05-verification, phase-06-ingest]
tech-stack:
  added: []
  patterns: [zsh noclobber exclusive staging, background watchdog with reaping, consumed interactive failure status, single-source active-to-target transition]
key-files:
  created: [core/cli/emitter_test.go]
  modified: [core/shell/zsh/hook.go, core/shell/zsh/live_terminal_test.go, core/cli/emitter.go]
key-decisions:
  - "Runtime subprocesses use a zsh-native child plus watchdog and bounded 1–99 second timeout input rather than GNU timeout."
  - "Expected public verb failures report through ZP_LAST_RUNTIME_STATUS/ZP_LAST_RUNTIME_ERROR and return zero to preserve an interactive ERR_EXIT or ERR_RETURN caller."
  - "The runtime emitter owns the complete executable transition, so the loader validates and evaluates one deactivate-then-apply source exactly once."
patterns-established:
  - "Use a noclobber-created 0600 staging file, always cleanup, and reaped command/watchdog processes for runtime source handling."
  - "Represent a profile switch as one source block whose final commands execute deactivate before apply; share that path between activate and checkout."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: "Runtime emitter, validator, staging, and evaluation failures are bounded, cleaned up, diagnosed, and consumed by direct hostile-option public verb calls."
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/live_terminal_test.go#TestLiveTerminalPublicVerbsFailOpenUnderErrExitAndErrReturn"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/live_terminal_test.go#TestLiveTerminalTimeoutsAreBoundedAndCleanedUp"
        status: pass
      - kind: other
        ref: "built CLI ERR_EXIT/ERR_RETURN plus sleeping-emitter and sleeping-validator probes"
        status: pass
    human_judgment: false
  - id: D2
    description: "activate and checkout both remove A-only environment, alias, function, option, and PATH state before applying B, with no residual state after deactivate."
    requirement: BOOT-01
    verification:
      - kind: integration
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterLiveTransitionRemovesAOnlyState"
        status: pass
      - kind: integration
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterLiveCheckoutTransitionRemovesAOnlyState"
        status: pass
      - kind: other
        ref: "built CLI real Store activate A -> activate B -> deactivate probe"
        status: pass
    human_judgment: false
metrics:
  duration: 36min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 04: Bounded Runtime Transition Summary

**The zsh loader now contains operational failures behind a portable deadline and switches a live terminal through one complete, validated A-to-B transaction.**

## Performance

- **Duration:** 36 min
- **Started:** 2026-07-29T18:01:03Z
- **Completed:** 2026-07-29T18:37:17Z
- **Tasks:** 2/2
- **Files modified:** 4

## Accomplishments

- Added a private noclobber staging primitive and a reaped child/watchdog runner for both `zsh-pro emit` and `zsh -n`, with a bounded runtime timeout and unconditional cleanup.
- Made expected activation, checkout, and deactivation failures fail open under direct `ERR_EXIT` and `ERR_RETURN` calls while retaining visible error status and diagnostics.
- Made `runtimeEmitter.Emit` return a complete source transaction that defines both phases and executes `zp_deactivate` before `zp_apply` whenever the active profile differs from the target.
- Added real Store plus real zsh regressions for both `activate A -> activate B -> deactivate` and `activate A -> checkout B -> deactivate`, including A-only ownership removal, base-PATH stability, and a one-validation-per-transition assertion.

## Task Commits

1. **Task 1: Bound and contain every runtime verb failure under hostile zsh error options** — `ccb8990` (RED test), `8591503` (implementation)
2. **Task 2: Emit and apply a complete active-profile transition for activate and checkout** — `37ec59d` (RED test), `d5a10f0` (implementation)

## Files Created/Modified

- `core/shell/zsh/hook.go` — bounded subprocess ownership, strict cleanup, runtime status slots, and shared activate/checkout switching.
- `core/shell/zsh/live_terminal_test.go` — direct hostile-option, failure-path, timeout, staging-collision, cleanup, and complete-source loader coverage.
- `core/cli/emitter.go` — emits self-executing apply/deactivate blocks and combines active-to-target halves in mutation order.
- `core/cli/emitter_test.go` — real git-backed Store and real zsh transition coverage for activate and checkout.

## Decisions Made

- A private zsh `noclobber` file primitive is used instead of a GNU-specific `timeout`/`mktemp` assumption; the watchdog owns command lifetime and reaps its own timer.
- Public runtime verbs deliberately expose operational failure through globals rather than their shell return code, so an interactive caller stays usable under hostile error options.
- The emitter, not the loader, owns transaction composition. This avoids duplicate/incomplete deactivation logic and gives one syntax-validation/evaluation boundary per switch.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test contract] Updated existing loader shims to emit complete executable source**
- **Found during:** Task 2 green verification
- **Issue:** Earlier fixtures supplied only `zp_apply`/`zp_deactivate` definitions because the loader appended invocation commands itself. The corrected emitter contract returns a complete executable source, so the old fixtures no longer represented production behavior.
- **Fix:** Appended the matching function invocation to existing test shim output and kept the loader responsible for only one evaluation of the returned block.
- **Files modified:** `core/shell/zsh/live_terminal_test.go`
- **Verification:** Focused zsh and CLI transition suites pass, including the one-validation assertion.
- **Committed in:** `d5a10f0`

---

**Total deviations:** 1 auto-fixed (Rule 1 test-contract correction).
**Impact on plan:** Required to keep the prior loader regressions aligned with the planned complete-source contract; no product scope expansion.

## Issues Encountered

- The first watchdog draft left a foreground timer running after fast child completion, so successful operations waited the full five-second default. The final watchdog traps cancellation, terminates/reaps its timer, and the focused suite completes in about 2.3 seconds.
- `status` is a read-only zsh special parameter; the loader uses `exit_status` for its local diagnostic code instead.

## TDD Gate Compliance

Both tasks recorded a failing RED commit before their matching GREEN implementation commit:

- Task 1: `ccb8990` -> `8591503`
- Task 2: `37ec59d` -> `d5a10f0`

## Known Stubs

None.

## Verification

- `go test ./core/shell/zsh -run 'LiveTerminal|HookScript|Timeout|ErrExit|ErrReturn|Cleanup' -count=1` — PASS
- `go test ./core/cli -run 'RuntimeEmitter|Transition' -count=1` — PASS
- `go test ./...` — PASS
- `make check` — PASS (`go vet`, `golangci-lint`, full suite)
- Built CLI live probes — PASS: A-to-B transition, direct `ERR_EXIT`/`ERR_RETURN` failure survival, sleeping emitter deadline, and sleeping validator deadline.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Plan 05-05 can build on the bounded, fail-open loader and complete transition source. No Plan 04 blocker remains.

## Self-Check

PASSED

- Confirmed all four implementation/test files and this summary exist.
- Confirmed task commits `ccb8990`, `8591503`, `37ec59d`, and `d5a10f0` exist in git history.
