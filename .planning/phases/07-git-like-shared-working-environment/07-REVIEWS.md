---
phase: 7
reviewers: [claude, coderabbit, opencode, cursor, ollama]
reviewed_at: 2026-08-17T08:01:54Z
plans_reviewed: [07-01-PLAN.md, 07-02-PLAN.md, 07-03-PLAN.md, 07-04-PLAN.md, 07-05-PLAN.md, 07-06-PLAN.md, 07-07-PLAN.md, 07-08-PLAN.md, 07-09-PLAN.md, 07-10-PLAN.md]
---

# Cross-AI Plan Review — Phase 7

Phase 7 was submitted to every locally available independent adapter. OpenCode completed the full source-grounded review prompt. CodeRabbit completed a narrower diff-only review of the phase directory. Claude, Cursor, and Ollama were invoked but did not return usable reviews; their failures are recorded below. Codex was intentionally not invoked because this workflow is already running in Codex and would not provide an independent review.

## Claude Review

**Status:** Failed — excluded from consensus.

The adapter returned immediately with:

> API Error: 402 Insufficient credits. Add more using https://openrouter.ai/settings/credits

No review content was produced.

---

## CodeRabbit Review

**Status:** Completed with 9 findings (8 major, 1 minor).

**Scope caveat:** The installed CodeRabbit CLI did not support the workflow's `--prompt-only` option, so the compatible `review --agent --base-commit f18a410` path was used against the Phase 7 directory. This is a diff-only review and was not given the source-grounding prompt. Its findings are folded into the synthesis, but its verdict is not weighted as a full plan-level vote.

1. **Major — `07-PATTERNS.md`: define one canonical durable authority, not one `Service` pointer.** Permit public path-bound and fresh authenticated descriptor-bound runtime `Service` instances, but require them to share the same underlying authority and reuse authenticated descriptors. Do not reconstruct a separate path-based authority or add a package-global singleton.

2. **Major — `07-09-PLAN.md`: protect unacknowledged local deltas before checkout/reset.** The dispatcher must publish the current shell delta before checkout or reset, or fail safely while preserving that delta. Add failed-publish-followed-by-checkout/reset coverage.

3. **Major — `07-09-PLAN.md`: the 250 ms deadline ends before parent-shell apply completes.** Extend deadline enforcement through protected eval, reverse replacement, fresh capture, and acknowledgement, or define a separately bounded shell-side sequence. Add active stall coverage for eval, capture, and acknowledgement with value-free timeout errors.

4. **Major — `07-03-PLAN.md`: multi-document state updates lack a crash-consistency contract.** Define recovery for updates spanning the canonical shared document and per-shell documents using a journal, generations, or a single atomically replaced combined document. Add crash-between-renames tests for `Acknowledge`, `ResolveShared`, and publish.

5. **Major — `07-PATTERNS.md`: `sync` cannot be direct binary dispatch.** Assign `sync` and `sync --resolve shared` to the sourced loader flow so parent-shell apply, verification, replacement reversal, and acknowledgement remain enforced.

6. **Minor — `07-10-PLAN.md`: positive command E2E cases use invalid bare arities.** Exercise `checkout <branch>` or `checkout -b <branch>` and `config set auto-apply <true|false>` in positive coverage. Move bare `checkout` and bare `config set auto-apply` to negative coverage if they must remain invalid.

7. **Major — `07-PATTERNS.md`: the private runtime contract omits operations.** Explicitly include all five operations: attach, publish, prepare, acknowledge, and resolve. Keep prepare distinct from pull, and cover attach/resolve in allowlists, exact-argument validation, deadline tests, and ownership mapping.

8. **Major — `07-05-PLAN.md`: revision reads need object and tree validation.** Require `PublishedRevision` to resolve to a commit in the expected repository and verify fixed `profile.json` and `profile.zsh` entries exist as blobs before reconstruction. Add valid-shaped blob/tree/tag and malformed-tree rejection tests.

9. **Major — `07-08-PLAN.md`: runtime results need a private exact-source channel.** Carry exact patch source bytes in a private in-memory payload alongside value-free metadata. Diagnostics and public status must expose metadata only; source bytes must not enter logs or returned value-bearing status.

---

## OpenCode Review

**Status:** Completed — full prompt-fed, repository-grounded review.

### 1. Summary

The Phase 7 plan set is unusually rigorous: it correctly identifies and reuses the codebase's two hardest-won invariants (the single-emitter zsh codegen seam in `core/shell/zsh/emit.go`, and the Git CAS/atomicity machinery in `core/store`), threads the secret-exclusion boundary consistently through all ten plans, and derives its concurrency model from an actually-executed spike. Its dependency graph is strictly serial and internally consistent, and the `must_haves` and prohibitions are unusually concrete.

However, the plans exhibit a handful of material gaps: retirement of the per-terminal `ZSHPRO_PROFILE`/`Store.Current()` current-branch authority has no single owner; two overlapping per-identity diff/projection implementations are introduced without a semantic-unification pin; opaque shell-ID derivation is unspecified even though it is an acknowledgement authority; and the same-key conflict-resolution contract is ambiguous about already-acknowledged but unpublished changes. Several symbols described as new are already present, such as `updateRefCAS`, which indicates an imperfect source-verification pass.

