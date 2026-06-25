# Phase 1: SPIKE — Zero-Residue Live Hot-Switch - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-25
**Phase:** 1-SPIKE — Zero-Residue Live Hot-Switch
**Areas discussed:** Go/no-go threshold, Managed-surface ambition, Fixture realism, Spike artifact fate

---

## Go/no-go threshold

| Option | Description | Selected |
|--------|-------------|----------|
| Strict | All five classes byte-identical or full no-go (product-blocking) | |
| Tiered → admission test | Byte-identical is the absolute bar AND the admission test; a class that fails is excluded to the master block, not product-dead; no-go only if core (aliases/env/PATH) fails | ✓ |
| Essential-only | Don't attempt the hard classes at all | |

**User's choice:** Byte-identical reverse is the absolute, non-negotiable bar — and it functions as the *admission test* for the managed set. A class that can't hit it is excluded (master block), not a product-killer; a true no-go fires only if the core classes can't be reversed.
**Notes:** User reframed byte-identical as "the baseline to measure determinism" — elevating it to a standing, measurable quality bar for the whole product (reused in Phases 4–5), not a one-off gate. Before committing, the user stress-tested viability: "if it is not byte-identical, what does it mean? how frequent? if we can't achieve this the project is unusable." Resolved after walking through (a) the failure-severity spectrum (cosmetic → accumulation → leftover-active → catastrophic), (b) per-class frequency (env/PATH/aliases/functions reliably solved by prior art; options = confirm-in-spike due to the `LOCAL_OPTIONS` trap; completion = doubtful), and (c) that the spike exists precisely to retire this existential risk cheaply and up front.

## Managed-surface ambition

| Option | Description | Selected |
|--------|-------------|----------|
| Include completion | Stress `fpath`/`compinit` reversal in the spike | ✓ (as data, not veto) |
| Scope completion out | Test only aliases/functions/env/PATH/options | |

**User's choice:** Measure all six classes, completion included.
**Notes:** Resolved cleanly by the admission-test framing from the go/no-go decision — since the byte-identical check *is* the admission test, measuring completion is exactly the point: the result decides whether it's switchable or drops to the master block. Its failure is data, never a veto.

## Fixture realism

| Option | Description | Selected |
|--------|-------------|----------|
| Synthetic | Hand-written two-profile fixtures, clean and deliberate | ✓ (for core proof) |
| Real `.zshrc` slice | Fixtures derived from the user's actual config | ✓ (one reality-check pass) |

**User's choice:** Synthetic for the clean core round-trip, then one pass against a real `~/.zshrc` slice as a reality check.
**Notes:** Safety established — both run in a sandboxed `zsh -f` subprocess; the spike never mutates the user's live shell.

## Spike artifact fate

| Option | Description | Selected |
|--------|-------------|----------|
| Pure throwaway | Prove it, write findings, discard the code | ✓ |
| Seed | Build loader/manifest carefully to seed Phase 4–5 | |

**User's choice:** Throwaway. ("Ok fine let's go ahead with the throwaway spike.")
**Notes:** Keep two durable outputs only — the go/no-go decision and the validated `Manifest` JSON shape (Phase 4's input). Loader code survives only as a reference snippet in the findings.

## Claude's Discretion

- Exact snapshot/diff harness mechanics (how state is dumped and compared).
- Precise synthetic fixture contents and the real-config slice chosen.
- `Manifest` JSON field names (the *shape* is the deliverable; naming is open).

## Deferred Ideas

- Per-profile **completion** as a switchable class — only if the spike proves it byte-identical reversible; else unmanaged for v2.0.
- Auto-activate-on-`cd` (AUTO-01) and remote profile sharing + trust gate (SHARE-01) — in REQUIREMENTS.md Future; out of scope for the spike and v2.0.
