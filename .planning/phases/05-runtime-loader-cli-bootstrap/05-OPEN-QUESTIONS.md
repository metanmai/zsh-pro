---
phase: 05-runtime-loader-cli-bootstrap
updated: 2026-07-02T00:00:00Z
open_count: 4
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
