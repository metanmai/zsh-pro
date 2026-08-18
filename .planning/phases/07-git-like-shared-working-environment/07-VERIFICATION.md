---
phase: 07-git-like-shared-working-environment
verified: 2026-08-18T04:10:21Z
status: gaps_found
score: 10/15 must-haves verified
behavior_unverified: 0
overrides_applied: 0
prohibitions_flagged: 7
re_verification:
  previous_status: gaps_found
  previous_score: 77/90
  gaps_closed:
    - "Durable admitted-identity ownership now survives fresh Registry and Service instances."
    - "Capture, serialization, transport cancellation, termination, and reap now share the bounded operation budget."
    - "Live patch operations now fail fast and install reverse recovery ownership before mutation."
    - "A first explicit sync now carries reconcile-required state and runs the complete reconcile pipeline."
    - "Recovered-persistence evidence now survives operation-scoped StateStore close and reopen."
  gaps_remaining:
    - "Private synchronization transport is hard-gated on Linux procfs and is unusable on the supported Darwin target."
    - "Canonical live-state equality is positional rather than semantic and order-independent."
    - "Transition footer validation rejects valid values containing a reply-variable assignment substring."
    - "Historical shell records are never retired, so 128 sessions permanently exhaust attachment capacity."
    - "Checkout/reset can commit durable state and then return failure when post-commit reconciliation fails."
  regressions:
    - "Five post-gap code-review blockers remain independently valid against the current source tree."
