---
phase: 02-issue-severity-tier
plan: 02
subsystem: api
tags: [go, json, wire-contract, agent-contract, render, dto, severity]

# Dependency graph
requires:
  - phase: 02-issue-severity-tier
    plan: 01
    provides: "model.Severity (+SevActionable zero / SevAdvisory), Severity.String(), Issue.Severity field, Analysis.HasActionableIssues() — the symbols this plan threads onto the wire and into the human report"
provides:
  - "dto.Issue.Severity — a non-omitempty 'severity' wire field present on every --json issue ('actionable' | 'advisory')"
  - "toDTO maps Severity via is.Severity.String() (model stays the single source of the int→string mapping; dto stays string-only)"
  - "Envelope issues_found now derives from a.HasActionableIssues() — it can never disagree with exit_code (advisory-only ⇒ false/0; actionable ⇒ true/3)"
  - "Human report: severity-aware per-issue glyph (! actionable / ~ advisory) in the single ISSUES list, plus a separate advisory tally in the summary"
affects: [03-relative-path-advisory, 04-coverage]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Wire field threaded manually through toDTO (model↔dto separation): dto.Issue gains a string field, render maps model.Severity→string via .String(); dto imports no model — mirrors the existing Kind: string(is.Kind) move"
    - "Single actionable predicate as the sole source of truth for BOTH the exit signal and the issues_found flag (HasActionableIssues consumed by ExitCode in 02-01 and by IssuesFound here) so they cannot drift"
    - "Presentation-only severity branch lives in human.go only; json.go emits exactly one marshaled envelope and adds no print/log (one-JSON-object agent contract preserved)"

key-files:
  created:
    - .planning/phases/02-issue-severity-tier/02-02-SUMMARY.md
  modified:
    - core/dto/analysis.go
    - core/render/json.go
    - core/render/human.go
    - core/render/render_test.go

key-decisions:
  - "Advisory marker glyph = '~' (Claude's discretion per A1/D-05) — a softer, single, non-blocking char distinct from the actionable '!'"
  - "Advisory tally wording = 'N issues, M advisories' rendered as a closing line of the ISSUES section (discretionary per A1/D-06) — advisories numerically visible but not folded into the actionable issue count"
  - "Reused the 02-01 HasActionableIssues() predicate for IssuesFound rather than re-deriving an actionable count in render (Pattern 3 / Pitfall 2) so issues_found and exit_code share one source"
  - "dto.Issue.Severity is non-omitempty (D-02) so agents can switch on it unconditionally; dto stays a string-only leaf (no core/model import)"

patterns-established:
  - "Pattern 2 (research): wire field threaded manually through toDTO with the int→string mapping invoked at the render seam"
  - "Pattern 4 (research): severity-aware human report — glyph-by-severity in one list + separate advisory tally"

requirements-completed: [SEV-01, SEV-02]

# Metrics
duration: 3min
completed: 2026-06-24
---

# Phase 2 Plan 2: Issue Severity Tier (Wire + Render) Summary

**Surfaced the 02-01 severity tier across both output surfaces: every `--json` issue now carries a non-omitempty `severity` string (mapped via `Severity.String()`), the envelope `issues_found` flag derives from `HasActionableIssues()` so it can never disagree with `exit_code` (advisory-only ⇒ `false`/`0`), and the human report uses a softer `~` for advisories in the single ISSUES list with a separate advisory tally — all behavior-stable for today's actionable-only configs except the additive field.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-06-24T15:52:04Z
- **Completed:** 2026-06-24
- **Tasks:** 3 (2 implementation + 1 verification gate)
- **Files modified:** 4

## Accomplishments

