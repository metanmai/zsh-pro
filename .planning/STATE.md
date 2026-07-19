---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Branchable Shell Environments
current_phase: 04
current_phase_name: manifest-builder-emit
status: executing
stopped_at: Completed 04-07-PLAN.md
last_updated: "2026-07-19T06:53:39.136Z"
last_activity: 2026-07-19
last_activity_desc: Completed 04-06 final-identity restoration
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 23
  completed_plans: 15
  percent: 50
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-25)

**Core value:** `checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses cleanly with zero residue, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).
**Current focus:** Phase 04 — manifest-builder-emit

## Current Position

Phase: 04 (manifest-builder-emit) — EXECUTING
Plan: 7 of 13 numeric plans complete (04-07 next)
Status: Ready to execute remaining Phase 04 gap plans
Last activity: 2026-07-19 — Completed 04-06 final-identity restoration

Progress: [███████░░░] 65%

## Performance Metrics

**Velocity:**

- Total plans completed: 10
- Average duration: — min
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 2 | - | - |
| 02 | 3 | - | - |
| 03 | 3 | - | - |

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

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v2.0 pivot]: Identity corrected from "read-only analyzer" to git-versioned branchable shell-environment manager; the parse → classify → introspect engine is now the ingest component, reused via the `Provider` seam.
- [Roadmap]: Spike-first — Phase 1 de-risks zero-residue *live* hot-switch (reversing aliases/functions/**options**, not just env) before any IR/store/CLI is built; it can reshape scope and fixes the `Manifest` shape.
- [Roadmap]: IR is the spine — store, manifest, and regeneration all serialize `model.Profile`, so it lands right after the spike (store-before-IR rejected: `Store.Read`/`Commit` are typed in terms of the IR).
- [Constraint]: No new dependencies — git via the `git` binary (mirrors the existing `zsh -f` subprocess); `go-git` explicitly rejected (new module + weak porcelain).
- [Constraint]: Only `core/shell/zsh/emit.go` ever writes zsh syntax; `core/profile`/`core/store`/`core/activate` stay shell-agnostic and never import the concrete provider (single composition root preserved).
- [Phase 1 spike]: Overall verdict GO (D-03): core classes aliases/env/PATH reverse byte-identical; functions+options admitted; completion's compinit excluded to master block (fpath array admittable).
- [Phase 1 spike]: Reality-check measures the slice's declared-name VALUE delta, not a live env name-set diff — the honest measure under zsh -f env inheritance (Pitfall 5); Phase 4 emitter must follow.
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

Last session: 2026-07-19T06:53:39.129Z
Stopped at: Completed 04-07-PLAN.md
Resume file: None
