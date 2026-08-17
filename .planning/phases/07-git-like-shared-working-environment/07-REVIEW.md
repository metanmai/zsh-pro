---
phase: 07-git-like-shared-working-environment
reviewed: 2026-08-17T17:19:30Z
depth: standard
files_reviewed: 48
files_reviewed_list:
  - core/activate/builder.go
  - core/activate/diff.go
  - core/activate/live_patch_test.go
  - core/activate/plan.go
  - core/cli/cli.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/ingest.go
  - core/cli/ingest_test.go
  - core/cli/runtime.go
  - core/cli/runtime_test.go
  - core/cli/store.go
  - core/cli/worktree.go
  - core/cli/worktree_test.go
  - core/cmd/zsh-pro/main.go
  - core/cmd/zsh-pro/main_test.go
  - core/model/ingest_transaction.go
  - core/model/worktree.go
  - core/model/worktree_test.go
  - core/shell/provider.go
  - core/shell/zsh/classify.go
  - core/shell/zsh/classify_test.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/introspect.go
  - core/shell/zsh/introspect_test.go
  - core/shell/zsh/regen.go
  - core/shell/zsh/regen_test.go
  - core/shell/zsh/worktree_live_test.go
  - core/store/dto.go
  - core/store/dto_test.go
  - core/store/git.go
  - core/store/git_test.go
  - core/store/store.go
  - core/store/store_test.go
  - core/worktree/atomic_unix.go
  - core/worktree/atomic_unix_test.go
  - core/worktree/diff.go
  - core/worktree/diff_test.go
  - core/worktree/registry.go
  - core/worktree/registry_test.go
  - core/worktree/service.go
  - core/worktree/service_test.go
  - core/worktree/state.go
  - core/worktree/state_test.go
  - scripts/perf-worktree.sh
findings:
  critical: 4
  warning: 1
  info: 0
  total: 5
status: issues_found
---

# Phase 07: Code Review Report

**Reviewed:** 2026-08-17T17:19:30Z
**Depth:** standard
**Files Reviewed:** 48
**Status:** issues_found

## Summary

The Phase 7 implementation has five confirmed defects. Four affect required shared-worktree correctness: admitted identities are not durable global ownership, prompt hooks are not actually bounded by the 250 ms budget, failed live patches can leave mutations without usable recovery ownership, and the first explicit `sync` can report success without synchronizing. A fifth defect loses the promised recovered-persistence marker whenever the normal one-command runtime closes its `StateStore`.

The complete repository test suite, relevant race suites, vet, and shell syntax check pass, but the current tests do not exercise the failing late-attach, oversized-capture, partial-worktree-apply, first-call sync, or close/reopen recovery paths described below.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: A globally shared admitted identity becomes permanently unacknowledgeable for later shells

**Classification:** BLOCKER

**Files:** `core/worktree/registry.go:111-124`, `core/worktree/registry.go:197-218`, `core/worktree/service.go:398-425`, `core/worktree/service.go:699-726`, `core/worktree/service.go:1052-1068`

**Issue:** `Registry.Admit` adds a safe post-attach identity only to the current in-memory `owned` map. Every runtime operation constructs a fresh registry, and each service path calls `Seed(state.Committed.Source)`, which replaces that map with source-owned identities only. The admitted identity is persisted in `state.Shared`, but its ownership is not. If shell A publishes a new alias and shell C attaches afterward while that alias is already present, C records it as `AdmissionPresentAtAttach` instead of managed state. Pull can still apply the shared alias, but acknowledgement filters it out again in `admittedSnapshot`; its fingerprint can never match the pending fingerprint of `state.Shared`. The shell remains behind with an unacknowledgeable pending transition.

**Fix:** Make canonical shared ownership durable. Seed the registry from the committed source plus the exact non-secret identities already present in canonical `state.Shared` (or persist a separate value-free admitted-identity set) before attach, publish, resolve, and acknowledge. Preserve pinned/live-secret exclusions. Add an integration test in which A publishes a new identity, C starts with that identity already present, then C attaches, pulls, and acknowledges the exact shared revision.

### CR-02: Capture, frame writing, and child reaping can exceed the 250 ms prompt-hook budget

**Classification:** BLOCKER

**File:** `core/shell/zsh/hook.go:541-608`, `core/shell/zsh/hook.go:701-739`

**Issue:** `_zp_worktree_capture` walks and copies every exported scalar, alias, function, path element, and option without checking the absolute deadline or enforcing the 10,000-record/2 MiB snapshot limits. `_zp_worktree_write_snapshot_records` repeats that work and performs synchronous writes without deadline checks. `_zp_worktree_invoke` does not begin its timed read until the whole frame has been written, and both its error/timeout path and `always` cleanup use unbounded `wait`. Consequently a large shell or a helper that stops consuming stdin/ignores `TERM` can block `precmd`, `line-finish`, or explicit sync well beyond 250 ms even though the Go decoder later rejects the oversized frame.

