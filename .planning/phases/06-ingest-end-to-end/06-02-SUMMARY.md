---
phase: 06-ingest-end-to-end
plan: 02
subsystem: ingest-publication
tags: [go, git-plumbing, update-ref, transactions, secrets]
requires:
  - phase: 06-ingest-end-to-end
    plan: 01
    provides: Store-bound ingest authority, exact baseline evidence, private object quarantine, and authenticated cleanup
provides:
  - prepared expected-ref publication with durable backend and object ordering
  - truthful independent ref-publication and quarantine-cleanup evidence
  - validated persisted SecretRef recommit without backend data access
  - branch-preserving legacy Commit adapter over the typed ingest state machine
affects: [06-03, 06-04, 06-05, PROF-03]
tech-stack:
  added: []
  patterns: [opaque validated refs, staged update-ref transactions, content-addressed object publication, outcome adapters]
key-files:
  created: []
  modified: [core/model/ingest_transaction.go, core/store/git.go, core/store/store.go, core/store/store_test.go, core/store/secret.go, core/store/secret_test.go]
key-decisions:
  - "Only update-ref prepare acknowledgement establishes the expected-ref lock; backend and final-object effects occur afterward, and commit is a separate final write."
  - "Publication truth and cleanup evidence remain independent, including when commit-response observation or authenticated cleanup requires recovery."
  - "Persisted SecretRefs are structurally validated before one backend-kind comparison and never trigger Retrieve, Store, or Delete."
  - "Legacy branch commits use a private validated-ref constructor and the shared ingest state machine without widening public main-only BeginIngest."
patterns-established:
  - "Encode ref mutations only from an opaque validated head ref and validated object IDs; every read, guard, and mutation is no-deref."
  - "Return value-free metadata only when publication is known committed; expose recovery and committed truth as separate evidence."
requirements-completed: []
requirements-contributed: [PROF-03]
coverage:
  - id: D1
    description: "Prepared no-deref ref locking serializes competing writers before backend or final-object effects, and commit follows durable publication."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/store/store_test.go task-1 exact selector set"
        status: pass
    human_judgment: false
  - id: D2
    description: "Candidate, expected, third, and unreadable ref observations preserve publication truth independently from cleanup evidence and guarded compensation."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/store/store_test.go commit-observation, recovery-guard, and cleanup-axis tests"
        status: pass
    human_judgment: false
  - id: D3
    description: "Literal secrets are redacted before object creation, dynamic values remain verbatim, and valid persisted references recommit unchanged without backend data access."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/store/secret_test.go task-2 exact selector set"
        status: pass
    human_judgment: true
    rationale: "The final repository and native macOS secret scans remain owned by Plan 06-05, so PROF-03 stays globally open."
metrics:
  duration: 38m
  completed: 2026-08-03
status: complete
---

# Phase 06 Plan 02: Prepared Ingest Publication Summary

**Prepared no-deref Git transactions now publish redacted ingest candidates only after winning the expected-ref lock, with durable effects, truthful recovery evidence, and branch-compatible legacy commits.**

## Performance

- **Duration:** 38 minutes
- **Started:** 2026-08-03T09:33:36Z
- **Completed:** 2026-08-03T10:11:26Z
- **Tasks:** 2/2
- **Files modified:** 6

## Accomplishments

- Added a long-lived `git update-ref --no-deref --stdin` session whose prepare acknowledgement locks the expected revision before backend or final-object effects and whose commit frame is withheld until those effects are durable.
- Added atomic Store/initialization/token claiming, guarded ambiguity recovery, content-addressed loose-object publication, and cleanup evidence that cannot rewrite publication truth.
- Added fail-closed persisted `SecretRef` pass-through with exact structural validation, one backend-kind comparison, no backend data access, and no second withheld report.
- Reimplemented source-compatible `Store.Commit` as a branch-preserving adapter over the same typed ingest transaction and stable committed/conflict/not-committed/recovery outcome matrix.

## Task Commits

1. **Task 1: Publish through a prepared expected-ref transaction** — `d799a58` (RED test), `641460b` (implementation)
2. **Task 2: Preserve value-free reports and validated SecretRefs** — `c5dfcbe` (RED test), `7c94e21` (implementation)

## Files Created/Modified

