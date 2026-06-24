---
phase: 01-trustworthy-line-numbers
reviewed: 2026-06-24T00:00:00Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - core/analyze/analyzer.go
  - core/shell/zsh/parse.go
  - core/testgen/graph.go
  - core/testgen/render.go
  - core/testgen/oracle.go
  - core/testgen/generator.go
  - core/testdata/fixtures/manifests.json
  - core/analyze/analyze_test.go
  - core/shell/zsh/parse_test.go
  - core/testgen/property_test.go
  - core/testgen/renderedlines_test.go
  - core/analyze/corpus_test.go
findings:
  critical: 0
  warning: 2
  info: 3
  total: 5
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-06-24T00:00:00Z
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

Adversarial review of the "trustworthy line numbers" phase: the editor-style
`countLines` helper (LINE-01), the `parse.go` comment-pull-up `StartLine` fix
(LINE-02), and the testgen pinning machinery (`RenderedLines` side-effect field,
oracle wiring, planted-pair leading comments, `checkLines=true`, optional corpus
`Lines`). I started from the hypothesis that the line-number logic was wrong and
tried to break it; the two production fixes survived that scrutiny.

**The two core fixes are correct.** I verified `countLines` against all the
adversarial edge cases — empty file (0), no trailing newline, single/multiple
trailing newlines, only-newlines, and CRLF (`\r\n` lines count correctly because
only `\n` is counted and the final byte is `\n`). I verified the `parse.go`
`StartLine` now reports the statement line (not the pulled-up comment line)
across trailing/below/multiple-leading/CRLF comment layouts. I confirmed the
property test's line assertions are **non-vacuous**: for seed 1, four issues
(`duplicate_alias`, `duplicate_path`, `reassigned_env`, `shadowed`) carry line
slices that are compared and match exactly, including the comment-pull-up cases
(`# first rl` on line 43, statement on 44, attributed to 44). I confirmed
`RenderedLines` is genuinely non-circular: it is an incremental running counter
in `RenderZsh`, not a re-count of bytes via the engine's formula, and it matches
a true editor count across empty/single/multi-line/comment-prefixed/pathological
graphs.

**Constraints all pass:** `gofmt -l` clean, `go vet ./...` clean, full suite
green. No new dependencies (`go.mod` direct deps = `mvdan.cc/sh/v3` only; the new
`bytes` import is stdlib). Layering intact: `core/testgen` production imports are
`core/model` + stdlib only; `core/analyze` stays shell-free (`core/model`,
`core/shell` interface, stdlib — no `core/shell/zsh`). The changed `Lines`/issue
`lines` wire values are the deliberate, accepted contract change and are NOT
reported as defects.

The findings below are real but secondary: two latent footguns in the
test-infrastructure layer (silent wrong-oracle if call order is violated; an
unguarded slice in `pick`) and minor robustness/coverage observations. None
block shipping the line-number fixes.

## Warnings

### WR-01: `Expected()` silently produces a wrong oracle if `RenderZsh()` was not called first

**File:** `core/testgen/oracle.go:18-48` (also `graph.go:36-42`, `render.go:8-33`)
**Issue:** `Expected()` reads `g.RenderedLines` for the total and `Node.Line` for
per-issue lines, both of which are populated only as a side effect of
`RenderZsh()`. If a caller invokes `Expected()` without first calling
`RenderZsh()`, there is no guard, no error, and no panic — it silently returns
`Analysis.Lines = 0` and every issue's `Lines = [0, 0]`. I verified this directly:

```
RenderedLines (unset) = 0
Expected().Lines = 0
issue duplicate_alias/kc lines=[0 0]
```

