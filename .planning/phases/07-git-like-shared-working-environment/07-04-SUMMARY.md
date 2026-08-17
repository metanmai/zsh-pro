---
phase: 07-git-like-shared-working-environment
plan: 04
subsystem: activation-shell-codegen
tags: [committed-projection, live-patch, zsh-codegen, tombstones, reversible-state]
requires:
  - phase: 07-01-lossless-worktree-contract
    provides: model-owned live normalization, semantic equality, overlays, tombstones, and committed worktree contracts
  - phase: 07-02-admission-and-live-secret-boundary
    provides: pinned SecretRef ownership and live-value exclusion boundaries
provides:
  - Complete source-preserving committed worktree projection construction with authoritative tombstones
  - Deterministic shell-neutral forward and replacement-reverse live patch operations
  - Validated canonical zsh regeneration for every supported committed live-state category
  - Exact reversible PATH and FPATH tied scalar/array transitions
affects: [07-05-store-dto, 07-06-cli, 07-08-live-provider, 07-09-runtime-hooks, 07-10-e2e]
tech-stack:
  added: []
  patterns: [source-plus-projection persistence, complete-input validation, typed emit ownership, replacement reverse planning]
key-files:
  created:
    - core/activate/live_patch_test.go
  modified:
    - core/activate/plan.go
    - core/activate/builder.go
    - core/activate/diff.go
    - core/shell/zsh/emit.go
    - core/shell/zsh/emit_test.go
    - core/shell/zsh/regen.go
    - core/shell/zsh/regen_test.go
key-decisions:
  - "Keep core/model normalization and equality as the semantic authority; because activate cannot import worktree without closing an existing dependency cycle, pin the thin activation traversal to worktree.DiffSnapshot with external golden tests."
  - "Retain both exact list endpoints in TransitionLiveList so concrete emitters can apply one ordered target and generate its byte-identical tied scalar/array replacement reverse."
  - "Keep RegenerateWorktree limited to complete-document validation and typed traversal; every added executable assignment, declaration, option, and removal string remains in emit.go."
  - "Derive existing environment export provenance from the preserved source history and treat newly projected LiveEnv identities as exported environment variables."
patterns-established:
  - "Committed source remains immutable history; a separately versioned live projection carries exact final state and authoritative removals."
  - "Live mutation plans are closed shell-neutral operations, while concrete quoting and executable source stay in the zsh emitter."
requirements-completed: [WORK-02, WORK-03, SYNC-01]
coverage:
  - id: D1
    description: "Complete Profile history is retained beside a versioned final-state projection whose ordered values, present-empty states, and tombstones follow model-owned semantics"
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/activate/live_patch_test.go#TestBuildEffectivePreservesCompleteSourceAndProjection"
        status: pass
      - kind: unit
        ref: "core/activate/live_patch_test.go#TestBuildEffectiveRejectsMalformedOverlayWithoutPartialDocument"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every semantic identity change produces one deterministic forward operation and one replacement reverse operation equivalent to categorized worktree diff"
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "core/activate/live_patch_test.go#TestBuildLivePatchMatchesCategorizedWorktreeDiff"
        status: pass
      - kind: unit
        ref: "core/activate/live_patch_test.go#TestBuildLivePatchRejectsMalformedCompleteInput"
        status: pass
    human_judgment: false
  - id: D3
    description: "Validated committed projections regenerate behavior-exact zsh for scalars, export state, aliases, multiline functions, lists, options, and tombstones with emit.go as sole new syntax owner"
    requirement: WORK-03
    verification:
      - kind: integration
        ref: "core/shell/zsh/regen_test.go#TestRegenerateWorktreeBehaviorExactProjectionAndTombstones"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/regen_test.go#TestRegenerateWorktreeRejectsMalformedOrHostileProjectionWithoutBytes"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/regen_test.go#TestWorktreeSyntaxOwnedByEmit"
        status: pass
    human_judgment: false
  - id: D4
    description: "PATH and FPATH remove exactly one owned occurrence while preserving unmanaged elements, duplicates, empties, order, and byte-identical tied scalar/array reverse state"
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestLivePatchListOccurrenceRemovalAndReplacementReverse"
        status: pass
    human_judgment: false
