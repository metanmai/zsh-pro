# Phase 2: Issue Severity Tier - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-24
**Phase:** 2-issue-severity-tier
**Areas discussed:** Severity scale, Severity names, Human report format, Summary & issues_found

---

## Severity scale

| Option | Description | Selected |
|--------|-------------|----------|
| Two: actionable + advisory | Exit-code axis is binary; only the dup/shadow-vs-relative split is needed; enum stays extensible via the zero-value contract | ✓ |
| Three+: error / warning / info | Richer scale now; collapses to actionable-vs-not for exit purposes anyway (presentational until needed) | |

**User's choice:** Two-value tier.
**Notes:** Decided first because it determines how many names are needed. Surfaced that the future classifier-precision work (PREC-01/02) surfaces *confidence on blocks/categories* (`Block.Conf`) — a different axis from issue severity — so the tier doesn't need to pre-build for it.

---

## Severity names

| Option | Description | Selected |
|--------|-------------|----------|
| actionable / advisory | Self-describing for the agent contract; matches REQUIREMENTS/PROJECT wording; avoids ok/error clash | ✓ |
| actionable / warning | FEATURES-research term (Lynis/CIS); pairs awkwardly with "actionable" | |
| error / warning | Classic linter vocab; "error" overstates a duplicate alias and collides with envelope ok/error | |

**User's choice:** `actionable` / `advisory`.
**Notes:** The FEATURES "warning" term referred to the relative-PATH *issue kind*, not this severity-tier label.

---

## Human report format

| Option | Description | Selected |
|--------|-------------|----------|
| Distinct marker, one list | Keep one ISSUES list; advisories use a softer marker (e.g. `~`) vs `!`; minimal new code | ✓ |
| Separate "Advisories" section | Clearest act-vs-FYI split; more structure/code + empty-section case | |
| Inline [advisory] tag | Same `!` + explicit `[advisory]` tag; slightly noisier | |

**User's choice:** Distinct marker, one list.

---

## Summary & issues_found

| Option | Description | Selected |
|--------|-------------|----------|
| Actionable-only flag + separate advisory count | Locks SEV-02; human summary shows "N issues, M advisories" | ✓ |
| Actionable-only flag, no advisory tally | Same contract; advisories only appear in the list | |
| Keep issues_found = any issue | Reverts SEV-02; self-contradictory envelope (issues_found:true / exit_code:0) | |

**User's choice:** Actionable-only flag + separate advisory count.
**Notes:** Final confirmation of the SEV-02 `issues_found` wire-contract change.

---

## Claude's Discretion

- Go `Severity` type representation (`int` + `iota` + `String()` vs string constants) — wire encoding is the string regardless.
- Exact softer marker character for advisories (`~`, `·`, …).
- Precise summary-line wording, as long as advisories are counted separately.
- Whether to add a shared `hasActionable()`/actionable-count helper for `ExitCode()` + `issues_found` + summary.

## Deferred Ideas

- Richer severity scale (error/warning/info) — only if a real need appears; enum is extensible.
- Severity-based filtering CLI flag (e.g. `--severity advisory`) — new capability, own future phase.
- Classifier confidence surfacing (PREC-01/02) — separate axis (`Block.Conf`), separate future phase.
