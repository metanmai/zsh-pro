---
phase: 04-manifest-builder-emit
updated: 2026-07-01T12:30:00Z
open_count: 3
---

# Open Questions — Phase 4 Manifest Builder + Emit

> Auto-decided under uncertainty while running unattended. Review and override as needed.
> Each item already has a tentative choice applied so downstream work could proceed.

## OQ-1: Keybindings / hooks class membership (deferred from Phase 1 A3)

- **Question:** Phase 1's spike deliberately did NOT admit or exclude `bindkey` state or hook arrays (`precmd_functions`/`chpwd_functions`) — it reported their presence as *data* and explicitly punted the manage-vs-master-block decision to Phase 4 (see `01-FINDINGS.md` "Keybinding / Hook Presence" and `01-MANIFEST-SHAPE.md` "Classes With NO Representation Here"). Should Phase 4's `model.Manifest` add a managed class for keybindings/hooks?
- **Tentative choice (applied):** NO — keybindings and hooks stay OUT of the managed set for v2.0. They route to the unmanaged master block, exactly like `compinit`. The `model.Manifest` has no field for them; SW-02's zero-residue property test covers only the six admitted classes (env scalars, PATH-like lists, aliases, functions, options — with FPATH as a list).
- **Alternatives:** (a) Add a `keybindings` NameSet-style part now and prove byte-reversibility in this phase; (b) add a reported-only "detected but unmanaged" advisory surface.
- **Why uncertain:** The spike measured `bindkey` at 144 entries and non-empty `precmd_functions` in a real inherited shell, so the class is real — but it was never put through the byte-identical round-trip that admitted the other six classes. Admitting it now would require its own reversibility proof, which is spike-territory, not builder-territory.
- **Impact:** Low for the core switch loop. Excluding keybindings/hooks does not affect SW-01/SW-02 zero-residue on the six admitted classes; it only limits which state a profile can switch. Reversible later (adding a class is additive to `model.Manifest`).
- **Confidence:** HIGH (safest, most reversible default; consistent with precision-over-recall and the Phase 1 completion exclusion). Recorded as an assumption, not a blocker.

## OQ-2: Manifest `env[].original` vs live-prior reconciliation

- **Question:** The validated shape carries a declarative `env[].original`, but the proven loader trusts the LIVE prior captured at apply time (`zp_capture_env`), not the static `original`. How does the emitted deactivate code reconcile the two?
- **Tentative choice (applied):** The manifest's `original` is the declarative record of intent; the emitted apply code captures the live prior into runtime undo state (`__ZP_ORIG_*`) at apply time, and the drift guard (`reverse only if live == applied`) governs restore. Deactivate restores to the captured *live* prior, using the manifest `original` only as the declarative expectation. (Directly follows the Phase 1 carry-forward.)
- **Alternatives:** (a) Trust the static `original` from the manifest (rejected by Phase 1 — breaks byte-identity under a base that differs from authored intent); (b) drop `original` from the manifest entirely (loses the declarative record and the unset-vs-empty encoding).
- **Why uncertain:** This is a design nuance between the manifest as a *record* and the loader as the *actor*; the exact code split lands in emit.go + core/activate during plan-phase.
- **Impact:** Medium — gets the drift guard (SW-02 criterion 3) right. Wrong choice reintroduces the clobber-a-hand-edit failure Phase 1 explicitly guarded against.
- **Confidence:** HIGH (dictated by the Phase 1 loader-reality finding). Locked in the SPEC constraints.

## OQ-3: Where the "captured base" for PATH lives across a switch

- **Question:** PATH is a delta vs a captured base. Is the base captured once at first-activate (session base) and reused across switches, or re-captured per switch?
- **Tentative choice (applied):** Base is captured ONCE at first activate into per-terminal runtime state (`ZP_BASE_PATH`-style), and every apply rebuilds `PATH` from that same base + the profile's additions/deletions — never `typeset -U`, never append. Deactivate rebuilds from the same base. (Phase 1 FM2 proved this keeps `$#path` stable across ≥5 cycles.)
- **Alternatives:** (a) Re-capture base on every switch (rejected — a switch would bake the prior profile's additions into the "base", causing PATH growth, the exact FM2 failure); (b) store base in the manifest (rejected — base is machine/session-specific runtime state, not portable profile data).
- **Why uncertain:** The precise capture *placement* (which loader entry point captures the base, and the re-capture guard) is called out in the ROADMAP research flags as "subtle" and is finalized in Phase 5's loader — Phase 4 emits the plan assuming a base exists in runtime state.
- **Impact:** Medium — wrong placement causes PATH accumulation (SW-02 criterion 2). The property test is the backstop.
- **Confidence:** MEDIUM-HIGH (the mechanism is locked by Phase 1; only the loader-side placement is a Phase 5 concern). Flagged so plan-phase treats base-capture placement as a Phase 4/5 seam contract.
