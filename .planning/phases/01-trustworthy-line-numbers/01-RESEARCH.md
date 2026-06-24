# Phase 1: Trustworthy Line Numbers - Research

**Researched:** 2026-06-24
**Domain:** Go CLI brownfield modification — parser line attribution, off-by-one counter fix, testgen oracle pinning
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01 (LINE-02):** Fix mis-attribution by redefining `Block.StartLine` to the statement's own line. In `core/shell/zsh/parse.go`, the comment-pull-up loop (`:36-40`) must stop overwriting `startLine`; keep updating the `start` offset (so `Block.Text` still includes the leading comment), but leave `startLine = stmt.Pos().Line()`. Net result: `Block.StartLine` = the statement's line.
- **D-02:** No new `Block` field and no reconciler changes. The four issue sites (`reconciler.go:46,69,103,109`) already read `StartLine` and inherit the fix for free. Adding a separate `StmtLine` field was explicitly rejected.
- **D-03 (LINE-01):** Replace `analyzer.go:28` with editor-style counting: empty file → `0`; otherwise `Count("\n") + (lastByte != '\n' ? 1 : 0)`. A trailing-newline file is not over-counted; an unterminated final line still counts.
- **D-04 (PIN-01):** Flip `core/testgen/property_test.go:18` `checkLines` to `true` and implement the gated block (`:89-92`) to assert both total `Lines` and each issue's `lines` slice across all 10 seeds.
- **D-05 (PIN-02):** Golden corpus stays minimal — `manifests.json` + `corpus_test.go` must remain green against corrected output, with the `empty.zsh` case set to a `0`-line count. Do not add `issue_lines`/`issue_names` assertions to the corpus.

### Claude's Discretion

- Exact variable/comment wording, test-helper structure, and **how the oracle/test sources the expected total `Lines`** are left to the planner/executor.

### Deferred Ideas (OUT OF SCOPE)

- Path-segment mis-naming in `dupPathIssues` (`./scripts` reported as `/scripts`) — v2 (PATH-01).
- Golden fixtures for `duplicate_path`/`shadowed` + `issue_names`/`issue_lines` corpus assertions — v2 (COV-01/COV-02).
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LINE-01 | `analyze` reports `Analysis.Lines` as the true line count — empty file → `0`; trailing-newline file not over-counted | D-03 formula; `analyze_test.go:151` update; `empty.zsh` corpus entry update |
| LINE-02 | When a config statement has a leading `#` comment, every issue involving that statement reports the statement's own line, not the comment's line | D-01 parse.go fix; confirmed via full `StartLine` reader audit |
| PIN-01 | The testgen oracle property test asserts line numbers (`checkLines = true`) — total `Lines` and each issue's `lines` slice — across all seeds, and passes | D-04; oracle already encodes correct statement lines via `Node.Line`; total `Lines` sourced from render counter |
| PIN-02 | The golden corpus passes against corrected output; `empty.zsh` case reflects a `0`-line count | D-05; corpus_test.go asserts no `Lines` today, manifests.json needs `"lines": 0` for empty.zsh |
</phase_requirements>

---

## Summary

This phase corrects two related but distinct line-number bugs in the `zsh-pro` analyzer and locks them with two test pins. The codebase is a well-layered Go CLI; the changes are strictly contained to three files (`parse.go`, `analyzer.go`, `property_test.go`) plus two golden-data updates (`manifests.json`, one assertion update in `analyze_test.go`). No new dependencies, no new types, no reconciler changes.

The LINE-02 fix (stop overwriting `startLine` in the comment-pull-up loop at `parse.go:39`) is verified safe: the full `StartLine` reader audit found exactly four readers — all in `reconciler.go` (`:46,69,103,109`) — and one test assertion in `parse_test.go:54-55` that currently validates the **buggy** behavior and must be updated to expect the statement line. No other production code reads `StartLine`.

