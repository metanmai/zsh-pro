---
phase: 07-git-like-shared-working-environment
plan: 15
subsystem: shell-runtime
tags: [zsh, shared-worktree, attach, reconciliation, bounded-sync]

requires:
  - phase: 07-11
    provides: durable admission and canonical shared-worktree state
  - phase: 07-14
    provides: fail-fast live apply and continuous reverse ownership through acknowledgement
provides:
  - Truthful exact-versus-reconcile attachment metadata preserved through replay
  - Canonical five-field value-free private ZPWA response grammar
  - One-call bounded first-sync reconciliation for an unattached mismatched retained zsh
affects: [phase-07-verification, shared-worktree-runtime, shell-runtime-security]

tech-stack:
  added: []
  patterns:
    - Derive convergence metadata in the same canonical transaction as durable shell state
    - Publish only from an applied shell; first mismatched attach proceeds directly to prepare/apply/capture/ack
    - Clear parent-local reconcile truth only after exact acknowledgement

key-files:
  created: []
  modified:
    - core/model/worktree.go
    - core/worktree/service.go
    - core/cli/runtime.go
    - core/shell/zsh/hook.go
    - core/shell/zsh/worktree_live_test.go

key-decisions:
  - "AttachResult.ReconcileRequired is derived from the same AttachState decision that sets AppliedRevision and Behind, and its complete value is stored in replay receipts."
  - "A first explicit sync that attaches mismatched skips publish and consumes the normal prepare/apply/fresh-capture/acknowledge pipeline under one absolute deadline."
  - "ZP_WORKTREE_RECONCILE_REQUIRED remains non-exported and is cleared only by exact acknowledgement or terminal-local worktree disable."

patterns-established:
  - "Truthful attachment: a durable head is locally applied only when the captured parent shell is exactly at that head."
  - "One-call convergence: mismatched attach is a routing decision, not a successful terminal state."

requirements-completed: [WORK-01, SYNC-01, SYNC-02]

coverage:
  - id: D1
    description: "Attachment distinguishes exact-at-head state from clean-reconcile state and replays the identical value-free result."
    requirement: WORK-01
    verification:
      - kind: unit
        ref: "core/worktree/service_test.go#TestAttachResultTruthfullyReportsCleanReconcile"
        status: pass
      - kind: unit
        ref: "core/model/worktree_test.go#TestWorktreePublicResultTypesStayValueFree"
        status: pass
    human_judgment: false
  - id: D2
    description: "The private attach response carries only the durable head, attached bit, and canonical reconcile-required bit."
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "core/cli/runtime_test.go#TestRuntimeAttachReplyCarriesReconcileState"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/hook_test.go#TestWorktreeAttachReplyGrammarCarriesReconcileState"
        status: pass
    human_judgment: false
  - id: D3
    description: "One explicit public sync reconciles a mismatched unattached retained zsh, while failures remain behind, recoverable, value-free, and usable."
    requirement: SYNC-02
    verification:
      - kind: e2e
        ref: "core/shell/zsh/worktree_live_test.go#TestWorktreeFirstExplicitSyncReconciles"
        status: pass
      - kind: e2e
        ref: "core/shell/zsh/worktree_live_test.go#TestWorktreeFirstExplicitSyncFailureStaysBehind"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=local go test -race -count=1 ./core/worktree ./core/cli ./core/shell/zsh -timeout=300s"
        status: pass
    human_judgment: false

duration: 31min
completed: 2026-08-18
status: complete
---

# Phase 07 Plan 15: Truthful First-Sync Reconciliation Summary

**Exact attachment now returns immediately, while a mismatched unattached retained zsh reaches the canonical head through one bounded public sync and reports success only after fresh exact acknowledgement.**

## Performance

- **Duration:** 31 min
- **Started:** 2026-08-18T03:09:26Z
- **Completed:** 2026-08-18T03:39:56Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments

- `AttachResult` now carries `ReconcileRequired`, derived atomically from the durable shell attach state and preserved identically by lost-response receipt replay.
- The private runtime transport emits exactly `ZPWA 1 <head> 1 <0|1>` and the loader rejects missing, extra, trailing, contradictory, and noncanonical reply shapes before mutating attachment globals.
- A mismatched first explicit sync keeps applied revision zero, skips publication, and runs prepare, Plan 07-14 apply/recovery, fresh capture, and exact acknowledgement under the original absolute deadline.
- Exact attach and repeated exact sync remain idempotent; auto-apply true/false affects prompt boundaries without blocking explicit sync.
- Real retained-zsh failure coverage proves prepare, timeout, apply, post-apply capture, and acknowledgement failures stay behind/value-free, retain reverse ownership after mutation, and leave the next command usable.

## Task Commits

Each task was committed atomically with its TDD gate:

1. **Task 1 RED: truthful attach result contract** - `855f109`
2. **Task 1 GREEN: atomic attach reconcile metadata** - `de28af4`
3. **Task 2 RED: five-field private reply contract** - `2164076`
4. **Task 2 GREEN: canonical reconcile-bit encoding** - `37ef313`
5. **Task 3 RED: retained-zsh first-sync contract** - `aa1515e`
6. **Task 3 GREEN: one-call mismatched reconciliation** - `462266d`
7. **Rule 3 integration fix: exact loader-symbol allowlist** - `a2a246b`
8. **Rule 1 test fix: race-safe retained stderr assertion** - `e07a025`
9. **Rule 1 test fix: deadline-faithful production harness** - `fd6b8c1`

## Files Created/Modified

