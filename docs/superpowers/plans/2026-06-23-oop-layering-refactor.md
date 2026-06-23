# OOP Layering Refactor Implementation Plan

> **Execution:** INLINE (tightly-coupled refactor — a signature change ripples across layers, so tasks are not independently committable in isolation). Proceed in the order below; the build + full suite must be green before each commit. A final independent code review follows the last task.

**Goal:** Restructure the read-only analyze engine onto role types (behavior-as-methods, no loose functions), a separate `dto` wire layer, and a `util` layer — with zero behavior change.

**Architecture:** Logic moves onto `Analyzer`/`reconciler`/`Renderer`/`zsh.Provider`/`CLI` as methods; `model` becomes pure domain data (no json tags); `dto` holds the wire structs; `render` maps model→dto. See `docs/superpowers/specs/2026-06-23-oop-layering-refactor-design.md`.

**Tech Stack:** Go 1.25 (toolchain auto), mvdan/sh v3.13.1.

## Global Constraints

- `--json` output is **byte-for-byte identical** to pre-refactor. Guard: golden corpus + a binary diff in the final task.
- Public CLI behavior + exit codes (0/1/2/3) unchanged.
- `dto`, `util`, `model` import nothing inside `core/` and no third-party libs (`dto` does NOT import `model`).
- `analyze` never imports `shell/zsh`. Only `cmd/zsh-pro` imports `shell/zsh`.
- No free-standing production functions doing work, except (by idiom): `util.*` stateless helpers, `model.Categories()` enumerator, immutable package `var` regexes/tables.
- Verify command after every task: `GOTOOLCHAIN=auto go build ./... && go vet ./... && go test ./...` (all green). `go` resolves to goenv 1.25.7 now.
- One type per file; gofmt clean.

---

### Task 1: model → per-type files + `Category.Description()`

**Files:**
- Delete: `core/model/model.go`
- Create: `core/model/category.go`, `core/model/block.go`, `core/model/identityset.go`, `core/model/issue.go`, `core/model/analysis.go`
- Modify: `core/render/render.go:24`, `core/model/model_test.go`

**Moves (verbatim, keep json tags for now):**
- `category.go`: `Category`, the 10 `Cat*` consts, `Categories()`. Replace the free `CategoryDescription(c Category) string` with method `func (c Category) Description() string` (same switch body).
- `block.go`: `BlockKind` + consts, `Confidence` + consts, `Block`.
- `identityset.go`: `IdentitySet`.
- `issue.go`: `IssueKind` + consts, `Issue` (keep json tags).
- `analysis.go`: `CategorySummary` (keep json tags), `Analysis` (keep json tags), `func (a Analysis) ExitCode() int`.

**Call-site updates:**
- `render.go:24`: `model.CategoryDescription(c.Category)` → `c.Category.Description()`.
- `model_test.go`: 3× `CategoryDescription(cat)` → `cat.Description()` (lines ~25/32/38). Keep `Categories()` and `ExitCode` tests as-is.

- [ ] Verify: `go test ./core/model/... ./core/render/...` green; `go build ./...` green. Commit `refactor(model): one type per file; Category.Description method`.

---

### Task 2: `dto` package — pure wire structs

**Files:**
- Create: `core/dto/analysis.go`, `core/dto/envelope.go`

`core/dto/analysis.go` (json tags replicate the CURRENT model tags exactly):
```go
// Package dto holds the wire-format (JSON) structs for zsh-pro output. These are
// data-only transfer objects, deliberately separate from the domain model in
// core/model. dto imports nothing inside core/ — it is a leaf.
package dto

// Analysis is the wire shape of an analysis result.
type Analysis struct {
	Path         string            `json:"path"`
	Lines        int               `json:"lines"`
	BlockCount   int               `json:"blocks"`
	OpaqueBlocks int               `json:"opaque_blocks"`
	Categories   []CategorySummary `json:"categories"`
	Issues       []Issue           `json:"issues"`
	HasSecrets   bool              `json:"has_secrets"`
	Introspected bool              `json:"introspected"`
	Notes        []string          `json:"notes,omitempty"`
}

// Issue is the wire shape of one reported problem.
type Issue struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Lines []int  `json:"lines,omitempty"`
	Note  string `json:"note,omitempty"`
}

// CategorySummary is the wire shape of one per-category rollup.
type CategorySummary struct {
	Category string   `json:"category"`
	Count    int      `json:"count"`
	Items    []string `json:"items,omitempty"`
}
```

