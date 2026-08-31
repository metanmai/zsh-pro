---
phase: 04-manifest-builder-emit
plan: 11
subsystem: store-security
tags: [git, keychain, secrets, transaction, rollback]
requires:
  - phase: 04-07
    provides: exact RuntimeValue semantics for literal secrets
provides:
  - side-effect-free secret preparation with ordered pending mutations
  - backend rollback and CAS-protected branch updates
affects: [04-12, loader, secret-dereference]
tech-stack:
  added: []
  patterns: [prepare-before-effects, snapshot-then-rollback, instance-local-ref-seams]
key-files:
  created: []
  modified: [core/store/secret.go, core/store/store.go, core/store/git.go, core/store/errors.go, core/store/keychain.go, core/store/secret_test.go, core/store/keychain_test.go]
key-decisions:
  - "Prepare redaction and pending writes before observing or mutating the secret backend; restore backend state before returning any late Commit error."
requirements-completed: [SW-01]
coverage:
  - id: D1
    description: Pure redaction preparation coalesces writes while preserving every withheld report entry.
    requirement: SW-01
    verification:
      - kind: unit
        ref: core/store/secret_test.go#TestPrepareSecretsIsPureAndCoalescesLastWrite
        status: pass
    human_judgment: false
  - id: D2
    description: Backend write and ambiguous ref outcomes restore prior backend and branch state with typed errors.
    requirement: SW-01
    verification:
      - kind: integration
        ref: core/store/secret_test.go#TestCommitRollsBackSecretWritesBeforeMovingRef
        status: pass
      - kind: integration
        ref: core/store/secret_test.go#TestCommitCompensatesAmbiguousRefFailure
        status: pass
    human_judgment: false
duration: 16min
completed: 2026-07-19
status: complete
---

# Phase 04 Plan 11: Atomic Secret Commit Summary

**Secret redaction is prepared without side effects, then backend writes and a CAS-protected branch update complete as one rollback-safe transaction.**

## Performance

- **Duration:** 16 min
- **Started:** 2026-07-19T07:05:00Z
- **Completed:** 2026-07-19T07:21:00Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments

- Split redaction from backend mutation, retaining exact semantic values in deterministic pending writes while keeping reports and committed profiles redacted.
- Distinguished a missing vault entry from an unavailable backend, including present-empty values, and retained fail-closed handling for ambiguous OS command framing.
- Made Commit snapshot all affected keys, roll back in reverse order, update branches with CAS as the final effect, and compensate a visible post-error ref update without overwriting a concurrent ref.
- Added transaction regressions for duplicate writes, backend rollback, and ref errors before and after a visible update.

## Task Commits

1. **Task 1: Separate secret preparation from backend mutation and model prior absence** - `283dccb` (feat)
2. **Task 2: Order commit effects and roll back backend state on every late error** - `f56623f` (fix)
3. **Task 3: Prove success and transaction break cases end to end** - `9ca922e`, `b874699` (test)

## Files Created/Modified

- `core/store/secret.go` - Builds a redacted profile, report, and coalesced mutations without backend access.
- `core/store/store.go` - Snapshots, rolls back, and CAS-updates refs through instance-local test seams.
- `core/store/git.go` - Provides CAS update and guarded deletion primitives.
- `core/store/errors.go` - Defines typed missing-key, rollback, and ref-conflict errors.
- `core/store/keychain.go` - Separates absent vault keys from unavailable backends and fails closed for ambiguous OS retrieval framing.
- `core/store/secret_test.go` - Covers preparation purity, backend rollback, and ambiguous ref compensation.
- `core/store/keychain_test.go` - Pins the typed missing-vault-key result.

## Decisions Made

- Preparation contains no backend calls; only Commit snapshots and mutates the backend after the complete git object exists.
- A ref error is treated as ambiguous until the ref is re-read; only this transaction's commit is CAS-compensated, while third-party movement is preserved.

## Deviations from Plan

None - plan requirements were implemented with a per-instance Store ref seam for adversarial ref outcomes.

## Checks

- `GOTOOLCHAIN=auto go test ./core/store -run 'PrepareSecrets|Keychain.*(Missing|Empty|NotFound|Newline|Exact)|ExcludeSecrets|Secret.*(Runtime|Dynamic|Unsafe)' -count=1` — passed
- `GOTOOLCHAIN=auto go test ./core/store -run 'Commit.*(Secret|Rollback|UpdateRef|Atomic|Unavailable|Concurrent|Ambiguous|Mutate)' -count=1` — passed
- `GOTOOLCHAIN=auto go test ./core/store -count=1` — passed
- `GOTOOLCHAIN=auto go test ./... -count=1` — passed
- `GOTOOLCHAIN=auto go build ./...` — passed
- `golangci-lint run` — passed

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

The store has an atomic secret/branch boundary and instance-local adversarial seams. Phase 04 gap plans can consume the typed retrieval and rollback contracts.

## Self-Check: PASSED

- Summary exists and all four task commits are present in git history.
- The final full test, build, and lint checks passed.
