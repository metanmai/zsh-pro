# Phase 5: Runtime Loader + CLI + Bootstrap - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning
**Mode:** Autonomous smart-discuss (`--auto`, unattended). Grey-area answers were auto-decided from prior-phase decisions > codebase patterns > domain conventions > ROADMAP criteria. No human was asked (`AskUserQuestion` suppressed). Medium/low-confidence calls are logged as OQ-05-05+ in `05-OPEN-QUESTIONS.md` (open_count bumped; OQ-05-01..04 preserved).

<domain>
## Phase Boundary

Wire the live terminal to the `zsh-pro` binary and make branch-switching visibly change the **current** shell with zero residue and fail-open safety. Five deliverables, all HOW-level (WHAT/WHY are locked in `05-SPEC.md` — 7 requirements — and inherited from Phase 4 OQ-3/OQ-5/OQ-18, Phase 3 D-13, and the Phase 1 loader reference snippet):

1. **`hook` subcommand + embedded loader** — a new CLI verb `zsh-pro hook` prints an embedded zsh loader (a `const` raw-string in `core/shell/zsh`, mirroring `introspectScript`) to stdout for `eval "$(zsh-pro hook)"`. The loader defines the `zp_*` runtime **helper bodies** the Phase-4-emitted apply/deactivate code calls, the `ZP_UNSET_SENTINEL`, and the five sourced shell functions `checkout`/`activate`/`deactivate`/`list`/`status`.
2. **Live-terminal switch verbs** — the sourced `checkout`/`activate`/`deactivate` shell functions call the binary to obtain the Phase-4-emitted code and `eval` it in the current shell (a child process cannot mutate its parent → these must be shell functions), exporting the per-terminal `ZSHPRO_PROFILE` (Phase 3 D-13). `ZP_BASE_PATH` is captured **once at first activate** per terminal and reused across switches (Phase 4 OQ-3 — placement finalized here).
3. **`list` / `status` reporting verbs** — read-only, backed by `Store.Branches` / `Store.Current`, wired through the CLI at the composition root.
4. **Idempotent BEGIN/END `.zshrc` installer** — an `install` verb writes a single marked block that sources the loader; re-running is byte-identical (never a second block); content outside the markers (the unmanaged master block) is preserved byte-for-byte.
5. **Fail-open + fast** — `command -v`/`[[ -r ]]` guards, `ZSHPRO_DISABLE=1` full no-op, `zsh -n`-validated emitted code with a per-terminal last-good fallback, and a zero-subprocess shell-start hot path verified within a `hyperfine` budget.

**In scope:** the `hook` verb + embedded loader const; the `zp_*` helper bodies (Phase 4 OQ-5 seam — Phase 4 emits bare calls, Phase 5 defines the bodies); the five sourced verbs; per-terminal state (`ZSHPRO_PROFILE`, `ZP_BASE_PATH`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, last-good slot); the idempotent installer + fail-open stub; the composition-root store→CLI wiring.

**Not in scope (later/other phases):** manifest/plan/emit codegen (`model.Manifest`, `core/activate`, `emit.go`) — Phase 4, consumed here; real `~/.zshrc` end-to-end ingest and out-of-block installer-append detection — Phase 6; secret **deref-on-switch** end-to-end (PROF-03) — Phase 6 (Phase 5 only drives the existing `store.KeychainDriver` seam if emitted code requires it, OQ-05-02); keybindings/hooks/`compinit` (master block, Phase 4 OQ-1); auto-activate on `cd` (future AUTO-01 — would violate the zero-subprocess hot path); multi-active/shared profiles (future); other shells (zsh-only milestone).

</domain>

<decisions>
## Implementation Decisions

### Area 1 — `zp_*` helper set + signatures (must match Phase 4's emitted calls)