- `core/model/ingest_transaction.go` — adds distinct prepare, backend, object-publication, and ref-commit failure codes.
- `core/store/git.go` — owns opaque validated head refs, direct-ref validation, the single typed mutation encoder, and staged update-ref sessions.
- `core/store/store.go` — implements claimed ingest publication, durable object linking, guarded recovery, independent cleanup evidence, and the legacy adapter.
- `core/store/store_test.go` — proves wire bytes, lock/effect order, no-deref behavior, authority claims, failure axes, cleanup, observation recovery, and concurrent-writer exclusion.
- `core/store/secret.go` — validates and passes through persisted secret references without backend data operations.
- `core/store/secret_test.go` — covers the secret call/effect matrix, redaction order, dynamic values, branch preservation, and value-free adapter outcomes.

## Decisions Made

- Treat update-ref start acknowledgement as process liveness only; prepare acknowledgement alone authorizes durable effects.
- Publish loose objects using exclusive content-addressed links, verify any preexisting object by its Git object ID, and sync every changed fanout plus the object root before ref commit.
- Compensate backend state after an uncertain commit response only while an exact replacement expected-ref guard is held; third or unreadable state retains evidence and requires recovery.
- Keep public ingest fixed to `main`; the legacy adapter alone constructs other validated head refs under Store-owned internal authority.
- Validate a persisted secret reference completely before calling `Kind` once and never call backend data methods for that path.

## Validation Results

- All 24 exact Task 1 selector names had cardinality one and passed — PASS.
- All 8 exact Task 2 selector names had cardinality one and passed — PASS.
- `GOTOOLCHAIN=local go test ./core/model ./core/store -count=1` — PASS.
- `GOTOOLCHAIN=local go vet ./core/model ./core/store` — PASS.
- `GOTOOLCHAIN=local go test ./... -count=1` — PASS.
- `GOTOOLCHAIN=local go vet ./...` — PASS.
- `golangci-lint run ./core/model/... ./core/store/...` — PASS, `0 issues`.
- Both GREEN commits passed the repository pre-commit lint hook — PASS.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Accepted a directory at an absent exact loose-ref leaf.**

- **Found during:** Task 1 full Store-suite validation.
- **Issue:** When `refs/heads/main` was absent but a nested ref such as `refs/heads/main/topic` existed, Git represented the exact loose-ref leaf as a directory and direct-ref validation incorrectly rejected the safe absent-ref state.
- **Fix:** Classified a directory at the exact leaf as exact-ref absence while preserving no-follow checks for every component and rejecting symbolic or filesystem-symlink redirection.
- **Files modified:** `core/store/git.go`, `core/store/store_test.go`
- **Commit:** `641460b`

**2. [Rule 1 - Bug] Made branch creation compare-and-swap instead of overwrite-capable.**

- **Found during:** Task 2 lint and shared-ref-path review after removing the legacy one-shot commit implementation.
- **Issue:** `Store.Create` used an unconditional ref update, so a concurrently created branch could be overwritten; the refactor also exposed the typed CAS helper as otherwise unused.
- **Fix:** Switched creation to an expected-zero `update-ref` compare-and-swap using the canonical object-ID width.
- **Files modified:** `core/store/store.go`
- **Commit:** `7c94e21`

---

**Total deviations:** 2 auto-fixed correctness issues.
**Impact on plan:** No scope expansion; both fixes strengthen the planned direct-ref and lost-update guarantees.

## Issues Encountered

- The first Task 1 full-suite run exposed the exact-ref directory case described above; after the focused fix, both task verifiers and the repository-wide gates passed.
- No authentication, dependency, environment, or human-action gate was encountered.

## TDD Gate Compliance

- Task 1: `d799a58` (failing typed publication contract) -> `641460b` (passing implementation)
- Task 2: `c5dfcbe` (failing persisted-ref/adapter contract) -> `7c94e21` (passing implementation)

## User Setup Required

None for this plan.

## Next Phase Readiness

- Plan 06-03 can consume the finalized `CommitIngest` outcome and transaction contract.
- `PROF-03` remains globally unchecked until the controller, installed-startup proof, native macOS evidence, and final Phase 6 verification complete.

## Self-Check: PASSED

- Confirmed all six implementation/test files and this summary exist.
- Confirmed all four RED/GREEN task commits exist in Git history.
- Confirmed there are no uncommitted Plan 06-02 code changes.

---

*Phase: 06-ingest-end-to-end*
*Completed: 2026-08-03*