The LINE-01 fix (replace the `strings.Count+1` formula at `analyzer.go:28`) requires updating one existing test assertion: `analyze_test.go:151` currently asserts `a.Lines != 6` for a 5-newline source where the last byte is `\n`. Under the corrected formula this is still 5, not 6 — so the test must be updated from `want 6` to `want 5`. The `empty.zsh` file is confirmed to be 0 bytes; the new formula correctly yields 0 for it, so the corpus entry needs a `"lines": 0` field added.

PIN-01 (flip `checkLines = true`) is the most nuanced piece. The oracle already records correct statement lines in `Node.Line` (set by `RenderZsh` after the comment, at the statement line). What the oracle does NOT provide is `Analysis.Lines` (the total line count). The safe non-circular approach is to derive the expected total from `RenderZsh`'s final running `line` counter. PIN-01 also only exercises LINE-02 for the `shadowed` issue kind today, because only shadow nodes receive a `Comment` in `generator.go:105`. A minimal generator change — adding a leading comment to one node of the duplicate-alias and reassigned-env planted pairs — ensures the pin actually exercises LINE-02 across multiple issue kinds.

**Primary recommendation:** Four code edits across three files, two test assertion updates, one generator tweak, one manifests.json addition.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Line count (`Analysis.Lines`) | Engine (`core/analyze/analyzer.go`) | — | Computed once from raw source bytes at analysis time |
| Per-issue line attribution (`Issue.Lines`) | Parser (`core/shell/zsh/parse.go`) | Reconciler reader | `Block.StartLine` set in parser flows into issue lines via reconciler |
| Oracle expected lines (total) | Testgen render (`core/testgen/render.go`) | Property test | `RenderZsh` already tracks the running `line` counter; expose final value |
| Oracle expected per-issue lines | Testgen oracle (`core/testgen/oracle.go`) | — | `Node.Line` already encodes correct statement lines; no change needed |
| Comment attachment to generator nodes | Testgen generator (`core/testgen/generator.go`) | — | Controls which issue kinds exercise the LINE-02 trigger |

---

## Standard Stack

No new packages. This phase touches only existing code. [VERIFIED: CLAUDE.md constraint "no new dependencies"]

| Component | Current Version | Role in This Phase |
|-----------|----------------|---------------------|
| `mvdan.cc/sh/v3` | v3.13.1 | `stmt.Pos().Line()` is the authoritative 1-based statement line — used as-is |
| Go stdlib `strings` | 1.25 | `strings.Count` stays; formula around it changes |
| Go stdlib `testing` | 1.25 | All test changes use existing test helpers |

---

## Package Legitimacy Audit

No new packages are installed. N/A.

---

## Architecture Patterns

### System Architecture Diagram

```
Source bytes
     │
     ▼
parse.go: stmt.Pos().Line() ──────────────────► Block.StartLine = statement line
     │ (comment-pull-up loop updates start offset only, NOT startLine)
     ▼
Block.Text includes leading comment; Block.StartLine = statement line
     │
     ▼
reconciler.go: reads b.StartLine ─────────────► Issue.Lines (correct after fix)

Source bytes ──► analyzer.go:28 (new formula) ► Analysis.Lines (correct after fix)
```

### Recommended Project Structure

No structural changes. All edits are in-place modifications to existing files:

```
core/
  analyze/
    analyzer.go          # LINE-01: formula change at line 28
    analyze_test.go      # update Lines assertion at line 151
  shell/zsh/
    parse.go             # LINE-02: stop overwriting startLine at line 39
    parse_test.go        # update StartLine assertion at lines 54-55
  testgen/
    generator.go         # PIN-01: add Comment to dup-alias / reassigned-env nodes
    property_test.go     # PIN-01: flip checkLines=true + implement assertion block
  testdata/fixtures/
    manifests.json       # PIN-02: add "lines": 0 for empty.zsh
```

### Pattern 1: LINE-02 Fix — Stop Overwriting startLine in Comment Pull-Up

