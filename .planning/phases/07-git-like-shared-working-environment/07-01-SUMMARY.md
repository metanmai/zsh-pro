---
phase: 07-git-like-shared-working-environment
plan: 01
subsystem: worktree-domain
tags: [worktree, semantic-diff, live-state, tombstones, revision-safety]
requires:
  - phase: 02-profile-ir
    provides: complete source-ordered model.Profile
  - phase: 04-manifest-builder-emit
    provides: presence-aware scalar/list/function/option conventions
provides:
  - Lossless shell-neutral live identity, value, snapshot, delta, projection, and workflow contracts
  - Model-owned final-occurrence, tombstone, ordered-list, and semantic-equality authority
  - Complete-input semantic snapshot diff, overlay application, committed projection, and categorized output
affects: [07-02-admission, 07-03-state-service, 07-04-activation-patch, 07-05-store-dto, 07-09-runtime-hooks]
tech-stack:
  added: []
  patterns: [presence-aware typed values, final-occurrence normalization, complete-input validation, value-free public metadata]
key-files:
  created:
    - core/model/worktree.go
    - core/model/worktree_test.go
    - core/worktree/diff.go
    - core/worktree/diff_test.go
  modified: []
key-decisions:
  - "LiveValue uses explicit Present plus one kind-selected pointer/list payload, so empty scalar/list/false option values remain distinct from removal."
  - "NormalizeLiveStates, NormalizeOverlay, and EqualLiveValue in core/model are the sole semantic authority consumed by worktree diff/projection code."
  - "Overlay updates retain existing source-order positions, new identities append in final overlay order, and final tombstones suppress every shadowed source occurrence."
requirements-completed: [WORK-02, SYNC-02]
coverage:
  - id: D1
    description: "Bounded lossless live-state domain contract with exact presence, kind/name validation, defensive copies, and value-free workflow metadata"
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/model/worktree_test.go#TestLiveValuePresenceCompatibilityAndEquality"
        status: pass
      - kind: unit
        ref: "core/model/worktree_test.go#TestSnapshotExactBoundsAndRevisionOverflow"
        status: pass
      - kind: security
        ref: "core/model/worktree_test.go#TestAttachResolveAndValueFreeMetadataContracts"
        status: pass
    human_judgment: false
  - id: D2
    description: "Deterministic per-identity semantic diff using final occurrence, exact typed equality, and stable kind/name ordering"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "core/worktree/diff_test.go#TestDiffSnapshotFinalOccurrenceSemanticEqualityAndOrdering"
        status: pass
      - kind: unit
        ref: "core/worktree/diff_test.go#TestDiffInvalidPairReturnsNoPartialChanges"
        status: pass
    human_judgment: false
  - id: D3
    description: "Source-order-preserving overlay/projection with authoritative tombstones and value-free categorized output"
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/worktree/diff_test.go#TestOverlayPreservesSourceOrderAndTombstonesShadowedOccurrences"
        status: pass
      - kind: unit
        ref: "core/worktree/diff_test.go#TestProjectionRetainsFinalTombstonesAndRejectsPartialOutput"
        status: pass
      - kind: unit
        ref: "core/worktree/diff_test.go#TestCategorizeDiffStableKindAndExactNameOrdering"
        status: pass
    human_judgment: false
metrics:
  duration: 13min
  completed: 2026-08-17
status: complete
---

# Phase 07 Plan 01: Lossless Worktree Contract and Semantic Diff Summary

**A bounded presence-aware live-state model now drives deterministic per-identity diffs and source-preserving tombstone projections without shell syntax or captured values in public metadata.**

## Performance

- **Duration:** 13 min
- **Started:** 2026-08-17T10:44:13Z
- **Completed:** 2026-08-17T10:57:27Z
- **Tasks:** 2/2
- **Files created:** 4

## Accomplishments

