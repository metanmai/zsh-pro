# zsh-pro — Graph-Based Config Test Generator Design Spec

**Status:** Approved design — ready for implementation planning
**Date:** 2026-06-23
**Scope:** A test-only subsystem that generates random zsh configs from a typed dependency graph, with a built-in oracle (expected analysis) for correctness testing plus a mutation-based fuzz mode for robustness. No change to the production engine. Lives under `core/testgen` + a `zsh-gen` CLI; drives the existing `analyze` engine in tests.

---

## Motivation

The golden corpus is thin: 6 small fixtures, and **2 of the 4 issue kinds (`duplicate_path`, `shadowed`) have no coverage at all**. Two hand-crafted "serious" cases written during validation immediately surfaced two real analyzer defects:

1. **Line mis-attribution** — an issue's *first* occurrence reports the line of the **leading comment**, not the actual statement (a block is `[leading comments + statement]` and issues report the block's `StartLine`). E.g. `alias gs` on lines 4 & 5 reports `[1, 5]`.
2. **Whole-file opaque fallback** — one unparseable construct collapses the *entire* file into a single opaque `misc` block, losing all analysis of the valid sections.

Hand-writing fixtures does not scale and is precisely the activity that misses edge cases. We want a generator that produces a `.zsh` file **and** its known-correct expected analysis from the same source of truth, so we can assert correctness over thousands of seeds — and a fuzz mode that hammers the "never crash, never mislead" guarantee.

## Decisions (locked with the user)

| Decision | Choice | Rationale |
|---|---|---|
| Primary goal | **Oracle core + fuzz mode** | Oracle proves *correctness* at scale (graph encodes known defects → expected analysis is free); fuzz proves *robustness* (survival on malformed input). Shared machinery. |
| Form / integration | **Shared lib + property test + CLI** | A `core/testgen` library (graph → `.zsh` + expected `Analysis`) consumed by an in-process Go property test (CI-native) **and** a `zsh-gen` CLI that dumps `.zsh` + expected manifests to disk for inspection / corpus seeding. |
| Generation engine | **Hand-rolled on stdlib `math/rand`** | Zero new dependencies (the project guards its single dep, mvdan/sh). A typed-graph generator is domain-specific, so a generic property-testing lib adds little leverage. Seed is logged for replay; domain-specific shrinking is a deferred fast-follow, not v1. |

## The graph model (source of truth)

A `ConfigGraph` is a set of **typed nodes** (shell entities) and **directed edges** (`depends-on`).

**Nodes**
- `EnvVar{Name, Value}` → `export NAME="Value"` — category `environment`
- `Alias{Name, Value}` → `alias name='Value'` — category `aliases`
- `Function{Name, Body}` → `name() { Body }` — category `functions`
- `PathEntry{Dir}` → `export PATH="Dir:$PATH"` — category `path`
- `Command{Kind, Args}` → `bindkey…` / `setopt…` / `source…` / `eval "$(tool init)"` — category `keybindings` / `options` / `plugins`
- `Secret{Name, Value}` → `export NAME="Value"` where `Name` matches the classifier's secret regex — category `secrets`

**Names are drawn from disjoint, classification-safe pools per node type** so the oracle's category prediction always matches the classifier's actual rules (e.g. a name containing `PATH` classifies as `path`, a name matching `TOKEN|SECRET|API_KEY|…` classifies as `secrets`). The generator must mirror `zsh.Classify` exactly; the safe pools guarantee no accidental cross-classification.

**Edges (`depends-on`)** — an `EnvVar` whose value references `$OTHER`; a `Function` that calls another function/alias; a `PathEntry` built from `$HOME`. Edges induce a **topological emission order**, which (a) makes configs realistic and (b) sets up order-sensitivity defects (a forward reference / cycle) as a fast-follow.

**Defects are planted as known graph patterns — this is the oracle's answer key:**

| Planted pattern | Oracle expects |
|---|---|
| two `Alias` nodes, same name | `duplicate_alias` (name, both lines) |
| two `EnvVar` nodes, same name | `reassigned_env` (name, both lines) |
| two `PathEntry` nodes, same `Dir` | `duplicate_path` (segment, both lines) |
| an `Alias` + a `Function` sharing a name | `shadowed` (name, both lines) |
| ≥1 `Secret` node | `has_secrets: true` |

## From one graph to both artifacts

1. **`(*ConfigGraph).RenderZsh() ([]byte, LineMap)`** — topo-sorts nodes, emits each as zsh syntax, optionally interleaving comments/blank lines, and records the line each node lands on (`LineMap`). Interleaving comments is deliberate: it exercises the block-segmentation path that produced Finding 1.
2. **`(*ConfigGraph).Expected() model.Analysis`** — derives the **known-correct** analysis from the graph + `LineMap`: per-category counts, the exact issue set (kind + name + correct statement lines), `has_secrets`.
3. **Oracle test** — `analyze(rendered)` compared against `Expected()` over many seeds.
4. **Fuzz mode** — mutate the rendered bytes, assert survival only (below).

