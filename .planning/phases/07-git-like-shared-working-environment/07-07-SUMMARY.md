---
phase: 07-git-like-shared-working-environment
plan: 07
subsystem: worktree-composition
tags: [git, ingest, worktree, composition-root, exact-oid, secret-policy, descriptor-authentication]
requires:
  - phase: 07-02
    provides: typed ingest publication and recovery evidence
  - phase: 07-03
    provides: canonical path-bound and authenticated-descriptor worktree state stores
  - phase: 07-05
    provides: exact committed-worktree DTO reads and expected-base Git publication
  - phase: 07-06
    provides: public worktree reader/workflow service and exact split-recovery policy
  - phase: 07-08
    provides: credential-bearing runtime adapter and late-bound worktree factory
provides:
  - authoritative exact PublishedRevision evidence for committed ingest outcomes
  - versioned combined ingest commits readable by exact OID with legacy source-read compatibility
  - post-finalize idempotent materialization and exact published-ref split repair
  - validated direct-branch Store revision resolution without legacy current authority
  - one lazy public path-bound Service and one configured descriptor-bound runtime factory using concrete zsh policy
affects: [07-09-runtime-hooks, 07-10-integration, shared-worktree-sync, ingest-bootstrap]
tech-stack:
  added: []
  patterns:
    - exact candidate OID evidence is assigned only at proven Git publication points
    - ingest publishes the combined source/projection DTO consumed by exact worktree reads
    - public authority is path-bound and lazy while runtime authority remains descriptor-bound and operation-scoped
    - security policy is one concrete provider configuration, while durable generation identity defines authority
key-files:
  created: []
  modified:
    - core/model/ingest_transaction.go
    - core/cli/ingest.go
    - core/cli/ingest_test.go
    - core/store/store.go
    - core/store/store_test.go
    - core/cmd/zsh-pro/main.go
    - core/cmd/zsh-pro/main_test.go
key-decisions:
  - "PublishedRevision is defensively copied and valid only with exact candidate-ref/object publication evidence; no post-commit ref lookup may manufacture it."
  - "Ingest candidates use the versioned CommittedWorktree DTO so recovery reads the published OID directly; legacy source-only objects remain source-readable but are never promoted into worktree authority by inference."
  - "The production public Service opens the canonical path lazily after initialization, while RuntimeWorktreeFactory retains only concrete zsh configuration and binds fresh Services from authenticated descriptors later."
  - "A materializer encountering an existing generation delegates to Service's under-lock exact-ref repair and succeeds only when the exact published projection equals shared state."
requirements-completed: [WORK-01, WORK-02, SYNC-01, SYNC-02]
coverage:
  - id: D1
    description: "Committed ingest evidence carries the exact published OID and materializes or repairs one clean canonical generation only after target and initializer finalization."
    requirement: WORK-01
    verification:
      - kind: unit
        ref: "core/cli/ingest_test.go#TestRunIngestMaterializesExactPublishedRevisionAfterFinalize"
        status: pass
      - kind: integration
        ref: "core/store/store_test.go#TestCommitIngestPublishedRevisionIsExactAndReplaySafe"
        status: pass
      - kind: e2e
        ref: "core/cmd/zsh-pro/main_test.go#TestMainIngestHumanAndJSON"
        status: pass
    human_judgment: false
  - id: D2
    description: "The public and late-bound runtime service configurations both use the real zsh live-secret policy, with SecretRefs pinned before value admission."
    requirement: WORK-02
    verification:
      - kind: integration
        ref: "core/cmd/zsh-pro/main_test.go#TestLiveSecretPolicyCompositionRejectsPostAttachSecretCanary"
        status: pass
      - kind: unit
        ref: "core/cli/emitter_test.go#TestRuntimeWorktreeFactoryBindsCanonicalDescriptorAndOwnsOnlyDuplicate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Public status repairs from an exact direct-branch OID into the single combined worktree generation without legacy current/profile-marker authority."
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "core/cmd/zsh-pro/main_test.go#TestCanonicalWorktreeAuthorityRepairsExactRevision"
        status: pass
      - kind: unit
        ref: "core/store/store_test.go#TestResolveWorktreeRevisionUsesValidatedDirectBranchAuthority"
        status: pass
    human_judgment: false
  - id: D4
    description: "Runtime worktree construction remains deferred until an authenticated RuntimeRoot descriptor exists, and the bound StateStore owns only its duplicate."
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "core/cli/emitter_test.go#TestRuntimeWorktreeFactoryBindsCanonicalDescriptorAndOwnsOnlyDuplicate"
        status: pass
      - kind: unit
        ref: "core/cmd/zsh-pro/main_test.go#TestMainWorktreeCompositionContract"
        status: pass
    human_judgment: false
metrics:
  duration: 25m
  completed: 2026-08-17
  tasks: 2
  files: 7
status: complete
---

# Phase 7 Plan 7: Exact Ingest Materialization and Canonical Composition Summary

**Ingest now publishes exact combined worktree revisions, materializes them after finalization, and composes concrete zsh policy through one canonical public generation plus late-bound descriptor runtime services.**

## Performance

