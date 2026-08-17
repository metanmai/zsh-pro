---
phase: 07-git-like-shared-working-environment
plan: 06
subsystem: worktree-cli
tags: [git, worktree, cli, cas, recovery, auto-apply, durable-state]
requires:
  - phase: 07-03
    provides: descriptor-bound transactional worktree state, status/diff, and shell coordination
  - phase: 07-04
    provides: typed semantic diff and complete effective projection construction
  - phase: 07-05
    provides: exact committed-worktree reads, current-base branch creation, and expected-base Git CAS publication
provides:
  - locked all-dirty commit, current-base branch, dirty-safe checkout, hard reset, and persisted auto-apply service policy
  - exact observed-ref recovery after proven Git publication and uncertain canonical-state save
  - narrow WorktreeReader and WorktreeWorkflow CLI dependencies with source-compatible constructors
  - strict value-safe public status, diff, commit, branch, checkout, reset, and config commands
  - hostile argv, sourced-shell ID, legacy-authority, CAS, fault, and real-Git workflow verification
affects: [07-07-composition, 07-09-runtime-hooks, public-cli, shared-worktree-sync]
tech-stack:
  added: []
  patterns:
    - one durable worktree transaction across dirty policy, expected base, repository effect, and canonical decision
    - exact-object reconciliation only when the observed projection equals the locked shared generation
    - complete argv and opaque shell-context validation before dependency access
    - value-free workflow output with stable category and branch ordering
key-files:
  created:
    - core/cli/worktree.go
    - core/cli/worktree_test.go
  modified:
    - core/worktree/service.go
    - core/worktree/service_test.go
    - core/cli/store.go
    - core/cli/cli.go
key-decisions:
  - "Treat State.Branch, State.BaseOID, and State.HeadRevision as the only generalized current-worktree authority; process-local profile markers are never consulted by Service workflow methods."
  - "Commit every semantic difference through activate.BuildEffective, then claim clean state only after reading the exact published object back and saving its canonical document."
  - "Repair a moved current ref only when its exact committed projection equals the locked shared state; never import a different external projection implicitly."
  - "Use ZSHPRO_SHELL_ID only as a validated optional status lookup key, and keep every Git-shaped CLI command on WorktreeReader/WorktreeWorkflow rather than the legacy Store surface."
requirements-completed: [WORK-03, SYNC-01, SYNC-02]
coverage:
  - id: D1
    description: "The service commits the complete dirty projection and implements current-base branch, dirty-safe checkout, hard reset, and persisted auto-apply policy under canonical locking."
    requirement: WORK-03
    verification:
      - kind: integration
        ref: "core/worktree/service_test.go#TestWorktreeWorkflowRealGitBoundary"
        status: pass
      - kind: unit
        ref: "core/worktree/service_test.go#TestWorktreeCommitIncludesEveryDirtyIdentityAndClearsOnlyCommittedOverlay"
        status: pass
    human_judgment: false
  - id: D2
    description: "Git publication and canonical-state split outcomes retain truthful recovery evidence and repair only through an exact observed object."
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: "core/worktree/service_test.go#TestWorktreeWorkflowRealGitBoundary/published_ref_repairs_exact_state_split"
        status: pass
      - kind: unit
        ref: "core/worktree/service_test.go#TestRecoveryAfterCommittedStateSaveFailureUsesExactPublishedRevision"
        status: pass
    human_judgment: false
  - id: D3
    description: "The public Git-shaped CLI validates complete argv and optional shell identity before access, delegates through narrow durable interfaces, and renders no captured values, raw errors, or object IDs."
    requirement: WORK-03
    verification:
      - kind: unit
        ref: "core/cli/worktree_test.go#TestUnsupportedGitAndMalformedWorktreeCommandsNeverReachDependencies"
        status: pass
      - kind: unit
        ref: "core/cli/worktree_test.go#TestWorktreeCommandsNeverUseLegacyStoreAuthority"
        status: pass
    human_judgment: false
  - id: D4
    description: "Status distinguishes shared truth, optional current-shell applied state, persisted auto-apply default, and effective override source."
    requirement: SYNC-01
    verification:
      - kind: unit
        ref: "core/cli/worktree_test.go#TestWorktreeStatusRendersSharedAndSourcedShellTruth"
        status: pass
      - kind: unit
        ref: "core/worktree/service_test.go#TestAutoApplyDefaultPreservesExplicitFalseOverrideAndDurableIdentity"
        status: pass
    human_judgment: false
metrics:
  duration: 29m
  completed: 2026-08-17
  tasks: 2
  files: 6
