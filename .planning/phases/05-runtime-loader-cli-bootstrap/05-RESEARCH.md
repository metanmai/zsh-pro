# Phase 5: Runtime Loader + CLI + Bootstrap — Research

**Researched:** 2026-07-02
**Domain:** zsh runtime loader / live-terminal state mutation, Go embedded-const codegen, idempotent `.zshrc` file rewrite, fail-open shell integration, startup-cost verification
**Confidence:** HIGH — the load-bearing zsh semantics, the idempotent-installer algorithm, the zero-residue PATH rebuild, the fail-open guards, and the zero-subprocess/startup-cost claims were all **executed** on this machine (zsh 5.9 arm64-apple-darwin25.0, go 1.25.7). `hyperfine` is ABSENT (a dev/CI tool, not a Go dep) — the startup budget was measured with an in-process `EPOCHREALTIME` proxy and the methodology for the real `hyperfine` gate is documented.

## Summary

Phase 5 has **no design ambiguity that blocks planning**. The SPEC (7 reqs), CONTEXT (D-01..D-20), reviewed remediation plans, and OQ-05-01..18 now record either an adopted decision or a bounded execution-time gate. This research verifies the underlying zsh/Go semantics and records the exact gates where a live Phase 4 interface or optional CI tool is the source of truth. Every load-bearing claim below is tagged `[VERIFIED: ran it]` with the experiment, or `[ASSERTED]` with the experiment that would settle it.

The five sub-areas reduce to one architectural spine: **a child process cannot mutate its parent shell** (verified: a `$(...)` subshell's `cd`/`export` do not escape to the parent, but `eval` of the same body does) — therefore the mutating verbs (`checkout`/`activate`/`deactivate`) MUST be **sourced zsh functions** that `eval "$(zsh-pro emit …)"`, and the loader that defines them MUST be embedded zsh living as a `const` in `core/shell/zsh` (mirroring `introspectScript`), fetched across a provider seam and printed by a pure `hook` verb. The hot path stays zero-subprocess by sourcing a **cached loader file** written at `install` time (never `eval "$(zsh-pro hook)"` per start); measured overhead of sourcing the pure-function-def loader is **~0.03 ms/shell-start** — three orders of magnitude under the < 10 ms budget (OQ-05-04).

**Primary recommendation:** Build exactly to CONTEXT D-01..D-20 as written. The research surfaced no reason to override any decision; it *confirmed* the `${(P)+var}=="1"` env guard, the `${slot+x}` shadow set-test (vs the buggy `[[ -n "$slot" ]]`), the PATH-rebuild-from-base zero-residue, the once-captured `ZP_BASE_PATH` guard, the slot-name sanitization (both a `typeset` correctness fix AND an injection block — an unsanitized `$(...)` in a slot name *executed* in E16), the byte-identical idempotent installer, and the fail-open guards. Plan-time gate: the `zp_*` helper set is NOT a settled "exact match" — Phase 4 commits by name only to `zp_capture_env`/`zp_restore_env` and documents shadow/PATH/option reverse ops INLINE, so reconcile the loader's provided helpers/state against the *actual* `emit.go` bare-call surface once Phase 4 lands, do not assume `zp_rebuild_path`/`zp_shadow_*` are in the contract (OQ-05-05/OQ-05-13).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Emit loader script (`hook`) | `core/shell/zsh` (const) → `core/cli` (dispatch) | composition root (seam wiring) | Only `core/shell/zsh` writes zsh syntax (milestone invariant); `hook` is a pure const print, no store |
| Define `zp_*` helper bodies | `core/shell/zsh` (inside the loader const) | — | The loader IS where the helpers Phase 4 emits bare calls to are defined (OQ-05-01, closes Phase 4 OQ-5) |
| Live-terminal mutation (`activate`/`checkout`/`deactivate`) | **sourced zsh function** (runtime) | binary `emit` subcommand (codegen) | A child binary cannot mutate its parent shell (VERIFIED E2); functions `eval` the binary's stdout |
| Read-only reporting (`list`/`status`) | `core/cli` + `core/store` | thin sourced wrapper | `Store.Branches`/`Store.Current` exist; verbs surface them; no `eval` needed |
| Per-terminal active state | env vars (`ZSHPRO_PROFILE`, `ZP_BASE_PATH`, `__ZP_ORIG_*`, shadow slots) | — | Phase 3 D-13: no shared file (avoids conda concurrent-activation race) |
| `.zshrc` block install | `core/cli` + Go file rewrite | filesystem | Byte-exact find-region-then-replace in Go (D-10) |
| Fail-open guarding | installed `.zshrc` stub (zsh) | — | `command -v` / `[[ -r ]]` / `ZSHPRO_DISABLE` — pure shell, zero subprocess |
| `zsh -n` validation | sourced verb (subprocess on explicit action) | binary self-check (alt) | Closest to the `eval` boundary; a subprocess is acceptable off the hot path (D-15) |
| Startup-cost verification | dev/CI (`hyperfine`) | in-test grep (structural) | Structural zero-subprocess grep is load-bearing; ms budget is the backstop |

## User Constraints (from CONTEXT.md)

> These are locked. Research does not re-open them; it verifies the behavior they assume.

### Locked Decisions (D-01..D-20, verbatim intent)
- **D-01** Loader helper/state surface is a plan-time RECONCILIATION GATE, not a settled "exact match": Phase 4 commits by name only to `zp_capture_env <var>` / `zp_restore_env <var> <applied>` (env); its shadow, PATH-rebuild, and option reverse ops are documented INLINE (Phase 4 D-12), and `emit.go` is not yet on disk. The loader definitively provides the two named env helpers + the runtime STATE the inline reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, `was_on`); PATH-rebuild/shadow are NOT assumed to be `zp_rebuild_path`/`zp_shadow_*` helpers. Signatures started from the Phase 1 reference snippet + Phase 4 D-12/OQ-8; reconciled against `emit.go`'s ACTUAL bare-call surface once Phase 4 lands (OQ-05-05/OQ-05-13).
- **D-02** Slot names sanitized to `[A-Za-z0-9_]` (C24) — correctness (`typeset` target) AND injection block.
- **D-03** `ZP_UNSET_SENTINEL` is a loader-defined constant, `typeset -g` once, shared by capture+restore.
- **D-04** `emulate -L` / `LOCAL_OPTIONS` FORBIDDEN in emitted apply/deactivate AND in `zp_*` helpers that touch options (opposite of `introspectScript`, which correctly isolates).
- **D-05** Verbs are sourced zsh SHELL FUNCTIONS; mutating ones `eval` the binary's stdout. `list`/`status` are thin binary/store wrappers, never `eval`.
- **D-06** `checkout` = validate-then-activate (deactivate-prior + activate-target + export `ZSHPRO_PROFILE`).
- **D-07** Shell function (not binary) exports `ZSHPRO_PROFILE` after a successful eval; `deactivate` unsets it.
- **D-08** `ZP_BASE_PATH` captured ONCE at first activate per terminal, re-capture guarded: `[[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"`. Guard lives in the sourced activate path, NOT the emitted code.
- **D-09** Install verb = `install`; markers = `# >>> zsh-pro >>>` / `# <<< zsh-pro <<<`.
- **D-10** Rewrite = find-region-then-replace, in Go, byte-exact; append if absent; collapse dupes; preserve outside-marker bytes; missing file ⇒ create.
- **D-11** Installed block is a THIN fail-open STUB (guards + source cached loader), not the loader itself.
- **D-13** Hot path zero-subprocess by construction — source a CACHED loader file, not `eval "$(zsh-pro hook)"` per start.
- **D-14** Guard order: `ZSHPRO_DISABLE` early-return → `[[ -r <cached-loader> ]]` → source.
- **D-15** Emitted code `zsh -n`-validated BEFORE `eval`, inside the verb (not the hot path).
- **D-16** Per-terminal LAST-GOOD record; validate the ENTIRE emitted block up front, refuse to eval any of it on failure (atomic per switch).
- **D-17** `hyperfine` budget < 10 ms added mean; structural zero-subprocess grep is the load-bearing guard.
- **D-18** New verbs dispatch from `CLI.Run`'s `switch args[0]`, mirroring `analyze`. Reuse `model.ExitCode` (0/1/2/3).
- **D-19** Store injected into `CLI` at the composition root; `core/cli` never imports the concrete store.
- **D-20** Loader const lives in `core/shell/zsh`; `core/cli` fetches it via a provider seam (`HookScript()`), holds no zsh text.