- **D-01: The loader defines exactly the helper set Phase 4's `emit.go` emits bare calls to (Phase 4 OQ-5/OQ-18 seam, closed HERE).** The set, with the signatures locked by the Phase 1 reference snippet and Phase 4 D-12/OQ-8:
  - `zp_capture_env <var>` — captures the LIVE prior of `$var` into slot `__ZP_ORIG_<var>`, or `$ZP_UNSET_SENTINEL` when unset, using `[[ "${(P)+var}" == "1" ]]` (the only correct unset-vs-empty test; `(P)` dereferences the value-as-name — correct here because the operand holds the env NAME, C21). Written with `typeset -g`.
  - `zp_restore_env <var> <applied>` — drift-guarded reverse: reverses ONLY if `[[ "${(P)+var}" == "1" && "${(P)var}" == "$applied" ]]`; a `$ZP_UNSET_SENTINEL` prior → `unset "$var"`, else `export "$var"="$prior"` (restores an empty prior exactly).
  - PATH-rebuild-from-base — `PATH="$ZP_BASE_PATH"; path=(<additions> $path)` on apply; `PATH="$ZP_BASE_PATH"` (plus re-apply of any surviving additions) on deactivate. Never `typeset -U`, never blind append (Phase 1 Pitfall 2). Whether this is a `zp_rebuild_path`-style helper or inlined by `emit.go` is Phase 4's emitted-shape choice; the loader provides whatever helper Phase 4's calls name (confirm the exact call surface against `emit.go` at plan time — OQ-05-05).
  - shadow-capture / shadow-restore for aliases + functions — capture prior body into a per-profile slot `ZP_<profile>_PRIOR_ALIAS_<name>` / `ZP_<profile>_PRIOR_FUNC_<name>` (`"${aliases[name]}"` / `"${functions[name]}"`); restore guarded by a **`${+name}` set-test** (`[[ -n "${(k)ZP_..._slot+x}" ]]`-style presence, per OQ-8) — **NOT** the raw snippet's buggy `[[ -n "$slot" ]]` (which silently drops an empty prior). Restore via `alias name="$prior"` / `functions[name]="$prior"`; a function body is restored by verbatim reassignment (never single-quote-wrapped — C6).
- **D-02: Slot names are sanitized to `[A-Za-z0-9_]` (C24).** Undo/shadow slot names embed the profile name and the var/alias/function name; any char outside `[A-Za-z0-9_]` (e.g. `feature/x`, `a b`, `x$(...)`) makes an invalid `typeset -g` target (exit 1) AND is an injection vector. The loader's slot-name derivation replaces non-`[A-Za-z0-9_]` with `_`. (Collision `a/b`≡`a_b` is a known low-risk edge — Phase 4 OQ-10; not re-litigated here.)
- **D-03: `ZP_UNSET_SENTINEL` is a loader-defined constant** (a fixed improbable marker string, `typeset -g` once at loader source time), shared by capture + restore so the unset-vs-empty encoding round-trips.
- **D-04: `emulate -L`/`LOCAL_OPTIONS` are FORBIDDEN in the emitted apply/deactivate AND in the `zp_*` helpers that touch options** (Phase 1 Pitfall 1 / Phase 4 D-10). Emitted `setopt`/`unsetopt` must escape function scope, so the sourced verbs `eval` plain code. (This is the deliberate opposite of `introspectScript`, which correctly uses `emulate -L zsh` for isolation.)

### Area 2 — Verb structure: sourced functions vs binary subcommands

