# Retrospective: zsh-pro

A living record of what we learned shipping each milestone.

## Milestone: v1.0 — Trustworthy Line Numbers

**Shipped:** 2026-06-24
**Phases:** 1 | **Plans:** 3 | **Tasks:** 7

### What Was Built
A focused correctness pass on the shipped `analyze` engine so `analyze --json` reports line numbers you can trust: total `Lines` is editor-accurate (empty → 0, trailing newline not over-counted) and every issue points at its real statement line rather than a leading comment. Both fixes are locked behind the 10-seed testgen oracle (`checkLines = true`) and the golden corpus.

### What Worked
- **The regression pin pre-existed.** `property_test.go` already held `checkLines = false` with "flip to true once fixed" — so the milestone was scoped precisely around flipping one switch and making it pass. Clear definition of done from day one.
- **Mutation-proven pins.** Both the phase verifier and the milestone integration check reintroduced each bug and confirmed the suite goes red, then green. That turned "tests pass" into "tests provably catch the regression."
- **Pre-accepted wire-contract change.** The corrected `lines`/`Lines` values were agreed as a deliberate contract change up front, so no churn debating whether changed output was a regression.
- **Non-circular oracle.** Sourcing the expected total from an independent `ConfigGraph.RenderedLines` counter (not the engine's `countLines`) kept the test honest — verified empirically that the two implementations are independent.

### What Was Inefficient
- **Mid-session environment loss.** macOS revoked the process's read access to `~/Documents` partway through (TCC), blocking git and file reads and stranding the phase close-out until access was restored. Environmental, not process — but it cost a detour.
- **Tooling false-positive.** `audit-open` flagged the completed golangci-lint quick task as "missing" because it scans for a SUMMARY filename different from the `${quick_id}-SUMMARY.md` that `/gsd:quick` writes. Harmless but required manual verification before milestone close.

### Patterns Established
- **golangci-lint gate** (v2, standard set) wired via `Makefile` + a tracked `.githooks/pre-commit` that lints only when Go files are staged. Installed as a standalone dev tool — kept *out* of go.mod to preserve the single-dependency architecture.
- **Fix lint findings in code, not config.** Unchecked terminal writes resolved with the `_, _ =` idiom rather than excluding `fmt.Fprint*` from errcheck globally — so the linter still catches ignored errors on real writers.
- **Stdout is sacred.** Added a hardened `--json` agent-contract test (exactly one envelope on stdout, clean stderr) as the guard for the machine-readable contract.

### Key Lessons
- Single-phase milestones still benefit from the full verify → audit gate; the mutation proofs caught nothing broken but raised confidence cheaply.
- Auditing "loose functions for utils" is a good habit, but locality and guardrails win: `countLines` stays package-private precisely because promoting it would invite a circular oracle.
- "Good practices from the jump" = protect the contract + match the mechanism to the program, not maximal tooling. Deferred a trace logger (would be stdlib `slog` to stderr, when a real need appears) over adding it speculatively.

### Cost Observations
- Model mix: subagents (executor, verifier, code-reviewer, integration-checker) all run on Opus per standing preference; orchestration on Opus.
- Sessions: 1 (resume → execute → tooling → audit → close).
- Notable: parallelizing the verifier (background) with code review kept wall-clock down; the TCC outage was the only real stall.

## Cross-Milestone Trends

_First milestone — this section becomes meaningful from v1.1 onward._

| Metric | v1.0 |
|--------|------|
| Phases | 1 |
| Plans | 3 |
| Prod LOC (core) | ~1,532 |
| Test LOC (core) | ~1,215 |
| Verification | passed 4/4 |