- **SEV-01 (wire):** Added `Severity string \`json:"severity"\`` to `dto.Issue` (non-omitempty, placed after `Note` — D-02) and mapped it in `toDTO` via `Severity: is.Severity.String()`. `core/dto` stays a string-only leaf (does not import `core/model`); the int→string mapping happens at the render seam exactly like the existing `Kind: string(is.Kind)`.
- **SEV-02 (signal integrity):** Changed the envelope `IssuesFound` from `len(a.Issues) > 0` to `a.HasActionableIssues()`. Both `issues_found` and `exit_code` now derive from the one 02-01 predicate, so an advisory-only envelope can never read `issues_found:true / exit_code:0`. The `len(a.Issues) > 0` predicate appears **zero** times in `json.go`; the `make([]dto.Issue, len(a.Issues))` nil-slice (null↔[]) guard is preserved exactly.
- **SEV-01 (human surface):** Made the per-issue glyph severity-aware — `!` for actionable, softer `~` for advisories (D-05) — within the **single** ISSUES list (no separate section). Added a closing `N issues, M advisories` tally so advisories are numerically visible but not counted as actionable issues (D-06).
- **Agent contract held:** Re-ran the `core/cli` contract guards (`TestRunJSONEmitsOneObject`, `TestRunMissingFileJSONErrorOnStdout`) — exactly one JSON object on stdout with clean stderr on both success and fail paths, unchanged with the new field (they decode into `dto.Envelope`, which simply finds `severity` newly present).
- **Zero fixture/golden churn (Pitfall 4 confirmed):** No `core/testdata/**` and no test outside this plan's four `files_modified` changed. The corpus runner asserts `model.Analysis` struct fields (kinds/categories/blocks/lines), never JSON bytes or severity; the testgen 10-seed oracle compares only `(Kind, Name, Lines)` — both stay green. Whole suite + `go vet ./...` + `gofmt -l` + `make check` (fmt-check + vet + lint, 0 issues + test) + `make build` all green.

## Task Commits

Each implementation task was committed atomically (conventional commits referencing 02-02):

1. **Task 1: Thread severity onto the JSON wire and make issues_found actionable-only** — `bf398e7` (feat) — TDD: failing `TestJSONIssueHasSeverityOnEveryIssue` + `TestJSONAdvisoryOnlyEnvelope` (RED confirmed — "issues[0] has no severity key" and "issues_found true and exit_code 0 disagree") then the dto field + `toDTO` mapping + `IssuesFound` flip (GREEN). RED and GREEN committed together because the severity-decode test cannot pass without the additive wire field.
2. **Task 2: Make the human report severity-aware (softer advisory glyph + tally)** — `9ae9212` (feat) — TDD: failing `TestHumanAdvisoryMarkerAndTally` (RED confirmed — advisory line wrongly showed `!`, no tally) then the glyph-by-severity + `N issues, M advisories` tally (GREEN). Existing `TestHumanIncludesCategoriesAndIssues` intact.
3. **Task 3: Confirm the agent contract and whole-suite regression hold** — verification-only gate, no production edits → no commit (cli contract tests + whole suite + vet + gofmt + `make check` + `make build` all green; edits confirmed confined to the four declared files).

**Plan metadata:** see the final `docs(02-02)` commit.

_Note: Both implementation tasks are TDD; RED was confirmed (assertion failures pinpointing the missing field / wrong flag / wrong glyph) before GREEN. RED and GREEN were committed together per task since each is a single additive surface whose behavior test cannot pass without the implementation._

## Files Created/Modified

- `core/dto/analysis.go` — Added `Severity string \`json:"severity"\`` to `dto.Issue` after `Note` (non-omitempty, D-02); re-gofmt'd the struct field alignment. No new import (stays string-only leaf).
- `core/render/json.go` — Added `Severity: is.Severity.String()` to the `dto.Issue` literal inside the existing `for i, is := range a.Issues` loop; changed `IssuesFound: len(a.Issues) > 0` → `IssuesFound: a.HasActionableIssues()`. Left the `if a.Issues != nil` / `make([]dto.Issue, len(a.Issues))` nil-slice guard and `ExitCode: int(a.ExitCode())` untouched.
- `core/render/human.go` — In the ISSUES loop, replaced the hardcoded `"!"` with a marker chosen by `is.Severity` (`~` for `model.SevAdvisory`); counted actionable vs advisory and appended a `%d issues, %d advisories` tally line. Kept the single ISSUES header and the empty-issues early-return.
- `core/render/render_test.go` — Added `advisoryOnlyAnalysis()` helper, `issuesFromJSON()` + `findLine()` helpers, `TestJSONIssueHasSeverityOnEveryIssue`, `TestJSONAdvisoryOnlyEnvelope`, and `TestHumanAdvisoryMarkerAndTally`. Kept `sampleAnalysis()`, `TestHumanIncludesCategoriesAndIssues`, and `TestJSONIsOneObjectWithContract` intact (they pin the actionable case and still pass).

## Decisions Made

All within Claude's-discretion bounds the CONTEXT/RESEARCH delegated (A1, A2) — no locked decision was reinterpreted:

