# zsh-pro

## What This Is

zsh-pro is a read-only zsh-config analyzer CLI. It parses a zsh config file (default `~/.zshrc`) with a real AST parser, classifies each entry (environment, aliases, functions, path, secrets, …), detects config issues (duplicate aliases, reassigned env vars, duplicate PATH entries, shadowed names), flags likely secrets, and emits either a human-readable report or a `--json` envelope for agents. As of milestone v1.0, the line numbers it reports are **correct** — the two documented line-number bugs are fixed and pinned by the testgen oracle.

## Core Value

`analyze --json` reports line numbers you can trust — every issue points at the real statement line, and the reported line count is accurate.

## Current Milestone: v1.1 Trustworthy PATH Analysis

**Goal:** Every PATH entry `analyze` reports is named correctly, genuine duplicates are caught across notations, and risky relative entries are flagged — without polluting the exit-code signal.

**Target features:**
- **Correct extraction** — split the PATH-family assignment value on `:` (verbatim entries, drop the `$PATH` self-reference), fixing both the `./scripts`→`/scripts` mis-naming and the unrooted-entry blind spot.
- **Semantic dedup** — notation-only canonicalization (`~` ≡ `$HOME` ≡ `${HOME}`, trailing/duplicate slashes normalized); no filesystem or live-`$HOME` resolution, so analysis stays deterministic and read-only-pure.
- **Relative-entry advisory** — a new issue kind for relative/unrooted PATH entries, including bare `.` and empty (current-directory) entries.
- **Issue severity tier** — a new severity field on `Issue` so the advisory surfaces without bumping the exit code; exit 3 stays reserved for genuine problems (duplicates, shadows).
- **Coverage** — golden fixtures for the two untested issue kinds (`duplicate_path`, `shadowed`), corpus assertions on `issue_names`/`issue_lines`, and the testgen oracle extended to relative/unrooted dup paths as the regression pin.

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
- ✓ **Line-count off-by-one fixed** — `Analysis.Lines` correct for every file shape (empty → 0; trailing `\n` not over-counted) — Phase 1 (LINE-01)
- ✓ **Issue line attribution fixed** — issues report the statement's own line, not a leading comment's line — Phase 1 (LINE-02)
- ✓ **Both fixes pinned by tests** — `checkLines = true`; the 10-seed oracle asserts total `Lines` + per-issue line slices (non-circular `RenderedLines`) — Phase 1 (PIN-01)
- ✓ **Golden corpus kept honest** — corpus passes against corrected output; `empty.zsh` pinned at 0 lines — Phase 1 (PIN-02)
- ✓ **Issue severity tier** — every issue carries a non-omitempty `actionable`/`advisory` severity; only actionable issues drive `exit_code` and `issues_found` (both via `HasActionableIssues()`, so they cannot disagree); the 4 existing kinds stay byte-identical (`SevActionable` is the zero value) — Phase 2 (SEV-01, SEV-02)

### Active

<!-- Milestone v1.1 (Trustworthy PATH Analysis). REQ-IDs detailed in REQUIREMENTS.md; mapped to phases by the roadmap. -->

- [ ] PATH entries are extracted by splitting the assignment value on `:`, so relative entries are named correctly (`./scripts`, not `/scripts`) and unrooted entries are detected
- [ ] Notation-equivalent entries (`~`/`$HOME`/`${HOME}`, trailing/duplicate slashes) are treated as the same entry for duplicate detection (notation-only; no filesystem/env resolution)
- [ ] Relative/unrooted PATH entries — including bare `.` and empty entries — are surfaced as a new advisory
- [ ] Golden fixtures cover `duplicate_path` and `shadowed`, the corpus asserts `issue_names`/`issue_lines`, and the testgen oracle pins relative/unrooted dup paths

### Out of Scope

<!-- Explicit boundaries with reasoning. -->

