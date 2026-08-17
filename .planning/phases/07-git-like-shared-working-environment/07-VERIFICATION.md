---
phase: 07-git-like-shared-working-environment
verified: 2026-08-17T17:31:30Z
status: gaps_found
score: 77/90 must-haves verified
behavior_unverified: 0
overrides_applied: 0
prohibitions_flagged: 7
gaps:
  - truth: "Canonical ownership of safely admitted live identities is durable and interpreted identically by fresh public and runtime Service instances."
    status: failed
    reason: "Registry.Admit records ownership only in one in-memory Registry, while every runtime Bind creates a fresh Registry and every Service path reseeds it from Committed.Source. A later shell that already has an A-published identity classifies it as present-at-attach unmanaged and cannot acknowledge the canonical Shared fingerprint."
    artifacts:
      - path: "core/worktree/registry.go"
        issue: "Admit mutates only Registry.owned; Seed replaces ownership from source entries."
      - path: "core/worktree/state.go"
        issue: "The combined canonical generation has Shared values but no value-free durable admitted-identity ownership."
      - path: "core/cli/emitter.go"
        issue: "RuntimeWorktreeFactory.Bind constructs a fresh Registry for every operation."
      - path: "core/worktree/service.go"
        issue: "Attach and Acknowledge seed only state.Committed.Source, so admittedSnapshot filters the later shell's identity."
    missing:
      - "Persist value-free canonical admitted identity ownership, or seed every operation from source ownership plus safe canonical Shared identities."
      - "Add a fresh-Service late-attach test: A publishes a new identity; C starts with it present; C attaches, prepares, applies, and acknowledges the exact head."
  - truth: "Every prompt/runtime synchronization path is bounded to the 250 ms absolute deadline and fails open without an unbounded write or child reap."
    status: failed
    reason: "The shell-side capture and frame serializer perform unbounded enumeration/copy/write work, and invoke waits synchronously before and after the timed read. The Go decoder's later size/deadline checks cannot bound work already performed by the interactive shell."
    artifacts:
      - path: "core/shell/zsh/hook.go"
        issue: "_zp_worktree_capture has no record/byte cap or deadline checks; _zp_worktree_write_snapshot_records writes synchronously; _zp_worktree_invoke uses unbounded wait in failure, timeout, and always cleanup paths."
      - path: "scripts/perf-worktree.sh"
        issue: "The sampler covers small no-op frames and invokes stage-seam tests, but does not exercise oversized captures, a helper that stops reading stdin, or a TERM-ignoring helper."
    missing:
      - "Enforce snapshot record/byte caps and the same absolute deadline incrementally during capture and serialization."
      - "Make frame delivery cancellable and helper termination/reaping bounded."
      - "Add real-zsh oversized-capture, blocked-reader, and TERM-ignoring-helper tests that prove the next command remains usable."
  - truth: "A failed live patch is fail-fast and retains an idempotent public recovery route for every mutation already applied."
    status: failed
    reason: "EmitLivePatch concatenates forward commands without explicit failure propagation. An intermediate failure can be masked by a later success; a final failure returns before the replacement reverse is assigned to an active or recovery pointer, leaving earlier mutations without owned recovery."
    artifacts:
      - path: "core/shell/zsh/emit.go"
        issue: "Forward operations are emitted sequentially with no per-operation fail-fast guard."
      - path: "core/shell/zsh/hook.go"
        issue: "_zp_worktree_apply_transition invokes apply_name before installing reverse_name in any recovery owner."
    missing:
      - "Emit explicit fail-fast status propagation for every forward operation."
      - "Install replacement reverse recovery ownership before apply; retain it through apply, deadline, capture, or acknowledgement failure; promote only after verification."
      - "Add real-zsh early- and final-operation failure tests with nonzero status and idempotent public recovery."
  - truth: "The first explicit sync on an unattached shell either fully reconciles to canonical state or reports failure/behind; it never reports a false successful convergence."
    status: failed
    reason: "Service.Attach records a mismatched shell as clean-reconcile with AppliedRevision zero but returns only head Revision. The hook stores that head as applied, sets ATTACHED_NOW, and _zp_worktree_sync immediately returns success without prepare/apply/fresh-capture/acknowledge."
    artifacts:
      - path: "core/worktree/service.go"
        issue: "AttachResult omits clean-reconcile/applied metadata even when ShellState is behind."
      - path: "core/shell/zsh/hook.go"
        issue: "_zp_worktree_ensure_attached_impl assigns the returned head as applied; _zp_worktree_sync returns immediately when attachment occurred."
    missing:
      - "Expose sufficient attach state or immediately run the reconcile pipeline on first explicit sync."
      - "Add a one-call real-zsh test that sources the loader and invokes exactly one sync against mismatched canonical state."
  - truth: "A recovered persistence/forensic append failure remains truthfully observable across the production operation-scoped StateStore close/reopen lifecycle."
    status: failed
    reason: "pendingRecoveredPersistence is only an in-memory StateStore boolean. Runtime Bind returns a newly opened operation-scoped StateStore that is closed after the helper call, so an append-failure marker disappears before the next transaction can project RecoveredPersistence."
    artifacts:
      - path: "core/worktree/atomic_unix.go"
        issue: "The pending marker is instance-local and is not included in canonical or descriptor-relative durable state."
      - path: "core/cli/emitter.go"
        issue: "Each runtime operation creates and closes a fresh StateStore, which loses the marker."
    missing:
      - "Persist a value-free recovery marker descriptor-relatively or atomically record RecoveredPersistence after canonical replacement."
      - "Add an append-failure test that closes, reopens through a newly authenticated descriptor, then verifies marker surfacing/clearing."
