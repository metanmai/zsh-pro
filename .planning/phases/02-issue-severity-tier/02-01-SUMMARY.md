---
phase: 02-issue-severity-tier
plan: 01
subsystem: api
tags: [go, enum, iota, exit-code, agent-contract, domain-model]

# Dependency graph
requires:
  - phase: 01-line-number-fix
    provides: "reviewed-clean model.Issue / model.Analysis types and the typed ExitCode contract this plan extends"
provides:
  - "model.Severity type (int+iota) with SevActionable as the zero value and SevAdvisory second"
  - "Severity.String() returning the self-describing wire labels 'actionable' / 'advisory'"
  - "Issue.Severity field — omitting it yields SevActionable, so the four existing issue kinds keep exit-3 behavior with zero construction-site edits"
  - "Analysis.HasActionableIssues() — the single actionable predicate; ExitCode() consults it instead of counting raw issues"
affects: [02-02-wire-and-render, 03-relative-path-advisory, 04-coverage]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Typed-int enum with iota where the FIRST constant is the safe default-by-omission (zero value), mirroring core/model/block.go Confidence"
    - "Model type owns its string projection via a String() method (mirrors Category.Description()) so dto stays string-only and both renderers share one mapping"
    - "Single shared predicate on the domain type as the sole source of truth for the exit signal (HasActionableIssues consumed by ExitCode; 02-02 will reuse it for issues_found)"

key-files:
  created:
    - .planning/phases/02-issue-severity-tier/02-01-SUMMARY.md
  modified:
    - core/model/issue.go
    - core/model/analysis.go
    - core/model/model_test.go

key-decisions:
  - "SevActionable is the zero value (int+iota, declared first) so no Issue{} literal in the reconciler or testgen needs editing — D-01"
  - "ExitCode() consults HasActionableIssues() rather than len(a.Issues); the old raw-count predicate is fully removed from analysis.go — D-04"
  - "Severity.String() default arm returns 'actionable' so any future-unset value degrades to the safe, exit-bumping label"

patterns-established:
  - "Pattern 1: typed-int enum + iota with zero-value-as-default + String() projection (the Severity type)"
  - "Pattern 3: single actionable predicate on Analysis shared by the exit-code logic (and, in 02-02, issues_found)"

requirements-completed: [SEV-01, SEV-02]

# Metrics
duration: 2min
completed: 2026-06-24
---

# Phase 2 Plan 1: Issue Severity Tier (Domain Layer) Summary

**Two-value `model.Severity` enum (int+iota, `SevActionable`=zero) with `String()`, an `Issue.Severity` field, and an `Analysis.HasActionableIssues()` predicate that `ExitCode()` now consults — advisories no longer bump exit 3, while the four existing issue kinds stay byte-identical with zero construction-site edits.**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-06-24T15:45:08Z
- **Completed:** 2026-06-24T15:47Z
- **Tasks:** 3 (2 implementation + 1 verification gate)
- **Files modified:** 3

## Accomplishments
- Added `type Severity int` with `SevActionable Severity = iota` (zero value) and `SevAdvisory`, plus `Severity.String()` returning the locked wire labels `"actionable"` / `"advisory"` (SEV-01).
- Added the `Severity` field to `model.Issue`; omitting it yields `SevActionable`, so every existing `Issue{}` literal in the reconciler and testgen stays correct with no edits (D-01, verified: `core/analyze` and `core/testgen` untouched).
- Added `Analysis.HasActionableIssues()` as the single actionable predicate and rewired `ExitCode()` to consult it — `len(a.Issues)` no longer drives the exit code (SEV-02 / D-04). An advisory-only `Analysis` now exits `ExitClean`; actionable or mixed exits `ExitActionable`.
- Pinned the contract with `TestSeverityString`, `TestHasActionableIssues`, and an extended table-driven `TestExitCode` that also asserts the invariant `(ExitCode()==ExitActionable) == HasActionableIssues()`.
- Confirmed the whole-repo regression: `go test ./...`, `go vet ./...`, `gofmt -l`, and `make check` (fmt-check + vet + lint + test, 0 lint issues) are all green with edits confined to `core/model/` — the "zero fixture/construction-site churn" claim (research Pitfall 4) holds.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the Severity type, String() method, and Issue.Severity field** - `0eb6b24` (feat) — TDD: failing `TestSeverityString` (RED, build-failed) then minimal implementation (GREEN) committed together as one additive feature.
2. **Task 2: Add HasActionableIssues() and make ExitCode() consult it** - `c658c08` (feat) — TDD: failing `TestHasActionableIssues` + extended `TestExitCode` (RED, build-failed) then predicate + `ExitCode()` rewrite (GREEN).
3. **Task 3: Confirm full model package + whole-suite regression is green** - verification-only gate, no production edits → no commit (whole suite + vet + gofmt + `make check` all green).

