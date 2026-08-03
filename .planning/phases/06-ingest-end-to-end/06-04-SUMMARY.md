---
phase: 06-ingest-end-to-end
plan: 04
subsystem: ingest-controller
tags: [go, cli, transactions, atomic-promotion, secret-redaction]
requires:
  - phase: 06-ingest-end-to-end
    plan: 02
    provides: Typed complete-Profile Store transaction outcomes, exact baseline evidence, and authenticated abort/finalization
  - phase: 06-ingest-end-to-end
    plan: 03
    provides: Original source evidence, independent install-candidate evidence, one guarded startup promotion, and recovery operations
provides:
  - strict ingest argument parsing with deterministic value-free human and JSON results
  - one concrete Store instance spanning initialization and every ingest transaction operation
  - complete source-ordered Profile persistence with EffectiveManaged used only as a reporting projection
  - one loader/target promotion followed by Store commit and finalize without a post-commit rewrite
  - filesystem-first compensation with recovery uncertainty disarming later destructive cleanup
affects: [06-05, PROF-03, ingest, startup-install]
tech-stack:
  added: []
  patterns: [closed safe-reason rendering, single-owner transaction composition, full-model persistence with projection-only reporting, filesystem-first compensation]
key-files:
  created: [core/cli/ingest.go, core/cli/ingest_test.go, core/dto/ingest.go]
  modified: [core/cli/cli.go, core/cmd/zsh-pro/main.go, core/cmd/zsh-pro/main_test.go]
key-decisions:
  - "Persist the complete source-ordered Profile; EffectiveManaged is an inspection-only projection and never filters Store input."
  - "Perform filesystem classification and restore-or-retain before Store abort, loader rollback, cache cleanup, or initializer rollback; uncertainty disarms every later destructive step."
  - "Finalize the existing guarded promotion after committed Store evidence and never construct or promote a post-commit target candidate."
  - "Capture one concrete Store pointer and its construction result at the composition root so initialization IDs and all transaction calls have one owner."
patterns-established:
  - "Ingest failures cross the output boundary only through a closed value-free reason enum."
  - "Target and candidate expectations retain separate evidence axes through prepare, promotion, commit, recovery, and finalize."
  - "A committed path promotes the loader and startup target exactly once, then finalizes owned artifacts without rewriting startup state."
requirements-completed: []
requirements-contributed: [PROF-03]
coverage:
  - id: D1
    description: "The ingest CLI accepts zero or one path plus repeatable --json, rejects invalid argv with exit 2, and emits deterministic value-free statement accounting."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/cli/ingest_test.go Task 1 exact five-test selector"
        status: pass
    human_judgment: false
  - id: D2
    description: "One exact Store pointer owns initialization and main Begin, Commit, Abort, and terminal cleanup while a second Store rejects the foreign ID before effects."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/cmd/zsh-pro/main_test.go#TestMainInitToBeginUsesExactCLIStore"
        status: pass
    human_judgment: false
  - id: D3
    description: "The controller commits the complete ordered Profile around one loader and startup promotion, finalizes on commit, and compensates filesystem-first on every noncommit path."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/cli/ingest_test.go Task 3 exact seventeen-test selector"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=local go test ./core/cli ./core/cmd/zsh-pro -count=1"
        status: pass
    human_judgment: false
  - id: D4
    description: "Source-literal recapture and programmatic persisted SecretRef pass-through stay distinct, with no source execution, backend access for persisted references, or secret-bearing output."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "TestRunIngestSourceLiteralMayRecapture, TestRunIngestProgrammaticSecretRefPassThroughIsSeparate, and TestRunIngestNeverExecutesSource"
        status: pass
    human_judgment: true
    rationale: "Plan 06-05 owns the final repository secret scan and linked end-to-end/native-platform evidence, so PROF-03 remains globally open."
metrics:
  duration: 27m
  completed: 2026-08-03
status: complete
---

# Phase 06 Plan 04: Ingest Controller Summary

**A strict value-free ingest CLI now commits one complete source-ordered Profile around exactly one guarded loader/startup promotion, with single-Store ownership and filesystem-first recovery.**

## Performance

- **Duration:** 27 minutes
- **Started:** 2026-08-03T21:48:00Z
- **Completed:** 2026-08-03T22:14:53Z
- **Tasks:** 3/3
- **Files modified:** 6

## Accomplishments

- Added `zsh-pro ingest [path] [--json]` with one-time path expansion, strict usage exits, a closed safe failure vocabulary, deterministic one-object JSON, and exact managed/unmanaged statement accounting.
- Bound Store initialization plus main Begin/Commit/Abort/finalization to one concrete `*store.Store`, preserving analyze availability when Store construction fails and proving cross-Store IDs fail before effects.
- Composed static Parse/Build, full ordered Profile persistence, projection-only `EffectiveManaged` accounting, one loader/target promotion, fixed-message Store commit, committed finalization, and filesystem-first compensation.
- Kept source-literal recapture distinct from persisted SecretRef pass-through, retained dynamic and post-END source content, and proved ingest never executes or sources input.

## Task Commits