metrics:
  duration: 20min
  completed: 2026-08-17
status: complete
---

# Phase 07 Plan 04: Exact Committed Projection and Reversible Zsh State Summary

**Complete source history now carries an exact typed final-state projection, deterministic replacement-reverse patch ownership, and canonical zsh regeneration for every supported live category.**

## Performance

- **Duration:** 20 min
- **Started:** 2026-08-17T12:11:14Z
- **Completed:** 2026-08-17T12:31:39Z
- **Tasks:** 2/2
- **Files created:** 1
- **Files modified:** 7

## Accomplishments

- Added `BuildEffective`, which preserves the entire source-ordered Profile and projects normalized final scalar, alias, function, PATH/FPATH, option, present-empty, and tombstone state without persisting SecretRef values.
- Added closed shell-neutral live operations plus `BuildLivePatch`, producing deterministic forward mutations and one complete replacement reverse with semantic equivalence pinned to categorized worktree diff.
- Added fail-closed committed-worktree validation and zsh-owned typed emission for exact values, source-derived export provenance, multiline functions, explicit removals, and options.
- Proved real-zsh PATH and FPATH occurrence removal preserves duplicates, empty and unmanaged elements, ordering, tied scalar/array state, and byte-identical reversal.

## Task Commits

1. **Task 1 RED: committed projection and live patch contract** — `393cb36` (test)
2. **Task 1 GREEN: exact projection and replacement reverse planning** — `1e16539` (feat)
3. **Task 2 RED: committed worktree rendering contract** — `5381ec1` (test)
4. **Task 2 GREEN: canonical zsh worktree rendering** — `7d4f705` (feat)

## Files Created/Modified

- `core/activate/plan.go` — Defines closed scalar, list, option, removal, and replacement-reverse live patch operations.
- `core/activate/builder.go` — Projects complete source plus overlay into ordered committed final state and tombstones.
- `core/activate/diff.go` — Validates complete snapshots and builds deterministic forward/reverse live operations from model semantic authority.
- `core/activate/live_patch_test.go` — Pins source preservation, projection ordering, semantic diff equivalence, reversibility, malformed-input behavior, and token-free boundaries.
- `core/shell/zsh/emit.go` — Sole owner of added committed/live scalar, alias, function, list, option, and tombstone zsh syntax.
- `core/shell/zsh/emit_test.go` — Covers every live category plus real-zsh PATH/FPATH occurrence and tied-state reversal.
- `core/shell/zsh/regen.go` — Validates committed documents, excludes pinned identities, preserves export provenance, and delegates typed rendering.
- `core/shell/zsh/regen_test.go` — Covers exact source behavior, SecretRef canaries, hostile/malformed zero output, legacy compatibility, and AST call ownership.

## Decisions Made

- Model normalization and typed value equality remain the only semantic authority. Activation's local traversal exists solely to avoid the existing `activate -> worktree -> shell -> activate` import cycle and is locked to `worktree.DiffSnapshot` by an external golden test.
- PATH/FPATH transitions carry exact before and after arrays. Applying the exact target makes occurrence ownership observable and deterministic, while the inverse endpoint restores both tied array and scalar bytes.
- `RegenerateWorktree` validates the whole document before emitting any bytes and delegates all syntax to typed helpers in `emit.go`; it does not build command or quoting fragments.
- Existing environment export behavior is reconstructed from complete source provenance. A projected environment identity absent from source is emitted as exported, matching the `LiveEnv` contract.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Avoided an activation/worktree import cycle**