- `core/model/worktree.go` - Adds value-free reconcile-required attachment metadata.
- `core/model/worktree_test.go` - Extends public result boundary coverage to the new boolean field.
- `core/worktree/service.go` - Derives exact/mismatch truth inside the attach transaction and receipt replay.
- `core/worktree/service_test.go` - Covers exact, mismatch, replay, and concurrent later-head attachment truth.
- `core/cli/runtime.go` - Encodes the strict five-field ZPWA response.
- `core/cli/runtime_test.go` - Covers canonical 0/1 replies, invalid results, and value/capability exclusion.
- `core/shell/zsh/hook.go` - Tracks parent-local reconcile state and routes mismatched first sync through pull before success.
- `core/shell/zsh/hook_test.go` - Enforces exact reply grammar and updates private transport fixtures.
- `core/shell/zsh/worktree_live_test.go` - Provides retained-zsh one-call success, failure, timeout, recovery, auto-apply, and idempotence evidence.
- `core/cmd/zsh-pro/main_test.go` - Keeps the installed-loader parameter allowlist exact for the new private non-exported marker.

## Decisions Made

- The durable head remains `AttachResult.Revision`; `ReconcileRequired` alone determines whether that head may become the parent shell's applied revision at attachment.
- A mismatched first explicit sync never publishes its initial snapshot. It immediately pulls the canonical target through the existing authenticated and recoverable transition pipeline.
- Reconcile-required becomes true before any nonempty transition applies and becomes false only after the exact acknowledgement response updates applied revision and baseline.
- Real-zsh success fixtures use a preallocated private credential but remain durably unattached, removing an artificial logging process from the production 250 ms transition budget while still proving first-call attach and convergence.

## Validation

- Focused model/worktree/CLI/zsh package suites - pass.
- `GOTOOLCHAIN=local go test -count=5 ./core/shell/zsh -run '^(TestWorktreeFirstExplicitSyncReconciles|TestWorktreeFirstExplicitSyncFailureStaysBehind)$' -timeout=90s` - pass.
- `GOTOOLCHAIN=local go test -count=1 ./... -timeout=240s` - pass.
- `GOTOOLCHAIN=local go test -race -count=1 ./core/worktree ./core/cli ./core/shell/zsh -timeout=300s` - pass.
- `GOTOOLCHAIN=local go vet ./...` - pass.
- `GOTOOLCHAIN=local golangci-lint run ./...` - pass with zero issues.
- `GOTOOLCHAIN=local go build ./...` - pass.
- `GOTOOLCHAIN=local make check` - pass, including vet, lint, and the full test suite.
- `git diff --check` - pass.

## TDD Gate Compliance

All three tasks have a failing `test(07-15)` commit before their corresponding passing `feat(07-15)` commit.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking integration test] Added the private reconcile marker to the exact installed-loader symbol allowlist**
- **Found during:** Overall repository verification
- **Issue:** The plan-required non-exported loader parameter correctly appeared in a clean shell, but the exact integration allowlist had not been included in the plan's declared test files.
- **Fix:** Added only `ZP_WORKTREE_RECONCILE_REQUIRED` to `core/cmd/zsh-pro/main_test.go` after explicit parent authorization.
- **Files modified:** `core/cmd/zsh-pro/main_test.go`
- **Verification:** Isolated symbol oracle, full repository suite, lint, and `make check` pass.
- **Committed in:** `a2a246b`

**2. [Rule 1 - Test race] Joined the retained shell before inspecting stderr**
- **Found during:** Touched-surface race gate
- **Issue:** The new value-leak assertion read a `bytes.Buffer` while `os/exec` could still write retained-shell stderr.
- **Fix:** Kept the next-command sentinel first, then closed/joined the shell before inspecting stderr.
- **Files modified:** `core/shell/zsh/worktree_live_test.go`
- **Verification:** Focused and full touched-surface race gates pass.
- **Committed in:** `e07a025`

**3. [Rule 1 - Test timing] Removed artificial helper overhead from successful first-sync evidence**
- **Found during:** Full repository verification
- **Issue:** A logging shell wrapper consumed the real 250 ms deadline under load and could make an otherwise correct acknowledgement miss its budget.
- **Fix:** Success/apply/capture cases now invoke the production binary directly with a preallocated but durably unattached credential; wrappers remain only where transport failure injection requires them.
- **Files modified:** `core/shell/zsh/worktree_live_test.go`
- **Verification:** Five repeated focused runs, full repository tests, full race gate, and `make check` pass.
- **Committed in:** `fd6b8c1`

---

**Total deviations:** 3 auto-fixed (1 Rule 3, 2 Rule 1)
**Impact on plan:** All changes are narrow verification/correctness support; no public surface, dependency, or production scope was added.

## Issues Encountered

- One early full shell-package run observed the pre-existing 250 ms two-shell timing-sensitive assertion. Its immediate isolated retry and every later shell, repository, race, and `make check` gate passed.
- The plan frontmatter names `WORK-01`, `SYNC-01`, and `SYNC-02`, but those IDs do not exist in the current `REQUIREMENTS.md`; the required completion handler reported all three as not found and made no requirements-file change.

## Known Stubs

None.

## Authentication Gates

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Final Phase 07 verification can rely on truthful attach metadata and a deterministic one-call first-sync oracle across exact, mismatch, failure, conflict, auto-apply, timeout, and idempotence paths.
- No blockers remain. Pre-existing unrelated untracked planning, graph, cache, and output paths were not touched.

## Self-Check: PASSED

- All ten modified files and this summary exist.
- All nine task/deviation commits exist in order with no tracked-file deletions.
- Focused, repeated real-zsh, full, race, vet, lint, build, `make check`, and diff-integrity gates pass.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-18*