unverified_prohibitions:
  - statement: "MUST NOT make source-statement history the owner of observable final live state."
    disposition: "unverified-prohibition — non-authoritative LLM judgment found no concrete violation; human review recommended"
  - statement: "MUST NOT turn ambient inherited state into managed state or capture a newly observed secret-like value."
    disposition: "unverified-prohibition — focused tests support the ordinary path; human review recommended"
  - statement: "MUST NOT let a per-process profile marker redefine durable branch/base/revision."
    disposition: "unverified-prohibition — source wiring supports the guard; human review recommended"
  - statement: "MUST NOT expose captured values through conflicts, exclusions, forensic events, errors, or retained artifacts."
    disposition: "unverified-prohibition — value-free structures and canary tests exist; human review recommended"
  - statement: "MUST NOT discard dirty shared state except through explicit reset --hard."
    disposition: "unverified-prohibition — command/service guards exist; human review recommended"
  - statement: "MUST NOT broaden the local workflow into staging, advanced history, remotes, multiple worktrees, or externally authored profile application."
    disposition: "unverified-prohibition — strict parser guards exist; human review recommended"
  - statement: "MUST NOT synchronize foreground commands, PWD, jobs, buffers, history, process trees, or terminal UI between independent shells."
    disposition: "unverified-prohibition — capture kinds exclude these categories; human review recommended"
---

# Phase 07: Git-Like Shared Working Environment Verification Report

**Phase Goal:** Deliver one Git-like shared working environment where independent zsh terminals use canonical durable branch/base/revision state, safely publish and synchronize supported live identities, expose strict Git-shaped workflows, preserve privacy and exact shell fidelity, recover conflicts/crashes truthfully, and keep prompt/runtime operations bounded and fail-open.

**Verified:** 2026-08-17T17:31:30Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Verdict

The phase goal is not achieved. Large portions of the typed model, Git persistence, strict CLI, canonical state machine, privacy boundary, and ordinary two-shell workflow are substantive and executable. Five concrete production paths nevertheless violate the goal: durable admitted ownership, the end-to-end prompt deadline, partial live-patch recovery, first-call explicit sync, and recovered-persistence marker survival.

The green tests are retained as positive evidence, not treated as proof of the omitted paths. No override exists. ROADMAP.md has no Phase 07 row and no later phase, so none of these gaps can be deferred under the milestone roadmap.

## Goal Achievement

### Plan Must-Have Coverage

All 90 declared truths from Plans 07-01 through 07-10 were checked against current source, wiring, and focused executable evidence. Repeated truths invalidated by the same production defect are counted separately because each is a declared phase contract.

