---
phase: 04-manifest-builder-emit
plan: 15
subsystem: activation
tags: [go, dto, structural-fidelity, manifest, zsh, path, fpath, zero-residue]
requires:
  - phase: 04-14
    provides: source-ordered legacy and semantic list reduction plus declaration verbatim behavior
provides:
  - presence-aware, versioned structural-fidelity persistence for parser source markers
  - a shared fail-closed representability gate for regeneration and manifest construction
  - late-bound dynamic provenance for validated legacy PATH and FPATH additions
affects: [phase-04-verification, phase-05-loader]
tech-stack:
  added: []
  patterns: [presence-aware DTO compatibility, shared representability gate, canonical list provenance]
key-files:
  created: []
  modified: [core/model/profile.go, core/store/dto.go, core/ir/build.go, core/ir/regen.go, core/activate/builder.go, core/shell/zsh/pipeline_test.go, core/shell/zsh/residue_test.go]
key-decisions:
  - "Missing, partial, and unsupported structural-fidelity DTOs remain unknown and re-save without inferred false marker values."
  - "Only strict simple parameter-plus-path additions are late-bound in legacy list compatibility; shell-evaluation-shaped source remains rejected."
patterns-established:
  - "Current parser Blocks always persist all structural markers, while historical DTO compatibility is conservative."
  - "Persisted-profile safety claims are exercised through Regenerate, Build, Diff, Emit, and zsh -f sentinel checks."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Historical and current forced managed structural forms stay verbatim and create no manifest intent."
    requirement: SW-01
    verification:
      - kind: integration
        ref: core/shell/zsh/pipeline_test.go#TestPipelineLegacyStructuralFidelityMatrix
        status: pass
      - kind: integration
        ref: core/shell/zsh/pipeline_test.go#TestPipelinePersistedOverrideManaged
        status: pass
    human_judgment: false
  - id: D2
    description: "Persisted legacy PATH and FPATH additions retain runtime parameter expansion, source order, and zero-residue restoration."
    requirement: SW-02
    verification:
      - kind: integration
        ref: core/shell/zsh/pipeline_test.go#TestPipelineStoreRoundTripLegacyDynamic
        status: pass
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestResidueStoreRoundTripLegacy
        status: pass
    human_judgment: false
duration: 29min
completed: 2026-07-27
status: complete
---

# Phase 4 Plan 15: Persisted Fidelity Closure Summary

**Versioned structural syntax persistence and strict late-bound legacy PATH/FPATH provenance, proven through DTO-to-live-zsh no-op and zero-residue paths.**

## Performance

- **Duration:** 29 min
- **Started:** 2026-07-27T11:46:06Z
- **Completed:** 2026-07-27T12:15:00Z
- **Tasks:** 3
- **Files modified:** 12

## Accomplishments

- Added a versioned, presence-aware DTO object for append, array, and flagged-alias syntax, retaining known false values while failing closed for absent, partial, and unsupported historical records.
- Routed current parser markers through IR and one shared representability predicate so forced append, array, flagged-alias, and declaration forms regenerate verbatim without manifest operations.
- Preserved validated dynamic legacy PATH/FPATH additions through canonical reduction, emitted late-bound under `zsh -f`, and restored captured bases without residue.

## Task Commits

1. **Task 1: Encode structural fidelity with a conservative historical-DTO state** — `9237952` (feat)
2. **Task 2: Prove the legacy-DTO safety boundary before runtime list work** — `ea25358` (feat)
3. **Task 3: Preserve legacy list dynamic provenance after the compatibility proof** — `bea2ae3` (test), `65b1b6d` (feat)

## Files Created/Modified

- `core/model/profile.go`, `core/store/dto.go` — model and store-local versioned structural-fidelity contract.
- `core/ir/build.go`, `core/ir/regen.go` — parser marker copying and verbatim regeneration gate.
- `core/activate/builder.go` — representability-gated manifest construction and strict dynamic legacy-list classifier.
- `core/model/profile_test.go`, `core/store/dto_test.go`, `core/ir/build_test.go`, `core/ir/regen_test.go`, `core/activate/builder_test.go` — model, persistence, IR, and reducer regression coverage.
- `core/shell/zsh/pipeline_test.go`, `core/shell/zsh/residue_test.go` — persisted DTO-to-zsh sentinel and residue proofs.

## Decisions Made

- An unknown persisted syntax shape cannot be promoted to a known-false scalar shape during re-save or by `OverrideManaged`.
- Dynamic legacy list compatibility permits one simple parameter reference with path-safe literal suffixes; command substitution, separators, whitespace/control characters, cross-list references, and extra/middle self references remain rejected.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated direct current-profile test fixtures with explicit known structural fidelity**
- **Found during:** Task 3
- **Issue:** Fixtures that model newly created current entries lacked the new required known-state bit, causing the deliberately fail-closed gate to reject them as historical unknowns.
- **Fix:** Marked only direct current-entry fixtures as structurally known; raw historical-DTO fixtures remain unknown and are still tested as no-operation cases.
- **Files modified:** `core/activate/builder_test.go`, `core/shell/zsh/pipeline_test.go`, `core/shell/zsh/residue_test.go`
- **Verification:** Focused activation/zsh suites and the full uncached workspace suite passed.
- **Committed in:** `bea2ae3`

**Total deviations:** 1 auto-fixed (Rule 1)

## TDD Gate Compliance

RED executions were run for Task 1 and Task 2 before their implementation changes, but their red tests were not committed separately because the task commits grouped each completed implementation atomically. Task 3's test/feature commits are separate. This is a process-record warning only; all required focused and full verification passed.

## Known Stubs

None.

## Threat Flags

None. The plan adds no new endpoint, auth, filesystem, or schema trust boundary.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./...` — PASS
- `GOTOOLCHAIN=auto go build ./...` — PASS
- `GOTOOLCHAIN=auto go vet ./...` — PASS
- `make check` — PASS (`golangci-lint run`, then `go test ./...`)
- Live `zsh -f` checks in the pipeline and residue regressions — PASS with distinct runtime `HOME` and `EXTRA` values.

## Next Phase Readiness

The persisted-profile and legacy-list blockers are closed in code and automated live-shell coverage. Phase 4 remains executing pending the parent workflow's independent verification; this plan does not mark the phase complete.

## Self-Check: PASSED

All twelve declared source/test files exist, all four task commits are present, and no tracked-file deletions were introduced.
