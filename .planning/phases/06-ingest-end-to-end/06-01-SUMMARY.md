---
phase: 06-ingest-end-to-end
plan: 01
subsystem: ingest-transaction-foundation
tags: [go, git-plumbing, quarantine, advisory-lock, darwin]
requires:
  - phase: 03-profile-store
    provides: typed profile persistence, secret references, and bare Git store
  - phase: 05-runtime-loader-cli-bootstrap
    provides: Store-bound install initialization and rollback seam
provides:
  - main-only exact-revision ingest baseline bound to an opaque Store-issued initialization ID
  - cross-process per-root transaction locking with fail-closed ownership checks
  - hermetic candidate Git index and object quarantine with replayable cleanup evidence
affects: [06-02, 06-03, 06-04, 06-05, PROF-03]
tech-stack:
  added: []
  patterns: [pre-IO token reservation, descriptor-relative cleanup, hermetic Git environment, immutable terminal evidence]
key-files:
  created: [core/model/ingest_transaction.go, core/model/withheld.go, core/store/install_transaction_darwin_test.go, .planning/phases/06-ingest-end-to-end/06-EXECUTION-BASE]
  modified: [core/store/git.go, core/store/store.go, core/store/install_transaction.go, core/store/store_root_private_unix.go, core/store/store_root_private_other.go, core/store/store_test.go]
key-decisions:
  - "Public ingest authority is fixed to main and bound to the exact Store/root/initialization record before any I/O."
  - "Candidate writes use a private index and object directory with inherited Git controls removed; final Store objects remain untouched until publication."
  - "Cleanup authority is one real per-root OS lock plus descriptor-relative identity checks; uncertainty retains recovery evidence instead of deleting paths."
patterns-established:
  - "Reserve the transaction token before observation, then terminalize that same record on every exit."
  - "Authenticate, mutate, and sync private transaction state while holding the shared per-root advisory lock."
requirements-completed: [PROF-03]
coverage:
  - id: D1
    description: "Store-issued initialization authority and exact optional-main baseline evidence are reserved before I/O and fail closed across wrong, forged, or cross-Store identities."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/store/store_test.go#TestInstallInitializationIDBindsStoreAndRoot and five exact Task 1 selectors"
        status: pass
    human_judgment: false
  - id: D2
    description: "Cooperating processes serialize complete transaction-root mutation intervals with a real per-root OS lock, while unsafe or discarded lock authority causes zero mutation."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/store/store_test.go#TestStoreTransactionRootLockProcessMatrix and Task 2 lock selectors"
        status: pass
    human_judgment: false
  - id: D3
    description: "Candidate Git objects stay in a hermetic private quarantine and Abort returns immutable removed, retained, or uncertain evidence after descriptor-relative cleanup."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "28 exact Task 3 core/store selectors"
        status: pass
      - kind: other
        ref: "Darwin amd64 and arm64 core/store test-binary cross-builds"
        status: pass
    human_judgment: true
    rationale: "Native Darwin execution of the production cleanup primitive is the explicit Phase 06-05 exact-SHA gate and has not yet run."
metrics:
  duration: 9h45m elapsed including interrupted handoff
  completed: 2026-08-03
status: complete
---

# Phase 06 Plan 01: Ingest Transaction Foundation Summary

**Main-only ingest now reserves exact Store authority before I/O, isolates candidate Git objects, and cleans private state only under authenticated cross-process locking.**

## Performance

- **Duration:** 9h45m elapsed, including an interrupted executor handoff
- **Started:** 2026-08-02T23:43:31Z
- **Completed:** 2026-08-03T09:28:09Z
- **Tasks:** 3/3
- **Files modified:** 10

## Accomplishments

- Added value-only ingest baseline, lifecycle, cleanup, and withheld-secret evidence bound to opaque Store-issued initialization and transaction identities.
- Added a persistent private transaction namespace with real same-root cross-process exclusion and fail-closed descriptor-relative cleanup.
- Added a hermetic bare-Git runner and unique candidate index/object quarantine, with exact absent/present/probe-failure semantics and immutable Abort replay.

## Task Commits

1. **Task 1: Capture the execution base and define truthful transaction evidence** — `6472b65` (RED test), `da7a2a2` (implementation)
2. **Task 2: Establish a real cross-process Store transaction-root lock** — `f761ec8` (RED test), `f544e6e` (implementation)
3. **Task 3: Bind durable baseline reads to an isolated candidate quarantine** — `36918fc` (RED test), `e040852` (implementation)

