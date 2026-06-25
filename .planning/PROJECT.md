# zsh-pro

## What This Is

zsh-pro is a **git-versioned, branchable shell-environment manager**. It ingests a zsh config (default `~/.zshrc`) with a real AST parser, classifies each entry (environment, aliases, functions, PATH, options, secrets, …) into a structured, regenerable representation, and stores it as a git-style repo where **each branch is an environment profile**. Switching branches live-reloads the terminal into that profile — via a sourced activate/deactivate manifest — so you can keep distinct shell environments (work, personal, a client's stack) and `checkout` between them.

The parse → classify → introspect engine (shipped across v1.0–v1.1 as a read-only analyzer) is the **ingest/understanding component** of this product, not the product itself. Earlier milestones over-framed that analyzer as the whole tool; v2.0 corrects the documented identity to the environment manager it was always meant to be.

## Core Value

`checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases, env, PATH, functions, options) applies and reverses cleanly with **zero residue**, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).

## Current Milestone: v2.0 Branchable Shell Environments

**Goal:** Turn the ingest engine into a manager — represent `~/.zshrc` as a categorized, regenerable store, make each git branch an environment profile, and let `checkout <branch>` live-reload the terminal into that profile with zero residue.

**Target features:**
- **Ingest & categorize** — parse `~/.zshrc` into a structured, regenerable representation split by category (aliases / env / PATH / functions / options), reusing the existing parser + `Cat*` classifier as the front-end.
- **Partial evaluation** — resolve static/constant values; keep dynamic ones (`$HOME`, `$(...)`, conditionals) unresolved so profiles stay portable across machines.
- **Git-backed profiles** — store the representation as a git-style repo; branches are switchable environment profiles.
- **Activate/deactivate manifest** — switching a live terminal deactivates the prior branch's managed state (unalias, `unset -f`, restore env, rebuild PATH from a captured base) then activates the new one, via a sourced shell integration (no parent-process mutation).
- **Declarative vs imperative split** — only declarative state is switchable; imperative run-once code stays in a thin bootstrapping `.zshrc` "master block".
- **(Frontier) zero-residue live hot-switch** — switching in an already-open terminal leaves no leftover aliases / PATH growth / stale env. **De-risked: the Phase 1 spike returned GO** — a live `zsh -f` `activate → switch → switch-back` is byte-identical across all six state classes; the validated `Manifest` shape is captured as Phase 4's input.

**Foundational note:** v1.1 (Trustworthy PATH Analysis) was parked partway (Phase 2 shipped) when the product identity was corrected from "analyzer" to "environment manager" — see [MILESTONES.md](MILESTONES.md). Its PATH parsing/canonicalization work is re-scoped under this milestone's ingest layer.

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
- ✓ **Zero-residue live hot-switch proven feasible (GO)** — Phase 1 spike asserts `$aliases`/`$functions`/`$PATH`/`$path`/exported-env/`$options` byte-identical after `activate → switch → switch-back`; no PATH accumulation across cycles; the conditional drift guard holds; `compinit` excluded to the master block (the `fpath` array itself is reversible). Durable outputs: `01-FINDINGS.md` + `01-MANIFEST-SHAPE.md` — v2.0 Phase 1 (SW-03)

### Active

<!-- Milestone v2.0 (Branchable Shell Environments). REQ-IDs defined in REQUIREMENTS.md; mapped to phases by the roadmap. -->

- [ ] Ingest `~/.zshrc` into a categorized, regenerable representation (aliases / env / PATH / functions / options)
- [ ] Partial evaluation — resolve static values, keep dynamic ones (`$HOME`/`$(...)`/conditionals) late-bound
- [ ] Git-backed environment profiles (branches); `checkout <branch>` selects a profile
- [ ] Sourced activate/deactivate manifest that switches a live terminal with zero residue (declarative state only)
- [ ] Thin bootstrapping `.zshrc` (master block + loader); imperative run-once code stays unmanaged

<!-- Parked from v1.1 (see milestones/v1.1-ROADMAP.md): trustworthy PATH extraction + notation dedup is re-scoped into the ingest layer above; analyzer-reporting/oracle work (duplicate_path/shadowed fixtures) is deferred. -->

### Out of Scope

<!-- Explicit boundaries with reasoning. Reframed for v2.0 (environment manager). -->

- **Filesystem / live-`$HOME` resolution** — dynamic values (`~`/`$HOME`/`$(...)`) stay late-bound and unresolved; resolving them against a specific machine's disk or environment would freeze a profile to that machine and destroy the portability that makes branches useful.
- **Owning the imperative startup surface** — arbitrary run-once code (daemons, `eval`, side-effecting init) is NOT made switchable; it stays in the unmanaged `.zshrc` "master block". Only declarative state is branch-switchable.
- **Other shells** (bash, fish) — v2.0 targets zsh only; the activation model is zsh-specific (`zmodload zsh/parameter`, `unalias` / `unset -f` semantics).
- **Multi-file config graphs** — single-entry-point ingest first; deep following of sourced files / `*.zsh` fragments is a later concern.
- **PATH ordering / precedence analysis** and **classifier precision overhaul** (the "uncertain" bucket, tightening over-captures) — captured design stances for later, independent of the manager's core switch loop.

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
| **v2.0 Phase 1 spike → GO**: zero-residue live hot-switch is feasible | Byte-identical six-class round-trip proven in a live `zsh -f`; core classes (aliases/env/PATH) + functions/options admitted MANAGED; `compinit` excluded to the master block. Carry-forwards for Phase 4: loaders must trust the *live prior* value (not a static `original` key), and measure *value*-delta (not env name-set) under `zsh -f` env inheritance. | ✓ Done (v2.0 Phase 1, SW-03) — `01-FINDINGS.md` + `01-MANIFEST-SHAPE.md` are the durable deliverables; the validated Manifest shape is Phase 4's literal input |

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
*Last updated: 2026-06-25 — pivoted to v2.0 (Branchable Shell Environments): corrected the project identity from "read-only analyzer" to a git-versioned, branchable shell-environment manager (the analyzer is now its ingest component). v1.1 parked partway (Phase 2 shipped) — see MILESTONES.md. · v2.0 Phase 1 (SPIKE) complete 2026-06-25 — zero-residue live hot-switch validated **GO**; proceeding to Phase 2 (IR + Partial Evaluation).*