### Claude's Discretion
- Exact Go identifiers, file names, and internal structure of the new CLI verb handlers and any store-facing interface in `core/cli` (D-18/D-19).
- Precise seam method exposing the loader const (`HookScript()` on Provider vs standalone `shell.Hooker`).
- Whether PATH-rebuild is a named `zp_*` helper or inlined by `emit.go` — loader supplies whatever the emitted calls name (OQ-05-05).
- Binary emit subcommand naming/shape (`zsh-pro emit apply <name>` vs two subcommands) (D-06 / OQ-05-06).
- Where `zsh -n` physically runs (sourced-function here-string vs binary self-check) (D-15 / OQ-05-07); last-good payload richness (D-16 / OQ-05-08).
- Cached-loader file location + refresh trigger (under store/data dir, zero-subprocess start preserved).
- Test internals (fixtures, `LookPath`-guarded zsh tests, `hyperfine`/`zsh -n` harness), provided the falsifiable properties are pinned.
- Commit granularity.

### Deferred Ideas (OUT OF SCOPE)
- Secret deref-on-switch end-to-end (PROF-03) — Phase 6; Phase 5 only drives the existing `store.KeychainDriver` seam if emitted code surfaces an unresolved `SecretRef` (OQ-05-02).
- Out-of-block installer-append detection/warning — Phase 6.
- Real `~/.zshrc` end-to-end ingest → baseline commit — Phase 6.
- Keybindings (`bindkey`) / hooks (`precmd_functions`/`chpwd_functions`) / `compinit` — unmanaged master block (Phase 4 OQ-1).
- Auto-activate on `cd` — future AUTO-01 (would violate the zero-subprocess hot path).
- Multi-active / concurrent managed profiles + shared-profile trust gates — future.
- Other shells (bash/fish) — zsh-only milestone.
- Marker-text / stub-body migration for already-installed blocks across releases — later concern.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| BOOT-01 | Single idempotent BEGIN/END-marked `.zshrc` block bootstraps the loader, preserves the unmanaged master block, never duplicates on re-run | Sub-area (c): Go find-region-then-replace POC ran twice → **byte-identical** (E-INSTALL); dup-block collapse and user-content preservation verified |
| BOOT-02 | Loader fail-open and fast — broken/missing/slow `zsh-pro` never locks the user out; guarded sourcing, `zsh -n`-validated manifests, last-good fallback, `ZSHPRO_DISABLE=1` escape hatch, small file-sourced startup cost, no subprocess on hot path | Sub-area (d): `command -v` / `[[ -r ]]` / `ZSHPRO_DISABLE` guards verified (E10/E11); `zsh -n` rejects bad code without executing (E3); sub-area (e): zero-subprocess grep clean + ~0.03 ms/start measured (E13/E14) |

---

## Sub-area (a): Embedded sourced loader emitted by `hook`

### The verified spine

