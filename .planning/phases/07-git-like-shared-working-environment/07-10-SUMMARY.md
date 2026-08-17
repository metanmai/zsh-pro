---
phase: 07-git-like-shared-working-environment
plan: 10
subsystem: testing
tags: [zsh, worktree, e2e, security, atomicity, performance]

requires:
  - phase: 07-git-like-shared-working-environment
    provides: combined worktree authority, Git workflow, authenticated loader transport, and bounded transition pipeline from Plans 07-06, 07-07, and 07-09
provides:
  - Production-binary two-retained-shell proof of the complete shared worktree lifecycle
  - Exactly 21 executable edge probes plus descriptor ownership and security fault matrices
  - Repeatable 50-cycle production sampler with actual latency and subprocess counts
  - E2E-discovered correctness repairs for capture, convergence, dispatcher, reset, and deactivate behavior
affects: [phase-07-verification, shared-worktree, installed-loader, shell-runtime]

tech-stack:
  added: []
  patterns:
    - Canonical identity-record fingerprints with byte-exact ordered-list values
    - Retained two-process zsh harness over framed stdin/stdout markers
    - Production no-op latency sampling with process-count sentinels

key-files:
  created:
    - core/shell/zsh/worktree_live_test.go
    - scripts/perf-worktree.sh
  modified:
    - core/shell/zsh/hook.go
    - core/shell/zsh/introspect.go
    - core/worktree/registry.go
    - core/worktree/state.go
    - core/worktree/service.go
    - core/worktree/service_test.go
    - core/worktree/atomic_unix_test.go
    - core/cli/worktree_test.go
    - core/cmd/zsh-pro/main_test.go
    - core/store/store_test.go

key-decisions:
  - "Snapshot identity records are canonicalized by exact kind/name only for fingerprint/equality checks; PATH/FPATH element order, duplicates, empties, values, and presence remain byte-significant."
  - "Capture and admission exclude only invalid or loader-reserved symbol prefixes; ordinary underscore-prefixed user symbols remain supported."
  - "CaptureBaseline is the actual parent-shell patch base, while AppliedRevision and AppliedBaseline remain causal acknowledgement/history authority."
  - "Retained config and checkout/reset dispatch update local state only after exact successful production commands and preserve delegated failure codes."

patterns-established:
  - "Real shell E2E: build once, isolate HOME/ZDOTDIR/runtime root, retain two independent zsh -f processes, and assert observable next-command behavior."
  - "Canonical comparison without canonical storage: sort a defensive copy for fingerprints and never reorder Service state or ordered list payloads."

requirements-completed: [WORK-01, WORK-02, WORK-03, SYNC-01, SYNC-02]

coverage:
  - id: D1
    description: Two independent production shells converge through attach, admission, sync, conflict resolution, Git workflow, reset, checkout, and deactivate
    requirement: SYNC-01
    verification:
      - kind: e2e
        ref: core/shell/zsh/worktree_live_test.go#TestWorktreeTwoShellEndToEnd
        status: pass
    human_judgment: false
  - id: D2
    description: Canonical combined state and real Git DTO boundaries survive replay, faults, exact tree validation, and path/descriptor construction
    requirement: WORK-03
    verification:
      - kind: integration
        ref: core/store/store_test.go#TestWorktreeGitWorkflow
        status: pass
      - kind: integration
        ref: core/cmd/zsh-pro/main_test.go#TestCanonicalWorktreeAuthority
        status: pass
    human_judgment: false
  - id: D3
    description: Exactly 21 named edge predicates execute across all Phase 7 requirements
    requirement: WORK-02
    verification:
      - kind: unit
        ref: core/worktree/service_test.go#TestWorktreeEdgeContract
        status: pass
    human_judgment: false
  - id: D4
    description: Authenticated descriptor ownership, atomic faults, malformed state, contention, and public prohibitions fail closed
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: core/worktree/atomic_unix_test.go#TestWorktreeSecurityFaultMatrix
        status: pass
      - kind: unit
        ref: core/cli/worktree_test.go#TestWorktreePublicSurfaceAggregate
        status: pass
    human_judgment: false
  - id: D5
    description: Fifty production no-op cycles report actual latency with zero Git and child-zsh calls and pass fake/real deadline margins
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: scripts/perf-worktree.sh --cycles 50
        status: pass
    human_judgment: false

duration: 59min
completed: 2026-08-17
status: complete
---

# Phase 7 Plan 10: Complete Shared Worktree Contract Summary

**A built production loader now has executable two-shell, atomic-fault, exact-arity, deadline, and measured performance proof, with E2E-discovered convergence defects repaired.**

## Performance

- **Duration:** 59 minutes
- **Started:** 2026-08-17T16:05:20Z
- **Completed:** 2026-08-17T17:04:29Z
- **Tasks:** 2
- **Files modified:** 16
- **Measured no-op cycles:** count 50, p50 31.221390 ms, p95 33.883810 ms, max 36.371470 ms, total 1549.601795 ms
- **Measured subprocesses:** Git 0, child zsh 0
- **Measured durable state:** 123605 bytes, revision 1, dirty identities 0

