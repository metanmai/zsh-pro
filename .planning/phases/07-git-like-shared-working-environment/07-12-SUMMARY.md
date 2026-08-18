---
phase: 07-git-like-shared-working-environment
plan: 12
subsystem: persistence
tags: [go, unix, openat, atomicity, recovery, concurrency]

requires:
  - phase: 07-git-like-shared-working-environment
    plan: 10
    provides: authenticated descriptor-bound StateStore, atomic canonical replacement, forensic append recovery signal, and production operation-scoped runtime ownership
provides:
  - Descriptor-relative durable recovered-persistence evidence with constant value-free protocol content
  - Canonical projection-before-clear ordering across close, reopen, crash, retry, and concurrent fresh Stores
  - Security regression coverage for malformed marker objects, root-path replacement, and serialized recovery races
affects: [phase-07-verification, state-store, runtime-worktree, forensic-recovery]

tech-stack:
  added: []
  patterns:
    - Fixed-name openat evidence authenticated by owner, regular-file type, 0600 mode, exact size, and exact protocol
    - Canonical replace and directory sync before unlink and directory sync
    - Deterministic pre-lock fault barrier for multi-Store serialization proof

key-files:
  created: []
  modified:
    - core/worktree/atomic_unix.go
    - core/worktree/atomic_unix_test.go

key-decisions:
  - "Recovered forensic persistence is represented by one constant value-free marker below the retained authenticated root descriptor; process-local Store memory is never causal authority."
  - "A pending marker is projected before and after the caller mutation, then cleared only after the canonical generation containing RecoveredPersistence is durably replaced."
  - "A valid pre-existing marker is idempotently reauthenticated and resynced; malformed, replaced, or rebound objects fail closed without being followed or unlinked."

patterns-established:
  - "Recovery evidence lifecycle: authenticate under lock, read exact marker, project canonical truth, replace and sync, unlink marker, sync directory."
  - "Operation-scoped persistence proof: close Store and caller descriptor, independently authenticate a fresh descriptor, reopen, surface once, and verify a later reopen stays canonical without re-projection."

requirements-completed: [WORK-01, SYNC-02]

coverage:
  - id: D1
    description: Forensic append failure leaves durable value-free descriptor-relative evidence that survives Store close and fresh authenticated reopen
    requirement: WORK-01
    verification:
      - kind: integration
        ref: core/worktree/atomic_unix_test.go#TestRecoveredPersistenceOperationScopedLifecycle
        status: pass
      - kind: unit
        ref: core/worktree/atomic_unix_test.go#TestRecoveredPersistenceMarkerPersistsAfterAppendFailure
        status: pass
    human_judgment: false
  - id: D2
    description: Recovery evidence is projected canonically before durable clear and remains truthful through failure and repeated reopen
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: core/worktree/atomic_unix_test.go#TestRecoveredPersistenceMarkerClearsOnlyAfterProjection
        status: pass
    human_judgment: false
  - id: D3
    description: Malformed markers, path replacement, and concurrent fresh Stores cannot redirect, forge, erase, or duplicate canonical recovery evidence
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: core/worktree/atomic_unix_test.go#TestRecoveredPersistenceMarkerRejectsPathReplacementAndMalformedContent
        status: pass
      - kind: integration
        ref: core/worktree/atomic_unix_test.go#TestRecoveredPersistenceConcurrentReopen
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-08-18
status: complete
---

# Phase 7 Plan 12: Durable Recovered-Persistence Evidence Summary

**A constant descriptor-relative marker now carries recovered forensic-persistence evidence across operation-scoped Store close/reopen, projects it into canonical shell state, and clears it only after durable surfacing.**

## Performance

- **Duration:** 15 minutes
- **Started:** 2026-08-18T01:09:46Z
- **Completed:** 2026-08-18T01:25:12Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Replaced `StateStore.pendingRecoveredPersistence` with a fixed `recovered-persistence` entry addressed only through the retained authenticated root descriptor.
- Authenticated marker owner, regular-file identity, `0600` mode, exact bounded constant protocol, binding, file sync, and directory sync without following a path or symlink.
- Made recovery projection authoritative before caller mutation and again before marshal, then ordered canonical replace/sync before marker unlink/sync; a repeated forensic append fault recreates the marker.
- Proved the exact runtime lifecycle through independent descriptors and Stores, including third reopen, hostile path replacement, and two fresh Stores racing at a deterministic pre-lock barrier.