- **[VERIFIED: ran it]** A function defined in an `eval`'d string persists in the *current* interactive shell: `eval "foo(){ print -r -- hi }"; foo` → `hi` (E1). This is why the loader can be sourced once and its verbs/helpers stay available.
- **[VERIFIED: ran it]** `zsh-pro hook`'s output is `zsh -n`-parseable when it is pure function/const definitions: `zsh -n loader_pure.zsh` → exit 0 (E14). Acceptance `zsh-pro hook | zsh -n` == 0 is achievable by keeping the loader body to definitions only.
- The loader const mirrors `introspectScript` (a `const` raw-string in `core/shell/zsh/introspect.go`). **Divergence (D-04):** `introspectScript` opens with `emulate -L zsh` for isolation; the loader's apply/deactivate helpers must be PLAIN so `setopt`/`unsetopt` escape function scope.

### Candidate approaches — how the loader const is embedded & emitted

| # | Approach | Correctness | Fail-open | Startup | Portability | Verdict |
|---|----------|-------------|-----------|---------|-------------|---------|
| A1 | **Single `const` raw-string in `core/shell/zsh`, exposed via `HookScript()` on the Provider seam, printed by `hook`** (CONTEXT D-20 default) | HIGH — one authoritative body; `core/cli` holds no zsh | Same as any (stub guards it) | N/A (hook is explicit) | HIGH | **RECOMMENDED** — matches `introspectScript` precedent + milestone invariant |
| A2 | Go `//go:embed loader.zsh` from a real `.zsh` file | HIGH; nicer syntax highlighting/lint of the source | same | same | HIGH | Viable, but adds an embedded asset + build-time file; the `const` precedent (`introspectScript`) is already established and dependency-free. Not worth diverging. |
| A3 | Assemble the loader in `core/cli` from fragments | LOW — puts zsh text in `core/cli`, **violates the milestone invariant** (D-20) | same | same | same | **REJECTED** by constraint |

**Recommended default:** A1. Seam = `HookScript() string` on the Provider composite (`shell.Hooker` sub-interface, mirroring `Regenerator`/`Introspector`) — `core/cli` calls `c.provider.HookScript()` and prints it. (OQ-05-10 default; HIGH on the invariant.)

### The `zp_*` helper contract (reconciled against Phase 4's emitted call shape)

> **Plan-time gate (OQ-05-05/OQ-05-13):** Phase 4 is documented but **not yet on disk** — `core/shell/zsh/emit.go`, `core/activate`, `core/model/manifest.go` do not exist yet, so there is no literal bare-call surface to match against. Phase 4's emit-vs-inline shape is Claude's discretion (Phase 4 OQ-5) and Phase 4 commits **by name only** to `zp_capture_env`/`zp_restore_env` (env); its shadow, PATH-rebuild, and option reverse ops are documented **INLINE** (Phase 4 D-12), NOT as a `zp_rebuild_path`/`zp_shadow_*` helper set. This table is therefore the **starting shape for reconciliation, NOT a matched contract**: the loader definitively provides the two named env helpers + the runtime STATE the inline reverse ops read/write; at plan/execute time, **diff the loader's provided helpers/state against `emit.go`'s ACTUAL bare calls** and provide exactly those (localized — both live in `core/shell/zsh`).

Derived from the Phase 1 Loader Reference Snippet (`01-FINDINGS.md` §Loader Reference Snippet) + Phase 4 D-12/OQ-8 + the validated manifest shape (`01-MANIFEST-SHAPE.md`). **Only `zp_capture_env`/`zp_restore_env` are Phase-4-committed named helpers; the PATH-rebuild and shadow-capture/restore rows below describe reverse-op BEHAVIOR that Phase 4 may emit INLINE — do NOT assume they are `zp_*` helper calls in the contract:**

| Helper (loader defines) | Signature | Body contract | Verified |
|-------------------------|-----------|---------------|----------|
| `zp_capture_env` | `zp_capture_env <var>` | Capture LIVE prior of `$var` into `__ZP_ORIG_<var>` (sanitized), or `$ZP_UNSET_SENTINEL` if unset, using `[[ "${(P)+var}" == "1" ]]`. `typeset -g`. | E5 |
| `zp_restore_env` | `zp_restore_env <var> <applied>` | Reverse ONLY if `[[ "${(P)+var}" == "1" && "${(P)var}" == "$applied" ]]`; `$ZP_UNSET_SENTINEL` prior ⇒ `unset "$var"`, else `export "$var"="$prior"`. | E5, E15 |
| PATH rebuild (helper OR inlined by emit — OQ-05-05) | e.g. `PATH="$ZP_BASE_PATH"; path=(<additions> $path)` on apply; `PATH="$ZP_BASE_PATH"` on deactivate | Never `typeset -U`, never blind append (Phase 1 Pitfall 2). | E7, E15 |
| shadow-capture (alias) | slot `ZP_<profile>_PRIOR_ALIAS_<name>` = `"${aliases[name]}"` | per-profile prior body as data | E9-analog |
| shadow-restore (alias) | guarded by **`${slot+x}` set-test** (NOT `[[ -n "$slot" ]]`); `alias name="$prior"` | restores an EMPTY prior correctly | E6 |
| shadow-capture (func) | slot `ZP_<profile>_PRIOR_FUNC_<name>` = `"${functions[name]}"` | live body captured verbatim as data | E9 |
| shadow-restore (func) | `functions[name]="$prior"` (verbatim — NEVER single-quote-wrapped, C6) | body is executable code; single-quoting breaks the definition | E9 |
| `ZP_UNSET_SENTINEL` | `typeset -g ZP_UNSET_SENTINEL=<improbable marker>` once | shared by capture+restore; unset-vs-empty round-trips | E5/E15 |

