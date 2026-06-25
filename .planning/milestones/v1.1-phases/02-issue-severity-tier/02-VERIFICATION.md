---
phase: 02-issue-severity-tier
verified: 2026-06-24T00:00:00Z
status: passed
score: 4/4 must-haves verified
overrides_applied: 0
---

# Phase 2: Issue Severity Tier Verification Report

**Phase Goal:** Every issue carries a severity, and only `actionable` issues drive the exit code and `issues_found` flag — so an advisory-only config exits 0 while genuine problems still exit 3. This is the additive vertical slice (model → dto → both renderers → exit-code) that Phases 3 and 4 build on.
**Verified:** 2026-06-24
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Every issue in `--json` and the human report carries a self-describing severity string (`"actionable"`/`"advisory"`), present on every issue object (non-omitempty) | ✓ VERIFIED | `core/dto/analysis.go:25` `Severity string \`json:"severity"\`` (1 occurrence, 0 omitempty). `core/render/json.go:51` maps `Severity: is.Severity.String()` for every issue in the loop. `core/render/human.go:43-50` emits per-issue marker. Test `TestJSONIssueHasSeverityOnEveryIssue` asserts the key is present + non-empty on both actionable and advisory issues. End-to-end binary run on actionable config produced `severity: "actionable"` on the wire. |
| 2 | The four existing issue kinds (`duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed`) remain `actionable` (zero value) and still drive exit 3, with NO edits to construction sites in `core/analyze/reconciler.go` | ✓ VERIFIED | `SevActionable Severity = iota` is the zero value (`core/model/issue.go:20`). All `model.Issue{}` literals omit Severity: `reconciler.go:54,77,134` and `testgen/oracle.go:71,98`. `grep` for `Severity`/`SevActionable`/`SevAdvisory` in `core/analyze/` and `core/testgen/` → 0 matches. `git log 0eb6b24^..9ae9212 -- core/analyze/reconciler.go` and `-- core/testgen/` → no commits (untouched). End-to-end binary run: duplicate_alias config exits 3, `severity: "actionable"`. Whole suite (incl. testgen 10-seed oracle) green. |
| 3 | Advisory-only config exits `0` with `issues_found: false`; any actionable issue exits `3` with `issues_found: true`; the two fields can never disagree (both derive from `Analysis.HasActionableIssues()`) | ✓ VERIFIED | `core/model/analysis.go:27-34` defines `HasActionableIssues()`; `ExitCode()` (`:39-44`) consults it; `len(a.Issues)` removed from analysis.go (grep → 0). `core/render/json.go:60` sets `IssuesFound: a.HasActionableIssues()`; old `len(a.Issues) > 0` predicate removed (grep → 0). Tests `TestHasActionableIssues`, `TestExitCode` (incl. invariant `(ExitCode()==ExitActionable)==HasActionableIssues()`), `TestJSONAdvisoryOnlyEnvelope` (explicit disagreement guard). End-to-end: clean config → exit 0 / `issues_found:false`; actionable → exit 3 / `issues_found:true`. |
| 4 | `analyze --json` emits exactly one JSON object on stdout on BOTH success and `fail` paths (agent contract holds with severity threaded through `toDTO`) | ✓ VERIFIED | `core/cli/cli.go:88-100` `fail()` routes JSON error to stdout. Tests `TestRunJSONEmitsOneObject` + `TestRunMissingFileJSONErrorOnStdout` pass uncached. End-to-end success path: 1 top-level JSON object, exit 3, 0 stderr bytes. End-to-end fail path (missing file): 1 JSON object on stdout (`ok:false`, `error` key), exit 1, 0 stderr bytes. `json.go` adds no print/log — emits exactly one marshaled envelope. |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `core/model/issue.go` | Severity type (int+iota, SevActionable first), `String()`, `Issue.Severity` field | ✓ VERIFIED | `type Severity int`; `SevActionable Severity = iota` then `SevAdvisory` (only 2 consts); `String()` returns "advisory"/"actionable" (default arm = actionable); `Issue.Severity` field after `Note`. |
| `core/model/analysis.go` | `HasActionableIssues()` predicate; `ExitCode()` consults it | ✓ VERIFIED | `func (a Analysis) HasActionableIssues() bool` ranges issues for `SevActionable`; `ExitCode()` calls it; `len(a.Issues)` fully removed (grep → 0). |
| `core/dto/analysis.go` | `dto.Issue.Severity` wire field, non-omitempty, no model import | ✓ VERIFIED | `Severity string \`json:"severity"\`` (non-omitempty); `grep "zsh-pro/core/model"` → 0 (dto stays a string-only leaf). |
| `core/render/json.go` | `toDTO` maps Severity via `String()`; `IssuesFound` via `HasActionableIssues()` | ✓ VERIFIED | `Severity: is.Severity.String()` (`:51`); `IssuesFound: a.HasActionableIssues()` (`:60`); nil-slice `make([]dto.Issue, len(a.Issues))` guard preserved (grep → 1); no stray print/log. |
| `core/render/human.go` | severity-aware glyph (`!`/`~`) + advisory tally | ✓ VERIFIED | Marker `!` default, `~` for `model.SevAdvisory` (`:43-49`); single ISSUES list (1 header); closing tally `%d issues, %d advisories` (`:61`). End-to-end human report confirmed. |
| `core/model/model_test.go` | SEV-01/SEV-02 truth-table tests | ✓ VERIFIED | `TestSeverityString` (incl. zero-value=actionable + Issue{} omitted-field assertions), `TestHasActionableIssues` (4 cases), `TestExitCode` (4 cases + agreement invariant). All non-vacuous, pass uncached. |
| `core/render/render_test.go` | JSON severity-presence + advisory-only-envelope + human marker/tally tests | ✓ VERIFIED | `TestJSONIssueHasSeverityOnEveryIssue`, `TestJSONAdvisoryOnlyEnvelope` (decodes real JSON, asserts severity strings + field agreement), `TestHumanAdvisoryMarkerAndTally`; existing `TestJSONIsOneObjectWithContract`/`TestHumanIncludesCategoriesAndIssues` intact. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `analysis.go ExitCode()` | `analysis.go HasActionableIssues()` | predicate call (replaces `len(a.Issues) > 0`) | ✓ WIRED | `ExitCode()` body: `if a.HasActionableIssues()`; old raw-count predicate removed from file. |
| `json.go toDTO` | `dto.Issue.Severity` | `Severity: is.Severity.String()` | ✓ WIRED | Present at `json.go:51` inside the issue loop; renders to the wire field. |
| `json.go toDTO IssuesFound` | `analysis.go HasActionableIssues()` | `IssuesFound: a.HasActionableIssues()` | ✓ WIRED | Present at `json.go:60`; both `issues_found` and `exit_code` derive from the one predicate (verified end-to-end they agree). |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `json.go` issue `severity` | `is.Severity` | `model.Issue.Severity`, set at reconciler construction sites (zero value → actionable) and threaded through `toDTO` | ✓ Real — end-to-end actionable config produced `severity:"actionable"` on stdout from a genuine parse, not a hardcoded literal | ✓ FLOWING |
| `json.go` `issues_found` | `a.HasActionableIssues()` | computed over `a.Issues` populated by the reconciler | ✓ Real — clean config → false/0, actionable config → true/3 end-to-end | ✓ FLOWING |
| `human.go` marker + tally | `is.Severity`, computed `actionable`/`advisory` counts | same `model.Issue.Severity` data path | ✓ Real — end-to-end human report shows `!` + `1 issues, 0 advisories` from a genuine parse | ✓ FLOWING |