Overall, this is a high-quality but high-risk plan set by scale: roughly fifteen new files, a new package, a parallel runtime factory, and a hard 250 ms prompt-path deadline, all landed serially with no contract-freeze verification until the final E2E plan.

### 2. Strengths

- **The single-emitter invariant is real and correctly traced.** `emit.go` genuinely owns zsh code generation, `regen.go` only templates, and `route.go` is the conservative declarative/imperative gate. Plan 07-04's prohibition on state-mutating command assembly in `regen.go` matches the source.

- **Reverse-ownership staleness is a real hazard that the plans correctly target.** The current loader's retained reverse function and `ZSHPRO_PROFILE`/`ZP_BASE_PATH` markers describe only the originally applied profile. Plans 07-04 and 07-08 correctly require replacement reverse ownership to be produced together with forward patches.

- **The secret boundary is handled carefully without classifier duplication.** The plans route live-secret decisions through the existing classifier and repeatedly require value-free status, conflict, event, and error metadata.

- **The concurrency model reuses proven machinery.** `CommitWorktree` builds on the existing `commit-tree` plus `update-ref` CAS session, and the recovery-required outcome mirrors the hardened ingest transaction model.

- **The no-Git-on-prompt-path invariant is correctly scoped.** Publish and pull touch only private state, while Git is crossed by explicit commit, branch, checkout, and reset operations.

### 3. High Concerns

#### H1 — No plan owns retirement of `ZSHPRO_PROFILE` / `Store.Current()` as current-branch authority

Research forbids using `ZSHPRO_PROFILE` as the shared current branch and retains it only as the shell's applied marker, but no plan explicitly owns removing or redefining the old behavior. `Store.Current()` reads it; current status/list paths call the old store APIs; and loader helpers read and write it. Plan 07-09 removes loader functions and 07-06 replaces binary status, but neither states the disposition of `Store.Current()`, `Store.Checkout()`, and `Store.Create()` or whether new checkout/reset synchronization still exports the marker.

This risks two competing current-branch authorities or stale state leaking into unmigrated paths. It affects 07-06, 07-07, and 07-09.

#### H2 — Two per-identity diff/projection implementations have no unification pin

`core/worktree/diff.go` and `core/activate` both implement final-occurrence normalization, tombstone authority, ordered element comparison, and per-identity add/change/remove behavior. The plans recognize analogous code but establish neither one canonical semantic implementation nor a cross-package test proving identical behavior. The syntax emitter may remain singular while semantic diff logic drifts.

This affects 07-01, 07-04, and downstream consumers.

#### H3 — Opaque shell-ID derivation is unspecified despite being an acknowledgement authority

The plans require an opaque `ZSHPRO_SHELL_ID` but do not define its entropy source, re-source persistence, or resistance to collision/spoofing across shells. Because `shells/<opaque-id>.json` controls per-shell acknowledgement and pending resolution, predictable IDs could let one process overwrite another shell's state. Existing runtime function naming already uses `crypto/rand`; an equivalent guarantee should be explicit.

This affects 07-03, 07-09, and 07-10.

#### H4 — Same-key resolution is ambiguous for locally acknowledged but unpublished changes

Plan 07-03 says `ResolveShared` discards only the requesting shell's unacknowledged local delta. In the precmd flow, however, a shell may already have applied and locally acknowledged its own change before discovering that another publisher won the same-key race. The plan does not clearly define whether or how `ResolveShared` reconstructs the pre-conflict state and safely discards that acknowledged-but-unpublished value.

This affects 07-03 and 07-09.

### 4. Medium Concerns

- **M1 — The 250 ms deadline is derived from prototype observations rather than a production SLA.** The final path includes full capture, registry admission, JSON loading, and for some operations another `zsh -n` child. Keep active boundary tests, but validate the constant against the production path and define fail-open behavior.

- **M2 — Single-element PATH/FPATH removal extends the emitter beyond its current rebuild-to-base model.** New removal behavior must prove it removes only owned elements and remains reversible. Add a live `zsh -f` add/remove/byte-identical round-trip.

- **M3 — Legacy `Store.Create`, `Checkout`, `Current`, and `Commit` remain as an undeprecated parallel surface.** Mark their post-migration status explicitly.

- **M4 — One `profile.json` blob with old and new decoders is a subtle migration surface.** Add an old-binary/new-binary compatibility test and require malformed/partial projections to fail closed without corrupting legacy source reads.

- **M5 — Some source inventory claims are inaccurate.** `updateRefCAS` and exact-revision helpers already exist; plans should identify reuse rather than describe these as new plumbing.

### 5. Low Concerns

- **L1 — The source-AST structural test is brittle.** A source parser that rejects executable command literals in `regen.go` may resist ordinary refactors; consider a narrower invariant mechanism.

- **L2 — Interface widening needs a clear composition-root boundary.** Keep concrete zsh dependencies out of shell-agnostic packages while injecting the narrow live-secret and emitter capabilities from the composition root.