**Slot-name derivation (D-02) — this is load-bearing and dual-purpose:**
- **[VERIFIED: ran it]** An unsanitized slot name with `/` is a hard `typeset` failure: `typeset -g "__ZP_ORIG_feature/x"=v` → `zsh:typeset: not valid in this context` (E16).
- **[VERIFIED: ran it]** An unsanitized slot name containing `$(...)` **executes the command substitution at name-construction time** — `name="__ZP_ORIG_$(echo PWNED >&2)"` printed `PWNED` (E16). Sanitizing `${name//[^A-Za-z0-9_]/_}` (verified E12: `feature/x`→`feature_x`, `x$(rm)`→`x__rm_`) is therefore **both** a correctness fix AND the injection block for slot names. Sanitize in the loader helper BEFORE any `typeset -g`/`${(P)}` on a derived name.

### `ZP_BASE_PATH` capture placement (Phase 4 OQ-3, finalized here)

- **[VERIFIED: ran it]** The once-capture guard works: `[[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"` — after profile A pollutes `PATH`, a second `capture` call leaves `ZP_BASE_PATH` at the original base, NOT `/work/bin` (E8). The guard MUST live in the sourced activate path *before any PATH mutation*, NOT in the emitted code (which assumes the base already exists — Phase 4 runtime contract).

---

## Sub-area (b): checkout/activate/deactivate/list/status as sourced shell functions

### The verified spine

- **[VERIFIED: ran it]** `$(cmd)` runs `cmd` in a **subshell** — its `cd` and `export` do NOT affect the parent (`X=$(cd /etc; export SUBVAR=x)` left parent `PWD` unchanged and `SUBVAR=UNSET`). But `eval "cd /etc; export SUBVAR=x"` **does** change the current shell (`PWD=/etc`, `SUBVAR=x`) (E2). **This is the entire reason the verbs are shell functions, not the bare binary.**
- **[VERIFIED: ran it]** `eval "$(binary)"` applies emitted code into the current shell: a fake binary printing `alias gs=…; export EMITTED_VAR=applied`, `eval`'d, made both the alias and the var live in the parent (E4).
- **[VERIFIED: ran it]** Full **A→B→deactivate zero-residue** through the eval-of-emitted-code path with the loader helpers + base guard: PRE `path=2 EDITOR=vim gs=none` → A `path=3 EDITOR=nvim gs=git status` → B `path=3 EDITOR=emacs gs=none` → POST `path=2 EDITOR=vim gs=none` == PRE (E15). The `$#path` does not grow across switches.

### Candidate approaches — verb / binary split

| # | Approach | Correctness | Fail-open | Startup | Verdict |
|---|----------|-------------|-----------|---------|---------|
| B1 | **Mutating verbs = sourced functions that `eval "$(zsh-pro emit …)"`; `list`/`status` = thin binary/store wrappers** (D-05 default) | HIGH — only path that mutates the live shell | verb-local `zsh -n` guard (d) | subprocess only on explicit verb | **RECOMMENDED** |
| B2 | All verbs as bare binary subcommands | **BROKEN** — a child cannot mutate its parent (E2) | n/a | n/a | **REJECTED** by shell semantics |
| B3 | Verbs `source` a per-switch tempfile the binary writes (instead of `eval "$(…)"`) | HIGH; identical effect (E2 shows `source`≡current shell) | same | one extra tempfile write per switch | Viable alt; `eval "$(…)"` is simpler and avoids tempfile cleanup. Tempfile is only preferable if the `zsh -n` harness wants a file operand (it accepts here-strings too — E3b). |

**Recommended default:** B1. `checkout`/`activate`/`deactivate` `eval` emitted code; `list`/`status` print binary/store output directly (never `eval`). The shell function (not the binary) exports `ZSHPRO_PROFILE` after a successful eval (D-07); `deactivate` unsets it.

### Emit subcommand shape (OQ-05-06)

| # | Approach | Verdict |
|---|----------|---------|
| C1 | **`zsh-pro emit <apply\|deactivate> <name>` (one internal subcommand, mode arg)** | **RECOMMENDED** (D-06 default) — smallest surface |
| C2 | Two subcommands `emit-apply` / `emit-deactivate` | more explicit, more surface — no benefit |
| C3 | Fold both into `checkout` printing a combined block | muddles validate-vs-emit split |

`checkout` = validate (`Store.Checkout`, which validates existence but does NOT export — Phase 3) → deactivate-prior + activate-target (Phase 4 plan ordering) → export `ZSHPRO_PROFILE`. `activate` shares the same eval-of-emitted-code path (first switch / explicit re-apply). Low confidence on the exact name only; correctness-neutral (OQ-05-06).

---

## Sub-area (c): Idempotent BEGIN/END `.zshrc` block installer

### The verified spine (Go POC ran)

A ~40-line Go find-region-then-replace POC (`installer.go`, scratchpad) was run against three scenarios:

- **[VERIFIED: ran it]** **Byte-identical on re-run:** install into a `.zshrc` with pre-existing user content, then install again → `diff` of the two results is **empty**; `grep -c` of the BEGIN marker == **1** (E-INSTALL scenario 1). User content (`export MY_VAR=1`, `alias myls=…`) survives.
- **[VERIFIED: ran it]** **Dup collapse (safe-repair):** a corrupted `.zshrc` with two managed blocks and user content between them → after install, exactly **1** BEGIN marker; the between-blocks user content is preserved (scenario 2). This matches D-10 "collapse to a single canonical block."
- **[VERIFIED: ran it]** **Missing file:** absent `.zshrc` → created containing just the block, 1 BEGIN marker (scenario 3).

