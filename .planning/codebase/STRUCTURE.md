# Codebase Structure

**Analysis Date:** 2026-06-23

## Directory Layout

```
zsh-pro/
├── core/                         # All Go source — module root is repo root
│   ├── cmd/                      # Composition roots (binary entry points)
│   │   ├── zsh-pro/              # Main analyzer binary
│   │   │   └── main.go           # Wires zsh.Provider → cli.New().Run()
│   │   └── zsh-gen/              # Fixture generator binary
│   │       ├── main.go           # Drives testgen.Generator, writes .zsh + manifests.json
│   │       └── main_test.go
│   ├── cli/                      # CLI role layer (flags, I/O, exit codes)
│   │   ├── cli.go
│   │   └── cli_test.go
│   ├── analyze/                  # Engine role layer (pipeline + reconciler)
│   │   ├── analyzer.go           # Orchestrates Parse → Classify → Introspect → Reconcile
│   │   ├── reconciler.go         # Stateless issue detectors (pure methods)
│   │   ├── analyze_test.go       # Unit tests with mockProvider
│   │   └── corpus_test.go        # Tier-1 golden corpus runner (external test package)
│   ├── render/                   # Render role layer (model → bytes)
│   │   ├── renderer.go           # Renderer interface
│   │   ├── human.go              # HumanRenderer{}
│   │   ├── json.go               # JSONRenderer{} — maps model → dto.Envelope
│   │   └── render_test.go
│   ├── shell/                    # Interface seam (shell-agnostic contracts)
│   │   ├── provider.go           # Parser, Classifier, Introspector, Provider interfaces
│   │   └── zsh/                  # Concrete zsh implementation
│   │       ├── zsh.go            # Provider struct{} declaration + Categories()
│   │       ├── parse.go          # mvdan/sh AST → []model.Block
│   │       ├── classify.go       # Regex + keyword rules → Category
│   │       ├── introspect.go     # `zsh -f` subprocess → IdentitySet
│   │       ├── parse_test.go
│   │       ├── classify_test.go
│   │       └── introspect_test.go
│   ├── model/                    # Leaf: shell-agnostic domain types
│   │   ├── analysis.go           # Analysis, CategorySummary structs
│   │   ├── block.go              # Block, BlockKind, Confidence
│   │   ├── category.go           # Category consts + taxonomy ordering
│   │   ├── exitcode.go           # ExitCode consts (0/1/2/3)
│   │   ├── identityset.go        # IdentitySet (dynamic introspection result)
│   │   ├── issue.go              # Issue, IssueKind
│   │   └── model_test.go
│   ├── dto/                      # Leaf: JSON wire-format structs (no internal imports)
│   │   ├── analysis.go           # Analysis, Issue, CategorySummary (json tags)
│   │   └── envelope.go           # Envelope (top-level agent output object)
│   ├── testgen/                  # Test-infrastructure subsystem (test files only import this)
│   │   ├── graph.go              # ConfigGraph, Node, NodeKind
│   │   ├── generator.go          # Generator.Build() — constructs random typed graphs
│   │   ├── oracle.go             # ConfigGraph.Expected() — derives correct model.Analysis
│   │   ├── render.go             # ConfigGraph.RenderZsh() — emits .zsh source + sets Node.Line
│   │   ├── mutate.go             # Mutator.Corrupt() — byte-level corruptions for fuzz
│   │   ├── graph_test.go
│   │   ├── generator_test.go
│   │   ├── oracle_test.go
│   │   ├── render_test.go        # Verifies rendered zsh parses with zero opaque blocks
│   │   ├── property_test.go      # Oracle property test — engine vs. oracle over 10 seeds
│   │   └── fuzz_test.go          # Fuzz survival — 40 corrupted configs, JSON must be valid
│   ├── util/                     # Leaf: stdlib-only helpers
│   │   └── path.go               # ExpandHome()
│   ├── buildinfo/                # Leaf: tool identity constants
│   │   ├── buildinfo.go          # Name="zsh-pro", Command="analyze", Version="0.1.0"
│   │   └── buildinfo_test.go
│   └── testdata/                 # Golden fixture corpus
│       └── fixtures/
│           ├── manifests.json    # Ground-truth expectations keyed by fixture filename
│           ├── clean_baseline.zsh
│           ├── duplicate_aliases.zsh
│           ├── empty.zsh
│           ├── installer_junk.zsh
│           ├── reassigned_env.zsh
│           └── secrets_inline.zsh
├── docs/                         # Design docs and planning (not compiled)
│   └── superpowers/
│       ├── plans/                # Implementation plans (one per feature phase)
│       └── specs/                # Design specs (one per feature)
├── go.mod                        # Module: zsh-pro; go 1.25.0; one dependency: mvdan.cc/sh/v3
├── go.sum
└── .gitignore
```

