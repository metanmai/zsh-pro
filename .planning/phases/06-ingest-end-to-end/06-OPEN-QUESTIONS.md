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
- **Tentative choice:** A small ingest-result DTO in `core/dto` rendered via a thin `core/render` path — consistent with the existing domain→DTO separation and the `analyze` precedent (`core/render/json.go` + `core/dto/envelope.go`). Fields include managed-entry count, unmanaged-statement count, optional unmanaged-source-line count, withheld list (name + line, NO value), warning list, ok/exit_code, and truthful transaction state.
- **Alternatives:** An inline `map[string]any`/anonymous struct marshaled directly in the CLI (lighter, but diverges from the established render/DTO seam and risks leaking store types onto the wire); reusing the `fail` envelope shape for success too (conflates error and success envelopes).
- **Why uncertain:** UX/serialization-surface choice; both satisfy the agent contract (exactly one JSON object on stdout). The `analyze` path establishes the DTO+renderer pattern, but ingest's payload is small enough that an inline struct is defensible.
- **Impact:** Low — the wire shape is trivially changeable; no requirement depends on the exact JSON structure, only that withheld + counts + warnings are present and no secret value / zsh text / store type reaches the wire.
- **Confidence:** Medium-High (the render/DTO seam is the established pattern; safe default follows it).

## OQ-06-04 — Seam for reading Phase 5 marker constants in the out-of-block scan

- **Question:** How does the out-of-block-append detector obtain Phase 5's BEGIN/END marker strings without duplicating them or crossing the composition-root layering line?
- **Tentative choice:** Co-locate the out-of-block detector with the Phase 5 installer (the package that already owns the marker sentinels + the `~/.zshrc` read-modify-write), so the marker text has a single source of truth; the ingest orchestration invokes it. If the detector must live in the ingest path and cross a package boundary, expose the marker/scan via a narrow seam (mirroring the `shell.Regenerator`/`shell.Hooker` provider-seam pattern) so `core/cli` holds no marker/zsh text.
- **Alternatives:** Re-declare the marker constants in the ingest package (rejected: two sources of truth — a marker drift silently breaks detection); read markers from a config value (over-engineered for two fixed constants).
- **Why uncertain:** The markers and installer now exist in `core/cli/install.go`; only the narrow internal scan/result seam remains an implementation choice.
- **Impact:** Low-Medium — determines where the detector lives and how it reaches the marker text; the BEHAVIOR (detect-and-warn, leave byte-identical) is locked independent of the seam. Reversible.
- **Confidence:** Medium (behavior locked; physical seam depends on Phase 5's landed installer package).

---

*Phase: 06-ingest-end-to-end*
*Logged: 2026-07-02 (--auto SPEC generation); OQ-06-03/OQ-06-04 added 2026-07-02 (autonomous smart-discuss)*