- **Filesystem / live-`$HOME` resolution of PATH entries** — v1.1 canonicalization is *notation-only* (string-level); resolving `~`/`$HOME` against the actual environment, or `..`/symlinks against disk, would make a read-only static analyzer env-dependent and non-deterministic.
- **PATH ordering / precedence analysis** (which earlier entry shadows a later one) — a separate order-sensitivity feature, not part of this correctness pass.
- **Whole-file opaque fallback** — filed under CONCERNS.md → ## Fragile Areas, *not* a path bug. Partial-parse recovery needs upstream `mvdan/sh` work; separate effort.
- **Classifier precision overhaul** (surface confidence, demote sub-high-confidence to an explicit "uncertain" bucket, tighten the `Contains("PATH")` / secret over-captures) — a captured design stance (REQUIREMENTS.md v2 PREC-*); its own future phase. Note: v1.1's *reconciler* PATH fix is independent of the *classifier's* PATH over-capture.
- **Completing the dynamic-introspection half** (consume the resolved `IdentitySet`) — top of the product backlog, but a feature, not a bug fix.
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
| Scope limited to the two line-number bugs | User chose the smallest, lowest-risk change; adjacent bugs (path mis-naming, corpus expansion) deferred | ✓ Done (Phase 1) — held to scope |
| Fix is pinned by tests (`checkLines` → true) | The regression pin already exists in `property_test.go` awaiting exactly this fix | ✓ Done (Phase 1, PIN-01/02) — oracle asserts total + per-issue lines across 10 seeds; golden corpus pins `empty.zsh` at 0 |
| Off-by-one fix: `len==0 ? 0 : Count("\n") + (lastByte!='\n' ? 1 : 0)` | Empty → 0; trailing-newline files counted correctly; `Lines` stays consistent with 1-based statement line numbers | ✓ Done (Phase 1, LINE-01) — implemented as recommended (editor-style `countLines`) |
| Mis-attribution fix: add a precise statement-line field on `Block` | Keeps `Block.StartLine` (the block's true start, incl. comments) intact; issues emit the exact statement line | ✓ Done (Phase 1, LINE-02) — **superseded**: no `Block` field added; instead deleted the comment-line overwrite in `parse.go` so `Block.StartLine` stays the statement line (reconciler untouched) |
| **Classifier: precision over recall.** Below high confidence, flag an explicit "uncertain" bucket (never a confident category), surface confidence in output, and tighten over-capturing PATH/secret rules. A silent false positive is worse than an honest "unsure." | User design principle (2026-06-24). NOTE — today's classifier does the *opposite*: it always assigns a category (`misc` fallback), `Block.Conf` is computed (`analyzer.go:38`) but read nowhere, and PATH (`classify.go:39` substring) / secret (`:34` substring) rules over-capture at Medium/High confidence. | — Pending (own future phase; **NOT** Phase 1) |
| **v1.1 scope (broad):** fix PATH extraction + semantic dedup + relative-entry advisory + severity tier + close `duplicate_path`/`shadowed` coverage | User chose the broad option across all three v1.1 scope questions (2026-06-24): fix it, catch notational duplicates, flag risky entries, and pin with real fixtures | — Pending (Milestone v1.1) |
| **PATH dedup is semantic but notation-only** (`~`/`$HOME`/`${HOME}` + slashes canonicalized; no filesystem/env resolution) | Catches real notational duplicates of the same dir while staying deterministic and read-only-pure; avoids the "`$HOME` reassigned mid-file" false positive | — Pending (Milestone v1.1) |
| **Relative-entry advisory is a new informational severity, not exit-3** | A relative/unrooted entry may be intentional; conflating it with genuine duplicates/shadows at exit 3 would degrade the agent signal. Introduces the first `Issue` severity tier (also seeds future classifier-precision work). Covers bare `.`/empty (cwd) entries — the classic PATH foot-gun | — Pending (Milestone v1.1) |

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
*Last updated: 2026-06-24 — milestone v1.1 (Trustworthy PATH Analysis): Phase 2 (Issue Severity Tier) complete — every issue carries an actionable/advisory severity; only actionable issues drive exit_code/issues_found. Next: Phase 3 (Trustworthy PATH Extraction & Detection).*