gaps:
  - truth: "Private worktree synchronization works on every platform the repository declares supported, including Darwin."
    status: failed
    reason: "The production zsh transport returns failure unless Linux /proc descriptor paths exist, while the repository explicitly supports Darwin and the only process/descriptor transport test skips non-Linux systems."
    root_cause: "The shell hook uses Linux procfs as its descriptor discovery and child-ownership abstraction instead of emitting or tracking a platform-portable owned PID/FD set."
    evidence:
      - "core/shell/zsh/hook.go:834-856 reads /proc/$$/fd and /proc/$$/task/$$/children."
      - "core/shell/zsh/hook.go:929-930 hard-fails every attach/publish/prepare/acknowledge/resolve invocation when /proc/$$/fd is absent."
      - "core/shell/zsh/worktree_live_test.go:159-162 skips the adversarial transport oracle on every non-Linux target."
      - "Darwin compilation succeeds, but that compile-only result cannot exercise the sourced hook."
    affected_requirements: [WORK-02, SYNC-01, SYNC-02]
    affected_plans: [07-03, 07-09, 07-10, 07-13]
    artifacts:
      - path: "core/shell/zsh/hook.go"
        issue: "Linux-only procfs checks sit on the production private-operation path."
      - path: "core/shell/zsh/worktree_live_test.go"
        issue: "No native-Darwin real-zsh attach/sync/cleanup test exists."
    missing:
      - "A Darwin-capable descriptor and child-ownership implementation that preserves exact owned-resource cleanup."
      - "A native-Darwin real-zsh attach, publish, pull, timeout, and survivor/FD cleanup test."
  - truth: "Live-state equality is semantic by identity and value; equivalent state is equal regardless of record order."
    status: failed
    reason: "equalLiveStates compares slice positions. Legitimate source replacement and live additions can produce a different record order for the same identity/value set, which makes exact shells appear behind and can reject exact repair or acknowledgement."
    root_cause: "The attach, repair, and behind paths use a second positional equality authority instead of the order-independent per-identity DiffSnapshot semantics."
    evidence:
      - "core/worktree/state.go:730-739 compares left[i] with right[i]."
      - "core/worktree/service.go:420-427 uses that comparison to decide exact-at-head versus clean reconcile."
      - "core/worktree/service.go:888-910 and 927 use the same comparison for repair and behind truth."
      - "Canonical-order tests pass, proving reordering is an intentional representation behavior rather than a prohibited input."
    affected_requirements: [WORK-02, SYNC-01, SYNC-02]
    affected_plans: [07-01, 07-05, 07-09, 07-15]
    artifacts:
      - path: "core/worktree/state.go"
        issue: "equalLiveStates is order-sensitive."
      - path: "core/worktree/service.go"
        issue: "Attach, repair, and status consume positional equality as convergence truth."
    missing:
      - "One validated order-independent equality helper, preferably based on an empty semantic DiffSnapshot."
      - "Attach/repair tests with deliberately reordered equivalent state and an appended earlier-category identity."
  - truth: "Every otherwise-valid managed scalar, alias, and function value can pass transition-footer validation, including values containing reply-variable text."
    status: failed
    reason: "Runtime validation counts raw ZP_WORKTREE_REPLY_*= substrings across the entire quoted shell program. A valid managed value containing one of those strings makes the count two and causes the runtime to reject its own emitted transition."
    root_cause: "The validator treats textual occurrence count as syntax/protocol structure and does not distinguish quoted payload bytes from the trusted footer suffix."
    evidence:
      - "core/cli/runtime.go:624-633 requires both an exact suffix and exactly one raw occurrence of each of five footer assignment substrings."
      - "core/shell/zsh/emit.go:226-249 appends the trusted footer after rendering arbitrary safely quoted managed values."
    affected_requirements: [WORK-02, WORK-03, SYNC-01, SYNC-02]
    affected_plans: [07-04, 07-08, 07-09, 07-14]
    artifacts:
      - path: "core/cli/runtime.go"
        issue: "bytes.Count scans quoted payload content as if it were top-level footer syntax."
    missing:
      - "Structural validation of the exact top-level footer or a separate trusted footer/metadata channel."
      - "Tests covering all five reply substrings inside env values, aliases, and multiline function bodies."
  - truth: "Historical terminal sessions cannot permanently exhaust the bounded shell-record table while protected active/pending/conflicted/unpublished sessions remain safe."
    status: failed
    reason: "Attach rejects the 129th distinct shell, but production never retires a shell record or associated receipts. Normal terminal churn therefore creates a permanent denial of service after 128 sessions."
    root_cause: "A safe CanGarbageCollectShell predicate exists without any authenticated detach, liveness lifecycle, cleanup call, or state.Shells deletion."
    evidence:
      - "core/worktree/service.go:398-400 rejects attachment when len(state.Shells) reaches MaxShellRecords."
      - "core/worktree/state.go:680-685 defines cleanup eligibility."
      - "Repository search finds no production caller of CanGarbageCollectShell and no delete(state.Shells, ...)."
    affected_requirements: [WORK-01, WORK-02, SYNC-01, SYNC-02]
    affected_plans: [07-03, 07-07, 07-09, 07-10]
    artifacts:
      - path: "core/worktree/service.go"
        issue: "Capacity is enforced before any safe retirement step."
      - path: "core/worktree/state.go"
        issue: "The eligibility predicate is orphaned from production lifecycle code."
    missing:
      - "An authenticated detach/liveness lifecycle or deterministic safe reclamation under the canonical state lock."
      - "A 128-retired-shells/129th-attach test plus non-eviction tests for active, behind, pending, conflicted, and unpublished shells."
  - truth: "Checkout and reset report whether the durable mutation committed separately from whether the invoking shell reconciled, and a failed reconcile has an explicit idempotent recovery result."
    status: failed
    reason: "The wrapper executes the durable checkout/reset command first and then performs a separate pull. If that pull fails, the wrapper returns nonzero even though the mutation has already committed, leaving the caller unable to interpret the result as mutation failure versus committed-but-pending reconciliation."
    root_cause: "The public command contract conflates an irreversible durable mutation with a later independently fallible shell-reconciliation operation."
    evidence:
      - "core/shell/zsh/hook.go:1390-1398 runs command zsh-pro checkout/reset before _zp_worktree_pull and returns the pull failure directly."
      - "The current full suite and six isolated E2E executions pass, so the earlier observed reset failure is timing-sensitive and did not reproduce; the deterministic post-commit failure branch remains present in source."
    affected_requirements: [WORK-03, SYNC-01, SYNC-02]
    affected_plans: [07-06, 07-09, 07-10]
    artifacts:
      - path: "core/shell/zsh/hook.go"
        issue: "A committed mutation can be followed by a failed pull and an undifferentiated nonzero public result."
    missing:
      - "A combined mutation/result transition or an explicit committed-reconciliation-pending result with idempotent recovery."
      - "Fault-injection tests at each post-mutation boundary that assert durable branch/revision truth and successful retry."
