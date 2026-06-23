# Coding Conventions

**Analysis Date:** 2026-06-23

## Naming Patterns

**Files:**
- One type (or one tightly-related cluster) per file; file named after the primary type in `snake_case`: `analyzer.go`, `reconciler.go`, `category.go`, `exitcode.go`, `identityset.go`
- Test files co-located with the package they test, named `<subject>_test.go`: `analyze_test.go`, `corpus_test.go`, `classify_test.go`

**Exported identifiers:**
- Types: `PascalCase` — `Analyzer`, `HumanRenderer`, `JSONRenderer`, `ConfigGraph`, `GenParams`
- Typed-enum constants: `CatAliases`, `KindAssignment`, `ConfHigh`, `ExitActionable`, `IssueDuplicateAlias` — prefix derived from the type name (`Cat`, `Kind`, `Conf`, `Exit`, `Issue`)
- Constructors: `New(...)` returning `*T`; every major type has exactly one: `analyze.New`, `cli.New`, `testgen.New`, `testgen.NewMutator`
- Methods that return a plain value use the noun form: `.Analyze(...)`, `.Render(...)`, `.Classify(...)`, `.Parse(...)`, `.Introspect(...)`, `.Build(...)`, `.RenderZsh()`, `.Expected()`

**Unexported identifiers:**
- Types: `camelCase` — `reconciler`, `mockProvider`, `errProvider`, `commandTmpl`, `manifest`
- Functions/methods: `camelCase` — `primaryName`, `duplicateNames`, `duplicatePaths`, `shadows`, `describe`, `wordLitPrefix`, `parseIntrospect`, `runAnalyze`, `toDTO`
- Private sentinel errors: typed via `type errTest string` implementing `error`, not `errors.New`
- Package-level vars: `camelCase` — `secretRe`, `pathSegRe`, `pluginHints`, `commandTmpls`

**Typed enums via `type X string` or `type X int` + constants:**
- `type Category string` with `Cat*` constants — `core/model/category.go`
- `type BlockKind string` with `Kind*` constants — `core/model/block.go`
- `type IssueKind string` with `Issue*` constants — `core/model/issue.go`
- `type ExitCode int` with `Exit*` constants — `core/model/exitcode.go`
- `type Confidence int` with `Conf*` constants (using `iota`) — `core/model/block.go`
- `type NodeKind int` with `Node*` constants (using `iota`) — `core/testgen/graph.go`

## Code Style

**Formatting:**
- `gofmt` enforced; all files must be `gofmt`-clean before commit
- Build/vet: `GOTOOLCHAIN=auto go build ./...`, `GOTOOLCHAIN=auto go vet ./...`

**Linting:**
- No external linter config detected; `go vet` is the minimum gate

**Line length:**
- No hard limit enforced; `gofmt` controls indentation; long lines are split at natural semantic breaks (function arguments, slice literals)

## Behavior on Role Types (Methods, Not Loose Functions)

All logic lives on methods of named types — never as package-level worker functions:
- `Analyzer.Analyze` owns the pipeline — `core/analyze/analyzer.go`
- `reconciler.primaryName`, `reconciler.duplicateNames`, `reconciler.duplicatePaths`, `reconciler.shadows` are pure methods on a zero-value `reconciler{}` — `core/analyze/reconciler.go`
- `Provider.Parse`, `Provider.Classify`, `Provider.Introspect`, `Provider.Categories` are methods on `zsh.Provider{}` — `core/shell/zsh/`
- `HumanRenderer.Render`, `JSONRenderer.Render` are methods on role-type structs — `core/render/`
- `CLI.Run`, `CLI.runAnalyze`, `CLI.fail` are methods on `CLI` — `core/cli/cli.go`
- `Generator.Build`, `Generator.pick` are methods on `Generator` — `core/testgen/generator.go`
- `ConfigGraph.Add`, `ConfigGraph.DependOn`, `ConfigGraph.RenderZsh`, `ConfigGraph.Expected` are methods on `ConfigGraph` — `core/testgen/`
- `Mutator.Corrupt` is a method on `Mutator` — `core/testgen/mutate.go`

**Zero-value structs as receivers:**
- `reconciler{}`, `Provider{}`, `HumanRenderer{}`, `JSONRenderer{}` are zero-value structs — all their methods are stateless and the receiver exists only for grouping

## Import Organization

**Order (enforced by `gofmt`/`goimports` convention):**
1. Standard library (`"bytes"`, `"encoding/json"`, `"os"`, `"regexp"`, `"sort"`, `"strings"`, etc.)
2. Blank line separator
3. Third-party (`"mvdan.cc/sh/v3/syntax"`)
4. Blank line separator
5. Internal (`"zsh-pro/core/..."`)

