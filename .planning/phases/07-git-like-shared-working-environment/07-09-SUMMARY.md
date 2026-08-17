---
phase: 07-git-like-shared-working-environment
plan: 09
subsystem: shell-runtime
tags: [zsh, worktree, capability, stdin-framing, deadlines, conflict-resolution]
requires:
  - phase: 07-03
    provides: credential-authenticated durable worktree Service and conflict state
  - phase: 07-07
    provides: canonical descriptor-bound production authority and policy composition
  - phase: 07-08
    provides: private live transition payloads and durable uint64-to-opaque acknowledgement binding
provides:
  - five-operation bounded ZPWT runtime transport with stdin-only shell capabilities
  - post-auth cryptographic shell identity and capability allocation
  - zero-process lazy sourced-loader attachment and safe-boundary convergence
  - exact public dispatcher with publish-before-mutation and explicit shared resolution
  - one sealed 250 ms runtime and parent-shell transition budget
affects: [shared-worktree-sync, sourced-loader, conflict-recovery, runtime-security]
tech-stack:
  added: []
  patterns:
    - versioned length-framed stdin protocol with exact byte and record caps
    - operation-scoped descriptor binding after authentication and frame validation
    - emitter-owned reply globals followed by fresh capture and idempotent acknowledgement
    - cooperative publish-only precmd and pull-only line-finish hooks
    - value-free bounded conflict metadata for explicit durable resolution
key-files:
  created: []
  modified:
    - core/cli/runtime.go
    - core/cli/runtime_test.go
    - core/shell/zsh/hook.go
    - core/shell/zsh/hook_test.go
    - core/cmd/zsh-pro/main.go
    - core/cmd/zsh-pro/main_test.go
key-decisions:
  - "Carry every private credential and operation field in one exact ZPWT v1 stdin frame; argv retains only the five operation names and bounded compatibility timeout."
  - "Return only bounded value-free conflict identity and durable token metadata from Publish so explicit shared resolution can address the Service's pending conflict without exposing captured values or capabilities."
  - "Start one absolute 250 ms parent budget before prepare or resolve and retain it through protected eval, reverse replacement, fresh capture, and acknowledgement."
  - "Keep legacy fail-open behavior only for an explicitly unsupported pre-worktree helper, an already-active legacy shell, or a deliberately replaced dispatcher; the installed shared dispatcher remains the normal route."
metrics:
  duration: 65m
  completed: 2026-08-17
  tasks: 2
  files: 6
status: complete
---

# Phase 7 Plan 9: Bounded Shared Worktree Loader Summary

**Independent zsh terminals now attach lazily and converge through an authenticated, stdin-only five-operation protocol with exact conflict resolution, fresh-state acknowledgement, and a sealed 250 ms fail-open budget.**

## Performance

- **Duration:** 65 minutes
- **Started:** 2026-08-17T14:56:08Z
- **Completed:** 2026-08-17T16:00:40Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added exactly five private runtime operations—attach, publish, prepare, acknowledge, and resolve—with the existing 1–99 second outer syntax but a non-extendable internal 250 ms transition limit.
- Added the `ZPWT` version-1 stdin envelope capped at exactly 2,101,248 bytes and 10,016 records. The decoder rejects unknown, duplicate, out-of-order, truncated, overlong, noncanonical, early-final, and trailing input before late descriptor binding.
- Authenticated the runtime root before allocating independent cryptographically random 256-bit shell IDs and capabilities. Later calls carry the pair only through stdin, never argv, environment, path, or fallback channels.
- Added exact bounded attach, publish, and acknowledge replies plus whole-buffer zsh-n validation for prepare/resolve. Pending transitions end in the emitter-owned protocol/revision/token/fingerprint/complete assignments; at-head no-op remains exact zero stdout.
- Installed a zero-subprocess sourced loader with retained lazy no-delta attachment, publish-only precmd, pull-only line-finish, explicit sync/resolution, pre-mutation publication for checkout/reset, fresh post-apply capture, and exact acknowledgement.
- Added process inspection, replay rejection, xtrace/history cleanup, frame boundary, fake-clock 249/251 ms, real 25/500 ms, malformed reply, metadata-only, no-op, and real-zsh lifecycle coverage.

