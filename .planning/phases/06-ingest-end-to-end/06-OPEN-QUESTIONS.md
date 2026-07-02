# Phase 6: Ingest End-to-End — Open Questions

Auto-resolved sub-decisions from `--auto` SPEC generation. Each was resolved to the safest, most reversible default so planning is unblocked. Revisit in `/gsd:discuss-phase 6` if any default is wrong.

The SPEC gate passed on the initial assessment (ambiguity 0.16, all dimensions above minimum), so no dimension is flagged below-minimum. These entries are gray-area implementation-shaping choices logged for transparency, not requirement gaps.

---

## OQ-06-01 — Ingest CLI verb name

- **Question:** What is the user-facing CLI verb that drives end-to-end ingest?
- **Tentative choice:** `ingest` (e.g. `zsh-pro ingest [path]`, default path `~/.zshrc`).
- **Alternatives:** `init` (overloads the store's `Init` concept and the installer bootstrap — rejected as ambiguous); `import` (reasonable synonym, but `ingest` is the term used throughout ROADMAP/REQUIREMENTS/CLAUDE.md — "ingest component", "ingest engine", "on-ramp"); `adopt` (matches the "adopt the messy file" moat framing but is non-obvious).
- **Why uncertain:** Naming is a UX choice; the milestone vocabulary strongly favors "ingest" but the phase also performs the first-time install, so a combined `init`/`setup` verb is defensible.
- **Impact:** Low — a verb name is trivially renameable; no requirement depends on the exact string. Affects only the CLI dispatch surface.
- **Confidence:** High (project vocabulary is consistent on "ingest").

## OQ-06-02 — Master-block physical location and relationship to the Phase 5 installer block

- **Question:** Where does the unmanaged "master block" (imperative run-once verbatim text) physically live, and how does it relate to the Phase 5 BEGIN/END managed block?
- **Tentative choice:** The master block is a distinct, installer-preserved region of `~/.zshrc` that holds imperative entries' verbatim `Text` (the run-once side-effecting code that is NOT switchable). It is the "content outside the markers … the unmanaged master block for imperative run-once code" that Phase 5's installer (BOOT-01) preserves byte-for-byte. Phase 6 routes imperative entries into this region; the exact marker/region contract is reconciled against the Phase 5 installer's actual implementation at plan time (a plan-time gate, mirroring how Phase 5 reconciled against Phase 4's emit surface).
- **Alternatives:** (a) Store the master block inside the git store as a separate committed file (rejected: REQUIREMENTS frames the master block as the `.zshrc` region the installer preserves, and imperative code is unmanaged/non-switchable so it does not belong in the switchable per-category store); (b) A single combined block where managed + master content share one BEGIN/END region (rejected: the managed block must be byte-identical/idempotent across re-installs, but the master block is user-owned imperative content — mixing them breaks idempotency).
- **Why uncertain:** The Phase 5 installer's exact marker sentinels and region layout are not yet on disk (Phase 5 is planned, not executed), so the precise seam Phase 6 routes into is a forward reference — same reconciliation-gate situation Phase 5 faced with Phase 4's `emit.go`.
- **Impact:** Medium — determines where imperative content is written and how re-ingest idempotency + out-of-block detection are implemented. Reversible: the routing target is a plan-time wiring decision, and the SPEC pins the *behavior* (imperative verbatim, never dropped; managed block byte-identical; out-of-block content preserved) independent of the exact file layout.
- **Confidence:** Medium — behavior is locked by REQUIREMENTS/ROADMAP; the physical seam depends on Phase 5's landed installer, so treat as a plan-time reconciliation gate against the actual Phase 5 marker contract.

---

*Phase: 06-ingest-end-to-end*
*Logged: 2026-07-02 (--auto SPEC generation)*
