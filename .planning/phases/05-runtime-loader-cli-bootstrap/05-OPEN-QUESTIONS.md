---
phase: 05-runtime-loader-cli-bootstrap
updated: 2026-07-02T00:00:00Z
open_count: 14
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


## OQ-05-11: Structural zero-subprocess grep must exclude verb function BODIES, not just the stub

- **Question:** Requirement 7's acceptance greps the shell-start path for `$(...)`/`git`/`zsh-pro`. But the cached loader that the stub sources DEFINES the verb functions, whose bodies legitimately contain `$(zsh-pro emit …)` and (via the store) `git`. A naive whole-file grep of the cached loader would false-fail. What exactly does the zero-subprocess assertion grep?
- **Tentative choice (applied):** The assertion targets the **start path only** = the installed stub block + the loader's top-level (sourced-at-load) statements, EXCLUDING the verb function bodies (which execute only on explicit invocation, never at source time). Concretely: grep the stub block (must be clean) and assert that no `$(`/backtick/`git`/bare-`zsh-pro`-command appears at the loader's top level outside a `funcname(){ … }` body. Verified (RESEARCH E13) that the stub block itself is clean; the loader's top level is pure `typeset`/function-definition. Sourcing a function definition does NOT execute its body — so a `$(zsh-pro …)` inside `activate(){ … }` is never run at start.
- **Alternatives:** (a) whole-file grep of the cached loader (rejected — false-fails on legitimate verb-body subprocess calls); (b) run the loader under a `zsh` trap that fails on any `exec`/command-substitution at source time (heavier; the grep-of-top-level is simpler and equally load-bearing).
- **Why uncertain:** The precise grep expression (how to reliably delimit "top level" vs "inside a function body" in the emitted loader text) is an implementation detail; a structural check that only inspects the stub + asserts the loader top-level is definitions-only is the safe form.
- **Impact:** Low-medium — a wrong grep either false-fails CI or misses a real subprocess. The `hyperfine` budget (< 10 ms) is the empirical backstop either way.
- **Confidence:** MEDIUM-HIGH. The invariant (no subprocess executes at source time) is firm and verified; only the exact grep encoding is open.

## OQ-05-12: `hyperfine` measurement fixture — how to install-vs-not-install a `.zshrc` block hermetically

- **Question:** `hyperfine 'zsh -i -c exit'` must compare startup WITH vs WITHOUT the managed block, without mutating the developer's real `~/.zshrc`. `hyperfine` is ABSENT locally (a dev/CI tool). How is the comparison staged?
- **Tentative choice (applied):** Use a throwaway `ZDOTDIR` pointing at a fixture dir containing a `.zshrc` with the block (and its cached loader) vs an empty `.zshrc`, e.g. `hyperfine 'ZDOTDIR=<with> zsh -i -c exit' 'ZDOTDIR=<without> zsh -i -c exit'`. This never touches the real `~/.zshrc`. Where `hyperfine` is absent (local dev, this research), an in-process `EPOCHREALTIME` loop over N sources of the cached loader is the proxy (RESEARCH E14 measured ~0.03 ms added, 380× under the 10 ms budget). The `hyperfine` run is a CI gate gated on the tool being present; the structural zero-subprocess grep (OQ-05-11) is the primary guarantee.
- **Alternatives:** (a) mutate `~/.zshrc` under test then restore (fragile, risks clobbering a real config); (b) skip `hyperfine` entirely and rely only on the structural grep (loses the empirical backstop). The `ZDOTDIR` fixture is the hermetic, reversible choice.
- **Why uncertain:** Whether CI has `hyperfine` installed, and the exact fixture wiring, are environment/plan details; the measurement is a backstop, not the load-bearing guard.
- **Impact:** Low — the structural grep is the real gate; the ms budget is a sanity backstop with a working proxy when `hyperfine` is unavailable.
- **Confidence:** MEDIUM-HIGH.

---

> The following were logged by claim-validation pass 1 (2026-07-02, `05-EVIDENCE.md` — C16 REFUTED, C15 UNVERIFIABLE). Each records a plan-time/CI gate the executor must honor; corrections already applied to 05-SPEC/05-CONTEXT/05-RESEARCH.

## OQ-05-13: `zp_*` helper-contract reconciliation gate — diff the loader against `emit.go`'s ACTUAL call surface once Phase 4 lands