`core/dto/envelope.go`:
```go
package dto

// Envelope is the single top-level JSON object emitted by every command.
type Envelope struct {
	Tool        string   `json:"tool"`
	Version     string   `json:"version"`
	Command     string   `json:"command"`
	OK          bool     `json:"ok"`
	IssuesFound bool     `json:"issues_found"`
	ExitCode    int      `json:"exit_code"`
	Analysis    Analysis `json:"analysis"`
}
```

- [ ] Verify: `go build ./...` green (no consumers yet). Commit `refactor(dto): wire-format types`.

---

### Task 3: `render` role types + dto mapping; strip model json tags

**Files:**
- Delete: `core/render/render.go`
- Create: `core/render/renderer.go`, `core/render/human.go`, `core/render/json.go`
- Modify: `core/cli/cli.go` (render call-sites), `core/render/render_test.go`, model files (strip json tags)

`renderer.go`:
```go
// Package render turns an Analysis into output. Renderers are role types: each
// is a pure function of the model and mutates nothing.
package render

import "zsh-pro/core/model"

// Renderer produces one output representation of an Analysis.
type Renderer interface {
	Render(a model.Analysis) ([]byte, error)
}
```

`human.go`: `type HumanRenderer struct{}` with `func (HumanRenderer) Render(a model.Analysis) ([]byte, error)` — body is the current `Human` builder, returning `[]byte(b.String()), nil`. Replace the `model.CategoryDescription` call (already `c.Category.Description()` after Task 1 — confirm).

`json.go`: `type JSONRenderer struct{}` with `func (JSONRenderer) Render(a model.Analysis) ([]byte, error)`. It builds a `dto.Envelope` from `model` + `buildinfo`, then `json.MarshalIndent(env, "", "  ")`. Mapping is a private method `func (JSONRenderer) toDTO(a model.Analysis) dto.Envelope` mapping field-by-field, copying slices as-is (preserve nil → omitted). `Issue.Kind`/`CategorySummary.Category` are `string(...)` of the model enums.

**Call-site updates:**
- `cli.go`: `render.JSON(a)` → `(render.JSONRenderer{}).Render(a)`; `render.Human(a)` (printed as string) → `b, _ := (render.HumanRenderer{}).Render(a)` then `fmt.Fprintln(stdout, string(b))`. (Human cannot error; ignore err or keep the error branch — keep symmetric error handling.)
- `render_test.go`: `Human(sampleAnalysis())` → `b, _ := (HumanRenderer{}).Render(sampleAnalysis()); out := string(b)`; `JSON(sampleAnalysis())` → `(JSONRenderer{}).Render(sampleAnalysis())`.
- **Strip json tags** from `model/issue.go` (`Issue`) and `model/analysis.go` (`CategorySummary`, `Analysis`) — now unused (dto is the wire source).

- [ ] Verify: `go test ./core/render/... ./core/cli/... ./core/model/...` green; full `go build ./...`. Commit `refactor(render): role types behind Renderer; map model→dto; model is now tag-free`.

---

### Task 4: `util` layer — `ExpandHome`

**Files:**
- Create: `core/util/path.go`
- Modify: `core/cli/cli.go` (remove local `expandHome`, call `util.ExpandHome`)

`util/path.go`:
```go
// Package util holds generic, dependency-free helpers shared across layers.
package util

import (
	"os"
	"path/filepath"
)

// ExpandHome resolves a leading ~ or ~/ to the user's home directory. On any
// failure it returns the path unchanged.
func ExpandHome(p string) string {
	if p == "~" || (len(p) >= 2 && p[:2] == "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
```
- `cli.go`: delete `func expandHome`, change `path = expandHome(path)` → `path = util.ExpandHome(path)`; add `"zsh-pro/core/util"` import, drop now-unused `path/filepath` if no longer used.

- [ ] Verify: `go test ./core/cli/...` green; `go build ./...`. Commit `refactor(util): extract ExpandHome into util layer`.