## Task Commits

1. **Task 1 RED: durable marker lifecycle and security contract** - `8dbd7bc`
2. **Task 1 GREEN: descriptor-relative marker implementation** - `fb4c1f4`
3. **Task 2 RED: operation-scoped lifecycle and concurrency contract** - `97a78e6`
4. **Task 2 GREEN: deterministic fresh-Store arbitration proof** - `00a0260`

## Files Created/Modified

- `core/worktree/atomic_unix.go` - Fixed marker protocol, authenticated descriptor-relative create/read/clear helpers, canonical projection ordering, and pre-lock fault barrier.
- `core/worktree/atomic_unix_test.go` - Append-failure durability, malformed/replacement security, projection-clear, exact close/reopen, and concurrent fresh-Store tests.

## Decisions Made

- The marker contains only `ZPWT recovered-persistence v1` plus its protocol newline; no state value, event value, source byte, capability, pathname, or attacker content is serialized.
- A callback observes pending recovery at transaction entry, but projection is repeated after the callback so it cannot accidentally erase recovery evidence before canonical persistence.
- A crash after canonical projection but before clear may repeat the truthful marker; an already projected canonical generation can clear it without inventing a second state transition.
- Fixed-name marker binding is compared immediately before unlink, and every marker operation reauthenticates both the root descriptor and current lock binding.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The first Task 1 RED version referenced not-yet-defined production constants, so the normal typechecking commit hook correctly rejected it. The test contract was changed to independent expected protocol literals, preserving a runtime RED failure without bypassing hooks.
- Task 2's lifecycle assertions were already green after Task 1; its RED failure isolated the missing deterministic pre-lock barrier needed to make the concurrent fresh-Store race non-vacuous.
- `STATE.md` entered with a stale `Plan: 2 of 15` position despite eleven existing summaries; the supported `state.advance-plan` handler was advanced to the disk-backed next position, Plan 13, after this summary was created.
- `ROADMAP.md` and `REQUIREMENTS.md` still have no Phase 7 row or `WORK-*`/`SYNC-*` definitions. Their supported handlers therefore produced no tracked change and reported `WORK-01`/`SYNC-02` not found; coverage remains explicit in this summary rather than inventing legacy traceability.
- No authentication gate, dependency installation, package lookup, or external service was required.

## Known Stubs

None.

## Verification

- Task 1 focused marker lifecycle/security command: passed.
- Task 2 focused operation-scoped lifecycle/concurrency command: passed.
- `GOTOOLCHAIN=local go test -count=1 ./core/worktree -timeout=60s`: passed.
- `GOTOOLCHAIN=local go test -count=1 ./... -timeout=120s`: passed.
- `GOTOOLCHAIN=local go test -race -count=1 ./core/worktree ./core/store ./core/cli ./core/shell/zsh -timeout=180s`: passed.
- `GOTOOLCHAIN=local go vet ./...`: passed.
- `GOTOOLCHAIN=local go build ./...`: passed.
- `make check`: passed with zero golangci-lint issues.
- Stub, value-bearing payload, pathname fallback, deletion, and threat-surface scans: passed.

## Threat Model Closure

- T-07-12-01/04/05: fixed no-follow descriptor-relative operations authenticate exact type, mode, owner, bounded protocol, and binding under the canonical lock.
- T-07-12-02: marker and diagnostics are constant/value-free; tests retain no sensitive values.
- T-07-12-03: canonical replace and directory sync precede marker removal and its directory sync.
- T-07-12-06: independent Stores serialize marker projection and clear under the existing advisory lock.
- No security-relevant surface beyond the plan's declared fixed private marker was introduced.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The recovered-persistence close/reopen gap in `07-VERIFICATION.md` is executable and green.
- Plans 07-13 through 07-15 remain for the other Phase 7 verification gaps; this plan introduces no blocker for them.

## Self-Check: PASSED

- Both modified source/test files exist and contain the declared marker symbols and lifecycle tests.
- All four Task 1/Task 2 RED/GREEN commits are present in Git history.
- No tracked file deletion or unrelated worktree mutation occurred; user-owned untracked paths remain untouched.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-18*
