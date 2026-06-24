---
phase: 01-trustworthy-line-numbers
plan: 03
subsystem: testing
tags: [testgen, oracle, property-test, golden-corpus, line-pin, go]

# Dependency graph
requires:
  - phase: 01-trustworthy-line-numbers (plan 01-01)
    provides: LINE-01 editor-style countLines — verified present before pinning (analyzer.go:104)
  - phase: 01-trustworthy-line-numbers (plan 01-02)
    provides: LINE-02 Block.StartLine = statement line — verified present before pinning (parse.go:33, overwrite gone)
provides:
  - "ConfigGraph.RenderedLines side-effect field — RenderZsh's non-circular render total, read by Expected()"
  - "Expected() now sets Analysis.Lines = g.RenderedLines (total Lines no longer omitted by the oracle)"
  - "Planted dupAlias/dupEnv pairs carry a leading comment on the first node — LINE-02 exercised across duplicate_alias, reassigned_env, and shadowed"
  - "property_test.go checkLines = true with an implemented gated block asserting total Lines + per-issue line slices across all 10 seeds"
  - "Optional `Lines *int` corpus manifest field with a conditional assertion; empty.zsh pinned at lines 0"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Side-effect field over signature change: RenderZsh writes ConfigGraph.RenderedLines so all 5 []byte-only callers keep compiling (RESEARCH Pattern 3)"
    - "Non-circular oracle total: the expected Lines comes from RenderZsh's running counter, never from re-applying the engine's countLines/strings.Count (Pitfall 2)"
    - "Optional pointer manifest field (Lines *int) so opt-in fixtures assert an exact value while existing fixtures whose counts shift stay green (Pitfall 6)"

key-files:
  created:
    - core/testgen/renderedlines_test.go
  modified:
    - core/testgen/graph.go
    - core/testgen/render.go
    - core/testgen/oracle.go
    - core/testgen/generator.go
    - core/testgen/property_test.go
    - core/analyze/corpus_test.go
    - core/testdata/fixtures/manifests.json

key-decisions:
  - "D-04: checkLines flipped false -> true; gated block asserts total Lines (LINE-01) AND each shared issue's Lines slice (LINE-02) across all 10 seeds, reusing the existing key/fail closures"
  - "D-05: golden corpus stays minimal — only empty.zsh opts into a Lines assertion (lines 0); no issue_lines/issue_names assertions added (declined v2 scope)"
  - "Total Lines sourced non-circularly from g.RenderedLines (RenderZsh counter), not the engine formula — oracle.go contains no countLines/strings.Count"

patterns-established:
  - "Pin via the primary regression harness: the 10-seed oracle property test asserts both correctness fixes; a regression of either now fails the testgen package"

requirements-completed: [PIN-01, PIN-02]

# Metrics
duration: 9min
completed: 2026-06-24
---

# Phase 1 Plan 03: Pin Trustworthy Line Numbers (PIN-01, PIN-02) Summary

**Locks the LINE-01 (total-Lines) and LINE-02 (per-issue statement-line) fixes behind the 10-seed oracle property test plus the golden corpus — flipping `checkLines` to true with a real assertion block, sourcing the expected total non-circularly from a new `ConfigGraph.RenderedLines` render counter, and giving planted dupAlias/dupEnv pairs a leading comment so LINE-02 is exercised beyond shadows.**

## Pre-flight: fixes verified present before pinning

Per the objective, both depended-on fixes were confirmed in the code BEFORE any pinning (would have STOPPED and reported if missing):

- **LINE-01 present** — `core/analyze/analyzer.go:28` is `Lines: countLines(src)` and `countLines` (lines 104-113) has the editor-style semantics with `if len(src) == 0 { return 0 }` first; the old `strings.Count(string(src), "\n") + 1` formula is gone.
- **LINE-02 present** — `core/shell/zsh/parse.go:33` sets `startLine := stmt.Pos().Line()` and the comment-pull-up loop (lines 38-42) updates only `start` (the offset), never `startLine`; the `startLine = c.Pos().Line()` overwrite is gone.

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-24T09:21Z (approx, first task commit)
- **Completed:** 2026-06-24T09:30:13Z
- **Tasks:** 3 (Tasks 1 & 2 via TDD)
- **Files modified:** 7 + 1 test file created

