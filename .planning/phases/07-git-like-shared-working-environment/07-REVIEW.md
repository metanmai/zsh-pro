---
phase: 07-git-like-shared-working-environment
reviewed: 2026-08-18T03:56:49Z
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
  critical: 5
  warning: 1
  info: 0
  total: 6
status: issues_found
---

# Phase 07: Code Review Report

**Reviewed:** 2026-08-18T03:56:49Z
**Depth:** standard
**Files Reviewed:** 48
**Status:** issues_found

## Summary

The post-gap implementation does not yet meet the shipping bar. The durable admitted-identity authority from 07-11, descriptor-bound recovered-persistence marker from 07-12, and fail-fast/reverse-ownership mechanics from 07-14 are materially present. The 07-13 transport bound is implemented on Linux, but its production hook is unusable on the other supported Unix target and the combined command lifecycle can commit durable mutations before its bounded reconciliation fails. The 07-15 attach bit and one-call path exist, but order-sensitive equality still classifies semantically exact shells as mismatched.

Validation was not green: `GOTOOLCHAIN=local go test -count=1 ./... -timeout=240s` failed `TestWorktreeTwoShellEndToEnd` at reset convergence, leaving durable revision 9 while the shell remained applied at revision 7 with a pending pull. A targeted three-run repetition failed the same live workflow at another convergence checkpoint. `go test -race -count=1 ./core/worktree ./core/cli` and `bash -n scripts/perf-worktree.sh` passed.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01 [BLOCKER]: The production hook disables every private worktree operation on macOS

**File:** `core/shell/zsh/hook.go:929-956`
**Also affected:** `core/shell/zsh/hook.go:831-857`, `core/shell/zsh/worktree_live_test.go:159-162`

**Issue:** `_zp_worktree_invoke` requires `/proc/$$/fd` before it starts attach, publish, prepare, acknowledge, or resolve. Its endpoint discovery and child-ownership checks also depend on Linux `/proc`. macOS does not mount Linux procfs by default, yet the repository has explicit Darwin implementations and treats Linux and Darwin as supported targets. Consequently the loader sources successfully on macOS but every worktree synchronization operation returns failure. The adversarial transport test hides this regression by skipping every non-Linux platform.

**Fix:** Replace procfs discovery and ownership with a portable descriptor/child ownership abstraction, or emit OS-specific hook helpers with a Darwin implementation. Keep ownership exact by tracking only descriptors and PIDs created by the invocation; do not fall back to broad process killing. Add a native-Darwin real-zsh test that covers attach, publish, prepare/apply/acknowledge, timeout cleanup, and absence of surviving child processes.

### CR-02 [BLOCKER]: Positional state equality reports order-only differences as reconciliation failures

**File:** `core/worktree/state.go:730-739`
**Also affected:** `core/worktree/service.go:420-427`, `core/worktree/service.go:888-890`, `core/worktree/service.go:900-910`, `core/shell/zsh/hook.go:656-700`

**Issue:** `equalLiveStates` compares slice positions even though identity order is not semantic. Shell capture emits a fixed category/name order, while `replaceWorkflowGeneration` assigns the source-ordered projection directly to `state.Shared`; live additions are also appended rather than globally reordered. A checkout whose source lists an alias before an environment assignment, or a later event that appends a new environment assignment after an alias, therefore makes an exact shell compare unequal. Attach then returns `ReconcileRequired=true`, repair refuses an otherwise exact published revision, and the 07-15 first-sync path needlessly reports the shell behind. This contradicts `DiffSnapshot`, which already compares final values by identity.

**Fix:** Make equality semantic and order-independent, for example by validating both snapshots and treating an empty `DiffSnapshot` result as equality. Use that helper consistently for attach truth, commit/repair verification, and shell-behind calculation. Add tests with deliberately non-canonical source ordering and with a newly appended identity from an earlier category, then prove a late exact shell attaches at head and a commit/repair succeeds.

### CR-03 [BLOCKER]: Legitimate managed values can invalidate the transition footer

**File:** `core/cli/runtime.go:624-633`
**Also affected:** `core/shell/zsh/emit.go:226-249`