**What:** The comment-pull-up loop at `parse.go:36-41` currently overwrites both `start` (the byte offset) and `startLine` (the line number) when it finds a leading comment. The fix keeps the `start` offset update (so `Block.Text` still includes the comment) but removes the `startLine` reassignment.

**Before (buggy):**
```go
// Source: core/shell/zsh/parse.go:33-50 (verified by file read)
startLine := stmt.Pos().Line()
for _, c := range stmt.Comments {
    if c.End().Offset() <= stmt.Pos().Offset() && c.Pos().Offset() < start {
        start = c.Pos().Offset()
        startLine = c.Pos().Line()   // BUG: overwrites with comment line
    }
}
```

**After (fix):**
```go
startLine := stmt.Pos().Line()       // locked to statement line — never changed
for _, c := range stmt.Comments {
    if c.End().Offset() <= stmt.Pos().Offset() && c.Pos().Offset() < start {
        start = c.Pos().Offset()     // offset update kept: Text includes comment
        // startLine intentionally NOT updated here
    }
}
```

**Key invariant confirmed:** `stmt.Pos().Line()` from `mvdan.cc/sh/v3` is the 1-based line of the statement keyword/word itself, not any associated comment. The `KeepComments(true)` option attaches comments to `stmt.Comments`; `stmt.Pos()` remains the statement position. [VERIFIED: parse.go:32-33 reads `startLine := stmt.Pos().Line()` then immediately enters the comment loop]

### Pattern 2: LINE-01 Fix — Editor-Style Line Counter

**What:** Replace the naive `strings.Count("\n") + 1` formula with the editor-style variant at `analyzer.go:28`.

**Before (buggy):**
```go
// Source: core/analyze/analyzer.go:28 (verified by file read)
Lines: strings.Count(string(src), "\n") + 1,
```

**After (fix):**
```go
Lines: func() int {
    if len(src) == 0 {
        return 0
    }
    n := strings.Count(string(src), "\n")
    if src[len(src)-1] != '\n' {
        n++
    }
    return n
}(),
```