## Accomplishments

- Drove a freshly built binary and installed loader through two retained independent `zsh -f` processes, proving stable private credentials, zero-process source/re-source, post-attach admission and secret rejection, auto/manual synchronization, disjoint and conflicting writes, exact shared resolution, public Git workflow, reset/checkout convergence, and residue-free idempotent deactivate.
- Added an exact 21-row requirement probe table, canonical descriptor ownership aggregate, atomic/security fault matrix, and exact public/prohibited command aggregate.
- Added a repeatable production sampler that reports actual distribution metrics, asserts zero Git/child-zsh calls, checks bounded durable state, and runs fake cumulative 249/251 ms plus active 25/500 ms deadline evidence.
- Repaired production issues that only appeared with the real installed loader: unsupported zle symbols, loader bookkeeping admission, worktree-only deactivate, record-order acknowledgements/resolution, stale auto-apply and behind status, masked mutation failures, and net-zero reset patch generation.

## Task Commits

1. **Task 1 production repair: unsupported capture symbols** - `548f35a`
2. **Task 1 production repair: loader bookkeeping admission** - `d72bc7f`
3. **Task 1 production repair: retained deactivate cleanup** - `a4594fb`
4. **Task 1 production repair: canonical fingerprints** - `68c6f02`
5. **Task 1 production repair: retained auto-apply config** - `9f78c32`
6. **Task 1 production repair: canonical convergence status/resolve** - `9dcafee`
7. **Task 1 production repair: mutation failure propagation** - `3db5d70`
8. **Task 1 production repair: current-capture pull patches** - `a9c2e7a`
9. **Task 1: two-shell lifecycle and authority/Git aggregates** - `4c59463`
10. **Task 2: 21 probes, fault/public aggregates, and performance sampler** - `c326ac3`
11. **Verification repair: exact private cleanup allowlist** - `f616c7b`

## Files Created/Modified

- `core/shell/zsh/worktree_live_test.go` - Built-binary two-retained-shell E2E harness and observable lifecycle proof.
- `scripts/perf-worktree.sh` - 50-cycle production sampler and fake/real deadline runner.
- `core/shell/zsh/hook.go` - Deterministic capture filtering, retained worktree deactivate, exact config cache updates, and truthful mutation rc propagation.
- `core/shell/zsh/introspect.go` - Model-compatible and reserved-symbol capture filtering before body/value reads.
- `core/worktree/registry.go` - Defense-in-depth exclusion for exact loader-reserved bookkeeping prefixes.
- `core/worktree/state.go` - Canonical defensive-copy snapshot fingerprints.
- `core/worktree/service.go` - Canonical status/resolve comparisons and current-capture pull patch generation.
- `core/worktree/service_test.go` - Canonical acknowledgement/resolution coverage and exact 21-probe contract.
- `core/worktree/atomic_unix_test.go` - Descriptor ownership and security fault aggregates.
- `core/cli/worktree_test.go` - Exact public/prohibited surface aggregate.
- `core/cmd/zsh-pro/main_test.go` - Canonical public/runtime authority aggregate and exact loader surface.
- `core/store/store_test.go` - Exact real-Git and DTO workflow aggregate.

## Decisions Made

- Canonicalize only top-level identity-record order for fingerprints; validation and every value/presence/list byte remain authoritative.
- Treat `_zp_` and `__zp_` as loader-reserved capture/admission prefixes without broadening the exclusion to ordinary underscore-prefixed user symbols.
- Derive user-visible behind truth from durable revision plus canonical baseline state, not a possibly stale cached boolean alone.
- Generate apply patches from the current captured parent state; retain applied state strictly for causal revision/acknowledgement history.
- Measure actual performance without inventing a comparison baseline or tighter latency SLA.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Filtered unsupported live symbol names before capture**
- **Found during:** Task 1 installed-loader attach
- **Issue:** zle functions such as `azhw:zle-history-line-set` entered a frame that the closed model identity grammar correctly rejected, aborting attach.
- **Fix:** Applied the same model-compatible name guard to both capture paths before any body/value read.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/introspect.go`, `core/shell/zsh/introspect_test.go`
- **Committed in:** `548f35a`

**2. [Rule 2 - Missing Critical] Excluded loader bookkeeping from live admission**
- **Found during:** Task 1 post-apply fresh capture
- **Issue:** Dynamic `__zp_worktree_reverse_*` functions were absent at attach and therefore appeared admissible, preventing acknowledgement.
- **Fix:** Capture now skips exact `_zp_`/`__zp_` prefixes and Registry rejects the same reserved bookkeeping defense-in-depth.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/introspect.go`, `core/worktree/registry.go`
- **Committed in:** `d72bc7f`

