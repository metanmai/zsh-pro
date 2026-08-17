---
phase: 07-git-like-shared-working-environment
plan: 03
subsystem: worktree-state
tags: [worktree, atomic-storage, capability-auth, revision-arbitration, conflict-resolution]
requires:
  - phase: 07-01-lossless-worktree-contract
    provides: validated live identities, snapshots, committed projections, and value-free public metadata
  - phase: 07-02-admission-and-live-secret-boundary
    provides: exact materialized ownership, attachment exclusions, SecretRef pinning, and absent-only admission
provides:
  - Verifier-at-rest combined worktree state with bounded events, receipts, shell history, conflicts, and pending operations
  - Descriptor-relative atomic StateStore with cancellation-aware locking, durable replacement, and recovery evidence
  - Authenticated replay-safe materialize, attach, publish, prepare, resolve, acknowledge, status, and diff lifecycle
affects: [07-04-activation-patch, 07-05-store-dto, 07-06-cli, 07-07-composition, 07-08-live-provider, 07-09-runtime-hooks]
tech-stack:
  added: []
  patterns: [verifier-at-rest capability authentication, one combined atomic generation, operation receipts, prepare-apply-verify-ack, first-lock-wins arbitration]
key-files:
  created:
    - core/worktree/state.go
    - core/worktree/state_test.go
    - core/worktree/atomic_unix.go
    - core/worktree/atomic_unix_test.go
    - core/worktree/service.go
    - core/worktree/service_test.go
  modified:
    - core/model/worktree.go
    - core/model/worktree_test.go
key-decisions:
  - "Persist only domain-separated capability verifiers; compare credentials in constant time inside the StateStore transaction before receipt lookup or mutation."
  - "Keep all authoritative shared, shell, pending, receipt, conflict, and event data in one canonical worktree.json generation replaced descriptor-relatively under one lock."
  - "Use first successful lock acquisition as publication order; overlapping losers retain their unpublished delta and durable conflict evidence until exact acknowledgement."
  - "Duplicate and re-authenticate descriptor constructor inputs so StateStore owns its lifecycle without taking ownership of caller descriptors or falling back to paths."
requirements-completed: [WORK-01, WORK-02, SYNC-01, SYNC-02]
coverage:
  - id: D1
    description: "Opaque credentials, verifier-only persistence, strict combined state, bounded replay/history collections, and distinct attach/conflict/acknowledgement transitions"
    requirement: WORK-01
    verification:
      - kind: unit
        ref: "core/model/worktree_test.go#TestShellCapabilityVerifierAndCredentialSerializationContract"
        status: pass
      - kind: unit
        ref: "core/worktree/state_test.go#TestStateRoundTripIsStableBoundedAndDefensivelyCopied"
        status: pass
      - kind: unit
        ref: "core/worktree/state_test.go#TestShellStateAttachResolveAndLocallyAcknowledgedLoserRemainDistinct"
        status: pass
    human_judgment: false
  - id: D2
    description: "Descriptor-relative StateStore serializes concurrent writers and durably exposes either the old or complete new canonical generation across faults and reopen"
    requirement: SYNC-01
    verification:
      - kind: unit
        ref: "core/worktree/atomic_unix_test.go#TestStateStorePathAndAuthenticatedDescriptorUseSameCanonicalGeneration"
        status: pass
      - kind: integration
        ref: "core/worktree/atomic_unix_test.go#TestStateStoreTwoProcessCounterRaceSerializes"
        status: pass
      - kind: unit
        ref: "core/worktree/atomic_unix_test.go#TestAtomicFaultsLeaveOldOrCompleteNewGeneration"
        status: pass
      - kind: unit
        ref: "core/worktree/atomic_unix_test.go#TestStateStoreRemainsBoundToAuthenticatedDescriptorAfterPathReplacement"
        status: pass
    human_judgment: false
  - id: D3
    description: "Authenticated service lifecycle supports safe admission, disjoint stale composition, first-lock overlap conflicts, exact acknowledgement, and replay-safe crash recovery"
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: "core/worktree/service_test.go#TestConcurrentOverlapFirstPublicationWinsAndLoserRemainsUnpublished"
        status: pass
      - kind: unit
        ref: "core/worktree/service_test.go#TestPrivateCredentialSpoofCrossShellReplayAndReceiptGuessAreValueFree"
        status: pass
      - kind: integration
        ref: "core/worktree/service_test.go#TestCrashReplayPublishResolveAndAcknowledgeIsIdempotent"
        status: pass
      - kind: unit
        ref: "core/worktree/service_test.go#TestPreparePullAndAcknowledgeRequireExactFreshTarget"
        status: pass
    human_judgment: false
metrics:
  duration: 46min
  completed: 2026-08-17
status: complete
---

