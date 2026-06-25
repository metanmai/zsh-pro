---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Branchable Shell Environments
status: planning
last_updated: "2026-06-25T05:32:32.099Z"
last_activity: 2026-06-25
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-24)

**Core value:** `analyze --json` reports output you can trust — every PATH entry is named exactly as written, genuine duplicates are caught across notations, and risky entries are flagged without polluting the exit-code signal.
**Current focus:** Phase 3 — trustworthy path extraction & detection

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining requirements
Last activity: 2026-06-25 — Milestone v2.0 started

## Performance Metrics

**Velocity:**

- Total plans completed: 5 (all in v1.0 / Phase 1)
- Average duration: ~5 min
- Total execution time: ~0.2 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 (v1.0) | 3 | ~14 min | ~5 min |
| 2 (v1.1) | 0 | - | - |
| 3 (v1.1) | 0 | - | - |
| 4 (v1.1) | 0 | - | - |
| 02 | 2 | - | - |

**Recent Trend:**

- Last 5 plans: 01-01 (4 min) · 01-02 (1 min) · 01-03 (9 min)
- Trend: Stable

*Updated after each plan completion*
| Phase 02 P01 | 2 | 3 tasks | 3 files |
| Phase 02 P02 | 3min | 3 tasks | 4 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v1.1 scope]: Broad option — fix PATH extraction + semantic dedup + relative-entry advisory + severity tier + close `duplicate_path`/`shadowed` coverage.
- [v1.1]: PATH dedup is semantic but **notation-only** (`~`/`$HOME`/`${HOME}` + slashes; no filesystem/env resolution).
- [v1.1]: Relative-entry advisory is a new **informational severity**, not exit-3 — `SevActionable` is the zero value so the 4 existing kinds stay byte-identical.
- [Roadmap]: Strict A→B→C order — Phase 2 (severity) MUST precede Phase 3 (PATH) so advisory-only configs never transiently exit 3.
- [Phase ?]: [02-01]: SevActionable is the zero value (int+iota) so the 4 existing issue kinds keep exit-3 with zero construction-site edits (D-01).
- [Phase ?]: [02-01]: ExitCode() consults Analysis.HasActionableIssues() (single actionable predicate); len(a.Issues) no longer drives the exit code (D-04).
- [Phase ?]: [02-02]: --json issues carry a non-omitempty 'severity' string (mapped via Severity.String()); dto stays string-only (D-02).
- [Phase ?]: [02-02]: envelope issues_found now derives from HasActionableIssues() so it can never disagree with exit_code; advisory-only is clean (false/0) — SEV-02 (D-04).
- [Phase ?]: [02-02]: human report uses softer '~' for advisories in the single ISSUES list + a separate advisory tally (D-05/D-06).

### Pending Todos

None yet.

### Blockers/Concerns

- **[Phase 2 gate]** Confirm with the user that `issues_found` redefined as "actionable-only" is an acceptable `analyze --json` wire-contract change (advisory-only envelope flips `issues_found:true→false`, `exit_code` stays 0).
- **[Phase 3 design prerequisite → Phase 4]** The oracle's two-field path-node split (rendered notation vs. independent dedup key) MUST be designed before the Phase 3 canonicalizer is written — retrofitting oracle independence is the highest recovery cost in the research (Pitfall #8, HIGH).
- **[Phase 3 open questions]** Extraction location (provider vs. reconciler-local), intra-statement duplicate policy (`PATH="/a:/a:$PATH"`), and the `CatPath` array-form classification seam check are unresolved — carry into Phase 3 planning, do not silently resolve.
- Correcting PATH `name`/`lines` and adding the `severity` field + `relative_path_entry` kind is a deliberate, accepted `analyze --json` wire-contract change.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-06-24T15:56:41.633Z
Stopped at: Phase 2 context gathered
Resume file: None

## Operator Next Steps

- Plan the first phase with `/gsd:plan-phase 2`
