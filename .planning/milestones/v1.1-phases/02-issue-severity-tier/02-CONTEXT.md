# Phase 2: Issue Severity Tier - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase adds a per-issue **severity** to `model.Issue`, surfaced in the human report and the `--json` envelope, where only `actionable` issues drive the exit code and the `issues_found` flag. It builds the *mechanism only* — the first advisory that uses it (`relative_path_entry`) is produced in Phase 3.

**In scope:** the `Severity` type + field, threading it through model → dto → both renderers, and making `ExitCode()` + `issues_found` + the human summary severity-aware. Delivers SEV-01 and SEV-02.

**Out of scope (other phases / deferred):** the relative-PATH advisory itself and CWE-427 (Phase 3); golden/oracle assertions on severity (Phase 4, COV-02/03); a richer severity scale; severity-filtering CLI flags; classifier confidence surfacing (PREC-*).
</domain>

<decisions>
## Implementation Decisions

### Severity Scale & Type
- **D-01:** Two-value tier only — `actionable` and `advisory`. `SevActionable` is the **zero value**, so the four existing issue kinds (`duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed`) keep their behavior with **no edits to their construction sites** (reconciler + testgen `Issue{}` literals). The enum stays extensible (add values later) without breaking the zero-value contract — do NOT add a 3+ scale speculatively.
- **D-02:** Severity is surfaced in `--json` as an **always-present (non-`omitempty`) self-describing string** (`"actionable"`/`"advisory"`) on every issue, so agents can switch on it unconditionally.

### Severity Vocabulary
- **D-03:** The two tiers are named `actionable` / `advisory` (not `error`/`warning`). Self-describing for the agent contract; avoids clashing with the envelope's `ok`/error semantics. (The FEATURES-research "warning" term referred to the relative-PATH *issue kind*'s conventional naming, not this severity-tier label.)

### Exit Code & issues_found Contract
- **D-04:** `Analysis.ExitCode()` and the envelope `issues_found` flag both count **actionable** issues only. An advisory-only config exits `0` with `issues_found: false`. Advisories never bump the exit code; exit 3 stays reserved for actionable issues. This is the SEV-02 wire-contract change — **confirmed in discussion**. It is inert for every existing config until Phase 3 introduces the first advisory (today all issues are actionable).

### Human Report Format
- **D-05:** Advisories render in the **single ISSUES list** with a distinct, softer marker (e.g. `~`) instead of the actionable `!`, so they read as non-blocking but stay inline. NOT a separate section.
- **D-06:** The human report **summary tallies advisories separately** from actionable issues — e.g. "N issues, M advisories" — so advisories are visible but clearly not counted as actionable "issues".

### Claude's Discretion
- The Go `Severity` type representation (`int` + `iota` + `String()` vs string constants) — the *wire encoding* is the string regardless (D-02).
- The exact softer marker character for advisories (`~`, `·`, …) and the precise summary-line wording, as long as advisories are visually + numerically distinguished from actionable issues (D-05/D-06).
- Whether to add a small helper (e.g. `Analysis.hasActionable()` / an actionable count) shared by `ExitCode()`, `issues_found`, and the summary.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone scope & requirements
- `.planning/REQUIREMENTS.md` — SEV-01, SEV-02 (delivered here); also PATH-03, the downstream advisory that consumes this tier in Phase 3.
- `.planning/ROADMAP.md` § "Phase 2: Issue Severity Tier" — goal, success criteria, the parked `issues_found` confirm (now resolved by D-04), and the oracle two-field design prerequisite to record for Phase 3/4.
- `.planning/PROJECT.md` — Key Decisions (advisory = informational severity; `issues_found` actionable-only) and Constraints (no new deps; layering: `core/analyze` shell-free, model/dto separation).

### Research (milestone-level — maps this phase file-by-file)
- `.planning/research/SUMMARY.md` — converged decisions: `SevActionable`=zero value, exit/`issues_found` actionable-only, severity-as-string.
- `.planning/research/ARCHITECTURE.md` — per-file integration map for the severity tier (model → dto → renderers → exit-code), the strict A→B→C build order, and the two-field oracle prerequisite.
- `.planning/research/PITFALLS.md` — severity-tier contract regressions: zero-value default, BOTH `ExitCode()` and `issues_found`, dto threading, one-JSON-object agent contract, golden byte-stability.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (files this phase modifies)
- `core/model/issue.go` — `Issue{Kind,Name,Lines,Note}` + 4 `IssueKind` consts. Add `Severity` field + a `Severity` type with `SevActionable` (zero) / `SevAdvisory`.
- `core/model/analysis.go:25` — `ExitCode()` is currently `if len(a.Issues) > 0 { ExitActionable }`. Change to count actionable only.
- `core/dto/analysis.go:20` — wire `Issue{kind,name,lines?,note?}`. Add `Severity string \`json:"severity"\`` (non-omitempty).
- `core/dto/envelope.go:9` — `IssuesFound bool \`json:"issues_found"\``.
- `core/render/json.go:43-60` — `toDTO` maps Issues, sets `IssuesFound: len(a.Issues) > 0` (→ actionable-only) and `ExitCode: int(a.ExitCode())`.
- `core/render/human.go:34-48` — "ISSUES" section; per-issue line `! <kind> <name> at lines [...] (note)`. Add the severity-aware marker (D-05) + summary advisory tally (D-06).

### Established Patterns (constraints)
- **model ↔ dto separation:** mapping lives only in `core/render/json.go` (`toDTO`). Thread `Severity` through BOTH `model.Issue` and `dto.Issue` manually.
- **Nil-slice preservation / golden byte-stability:** `toDTO` preserves nil `Categories`/`Issues` as JSON `null` (no omitempty). Adding `severity` to every issue changes existing fixtures' JSON bytes → update golden expectations within Phase 2; full name/line/severity corpus assertions are Phase 4 (COV-02).
- **Agent contract:** exactly one JSON object on stdout (success + `fail` paths) must still hold with the new field.
- **No new dependencies;** `core/analyze` stays shell-free; `core/model` is a leaf.

### Integration Points
- Advisory severity is *produced* in Phase 3 (the `relative_path_entry` detector sets `SevAdvisory`). Phase 2 only builds + threads the field and flips the exit/`issues_found`/summary logic. In Phase 2 every issue is actionable → output is behavior-stable except the additive `severity` field.
</code_context>

<specifics>
## Specific Ideas

- Design ethos for this tier (from the v1.0 retrospective): "match the mechanism to the need, not maximal tooling" — keep it two-valued; no speculative scale.
- `relative_path_entry` + CWE-427 are Phase 3 concerns; do not pre-build them here.
</specifics>

<deferred>
## Deferred Ideas

- **Richer severity scale** (e.g. `error`/`warning`/`info`) — only if a real need appears; the enum is extensible. Not now.
- **Severity-based filtering** (e.g. `--severity advisory` CLI flag) — a new capability; its own future phase.
- **Classifier confidence surfacing** (PREC-01/02) — a separate axis (`Block.Conf` on blocks/categories, not `Issue.Severity`); separate future phase.
</deferred>

---

*Phase: 02-issue-severity-tier*
*Context gathered: 2026-06-24*
