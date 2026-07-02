# Phase 6: Ingest End-to-End — Specification

**Created:** 2026-07-02
**Ambiguity score:** 0.16 (gate: ≤ 0.20)
**Requirements:** 5 locked
**Mode:** `--auto` (interview skipped — initial ambiguity already below gate; derived from ROADMAP 4 success criteria + REQUIREMENTS PROF-03 + Phase 2/3 landed seams + the Phase 5 SPEC boundary that explicitly hands ingest + out-of-block-append detection to Phase 6)

## Goal

`zsh-pro` ingests a real `~/.zshrc` end-to-end — parse → classify (declarative/imperative split) → partial-eval → regenerate → commit to the baseline (`main`) branch of the store — capturing declarative state, routing imperative run-once lines to an unmanaged master block (never silently dropped), excluding detected secrets by default with an explicit withheld report, keeping re-ingest idempotent, detecting installer appends that landed outside the managed block, and round-tripping the committed baseline back to a behavior-equivalent `.zshrc` (the Phase 2 oracle held end-to-end against a real file).

## Background

The composing pieces already exist on disk from Phases 2 and 3; Phase 6 is the on-ramp that wires them into one path against the **final** IR shape. Grounded current state:

- **IR (Phase 2):** `core/ir/Build(blocks []model.Block, c shell.Classifier) model.Profile` builds a categorized, regenerable `model.Profile` from parsed blocks; `core/ir/Regenerate(p model.Profile, r shell.Regenerator) []byte` regenerates behavior-equivalent zsh; `core/ir/route.go`'s `routeManaged` is the ING-02 declarative/imperative gate (imperative → verbatim `Text`, never templated). `model.Entry` carries `Text` (verbatim, D-01), `Category`, `Managed`, and `Override` (`EffectiveManaged()`). The byte-identical round-trip oracle exists (`core/ir/roundtrip_test.go`).
- **Store (Phase 3):** `core/store/Store` exposes `Init` (idempotent bare repo + `main` baseline), `Branches`, `Current`, `Checkout`, `Create`, `Commit(ctx, branch, p, msg) (WithheldReport, error)`, `Read`. `Commit` already runs secret exclusion (`excludeSecrets` → `SecretRef` + keychain/vault capture) and returns a `WithheldReport`. Serialization is lossless (`Read(Commit(p))` round-trip pinned).
- **Secret detection:** the shipped `CatSecrets` classifier verdict + parser `Dynamic` flag drive store-side exclusion (D-08); already-dynamic secrets commit verbatim.
- **Parser entry point:** `shell.Provider.Parse(src []byte) ([]model.Block, error)` (opaque-fallback, never errors to caller); `Classify` supplies the per-block category.

What does NOT exist yet (the Phase 6 delta):
- **No end-to-end ingest orchestration.** Nothing reads a real `~/.zshrc` file, drives `Parse → Build → (Regenerate) → Commit(main)` as a single flow, and reports the result. `core/cmd/zsh-pro/main.go` constructs the store but holds it unused pending the store-backed verbs.
- **No CLI ingest verb.** `core/cli/cli.go` dispatches only `analyze` from `Run`'s `switch args[0]`; there is no `ingest` verb.
- **No unmanaged master-block writer.** Imperative entries are flagged in the IR (`EffectiveManaged() == false`) and regenerated verbatim, but there is no path that routes them into a dedicated, preserved "master block" region of `.zshrc` for imperative run-once code.
- **No out-of-block installer-append detection.** Nothing inspects `~/.zshrc` for content a later installer appended outside the managed BEGIN/END block and surfaces it as a warning.

**Boundary with Phase 5 (locked):** Phase 5 (BOOT-01) owns the idempotent BEGIN/END `.zshrc` **block installer** — the writer, marker sentinels, byte-exact idempotency, and master-block *preservation* mechanism. The Phase 5 SPEC explicitly defers to Phase 6: (a) real `~/.zshrc` end-to-end ingest, and (b) out-of-block installer-append detection/warning. Phase 6 therefore **reuses** the Phase 5 installer's marker contract and master-block region — it does not re-implement the block writer — and adds the ingest pipeline that populates the store baseline, the imperative→master-block content routing, the ingest-side secret withheld report, the out-of-block-append detector, and the end-to-end round-trip oracle.

## Requirements

