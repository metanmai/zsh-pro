---
phase: 07-git-like-shared-working-environment
plan: 11
subsystem: worktree-state
tags: [zsh, worktree, admission, canonical-state, concurrency, security]

requires:
  - phase: 07-git-like-shared-working-environment
    provides: authenticated canonical worktree state, value-safe Registry policy, operation receipts, and exact prepare/apply/acknowledge flow from Plans 07-02, 07-03, 07-07, and 07-10
provides:
  - Strict value-free admitted identity ownership in the canonical worktree generation
  - Policy-revalidated ownership restoration for every fresh Registry and Service
  - Atomic deterministic admission recording with legacy Shared-only migration
  - Fresh late-terminal exact-head convergence plus replay and concurrency proof
affects: [phase-07-verification, shared-worktree, registry, runtime-service, late-shell-sync]

tech-stack:
  added: []
  patterns:
    - Canonical kind/name-only admission metadata with explicit absent-field migration state
    - Transaction-local source seeding plus policy-revalidated durable ownership restoration
    - Same-lock admission, shared-value, event, shell-state, and receipt persistence

key-files:
  created: []
  modified:
    - core/worktree/state.go
    - core/worktree/state_test.go
    - core/worktree/registry.go
    - core/worktree/registry_test.go
    - core/worktree/service.go
    - core/worktree/service_test.go

key-decisions:
  - "Admitted ownership persists only exact model.Identity kind/name records; live values, capabilities, verifiers, shell snapshots, tokens, and errors remain outside the ownership field."
  - "Fresh registries restore durable admissions only after the existing identity, bookkeeping, volatile, pinned-secret, and concrete live-secret policy revalidates the complete set."
  - "Legacy field absence migrates only safe non-source identities already present in canonical Shared; explicit empty metadata remains authoritative and no ShellState or caller snapshot participates."

patterns-established:
  - "Canonical admission restoration: Seed committed Source, validate durable identities, then RestoreAdmitted under the StateStore transaction before interpreting live identities."
  - "Atomic admission append: sort and validate a defensive identity copy before the accepted live change, event, receipt, and metadata share one canonical replacement."

requirements-completed: [WORK-01, WORK-02, SYNC-01, SYNC-02]

coverage:
  - id: D1
    description: Canonical state round-trips a strict bounded duplicate-free admitted identity list containing kind/name metadata only, and fresh registries restore the same safe ownership decisions
    requirement: WORK-01
    verification:
      - kind: unit
        ref: core/worktree/state_test.go#TestStateAdmittedIdentitiesStrictValueFreeRoundTrip
        status: pass
      - kind: unit
        ref: core/worktree/registry_test.go#TestRegistryRestoreAdmittedRevalidatesPolicy
        status: pass
    human_judgment: false
  - id: D2
    description: A fresh late shell prepares and acknowledges the exact admitted head while replay and concurrent disjoint or overlapping admissions remain deterministic and duplicate-free
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: core/worktree/service_test.go#TestFreshServiceLateShellUsesDurableAdmission
        status: pass
      - kind: integration
        ref: core/worktree/service_test.go#TestConcurrentDurableAdmissionsAreDeterministic
        status: pass
      - kind: unit
        ref: core/worktree/service_test.go#TestWorktreeEdgeContract
        status: pass
    human_judgment: false
  - id: D3
    description: Legacy schema-v1 state derives admission candidates solely from safe canonical Shared identities and excludes shell-local present-at-attach state
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: core/worktree/service_test.go#TestLegacyAdmissionMigrationUsesCanonicalSharedOnly
        status: pass
    human_judgment: false

duration: 17min
completed: 2026-08-18
status: complete
---

# Phase 7 Plan 11: Durable Admitted Identity Ownership Summary

**Canonical kind/name-only admission metadata now survives fresh Services and drives exact late-terminal prepare/apply/acknowledge convergence without widening the live-value or secret boundary.**

## Performance

- **Duration:** 17 minutes
- **Started:** 2026-08-18T00:50:00Z
- **Completed:** 2026-08-18T01:06:20Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added bounded, canonical, duplicate-free `admitted_identities` state encoding with strict nested-field decoding, defensive cloning, explicit present-empty materialization, and preserved legacy field absence.
- Added all-or-nothing `Registry.RestoreAdmitted`, which reuses the existing closed identity and concrete live-secret policy without altering source ownership or SecretRef pinning.
- Reconstructed source plus admitted ownership inside authenticated StateStore transactions and recorded each new admission atomically with shared values, events, shell transitions, and operation receipts.
- Proved an A-publishes/late-C fresh-Service flow through stale local attach, exact current-head prepare, exact snapshot acknowledgement, idempotent replay, and concurrent disjoint/same-identity arbitration.