unverified_prohibitions:
  - statement: "MUST NOT make source-statement history the owner of observable final live state."
    disposition: "unverified-prohibition — non-authoritative LLM judgment found no concrete violation; human review recommended"
  - statement: "MUST NOT turn ambient inherited state into managed state or capture a newly observed secret-like value."
    disposition: "unverified-prohibition — focused tests support the ordinary production-hook path; human review recommended"
  - statement: "MUST NOT let a per-process profile marker redefine durable branch/base/revision."
    disposition: "unverified-prohibition — source wiring supports the guard; human review recommended"
  - statement: "MUST NOT expose captured values through conflicts, exclusions, forensic events, errors, or retained artifacts."
    disposition: "unverified-prohibition — value-free structures and canary tests exist; human review recommended"
  - statement: "MUST NOT discard dirty shared state except through explicit reset --hard."
    disposition: "unverified-prohibition — command/service guards exist; human review recommended"
  - statement: "MUST NOT broaden the local workflow into staging, advanced history, remotes, multiple worktrees, or externally authored profile application."
    disposition: "unverified-prohibition — strict parser guards exist; human review recommended"
  - statement: "MUST NOT synchronize foreground commands, PWD, jobs, buffers, history, process trees, or terminal UI between independent shells."
    disposition: "unverified-prohibition — production hook exclusions support the ordinary path; exported capture mismatch is noted as a warning; human review recommended"
---

# Phase 07: Git-Like Shared Working Environment Verification Report

**Phase Goal:** Deliver one Git-like shared working environment where independent zsh terminals use canonical durable branch/base/revision state, safely publish and synchronize supported live identities, expose strict Git-shaped workflows, preserve privacy and exact shell fidelity, recover conflicts/crashes truthfully, and keep prompt/runtime operations bounded and fail-open.

**Verified:** 2026-08-18T04:10:21Z
**Status:** gaps_found
**Re-verification:** Yes — after Plans 07-11 through 07-15 attempted closure of the five prior gaps

## Verdict

The phase goal is still not achieved.

Plans 07-11 through 07-15 close the five specific failure mechanisms from the 2026-08-17 verification. Their focused regressions pass, the live two-shell workflow passes five consecutive isolated runs, and one complete workspace test run is green. Those closures do not neutralize five different, independently observable blockers found after the gap work: Darwin transport is disabled by a Linux-procfs hard gate, convergence equality is order-sensitive, valid payload text can poison footer validation, historical shells permanently exhaust the record cap, and checkout/reset cannot distinguish committed mutation from failed reconciliation.

The earlier review's isolated `TestWorktreeTwoShellEndToEnd` reset failure did not reproduce in the current full run or five isolated repetitions. It is therefore not used as direct failure evidence. The checkout/reset finding remains a blocker because the source contains an unconditional committed-then-pull-failed branch; timing only determines whether that branch is reached.

## Goal Achievement

### Closure of the Five Prior Gaps

| Prior gap | Closure plan | Status | Current evidence |
| --- | --- | --- | --- |
| Safely admitted identity ownership was process-local. | 07-11 | CLOSED | `State.AdmittedIdentities`, `Registry.RestoreAdmitted`, transaction-local restoration/append, fresh-Service and concurrent-admission tests pass. |
| Capture/write/transport/reap escaped the single operation budget. | 07-13 | CLOSED | Incremental caps/deadline checks and owned transport abort/reap exist; capture/frame and adversarial real-zsh tests pass. The 472–500 ms driver cases execute a fail-open prompt attempt and a separate explicit-sync attempt, each with its own 250 ms budget. |
| Partial patch failure could be masked or lose reverse ownership. | 07-14 | CLOSED | Per-operation checked emission, pre-mutation recovery ownership, public retry, and early/final/post-apply failure tests pass. |
| First explicit sync could falsely report success after mismatched attach. | 07-15 | CLOSED | `ReconcileRequired` crosses Service/runtime/strict loader parsing and one-call success/failure tests pass. The separate positional-equality blocker can still create false divergence. |
| Recovered-persistence evidence disappeared across Store reopen. | 07-12 | CLOSED | Descriptor-relative marker write/read/project/clear lifecycle and close/reopen/concurrency tests pass. |