**Plan metadata:** see final `docs(02-01)` commit.

_Note: Tasks 1 and 2 are TDD; RED was confirmed (compile failure on the undefined symbols) before GREEN. RED and GREEN were committed together per task since each is a single additive symbol whose test cannot compile without the symbol._

## Files Created/Modified
- `core/model/issue.go` - Added `type Severity int`, the `SevActionable`/`SevAdvisory` const block, `Severity.String()`, and the `Severity` field on `Issue` (placed after `Note`).
- `core/model/analysis.go` - Added `HasActionableIssues()`; changed `ExitCode()` body from `if len(a.Issues) > 0` to `if a.HasActionableIssues()`.
- `core/model/model_test.go` - Added `TestSeverityString` and `TestHasActionableIssues`; converted/extended `TestExitCode` to a table covering clean / actionable / advisory-only / mixed plus the exit-code↔predicate agreement invariant.

## Decisions Made
None beyond the locked decisions — followed the plan and CONTEXT.md (D-01..D-04) exactly:
- `int`+`iota` chosen over a string-typed enum (research-recommended) so the zero value is *genuinely* `SevActionable` with no empty-string special-casing — mirrors the in-repo `Confidence` precedent.
- Used a named `HasActionableIssues()` predicate (the discretionary choice the research marked marginally cleaner) so `ExitCode()` and the future `issues_found` flag share one source of truth.
- The `String()` `default:` arm returns `"actionable"` deliberately, so a future-unset/unhandled severity degrades to the safe, visible, exit-bumping label.

## Deviations from Plan

None - plan executed exactly as written. No bugs, missing functionality, or blocking issues were encountered; no construction sites, fixtures, or non-model tests were touched (confirmed via `git diff --name-only`).

## Issues Encountered
None.

## Known Stubs
None - this plan delivers a complete internal enum + predicate with full test coverage. No placeholder values, mock data sources, or unwired components; advisory severity is *consumed* (set to `SevAdvisory`) starting in Phase 3, but the mechanism itself is fully functional and the `SevAdvisory`→`ExitClean` path is exercised by hand-constructed unit tests now.

## Threat Surface
No new trust boundary or external-input surface (per the plan's threat model: an internal domain enum + a pure predicate; `Severity` is set internally, never parsed). The signal-integrity mitigation `T-02-01` is delivered exactly as planned — the `HasActionableIssues()` predicate is the sole source of truth and the SEV-02 truth-table tests pin that advisories can neither fabricate nor suppress the exit-3 signal an agent depends on. No threat flags raised.

## User Setup Required
None - no external service configuration required; `go.mod`/`go.sum` unchanged (no new dependencies).

## Next Phase Readiness
- **Ready for 02-02 (wire + render):** the symbols 02-02 compiles against now exist — `model.Severity`, `SevActionable`/`SevAdvisory`, `Severity.String()`, `Issue.Severity`, and `Analysis.HasActionableIssues()`. 02-02 will add `dto.Issue.Severity` (non-omitempty), map it in `toDTO` via `.String()`, set `IssuesFound: a.HasActionableIssues()`, and add the human marker (`~`) + advisory tally (D-02/D-05/D-06).
- **No blockers.** The wire-contract change (issue gains `severity`; `issues_found`/`exit_code` are actionable-only) is inert for every existing config until Phase 3 plants the first `SevAdvisory`.

## Self-Check: PASSED

- Files verified present: `core/model/issue.go`, `core/model/analysis.go`, `core/model/model_test.go`, `.planning/phases/02-issue-severity-tier/02-01-SUMMARY.md`
- Commits verified in git history: `0eb6b24` (Task 1), `c658c08` (Task 2)

---
*Phase: 02-issue-severity-tier*
*Completed: 2026-06-24*
