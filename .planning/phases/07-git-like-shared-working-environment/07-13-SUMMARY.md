---
phase: 07-git-like-shared-working-environment
plan: 13
subsystem: shell-runtime
tags: [zsh, deadlines, coprocess, backpressure, process-cleanup, fail-open]

requires:
  - phase: 07-10
    provides: real-zsh worktree lifecycle, exact deadline seams, and prompt sampler
provides:
  - Incremental 10,000-record and 2 MiB capture/frame admission under one absolute 250 ms deadline
  - Cancellable private-frame delivery with owned TERM/KILL escalation and bounded reap
  - Repeatable real-zsh oversize, backpressure, and TERM-ignore fail-open evidence
affects: [07-14, 07-15, phase-07-verification, shell-runtime-security]

tech-stack:
  added: []
  patterns:
    - One caller-owned absolute deadline propagated through capture, serialization, transport, and cleanup
    - Tracked helper/writer/closer ownership with a final 25 ms termination reserve

key-files:
  created: []
  modified:
    - core/shell/zsh/hook.go
    - core/shell/zsh/hook_test.go
    - core/shell/zsh/worktree_live_test.go
    - scripts/perf-worktree.sh
    - core/cmd/zsh-pro/main_test.go

key-decisions:
  - "Reserve the final 25 ms of the original deadline for shutdown, with KILL beginning 10 ms before expiry and signals limited to verified direct children."
  - "Use a 120,000-byte backpressure fixture: larger than the pipe capacity but below Linux's per-environment-string exec limit, while retaining a separate 2 MiB-plus-one oversize case."
  - "An explicit capture failure retains a value-free visible error while prompt hooks remain fail-open and the applied revision stays unchanged."

patterns-established:
  - "Incremental admission: validate deadline, count, and bytes before retaining or writing each live-state record."
  - "Bounded transport: duplicate private coprocess endpoints, close inherited magic endpoints, background the writer, and reap every owned PID before return."

requirements-completed: [WORK-02, SYNC-01, SYNC-02]

coverage:
  - id: D1
    description: "Capture and serialization obey exact record/byte limits and the original cumulative 250 ms deadline."
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/shell/zsh/hook_test.go#TestWorktreeCaptureAndFrameUseOneIncrementalDeadline"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/hook_test.go#TestWorktreeTransitionFakeClockCumulative249And251"
        status: pass
    human_judgment: false
  - id: D2
    description: "Blocked writers and TERM-ignoring helpers are cancelled, KILLed when necessary, reaped, and leave the same zsh usable."
    requirement: SYNC-02
    verification:
      - kind: e2e
        ref: "core/shell/zsh/worktree_live_test.go#TestWorktreeAbsoluteDeadlineAdversarialTransport"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -timeout=60s"
        status: pass
    human_judgment: false
  - id: D3
    description: "The production prompt sampler retains ordinary latency/process metrics and reports repeated value-free adversarial cleanup evidence."
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "fresh production binary plus scripts/perf-worktree.sh --cycles 50"
        status: pass
    human_judgment: false

duration: 67min
completed: 2026-08-18
status: complete
---

# Phase 07 Plan 13: Absolute Worktree Lifecycle Bound Summary

**Incremental capture, cancellable frame delivery, and owned TERM/KILL reap now complete inside one absolute 250 ms shell-operation lifecycle.**

## Performance

- **Duration:** 67 min
- **Started:** 2026-08-18T01:29:42Z
- **Completed:** 2026-08-18T02:36:24Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- Capture and frame serialization now enforce the unchanged entry deadline, 10,000-record cap, 2 MiB snapshot cap, and 2,101,248-byte private-frame cap incrementally before retention or write.
- Frame delivery uses a tracked background writer; timeout cleanup closes descriptors, verifies direct-child ownership, escalates TERM to KILL, and reaps writer/helper/closer without an unbounded wait.
- Real retained `zsh -f` tests prove oversized capture, blocked stdin, and TERM-ignore failures keep the applied revision unchanged, expose explicit behind/error state, leak no children or pipe FDs, and execute the next-command sentinel.
- The sampler repeats all three adversarial modes twice while retaining ordinary p50/p95/max metrics and zero Git/child-zsh hot-path counts.

## Task Commits

Each task was committed atomically with its TDD gate:

1. **Task 1 RED: incremental deadline/cap contract** - `aa8baa3`
2. **Task 1 GREEN: bounded capture and frame serialization** - `7006033`
3. **Task 2 RED: adversarial transport deadline contract** - `a24ec8e`
4. **Task 2 GREEN: cancellable transport and bounded reap** - `080fbde`
5. **Task 3 RED: adversarial driver reporting contract** - `86ecd7c`
6. **Task 3 GREEN: retained adversarial sampler** - `5253cab`
7. **Integration deviation: exact installed-symbol allowlist** - `c46eb00`

## Files Created/Modified

- `core/shell/zsh/hook.go` - Incremental capture/frame admission, deadline propagation, cancellable delivery, visible capture errors, and owned process cleanup.
- `core/shell/zsh/hook_test.go` - Exact fake 249/251 cumulative boundaries and count/byte plus-one contract.
- `core/shell/zsh/worktree_live_test.go` - Retained real-zsh oversize, blocked-reader, TERM-ignore, cleanup, fail-open, and report evidence.
- `scripts/perf-worktree.sh` - Repeated adversarial report validation plus preserved ordinary prompt/process metrics.
- `core/cmd/zsh-pro/main_test.go` - Narrow exact allowlist for the new private loader helpers.

