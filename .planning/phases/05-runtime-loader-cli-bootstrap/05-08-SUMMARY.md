---
phase: 05-runtime-loader-cli-bootstrap
plan: 08
subsystem: shell-runtime-transition
tags: [go, zsh, runtime-loader, activation, reverse, secretref, live-zsh]
requires:
  - phase: 05-04
    provides: bounded source validation and the sourced runtime transition boundary
  - phase: 05-05
    provides: activation-only SecretRef resolution through the CLI resolver seam
  - phase: 05-06
    provides: fail-open cached-loader behavior and normalized runtime dependencies
  - phase: 05-07
    provides: private runtime staging and presence-aware environment restoration
provides:
  - target-only apply source paired with a retained target-specific zp_deactivate function
  - shell-local activation tracking that cannot be spoofed by inherited ZSHPRO_PROFILE
  - resolver-loss-safe reverse operations with direct native-zsh coverage
affects: [phase-05-verification, phase-06-ingest, BOOT-01, BOOT-02]
tech-stack:
  added: []
  patterns: [paired target apply/reverse source, shell-local activation marker, retained reverse before target replacement]
key-files:
  created: []
  modified: [core/cli/emitter.go, core/cli/emitter_test.go, core/shell/zsh/hook.go, core/shell/zsh/hook_test.go, core/shell/zsh/live_terminal_test.go]
key-decisions:
  - "Apply emission reads and resolves only its requested target; the current shell owns active-state reversal."
  - "ZP_ACTIVE_PROFILE is a non-exported authoritative marker, while ZSHPRO_PROFILE remains post-evaluation metadata for child processes."
  - "The unavailable local hyperfine tool leaves the performance target behavior-unverified rather than approximated."
patterns-established:
  - "Every apply payload defines its own zp_deactivate before defining and invoking zp_apply."
  - "A switch invokes the retained reverse before evaluating a new target payload; direct deactivate never shells out for a reverse payload."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: "An explicit main profile can switch to B and deactivate with no profile-owned residue."
    requirement: BOOT-01
    verification:
      - kind: integration
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterLiveTransitionRemovesAOnlyState"
        status: pass
      - kind: integration
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterLiveCheckoutTransitionRemovesAOnlyState"
        status: pass
    human_judgment: false
  - id: D2
    description: "A secret-bearing activation reverses after resolver loss without re-reading the active SecretRef or requesting binary deactivate output."
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterSwitchDoesNotResolveAnActiveSecretAgain"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/live_terminal_test.go#TestLiveTerminalRetainedSecretReverseSurvivesUnavailableBinary"
        status: pass
    human_judgment: false
  - id: D3
    description: "The integrated runtime repair tree is formatted, vetted, linted, and fully tested."
    requirement: BOOT-02
    verification:
      - kind: other
        ref: "make check"
        status: pass
    human_judgment: false
  - id: D4
    description: "The sourced-activation timing budget is measured with the external hyperfine harness."
    requirement: BOOT-02
    verification:
      - kind: manual_procedural
        ref: "perf_dir=$(mktemp -d); go build -o $perf_dir/zsh-pro ./core/cmd/zsh-pro; ZSHPRO_BIN=$perf_dir/zsh-pro scripts/perf-hyperfine.sh"
        status: unknown
    human_judgment: true
    rationale: "hyperfine is absent in this environment, so no measured timing result exists."
metrics:
  duration: 17min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 08: Retained Runtime Reverse Summary

**Target-only profile activation now retains the matching in-shell reverse, so explicit main transitions and secret-bearing reversals stay clean without consulting persisted active state.**

## Performance

- **Duration:** 17 min
- **Started:** 2026-07-29T22:45:39Z
- **Completed:** 2026-07-29T23:02:17Z
- **Tasks:** 3/3
- **Files modified:** 5

## Accomplishments

- Replaced Store.Current-derived transition emission with a target-only apply plan plus a separately generated target reverse retained in the sourcing shell.
- Added a non-exported ZP_ACTIVE_PROFILE marker that distinguishes a real loader activation from inherited ZSHPRO_PROFILE metadata, prevents repeated activation layers, and advances only after evaluation succeeds.
- Made switches and explicit deactivation use the retained reverse before a new payload can replace it; secret cleanup therefore survives later resolver loss and an unavailable binary deactivate route.
- Added direct native-zsh coverage for main -> B -> deactivate, same-profile activation, inherited state, failed evaluations, and secret reverse operations.

## Task Commits

1. **Task 1: Pair every target apply payload with its retained reverse function** — `ac0ae25` (RED regressions), `860c53c` (target-only emitter and marker implementation)
2. **Task 2: Make secret-profile reverse operations independent of resolver availability** — `e58522c` (stateful resolver and unavailable-binary reverse proof)
3. **Task 3: Run the repository-wide quality gate after all gap repairs** — `ca959f4` (marker-aware fail-open regression correction); `make check` passed

## Files Created/Modified

- `core/cli/emitter.go` — emits target-only apply source paired with the target's retained reverse and no longer consults Store.Current.
- `core/cli/emitter_test.go` — proves target-only reads, explicit main transitions, and stateful resolver-loss behavior.
- `core/shell/zsh/hook.go` — owns marker-based lifecycle state, retained reverse invocation, and no-binary deactivation.
- `core/shell/zsh/hook_test.go` — pins the non-exported activation-marker contract.
- `core/shell/zsh/live_terminal_test.go` — exercises source-level marker, failed-evaluation, secret-reverse, and fail-open contracts in native zsh.

