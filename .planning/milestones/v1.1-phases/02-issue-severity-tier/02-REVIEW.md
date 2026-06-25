---
phase: 02-issue-severity-tier
reviewed: 2026-06-24T00:00:00Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - core/dto/analysis.go
  - core/model/analysis.go
  - core/model/issue.go
  - core/model/model_test.go
  - core/render/human.go
  - core/render/json.go
  - core/render/render_test.go
findings:
  critical: 0
  warning: 2
  info: 3
  total: 5
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-06-24
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

Phase 2 adds a two-value `model.Severity` tier (`SevActionable` zero value / `SevAdvisory`) and threads it through `model.Issue` → `dto.Issue` → both renderers, while routing the exit code and the envelope `issues_found` flag through a single new `Analysis.HasActionableIssues()` source of truth.

The implementation is correct against every contract called out in `02-CONTEXT.md`: `SevActionable` is verifiably the zero value (proven by `TestSeverityString` and `TestHasActionableIssues`), `ExitCode()` and `issues_found` both derive from `HasActionableIssues()` so they cannot disagree (cross-checked by an explicit invariant assertion in both `TestExitCode` and `TestJSONAdvisoryOnlyEnvelope`), the JSON `severity` field is non-omitempty and always emitted, and layering is clean (`core/dto` and `core/model` import nothing internal; mapping lives only in `core/render/json.go::toDTO`). No new dependencies were added.

I independently verified: `go build ./...` passes, `go vet ./...` clean, `gofmt -l` clean on all seven files, `golangci-lint run` reports 0 issues, and the full `go test ./...` suite passes (including the testgen oracle property test, the corpus runner, and the CLI agent-contract test, run fresh with `-count=1`). The `manifests.json` corpus is an expectations manifest (issue *kinds*, not rendered JSON bytes), so the CONTEXT's "golden byte-stability" concern does not apply — no committed rendered-JSON golden exists that the new `severity` field would stale.

No blockers. The findings below are a latent extensibility inconsistency the code's own comments contradict, and minor robustness/quality observations.

## Warnings

### WR-01: Three-way divergence in how a future (non-zero, non-advisory) Severity value is treated

**File:** `core/model/issue.go:29-36`, `core/model/analysis.go:27-34`, `core/render/human.go:43-49`
**Issue:** The code is two-valued *today* (so this cannot manifest now), but three independent call sites disagree on how they would classify a hypothetical future `Severity` value other than `SevActionable`/`SevAdvisory` — e.g. a later `SevError Severity = iota+2`. The comments explicitly advertise the enum as "extensible without breaking that zero-value contract" (`issue.go:16`) and the `String()` default arm is justified as making "an un-handled severity degrade to the safe, exit-bumping label" (`issue.go:28`). But the three sites do not actually agree on "exit-bumping":

- `Severity.String()` (`issue.go:32-34`): `default` → `"actionable"` — treats any non-advisory value as actionable on the wire.
- `HasActionableIssues()` (`analysis.go:29`): `is.Severity == SevActionable` — counts a value as actionable *only if it equals the zero value*. A future `Severity(2)` is **not** counted, so it would **not** bump the exit code or `issues_found`.
- `human.go` marker/tally (`human.go:44-49`): `if is.Severity == model.SevAdvisory { ~ / advisory++ } else { ! / actionable++ }` — treats any non-advisory value as actionable (matches `String()`, disagrees with `HasActionableIssues()`).

Net effect for a future `Severity(2)`: the JSON string says `"actionable"` and the human report marks it `!` and tallies it as actionable, yet `exit_code`/`issues_found` would report the analysis as clean. That is exactly the `ExitCode()`/`issues_found` ↔ rendered-output disagreement the phase set out to make impossible, merely deferred to the next enum value. It also directly contradicts the `String()` doc comment's stated safety intent (degrade to "exit-bumping"), since `HasActionableIssues` would not bump.

This is a WARNING, not a BLOCKER: CONTEXT/CLAUDE.md mandate a strictly two-valued type ("do NOT add a 3+ scale speculatively"), so no third value exists and no incorrect behavior ships today. The risk is a latent trap for whoever adds the next severity.