## Accomplishments

- Added `ConfigGraph.RenderedLines int` (graph.go) as a documented side-effect field; `RenderZsh` sets `g.RenderedLines = line - 1` from its own running counter just before returning, with the `[]byte`-only signature unchanged (all 5 callers still compile).
- `Expected()` (oracle.go) now sets `a.Lines = g.RenderedLines` — the oracle previously never set a total Lines. No `countLines`/`strings.Count` call was introduced (non-circular, Pitfall 2). Updated the stale `Expected()` doc comment that claimed the engine "may differ … (a known bug)".
- `generator.go Build()` adds `Comment: fmt.Sprintf("first %s", name)` to the FIRST node of each planted dupAlias and dupEnv pair (second nodes left comment-free), so the rendered source places a `# comment` line above the first occurrence — exercising LINE-02 for duplicate_alias and reassigned_env, not only shadowed.
- `property_test.go`: flipped `const checkLines = false` -> `true`, removed the stale "Flip to true once those are fixed" comment, and implemented the previously-empty gated block to assert `got.Lines == want.Lines` then compare each shared issue's `Lines` slice via `fmt.Sprint` (reusing the existing `key` and `fail` closures). `TestOracleProperty` passes for all 10 seeds with the gate on.
- `corpus_test.go`: added an OPTIONAL `Lines *int` (`json:"lines"`) field to the `manifest` struct with a conditional `if m.Lines != nil && a.Lines != *m.Lines` assertion; `manifests.json` `empty.zsh` now declares `"lines": 0` (the only opt-in entry). No `issue_lines`/`issue_names` assertions added (D-05).

## Task Commits

Each task committed atomically (specific paths; never `git add -A`):

1. **Task 1 (RED): failing RenderedLines + dup-comment pins** — `e3f115d` (test)
2. **Task 1 (GREEN): RenderedLines plumbing + oracle wiring + generator comments** — `a86b328` (feat)
3. **Task 2: checkLines=true + line-assertion block** — `d56303a` (test)
4. **Task 3: optional corpus Lines + empty.zsh lines 0** — `0266701` (test)

No REFACTOR commit was needed for Task 1 — the GREEN implementation was minimal and gofmt-clean. Task 2's flip+block IS the behavioral lock (test and assertion live in the same file), so it is a single `test` commit. Task 3 is `type="auto"` (not TDD) and is a single `test` commit.

**Plan metadata:** committed separately after this SUMMARY (docs: complete plan), bundling STATE.md and ROADMAP.md.

## TDD Gate Compliance

Task 1 followed RED -> GREEN explicitly: the RED commit (`e3f115d`) failed to compile (`g.RenderedLines undefined`) — the expected RED reason — and GREEN (`a86b328`) made it pass. Task 2 is itself a test-harness change (flip the gate + implement assertions); its lock is the now-active 10-seed comparison. Task 3 is non-TDD by design (manifest/struct wiring).

## Files Created/Modified

- `core/testgen/graph.go` — added `RenderedLines int` field (with doc comment) to `ConfigGraph`.
- `core/testgen/render.go` — `RenderZsh` sets `g.RenderedLines = line - 1` before `return`; signature unchanged.
- `core/testgen/oracle.go` — `Expected()` sets `a.Lines = g.RenderedLines`; refreshed the stale doc comment.
- `core/testgen/generator.go` — leading `Comment` on the first node of each dupAlias and dupEnv planted pair.
- `core/testgen/property_test.go` — `checkLines = true`; implemented the gated total + per-issue line assertions; removed stale comment.
- `core/testgen/renderedlines_test.go` — NEW internal-package test: `TestRenderedLinesMatchesSource` (RenderedLines == editor count of rendered bytes, Expected().Lines == RenderedLines) and `TestDupAliasAndDupEnvCarryLeadingComment` (first node of each dup pair has a comment, second does not).
- `core/analyze/corpus_test.go` — optional `Lines *int` manifest field + conditional assertion.
- `core/testdata/fixtures/manifests.json` — `empty.zsh` gains `"lines": 0` (only entry changed).

## Decisions Made