- **D-05: The verbs are sourced zsh SHELL FUNCTIONS defined by the loader; the mutation-bearing ones `eval` the binary's stdout.** `activate`/`checkout` run `eval "$(zsh-pro <emit-verb> <name>)"` to apply emitted code into the *current* shell (a child binary cannot mutate its parent). `deactivate` `eval`s the emitted deactivate code. `list`/`status` are thin wrappers over the binary's read-only output (they may print binary stdout directly; they never `eval`). This split — functions for live mutation, binary for codegen/data — is the load-bearing structure of Requirement 2.
- **D-06: `checkout` = validate-then-activate.** `checkout <name>` asks the binary to validate the target (`Store.Checkout`, which validates existence but does not export — Phase 3), then performs the deactivate-of-prior + activate-of-target (Phase 4's plan ordering), then exports `ZSHPRO_PROFILE=<name>`. `activate <name>` is the activate path (used for first switch / explicit re-apply); the two share the same underlying eval-of-emitted-code path. Exact naming of the binary emit subcommand(s) the functions call is Claude's discretion (OQ-05-06 — safe default: a single hidden/internal `zsh-pro emit <apply|deactivate> <name>` alongside the public `hook`).
- **D-07: `ZSHPRO_PROFILE` is exported by the shell function (not the binary) after a successful eval** (Phase 3 D-13 — the binary validates, the function exports, per-terminal). `deactivate` unsets `ZSHPRO_PROFILE` (⇒ `status` reports `main`) after the eval'd deactivate reverses declarative state.
- **D-08: `ZP_BASE_PATH` is captured ONCE at first activate per terminal, guarded against re-capture (Phase 4 OQ-3 placement, finalized here).** The guard is a set-test on the runtime slot: `[[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"` — the first `activate`/`checkout` in a terminal snapshots the base; subsequent switches reuse it and never re-capture (re-capturing would fold a previous profile's additions into the base and grow `$#path`). The re-capture guard lives in the sourced activate path (before any PATH mutation), NOT in the emitted code (which assumes the base already exists — Phase 4 runtime-state contract).

### Area 3 — Idempotent `.zshrc` block installer

- **D-09: Install verb = `install`; markers = `# >>> zsh-pro >>>` (BEGIN) / `# <<< zsh-pro <<<` (END)** (OQ-05-03, applied). The conda-style `>>>`/`<<<` convention is grep-friendly for the `grep -c` idempotency assertion and unambiguous as zsh comments.
- **D-10: The rewrite algorithm is find-region-then-replace, in Go, byte-exact.** Read `~/.zshrc`; if a BEGIN marker exists, replace the entire inclusive BEGIN..END region with the freshly-rendered block; if absent, append the block (with a single leading newline separator) at EOF. Everything outside `[BEGIN, END]` is preserved verbatim (byte-for-byte). Rendering is deterministic so an unchanged install writes a byte-identical region (Requirement 4 acceptance: second run leaves the marked region identical, `grep -c` BEGIN == 1). If multiple BEGIN markers are somehow present (corrupted state), collapse to a single canonical block (safe-repair, not error). Missing `~/.zshrc` ⇒ create it containing just the block.
- **D-11: The installed block is a THIN fail-open STUB, not the loader itself.** The block does not inline the loader; it guards and sources. Shape (rendered as a Go template/const string): early-return on `ZSHPRO_DISABLE=1`; `command -v zsh-pro` existence guard; then `eval "$(zsh-pro hook)"` OR source a cached loader file — see D-13. Keeping the block thin means marker text + guard logic stays stable across releases (a marker/loader-body change would otherwise orphan old blocks).
- **D-12: The unmanaged master block is everything the installer never touches** — it is not a second marked region the installer manages; it is simply the user's own `.zshrc` content outside the zsh-pro markers, which D-10 preserves by construction. Phase 6 owns detecting/ warning about out-of-block appends; Phase 5 only guarantees non-destruction.

### Area 4 — Fail-open guards, `zsh -n` validation, last-good, zero-subprocess

- **D-13: The shell-start HOT PATH is zero-subprocess by construction.** The installed stub, on plain shell start (no explicit verb), MUST NOT run `git`, MUST NOT invoke the `zsh-pro` binary, and MUST NOT use `$(...)`. To satisfy this AND still get helper/function definitions cheaply, the stub sources a **cached loader script file** written at `install` time (e.g. under the store/data dir), not `eval "$(zsh-pro hook)"` on every start (that is a subprocess). Guard: `[[ -r <cached-loader> ]] && source <cached-loader>`. The binary invocation happens only inside the verbs (explicit user action). `install` (and, later, a self-heal) regenerates the cached file from `zsh-pro hook`. This is the primary structural guarantee for Requirement 7; the `hyperfine` number is the backstop. (If plan-phase finds a cheaper form that is still zero-subprocess on start, that is Claude's discretion — the invariant is "no `$(...)`/`git`/binary on the start path", grep-verified.)
- **D-14: Guard order in the stub is: `ZSHPRO_DISABLE` early-return → `[[ -r <cached-loader> ]]` readability → source.** `ZSHPRO_DISABLE=1` returns before defining any verb or sourcing anything (full no-op — Requirement 5). An absent/unreadable cached loader is a silent no-op (no error, `.zshrc` continues to an interactive prompt). No stub path may emit a non-zero exit that aborts `.zshrc`.
- **D-15: Emitted apply/deactivate code is `zsh -n`-validated BEFORE it is `eval`'d, inside the verb (not the hot path).** The verb captures the binary's emitted code into a variable/tempfile, runs `zsh -n` on it, and only `eval`s it if syntax-valid. `zsh -n` here is acceptable because it is on an explicit verb invocation, not shell start. Where the `zsh -n` check lives (in the sourced function via a `zsh -n <<< "$code"` here-string, or the binary self-validates before printing) is Claude's discretion (OQ-05-07 — safe default: validate in the sourced verb with `zsh -n` on the captured string, closest to the `eval` boundary).
- **D-16: A per-terminal LAST-GOOD record lets a failed switch fall back instead of half-applying.** The last successfully-applied profile name (and enough to re-apply or safely no-op) is retained in a per-terminal global (e.g. `ZP_LAST_GOOD_PROFILE`). If a switch's emitted code fails `zsh -n` (or the binary errors), the verb aborts BEFORE eval'ing — the live shell keeps the prior good state (no partial apply, Requirement 6). Since Phase 4's plan is deactivate-then-activate, the fail-safe is to validate the ENTIRE emitted block up front and refuse to eval any of it on failure (atomic per switch), rather than eval'ing the deactivate half then discovering the activate half is broken. The exact last-good payload (name only vs. name+manifest ref) is Claude's discretion (OQ-05-08 — safe default: profile name only; re-derivation via the binary on demand, since the store is the source of truth).
- **D-17: The `hyperfine` budget is < 10 ms added mean startup cost** (OQ-05-04, applied), measured `hyperfine 'zsh -i -c exit'` with the block installed vs not. The structural zero-subprocess grep check (D-13) is the load-bearing guarantee; the millisecond budget is the empirical backstop. Tighten if the measured overhead is far below it during plan/execute.

### Area 5 — CLI + composition-root wiring

- **D-18: The new verbs dispatch from `CLI.Run`'s `switch args[0]`, mirroring the existing `analyze` case.** Add `hook` (prints the embedded loader const — pure, no store), the internal emit subcommand(s) (D-06), `list`, `status`, and `install`. `hook`/`install` need no store (hook is a pure const print; install writes `.zshrc` + the cached loader). `list`/`status`/emit need the store. Exit codes reuse the typed `model.ExitCode` contract (0 clean / 1 runtime / 2 usage / 3 actionable) already in `cli.go`.
- **D-19: The store is injected into `CLI` at the composition root (`core/cmd/zsh-pro/main.go`), never imported by `core/cli` directly.** `main.go` already constructs the store (currently `_, _ = s, err`); Phase 5 threads it into `cli.New` via a store interface/dependency, keeping `core/cli` dependent on abstractions (the existing pattern: `cli.New(provider)` gains a store parameter or a small store-facing interface). A store-init error stays non-fatal for `analyze`/`hook` (which do not need the store) and surfaces only when a store-backed verb (`list`/`status`/`checkout`) runs — matching main.go's existing comment. Exact injection shape (add a param to `cli.New` vs a setter vs a narrow `store`-facing interface in `core/cli`) is Claude's discretion (OQ-05-09 — safe default: extend `cli.New(provider, store)` with a minimal store interface declared in `core/cli`, satisfied by `*store.Store`).
- **D-20: The loader const lives in `core/shell/zsh` (the milestone invariant — only that package writes zsh syntax), alongside `emit.go` and `introspect.go`.** `core/cli`'s `hook` handler asks the provider (via a new interface method, e.g. `HookScript() string`, mirroring how `Regenerator`/`Introspector` sit on the provider seam) for the loader string and prints it — `core/cli` never contains zsh text. Exact seam name/shape is Claude's discretion (OQ-05-10 — safe default: a `shell.Hooker`/`HookScript()` method on the Provider composite, mirroring `Regenerator`).

### Claude's Discretion

- Exact Go identifiers, file names, and internal structure of the new CLI verb handlers and any store-facing interface in `core/cli` (D-18/D-19), provided layering holds (`core/cli` depends on abstractions; concrete store/`zsh` wired only at `main.go`).
- The precise seam method exposing the loader const (`HookScript()` on the Provider vs a standalone `shell.Hooker`), provided the const lives in `core/shell/zsh` and `core/cli` holds no zsh text (D-20).
- Whether PATH-rebuild is a named `zp_*` helper or inlined by `emit.go` — the loader supplies whatever Phase 4's emitted calls name; confirm the exact call surface against the actual `emit.go` when both land (OQ-05-05).
- The binary emit subcommand naming/shape the verbs call (`zsh-pro emit apply <name>` etc.) and whether it is one subcommand with a mode arg or two (D-06 / OQ-05-06).
- Where `zsh -n` validation physically runs (sourced function here-string vs binary self-check) (D-15 / OQ-05-07) and the last-good payload richness (D-16 / OQ-05-08).
- The exact cached-loader file location and refresh trigger, provided the shell-start path stays zero-subprocess (D-13) and the file is under the store/data dir (not a shared "current profile" file — Phase 3 D-13 no-shared-file rule).
- Test internals: fixtures, how the zsh-requiring live-terminal wiring tests are structured (following the `LookPath`-guarded `zsh -f` precedent in `introspect_test.go`/`corpus_test.go`), and the `hyperfine`/`zsh -n` harness wiring — provided the falsifiable properties (idempotency byte-exactness, fail-open reaches prompt, zero-subprocess start path, `$#path` stability) are pinned.
- Commit granularity within the phase (GSD `gsd-sdk query commit` flow).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`core/shell/zsh/introspect.go` `introspectScript`** — the exact `const` raw-string zsh embed pattern the loader const mirrors (a Go `const` in `core/shell/zsh`). Also the `exec.CommandContext(ctx, "zsh", "-f", ...)` + 5s-timeout + graceful-degradation shape, and the `LookPath`-guarded zsh-requiring test precedent (`introspect_test.go`) the live-terminal wiring tests follow. NOTE the deliberate divergence: `introspectScript` uses `emulate -L zsh` for ISOLATION; the loader's apply/deactivate must be PLAIN so options escape scope (D-04).
- **`core/cli/cli.go`** — `CLI.Run`'s `switch args[0]` dispatch (currently only `analyze`), the typed `model.ExitCode` contract, and the `fail` helper (structured JSON error to stdout in `--json`, human line to stderr). New verbs slot into the same switch.
- **`core/cmd/zsh-pro/main.go`** — the sole composition root; already constructs the store (`store.New(dir, provider, kc)`) and holds it as `_, _ = s, err` with a comment explicitly reserving the store-consuming verbs for Phase 5. This is where the store is threaded into the CLI.
- **`core/store/store.go`** — `Branches(ctx)` (for `list`), `Current()` (reads `ZSHPRO_PROFILE`, unset⇒`main`, for `status`), `Checkout(ctx, name)` (validates existence, does NOT export — the loader exports), `Read(ctx, branch)` (the profile the emit path consumes). All the verbs' data source already exists.
- **Phase 1 Loader Reference Snippet** (`01-FINDINGS.md` §"Loader Reference Snippet") — the durable reference for the `zp_*` helper bodies: `zp_capture_env`/`zp_restore_env` with `${(P)+var}=="1"`, PATH-from-base rebuild, shadow-body capture/restore. Reproduce WITH the Phase 4 OQ-8 correction (shadow restore uses `${+name}` set-tests, NOT the snippet's buggy `[[ -n "$slot" ]]`).
- **`store.KeychainDriver`** (`core/store/keychain.go`, wired at main.go) — the secret-deref seam Phase 5 drives only if emitted code carries an unresolved `SecretRef` (OQ-05-02); end-to-end PROF-03 is Phase 6.

### Established Patterns
- **Single composition root:** only `core/cmd/zsh-pro/main.go` imports `core/shell/zsh` and the concrete store. `core/cli` depends on interfaces (`shell.Provider`, and a new store-facing abstraction). The loader const + `zp_*` helpers are zsh syntax → they belong in `core/shell/zsh`, reached via a provider seam, NOT assembled in `core/cli`.
- **Typed exit codes + agent JSON contract** (`model.ExitCode`, `cli.fail`) — new verbs honor the same contract.
- **Sandboxed `zsh -f` subprocess with `LookPath` skip-guard** — the pattern for the `zsh -n` validation and the zsh-requiring live-terminal wiring tests (skip cleanly when zsh absent).
- **`core/model` is dependency-free; no new Go deps** — the loader is embedded zsh (a `const`); `hyperfine`/`zsh -n` are dev/CI tools, not modules (SPEC constraint).

### Integration Points
- **Input:** `Store.Read(<branch>)` → `model.Profile` → (Phase 4) `model.Manifest`/`Plan` → (Phase 4) `emit.go` apply/deactivate zsh code. Phase 5's verbs obtain that emitted code from the binary and `eval` it live.
- **Runtime-state contract (Phase 4↔5 seam, this phase owns the runtime half):** the emitted code assumes per-terminal state exists — `ZP_BASE_PATH` (D-08 captures it once at first activate), `__ZP_ORIG_<var>` env-undo slots (written by `zp_capture_env`, read by `zp_restore_env`), `ZP_<profile>_PRIOR_ALIAS_/FUNC_<name>` shadow slots. Phase 5 defines the helpers that read/write these slots.
- **Output:** `zsh-pro hook` stdout → sourced (via the cached loader file the installed `.zshrc` block sources). `install` writes both the `.zshrc` block and the cached loader file.
- **Composition-root delta:** `main.go` threads the already-constructed store into `cli.New`; store-init errors stay non-fatal for non-store verbs (`analyze`/`hook`), surfacing only on `list`/`status`/`checkout`.

</code_context>

<specifics>
## Specific Ideas

- **A child process cannot mutate its parent shell — hence sourced FUNCTIONS, not a bare binary** (Requirement 2). `activate`/`checkout`/`deactivate` are zsh functions that `eval "$(zsh-pro …)"`; only `list`/`status` can be thin binary wrappers. This is the reason the loader exists at all.
- **Zero-subprocess on shell start is the load-bearing perf guarantee** (Requirement 7 / BOOT-02): the installed stub sources a CACHED loader file (`[[ -r … ]] && source …`), it does NOT `eval "$(zsh-pro hook)"` on every start (that is a subprocess). Binary/`git`/`$(...)` calls happen ONLY inside explicit verbs. Grep-verify the start path has none; `hyperfine` (< 10 ms) is the backstop.
- **Fail-open is absolute** (Requirement 5): `ZSHPRO_DISABLE=1` → full early-return no-op; absent/unreadable binary or cached loader → silent no-op reaching a working prompt; no stub path emits a shell-aborting non-zero exit.
- **`ZP_BASE_PATH` captured ONCE, re-capture guarded** (Phase 4 OQ-3, placement finalized here): `[[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"` at first activate. Re-capturing per switch would fold prior additions into the base and grow `$#path` — the exact residue the Phase 4 property test catches.
- **Reproduce the Phase 1 snippet WITH the OQ-8 correction, not verbatim**: shadow restore uses `${+name}` set-tests (correctly restores an empty prior), NOT the raw snippet's `[[ -n "$slot" ]]` (which silently drops it). Env path keeps `${(P)+var}=="1"` (correct — operand holds the name). Slot names sanitized to `[A-Za-z0-9_]` (C24, also an injection block).
- **Idempotency is byte-exact** (Requirement 4): find-BEGIN..END-then-replace; append only if absent; everything outside the markers preserved byte-for-byte; second run ⇒ byte-identical marked region, `grep -c` BEGIN == 1.
- **`zsh -n` before `eval`, atomically per switch** (Requirement 6): validate the ENTIRE emitted apply/deactivate block up front; refuse to eval any of it on failure so the shell keeps the last-good state (no half-apply). `zsh -n` runs on the explicit verb, never on the hot path.

</specifics>

<deferred>
## Deferred Ideas

- **Secret deref-on-switch end-to-end (PROF-03)** — Phase 6 owns the real-`~/.zshrc` ingest round-trip; Phase 5 only drives the existing `store.KeychainDriver` seam if the Phase-4-emitted code surfaces an unresolved `SecretRef` at apply time (OQ-05-02).
- **Out-of-block installer-append detection/warning** (user code appended below the zsh-pro block) — Phase 6; Phase 5 only guarantees non-destruction of content outside the markers.
- **Real `~/.zshrc` end-to-end ingest → baseline commit** — Phase 6.
- **Keybindings (`bindkey`) / hooks (`precmd_functions`/`chpwd_functions`) / `compinit`** — routed to the unmanaged master block (Phase 4 OQ-1); not switchable in v2.0.
- **Auto-activate on `cd`** (a `chpwd`/`precmd` per-prompt hook) — future AUTO-01; a per-prompt hook would violate the zero-subprocess hot-path constraint.
- **Multi-active / concurrent managed profiles per terminal + shared-profile trust gates** — single-active per terminal (Phase 3 D-13); SHARE-01 is a future milestone.
- **Other shells (bash/fish)** — the activation model is zsh-specific for the whole milestone.
- **Marker-text / stub-body migration** for already-installed blocks across releases — a later concern once markers ship and must stay stable (noted in D-11).

</deferred>

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Context gathered: 2026-07-02 (autonomous smart-discuss, unattended — AskUserQuestion suppressed)*
*New open questions logged: OQ-05-05..OQ-05-10 in 05-OPEN-QUESTIONS.md (safest reversible defaults applied; do not block planning).*
*Next step: /gsd:plan-phase 5 — loader const structure + `zp_*` helper bodies (matched to emit.go's actual call surface), verb function wiring, installer rewrite algorithm, zsh -n + last-good harness, hot-path perf verification, composition-root store injection.*
