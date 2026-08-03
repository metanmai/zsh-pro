---
phase: 06-ingest-end-to-end
plan: 03
subsystem: startup-transaction
tags: [go, zsh, renameat2, renameatx-np, journaling, crash-recovery, filesystem-security]
requires:
  - phase: 06-ingest-end-to-end
    plan: 01
    provides: Store initialization authority, private transaction namespace patterns, and authenticated cleanup
  - phase: 06-ingest-end-to-end
    plan: 02
    provides: Typed ingest publication outcomes and filesystem-first compensation requirements
provides:
  - byte-exact original source, eligible source, marker topology, and one canonical install candidate
  - audited Linux and Darwin exchange/no-replace adapters with typed unsupported behavior
  - authenticated same-filesystem capability probe before Store, cache, loader, or target effects
  - private one-peer promotion journal with guarded reverse, recovery classification, and durable cleanup
  - descriptor-held per-target-root advisory serialization for cooperating zsh-pro processes
affects: [06-04, 06-05, PROF-03, install, ingest]
tech-stack:
  added: []
  patterns: [per-call syscall injection, descriptor-relative mutation authority, independent target-candidate evidence, durable journal state machine]
key-files:
  created: [core/cli/atomic_rename.go, core/cli/atomic_rename_linux.go, core/cli/atomic_rename_darwin.go, core/cli/atomic_rename_other.go, core/cli/atomic_rename_supported_test.go, core/cli/atomic_rename_unsupported_test.go]
  modified: [core/cli/install.go, core/cli/install_transaction.go, core/cli/install_test.go, core/cli/atomic_rename_test.go, core/cli/store.go, core/store/store_root_private_other.go]
key-decisions:
  - "Keep expectedTarget bound to the original bounded snapshot and expectedCandidate bound to independently fsynced peer evidence; neither axis is resampled or substituted."
  - "Use one private exchange-peer basename across candidate, displaced-occupant, and guarded-reverse states; no second recovery name is created."
  - "Treat absent-target post-create rollback as recovery-required because this phase has no audited identity-conditional inverse for recreating absence."
  - "Permit safe loader/cache/initializer compensation after a pre-namespace target-parent authentication failure, while retaining the unauthenticated target journal for recovery."
patterns-established:
  - "Run a pure adapter check and authenticated same-filesystem exchange/no-replace probe before initialization or visible startup effects."
  - "Confine target and transaction namespace mutations to one descriptor-authorized helper under a held root lock."
  - "Fsync candidate/evidence, transaction directory, journal, and every changed directory at explicit state-machine barriers."
requirements-completed: []
requirements-contributed: [PROF-03]
coverage:
  - id: D1
    description: "Install and ingest preparation preserve every ordinary startup byte, classify exact marker topology, and produce one independent canonical candidate."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/cli/install_test.go Task 1 exact selector set"
        status: pass
    human_judgment: false
  - id: D2
    description: "Linux and Darwin use audited atomic exchange and no-replace syscalls with per-call injection, strict basenames, and fail-closed other-platform behavior."
    requirement: PROF-03
    verification:
      - kind: unit
        ref: "core/cli/atomic_rename_test.go and platform-tagged atomic rename tests"
        status: pass
      - kind: other
        ref: "CGO-disabled Linux/Darwin amd64/arm64 and FreeBSD amd64 builds"
        status: pass
    human_judgment: false
  - id: D3
    description: "One authenticated private peer and strict journal promote, reverse, recover, or retain startup state without destroying concurrent occupants."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "core/cli/install_test.go Task 3 exact 37-test selector"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=local go test ./core/cli -count=1"
        status: pass
    human_judgment: false
  - id: D4
    description: "Secret-bearing source bytes occur outside the installed target only in the private exchange peer and are removed on durable finalize or retained with value-free recovery reporting."
    requirement: PROF-03
    verification:
      - kind: integration
        ref: "TestInstallSecretBearingPeerConfinedAndRemovedOnFinalize and TestInstallSecretBearingPeerRetainedOnlyForRecovery"
        status: pass
    human_judgment: true
    rationale: "Plan 06-05 still owns the final repository/native-platform secret scan and end-to-end fixture evidence, so PROF-03 remains globally open."
metrics:
  duration: 48m
  completed: 2026-08-03
status: complete
---