### The algorithm (proven correct — recommend as-is)

```
begin = "# >>> zsh-pro >>>";  end = "# <<< zsh-pro <<<"
render() -> deterministic canonical block (begin..end inclusive)   # determinism => byte-identical re-run
if !contains(cur, begin):
    if cur == "":            return block + "\n"                    # create
    ensure cur ends with "\n"; return cur + "\n" + block + "\n"     # append at EOF
else:
    walk each [begin..end] region:
        emit bytes before the first region verbatim
        emit the fresh block ONCE at the first region's position
        SKIP (drop) every subsequent region (dup collapse)
        preserve all bytes between/after regions verbatim
```

Atomicity: write to a temp file in the same dir + `os.Rename` (atomic on the same filesystem) is the safe replacement pattern (standard Go idiom; not separately benchmarked — `[ASSERTED]`, settled by any crash-during-write test). D-11: the block is a THIN STUB, not the loader — it guards and sources the cached loader (see sub-area d).

### Candidate approaches — region matching

| # | Approach | Correctness | Verdict |
|---|----------|-------------|---------|
| D1 | **Plain `strings.Index` begin→end scan, drop dupes** (POC) | HIGH — verified byte-identical + dup-collapse + preservation | **RECOMMENDED** — no regex, no deps |
| D2 | Regex `(?s)BEGIN.*?END` replace | HIGH but adds regex subtlety (`.` vs newline flags); no benefit over D1 | acceptable, unnecessary |
| D3 | Line-oriented scan (split, filter, rejoin) | risks trailing-newline drift → breaks byte-identical | avoid |

**Recommended default:** D1 exactly as the POC. Everything outside `[begin, end]` preserved byte-for-byte; determinism of `render()` guarantees the second run is byte-identical.

---

## Sub-area (d): Fail-open + fast

### The verified spine

- **[VERIFIED: ran it]** `ZSHPRO_DISABLE=1` early-return no-op: a stub whose first line is `[[ -n "$ZSHPRO_DISABLE" ]] && return 0` defines **no** verbs when disabled (`whence -w activate` → not defined), and defines them when unset (E10). Full no-op confirmed.
- **[VERIFIED: ran it]** Absent-binary guard: `command -v definitely_not_a_binary >/dev/null` is a clean no-op (E11). Unreadable cached loader: `[[ -r /no/such/loader ]] && source …` is skipped, and the shell reaches the prompt-equivalent line (E11). No path emits a shell-aborting non-zero exit.
- **[VERIFIED: ran it]** `zsh -n` rejects a syntax error **without executing**: `zsh -n bad.zsh` → exit 1 and the `print SHOULD_NOT_RUN` in the bad file never fired (E3). Works on a file operand AND a here-string (`zsh -n <<< "$code"` → 1 on bad, 0 on good — E3b), so the verb can validate a captured string in-place.

### Guard order (D-14, verified shape)

```zsh
# >>> zsh-pro >>>
[[ -n "$ZSHPRO_DISABLE" ]] && return 0                          # 1. full no-op escape hatch
[[ -r "$HOME/.zsh-pro/loader.zsh" ]] && source "$HOME/.zsh-pro/loader.zsh"   # 2. readability guard, then source cached loader
# <<< zsh-pro <<<
```

### Candidate approaches — where `zsh -n` runs (OQ-05-07)

| # | Approach | Correctness | Fail-open | Verdict |
|---|----------|-------------|-----------|---------|
| E1 | **Sourced verb runs `zsh -n <<< "$emitted"` on the captured string, then `eval` only if exit 0** (D-15 default) | HIGH — check is adjacent to the `eval` boundary | subprocess only on explicit verb (off hot path) | **RECOMMENDED** |
| E2 | Binary self-validates before printing | moves check away from eval boundary; still trusts transport | same | weaker guard site |
| E3 | Both (defense in depth) | strongest | redundant for v2.0 | acceptable but over-built |

### Last-good fallback (D-16 / OQ-05-08)

| # | Payload | Correctness | Verdict |
|---|---------|-------------|---------|
| F1 | **Profile NAME only (`ZP_LAST_GOOD_PROFILE`)** | Sufficient — atomic up-front validation (D-16) means a failed switch is rejected BEFORE any `eval`, so the live shell already holds the prior good declarative state; the name is a report/recovery aid, not a re-apply payload | **RECOMMENDED** |
| F2 | Name + cached emitted code per terminal | faster recovery, but stale-cache + per-terminal storage complexity | over-engineered |
| F3 | Name + manifest hash (drift detection) | over-engineered for v2.0 | no |

**Atomicity rule (D-16):** Because Phase 4's plan is deactivate-then-activate, validate the **ENTIRE** emitted apply+deactivate block up front and refuse to `eval` any of it on failure — never eval the deactivate half then discover the activate half is broken. `zsh -n` on the whole captured block satisfies this (E3/E3b).

---

## Sub-area (e): Perf verification — zero-subprocess + startup budget

### The verified spine

- **[VERIFIED: ran it]** **Zero-subprocess structural grep on the stub start-path** (E13): the installed stub contains `$(` × 0, backtick × 0, `git ` × 0, and no bare `zsh-pro` invocation as a command (the string `zsh-pro` appears only inside the marker comment and the cached-loader path, not in command position). The load-bearing grep is: assert the start path (the stub + the cached loader, minus verb function *bodies*) contains no `$(`, no backtick, no `git`, no `zsh-pro` command. Verb bodies legitimately contain `$(zsh-pro emit …)` — those run only on explicit invocation, not on source.
- **[VERIFIED: ran it]** **Startup cost** (E14, in-process `EPOCHREALTIME` proxy over N=300; `hyperfine` ABSENT — a dev/CI tool, not a Go dep):
  - baseline (source empty file): **0.042 ms/iter**
  - source the pure-function-def loader: **0.069 ms/iter**
  - **ADDED cost ≈ 0.026 ms/shell-start** — ~380× under the < 10 ms budget (OQ-05-04). Pure function/const definitions are effectively free.

