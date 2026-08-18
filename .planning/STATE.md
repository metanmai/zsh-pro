---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Branchable Shell Environments
current_phase: 07
current_phase_name: Git-Like Shared Working Environment
status: executing
stopped_at: Completed 07-11-PLAN.md
last_updated: "2026-08-18T01:07:36.781Z"
last_activity: 2026-08-18
last_activity_desc: Phase 07 execution started
progress:
  total_phases: 6
  completed_phases: 6
  total_plans: 40
  completed_plans: 40
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-25)

**Core value:** `checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses cleanly with zero residue, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).
**Current focus:** Phase 07 — Git-Like Shared Working Environment

## Current Position

Phase: 07 (Git-Like Shared Working Environment) — EXECUTING
Plan: 2 of 15
Status: Ready to execute
Last activity: 2026-08-18 — Phase 07 execution started

Progress: [██████████] 100%

## Performance Metrics

**Velocity:**

- Total plans completed: 40
- Average duration: — min
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 02 | 3 | - | - |
| 03 | 3 | - | - |
| 04 | 19 | - | - |
| 5 | 8 | - | - |
| 6 | 5 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 01 P02 | 7 | 2 tasks | 2 files |
| Phase 02 P01 | 18 | 3 tasks | 8 files |
| Phase 02 P02 | 5 | 3 tasks | 8 files |
| Phase 02 P03 | 11 | 3 tasks | 8 files |
| Phase 03 P01 | 9 | 3 tasks | 8 files |
| Phase 03 P02 | 9 | 2 tasks | 6 files |
| Phase 03 P03 | 35 | 4 tasks | 5 files |
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 04 P03 | 13min | 3 tasks | 11 files |
| Phase 04 P04 | 12min | 2 tasks | 9 files |
| Phase 04 P05 | 18min | 2 tasks | 1 files |
| Phase 04 P06 | 74 min | 3 tasks | 10 files |
| Phase 04 P07 | 3 min | 2 tasks | 4 files |
| Phase 04 P10 | 4 min | 2 tasks | 2 files |
| Phase 04 P08 | 18 min | 3 tasks | 9 files |
| Phase 04 P11 | 16 min | 3 tasks | 7 files |
| Phase 04 P09 | 18 min | 3 tasks | 10 files |
| Phase 04 P12 | 31 min | 3 tasks | 2 files |
| Phase 04 P13 | 3 min | 3 tasks | 7 files |
| Phase 04 P14 | 8 min | 3 tasks | 9 files |
| Phase 04 P15 | 29 | 3 tasks | 12 files |
| Phase 04-manifest-builder-emit P16 | 6 | 2 tasks | 12 files |
| Phase 04 P17 | 8 | 2 tasks | 3 files |
| Phase 04-manifest-builder-emit P18 | 7 | 2 tasks | 11 files |
| Phase 05 P01 | 8 min | 4 tasks | 11 files |
| Phase 05 P02 | 10 min | 3 tasks | 8 files |
| Phase 05 P03 | 19min | 3 tasks | 5 files |
| Phase 05 P04 | 36 min | 2 tasks | 4 files |
| Phase 05 P05 | 9min | 2 tasks | 7 files |
| Phase 06 P01 | 585min | 3 tasks | 10 files |
| Phase 06 P02 | 38m | 2 tasks | 6 files |
| Phase 06 P03 | 48m | 3 tasks | 12 files |
| Phase 06 P04 | 27m | 3 tasks | 6 files |
| Phase 06 P05 | — | 3 tasks | — |
| Phase 07-git-like-shared-working-environment P01 | 13min | 2 tasks | 4 files |
| Phase 07-git-like-shared-working-environment P02 | 13min | 2 tasks | 5 files |
| Phase 07 P03 | 46min | 3 tasks | 8 files |
| Phase 07-git-like-shared-working-environment P04 | 20min | 2 tasks | 8 files |
| Phase 07 P05 | 26min | 2 tasks | 6 files |
| Phase 07 P08 | 30m | 2 tasks | 6 files |
| Phase 07 P06 | 29m | 2 tasks | 6 files |
| Phase 07-git-like-shared-working-environment P07 | 25m | 2 tasks | 7 files |
| Phase 07-git-like-shared-working-environment P09 | 65m | 2 tasks | 6 files |
| Phase 07 P10 | 59min | 2 tasks | 16 files |
| Phase 07 P11 | 17min | 2 tasks | 6 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v2.0 pivot]: Identity corrected from "read-only analyzer" to git-versioned branchable shell-environment manager; the parse → classify → introspect engine is now the ingest component, reused via the `Provider` seam.
- [Roadmap]: Spike-first — Phase 1 de-risks zero-residue *live* hot-switch (reversing aliases/functions/**options**, not just env) before any IR/store/CLI is built; it can reshape scope and fixes the `Manifest` shape.
- [Roadmap]: IR is the spine — store, manifest, and regeneration all serialize `model.Profile`, so it lands right after the spike (store-before-IR rejected: `Store.Read`/`Commit` are typed in terms of the IR).
- [Constraint]: No new dependencies — git via the `git` binary (mirrors the existing `zsh -f` subprocess); `go-git` explicitly rejected (new module + weak porcelain).
- [Constraint]: Only `core/shell/zsh/emit.go` ever writes zsh syntax; `core/profile`/`core/store`/`core/activate` stay shell-agnostic and never import the concrete provider (single composition root preserved).
- [Phase ?]: [02-01]: IR value-capture via additive Block.Value/Dynamic at parse time (Approach A); static/dynamic detection in the core/shell/zsh AST tier keeps core/ir shell-free
- [Phase ?]: [02-01]: routeManaged ING-02 gate admits 5 reversible classes; bare setopt/unsetopt routes imperative (#2); confidence never gates routing (D-06); ManagedOverride wins over auto verdict (D-07)
- [Phase 02]: [02-03] Array assignments route imperative (verbatim Text), not templated — additive Block.Array flag detected at parse time; same D-04/D-06 lineage as BL-02/WR-01/WR-02
- [Phase ?]: [03-01] SecretRef placement = Entry.Secret *SecretRef (omitempty) with cleared Value when set; additive dependency-free in core/model (critical decision #2)
- [Phase ?]: [03-01] Profile<->JSON serialization via a store-local DTO (entryDTO/profileDTO), not json tags on model.Entry — keeps the Phase 2 IR struct pure (critical decision #1, option b)
- [Phase ?]: [03-01] git driver mirrors introspect.go subprocess shape verbatim (5s timeout + LookPath guard + degrade); no go-git, no new dependency (PROF-01, critical decision #4)
- [Phase ?]: [03-01] mapGitError maps every git failure to ErrGitCommand and never embeds raw stderr; all store errors are zsh-pro-phrased errStore sentinels (D-11)
- [Phase ?]: [03-02] Commit is plumbing-to-branch (temp index -> write-tree -> commit-tree -> update-ref), zero checkout — the conda concurrent-activation race avoided by construction (D-12, critical decision #4)
- [Phase ?]: [03-02] Active profile per-terminal via ZSHPRO_PROFILE (name only; unset => main); Checkout validates existence but does NOT export (export is Phase 5 loader) (D-13)
- [Phase ?]: [03-02] New/Commit/KeychainDriver/WithheldReport declared in FINAL form this wave so Plan 03 changes no signature and edits no test in this plan
- [Phase ?]: [03-02] UnmarshalProfile preserves nil Entries for an entry-less profile (mirrors cloneNames) so Read(Commit(model.Profile{})) is reflect.DeepEqual to its input (Rule 1 fix)
- [Phase ?]: [03-03] excludeSecrets removes the literal from BOTH Entry.Text AND Entry.Value — Text is the JSON text field + the Regenerator empty-Value fallback, so clearing only Value leaked into profile.json AND profile.zsh (T-03-03 Rule 1 fix); both get an inert kind:key placeholder, Secret is authoritative
- [Phase ?]: [03-03] SecretRef.Kind = kc.Kind() from the live backend (not hardcoded) so the vault fallback yields a file-kind reference Ph4/5 derefs from the vault (T-03-09)
- [Phase ?]: [03-03] D-08 literal-vs-dynamic split reuses the shipped CatSecrets verdict + parser Dynamic flag (no new regex/AST); store stays shell-agnostic; main.go is the sole core/shell/zsh importer; PROF-03 store-side half done, completes Ph6
- [Phase 04]: ValueModeLegacy is the only state allowed to use the historical Value and Dynamic fallback. — New parser output is always explicit, while older stored profiles remain backward compatible.
- [Phase 04]: Literal decoding uses an AST allowlist and marks every unmodeled static form Unsupported. — Fail-closed parsing prevents source syntax from being mistaken for safe runtime data.
- [Phase 04]: Function bodies use pointer presence and strip braces only for plain unmodified block statements. — Empty bodies stay distinct from missing bodies and behavior-bearing modifiers remain intact.
- [Phase 04]: Secret exclusion clears RuntimeValue and marks redacted entries Unsupported while SecretRef remains authoritative. — Decoded literals must not bypass the existing store redaction boundary or activate placeholders.
- [Phase 04]: Explicit manifest provenance is authoritative even when false; only absent provenance uses legacy heuristic fallback.
- [Phase 04]: Function body map-key presence distinguishes a valid empty function from missing activation data.
- [Phase 04]: The complete residue oracle uses sorted type-aware live dereferencing and qqqq field encoding. — This keeps arbitrary NUL, newline, backslash, and metacharacter data collision-safe without decoding snapshots.
- [Phase 04]: Renderer negative controls assert their intended failure modes and restore seams with cleanup before a real-emitter rerun. — This proves the residue test is falsifiable and cannot pass because a mutant leaked into later tests.
- [Phase 04]: Final manifest identities own restoration; explicit scalar export provenance and presence-aware hex slots preserve exact prior shell state.
- [Phase 04]: Source status is captured before protocol emission, so any source failure makes introspection unavailable. — Prevents later successful print commands from masking missing, invalid, or explicitly failing configurations.
- [Phase 04]: The line parser receives only the identity prefix; alias and function bodies stay NUL-framed payloads. — Exact marker-shaped body lines cannot be reinterpreted as protocol section controls.
- [Phase 04]: A semantic PATH or FPATH list is present only with one same-list Self marker; nil preserves legacy and unsupported entries. — Activation must never infer a base reference from raw source or a cross-list expansion.
- [Phase 04]: Dynamic list additions retain validated simple-parameter source as scalar expressions, with zsh deciding zero, one, or many elements at runtime. — Pre-splitting dynamic text would freeze runtime cardinality and violate tied-list semantics.
- [Phase 04]: Secret preparation is side-effect free; Commit snapshots all keys, writes the backend before a CAS ref update, and compensates only its own visible ref result. — This preserves exact prior backend state and never overwrites concurrent ref movement.
- [Phase 04]: The final residue oracle uses accepted source fixtures through Parse, IR, Build, Diff, and Emit; observable PATH/FPATH presence and qqqq-encoded multiplicity remain explicit while emitter bookkeeping stays internal.
- [Phase ?]: Unmodeled declaration commands stay verbatim-imperative; only ValueModeLegacy may use same-list raw list fallback; multi-name functions reduce atomically.
- [Phase ?]: 04-14: Legacy PATH/FPATH entries now flow through the semantic token reducer, preserving source order and provenance.
- [Phase ?]: 04-14: Persisted forced declarations require representability and otherwise regenerate verbatim with no manifest intent.
- [Phase ?]: 04-15: Structural syntax markers must remain presence-aware across DTO persistence; unknown historical state fails closed.
- [Phase ?]: 04-15: Structural syntax markers are presence-aware; absent, partial, and unsupported DTO fidelity remains unknown and verbatim/no-operation.
- [Phase ?]: 04-15: Legacy PATH/FPATH additions retain late binding only for strict simple parameter plus path-safe suffix source.
- [Phase ?]: Structural fidelity is known only for complete v2 records; absent, v1, partial, and unsupported DTO data re-saves unknown.
- [Phase ?]: OverrideManaged cannot erase indexed or declaration-attribute semantics; rejected entries remain verbatim and create no manifest intent.
- [Phase ?]: 04-17: export -- is a non-semantic delimiter but its following assignment remains a supported exported scalar.
- [Phase ?]: 04-17: persisted rejected indexed and flagged declarations are proven by type-aware live-zsh and full-state residue tests.
- [Phase ?]: V3 structural fidelity preserves alias and option source shape; v1/v2 and partial records fail closed.
- [Phase ?]: Forced management requires exact assignment, assigned alias, and modeled option syntax before lowering.
- [Phase ?]: Phase 4 emit blocks keep their scalar helpers self-contained; the Phase 5 loader only owns named env helpers and terminal base state.
- [Phase ?]: The Phase 5 composition root injects the real Phase 4 runtime emitter, retaining NotReadyEmitter only as a fail-closed fallback.
- [Phase ?]: Store initialization failure becomes an explicit nil cli.Store interface so only store-backed verbs fail rather than panic.
- [Phase ?]: 05-02: Installer and stub share ZSHPRO_HOME or HOME/.zsh-pro; cached loader skew is explicit and refreshed by install.
- [Phase ?]: 05-02: Switches validate one emitted block and report runtime partial failure; last-good advances only after success.
- [Phase ?]: Installer markers are exact physical lines; marker-like user content is never managed.
- [Phase ?]: Install paths require an absolute HOME and absolute configured ZDOTDIR/ZSHPRO_HOME before filesystem mutation.
- [Phase ?]: Cached loaders validate in a private same-directory candidate with a five-second deadline before rename.
- [Phase ?]: Hyperfine means are decoded by dependency-free Go rather than Bash regexes.
- [Phase ?]: Runtime subprocesses use a zsh-native child plus watchdog and bounded 1-99 second timeout input rather than GNU timeout.
- [Phase ?]: Expected public verb failures report through ZP_LAST_RUNTIME_STATUS/ZP_LAST_RUNTIME_ERROR and return zero to preserve an interactive ERR_EXIT or ERR_RETURN caller.
- [Phase ?]: The runtime emitter owns the complete executable transition, so the loader validates and evaluates one deactivate-then-apply source exactly once.
- [Phase ?]: Runtime SecretRefs resolve only on an activation copy after backend-kind validation, preserving persisted redaction.
- [Phase ?]: Nil-like runtime dependencies are normalized at CLI constructors so public verbs fail through cli.fail instead of panicking.
- [Phase ?]: Only update-ref prepare acknowledgement establishes the expected-ref lock; backend and final-object effects occur afterward, and commit is a separate final write.
- [Phase ?]: Publication truth and cleanup evidence remain independent, including when commit-response observation or authenticated cleanup requires recovery.
- [Phase ?]: Persisted SecretRefs are structurally validated before one backend-kind comparison and never trigger Retrieve, Store, or Delete.
- [Phase ?]: Legacy branch commits use a private validated-ref constructor and the shared ingest state machine without widening public main-only BeginIngest.
- [Phase ?]: 06-03: expectedTarget remains the original bounded snapshot while expectedCandidate comes only from independently durable peer evidence.
- [Phase ?]: 06-03: one private exchange peer owns candidate, displaced occupant, and guarded reverse states; absent post-create rollback remains recovery-required.
- [Phase ?]: 06-03: pure adapter checks and a real same-filesystem exchange/no-replace probe precede Store, cache, loader, and target effects.
- [Phase ?]: [06-04]: Persist the complete source-ordered Profile; EffectiveManaged is an inspection-only projection and never filters Store input.
- [Phase ?]: [06-04]: Filesystem restore-or-retain precedes Store, loader, cache, and initializer compensation; recovery uncertainty disarms later destructive steps.
- [Phase ?]: [06-04]: A committed ingest finalizes the existing guarded promotion and never rewrites the startup target after commit.
- [Phase ?]: [06-04]: One concrete Store pointer owns initialization IDs and every ingest transaction operation.
- [Phase ?]: Live values use explicit presence plus a kind-selected payload so present-empty remains distinct from removal.
- [Phase ?]: Model normalization and equality are the sole live-state semantic authority; diff and activation consumers must delegate to them.
- [Phase 07]: Live secret identity decisions delegate to zsh.Provider.Classify so secretRe remains the sole classifier authority. — The same ingest and runtime identity policy cannot drift.
- [Phase 07]: Registry ownership uses final EffectiveManaged and Representable source occurrences while SecretRefs remain metadata-owned but live-value-ineligible. — Source shadowing and resolved secret literals must not enter shared state.
- [Phase 07]: Admission requires exact absence from an attachment issued by the same registry and returns value-free stable exclusions. — Ambient inherited state must never become managed implicitly.
- [Phase 07]: Missing or typed-nil LiveSecretPolicy injection fails closed with policy-missing. — No captured value may cross the boundary without a production secret verdict.
- [Phase 07]: Persist only capability verifiers and authenticate requests in constant time inside the state transaction — Raw credentials must never reach canonical state, receipts, errors, or forensic output.
- [Phase 07]: Use one descriptor-relative atomic worktree.json generation under one fixed lock — Shared, shell, pending, receipt, conflict, and event authority must not split across crash-consistency domains.
- [Phase 07]: Retain overlapping loser deltas and conflicts until exact acknowledgement — First-lock-wins arbitration must not silently discard local intent or clear evidence before apply verification.
- [Phase 07]: Duplicate and re-authenticate StateStore directory descriptors — The store owns its lifecycle while remaining bound to the authenticated directory despite path replacement.
- [Phase 07]: Keep core/model normalization and equality authoritative; pin activate traversal to worktree diff by external golden test because a direct import would create a cycle.
- [Phase 07]: Carry exact before/after PATH and FPATH endpoints so replacement reverse restores tied scalar and array state byte-identically.
- [Phase 07]: Keep committed-worktree validation and traversal in regen.go while emit.go solely owns all added executable zsh syntax.
- [Phase 07]: Keep legacy source entries at the historical top-level wire location and add one presence-aware versioned worktree projection.
- [Phase 07]: Authenticate exact worktree revisions as direct commit IDs with a complete strict two-blob root tree.
- [Phase 07]: Derive the narrow WorktreeRegenerator from the injected provider while preserving the source-compatible Store constructor.
- [Phase 07]: Lock the expected ref transaction before secret writes and preserve proven committed recovery evidence.
- [Phase 07]: Capture current zsh in place with a sourced builtin-only function; retain child zsh only for file introspection.
- [Phase 07]: Keep executable patch source unexported and public RuntimePatchMetadata limited to uint64 fields and a fixed SHA-256 fingerprint.
- [Phase 07]: Make concrete zsh the sole owner of the complete ordered acknowledgement assignment footer.
- [Phase 07]: Resolve decimal reply token handles through descriptor-bound durable pending state before Service transactionally verifies the opaque token and credential.
- [Phase ?]: [07-06]: Durable State.Branch/BaseOID/HeadRevision are the sole shared workflow authority; process-local profile markers never select workflow state.
- [Phase ?]: [07-06]: Commit every semantic dirty identity through BuildEffective and claim clean only after exact published-object readback plus canonical save.
- [Phase ?]: [07-06]: Observed-ref repair is allowed only when the exact committed projection already equals locked shared state; different external projections are never imported implicitly.
- [Phase ?]: [07-06]: Validate full Git-shaped argv and optional ZSHPRO_SHELL_ID before access; use the ID only for shell status lookup and render value-free output.
- [Phase ?]: [07-07]: PublishedRevision comes only from exact candidate publication evidence and is never inferred from a later ref lookup.
- [Phase ?]: [07-07]: Ingest publishes a versioned combined worktree DTO; legacy source-only objects remain readable only through the legacy source decoder.
- [Phase ?]: [07-07]: Public worktree authority is lazy path-bound state, while runtime Services remain late-bound to authenticated descriptors with the same concrete zsh policy.
- [Phase ?]: [07-07]: Existing-generation materialization delegates to exact under-lock Service repair and succeeds only when the published projection equals shared state.
- [Phase ?]: [07-09]: Carry every private credential and operation field in one exact bounded ZPWT v1 stdin frame.
- [Phase ?]: [07-09]: Publish returns only bounded value-free conflict identity and durable token metadata needed for explicit shared resolution.
- [Phase ?]: [07-09]: One sealed 250 ms absolute budget spans runtime work and parent eval, fresh capture, and acknowledgement.
- [Phase ?]: [07-09]: Legacy fail-open routing is limited to unsupported helpers, already-active legacy shells, or deliberately replaced dispatchers.
- [Phase ?]: Canonicalize top-level live identity record order only for comparison fingerprints while preserving exact ordered-list and value semantics.
- [Phase ?]: Use the current captured parent shell as the patch base while retaining applied state as causal acknowledgement authority.
- [Phase ?]: Update retained config and mutation state only after exact successful production commands and preserve delegated failure codes.
- [Phase 07]: Admitted ownership persists only exact kind/name metadata; live values and shell credentials remain outside the ownership field.
- [Phase 07]: Fresh registries restore durable admissions only after the existing identity and live-secret policy revalidates the complete set.
- [Phase 07]: Legacy field absence migrates only safe non-source identities already present in canonical Shared, never ShellState or caller snapshots.

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

None yet.

### Blockers/Concerns

[Issues that affect future work]

- [Phase 1 gate]: Zero-residue feasibility for the alias/function/option (and any completion/keybinding/hook) delta is the single material unknown. The spike must define kill-criteria up front; if some state class is un-cleanly-reversible, narrow the managed set before the manifest is designed.
- [Phase 3 decision — RESOLVED 2026-06-27]: Secret handling settled in 03-CONTEXT.md as a **reference/deref model** (not a plain exclude/gate): literal secrets → `SecretRef` (resolver-agnostic `kind:key`, keychain-default + git-ignored file fallback); already-dynamic secrets commit verbatim; full store-side exclusion built in Ph3, deref-on-switch in Ph4/5. Elevates PROF-03 — planner to update REQUIREMENTS traceability (PROF-03 starts Ph3).
- [Phase 5 / deferred]: Trust model for shared/`git pull`'d profiles (SHARE-01) is out of scope for v2.0 (single-user local); revisit if a team/shared store enters scope.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| Analyzer | v1.1 Phase 3 — Trustworthy PATH Extraction & Detection (AST split + notation dedup + relative advisory) | Re-scoped into v2.0 ingest (ING-01) | 2026-06-25 (v2.0 pivot) |
| Analyzer | v1.1 Phase 4 — PATH Coverage & Oracle Pin (COV-01/02/03) | Parked | 2026-06-25 (v2.0 pivot) |
| Ergonomics | AUTO-01 — auto-activate on `cd` (direnv-style hook) | Future | 2026-06-25 |
| Sharing | SHARE-01 — pull/push profiles from a remote with a trust gate | Future | 2026-06-25 |

## Session Continuity

Last session: 2026-08-18T01:07:36.774Z
Stopped at: Completed 07-11-PLAN.md
Resume file: None
