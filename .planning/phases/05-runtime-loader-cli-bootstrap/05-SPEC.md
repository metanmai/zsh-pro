# Phase 5: Runtime Loader + CLI + Bootstrap — Specification

**Created:** 2026-07-02
**Ambiguity score:** 0.14 (gate: ≤ 0.20)
**Requirements:** 7 locked
**Mode:** `--auto` (interview skipped — initial ambiguity already below gate; derived from ROADMAP 4 success criteria + REQUIREMENTS BOOT-01/BOOT-02 + Phase 4 seam decisions OQ-3/OQ-5/OQ-18 + the `introspectScript` embed pattern)

## Goal

A sourced zsh loader emitted from a `hook` subcommand wires the live terminal to the `zsh-pro` binary: `checkout`/`activate`/`deactivate` `eval` the binary's Phase-4-emitted apply/deactivate code and visibly change the **current** terminal (not just new shells) while `list`/`status` report branches and the active profile; an idempotent BEGIN/END `.zshrc` block bootstraps the loader while preserving the unmanaged master block; and the loader is fail-open and fast — a broken/missing/slow `zsh-pro` never locks the user out, and shell start adds only a small, file-sourced cost with **zero subprocesses** on the hot path.

## Background

Phase 4 (Manifest Builder + Emit) produces the reversible record and the zsh apply/deactivate code: `core/activate` builds a `model.Manifest` and a shell-agnostic `Plan`, and `core/shell/zsh/emit.go` renders that plan to zsh code. **Reconciliation gate (not a settled fact):** Phase 4's emit-vs-inline shape is **Claude's discretion (Phase 4 OQ-5/OQ-18)** and `emit.go`/`core/activate` do **not yet exist on disk** — so there is no literal bare-call surface to match against yet. Phase 4 commits **by name only** to the two env helpers `zp_capture_env`/`zp_restore_env`; the shadow, PATH-rebuild, and option reverse ops are documented **INLINE** (shadows: `alias name=$prior`/`functions[name]=$prior` guarded by `${+name}`; PATH: `PATH="$ZP_BASE_PATH"; path=(<additions> $path)`; options: `SetOption`/`RestoreOption` with an inline `[[ -o opt ]]`→`was_on` capture), NOT a `zp_rebuild_path`/`zp_shadow_*` helper set. Phase 4 explicitly deferred to Phase 5: (a) the sourced runtime loader itself, (b) the five CLI verbs, (c) per-terminal active-state wiring, (d) the `.zshrc` bootstrap block, and (e) **the base-capture placement** — which loader entry point captures `ZP_BASE_PATH` and the re-capture guard (Phase 4 OQ-3, "finalized in Phase 5's loader"). Phase 5 therefore **definitively provides** the two named env helpers (`zp_capture_env`, `zp_restore_env`) AND the per-terminal runtime **STATE** the Phase-4 inline reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, per-option `was_on` slots); whether PATH-rebuild/shadow logic is a further `zp_*` helper or emitted inline is reconciled against `emit.go`'s ACTUAL call surface once Phase 4 lands (plan-time gate, OQ-05-05/OQ-05-13). Phase 5 captures `ZP_BASE_PATH` **once at first activate** into per-terminal runtime state (Phase 4 OQ-3 mechanism locked; placement lands here).

Phase 3 (Git-Backed Store) built `core/store` with the verbs' data source: `Store.Branches` (list), `Store.Current` (reads `ZSHPRO_PROFILE`, unset ⇒ `main`), `Store.Checkout` (validates existence but does NOT export — "the env-var export on switch is Phase 5 the sourced loader"), `Store.Create`, `Store.Read` (returns a `model.Profile`), and `Store.Commit`. Phase 3 D-13 pins the active profile as **per-terminal** via `ZSHPRO_PROFILE` (name only) — never a shared global file — so the loader exports it per-terminal.

