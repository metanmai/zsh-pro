---
phase: 01-trustworthy-line-numbers
plan: 02
subsystem: parser
tags: [line-attribution, zsh, mvdan-sh, ast, go]

# Dependency graph
requires:
  - phase: 01-trustworthy-line-numbers (plan 01-01)
    provides: LINE-01 editor-style line count (independent fix; same phase, no code coupling)
provides:
  - "Block.StartLine = the statement's own line (not the leading-comment line) for every parsed block"
  - "Per-issue line attribution corrected for free at reconciler.go:46,69,103,109 (duplicate alias/env, duplicate path, shadowed)"
  - "parse_test.go assertion pinning StartLine to the statement line (want 2)"
affects: [01-03 (testgen oracle pin PIN-01/PIN-02 — relies on corrected per-issue lines), reconciler line-reporting consumers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Redefine-in-place over add-a-field: a single-consumer struct value (Block.StartLine) is repurposed by deleting the overwrite rather than introducing a parallel field (D-01/D-02)"

key-files:
  created: []
  modified:
    - core/shell/zsh/parse.go
    - core/shell/zsh/parse_test.go

key-decisions:
  - "D-01: Block.StartLine redefined to the statement line by deleting the startLine = c.Pos().Line() overwrite in the comment-pull-up loop; the start-offset update is kept so Block.Text still includes the comment."
  - "D-02: No new Block field added; core/analyze/reconciler.go untouched — its four StartLine readers inherit the fix unchanged."

patterns-established:
  - "Zero-churn correctness fix: when exactly one production reader path consumes a value, fix the producer in place rather than threading a new field through the model."

requirements-completed: [LINE-02]

# Metrics
duration: 1min
completed: 2026-06-24
---

# Phase 1 Plan 02: Per-Issue Line Mis-Attribution (LINE-02) Summary

**Deleting one `startLine = c.Pos().Line()` overwrite in the zsh comment-pull-up loop makes `Block.StartLine` report the statement's own line instead of the leading-comment line — every issue (duplicate alias/env, duplicate path, shadowed) inherits the fix unchanged via reconciler.go, with no new model field.**

## Performance

- **Duration:** ~1 min
- **Started:** 2026-06-24T13:12:24+05:30 (first task commit)
- **Completed:** 2026-06-24T13:12:48+05:30 (second task commit)
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Removed the bug: the comment-pull-up loop in `core/shell/zsh/parse.go` no longer overwrites `startLine` with the comment's line. `startLine` now stays at its line-33 initialization (`stmt.Pos().Line()` — the statement's own line).
- Preserved Text context: the `start = c.Pos().Offset()` update is kept, so `Block.Text` still begins at the pulled-up `#` comment.
- Updated the loop's doc comment to state the new invariant (StartLine = statement line; Text carries the comment for context).
- Updated the `TestParseClassifiesKinds` assertion from `want 1` to `want 2` (the alias statement is on line 2; line 1 is its leading comment).
- Confirmed the reconciler is untouched and inherits the fix: `core/analyze/reconciler.go` is NOT in the plan diff.

## Task Commits

Each task was committed atomically:

1. **Task 1: Delete the startLine overwrite + update the loop doc comment (LINE-02, D-01/D-02)** - `2aff7c7` (fix)
2. **Task 2: Update the StartLine assertion to the statement line (want 2) + run zsh/analyze suites** - `abd4086` (test)

_Note: Task 1 is flagged `tdd="true"`, but its action is a pure deletion of the buggy overwrite; the behavioral lock lives in Task 2's `want 1 -> want 2` assertion (which fails against pre-fix code and passes after). Recorded as a single fix commit per the plan's `<action>`._

## Files Created/Modified
- `core/shell/zsh/parse.go` - Deleted `startLine = c.Pos().Line()` (line 39) inside the comment-pull-up loop; kept the `start = c.Pos().Offset()` update; rewrote the loop doc comment to state StartLine = statement line.
- `core/shell/zsh/parse_test.go` - `TestParseClassifiesKinds`: StartLine assertion changed from `!= 1`/`want 1 (leading comment)` to `!= 2`/`want 2 (statement line, not leading-comment line)`, with an updated explanatory comment.

## Decisions Made
None beyond the pre-locked D-01/D-02 — followed the plan as specified. The redefine-in-place approach (delete the overwrite, add no field, touch no reconciler) was the user's locked choice; the executor only chose the exact wording of the doc comment and the test message.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## Verification

- `GOTOOLCHAIN=auto go build ./core/shell/zsh/` and `GOTOOLCHAIN=auto go vet ./core/shell/zsh/` — clean.
- Guard greps: `startLine := stmt.Pos().Line()` (line 33) intact; `startLine = c.Pos().Line()` gone.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh/ -run TestParseClassifiesKinds -count=1` — PASS (confirmed a real test, not "no tests to run").
- `GOTOOLCHAIN=auto go test ./core/shell/zsh/ ./core/analyze/ -count=1` — both packages `ok`.
- `git diff --name-only HEAD~2 HEAD` = `core/shell/zsh/parse.go`, `core/shell/zsh/parse_test.go` only — `core/analyze/reconciler.go` NOT in the diff (D-02 honored); no new Block field added (D-01 satisfied via deletion).
- Pre-existing uncommitted `.planning/config.json` change left untouched and unstaged throughout, per plan constraints.

## Next Phase Readiness
- LINE-02 is satisfied: with a leading `#` comment, every issue reports the statement's own line. The analyze package stays green against the corrected attribution (its mockProvider tests use directly-set StartLine integers and are unaffected).
- Plan 01-03 (PIN-01/PIN-02, testgen oracle + corpus) can now pin the corrected per-issue lines. Per RESEARCH § "Pattern 5", note that today only the shadow node carries a `Comment`, so 01-03 should add leading comments to the dup-alias/reassigned-env planted nodes for the LINE-02 pin to exercise those issue kinds non-vacuously.

## Self-Check: PASSED

- FOUND: core/shell/zsh/parse.go
- FOUND: core/shell/zsh/parse_test.go
- FOUND: .planning/phases/01-trustworthy-line-numbers/01-02-SUMMARY.md
- FOUND commit: 2aff7c7 (Task 1 fix)
- FOUND commit: abd4086 (Task 2 test)

---
*Phase: 01-trustworthy-line-numbers*
*Completed: 2026-06-24*