1. **End-to-end ingest to baseline**: Ingesting a real `~/.zshrc` produces a `main`-branch profile committed to the store, with declarative state captured and imperative lines routed to the unmanaged master block — never silently dropped.
   - Current: No orchestration wires `Parse → ir.Build → Store.Commit(main)`; imperative entries are IR-flagged but never routed to a master block.
   - Target: An ingest entry point reads a `~/.zshrc` file (default `~/.zshrc`, overridable by an explicit path argument), calls `provider.Parse` → `ir.Build`, commits the resulting `model.Profile` to the `main` (baseline) branch via `Store.Commit`, and routes every `EffectiveManaged() == false` (imperative) entry's verbatim `Text` to the unmanaged master block region rather than the templated declarative store. Every source statement is accounted for — either committed as a managed entry or routed to the master block; none is dropped.
   - Acceptance: Running ingest on a fixture `~/.zshrc` containing both declarative (alias/export/PATH) and imperative (`eval "$(...)"`, function definition invoked at load, `compinit`) lines creates/updates the `main` branch in the store; reading `main` back yields a `model.Profile` whose managed entries equal the declarative subset; every imperative line's verbatim text appears in the master-block output; a line count check confirms `managed_entries + master_block_lines` accounts for every non-blank, non-comment source statement (zero silently dropped).