Because this is the *oracle* (the source of truth the property test asserts the
engine against), a silently-zeroed oracle is exactly the kind of regression that
makes a strict line assertion vacuously pass or produce a confusing failure.
The ordering coupling is documented in three doc comments and is even called out
as an anti-pattern in `CLAUDE.md` ("Calling `RenderZsh` after `Expected` in
testgen"), and all current callers honor it — so this is a latent footgun rather
than an active bug, hence WARNING not BLOCKER.

**Fix:** Make the contract enforceable instead of advisory. Either have
`Expected()` lazily render if it has not run, or fail loudly when the side-effect
state is absent. Minimal loud-fail guard:

```go
func (g *ConfigGraph) Expected() model.Analysis {
	if g.RenderedLines == 0 && len(g.Nodes) > 0 {
		panic("testgen: Expected() called before RenderZsh(); Node.Line/RenderedLines unset")
	}
	var a model.Analysis
	a.Lines = g.RenderedLines
	// ...
}
```

(Or, preferred: add a `rendered bool` flag set by `RenderZsh` and asserted in
`Expected`, so a legitimately empty graph with `RenderedLines == 0` is still
distinguishable from an unrendered one.)

### WR-02: `pick` panics on an out-of-range slice when a defect count exceeds its pool

**File:** `core/testgen/generator.go:49-53` (consumers at `62-69`)
**Issue:** `pick` ends with `return cp[:n]`. If `n` exceeds `len(pool)`, this is
an out-of-range slice expression and panics. `Build` calls `pick` with sums like
`p.Aliases + p.DupAliases + p.Shadows`; the `GenParams` doc comment delegates the
fit-within-pool obligation to the caller ("Defect counts must fit within the
relevant pool (caller's responsibility)"). The current `propParams`/`sampleParams`
all fit (alias draw 6 ≤ 10, env 5 ≤ 10, path 4 ≤ 7, func 3 ≤ 8, secret 1 ≤ 6), so
nothing panics today. But a future seed-matrix or param bump that overshoots a
pool turns a "config too big" mistake into an opaque slice panic with no hint at
which pool overflowed.

**Fix:** Bounds-check in `pick` and fail with an actionable message:

```go
func (gen *Generator) pick(pool []string, n int) []string {
	if n > len(pool) {
		panic(fmt.Sprintf("testgen: pick(%d) exceeds pool size %d", n, len(pool)))
	}
	cp := append([]string(nil), pool...)
	gen.rng.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp[:n]
}
```

## Info

### IN-01: Property test compares issue lines only for the intersection of want/got issue keys

**File:** `core/testgen/property_test.go:97-106`
**Issue:** The line-slice comparison loops over `got.Issues` and checks lines only
when the key also exists in `wantLines` (`if wl, ok := wantLines[k]; ok && ...`).
Issue-set equality is asserted separately at lines 84-88, so in practice every
key is in both maps and all four issues are compared (verified: 4 non-vacuous
comparisons for seed 1). The check is therefore sound today. The note is that the
line assertion's completeness is *implicitly* guaranteed by the earlier set-
equality assertion rather than by the line loop itself; if the set-equality block
were ever weakened, the line loop would degrade to comparing only the overlap
without complaint.

**Fix:** Optional hardening — assert that the number of compared line-slices
equals `len(want.Issues)` so the line check cannot silently become partial:

```go
compared := 0
for _, is := range got.Issues {
	if wl, ok := wantLines[key(is)]; ok {
		compared++
		if fmt.Sprint(is.Lines) != fmt.Sprint(wl) { /* fail */ }
	}
}
if compared != len(want.Issues) {
	fail("only %d/%d issue line-slices compared", compared, len(want.Issues))
}
```

### IN-02: `RenderedLines` non-circularity is validated against the engine's own formula

**File:** `core/testgen/renderedlines_test.go:14-23` (`editorLines`) vs
`core/analyze/analyzer.go:104-113` (`countLines`)
**Issue:** `TestRenderedLinesMatchesSource` cross-checks `RenderedLines` against
`editorLines`, which is a byte-for-byte reimplementation of the engine's
`countLines`. This is acceptable and the intent is documented — it proves the
incremental render counter equals an editor count without *importing* the engine,
and the property test does the truly-independent comparison (engine `countLines`
vs oracle running counter). The observation is that the two formulas are now
duplicated in three places (`countLines`, `editorLines`, and the inline copies I
used to verify); a future change to editor semantics must be mirrored by hand in
each. Not a correctness defect — a duplication/maintenance note.

**Fix:** None required. If desired, document the deliberate triplication with a
one-line cross-reference comment in `editorLines` pointing at `analyzer.countLines`
so the "keep these in sync" obligation is explicit.

### IN-03: `parse.go` comment pull-up has no adjacency check, so a detached comment is absorbed into `Text`

**File:** `core/shell/zsh/parse.go:38-42`
**Issue:** The pull-up loop only requires `c.End().Offset() <= stmt.Pos().Offset()`
and `c.Pos().Offset() < start`; it does not require the comment to be immediately
adjacent. A comment separated from the statement by a blank line is still pulled
into `Text` (verified: `"# detached\n\nalias gs='x'"` → one block, `Text` includes
the detached comment). This does NOT affect `StartLine`, which correctly stays the
statement line (3 in that case) — so the phase's line-number contract is
unaffected. It is a pre-existing `Text`-composition behavior, surfaced here only
because LINE-02 touched this loop. Flagging for awareness, not as a regression.

**Fix:** None required for this phase. If `Text` fidelity matters later, gate the
pull-up on line adjacency (e.g. only absorb comments whose end line is exactly the
statement's start line minus the comment block's height).

---

_Reviewed: 2026-06-24T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