### `hyperfine` methodology (the real CI gate — tool is a dev dep, install separately)

```bash
# hyperfine is NOT a Go module. Install as a dev/CI tool only (e.g. brew install hyperfine).
# Compare interactive startup with the managed block installed vs not:
hyperfine --warmup 3 \
  'ZDOTDIR=<fixture-with-block>    zsh -i -c exit' \
  'ZDOTDIR=<fixture-without-block> zsh -i -c exit'
# PASS: added mean < 10 ms (OQ-05-04). Structural grep (E13) is the primary guard; ms budget is the backstop.
```

### Candidate approaches — hot-path definition source (D-13)

| # | Approach | Zero-subprocess? | Startup | Verdict |
|---|----------|------------------|---------|---------|
| G1 | **Stub sources a CACHED loader file** written at `install` time; `[[ -r <cached> ]] && source <cached>` | YES — no `$(...)`, no binary call on start (E13) | ~0.03 ms added (E14) | **RECOMMENDED** (D-13) |
| G2 | Stub runs `eval "$(zsh-pro hook)"` on every start | **NO** — a subprocess (the binary) on the hot path | binary spawn per shell start (10s+ ms) | **REJECTED** by Req 7 |
| G3 | Stub inlines the entire loader body | YES zero-subprocess | slightly larger source, but marker/body must change per release → orphans old blocks (D-11) | avoid — keep the block thin |

**Recommended default:** G1. Cached loader lives under the store/data dir (e.g. `$HOME/.zsh-pro/loader.zsh`); `install` (and any future self-heal) regenerates it from `zsh-pro hook`. Grep-verify the start path; `hyperfine` is the backstop.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Unset-vs-empty env detection | A `[[ -z ]]` / `-v` check | `[[ "${(P)+var}" == "1" ]]` where the operand holds the NAME | `-z` conflates unset with empty; verified E5 the `(P)+` form distinguishes all three states |
| Shadow-restore of an empty prior | `[[ -n "$slot" ]] && restore` | `[[ -n "${slot+x}" ]] && restore` (set-test) | The `-n` form silently DROPS a valid empty prior — verified WRONG in E6 |
| PATH switch | blind `path=(new $path)` per switch, or `typeset -U` | rebuild from `$ZP_BASE_PATH` every apply | blind append grows `$#path` (E7: 2→4 residue); rebuild is zero-residue (E7/E15) |
| Slot-name construction | interpolate the raw profile/var name | sanitize `${name//[^A-Za-z0-9_]/_}` first | raw `/` fails `typeset`; raw `$(...)` EXECUTES (E16) |
| `.zshrc` idempotent edit | append-and-hope / sed-in-place | Go find-region-then-replace + atomic rename (POC) | verified byte-identical + dup-collapse + preservation (E-INSTALL) |
| Live-shell mutation | a binary that "sets" env | sourced function that `eval`s emitted code | a child cannot mutate its parent (E2) |
| Syntax-check before eval | trust the binary's output | `zsh -n <<< "$code"` in the verb | rejects bad code without executing (E3/E3b), adjacent to the eval boundary |
| Options that must persist | `emulate -L` / `LOCAL_OPTIONS` in apply | PLAIN functions | those auto-revert `setopt` at function return (Phase 1 Pitfall 1; D-04) |

**Key insight:** Every one of these has a "looks right, silently wrong" trap that the Phase 1 raw snippet or a naive first implementation falls into — the CONTEXT decisions already encode the corrections, and the experiments here confirm each correction is load-bearing.

## Common Pitfalls

### Pitfall 1: `eval "$(zsh-pro hook)"` on the hot path
**What goes wrong:** Sourcing the loader by shelling out to the binary on every shell start spawns a subprocess, blowing Req 7 (and the < 10 ms budget by ~1000×). **Avoid:** source a cached loader file (D-13/G1). **Warning sign:** the stub's start path contains `$(` or `zsh-pro` in command position (grep-verify — E13).

### Pitfall 2: Re-capturing `ZP_BASE_PATH` per switch
**What goes wrong:** folds a prior profile's additions into the "base," so PATH grows monotonically. **Avoid:** the once-only guard (D-08, verified E8), placed in the sourced activate path before any PATH mutation. **Warning sign:** `$#path` grows across `activate A → activate B` (the Phase 4 property test catches it).

### Pitfall 3: Shadow-restore with `[[ -n "$slot" ]]`
**What goes wrong:** a profile that shadowed an alias whose prior body was `""` silently loses the restore → residue. **Avoid:** the `${slot+x}` set-test (verified E6). This is the exact OQ-8 correction to the Phase 1 raw snippet.

### Pitfall 4: `emulate -L` / `LOCAL_OPTIONS` in the emitted apply/helpers
**What goes wrong:** `setopt`/`unsetopt` auto-revert at function return, so option changes never take effect in the terminal. **Avoid:** PLAIN functions (D-04). Deliberate opposite of `introspectScript`.

### Pitfall 5: Half-applied shell on a failing switch
**What goes wrong:** eval the deactivate half, then the activate half fails `zsh -n` → shell left in a broken intermediate state. **Avoid:** validate the ENTIRE emitted block up front, refuse to eval any of it on failure (D-16 atomicity, verified E3).