What exists today vs the Phase 5 delta:
- **CLI:** `core/cli/cli.go` has exactly one verb (`analyze`) dispatched from `Run`'s `switch args[0]`; `Version`/unknown-command/usage are handled. No `hook`/`checkout`/`activate`/`deactivate`/`list`/`status` verb exists.
- **Loader:** No embedded loader script exists. The embed pattern to mirror is `introspectScript` — a `const` raw-string zsh script in `core/shell/zsh/introspect.go` run via `exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)`. Phase 5's loader is *emitted for sourcing* (printed to stdout by `hook`), not run by the binary.
- **Composition root:** `core/cmd/zsh-pro/main.go` already constructs the store (`store.New(dir, provider, kc)`) and holds it as `_, _ = s, err` with the comment "The CLI verbs that consume the store (checkout/create/list/status) arrive in Phase 5 … non-fatal here … surfaces when a store-backed verb is wired in Phase 5." Phase 5 wires the store into the CLI.
- **Bootstrap:** No `.zshrc` installer exists. No BEGIN/END managed block, no idempotency logic, no master-block preservation.
- **Fail-open/fast:** No `command -v`/`[[ -r ]]` guards, no `zsh -n` manifest validation, no last-good fallback, no `ZSHPRO_DISABLE` escape hatch, no `hyperfine` perf budget.

## Requirements

1. **`hook` subcommand emits the embedded loader**: `zsh-pro hook` prints the sourced loader script to stdout.
   - Current: No `hook` verb; the only embedded zsh script is `introspectScript` (run internally, never emitted).
   - Target: `zsh-pro hook` (mirroring the `introspectScript` `const` embed pattern in `core/shell/zsh`) writes a complete zsh loader to stdout for `eval "$(zsh-pro hook)"`. The loader definitively provides the two named env helpers the Phase-4-emitted code calls (`zp_capture_env`, `zp_restore_env`) plus the per-terminal runtime STATE the Phase-4 inline reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, per-option `was_on` slots), and the shell functions `checkout`/`activate`/`deactivate`/`list`/`status`. **Reconciliation gate (OQ-05-05/OQ-05-13):** Phase 4's emit-vs-inline shape is Claude's discretion (Phase 4 OQ-5) and `emit.go` is not yet on disk; PATH-rebuild and shadow-capture/restore may be emitted INLINE by `emit.go` (reading the runtime STATE above) rather than as `zp_rebuild_path`/`zp_shadow_*` helper calls, so the loader must NOT assume those helpers are in the contract — the plan's first task diffs the loader's provided helpers/state against `emit.go`'s ACTUAL bare-call surface once Phase 4 lands and provides exactly that. The emitted script is `zsh -n`-parseable.
   - Acceptance: `zsh-pro hook | zsh -n` exits 0 (syntax-valid); the output defines all five verb functions, the two named env helpers (`zp_capture_env`/`zp_restore_env`), and the runtime state the emitted reverse ops read (grep-verified); the loader script lives as an embedded constant in `core/shell/zsh` (grep-verified — not assembled in `core/cli`).

2. **Live-terminal switch via sourced verbs**: `checkout`/`activate`/`deactivate` `eval` the binary's emitted code and change the **current** terminal.
   - Current: `Store.Checkout` validates a branch but does not export `ZSHPRO_PROFILE` or apply anything; nothing mutates a live shell.
   - Target: The sourced `checkout`/`activate` functions call the binary to obtain the Phase-4-emitted apply code and `eval` it in the current shell (a child process cannot mutate its parent, so the verbs are **shell functions**, not the bare binary), applying declarative state (env/aliases/functions/PATH/options) live; they export the per-terminal `ZSHPRO_PROFILE` (Phase 3 D-13). `deactivate` `eval`s the emitted deactivate code and unsets/reverts the per-terminal active state. `ZP_BASE_PATH` is captured **once at first activate** into per-terminal runtime state and reused across switches (Phase 4 OQ-3 — placement finalized here), never re-captured per switch.
   - Acceptance: In an already-open shell, sourcing the loader then running `activate <A>` makes an alias/env/PATH entry from profile A present in the *current* shell; `deactivate` removes them with no residue; `$ZSHPRO_PROFILE` reflects the active profile after `checkout` and is unset/`main` after `deactivate`; repeated `activate A → activate B → deactivate` does not grow `$#path` (delegates to the Phase 4 zero-residue property; Phase 5 asserts the live-terminal wiring end-to-end).

