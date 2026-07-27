---
phase: 04-manifest-builder-emit
plan: 16
subsystem: profile-fidelity
tags: [go, zsh, parser, dto, structural-fidelity, manifest, activation]
requires:
  - phase: 04-15
    provides: presence-aware structural fidelity and a shared representability boundary
provides:
  - parser-owned indexed-assignment and semantic declaration-flag metadata
  - version-2 complete structural-fidelity persistence with conservative legacy decoding
  - a forced-managed safety gate that leaves indexed and flagged declarations verbatim and out of manifests
affects: [04-17, phase-04-verification, profile-store, activation]
tech-stack:
  added: []
  patterns: [complete-v2 DTO presence checks, fail-closed parser source fidelity, shared representability gate]
key-files:
  created: []
  modified: [core/model/block.go, core/model/profile.go, core/store/dto.go, core/ir/build.go, core/shell/zsh/parse.go]
key-decisions:
  - "Only complete structural-fidelity v2 records are known; v1, partial, absent, and unsupported records re-save without fidelity data."
  - "OverrideManaged chooses intent but cannot erase indexed or declaration-attribute semantics."
patterns-established:
  - "Parser-owned source shape crosses Block to Entry through copied metadata, then persists only as a complete versioned record."
  - "Unknown or behavior-bearing source forms are verbatim/no-operation in both regeneration and manifest construction."
requirements-completed: [SW-01]
coverage:
  - id: D1
    description: Indexed assignments and declaration attributes retain source fidelity through parser, profile, and DTO boundaries.
    requirement: SW-01
    verification:
      - kind: unit
        ref: core/shell/zsh/parse_test.go#TestParseCapturesIndexedAndDeclarationFlags
        status: pass
      - kind: unit
        ref: core/store/dto_test.go#TestStructuralFidelityDTOCompatibilityMatrix
        status: pass
    human_judgment: false
  - id: D2
    description: Forced managed indexed and semantic flagged exports remain verbatim and emit no activation intent, while ordinary exports remain supported.
    requirement: SW-01
    verification:
      - kind: unit
        ref: core/ir/regen_test.go#TestRegenerateForcedManagedStructuralShapesStayVerbatim
        status: pass
      - kind: unit
        ref: core/activate/builder_test.go#TestBuildRejectsOverrideManagedDeclarations
        status: pass
    human_judgment: false
duration: 6min
completed: 2026-07-27
status: complete
---

# Phase 4 Plan 16: Indexed and Declaration Fidelity Summary

**Indexed assignments and semantic export attributes now persist as explicit source shape, with v2 compatibility and fail-closed regeneration/manifest lowering.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-27T12:26:43Z
- **Completed:** 2026-07-27T12:32:07Z
- **Tasks:** 2
- **Files modified:** 12

## Accomplishments

- Captured numeric and associative assignment subscripts plus semantic declaration flags, while preserving ordinary scalar data and `export --` behavior.
- Advanced structural-fidelity persistence to complete, presence-aware v2 metadata; legacy and partial records stay unknown and omit fidelity on re-save.
- Reused the shared representability boundary so forced indexed and flagged declarations regenerate verbatim and produce no manifest operation; ordinary unflagged exports remain exported scalars.

## Task Commits

1. **Task 1: Capture subscripts and semantic declaration flags in every parser assignment path** — `92436dd` (feat)
2. **Task 2: Persist complete fidelity and share the fail-closed regeneration and builder gate** — `0c9e854` (test), `f20db32` (feat)
3. **Rule 2 follow-up: fail closed on runtime-dependent declaration attributes** — `aad3540` (fix)

## Files Created/Modified

- `core/model/block.go`, `core/shell/zsh/parse.go` — capture parser-owned indexed and declaration-flag source shape.
- `core/model/profile.go`, `core/store/dto.go` — define the v2 fidelity contract and reject behavior-bearing metadata from representation.
- `core/ir/build.go` — copy metadata without aliasing and retain unknown fidelity for opaque parser inputs.
- `core/*/*_test.go` — cover parser, model, DTO compatibility, IR regeneration, and manifest builder safety paths.

## Decisions Made

- A complete empty declaration-flag sequence is persisted distinctly from absent metadata; absence remains unknown.
- A dynamic declaration argument is opaque because it may become an option only at runtime; it cannot be treated as a known unflagged declaration.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Made runtime-dependent declaration arguments fail closed**
- **Found during:** Task 2
- **Issue:** A dynamic declaration argument could expand into a semantic option while appearing unflagged in the AST-derived metadata.
- **Fix:** Marked the statement opaque and propagated opaque parser input as unknown structural fidelity, excluding it from forced managed lowering.
- **Files modified:** `core/shell/zsh/parse.go`, `core/shell/zsh/parse_test.go`, `core/ir/build.go`
- **Verification:** Focused parser and all core/full Go test suites passed.
- **Committed in:** `aad3540` (with the dependent IR boundary in `f20db32`)

**2. [Rule 1 - Bug] Preserved complete empty declaration-flag slices across parser/store round trips**
- **Found during:** Task 2
- **Issue:** Copying an empty slice with a nil append target converted it to nil, breaking exact current-profile round trips after DTO decoding.
- **Fix:** Materialized a non-nil empty declaration-flag list for complete parser output and used length-preserving cloning.
- **Files modified:** `core/ir/build.go`, `core/ir/build_test.go`
- **Verification:** `TestRoundTripReadCommit` and the full uncached suite passed.
- **Committed in:** `f20db32`

**Total deviations:** 2 auto-fixed (Rule 2: 1, Rule 1: 1). Both preserve the plan's fail-closed source-fidelity goal without expanding the manifest or renderer scope.

## Known Stubs

None. The placeholder-text scan found only an existing secret-exclusion test fixture, not a runtime stub.

## Threat Flags

None. This plan adds no new network, authentication, filesystem, or schema boundary; it hardens the existing source-to-profile trust boundary.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run 'TestParse(Captures|Retains)' -v` — PASS
- Focused model/store/IR/activation fidelity tests — PASS (the plan's supplied anchored expression only selects the DTO matrix; explicit named tests also exercised the model, IR, and builder cases).
- `GOTOOLCHAIN=auto go test -count=1 ./core/model ./core/store ./core/ir ./core/activate ./core/shell/zsh` — PASS
- `GOTOOLCHAIN=auto go test -count=1 ./...` — PASS
- `GOTOOLCHAIN=auto go vet ./...` — PASS

## Next Phase Readiness

Plan 17 can now consume persisted v2 source fidelity for its live-zsh and residue proof. It was not executed by this plan.

## Self-Check: PASSED

All declared source/test files and all four task commits are present; no tracked-file deletions were introduced.

*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-27*
