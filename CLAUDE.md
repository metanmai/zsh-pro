<!-- GSD:project-start source:PROJECT.md -->
## Project

**zsh-pro**

zsh-pro is a **git-versioned, branchable shell-environment manager** (currently milestone v2.0). It ingests a zsh config (default `~/.zshrc`) with a real AST parser, classifies each entry into a structured, regenerable representation, and stores it as a git-style repo where each branch is an environment profile; `checkout <branch>` live-reloads the terminal into that profile via a sourced activate/deactivate manifest. The parse → classify → introspect engine (built across v1.0–v1.1 as a read-only analyzer) is the **ingest component**, not the product. See `.planning/PROJECT.md` for the authoritative current identity and milestone.

**Core Value:** `checkout <branch>` yields a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses with zero residue, while dynamic values (`$HOME`/`$(...)`) stay late-bound so profiles stay portable.

### Constraints

- **Tech stack**: Go 1.25+; single external dependency (`mvdan.cc/sh/v3`) — **no new dependencies** without explicit discussion.
- **Architecture**: Respect the existing layering — `core/analyze` stays shell-free (interface seam only); `core/testgen` imports only `core/model`; `core/shell/zsh` is touched only via the `Provider` seam / composition root. New environment-manager surface (storage, shell integration, activation) should compose with this seam, not bypass it.
- **Activation safety**: Branch switching must be zero-residue on declarative state (deactivate-then-activate) and must never freeze dynamic values — portability is a hard requirement.
- **Testing**: TDD. The `testgen` oracle property test remains a regression pin for the ingest engine.
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.25.0 — entire codebase (`core/`, `go.mod`)
- Zsh (shell script) — test fixtures and the embedded `introspectScript` constant (`core/shell/zsh/introspect.go`); not compiled
## Runtime
- Go toolchain 1.25.0 (as declared in `go.mod`)
- Go modules (`go mod`)
- Lockfile: `go.sum` present and committed
## Frameworks
- Standard library only — no web or application framework; the CLI is hand-rolled in `core/cli/cli.go`
- Standard `testing` package — all test files use `testing.T`
- `github.com/go-quicktest/qt v1.101.0` — assertion helpers (pulled in transitively by `mvdan.cc/sh`)
- `github.com/google/go-cmp v0.7.0` — deep equality comparisons (transitive dependency)
- No separate build tool; standard `go build ./...` and `go test ./...`
- Binary targets:
## Key Dependencies
- `mvdan.cc/sh/v3 v3.13.1` — the only non-stdlib dependency; provides the zsh-variant parser (`syntax.NewParser` with `syntax.LangZsh`) used in `core/shell/zsh/parse.go`
- `github.com/go-quicktest/qt v1.101.0` — transitive test dependency (not directly imported in test files discovered)
- `github.com/google/go-cmp v0.7.0` — transitive dependency
- `github.com/kr/pretty v0.3.1` — transitive dependency
- `github.com/rogpeppe/go-internal v1.14.1` — transitive dependency
## Configuration
- No `.env` file or environment-variable configuration at runtime
- The only runtime environment dependency is a `zsh` binary on `$PATH` for introspection (`core/shell/zsh/introspect.go`); its absence degrades gracefully to static-only mode
- `go.mod` at repo root declares module name `zsh-pro` and Go version
- `go.sum` at repo root pins all dependency hashes
- No `Makefile`, `Taskfile`, or additional build config detected
## Platform Requirements
- Go 1.25+ toolchain
- `zsh` available in `$PATH` for integration/corpus tests (introspection path); static tests run without it
- Self-contained binary; no runtime dependencies beyond `zsh` on `$PATH`
- Designed for macOS/Linux (zsh introspection script uses zsh builtins: `zsh/parameter` module, `emulate -L zsh`)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- One type (or one tightly-related cluster) per file; file named after the primary type in `snake_case`: `analyzer.go`, `reconciler.go`, `category.go`, `exitcode.go`, `identityset.go`
- Test files co-located with the package they test, named `<subject>_test.go`: `analyze_test.go`, `corpus_test.go`, `classify_test.go`
- Types: `PascalCase` — `Analyzer`, `HumanRenderer`, `JSONRenderer`, `ConfigGraph`, `GenParams`
- Typed-enum constants: `CatAliases`, `KindAssignment`, `ConfHigh`, `ExitActionable`, `IssueDuplicateAlias` — prefix derived from the type name (`Cat`, `Kind`, `Conf`, `Exit`, `Issue`)
- Constructors: `New(...)` returning `*T`; every major type has exactly one: `analyze.New`, `cli.New`, `testgen.New`, `testgen.NewMutator`
- Methods that return a plain value use the noun form: `.Analyze(...)`, `.Render(...)`, `.Classify(...)`, `.Parse(...)`, `.Introspect(...)`, `.Build(...)`, `.RenderZsh()`, `.Expected()`
- Types: `camelCase` — `reconciler`, `mockProvider`, `errProvider`, `commandTmpl`, `manifest`
- Functions/methods: `camelCase` — `primaryName`, `duplicateNames`, `duplicatePaths`, `shadows`, `describe`, `wordLitPrefix`, `parseIntrospect`, `runAnalyze`, `toDTO`
- Private sentinel errors: typed via `type errTest string` implementing `error`, not `errors.New`
- Package-level vars: `camelCase` — `secretRe`, `pathSegRe`, `pluginHints`, `commandTmpls`
- `type Category string` with `Cat*` constants — `core/model/category.go`
- `type BlockKind string` with `Kind*` constants — `core/model/block.go`
- `type IssueKind string` with `Issue*` constants — `core/model/issue.go`
- `type ExitCode int` with `Exit*` constants — `core/model/exitcode.go`
- `type Confidence int` with `Conf*` constants (using `iota`) — `core/model/block.go`
- `type NodeKind int` with `Node*` constants (using `iota`) — `core/testgen/graph.go`
## Code Style
- `gofmt` enforced; all files must be `gofmt`-clean before commit
- Build/vet: `GOTOOLCHAIN=auto go build ./...`, `GOTOOLCHAIN=auto go vet ./...`
- Lint: `golangci-lint` v2 (`.golangci.yml`, standard set — errcheck/govet/ineffassign/staticcheck/unused + gofmt), installed as a standalone dev tool (`brew install golangci-lint`), intentionally **not** a go.mod dependency. Run `make lint`; `make check` = fmt-check + vet + lint + test. A `.githooks/pre-commit` hook gates staged Go changes — enable once per clone with `make hooks`
- No hard limit enforced; `gofmt` controls indentation; long lines are split at natural semantic breaks (function arguments, slice literals)
## Behavior on Role Types (Methods, Not Loose Functions)
- `Analyzer.Analyze` owns the pipeline — `core/analyze/analyzer.go`
- `reconciler.primaryName`, `reconciler.duplicateNames`, `reconciler.duplicatePaths`, `reconciler.shadows` are pure methods on a zero-value `reconciler{}` — `core/analyze/reconciler.go`
- `Provider.Parse`, `Provider.Classify`, `Provider.Introspect`, `Provider.Categories` are methods on `zsh.Provider{}` — `core/shell/zsh/`
- `HumanRenderer.Render`, `JSONRenderer.Render` are methods on role-type structs — `core/render/`
- `CLI.Run`, `CLI.runAnalyze`, `CLI.fail` are methods on `CLI` — `core/cli/cli.go`
- `Generator.Build`, `Generator.pick` are methods on `Generator` — `core/testgen/generator.go`
- `ConfigGraph.Add`, `ConfigGraph.DependOn`, `ConfigGraph.RenderZsh`, `ConfigGraph.Expected` are methods on `ConfigGraph` — `core/testgen/`
- `Mutator.Corrupt` is a method on `Mutator` — `core/testgen/mutate.go`
- `reconciler{}`, `Provider{}`, `HumanRenderer{}`, `JSONRenderer{}` are zero-value structs — all their methods are stateless and the receiver exists only for grouping
## Import Organization
## Dependency Injection at the Composition Root
- `core/shell/zsh` (the concrete provider) is imported **only** by `core/cmd/zsh-pro/main.go` and test packages that need end-to-end wiring (`core/cli/cli_test.go`, `core/analyze/corpus_test.go`, `core/testgen/*_test.go`)
- All other packages depend on the `shell.Provider` interface, not the concrete type
- `cli.New(p shell.Provider)` and `analyze.New(p shell.Provider)` receive the provider via constructor injection; no global state
- This is documented explicitly in `core/cli/cli.go` package comment
## Error Handling
- Errors are returned, never panicked in production code. `Analyzer.Analyze` states this explicitly: "It never panics; a failed/unavailable introspection degrades to static-only"
- Graceful degradation: when `provider.Introspect` returns `err != nil` or `ids.Available == false`, `a.Introspected` is set to `false` and a human-readable note is appended to `a.Notes` — `core/analyze/analyzer.go:84-88`
- Parse failures degrade to a single opaque `Block` (`Opaque: true`) rather than returning an error — `core/shell/zsh/parse.go:22-27`
- The `fail` helper in `core/cli/cli.go` emits a structured JSON error to stdout (not stderr) in `--json` mode, ensuring the agent contract (exactly one JSON object on stdout) holds even on runtime errors
- Errors returned from `os.ReadFile`, `cmd.Run`, `json.MarshalIndent`, `r.Render` are propagated to the caller with context via `fmt.Sprintf`
- `var _ shell.Provider = Provider{}` in `core/shell/zsh/introspect.go` enforces that `zsh.Provider` satisfies the interface at compile time
## Logging
- No logging framework; all user-facing output goes through `io.Writer` arguments (`stdout`, `stderr`) injected into `CLI.Run`
- Notes and warnings surface via `model.Analysis.Notes []string` accumulated during analysis
## Comments
- Every exported type, constructor, and method has a doc comment (all-caps first word for commands/functions is standard Go doc convention — e.g. `// New returns...`, `// Analyze produces...`, `// Render returns...`)
- Package-level `// Package X ...` comments on every package
- Non-obvious design decisions are explained inline, often with multi-sentence rationale (see `reconciler.shadows` rationale in `core/analyze/reconciler.go:84-94`, `TestAnalyzeIgnoresInheritedShadowIdentities` rationale in `core/analyze/analyze_test.go:62-73`)
- `TODO`/backlog items noted inline with a version qualifier, e.g. "v1 scope:" or "v1 scoping:"
- Comments above the declaration, never inline for anything more than a short parenthetical
- Short trailing comments used on const blocks for disambiguation: `KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar`
## Function Design
- Constructors take only what is needed for the lifetime of the type: `New(p shell.Provider) *Analyzer`, `New(rng *rand.Rand) *Generator`
- Methods on stateless zero-value types take only the data they need per call
- Avoid variadic parameters; use explicit struct types (`GenParams`) when there are many configuration knobs
- Methods return `(result, error)` when fallibility exists (`Parse`, `Introspect`, `Render`)
- Methods that cannot fail omit the error: `Classify`, `Categories`, `primaryName`, `duplicateNames`
- `Analyze` returns the value directly (no error) and encodes failure as `a.Introspected=false` + `a.Notes`
## Module Design
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## System Overview
```text
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
- One composition root (`core/cmd/zsh-pro/main.go`) is the only place that imports the concrete `zsh.Provider` and wires it to the CLI
- Every other package depends on the `shell.Provider` interface, not the concrete implementation
- The analysis engine is a single-pass pipeline: Parse → Classify → Introspect → Reconcile
- Domain model (`core/model`) and wire format (`core/dto`) are kept separate; `core/render/json.go` maps between them
- Dependency direction is strictly inward toward leaf packages (`model`, `dto`, `util`, `buildinfo`)
## Layers
- Purpose: Instantiates concrete dependencies and wires them together
- Location: `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-gen/main.go`
- Contains: `main()` entry points only; minimal logic
- Depends on: `core/cli`, `core/shell/zsh`, `core/testgen`
- Used by: OS process entry
- Purpose: Owns the user-facing contract (flags, I/O streams, exit codes, agent JSON contract)
- Location: `core/cli/cli.go`
- Contains: `CLI` struct, `Run()`, `runAnalyze()`, `fail()`
- Depends on: `shell.Provider` (interface), `core/analyze`, `core/render`, `core/model`, `core/util`, `core/buildinfo`
- Used by: `core/cmd/zsh-pro/main.go`
- Purpose: Drives the shell.Provider pipeline and reconciles static + dynamic views
- Location: `core/analyze/analyzer.go`, `core/analyze/reconciler.go`
- Contains: `Analyzer` struct (orchestrator), `reconciler` struct (pure stateless detectors)
- Depends on: `shell.Provider` (interface), `core/model`
- Used by: `core/cli`, test packages
- Purpose: Maps `model.Analysis` to output bytes; pure, no side effects
- Location: `core/render/renderer.go`, `core/render/human.go`, `core/render/json.go`
- Contains: `Renderer` interface, `HumanRenderer{}`, `JSONRenderer{}`
- Depends on: `core/model`, `core/dto`, `core/buildinfo`
- Used by: `core/cli`
- Purpose: ISP-segregated interfaces that decouple the engine from zsh specifics
- Location: `core/shell/provider.go`
- Contains: `Parser`, `Classifier`, `Introspector`, `Provider` (composite) interfaces
- Depends on: `core/model`
- Used by: `core/analyze`, `core/cli`
- Purpose: Implements `shell.Provider` for zsh — the only shell-specific package
- Location: `core/shell/zsh/zsh.go`, `core/shell/zsh/parse.go`, `core/shell/zsh/classify.go`, `core/shell/zsh/introspect.go`
- Contains: `Provider struct{}` (zero-value, method receiver)
- Depends on: `mvdan.cc/sh/v3/syntax` (AST parser), `core/model`, `core/shell`, `core/buildinfo`
- Used by: `core/cmd/zsh-pro/main.go` (composition root only), test files
- Purpose: Domain types, wire-format types, stdlib helpers, constants; no internal imports
- Location: `core/model/`, `core/dto/`, `core/util/path.go`, `core/buildinfo/buildinfo.go`
- Contains: Types and pure functions only
- Depends on: stdlib only (`core/dto`, `core/util`, `core/buildinfo`); `core/model` depends on nothing internal
- Used by: all other packages
- Purpose: Builds deterministic random config graphs, derives oracle expectations, renders .zsh source, mutates for fuzz
- Location: `core/testgen/graph.go`, `core/testgen/generator.go`, `core/testgen/oracle.go`, `core/testgen/render.go`, `core/testgen/mutate.go`
- Contains: `ConfigGraph`, `Node`, `Generator`, `Mutator`; all test infrastructure
- Depends on: `core/model` only
- Used by: `core/cmd/zsh-gen/main.go`, test files in `core/testgen/`
## Data Flow
### Primary Analysis Request Path
### Testgen / Config Generation Path
### Property / Fuzz Test Path
- No global mutable state; all state is function-local or on the `Analyzer` struct (which holds the injected `provider`)
- `reconciler` carries no state; all methods are pure functions
- `Provider{}` (zsh) is a zero-value struct; all methods use only their arguments
- The `model.Analysis` returned by `Analyze()` is a value type — read-only by convention
## Key Abstractions
- Purpose: The primary seam — decouples the engine from any concrete shell
- Location: `core/shell/provider.go`
- Pattern: Interface Segregation — three narrow interfaces (`Parser`, `Classifier`, `Introspector`) composed into `Provider`; callers depend only on the narrowest interface they need
- Purpose: Structural representation of one parsed zsh statement (plus leading comments)
- Location: `core/model/block.go`
- Pattern: Value type enriched in two passes (parser fills `Kind`/`Names`/`Exported`; classifier fills `Category`/`Conf`)
- Purpose: Complete, read-only result consumed by renderers
- Location: `core/model/analysis.go`
- Pattern: Value type returned by `Analyzer.Analyze()`; `ExitCode()` is derived (no stored state)
- Purpose: Single top-level JSON object for the agent contract
- Location: `core/dto/envelope.go`
- Pattern: `JSONRenderer.toDTO()` maps domain → DTO explicitly; DTO imports nothing from `core/`
- Purpose: Decouples CLI from output format; enables human and JSON modes
- Location: `core/render/renderer.go`
- Pattern: Role type (zero-value struct implements interface); `cli.go` selects at runtime
- Purpose: Deterministic random config builder with oracle; test infrastructure only
- Location: `core/testgen/graph.go`, `core/testgen/generator.go`, `core/testgen/oracle.go`
- Pattern: Build graph → RenderZsh → Expected; topological order guaranteed by append-only insertion
## Entry Points
- Location: `core/cmd/zsh-pro/main.go`
- Triggers: `zsh-pro` binary executed from shell
- Responsibilities: Constructs `zsh.Provider{}`, wires to `cli.New()`, calls `Run()`, passes exit code to `os.Exit`
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
### Iterating `model.IdentitySet` for issue detection
### Calling `RenderZsh` after `Expected` in testgen
## Error Handling
- `Provider.Parse()` returns `([]model.Block, error)` but the engine ignores the error: a parse failure produces an opaque fallback block (`core/shell/zsh/parse.go:22-28`)
- `Provider.Introspect()` failure sets `Analysis.Introspected = false` and appends a human-readable note; execution continues with static-only results (`core/analyze/analyzer.go:84-88`)
- `cli.fail()` emits structured errors: in `--json` mode a well-formed error envelope goes to `stdout` (agent contract); in human mode the message goes to `stderr` (`core/cli/cli.go:88-99`)
- Exit codes are typed via `model.ExitCode` and derived from `model.Analysis.ExitCode()`: 0 (clean), 1 (runtime error), 2 (usage error), 3 (actionable issues)
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