## Task Commits

Each task used RED/GREEN TDD commits with normal repository hooks:

1. **Task 1: Freeze five bounded credential-forwarding runtime operations and exact reply output**
   - `e821101` — `test(07-09): define bounded runtime worktree transport`
   - `30b4523` — `feat(07-09): add bounded runtime worktree transport`
2. **Task 2: Attach lazily, route exact public commands, and converge at safe boundaries**
   - `24bbff5` — `test(07-09): define lazy shared loader contract`
   - `270dfdf` — `feat(07-09): install bounded shared worktree loader`
   - `9bdd9c8` — `fix(07-09): preserve legacy loader fail-open boundaries`

## Files Created/Modified

- `core/cli/runtime.go` — Exact private allowlist, sealed budget, descriptor authentication/binding order, credential allocation, strict frame decoder, result encoders, and whole-program validation/output.
- `core/cli/runtime_test.go` — Exact frame byte/record boundaries, malformed/trailing rejection, auth/bind/close ordering, credential allocation/forwarding, reply output, fake clock, real deadline, and deadline-aware stdin coverage.
- `core/shell/zsh/hook.go` — Lazy semantic attachment, builtin frame writer, protected helper lifecycle, parent transition pipeline, public dispatcher, hooks, conflict resolution, and fail-open cleanup.
- `core/shell/zsh/hook_test.go` — Real-zsh lifecycle, zero-process source, frame inspection, capability secrecy, process inspection, fake/real deadline, reply cleanup, and exact hook/dispatcher contracts.
- `core/cmd/zsh-pro/main.go` — Production `RuntimeWorktreeFactory` binding over the authenticated runtime root and existing canonical authority.
- `core/cmd/zsh-pro/main_test.go` — Factory composition and exact installed symbol allowlist coverage.

## Decisions Made

- The private helper owns no public path or authority fallback. It authenticates the retained runtime root, decodes and validates one frame, binds one operation-scoped adapter, and closes that adapter exactly once.
- The stable shell ID and capability are the only long-lived private pair. Per-call capability copies, operation IDs, tokens, source, response buffers, descriptors, xtrace state, and private history context are cleared or restored on every exit.
- Publish responses expose only conflict kind, identity kind/name, and durable conflict token. They do not expose the capability, captured value, patch source, object identifier, raw error, or other secret-like material.
- A nonempty prepare/resolve response is applied only after exact syntax validation; acknowledgement happens only after canonical reply parsing and a fresh semantic capture. Lost acknowledgement responses remain safe through stable operation IDs and the durable 07-08 token binding.
- Source-time behavior is definition and cooperative registration only. No helper, Git command, capture process, attach, publish, or command substitution runs while sourcing.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Added a bounded value-free Publish result channel**

- **Found during:** Task 2 explicit-resolution integration
- **Issue:** The planned runtime interface returned no parent-visible Publish result, so the sourced parent could not retain the durable conflict identity/token required by `sync --resolve shared` after the helper exited.
- **Fix:** Added exact `ZPWP 1 <shared-revision> <count>` framing with bounded ordered conflict kind, identity kind/name, and durable token records. Validation rejects malformed, oversized, incomplete, or trailing responses; no capability, captured value, patch source, object ID, or raw Service error is returned.
- **Files modified:** `core/cli/runtime.go`, `core/cli/runtime_test.go`, `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`
- **Commit:** `270dfdf`

**2. [Rule 1 - Bug] Preserved prior loader fail-open behavior without weakening normal shared routing**

- **Found during:** Task 2 full real-zsh regression suite
- **Issue:** Replacing the Phase 5 public wrappers made a failed/sleeping `list` leak partial output or block, made an unattached legacy `status` consult an unsupported helper, and mistook established runtime-helper dispatcher overrides or already-active legacy state for the installed shared dispatcher.
- **Fix:** Restored bounded all-or-none `list`, local unattached legacy `status`, an unsupported-helper latch, and compatibility routing for an explicitly replaced dispatcher or already-active legacy shell. Normal newly sourced operation still uses the exact shared dispatcher and private protocol.
- **Files modified:** `core/shell/zsh/hook.go`, `core/cmd/zsh-pro/main_test.go`
- **Commit:** `9bdd9c8`