Note: the `SevAdvisory` value cannot yet flow end-to-end through the binary because no detector sets it until Phase 3 (`relative_path_entry`) — this is the documented, intended scope boundary (CONTEXT D-01). The advisory branch is fully exercised by hand-constructed unit fixtures (`advisoryOnlyAnalysis()`), which is appropriate for a mechanism-only phase.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Actionable config: one JSON object, exit 3, severity present | `zsh-pro analyze dup.zshrc --json` | 1 JSON object, exit 3, 0 stderr bytes, `issues_found:true` `exit_code:3`, issue `severity:"actionable"` | ✓ PASS |
| Clean config: exit 0, issues_found false | `zsh-pro analyze clean.zshrc --json` | exit 0, `issues_found:false`, `exit_code:0`, no issues | ✓ PASS |
| Fail path: one JSON object on stdout, clean stderr | `zsh-pro analyze does-not-exist.zshrc --json` | 1 JSON object, exit 1, 0 stderr bytes, `ok:false` + `error` key | ✓ PASS |
| Human report severity marker + tally | `zsh-pro analyze dup.zshrc` | `! duplicate_alias gs ...` + `1 issues, 0 advisories` | ✓ PASS |
| Full suite green (uncached targeted + `go test ./...`) | `go test ./... ; go vet ./... ; gofmt -l` | all packages ok; vet exit 0; gofmt no drift | ✓ PASS |