- **Question:** Claim-validation pass 1 REFUTED C16: the Phase 5 docs asserted the loader "defines EXACTLY the helper set Phase 4's `emit.go` emits bare calls to" (enumerating `zp_capture_env`/`zp_restore_env`/`zp_rebuild_path`/shadow-capture/shadow-restore). But (a) `emit.go`/`core/activate` do NOT exist on disk yet — no bare-call surface to match; (b) Phase 4 leaves emit-vs-inline as Claude's discretion (Phase 4 OQ-5/OQ-18), NOT locked; (c) Phase 4 commits by NAME only to `zp_capture_env`/`zp_restore_env` — shadows (`RestoreShadowedAlias/Func`), PATH-rebuild (`PATH="$ZP_BASE_PATH"; path=(<additions> $path)`), and options (`SetOption`/`RestoreOption` with inline `[[ -o opt ]]`→`was_on`) are documented INLINE; `zp_rebuild_path`/`zp_shadow_*` appear NOWHERE in Phase 4. What exactly does the loader guarantee, and how is the surface finalized?
- **Tentative choice (applied):** The docs were reworded (05-SPEC Req 1 + Background + in-scope + Constraints; 05-CONTEXT deliverable 1 + in-scope + D-01 + the PATH-rebuild bullet + the shadow bullet; 05-RESEARCH Primary Recommendation + D-01 + sub-area (a) gate box/table) from a settled "exact match" FACT into a **plan-time reconciliation gate**. The loader DEFINITIVELY provides the two Phase-4-committed named env helpers (`zp_capture_env`, `zp_restore_env`) AND the per-terminal runtime STATE the Phase-4 INLINE reverse ops read/write (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, per-option `was_on`). It does NOT assume `zp_rebuild_path`/`zp_shadow_*` are in the contract. **The plan's FIRST task must diff the loader's provided helpers + state against `emit.go`'s REAL emitted call/state surface once Phase 4 lands and adjust** (subsumes OQ-05-05's diff step; localized — both in `core/shell/zsh`).
- **Alternatives:** (a) keep the "exact set" wording (rejected — overstates a not-yet-locked, largely-inline Phase 4 contract; refuted by Phase 4's own D-12/OQ-5 evidence); (b) invent the `zp_rebuild_path`/`zp_shadow_*` helpers and force Phase 4 to call them (rejected — that would re-open Phase 4's emit-vs-inline discretion; Phase 5 must consume whatever `emit.go` emits).
- **Why uncertain:** `emit.go` does not exist yet; the exact bare-call/inline split is byte-confirmable only when both land. Every underlying zsh SEMANTIC (C4–C8) is independently PROVEN; only the contract *shape* was overstated.
- **Impact:** Medium — a mismatch between the loader's provided surface and `emit.go`'s calls breaks activation until reconciled. Reversible (localized signature/state edit in one package); the Phase 4 zero-residue test + Phase 5 live-terminal wiring test are the backstop.
- **Confidence:** HIGH on the runtime-state contract + the two named env helpers; the "diff against real `emit.go`" step is a plan-phase gate, not a decision.

## OQ-05-14: `hyperfine` perf budget verified on CI, not locally (C15 UNVERIFIABLE here)

- **Question:** Claim-validation pass 1 marked C15 UNVERIFIABLE: the < 10 ms added-startup budget is stated as gated by `hyperfine 'zsh -i -c exit'`, but `hyperfine` is ABSENT on the local box (a dev/CI tool, not a Go dep), so the stated gate could not be run. Is the budget confirmed?
- **Tentative choice (applied):** The < 10 ms budget STANDS as the SPEC criterion; only its verification is deferred. Notes added at 05-SPEC Req 7 Acceptance and 05-CONTEXT D-17 record that the `hyperfine` millisecond budget is a **CI-machine gate** (install `hyperfine` via `brew`). The **load-bearing guarantee is the C14 zero-subprocess structural grep** (PROVEN); an in-process `EPOCHREALTIME` proxy (~0.01 ms added, ~900× under budget) is interim evidence pending the real `hyperfine` run in CI.
- **Alternatives:** (a) drop the ms budget and rely only on the structural grep (rejected — loses the empirical backstop); (b) treat the proxy as the gate (rejected — proxy is not interactive `zsh -i -c exit` startup, so it is evidence not a gate).
- **Why uncertain:** The absolute ms number depends on the measurement machine and needs the real tool; the structural zero-subprocess check is the true guarantee, the ms budget a sanity backstop.
- **Impact:** Low — the structural grep is the real gate; the ms budget is a backstop with a working proxy where `hyperfine` is unavailable.
- **Confidence:** HIGH that the budget is comfortably met (proxy ~900× under); the real `hyperfine` confirmation is a CI-machine step.

---

> The following were logged during the `--reviews` replan (2026-07-02, cycle 1 → addressing `05-REVIEWS.md` C1–C13 + M0–M10). Unattended mode: recommended/safest-reversible defaults chosen silently; the medium/low-confidence ones are recorded here.

## OQ-05-15: CLI→emit seam shape — `Emitter` interface vs re-using `Store.Read` (C2)

