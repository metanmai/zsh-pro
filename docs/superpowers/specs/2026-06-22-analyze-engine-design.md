# zsh-pro — Analyze Engine (v1) Design Spec

**Status:** Approved design — ready for implementation planning
**Date:** 2026-06-22
**Scope:** The first backend piece of zsh-pro — a *read-only* configuration analysis engine for zsh, exposed through an `analyze` command.

---

## Context

zsh-pro is a CLI tool (with a future TUI) that owns the **organization** of a user's shell configuration — keeping a `.zshrc` tidy, addressable, and portable — without owning installation, the plugin framework, or sync transport. The full product vision lives in the original design doc (`zsh-config-tool-design.md`, kept outside this repo).

This spec covers only the **first piece**: a read-only engine that parses a `.zshrc`, classifies it into categories, and reports duplicates, shadowing, and conflicts. **Nothing is written or reorganized.** It is simultaneously:

- the **foundation** every later command parses through, and
- a standalone, **safe** artifact we can put in front of users to validate demand.

The agreed build ramp (easy → hard), for orientation:

1. **analyze (read-only)** ← *this spec*
2. split / adopt (first write; conservative, with a run-both-and-diff safety net)
3. reconcile / merge engine + tidy (quarantine sweep)
4. profiles / import-export / sync polish
5. "clever" order-safety + smarter reorganization

(The TUI slots in wherever wedge-validation findings say it should.)

---

## Goals

- Parse a real-world `.zshrc` into structured, categorized blocks.
- Detect duplicates, shadowed definitions, and conflicts by **identity** (alias name, env var, PATH entry, function name) — not by line.
- Render the analysis as human output **and** as `--json` for scripting/agents.
- Be trustworthy on messy real-world configs — never crash, never silently mislead.

## Non-goals (v1)

- No writing, `split`, `adopt`, reorganization, or order-safety logic.
- No TUI (the `ui/` directory is scaffolded but unbuilt).
- No multi-shell **product** — zsh only. The architecture is shell-agnostic so `bash-pro` can fork later; we are **not** building bash now.
- No installation, sync, profiles, or import/export.

## Success criteria

- Correctly parses and classifies a corpus of **real-world** configs, not just clean samples.
- Reported conflicts match what zsh **actually resolves** at runtime.
- Validated against a two-tier test corpus (see Testing).

---

## Architecture

### Language & repository

