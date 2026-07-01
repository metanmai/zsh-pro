---
phase: 05-runtime-loader-cli-bootstrap
updated: 2026-07-02T00:00:00Z
open_count: 10
---

# Open Questions — Phase 5 Runtime Loader + CLI + Bootstrap

> Auto-decided under uncertainty while running unattended. Review and override as needed.
> Each item already has a tentative choice applied so downstream work could proceed.

## OQ-05-01: `zp_*` runtime helper ownership — loader defines the bodies

- **Question:** Phase 4 pinned `emit.go` to emit **bare calls** to a fixed `zp_*` helper set (OQ-5/OQ-18) and deferred the helper *definitions* to "the Phase 5 loader." Does the Phase 5 loader own and define the `zp_*` helper bodies (`zp_capture_env`, `zp_restore_env`, PATH-rebuild-from-base, shadow-capture/restore, `ZP_UNSET_SENTINEL`)?
- **Tentative choice (applied):** YES — the embedded loader (emitted by `hook`, living as a `const` in `core/shell/zsh` alongside `emit.go`) defines the `zp_*` helper bodies once. The Phase-4-emitted apply/deactivate code calls them; sourcing the loader makes them available in the terminal. This directly closes the Phase 4 OQ-5 seam ("whether Phase 5 loader defines them … finalized in plan-phase").
- **Alternatives:** (a) `emit.go` emits self-contained helper preambles per block (Phase 4 explicitly DROPPED this — OQ-18 pinned bare calls); (b) a separate `helpers` subcommand distinct from `hook` (extra surface, no benefit — one sourced loader is simplest).
- **Why uncertain:** The exact helper set + signatures must match what Phase 4's `emit.go` actually emits calls to; that firms up when both land together. The helper *behavior* is locked by Phase 1 (reference snippet) + Phase 4 OQ-8 corrections (`${+name}` set-tests, `${(P)+var}==1`).
- **Impact:** Medium — a helper-signature mismatch between the loader and emitted calls breaks activation. Reversible (both are in `core/shell/zsh`; a signature edit is localized). The zero-residue property test (Phase 4) + live-terminal wiring test (Phase 5) are the backstop.
- **Confidence:** HIGH (dictated by Phase 4 OQ-5/OQ-18 bare-call decision). Recorded as an assumption, not a blocker.

## OQ-05-02: Secret deref-on-switch — drive existing keychain seam; end-to-end completion deferred to Phase 6

- **Question:** PROF-03's runtime half ("deref-on-switch": resolve a `SecretRef` `kind:key` from the keychain/vault at apply time) was slated for "Ph4/5" (REQUIREMENTS PROF-03, STATE blocker). Does Phase 5 complete secret deref-on-switch?
- **Tentative choice (applied):** Phase 5 drives the EXISTING keychain seam (`store.KeychainDriver`, already wired at the composition root) IF the Phase-4-emitted apply code carries a `SecretRef` that must be resolved at activation — i.e. Phase 5 provides the runtime resolution path the emitted code needs. But the end-to-end PROF-03 completion (a real ingested `~/.zshrc` whose literal secrets round-trip through exclusion → `SecretRef` → deref-on-switch) is validated in **Phase 6** (which owns the end-to-end ingest path and the PROF-03 traceability checkmark). Phase 5 does not gate on a real ingested profile; it wires the resolution seam and tests it with a fixture `SecretRef`.
- **Alternatives:** (a) Fully complete + validate PROF-03 end-to-end here (rejected — Phase 6 owns the real-`~/.zshrc` ingest path; the checkmark stays with Ph6 per REQUIREMENTS traceability); (b) defer ALL secret runtime work to Phase 6 (rejected — the loader is where apply-time resolution physically happens, so the seam is exercised here).
- **Why uncertain:** Whether Phase 4's emitted code actually surfaces a `SecretRef` at the apply boundary (vs. resolving earlier in the builder) is a Phase 4 emit detail; if Phase 4 resolves secrets before emit, Phase 5 has nothing to deref and this is a no-op.
- **Impact:** Medium — mis-scoping could either duplicate Phase 6 work or leave a runtime gap. Chosen split keeps the seam here, the end-to-end proof in Phase 6.
- **Confidence:** MEDIUM-HIGH (aligned with REQUIREMENTS traceability: PROF-03 completes Ph6). Flagged so plan-phase confirms the Phase 4 emit boundary for secrets.

## OQ-05-03: Install verb name + BEGIN/END marker sentinels

