# Milestones

## v1.0 Trustworthy Line Numbers (Shipped: 2026-06-24)

**Phases completed:** 1 phases, 3 plans, 7 tasks

**Key accomplishments:**

- Replaced `Analysis.Lines = strings.Count(string(src), "\n") + 1` with an editor-style `countLines` helper so an empty file reports 0 and a trailing-newline file is no longer over-counted by one.
- Deleting one `startLine = c.Pos().Line()` overwrite in the zsh comment-pull-up loop makes `Block.StartLine` report the statement's own line instead of the leading-comment line — every issue (duplicate alias/env, duplicate path, shadowed) inherits the fix unchanged via reconciler.go, with no new model field.
- Locks the LINE-01 (total-Lines) and LINE-02 (per-issue statement-line) fixes behind the 10-seed oracle property test plus the golden corpus — flipping `checkLines` to true with a real assertion block, sourcing the expected total non-circularly from a new `ConfigGraph.RenderedLines` render counter, and giving planted dupAlias/dupEnv pairs a leading comment so LINE-02 is exercised beyond shadows.

---

## v1.1 Trustworthy PATH Analysis (Parked: 2026-06-25 — superseded by the v2.0 pivot)

**Status:** Partially shipped — Phase 2 of 3 done; Phases 3–4 deferred when the project pivoted to v2.0 (Branchable Shell Environments).

**Shipped:**

- Phase 2 — Issue Severity Tier: per-issue `actionable`/`advisory` severity; only actionable issues drive `exit_code`/`issues_found` via a single `HasActionableIssues()` predicate (they can never disagree); the four existing issue kinds stay byte-identical (zero-value `SevActionable`). Verified 4/4, code-reviewed, and review-fixed (`Severity.IsActionable()` consolidation + grammar).

**Deferred / re-scoped under v2.0:**

- Phase 3 — Trustworthy PATH Extraction & Detection (AST split extraction + notation-only dedup + relative advisory)
- Phase 4 — PATH Coverage & Oracle Pin

**Why parked:** The documented "read-only analyzer" identity had drifted from the real intent — a branchable shell-environment manager. The remaining analyzer-reporting work was deprioritized; the PATH-ingest/canonicalization need is re-captured under v2.0, where the parser/classifier become the ingest component.

Archived detail: [milestones/v1.1-ROADMAP.md](v1.1-ROADMAP.md) · [milestones/v1.1-REQUIREMENTS.md](v1.1-REQUIREMENTS.md) · [milestones/v1.1-phases/](v1.1-phases/)

---
