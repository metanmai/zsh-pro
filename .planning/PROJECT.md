# zsh-pro

## What This Is

zsh-pro is a read-only zsh-config analyzer CLI. It parses a zsh config file (default `~/.zshrc`) with a real AST parser, classifies each entry (environment, aliases, functions, path, secrets, …), detects config issues (duplicate aliases, reassigned env vars, duplicate PATH entries, shadowed names), flags likely secrets, and emits either a human-readable report or a `--json` envelope for agents. This milestone makes the line numbers it reports **correct** — fixing the two documented line-number bugs in the engine.

## Core Value

`analyze --json` reports line numbers you can trust — every issue points at the real statement line, and the reported line count is accurate.

## Requirements

### Validated

<!-- Inferred from the existing codebase (see .planning/codebase/). Shipped and relied upon. -->

- ✓ Read-only analysis of a single zsh config file via `analyze` (default `~/.zshrc`) — existing
- ✓ AST-based pipeline: parse → classify → introspect → reconcile → render (`mvdan.cc/sh` LangZsh) — existing
- ✓ Static issue detection across 4 kinds: `duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed` — existing
- ✓ Category classification with confidence levels (environment, aliases, functions, path, secrets, options, plugins, keybindings, misc) — existing
- ✓ Secret detection (regex-based) surfacing `has_secrets` — existing
- ✓ Two output modes: human-readable report and `--json` agent contract (`dto.Envelope`) — existing
- ✓ Typed exit-code contract: 0 clean / 1 runtime error / 2 usage error / 3 actionable issues — existing
- ✓ Best-effort `zsh -f` liveness introspection that degrades gracefully when zsh is absent — existing
- ✓ Shell-agnostic `shell.Provider` seam (ISP interfaces; single composition root) — existing
- ✓ Graph-based test generator with a correctness oracle + mutation fuzz harness (`core/testgen` + `zsh-gen` CLI) — existing

### Active

<!-- This milestone. Both bugs live in CONCERNS.md → ## Known Bugs; the regression pin already waits in property_test.go. -->

- [ ] **Line-count off-by-one** — `Analysis.Lines` is correct for every file shape: empty file → `0`; a file ending in `\n` is not over-counted by one (`analyzer.go:28`).
- [ ] **Issue line mis-attribution** — when a statement has a leading `#` comment, its issue reports the **statement** line, not the comment's line (`parse.go:36–40` → `reconciler.go:46,67` via `Block.StartLine`).
- [ ] **Pin the fix with tests** — flip `core/testgen/property_test.go` `checkLines` from `false` to `true` so the oracle property test asserts `Lines` and per-issue line slices across all 10 seeds.
- [ ] **Golden corpus stays honest** — the existing `manifests.json` corpus passes against the corrected output; any manifest that encoded the buggy line behaviour is corrected.

### Out of Scope

<!-- Explicit boundaries with reasoning. -->

- **Whole-file opaque fallback** — filed under CONCERNS.md → ## Fragile Areas, *not* a line bug. Partial-parse recovery needs upstream `mvdan/sh` work; separate effort.
- **Path-segment mis-naming** (`dupPathIssues` reporting `./scripts` as `/scripts`, skipping unrooted entries) — a real wrong-output bug, but the user scoped this milestone to *just* the two line bugs.
- **Adding golden fixtures for `duplicate_path` / `shadowed`** and asserting `issue_names` in the corpus — declined for this milestone (corpus update here is limited to keeping existing fixtures honest with the corrected lines).
- **Completing the dynamic-introspection half** (consume the resolved `IdentitySet`) — top of the product backlog, but a feature, not a bug fix.
- **Classifier precision overhaul** (surface confidence, demote sub-high-confidence to an explicit "uncertain" bucket, tighten PATH/secret over-captures) — a captured design stance (see Key Decisions + REQUIREMENTS.md v2 PREC-*); its own future phase, out of scope for this line-number milestone.
- **New commands / multi-file / other shells** (`fix`/`doctor`, `--paths`, bash-pro) — product ramp, not this milestone.

## Context

- **Brownfield.** The v1 analyze engine and the graph-based test generator both shipped and were reviewed clean (2026-06-22 / 2026-06-23). This milestone is a focused correctness pass on top of that.
- Both target bugs are already documented: `.planning/codebase/CONCERNS.md` → **## Known Bugs**, and `docs/BACKLOG.md`.
- The regression pin already exists: `core/testgen/property_test.go:18` holds `const checkLines = false` with the comment *"Flip to true once those are fixed."* This milestone is what flips it.
- Exact bug sites:
  - Off-by-one: `core/analyze/analyzer.go:28` — `Lines: strings.Count(string(src), "\n") + 1`.
  - Mis-attribution: `core/shell/zsh/parse.go:36–40` (comment-pull-up sets `startLine` to the comment line) → consumed in `core/analyze/reconciler.go:46,67` via `Block.StartLine`; `core/model/block.go` is where a precise statement line would live.
- The corpus today asserts issue `Kind` only and does not assert `Lines`, so the engine fix is unlikely to break golden fixtures — but `empty.zsh` (currently `min_blocks: 0`, no `Lines` assertion) is the canonical empty-file case to verify.

## Constraints

- **Tech stack**: Go 1.25+; single external dependency (`mvdan.cc/sh/v3`) — **no new dependencies** for this work.
- **Architecture**: Respect the existing layering — `core/analyze` stays shell-free (interface seam only); `core/testgen` imports only `core/model`; `core/shell/zsh` is touched only via the `Provider` seam / composition root.
- **Compatibility**: Correcting the line numbers is a deliberate `analyze --json` wire-contract change — accepted by the user. It alters emitted `Lines` and issue `lines` values.
- **Testing**: TDD. The `testgen` oracle property test (10 seeds) is the primary regression pin; line assertions stay on (`checkLines = true`) after this work.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Scope limited to the two line-number bugs | User chose the smallest, lowest-risk change; adjacent bugs (path mis-naming, corpus expansion) deferred | — Pending |
| Fix is pinned by tests (`checkLines` → true) | The regression pin already exists in `property_test.go` awaiting exactly this fix | — Pending |
| Off-by-one fix: `len==0 ? 0 : Count("\n") + (lastByte!='\n' ? 1 : 0)` | Empty → 0; trailing-newline files counted correctly; `Lines` stays consistent with 1-based statement line numbers | — Pending (recommended; confirm in plan) |
| Mis-attribution fix: add a precise statement-line field on `Block` | Keeps `Block.StartLine` (the block's true start, incl. comments) intact; issues emit the exact statement line | — Pending (recommended; confirm in plan) |
| **Classifier: precision over recall.** Below high confidence, flag an explicit "uncertain" bucket (never a confident category), surface confidence in output, and tighten over-capturing PATH/secret rules. A silent false positive is worse than an honest "unsure." | User design principle (2026-06-24). NOTE — today's classifier does the *opposite*: it always assigns a category (`misc` fallback), `Block.Conf` is computed (`analyzer.go:38`) but read nowhere, and PATH (`classify.go:39` substring) / secret (`:34` substring) rules over-capture at Medium/High confidence. | — Pending (own future phase; **NOT** Phase 1) |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-24 after initialization*