## TDD Gate Compliance

- RED gates: `e821101`, `24bbff5`
- GREEN gates: `30b4523`, `270dfdf`
- Regression correction: `9bdd9c8`
- Normal pre-commit hooks remained enabled for all five commits and reported zero lint issues.

## Verification

- Both plan-scoped exact test commands — passed
- `GOTOOLCHAIN=local go test ./core/worktree ./core/cli ./core/shell/zsh ./core/cmd/zsh-pro -count=1` — passed
- `GOTOOLCHAIN=local go test ./... -count=1` — passed
- `GOTOOLCHAIN=local go test -race ./... -count=1` — passed
- `GOTOOLCHAIN=local go vet ./...` — passed
- `golangci-lint run ./...` — passed with 0 issues
- `GOTOOLCHAIN=local go build ./...` — passed
- `git diff --check` — passed
- AST/symbol allowlist, frame-token, stub, secret, and threat-surface scans — passed
- Real-zsh source, lifecycle, xtrace, `/proc` process inspection, 25/500 ms deadline, legacy fail-open, descriptor-race, and next-command survival cases — passed

## Known Stubs

None. Modified production files contain no TODO, FIXME, placeholder, coming-soon, not-implemented, panic, or empty hardcoded result path. Placeholder matches in `main_test.go` are pre-existing assertions for reviewed fixture substitution.

## Auth Gates

None.

## Tracking Notes

- `state.advance-plan` advanced the sequential counter from plan 9 to plan 10 of 10; Plan 07-10 remains the final unexecuted plan, so Phase 07 stays in progress.
- `roadmap.update-plan-progress` found nine Phase 07 summaries and reported 9/10 in progress. The repository's current `ROADMAP.md` has no Phase 07 row, so the handler made no file change.
- The plan declares WORK-02, SYNC-01, and SYNC-02, but those identifiers do not exist in the current `REQUIREMENTS.md`; the required mark-complete call reported all three as `not_found` and made no file change. No substitute identifiers were invented.

## Threat Model Coverage

- **T-07-35:** A sealed runtime budget begins before authentication/input/binding/service/output; the parent absolute deadline spans prepare/resolve through eval, reverse ownership, fresh capture, and acknowledgement. Fake 249/251 ms and real 25/500 ms tests prove admission and fail-open termination.
- **T-07-36 / T-07-40C:** Capabilities and source stay out of argv, environment, public output, xtrace, history, and value-bearing errors. Bounded responses and always-block cleanup prevent retained call-local disclosure.
- **T-07-37:** Source starts no process, retains the first semantic snapshot, completes one no-delta attach, and refuses publication before attachment.
- **T-07-38:** Whole-buffer zsh-n validation, exact five reply assignments, protected eval, canonical reply validation, fresh capture, durable-token recovery, and idempotent acknowledgement prevent false convergence.
- **T-07-39:** Only precmd publishes and only line-finish pulls; no PWD, jobs, command buffer, process tree, history, or terminal UI state is synchronized.
- **T-07-40 / T-07-40A / T-07-40B:** Exact public/private arity, authenticated descriptor binding, independent random credentials, stdin-only forwarding, replay rejection, and publish-before-checkout/reset prevent spoofing and loss of unpublished local state.

No new unplanned network endpoint, schema, dependency, or external service was introduced. Runtime-root file access and authenticated private transport are the planned trust-boundary changes.

## Next Phase Readiness

The shared worktree lifecycle is installed end to end. Independent terminals can attach, publish, pull, explicitly synchronize or resolve, verify fresh parent state, and acknowledge without exposing the shell capability or blocking the next prompt beyond the sealed budget.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-17*

## Self-Check: PASSED

All six modified implementation/test files and this summary exist; all five task/deviation commits are present in repository history; required frontmatter marks the plan complete; full tests, race, vet, lint, build, and contract scans passed.