3. **`list` and `status` report branches and active profile**: read-only reporting verbs.
   - Current: `Store.Branches` and `Store.Current` exist but no verb surfaces them.
   - Target: `list` prints the profiles (= git branches via `Store.Branches`); `status` prints the active per-terminal profile (`Store.Current`; unset ⇒ `main`) and whether a profile is currently activated in this terminal. Both are backed by store calls wired through the CLI at the composition root.
   - Acceptance: With ≥2 branches in the store, `list` prints all branch names; `status` prints the value of `ZSHPRO_PROFILE` (or `main` when unset) and does not error when no profile is active.

4. **Idempotent BEGIN/END `.zshrc` block installer**: re-running the installer never duplicates or drifts the managed block.
   - Current: No installer, no managed block, no idempotency.
   - Target: An install verb writes a single BEGIN/END-marked block into `~/.zshrc` (marker sentinels, e.g. `# >>> zsh-pro >>>` / `# <<< zsh-pro <<<`) that sources/`eval`s the loader. Re-running the installer replaces the existing marked block in place (or leaves it byte-identical if unchanged) and **never** appends a second block. Content outside the markers — the unmanaged master block for imperative run-once code — is preserved byte-for-byte.
   - Acceptance: Running the installer twice leaves the region between the BEGIN/END markers byte-identical after the second run (exactly one marked block; `grep -c` of the BEGIN marker returns 1); bytes outside the markers are unchanged (diff of the non-marker region is empty); on a `.zshrc` with pre-existing user content, that content survives the install.

5. **Fail-open guarded loader stub**: a broken, missing, or slow `zsh-pro` never locks the user out of a working shell.
   - Current: No guarded sourcing, no escape hatch.
   - Target: The `.zshrc` stub guards every activation with `command -v zsh-pro` and `[[ -r <path> ]]` (or equivalent existence/readability tests) so an absent or unreadable binary/loader is a silent no-op rather than an error that aborts `.zshrc`; `ZSHPRO_DISABLE=1` fully no-ops the loader (the stub returns early, defining no verbs and running no activation). A malformed loader/manifest never propagates a non-zero exit that would break shell startup.
   - Acceptance: With `zsh-pro` removed from `$PATH`, starting a shell that sources the stub still reaches an interactive prompt (no error, exit reaches prompt); with `ZSHPRO_DISABLE=1` set, sourcing the stub defines no `checkout`/`activate` functions and performs no activation; a deliberately corrupted loader input does not abort shell startup.

6. **`zsh -n`-validated manifests with last-good fallback**: generated activation code is validated before it is `eval`'d, and a bad manifest falls back to the last known-good.
   - Current: No validation, no fallback.
   - Target: Before the loader `eval`s emitted apply/deactivate code, that code is validated with `zsh -n` (syntax check, no execution); a manifest/emitted-code that fails validation is rejected and the loader falls back to the last-good activation state (or a no-op) rather than `eval`ing broken code into the live shell. A last-good record is retained per terminal so a failed switch does not leave the shell in a half-applied state.
   - Acceptance: An emitted-code fixture that fails `zsh -n` is not `eval`'d (the live shell's aliases/env are unchanged from before the attempted switch); after a successful switch, the last-good record reflects the applied profile; a subsequent failing switch leaves the prior good state intact.

7. **Zero-subprocess, in-budget hot path**: shell start adds only a small, file-sourced cost with no subprocess on the hot path.
   - Current: No loader, so no measured startup cost — and no budget defined.
   - Target: The shell-start hot path (sourcing the stub during `.zshrc`) runs **zero subprocesses** — no `git`, no `zsh-pro` binary invocation, no `$(...)` command substitution — when no explicit activation is requested; it only defines functions and reads per-terminal state from env vars already present. Subprocess work (calling the binary to emit code, calling `git` via the store) happens **only** on an explicit `checkout`/`activate`/`deactivate`/`list`/`status` invocation, never on plain shell start. The added startup cost is verified within a stated budget via `hyperfine 'zsh -i -c exit'`.
   - Acceptance: A test/inspection confirms the stub's shell-start path contains no `$(...)`, no `git`, and no `zsh-pro` invocation (grep-verified on the emitted stub's start path); `hyperfine 'zsh -i -c exit'` with the block installed vs. not installed shows the added mean startup cost is within the budget locked in discuss-phase (default target: **< 10 ms** added mean; see OQ-05-04). **The `hyperfine` millisecond budget is a CI-machine gate** (`hyperfine` is a dev/CI tool — install via `brew`, NOT a Go dep — and is absent on the local box, so the ms number could not be gated here). The load-bearing guarantee is the **zero-subprocess structural grep** (C14 PROVEN); an in-process `EPOCHREALTIME` proxy shows ~0.01 ms added (~900× under budget) as interim evidence pending the real `hyperfine` run in CI (OQ-05-14).

