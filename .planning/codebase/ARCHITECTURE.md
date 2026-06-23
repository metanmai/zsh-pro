<!-- refreshed: 2026-06-23 -->
# Architecture

**Analysis Date:** 2026-06-23

## System Overview

```text
┌──────────────────────────────────────────────────────────────────────┐
│              Composition Roots (cmd layer)                           │
│  `core/cmd/zsh-pro/main.go`          `core/cmd/zsh-gen/main.go`     │
│  Injects zsh.Provider → cli.New()    Drives testgen.Generator        │
└────────────────────────┬─────────────────────────┬───────────────────┘
                         │                         │
                         ▼                         ▼
┌──────────────────────────────┐   ┌───────────────────────────────────┐
│   Role Layer: core/cli       │   │   Role Layer: core/testgen        │
│  `core/cli/cli.go`           │   │  `core/testgen/generator.go`      │
│  Wires flags / I/O to engine │   │  `core/testgen/graph.go`          │
│  Owns exit-code contract     │   │  `core/testgen/oracle.go`         │
│  Depends on: shell.Provider, │   │  `core/testgen/render.go`         │
│  analyze, render, model, util│   │  `core/testgen/mutate.go`         │
└──────────────────┬───────────┘   │  Imports: model only (leaf)       │
                   │               └───────────────────────────────────┘
                   ▼
┌──────────────────────────────────────────────────────────────────────┐
│              Role Layer: core/analyze                                │
│  `core/analyze/analyzer.go`   `core/analyze/reconciler.go`          │
│  Drives Parse → Classify → Introspect; reconciles into model.Analysis│
│  Depends on: shell.Provider (interface), core/model                  │
└────────┬─────────────────────────────────────────────────────────────┘
         │ shell.Provider interface seam (core/shell/provider.go)
         ▼
┌──────────────────────────────────────────────────────────────────────┐
│              Shell Interface Layer: core/shell                       │
│  `core/shell/provider.go`                                            │
│  Defines: Parser, Classifier, Introspector, Provider (composite)    │
└────────────────────┬─────────────────────────────────────────────────┘
                     │  (only core/cmd/zsh-pro instantiates concrete impl)
                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│              Concrete Shell Impl: core/shell/zsh                     │
│  `core/shell/zsh/zsh.go`       Provider struct{}                    │
│  `core/shell/zsh/parse.go`     uses mvdan.cc/sh/v3 AST → []Block    │
│  `core/shell/zsh/classify.go`  regex + keyword rules → Category     │
│  `core/shell/zsh/introspect.go` runs `zsh -f` subprocess → IdentitySet│
└──────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────────────┐
│              Leaf Packages (no internal imports except each other)   │
│  `core/model/`   domain types (Block, Analysis, Category, Issue…)   │
│  `core/dto/`     JSON wire types (Envelope, Analysis, Issue…)       │
│  `core/util/`    ExpandHome() — stdlib only                         │
│  `core/buildinfo/` Name, Command, Version constants                  │
└──────────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Key Files |
|-----------|----------------|-----------|
| `core/cmd/zsh-pro` | Composition root — sole package that imports `zsh.Provider` and injects it | `core/cmd/zsh-pro/main.go` |
| `core/cmd/zsh-gen` | Testgen CLI — generates seeded .zsh fixtures + manifests.json | `core/cmd/zsh-gen/main.go` |
| `core/cli` | Flag parsing, I/O wiring, exit-code contract, JSON agent contract | `core/cli/cli.go` |
| `core/analyze` | Engine orchestration + reconciler (static issue detection) | `core/analyze/analyzer.go`, `core/analyze/reconciler.go` |
| `core/render` | Pure `model.Analysis → []byte` renderers (human + JSON) | `core/render/renderer.go`, `core/render/human.go`, `core/render/json.go` |
| `core/shell` | ISP interface seam — Parser, Classifier, Introspector, Provider | `core/shell/provider.go` |
| `core/shell/zsh` | Concrete zsh implementation of `shell.Provider` | `core/shell/zsh/parse.go`, `core/shell/zsh/classify.go`, `core/shell/zsh/introspect.go` |
| `core/model` | Shell-agnostic domain types; no internal dependencies | `core/model/block.go`, `core/model/analysis.go`, `core/model/category.go`, `core/model/issue.go`, `core/model/exitcode.go`, `core/model/identityset.go` |
| `core/dto` | JSON wire-format structs; imports nothing inside `core/` | `core/dto/analysis.go`, `core/dto/envelope.go` |
| `core/testgen` | Test-infrastructure only — generates random graphs, oracles, mutators | `core/testgen/generator.go`, `core/testgen/graph.go`, `core/testgen/oracle.go`, `core/testgen/render.go`, `core/testgen/mutate.go` |
| `core/util` | Stdlib-only helpers (path expansion) | `core/util/path.go` |
| `core/buildinfo` | Tool identity constants — Name, Command, Version | `core/buildinfo/buildinfo.go` |

## Pattern Overview

**Overall:** Clean Architecture with Interface Segregation, pipeline execution, composition-root injection

**Key Characteristics:**
- One composition root (`core/cmd/zsh-pro/main.go`) is the only place that imports the concrete `zsh.Provider` and wires it to the CLI
- Every other package depends on the `shell.Provider` interface, not the concrete implementation
- The analysis engine is a single-pass pipeline: Parse → Classify → Introspect → Reconcile
- Domain model (`core/model`) and wire format (`core/dto`) are kept separate; `core/render/json.go` maps between them
- Dependency direction is strictly inward toward leaf packages (`model`, `dto`, `util`, `buildinfo`)

## Layers

**Composition Root (`core/cmd/`):**
- Purpose: Instantiates concrete dependencies and wires them together
- Location: `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-gen/main.go`
- Contains: `main()` entry points only; minimal logic
- Depends on: `core/cli`, `core/shell/zsh`, `core/testgen`
- Used by: OS process entry

**CLI Role Layer (`core/cli`):**
- Purpose: Owns the user-facing contract (flags, I/O streams, exit codes, agent JSON contract)
- Location: `core/cli/cli.go`
- Contains: `CLI` struct, `Run()`, `runAnalyze()`, `fail()`
- Depends on: `shell.Provider` (interface), `core/analyze`, `core/render`, `core/model`, `core/util`, `core/buildinfo`
- Used by: `core/cmd/zsh-pro/main.go`

**Engine Role Layer (`core/analyze`):**
- Purpose: Drives the shell.Provider pipeline and reconciles static + dynamic views
- Location: `core/analyze/analyzer.go`, `core/analyze/reconciler.go`
- Contains: `Analyzer` struct (orchestrator), `reconciler` struct (pure stateless detectors)
- Depends on: `shell.Provider` (interface), `core/model`
- Used by: `core/cli`, test packages

**Render Role Layer (`core/render`):**
- Purpose: Maps `model.Analysis` to output bytes; pure, no side effects
- Location: `core/render/renderer.go`, `core/render/human.go`, `core/render/json.go`
- Contains: `Renderer` interface, `HumanRenderer{}`, `JSONRenderer{}`
- Depends on: `core/model`, `core/dto`, `core/buildinfo`
- Used by: `core/cli`

**Shell Interface Seam (`core/shell`):**
- Purpose: ISP-segregated interfaces that decouple the engine from zsh specifics
- Location: `core/shell/provider.go`
- Contains: `Parser`, `Classifier`, `Introspector`, `Provider` (composite) interfaces
- Depends on: `core/model`
- Used by: `core/analyze`, `core/cli`

**Concrete Shell Impl (`core/shell/zsh`):**
- Purpose: Implements `shell.Provider` for zsh — the only shell-specific package
- Location: `core/shell/zsh/zsh.go`, `core/shell/zsh/parse.go`, `core/shell/zsh/classify.go`, `core/shell/zsh/introspect.go`
- Contains: `Provider struct{}` (zero-value, method receiver)
- Depends on: `mvdan.cc/sh/v3/syntax` (AST parser), `core/model`, `core/shell`, `core/buildinfo`
- Used by: `core/cmd/zsh-pro/main.go` (composition root only), test files

**Leaf Packages (`core/model`, `core/dto`, `core/util`, `core/buildinfo`):**
- Purpose: Domain types, wire-format types, stdlib helpers, constants; no internal imports
- Location: `core/model/`, `core/dto/`, `core/util/path.go`, `core/buildinfo/buildinfo.go`
- Contains: Types and pure functions only
- Depends on: stdlib only (`core/dto`, `core/util`, `core/buildinfo`); `core/model` depends on nothing internal
- Used by: all other packages

**Test-Generation Subsystem (`core/testgen`):**
- Purpose: Builds deterministic random config graphs, derives oracle expectations, renders .zsh source, mutates for fuzz
- Location: `core/testgen/graph.go`, `core/testgen/generator.go`, `core/testgen/oracle.go`, `core/testgen/render.go`, `core/testgen/mutate.go`
- Contains: `ConfigGraph`, `Node`, `Generator`, `Mutator`; all test infrastructure
- Depends on: `core/model` only
- Used by: `core/cmd/zsh-gen/main.go`, test files in `core/testgen/`

## Data Flow

### Primary Analysis Request Path

1. `main()` constructs `cli.New(zsh.Provider{})` and calls `.Run(os.Args[1:], os.Stdout, os.Stderr)` (`core/cmd/zsh-pro/main.go:11`)
2. `cli.Run()` dispatches to `runAnalyze()` based on the `analyze` subcommand (`core/cli/cli.go:37-43`)
3. `runAnalyze()` reads the file from disk and calls `analyze.New(c.provider).Analyze(src, path)` (`core/cli/cli.go:70`)
4. `Analyzer.Analyze()` calls `provider.Parse(src)` → `[]model.Block` (`core/analyze/analyzer.go:25`)
5. For each block, calls `provider.Classify(block)` → `(model.Category, model.Confidence)`, mutates `block.Category`/`block.Conf` (`core/analyze/analyzer.go:36-43`)
6. `reconciler` methods detect issues from classified blocks: `duplicateNames`, `duplicatePaths`, `shadows` — all stateless, pure (`core/analyze/reconciler.go`)
7. `provider.Introspect(path)` runs `zsh -f -c <script>` as a subprocess (5 s timeout) → `model.IdentitySet`; on failure, `a.Introspected = false` and a note is appended (`core/analyze/analyzer.go:83-89`)
8. Issues are sorted deterministically (by Kind then Name) and `model.Analysis` is returned (`core/analyze/analyzer.go:91-97`)
9. `cli.runAnalyze()` selects a renderer (`HumanRenderer` or `JSONRenderer`) and calls `r.Render(a)` (`core/cli/cli.go:72-81`)
10. `JSONRenderer.Render()` maps domain model → `dto.Envelope` via `toDTO()` before marshalling (`core/render/json.go:23-63`)
11. Output is written to `stdout`; `a.ExitCode()` determines process exit code (0/1/2/3)

### Testgen / Config Generation Path

1. `zsh-gen` calls `testgen.New(rng).Build(params)` → `*ConfigGraph` (`core/cmd/zsh-gen/main.go:41`)
2. `g.RenderZsh()` emits deterministic `.zsh` source, setting `Node.Line` on each node (`core/testgen/render.go`)
3. `g.Expected()` derives the correct `model.Analysis` by walking the graph (oracle) (`core/testgen/oracle.go`)
4. `zsh-gen` writes `.zsh` files and a `manifests.json` corpus entry to disk for promotion to golden fixtures

### Property / Fuzz Test Path

1. Test code calls `testgen.New(rng).Build(params)`, `g.RenderZsh()`, `g.Expected()` to get input + oracle
2. Runs the real engine: `analyze.New(zsh.Provider{}).Analyze(src, path)`
3. `assertStrict()` compares oracle categories, issue (kind, name) set, `has_secrets`, zero opaque blocks
4. Fuzz variant: `testgen.NewMutator(rng).Corrupt(src)` corrupts bytes; engine must not panic, JSON must be valid

**State Management:**
- No global mutable state; all state is function-local or on the `Analyzer` struct (which holds the injected `provider`)
- `reconciler` carries no state; all methods are pure functions
- `Provider{}` (zsh) is a zero-value struct; all methods use only their arguments
- The `model.Analysis` returned by `Analyze()` is a value type — read-only by convention

## Key Abstractions

**`shell.Provider` interface:**
- Purpose: The primary seam — decouples the engine from any concrete shell
- Location: `core/shell/provider.go`
- Pattern: Interface Segregation — three narrow interfaces (`Parser`, `Classifier`, `Introspector`) composed into `Provider`; callers depend only on the narrowest interface they need

**`model.Block`:**
- Purpose: Structural representation of one parsed zsh statement (plus leading comments)
- Location: `core/model/block.go`
- Pattern: Value type enriched in two passes (parser fills `Kind`/`Names`/`Exported`; classifier fills `Category`/`Conf`)

**`model.Analysis`:**
- Purpose: Complete, read-only result consumed by renderers
- Location: `core/model/analysis.go`
- Pattern: Value type returned by `Analyzer.Analyze()`; `ExitCode()` is derived (no stored state)

**`dto.Envelope`:**
- Purpose: Single top-level JSON object for the agent contract
- Location: `core/dto/envelope.go`
- Pattern: `JSONRenderer.toDTO()` maps domain → DTO explicitly; DTO imports nothing from `core/`

**`render.Renderer` interface:**
- Purpose: Decouples CLI from output format; enables human and JSON modes
- Location: `core/render/renderer.go`
- Pattern: Role type (zero-value struct implements interface); `cli.go` selects at runtime

**`testgen.ConfigGraph` + `testgen.Generator`:**
- Purpose: Deterministic random config builder with oracle; test infrastructure only
- Location: `core/testgen/graph.go`, `core/testgen/generator.go`, `core/testgen/oracle.go`
- Pattern: Build graph → RenderZsh → Expected; topological order guaranteed by append-only insertion

## Entry Points

**`core/cmd/zsh-pro/main.go`:**
- Location: `core/cmd/zsh-pro/main.go`
- Triggers: `zsh-pro` binary executed from shell
- Responsibilities: Constructs `zsh.Provider{}`, wires to `cli.New()`, calls `Run()`, passes exit code to `os.Exit`

**`core/cmd/zsh-gen/main.go`:**
- Location: `core/cmd/zsh-gen/main.go`
- Triggers: `zsh-gen` binary executed from shell
- Responsibilities: Parses `-n`, `-seed`, `-out` flags; calls `generate()` which uses `testgen.Generator` to emit fixture files

## Architectural Constraints

- **Single composition root:** Only `core/cmd/zsh-pro/main.go` (and tests in external test packages) import `core/shell/zsh`. All other production code sees only the `shell.Provider` interface.
- **Threading:** Single-threaded. `zsh -f` subprocess runs synchronously with a 5-second `context.WithTimeout`. No goroutines in the engine.
- **Global state:** None. `zsh.Provider{}` is a zero-value struct. `reconciler{}` is instantiated per `Analyzer`. The only package-level values are constant slices/regexps in `core/shell/zsh/classify.go` (`secretRe`, `pluginHints`) and `core/testgen/generator.go` (name pools, command templates) — all read-only.
- **Circular imports:** None by construction. Dependency direction: `cmd → cli/testgen → analyze/render → shell (interface) → model/dto`. `core/shell/zsh` is imported only at composition root; it may freely import `core/model` and `core/shell` without a cycle.
- **Opaque fallback:** `Provider.Parse()` never returns an error to the caller; an unparseable file or statement becomes an `Opaque: true` block so the engine always produces a result.
- **Dynamic introspection is best-effort:** Introspect failure degrades to static-only analysis; the engine never crashes. The resolved `IdentitySet` tables (aliases, functions, env, path, options) are captured but intentionally not yet consumed by issue detection (v1 scope).

## Anti-Patterns

### Importing `core/shell/zsh` outside the composition root

**What happens:** A package other than `core/cmd/zsh-pro` or an external test file imports `zsh-pro/core/shell/zsh` directly.
**Why it's wrong:** It breaks the shell-agnostic seam — any new shell would require changing that package, and `analyze` / `cli` become coupled to zsh.
**Do this instead:** Accept `shell.Provider` (or `shell.Parser` / `shell.Classifier`) as a parameter; let `core/cmd/zsh-pro/main.go` inject the concrete type.

### Iterating `model.IdentitySet` for issue detection

**What happens:** Shadow or issue detectors walk `ids.Aliases` / `ids.Functions` from the resolved `IdentitySet`.
**Why it's wrong:** The resolved set includes inherited zsh defaults (`run-help`, `compinit`, built-in env vars) not defined by the user's file — produces false-positive shadow issues. The existing `TestAnalyzeIgnoresInheritedShadowIdentities` test guards against this regression (`core/analyze/analyze_test.go:74`).
**Do this instead:** Scope all static issue detection to the parsed `[]model.Block`; use the `IdentitySet` only for the `Introspected` flag (current behaviour in `core/analyze/analyzer.go:83-89`).

### Calling `RenderZsh` after `Expected` in testgen

**What happens:** `g.Expected()` is called before `g.RenderZsh()`.
**Why it's wrong:** `Expected()` reads `Node.Line` fields that `RenderZsh()` fills. The oracle will record line 0 for all nodes.
**Do this instead:** Always call `src := g.RenderZsh()` first, then `want := g.Expected()` (as in `core/testgen/property_test.go:35-37`).

## Error Handling

**Strategy:** Errors are surfaced to the CLI boundary; the engine itself never returns an error to callers — it degrades gracefully.

**Patterns:**
- `Provider.Parse()` returns `([]model.Block, error)` but the engine ignores the error: a parse failure produces an opaque fallback block (`core/shell/zsh/parse.go:22-28`)
- `Provider.Introspect()` failure sets `Analysis.Introspected = false` and appends a human-readable note; execution continues with static-only results (`core/analyze/analyzer.go:84-88`)
- `cli.fail()` emits structured errors: in `--json` mode a well-formed error envelope goes to `stdout` (agent contract); in human mode the message goes to `stderr` (`core/cli/cli.go:88-99`)
- Exit codes are typed via `model.ExitCode` and derived from `model.Analysis.ExitCode()`: 0 (clean), 1 (runtime error), 2 (usage error), 3 (actionable issues)

## Cross-Cutting Concerns

**Logging:** None — the tool is a read-only CLI that writes its entire output to the provided `io.Writer` streams. No internal logger.
**Validation:** Input validation is implicit — the parser accepts any bytes and degrades gracefully; flag validation is in `cli.runAnalyze()`.
**Authentication:** Not applicable — local filesystem only; no network calls except the subprocess `zsh -f`.

---

*Architecture analysis: 2026-06-23*
