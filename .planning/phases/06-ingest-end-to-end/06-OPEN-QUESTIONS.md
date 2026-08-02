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

## OQ-06-02 — Startup adoption layout (SUPERSEDED 2026-08-02)

- **Superseding decision:** There is no physical unmanaged block or generated complement. Preserve every ordinary startup byte at its original location and persist the complete redacted source-ordered Profile. `EffectiveManaged` is activation/reporting-only. First adoption appends the landed canonical loader region; installed/re-ingest replaces or collapses only exact marker regions. The controller promotes that canonical install candidate once before Store commit and never rewrites the target after commit.
- **Evidence:** Landed `core/cli/install.go` owns exact markers `# >>> zsh-pro >>>` / `# <<< zsh-pro <<<`, byte-preserving append/replace/collapse behavior, and loader-before-target order. P6-011/P6-012 exposed the full-Profile/order failures in the earlier tentative choice.
- **Status:** CLOSED — superseded by `06-CONTEXT.md` D-05/D-06/D-07/D-12 and `06-SPEC.md` Requirements 1/3/5.

## OQ-06-03 — Ingest `--json` output shape (dedicated DTO/renderer vs inline struct)

- **Question:** Does the `ingest` verb's `--json` output get a dedicated `core/dto` type + `core/render` renderer (like `analyze`), or a smaller inline JSON struct emitted from the CLI?
- **Tentative choice:** A small ingest-result DTO in `core/dto` rendered via a thin `core/render` path — consistent with the existing domain→DTO separation and the `analyze` precedent (`core/render/json.go` + `core/dto/envelope.go`). Locked fields include `managed_entries`, `unmanaged_statements`, `source_statements`, `accounted_statements`, optional `unmanaged_source_lines`, withheld list (name + line, NO value), warning list, ok/exit_code, and truthful transaction state. The required invariant is `managed_entries + unmanaged_statements = accounted_statements = source_statements`.
- **Alternatives:** An inline `map[string]any`/anonymous struct marshaled directly in the CLI (lighter, but diverges from the established render/DTO seam and risks leaking store types onto the wire); reusing the `fail` envelope shape for success too (conflates error and success envelopes).
- **Why uncertain:** UX/serialization-surface choice; both satisfy the agent contract (exactly one JSON object on stdout). The `analyze` path establishes the DTO+renderer pattern, but ingest's payload is small enough that an inline struct is defensible.
- **Impact:** Low for DTO-versus-inline placement only. The complete accounting fields/invariant, withheld metadata, warnings, transaction state, one-object JSON framing, and absence of secret values/zsh text/store internals are required and are not discretionary.
- **Confidence:** Medium-High (the render/DTO seam is the established pattern; safe default follows it).

## OQ-06-04 — Marker-detector ownership (CLOSED 2026-08-02)

- **Question:** How does the out-of-block-append detector obtain Phase 5's BEGIN/END marker strings without duplicating them or crossing the composition-root layering line?
- **Decision:** Co-locate the detector with the landed Phase 5 installer in `core/cli`, where `install.go` already owns the marker sentinels and `.zshrc` topology scan. The ingest controller calls that package-private scan/result seam. No marker or zsh text crosses a package boundary and there is exactly one marker source of truth.
- **Alternatives:** Re-declare the marker constants in the ingest package (rejected: two sources of truth — a marker drift silently breaks detection); read markers from a config value (over-engineered for two fixed constants).
- **Why resolved:** The markers and installer are already in `core/cli/install.go`, and Phase 6's controller is also in `core/cli`; introducing another package seam would add indirection without changing ownership.
- **Impact:** Low — only package-private identifier placement remains discretionary. Detection/warning behavior and the single marker source of truth are locked.
- **Confidence:** High (verified against the landed package layout).

---

*Phase: 06-ingest-end-to-end*
*Logged: 2026-07-02 (--auto SPEC generation); OQ-06-03/OQ-06-04 added 2026-07-02 (autonomous smart-discuss)*