- **Advisory marker = `~`** (A1/D-05): a softer, single, non-blocking glyph clearly distinct from the actionable `!`.
- **Tally = `N issues, M advisories`** as a closing line of the ISSUES section (A1/D-06): advisories numerically visible and presented separately from the actionable issue count; placement chosen to sit with the issue list rather than the `%d lines, %d blocks` header.
- **Reused `HasActionableIssues()` for `issues_found`** (A2/Pattern 3) rather than re-deriving an actionable count in render — one source of truth shared with `ExitCode()`, so the two fields cannot drift (the explicit SEV-02 consistency property).

## Deviations from Plan

None — plan executed exactly as written. No bugs (Rule 1), missing critical functionality (Rule 2), blocking issues (Rule 3), or architectural changes (Rule 4) were encountered. No construction sites, fixtures, goldens, or non-render tests were touched. The only mid-task self-correction was a test-helper bug in my own new test (decoding `issues` at the envelope top level instead of under `analysis`) caught and fixed during the RED step before any implementation — not a deviation from the plan.

## Issues Encountered

None. (One self-inflicted test-helper path bug during RED, fixed immediately — see Deviations.)

## Known Stubs

None. The `severity` field is fully wired through both surfaces and the `SevAdvisory` branch is exercised now by hand-constructed unit-test fixtures (`advisoryOnlyAnalysis()` and the mixed-severity human test). The first *production* advisory that sets `SevAdvisory` (`relative_path_entry`) arrives in Phase 3 — this is documented intent (the mechanism is complete and functional), not a placeholder.

## Threat Surface

No new trust boundary or external-input surface. The only boundary touched is the existing `analyze --json` stdout contract; this plan adds one additive output field and changes the `issues_found` flag's meaning (actionable-only) over data the engine already produced — no new input parsing, file, network, or exec surface.

- **T-02-02 (Tampering, output stream)** delivered: the human glyph/tally lives only in `human.go`; `json.go` emits exactly one marshaled envelope and adds no print/log. The `core/cli` agent-contract tests (one JSON object on stdout, clean stderr, success + fail) were re-run as a guard and pass.
- **T-02-03 (Tampering/Repudiation, signal integrity)** delivered: both `issues_found` and `exit_code` derive from the single `HasActionableIssues()` predicate, so an advisory cannot be reported as a blocking issue nor a duplicate hidden (`TestJSONAdvisoryOnlyEnvelope` + the actionable `TestJSONIsOneObjectWithContract` regression pin this).
- **T-02-SC (package installs)**: not applicable — `go.mod`/`go.sum` unchanged, no installs.

No threat flags raised (no security-relevant surface beyond the planned additive field was introduced).

## User Setup Required

None — no external service configuration; `go.mod`/`go.sum` unchanged (no new dependencies).

## Next Phase Readiness

- **Ready for Phase 3 (relative-path advisory):** the full severity surface is live end-to-end — `model.Severity` (02-01) is now threaded to `dto.Issue.Severity` on the wire and to the human glyph/tally. Phase 3's `relative_path_entry` detector only needs to construct `model.Issue{..., Severity: model.SevAdvisory}`; it will automatically render `severity:"advisory"`, the softer `~` marker, a separate tally entry, and leave `issues_found`/`exit_code` clean.
- **No blockers.** The wire-contract change (every issue gains `severity`; `issues_found`/`exit_code` are actionable-only) is inert for every existing config until Phase 3 plants the first `SevAdvisory`.
- **Carry-forward (unchanged from STATE.md):** the Phase 3 design prerequisites (oracle two-field path-node split, extraction location, intra-statement duplicate policy, `CatPath` array-form classification) remain open Phase 3/4 planning items — this plan touched none of them.

## Self-Check: PASSED

- Files verified present: `core/dto/analysis.go`, `core/render/json.go`, `core/render/human.go`, `core/render/render_test.go`, `.planning/phases/02-issue-severity-tier/02-02-SUMMARY.md`
- Commits verified in git history: `bf398e7` (Task 1), `9ae9212` (Task 2)
- Verification gates green: `make check` (fmt-check + vet + lint 0 issues + test), `make build`, `go test ./...`, `go vet ./...`, `gofmt -l core/render/ core/dto/` (empty)
- Acceptance greps: `json:"severity"` present and non-omitempty; dto has no quoted `core/model` import; `Severity: is.Severity.String()` and `IssuesFound: a.HasActionableIssues()` present; `len(a.Issues) > 0` count = 0; `make([]dto.Issue, len(a.Issues))` guard count = 1; one rendered ISSUES header

---
*Phase: 02-issue-severity-tier*
*Completed: 2026-06-24*
