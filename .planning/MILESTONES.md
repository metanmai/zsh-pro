# Milestones

## v1.0 Trustworthy Line Numbers (Shipped: 2026-06-24)

**Phases completed:** 1 phases, 3 plans, 7 tasks

**Key accomplishments:**

- Replaced `Analysis.Lines = strings.Count(string(src), "\n") + 1` with an editor-style `countLines` helper so an empty file reports 0 and a trailing-newline file is no longer over-counted by one.
- Deleting one `startLine = c.Pos().Line()` overwrite in the zsh comment-pull-up loop makes `Block.StartLine` report the statement's own line instead of the leading-comment line — every issue (duplicate alias/env, duplicate path, shadowed) inherits the fix unchanged via reconciler.go, with no new model field.
- Locks the LINE-01 (total-Lines) and LINE-02 (per-issue statement-line) fixes behind the 10-seed oracle property test plus the golden corpus — flipping `checkLines` to true with a real assertion block, sourcing the expected total non-circularly from a new `ConfigGraph.RenderedLines` render counter, and giving planted dupAlias/dupEnv pairs a leading comment so LINE-02 is exercised beyond shadows.

---
