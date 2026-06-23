# zsh-pro — OOP Layering Refactor Design Spec

**Status:** Approved design — ready for implementation planning
**Date:** 2026-06-23
**Scope:** Behavior-preserving restructure of the existing read-only analyze engine. No new features; public CLI behavior and `--json` output are unchanged.

---

## Motivation

Three organizational directives from the user, in priority order of how they reshape the code:

1. **Attach behavior to types (OOP).** No free-standing functions doing work in production code — every function becomes a method on the cohesive type that owns that job. "I don't want unassociated functions just lying around."
2. **DTOs in their own files, separate from the domain ("class") structs.** Today the `model` structs carry the `json` tags themselves — they *are* the wire format. The wire format must become its own layer.
3. **Helpers / utils in their own layer.**

These refine the project's founding principle — "separation of concerns, loosely coupled, modularized, not spaghettified" — without changing what the engine does.

## Decisions (locked with the user)

| Decision | Choice | Rationale |
|---|---|---|
| Where behavior lives | **Role types + thin model** | Logic sits on `Analyzer`/`Reconciler`/`Renderer`/`zsh.Provider`/`CLI` as methods. Domain structs keep only *intrinsic* methods. Idiomatic Go; no god-object; keeps `model` pure. |
| DTO separation depth | **Separate DTO types + mapping** | A `dto` package holds wire structs (the `json` tags); `model` loses its tags and becomes pure domain; the `JSONRenderer` maps domain → dto. |

The rejected alternative for #1 — a **rich domain model** (analysis/render logic as methods on `Block`/`Analysis`) — would turn `Analysis` into a god-object and pull cross-layer logic into `model`, recreating the coupling we set out to avoid.

## Target layout (one type per file; layers explicit)

```
core/
  model/        PURE domain data + intrinsic methods only. No json tags, no logic, no third-party imports.
    category.go     Category, consts, Categories() enumerator, (Category).Description()
    block.go        Block, BlockKind, Confidence
    identityset.go  IdentitySet
    issue.go        Issue, IssueKind
    analysis.go     CategorySummary, Analysis, (Analysis).ExitCode()
  dto/          PURE wire-format structs (json tags). Zero internal deps — a leaf.
    analysis.go     Analysis, Issue, CategorySummary
    envelope.go     Envelope
  util/         The utils layer — generic, dependency-free helpers.
    path.go         ExpandHome(string) string
  shell/        The segregated seam (interfaces) — unchanged.
    provider.go     Parser, Classifier, Introspector, Provider
    zsh/            zsh.Provider; describe/wordLitPrefix/parseIntrospect become PRIVATE METHODS on Provider.
  analyze/      Role types.
    analyzer.go     Analyzer{provider}; New(shell.Provider); (*Analyzer).Analyze(src, path) model.Analysis
    reconciler.go   reconciler; methods: summaries / duplicateNames / duplicatePaths / shadows
  render/       Role types behind one interface.
    renderer.go     Renderer interface { Render(model.Analysis) ([]byte, error) }
    human.go        HumanRenderer
    json.go         JSONRenderer (owns model→dto mapping + envelope assembly)
  cli/          Role type.
    cli.go          CLI{provider shell.Provider}; New(shell.Provider); (*CLI).Run(args, out, err) int
  cmd/zsh-pro/  Composition root — the ONLY importer of shell/zsh; wires zsh.Provider into cli.New.
```

## Role-type catalog (every current free function gets a home)

| Today (free function / package var) | Becomes |
|---|---|
| `analyze.Analyze(p, src, path)` | `(*analyze.Analyzer).Analyze(src, path)`; provider injected via `analyze.New(p)` |
| `analyze.dupNameIssues` / `dupPathIssues` / `shadowIssues` | private methods on `reconciler` |
| `analyze.primaryName` (summary.go) | private method on `reconciler`; `summary.go` is removed, folded into `reconciler.go` |
| `analyze.pathSegRe` (var) | unexported package var in `reconciler.go` (immutable compiled regex — see Idiom calls) |
| `render.Human(a)` | `(HumanRenderer).Render(a)` |
| `render.JSON(a)` | `(JSONRenderer).Render(a)` (maps `model` → `dto`, assembles `dto.Envelope`) |
| `render.envelope` (struct) | `dto.Envelope` (moved out of `render`, json tags live here) |
| `model.CategoryDescription(c)` | `(c Category).Description()` |
| `zsh.describe` / `wordLitPrefix` / `parseIntrospect` | private methods on `zsh.Provider` |
| `zsh.secretRe` / `pluginHints` (vars) | unexported package vars (immutable — see Idiom calls) |
| `cli.runAnalyze` / `fail` | private methods on `*CLI` |
| `cli.expandHome` | `util.ExpandHome` (moved to the util layer) |
| `cli.Run` | `(*CLI).Run`; `main` constructs `cli.New(zsh.Provider{})` |

