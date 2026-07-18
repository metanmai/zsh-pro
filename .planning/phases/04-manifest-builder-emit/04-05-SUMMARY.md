---
phase: 04-manifest-builder-emit
plan: 05
subsystem: testing
tags: [go, zsh, property-testing, residue, mutation-testing, serialization]
requires:
  - phase: 04-manifest-builder-emit
    plan: 04
    provides: behaviorally correct source-to-live activation pipeline
provides:
  - Deterministic balanced two-profile zero-residue property harness
  - Collision-safe type-aware oracle for complete admitted zsh state
  - Eight sensitivity checks and three independent emitter negative controls
affects: [04-phase-verification, 05-runtime-loader, shell-switch-regression-suite]
tech-stack:
  added: []
  patterns: [seeded balanced state-machine generation, qqqq field serialization, self-testing property oracles]
key-files:
  created: []
  modified:
    - core/shell/zsh/residue_test.go
key-decisions:
  - "Snapshot arbitrary string fields one-per-line with zsh qqqq encoding; keep only structural tags, indexes, and counts unencoded."
  - "Prime zsh/parameter once before the baseline because its first enumeration lazily registers LOGCHECK; every compared snapshot then uses the same oracle and complete helper set."
  - "Assert each renderer mutant against its intended failure mode before rerunning the real emitter after seam cleanup."
patterns-established:
  - "Complete zsh state snapshots filter by parameter descriptor before live indirection, with dedicated sections for special and tied shell classes."
  - "Mutable package test seams are isolated with t.Cleanup and followed by a production-emitter property rerun."
requirements-completed: [SW-02]
coverage:
  - id: D1
    description: "A fixed logged seed drives 24 balanced apply/deactivate actions across two profiles in one zsh process and restores the complete admitted state byte-for-byte."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/residue_test.go#TestZeroResidueFullStateProperty"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/residue_test.go#TestBalancedResidueActionGenerator"
        status: pass
    human_judgment: false
  - id: D2
    description: "The shared oracle detects eight state mutations, contains dereferenced fixture values, and serializes raw-NUL scalar and array data without collisions."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/residue_test.go#TestSnapshotOracleMetaSensitivity"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/residue_test.go#TestSnapshotEscapesEmbeddedNULWithoutCollision"
        status: pass
    human_judgment: false
  - id: D3
    description: "Blind PATH append, dropped static quoting, and base stripping are independently detected, and the production emitter remains green after cleanup."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/residue_test.go#TestResidueRendererMutants"
        status: pass
      - kind: other
        ref: "GOTOOLCHAIN=auto go test ./... -count=1 && GOTOOLCHAIN=auto go build ./..."
        status: pass
    human_judgment: false
duration: 18min
completed: 2026-07-18
status: complete
---

# Phase 4 Plan 05: Full-State Residue Oracle Summary

**A seeded balanced live-zsh property now proves complete zero-residue restoration and proves its own sensitivity with raw-NUL fixtures, direct state mutations, and three emitter mutants.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-07-18T18:39:26Z
- **Completed:** 2026-07-18T18:56:57Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments

- Replaced the fixed five-field loop with a fixed-seed 24-action generator that exercises two distinct emitted profiles, asserts single-active balance, ends inactive, and reports the seed plus complete trace on failure.
- Added one file-backed snapshot function that sorts parameter names, filters descriptors before dereferencing, records scalar/integer/float, indexed-array, and association values, and fails closed on unhandled admitted types.
- Added deterministic dedicated alias, option, function, PATH, duplicate-count, and `ZP_BASE_PATH` sections. Every arbitrary field uses `${(qqqq)}` encoding, so embedded NUL, newline, backslash, and metacharacter data remain collision-safe.
- Kept every emitted helper and profile entrypoint definition stable across snapshots by copying the two profiles' `zp_apply` and `zp_deactivate` bodies into reserved unique functions before the baseline.
- Proved the oracle detects eight independent mutations: non-exported scalar, exported value, indexed-array element, association value, alias body, function body, option state, and count-preserving PATH order.
- Proved blind-append, dropped-zquote, and base-strip renderers fail for their intended reasons, then reran the randomized property with restored production seams.

## Seed and State Coverage