## Boundaries

**In scope:**
- A `hook` subcommand that prints an embedded zsh loader (mirroring the `introspectScript` `const` embed pattern in `core/shell/zsh`) to stdout for `eval "$(zsh-pro hook)"`.
- The loader's definition of the two named env helper bodies the Phase-4-emitted code calls (`zp_capture_env`/`zp_restore_env`) plus the per-terminal runtime STATE the Phase-4 inline reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, `was_on`) — the Phase 4 OQ-5 seam. Phase 4's emit-vs-inline shape is its discretion (OQ-5/OQ-18) and `emit.go` is not yet on disk; the loader provides exactly the helper/state surface `emit.go` actually calls, reconciled at plan time (OQ-05-05/OQ-05-13) — it does NOT assume a `zp_rebuild_path`/`zp_shadow_*` helper set Phase 4 may emit inline.
- `checkout`/`activate`/`deactivate`/`list`/`status` as sourced shell functions that wire to the binary + `core/store`, changing the **current** terminal live.
- Per-terminal active-state wiring: export `ZSHPRO_PROFILE` on switch; `ZP_BASE_PATH` captured once at first activate (Phase 4 OQ-3 placement).
- An idempotent BEGIN/END-marked `.zshrc` block installer that preserves the unmanaged master block.
- Fail-open guarded stub (`command -v`, `[[ -r ]]`, `ZSHPRO_DISABLE=1` no-op), `zsh -n`-validated manifests with a per-terminal last-good fallback.
- A zero-subprocess shell-start hot path with a `hyperfine`-verified startup budget.
- Composition-root wiring of the store into the new CLI verbs (`core/cmd/zsh-pro/main.go` — the sole `core/shell/zsh` + concrete-store importer).

**Out of scope:**
- The manifest/plan/emit codegen itself (`model.Manifest`, `core/activate`, `core/shell/zsh/emit.go`) — that is Phase 4; Phase 5 consumes its emitted code and calls its `zp_*` helper contract.
- Real `~/.zshrc` end-to-end ingest (parse → classify → partial-eval → regenerate → commit to baseline) and out-of-block installer-append detection/warning — that is Phase 6.
- Secret **deref-on-switch** end-to-end resolution (`SecretRef` → keychain/vault at apply time) — the runtime half of PROF-03; Phase 5 may drive the existing keychain seam if the emitted code requires it, but the end-to-end ingest-side completion is Phase 6.
- Managing keybindings (`bindkey`) and hooks (`precmd_functions`/`chpwd_functions`) — routed to the master block per Phase 4 OQ-1 (unmanaged; not switchable in v2.0).
- Auto-activate on `cd` (a `chpwd`/`precmd` per-prompt hook) — future AUTO-01; v2.0 is explicit-`checkout` only, and per-prompt hooks would violate the zero-subprocess hot-path constraint.
- Multi-active / concurrent managed profiles per terminal and shared-profile trust gates — single-active per terminal (Phase 3 D-13); SHARE-01 is a future milestone.
- Other shells (bash/fish) — the activation model is zsh-specific for the whole milestone.

## Constraints

