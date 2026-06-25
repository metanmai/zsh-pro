# Phase 1: SPIKE — Zero-Residue Live Hot-Switch - Context

**Gathered:** 2026-06-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 is a **throwaway derisk experiment**. It delivers two durable outputs: (1) a **go/no-go decision** on whether a live, already-open zsh terminal can apply a profile's declarative state and reverse it with **zero residue** — reversing aliases, functions, env, PATH, **and options**, not just env — and (2) the **validated `Manifest` JSON shape** that becomes the input to Phase 4. It uses a hand-written loader + hard-coded fixture profiles run in a sandboxed `zsh -f`.

Not in this phase: the IR / store / CLI / runtime (Phases 2–6); any change to the user's real shell; product-quality code.
</domain>

<decisions>
## Implementation Decisions

### Determinism bar (the core decision)
- **D-01:** **Byte-identical reverse is the absolute bar for any managed state** — measured by snapshot-diff equality on the state tables (env, aliases, functions, `$PATH`/`$path`, options) dumped as text. No "best-effort" management. This is the project's standing **measurement instrument for determinism**, reused to verify the real switch loop in Phases 4–5 — not a one-off spike criterion.
- **D-02:** The byte-identical check is the **admission test** for what's switchable. A state class earns into the managed set only by provably reversing byte-identical. A class that fails (most likely completion) is **excluded to the unmanaged master block** — a *scoping* outcome, NOT a product-killer.
- **D-03:** A true **no-go** fires only if the **core** classes (aliases / env / PATH) cannot hit the bar — i.e. deterministic switching is impossible at all. Low-risk (these are simple tables that direnv/shadowenv/conda/Lmod reverse reliably in production). Failure of options or completion narrows scope; it does not kill the project.
- **D-04:** "Byte-identical" means exact: PATH **order** preserved, "unset" distinct from `""`, no trailing/duplicate-slash drift, no **accumulation** across repeated switches. The **drift guard** is the carve-out — reverse only what the profile applied; never clobber a value the user changed by hand mid-session.

### Managed-surface scope for the spike
- **D-05:** **Measure all six classes** — aliases, functions, env, PATH, options, AND completion (`fpath`/`compinit`). Completion is included as **data, not veto**: its byte-identical result decides admission (switchable) vs. exclusion (master block). The **`LOCAL_OPTIONS`/`emulate -L` trap** is the specific mechanic the spike MUST confirm: option (and any state) changes made *inside* the loader function get silently auto-reverted at function exit, so apply must **escape function scope** (parent-scope `eval` of emitted code).

### Fixtures & safety
- **D-06:** Prove the mechanism with **synthetic, hand-written two-profile fixtures** (clean, every case deliberate) for the core round-trip; **then run one pass against a slice of a real `~/.zshrc`** as a reality check to surface messy real-world patterns the synthetic case won't.
- **D-07:** The spike runs entirely in a **sandboxed `zsh -f`** (no rc files) subprocess — it does NOT mutate the user's real, live shell. Mirrors the existing `introspect.go` `zsh -f -c` + timeout + graceful-degrade pattern.

### Spike artifact fate
- **D-08:** **Throwaway code.** Keep only two durable outputs: the **go/no-go writeup** (which classes are cleanly reversible → managed; which are excluded → master block) and the **validated `Manifest` JSON shape** (the real deliverable; Phase 4's input). The loader script survives only as a reference snippet inside the findings, not as production code.

### Claude's Discretion
- The exact snapshot/diff harness mechanics, the precise fixture contents, and the manifest JSON field names are left to research/planning — provided D-01 (byte-identical, exact) and D-05 (all six classes) hold.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Activation / manifest design (load-bearing)
- `.planning/research/ARCHITECTURE.md` — the `Manifest` / shadowenv `undo::Data` reference shape (env scalars with drift guard; PATH-like vars as additions/deletions deltas vs a captured base); emit-and-source runtime; the explicit spike step list; the `LOCAL_OPTIONS` option-reversal warning.
- `.planning/research/PITFALLS.md` — zero-residue pitfalls; **capture-base-before-mutation** as the root principle; per-shell (not global) active state; fail-open loader.
- `.planning/research/FEATURES.md` — direnv reverse-diff (`DIRENV_DIFF`) as the north star; conda snapshot-overwrite as the documented anti-pattern; Lmod reference-counted (ownership-aware) PATH restore.
- `.planning/research/STACK.md` — `eval "$(...)"` vs pipe-to-source (zsh subshell trap); zsh builtins (`unalias`, `unset -f`, `unsetopt`, `typeset -U path`, `zmodload zsh/parameter`); codegen via string templating.
- `.planning/research/SUMMARY.md` — cross-cutting synthesis + build order.

### Phase contract
- `.planning/ROADMAP.md` §"Phase 1: SPIKE — Zero-Residue Live Hot-Switch" — goal + the 4 success criteria.
- `.planning/REQUIREMENTS.md` — SW-03 (the requirement this phase validates).

### Reusable engine code
- `core/shell/zsh/introspect.go` — `introspectScript` already dumps aliases/functions/exported-env/`$path`/on-options under `zsh -f` via `zmodload zsh/parameter`; the snapshot mechanism the spike reuses, plus the `zsh -f -c <script>` + 5s-timeout + `Available:false` graceful-degrade subprocess pattern.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `core/shell/zsh/introspect.go` (`introspectScript`): the exact zsh snippet to snapshot aliases/functions/env/`$path`/options as text — directly reusable as the spike's before/after capture.
- `core/testgen/render.go`: the project's existing string-templated zsh codegen pattern (precedent for emitting zsh by templating rather than via the mvdan/sh printer).

### Established Patterns
- Sandboxed subprocess: `exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)` with a timeout and graceful degradation — the spike's harness should follow this shape.

### Integration Points
- None yet — the spike is standalone (a scratch script + fixtures), deliberately not wired into the engine. Its OUTPUT (manifest shape + go/no-go) feeds Phase 4.

</code_context>

<specifics>
## Specific Ideas

- The user framed byte-identical reverse as **"the baseline to measure determinism"** — determinism is a first-class, *measurable* quality bar for the whole product, not just this spike.
- The user's stated concern: a switcher that leaves residue is **unusable**. The spike exists to retire exactly that risk cheaply, before any product code is built on the assumption.

</specifics>

<deferred>
## Deferred Ideas

- Per-profile **completion** as a managed/switchable class — only if the spike proves it byte-identical reversible; otherwise it stays unmanaged (master block) for v2.0.
- Auto-activate-on-`cd` (AUTO-01) and remote profile sharing + trust gate (SHARE-01) — already in REQUIREMENTS.md Future; out of scope for the spike and v2.0.

</deferred>

---

*Phase: 1-SPIKE — Zero-Residue Live Hot-Switch*
*Context gathered: 2026-06-25*