## Decisions Made

- The original safe-boundary deadline remains the only parent clock; stages never restart it. The last 25 ms is cleanup reserve and the final 10 ms is reserved for KILL/reap.
- Coprocess signals target only retained PIDs still verified as direct children; no process-group signal or broad process matching is used.
- The backpressure payload is 120,000 bytes so it fills the pipe without tripping Linux's single-environment-string exec ceiling; the independent 2 MiB-plus-one case proves oversize rejection.
- Explicit capture failure is visible through a constant value-free error, while prompt hooks still return control and never acknowledge the failed state.

## Validation

- `GOTOOLCHAIN=local go test -count=1 ./... -timeout=180s` — pass.
- `GOTOOLCHAIN=local go test -race ./core/worktree ./core/store -count=1 -timeout=120s` — pass.
- `GOTOOLCHAIN=local go vet ./...` — pass.
- `GOTOOLCHAIN=local go build ./...` — pass.
- `GOTOOLCHAIN=local make check` — pass, including golangci-lint with zero issues.
- Fresh production binary plus `scripts/perf-worktree.sh --cycles 50` — pass: count 50, p50 64.098120 ms, p95 87.690830 ms, max 156.184197 ms, Git calls 0, child-zsh calls 0.
- Repeated adversarial report — pass: every mode reports survivors 0, pipe delta 0, applied revision unchanged, visible error/behind, and continuation 1.

## TDD Gate Compliance

All three tasks have a failing `test(07-13)` commit before their corresponding passing `feat(07-13)` commit.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Avoided zsh's readonly `status` special parameter**
- **Found during:** Task 2 GREEN
- **Issue:** A local named `status` aborted the reap helper and prevented PID cleanup.
- **Fix:** Renamed the local to `child_status` and retained exact wait status propagation.
- **Files modified:** `core/shell/zsh/hook.go`
- **Verification:** All three adversarial cases and the full shell package pass.
- **Committed in:** `080fbde`

**2. [Rule 1 - Bug] Preserved unsupported-helper exit 64 during writer SIGPIPE**
- **Found during:** Task 2 full shell-package verification
- **Issue:** An early helper exit could make the still-scheduled writer appear live, converting the compatibility exit into timeout 124.
- **Fix:** Bounded normal completion polls all tracked children to the termination deadline and gives the helper result precedence over the expected writer SIGPIPE.
- **Files modified:** `core/shell/zsh/hook.go`
- **Verification:** Legacy live-terminal tests and the full `core/shell/zsh` package pass.
- **Committed in:** `080fbde`

**3. [Rule 3 - Blocking] Corrected the backpressure fixture below Linux exec's per-string limit**
- **Found during:** Task 2 GREEN
- **Issue:** A 512 KiB exported test value made the kernel reject helper execution before the pipe test began.
- **Fix:** Used 120,000 bytes, still larger than pipe capacity; retained the separate 2 MiB-plus-one oversize case.
- **Files modified:** `core/shell/zsh/worktree_live_test.go`
- **Verification:** Fake helper starts, backpressure occurs, and cleanup assertions pass repeatedly.
- **Committed in:** `080fbde`

**4. [Rule 2 - Missing Critical] Retained visible explicit failure for oversized capture**
- **Found during:** Task 3 report hardening
- **Issue:** Oversize failed safely but left `ZP_WORKTREE_LAST_ERROR` empty, hiding the explicit sync failure state.
- **Fix:** Set constant `worktree capture failed` without including captured values.
- **Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/worktree_live_test.go`
- **Verification:** Oversize now proves nonzero explicit return, unchanged revision, nonempty error, and prompt continuation.
- **Committed in:** `5253cab`

**5. [Rule 3 - Blocking] Updated the exact installed-loader symbol allowlist**
- **Found during:** Final full repository gate
- **Issue:** The out-of-plan integration oracle rejected the plan-required new private loader helpers.
- **Fix:** With orchestrator authorization, added only the required capture/transport helper symbols; no public surface changed.
- **Files modified:** `core/cmd/zsh-pro/main_test.go`
- **Verification:** `TestMainIngestAllowsOnlyExactLoaderSymbols`, `make check`, and the uncached full suite pass.
- **Committed in:** `c46eb00`

---

**Total deviations:** 5 auto-fixed (2 Rule 1 bugs, 1 Rule 2 missing critical behavior, 2 Rule 3 blockers).
**Impact on plan:** Every deviation was required to make the planned timing/security assertions non-vacuous or restore an exact integration gate; no product scope or public command surface expanded.

## Issues Encountered

- Zsh does not expose ordinary variables for its magic coprocess endpoints. The implementation duplicates the private transport endpoints, starts an inert replacement, identifies only newly owned pipe descriptors, closes them, and proves the parent returns to its baseline pipe count.
- The exact full-suite loader allowlist was outside the initial file list; work paused for narrow orchestrator authorization before that integration-only test was updated.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 07-14/07-15 and final Phase 07 verification can rely on executable whole-lifecycle timing and cleanup evidence.
- No blockers remain. Unrelated pre-existing untracked planning/graph paths were not touched.

## Self-Check: PASSED

- All five modified files exist.
- All seven task/integration commits exist.
- Full, race, vet, lint/check, build, focused real-zsh, and 50-cycle sampler gates pass.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-18*