- **No new dependencies** — the loader is embedded zsh (a Go `const`, mirroring `introspectScript`); verbs shell out to the already-present `zsh-pro` binary and `git` (via `core/store`), and `hyperfine`/`zsh -n` are dev/CI tools, not Go modules.
- **Embed pattern** — the loader lives as an embedded string constant in `core/shell/zsh` (the `introspectScript` precedent), NOT hand-assembled in `core/cli`; the CLI dispatches the `hook` verb which emits it. Only `core/shell/zsh` writes zsh syntax (the milestone invariant; Phase 5's loader helpers are zsh and belong there alongside `emit.go`).
- **Composition root** — the store is wired into the CLI only at `core/cmd/zsh-pro/main.go` (the sole importer of `core/shell/zsh` and the concrete store drivers); `core/cli` continues to depend on the `shell.Provider` interface and receives the store via injection, never importing the concrete provider/store drivers itself.
- **Per-terminal state, no shared file** — active profile is carried in `ZSHPRO_PROFILE` (Phase 3 D-13); runtime undo/base state (`ZP_BASE_PATH`, `__ZP_ORIG_*`, `ZP_*_PRIOR_*`) is per-terminal env/global state. No shared "current profile" file (avoids the conda concurrent-activation race).
- **`zp_*` helper contract is a plan-time reconciliation gate, not a settled "exact match" fact** — Phase 4 commits by name ONLY to `zp_capture_env`/`zp_restore_env`; its shadow/PATH/option reverse ops are documented INLINE (Phase 4 D-12), and `emit.go`/`core/activate` do not yet exist on disk, so there is no literal bare-call surface to match yet. The loader definitively provides the two named env helpers + the runtime STATE the inline reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, `was_on`); it does NOT assume `zp_rebuild_path`/`zp_shadow_*` are in the contract. The plan's first task diffs the loader's provided helpers/state against `emit.go`'s ACTUAL bare-call surface once Phase 4 lands and provides exactly that (OQ-05-05/OQ-05-13). The behavior all of these rely on reproduces the Phase 1 loader-reference WITH the OQ-8 corrections (`${+name}` set-tests for shadow restore, `${(P)+var}==1` for env unset-vs-empty), NOT the buggy `-n`/`${(P)+literalName}` guards from the raw Phase 1 snippet.
- **Fail-open is absolute** — no failure mode of `zsh-pro` (absent, unreadable, malformed loader, failing manifest, timeout) may abort `.zshrc` or block reaching an interactive prompt. Guards are `command -v` + `[[ -r ]]`; `ZSHPRO_DISABLE=1` is a full early-return no-op.
- **Zero subprocess on shell start** — the hot path defines functions and reads env only; NO `git`, NO binary call, NO `$(...)` unless an explicit verb is invoked. `hyperfine 'zsh -i -c exit'` verifies the budget.
- **Idempotency is byte-exact** — the BEGIN/END block is byte-identical across re-installs; content outside the markers is never touched.
- **Testing: TDD.** The idempotency, fail-open, and zero-subprocess properties are falsifiable and pinned by tests; emitted loader/manifest code is `zsh -n`-validated; `make check` (fmt-check + vet + lint + test) stays green. The Phase 4 zero-residue property test remains the SW-02 pin; Phase 5 adds the live-terminal wiring assertions.

## Acceptance Criteria

- [ ] `zsh-pro hook | zsh -n` exits 0; the output defines `checkout`/`activate`/`deactivate`/`list`/`status` and the `zp_*` helper set; the loader is an embedded constant in `core/shell/zsh` (not assembled in `core/cli`).
- [ ] In an already-open shell, sourcing the loader then `activate <A>` makes an alias/env/PATH entry from A present in the **current** shell; `deactivate` reverses it with no residue; `$ZSHPRO_PROFILE` tracks the active profile and is cleared/`main` after deactivate.
- [ ] `list` prints all store branches; `status` prints the active profile (`main` when `ZSHPRO_PROFILE` unset) without error when nothing is active.
- [ ] Running the installer twice leaves the BEGIN/END region byte-identical (exactly one marked block, `grep -c` BEGIN == 1) and leaves all bytes outside the markers unchanged; pre-existing user `.zshrc` content survives.
- [ ] With `zsh-pro` absent from `$PATH`, a shell sourcing the stub still reaches an interactive prompt (no abort); `ZSHPRO_DISABLE=1` makes the stub a full no-op (defines no verbs, performs no activation).
- [ ] Emitted apply/deactivate code that fails `zsh -n` is NOT `eval`'d (the live shell is unchanged), and a failing switch falls back to the last-good state rather than a half-applied shell.
- [ ] The shell-start hot path contains no `$(...)`, no `git`, and no `zsh-pro` invocation (grep-verified); `hyperfine 'zsh -i -c exit'` shows added mean startup cost within the budget locked in discuss-phase (default < 10 ms).
- [ ] `ZP_BASE_PATH` is captured once at first activate per terminal and reused across switches (not re-captured per switch); repeated `activate A → activate B → deactivate` does not grow `$#path`.

