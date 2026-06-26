---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Branchable Shell Environments
status: executing
stopped_at: Phase 2 context gathered
last_updated: "2026-06-26T20:31:31.503Z"
last_activity: 2026-06-26 -- Phase 02 execution started
progress:
  total_phases: 6
  completed_phases: 1
  total_plans: 4
  completed_plans: 2
  percent: 17
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-25)

**Core value:** `checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses cleanly with zero residue, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).
**Current focus:** Phase 02 — IR + Partial Evaluation

## Current Position

Phase: 02 (IR + Partial Evaluation) — EXECUTING
Plan: 1 of 2
Status: Executing Phase 02
Last activity: 2026-06-26 -- Phase 02 execution started

Progress: [██████████] 100% (phase 1)

## Performance Metrics

**Velocity:**

- Total plans completed: 2
- Average duration: — min
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 2 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 01 P02 | 7 | 2 tasks | 2 files |

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

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

None yet.

### Blockers/Concerns

[Issues that affect future work]

- [Phase 1 gate]: Zero-residue feasibility for the alias/function/option (and any completion/keybinding/hook) delta is the single material unknown. The spike must define kill-criteria up front; if some state class is un-cleanly-reversible, narrow the managed set before the manifest is designed.
- [Phase 3 decision]: Secret-handling default in the synced tree (exclude `CatSecrets` by default vs `--include-secrets` gate) — confirm with user during planning (no encryption dependency allowed).
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

Last session: 2026-06-26T11:57:21.304Z
Stopped at: Phase 2 context gathered
Resume file: .planning/phases/02-ir-partial-evaluation/02-CONTEXT.md
