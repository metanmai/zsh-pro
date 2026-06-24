# Phase 1: Trustworthy Line Numbers - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-24
**Phase:** 1-Trustworthy Line Numbers
**Areas discussed:** Statement-line data model

---

## Gray-Area Selection

Three gray areas were presented for selection:

| Area | Description | Selected for discussion |
|------|-------------|-------------------------|
| Statement-line data model | LINE-02 fix shape: redefine `StartLine` vs. add a `StmtLine` field | ✓ |
| Line-count semantics | LINE-01 corrected formula / edge cases | (taken as recommended) |
| Corpus pin depth | PIN-02: property-test pin only vs. also asserting lines in `manifests.json` | (taken as recommended) |

The user selected only **Statement-line data model**; the other two were accepted as recommended (see CONTEXT.md D-03, D-05).

---

## Statement-line data model

**Question:** How should the statement line be represented so issues stop reporting the leading-comment line?

| Option | Description | Selected |
|--------|-------------|----------|
| Redefine StartLine | Stop overwriting `startLine` in parse.go's comment loop, so `Block.StartLine` = statement line. Issues already read `StartLine` → zero reconciler/model changes, one parser change. Text still includes the leading comment. | ✓ |
| Add a StmtLine field | Keep `StartLine` = comment line; add `Block.StmtLine`; repoint the 4 reconciler sites. More explicit, keeps Text/StartLine consistent, but adds a field whose `StartLine` counterpart then has no reader. | |
| You decide | Lock the recommended approach and move on. | |

**User's choice:** Redefine StartLine
**Notes:** Chosen because `StartLine`'s only consumer is issue reporting (`reconciler.go:46,69,103,109`), so redefining it fixes all four issue kinds with no reconciler or model churn. Accepted that `Block.StartLine` will no longer equal the first line of `Block.Text` (Text keeps the pulled-up comment) — a benign internal mismatch.

---

## Closing gate

**Question:** Ready for context, or explore more gray areas?
**User's choice:** I'm ready for context.

---

## Claude's Discretion

- Exact variable/comment wording and test-helper structure.
- How the oracle/test sources the expected total `Lines` for the PIN-01 assertion (flagged in CONTEXT.md code_context as a planning watch-out).

## Deferred Ideas

- Path-segment mis-naming in `dupPathIssues` (`./scripts` → `/scripts`; unrooted entries skipped) — v2 (PATH-01).
- Golden fixtures for `duplicate_path` / `shadowed` and `issue_names`/`issue_lines` corpus assertions — v2 (COV-01 / COV-02).

These were pre-scoped to v2 during `/gsd:new-project`, not raised as new scope creep during this discussion.