# Phase 07 Plan 03: Durable Shared Worktree State Summary

**A verifier-authenticated, descriptor-relative state service now atomically arbitrates concurrent shell publications, preserves durable conflict and replay evidence, and clears local state only after exact acknowledgement.**

## Performance

- **Duration:** 46 min
- **Started:** 2026-08-17T11:21:29Z
- **Completed:** 2026-08-17T12:07:36Z
- **Tasks:** 3/3
- **Files created:** 6
- **Files modified:** 2

## Accomplishments

- Added opaque shell capabilities whose raw credential bytes never serialize; combined state stores only domain-separated verifiers and authenticates in constant time under the same transaction that reads receipts or mutates state.
- Added strict, stable combined state encoding for shared revision history, per-shell baselines and unpublished deltas, pending apply targets, operation receipts, conflicts, and bounded value-free audit events.
- Implemented an authenticated-root StateStore that owns a duplicate descriptor, rejects symlink/type/mode/UID substitution, takes a cancellation-aware advisory lock, and commits canonical generations through full-write, file sync, rename, and directory sync.
- Implemented exact materialization, attachment, safe live admission, stale disjoint composition, first-lock-wins overlap arbitration, explicit shared resolution, prepare/apply/verify/acknowledge, and durable replay receipts.
- Proved old-or-complete-new visibility, response-loss reopen behavior, cross-process serialization, local loser preservation, exact acknowledgement semantics, and public value-free status/diff output.

## Task Commits

1. **Task 1 RED: durable state and credential contract** — `f61898b` (test)
2. **Task 1 GREEN: strict combined state and verifier credentials** — `8e21892` (feat)
3. **Task 2 RED: atomic descriptor StateStore contract** — `51441a4` (test)
4. **Task 2 GREEN: atomic descriptor-relative persistence** — `f853ab2` (feat)
5. **Task 3 RED: worktree arbitration lifecycle** — `cc362c9` (test)
6. **Task 3 GREEN: authenticated arbitration service** — `0042abe` (feat)
7. **Task 3 hardening: canonical forensic event filename** — `12d5caa` (fix)
8. **Task 3 hardening: locked transition boundaries** — `f3dcd7f` (fix)
9. **Task 3 hardening: Unix portability** — `8760104` (fix)

## Files Created/Modified

- `core/model/worktree.go` — Defines opaque capabilities, verifier derivation, credential-bearing requests, pending/conflict metadata, and exact acknowledgement fields.
- `core/model/worktree_test.go` — Pins credential serialization, value presence, normalization, bounds, overflow, and public metadata contracts.
- `core/worktree/state.go` — Defines strict combined state, canonical encoding, cloning, validation, legal transitions, compaction, and garbage collection.
- `core/worktree/state_test.go` — Covers stable round trips, 128/129 boundaries, receipt unions, verifier canaries, and distinct shell lifecycle states.
- `core/worktree/atomic_unix.go` — Implements authenticated descriptor-relative lifecycle, fixed-name locking and replacement, durability faults, and forensic recovery evidence.
- `core/worktree/atomic_unix_test.go` — Covers descriptor ownership, substitution and symlink rejection, atomicity, contention, process races, reopen, and reader visibility.
- `core/worktree/service.go` — Implements authenticated materialize, attach, publish, prepare, resolve, acknowledge, status, and diff transitions.
- `core/worktree/service_test.go` — Covers exact replay, admission, authorization privacy, stale composition, concurrent overlap, history gaps, resolution, crash replay, and acknowledgements.

## Decisions Made

- Raw credentials are ephemeral request material. Only SHA-256 domain-separated verifiers persist, and every private operation performs constant-time authorization inside the locked transaction before observing receipts.
- `worktree.json` is the single canonical authority. The append-only `events.jsonl` file is bounded, value-free forensic evidence whose failure cannot reverse a successful canonical commit.
- Pending work records an exact target revision, fingerprint, and acknowledgement token. Only an exact matching acknowledgement clears pending state, conflicts, or unpublished deltas; later shared heads remain intact.
- First successful lock acquisition defines concurrent publication order. Disjoint stale changes compose, while overlapping losers retain both their local unpublished delta and durable conflict evidence.
- Descriptor-based stores duplicate and immediately re-authenticate the supplied directory descriptor, keep all lifecycle I/O descriptor-relative, and remain bound to that directory even if its original path is replaced.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Made Task 2 RED cleanup paths pass normal lint hooks**

- **Found during:** Task 2 RED commit
- **Issue:** Intentionally failing StateStore tests had unchecked descriptor cleanup calls that the normal pre-commit lint hook rejected before the RED contract could be recorded.
- **Fix:** Added test cleanup helpers that report close failures while preserving the intended missing-StateStore behavior.
- **Files modified:** `core/worktree/atomic_unix_test.go`
- **Verification:** The targeted RED suite still failed on the planned missing implementation and the normal hook then passed.
- **Commit:** `51441a4`