### Pitfall 6: A marker-text or stub-body change between releases orphans installed blocks
**What goes wrong:** a `.zshrc` block from v2.0 no longer matches a v2.1 marker → duplicate blocks / dead stub. **Avoid:** keep the block THIN and the markers STABLE (D-11); the cached-loader indirection means loader-body changes don't touch the installed block. **This is a deferred migration concern** — noted for later, not built in Phase 5.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| zsh | loader, `zsh -n` validation, live-terminal tests | ✓ | 5.9 (arm64-apple-darwin25.0) | tests skip via `LookPath` guard when absent (existing precedent) |
| go | build, installer, all code | ✓ | 1.25.7 | — |
| git | `core/store` (list/checkout data) | ✓ (per Phase 3 — `ErrGitAbsent` degrades) | — | store degrades; not on hot path |
| hyperfine | startup-budget CI gate (Req 7 backstop) | ✗ | — | **dev/CI tool, NOT a Go dep.** Structural zero-subprocess grep (E13) is the load-bearing guard; `hyperfine` install (`brew install hyperfine`) is a CI-machine concern, and an `EPOCHREALTIME` in-process proxy (E14) works without it |

**Missing dependencies with fallback:** `hyperfine` — the phase can ship and verify Req 7 via the structural grep + the `EPOCHREALTIME` proxy; the `hyperfine` number is the backstop, gated on the tool being present in CI.
**Missing dependencies with no fallback:** none — nothing blocks execution.

## Validation Architecture

*(nyquist_validation not explicitly false in `.planning/config.json` → included. Go stdlib `testing`.)*

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (no test framework dep) |
| Config file | none — `go test ./...`; `make check` = fmt-check + vet + lint + test |
| Quick run command | `go test ./core/cli/... ./core/shell/zsh/...` |
| Full suite command | `make check` |

### Phase Requirements → Test Map
| Req | Behavior | Test Type | Automated Command | Exists? |
|-----|----------|-----------|-------------------|---------|
| BOOT-01 | installer byte-identical on re-run; dup-collapse; preserve outside markers; create on missing | unit (Go, no zsh) | `go test ./core/cli/... -run Install` | ❌ Wave 0 |
| BOOT-01 | `grep -c` BEGIN == 1 after 2 installs | unit | (asserted in the install test) | ❌ Wave 0 |
| BOOT-02 | `zsh-pro hook \| zsh -n` == 0; defines 5 verbs + `zp_*` set | integration (`LookPath`-guarded zsh) | `go test ./core/shell/zsh/... -run Hook` | ❌ Wave 0 |
| BOOT-02 | live-terminal `activate`/`deactivate` zero-residue in the CURRENT shell; `$#path` stable | integration (zsh, mirrors Phase 4 property test) | `go test ./core/shell/zsh/... -run LiveTerminal` | ❌ Wave 0 |
| BOOT-02 | `ZSHPRO_DISABLE=1` defines no verbs; absent binary reaches prompt | integration (zsh) | `go test -run FailOpen` | ❌ Wave 0 |
| BOOT-02 | emitted code failing `zsh -n` is NOT eval'd; last-good intact | integration (zsh) | `go test -run LastGood` | ❌ Wave 0 |
| BOOT-02 | start path has no `$(`, `git`, `zsh-pro`-command (structural grep) | unit (string assert on `HookScript()`) | `go test ./core/shell/zsh/... -run ZeroSubprocess` | ❌ Wave 0 |
| BOOT-02 | added startup cost within budget | manual/CI (`hyperfine`) | `hyperfine …` (dev tool) | manual — CI gate |

### Sampling Rate
- **Per task commit:** `go test ./core/cli/... ./core/shell/zsh/...`
- **Per wave merge:** `make check`
- **Phase gate:** `make check` green + `hyperfine` budget confirmed (where the tool is present) before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `core/cli/install_test.go` — idempotency (byte-identical, dup-collapse, preservation, create) — covers BOOT-01. **No zsh needed** — pure Go string/file test (POC proves the shape).
- [ ] `core/shell/zsh/hook_test.go` — `hook | zsh -n` == 0, grep the 5 verbs + `zp_*` set, structural zero-subprocess grep on the start path — covers BOOT-02.
- [ ] `core/shell/zsh/live_terminal_test.go` — `LookPath`-guarded zsh test (follows `introspect_test.go`/Phase 4 property-test precedent): source loader → `activate A` → assert live alias/env/PATH → `deactivate` → assert byte-identical snapshot + `$#path` stable; fail-open + last-good sub-cases. Covers BOOT-02.
- [ ] Test helper: `LookPath("zsh")` skip-guard (existing pattern in `introspect_test.go`).
- [ ] Framework install: none — Go stdlib `testing` already in use.

## Security Domain

*(security_enforcement not disabled in config → included.)*

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation / Output Encoding | **yes** | Slot-name sanitization `${name//[^A-Za-z0-9_]/_}` (D-02, verified E16); Phase 4 owns value-quoting (`'\''` escape) — Phase 5 consumes emitted code and validates it with `zsh -n` before eval |
| V6 Cryptography | no (Phase 5) | Secret material stays in the Phase 3 keychain/vault seam; Phase 5 only drives `store.KeychainDriver` if emitted code carries an unresolved `SecretRef` (OQ-05-02) — end-to-end PROF-03 is Phase 6 |
| V10 Malicious Code / Code Injection | **yes** | `eval` of emitted code is the trust boundary; `zsh -n` gate (E3) + slot-name sanitize (E16) + Phase 4's injection-safe emission (T-01-06) |
| V12 Files & Resources | **yes** | `.zshrc` rewrite: atomic temp-file + rename; never touch bytes outside markers (E-INSTALL) |

