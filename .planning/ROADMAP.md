# Roadmap: zsh-pro

## Overview

A focused correctness pass on the shipped `analyze` engine: make every reported line number trustworthy. The two documented line-number bugs (off-by-one line count, comment-offset issue mis-attribution) are corrected, then locked in by flipping the `testgen` oracle's `checkLines` pin to `true` and confirming the golden corpus stays honest against the corrected output. One coherent, verifiable capability — delivered in a single phase.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Trustworthy Line Numbers** - Fix both line-number bugs and pin them with the oracle + golden corpus

## Phase Details

### Phase 1: Trustworthy Line Numbers

**Goal**: `analyze --json` reports a correct `Analysis.Lines` count and attributes every issue to its real statement line, with both fixes pinned so they cannot silently regress.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: LINE-01, LINE-02, PIN-01, PIN-02
**Success Criteria** (what must be TRUE):

  1. Running `analyze --json` on an empty file reports `Lines: 0`, and a file ending in a trailing newline is not counted one line too high.
  2. When a config statement has a leading `#` comment, every issue it generates (duplicate, reassigned, shadow) reports the statement's own line number, not the comment's line.
  3. The `testgen` oracle property test runs with `checkLines = true` and passes — asserting total `Lines` and each issue's `lines` slice across all 10 seeds.
  4. The existing golden corpus (`manifests.json` + `corpus_test.go`) passes against the corrected output, with the `empty.zsh` case reflecting a `0`-line count.

**Plans**: 3 plans (01-01 and 01-02 touch independent files and run in parallel in Wave 1; 01-03 pins both fixes in Wave 2 once they land)
**UI hint**: no

Plans:
**Wave 1**

- [x] 01-01-PLAN.md — Fix off-by-one line count: `analyzer.go` editor-style `countLines` (empty → 0, trailing newline not over-counted) + update the `want 6` → `want 5` assertion (LINE-01)
- [x] 01-02-PLAN.md — Fix issue line mis-attribution: stop overwriting `startLine` in the `parse.go` comment-pull-up loop so `Block.StartLine` is the statement line (reconciler unchanged) + update the `want 1` → `want 2` assertion (LINE-02)

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 01-03-PLAN.md — Pin both fixes: add `ConfigGraph.RenderedLines` for a non-circular total, comment the dup-alias/reassigned-env pairs, flip `checkLines` to true in `property_test.go`, and make the golden corpus assert `empty.zsh` at 0 lines (PIN-01, PIN-02)

## Progress

**Execution Order:**
Phases execute in numeric order: 1

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Trustworthy Line Numbers | 2/3 | In Progress|  |
