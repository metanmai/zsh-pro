# Phase 2: IR + Partial Evaluation - Context

**Gathered:** 2026-06-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the **IR spine** the rest of the milestone serializes against: a near-lossless, regenerable `model.Profile`/`model.Entry` constructed from the parser's existing `[]model.Block` via the reused `Parser`/`Classifier` seam. Each entry is classified **declarative (switchable)** vs **imperative (unmanaged)** — the switchability gate (ING-02) — and tagged **static vs dynamic** for portability (EVAL-01). The IR regenerates a **behavior-equivalent** `.zsh`, with untouched/imperative statements emitted verbatim and declarative slices rebuilt through a templater.

**In scope:** the `Profile`/`Entry` types; IR construction from `[]model.Block`; the declarative/imperative routing gate; the static/dynamic partial-eval pass (static AST inspection only, no execution); per-entry templated regeneration; the round-trip equivalence oracle.

**Not in scope (later phases):** git-backed store (Ph3); the runtime `Manifest` build + per-category emit + activate/deactivate (Ph4); the loader/CLI/bootstrap (Ph5); secret exclusion from the committed tree (Ph6, PROF-03); the CLI/UX to flip a manual managed/unmanaged override (Ph5–6); PATH add/delete segmentation (Ph4). No filesystem/`$HOME`/`$(...)` resolution, ever (`util.ExpandHome` must not appear in the IR).

</domain>

<decisions>
## Implementation Decisions

### IR shape & granularity
- **D-01: Entry keeps the verbatim source text.** Each `Entry` holds the original `Block.Text` byte-for-byte **plus** the Phase-2 derived fields (declarative/imperative verdict, static/dynamic tag, category, override). Regenerating an untouched/imperative entry is "print the stored text" — lossless by construction, and the round-trip oracle starts from a guaranteed-correct baseline. (Rejected: a fresh transform that discards the Block — risks silent drift against the "near-lossless / verbatim" requirement.)
- **D-02: `Profile` stores a single ordered list in source order; category is a field, not the storage.** "Grouped by category" (ING-01 wording) is a computed view/iteration over that list, not the layout. Source order is the source of truth — this defuses the cross-category load-order hazard up front and stays trivially lossless. (Rejected: per-category buckets as storage — throws away cross-category order.)
- **D-03: PATH-like values are kept verbatim in Phase 2; segmentation is deferred to Phase 4.** A `PATH`/`FPATH` assignment is stored as one verbatim value like any other env entry (tagged dynamic when it contains `$HOME`/`${...}`/`$(...)`). The add/delete delta computation vs a captured base (the validated `lists[].additions/deletions` shape) lives in Phase 4, where the manifest and captured base actually exist. EVAL-01 static/dynamic tagging still applies to PATH values here.