status: complete
---

# Phase 7 Plan 6: Shared Worktree Workflow and CLI Summary

**A locked all-dirty Git workflow now exposes durable status, diff, commit, branch, checkout, hard reset, and auto-apply configuration through a strict value-safe CLI.**

## Performance

- **Duration:** 29 minutes
- **Started:** 2026-08-17T13:44:15Z
- **Completed:** 2026-08-17T14:13:41Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added Service workflow methods that use only durable branch/base/revision state, hold the worktree transaction across policy and repository effects, commit every effective dirty identity, reject dirty checkout, and reserve discard semantics for explicit `reset --hard`.
- Added exact Git/state split recovery: a proven committed result remains committed/recovery-required, and the next status reads the observed exact object and repairs only when its projection equals the locked shared generation.
- Added narrow CLI reader/workflow interfaces plus the exact public command surface, including bare branch listing and current-base branch creation, while preserving existing constructor/list compatibility for later composition.
- Added complete hostile-input and value-safety coverage: malformed argv and shell IDs make zero service calls, unsupported Git breadth and direct binary sync stay absent, status/diff never print object IDs or captured values, and raw service errors never escape.
- Proved the service against actual Git refs for complete commit/readback, current-base create, dirty checkout/reset, expected-base CAS conflict, and publication/state split repair.

## Task Commits

Each task used RED/GREEN TDD commits with normal repository hooks:

1. **Task 1: Enforce locked all-dirty commit, branch, checkout, reset, and config policy**
   - `1b13a23` — `test(07-06): define shared worktree workflow contract`
   - `c2c0774` — `feat(07-06): enforce shared worktree workflow policy`
2. **Task 2: Expose the exact value-safe Git-shaped CLI surface**
   - `7620ba8` — `test(07-06): define strict worktree CLI contract`
   - `1bdef9d` — `feat(07-06): expose strict value-safe worktree CLI`
   - `6bc224a` — `fix(07-06): support durable branch listing`
3. **Plan verification hardening**
   - `0640a79` — `test(07-06): exercise real Git workflow boundary`

## Files Created/Modified

- `core/worktree/service.go` — Durable repository boundary, all-dirty commit, branch/list/checkout/reset/config operations, workflow status provenance, and exact observed-ref repair.
- `core/worktree/service_test.go` — Deterministic service/fault matrix plus actual-Git ref/CAS/split-recovery verification.
- `core/cli/store.go` — Narrow WorktreeReader and WorktreeWorkflow interfaces, kept separate from legacy profile Store compatibility.
- `core/cli/worktree.go` — Closed command parser, opaque shell-ID validation, narrow delegation, and value-safe stable renderers.
- `core/cli/worktree_test.go` — Accepted/forbidden command matrix, sourced-shell behavior, output canaries, typed outcomes, and poison legacy-store coverage.
- `core/cli/cli.go` — Worktree dependency injection and Git-shaped command dispatch while existing constructors remain source compatible.

## Decisions Made

- Workflow object identity is irrelevant: independent Service values see the same truth through the canonical durable generation.
- A clean commit creates no Git object. A dirty commit has no path/staging selection and builds one complete final projection from all semantic changes.
- Checkout refuses shared dirt and durable shell conflict/pending evidence before target access. ResetHard alone reads the exact current base and publishes the discard as a revision event.
- A moved current ref is not generalized pull behavior. It repairs canonical state only when the exact object is already the shared generation; any different projection remains recovery-required or an expected-base commit conflict.
- Public rendering deliberately omits BaseOID/commit OID and raw dependency errors. Branch and identity names are revalidated before output, categories and branches are sorted, and only boolean/count/revision/status data is emitted.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Added an exact branch-to-revision repository seam**

- **Found during:** Task 1 repository contract implementation
- **Issue:** Checkout and observed-ref split repair require an exact target OID, but Plan 07-05 exposes branch names and exact-OID reads without an exported branch-to-OID lookup.
- **Fix:** Added the narrow `Repository.ResolveWorktreeRevision` service dependency, used only under the worktree transaction before exact `ReadWorktreeRevision` authentication. No legacy `Current` or symbolic profile marker was used.
- **Files modified:** `core/worktree/service.go`, `core/worktree/service_test.go`
- **Commit:** `c2c0774`

**2. [Rule 1 - Bug] Restored the plan's accepted bare `branch` command**