# Phase 06 Plan 03: Guarded Startup Promotion Summary

**Byte-exact install preparation now promotes one independently authenticated candidate through audited exchange/no-replace syscalls, a private one-peer journal, guarded reversal, and deterministic crash recovery.**

## Performance

- **Duration:** 48 minutes
- **Started:** 2026-08-03T20:57:38Z
- **Completed:** 2026-08-03T21:45:08Z
- **Tasks:** 3/3
- **Files modified:** 12

## Accomplishments

- Unified ordinary install and future ingest preparation around one exact marker scanner, complete eligible source, bounded original snapshot, and canonical byte-preserving candidate.
- Added local-Go-1.25-audited Linux `renameat2` and Darwin `renameatx_np` adapters for exchange and exclusive creation, including exact architecture traps, strict basename validation, typed unsupported results, and executable other-platform selection.
- Added a pure capability gate followed by a real same-filesystem private probe before Store initialization, cache creation, loader promotion, or target mutation.
- Added current-EUID mode-0700 transaction directories, a fixed authenticated root lock, exactly one exchange peer, value-free candidate evidence, and a strict versioned journal with retained descriptor identity.
- Added independent pre/post target and candidate evidence, guarded late-substitution reversal, absent-target recovery retention, eight fresh-recovery rows, directory-fsync uncertainty handling, and secret-confinement assertions.

## Task Commits

1. **Task 1: Preserve exact source/layout and one canonical install candidate** — `0e47b7d` (RED test), `329138f` (implementation)
2. **Task 2: Implement audited Linux/Darwin atomic rename adapters** — `cd8bc75` (RED test), `c533b77` (implementation)
3. **Task 3: Integrate guarded promotion, journal, and recovery** — `c916011` (RED test), `2a82de8` (implementation)

## Files Created/Modified

- `core/cli/atomic_rename.go` — common validation, typed unsupported classification, sibling wrapper, and cross-directory per-call adapter.
- `core/cli/atomic_rename_linux.go` — complete Go 1.25 Linux architecture/trap table, exchange/no-replace flags, and descriptor lock.
- `core/cli/atomic_rename_darwin.go` — audited Darwin trap/flags and descriptor lock.
- `core/cli/atomic_rename_other.go` — exact fail-closed adapter selected on unsupported platforms or the narrow test tag.
- `core/cli/atomic_rename_test.go` — pure capability, validation, parallel injection, and local-toolchain ABI/fallback audit.
- `core/cli/atomic_rename_supported_test.go` — direct exchange, no-replace, late-target, and injected-unsupported runtime tests.
- `core/cli/atomic_rename_unsupported_test.go` — locally executable production other-adapter zero-call proof.
- `core/cli/install_transaction.go` — marker/source preparation plus authenticated probe, journal, promotion, reversal, recovery, cleanup, and lock state machine.
- `core/cli/install.go` — routes install through preflight and the one guarded target promotion with typed compensation.
- `core/cli/install_test.go` — exact order/layout plus 37 crash, authentication, identity, secret, lock, and compensation contracts.
- `core/cli/store.go` — carries canonical initialization root and authenticated ingest transaction authority through the install seam.
- `core/store/store_root_private_other.go` — fail-closed unsupported-platform helper definitions required for FreeBSD compilation.

## Decisions Made

- Extended the audited rename call internally to accept distinct retained directory descriptors. The public-in-package sibling wrapper remains unchanged for direct adapter tests, while the secret-bearing peer stays below the private transaction directory instead of beside `.zshrc`.
- Kept journal content free of source bytes: candidate evidence stores only identity, digest, mode, and ownership; the full candidate exists only at the private exchange peer and installed target.
- Held the stable root lock across transaction authentication, namespace mutation, postvalidation, journal transitions, and all changed-directory syncs. External writers remain governed by exchange/no-replace plus postvalidation, not by advisory-lock claims.
- Retained an open journal descriptor so unlink/recreate substitution cannot evade an inode-only check through immediate inode reuse.
- Preserved the landed ordinary-install guarantee that a pre-namespace parent-mode/authentication failure rolls back the already-promoted loader and Store initialization; the unauthenticated target journal itself remains untouched for recovery.

## Validation Results