## Directory Purposes

**`core/cmd/`:**
- Purpose: Composition roots — the only packages that import `core/shell/zsh` and wire concrete dependencies
- Contains: `main.go` only; no business logic
- Key files: `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-gen/main.go`

**`core/cli/`:**
- Purpose: Owns the user-facing CLI contract — flag parsing, stdout/stderr routing, exit codes, JSON agent contract for `--json` mode
- Contains: `CLI` struct and its methods
- Key files: `core/cli/cli.go`

**`core/analyze/`:**
- Purpose: Engine orchestration; drives the Parse → Classify → Introspect pipeline; stateless reconciler detects all four issue kinds from classified blocks
- Contains: `Analyzer` (orchestrator), `reconciler` (pure detectors)
- Key files: `core/analyze/analyzer.go`, `core/analyze/reconciler.go`

**`core/render/`:**
- Purpose: Output formatting; pure functions on `model.Analysis`
- Contains: `Renderer` interface, `HumanRenderer`, `JSONRenderer` (maps domain → dto before marshalling)
- Key files: `core/render/renderer.go`, `core/render/human.go`, `core/render/json.go`

**`core/shell/`:**
- Purpose: Shell-agnostic interface seam; defines the three segregated interfaces and their composite
- Contains: Interfaces only — no implementation
- Key files: `core/shell/provider.go`

**`core/shell/zsh/`:**
- Purpose: Only concrete shell implementation; uses `mvdan.cc/sh/v3` AST for parsing; spawns `zsh -f` for dynamic introspection
- Contains: `Provider struct{}` + three method groups (parse, classify, introspect)
- Key files: `core/shell/zsh/parse.go`, `core/shell/zsh/classify.go`, `core/shell/zsh/introspect.go`

**`core/model/`:**
- Purpose: Leaf — shared domain types (one type per file convention)
- Contains: Value types only; one method per type for derived values (`ExitCode()`, `Description()`, `Categories()`)
- Key files: every `.go` file is one type

**`core/dto/`:**
- Purpose: Leaf — JSON wire-format types; deliberately separate from model; imports nothing inside `core/`
- Contains: Mirror of model types with `json` struct tags; `Envelope` is the single top-level output object
- Key files: `core/dto/analysis.go`, `core/dto/envelope.go`

**`core/testgen/`:**
- Purpose: Test-infrastructure subsystem — generates typed dependency graphs, derives oracle expectations, renders zsh, mutates bytes for fuzz; never imported by production packages
- Contains: `ConfigGraph`, `Node`, `Generator`, `Mutator`; all files are either implementation or test
- Key files: `core/testgen/graph.go`, `core/testgen/generator.go`, `core/testgen/oracle.go`

**`core/testdata/fixtures/`:**
- Purpose: Golden corpus — `.zsh` fixture files + `manifests.json` expectations for data-driven corpus tests
- Contains: Hand-authored or `zsh-gen`-promoted `.zsh` files; `manifests.json`
- Generated: Promoted by `zsh-gen -out core/testdata/fixtures/`; committed to the repo

**`docs/superpowers/`:**
- Purpose: Design specs and implementation plans — not compiled, not generated
- Contains: Markdown only
- Committed: Yes

## Naming Conventions