**Fix:** Make the actionable predicate "not advisory" rather than "equals the zero value", so all three sites share one polarity and a future value defaults to the same safe (exit-bumping) side everywhere the comments already promise:
```go
// core/model/analysis.go
func (a Analysis) HasActionableIssues() bool {
    for _, is := range a.Issues {
        if is.Severity != SevAdvisory { // every non-advisory severity is actionable
            return true
        }
    }
    return false
}
```
Alternatively, add a single `func (s Severity) IsActionable() bool { return s != SevAdvisory }` on the model type and have `HasActionableIssues`, the `human.go` marker branch, and (implicitly) `String()` all key off it, so the "actionable vs advisory" cut exists in exactly one place — matching the phase's own "single source of truth" design ethos.

### WR-02: Human summary tally pluralizes unconditionally ("1 issues, 0 advisories")

**File:** `core/render/human.go:61`
**Issue:** `fmt.Fprintf(&b, "   %d issues, %d advisories\n", actionable, advisory)` always prints the plural nouns, producing user-facing grammar like `1 issues, 0 advisories` and `1 advisories`. The rest of the human report is otherwise polished and human-facing (e.g. "(none found — nice and clean)"). `TestHumanAdvisoryMarkerAndTally` only asserts the substring `"advisor"` and the digit `"1"` appear on the tally line, so it does not catch this. Severity is Warning rather than Info because it is incorrect *output* a user reads on the primary report's most common case (a single actionable issue), not merely an internal style nit.

**Fix:** Pluralize per count, e.g.:
```go
plural := func(n int, one, many string) string {
    if n == 1 {
        return many[:0] + one // or just: if n == 1 { return one }; return many
    }
    return many
}
fmt.Fprintf(&b, "   %d %s, %d %s\n",
    actionable, map[bool]string{true: "issue", false: "issues"}[actionable == 1],
    advisory, map[bool]string{true: "advisory", false: "advisories"}[advisory == 1])
```
(Any equivalent singular/plural selection is fine; the discretion granted in D-06 covers exact wording — this only asks that the wording be grammatical.)

## Info

### IN-01: Advisory tally line omitted entirely when there are zero advisories may surprise agents/scripts grepping the human report

**File:** `core/render/human.go:34-62`
**Issue:** The summary tally line is only emitted when `len(a.Issues) > 0` (the early return at lines 35-38 prints "(none found …)" and returns before any tally). So a config with zero issues has no "0 issues, 0 advisories" line, while a config with issues does. This is internally consistent and arguably intended (the empty case has its own friendly message), but the asymmetry is worth noting for anyone who later writes tooling against the human format. Not a defect — the JSON envelope is the agent contract, and it is unaffected.
**Fix:** None required. If a uniform machine-greppable summary is ever wanted, emit the tally in both branches; otherwise leave as-is.

### IN-02: `actionable`/`advisory` counters are recomputed in the renderer rather than reusing the model's source of truth

**File:** `core/render/human.go:39-49`
**Issue:** `human.go` recounts actionable issues with its own `else` branch instead of leaning on `model`'s actionable/advisory classification. This is currently harmless because the renderer needs a per-issue marker anyway, but it is the *fourth* place that encodes the "advisory vs not" decision (alongside `String()`, `HasActionableIssues`, and the polarity discussed in WR-01). Consolidating onto a single `Severity.IsActionable()` helper (see WR-01 fix) would remove the duplication and keep the marker, the tally, and the exit logic provably in lockstep.
**Fix:** Fold into the WR-01 remediation; no separate change needed.

### IN-03: `dto.Issue.Severity` doc comment hard-codes the closed value set

**File:** `core/dto/analysis.go:25`
**Issue:** The trailing comment `// always present (D-02): "actionable" | "advisory"` enumerates the exact two wire strings. That is accurate and useful today, but if the `Severity` enum is ever extended (the model comments invite it), this comment and the model's `String()` mapping must be updated together or they will drift. Purely a documentation-maintenance note.
**Fix:** Optional — phrase as open-ended (e.g. `// always present (D-02); self-describing severity label, see model.Severity.String()`) so the canonical value list lives in exactly one place (`model/issue.go`).

---

_Reviewed: 2026-06-24_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