- Task 1 exact 13-test name/cardinality selector — PASS.
- Task 2 exact 4-test selector plus Linux/Darwin amd64/arm64 and FreeBSD amd64 test-binary compilation — PASS.
- Task 3 unsupported-tag exact test, 4 common adapter tests, 4 local platform tests, and 37 install/journal tests — PASS.
- `GOTOOLCHAIN=local go test ./core/cli -count=1` and `GOTOOLCHAIN=local go vet ./core/cli` — PASS.
- `GOTOOLCHAIN=local go build ./...`, `go test ./... -count=1`, and `go vet ./...` — PASS.
- `GOTOOLCHAIN=local golangci-lint run` — PASS, `0 issues`.
- CGO-disabled full repository builds for Linux amd64/arm64, Darwin amd64/arm64, and FreeBSD amd64 — PASS.
- All three GREEN commits passed the repository pre-commit lint hook — PASS.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Completed the unsupported Store adapter for FreeBSD compilation.**

- **Found during:** Task 2 cross-platform verification.
- **Issue:** The pre-existing `core/store` other-platform file omitted fail-closed namespace, lock, ownership, and directory-sync helpers referenced by the untagged Store transaction implementation, so the required FreeBSD CLI test binary could not compile.
- **Fix:** Added unsupported definitions that always reject authentication/sync authority and never mutate storage.
- **Files modified:** `core/store/store_root_private_other.go`
- **Verification:** FreeBSD amd64 CLI test binary and final full repository build compile with `CGO_ENABLED=0 GOTOOLCHAIN=local`.
- **Commit:** `c533b77`

**2. [Rule 1 - Bug] Preserved landed loader rollback before any target namespace syscall.**

- **Found during:** Task 3 full CLI-suite verification.
- **Issue:** A target-parent mode/authentication change detected before the target syscall was classified as recovery-required and initially left the already-promoted loader in place, breaking the landed startup rollback contract even though the visible target was untouched.
- **Fix:** Track whether a target namespace syscall actually succeeded. Pre-namespace recovery retains the target journal but safely restores loader/cache/initializer state; post-namespace uncertainty still disarms later compensation.
- **Files modified:** `core/cli/install.go`, `core/cli/install_transaction.go`
- **Verification:** `TestInstallRollsBackLoaderWhenPreparedZshrcCannotBePromoted`, all 37 Task 3 contracts, and the full CLI suite pass.
- **Commit:** `2a82de8`

---

**Total deviations:** 2 auto-fixed (1 blocking build issue, 1 correctness bug).
**Impact on plan:** Both changes were required to satisfy the planned cross-platform and startup-safety gates; no feature scope was added.

## Issues Encountered

- The first FreeBSD cross-build exposed missing fail-closed Store helper symbols; two narrow unsupported definitions passes resolved the complete compile surface.
- The first full Task 3 selector exposed three state-machine defects: prepared-journal recovery after exchange, journal unlink/recreate identity detection, and absent-target created-state authentication. Fresh descriptor anchoring and state-aware authentication resolved all three.
- The first full CLI-suite run exposed the pre-namespace loader rollback regression documented above.
- No authentication, package-install, external-service, or human-action gate was encountered.

## TDD Gate Compliance

- Task 1: `0e47b7d` (failing source/layout/install transaction contracts) -> `329138f` (passing implementation)
- Task 2: `cd8bc75` (failing atomic adapter contracts) -> `c533b77` (passing audited adapters)
- Task 3: `c916011` (failing guarded promotion contracts) -> `2a82de8` (passing promotion/recovery implementation)

## Known Stubs

None. The created and modified production files contain no TODO/FIXME/placeholder path, mock data source, or goal-blocking empty implementation.

## User Setup Required

None for this plan.

## Next Phase Readiness

- Plan 06-04 can consume `preparedIngestInstall`, `guardedInstallTransaction`, and `promotionOutcome` for one loader/target promotion around the typed Store commit.
- Plan 06-05 can exercise the real end-to-end fixture, native macOS evidence, and final repository secret scan.
- `PROF-03` remains globally unchecked until those controller and final-evidence plans complete.

## Self-Check: PASSED

- Confirmed all 11 production/test files and this summary exist.
- Confirmed all six RED/GREEN task commits exist in Git history.
- Confirmed the summary records `status: complete`, preserves `PROF-03` as contributed rather than globally completed, and contains no unresolved stub.

---
*Phase: 06-ingest-end-to-end*
*Completed: 2026-08-03*