- Added the complete Phase 7 shell-neutral domain model: six closed live kinds, exact identities, explicit presence, scalar/list/option payloads, snapshots, changes, overlays, committed source-plus-projection documents, revisions, conflicts, exclusions, attach/publish/pull/acknowledge/resolve contracts, status/diff summaries, and commit results.
- Centralized validation, final-occurrence normalization, semantic equality, tombstone precedence, deep-copy behavior, 2 MiB/10,000-record snapshot bounds, and overflow-safe revision transitions in `core/model`.
- Added complete-input `DiffSnapshot`, `ApplyOverlay`, `BuildProjection`, and `CategorizeDiff` transforms that return no partial output on malformed input, preserve ordered PATH/FPATH elements, and emit stable env/alias/function/PATH/FPATH/option plus exact-name order.
- Kept the new model/worktree packages shell-neutral: no concrete zsh import, executable shell text, filesystem/network side effect, JSON comparison, or whole-profile replacement request was introduced.

## Task Commits

1. **Task 1 RED: live worktree model contract** — `086fa4c` (test)
2. **Task 1 GREEN: live worktree domain model** — `da25411` (feat)
3. **Task 2 RED: semantic worktree diff contract** — `1e8b290` (test)
4. **Task 2 GREEN: deterministic semantic diff/projection** — `ac3871c` (feat)

## Decisions Made

- Present-empty values are represented by `Present: true` and a selected payload pointer/list; removal is `Present: false` with no payload, while committed removal remains an explicit tombstone.
- `core/model/worktree.go` owns normalization and semantic equality. `core/worktree/diff.go` delegates to those helpers and owns only validation orchestration, comparison, overlay application, and presentation ordering.
- Existing identities retain their final source-order position through overlay updates; newly admitted identities append deterministically, and tombstones remove the identity rather than revealing an earlier repeated assignment.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added compile-only API scaffolds for linted RED commits**

- **Found during:** Task 1 RED and Task 2 RED
- **Issue:** The mandatory pre-commit hook runs `golangci-lint`, which rejects Go RED tests that reference not-yet-declared symbols before the failing contract can be committed.
- **Fix:** Added minimal compile-only public declarations returning explicit not-implemented errors, confirmed the RED tests failed behaviorally, committed each RED gate through the normal hook, then replaced every scaffold in its GREEN implementation.
- **Files modified:** `core/model/worktree.go`, `core/worktree/diff.go`
- **Verification:** Production stub scan is clean; targeted GREEN tests, vet, hook lint, and full repository tests pass.
- **Commits:** `086fa4c`, `1e8b290`, replaced by `da25411`, `ac3871c`

**Total deviations:** 1 auto-fixed blocking class applied consistently to both TDD tasks. **Impact:** Preserved normal hooks and observable RED/GREEN history; no scaffold remains in production.

## TDD Gate Compliance

- RED commits exist and precede GREEN commits for both tasks.
- Each RED run failed on the intended unimplemented contract after the compile-only scaffold was added.
- Each GREEN run passed the exact task verification command; no separate refactor commit was necessary.

## Verification

- `GOTOOLCHAIN=local go test ./core/model -run 'Test(Live|Worktree|Committed|Attach|Resolve|Snapshot|Revision)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/worktree -run 'Test(ValidateSnapshot|Diff|Overlay|Projection|Categorize)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/model ./core/worktree -count=1` — passed.
- `GOTOOLCHAIN=local go vet ./core/model ./core/worktree` — passed.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed across the repository.
- All four commits passed the normal `golangci-lint` pre-commit hook; no `--no-verify` bypass was used.
- Structural scan confirmed neither production package imports `core/shell/zsh` or introduces executable shell mutation text.

## Known Stubs

None. The compile-only RED scaffolds were fully replaced in the GREEN commits.

## Issues Encountered

None beyond the resolved lint-hook RED-commit deviation above. The GSD requirements updater found no `WORK-02` or `SYNC-02` records in the current `REQUIREMENTS.md`, so completion is recorded in this SUMMARY coverage/frontmatter without inventing missing requirement rows.

## Next Phase Readiness

Ready for Plan 07-02 to build admission and secret-policy boundaries on the validated `Identity`, snapshot, exclusion, and committed projection contracts.

## Self-Check: PASSED

- All four declared implementation/test files and this SUMMARY exist.
- All four RED/GREEN task commits are present in repository history.
- Plan-scoped tests and vet passed again after SUMMARY creation.