- **Found during:** Task 1 GREEN
- **Issue:** Directly importing `core/worktree` from `core/activate` would close the existing `activate -> worktree -> shell -> activate` dependency cycle.
- **Fix:** Consumed `core/model` normalization and equality directly in a thin deterministic traversal, then added an external cross-package golden test comparing operation identities against `worktree.DiffSnapshot` and categorized output.
- **Files modified:** `core/activate/diff.go`, `core/activate/live_patch_test.go`
- **Verification:** The golden test covers repeated identities, tombstones, present-empty values, ordered lists, and every live category.
- **Commit:** `1e16539`

**2. [Rule 1 - Bug] Corrected real-zsh option and assertion fixtures**

- **Found during:** Task 2 GREEN
- **Issue:** The initial RED fixture used zsh's semantic `NO_` option prefix as an ordinary disabled option name and left a successful negative predicate as the script's final nonzero status.
- **Fix:** Used `AUTO_CD` for the disabled-state assertion, compared multiline scalar bytes through ANSI-C quoting, and terminated the behavior script with an explicit success command.
- **Files modified:** `core/shell/zsh/emit_test.go`, `core/shell/zsh/regen_test.go`
- **Verification:** The exact plan regex suite and full real-zsh package suite pass.
- **Commit:** `7d4f705`

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug). **Impact:** Both corrections preserve the planned architecture and strengthen executable behavior evidence without expanding production scope.

## TDD Gate Compliance

- Both tasks have committed RED tests before their GREEN implementation commits.
- Each RED suite failed on the intended missing projection/patch or worktree-rendering behavior.
- Both GREEN implementations pass their exact plan verification commands, full package suites, vet, and normal lint hooks.

## Verification

- Exact Task 1 plan regex suite — passed.
- Exact Task 2 plan regex suite — passed.
- Real-zsh PATH/FPATH occurrence and tied scalar/array reverse test — passed.
- `GOTOOLCHAIN=local go test ./core/activate ./core/shell/zsh -count=1` — passed.
- `GOTOOLCHAIN=local go vet ./core/activate ./core/shell/zsh` — passed.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed across the repository.
- `GOTOOLCHAIN=local go vet ./...` — passed across the repository.
- `golangci-lint run ./...` — passed with 0 issues.
- `git diff --check` — passed.
- Structural ownership, token-free source, hostile-name, all-or-nothing, SecretRef canary, and stub scans passed.
- Every task commit passed the normal pre-commit hook; no hook bypass was used.

## Known Stubs

None. Empty strings and empty lists in the owned tests are intentional present-empty semantic fixtures; no placeholder implementation remains.

## Threat Review

- T-07-15 is mitigated by complete pre-emission validation, conservative name checks, canonical quoting, zero-output error tests, `zsh -n`, real-zsh behavior tests, and AST call ownership.
- T-07-16 is mitigated by omitting pinned SecretRef identities from projections and rejecting them during regeneration; captured secret canaries do not cross generated source.
- T-07-17 and T-07-18 are mitigated by authoritative tombstones, model-owned semantic equality, paired forward/replacement-reverse planning, and apply/reverse tests.
- T-07-18A is mitigated by exact endpoint list transitions and real PATH/FPATH duplicate/empty/unmanaged-base tied-state round trips.
- No unplanned network, authentication, filesystem, or schema trust boundary was introduced.

## Issues Encountered

None unresolved. As in prior Phase 7 plans, the current milestone `REQUIREMENTS.md` has no `WORK-02`, `WORK-03`, or `SYNC-01` rows; completion is recorded in this SUMMARY's required frontmatter and coverage matrix without inventing requirement records.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for store DTO, worktree workflow, live-provider, and runtime-hook plans to persist, emit, apply, verify, and acknowledge exact committed live state with replacement reverse ownership.

## Self-Check: PASSED

- All eight declared implementation/test files and this SUMMARY exist.
- All four TDD task commits exist in RED then GREEN order.
- Full tests, vet, lint, security scans, stub scans, and normal hooks pass.