2. **Secret exclusion + withheld report on ingest**: Detected secrets are excluded from the committed baseline by default and the user is told exactly what was withheld.
   - Current: `Store.Commit` runs `excludeSecrets` and returns a `WithheldReport`, but no ingest path surfaces that report to the user.
   - Target: The ingest path reuses the shipped secret detection end-to-end (via `Store.Commit`'s existing `excludeSecrets`); literal secrets are replaced by a `SecretRef` and captured to the keychain/vault backend, kept out of the committed blobs; the returned `WithheldReport` is surfaced to the user (stdout in human mode; structured in `--json` mode if JSON output is offered) naming what was withheld. Already-dynamic secrets commit verbatim.
   - Acceptance: Ingesting a fixture `~/.zshrc` containing a literal secret assignment (e.g. `export API_TOKEN=sk-abc123`) commits a baseline whose committed tree contains no occurrence of the literal secret value (`grep` of the committed blobs returns nothing), and the ingest output reports that variable as withheld; a fixture containing an already-dynamic secret (`export API_TOKEN=$(op read ...)`) commits that line verbatim and does not report it withheld.

3. **Idempotent re-ingest / re-install**: Re-running ingest/install leaves the managed `.zshrc` block byte-identical the second time.
   - Current: No idempotency guarantee across an ingest re-run (no ingest path exists).
   - Target: Running ingest a second time on an unchanged `~/.zshrc` produces a byte-identical managed BEGIN/END block (reusing the Phase 5 installer's byte-exact rewrite) and does not append a duplicate block or duplicate committed entries; content outside the managed markers is preserved byte-for-byte.
   - Acceptance: Running ingest twice on the same fixture leaves the region between the BEGIN/END markers byte-identical after the second run (`diff` of the marked region is empty; `grep -c` of the BEGIN marker returns 1), and bytes outside the markers are unchanged (`diff` of the non-marker region is empty).

4. **Out-of-block installer-append detection**: Installer appends that landed outside the managed block are detected and surfaced as a warning rather than clobbered.
   - Current: No inspection of `~/.zshrc` for out-of-marker appends; nothing warns.
   - Target: On ingest/install, the tool scans `~/.zshrc` for content added **after** the managed BEGIN/END block by a later installer append (e.g. a version-manager or tool installer that appends its own lines to the end of the file). Such out-of-block content is detected and surfaced to the user as a warning (identifying that lines exist outside the managed markers that may need manual attention) and is left in place — never silently clobbered or moved.
   - Acceptance: On a fixture `~/.zshrc` that has the managed BEGIN/END block followed by trailing lines an installer appended (e.g. `# added by nvm` + `export NVM_DIR=...`), ingest emits a warning naming that out-of-block content exists, exits without error, and leaves those trailing lines byte-identical in the file (no clobber, no reorder).

5. **End-to-end round-trip equivalence**: The committed baseline round-trips to a behavior-equivalent `.zshrc` — the Phase 2 round-trip oracle holds against a real, end-to-end ingested file.
   - Current: The Phase 2 byte-identical round-trip oracle exists for synthetic IR fixtures; it has never been exercised against a real, end-to-end ingested `~/.zshrc`.
   - Target: After ingest commits `main`, regenerating from the committed `model.Profile` (`Store.Read(main)` → `ir.Regenerate`) reproduces a behavior-equivalent `.zshrc` — untouched statements emitted verbatim (`Block.Text`), only rewritten declarative slices templated — passing the Phase 2 round-trip oracle applied to a real ingested file.
   - Acceptance: A test ingests a representative real-shaped `~/.zshrc` fixture, reads back `main`, regenerates, and asserts the Phase 2 round-trip oracle passes (regenerated output parses to a `model.Profile` structurally equal to the committed one; verbatim/imperative statements are byte-identical to source; templated declarative slices are behavior-equivalent).

## Boundaries

**In scope:**
- An end-to-end ingest entry point: read `~/.zshrc` (default path, path-overridable) → `provider.Parse` → `ir.Build` → `Store.Commit(main)`.
- A CLI verb that drives ingest (see OQ-06-01 for the verb name default).
- Routing imperative (`EffectiveManaged() == false`) entries' verbatim `Text` into the unmanaged master-block region (reusing Phase 5's master-block/marker contract).
- Surfacing the `WithheldReport` from `Store.Commit` to the user on ingest (reusing the shipped secret detection + store-side exclusion end-to-end).
- Idempotent re-ingest (byte-identical managed block on re-run; no duplicate commits/blocks) — reusing the Phase 5 installer's byte-exact rewrite.
- Out-of-block installer-append **detection + warning** (read-only surfacing; leaves the appended content untouched).
- An end-to-end round-trip test that exercises the Phase 2 oracle against a real-shaped ingested `~/.zshrc` fixture.
- Composition-root wiring of the ingest verb into the CLI at `core/cmd/zsh-pro/main.go` (the sole `core/shell/zsh` + concrete-store importer).

**Out of scope:**
- The BEGIN/END `.zshrc` block **writer/installer** itself (marker sentinels, byte-exact idempotent rewrite, master-block preservation mechanism) — that is Phase 5 (BOOT-01); Phase 6 consumes it.
- The IR, partial-eval, classifier, and regenerator internals — those are Phase 2 (reused verbatim; Phase 6 does not modify `core/ir` or `core/shell/zsh` parse/classify/regen logic beyond wiring).
- Store internals (git driver, serialization, secret-exclusion mechanism, keychain/vault) — those are Phase 3 (reused verbatim).
- Runtime deref-on-switch of `SecretRef` at apply time — the runtime half of PROF-03, handled in the Phase 4/5 activation path; Phase 6 only excludes-and-reports at ingest time.
- Multi-file config graphs (sourced fragments, `*.zsh` beyond the single entry point) — explicitly out of scope for the whole milestone (single-entry ingest only).
- Filesystem/live-`$HOME`/`$(...)` resolution — dynamic values stay late-bound verbatim (EVAL-01 invariant); ingest performs no execution of user config (static AST inspection only).
- Other shells (bash/fish) — the model is zsh-specific for the whole milestone.
- Auto-activate on `cd`, remote profile sharing/trust — future (AUTO-01/SHARE-01).

## Constraints

- **No new dependencies** — ingest composes the already-present parser (`mvdan.cc/sh/v3` via the `Provider` seam), `core/ir`, `core/store` (which shells out to `git`), and the shipped secret detection. No new Go module.
- **Static-only partial-eval / no execution** — ingest performs static AST inspection only; it never `eval`s or executes the user's `~/.zshrc`, and `util.ExpandHome` is never used inside the IR path. Dynamic values (`$HOME`, `${...}`, `$(...)`, backticks, conditionals) stay late-bound verbatim (EVAL-01).
- **Precision over recall on the declarative/imperative gate** — when classification is uncertain, the entry routes imperative (unmanaged master block); a misclassified imperative line is a zero-residue violation by construction, so the ING-02 gate stays strict (a lossy template is never safe; a verbatim round-trip always is).
- **`Block.Text` is the round-trip anchor** — imperative and untouched statements are emitted verbatim from `Text`; only rewritten declarative slices are templated (ING-01).
- **Single composition root** — the ingest verb is wired into the CLI only at `core/cmd/zsh-pro/main.go`; `core/cli` depends on the `shell.Provider` interface and receives the store via injection, never importing the concrete provider/store drivers. `core/ir`/`core/store` stay shell-agnostic.
- **Reuse the Phase 5 installer, do not re-implement it** — the BEGIN/END marker contract, byte-exact idempotent rewrite, and master-block preservation come from Phase 5 (BOOT-01). Phase 6's idempotency and out-of-block detection are built on top of that contract, not a second block writer.
- **Reuse the shipped secret detection end-to-end** — no new regex/AST; exclusion goes through `Store.Commit`'s existing `excludeSecrets` (PROF-03 store-side half, completed here end-to-end for the real-file path).
- **Testing: TDD.** The end-to-end round-trip oracle (Phase 2 oracle against a real ingested file), idempotency (byte-identical managed block), and secret-withhold/out-of-block-detection properties are falsifiable and pinned by tests; `make check` (fmt-check + vet + lint + test) stays green. The Phase 2 round-trip oracle remains the ING-01 regression pin.

## Acceptance Criteria

- [ ] Ingesting a mixed declarative/imperative `~/.zshrc` fixture commits/updates the `main` branch; managed entries equal the declarative subset and every imperative line's verbatim text appears in the master-block output; no source statement is silently dropped (`managed_entries + master_block_lines` accounts for every non-blank, non-comment statement).
- [ ] Ingesting a `~/.zshrc` with a literal secret commits a baseline whose committed tree contains no occurrence of the secret value (`grep` returns nothing) and the ingest output reports it withheld; an already-dynamic secret commits verbatim and is not reported withheld.
- [ ] Running ingest twice on an unchanged file leaves the managed BEGIN/END region byte-identical (`diff` empty, `grep -c` BEGIN == 1) and leaves bytes outside the markers unchanged.
- [ ] On a `~/.zshrc` with content appended outside the managed block, ingest warns that out-of-block content exists, exits without error, and leaves that content byte-identical (no clobber, no reorder).
- [ ] After ingest, `Store.Read(main)` → `ir.Regenerate` passes the Phase 2 round-trip oracle against a real-shaped ingested fixture (regenerated output structurally equal to committed profile; verbatim/imperative statements byte-identical; declarative slices behavior-equivalent).
- [ ] The ingest verb is dispatched from the CLI and wired at `core/cmd/zsh-pro/main.go` only; `core/cli`, `core/ir`, and `core/store` do not import `core/shell/zsh`.

## Ambiguity Report

| Dimension          | Score | Min  | Status | Notes                                                                                          |
|--------------------|-------|------|--------|------------------------------------------------------------------------------------------------|
| Goal Clarity       | 0.88  | 0.75 | ✓      | 4 explicit ROADMAP success criteria; deliverables named (ingest path, imperative→master routing, secret withhold, out-of-block detect, round-trip oracle) |
| Boundary Clarity   | 0.85  | 0.70 | ✓      | Phase 5 SPEC explicitly hands ingest + out-of-block-append detection to Phase 6; installer stays Phase 5; in/out explicit |
| Constraint Clarity | 0.80  | 0.65 | ✓      | No new deps; static-only no-exec; precision-over-recall gate; `Block.Text` anchor; single composition root; reuse Phase 5 installer + shipped secret detection |
| Acceptance Criteria| 0.82  | 0.70 | ✓      | Falsifiable: byte-identical block, grep-verified secret absence, no-clobber out-of-block, round-trip oracle, zero-dropped line accounting |
| **Ambiguity**      | 0.16  | ≤0.20| ✓      | Below gate; interview skipped under --auto (Step 3)                                             |

Status: ✓ = met minimum, ⚠ = below minimum (planner treats as assumption)

**No dimension is below minimum.** Two sub-decisions were auto-resolved to safe/reversible defaults and logged in `06-OPEN-QUESTIONS.md` (OQ-06-01 ingest verb name → `ingest`; OQ-06-02 master-block physical location + relationship to the Phase 5 installer's block → the master block is a distinct, installer-preserved region of `~/.zshrc` holding imperative verbatim text, reconciled against the Phase 5 installer's actual marker/region contract at plan time). Neither blocks planning.

## Interview Log

| Round | Perspective    | Question summary                          | Decision locked                                                                     |
|-------|----------------|-------------------------------------------|-------------------------------------------------------------------------------------|
| —     | (auto-derived) | Initial ambiguity ≤ 0.20 → interview skipped per Step 3 | SPEC derived from ROADMAP 4 success criteria + PROF-03 + Phase 2/3 landed seams + the Phase 5 SPEC boundary (Phase 5 owns the installer; Phase 6 owns ingest + out-of-block detection) |
| auto  | Researcher     | What exists vs the Phase-6 delta?         | `ir.Build`/`ir.Regenerate`/round-trip oracle (Ph2) + `Store.Commit` with WithheldReport + secret exclusion (Ph3) all landed; missing: end-to-end orchestration, ingest CLI verb, imperative→master-block routing, out-of-block-append detection |
| auto  | Simplifier     | Irreducible core?                         | 5 requirements: end-to-end ingest to baseline, secret withhold report, idempotent re-ingest, out-of-block detection/warning, end-to-end round-trip oracle |
| auto  | Boundary Keeper| What is NOT this phase?                   | The BEGIN/END block writer/installer → Phase 5 (consumed here); IR/store internals → Phase 2/3 (reused verbatim); runtime secret deref → Phase 4/5; multi-file graphs + other shells + auto-cd + sharing → out/future |
| auto  | Failure Analyst| What invalidates ingest correctness?      | A silently dropped imperative line (zero-residue violation), a leaked literal secret in a committed blob, a duplicated/drifted managed block on re-ingest, clobbering an out-of-block installer append, a round-trip that changes behavior — each pinned as an acceptance criterion |

---

*Phase: 06-ingest-end-to-end*
*Spec created: 2026-07-02*
*Next step: /gsd:discuss-phase 6 — implementation decisions (ingest verb flag/arg design, imperative→master-block wiring against the Phase 5 marker contract, WithheldReport rendering format, out-of-block-append scan strategy, real-shaped round-trip fixture selection)*