| Plan | Verified | Failed truths | Evidence |
| --- | ---: | --- | --- |
| 07-01 typed live-state contract | 7/7 | — | Canonical typed snapshot/equality/diff code is substantive; boundary and semantic tests exist. |
| 07-02 registry/privacy boundaries | 6/6 | — | Source seeding, secret pinning, exact admission rules, and narrow provider interfaces exist. The later global-ownership requirement is introduced by the combined-authority plans below. |
| 07-03 durable state machine | 10/11 | T7 | worktree.json does not contain durable admitted-identity ownership, so not every causal ownership record is in the canonical generation. |
| 07-04 exact regeneration/patch | 6/7 | T4 | Successful patch/reverse fidelity passes, but a partial forward failure can lose reverse ownership and therefore cannot guarantee exact later restore. |
| 07-05 Git persistence | 8/8 | — | Versioned DTO, exact Git object validation, expected-base CAS, legacy compatibility, and redacted SecretRef paths are substantive and wired. |
| 07-06 strict public workflows | 8/8 | — | status/diff/commit/branch/checkout/reset/config parsing and canonical workflow delegation are present; unsupported verbs fail before dependency access. |
| 07-07 composition/canonical authority | 5/7 | T5, T7 | State bytes share one root, but distinct fresh Registry/Service instances do not interpret admitted ownership equivalently; ephemeral registry memory remains decision authority. |
| 07-08 private capture/patch adapters | 8/9 | T3 | Typed operations and private reply transport exist, but the emitted forward function is not fail-fast and replacement reverse ownership is not recoverable on partial apply. |
| 07-09 hook/runtime integration | 10/13 | T3, T11, T13 | First sync bypasses reconcile; the parent deadline excludes unbounded capture/write/reap work; partial apply has no recovery owner. |
| 07-10 production proof | 9/14 | T1, T4, T11, T13, T14 | The positive E2E/sampler passes, but it does not prove late attach, first-call sync, oversized/backpressured transport, partial apply, or close/reopen recovery; those current paths are broken. |

**Score:** 77/90 truths verified

### Consolidated Observable Outcomes

| # | Outcome | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Exact typed live-state values, tombstones, lists, functions, and semantic per-identity diff exist. | VERIFIED | core/model/worktree.go, core/worktree/diff.go, and focused boundary/golden tests. |
| 2 | Strict status/diff/commit/branch/checkout/reset/config routing uses canonical durable state and exact Git objects. | VERIFIED | core/worktree/service.go, core/cli/worktree.go, core/store/store.go, core/store/git.go. |
| 3 | SecretRef literals and unsupported ambient/process-local categories remain outside canonical/status/event transport. | VERIFIED | Registry classifier/pinning, value-free state types, capture kind allowlist, and security canary tests. |
| 4 | Safely admitted identity ownership survives fresh operation-scoped services and later-shell attachment. | FAILED | Registry ownership is memory-only and reseeded from source on every fresh service. |
| 5 | The whole parent-shell synchronization operation is bounded to 250 ms and fail-open. | FAILED | Capture, serialization, pipe delivery, and waits are outside or ignore the bound. |
| 6 | Live patch failure is fail-fast and recoverable without losing applied-state ownership. | FAILED | Forward commands can mask failure; reverse ownership is installed only after apply succeeds. |
| 7 | One first explicit sync fully reconciles an unattached, mismatched shell. | FAILED | Attach reports head only and sync returns immediately on ATTACHED_NOW. |
| 8 | Conflict resolution and ordinary prepare/apply/fresh-capture/ack flow are durable and idempotent. | VERIFIED | Focused service tests and the ordinary two-shell E2E pass; this does not override outcomes 4–7. |
| 9 | Canonical replacement and recovery evidence survive crashes and operation-scoped reopen. | FAILED | Canonical state replacement is atomic, but the forensic-append recovery marker is only instance-local. |
| 10 | Fifty ordinary no-op prompt cycles avoid Git/child-zsh introspection and report actual measurements. | VERIFIED | Independent sampler: count 50, p50 30.182838 ms, p95 35.557270 ms, max 37.959099 ms, Git calls 0, child-zsh calls 0. |

## Required Artifacts

The artifact verifier reported 33/36 pattern checks automatically. Its three misses are matcher false positives, not missing code:

- core/shell/zsh/classify.go defines func (p Provider) IsLiveSecretIdentity rather than the exact unnamed-receiver text.
- core/worktree/state.go declares StateSchemaVersion inside a const block.
- core/shell/zsh/emit.go defines EmitRuntimeTransition on a named provider receiver.

Manual inspection confirms all 36 declared artifact paths exist and are substantive. Final status includes wiring and behavior:

| Plan artifacts | Exists/substantive | Wiring status | Final status |
| --- | --- | --- | --- |
| 07-01 (2), 07-05 (3), 07-06 (3) | Yes | Model → diff → Git/workflow routes are exercised | VERIFIED |
| 07-02 (3) | Yes | Ordinary attach/admit works; admitted ownership is not durable across fresh registries | PARTIAL |
| 07-03 (4) | Yes | Combined state/store/service are wired; durable ownership and close/reopen recovery marker are absent | FAILED |
| 07-04 (5), 07-08 (3) | Yes | Patch planning/emission is wired; partial apply has no fail-fast recovery contract | FAILED |
| 07-07 (3) | Yes | Path and descriptor stores address the same bytes; fresh services are not semantically equivalent for admitted ownership | FAILED |
| 07-09 (3) | Yes | Hook/runtime/service path is live; first sync and parent deadline/recovery wiring are broken | FAILED |
| 07-10 (7) | Yes | Test and sampler artifacts execute, but omit the five failing production paths | PARTIAL |

## Key Link Verification

The automated key-link query could not interpret the plans' symbol-level from/to descriptions as file paths, so every declared link was traced manually.

| Link | Status | Details |
| --- | --- | --- |
| LiveSnapshot → model normalization → worktree diff/overlay → activate patch | WIRED | One typed semantic authority is consumed across model/worktree/activate. |
| CommittedWorktree → DTO/Git blobs → ReadWorktreeRevision/regeneration | WIRED | Exact fixed-object validation and source/projection reconstruction are present. |
| CLI status/diff/commit/branch/checkout/reset/config → WorktreeWorkflow → Service | WIRED | Strict complete-argv validation precedes service dependency use. |
| RuntimeRoot descriptor → operation StateStore → Service → worktree.json | PARTIAL | Same bytes are addressed, but each Bind also creates a fresh Registry whose admitted ownership is not in those bytes. |
| Loader capture → bounded frame → runtime decoder/service | FAILED | Decoder is bounded, but shell capture/frame production and child cleanup are not. |
| Attach → first explicit sync → prepare/apply/fresh-capture/acknowledge | FAILED | ATTACHED_NOW returns before reconciliation and stores head as applied. |
| Service Prepare/Resolve → emitted forward/reverse → parent apply → acknowledge | FAILED | Success path is wired; partial apply can be masked or lose reverse recovery ownership. |
| StateStore canonical replace → forensic recovery marker → next operation reopen | FAILED | The marker ends at StateStore.Close because it is only a boolean field. |

## Data-Flow Trace

| Data | Source | Durable/consumer path | Status |
| --- | --- | --- | --- |
| Canonical branch/base/revision/shared values | worktree.json under authenticated root | Service → CLI/runtime → Git/worktree decisions | FLOWING |
| Source-owned live identities and SecretRef pinning | Committed.Source | Registry.Seed → attach/publish/ack filters | FLOWING |
| Post-attach admitted ownership | Registry.Admit | In-memory Registry.owned only; new Bind loses it | DISCONNECTED FROM DURABLE AUTHORITY |
| First attach reconcile state | ShellState AttachStateCleanReconcile/Behind | AttachResult exposes head only; hook records it as applied | HOLLOW RESPONSE |
| Shell live snapshot | zsh parameter/alias/function/path/option tables | capture arrays → synchronous frame → Go decoder | FLOWING BUT UNBOUNDED |
| Replacement reverse | EmitLivePatch function | installed as active only after forward succeeds | DISCONNECTED ON PARTIAL FAILURE |
| RecoveredPersistence | append failure | StateStore.pendingRecoveredPersistence → next transaction on same instance only | DISCONNECTED AFTER CLOSE |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Ordinary production two-shell workflow | GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -run ^TestWorktreeTwoShellEndToEnd$ -timeout=30s | Passed in 2.897 s | PASS; does not cover late-shell pre-existing admitted identity or one-call first sync |
| Existing cumulative/real timing seam | GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -run ^TestWorktreeTransitionRealTime25And500MillisecondMargins$ -timeout=15s | Passed in 0.358 s | PASS; does not cover capture/frame/backpressure/reap |
| Existing forensic append recovery | GOTOOLCHAIN=local go test -count=1 ./core/worktree -run ^TestFaultForensicAppendDoesNotReverseCanonicalAndRecordsRecovery$ -timeout=10s | Passed in 0.015 s | PASS; reuses one StateStore and omits close/reopen |
| Successful patch/reverse fidelity | GOTOOLCHAIN=local go test -count=1 ./core/shell/zsh -run ^TestEmitLivePatchOwnsExactApplyAndReplacementReverse$ -timeout=10s | Passed in 0.007 s | PASS; has no partial mutation failure |
| Declared 50-cycle driver | fresh production binary; ZSHPRO_BIN=... scripts/perf-worktree.sh --cycles 50 | count 50; p50 30.182838 ms; p95 35.557270 ms; max 37.959099 ms; Git/child-zsh 0/0; embedded tests pass | PASS for small no-op workload; not proof of adversarial bound |