### 6. Recommended Revisions

1. Assign explicit ownership for retiring or redefining `ZSHPRO_PROFILE`, `Store.Current()`, `Store.Checkout()`, and `Store.Create()`. Prove a stale shell marker cannot override durable worktree truth.
2. Choose one diff/projection authority or add golden cross-package equivalence tests covering tombstones, present-empty values, ordered lists, and repeated entries.
3. Specify crypto-random shell IDs, idempotent re-source behavior, and cross-shell isolation tests.
4. Define `ResolveShared` behavior for the already-acknowledged losing publisher and add that exact two-shell same-key case.
5. Treat the 250 ms constant as a measured, tunable gate and cover the complete apply/capture/ack path.
6. Prove PATH/FPATH element-removal reversibility in live zsh.
7. Correct existing-symbol inventory and explicitly deprecate or remove parallel legacy store APIs.

### 7. Risk Assessment

**HIGH.** The rating is driven by the correctness core and implementation scale rather than weak planning discipline. The review would not discard the plan set, but recommends resolving H1-H4 before Wave 4 or later execution because late correction would be migration-heavy.

---

## Cursor Review

**Status:** Failed — excluded from consensus.

The adapter returned immediately with:

> Error: Authentication required. Please run `agent login` first, or set `CURSOR_API_KEY` environment variable.

No review content was produced.

---

## Ollama Review

**Status:** Failed — excluded from consensus.

The configured local model `qwen2.5vl:7b` stopped while processing the full prompt. The server returned:

> model runner has unexpectedly stopped, this may be due to resource limitations or an internal error, check ollama server logs for details

No review content was produced.

---

## Consensus Summary

Only OpenCode completed the full source-grounded review, so there is no strict multi-reviewer plan-level consensus. CodeRabbit's diff-only findings provide useful independent corroboration on several boundaries, but do not count as an equivalent grounded verdict. Claude, Cursor, and Ollama produced no review content.

### Agreed Strengths

No strength meets the strict “raised by two full prompt-fed reviewers” threshold. OpenCode nevertheless found the plan set unusually disciplined in its single-emitter ownership, secret-exclusion boundary, Git CAS reuse, and no-Git prompt path. CodeRabbit did not contradict those findings, but its diff-only scope is insufficient to turn them into consensus.

### Agreed Concerns

1. **The durable authority and migration boundary is not singular enough.** OpenCode found no explicit owner for retiring `ZSHPRO_PROFILE`/`Store.Current()` as shared truth. CodeRabbit independently rejected wording that equated one canonical authority with one `Service` pointer. Replanning should name the single durable authority, distinguish it from service instances and shell-local applied markers, and assign removal/deprecation of every competing legacy API.

2. **Shell transitions need an end-to-end safety contract, not only helper-level transactions.** OpenCode identified ambiguity when a losing same-key publisher has already applied and acknowledged its local change. CodeRabbit separately found that checkout/reset can overwrite an unpublished delta, that multi-document state changes lack crash recovery, and that the deadline stops before eval/capture/acknowledgement. Together these require one explicit state machine covering publish, prepare, protected apply, fresh capture, acknowledgement, conflict resolution, checkout/reset preconditions, and crash recovery.

3. **The runtime/sync surface is underspecified at security-critical seams.** CodeRabbit found missing private operations, incorrect direct dispatch for sync, insufficient revision-object validation, and no explicit private exact-source transport. OpenCode's concerns about shell-ID authority and duplicated semantic diff ownership reinforce the same theme: private runtime inputs, state identities, semantic transformation ownership, and source-bearing outputs all need exact contracts before implementation.

### Divergent Views

- **Overall disposition:** OpenCode rates the phase HIGH risk but would keep the plan set and require H1-H4 before later waves. CodeRabbit supplies eight additional major diff findings but no source-grounded overall verdict. With only one full reviewer, convergence is not established; the safe disposition is **replan required, not plan rejection**.

- **Deadline framing:** OpenCode questions whether 250 ms is a validated production threshold. CodeRabbit accepts the threshold's existence but says its enforcement scope is incomplete. Replanning should both measure the production path and cover the whole transition sequence.

- **Canonical service shape:** CodeRabbit explicitly permits multiple service instances over one durable authority. OpenCode focuses on eliminating competing durable/legacy authorities. These views are compatible once the plan separates authority identity from object identity.

### Replanning Priorities

1. Define the one durable worktree authority, the shell-local applied marker, service-instance rules, and the removal/deprecation owner for all legacy current/checkout/create paths.
2. Specify a crash-consistent transition state machine for unpublished local deltas, same-key losing publishers, checkout/reset, publish/prepare/apply/capture/acknowledge/resolve, and complete deadline behavior.
3. Freeze the private runtime and semantic contracts: all five operations, loader-only sync dispatch, authenticated high-entropy shell IDs, exact-source/private-metadata separation, validated commit/tree reads, and one diff/projection authority.

**Verdict: NOT CONVERGED — revise the Phase 7 plans before execution.**