Alternatively as a named helper (preferred by the codebase's function-per-concern style):
```go
Lines: countLines(src),
```
```go
// countLines returns the number of lines in src using editor-style counting:
// an empty file is 0 lines; a file ending in \n counts only the lines above
// the final newline; an unterminated final line still counts as one line.
func countLines(src []byte) int {
    if len(src) == 0 {
        return 0
    }
    n := bytes.Count(src, []byte{'\n'})
    if src[len(src)-1] != '\n' {
        n++
    }
    return n
}
```
(Use `bytes.Count` to avoid the `string(src)` allocation, consistent with the function taking `[]byte`.)

**Behaviour table:**

| Input | Old result | New result | Correct? |
|-------|-----------|-----------|---------|
| `""` (0 bytes) | 1 | 0 | Yes |
| `"foo"` (no newline) | 1 | 1 | Yes (unchanged) |
| `"foo\n"` (trailing newline) | 2 | 1 | Yes |
| `"foo\nbar"` (no trailing newline) | 2 | 2 | Yes (unchanged) |
| `"foo\nbar\n"` (trailing newline) | 3 | 2 | Yes |

### Pattern 3: PIN-01 — Sourcing the Expected Total Lines

**The non-circularity constraint:** The oracle (`Expected()`) must not call `countLines` or any variant of the engine's formula — that would prove nothing. The independent source already exists: `RenderZsh` maintains a running `line` counter and at the end of the loop `line` equals `(total lines emitted) + 1` (it is incremented past the last blank separator line). The expected total line count is therefore `line - 1` after the loop completes.

**Recommended approach:** Add a `Lines int` field to `ConfigGraph` (or expose the final counter via a return value from `RenderZsh`). Simplest: change `RenderZsh` signature to return `([]byte, int)` where the int is the total rendered line count, and store that in `ConfigGraph.Lines` for oracle use.

```go
// RenderZsh emits the graph as .zsh source. Returns the source and the total
// rendered line count (for use by the oracle's Lines assertion).
func (g *ConfigGraph) RenderZsh() ([]byte, int) {
    var b strings.Builder
    line := 1
    for _, n := range g.Nodes {
        if n.Comment != "" {
            fmt.Fprintf(&b, "# %s\n", n.Comment)
            line++
        }
        n.Line = line
        stmt := n.render()
        b.WriteString(stmt)
        b.WriteByte('\n')
        line += strings.Count(stmt, "\n") + 1
        b.WriteByte('\n') // blank separator
        line++
    }
    total := line - 1  // line points one past the final separator
    return []byte(b.String()), total
}
```

Then `Expected()` sets `a.Lines = g.Lines` (stored by the caller after `RenderZsh` returns).

**Alternative (no signature change):** Store the rendered line count on `ConfigGraph` as a side-effect field (`g.RenderedLines`) set by `RenderZsh`. This keeps the current call-site API identical except for reading `g.RenderedLines` in `Expected()`.

**Watch-out:** `property_test.go` currently calls `src := g.RenderZsh()` — if the signature changes to `([]byte, int)`, all callers (including `render_test.go`, `fuzz_test.go`) must be updated. The side-effect field approach avoids this.

**Recommended:** Side-effect field on `ConfigGraph` — least caller disruption:
```go
// in graph.go
type ConfigGraph struct {
    Nodes         []*Node
    RenderedLines int  // set by RenderZsh; read by Expected()
}
```
Then in `RenderZsh`: `g.RenderedLines = line - 1` just before return.
Then in `Expected()`: `a.Lines = g.RenderedLines`.

**Ordering constraint preserved:** `RenderZsh` must still be called before `Expected()` — this was already a documented requirement (ARCHITECTURE.md anti-patterns section).

### Pattern 4: PIN-01 — Implementing the Line Assertions in property_test.go

```go
if checkLines {
    if got.Lines != want.Lines {
        fail("Lines = %d; want %d", got.Lines, want.Lines)
    }
    wantLines := map[string][]int{}
    for _, is := range want.Issues {
        wantLines[key(is)] = is.Lines
    }
    for _, is := range got.Issues {
        k := key(is)
        if wl, ok := wantLines[k]; ok {
            if fmt.Sprint(is.Lines) != fmt.Sprint(wl) {
                fail("issue %s lines = %v; want %v", k, is.Lines, wl)
            }
        }
    }
}
```

### Pattern 5: PIN-01 Coverage Gap — Adding Comments to dup-alias and reassigned-env Nodes

**Current state:** In `generator.go:105`, only the shadow-alias node gets a `Comment`:
```go
g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo alias", Cat: model.CatAliases, Comment: fmt.Sprintf("%s alias", name)})
```
All other planted-defect nodes (`dupAlias`, `dupEnv`, `dupPath`) have `Comment: ""`.

This means the LINE-02 trigger (a statement preceded by a `#` comment) is only exercised for `shadowed` issues. The duplicate-alias and reassigned-env pins would pass vacuously even if the comment-line bug were still present for those issue kinds.

**Minimal fix:** Add a `Comment` to the first node of each `dupAlias` and `dupEnv` pair. `dupPath` is less critical (paths are matched by regex on `Block.Text`, not `Block.StartLine`, but the `StartLine` still flows into the issue's `Lines` — adding a comment there is also correct).

```go
// In generator.go Build(), planted defects section:
for _, name := range dupAlias {
    g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo first", Cat: model.CatAliases,
        Comment: fmt.Sprintf("first %s", name)})  // ADD: triggers LINE-02 for duplicate_alias
    g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo second", Cat: model.CatAliases})
}
for _, name := range dupEnv {
    g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "first", Cat: model.CatEnvironment,
        Comment: fmt.Sprintf("first %s", name)})  // ADD: triggers LINE-02 for reassigned_env
    g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "second", Cat: model.CatEnvironment})
}
```

**Oracle/render impact:** None. `RenderZsh` already handles `n.Comment != ""` by emitting the `# comment\n` line and incrementing `line` before recording `n.Line`. Adding comments to more nodes will shift the `n.Line` values for subsequent nodes (existing oracle issue-lines will change), but the oracle reads `n.Line` which is set by `RenderZsh` — so oracle and engine see the same new line numbers. The property test is seed-deterministic; the exact line numbers will shift but the oracle-vs-engine comparison will remain valid.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| Line counting | Custom byte scanner | `bytes.Count(src, []byte{'\n'})` + last-byte check (stdlib) |
| AST line numbers | Custom offset-to-line conversion | `stmt.Pos().Line()` from `mvdan.cc/sh/v3` — already used |
| Deterministic PRNG in tests | Custom seeding | `rand.New(rand.NewSource(seed))` — already used |

---

## Common Pitfalls

### Pitfall 1: Removing the startLine assignment instead of the comment-loop assignment

**What goes wrong:** The developer removes the `startLine := stmt.Pos().Line()` initialization at line 33 or replaces it with 1, rather than removing only the `startLine = c.Pos().Line()` reassignment inside the loop at line 39.
**Why it happens:** Both lines assign to `startLine`; the wrong one looks like the "override."
**How to avoid:** The fix is a one-line deletion inside the for-loop body — line 39 only. The `startLine := stmt.Pos().Line()` at line 33 is the correct initialization and must be kept.
**Warning signs:** If `Block.StartLine` becomes 0 for all blocks, the initialization was accidentally removed.

### Pitfall 2: Circular total-Lines oracle (re-applying the engine formula)

**What goes wrong:** `Expected()` sets `a.Lines` by calling `strings.Count(src, "\n") + ...` or any variant of the corrected formula on the rendered source. This proves nothing — if the engine has the same formula, the test will always pass even if the formula is wrong.
**Why it happens:** It looks like the obvious "compute expected from the same source."
**How to avoid:** Source the expected total exclusively from `RenderZsh`'s running `line` counter (the `g.RenderedLines` field approach). The render counter is computed by a different code path (string builder + per-statement accounting) than the engine's byte scanner.
**Warning signs:** The test passes even when `checkLines = true` is set but the LINE-01 formula is reverted.

### Pitfall 3: Forgetting to update the parse_test.go StartLine assertion

**What goes wrong:** `parse_test.go:54-55` asserts `blocks[0].StartLine == 1` with the comment "want 1 (leading comment)". After the LINE-02 fix, the statement `alias gs=...` is on line 2 (line 1 is the `# my aliases` comment). The test will fail with `got 2, want 1` after the fix.
**Why it happens:** The test was written to document the current (buggy) behaviour.
**How to avoid:** Update the test to `want 2` (the statement line) and update the comment to `// want 2 (statement line, not leading-comment line)`.
**Warning signs:** `go test ./core/shell/zsh/...` fails with "alias block StartLine = 2, want 1 (leading comment)."

### Pitfall 4: analyze_test.go Lines assertion expects the buggy count

**What goes wrong:** `analyze_test.go:151` asserts `a.Lines != 6` with comment `// 5 newlines + 1`. The source (`src`) has 5 newlines and its last byte is `\n`. Under the corrected formula, `Lines = 5` (not 6). The test will fail with `got 5, want 6` after the LINE-01 fix.
**Why it happens:** The test was written to verify the current (off-by-one) behaviour.
**How to avoid:** Update the assertion from `want 6` to `want 5` and update the comment from `// 5 newlines + 1` to `// 5 lines (last byte is \n, not double-counted)`.
**Warning signs:** `go test ./core/analyze/...` fails immediately after the LINE-01 change.

### Pitfall 5: Empty-file formula branch missing

**What goes wrong:** The fix uses `Count("\n") + (lastByte != '\n' ? 1 : 0)` without a guard for `len(src) == 0`. Accessing `src[len(src)-1]` on a zero-length slice panics.
**Why it happens:** The guard is easy to forget when thinking only about the off-by-one case.
**How to avoid:** The `if len(src) == 0 { return 0 }` early-return must be the first thing in `countLines`.
**Warning signs:** `TestCorpusGolden/empty.zsh` panics; `go test -run TestCorpusGolden ./core/analyze/...` shows "index out of range."

### Pitfall 6: manifests.json lacking a "lines" field for empty.zsh

**What goes wrong:** `corpus_test.go` currently does not read a `lines` field from manifests — the `manifest` struct has no such field. D-05 says `empty.zsh` must "reflect a 0-line count," but the corpus test does not currently assert `Lines` at all. If the planner only adds `"lines": 0` to manifests.json without adding the corresponding struct field and assertion in `corpus_test.go`, the JSON entry is ignored silently.
**Why it happens:** Misreading D-05 as "just add to JSON."
**How to avoid:** Two coordinated changes: (1) add `Lines int` and `AssertLines bool` (or a pointer `*int`) to the `manifest` struct in `corpus_test.go`, and (2) add the assertion `if m.Lines != 0 { if a.Lines != m.Lines { ... } }` (or equivalent). Then set `"lines": 0` in the JSON for `empty.zsh`.

Alternatively — and more minimally aligned with D-05 — add a `Lines *int` field (pointer so the field is optional, not asserted for fixtures that don't set it) and a targeted assertion only when non-nil. This keeps all other fixture entries unmodified.

---

## StartLine Reader Audit (LINE-02 Safety Verification)

Full grep of `StartLine` across the entire codebase (`core/**/*.go`):

| File | Line(s) | Role | Change needed? |
|------|---------|------|----------------|
| `core/model/block.go:29` | Declaration | `Block.StartLine int // 1-based line in source` | No — field kept |
| `core/shell/zsh/parse.go:24` | Write | Opaque fallback sets `StartLine: 1` | No — whole-file starts at line 1, correct |
| `core/shell/zsh/parse.go:49` | Write | Normal block sets `StartLine: int(startLine)` | Indirect — `startLine` will now always hold the statement line after fix |
| `core/shell/zsh/parse.go:35` | Comment | "block's Text and StartLine reflect the documentation" | Update comment to say StartLine = statement line |
| `core/analyze/reconciler.go:46` | Read | `duplicateNames`: `lines[n] = append(lines[n], b.StartLine)` | No — free payoff |
| `core/analyze/reconciler.go:69` | Read | `duplicatePaths`: `lines[seg] = append(lines[seg], b.StartLine)` | No — free payoff |
| `core/analyze/reconciler.go:103` | Read | `shadows`: `aliasLines[n] = b.StartLine` | No — free payoff |
| `core/analyze/reconciler.go:109` | Read | `shadows`: `funcLines[n] = b.StartLine` | No — free payoff |
| `core/shell/zsh/parse_test.go:54-55` | Assert | `blocks[0].StartLine != 1` with comment "want 1 (leading comment)" | YES — must update to expect statement line (2) |
| `core/analyze/analyze_test.go:28-29,50,77,102-103,133-136,179-182,215` | Mock data | Hand-crafted `StartLine` values in mockProvider blocks | No — mockProvider bypasses the parser; these are directly set integers |

**Verdict:** The only external reader is the four `reconciler.go` read sites (free payoff) and one test assertion in `parse_test.go` that explicitly checks the buggy behaviour. No other production code reads `StartLine`. Redefining `StartLine` = statement line is safe.

---

## PIN-02 Corpus Impact

**Current manifest struct fields:** `MinBlocks`, `IssueKinds`, `HasSecrets`, `ExpectCategories` — no `Lines` field.

**Current corpus assertions:** `a.BlockCount`, `a.HasSecrets`, `a.OpaqueBlocks`, issue kind presence/absence, category presence. The corpus test does NOT currently assert `a.Lines`.

**Impact of LINE-01 fix on existing fixtures:**

| Fixture | Last byte | Old Lines | New Lines | Manifest asserts Lines? | Action |
|---------|-----------|-----------|-----------|------------------------|--------|
| `empty.zsh` | (0 bytes) | 1 | 0 | No | Add `"lines": 0` + struct + assertion |
| `duplicate_aliases.zsh` | `\n` | 4 | 3 | No | None needed |
| `reassigned_env.zsh` | `\n` | 3 | 2 | No | None needed |
| `clean_baseline.zsh` | likely `\n` | N+1 | N | No | None needed |
| `installer_junk.zsh` | likely `\n` | N+1 | N | No | None needed |
| `secrets_inline.zsh` | likely `\n` | N+1 | N | No | None needed |

**Verdict:** No existing corpus assertion breaks from the LINE-01 fix (nothing asserts `Lines` today). The only change required for PIN-02 is adding `Lines` support for `empty.zsh`. All other fixtures pass without changes.

**LINE-02 fix on corpus:** The corpus test does not assert `Issue.Lines` values (only `IssueKind` strings). No existing corpus assertion breaks from the LINE-02 fix.

---

## Files That Assert Buggy Values (Must Update Alongside Fix)

| File | Line(s) | Buggy Assertion | After Fix |
|------|---------|-----------------|-----------|
| `core/shell/zsh/parse_test.go:54-55` | `blocks[0].StartLine != 1` (comment: "want 1 (leading comment)") | LINE-02: expects comment line | Change to `!= 2` + update comment to "want 2 (statement line)" |
| `core/analyze/analyze_test.go:151-152` | `a.Lines != 6` (comment: "5 newlines + 1") | LINE-01: expects old formula | Change to `!= 5` + update comment to "5 lines (trailing \\n not double-counted)" |

These are the only two tests that assert the current buggy behaviour. Both use `mockProvider` / direct byte input so no other test is affected by the formula change.

---

## Execution Order

The four changes form a dependency chain:

1. **parse.go** (LINE-02) — foundational; reconciler readers are automatically correct after this.
2. **analyzer.go** (LINE-01) — independent of LINE-02; can be done in either order.
3. **parse_test.go + analyze_test.go** (update buggy assertions) — must follow their respective fixes or tests fail.
4. **generator.go** (add comments to dup-alias/reassigned-env nodes) — must precede or accompany property_test.go change.
5. **render.go / graph.go** (expose `RenderedLines`) — must precede oracle.go change.
6. **oracle.go** (set `a.Lines = g.RenderedLines`) — depends on RenderedLines being available.
7. **property_test.go** (flip `checkLines = true` + implement assertion) — depends on oracle providing Lines and on both engine fixes being in place.
8. **manifests.json + corpus_test.go** (PIN-02) — can be done in any order; depends on LINE-01 fix.

---

## Open Questions

1. **`RenderZsh` signature change vs side-effect field**
   - What we know: Current callers are `property_test.go:35`, `render_test.go`, `fuzz_test.go`, `main_test.go` (zsh-gen). Each calls `g.RenderZsh()` and assigns to a `[]byte`.
   - What's unclear: Are there other callers (e.g., in cmd/zsh-gen/main.go) that need the total line count? The zsh-gen main likely does not use it.
   - Recommendation: Use the side-effect field (`g.RenderedLines`) — zero signature-change disruption, all existing call-sites compile unchanged.

2. **Exact `duplicate_aliases.zsh` line count for reference**
   - What we know: File content is `alias gs='git status'\nalias ll='ls -l'\nalias gs='git switch'` — 3 lines + trailing newline = 3 lines (new formula), 4 lines (old formula). Not asserted by corpus today.
   - What's unclear: Whether the planner wants to add a `lines: 3` assertion there for belt-and-suspenders. D-05 says no.
   - Recommendation: Leave it out per D-05. The property test is the line pin.

---

## Environment Availability

Step 2.6: SKIPPED (no external tools required — all changes are in-file Go edits and test data updates).

---

## Validation Architecture

nyquist_validation is explicitly set to false in .planning/config.json — section omitted per instructions.

---

## Security Domain

security_enforcement is not set in config.json (absent = enabled by default), but this phase makes no changes affecting authentication, session management, access control, input validation, or cryptography. It modifies a pure arithmetic formula and a parser local-variable assignment. No ASVS categories apply.

---

## Sources

### Primary (HIGH confidence)
- `core/shell/zsh/parse.go` — verified directly: `startLine := stmt.Pos().Line()` at line 33; `startLine = c.Pos().Line()` at line 39; `StartLine: int(startLine)` at line 49
- `core/analyze/analyzer.go` — verified directly: `Lines: strings.Count(string(src), "\n") + 1` at line 28
- `core/analyze/reconciler.go` — verified directly: four `b.StartLine` reads at lines 46, 69, 103, 109
- `core/shell/zsh/parse_test.go` — verified directly: buggy assertion at lines 54-55
- `core/analyze/analyze_test.go` — verified directly: buggy `Lines` assertion at line 151-152
- `core/testgen/render.go` — verified directly: `line` counter tracking; `Node.Line = line` set after comment, before statement
- `core/testgen/oracle.go` — verified directly: `Expected()` sets category/issue fields but NOT `a.Lines`
- `core/testgen/generator.go` — verified directly: only shadow-alias node (`line 105`) has `Comment` set
- `core/testgen/property_test.go` — verified directly: `checkLines = false` at line 18; empty gated block at lines 89-92
- `core/testdata/fixtures/manifests.json` — verified directly: no `lines` field in any entry; `empty.zsh` entry has no line assertion
- `core/analyze/corpus_test.go` — verified directly: `manifest` struct has no `Lines` field; test does not assert `a.Lines`
- `core/testdata/fixtures/empty.zsh` — verified: file is 0 bytes
- `CLAUDE.md` — project constraints: no new dependencies, TDD, test the engine not mocks for property tests

### Secondary (MEDIUM confidence)
- `mvdan.cc/sh/v3` `stmt.Pos().Line()` returns the 1-based statement line (not comment line) — inferred from the parser option `KeepComments(true)` separating comment AST nodes from the statement position, and the existing code pattern. [ASSUMED: not directly verified against mvdan.cc/sh/v3 documentation in this session, but consistent with the existing code's initialization at parse.go:33 which captures it before the comment loop]

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `stmt.Pos().Line()` is the 1-based statement line, not the comment line | Sources | If wrong, `startLine := stmt.Pos().Line()` would already be the comment line and the fix would not help. Risk: LOW — the existing code already uses this value as the "correct" starting point before the comment-loop overwrites it, and the code comment at parse.go:35 says "Pull in leading comments ... so the block's Text and StartLine reflect the documentation above it" (implying the initial value was the statement line). |
| A2 | No caller outside the audited set reads `Block.StartLine` | StartLine Reader Audit | If a hidden reader exists (e.g. in a future file or a test helper not grepped), it would get the new statement-line value silently. Risk: VERY LOW — full grep of `core/**/*.go` showed only the documented readers. |

**All critical claims were verified by direct file inspection. Only two assumptions remain, both low-risk.**

---

## Metadata

**Confidence breakdown:**
- LINE-01 fix: HIGH — formula is trivially verified, edge cases documented
- LINE-02 fix: HIGH — full reader audit performed, one test assertion confirmed buggy
- PIN-01 oracle sourcing: HIGH — RenderZsh counter approach is non-circular and directly grounded in the source
- PIN-01 coverage gap: HIGH — generator.go directly read; only shadow nodes have Comment confirmed
- PIN-02 corpus impact: HIGH — manifests.json and corpus_test.go directly read; no Lines assertion exists

**Research date:** 2026-06-24
**Valid until:** Stable (no external dependencies change; all findings are from direct code inspection)
