---
phase: 04-manifest-builder-emit
plan: 14
subsystem: activation
tags: [go, zsh, manifest, dto, path, fpath, zero-residue]
requires:
  - phase: 04-13
    provides: declaration routing and semantic list admission guards
provides:
  - one source-ordered reducer for semantic and legacy PATH/FPATH entries
  - persisted declaration representability guard and verbatim regeneration
  - DTO-to-live-zsh fidelity and zero-residue regression coverage
affects: [phase-04-review, runtime-loader]
tech-stack:
  added: []
  patterns: [canonical list token reduction, declaration representability boundary]
key-files:
  created: []
  modified: [core/activate/builder.go, core/model/profile.go, core/ir/regen.go, core/shell/zsh/pipeline_test.go, core/shell/zsh/residue_test.go]
key-decisions:
  - "ValueModeLegacy remains explicit compatibility input but is reduced through the semantic list token stream."
  - "OverrideManaged stays durable metadata; unrepresented declaration attributes independently block regeneration templating and manifest intent."
patterns-established:
  - "Persist profiles through store.MarshalProfile and store.UnmarshalProfile before testing builder behavior."
  - "Use emitted zsh syntax checks plus zsh -f apply/deactivate and snapshot restoration for list fidelity."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: Canonical legacy and semantic PATH/FPATH list composition through the persisted production pipeline.
    requirement: SW-01
    verification:
      - kind: integration
        ref: core/shell/zsh/pipeline_test.go#TestPipelineStoreRoundTripLegacy
        status: pass
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestResidueStoreRoundTripLegacy
        status: pass
    human_judgment: false
  - id: D2
    description: Persisted forced declaration forms regenerate verbatim and produce no activation operations.
    requirement: SW-02
    verification:
      - kind: integration
        ref: core/shell/zsh/pipeline_test.go#TestPipelinePersistedOverrideManaged
        status: pass
    human_judgment: false
duration: 8min
completed: 2026-07-27
status: complete
---

# Phase 04 Plan 14: Final fidelity gap closure Summary

**Canonical source-ordered PATH/FPATH reduction and declaration-safe persisted profiles, proven through real emitted zsh apply/deactivate cycles.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-27T10:54:55Z
- **Completed:** 2026-07-27T11:02:11Z
- **Tasks:** 3
- **Files modified:** 9

## Accomplishments

- Routed explicit legacy PATH/FPATH compatibility values through the same ordered token reducer as semantic lists, emitting one delta per canonical identity.
- Added a model-level declaration representability guard so forced integer, readonly, tied, and local declarations remain verbatim and cannot create activation intent after storage.
- Added DTO-round-tripped, syntax-checked `zsh -f` proofs for repeated and mixed list input, declaration no-ops, and exact residue restoration.

## Task Commits

1. **Task 1: Canonical list composition** — `1e9754b` (test), `75d78ec` (feat)
2. **Task 2: Persisted declaration boundary** — `a9088b1` (test), `f38eaed` (feat)
3. **Task 3: Production store-to-live-zsh proof** — `543dfbf` (test)

## Files Created/Modified

- `core/activate/builder.go` — merges legacy and semantic list contributions into one ordered reducer and blocks unrepresentable declarations.
- `core/model/profile.go` — defines declaration representability without changing `EffectiveManaged` persistence semantics.
- `core/ir/regen.go` — keeps unsupported forced declarations verbatim.
- `core/activate/builder_test.go`, `core/model/profile_test.go`, `core/ir/regen_test.go`, `core/store/dto_test.go` — pin composition, representability, regeneration, and persistence contracts.
- `core/shell/zsh/pipeline_test.go`, `core/shell/zsh/residue_test.go` — exercise DTO round trips, syntax checks, live zsh, and snapshot restoration.

## Decisions Made

- Kept `ValueModeLegacy` as the sole compatibility route, but converted its existing same-list parse result into canonical tokens instead of directly appending manifest deltas.
- Kept `OverrideManaged` authoritative for persisted intent while requiring declaration representability separately at both regeneration and builder boundaries.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Resolved a local variable collision in the legacy reducer**

- **Found during:** Task 1
- **Issue:** The compatibility parser's existing `base` map conflicted with the new integer base position.
- **Fix:** Renamed the position to `baseIndex` before emitting the `ListDelta`.
- **Files modified:** `core/activate/builder.go`
- **Verification:** Focused builder composition tests passed.
- **Committed in:** `75d78ec`

**2. [Rule 3 - Blocking] Kept red tests hook-compatible**

- **Found during:** Task 2
- **Issue:** The pre-commit type-check prevented a red commit that referenced a method not yet implemented.
- **Fix:** Kept the red cases as compile-valid runtime failures; added the model predicate unit coverage with the green implementation commit.
- **Files modified:** `core/activate/builder_test.go`, `core/ir/regen_test.go`, `core/store/dto_test.go`, `core/model/profile_test.go`
- **Verification:** The red test run failed before implementation; the focused suite passed after implementation.
- **Committed in:** `a9088b1`, `f38eaed`

**Total deviations:** 2 auto-fixed (1 Rule 1, 1 Rule 3)

## Known Stubs

None.

## Issues Encountered

The residue helper observes emitted function definitions as live state. The new regression removes those test-local definitions after deactivation before comparing snapshots, matching the existing property test's reserved helper-name isolation.

## User Setup Required

None.

## Next Phase Readiness

The final Phase 4 plan is implemented and fully validated. Phase completion remains pending the parent workflow's independent post-wave review and verification.

---

*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-27*

## Self-Check: PASSED

All nine planned source/test files and the five task commits exist; no tracked-file deletions were introduced.