**Fix:** Enforce record and byte caps incrementally during capture, pass the absolute deadline into capture/serialization, and check it inside every enumeration and write loop. Make frame delivery cancellable rather than synchronously waiting on a potentially full pipe. After timeout, use a bounded termination/reap sequence and never perform an unbounded `wait` on the interactive path. Add real-zsh tests with more than 10,000 records, more than 2 MiB of function/value data, a helper that stops reading stdin, and a helper that ignores `TERM`; each hook must return within the configured bound and leave the next command usable.

### CR-03: A failed live patch can be masked or lose the reverse needed to recover partial shell mutations

**Classification:** BLOCKER

**Files:** `core/shell/zsh/emit.go:168-178`, `core/shell/zsh/hook.go:903-915`

**Issue:** `EmitLivePatch` concatenates mutation commands into a function without fail-fast guards. In zsh, an intermediate failure such as assigning a readonly exported parameter is masked if a later mutation succeeds. If the final mutation fails, `_zp_worktree_apply_transition` returns before installing `reverse_name` in any active/recovery pointer. Either path can leave earlier operations applied; in the latter path the generated reverse exists only under an internal name that public recovery does not own, while `old_reverse` cannot reverse the new partial patch. The service correctly refuses the later acknowledgement, but the parent shell has already been mutated and may not have a usable recovery route.

**Fix:** Emit every forward mutation with explicit fail-fast status propagation. Before invoking the apply function, install the replacement reverse in a dedicated recovery pointer using the same ownership discipline as the existing profile payload path. On any apply, deadline, capture, or acknowledgement failure, keep that recovery pointer and report it; promote it to `ZP_ACTIVE_REVERSE_FN` only after successful application/verification, and remove the old reverse only after promotion succeeds. Add real-zsh tests where an early and a final operation fail after at least one successful mutation, verifying nonzero status, no masked success, and an idempotent public recovery path.

### CR-04: The first explicit sync on an unattached shell reports success without reconciling

**Classification:** BLOCKER

**Files:** `core/shell/zsh/hook.go:771-822`, `core/shell/zsh/hook.go:999-1012`, `core/worktree/service.go:417-425`

**Issue:** A mismatched attach is durably recorded as `AttachStateCleanReconcile` with `AppliedRevision == 0`, but the attach reply exposes only the head revision and the shell stores it as `ZP_WORKTREE_APPLIED_REVISION`. `_zp_worktree_sync` then returns 0 immediately whenever attachment happened in that call. In a freshly sourced non-interactive shell, `zsh-pro sync` can therefore claim success while none of the shared state was applied. A subsequent publication also uses the false head revision and fails with `ErrNeedsReconcile` until a later pull repairs it.

**Fix:** Do not treat attachment as synchronization. Return enough attach metadata to distinguish at-head from clean-reconcile, or have first-call sync immediately run the prepare/apply/fresh-capture/acknowledge path and skip publication until that succeeds. Add a real-zsh test that sources the loader and invokes exactly one `zsh-pro sync` without preceding hooks; on exit 0, all shared identities and the applied revision must already match canonical state.

## Warnings

### WR-01: Forensic append failures lose their recovery marker when the operation-scoped store closes

**Classification:** WARNING

**Files:** `core/worktree/atomic_unix.go:65-68`, `core/worktree/atomic_unix.go:195-200`, `core/worktree/atomic_unix.go:229-232`, `core/cli/emitter.go:106-131`

**Issue:** After canonical replacement succeeds, an `events.jsonl` append failure sets only `StateStore.pendingRecoveredPersistence`. The runtime factory creates and closes a fresh `StateStore` for each helper operation, so that boolean disappears before any later transaction can project `RecoveredPersistence` into durable shell state. The existing test reuses one store instance and therefore does not represent the production close/reopen lifecycle. Operators receive neither the promised next-transaction marker nor durable evidence that the forensic append failed.

**Fix:** Persist the pending marker descriptor-relatively (for example, as a fixed-name, value-free recovery flag) or perform a second atomic canonical update that records `RecoveredPersistence` without rolling back the successful transition. Add a test that injects an append failure, closes the store, reopens it from a newly authenticated descriptor, performs the next transaction, and verifies that the recovery marker is surfaced and then cleared according to the chosen lifecycle.

## Verification Performed

- `GOTOOLCHAIN=local go test -count=1 ./...` — passed
- `GOTOOLCHAIN=local go test -race -count=1 ./core/worktree ./core/store ./core/cli ./core/shell/zsh` — passed
- `GOTOOLCHAIN=local go vet ./...` — passed
- `bash -n scripts/perf-worktree.sh` — passed

---

_Reviewed: 2026-08-17T17:19:30Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