### Consolidated Observable Truths

| # | Truth | Status | Evidence |
| ---: | --- | --- | --- |
| 1 | Typed live identities preserve exact scalar/list/function/option semantics and diff by final identity value. | VERIFIED | Model/diff/overlay code and boundary/golden tests pass. |
| 2 | Ordinary supported live values converge across two retained independent zsh processes. | VERIFIED | `TestWorktreeTwoShellEndToEnd` passes five consecutive isolated runs plus the full suite. |
| 3 | Strict status/diff/commit/branch/checkout/reset/config routing uses canonical durable state and exact Git objects. | VERIFIED | CLI/workflow/store wiring and full tests pass; post-commit result ambiguity is isolated in Truth 15. |
| 4 | Production-hook capture excludes secret-like, bookkeeping, volatile, and process-local identities from ordinary synchronization. | VERIFIED | Hook exclusions, policy/SecretRef pinning, and current security/canary tests pass. |
| 5 | Canonical state, events, locks, and revisions are bounded, authenticated, atomic, and crash-recoverable. | VERIFIED | StateStore/descriptor/Git/recovery tests pass. |
| 6 | Safely admitted ownership survives fresh Service/Registry instances and late-shell convergence. | VERIFIED | Plan 07-11 focused fresh-instance and concurrency tests pass. |
| 7 | A Linux parent-shell operation incrementally enforces caps and one deadline through transport abort/reap while remaining fail-open. | VERIFIED | Plan 07-13 focused tests and the production sampler pass. |
| 8 | Partial live-patch failure is fail-fast, reverse-owned before mutation, and recoverable by a public retry. | VERIFIED | Plan 07-14 focused emitter and real-zsh recovery tests pass. |
| 9 | One first explicit sync truthfully reconciles a mismatched fresh shell or remains behind/nonzero. | VERIFIED | Plan 07-15 Service/runtime/real-zsh tests pass. |
| 10 | Recovered-persistence evidence survives close/reopen and clears only after canonical projection. | VERIFIED | Plan 07-12 lifecycle and concurrent reopen tests pass. |
| 11 | Private synchronization transport works on Linux and Darwin, the repository's supported Unix targets. | FAILED | Production hook hard-requires Linux `/proc`; its transport oracle skips non-Linux. |
| 12 | Equivalent live state compares equal independent of record order. | FAILED | `equalLiveStates` compares slice positions and is used for attach, repair, and behind state. |
| 13 | Arbitrary valid managed values cannot be mistaken for private reply-footer structure. | FAILED | Raw substring counting rejects safely quoted payloads containing reply assignment text. |
| 14 | Retired shell history cannot permanently exhaust attachment capacity. | FAILED | No production deletion/detach/GC consumes `CanGarbageCollectShell`; the 129th distinct shell is rejected forever. |
| 15 | Checkout/reset separately report durable commit truth and shell reconciliation truth. | FAILED | Durable mutation precedes a separately fallible pull whose failure is returned as the command result. |

**Score:** 10/15 truths verified

### Required Artifacts