- **Found during:** Plan-wide public-surface scan after Task 2 GREEN
- **Issue:** The first negative matrix incorrectly classified bare `branch` as invalid even though the plan inventory explicitly accepts both `branch` and `branch <name>`.
- **Fix:** Added branch listing to WorktreeReader, validated and sorted returned names, delegated exactly once, and retained `list` compatibility separately.
- **Files modified:** `core/cli/store.go`, `core/cli/worktree.go`, `core/cli/worktree_test.go`
- **Commit:** `6bc224a`

**3. [Rule 2 - Missing Critical Verification] Added the required real-Git workflow boundary test**

- **Found during:** Plan-wide verification audit
- **Issue:** The service/fault matrix was deterministic but initially exercised a repository fake only; the plan also requires focused real-Git evidence.
- **Fix:** Added a test-only adapter over the shipped Store operations and actual direct refs, covering complete publication/readback, current-base creation, dirty checkout/reset, CAS conflict, and exact split repair.
- **Files modified:** `core/worktree/service_test.go`
- **Commit:** `0640a79`

**Total deviations:** 3 auto-fixed (1 Rule 1, 2 Rule 2)

**Impact on plan:** All fixes close stated workflow or verification requirements without adding public Git breadth, dependencies, runtime hooks, ingest behavior, or production Store mechanics.

## TDD Gate Compliance

- Task 1 RED: `1b13a23`
- Task 1 GREEN: `c2c0774`
- Task 2 RED: `7620ba8`
- Task 2 GREEN: `1bdef9d`
- Contract correction: `6bc224a`
- Real-Git verification: `0640a79`
- Normal pre-commit hooks remained enabled and reported zero lint issues for every successful commit.

## Verification

- `GOTOOLCHAIN=local go test ./core/worktree -run 'Test(Worktree|Commit|Branch|Checkout|Reset|AutoApply|Recovery)' -count=1` — passed
- `GOTOOLCHAIN=local go test ./core/cli -run 'Test(Worktree|Status|SourcedShellID|Diff|Commit|Branch|Checkout|Reset|AutoApply|UnsupportedGit)' -count=1` — passed
- `GOTOOLCHAIN=local go test ./core/worktree ./core/cli -count=1` — passed
- `GOTOOLCHAIN=local go test ./... -count=1` — passed
- `GOTOOLCHAIN=local go vet ./...` — passed
- `GOTOOLCHAIN=local go build ./...` — passed
- `GOTOOLCHAIN=local golangci-lint run` — passed with 0 issues
- Stub/value/legacy-authority/unsupported-command/threat scans — passed for the new durable workflow path.

## Known Stubs

None. The modified production workflow contains no TODO, FIXME, placeholder, coming-soon, panic, or not-implemented path.

## Auth Gates

None.

## Tracking Notes

- The plan declares WORK-03, SYNC-01, and SYNC-02, but those IDs are absent from the current `.planning/REQUIREMENTS.md`; the required completion command reported all three as `not_found` and made no file change. No replacement IDs were invented.
- Plan 07-05 does not export branch-to-OID resolution. Plan 07-07 composition must supply the narrow `Repository.ResolveWorktreeRevision` boundary (preferably by exporting the existing Store ref observation capability) before injecting the concrete Store; it must not fall back to legacy `Current`, `Checkout`, or process-local markers.

## Threat Model Coverage

- **T-07-23:** Checkout checks the complete shared diff and durable conflict/pending evidence under one lock before repository target access; ResetHard is the only discard path and reads the exact stored base.
- **T-07-24:** Committed/ref-moved outcomes remain typed and value-free, uncertain state save returns recovery-required, and status repairs only from an exact observed object matching shared state.
- **T-07-25:** The whole argv and optional shell ID are validated before service access; exact lowercase booleans and flag shapes are closed, bounded, and tested with zero-call negatives.
- **T-07-26:** Status/diff/commit output omits raw object IDs, service errors, and captured values; returned branch/diff identities are validated before stable rendering.
- **T-07-26A:** Service workflow decisions never read `ZSHPRO_PROFILE`, `ZP_ACTIVE_PROFILE`, or Store.Current, and poison-store tests prove every injected Phase 7 CLI workflow command avoids the legacy Store surface.

No new network endpoint, authentication path, schema, package, dependency, or unplanned production file-access surface was introduced.

## Next Phase Readiness

Plan 07-07 can compose one public Service/reader/workflow/materializer over the canonical StateStore and Store, provided it closes the explicit branch-to-OID adapter seam without reintroducing legacy current-profile authority. Plan 07-09 can then consume the validated optional shell ID for applied/behind status while keeping sync operations sourced-loader-only.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-17*

## Self-Check: PASSED

All six implementation/test files and this summary exist; all six task/deviation commits are present in repository history; required frontmatter marks the plan complete.