---

### Task 5: `analyze` role types — `Analyzer` + `reconciler`

**Files:**
- Delete: `core/analyze/analyze.go`, `core/analyze/issues.go`, `core/analyze/summary.go`
- Create: `core/analyze/analyzer.go`, `core/analyze/reconciler.go`
- Modify: `core/cli/cli.go`, `core/analyze/analyze_test.go`, `core/analyze/corpus_test.go`

`analyzer.go`:
```go
// Package analyze drives a shell.Provider and reconciles the static and dynamic
// views into one model.Analysis. It depends only on core/model and core/shell.
package analyze

import (
	"sort"
	"strings"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// Analyzer reconciles a parsed+classified config into a read-only Analysis.
type Analyzer struct {
	provider shell.Provider
	rec      reconciler
}

// New returns an Analyzer bound to a shell Provider.
func New(p shell.Provider) *Analyzer { return &Analyzer{provider: p} }

// Analyze produces a read-only Analysis. It never panics; a failed/unavailable
// introspection degrades to static-only with an explanatory note.
func (az *Analyzer) Analyze(src []byte, path string) model.Analysis {
	// body: the current analyze.Analyze, with these substitutions:
	//   p.Parse/Classify/Categories/Introspect -> az.provider.Parse/...
	//   primaryName(b)        -> az.rec.primaryName(b)
	//   dupNameIssues(...)    -> az.rec.duplicateNames(...)
	//   dupPathIssues(...)    -> az.rec.duplicatePaths(...)
	//   shadowIssues(blocks)  -> az.rec.shadows(blocks)
	// (sort import retained for the final SliceStable; strings for line count.)
}
```

`reconciler.go`: `type reconciler struct{}` plus `pathSegRe` package var (moved from issues.go) and methods:
- `func (reconciler) primaryName(b model.Block) string` — verbatim body from summary.go.
- `func (reconciler) duplicateNames(blocks []model.Block, kind model.IssueKind) []model.Issue` — verbatim `dupNameIssues` body.
- `func (reconciler) duplicatePaths(blocks []model.Block) []model.Issue` — verbatim `dupPathIssues` body.
- `func (reconciler) shadows(blocks []model.Block) []model.Issue` — verbatim `shadowIssues` body (keep the long explanatory comment).

**Call-site updates:**
- `cli.go`: `analyze.Analyze(zsh.Provider{}, src, path)` → `analyze.New(zsh.Provider{}).Analyze(src, path)` (zsh import still present here until Task 7).
- `analyze_test.go`: 5× `Analyze(p, ...)` → `New(p).Analyze(...)` (drop the now-removed `src`-first/provider-first arg order: `New(p).Analyze(src, path)`).
- `corpus_test.go:45`: `analyze.Analyze(zsh.Provider{}, src, filepath.Join(...))` → `analyze.New(zsh.Provider{}).Analyze(src, filepath.Join(...))`.

- [ ] Verify: `go test ./core/analyze/... ./core/cli/...` green; `go build ./...`. Commit `refactor(analyze): Analyzer + reconciler role types`.

---

### Task 6: `zsh` — attach parse/introspect helpers as Provider methods

**Files:**
- Modify: `core/shell/zsh/parse.go`, `core/shell/zsh/introspect.go`

- `parse.go`: `func describe(stmt *syntax.Stmt, b *model.Block)` → `func (Provider) describe(...)`; `func wordLitPrefix(w *syntax.Word) string` → `func (Provider) wordLitPrefix(...)`. Update internal calls inside `Parse`: `describe(stmt, &b)` → `p.describe(stmt, &b)` (give `Parse` a named receiver `p`); inside `describe`, `wordLitPrefix(w)` → `p.wordLitPrefix(w)` (named receiver `p`).
- `introspect.go`: `func parseIntrospect(s string) model.IdentitySet` → `func (Provider) parseIntrospect(...)`; in `Introspect`, give it receiver `p` and call `p.parseIntrospect(out.String())`.
- `secretRe`/`pluginHints` stay as package vars (idiom). Tests call only public `Parse`/`Classify`/`Introspect` — unaffected.