- **Question:** BOOT-01 mandates an idempotent BEGIN/END-marked `.zshrc` block but does not name the install verb or the exact marker sentinels. What are they?
- **Tentative choice (applied):** Install verb = **`install`** (`zsh-pro install`), consistent with the imperative-verb style of the existing CLI. Marker sentinels = **`# >>> zsh-pro >>>`** (BEGIN) and **`# <<< zsh-pro <<<`** (END) — the widely-recognized conda/`>>>`-style managed-block convention, unambiguous as zsh comments and easy to `grep -c` for the idempotency assertion.
- **Alternatives:** (a) `bootstrap`/`init`/`setup` as the verb name (all defensible; `install` is the most conventional for "write my `.zshrc` hook"); (b) alternate marker text (`# BEGIN zsh-pro` / `# END zsh-pro`) — functionally equivalent; the `>>>`/`<<<` form is chosen for prior-art recognizability and low collision risk.
- **Why uncertain:** Pure naming/convention; no correctness impact as long as the markers are stable across releases (a marker-text change between versions would orphan an old block).
- **Impact:** Low — cosmetic + a one-time convention lock. Reversible before first ship; after ship the markers must stay stable (a migration concern for a later phase).
- **Confidence:** MEDIUM-HIGH. Recorded so discuss-phase can rename if the user prefers a different verb/marker.

## OQ-05-04: Hot-path startup budget number

- **Question:** BOOT-02 requires "only a small, file-sourced startup cost" verified via `hyperfine 'zsh -i -c exit'`, but does not state a numeric budget. What is the pass/fail threshold?
- **Tentative choice (applied):** Default target = **< 10 ms added mean** startup cost (block installed vs. not installed), measured by `hyperfine 'zsh -i -c exit'`. Rationale: the hot path is pure function-definition + env-read with zero subprocesses, so single-digit-millisecond overhead is the realistic ceiling; 10 ms is a generous, human-imperceptible bound that still fails loudly if a subprocess sneaks onto the hot path. Confirm the achievable number empirically during discuss/plan and tighten if the measured overhead is far below it.
- **Alternatives:** (a) A relative budget (e.g. < 5 % of baseline `zsh -i -c exit`) — noisier on fast machines; (b) < 5 ms (tighter, but risks false-fail on slow CI); (c) < 20 ms (looser, but would tolerate an accidental subprocess). The zero-subprocess *structural* check (grep for `$(...)`/`git`/`zsh-pro` on the start path) is the primary guard; the `hyperfine` number is the empirical backstop.
- **Why uncertain:** The absolute number depends on the measurement machine and zsh version; the structural zero-subprocess check is the load-bearing guarantee, the millisecond budget is a sanity backstop.
- **Impact:** Low-medium — a wrong number either false-fails on slow hardware or tolerates regression. Pairing it with the structural grep check de-risks either direction.
- **Confidence:** MEDIUM. Flagged so discuss/plan-phase locks the exact figure against a real `hyperfine` run.

---

> The following were auto-decided during smart-discuss (Phase 5 CONTEXT, 2026-07-02, unattended). Each has a safe reversible default applied so planning can proceed; confirm against the actual Phase-4 `emit.go` call surface and codebase during plan-phase.

## OQ-05-05: Exact `zp_*` helper call surface must match Phase 4's actual `emit.go` output