- **Question:** C2 requires `core/cli` to reach Phase 4's emit path WITHOUT importing `core/shell/zsh` (D-19). Should the seam be a dedicated `Emitter` interface, or should the CLI derive emitted code from `Store.Read(profile)` + the provider's `Regenerator`?
- **Tentative choice (applied):** A dedicated narrow `cli.Emitter` interface (`Emit(ctx, mode, name) (string, error)`) injected at the composition root, with a `notReadyEmitter` stub for the pre-Phase-4 window (compile-time seam present; runtime `fail` "emit path not yet available"; never fake apply code). Mirrors the `Store`/`Provider` injection discipline.
- **Alternatives:** (a) `Store.Read` + `Regenerator` in the CLI (rejected — pushes emit orchestration/ordering into `core/cli`, duplicating Phase 4's emit logic and widening the CLI's responsibility); (b) let the sourced verb shell out to `zsh-pro emit` and have the CLI never model emit at all (rejected — the CLI still needs a production `emit` verb handler; the seam is where Phase 4 plugs in).
- **Why uncertain:** Phase 4's `emit.go` public entry-point shape is not yet on disk; the `Emit(mode,name)` signature is the expected contract but is a RE-DIFF obligation (05-CONTRACT.md, BLOCKING phase-exit).
- **Impact:** Medium — reversible (a one-package interface swap) but load-bearing for Requirement 2; the RE-DIFF gate is the backstop.
- **Confidence:** MEDIUM-HIGH.

## OQ-05-16: Runtime-failure recovery depth — re-apply last-good vs report-only (C11)

- **Question:** C11 requires a runtime-error-mid-`eval` path (readonly-var assignment, `path=()` glob failure under `no_unset`, etc.) to not be silently swallowed. Should the verb attempt an automatic re-apply of `ZP_LAST_GOOD_PROFILE`, or only report and instruct the user?
- **Tentative choice (applied):** The plan permits EITHER — re-derive+re-apply last-good when safe, OR emit a clear "switch failed; shell may be partially changed; run `checkout <last-good>`" report — and the test asserts the recovery/report path FIRES (not silent success). The honest contract wording is downgraded from "atomic / no half-apply" to "syntax-validated + emit-gated + best-effort with runtime-failure recovery/report".
- **Alternatives:** (a) claim true atomicity via a full declarative-state snapshot/rollback around every eval (rejected for this phase — a general shell-state snapshot is heavy and out of scope; deferred as a possible Phase 6 hardening); (b) report-only with no recovery attempt (acceptable fallback, allowed by the plan).
- **Why uncertain:** True runtime atomicity needs a save-point/rollback design that Phase 4's emit shape has not settled; the safest honest stance is best-effort + explicit recovery + truthful wording.
- **Impact:** Medium — affects the strength of the Requirement-6 guarantee; reversible (recovery depth can be strengthened later without changing the seam).
- **Confidence:** MEDIUM. Deferred-with-reason: full snapshot/rollback atomicity is explicitly out of Phase 5 scope (heavyweight, Phase-4-shape-dependent); logged as a candidate future hardening.

## OQ-05-17: `command -v zsh-pro` guard placement — stub vs verb bodies (C9 / D-11 vs D-13)

- **Question:** D-11 mandates a `command -v zsh-pro` guard in the stub; D-13 mandates the stub source a CACHED loader with zero subprocess (the binary is not invoked on the hot path). C9 flagged the guard was dropped. Where does the guard live so both hold?
- **Tentative choice (applied):** KEEP the `command -v zsh-pro` guard IN the stub (it is a builtin, zero-subprocess — satisfies D-13's no-`$(...)`/no-binary-exec constraint) as an AND-condition with the `[[ -r <cached> ]]` readability guard, AND the verb bodies (which DO shell out) remain the place the binary is actually invoked. This honors D-11's SPEC-locked guard without re-opening D-13. No SPEC amendment needed — both decisions are satisfied simultaneously.
- **Alternatives:** (a) relocate the guard solely to the verb bodies + amend 05-SPEC that D-13 supersedes the stub guard (rejected — unnecessary; `command -v` is a builtin and does not violate the zero-subprocess invariant, so the stub can keep it); (b) drop it entirely (rejected — that is the C9 defect).
- **Why uncertain:** Low — `command -v` being a zsh builtin (no fork) is well-established; the structural grep asserts no `$(`/binary-exec, which `command -v` does not trip.
- **Impact:** Low — reversible; the fail-open test now exercises the real guard.
- **Confidence:** HIGH.

## OQ-05-18: `05-CONTRACT.md` as a durable read_first source (C2/C16 + LOW concern)

- **Question:** The review's LOW concern noted the C16 contract table lived in the SUMMARY (written at completion) but Task 2 read_first depended on it — a SUMMARY cannot be a read_first input. Where does the durable contract live?
- **Tentative choice (applied):** Task 1 now writes a dedicated durable `05-CONTRACT.md` IMMEDIATELY (both the loader helper/state contract AND the CLI→emit subcommand/seam contract), with a `[ ] RE-DIFF ... (BLOCKING phase-exit)` checkbox obligation. Task 2/3 read_first cite `05-CONTRACT.md`; the SUMMARY links to it. This makes the re-diff a durably-recorded blocking obligation, not a SUMMARY note.
- **Alternatives:** (a) keep it in the SUMMARY (rejected — the read_first-timing defect); (b) inline the contract in each downstream task's read_first prose (rejected — drift risk across two plans).
- **Why uncertain:** Low.
- **Impact:** Low — improves traceability and closes the read_first-ordering gap.
- **Confidence:** HIGH.