### Declarative/imperative routing (the switchability gate — ING-02)
- **D-04: Within the `CatOptions` grab-bag, the declarative set is Claude's discretion at planning — under a hard rule: if a command's reversal was not proven byte-identical in the Phase 1 spike, it defaults to imperative (precision over recall).** Documented lean: only `setopt`/`unsetopt` are declarative (the spike's proven options class); `zstyle`, `autoload`, `compinit`, `compdef`, `zmodload` → master block.
- **D-05: Declarative/imperative and static/dynamic are ORTHOGONAL axes.** A declarative entry whose value is dynamic (`export GOPATH=$HOME/go`, `export BREW=$(brew --prefix)`) **stays declarative/switchable** and is **separately** tagged dynamic so the value is stored verbatim and never resolved. The manifest/loader applies the literal string; zsh expands it per-machine at apply time. A dynamic value is **never** grounds for banishing an entry to the imperative master block — that would gut EVAL-01 and shrink the switchable surface.
- **D-06: Maximize the switchable surface; route conservatively only when truly unrecognizable.** Default routing keys off `Block.Kind`/`Category`/`CmdName` membership in the 5 admitted reversible classes (env / PATH / aliases / functions / options). **Only `Opaque` blocks and genuinely-unknown categories default to imperative.** Confidence is **not** used as a blunt gate that dumps medium-confidence declaratives into the master block. Rationale (user, 2026-06-26): a master block that swallows everything makes the tool useless — its value is how much it can manage. The byte-identical reverse (spike instrument) remains the correctness backstop: managing more is safe precisely because every managed class has a proven mechanical reverse, and a *truly* imperative line wrongly managed is the one real bug (ING-02), which the override (D-07) lets the human correct.
- **D-07: The IR carries a per-entry override field now; the CLI/UX to set it is deferred.** `Entry` carries something like `ManagedOverride: auto | forced-managed | forced-unmanaged`. The auto-verdict is only the **default**; a manual override **wins** and **persists** through round-trip and into Ph3's store / Ph4's manifest. The user is the final arbiter — they can pull an entry into a managed bucket or push it out to the master block. The command to flip it (and any review-the-master-block UX) is deferred to the CLI phases (Ph5–6). Building the field now avoids a breaking change to the stored shape later.

### Regeneration order & equivalence oracle
- **D-08: The round-trip oracle executes and diffs state tables under sandboxed `zsh -f`, reusing the Phase 1 instrument.** Source the original `.zsh` and the regenerated `.zsh` each under `zsh -f`, snapshot the state tables (env / aliases / functions / `$path` / options) via the existing `introspectScript`, and assert **byte-identical**. This measures actual behavior (the project's standing determinism bar), tolerates safe whitespace/structure differences, and reuses the proven spike harness. (Note: this is a zsh-requiring test, like `corpus_test.go`; the pure `testgen` property oracle stays pure and composes alongside it.) Rejected: byte-identical source text (too brittle for a regenerable IR); re-parse idempotency alone (proves IR stability, not shell behavior).
- **D-09: Phase 2 regeneration emits in original source order — not regrouped by category.** Zero load-order risk; behavior-equivalence is nearly free. The goal's "per-category `.zsh`" wording is satisfied by the IR being *organized* so grouping is a view (D-02); the actual per-category **emit** is a Phase 4 manifest/activate concern, not a Phase 2 round-trip concern. This is a deliberate reading of the goal wording, recorded here so the planner doesn't regroup prematurely.

### Templating scope this phase
- **D-10: Template declarative entries (rebuild from structured fields); emit everything else verbatim.** Declarative entries (env / PATH / aliases / functions / options) are emitted by the templater **from their structured fields** (`Names`, value, kind), so the oracle genuinely proves the IR can *regenerate* the switchable surface — not merely echo stored text (which would make the round-trip a tautology). Imperative/unmanaged entries and anything `Opaque` stay verbatim. Dynamic values are kept **late-bound verbatim within** the templated output (the template emits `$HOME/go`, never the resolved path).

### Claude's Discretion
- The exact `Profile`/`Entry` field names and Go types, package placement (within `core/model` + a new IR-build seam consistent with the existing layering), and the templater's internal structure — provided D-01 (verbatim text retained), D-02 (ordered list), and the `core/model`-stays-dependency-free constraint hold.
- The precise declarative set inside `CatOptions` (D-04), under the stated hard rule.
- The static/dynamic detector's exact implementation, provided it triggers on `$HOME`/`${...}`/`$(...)`/backticks/conditionals and performs **no execution** and **no `util.ExpandHome`**.
- Whether imperative entries are represented as a distinct `Entry` kind/flag vs a category — provided the round-trip and override semantics hold.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase contract
- `.planning/ROADMAP.md` §"Phase 2: IR + Partial Evaluation" — goal + the 4 success criteria (the scope anchor).
- `.planning/REQUIREMENTS.md` — **ING-01** (categorized, regenerable, verbatim-preserving ingest), **ING-02** (declarative vs imperative; misclassification = zero-residue violation), **EVAL-01** (static = syntactically constant; dynamic kept late-bound; portability preserved).

### What the IR must feed (Phase 1 spike outputs — load-bearing)
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md` — the **validated `Manifest` JSON shape** that becomes Phase 4's runtime undo record (env scalars w/ drift guard · `lists[].additions/deletions` for PATH/FPATH · `aliases.added/shadowed` · `functions.added/shadowed` · `options.enabled/was_on`). The IR must carry enough to *produce* this. Also documents which classes are excluded (compinit/keybindings/hooks → master block).
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md` — go/no-go writeup; the 5 admitted reversible classes; the byte-identical bar; the drift guard; the `LOCAL_OPTIONS`/`emulate -L` option-auto-revert trap.

### Reusable engine code (the IR front-end + oracle instrument)
- `core/model/block.go` — the `Block` (Text, StartLine, Kind, CmdName, Names, Exported, Opaque, Category, Conf) the IR is built from.
- `core/model/category.go` — the `Category` taxonomy + `Categories()` canonical load order.
- `core/shell/zsh/classify.go` — the current `Block → Category` classifier (reused as the IR's classification front-end; note the `CatOptions` grab-bag at lines 44–47 relevant to D-04).
- `core/shell/zsh/parse.go` — the parser producing `[]model.Block` (opaque-fallback behavior).
- `core/shell/zsh/introspect.go` — `introspectScript` + the `zsh -f -c` + timeout + graceful-degrade subprocess pattern; the **state-table snapshot instrument** the round-trip oracle (D-08) reuses.
- `core/shell/provider.go` — the `Parser`/`Classifier`/`Provider` ISP seam the IR build composes with (must not bypass it).
- `core/testgen/property_test.go` — the existing oracle/regression pin the new round-trip oracle composes alongside.
- `core/testgen/render.go` — the project's existing string-templated zsh codegen pattern; precedent for the declarative templater (D-10) rather than the mvdan/sh printer.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `introspectScript` (`core/shell/zsh/introspect.go`): dumps aliases/functions/exported-env/`$path`/on-options under `zsh -f` — directly reusable as the round-trip oracle's before/after capture (D-08).
- `core/testgen/render.go`: string-templated zsh codegen — the precedent the declarative templater (D-10) should follow.
- `zsh.Provider.Parse`/`Classify` (`core/shell/zsh/`): the IR build's front-end; reuse via the `shell.Provider` seam, do not re-parse.

### Established Patterns
- `core/model` stays dependency-free; `core/analyze` stays shell-free (interface seam only). The IR types belong in `core/model`; any IR-build orchestration composes with the `Provider` seam at the right layer, never importing `core/shell/zsh` outside the composition root.
- Sandboxed subprocess: `exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)` + timeout + graceful degradation — the oracle harness follows this shape (and skips cleanly when zsh is absent, like `corpus_test.go`).
- TDD with the `testgen` oracle as the regression pin; the new round-trip oracle is the Phase 2 correctness gate.

### Integration Points
- Input: `[]model.Block` from the existing parser/classifier.
- Output consumed downstream: Ph3 (git store serializes the `Profile`), Ph4 (manifest builder reads the IR to produce the validated `Manifest` shape + per-category emit). The per-entry override (D-07) and verbatim text (D-01) must survive serialization into Ph3/Ph4.

</code_context>

<specifics>
## Specific Ideas

- **User design stance (2026-06-26):** "If too many options are in the master block, the tool becomes less and less useful. Build it to be durable and handle as many cases as possible. But the user can, in the end, move it themselves... Manually move it into a bucket if they wish. Ball is in their court." → drove D-06 (maximize managed surface) + D-07 (per-entry override; human is final arbiter).
- **Determinism is a measurable bar** (carried from Phase 1): the round-trip oracle reuses the spike's execute-and-diff instrument rather than asserting equivalence structurally — behavior is measured, not claimed (D-08).
- **"Regenerable" must be a tested property, not a claim** → declarative entries are rebuilt through the templater so the oracle exercises real regeneration (D-10), not a verbatim echo tautology.

</specifics>

<deferred>
## Deferred Ideas

- **CLI/UX to set the managed/unmanaged override** (flip `ManagedOverride`, review the master block) — the IR field ships in Phase 2 (D-07); the command to set it belongs to the CLI phases (Ph5–6) or backlog.
- **PATH add/delete segmentation** into `additions`/`deletions` vs a captured base — Phase 4 (manifest builder), where the captured base exists (D-03).
- **Per-category emit** (category-grouped `.zsh` output) — Phase 4 manifest/activate path; Phase 2 emits in source order (D-09).
- **Secret exclusion from the committed profile** (PROF-03) — Phase 6; the IR may *tag* `CatSecrets` entries but does not implement exclusion here.
- **keybindings / hooks / completion (`compinit`) as future managed classes** — stay in the master block for v2.0 (spike verdict); reconsider only if proven byte-reversible later.

</deferred>

---

*Phase: 2-ir-partial-evaluation*
*Context gathered: 2026-06-26*
