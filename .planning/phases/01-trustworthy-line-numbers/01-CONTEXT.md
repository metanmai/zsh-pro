# Phase 1: Trustworthy Line Numbers - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Make `analyze`'s reported line numbers correct, and lock them with tests. Two documented bugs: the total `Analysis.Lines` count (off-by-one) and per-issue line attribution (reports the leading-comment line instead of the statement line). Scope is exactly these two line-number bugs plus their test pins — **not** path-segment mis-naming, corpus expansion, opaque-fallback recovery, or any feature work (those are v2 / out of scope per REQUIREMENTS.md).

</domain>

<decisions>
## Implementation Decisions

### Statement-Line Attribution — LINE-02 (discussed)
- **D-01:** Fix mis-attribution by **redefining `Block.StartLine` to the statement's own line.** In `core/shell/zsh/parse.go`, the comment-pull-up loop (`:36-40`) must stop overwriting `startLine`; keep updating the `start` *offset* (so `Block.Text` still includes the leading comment for context), but leave `startLine = stmt.Pos().Line()`. Net result: `Block.StartLine` = the statement's line.
- **D-02:** **No new `Block` field and no reconciler changes.** The four issue sites (`reconciler.go:46,69,103,109`) already read `StartLine` and inherit the fix for free. Adding a separate `StmtLine` field was explicitly rejected — `StartLine` has no other consumer, so the comment-line value it holds today is effectively dead.
- Accepted trade-off: `Block.StartLine` will no longer equal the first line of `Block.Text` (Text still begins at the pulled-up comment). This benign internal mismatch is fine — `StartLine` means "the construct's line," Text carries the doc comment as context.

### Line-Count Total — LINE-01 (recommended, accepted)
- **D-03:** Replace `analyzer.go:28` (`strings.Count(string(src), "\n") + 1`) with editor-style counting: empty file → `0`; otherwise `Count("\n") + (lastByte != '\n' ? 1 : 0)`. A trailing-newline file is not over-counted; an unterminated final line still counts. Guarantees `Lines` ≥ every issue's statement line.

### Test Pinning — PIN-01 / PIN-02 (recommended, accepted)
- **D-04:** Flip `core/testgen/property_test.go:18` `checkLines` to `true` and implement the gated block (`:89-92`) to assert both total `Lines` and each issue's `lines` slice across all 10 seeds. Update the now-stale comment ("Flip to true once those are fixed").
- **D-05:** Golden corpus stays minimal — `manifests.json` + `corpus_test.go` must remain green against corrected output, with the `empty.zsh` case set to a `0`-line count. Do **not** add `issue_lines` / `issue_names` assertions to the corpus (that is the declined v2 scope; the property test is the line pin).

### Claude's Discretion
- Exact variable/comment wording, test-helper structure, and **how the oracle/test sources the expected total `Lines`** are left to the planner/executor (see watch-outs in code_context).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning docs
- `.planning/PROJECT.md` — scope, Core Value, Key Decisions (recommended fix approaches, now confirmed here).
- `.planning/REQUIREMENTS.md` — LINE-01, LINE-02, PIN-01, PIN-02 (v1); PATH-01 / COV-01 / COV-02 (v2, out of scope).
- `.planning/ROADMAP.md` § "Phase 1: Trustworthy Line Numbers" — success criteria.

### Codebase map (authoritative bug documentation)
- `.planning/codebase/CONCERNS.md` § "Known Bugs" — exact descriptions + file:line locations of both bugs.
- `.planning/codebase/ARCHITECTURE.md` — the parse→classify→reconcile pipeline and the `Block.StartLine` → `Issue.Lines` flow.
- `.planning/codebase/TESTING.md` — golden corpus + `testgen` oracle/fuzz harness layout.

### External docs
- None beyond the above — requirements fully captured in the decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `core/testgen/property_test.go` — the regression harness (`assertStrict` + the 10-seed loop). PIN-01 lives here.
- `core/testgen/oracle.go` — `Expected()` already records each issue's `Lines` from `Node.Line`; `core/testgen/render.go` sets `Node.Line` to the **statement** line (it emits any `# comment` first, *then* records the line). So the oracle already encodes correct statement lines — once the engine is fixed, per-issue line assertions should match without oracle changes.

### Established Patterns
- `core/analyze` is shell-free (depends on the `shell.Provider` interface, not `zsh`). The LINE-02 change is confined to `core/shell/zsh/parse.go`; the reconciler in `core/analyze` needs **no** edit.
- Deterministic issue sort (Kind, then Name) at `analyzer.go:91-96` — unaffected by this work.

### Integration Points (exact change sites)
- `core/analyze/analyzer.go:28` — LINE-01 line-count formula.
- `core/shell/zsh/parse.go:33-50` — LINE-02: stop overwriting `startLine` in the comment loop (keep the `start` offset update for Text).
- `core/shell/zsh/parse.go:22-27` — opaque whole-file fallback sets `StartLine: 1`; already consistent with "statement line" (whole file starts at line 1) — no change needed.
- `core/analyze/reconciler.go:46,69,103,109` — these read `StartLine`; **no change** (the payoff of the redefine approach).
- `core/testgen/property_test.go:18,89-92` — flip `checkLines`, implement the line assertions.
- `core/testdata/fixtures/manifests.json` + `core/analyze/corpus_test.go` — keep green; `empty.zsh` → 0 lines.

### Watch-outs for planning/research (high-value)
- **The oracle omits the total `Lines`.** `Expected()` does not set `Analysis.Lines` (zero-valued today). Enabling the total-`Lines` assertion needs an *independent* expected-total source — derive it deterministically from the graph/render (`RenderZsh` already tracks a running `line` counter), **not** by re-applying the engine's own corrected formula (that would be circular and prove nothing).
- **PIN-01 only pins LINE-02 if generated configs contain leading comments.** The mis-attribution trigger is a statement with a `#` comment directly above it. Confirm the generator emits comment-bearing nodes (`Node.Comment`); if `propParams()`/`Build` don't, add a targeted commented-statement case (a duplicate or shadow with a leading comment) so the pin actually exercises the bug rather than passing vacuously.

</code_context>

<specifics>
## Specific Ideas

- The user explicitly chose "redefine `StartLine`" over adding a field, valuing the zero-churn fix (issues already read `StartLine`, so the reconciler is untouched).

</specifics>

<deferred>
## Deferred Ideas

- **Path-segment mis-naming** in `dupPathIssues` (`./scripts` reported as `/scripts`; unrooted entries skipped) — v2 (PATH-01).
- **Golden fixtures for `duplicate_path` / `shadowed`** + `issue_names` / `issue_lines` corpus assertions — v2 (COV-01 / COV-02).

None of these arose as scope creep — they were pre-scoped to v2 during `/gsd:new-project`. Discussion stayed within phase scope.

</deferred>

---

*Phase: 1-Trustworthy Line Numbers*
*Context gathered: 2026-06-24*