- **Go**, a **single module** at the repo root. Monorepo split into `core/` (engine + CLI) and `ui/` (future TUI).
- **Distribution:** a single static binary — the main reason for Go over the Python prototype (which becomes reference-only).
- **Parser:** [mvdan/sh](https://github.com/mvdan/sh) used **in-process** — it supports zsh since v3.13 (`LangVariant = Zsh`).

### Shell-agnostic seam

Everything shell-specific sits behind a `Provider` interface; the rest of the engine is shell-agnostic and reused across shells.

- **Shell-specific** (the zsh `Provider` impl): parse dialect, classification rules + taxonomy, introspection mechanism.
- **Shell-agnostic** (core): pipeline orchestration, the `Analysis` model, reconciliation, dup/shadow detection, rendering, CLI.
- **`bash-pro`** (future) = a fork that swaps `core/shell/zsh` → `core/shell/bash` (mvdan/sh `LangVariant = Bash`; introspection via `compgen`/`declare`/`alias`; bash rules). Everything in `model/ analyze/ render/ cli/` is reused untouched.

> **Caveat (recorded deliberately):** we have exactly one shell as a data point. Design the `Provider` interface around what zsh needs and keep it minimal — expect to adjust it when `bash-pro` is real. Premature over-abstraction is the only risk here, and it is easy to avoid.

### Components

```
zsh-pro/                     (monorepo, one Go module)
  core/
    model/        agnostic types: Block, Category, Issue, IdentitySet, Analysis
    shell/        the seam — Provider { Parse, Classify, Introspect, Categories }
      zsh/        the zsh impl:
        parse.go        mvdan/sh, LangVariant=Zsh
        classify.go     zsh rules + taxonomy  (classifier "B" lives here)
        introspect.go   zsh -f + zsh/parameter
    analyze/      agnostic: drive Provider -> reconcile static+dynamic -> Analysis
    render/       agnostic: Analysis -> human / --json
    cli/          agnostic command wiring (analyze)
    cmd/zsh-pro/  main() — selects the zsh provider, hands it to cli
    testdata/     mock fixtures + ground-truth manifests
  ui/             TUI (Bubble Tea) — scaffolded now, built later
```

Each package has one clear job and is testable in isolation.

### Static + dynamic composition (key insight)

Static and dynamic views are **complementary, not redundant**:

- **Static (mvdan/sh AST)** — sees *every definition and its position* → this is what detects duplicates/shadowing (the count + locations, e.g. "`gs` defined at lines 10, 40, 90").
- **Dynamic (zsh introspection)** — sees only the *resolved end-state* → the **winner** ("`gs` is effectively the line-90 one"), and it catches identities created by mechanisms static parsing can't read (`eval "$(starship init)"`, `source somefile`).
- **The mismatch is itself signal:** defined-but-not-live → dead/conditional code; live-but-unattributed → produced by an opaque init.

### Classifier — approach B

Ordered deterministic rules over the AST node (assignment? `alias`? function? `setopt`? `eval "$(… init)"`?) plus name patterns (PATH/fpath, secret-ish, plugin hints), **with a confidence score and an explicit "unsure → review" bucket** — low-confidence blocks are *flagged*, not silently misfiled.

- Rejected **A** (plain best-effort rules, no confidence): silent misfiling erodes trust.
- Rejected **ML**: overkill, opaque, non-deterministic, against the transparent-text spine.

### Category taxonomy

`environment → path → secrets → plugins/init → options → keybindings → functions → aliases → local → misc`

(This order also encodes load-order for later commands. For `analyze` it is purely classification.)

---

## Data flow

One pass — static and dynamic run independently, then reconcile:

```
.zshrc → parse (mvdan/sh) → []Block → classify → categorized blocks ┐
       → introspect (zsh -f + zsh/parameter) → IdentitySet ─────────┤
                                                                     ▼
                          analyze reconciles → Analysis → render (human | --json)
```

Renderers are pure consumers of the `Analysis` model.

## Failure modes (trust-critical)

- **Parse:** a block mvdan/sh can't handle (incomplete zsh support) → captured as an **opaque block**, never a crash. Analysis still completes.
- **Introspection:** zsh missing / the config errors out / exceeds a timeout → **degrade to static-only and say so** ("couldn't safely run your config to verify live conflicts — showing static analysis only"). Never crash, never silently pretend we had ground truth.
- **Sandbox:** introspection runs `zsh -f` (no rc files), with the sourced config's stdout/stderr suppressed and a hard timeout; we read back only the `zsh/parameter` dumps.
- **`--json` on every path:** success or failure emits one structured object (`{"ok": false, "error": …}`).

## CLI / agent contract

- `zsh-pro analyze [path] [--json]` — `path` defaults to `~/.zshrc`.
- Exit codes (inherited from the prototype): `0` clean · `1` runtime error · `2` usage error · `3` actionable (issues found).
- `--json` emits exactly one JSON object on stdout; human output is the default.

## Testing

**Two-tier corpus:**

- **Tier 1 — hand-authored mock fixtures** with ground-truth manifests → exact assertions (golden tests). Coverage taxonomy:
  - **Core** — an installer-junk pile (conda/nvm/pyenv/fzf appended at bottom); a clean, well-organized baseline (control).
  - **Conflict/identity** — duplicate aliases, an env var reassigned multiple ways, a PATH entry added twice, cross-type shadowing (alias shadowed by a function).
  - **Order sensitivity** — two PATH prepends whose order changes the winner; var-before-use; `fpath`-before-`compinit`.
  - **zsh-specific syntax** — `setopt`/`zstyle`/`bindkey`/glob qualifiers/anonymous functions, deliberately including constructs mvdan/sh is documented to choke on (numeric-range globs, arithmetic math funcs) → validates the opaque-block fallback.
  - **Structural edge cases** — heredocs, multiline functions, line continuations, `if`/`case` blocks, misattached comments.
  - **Secrets** — inline API keys/tokens → detection + the "don't sync" path.
  - **Pathological** — empty file, comments-only, an actual syntax error → must degrade, never crash.
- **Tier 2 — real-world configs** (fast-follow): unlabeled; tested for "doesn't choke / output looks sane." Scrub stray secrets before committing.

Plus **`--json` snapshot tests** (lock the machine-output shape) and **static-vs-dynamic agreement checks** on fixtures.

> Generating the Tier-1 fixtures + manifests is the **first implementation task** — their labels depend on the final taxonomy/conflict model, so they can't be authoritatively written before the engine's model is fixed.

---

## Key decisions

| Decision | Choice |
|---|---|
| Order of work | Backend-first; the read-only analyze engine is piece #1 |
| Language | Go (Python prototype → reference only) |
| Repo | Single Go module; monorepo `core/` + `ui/` |
| Multi-shell | Shell-agnostic `Provider` seam; zsh first; `bash-pro` = future fork |
| Analysis method | Static (mvdan/sh AST) + dynamic (zsh introspection), reconciled |
| Parser | mvdan/sh in-process (`LangVariant=Zsh`) |
| Classifier | Ordered rules + confidence + "unsure → review" bucket (B) |
| CLI contract | Keep the prototype's `--json` + exit-code (0/1/2/3) contract |

## Open questions / deferred

- Exact `Provider` interface shape — finalize against zsh; revisit for `bash-pro`.
- When to add Tier-2 real configs (after the engine basically works).
- TUI timing (per wedge-validation findings).
- Everything past `analyze` in the build ramp.