- Followed locked **D-04** and **D-05** exactly. Wrote `renderedlines_test.go` in the internal `package testgen` so it can read the unexported-from-the-external-package `RenderedLines`/`Comment`/node kinds directly (mirrors graph_test.go / oracle_test.go).
- For the RED test's non-circularity proof, the test computes the expected total via a local `editorLines` over the rendered bytes (newline-count style) rather than calling the engine — proving `RenderedLines` is a faithful render total without coupling to `core/analyze`.

## Deviations from Plan

None — plan executed exactly as written. The new `renderedlines_test.go` is the TDD RED artifact for Task 1 (`tdd="true"`), not a scope addition; it pins exactly the behavior Task 1's `<behavior>` block specifies.

## Issues Encountered

None. Baseline was green before changes; the RED test failed for the expected reason (`g.RenderedLines undefined`); GREEN passed on first implementation; the property test passed across all 10 seeds on first run with the gate on (because the depended-on fixes were already in place).

## Verification Evidence

Final gate — all green (`GOTOOLCHAIN=auto`):

- `go build ./...` — BUILD OK.
- `go vet ./...` — VET OK.
- `gofmt -l core/` — clean (no files listed).
- `go test ./... -count=1` — all packages `ok` (analyze, buildinfo, cli, cmd/zsh-gen, model, render, shell/zsh, testgen).
- `go test ./core/testgen/ -run TestOracleProperty -count=1 -v` — PASS for seeds 1, 2, 3, 5, 8, 13, 21, 34, 55, 89 with `checkLines = true`.
- `go test ./core/analyze/ -run TestCorpusGolden -count=1 -v` — PASS including `empty.zsh` asserting `lines == 0`.

Non-vacuity / regression-pin proofs (temporary, reverted):

- A throwaway sanity test confirmed every property seed yields oracle `Lines > 0` and at least one issue with a non-empty `Lines` slice — so the gated assertions are meaningful, not skipped.
- Temporarily setting `empty.zsh` to `"lines": 99` made `TestCorpusGolden/empty.zsh` FAIL with `lines = 0, want 99`; restoring to `0` passes — proving the optional corpus assertion actually fires.

Guard greps:

- `core/testgen/graph.go` contains `RenderedLines int`; `render.go` contains `g.RenderedLines = line - 1`; `oracle.go` contains `a.Lines = g.RenderedLines` and NO `strings.Count`/`countLines`.
- `property_test.go` contains `const checkLines = true` and no longer contains `Flip to true`.
- `manifests.json` has exactly one `"lines"` key (`empty.zsh`); `python3 -m json.tool` validates.

Scope/safety:

- Diff across the plan touches exactly the 7 plan-listed files plus the TDD test file; `git diff --diff-filter=D` reports no deletions; no untracked files left behind.
- The pre-existing uncommitted `.planning/config.json` change was never staged or modified (per plan constraint).

## Next Phase Readiness

- PIN-01 satisfied: the 10-seed oracle property test now asserts total `Lines` (LINE-01) and per-issue line slices (LINE-02) — a regression of either fix fails `go test ./core/testgen/`.
- PIN-02 satisfied: the golden corpus passes against the corrected output with `empty.zsh` pinned at 0 lines; the optional `Lines *int` field lets future fixtures opt into exact line assertions without disturbing existing ones.
- Phase 01 (trustworthy-line-numbers) is now complete across all 3 plans: LINE-01 fixed (01-01), LINE-02 fixed (01-02), both pinned (01-03).

## Self-Check: PASSED

- FOUND: `.planning/phases/01-trustworthy-line-numbers/01-03-SUMMARY.md`
- FOUND: `core/testgen/graph.go`, `core/testgen/render.go`, `core/testgen/oracle.go`, `core/testgen/generator.go`, `core/testgen/property_test.go`, `core/testgen/renderedlines_test.go`, `core/analyze/corpus_test.go`, `core/testdata/fixtures/manifests.json`
- FOUND commits: `e3f115d` (RED), `a86b328` (GREEN), `d56303a` (Task 2), `0266701` (Task 3)
- Artifact contracts: `RenderedLines int` present; `g.RenderedLines = line - 1` present; `a.Lines = g.RenderedLines` present; `const checkLines = true` present; `"lines": 0` present; `Lines *int` present.

---
*Phase: 01-trustworthy-line-numbers*
*Completed: 2026-06-24*