## Probe Execution

No probe-*.sh file or explicit probe declaration exists for Phase 07. The declared scripts/perf-worktree.sh driver was independently executed above.

## Requirements Coverage

REQUIREMENTS.md is a legacy milestone file containing neither definitions nor Phase 07 mappings for WORK-01, WORK-02, WORK-03, SYNC-01, or SYNC-02. The descriptions below therefore come only from plan usage; they are delivery evidence, not invented requirements traceability.

| ID | Declared by plans | Delivery evidence | Traceability |
| --- | --- | --- | --- |
| WORK-01 | 07-03, 07-05, 07-07, 07-10 | Materialization/Git/state authority exists, but admission ownership and recovery evidence are not wholly canonical | MISSING from REQUIREMENTS.md |
| WORK-02 | 07-01–04, 07-07–10 | Typed fidelity, privacy, capture, and ordinary admission exist; durable admission and partial-patch safety fail | MISSING from REQUIREMENTS.md |
| WORK-03 | 07-04–07, 07-09–10 | Strict Git-shaped workflows and exact committed projection are implemented and focused checks pass | MISSING from REQUIREMENTS.md |
| SYNC-01 | 07-03–04, 07-06–10 | Ordinary sync/resolve works; first sync and partial-apply recovery fail | MISSING from REQUIREMENTS.md |
| SYNC-02 | 07-01–03, 07-05–10 | Private credential/state/conflict transport exists; durable ownership, end-to-end bound, and reopen recovery fail | MISSING from REQUIREMENTS.md |

This planning-traceability debt is recorded separately from the observable delivery blockers. No legacy requirement is mapped to Phase 07, so there is no additional orphaned legacy ID to assess.

## Anti-Patterns and Prohibitions

No unreferenced TBD, FIXME, or XXX marker was found in the Phase 07 production files. Matches for placeholder and empty returns were legitimate SecretRef placeholder construction and typed byte-slice returns, not stubs.

The plans contain 18 unresolved prohibition entries representing nine distinct statements. Two are observably violated and are already blocking gaps:

- Failures must not prevent the next prompt/command: the unbounded frame-write/reap paths can block it.
- Behind/conflicted state must not be presented as converged: first explicit sync can return success while the durable shell is clean-reconcile/behind.

The remaining seven are recorded in frontmatter as non-authoritative LLM judgments with human review recommended. They are not silently counted as green prohibitions.

## Human Verification Recommended

No visual or external-service check is needed before recognizing the five automated blockers. After those gaps are fixed, a human should review the seven remaining judgment-tier prohibitions and run retained interactive zsh UAT for:

1. A later third terminal attaching after a new shared identity already exists locally.
2. A single explicit sync immediately after sourcing, with canonical state intentionally different.
3. Oversized state and a non-cooperative helper, confirming the next typed command is usable promptly.
4. Early/final partial patch failures followed by the public recovery/deactivation route.

## Gaps Summary

Five root gaps block the phase goal. They are not deferred by any later roadmap phase:

1. Canonical shared values do not carry canonical admitted-identity ownership.
2. The advertised 250 ms bound excludes shell capture, frame delivery, and reaping.
3. Partial live-patch failure can be masked or lose recovery ownership.
4. First explicit sync can falsely succeed without reconciliation.
5. Recovered-persistence evidence is lost when the operation-scoped store closes.

The next action is gap planning:

    $gsd-plan-phase 7 --gaps

---

_Verified: 2026-08-17T17:31:30Z_
_Verifier: the agent (gsd-verifier)_