## Decisions Made

- The child binary validates and emits only a requested target; it never infers or re-resolves an active profile from persisted/environment state.
- The loader owns active lifecycle state with a non-exported marker, while ZSHPRO_PROFILE is updated only after successful source evaluation.
- Secret reverse behavior is intentionally a retained-shell capability, not a second resolver-dependent deactivate emission.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test contract] Updated the CLI live-transition fixture for private runtime staging.**
- **Found during:** Task 1 RED reproduction.
- **Issue:** The legacy fixture supplied TMPDIR but not the private ZSHPRO_HOME runtime root required by Plan 05-07, masking the intended transition defect before source evaluation.
- **Fix:** Created and injected the owned runtime root in the fixture before asserting the explicit main transition.
- **Files modified:** `core/cli/emitter_test.go`
- **Verification:** Target-only and native-zsh transition tests passed.
- **Committed in:** `ac0ae25`

**2. [Rule 1 - Test contract] Paired pre-existing live-loader apply shims with their retained reverse functions.**
- **Found during:** Task 2 focused verification.
- **Issue:** A legacy apply-only shim caused the repaired loader to call an undefined zp_deactivate during a switch.
- **Fix:** Made each shim return the same paired apply/reverse source contract as the production emitter.
- **Files modified:** `core/shell/zsh/live_terminal_test.go`
- **Verification:** Focused secret, switch, and deactivate suites passed.
- **Committed in:** `e58522c`

**3. [Rule 1 - Test contract] Corrected fail-open deactivation expectations for inherited profile metadata.**
- **Found during:** Task 3 `make check`.
- **Issue:** The old test expected deactivation to treat inherited ZSHPRO_PROFILE as active, contradicting the new marker boundary.
- **Fix:** Distinguished failed activate/checkout calls from marker-absent deactivation, which correctly no-ops without calling the binary.
- **Files modified:** `core/shell/zsh/live_terminal_test.go`
- **Verification:** Focused hostile-option regression and final `make check` passed.
- **Committed in:** `ca959f4`

---

**Total deviations:** 3 auto-fixed Rule 1 test-contract corrections.
**Impact on plan:** All corrections kept established tests representative of the planned runtime contracts; no production scope expanded.

## Issues Encountered

- The original requested transition failures were reproduced first. Their initial exit status 60 was caused by the obsolete fixture missing the new private runtime root; after aligning that prerequisite, the RED suite exposed the intended target-only/reverse and marker gaps.
- Task 2's new resolver-loss proof passed once Task 1 landed because retained target reverse is the shared implementation required by both tasks; no duplicate resolver-state mechanism was added.

## TDD Gate Compliance

- Task 1: `ac0ae25` (RED) -> `860c53c` (GREEN).
- Task 2: `e58522c` adds the stateful resolver and native-zsh proof after Task 1's shared paired-reverse implementation; it passed immediately because the implementation deliberately makes reverse paths resolver-independent.

## Known Stubs

None. The changed runtime paths use real emitted source, retained functions, and direct native-zsh execution; the fixture secret value is intentional adversarial test data.

## Verification

- `go test ./core/cli ./core/shell/zsh -run 'Test.*(Main|Transition|Apply|Activate|Deactivate|Inherited|Eval|Marker|Target|Secret|Resolver|Switch)' -count=1 && go vet ./core/cli ./core/shell/zsh` — PASS.
- `go test ./core/cli -run 'TestRuntimeEmitter(LiveTransitionRemovesAOnlyState|LiveCheckoutTransitionRemovesAOnlyState|ApplyUsesOnlyTargetAndRetainsTargetReverse|SwitchDoesNotResolveAnActiveSecretAgain)' -count=1` — PASS.
- `go test ./core/shell/zsh -run 'TestLiveTerminal(ActivationMarkerDistinguishesInheritedProfile|FailedEvalKeepsActivationMarkerAndProfile|RetainedSecretReverseSurvivesUnavailableBinary|LoaderSwitchesCurrentShellWithoutResidue)' -count=1` — PASS.
- `make check` — PASS: gofmt check, `go vet ./...`, `golangci-lint run`, and full `go test ./...`.
- `hyperfine` — absent. The prescribed external-binary timing command was not substituted, so the less-than-10-ms result remains **behavior-unverified**.

## User Setup Required

None - no external service configuration is required.

## Next Phase Readiness

- The final runtime loader repair tree is ready for Phase 5 re-verification: explicit main, inherited environment, resolver loss, and fail-open boundaries have automated native-zsh evidence.
- Run the documented `scripts/perf-hyperfine.sh` command on a machine with `hyperfine` before claiming the startup timing budget.

## Self-Check: PASSED

- Confirmed all five implementation/test artifacts and this summary exist.
- Confirmed task commits `ac0ae25`, `860c53c`, `e58522c`, and `ca959f4` exist in Git history.

---
*Phase: 05-runtime-loader-cli-bootstrap*
*Completed: 2026-07-29*