## Files Created/Modified

- `.planning/phases/06-ingest-end-to-end/06-EXECUTION-BASE` — pins the reviewed pre-execution commit `557c818845f4239d5d50ecab02e37731bc117f10`.
- `core/model/ingest_transaction.go` — defines neutral opaque authority, baseline, lifecycle, and terminal evidence.
- `core/model/withheld.go` — defines store-neutral withheld-secret metadata.
- `core/store/git.go` — owns the allowlisted hermetic bare-Git environment and private candidate object plumbing.
- `core/store/store.go` — exposes the main-only Begin/Abort registry surface.
- `core/store/install_transaction.go` — binds initializer authority, token lifecycle, quarantine ownership, and cleanup.
- `core/store/store_root_private_unix.go` — implements the Linux/Darwin authenticated transaction namespace and advisory lock.
- `core/store/store_root_private_other.go` — fails closed where the transaction lock is unsupported.
- `core/store/store_test.go` — covers authority, cross-process locking, hostile Git input, baseline cases, cleanup substitution, and replay.
- `core/store/install_transaction_darwin_test.go` — defines native Darwin production-helper cleanup gates for Phase 06-05.

## Decisions Made

- Fixed the public ingest baseline to `refs/heads/main`; generic branch compatibility remains private to the later Store adapter.
- Removed all inherited `GIT_*` controls before setting the canonical bare repository, private index, private object directory, and controlled alternate.
- Treated a held advisory-lock descriptor as the cooperating-process authority and retained recovery evidence whenever identity or durability could not be proven.

## Verification

- Immutable execution-base equality and commit existence — PASS.
- All 28 exact Task 3 selectors — PASS.
- `GOTOOLCHAIN=local go test ./core/model ./core/store -count=1` — PASS.
- `GOTOOLCHAIN=local go vet ./core/model ./core/store` — PASS.
- `golangci-lint run ./core/model/... ./core/store/...` — PASS, `0 issues`.
- Darwin amd64 and arm64 `core/store` test-binary cross-builds — PASS.
- Pre-commit repository lint hook — PASS, `0 issues`.

## Deviations from Plan

### Auto-fixed Issues

**1. Hardened cleanup identity checks against inode reuse observed during repeated full-store validation.**

- **Found during:** Task 3 full-suite stabilization.
- **Issue:** A replaced entry could theoretically reuse an inode and make a weak identity comparison look unchanged.
- **Fix:** Strengthened retained-handle identity checks before descriptor-relative cleanup.
- **Files modified:** `core/store/install_transaction.go`, `core/store/store_test.go`
- **Verification:** The exact 28 selectors passed and the full Store suite passed three consecutive runs before final validation.
- **Committed in:** `e040852`

---

**Total deviations:** 1 auto-fixed correctness hardening.
**Impact on plan:** No scope expansion; the change strengthens the planned replacement-retention guarantee.

## Issues Encountered

- The first executor was interrupted after GREEN validation but before the Task 3 commit; the intact worktree was resumed, the fixed final gate sequence rerun, and the implementation committed without recreating or weakening tests.
- Native Darwin tests cannot execute on this Linux host. Their test binaries compile for amd64 and arm64; exact native runtime execution remains the mandatory 06-05 evidence gate.

## TDD Gate Compliance

- Task 1: `6472b65` (failing contract) -> `da7a2a2` (passing implementation)
- Task 2: `f761ec8` (failing contract) -> `f544e6e` (passing implementation)
- Task 3: `36918fc` (failing contract) -> `e040852` (passing implementation)

## User Setup Required

None for this plan. Native macOS execution is explicitly owned by Plan 06-05.

## Next Phase Readiness

- Plan 06-02 can publish the isolated candidate through a prepared expected-ref transaction using the same reserved token and exact baseline.
- `PROF-03` remains globally open until the controller, installed-startup proof, native macOS evidence, and final Phase 6 verifier pass.

## Self-Check: PASSED

- Confirmed all ten plan artifacts exist.
- Confirmed all six RED/GREEN task commits exist in Git history.
- Confirmed there are no uncommitted Plan 06-01 code changes.

---

*Phase: 06-ingest-end-to-end*
*Completed: 2026-08-03*