**Issue:** Footer validation counts raw occurrences of each `ZP_WORKTREE_REPLY_*=` substring across the entire emitted shell program. Managed scalar and function values are allowed to contain arbitrary quoted text, including those literal substrings. For example, an alias body containing `ZP_WORKTREE_REPLY_PROTOCOL=` is safely quoted by the emitter but makes the raw count equal two, so prepare/resolve rejects its own generated patch as an invalid footer. The shell remains pending/behind and cannot synchronize that otherwise valid value.

**Fix:** Validate the footer structurally instead of scanning quoted payload bytes. Prefer returning the generated program and trusted footer metadata as separate values from the emitter, or parse only top-level assignments in the exact suffix. Add runtime tests for all five footer variable substrings inside environment values, aliases, and multiline function bodies.

### CR-04 [BLOCKER]: Historical shell records permanently exhaust the 128-shell cap

**File:** `core/worktree/service.go:398-400`
**Also affected:** `core/worktree/state.go:680-685`

**Issue:** Attach rejects a new shell once `state.Shells` contains 128 entries. `CanGarbageCollectShell` defines an eligibility predicate, but no production code calls it, no code deletes a shell or its receipts, and deactivate has no authenticated detach operation. Shell IDs are per terminal session, so ordinary use accumulates durable records forever; after 128 historical sessions, every future terminal is permanently locked out even when all old records are clean and have no pending/conflicted/unpublished state.

**Fix:** Add an authenticated detach/liveness lifecycle and perform deterministic cleanup under the state lock, deleting associated receipts atomically. As a safe fallback, reclaim only records proven inactive and satisfying the existing no-pending/no-conflict/no-unpublished predicate before enforcing the cap. Test 128 clean retired sessions followed by a successful 129th attach, and prove that active, behind, pending, conflicted, or unpublished shells are never evicted.

### CR-05 [BLOCKER]: Checkout/reset can mutate durable state and then return failure with the shell stale

**File:** `core/shell/zsh/hook.go:1390-1398`

**Issue:** The wrapper spends one 250 ms deadline on pre-mutation attachment/publication, executes the irreversible `command zsh-pro checkout/reset`, and only afterward calls `_zp_worktree_pull`. If that final pull times out or fails, the wrapper returns nonzero even though the branch/reset already committed. The current test suite reproduced exactly this split state: reset advanced durable state to revision 9, returned `worktree pull failed`, and left the shell at revision 7 with a pending transition. Users receive a failed-command signal but cannot safely assume the requested mutation did not occur.

**Fix:** Collapse mutation and patch preparation into one authenticated core operation so the shell receives the exact resulting transition without another helper startup, and define an explicit recoverable result for an apply/ack failure after commit. At minimum, distinguish “mutation committed; reconciliation pending” from mutation failure and make a subsequent idempotent sync recover it. Add fault tests at every post-mutation boundary and require truthful branch/revision status plus deterministic recovery; keep the absolute interaction bound intact.

## Warnings

### WR-01 [WARNING]: The exported live-capture implementation disagrees with the hook capture contract

**File:** `core/shell/zsh/introspect.go:32-45`
**Also affected:** `core/shell/zsh/hook.go:647-662`, `core/shell/zsh/introspect_test.go:162-171`

**Issue:** `Provider.LiveCaptureSource` includes every exported scalar except PATH/FPATH, so it records volatile process-local values such as PWD, OLDPWD, SHLVL, and `_`. The production hook explicitly excludes those variables. The “no process-local records” test only searches the Go source string for NUL-delimited lower-case words and never executes the capture to assert that the identities are absent. This leaves two implementations of the same advertised capture contract with incompatible snapshots and makes future consumers vulnerable to order/churn and admission behavior that the hook path does not have.

**Fix:** Share one capture definition or one explicit exclusion list between the provider source and hook source. Execute a real zsh capture in the test and assert the decoded snapshot contains none of PWD, OLDPWD, SHLVL, or `_` while retaining ordinary exported variables.

---

_Reviewed: 2026-08-18T03:56:49Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