- **Question:** Phase 4 is documented but NOT yet executed — `core/shell/zsh/emit.go`, `core/activate`, and `core/model/manifest.go` do not exist on disk yet. The loader's `zp_*` helper set/signatures (CONTEXT D-01) are locked from the Phase 1 reference snippet + Phase 4 design (D-12/OQ-8), but the LITERAL bare calls `emit.go` emits (helper names, arg order, whether PATH-rebuild is a named helper vs inlined) can only be byte-confirmed once emit.go lands.
- **Tentative choice (applied):** Lock the helper contract to the Phase 1 snippet + Phase 4 D-12/OQ-8 shapes (`zp_capture_env <var>`, `zp_restore_env <var> <applied>`, shadow slots `ZP_<profile>_PRIOR_ALIAS_/FUNC_<name>`, PATH `PATH="$ZP_BASE_PATH"; path=(<additions> $path)`). At plan/execute time, diff the loader's defined helpers against `emit.go`'s actual emitted calls and reconcile any signature mismatch (localized edit — both live in `core/shell/zsh`).
- **Alternatives:** (a) block Phase 5 until Phase 4 executes (rejected — CONTEXT is produced ahead of execution by design; the contract is a locked seam, not undefined); (b) invent a new helper set (rejected — must match Phase 4's bare-call seam OQ-5/OQ-18).
- **Why uncertain:** The two land together; a helper-signature mismatch breaks activation until reconciled. The Phase 4 zero-residue property test + Phase 5 live-terminal wiring test are the backstops.
- **Impact:** Medium. Reversible (localized signature edit in one package).
- **Confidence:** HIGH on the contract shape; the "confirm against real emit.go" step is a plan-phase gate, not a decision.

## OQ-05-06: Binary emit subcommand naming the verbs call

- **Question:** The sourced `activate`/`checkout`/`deactivate` functions `eval "$(zsh-pro <emit-verb> <name>)"` to fetch Phase-4-emitted code. What is the emit subcommand's name/shape?
- **Tentative choice (applied):** A single internal subcommand `zsh-pro emit <apply|deactivate> <name>` (mode arg), distinct from the public `hook`. Keeps the CLI surface small; the mode arg avoids two near-identical verbs.
- **Alternatives:** (a) two subcommands (`emit-apply`/`emit-deactivate`) — more explicit but more surface; (b) fold into `checkout` printing both — muddles the validate-vs-emit split.
- **Why uncertain:** Pure naming; no correctness impact as long as the sourced functions and the CLI dispatch agree.
- **Impact:** Low. Reversible before ship.
- **Confidence:** MEDIUM-HIGH.

## OQ-05-07: Where `zsh -n` validation physically runs

- **Question:** Emitted code must be `zsh -n`-validated before `eval` (Req 6). Does the sourced function run `zsh -n` on the captured string, or does the binary self-validate before printing?
- **Tentative choice (applied):** Validate in the sourced verb, `zsh -n` on the captured emitted string (here-string/tempfile), closest to the `eval` boundary — the function is the last checkpoint before mutation, so it is the correct guard site. `zsh -n` here is a subprocess, but only on an explicit verb (not the hot path), so it is allowed.
- **Alternatives:** (a) binary self-validates before printing — moves the check away from the eval boundary and still requires trusting the transport; (b) both (defense-in-depth) — acceptable but redundant for v2.0.
- **Why uncertain:** Either satisfies the acceptance criterion; the sourced-verb site is chosen for being adjacent to the actual `eval`.
- **Impact:** Low-medium. Reversible.
- **Confidence:** MEDIUM-HIGH.

## OQ-05-08: Last-good record payload richness

- **Question:** The per-terminal last-good record (Req 6) lets a failed switch fall back. Does it store just the profile name, or name + manifest/emitted-code reference?
- **Tentative choice (applied):** Profile NAME only (`ZP_LAST_GOOD_PROFILE`). The store is the source of truth; re-derivation goes through the binary on demand. Since a failed switch is rejected BEFORE any eval (atomic per-switch validation, D-16), the shell already holds the prior good declarative state — the last-good name is a report/recovery aid, not a re-apply payload.
- **Alternatives:** (a) cache the last-good emitted code per terminal (faster recovery, but stale-cache + per-terminal-storage complexity); (b) name + manifest hash (drift detection — over-engineered for v2.0).
- **Why uncertain:** Depends on whether recovery ever needs to RE-APPLY (vs just not-corrupt). With up-front atomic validation, re-apply is not needed on the failure path.
- **Impact:** Low. Reversible.
- **Confidence:** MEDIUM.

## OQ-05-09: Store injection shape into `core/cli`

- **Question:** `core/cli` must not import the concrete store. How is the store threaded in from the composition root?
- **Tentative choice (applied):** Extend `cli.New(provider, store)` where `store` is a minimal store-facing interface DECLARED IN `core/cli` (e.g. `Branches(ctx) ([]string, error)`, `Current() string`, `Read(ctx, name) (model.Profile, error)`, `Checkout(ctx, name) error`), satisfied by `*store.Store`; `main.go` passes the concrete `*store.Store`. Mirrors the existing `shell.Provider`-interface injection discipline.
- **Alternatives:** (a) a setter (`cli.SetStore`) — mutable, less clean; (b) import `*store.Store` directly into `core/cli` — VIOLATES the composition-root layering (rejected).
- **Why uncertain:** Naming/shape only; the layering constraint is firm.
- **Impact:** Low-medium. Reversible.
- **Confidence:** HIGH on "interface declared in core/cli, injected at main.go"; MEDIUM on the exact method set.

## OQ-05-10: Provider seam exposing the loader const

- **Question:** The loader const lives in `core/shell/zsh`; `core/cli`'s `hook` handler must fetch it without holding zsh text. What seam exposes it?
- **Tentative choice (applied):** A `HookScript() string` method on the Provider composite (a new `shell.Hooker` sub-interface, mirroring how `Regenerator`/`Introspector` compose into `shell.Provider`). `core/cli` calls `c.provider.HookScript()` and prints it — no zsh text in `core/cli`.
- **Alternatives:** (a) a standalone `shell.Hooker` interface injected separately — extra wiring, no benefit; (b) a package-level exported const `zsh.LoaderScript` read at main.go and passed in — leaks the const shape across the seam.
- **Why uncertain:** Seam naming/placement only; the invariant (const in `core/shell/zsh`, `core/cli` zsh-text-free) is firm.
- **Impact:** Low. Reversible.
- **Confidence:** HIGH on the invariant; MEDIUM-HIGH on `HookScript()` on the Provider.
