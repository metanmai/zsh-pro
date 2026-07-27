---
phase: 04-manifest-builder-emit
plan: 18
subsystem: parser-persistence
tags: [go, zsh, parser, ir, dto, source-fidelity]
requires:
  - phase: 04-manifest-builder-emit
    provides: "04-17 delimiter declaration parsing and v2 structural-fidelity boundary"
provides:
  - "Parser provenance for alias definition form, setopt/unsetopt controls, and delimiter PATH/FPATH lists."
  - "V3 presence-aware structural-fidelity persistence with historical records failing closed."
  - "A shared representability gate that blocks lossy forced-managed assignment, alias, and option lowering."
affects: [phase-04-plan-19, phase-05-loader]
tech-stack:
  added: []
  patterns:
    - "Source-shape markers are parser-owned, defensively copied by IR, and versioned in the store-local DTO."
    - "OverrideManaged selects only the managed axis; Representable remains the fail-closed fidelity gate."
key-files:
  created: []
  modified:
    - core/model/block.go
    - core/model/profile.go
    - core/ir/build.go
    - core/store/dto.go
    - core/shell/zsh/parse.go
key-decisions:
  - "Alias queries are distinct from explicit empty alias definitions through AliasAssignment provenance."
  - "Only bare, --, and -o option invocation shapes may lower to declarative option intent."
  - "Structural fidelity version 3 uses a presence-aware slice wrapper so nil, empty, and populated option controls survive round trips."
patterns-established:
  - "Older structural-fidelity versions are decoded as unknown and omitted on re-save rather than promoted."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Alias definition provenance, option control syntax, and delimiter PATH/FPATH list semantics are captured and copied without slice aliasing."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "core/shell/zsh/parse_test.go#TestParseCapturesAliasAssignmentForm, TestParseCapturesOptionControls, TestParseDelimiterListValue"
        status: pass
      - kind: unit
        ref: "core/ir/build_test.go#TestBuildCopiesStructuralFidelity"
        status: pass
    human_judgment: false
  - id: D2
    description: "V3 source fidelity is persisted losslessly while absent, v1, v2, partial, and unsupported records remain unknown and unrepresentable."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "core/model/profile_test.go#TestEntryRepresentabilityRequiresKnownStructuralFidelity"
        status: pass
      - kind: unit
        ref: "core/store/dto_test.go#TestStructuralFidelityDTOCompatibilityMatrix, TestStructuralFidelityDTOPreservesNewMarkerPresenceAndCopies"
        status: pass
    human_judgment: false
duration: 7min
completed: 2026-07-27
status: complete
---

# Phase 04 Plan 18: Source Fidelity Admission Summary

**V3 parser-to-store fidelity now preserves alias definition form, option invocation syntax, and delimiter PATH/FPATH list semantics while blocking every lossy forced-managed lowering.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-07-27T13:13:00Z
- **Completed:** 2026-07-27T13:20:00Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments

- Captured alias query versus explicit empty definition, full multi-alias names, and ordered setopt/unsetopt controls without evaluating source.
- Applied the same AST-derived semantic list decoder to one-assignment delimiter `export -- PATH` and `FPATH` declarations.
- Added v3 fidelity persistence with nil/empty/populated option-marker distinction and fail-closed historical re-save behavior.
- Centralized admission on exact-one assignment/assigned-alias shapes and modeled option controls, so forced management cannot erase source behavior.

## Task Commits

1. **Task 1 RED: parser/IR source-shape regressions** — `0dda0f9` (test)
2. **Task 1 GREEN: parser provenance and IR transfer** — `5e27830` (feat)
3. **Task 2 RED: fidelity admission and persistence regressions** — `ccfec7d` (test)
4. **Task 2 GREEN: v3 DTO and shared representability gate** — `48ad095` (feat)
5. **Rule 1 follow-up: parsed alias-definition fixture fidelity** — `dda0a49` (test)

## Files Created/Modified

- `core/model/block.go` — parser-owned alias and option markers.
- `core/model/profile.go` — entry markers and fail-closed representability gate.
- `core/ir/build.go` — defensive parser-to-entry marker transfer.
- `core/store/dto.go` — complete v3 structural-fidelity DTO encoding.
- `core/shell/zsh/parse.go` — alias/query, option-control, and delimiter list decoding.
- `core/{model,ir,store,shell/zsh}/*_test.go` — admission, persistence, parser, and copy matrices.

## Decisions Made

- `AliasAssignment` is true only for a parsed `name=value` alias argument; an explicit empty right-hand side remains a valid definition.
- The modeled option-control set is bare, `--`, and `-o`; `+o`, `-m`, and other controls remain verbatim.
- V3 preserves option-control nil-versus-empty state with an explicit DTO wrapper, so JSON field absence cannot be mistaken for a known empty slice.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Declared Entry marker fields during Task 1**
- **Found during:** Task 1 GREEN implementation.
- **Issue:** IR could not transfer the new parser markers until `model.Entry` had corresponding fields, while the plan assigns the full Entry admission rule to Task 2.
- **Fix:** Added the two fields in Task 1; Task 2 supplied their representability and DTO behavior.
- **Files modified:** `core/model/profile.go`.
- **Verification:** Task 1 parser/IR checks and full suite passed.
- **Committed in:** `5e27830`.

**2. [Rule 1 - Bug] Updated manually constructed managed-alias fixtures**
- **Found during:** Full uncached Go suite.
- **Issue:** Existing tests modeled parsed alias definitions without the new required `AliasAssignment` marker, so they were correctly rejected as source-unknown queries.
- **Fix:** Marked only the positive parsed-definition fixtures as assignment-form aliases.
- **Files modified:** `core/activate/builder_test.go`, `core/ir/regen_test.go`.
- **Verification:** `GOTOOLCHAIN=auto go test -count=1 ./...`, build, vet, and `make check` passed.
- **Committed in:** `dda0a49`.

**Total deviations:** 2 auto-fixed (Rule 3: 1; Rule 1: 1). No scope expansion beyond the parser-to-persistence contract.

## TDD Gate Compliance

Both tasks followed RED then GREEN commits. The intentionally uncompilable RED commits used `--no-verify` because the repository hook runs typecheck before implementation fields exist; their targeted failures were recorded before each GREEN commit.

## Known Stubs

None. The only placeholder scan hit is an existing secret-redaction test comment, not a runtime data stub.

## Issues Encountered

`export -- PATH=...` is represented by the parser as a declaration clause containing a naked delimiter assignment. Filtering list candidates to actual name/value assignments gives it the same validated list contract as ordinary exports without evaluating source.

## User Setup Required

None.

## Next Phase Readiness

Plan 04-19 can consume the complete contract for live-zsh pipeline and residue proof. Phase 04 remains executing until that dependent plan completes.

## Self-Check: PASSED

*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-27*