## Ambiguity Report

| Dimension          | Score | Min  | Status | Notes                                                                             |
|--------------------|-------|------|--------|-----------------------------------------------------------------------------------|
| Goal Clarity       | 0.90  | 0.75 | ✓      | 4 explicit ROADMAP success criteria; exact deliverables named (loader, 5 verbs, BEGIN/END block, fail-open) |
| Boundary Clarity   | 0.82  | 0.70 | ✓      | In/out explicit; helper-ownership + secret-deref split logged as OQ-05-01/02      |
| Constraint Clarity | 0.85  | 0.65 | ✓      | No new deps; embed pattern; zero-subprocess hot path; `hyperfine`/`zsh -n`; layering pinned |
| Acceptance Criteria| 0.85  | 0.70 | ✓      | Idempotency (byte-identical), fail-open, perf budget, live-terminal change — all falsifiable |
| **Ambiguity**      | 0.14  | ≤0.20| ✓      | Below gate; interview skipped under --auto (Step 3)                               |

Status: ✓ = met minimum, ⚠ = below minimum (planner treats as assumption)

**No dimension is below minimum.** Four sub-decisions were auto-resolved to safe/reversible defaults and logged in `05-OPEN-QUESTIONS.md` (OQ-05-01 `zp_*` helper ownership → loader owns bodies, matches Phase 4 OQ-5 bare-call seam; OQ-05-02 secret deref-on-switch → drive existing keychain seam if emitted code requires, end-to-end completion deferred to Phase 6; OQ-05-03 install verb naming/marker sentinels → `install`/`# >>> zsh-pro >>>`; OQ-05-04 hot-path startup budget → < 10 ms added mean, confirm empirically in discuss/plan). None block planning.

## Interview Log

| Round | Perspective    | Question summary                          | Decision locked                                                                     |
|-------|----------------|-------------------------------------------|-------------------------------------------------------------------------------------|
| —     | (auto-derived) | Initial ambiguity ≤ 0.20 → interview skipped per Step 3 | SPEC derived from ROADMAP 4 success criteria + BOOT-01/BOOT-02 + Phase 4 OQ-3/OQ-5/OQ-18 seam + `introspectScript` embed pattern |
| auto  | Researcher     | What exists vs the Phase-5 delta?         | CLI has only `analyze`; no embedded loader (only `introspectScript`); store built (Branches/Current/Checkout/Create/Read) but no verb; main.go holds store as `_,_=s,err` awaiting Phase 5; no installer/fail-open/perf |
| auto  | Simplifier     | Irreducible core?                         | 7 requirements: `hook` loader emit, live-terminal switch verbs, list/status, idempotent BEGIN/END installer, fail-open guarded stub, `zsh -n`+last-good, zero-subprocess in-budget hot path |
| auto  | Boundary Keeper| What is NOT this phase?                   | Manifest/emit codegen → Ph4 (consumed here); real ingest + out-of-block append warning → Ph6; keybindings/hooks → master block (Ph4 OQ-1); auto-cd + multi-active + other shells → future/out |
| auto  | Failure Analyst| What invalidates fail-open/idempotency?   | A duplicated/drifted `.zshrc` block; a broken loader aborting shell start; a subprocess on the hot path blowing the budget; `eval`ing `zsh -n`-invalid code into a live shell; re-capturing `ZP_BASE_PATH` per switch (PATH growth) — each pinned as a constraint/criterion |

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Spec created: 2026-07-02*
*Next step: /gsd:discuss-phase 5 — implementation decisions (loader script structure, `zp_*` helper bodies, verb function wiring, installer marker/rewrite strategy, `zsh -n` validation harness, last-good record format, hot-path perf budget confirmation)*