### Known Threat Patterns for zsh loader / codegen

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Command substitution in a derived slot/var name executes at name-build time | Elevation of Privilege / Tampering | Sanitize slot names to `[A-Za-z0-9_]` BEFORE `typeset`/`(P)` (verified E16 — `$(…)` in a name ran `PWNED`) |
| Malformed emitted code `eval`'d into the live shell | Tampering | `zsh -n` the ENTIRE block before eval; atomic refuse-all-on-failure (D-16, verified E3) |
| `.zshrc` corruption / clobbering user content | Tampering / DoS | find-region-then-replace preserves outside-marker bytes byte-for-byte; atomic rename (E-INSTALL) |
| A broken/absent binary aborting `.zshrc` (lockout) | Denial of Service | `command -v` + `[[ -r ]]` guards; `ZSHPRO_DISABLE=1` full no-op; no non-zero exit on the stub path (verified E10/E11) — fail-open is absolute |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The loader's `zp_*` helper signatures will match `emit.go`'s literal bare calls once Phase 4 lands | (a) helper contract / OQ-05-05 | Activation breaks until reconciled; **Medium** but localized (both in `core/shell/zsh`) — plan-time diff gate resolves it. HIGH confidence on the contract *shape* (Phase 1 + Phase 4 D-12/OQ-8). |
| A2 | Atomic `.zshrc` replacement via same-dir temp + `os.Rename` is crash-safe | (c) | A crash mid-write could truncate `.zshrc`; **Low** — standard Go idiom; settled by a crash-injection test. Not separately run. |
| A3 | Phase 4's emitted code does not surface an unresolved `SecretRef` at the apply boundary (may be resolved earlier in the builder) | OQ-05-02 | If it does, Phase 5 must drive `store.KeychainDriver` at apply time; **Medium** — confirm against `emit.go` at plan time; end-to-end PROF-03 is Phase 6 regardless. |
| A4 | The < 10 ms `hyperfine` budget is comfortably met (proxy shows ~0.03 ms) | (e) / OQ-05-04 | Real `hyperfine` on a slow CI box could differ; **Low** — proxy is 380× under budget; structural grep is the true guard. Confirm with real `hyperfine` when the tool is present. |

## Open Questions (RESOLVED)

`05-OPEN-QUESTIONS.md` records OQ-05-01..18 with `open_count: 0`. Interface-dependent items are explicit execution-time gates: Task 1 of 05-01 must reconcile the helper/state and CLI-to-emit surfaces against the completed Phase 4 output before phase exit, and the optional `hyperfine` result runs only where the executable is present while the structural start-path gate always runs. Sol review findings are adopted by 05-03 through 05-05: safe installer targeting/cache promotion, portable runtime deadlines and ERR_EXIT/ERR_RETURN-safe reporting, complete active-profile transitions, SecretRef resolution, and typed-nil normalization. No unresolved question remains.

## Sources

### Primary (HIGH — executed on this machine)
- Local experiments E1–E16 + the Go installer POC (scratchpad; zsh 5.9, go 1.25.7) — every load-bearing claim above.
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md` §Loader Reference Snippet — the `zp_*` helper reference.
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md` — the validated manifest shape (Phase 4 input, Phase 5 consumes emitted code from it).
- `.planning/phases/04-manifest-builder-emit/04-CONTEXT.md` D-10/D-12/D-13 (OQ-5/OQ-8) — the emit/`zp_*` bare-call seam.
- `core/shell/zsh/introspect.go` — the `const` embed + `zsh -f -c` + `LookPath`-guard precedent.
- `core/cli/cli.go` — `Run` switch dispatch, `model.ExitCode` contract, `fail` helper.
- Phase 5 SPEC / CONTEXT / OQ-05-01..10 — locked requirements and decisions.

### Secondary (MEDIUM)
- zsh 5.9 parameter-expansion semantics (`${(P)+var}`, `${name+x}`, `typeset -g`, `functions[]`/`aliases[]`) — verified empirically (E5/E6/E9) rather than only cited.

### Tertiary (LOW)
- `hyperfine` methodology — tool ABSENT locally; methodology stated, not executed here.

## Metadata

**Confidence breakdown:**
- Loader / `zp_*` contract: HIGH (behavior verified E5/E6/E9/E15) — pending the OQ-05-05 signature-diff gate against real `emit.go`.
- Verb structure / eval-of-binary: HIGH (E1/E2/E4/E15).
- Idempotent installer: HIGH (Go POC ran, all 3 scenarios).
- Fail-open + `zsh -n` + last-good: HIGH (E3/E10/E11).
- Zero-subprocess + startup budget: HIGH structural (E13) / MEDIUM absolute-ms (proxy, not real `hyperfine`).

**Research date:** 2026-07-02
**Valid until:** ~2026-08-01 (stable — zsh semantics and the codebase seams are settled; the only moving piece is the pending Phase 4 `emit.go` call surface, gated by OQ-05-05).

## RESEARCH COMPLETE

Phase 5's design is fully de-risked: every load-bearing zsh semantic (eval-persistence, subshell isolation, `${(P)+var}` unset-vs-empty, `${slot+x}` shadow set-test, PATH rebuild-from-base zero-residue, once-captured `ZP_BASE_PATH`, slot-name sanitize-or-inject, `ZSHPRO_DISABLE`/`command -v`/`[[ -r ]]` fail-open, `zsh -n` rejection without execution) plus the Go idempotent-installer algorithm and the ~0.03 ms zero-subprocess startup cost were **executed and confirmed** on zsh 5.9 / go 1.25.7. The remaining work is fully planned and gated: the Task-1 Phase 4 surface reconciliation is blocking, the structural start-path check is mandatory in every environment, and the optional `hyperfine` timing backstop is a CI/tool-available check. Sol review remediations are covered by 05-03 through 05-05; none is an unresolved planning question.