**Files:**
- One type or one interface per file, named after its primary type/role: `analyzer.go`, `reconciler.go`, `human.go`, `json.go`, `provider.go`
- Test files: `<subject>_test.go` co-located with the package under test
- External test packages use `package <pkg>_test` (e.g. `corpus_test.go` is `package analyze_test`)
- Fixture files: descriptive snake_case with `.zsh` extension (`duplicate_aliases.zsh`, `secrets_inline.zsh`)

**Directories:**
- Role/concept names in lowercase (no underscores at the directory level): `analyze`, `render`, `shell`, `model`, `dto`, `cli`, `cmd`, `testgen`, `util`, `buildinfo`
- Concrete implementations nest under their interface package: `shell/zsh/` under `shell/`
- Binary names mirror the directory under `cmd/`: `cmd/zsh-pro/` → `zsh-pro` binary

**Packages:**
- Package name matches directory name: `package analyze`, `package render`, `package shell`, `package zsh`, `package model`, `package dto`, `package cli`, `package testgen`, `package util`, `package buildinfo`, `package main` (for cmd dirs)

**Types:**
- Exported structs use PascalCase: `Analyzer`, `HumanRenderer`, `JSONRenderer`, `ConfigGraph`, `GenParams`
- Unexported structs use camelCase: `reconciler`, `mockProvider`, `errProvider`, `commandTmpl`
- Interface types are role nouns: `Parser`, `Classifier`, `Introspector`, `Provider`, `Renderer`
- String-typed constants: `BlockKind`, `Category`, `IssueKind` — all exported string constants

## Where to Add New Code

**New shell implementation (e.g. bash support):**
- Create `core/shell/bash/` mirroring `core/shell/zsh/`
- Implement `shell.Provider` interface (`Parser`, `Classifier`, `Introspector`, `Categories()`)
- Add a new composition root at `core/cmd/bash-pro/main.go` that injects `bash.Provider{}`
- Do NOT modify `core/analyze`, `core/cli`, or `core/render`

**New issue kind (e.g. undefined variable):**
- Add constant to `core/model/issue.go`
- Add detection method to `core/analyze/reconciler.go`
- Call from `core/analyze/analyzer.go:Analyze()` at the appropriate point in the pipeline
- Add a new fixture to `core/testdata/fixtures/` and update `manifests.json`
- Add unit test case in `core/analyze/analyze_test.go`

**New output format (e.g. SARIF):**
- Add `core/render/sarif.go` implementing `render.Renderer`
- Add a new flag in `core/cli/cli.go:runAnalyze()` to select the renderer
- No changes to `core/analyze`, `core/model`, or `core/dto` required

**New category:**
- Add constant to `core/model/category.go`
- Add to the ordered slice returned by `model.Categories()`
- Add classification rule in `core/shell/zsh/classify.go`
- Update `manifests.json` and relevant fixtures as needed

**New helper utility:**
- Add to `core/util/` — must import only stdlib; one function per file if substantial

**New fixture for corpus testing:**
- Drop a `.zsh` file in `core/testdata/fixtures/`
- Add a corresponding entry to `core/testdata/fixtures/manifests.json`
- No test code changes required (harness is data-driven via `corpus_test.go`)

**Generating new corpus fixtures programmatically:**
- Run: `go run ./core/cmd/zsh-gen -out core/testdata/fixtures -n N -seed S`
- Promotes generated `.zsh` + `manifests.json` directly into the fixture directory

## Special Directories

**`core/testdata/`:**
- Purpose: Golden corpus fixtures consumed by `core/analyze/corpus_test.go`
- Generated: Partially (via `zsh-gen`); hand-authored files also present
- Committed: Yes — fixtures are version-controlled

**`.planning/`:**
- Purpose: GSD planning documents (codebase maps, phase plans)
- Generated: By GSD tooling
- Committed: Yes

**`docs/superpowers/`:**
- Purpose: Design specs and implementation plans written before/during feature development
- Generated: No (human-authored)
- Committed: Yes

---

*Structure analysis: 2026-06-23*
