---
phase: 01-trustworthy-line-numbers
verified: 2026-06-24T09:39:59Z
status: passed
score: 4/4 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
notes:
  - "ROADMAP declares `Mode: mvp` for this phase, but the phase goal is a technical correctness statement, not a User Story (user-story.validate -> valid:false). Standard goal-backward verification was applied against the 4 explicit Success Criteria, which is the correct instrument for a bug-fix phase. The mode/goal-format mismatch is a metadata observation for human review, not a goal-achievement gap. If MVP-mode semantics are desired here, re-run /gsd mvp-phase 01 to set a User Story goal; otherwise consider clearing the mvp mode on this correctness phase."
---

# Phase 1: Trustworthy Line Numbers Verification Report

**Phase Goal:** `analyze --json` reports a correct `Analysis.Lines` count and attributes every issue to its real statement line, with both fixes pinned so they cannot silently regress.
**Verified:** 2026-06-24T09:39:59Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal decomposes into two correctness fixes (LINE-01 total line count, LINE-02 per-issue statement-line attribution) and two regression pins (PIN-01 testgen oracle, PIN-02 golden corpus). All four are achieved in the actual codebase, build/vet/full-suite are green, and both fixes were proven non-vacuously pinned by reintroducing each bug and confirming the harness catches it.

The deliberate `analyze --json` wire-contract change (altered `Lines` and issue `lines` values) is accepted per PROJECT.md "Constraints > Compatibility" and was NOT treated as a regression — it is the intended outcome.

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | `analyze --json` on an empty file reports `Lines: 0`, and a trailing-newline file is not counted one line too high (LINE-01) | ✓ VERIFIED | `core/analyze/analyzer.go:104-113` `countLines` has `if len(src) == 0 { return 0 }` as its first statement, then `bytes.Count(src, []byte{'\n'})` + 1 only when last byte != `\n`. Wired at `:28` `Lines: countLines(src)`. Old `strings.Count(string(src), "\n") + 1` gone (grep across `core/` returns nothing; `bytes` imported, `strings` not). `TestCountLines` (analyze_test.go:244-264) pins empty=0, `foo\n`=1, `foo\nbar`=2, `\n\n\n\n\n`=5 — all PASS. End-to-end probe: `# x\nexport EDITOR=vim\nexport EDITOR=nvim\n` -> `Lines=3` (correct). |
| 2 | A statement with a leading `#` comment reports its own line in every issue (duplicate, reassigned, shadow), not the comment line (LINE-02) | ✓ VERIFIED | `core/shell/zsh/parse.go:33` keeps `startLine := stmt.Pos().Line()`; the comment-pull-up loop (`:38-42`) updates only `start = c.Pos().Offset()` — the `startLine = c.Pos().Line()` overwrite is gone (grep across `core/` confirms). `:50` reads `StartLine: int(startLine)`. `reconciler.go` unchanged (readers at :46,69,103,108 inherit the fix). `TestParseClassifiesKinds` asserts `blocks[0].StartLine == 2` (statement line, comment on line 1) — PASS. End-to-end probe proves it for ALL kinds: reassigned_env `lines=[2 3]` (comment on 1), shadowed `lines=[2 4]` (comment on 1) — statement lines, never the comment line. |
| 3 | The testgen oracle property test runs with `checkLines = true` and passes, asserting total `Lines` AND each issue's `lines` slice across all 10 seeds; total sourced non-circularly from RenderZsh (PIN-01) | ✓ VERIFIED | `property_test.go:19` `const checkLines = true` (stale "Flip to true" comment removed). Gated block (`:90-107`) asserts `got.Lines != want.Lines` then per-issue `Lines` slices via `fmt.Sprint`. `TestOracleProperty` PASS for all 10 seeds (1,2,3,5,8,13,21,34,55,89) with gate ON (verbose run confirmed). Non-circular: oracle.go imports only `sort`+`model`, reads `a.Lines = g.RenderedLines` (`:22`); `RenderedLines` set in `render.go:31` from RenderZsh's own counter `line - 1`; NO `countLines`/`strings.Count` call in oracle.go. `renderedlines_test.go` independently pins `RenderedLines == editorLines(rendered)` computed locally. LINE-02 exercised beyond shadows: generator.go gives dupAlias (`:97`) and dupEnv (`:101`) first nodes a leading `Comment`. |
| 4 | The golden corpus (`manifests.json` + `corpus_test.go`) passes against corrected output, `empty.zsh` reflects a 0-line count, WITHOUT adding issue_lines/issue_names assertions (PIN-02, D-05) | ✓ VERIFIED | `corpus_test.go:29` adds `Lines *int` (`json:"lines"`); conditional assertion `:58` `if m.Lines != nil && a.Lines != *m.Lines`. `manifests.json:37` `empty.zsh` has `"lines": 0`; it is the only entry with a `lines` key (grep confirms); `empty.zsh` is genuinely 0 bytes (`wc -c` = 0). `TestCorpusGolden/empty.zsh` PASS (verbose confirmed). Corpus stays minimal — runner asserts only BlockCount, HasSecrets, Lines (opt-in), OpaqueBlocks, issue Kinds, Categories; NO issue_lines/issue_names assertions (D-05 honored). |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ----------- | ------ | ------- |
| `core/analyze/analyzer.go` | Editor-style `countLines` wired into `Analysis.Lines` | ✓ VERIFIED | `func countLines` (:104), empty guard first, `Lines: countLines(src)` (:28); `bytes` imported, `strings` removed; build+vet clean |
| `core/analyze/analyze_test.go` | Corrected Lines assertion (`want 5`) + `TestCountLines` | ✓ VERIFIED | `a.Lines != 5` / `want 5` (:151-152); table-driven `TestCountLines` (:244) with 6 edge cases |
| `core/shell/zsh/parse.go` | Comment-pull-up updates only start offset, never startLine | ✓ VERIFIED | `start = c.Pos().Offset()` kept (:40); `startLine = c.Pos().Line()` deleted; `startLine := stmt.Pos().Line()` (:33) intact; doc comment updated |
| `core/shell/zsh/parse_test.go` | StartLine assertion locks statement line (`want 2`) | ✓ VERIFIED | `blocks[0].StartLine != 2` / `want 2 (statement line, not leading-comment line)` (:55-56) |
| `core/analyze/reconciler.go` | Unchanged; inherits the fix via Block.StartLine | ✓ VERIFIED | Four StartLine readers at :46,69,103,108; logic unchanged; not in plan 01-02 diff (D-02 honored) |
| `core/testgen/graph.go` | `RenderedLines int` field on ConfigGraph | ✓ VERIFIED | `:41` `RenderedLines int` with doc comment |
| `core/testgen/render.go` | RenderZsh sets `g.RenderedLines` (signature unchanged) | ✓ VERIFIED | `:31` `g.RenderedLines = line - 1` before return; signature still `RenderZsh() []byte` (:12) — all 5 callers compile (build OK) |
| `core/testgen/oracle.go` | Expected sets `a.Lines = g.RenderedLines` (non-circular) | ✓ VERIFIED | `:22` `a.Lines = g.RenderedLines`; imports only `sort`+`model`; no countLines/strings.Count |
| `core/testgen/generator.go` | Leading Comment on first node of each dupAlias/dupEnv pair | ✓ VERIFIED | dupAlias first node `:97` `Comment: fmt.Sprintf("first %s", name)`; dupEnv first node `:101` same; second nodes comment-free |
| `core/testgen/property_test.go` | `checkLines = true` + implemented line-assertion block | ✓ VERIFIED | `:19` const true; non-empty gated block (:90-107) asserting total + per-issue lines |
| `core/analyze/corpus_test.go` | Optional `Lines *int` manifest field + conditional assertion | ✓ VERIFIED | `:29` `Lines *int` json:"lines"; `:58` conditional assertion |
| `core/testdata/fixtures/manifests.json` | `empty.zsh` declares `lines: 0` | ✓ VERIFIED | `:37` `"lines": 0`; valid JSON; only entry with a lines key |
| `core/testgen/renderedlines_test.go` | New test pinning RenderedLines + dup-comment coverage | ✓ VERIFIED (bonus pin) | `TestRenderedLinesMatchesSource` + `TestDupAliasAndDupEnvCarryLeadingComment`; non-circular local `editorLines` |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| `analyzer.go Analyze()` | `analyzer.go countLines()` | `Lines: countLines(src)` literal | ✓ WIRED | Present at :28; helper at :104 |
| `parse.go` | `reconciler.go` | `Block.StartLine` consumed at reconciler :46,69,103,108 | ✓ WIRED | Producer fixed; all four readers inherit corrected statement line; reconciler untouched |
| `render.go RenderZsh()` | `oracle.go Expected()` | `g.RenderedLines` side-effect field (RenderZsh before Expected) | ✓ WIRED | render.go:31 sets it; oracle.go:22 reads it; property_test.go:36-37 calls RenderZsh then Expected in order |
| `property_test.go` | `core/analyze` + oracle | `checkLines` gate compares got.Lines / issue lines vs want | ✓ WIRED | Gate true; block compares both; TestOracleProperty PASS x10 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `oracle.go Expected().Lines` | `a.Lines` | `g.RenderedLines` set by RenderZsh's running counter | Yes — render counter, non-circular | ✓ FLOWING |
| `corpus_test.go Lines assertion` | `a.Lines` (engine) vs `*m.Lines` (manifest) | real `Analyze()` over fixture bytes; manifest JSON | Yes — engine output compared to declared 0 | ✓ FLOWING |
| issue `lines` slices (oracle) | `n.Line` per node | set by RenderZsh per statement | Yes — real rendered line numbers | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Empty/trailing-newline line count (LINE-01) | `go test ./core/analyze/ -run TestCountLines` | PASS (6/6 cases incl. empty=0, `foo\n`=1, `\n\n\n\n\n`=5) | ✓ PASS |
| Statement-line attribution, alias (LINE-02) | `go test ./core/shell/zsh/ -run TestParseClassifiesKinds` | PASS (StartLine==2, comment on 1) | ✓ PASS |
| LINE-02 across reassigned_env + shadowed | end-to-end probe through real engine | reassigned_env `lines=[2 3]`, shadowed `lines=[2 4]` — statement lines, not comment | ✓ PASS |
| Oracle pin, 10 seeds, gate ON (PIN-01) | `go test ./core/testgen/ -run TestOracleProperty -v` | PASS all 10 seeds | ✓ PASS |
| Corpus empty.zsh lines==0 (PIN-02) | `go test ./core/analyze/ -run TestCorpusGolden -v` | PASS incl. empty.zsh | ✓ PASS |
| Build / vet / full suite | `go build ./...`; `go vet ./...`; `go test ./... -count=1` | all exit 0, every package `ok` | ✓ PASS |
| Non-vacuity A: reintroduce LINE-01 `+1` | mutate countLines, run oracle + analyze, revert | TestOracleProperty FAIL, TestCountLines FAIL (`only_newlines = 6, want 5`); restored clean | ✓ PASS (pin is real) |
| Non-vacuity B: reintroduce LINE-02 overwrite | mutate parse.go, run parse + oracle, revert | TestParseClassifiesKinds FAIL, TestOracleProperty FAIL; restored clean | ✓ PASS (pin is real) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| LINE-01 | 01-01 (frontmatter `requirements: [LINE-01]`) | `Analysis.Lines` true count — empty=0, trailing-newline not over-counted | ✓ SATISFIED | countLines fix + TestCountLines; REQUIREMENTS.md:12 marked [x], Traceability row Phase 1 Complete |
| LINE-02 | 01-02 (frontmatter `requirements: [LINE-02]`) | Leading-comment statement reports its own line in every issue | ✓ SATISFIED | parse.go overwrite removed + probe across 3 kinds; REQUIREMENTS.md:13 [x], Traceability Phase 1 Complete |
| PIN-01 | 01-03 (frontmatter `requirements: [PIN-01]`) | Oracle property test asserts lines (checkLines=true) across all seeds, passes | ✓ SATISFIED | checkLines=true + gated block + 10-seed PASS, non-circular total; REQUIREMENTS.md:17 [x], Traceability Complete |
| PIN-02 | 01-03 (frontmatter `requirements: [PIN-02]`) | Golden corpus passes vs corrected output; empty.zsh reflects 0-line count | ✓ SATISFIED | optional Lines field + empty.zsh lines:0 + TestCorpusGolden PASS; REQUIREMENTS.md:18 [x], Traceability Complete |