- **Duration:** 25 minutes
- **Started:** 2026-08-17T14:26:51Z
- **Completed:** 2026-08-17T14:51:03Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments

- Added validated, defensive `PublishedRevision` evidence only at authoritative candidate publication points and preserved it through committed cleanup recovery and terminal replay.
- Changed ingest publication to write a versioned complete source/projection DTO and matching generated zsh blob, enabling exact-OID materialization without a legacy-current or ref-inference fallback.
- Added post-finalize materialization with truthful committed/recovery output, exact repeat behavior, and next-status repair through Service's locked exact-projection reconciliation.
- Added the narrow Store direct-branch revision resolver required by the Phase 7 Repository contract, including hostile, missing, and direct-ref tests.
- Composed a lazy public path-bound StateStore/Registry/Service and retained a descriptor-bound RuntimeWorktreeFactory using the same concrete `zsh.Provider` decoder/emitter/secret-policy configuration.
- Proved real-provider post-attach secret identities remain value-free and confirmed runtime credentials and descriptor ownership remain inside the existing Service/factory contracts.

## Task Commits

Each task used RED/GREEN TDD commits with normal repository hooks:

1. **Task 1: Carry exact commit evidence into idempotent materialization and repair**
   - `ab435e3` — `test(07-07): define exact ingest materialization contract`
   - `0b2b569` — `feat(07-07): materialize exact published ingest revision`
2. **Task 2: Inject the concrete classifier policy into one canonical late-bound authority contract**
   - `1e1c803` — `test(07-07): define canonical worktree composition contract`
   - `3aae59b` — `feat(07-07): compose canonical durable worktree authority`

## Files Created/Modified

- `core/model/ingest_transaction.go` — Exact published-revision constructor, validation, and defensive outcome cloning.
- `core/cli/ingest.go` — Optional source-compatible materializer seam, post-finalize exact invocation, and recovery truth.
- `core/cli/ingest_test.go` — Publication validation, ordering, split-failure, noncommit, and no-disclosure tests.
- `core/store/store.go` — Exact published candidate evidence, combined ingest DTO publication, idempotent exact repeat, and validated direct-branch resolution.
- `core/store/store_test.go` — Authoritative evidence matrices, exact DTO/blob/source compatibility, replay, resolver, hostile-ref, and recovery tests.
- `core/cmd/zsh-pro/main.go` — Lazy public path-bound worktree authority, materializing ingest wrapper, exact status repair, and configured runtime factory.
- `core/cmd/zsh-pro/main_test.go` — Structural composition contract, exact canonical repair, and real zsh secret-policy canary.

## Decisions Made

- The candidate OID known inside the prepared ref transaction is the only source of `PublishedRevision`; no later branch lookup can turn ambiguous state into authoritative evidence.
- The exact published commit itself must contain the complete versioned worktree DTO. `ReadWorktreeRevision` remains strict and does not invent projection state for legacy source-only objects.
- All secret-category identities are omitted from the materialized projection while redacted SecretRefs and late-bound dynamic source remain in the source DTO; Registry seeding owns and pins those identities before attachment.
- Public construction is lazy so `analyze` does not create or require storage. Once bound, the public Service retains one path-authenticated StateStore. Runtime construction remains deferred to `RuntimeWorktreeFactory.Bind` and never receives the public path or StateStore.
- Distinct Service pointers are correct. The authenticated combined `worktree.json` generation, exact branch/OID, and unchanged under-lock credential checks define authority.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added authoritative Store publication evidence**

- **Found during:** Task 1 RED
- **Issue:** The plan required exact `PublishedRevision`, but Store outcomes had no authoritative value and CLI could not infer one safely.
- **Fix:** Assigned the validated candidate OID only after a successful ref transaction or exact observed-candidate recovery, and cloned it in terminal outcomes.
- **Files modified:** `core/store/store.go`, `core/store/store_test.go`
- **Verification:** Committed, replay, cleanup-recovery, conflict, and failure matrices passed.
- **Committed in:** `0b2b569`

**2. [Rule 2 - Missing Critical Functionality] Published combined ingest candidates**

- **Found during:** Task 2 production composition E2E
- **Issue:** Existing ingest commits contained legacy source-only `profile.json`, while the security contract correctly requires `ReadWorktreeRevision` to reject objects with no authenticated projection. Exact materialization therefore had no valid DTO at the published OID.
- **Fix:** Built the projection from the redacted full Profile, excluded every secret-classified identity, persisted the versioned combined DTO plus matching generated zsh bytes, preserved explicit-empty source fidelity and legacy source decoding, and treated an identical exact prior document as a no-move repeat.
- **Files modified:** `core/store/store.go`, `core/store/store_test.go`
- **Verification:** Exact readback, raw blob agreement, legacy `Read`, repeat OID, real ingest E2E, and full Store suites passed.
- **Committed in:** `3aae59b`

**3. [Rule 3 - Blocking] Implemented the Phase 07-06-to-07-07 Repository resolver seam**