### Assertion strategy (honest about the two known bugs)

- **Strict from day one** (must pass): category set + per-category counts; `opaque_blocks == 0` (valid mode); `has_secrets`; and the **issue set keyed by `(kind, name)`**.
- **Bug-exposing** (encode *correct* values, expect to surface known defects): the total `Lines` field (off-by-one, found in validation) and issue `lines` arrays (Finding 1). These land as assertions gated behind a flag / reported as "known divergence," so the strict core stays green while the two inaccuracies are documented in code and `docs/BACKLOG.md`. Fixing either is a deliberate `--json` wire-contract change (it alters emitted `lines`/`lines` values and the golden corpus), tracked separately — **not** in scope here. When fixed, the gated assertions flip to strict.

This makes the generator double as a regression pin for the two findings without conflating "build the generator" with "change the output contract."

## Fuzz mode

Take a valid rendered graph and apply mutation operators: drop a closing token (`fi`/`done`/`}`), unbalance quotes, truncate mid-construct, inject random and non-UTF-8 bytes, explode nesting depth. Then assert **survival only** — no oracle, since output is unpredictable:
- the process never panics (exit ∈ {0,1,3}; never a Go panic / non-graceful crash);
- `--json` always emits exactly one **valid JSON** object with an `ok` field;
- human mode never emits a partial/garbled report.

The seed is logged on any failure for exact replay.

## Determinism & reproducibility

All randomness flows from one injected `*math/rand.Rand` (seeded from a CLI flag or, in the property test, from a fixed seed list + a bounded number of random seeds so CI is deterministic). **On failure the seed is logged**, and re-running with that seed reproduces the exact graph. Automatic shrinking to a minimal failing graph is deferred; the documented manual path is "replay the seed, then delete nodes until it passes."

## Target layout (one type per file; same OOP idiom as the engine)

```
core/testgen/
  graph.go      ConfigGraph + node/edge types; intrinsic methods (AddNode, edges, topo order)
  generator.go  Generator{rng}; New(rng); (*Generator).Build(GenParams) *ConfigGraph
                GenParams = node counts per type + which defects to plant (seeded construction)
  render.go     (*ConfigGraph).RenderZsh() ([]byte, LineMap)
  oracle.go     (*ConfigGraph).Expected() model.Analysis
  mutate.go     Mutator{rng}; (*Mutator).Corrupt([]byte) []byte   — fuzz operators
  testgen_test.go  property tests: oracle (strict core) + fuzz (survival), seed-logged
core/cmd/zsh-gen/
  main.go       CLI composition root: emit N random .zsh + expected manifests to a target dir
```

## Architecture rules (consistent with the engine's amended rules)

1. **Leaves stay leaves.** `core/testgen` imports only `core/model` (a leaf) to express `Expected()`. It does **not** import `analyze`/`render`/`cli`. The *property test* (`testgen_test.go`) imports `analyze` + `shell/zsh` + `testgen` to close the loop — tests may reach across layers; the library may not.
2. **Behavior lives on role types.** `Generator`, `ConfigGraph`, `Mutator` carry their logic as methods; no loose functions doing work. Name pools / syntax templates are immutable package-level `var`s (the idiom call already used for `secretRe`/`pluginHints`).
3. **No global mutable state.** The PRNG is injected (`New(rng)`), never package-global — this is what makes runs reproducible.
4. **`cmd/zsh-gen` is a thin composition root** — flag parsing + wiring + file I/O only; all generation logic lives in `testgen`.
5. **Errors returned, never panicked**; the CLI writes only inside its target dir.

## Integration with the existing corpus

The `zsh-gen` CLI emits, per case, a `<name>.zsh` plus an expected-results entry in the **same `manifests.json` schema** the golden corpus already uses (`core/testdata/fixtures/manifests.json`). Generated cases can therefore be promoted into the permanent corpus by copying the files in — the existing `corpus_test.go` harness validates them unchanged. (Promotion is manual and out of scope to automate here.)

## Testing & safety

- The property test runs inside `go test` with a bounded iteration count and a fixed seed list (deterministic in CI) plus a small number of random seeds. Total wall-clock stays within the suite's norms; each generated config sources under `zsh -f` well within the 5s introspection timeout.
- Fuzz iterations are bounded; mutated bytes are written only to temp paths.
- The generator never shells out except through the engine under test.

## Out of scope (v1)

- **Order-sensitivity defects** (forward references, PATH precedence, `fpath`/`compinit` ordering) — the edges model it; a fast-follow once the core lands.
- **Automatic shrinking** — seed-replay only for now.
- **Fixing the two analyzer findings** — deliberate wire-contract changes, tracked separately in `docs/BACKLOG.md`; this effort only *exposes/pins* them.
- **Tier-2 real-world configs** and **non-zsh** generation.
```