1. **Task 1: Enforce strict arguments and safe managed/unmanaged output** — `df4c8d5` (RED test), `af8aa43` (implementation)
2. **Task 2: Prove one exact Store owns initialization and every ingest operation** — `387f392` (RED test), `5711896` (implementation)
3. **Task 3: Commit the complete Profile around one non-destructive startup promotion** — `8d76b75` (RED test), `24168a2` (implementation)

## Files Created/Modified

- `core/cli/ingest.go` — strict argv parsing, safe rendering, complete-Profile controller, exact promotion/commit ordering, and compensation matrix.
- `core/cli/ingest_test.go` — argument, output, accounting, Profile, secret-boundary, promotion-cardinality, conflict, and filesystem-first transaction proofs.
- `core/dto/ingest.go` — stable value-free ingest result and withheld-entry DTOs.
- `core/cli/cli.go` — dispatches the new ingest verb without changing the public `CLI.Run` signature.
- `core/cmd/zsh-pro/main.go` — constructs one Store and injects its initializer and transaction authority through the CLI composition root.
- `core/cmd/zsh-pro/main_test.go` — real Store-bound initialization identity and cross-Store rejection proof.

## Decisions Made

- The complete `model.Profile` is the only Store commit input. `EffectiveManaged` computes reporting/activation counts without selecting, merging, or physically complementing persisted entries.
- Original target and independently durable candidate evidence remain separate throughout validation; neither is resampled or substituted before commit.
- Committed Store evidence makes the already-promoted startup state authoritative. The controller only finalizes its journal/artifacts and has no post-commit rewrite path.
- Every noncommit outcome after promotion enters one compensation helper. Filesystem restore-or-retain classification runs first, and any uncertainty stops later Store, loader, cache, and initializer compensation.
- The composition root captures one Store pointer and one construction error. Store-backed verbs reuse that result, while analyze remains independent.

## Validation Results

- Task 1 exact five-test name/cardinality selector — PASS.
- Task 2 exact Store-identity test name/cardinality selector — PASS.
- Task 3 exact seventeen-test name/cardinality selector — PASS.
- `GOTOOLCHAIN=local go test ./core/cli ./core/cmd/zsh-pro -count=1` — PASS.
- `GOTOOLCHAIN=local go vet ./core/cli ./core/cmd/zsh-pro` — PASS.
- Local toolchain gate confirmed Go 1.25.x with `GOTOOLCHAIN=local` — PASS.
- Stale-architecture scans confirmed one Store commit, one target promotion, one loader promotion, one `EffectiveManaged` projection, and no physical-complement, post-commit-target, source-execution, or source-SecretRef authority path — PASS.
- `git diff --check` and every RED/GREEN commit's repository lint hook — PASS.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Finalized prepared target journals on failures before target promotion.**

- **Found during:** Task 3 implementation review.
- **Issue:** Pre-promotion failure handling used an empty promotion outcome, so an already-prepared target transaction could retain its journal instead of terminalizing owned evidence.
- **Fix:** Route prepared pre-promotion transactions through their own rollback/finalize state before later cleanup.
- **Files modified:** `core/cli/ingest.go`
- **Verification:** Task 3 transaction matrix, pre-commit compensation selector, and full CLI suite pass.
- **Commit:** `24168a2`

**2. [Rule 1 - Bug] Preserved rollback evidence when loader promotion partially succeeded.**

- **Found during:** Task 3 implementation review.
- **Issue:** The error branch discarded loader promotion state before compensation, which could make a partially promoted loader impossible to restore safely.
- **Fix:** Retain the promotion result on error and pass its evidence into filesystem-first compensation.
- **Files modified:** `core/cli/ingest.go`
- **Verification:** Task 3 outcome/compensation matrix and full CLI suite pass.
- **Commit:** `24168a2`

---

**Total deviations:** 2 auto-fixed correctness bugs.
**Impact on plan:** Both fixes enforce the planned transaction-safety contract; no feature scope was added.

## Issues Encountered

None beyond the two inline correctness fixes documented above. No authentication, package-install, external-service, or human-action gate was encountered.

## TDD Gate Compliance

- Task 1: `df4c8d5` (failing CLI/output contracts) -> `af8aa43` (passing implementation)
- Task 2: `387f392` (failing composition Store identity proof) -> `5711896` (passing implementation)
- Task 3: `8d76b75` (failing ingest transaction matrix) -> `24168a2` (passing controller)

## Known Stubs

None. The created and modified production files contain no TODO/FIXME/placeholder path, mock data source, or goal-blocking empty implementation.

## User Setup Required

None for this plan.

## Next Phase Readiness

- Plan 06-05 can exercise the real fixture through this controller, assert the three linked source/Profile/install properties, collect native macOS evidence, and run the final secret scan.
- `PROF-03` remains globally unchecked until Plan 06-05 and final verification complete.

## Self-Check: PASSED

- Confirmed all six production/test files and this summary exist.
- Confirmed all six RED/GREEN task commits exist in Git history.
- Confirmed the summary records `status: complete`, preserves `PROF-03` as contributed rather than globally completed, and contains no unresolved stub.

---
*Phase: 06-ingest-end-to-end*
*Completed: 2026-08-03*
