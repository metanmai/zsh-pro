---
phase: 01-trustworthy-line-numbers
plan: 01
subsystem: testing
tags: [line-count, analyzer, off-by-one, go, bytes-count, tdd]

# Dependency graph
requires: []
provides:
  - "Editor-style countLines(src) helper in core/analyze/analyzer.go (empty=0, trailing newline not over-counted, unterminated final line counts)"
  - "Analysis.Lines is now the true editor-style line count, guaranteed >= every issue's 1-based statement line"
  - "TestCountLines unit pin locking the six LINE-01 edge cases"
  - "TestAnalyzeBucketsCategoriesAndFlagsSecrets updated to the corrected count (want 5)"
affects:
  - "01-02 (per-issue line trust — relies on Lines being a trustworthy ceiling)"
  - "01-03 (testgen total-Lines pin — flips checkLines=true against this corrected Lines)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Function-per-concern package-private helper (countLines) with empty-input guard as the first statement"
    - "bytes.Count over []byte source instead of strings.Count(string(src), ...) to avoid the string allocation"

key-files:
  created: []
  modified:
    - core/analyze/analyzer.go
    - core/analyze/analyze_test.go

key-decisions:
  - "D-03: editor-style line count — empty file is 0; add 1 only when the final byte is not a newline"
  - "Removed the now-unused strings import and added bytes (strings was used ONLY by the old +1 formula in this file)"
  - "Unit-tested countLines directly via the internal test package rather than only through Analyze, to lock all edge cases including the empty-file panic guard"

patterns-established:
  - "Editor-style line counting: len(src)==0 -> 0; else bytes.Count(\\n) + (lastByte != \\n ? 1 : 0)"
  - "Empty-input guard is the first statement in any helper that indexes src[len(src)-1]"

requirements-completed: [LINE-01]

# Metrics
duration: 4min
completed: 2026-06-24
---

# Phase 1 Plan 01: Line-Count Off-By-One (LINE-01) Summary

**Replaced `Analysis.Lines = strings.Count(string(src), "\n") + 1` with an editor-style `countLines` helper so an empty file reports 0 and a trailing-newline file is no longer over-counted by one.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-06-24T07:34Z
- **Completed:** 2026-06-24T07:38:07Z
- **Tasks:** 2 (Task 1 via TDD RED→GREEN)
- **Files modified:** 2

## Accomplishments
- Added `countLines(src []byte) int` to `core/analyze/analyzer.go` with editor-style semantics and an `if len(src) == 0 { return 0 }` guard as its first statement (no panic on `empty.zsh`).
- Wired `Analysis.Lines = countLines(src)` into `Analyze`, removing the off-by-one `+ 1` formula at the former line 28.
- Swapped the `strings` import for `bytes` (`strings` was used only by the deleted formula in this file; `bytes.Count` avoids the `string(src)` allocation).
- Locked the corrected behaviour with a new table-driven `TestCountLines` (6 cases) and updated the one existing assertion in `TestAnalyzeBucketsCategoriesAndFlagsSecrets` from `want 6` to `want 5`.

## Task Commits

Each task was committed atomically (Task 1 followed the TDD test→feat cycle):

1. **Task 1 (RED): failing TestCountLines** - `4629159` (test)
2. **Task 1 (GREEN): countLines helper + wiring + import swap** - `5e4c148` (feat)
3. **Task 2: corrected Lines assertion (want 5)** - `1f1705f` (test)

No REFACTOR commit was needed — the GREEN implementation was already minimal and gofmt-clean.

**Plan metadata:** committed separately after this SUMMARY (docs: complete plan).

_Note: TDD tasks may have multiple commits (test → feat → refactor)._

## Files Created/Modified
- `core/analyze/analyzer.go` - Added the `countLines` helper; `Lines: countLines(src)`; imports now `bytes` (not `strings`).
- `core/analyze/analyze_test.go` - Added `TestCountLines`; updated the `a.Lines` assertion (condition, message, and inline comment) to expect 5.

## Behaviour locked (countLines)

| Input | Old | New (correct) |
|-------|-----|---------------|
| `""` (0 bytes) | 1 | 0 |
| `"foo"` | 1 | 1 |
| `"foo\n"` | 2 | 1 |
| `"foo\nbar"` | 2 | 2 |
| `"foo\nbar\n"` | 3 | 2 |
| `"\n\n\n\n\n"` | 6 | 5 |

## Decisions Made
- Followed locked decision **D-03** exactly: empty file → 0; otherwise newline count plus 1 only when the last byte is not `\n`.
- Removed the `strings` import rather than keeping it: in `analyzer.go` it was referenced ONLY by the old `strings.Count(string(src), "\n") + 1` formula, so leaving it would have failed the build with an unused-import error. The plan anticipated this ("remove it only if it becomes genuinely unused").
- Used `bytes.Count(src, []byte{'\n'})` to match the `[]byte` argument and avoid the `string(src)` conversion (RESEARCH "Don't Hand-Roll").
- Added a direct unit test of `countLines` (internal `package analyze` test) in addition to the existing through-`Analyze` assertion, so the empty-file guard and every edge case are pinned independently.

## Deviations from Plan

None - plan executed exactly as written. (The `strings`→`bytes` import swap was explicitly contemplated by Task 1's action text and is not a deviation.)

## Issues Encountered
None. The RED test failed for the expected reason (`undefined: countLines`), GREEN passed on first implementation, and the full repository test suite (`go test ./...`) is green.

## Verification Evidence
- `GOTOOLCHAIN=auto go build ./...` — succeeds (no unused-import error after the `strings`→`bytes` swap).
- `GOTOOLCHAIN=auto go vet ./core/analyze/` — clean; `gofmt -l` reports both files clean.
- `GOTOOLCHAIN=auto go test ./core/analyze/ -count=1` — `ok` (TestCountLines, TestAnalyzeBucketsCategoriesAndFlagsSecrets with `want 5`, and the golden corpus incl. `empty.zsh` which now exercises the empty-file guard).
- `GOTOOLCHAIN=auto go test ./... -count=1` — all packages `ok` (testgen still green; its `checkLines` pin against the corrected total Lines lands in plan 01-03).
- `core/analyze/analyzer.go` no longer contains `strings.Count(string(src), "\n") + 1`; `func countLines` exists with `if len(src) == 0 { return 0 }` as its first statement.

## Next Phase Readiness
- LINE-01 satisfied: `Analysis.Lines` is the true editor-style line count and is a trustworthy ceiling for per-issue lines.
- Plan 01-02 (LINE-02 / per-issue statement-line attribution in `parse.go`) is unblocked — this plan deliberately touched neither `parse.go`, the reconciler, nor any testgen/corpus file.
- Plan 01-03 will flip `core/testgen/property_test.go` `checkLines` to `true` and assert the total `Lines` against this corrected value; no contradictory assertions were introduced here.

## Self-Check: PASSED

- FOUND: `.planning/phases/01-trustworthy-line-numbers/01-01-SUMMARY.md`
- FOUND: `core/analyze/analyzer.go`
- FOUND commits: `4629159` (test/RED), `5e4c148` (feat/GREEN), `1f1705f` (test/Task 2)
- Artifact contracts: `func countLines` present; `Lines: countLines(src)` wired (gofmt-aligned); `want 5` present; old `+ 1` formula absent.

---
*Phase: 01-trustworthy-line-numbers*
*Completed: 2026-06-24*
