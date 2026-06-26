# Phase 2: IR + Partial Evaluation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-26
**Phase:** 02-ir-partial-evaluation
**Areas discussed:** IR shape & granularity, Declarative/imperative routing, Regeneration order & oracle, Templating scope

---

## IR shape & granularity

### Entry vs Block

| Option | Description | Selected |
|--------|-------------|----------|
| Entry keeps verbatim text | Holds original Block.Text byte-for-byte + derived fields; lossless round-trip baseline | ✓ |
| Entry is a fresh transform (no raw text) | Copies only structured fields, rebuilds text via templates for every entry; risks silent drift | |
| You decide during planning | Defer, constrained by dependency-free model + verbatim success criterion | |

**User's choice:** Entry keeps verbatim text.

### Profile shape

| Option | Description | Selected |
|--------|-------------|----------|
| Single ordered list + category field | One list in source order; grouping is a computed view; preserves load order | ✓ |
| Buckets keyed by category | Per-category slices as storage; throws away cross-category source order | |
| You decide during planning | Defer | |

**User's choice:** Single ordered list + category field.

### PATH split

| Option | Description | Selected |
|--------|-------------|----------|
| Keep verbatim now, segment in Phase 4 | Store PATH as one verbatim value tagged dynamic; deltas computed in Ph4 | ✓ |
| Segment PATH in the IR now | Parse into ordered segment list as a first-class Entry part | |
| You decide during planning | Defer, EVAL-01 still applies | |

**User's choice:** Keep verbatim now, segment in Phase 4.

---

## Declarative/imperative routing

### Options gate (CatOptions grab-bag)

| Option | Description | Selected |
|--------|-------------|----------|
| Only setopt/unsetopt are declarative | Matches the spike's proven options class; the rest → master block | |
| setopt/unsetopt + zstyle declarative | Also admit zstyle; weaker guarantee (never proven reversible) | |
| You decide during planning | Defer, hard rule: not-proven-byte-reversible-in-spike ⇒ imperative | ✓ |

**User's choice:** You decide during planning.
**Notes:** Hard rule retained — precision over recall. Documented lean in CONTEXT: only setopt/unsetopt declarative; zstyle/autoload/compinit/compdef/zmodload → master block.

### Dynamic value routing (orthogonality)

| Option | Description | Selected |
|--------|-------------|----------|
| Stays declarative, value kept late-bound | Two axes orthogonal; dynamic value stored verbatim, applied literally, expanded per-machine | ✓ |
| Dynamic value forces imperative | Pushes portable env/PATH to master block; contradicts EVAL-01 | |
| You decide during planning | Defer | |

**User's choice:** Stays declarative, value kept late-bound.

### Uncertain gate

| Option | Description | Selected |
|--------|-------------|----------|
| Opaque OR below-high confidence | Declarative only if non-Opaque + admitted category + High confidence; safest but more in master block | |
| Opaque OR unknown-category only | Route by Kind/Category/CmdName membership; only Opaque + unknown go imperative; confidence not a gate | ✓ |
| You decide during planning | Defer, err toward unmanaged on doubt | |

**User's choice:** Opaque OR unknown-category only.
**Notes:** User directive — "If too many options are in the master block, the tool becomes less and less useful. Build it durable, handle as many cases as possible. But the user can move it themselves... Ball is in their court." Drove the maximize-managed stance (D-06) plus the per-entry override (D-07).

### Override scope

| Option | Description | Selected |
|--------|-------------|----------|
| IR field now, CLI/UX deferred | Entry carries ManagedOverride (auto/forced-managed/forced-unmanaged); flip command deferred to Ph5–6 | ✓ |
| Full manual-move in Phase 2 | Build field + a way to set it now; but no CLI surface yet → throwaway entry point | |
| Defer the whole override concept | No field in the IR; retrofitting later breaks the stored shape | |

**User's choice:** IR field now, CLI/UX deferred.

---

## Regeneration order & oracle

### Round-trip oracle

| Option | Description | Selected |
|--------|-------------|----------|
| Execute-and-diff state tables (reuse spike instrument) | Source original + regenerated under zsh -f, snapshot via introspectScript, assert byte-identical | ✓ |
| Re-parse idempotency (IR fixpoint) | Regenerate → re-parse → assert IR == IR; pure/fast but proves structure not behavior | |
| Byte-identical source text | Assert text == text; too brittle for a regenerable IR | |
| You decide during planning | Defer, must validate behavior + reuse spike instrument | |

**User's choice:** Execute-and-diff state tables (reuse spike instrument).

### Emit order

| Option | Description | Selected |
|--------|-------------|----------|
| Preserve original source order | Emit in source order; grouping stays an in-memory view; zero load-order risk | ✓ |
| Group by category (canonical load order) | Emit category blocks in Categories() order; risks cross-category order dependencies | |
| You decide during planning | Defer | |

**User's choice:** Preserve original source order.
**Notes:** Deliberate reading of the goal's "per-category .zsh" wording — per-category emit is deferred to Phase 4's manifest path.

---

## Templating scope

### Templating

| Option | Description | Selected |
|--------|-------------|----------|
| Template declarative, verbatim the rest | Declarative entries rebuilt from structured fields (oracle proves regeneration); imperative/Opaque verbatim; dynamic late-bound verbatim | ✓ |
| Everything verbatim this phase | Echo stored Text for all; round-trip becomes a tautology, defers regeneration risk to Ph4 | |
| You decide during planning | Defer, oracle must prove real regeneration | |

**User's choice:** Template declarative, verbatim the rest.

---

## Claude's Discretion

- Exact `Profile`/`Entry` field names, Go types, and package placement (within `core/model` + an IR-build seam consistent with existing layering).
- The precise declarative set inside `CatOptions` (D-04), under the hard byte-reversibility rule.
- The static/dynamic detector's implementation (no execution, no `util.ExpandHome`).
- Whether imperative entries are a distinct Entry kind/flag vs a category.

## Deferred Ideas

- CLI/UX to set the managed/unmanaged override (Ph5–6 / backlog).
- PATH add/delete segmentation vs a captured base (Phase 4).
- Per-category emit (Phase 4 manifest/activate).
- Secret exclusion from the committed profile — PROF-03 (Phase 6).
- keybindings / hooks / completion as future managed classes (stay in master block for v2.0).
