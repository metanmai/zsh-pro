---
phase: 02-issue-severity-tier
fixed_at: 2026-06-24T00:00:00Z
review_path: .planning/phases/02-issue-severity-tier/02-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 02: Code Review Fix Report

**Fixed at:** 2026-06-24
**Source review:** .planning/phases/02-issue-severity-tier/02-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 5 (WR-01, WR-02, IN-01, IN-02, IN-03)
- Fixed: 5
- Skipped: 0

The five findings were resolved with three atomic commits. WR-01, IN-02, and the
human-renderer half of WR-01 collapse into one coherent "single source of truth"
change (a new `Severity.IsActionable()` predicate), matching the reviewer's
recommended remediation and the phase's own design ethos. WR-02 (grammar) and
IN-03 (doc comment) are independent. IN-01 was marked "None required" by the
reviewer and is documented below as intentionally left as-is.

All changes are behavior-preserving for the current two-value enum: `==
SevActionable` and `!= SevAdvisory` are equivalent today, so the JSON wire
contract is unchanged (severity strings stay `"actionable"`/`"advisory"`;
`issues_found`/`exit_code` behavior identical). Only the human report grammar
(WR-02) changes user-visible output. `make check` (fmt-check + vet +
golangci-lint + full test suite, including the testgen oracle property test with
`checkLines=true`) is fully green.

## Fixed Issues

### WR-01: Three-way divergence in how a future (non-zero, non-advisory) Severity value is treated

**Files modified:** `core/model/issue.go`, `core/model/analysis.go`, `core/render/human.go`, `core/model/model_test.go`
**Commits:** 96e6719 (model + test), b35c346 (human renderer)
**Applied fix:** Added `func (s Severity) IsActionable() bool { return s != SevAdvisory }` to `core/model/issue.go` as the single source of truth for the "actionable vs advisory" cut. Rewrote `Analysis.HasActionableIssues()` to call `is.Severity.IsActionable()` instead of `is.Severity == SevActionable`, and pointed the human renderer's per-issue marker/tally branch at the same helper (inverting the `if` so the actionable case is the explicit branch). Now all sites share one polarity — a future non-zero, non-advisory severity (e.g. `Severity(2)`) defaults to the same safe, exit-bumping side everywhere, closing the latent trap the code's own comments warned about. Added `TestSeverityIsActionable` covering `SevActionable`, `SevAdvisory`, the zero value, and a future `Severity(2)` value (asserting it reads as actionable). Note: this is a refactor that is behavior-preserving for today's two-value enum; the existing tests stayed green, confirming no regression.

### WR-02: Human summary tally pluralizes unconditionally ("1 issues, 0 advisories")

**Files modified:** `core/render/human.go`
**Commit:** b35c346
**Applied fix:** Added a small local `plural(n, one, many)` helper and changed the tally `Fprintf` to select singular/plural per count: `1 issue` / `2 issues` and `1 advisory` / `2 advisories`. The existing `TestHumanAdvisoryMarkerAndTally` (which asserts the substring `"advisor"` and the digit `"1"`) still passes.

### IN-01: Advisory tally line omitted entirely when there are zero advisories

**Files modified:** none
**Commit:** n/a (intentionally left as-is)
**Applied fix:** Per the reviewer's "None required" guidance, left as-is. The zero-issue case has its own friendly message ("(none found — nice and clean)") and the JSON envelope — not the human report — is the agent contract, so no machine-greppable uniform tally is needed. Emitting a `0 issues, 0 advisories` line in the empty case would be over-engineering against an explicit "do not over-engineer" instruction.

### IN-02: `actionable`/`advisory` counters recomputed in the renderer rather than reusing the model's source of truth

**Files modified:** `core/render/human.go` (folded into the WR-01 remediation)
**Commits:** 96e6719 (helper), b35c346 (renderer keys off it)
**Applied fix:** Folded into WR-01 as the reviewer directed. The human renderer's marker/tally branch now keys off `Severity.IsActionable()`, so the marker, the tally, and the exit logic are provably in lockstep on one predicate. The renderer still needs its own per-issue marker (unavoidable), but the *decision* of "advisory vs not" now lives in exactly one place.

### IN-03: `dto.Issue.Severity` doc comment hard-codes the closed value set

**Files modified:** `core/dto/analysis.go`
**Commit:** 3a6778b
**Applied fix:** Reworded the trailing comment from `// always present (D-02): "actionable" | "advisory"` to `// always present (D-02); self-describing severity label, see model.Severity.String()`, so the canonical value list lives in exactly one place (`model/issue.go`) and the two cannot drift when the enum is extended. No layering violation: it is a comment-only change and `core/dto` still imports nothing internal.

---

_Fixed: 2026-06-24_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