`model.Categories()` stays a package function: it is the canonical taxonomy enumerator (a data constructor), not loose business logic — analogous to a `NewX`/factory. See Idiom calls.

## DTO / wire-contract preservation (trust-critical)

The `--json` output must be **byte-for-byte identical** after the refactor. `dto.Envelope`, `dto.Analysis`, `dto.Issue`, and `dto.CategorySummary` replicate the exact field order, json tag names, and `omitempty` flags currently on `render.envelope` + the `model` structs:

- Envelope: `tool, version, command, ok, issues_found, exit_code, analysis`.
- Analysis: `path, lines, blocks, opaque_blocks, categories, issues, has_secrets, introspected, notes(omitempty)`.
- Issue: `kind, name, lines(omitempty), note(omitempty)`.
- CategorySummary: `category, count, items(omitempty)`.

The `JSONRenderer` maps field-by-field, preserving nil-vs-empty slice semantics so `omitempty` behaves identically. The golden corpus tests + `render_test` are the regression guard. The human-mode `fail` error object (the `--json` error path in `cli`) keeps its current shape.

## Dependency rules (unchanged direction, now with two new leaves)

- `model`, `dto`, `util` are **leaves** — they import nothing inside `core/` (and no third-party libs).
- `dto` does **not** import `model`; mapping lives in `render` (which already knows both). This keeps `dto` a pure standalone wire-format leaf.
- `analyze` depends only on `model` + `shell` (interfaces) — never on `shell/zsh`.
- `render` depends on `model` + `dto` + `buildinfo`.
- `cli` depends on `model` + `shell` + `analyze` + `render` + `util` — **no longer on `shell/zsh`** (cleaner than today). The zsh import moves entirely to `cmd/zsh-pro` (composition root). `cli_test` may import `zsh` to wire a real provider.

## Idiom calls (where strict OOP would fight Go — flagged, not silently bent)

- **Compiled regexes / data tables** (`secretRe`, `pluginHints`, `pathSegRe`) stay as unexported package-level `var`s in the file of the role type that uses them. They are immutable data, not loose functions; package-level `regexp.MustCompile` is the Go idiom (recompiling per call/construction is wasteful). This honors "no global *mutable* state."
- **`util`** holds stateless helpers as namespaced package functions (`util.ExpandHome`). Wrapping a stateless string helper in a method receiver is un-idiomatic; the "own layer" requirement is satisfied by the dedicated package.
- **`model.Categories()`** stays a package function (taxonomy enumerator / factory).

## Testing & safety

- Pure restructuring with a strong regression net: unit tests per package + the golden-corpus tests over `core/testdata/fixtures`. Tests stay green at **every** commit.
- Test call-sites updated to the new constructors/methods: `analyze_test`/`corpus_test` (`analyze.New(p).Analyze`), `render_test` (`HumanRenderer{}.Render` / `JSONRenderer{}.Render`), `cli_test` (`cli.New(zsh.Provider{}).Run`), `model_test` (`cat.Description()`). The zsh `parse_test`/`introspect_test`/`classify_test` call only public methods and are unaffected by the helper→method conversions.
- Execution is **inline** (tightly-coupled refactor — independent per-task commits are not possible when a signature change ripples across layers), proceeding in a dependency-safe order so the build + suite are green after each commit, followed by an independent whole-branch code review.

## Amended Architecture Rules (supersede the analyze-engine spec's rules)

1. **Dependencies point inward.** `model`/`dto`/`util` are leaves; nothing imports `shell/zsh` except the composition root (`cmd/zsh-pro`).
2. **`model` is pure** — domain data + intrinsic methods only; no json tags, no logic, no I/O, no third-party imports. (Wire concerns live in `dto`.)
3. **Behavior lives on role types.** No free-standing production functions doing work; each becomes a method on the type that owns the job. Exceptions, by idiom: pure stateless helpers in the dedicated `util` package, type enumerators/factories (`Categories()`), and immutable package-level data (`var` regexes/tables).
4. **Segregated interfaces** — `Parser`/`Classifier`/`Introspector`; consumers depend on the narrowest they need.
5. **Errors are returned, never panicked**; the engine degrades gracefully.
6. **One type per file**, files kept small; **no global mutable state**.

## Out of scope (unchanged from v1)

No new behavior, no consumption of the resolved `IdentitySet` (still the backlog's top item), no writing/`split`/`adopt`, no TUI.