- [ ] Verify: `go test ./core/shell/zsh/...` green; `go build ./...`. Commit `refactor(zsh): parse/introspect helpers are now Provider methods`.

---

### Task 7: `cli` role type + composition root in main

**Files:**
- Delete: `core/cli/cli.go`
- Create: `core/cli/cli.go` (rewritten as a role type)
- Modify: `core/cmd/zsh-pro/main.go`, `core/cli/cli_test.go`

`cli.go` (depends on `shell` interface + `analyze`/`render`/`util`/`buildinfo`; NO `shell/zsh`):
```go
package cli

// imports: encoding/json, fmt, io, os, zsh-pro/core/{analyze,buildinfo,render,shell,util}

// CLI wires flags and I/O to the engine for a given shell Provider.
type CLI struct{ provider shell.Provider }

// New returns a CLI bound to a Provider.
func New(p shell.Provider) *CLI { return &CLI{provider: p} }

// Run executes a command. Exit codes: 0 clean, 1 runtime, 2 usage, 3 actionable.
func (c *CLI) Run(args []string, stdout, stderr io.Writer) int { /* current Run body */ }

func (c *CLI) runAnalyze(args []string, stdout, stderr io.Writer) int {
	// current runAnalyze body, with:
	//   expandHome(path)            -> util.ExpandHome(path)
	//   analyze.Analyze(zsh.Provider{}, src, path) -> analyze.New(c.provider).Analyze(src, path)
	//   render.JSON(a)/Human(a)     -> (render.JSONRenderer{}).Render(a) / (render.HumanRenderer{}).Render(a)
	//   fail(...)                   -> c.fail(...)
}

func (c *CLI) fail(stdout, stderr io.Writer, asJSON bool, msg string) int { /* current fail body */ }
```

`main.go`:
```go
package main

import (
	"os"

	"zsh-pro/core/cli"
	"zsh-pro/core/shell/zsh"
)

func main() {
	os.Exit(cli.New(zsh.Provider{}).Run(os.Args[1:], os.Stdout, os.Stderr))
}
```

`cli_test.go`: add import `"zsh-pro/core/shell/zsh"`; replace each `Run([]string{...}, &out, &errBuf)` (5×) with `New(zsh.Provider{}).Run([]string{...}, &out, &errBuf)`.

- [ ] Verify: `go test ./...` green; `go build ./...`. Commit `refactor(cli): CLI role type; main is the sole composition root`.

---

### Task 8: whole-program verification

**Files:** none (verification only)

- [ ] `gofmt -l core` → empty (else `gofmt -w core`).
- [ ] `go vet ./...` → clean.
- [ ] `go test ./...` → all green.
- [ ] Agnostic boundary: `go list -deps ./core/analyze | grep 'shell/zsh'` → empty; `go list -deps ./core/model ./core/dto ./core/util | grep 'zsh-pro/core'` → empty (leaves).
- [ ] **`--json` byte-identical**: build the binary; run `./bin analyze core/testdata/fixtures/duplicate_aliases.zsh --json` and diff against the same output captured from `main` before the refactor (capture pre-image first). Expect no diff.
- [ ] Commit any gofmt-only changes: `chore: gofmt`.

---

## Self-Review

**Spec coverage:** role-type catalog (Tasks 1,3,5,6,7) ✓; dto layer (Task 2) ✓; util layer (Task 4) ✓; model purity / tag strip (Tasks 1,3) ✓; dependency rules + agnostic check (Task 8) ✓; byte-identical json (Tasks 2,3,8) ✓; idiom calls — regexes/Categories stay (Tasks 5,6,1) ✓.

**Placeholder scan:** code blocks reference verbatim moves of named, existing functions (bodies exist in the current tree); new code (dto, interface, constructors, util, main) is given in full. No TBD/vague steps.

**Type consistency:** `analyze.New(p) *Analyzer` + `(*Analyzer).Analyze(src, path)`; `render.{Human,JSON}Renderer` + `Renderer.Render(model.Analysis) ([]byte,error)`; `cli.New(p) *CLI` + `(*CLI).Run`; `(Category).Description()`; `dto.{Envelope,Analysis,Issue,CategorySummary}` — consistent across tasks and matches the spec catalog.