**Module path:** `zsh-pro` (declared in `go.mod`)

**Path aliases:** None — all imports use full `zsh-pro/core/...` paths

**Leaf package rule:** `dto` imports nothing inside `core/`; `model` imports nothing inside `core/`; `util` imports nothing inside `core/`; `buildinfo` imports nothing inside `core/`

## Dependency Injection at the Composition Root

- `core/shell/zsh` (the concrete provider) is imported **only** by `core/cmd/zsh-pro/main.go` and test packages that need end-to-end wiring (`core/cli/cli_test.go`, `core/analyze/corpus_test.go`, `core/testgen/*_test.go`)
- All other packages depend on the `shell.Provider` interface, not the concrete type
- `cli.New(p shell.Provider)` and `analyze.New(p shell.Provider)` receive the provider via constructor injection; no global state
- This is documented explicitly in `core/cli/cli.go` package comment

## Error Handling

**Patterns:**
- Errors are returned, never panicked in production code. `Analyzer.Analyze` states this explicitly: "It never panics; a failed/unavailable introspection degrades to static-only"
- Graceful degradation: when `provider.Introspect` returns `err != nil` or `ids.Available == false`, `a.Introspected` is set to `false` and a human-readable note is appended to `a.Notes` — `core/analyze/analyzer.go:84-88`
- Parse failures degrade to a single opaque `Block` (`Opaque: true`) rather than returning an error — `core/shell/zsh/parse.go:22-27`
- The `fail` helper in `core/cli/cli.go` emits a structured JSON error to stdout (not stderr) in `--json` mode, ensuring the agent contract (exactly one JSON object on stdout) holds even on runtime errors
- Errors returned from `os.ReadFile`, `cmd.Run`, `json.MarshalIndent`, `r.Render` are propagated to the caller with context via `fmt.Sprintf`

**Compile-time interface check:**
- `var _ shell.Provider = Provider{}` in `core/shell/zsh/introspect.go` enforces that `zsh.Provider` satisfies the interface at compile time

## Logging

- No logging framework; all user-facing output goes through `io.Writer` arguments (`stdout`, `stderr`) injected into `CLI.Run`
- Notes and warnings surface via `model.Analysis.Notes []string` accumulated during analysis

## Comments

**When to comment:**
- Every exported type, constructor, and method has a doc comment (all-caps first word for commands/functions is standard Go doc convention — e.g. `// New returns...`, `// Analyze produces...`, `// Render returns...`)
- Package-level `// Package X ...` comments on every package
- Non-obvious design decisions are explained inline, often with multi-sentence rationale (see `reconciler.shadows` rationale in `core/analyze/reconciler.go:84-94`, `TestAnalyzeIgnoresInheritedShadowIdentities` rationale in `core/analyze/analyze_test.go:62-73`)
- `TODO`/backlog items noted inline with a version qualifier, e.g. "v1 scope:" or "v1 scoping:"

**Style:**
- Comments above the declaration, never inline for anything more than a short parenthetical
- Short trailing comments used on const blocks for disambiguation: `KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar`

## Function Design

**Size:** Functions are short and focused; the longest production function is `Analyzer.Analyze` at ~75 lines, which is the intentional pipeline owner. Most methods are under 30 lines.

**Parameters:**
- Constructors take only what is needed for the lifetime of the type: `New(p shell.Provider) *Analyzer`, `New(rng *rand.Rand) *Generator`
- Methods on stateless zero-value types take only the data they need per call
- Avoid variadic parameters; use explicit struct types (`GenParams`) when there are many configuration knobs

**Return Values:**
- Methods return `(result, error)` when fallibility exists (`Parse`, `Introspect`, `Render`)
- Methods that cannot fail omit the error: `Classify`, `Categories`, `primaryName`, `duplicateNames`
- `Analyze` returns the value directly (no error) and encodes failure as `a.Introspected=false` + `a.Notes`

## Module Design

**Exports:** Each package exports only the minimum needed: one primary type + its constructor + its methods. Helper types (`reconciler`, `mockProvider`) are unexported.

**Barrel files:** None used. Each file is a focused unit; callers import specific subpackages.

**Interface segregation:** `core/shell` splits the provider into `Parser`, `Classifier`, `Introspector` before composing them in `Provider`; callers that only need one concern can depend on the narrowest interface.

---

*Convention analysis: 2026-06-23*