- **Found during:** Task 2 composition
- **Issue:** `worktree.Repository` already required `ResolveWorktreeRevision`, but the concrete Store lacked it, preventing production Service injection.
- **Fix:** Added a narrow local-branch resolver using validated branch values, direct-ref authentication/observation, and exact commit-type validation. It never calls legacy `Current`, accepts caller revision expressions, or executes raw Git in `main`.
- **Files modified:** `core/store/store.go`, `core/store/store_test.go`, `core/cmd/zsh-pro/main.go`
- **Verification:** Success, missing, hostile-name, symbolic-loose-ref, and production interface assertion tests passed.
- **Committed in:** `3aae59b`

**4. [Rule 1 - Bug] Preserved direct-constructor ingest compatibility**

- **Found during:** Full CLI regression gate after Task 2 GREEN
- **Issue:** Treating a missing additive materializer as a production failure broke existing direct CLI embedders and Phase 6 E2E fixtures, contrary to the plan's constructor compatibility requirement.
- **Fix:** Materialization is invoked when the additive interface is present; the sole production composition always supplies it through the transaction wrapper, while legacy direct constructors retain prior behavior.
- **Files modified:** `core/cli/ingest.go`
- **Verification:** Full CLI ingest E2E and production main ingest suites passed.
- **Committed in:** `3aae59b`

**Total deviations:** 4 auto-fixed (2 Rule 3, 1 Rule 2, 1 Rule 1)

**Impact on plan:** Each correction was necessary to make the planned exact-OID composition executable while preserving prior APIs and security invariants. No new command, runtime operation, dependency, or alternate authority was added.

## TDD Gate Compliance

- Task 1 RED: `ab435e3`
- Task 1 GREEN: `0b2b569`
- Task 2 RED: `1e1c803`
- Task 2 GREEN: `3aae59b`
- Normal pre-commit hooks remained enabled and reported zero lint issues for every commit.

## Verification

- Plan Task 1 published-revision/materialization/recovery/pinned-secret test command — passed.
- Plan Task 2 main composition/canonical authority/runtime/policy/construction/analyze test command — passed.
- Exact Store ingest DTO, blob agreement, replay, cleanup, conflict, recovery, and resolver tests — passed.
- Runtime factory authenticated-descriptor, duplicate ownership, typed-nil, credential-forwarding, and numeric acknowledgement tests — passed.
- `GOTOOLCHAIN=local go test ./core/model ./core/cli ./core/cmd/zsh-pro -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/store ./core/worktree -count=1` — passed.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed.
- `GOTOOLCHAIN=local go vet ./...` — passed.
- `GOTOOLCHAIN=local go build ./...` — passed.
- `golangci-lint run` — passed with 0 issues.
- AST/stub/secret/threat/legacy-authority scans — passed. Placeholder hits are limited to reviewed secret-test fixture assertions.

## Known Stubs

None. Modified production paths contain no TODO, FIXME, coming-soon, placeholder UI data, panic, or not-implemented branch.

## Auth Gates

None.

## Tracking Notes

- The plan declares WORK-01, WORK-02, SYNC-01, and SYNC-02, but those IDs are absent from the current `.planning/REQUIREMENTS.md`; no substitute IDs were invented.
- Runtime worktree operation dispatch remains Plan 07-09 work. This plan configures and retains the descriptor-bound factory without eagerly binding or adding a private verb.

## Threat Model Coverage

- **T-07-27:** Exact candidate evidence, combined authenticated DTOs, post-finalize ordering, and locked exact-ref repair prevent false publication or inferred recovery authority.
- **T-07-28:** Concrete zsh policy is injected on public and runtime configurations; secret-category projection values are excluded and SecretRefs are pinned during materialization.
- **T-07-29:** Public path binding and runtime descriptor binding address the same canonical combined generation without singleton, fallback, reopen, legacy current, or process-marker authority.
- **T-07-29A:** RuntimeWorktreeFactory forwards credential-bearing requests unchanged; capability verification and receipt access remain inside Service transactions.
- **T-07-30:** Store construction failures remain captured, public binding is lazy, dependent worktree verbs fail closed, and analyze remains available.

No new network endpoint, external authentication path, dependency, or unplanned schema was introduced. The planned path-bound public file-access surface and descriptor-bound runtime surface are covered by the threat register above.

## Issues Encountered

- Combined DTO validation exposed that dynamic secret identities were being projected despite their source being correctly classified as secret. The projection now excludes all secret-category identities while retaining only safe late-bound/redacted source metadata.
- Exact source compatibility exposed an explicit-empty versus nil slice normalization at the in-memory worktree constructor. Ingest keeps the validated persisted source shape directly and uses defensive worktree cloning only for runtime state copies.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 07-09 can bind validated runtime requests through the configured descriptor-only factory and existing credential-bearing adapter.
- Plan 07-10 can exercise end-to-end shared generation equivalence, exact ingest bootstrap/repair, and runtime hook behavior.
- No blockers remain.

## Self-Check: PASSED

- All seven modified source/test files and this Summary exist.
- RED/GREEN commits `ab435e3`, `0b2b569`, `1e1c803`, and `3aae59b` are present in repository history.
- Complete status, requirement metadata, verification evidence, stub scan, and threat coverage are recorded above.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-17*
