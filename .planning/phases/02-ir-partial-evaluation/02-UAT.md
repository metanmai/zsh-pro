---
status: complete
phase: 02-ir-partial-evaluation
source: [02-01-SUMMARY.md, 02-02-SUMMARY.md]
started: 2026-06-26T21:20:24Z
updated: 2026-06-26T21:25:15Z
---

## Current Test

[testing complete]

## Tests

### 1. Cold-Start Build & Test Smoke
expected: From scratch (no cache), `go build ./...` and `go test -count=1 ./...` succeed; the round-trip oracle TestRegenRoundTrip runs (zsh present) and passes.
result: pass

### 2. Real-Config Round-Trip
expected: Building the IR from your actual ~/.zshrc and regenerating it produces a behavior-equivalent config under zsh -f — no panic, no dropped/corrupted entries (the real-world version of the curated oracle).
result: issue
reported: "Aliases + Functions differ: regenerating my real ~/.zshrc emits `plugins=(git zsh-autosuggestions zsh-syntax-highlighting)` as bare `plugins=`, silently dropping the array value. oh-my-zsh then loads zero plugins, so all git aliases/functions and autosuggestions/syntax-highlighting vanish. Env, Options, PATH round-tripped identically."
severity: major
root_cause: "parse.go:80-82 only captures scalar `a.Value`; for an array assignment (`name=(...)`) mvdan/sh populates `a.Array` (a.Value is nil), so b.Value stays empty. The single-name plain-`=` entry routes managed and the scalar templater emits `name=` with no value — silent corruption of a near-universal config shape. Same class as BL-02 but for ARRAY-valued assignments, which no oracle/review fixture covered."

## Summary

total: 2
passed: 1
issues: 1
pending: 0
skipped: 0

## Gaps

- truth: "Regenerating a real ~/.zshrc produces a behavior-equivalent config (no dropped/corrupted entries)."
  status: failed
  reason: "User reported: array assignment `plugins=(...)` regenerates as bare `plugins=`, emptying the array and breaking oh-my-zsh plugin load (git aliases/functions, autosuggestions, syntax-highlighting all lost). Env/Options/PATH round-trip fine."
  severity: major
  test: 2
  root_cause: "parse.go:80-82 captures only scalar a.Value; array assignments (a.Array != nil, a.Value == nil) leave b.Value empty. routeManaged admits the single-name plain-= entry as managed; the scalar templater emits `name=`. Array-valued assignments are not faithfully templatable and must route imperative-verbatim (same remedy as multi-name/+=/flagged shapes from BL-02/WR-01/WR-02)."
  artifacts: [core/shell/zsh/parse.go, core/ir/route.go, core/shell/zsh/regen.go, core/ir/roundtrip_test.go]
  missing: ["array-assignment detection at parse time (e.g. model.Block.Array flag set when a.Array != nil)", "routeManaged routes array assignments imperative", "oracle fixture covers name=(...) array assignment + a multi-line array", "regression test for plugins=(...) round-trip"]