**2. [Rule 1 - Bug] Corrected the fixed forensic event filename**

- **Found during:** Post-GREEN threat and contract audit
- **Issue:** The implementation initially used `worktree.events`, while the plan requires the fixed derived filename `events.jsonl`.
- **Fix:** Renamed the fixed descriptor-relative forensic target and updated its assertions without changing canonical authority.
- **Files modified:** `core/worktree/atomic_unix.go`, `core/worktree/atomic_unix_test.go`
- **Verification:** Atomic, recovery, scoped, and full repository suites pass with the exact filename.
- **Commit:** `12d5caa`

**3. [Rule 2 - Missing Critical] Hardened transaction-bound authorization and key binding**

- **Found during:** Final security self-audit
- **Issue:** Mutable request inputs, a missing-shell timing shortcut, and insufficient pending/conflict identity binding could weaken the required authorization and acknowledgement invariants.
- **Fix:** Snapshotted request inputs, re-authenticated fixed lock bindings around mutation, performed a dummy constant-time verifier comparison for missing shells, revalidated deltas under lock, and required exact pending/conflict identity matches.
- **Files modified:** `core/worktree/atomic_unix.go`, `core/worktree/service.go`, `core/worktree/service_test.go`
- **Verification:** Authentication, replay, concurrent publication, resolution, acknowledgement, race, and full repository suites pass.
- **Commit:** `f3dcd7f`

**4. [Rule 3 - Blocking] Preserved descriptor-relative semantics on Darwin**

- **Found during:** Final portability cross-compilation
- **Issue:** Go's `syscall` package does not export the Linux-named descriptor-relative helpers used by the first GREEN implementation when compiling for Darwin.
- **Fix:** Added small architecture-aware raw syscall wrappers inside the owned Unix StateStore file, retaining fixed-name descriptor-relative operations without a path fallback or new dependency.
- **Files modified:** `core/worktree/atomic_unix.go`
- **Verification:** Darwin amd64, Darwin arm64, and Linux arm64 cross-compilation pass; Linux targeted, race, vet, lint, and full repository gates remain green.
- **Commit:** `8760104`

**Total deviations:** 4 auto-fixed (1 bug, 1 missing critical, 2 blocking). **Impact:** The corrections preserve the planned architecture while strengthening filename compliance, transaction security, normal-hook TDD history, and supported Unix portability.

## TDD Gate Compliance

- All three tasks have committed RED tests preceding their GREEN implementations.
- Each RED run failed on the intended missing credential/state, atomic store, or service lifecycle behavior rather than unrelated syntax or import failures.
- All GREEN implementations pass their exact plan verification suites; the three follow-up fix commits preserve every TDD contract.

## Verification

- Exact Task 1 state/model regex suite — passed.
- Exact Task 2 atomic persistence regex suite — passed.
- Exact Task 3 arbitration and lifecycle regex suite — passed.
- `GOTOOLCHAIN=local go test ./core/model ./core/worktree -count=1` — passed.
- `GOTOOLCHAIN=local go test -race ./core/worktree -run 'Test(Concurrent|Lock|Attach|Publish|ResolveShared|Acknowledge|Crash|Replay)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed across the repository after final hardening.
- `GOTOOLCHAIN=local go vet ./...` — passed across the repository.
- `golangci-lint run ./...` — passed with 0 issues.
- Darwin amd64, Darwin arm64, and Linux arm64 `go test -c` cross-compilation for `core/worktree` — passed.
- `git diff --check` — passed.
- Stub and secret scans found no placeholders and no raw capability persisted in canonical state.
- Every RED, GREEN, and hardening commit passed the normal pre-commit hooks; no hook bypass was used.

## Known Stubs

None. Empty collections and nil returns in the owned files are validated absence, optional-state, or error semantics rather than placeholders.

## Issues Encountered

None unresolved. As in Plans 07-01 and 07-02, the current milestone `REQUIREMENTS.md` has no `WORK-01`, `WORK-02`, `SYNC-01`, or `SYNC-02` rows; completion is recorded in this SUMMARY's required frontmatter and coverage matrix without inventing requirement records.

## Next Phase Readiness

Ready for downstream activation, CLI, composition, provider, and runtime-hook plans to consume one durable revision authority and the exact prepare/apply/verify/acknowledge lifecycle.

## Self-Check: PASSED

- All eight declared implementation/test files and this SUMMARY exist.
- All nine RED/GREEN/hardening commits are present in repository history.
- Coverage metadata validates and the final full test, race, vet, lint, portability, and diff gates pass after hardening.