## Task Commits

1. **Task 1 RED: strict durable state and Registry restoration contract** - `1bd1849`
2. **Task 1 GREEN: canonical state field and policy-revalidated restoration** - `4fa60a5`
3. **Task 2 RED: fresh-Service late-C and concurrency contract** - `648c10f`
4. **Task 2 GREEN: transaction-local restoration, migration, and atomic recording** - `d5d93d6`

## Files Created/Modified

- `core/worktree/state.go` - Canonical admitted identity field, strict DTO presence handling, bounds, ordering, validation, and cloning.
- `core/worktree/state_test.go` - Value-free round-trip, absent/present compatibility, duplicate/order/invalid, and unknown value-bearing field rejection.
- `core/worktree/registry.go` - All-or-nothing policy-revalidated durable admission restoration.
- `core/worktree/registry_test.go` - Fresh-registry equivalence, unsafe-class rejection, ambient exclusion, idempotency, and no-partial-mutation coverage.
- `core/worktree/service.go` - Transaction-local source/admission restoration, safe legacy migration, canonical append, and atomic admission use across live workflows.
- `core/worktree/service_test.go` - Fresh late-C exact-head, replay, migration, secret/capability canary, and real concurrent publication coverage.

## Decisions Made

- Keep admitted ownership as exact identity metadata only; `Shared` remains the sole value-bearing authority and shell credentials remain verifier-authenticated views.
- Preserve missing-versus-empty admission metadata privately so only genuinely legacy payloads may migrate from canonical `Shared`; explicit empty is never broadened from ambient or shell-local inputs.
- Re-run the same production policy on every fresh restoration and fail the complete restoration rather than partially owning an unsafe durable set.
- Maintain canonical order by exact kind then name and make repeated append/replay idempotent under the existing one-generation lock.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Renamed the admitted-identity ordering helper**
- **Found during:** Task 1 GREEN compilation
- **Issue:** The initial helper name collided with the existing package-level `identityLess` in `diff.go`.
- **Fix:** Scoped the new name to `admittedIdentityLess` without changing ordering semantics.
- **Files modified:** `core/worktree/state.go`
- **Verification:** Focused Task 1 tests, full worktree tests, vet, lint, build, and race checks passed.
- **Committed in:** `4fa60a5`

---

**Total deviations:** 1 auto-fixed (1 Rule 3 blocking compile issue).
**Impact on plan:** The repair was naming-only, preserved the planned canonical ordering, and introduced no scope or API change.

## Issues Encountered

- The mandatory pre-commit typecheck cannot commit RED tests that reference not-yet-declared Go symbols. The RED contract therefore used runtime interface and strict JSON assertions; it still failed for the intended missing `Registry.RestoreAdmitted` and `admitted_identities` behaviors, while normal hooks remained enabled.
- `ROADMAP.md` has no Phase 7 row and `REQUIREMENTS.md` does not define the Plan 07 `WORK-*`/`SYNC-*` identifiers. SDK handlers are invoked at closeout, but no unrelated roadmap or legacy requirement content is fabricated.
- No authentication gates, package installs, external services, or user setup were required.

## Known Stubs

None.

## Verification

- Task 1 exact focused command: passed.
- Task 2 exact focused command, including the retained 21-edge aggregate: passed.
- `GOTOOLCHAIN=local go test -count=1 ./core/worktree -timeout=60s`: passed.
- `GOTOOLCHAIN=local go test -count=1 ./...`: passed.
- `GOTOOLCHAIN=local go test -race ./core/worktree ./core/store -count=1`: passed.
- `GOTOOLCHAIN=local go build ./...`: passed.
- `GOTOOLCHAIN=local make check`: passed; `go vet`, `gofmt -l`, full tests, and `golangci-lint` reported no issues.
- Admission JSON, secret/capability canaries, legacy migration, duplicate/order limits, stub patterns, and threat-boundary anchors: passed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Verification gap 1 now has executable fresh-instance closure evidence without weakening the Phase 7 21-edge matrix.
- Plans 07-12 through 07-15 can address the remaining independent deadline, partial-apply recovery, first-sync, and recovered-persistence gaps.
- Phase 7 remains in gap-closure execution; no completion claim is made for the other four verification findings.

## Self-Check: PASSED

- All six modified task files exist.
- RED/GREEN commits `1bd1849`, `4fa60a5`, `648c10f`, and `d5d93d6` exist in history.
- Every verification command listed above completed successfully on the final task state.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-18*
