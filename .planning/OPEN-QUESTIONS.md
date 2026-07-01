# Open Questions — v2.0 Branchable Shell Environments

Aggregated across phases by `/gsd:docs-autonomous`. **Review before executing affected phases.**
Every item already has a tentative safe/reversible default applied so downstream work could proceed —
this is the audit + override queue for the returning user. Most are HIGH-confidence auto-decisions
recorded for transparency; the ⚠ items are the ones most worth a look.

## Phase 4 — Manifest Builder + Emit   ([details](phases/04-manifest-builder-emit/04-OPEN-QUESTIONS.md))

29 parked decisions (OQ-1..OQ-29). Docs converged: adversarial review 0 HIGH (16→11→4→1→1→0 over 6 cycles); 32 claims validated (19 PROVEN, 1 STATIC-VALIDATED, 12 REFUTED-then-corrected, 0 unverified). Highlights:

- **OQ-1** — keybindings/hooks (`bindkey`, `precmd_functions`) stay OUT of the managed set for v2.0 → unmanaged master block (like `compinit`). HIGH confidence; additive to admit later.
- ⚠ **OQ-3** — PATH "captured base" is per-terminal runtime state captured ONCE at first activate; the exact loader-side capture *placement* is a Phase 4/5 seam (finalized in Phase 5). MEDIUM-HIGH; the property test is the backstop.
- ⚠ **OQ-4 / OQ-11** — `NameSet` shape: resolved to TWO distinct types (`AliasSet` map / `FuncSet` array) to match the validated `01-MANIFEST-SHAPE.md` fixture byte-for-byte (a uniform map provably can't unmarshal `functions.added: [...]` — C16). MEDIUM; the planner may keep a uniform map if it also edits the fixture.
- **OQ-5 / OQ-9 / OQ-18** — `emit.go` emits bare `zp_*` helper CALLS (helpers defined by the Phase-5 loader; the property test supplies them inline); Hybrid-preamble option dropped. HIGH confidence.
- **OQ-6** — injection split: static values single-quote-wrapped (inert), dynamic values verbatim (late-bound). PROVEN (C5/C7).
- **OQ-7 / OQ-17** — function/alias body dump uses NUL-delimited (multi-line-safe) framing, not a line/`\t` format. PROVEN (C14/C23).
- **OQ-8** — Phase 1 loader snippet's shadow-restore guards were BUGGY (`-n` / `${(P)+literal}`); corrected to `${+name}` set-tests. PROVEN (C1/C2).
- **OQ-10** — runtime undo-slot names embed the (sanitized) profile name; adversarial names sanitized. PROVEN (C24).
- **OQ-12** — 7 load-bearing corrections auto-applied from claim-validation pass 1 (C1/C6/C7/C10/C11/C19/C23) — see 04-EVIDENCE.md.
- **OQ-13..OQ-29** — cycle-review-derived decisions, each auto-resolved with the safest default and validated: multi-managed PATH co-ownership deferred to Phase 5 (single-active v2.0, OQ-13); type-class snapshot filter (OQ-20/C28); balanced-sequence residue harness (OQ-22); real SchemaV1 gate (OQ-15); option-name + alias/func-name identifier-grammar validation (OQ-25/CH-19, C29/C31); PATH addition-segment static/dynamic split (OQ-29/CH-24, C32).

_No residual HIGH review concerns and no UNVERIFIABLE claims for Phase 4._

<!-- Phases 5, 6 pending documentation -->