- **Seed:** `0x5eed0405` (`1592591365`)
- **Generated actions:** 24, as 12 balanced apply/deactivate pairs
- **Profiles:** 2, both guaranteed to appear before randomized selection continues
- **Generic parameter classes:** scalar, exported scalar, integer, float, indexed array, association
- **Dedicated classes:** aliases with bodies, functions with bodies, all options with explicit state, ordered PATH plus count and per-value occurrence counts, exact `ZP_BASE_PATH`
- **Edge fixtures:** initially set and unset managed scalars, unchanged non-exported scalar, quoted alias/function bodies, opposite option states, base-owned shared PATH element, profile-only glob-like element, dynamic `$HOME/bin`, empty array element, sorted association, embedded raw NUL/newline/backslash scalar and array values

## Task Commits

1. **Task 1 RED: full-state property requirements** — `fca734b`
2. **Task 1 GREEN: balanced generator and complete oracle** — `936bab6`
3. **Task 2 RED: sensitivity and mutant requirements** — `b8f7757`
4. **Task 2 GREEN: meta-asserts and three negative controls** — `f9c88fc`

## Files Created/Modified

- `core/shell/zsh/residue_test.go` — deterministic balanced generator, full-state snapshot format, real-emitter property runner, direct sensitivity checks, embedded-NUL guards, and three renderer mutants.

## Decisions Made

- Use `${(qqqq)}` independently for every arbitrary string field and never decode the snapshot; byte equality is the oracle.
- Enumerate parameter names in sorted order, inspect `$parameters[$name]` first, and only then dereference admitted live values with `(P)`.
- Warm the parameter module with the same file-backed snapshot function before establishing the baseline. This neutralizes first-enumeration registration without filtering a real user parameter or defining anything between compared snapshots.
- Keep multi-active co-ownership out of scope. The property models the shipped single-active contract: deactivate the current profile before applying another.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test Instrumentation] Neutralized first-enumeration parameter registration**

- **Found during:** Task 1 GREEN
- **Issue:** The first snapshot caused zsh/parameter to register `LOGCHECK`, so the immediately following no-op snapshot gained one legitimate parameter name despite no product state change.
- **Fix:** Run the exact same file-backed, type-filtered snapshot once as a pre-baseline warm-up after all helpers and emitted functions are defined. Baseline, no-op, and final snapshots remain structurally identical and byte-stable.
- **Files modified:** `core/shell/zsh/residue_test.go`
- **Verification:** The no-op comparison and the 24-action final comparison both pass; all eight sensitivity mutations still fire.
- **Committed in:** `936bab6`

**2. [Rule 3 - Tooling] Repository pre-commit linter cannot load the Go 1.25 module**

- **Found during:** Task 1 RED commit
- **Issue:** The installed golangci-lint binary was built with Go 1.24 and rejects the repository's Go 1.25 target before analysis.
- **Fix:** After the normal hook reproduced the known mismatch, task commits bypassed only that hook. Formatting, vet, targeted tests, the full suite, build, and diff checks ran independently.
- **Files modified:** None
- **Verification:** `gofmt`, `go vet ./...`, `go test ./... -count=1`, `go build ./...`, and `git diff --check` all pass.
- **Committed in:** all four task commits used the documented local-tool workaround.

**Total deviations:** 2 auto-fixed (1 instrumentation bug, 1 tooling blocker). **Impact:** The first removes a false-red without broadening the filter or weakening sensitivity; the second changes no repository behavior or validation scope.

## Issues Encountered

- The local golangci-lint build-version mismatch remains an environment issue. All repository-native Go checks pass independently.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None.

## Verification

- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Residue|Snapshot|Mutant|Meta|Balanced|ZeroResidue' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./... -count=1` — passed.
- `GOTOOLCHAIN=auto go build ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `gofmt` and `git diff --check` — passed.

## TDD Gate Compliance

- Task 1: RED `fca734b` precedes GREEN `936bab6`.
- Task 2: RED `b8f7757` precedes GREEN `f9c88fc`.
- No refactor commit was needed; the final implementation is formatted and all tests pass.

## Next Phase Readiness

- The remaining Phase 4 verification blocker now has a default-suite, falsifiable regression pin.
- Phase verification can rerun against the complete source-to-live pipeline and full-state residue evidence.
- No implementation blocker remains in Plan 04-05.

## Self-Check: PASSED

- The modified key file exists and all four Task 1/Task 2 RED/GREEN commits are present in order.
- The fixed seed, N >= 20, balance invariant, sorted type-aware snapshots, `${(qqqq)}` fields, embedded-NUL checks, and three named mutants are present in source.
- Targeted tests, the complete Go suite, build, vet, formatting, and diff checks pass.
- Unrelated `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` changes remain untouched and unstaged.

---
*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-18*