All artifacts declared by Plans 07-11 through 07-15 exist, are substantive, and are wired. Their gap-specific behavior tests pass. Current status reflects the wider phase goal rather than file existence.

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/worktree/state.go` / `registry.go` / `service.go` | Durable admitted-identity authority | VERIFIED | Value-free normalized identities restore and append under the canonical transaction. |
| `core/worktree/atomic_unix.go` | Durable recovered-persistence marker | VERIFIED | Fixed-name descriptor-relative lifecycle survives Store reopen. |
| `core/shell/zsh/hook.go` | Bounded transport, recovery ownership, truthful first sync | PARTIAL | Gap-specific Linux behavior is present; Darwin procfs hard gate and checkout/reset result ambiguity remain. |
| `core/shell/zsh/emit.go` | Checked forward patch and replacement reverse | VERIFIED | Each operation is guarded and recovery ownership is installed before mutation. |
| `core/model/worktree.go` / `core/cli/runtime.go` | Reconcile metadata and strict transport | PARTIAL | Attach bit is wired; runtime footer validation rejects valid payload substrings. |
| `core/shell/zsh/worktree_live_test.go` | Real-zsh convergence/failure proof | PARTIAL | Linux coverage is strong; native-Darwin transport coverage is intentionally skipped. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `State.AdmittedIdentities` | fresh `Registry`/`Service` | restore under each canonical transaction | WIRED | Late-shell exact-head test passes. |
| forensic append failure | next operation-scoped Store | descriptor-relative marker projection | WIRED | Close/reopen and concurrent lifecycle tests pass. |
| emitted patch | parent shell mutation | checked forward + preinstalled reverse recovery | WIRED | Partial-failure recovery tests pass. |
| attach mismatch | exact acknowledgement | reconcile bit → prepare/apply/fresh capture/ack | WIRED | One-call success/failure tests pass. |
| semantic state | attach/repair/behind | `equalLiveStates` | NOT WIRED CORRECTLY | Consumer exists, but uses positional rather than semantic equality. |
| valid emitted source | runtime footer validator | exact suffix plus raw whole-program substring counts | NOT WIRED CORRECTLY | Quoted payload bytes are treated as protocol structure. |
| public checkout/reset | durable mutation and shell reconciliation | command, then independent pull | PARTIAL | No truthful committed-but-reconciliation-pending result. |
| supported Darwin zsh | private runtime helper | `/proc/$$/...` | NOT WIRED | Linux-only resource discovery blocks the operation before request framing. |

### Data-Flow Trace

| Data | Source | Consumer | Status |
| --- | --- | --- | --- |
| Canonical branch/base/revision/shared values | authenticated `worktree.json` and exact Git objects | Service, CLI, runtime, regeneration | FLOWING |
| Value-free admitted identities | `State.AdmittedIdentities` | fresh Registry policy and late-shell target | FLOWING |
| Recovered-persistence marker | fixed descriptor-relative file | next canonical transaction | FLOWING |
| Attach reconcile truth | `ShellState` | `AttachResult` → ZPWA → loader pull/ack | FLOWING |
| Semantic equality | two identity/value sets | attach/repair/status | HOLLOW SEMANTICS — positional comparison |
| Shell lifecycle | `State.Shells` map | attachment capacity | DISCONNECTED — eligibility exists, retirement does not |
| Mutation outcome | durable checkout/reset then pull | public return code | AMBIGUOUS — commit and reconciliation collapse into one status |

## Behavioral Validation

### Commands and Results

| Check | Command | Result | Status |
| --- | --- | --- | --- |
| Durable admission + recovery marker + attach truth | `go test -count=1 ./core/worktree -run '^(TestFreshServiceLateShellUsesDurableAdmission|TestConcurrentDurableAdmissionsAreDeterministic|TestRecoveredPersistenceOperationScopedLifecycle|TestRecoveredPersistenceConcurrentReopen|TestAttachResultTruthfullyReportsCleanReconcile)$'` | `ok`, 0.169 s | PASS |
| Deadline + patch recovery + first sync | `go test -count=1 ./core/shell/zsh -run '^(TestWorktreeCaptureAndFrameUseOneIncrementalDeadline|TestWorktreeAbsoluteDeadlineAdversarialTransport|TestEmitLivePatchFailsFastPerOperation|TestWorktreeRecoveryOwnerPrecedesMutation|TestWorktreePartialPatchFailureRecovery|TestWorktreeFirstExplicitSyncReconciles|TestWorktreeFirstExplicitSyncFailureStaysBehind)$'` | `ok`, 7.754 s | PASS |
| Live two-shell convergence, repeated | `go test -count=5 ./core/shell/zsh -run '^TestWorktreeTwoShellEndToEnd$'` | `ok`, 21.173 s | PASS 5/5 |
| Canonical reorder behavior, repeated | `go test -count=3 ./core/worktree -run '^(TestAcknowledgeCanonicalizesFreshIdentityRecordOrder|TestResolveSharedCanonicalizesFreshIdentityRecordOrderOnly)$'` | `ok`, 1.187 s | PASS 3/3 |
| Full workspace | `go test -count=1 ./... -timeout=240s` | all packages passed | PASS |
| Production sampler | build `/tmp/phase7-zsh-pro`; `ZSHPRO_BIN=/tmp/phase7-zsh-pro bash scripts/perf-worktree.sh --cycles 50` | p50 61.949 ms, p95 89.688 ms, max 111.566 ms; Git calls 0; child-zsh calls 0; adversarial cleanup/sentinel checks pass | PASS |
| Darwin compile surface | `GOOS=darwin GOARCH=amd64 go test -c ./core/shell/zsh -o /tmp/phase7-zsh-darwin.test` | 7.2 MiB test binary produced | PASS, COMPILE ONLY |

The full suite was run once. Focused repetitions, not repeated full-suite runs, were used to distinguish the earlier E2E report from a reproducible failure. Current evidence does not reproduce the reset convergence failure, but no existing test forces a post-commit pull failure and checks the public result contract.

### macOS Transport Evidence

The repository declares Linux and Darwin as supported in platform-specific runtime/store files and in `core/cmd/zsh-pro/main_test.go`. A Darwin test binary compiles, but the sourced hook is shell code and unconditionally checks Linux procfs at runtime. The only adversarial transport test explicitly skips non-Linux systems.

Apple's open-source XNU exposes process inspection through the `proc_info` syscall and libproc-family APIs, reinforcing that Darwin needs a native resource-ownership path rather than an assumed Linux `/proc` layout. [Apple XNU syscall table](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/syscalls.master), [Apple XNU libproc implementation](https://github.com/apple-oss-distributions/xnu/blob/main/libsyscall/wrappers/libproc/libproc.c)

### Anti-Patterns and Warning

No unreferenced `TBD`, `FIXME`, or `XXX` marker was found in the Phase 07 production files.

One material warning remains: `Provider.LiveCaptureSource` in `core/shell/zsh/introspect.go:32-45` includes every exported scalar except PATH/FPATH, while the production hook excludes PWD, OLDPWD, SHLVL, and `_`. `TestLiveCaptureSourceHasNoChildOrProcessLocalRecords` only searches source text for weak forbidden substrings and does not assert decoded identity absence. The provider is not the production synchronization capture path today, so this is a warning rather than a sixth goal blocker, but the duplicate capture contracts should be unified before a new consumer uses the exported provider.

## Requirements Coverage

`REQUIREMENTS.md` and `ROADMAP.md` still stop at Phase 06. They define neither Phase 07 nor WORK-01/02/03 and SYNC-01/02, although all fifteen Phase 07 plans declare those IDs. The table therefore maps against plan declarations; registry traceability itself remains a planning warning.

| Requirement | Plan-declared intent | Status | Blocking evidence |
| --- | --- | --- | --- |
| WORK-01 | One canonical durable worktree/generation and safe terminal lifecycle | BLOCKED | Historical shell records permanently exhaust the attachment cap. |
| WORK-02 | Exact shared live-state synchronization across supported terminals | BLOCKED | Darwin transport hard gate, order-sensitive equality, footer payload collision. |
| WORK-03 | Strict truthful Git-shaped public mutations | BLOCKED | Checkout/reset conflates committed durable mutation with failed shell reconciliation. |
| SYNC-01 | Safe bounded reconciliation at explicit/automatic boundaries | BLOCKED | Darwin has no working private transport; post-mutation recovery result is ambiguous. |
| SYNC-02 | Truthful exact convergence and recovery | BLOCKED | Order-only divergence, footer rejection, shell-cap exhaustion, checkout/reset split state. |

## Human Verification and Prohibitions

No additional human action can turn the five observable source defects into a pass. Seven judgment-tier must-NOT statements remain non-authoritatively reviewed and are preserved in frontmatter as `unverified_prohibitions`; human review is recommended before shipping even after blocker closure.

## Deferred-Item Check

No blocker is deferred. `roadmap.analyze` contains only Phases 01–06 and no later Phase 07 milestone work that specifically owns these defects.

## Gaps Summary

Five independent blockers remain. They are not restatements of the five prior gaps: Plans 07-11 through 07-15 materially fixed those mechanisms. The next closure plan must address platform-portable owned transport, one semantic equality authority, structurally separated footer validation, authenticated shell retirement, and truthful post-mutation reconciliation results. The exported live-capture mismatch and absent Phase 07 requirement registry should be corrected as warnings alongside that work.

---

_Verified: 2026-08-18T04:10:21Z_
_Verifier: the agent (gsd-verifier)_