**3. [Rule 1 - Bug] Made public deactivate consume retained worktree state**
- **Found during:** Task 1 real deactivate
- **Issue:** A valid worktree reverse function without legacy `ZP_ACTIVE_PROFILE` made public deactivate a no-op and allowed immediate reattachment.
- **Fix:** Worktree-only deactivate executes the protected reverse once, removes hooks/functions, and clears private attachment metadata idempotently.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`
- **Committed in:** `a4594fb`

**4. [Rule 1 - Bug] Canonicalized identity-record fingerprints**
- **Found during:** Task 1 cross-shell alias acknowledgement
- **Issue:** Shared append order and real zsh enumeration order differed despite identical identity/value state.
- **Fix:** Fingerprints sort a normalized defensive copy by exact kind/name while preserving ordered list payloads.
- **Files modified:** `core/worktree/state.go`, `core/worktree/state_test.go`, `core/worktree/service_test.go`
- **Committed in:** `68c6f02`

**5. [Rule 1 - Bug] Refreshed retained auto-apply defaults immediately**
- **Found during:** Task 1 persisted manual-mode flow
- **Issue:** Successful `config set auto-apply false` persisted the default but the same shell kept a stale `true` cache until an unrelated status call.
- **Fix:** The exact config dispatcher updates its retained cache only after exit 0 and preserves explicit override precedence.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`
- **Committed in:** `9f78c32`

**6. [Rule 1 - Bug] Derived passive behind status and canonical resolve freshness**
- **Found during:** Task 1 manual receiver and same-key resolution
- **Issue:** A passive receiver displayed `behind: false`, and ResolveShared rejected an identical fresh capture solely because record order differed.
- **Fix:** Status derives canonical durable convergence; ResolveShared compares validated canonical fingerprints without mutating state order.
- **Files modified:** `core/worktree/service.go`, `core/worktree/service_test.go`
- **Committed in:** `9dcafee`

**7. [Rule 1 - Bug] Preserved checkout/reset validation and delegated failures**
- **Found during:** Task 1 exact negative arities and dirty checkout
- **Issue:** The sourced dispatcher returned 0 after a failed delegated command because the trailing false `if` status was lost; incomplete `checkout -b` was not rejected locally.
- **Fix:** Exact arities are validated before helper work, the delegated rc is captured immediately, and pull runs only after rc 0.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`
- **Committed in:** `3db5d70`

**8. [Rule 1 - Bug] Generated net-zero reset patches from current capture**
- **Found during:** Task 1 reset parent convergence
- **Issue:** Publish then reset returned Shared to the old applied value, yielding an empty AppliedBaseline diff while the real parent still held the dirty alias; fresh acknowledgement failed.
- **Fix:** PreparePull patches current CaptureBaseline to Shared while keeping AppliedRevision/Baseline as causal history authority.
- **Files modified:** `core/worktree/service.go`, `core/worktree/service_test.go`
- **Committed in:** `a9c2e7a`

**9. [Rule 3 - Blocking] Updated the exact loader symbol allowlist**
- **Found during:** Full `go test ./...`
- **Issue:** The private `_zp_worktree_disable` helper introduced by the deactivate repair was intentionally installed but absent from the exact allowlist test.
- **Fix:** Added only that private helper to the expected installed surface.
- **Files modified:** `core/cmd/zsh-pro/main_test.go`
- **Committed in:** `f616c7b`

---

**Total deviations:** 9 auto-fixed (8 Rule 1 correctness bugs, 1 Rule 3 verification blocker).
**Impact on plan:** All repairs were required by the plan's real production E2E contract and remained within explicitly authorized files; no public command, persistence schema, dependency, or product scope was added.

## Issues Encountered

- The performance fixture initially inherited ambient environment state; switching its retained zsh process to `env -i` matched the production E2E isolation contract and produced stable, value-safe measurements.
- The legacy milestone `REQUIREMENTS.md` does not declare the Plan 07 `WORK-*`/`SYNC-*` identifiers and `ROADMAP.md` still ends at Phase 6; the required SDK handlers therefore reported those IDs/phase row absent and made no speculative edits. Completion coverage is recorded in this plan summary and canonical `STATE.md` is at 100%/ready for verification.
- No authentication gates or external service blockers occurred.

## Known Stubs

None.

## Verification

- Exact Task 1 and Task 2 plan commands: passed.
- `GOTOOLCHAIN=local go test -count=1 ./...`: passed via final `make check` after the allowlist correction.
- `GOTOOLCHAIN=local go test -race ./core/worktree ./core/store -count=1`: passed.
- `GOTOOLCHAIN=local go vet ./...`: passed.
- `GOTOOLCHAIN=local go build ./...`: passed.
- `make check`: passed with zero golangci-lint issues.
- Stub, credential-shape, broadened-Git-surface, artifact, executable, exact-probe-count, diff, and threat-control-anchor scans: passed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 07-10 is fully automated and ready for Phase 7 verification/milestone closure.
- No blockers remain; only pre-existing user-owned untracked paths remain untouched.

## Self-Check: PASSED

- All declared created/modified artifacts exist.
- All 11 implementation, repair, and task commits are present.
- Summary status, completed requirements, executable sampler, and exact 21-row probe count are verified on disk.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-17*