### Probe Execution

Not applicable — this phase declares no probes (`scripts/*/tests/probe-*.sh`); it is verified via the Go test suite and behavioral spot-checks above.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| SEV-01 | 02-01, 02-02 | Every issue carries a self-describing severity string in both surfaces; `actionable` is the zero-value default so the four existing kinds are unchanged | ✓ SATISFIED | Truths 1, 2 + artifacts (model Severity type, dto field, json mapping, human marker). |
| SEV-02 | 02-01, 02-02 | Exit code and `issues_found` reflect only actionable issues; advisory-only exits 0; exit 3 reserved for genuine problems | ✓ SATISFIED | Truths 3, 4 + `HasActionableIssues()` as single source of truth for both fields (key links + end-to-end). |

No orphaned requirements: REQUIREMENTS.md maps exactly SEV-01, SEV-02 to Phase 2, and both appear in PLAN frontmatter. All 8 milestone requirements remain mapped (Phase 2: 2 · Phase 3: 3 · Phase 4: 3).

### Anti-Patterns Found

None. Grep for `TODO|FIXME|XXX|TBD|HACK|PLACEHOLDER|placeholder|not implemented|coming soon` across all five phase-modified production files returned no matches. No empty/stub returns, no orphaned artifacts, no debt markers.

### Human Verification Required

None. All four success criteria were verified programmatically: unit tests (uncached), `go vet`, `gofmt`, and direct end-to-end execution of the built binary on the success path, fail path, clean config, and human report. The only behavior not exercisable end-to-end (a production-set `SevAdvisory`) is out of scope until Phase 3 by design and is fully covered by unit fixtures.

### Gaps Summary

No gaps. The phase goal is achieved in the codebase:

- The `Severity` type exists with `SevActionable` as the genuine zero value, `String()` returns the locked wire labels, and `Issue` carries the field.
- `HasActionableIssues()` is the single source of truth; both `ExitCode()` and the envelope `issues_found` derive from it, and the old `len(a.Issues)`-based predicates are fully removed (verified by grep in both files).
- The four existing issue kinds are byte-identical in behavior: their construction sites in `core/analyze/reconciler.go` and `core/testgen/oracle.go` are unchanged (git-confirmed) and inherit `SevActionable` via the zero value; the whole suite including the testgen 10-seed oracle stays green with edits confined to `core/model`, `core/dto`, `core/render`.
- The agent contract holds on both the success and fail paths (one JSON object on stdout, clean stderr) with the new `severity` field threaded through `toDTO`, confirmed by the cli tests and by running the actual binary.

All four SUMMARY.md claims were independently verified against the live source, git history, and runtime behavior — they hold.

---

_Verified: 2026-06-24_
_Verifier: Claude (gsd-verifier)_