All 4 requirement IDs declared in plan frontmatter (LINE-01, LINE-02, PIN-01, PIN-02) are present in REQUIREMENTS.md, marked complete, and mapped to Phase 1 in the Traceability table (0 unmapped, 0 orphaned). No requirement mapped to Phase 1 in REQUIREMENTS.md is missing from a plan.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TODO/FIXME/XXX/HACK/PLACEHOLDER markers in any of the 10 modified files | — | No debt-marker gate triggered |

No stubs, hollow props, empty returns flowing to output, or unreferenced debt markers detected in modified files. The opaque-fallback `StartLine: 1` (parse.go:24) is correct and out of scope (whole-file fallback per PROJECT.md Out of Scope), not an anti-pattern.

### Human Verification Required

None. Every Success Criterion is verifiable programmatically and was confirmed via build, vet, full test suite, two targeted verbose test runs, an end-to-end engine probe covering all three comment-bearing issue kinds, and two mutation (non-vacuity) proofs. No visual, real-time, or external-service behavior is involved (read-only local CLI, pure arithmetic + AST line numbers).

### Gaps Summary

No gaps. All four success criteria are achieved in the actual codebase with non-vacuous regression pins:

- **LINE-01** — editor-style `countLines` replaces the off-by-one formula; empty=0, trailing newline not over-counted; pinned by `TestCountLines` and the oracle total.
- **LINE-02** — the `startLine` overwrite is deleted; `Block.StartLine` is the statement line; verified end-to-end for duplicate_alias, reassigned_env, and shadowed; reconciler untouched (D-01/D-02).
- **PIN-01** — `checkLines = true` with an implemented total + per-issue line assertion passing across all 10 seeds; the expected total is sourced non-circularly from `RenderZsh`'s counter (`ConfigGraph.RenderedLines`), not the engine formula; LINE-02 exercised beyond shadows via planted dup-pair comments.
- **PIN-02** — the golden corpus passes against the corrected output with `empty.zsh` pinned at 0 lines via an opt-in `Lines *int` field; no issue_lines/issue_names assertions added (D-05).

The two mutation proofs (reintroducing each bug) confirm the pins fail on regression, so neither fix can silently regress — directly satisfying the goal's "pinned so they cannot silently regress" clause.

**One non-blocking metadata note (see frontmatter `notes`):** ROADMAP declares `Mode: mvp` for this phase, but the phase goal is a technical correctness statement rather than a User Story (`user-story.validate` -> `valid:false`). The 4 explicit ROADMAP Success Criteria are crisp and fully codebase-verifiable, so standard goal-backward verification was the correct instrument and the goal is achieved. This is a metadata/labeling mismatch for human awareness, not a goal-achievement gap; it does not block proceeding.

---

_Verified: 2026-06-24T09:39:59Z_
_Verifier: Claude (gsd-verifier)_
