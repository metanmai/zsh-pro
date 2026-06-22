# zsh-pro Analyze Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only zsh configuration analysis engine that parses a `.zshrc`, classifies it into categories, and reports duplicates/shadowing/conflicts — as human output and `--json`.

**Architecture:** A pipeline of small Go packages. A shell-agnostic `core` (model, reconcile, render, CLI) drives a shell-specific `Provider`; the zsh `Provider` parses with mvdan/sh (static) and introspects via a sandboxed `zsh -f` + `zsh/parameter` (dynamic). The two views are reconciled into one `Analysis` model that the renderers consume. Nothing is ever written or reorganized.

**Tech Stack:** Go (1.22+), [mvdan.cc/sh/v3](https://github.com/mvdan/sh) for parsing (`LangVariant=Zsh`, ≥ v3.13), the system `zsh` binary for introspection, Go stdlib for everything else.

**Spec:** `docs/superpowers/specs/2026-06-22-analyze-engine-design.md`

## Global Constraints

- Single Go module, module path `zsh-pro`. Monorepo: all engine code under `core/`; `ui/` reserved for the future TUI (untouched here).
- Go version floor: `go 1.22`.
- External dependency limited to `mvdan.cc/sh/v3` (≥ v3.13.0, for zsh support). No other third-party deps.
- Read-only: the engine reads `~/.zshrc` and runs it in a sandbox; it never writes to user files.
- `zsh` is required for the dynamic pass; if it is missing/errors/times out, **degrade to static-only and say so** — never crash.
- CLI contract: exit codes `0` clean · `1` runtime error · `2` usage error · `3` actionable (issues found). `--json` emits exactly one JSON object on stdout.
- Binary name: `zsh-pro`.

**Architecture rules (every task must honor these):**
1. Dependencies point inward to `model`; nothing in `core/` imports `core/shell/zsh` except the composition root (`cli`/`main`).
2. `model` is pure data + trivial methods — no logic, no I/O, no third-party imports.
3. Concerns sit behind segregated interfaces (`Parser`, `Classifier`, `Introspector`); consumers depend on the narrowest interface they need.
4. Errors are returned, never panicked; degrade gracefully (never crash on bad input or a missing/erroring zsh).
5. One responsibility per file; files kept small; no global mutable state — pass dependencies and I/O explicitly.

---

### Task 1: Scaffold the Go module and buildable skeleton

**Files:**
- Create: `go.mod`
- Create: `core/buildinfo/buildinfo.go`
- Test: `core/buildinfo/buildinfo_test.go`
- Create: `core/cmd/zsh-pro/main.go`
- Create: `.gitignore`

**Interfaces:**
- Consumes: nothing.
- Produces: `buildinfo.Version string` constant; a `zsh-pro` binary that prints its version.

- [ ] **Step 1: Initialize the module and add the dependency**

Run:
```bash
go mod init zsh-pro
go get mvdan.cc/sh/v3@latest
```
Expected: `go.mod` created with `module zsh-pro` and a `require mvdan.cc/sh/v3 vX.Y.Z` line where `X.Y.Z >= 3.13.0`. If the resolved version is below 3.13, run `go get mvdan.cc/sh/v3@v3.13.0`.

- [ ] **Step 2: Write the failing test**

Create `core/buildinfo/buildinfo_test.go`:
```go
package buildinfo

import "testing"

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./core/buildinfo/ -run TestVersionIsSet -v`
Expected: FAIL — build error, `undefined: Version`.

- [ ] **Step 4: Write the minimal implementation**

Create `core/buildinfo/buildinfo.go`:
```go
// Package buildinfo holds static build metadata.
package buildinfo

// Version is the zsh-pro release version.
const Version = "0.1.0"
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./core/buildinfo/ -run TestVersionIsSet -v`
Expected: PASS.

- [ ] **Step 6: Add the entrypoint and gitignore**

Create `core/cmd/zsh-pro/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"zsh-pro/core/buildinfo"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("zsh-pro %s\n", buildinfo.Version)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: zsh-pro analyze [path] [--json]")
	os.Exit(2)
}
```

Create `.gitignore`:
```
/zsh-pro
*.test
```

- [ ] **Step 7: Verify the build and binary**

Run:
```bash
go build ./... && go vet ./... && go run ./core/cmd/zsh-pro --version
```
Expected: builds clean, prints `zsh-pro 0.1.0`.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum core/ .gitignore
git commit -m "feat: scaffold Go module and buildable skeleton"
```

---

### Task 2: Core model types

**Files:**
- Create: `core/model/model.go`
- Test: `core/model/model_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `model.Category` (+ constants + `Categories() []Category`), `model.BlockKind` (+ constants), `model.Confidence` (+ constants), `model.Block`, `model.IdentitySet`, `model.IssueKind` (+ constants), `model.Issue`, `model.CategorySummary`, `model.Analysis` (+ method `ExitCode() int`).

- [ ] **Step 1: Write the failing test**

Create `core/model/model_test.go`:
```go
package model

import "testing"

func TestCategoriesOrderedAndComplete(t *testing.T) {
	got := Categories()
	want := []Category{
		CatEnvironment, CatPath, CatSecrets, CatPlugins, CatOptions,
		CatKeybindings, CatFunctions, CatAliases, CatLocal, CatMisc,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d categories, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestExitCode(t *testing.T) {
	clean := Analysis{}
	if clean.ExitCode() != 0 {
		t.Errorf("clean analysis: got %d want 0", clean.ExitCode())
	}
	dirty := Analysis{Issues: []Issue{{Kind: IssueDuplicateAlias, Name: "gs"}}}
	if dirty.ExitCode() != 3 {
		t.Errorf("analysis with issues: got %d want 3", dirty.ExitCode())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/model/ -v`
Expected: FAIL — build error, undefined identifiers.

- [ ] **Step 3: Write the implementation**

Create `core/model/model.go`:
```go
// Package model holds the shell-agnostic data types shared across the engine.
package model

// Category is one bucket in the (load-order) taxonomy.
type Category string

const (
	CatEnvironment Category = "environment"
	CatPath        Category = "path"
	CatSecrets     Category = "secrets"
	CatPlugins     Category = "plugins"
	CatOptions     Category = "options"
	CatKeybindings Category = "keybindings"
	CatFunctions   Category = "functions"
	CatAliases     Category = "aliases"
	CatLocal       Category = "local"
	CatMisc        Category = "misc"
)

// Categories returns the categories in taxonomy (load) order.
func Categories() []Category {
	return []Category{
		CatEnvironment, CatPath, CatSecrets, CatPlugins, CatOptions,
		CatKeybindings, CatFunctions, CatAliases, CatLocal, CatMisc,
	}
}

// CategoryDescription is a human-readable label for a category.
func CategoryDescription(c Category) string {
	switch c {
	case CatEnvironment:
		return "Environment variables / exports"
	case CatPath:
		return "PATH / fpath manipulation"
	case CatSecrets:
		return "Secrets & tokens (do not sync)"
	case CatPlugins:
		return "Plugin managers, framework & tool init"
	case CatOptions:
		return "Shell options (setopt/zstyle/autoload)"
	case CatKeybindings:
		return "Key bindings (bindkey)"
	case CatFunctions:
		return "Function definitions"
	case CatAliases:
		return "Aliases"
	case CatLocal:
		return "Machine / OS-specific overrides"
	default:
		return "Uncategorized — review by hand"
	}
}

// BlockKind is the agnostic structural shape of a parsed block, extracted by
// the shell Provider's parser so the (shell-specific) classifier and the
// (agnostic) reconciler can work without re-parsing.
type BlockKind string

const (
	KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar
	KindAlias      BlockKind = "alias"       // alias gs=...
	KindFuncDecl   BlockKind = "func"        // foo() { ... }
	KindCommand    BlockKind = "command"     // a simple command call (setopt, bindkey, source, eval, ...)
	KindCompound   BlockKind = "compound"    // if/for/while/case/{...}/subshell
	KindOther      BlockKind = "other"
)

// Confidence expresses how sure the classifier is about a block's category.
type Confidence int

const (
	ConfLow Confidence = iota
	ConfMedium
	ConfHigh
)

// Block is one logical construct from the source (plus leading comments).
type Block struct {
	Text      string     // original source text (incl. leading comments)
	StartLine int        // 1-based line in source
	Kind      BlockKind  // structural shape (set by parser)
	CmdName   string     // leading command word for KindCommand (e.g. "setopt")
	Names     []string   // assigned var names / func name / alias name(s)
	Exported  bool       // assignment used `export`
	Opaque    bool       // parser could not structurally understand this block
	Category  Category   // set by classifier
	Conf      Confidence // set by classifier
}

// IdentitySet is the resolved end-state captured by dynamic introspection.
type IdentitySet struct {
	Aliases   map[string]bool // alias names present after sourcing
	Functions map[string]bool // function names present after sourcing
	Env       map[string]bool // exported variable names present after sourcing
	Path      []string        // resolved $path entries, in order
	Options   map[string]bool // shell options that are "on"
	Available bool            // false when introspection failed (static-only)
}

// IssueKind enumerates the problems the engine reports.
type IssueKind string

const (
	IssueDuplicateAlias IssueKind = "duplicate_alias"
	IssueReassignedEnv  IssueKind = "reassigned_env"
	IssueDuplicatePath  IssueKind = "duplicate_path"
	IssueShadowed       IssueKind = "shadowed"
)

// Issue is one reported problem.
type Issue struct {
	Kind  IssueKind `json:"kind"`
	Name  string    `json:"name"`
	Lines []int     `json:"lines,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// CategorySummary is the per-category rollup for the report.
type CategorySummary struct {
	Category Category `json:"category"`
	Count    int      `json:"count"`
	Items    []string `json:"items"`
}

// Analysis is the complete read-only result the renderers consume.
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

// ExitCode is 3 when actionable issues exist, else 0. Runtime (1) and usage
// (2) errors are handled by the CLI, not derived from a successful Analysis.
func (a Analysis) ExitCode() int {
	if len(a.Issues) > 0 {
		return 3
	}
	return 0
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/model/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add core/model/
git commit -m "feat: add core model types"
```

---

### Task 3: zsh parser → blocks (+ Provider interface)

**Files:**
- Create: `core/shell/provider.go`
- Create: `core/shell/zsh/zsh.go`
- Create: `core/shell/zsh/parse.go`
- Test: `core/shell/zsh/parse_test.go`

**Interfaces:**
- Consumes: `model.Block`, `model.BlockKind` constants.
- Produces: segregated `shell.Parser`, `shell.Classifier`, `shell.Introspector` interfaces (composed as `shell.Provider`); `zsh.Provider` struct; `(zsh.Provider) Parse(src []byte) ([]model.Block, error)`.

- [ ] **Step 1: Confirm the mvdan/sh zsh constant**

Run: `go doc mvdan.cc/sh/v3/syntax LangVariant`
Expected: a list of `LangVariant` constants including a zsh one (e.g. `LangZsh`). Use the exact name printed in the next step's `syntax.Variant(...)` call if it differs from `LangZsh`.

- [ ] **Step 2: Define the segregated seam interfaces**

Create `core/shell/provider.go`:
```go
// Package shell defines the shell-agnostic seam. Each concern is its own small
// interface (interface segregation); Provider composes them for wiring at the
// composition root. The rest of the engine depends on the narrowest interface
// it needs.
package shell

import "zsh-pro/core/model"

// Parser turns source into ordered, structurally-described blocks.
type Parser interface {
	Parse(src []byte) ([]model.Block, error)
}

// Classifier owns the category space and assigns a block to it.
type Classifier interface {
	Classify(b model.Block) (model.Category, model.Confidence)
	Categories() []model.Category
}

// Introspector runs the config in a sandbox and returns the resolved identity
// set. On failure it returns IdentitySet{Available: false}.
type Introspector interface {
	Introspect(path string) (model.IdentitySet, error)
}

// Provider composes the three concerns for convenient wiring.
type Provider interface {
	Parser
	Classifier
	Introspector
}
```

- [ ] **Step 3: Write the failing test**

Create `core/shell/zsh/parse_test.go`:
```go
package zsh

import (
	"testing"

	"zsh-pro/core/model"
)

func TestParseClassifiesKinds(t *testing.T) {
	src := []byte(`# my aliases
alias gs='git status'
export EDITOR=nvim
PATH="$HOME/bin:$PATH"
greet() { echo hi }
setopt AUTO_CD
if [[ "$OSTYPE" == darwin* ]]; then
  alias ls='ls -G'
fi
`)
	blocks, err := (Provider{}).Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(blocks) != 6 {
		t.Fatalf("got %d blocks, want 6", len(blocks))
	}

	cases := []struct {
		idx     int
		kind    model.BlockKind
		cmdName string
		name    string
	}{
		{0, model.KindAlias, "alias", "gs"},
		{1, model.KindAssignment, "export", "EDITOR"},
		{2, model.KindAssignment, "", "PATH"},
		{3, model.KindFuncDecl, "", "greet"},
		{4, model.KindCommand, "setopt", ""},
		{5, model.KindCompound, "", ""},
	}
	for _, c := range cases {
		b := blocks[c.idx]
		if b.Kind != c.kind {
			t.Errorf("block %d: kind %q, want %q", c.idx, b.Kind, c.kind)
		}
		if c.cmdName != "" && b.CmdName != c.cmdName {
			t.Errorf("block %d: cmdName %q, want %q", c.idx, b.CmdName, c.cmdName)
		}
		if c.name != "" && (len(b.Names) == 0 || b.Names[0] != c.name) {
			t.Errorf("block %d: names %v, want first=%q", c.idx, b.Names, c.name)
		}
	}
	// leading comment is attached to the alias block.
	if blocks[0].StartLine != 1 {
		t.Errorf("alias block StartLine = %d, want 1 (leading comment)", blocks[0].StartLine)
	}
}

func TestParseOpaqueOnUnparseable(t *testing.T) {
	// A hard syntax error must not crash; it yields one opaque block.
	blocks, err := (Provider{}).Parse([]byte("if then fi fi ;;"))
	if err != nil {
		t.Fatalf("Parse should not error on bad input, got %v", err)
	}
	if len(blocks) != 1 || !blocks[0].Opaque {
		t.Fatalf("want one opaque block, got %+v", blocks)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./core/shell/zsh/ -run TestParse -v`
Expected: FAIL — `Provider` / `Parse` undefined.

- [ ] **Step 5: Write the implementation**

Create `core/shell/zsh/zsh.go`:
```go
// Package zsh is the zsh implementation of shell.Provider.
package zsh

// Provider implements shell.Provider for zsh.
type Provider struct{}
```

Create `core/shell/zsh/parse.go`:
```go
package zsh

import (
	"bytes"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"zsh-pro/core/model"
)

// Parse turns zsh source into ordered, structurally-described blocks. A block
// that the parser cannot understand (or a whole file that fails to parse) is
// returned as an opaque block rather than causing an error.
func (Provider) Parse(src []byte) ([]model.Block, error) {
	parser := syntax.NewParser(
		syntax.Variant(syntax.LangZsh), // confirmed in Step 1
		syntax.KeepComments(true),
	)
	file, err := parser.Parse(bytes.NewReader(src), "")
	if err != nil {
		return []model.Block{{
			Text:      string(src),
			StartLine: 1,
			Kind:      model.KindOther,
			Opaque:    true,
		}}, nil
	}

	var blocks []model.Block
	for _, stmt := range file.Stmts {
		start := stmt.Pos().Offset()
		// Pull in leading comments that sit directly above the statement.
		for _, c := range stmt.Comments {
			if c.End().Offset() <= stmt.Pos().Offset() && c.Pos().Offset() < start {
				start = c.Pos().Offset()
			}
		}
		end := stmt.End().Offset()
		if int(end) > len(src) {
			end = uint(len(src))
		}

		b := model.Block{
			Text:      strings.TrimRight(string(src[start:end]), "\n"),
			StartLine: int(stmt.Pos().Line()),
		}
		describe(stmt, &b)
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// describe fills in the agnostic structural fields (Kind, CmdName, Names,
// Exported) from the mvdan/sh AST node.
func describe(stmt *syntax.Stmt, b *model.Block) {
	switch c := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		// Pure assignment: leading assignments and no command words.
		if len(c.Args) == 0 {
			b.Kind = model.KindAssignment
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
			}
			return
		}
		name := c.Args[0].Lit()
		b.CmdName = name
		switch name {
		case "alias":
			b.Kind = model.KindAlias
			for _, w := range c.Args[1:] {
				lit := w.Lit()
				if i := strings.IndexByte(lit, '='); i > 0 {
					b.Names = append(b.Names, lit[:i])
				}
			}
		case "export", "typeset", "declare", "local", "readonly":
			b.Kind = model.KindAssignment
			b.Exported = name == "export"
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
			}
			for _, w := range c.Args[1:] {
				lit := w.Lit()
				if lit == "" || strings.HasPrefix(lit, "-") {
					continue
				}
				if i := strings.IndexByte(lit, '='); i > 0 {
					b.Names = append(b.Names, lit[:i])
				} else {
					b.Names = append(b.Names, lit)
				}
			}
		default:
			b.Kind = model.KindCommand
		}
	case *syntax.FuncDecl:
		b.Kind = model.KindFuncDecl
		if c.Name != nil {
			b.Names = append(b.Names, c.Name.Value)
		}
	case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause,
		*syntax.CaseClause, *syntax.Block, *syntax.Subshell:
		b.Kind = model.KindCompound
	default:
		b.Kind = model.KindOther
	}
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./core/shell/zsh/ -run TestParse -v`
Expected: PASS. If a `cmdName`/`Names` assertion fails, re-check `Word.Lit()` against `go doc mvdan.cc/sh/v3/syntax.Word` and adjust `describe`; if the zsh `Variant` constant name differs, fix it from Step 1.

- [ ] **Step 7: Commit**

```bash
git add core/shell/
git commit -m "feat: zsh parser to structured blocks via mvdan/sh"
```

---

### Task 4: zsh classifier

**Files:**
- Create: `core/shell/zsh/classify.go`
- Test: `core/shell/zsh/classify_test.go`

**Interfaces:**
- Consumes: `model.Block`, `model.Category`/`model.Confidence` constants.
- Produces: `(zsh.Provider) Classify(b model.Block) (model.Category, model.Confidence)`.

- [ ] **Step 1: Write the failing test**

Create `core/shell/zsh/classify_test.go`:
```go
package zsh

import (
	"testing"

	"zsh-pro/core/model"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		b    model.Block
		want model.Category
		conf model.Confidence
	}{
		{"alias", model.Block{Kind: model.KindAlias, CmdName: "alias", Names: []string{"gs"}}, model.CatAliases, model.ConfHigh},
		{"func", model.Block{Kind: model.KindFuncDecl, Names: []string{"greet"}}, model.CatFunctions, model.ConfHigh},
		{"env", model.Block{Kind: model.KindAssignment, Names: []string{"EDITOR"}, Text: "export EDITOR=nvim"}, model.CatEnvironment, model.ConfHigh},
		{"path", model.Block{Kind: model.KindAssignment, Names: []string{"PATH"}, Text: `PATH="$HOME/bin:$PATH"`}, model.CatPath, model.ConfMedium},
		{"secret", model.Block{Kind: model.KindAssignment, Names: []string{"GITHUB_TOKEN"}, Text: "export GITHUB_TOKEN=abc"}, model.CatSecrets, model.ConfHigh},
		{"setopt", model.Block{Kind: model.KindCommand, CmdName: "setopt", Text: "setopt AUTO_CD"}, model.CatOptions, model.ConfHigh},
		{"bindkey", model.Block{Kind: model.KindCommand, CmdName: "bindkey", Text: "bindkey '^R' history-incremental-search-backward"}, model.CatKeybindings, model.ConfHigh},
		{"eval-init", model.Block{Kind: model.KindCommand, CmdName: "eval", Text: `eval "$(starship init zsh)"`}, model.CatPlugins, model.ConfMedium},
		{"source", model.Block{Kind: model.KindCommand, CmdName: "source", Text: "source ~/.oh-my-zsh/oh-my-zsh.sh"}, model.CatPlugins, model.ConfMedium},
		{"os-conditional", model.Block{Kind: model.KindCompound, Text: `if [[ "$OSTYPE" == darwin* ]]; then alias ls='ls -G'; fi`}, model.CatLocal, model.ConfMedium},
		{"opaque", model.Block{Opaque: true, Text: "garbled"}, model.CatMisc, model.ConfLow},
		{"unknown-cmd", model.Block{Kind: model.KindCommand, CmdName: "fortune", Text: "fortune"}, model.CatMisc, model.ConfLow},
	}
	for _, c := range cases {
		gotCat, gotConf := (Provider{}).Classify(c.b)
		if gotCat != c.want || gotConf != c.conf {
			t.Errorf("%s: got (%q, %d), want (%q, %d)", c.name, gotCat, gotConf, c.want, c.conf)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/shell/zsh/ -run TestClassify -v`
Expected: FAIL — `Classify` undefined.

- [ ] **Step 3: Write the implementation**

Create `core/shell/zsh/classify.go`:
```go
package zsh

import (
	"regexp"
	"strings"

	"zsh-pro/core/model"
)

var secretRe = regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWD|PASSWORD|API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|CLIENT[_-]?SECRET|AUTH[_-]?TOKEN|APIKEY)`)

var pluginHints = []string{
	"oh-my-zsh", "ohmyzsh", "zinit", "antigen", "zplug", "zgen",
	"antibody", "sheldon", "zcomet", "starship", "zoxide", "pyenv", "nvm",
}

// Classify maps a parsed block to a category with a confidence. Low-confidence
// results land in CatMisc so the report can flag them for review rather than
// silently misfiling them.
func (Provider) Classify(b model.Block) (model.Category, model.Confidence) {
	if b.Opaque {
		return model.CatMisc, model.ConfLow
	}
	text := strings.ToLower(b.Text)

	switch b.Kind {
	case model.KindAlias:
		return model.CatAliases, model.ConfHigh
	case model.KindFuncDecl:
		return model.CatFunctions, model.ConfHigh
	case model.KindAssignment:
		for _, n := range b.Names {
			if secretRe.MatchString(n) {
				return model.CatSecrets, model.ConfHigh
			}
		}
		for _, n := range b.Names {
			u := strings.ToUpper(n)
			if u == "PATH" || u == "FPATH" || u == "MANPATH" || u == "CDPATH" || strings.Contains(u, "PATH") {
				return model.CatPath, model.ConfMedium
			}
		}
		return model.CatEnvironment, model.ConfHigh
	case model.KindCommand:
		switch b.CmdName {
		case "setopt", "unsetopt", "zstyle", "autoload", "compinit", "compdef", "zmodload":
			return model.CatOptions, model.ConfHigh
		case "bindkey":
			return model.CatKeybindings, model.ConfHigh
		case "source", ".":
			return model.CatPlugins, model.ConfMedium
		case "eval":
			if strings.Contains(text, "init") || strings.Contains(text, "hook") || strings.Contains(text, "shellenv") {
				return model.CatPlugins, model.ConfMedium
			}
			return model.CatMisc, model.ConfLow
		}
		for _, h := range pluginHints {
			if strings.Contains(text, h) {
				return model.CatPlugins, model.ConfMedium
			}
		}
		return model.CatMisc, model.ConfLow
	case model.KindCompound:
		if strings.Contains(text, "ostype") || strings.Contains(text, "uname") ||
			strings.Contains(text, "darwin") || strings.Contains(text, "linux") ||
			strings.Contains(text, "$host") || strings.Contains(text, "hostname") {
			return model.CatLocal, model.ConfMedium
		}
		return model.CatMisc, model.ConfLow
	default:
		for _, h := range pluginHints {
			if strings.Contains(text, h) {
				return model.CatPlugins, model.ConfLow
			}
		}
		return model.CatMisc, model.ConfLow
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/shell/zsh/ -run TestClassify -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/shell/zsh/classify.go core/shell/zsh/classify_test.go
git commit -m "feat: zsh classifier with confidence and unsure bucket"
```

---

### Task 5: zsh introspection (+ Categories + Provider assertion)

**Files:**
- Create: `core/shell/zsh/introspect.go`
- Test: `core/shell/zsh/introspect_test.go`

**Interfaces:**
- Consumes: `model.IdentitySet`, `model.Category`, `shell.Provider`.
- Produces: `(zsh.Provider) Introspect(path string) (model.IdentitySet, error)`; `(zsh.Provider) Categories() []model.Category`; compile-time assertion `var _ shell.Provider = Provider{}`.

- [ ] **Step 1: Write the failing test**

Create `core/shell/zsh/introspect_test.go`:
```go
package zsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntrospectReadsAliasesAndFunctions(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping dynamic introspection test")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rc.zsh")
	content := "alias gs='git status'\ngreet() { echo hi }\nexport MYVAR=1\npath+=(\"$HOME/bin\")\nsetopt AUTO_CD\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err := (Provider{}).Introspect(cfg)
	if err != nil {
		t.Fatalf("Introspect error: %v", err)
	}
	if !ids.Available {
		t.Fatal("expected Available=true")
	}
	if !ids.Aliases["gs"] {
		t.Errorf("alias gs missing from %v", ids.Aliases)
	}
	if !ids.Functions["greet"] {
		t.Errorf("function greet missing from %v", ids.Functions)
	}
	if !ids.Env["MYVAR"] {
		t.Errorf("env MYVAR missing from %v", ids.Env)
	}
}

func TestIntrospectMissingFileDegrades(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	ids, err := (Provider{}).Introspect("/nonexistent/path/rc.zsh")
	// Sourcing a missing file is suppressed; introspection still returns the
	// (empty) resolved set with Available=true. The key guarantee: no panic.
	if err == nil && !ids.Available {
		t.Fatal("inconsistent: no error but Available=false")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/shell/zsh/ -run TestIntrospect -v`
Expected: FAIL — `Introspect` undefined.

- [ ] **Step 3: Write the implementation**

Create `core/shell/zsh/introspect.go`:
```go
package zsh

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

var _ shell.Provider = Provider{}

// Categories returns the taxonomy in load order.
func (Provider) Categories() []model.Category { return model.Categories() }

// introspectScript runs under `zsh -f` (no rc files). It sources the target
// ($1) with output suppressed, then dumps the resolved identity tables in a
// section-delimited format.
const introspectScript = `
emulate -L zsh
zmodload zsh/parameter 2>/dev/null
source "$1" >/dev/null 2>&1
print -r -- '##ALIASES##'
for k in "${(@k)aliases}"; do print -r -- "$k"; done
print -r -- '##FUNCTIONS##'
for k in "${(@k)functions}"; do print -r -- "$k"; done
print -r -- '##ENV##'
for k v in "${(@kv)parameters}"; do [[ "$v" == *export* ]] && print -r -- "$k"; done
print -r -- '##PATH##'
for p in $path; do print -r -- "$p"; done
print -r -- '##OPTIONS##'
for k in "${(@k)options}"; do [[ "${options[$k]}" == on ]] && print -r -- "$k"; done
print -r -- '##END##'
`

// Introspect runs the config in a sandboxed zsh and returns the resolved
// identity set. Any failure (zsh missing, timeout) returns Available:false.
func (Provider) Introspect(path string) (model.IdentitySet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, "zsh-pro", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return model.IdentitySet{Available: false}, err
	}
	return parseIntrospect(out.String()), nil
}

func parseIntrospect(s string) model.IdentitySet {
	ids := model.IdentitySet{
		Aliases:   map[string]bool{},
		Functions: map[string]bool{},
		Env:       map[string]bool{},
		Options:   map[string]bool{},
		Available: true,
	}
	section := ""
	for _, line := range strings.Split(s, "\n") {
		switch line {
		case "##ALIASES##", "##FUNCTIONS##", "##ENV##", "##PATH##", "##OPTIONS##", "##END##":
			section = line
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "##ALIASES##":
			ids.Aliases[line] = true
		case "##FUNCTIONS##":
			ids.Functions[line] = true
		case "##ENV##":
			ids.Env[line] = true
		case "##PATH##":
			ids.Path = append(ids.Path, line)
		case "##OPTIONS##":
			ids.Options[line] = true
		}
	}
	return ids
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./core/shell/zsh/ -v`
Expected: PASS (all zsh package tests; introspection tests skip if `zsh` is absent). The `var _ shell.Provider = Provider{}` line will fail to compile if any interface method is missing — that is the intended completeness check.

- [ ] **Step 5: Commit**

```bash
git add core/shell/zsh/introspect.go core/shell/zsh/introspect_test.go
git commit -m "feat: sandboxed zsh introspection via zsh/parameter"
```

---

### Task 6: analyze / reconcile

**Files:**
- Create: `core/analyze/analyze.go`
- Test: `core/analyze/analyze_test.go`

**Interfaces:**
- Consumes: `shell.Provider`, `model.Block`, `model.Analysis`, `model.Issue`, `model.IdentitySet`.
- Produces: `analyze.Analyze(p shell.Provider, src []byte, path string) model.Analysis`.

- [ ] **Step 1: Write the failing test**

Create `core/analyze/analyze_test.go`:
```go
package analyze

import (
	"testing"

	"zsh-pro/core/model"
)

// mockProvider returns pre-baked blocks and identities so the reconciler can
// be tested deterministically without invoking zsh.
type mockProvider struct {
	blocks []model.Block
	ids    model.IdentitySet
}

func (m mockProvider) Parse(_ []byte) ([]model.Block, error) { return m.blocks, nil }
func (m mockProvider) Classify(b model.Block) (model.Category, model.Confidence) {
	return b.Category, b.Conf
}
func (m mockProvider) Introspect(_ string) (model.IdentitySet, error) { return m.ids, nil }
func (m mockProvider) Categories() []model.Category                   { return model.Categories() }

func TestAnalyzeDetectsDuplicateAlias(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 1, Category: model.CatAliases, Conf: model.ConfHigh},
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 9, Category: model.CatAliases, Conf: model.ConfHigh},
		},
		ids: model.IdentitySet{Available: true, Aliases: map[string]bool{"gs": true}},
	}
	a := Analyze(p, []byte("x\ny\n"), "/tmp/rc")
	if a.ExitCode() != 3 {
		t.Fatalf("expected actionable exit code 3, got %d", a.ExitCode())
	}
	if len(a.Issues) != 1 || a.Issues[0].Kind != model.IssueDuplicateAlias || a.Issues[0].Name != "gs" {
		t.Fatalf("expected one duplicate_alias issue for gs, got %+v", a.Issues)
	}
	if want := []int{1, 9}; len(a.Issues[0].Lines) != 2 || a.Issues[0].Lines[0] != want[0] || a.Issues[0].Lines[1] != want[1] {
		t.Errorf("lines = %v, want %v", a.Issues[0].Lines, want)
	}
	if !a.Introspected {
		t.Error("expected Introspected=true")
	}
}

func TestAnalyzeDegradesWhenIntrospectionUnavailable(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{{Kind: model.KindAssignment, Names: []string{"EDITOR"}, Category: model.CatEnvironment, StartLine: 1}},
		ids:    model.IdentitySet{Available: false},
	}
	a := Analyze(p, []byte("export EDITOR=nvim\n"), "/tmp/rc")
	if a.Introspected {
		t.Error("expected Introspected=false")
	}
	if len(a.Notes) == 0 {
		t.Error("expected a note explaining static-only degradation")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/analyze/ -v`
Expected: FAIL — `Analyze` undefined.

- [ ] **Step 3: Write the implementation**

Create `core/analyze/analyze.go`:
```go
// Package analyze is the shell-agnostic reconciler: it drives a Provider and
// merges the static (parse+classify) and dynamic (introspect) views into one
// Analysis.
package analyze

import (
	"regexp"
	"sort"
	"strings"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// Analyze produces a read-only Analysis. src is the raw config; path is used
// for introspection and reporting.
func Analyze(p shell.Provider, src []byte, path string) model.Analysis {
	blocks, _ := p.Parse(src)
	a := model.Analysis{
		Path:       path,
		Lines:      strings.Count(string(src), "\n") + 1,
		BlockCount: len(blocks),
	}

	buckets := map[model.Category][]model.Block{}
	for i := range blocks {
		cat, conf := p.Classify(blocks[i])
		blocks[i].Category = cat
		blocks[i].Conf = conf
		if blocks[i].Opaque {
			a.OpaqueBlocks++
		}
		buckets[cat] = append(buckets[cat], blocks[i])
	}

	for _, cat := range p.Categories() {
		bs := buckets[cat]
		if len(bs) == 0 {
			continue
		}
		var items []string
		for _, b := range bs {
			items = append(items, primaryName(b))
		}
		a.Categories = append(a.Categories, model.CategorySummary{
			Category: cat, Count: len(bs), Items: items,
		})
	}
	a.HasSecrets = len(buckets[model.CatSecrets]) > 0

	// Static issue detection (definitions are visible in the AST).
	a.Issues = append(a.Issues, dupNameIssues(buckets[model.CatAliases], model.IssueDuplicateAlias)...)
	envBlocks := append(append([]model.Block{}, buckets[model.CatEnvironment]...), buckets[model.CatSecrets]...)
	a.Issues = append(a.Issues, dupNameIssues(envBlocks, model.IssueReassignedEnv)...)
	a.Issues = append(a.Issues, dupPathIssues(buckets[model.CatPath])...)

	// Dynamic reconciliation (resolved end-state).
	ids, err := p.Introspect(path)
	if err != nil || !ids.Available {
		a.Introspected = false
		a.Notes = append(a.Notes, "introspection unavailable — showing static analysis only")
	} else {
		a.Introspected = true
		a.Issues = append(a.Issues, shadowIssues(ids)...)
	}

	sort.SliceStable(a.Issues, func(i, j int) bool {
		if a.Issues[i].Kind != a.Issues[j].Kind {
			return a.Issues[i].Kind < a.Issues[j].Kind
		}
		return a.Issues[i].Name < a.Issues[j].Name
	})
	return a
}

func primaryName(b model.Block) string {
	if len(b.Names) > 0 {
		return b.Names[0]
	}
	first := strings.TrimSpace(strings.SplitN(b.Text, "\n", 2)[0])
	if len(first) > 48 {
		return first[:48] + "…"
	}
	return first
}

func dupNameIssues(blocks []model.Block, kind model.IssueKind) []model.Issue {
	lines := map[string][]int{}
	for _, b := range blocks {
		for _, n := range b.Names {
			lines[n] = append(lines[n], b.StartLine)
		}
	}
	var out []model.Issue
	for name, ls := range lines {
		if len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: kind, Name: name, Lines: ls, Note: "last definition wins"})
		}
	}
	return out
}

var pathSegRe = regexp.MustCompile(`(?:\$HOME|~|/)[^:"'\s)]+`)

func dupPathIssues(blocks []model.Block) []model.Issue {
	lines := map[string][]int{}
	for _, b := range blocks {
		for _, seg := range pathSegRe.FindAllString(b.Text, -1) {
			lines[seg] = append(lines[seg], b.StartLine)
		}
	}
	var out []model.Issue
	for seg, ls := range lines {
		if len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: model.IssueDuplicatePath, Name: seg, Lines: ls})
		}
	}
	return out
}

func shadowIssues(ids model.IdentitySet) []model.Issue {
	var out []model.Issue
	for name := range ids.Aliases {
		if ids.Functions[name] {
			out = append(out, model.Issue{
				Kind: model.IssueShadowed, Name: name,
				Note: "defined as both an alias and a function",
			})
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/analyze/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add core/analyze/
git commit -m "feat: reconcile static and dynamic views into Analysis"
```

---

### Task 7: renderers (human + --json)

**Files:**
- Create: `core/render/render.go`
- Test: `core/render/render_test.go`

**Interfaces:**
- Consumes: `model.Analysis`, `model.CategoryDescription`.
- Produces: `render.Human(a model.Analysis) string`; `render.JSON(a model.Analysis) ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

Create `core/render/render_test.go`:
```go
package render

import (
	"encoding/json"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func sampleAnalysis() model.Analysis {
	return model.Analysis{
		Path:       "/home/u/.zshrc",
		Lines:      42,
		BlockCount: 7,
		Categories: []model.CategorySummary{
			{Category: model.CatAliases, Count: 2, Items: []string{"gs", "ll"}},
		},
		Issues:       []model.Issue{{Kind: model.IssueDuplicateAlias, Name: "gs", Lines: []int{1, 9}, Note: "last definition wins"}},
		Introspected: true,
	}
}

func TestHumanIncludesCategoriesAndIssues(t *testing.T) {
	out := Human(sampleAnalysis())
	for _, want := range []string{"aliases", "gs", "duplicate", "1", "9"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q\n---\n%s", want, out)
		}
	}
}

func TestJSONIsOneObjectWithContract(t *testing.T) {
	b, err := JSON(sampleAnalysis())
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("output is not a single JSON object: %v", err)
	}
	if env["tool"] != "zsh-pro" {
		t.Errorf("tool = %v, want zsh-pro", env["tool"])
	}
	if env["ok"] != true {
		t.Errorf("ok = %v, want true", env["ok"])
	}
	if env["exit_code"].(float64) != 3 {
		t.Errorf("exit_code = %v, want 3", env["exit_code"])
	}
	if env["issues_found"] != true {
		t.Errorf("issues_found = %v, want true", env["issues_found"])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/render/ -v`
Expected: FAIL — `Human` / `JSON` undefined.

- [ ] **Step 3: Write the implementation**

Create `core/render/render.go`:
```go
// Package render turns an Analysis into human or JSON output. Both are pure
// functions of the model; neither mutates it.
package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
)

// Human returns the terminal-friendly report.
func Human(a model.Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  zsh-pro — analysis of %s\n", a.Path)
	fmt.Fprintf(&b, "  %d lines, %d blocks", a.Lines, a.BlockCount)
	if a.OpaqueBlocks > 0 {
		fmt.Fprintf(&b, " (%d not structurally understood)", a.OpaqueBlocks)
	}
	b.WriteString("\n\n  CATEGORIES\n")
	for _, c := range a.Categories {
		fmt.Fprintf(&b, "   - %-12s %3d  %s\n", c.Category, c.Count, model.CategoryDescription(c.Category))
	}
	if a.HasSecrets {
		b.WriteString("\n  !  Secrets detected — keep these in a .gitignore'd file; never sync them.\n")
	}
	if !a.Introspected {
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "\n  !  %s\n", n)
		}
	}

	b.WriteString("\n  ISSUES\n")
	if len(a.Issues) == 0 {
		b.WriteString("   (none found — nice and clean)\n")
		return b.String()
	}
	for _, is := range a.Issues {
		line := fmt.Sprintf("   ! %-16s %s", is.Kind, is.Name)
		if len(is.Lines) > 0 {
			line += fmt.Sprintf("  at lines %v", is.Lines)
		}
		if is.Note != "" {
			line += "  (" + is.Note + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// envelope mirrors the prototype's machine-readable contract.
type envelope struct {
	Tool        string         `json:"tool"`
	Version     string         `json:"version"`
	Command     string         `json:"command"`
	OK          bool           `json:"ok"`
	IssuesFound bool           `json:"issues_found"`
	ExitCode    int            `json:"exit_code"`
	Analysis    model.Analysis `json:"analysis"`
}

// JSON returns exactly one JSON object (the agent contract).
func JSON(a model.Analysis) ([]byte, error) {
	env := envelope{
		Tool:        "zsh-pro",
		Version:     buildinfo.Version,
		Command:     "analyze",
		OK:          true,
		IssuesFound: len(a.Issues) > 0,
		ExitCode:    a.ExitCode(),
		Analysis:    a,
	}
	return json.MarshalIndent(env, "", "  ")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/render/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add core/render/
git commit -m "feat: human and JSON renderers"
```

---

### Task 8: CLI wiring + exit codes

**Files:**
- Create: `core/cli/cli.go`
- Test: `core/cli/cli_test.go`
- Modify: `core/cmd/zsh-pro/main.go` (replace the Task 1 stub)

**Interfaces:**
- Consumes: `analyze.Analyze`, `render.Human`, `render.JSON`, `zsh.Provider`, `model.Analysis`.
- Produces: `cli.Run(args []string, stdout, stderr io.Writer) int`.

- [ ] **Step 1: Write the failing test**

Create `core/cli/cli_test.go`:
```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rc.zsh")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunCleanConfigExitsZero(t *testing.T) {
	p := writeRC(t, "export EDITOR=nvim\nalias ll='ls -l'\n")
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", p}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "CATEGORIES") {
		t.Errorf("missing report body:\n%s", out.String())
	}
}

func TestRunDuplicateExitsThree(t *testing.T) {
	p := writeRC(t, "alias gs='git status'\nalias gs='git switch'\n")
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", p}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestRunJSONEmitsOneObject(t *testing.T) {
	p := writeRC(t, "alias gs='git status'\nalias gs='git switch'\n")
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", p, "--json"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		t.Errorf("expected a single JSON object, got:\n%s", s)
	}
}

func TestRunMissingFileExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", "/no/such/rc"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestRunUnknownCommandExitsTwo(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run([]string{"frobnicate"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/cli/ -v`
Expected: FAIL — `Run` undefined.

- [ ] **Step 3: Write the implementation**

Create `core/cli/cli.go`:
```go
// Package cli wires flags and I/O to the engine. Run returns an exit code and
// writes to the provided streams, so it is testable without os.Exit.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"zsh-pro/core/analyze"
	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
	"zsh-pro/core/render"
	"zsh-pro/core/shell/zsh"
)

// Run executes a command. Exit codes: 0 clean, 1 runtime error, 2 usage,
// 3 actionable.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: zsh-pro analyze [path] [--json]")
		return 2
	}
	switch args[0] {
	case "--version", "-v":
		fmt.Fprintf(stdout, "zsh-pro %s\n", buildinfo.Version)
		return 0
	case "analyze":
		return runAnalyze(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
		return 2
	}
}

func runAnalyze(args []string, stdout, stderr io.Writer) int {
	path := "~/.zshrc"
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case len(a) > 0 && a[0] == '-':
			fmt.Fprintf(stderr, "zsh-pro: unknown flag %q\n", a)
			return 2
		default:
			path = a
		}
	}
	path = expandHome(path)

	src, err := os.ReadFile(path)
	if err != nil {
		return fail(stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
	}

	a := analyze.Analyze(zsh.Provider{}, src, path)

	if asJSON {
		b, err := render.JSON(a)
		if err != nil {
			return fail(stderr, true, fmt.Sprintf("render: %v", err))
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintln(stdout, render.Human(a))
	}
	return a.ExitCode()
}

func expandHome(p string) string {
	if p == "~" || (len(p) >= 2 && p[:2] == "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// fail emits a runtime error (exit 1) in either mode.
func fail(stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": "zsh-pro", "version": buildinfo.Version, "command": "analyze",
			"ok": false, "error": msg, "exit_code": 1,
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Fprintln(stderr, string(b))
	} else {
		fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return 1
}

var _ = model.Analysis{} // keep model import explicit for readers
```

Replace `core/cmd/zsh-pro/main.go` entirely with:
```go
package main

import (
	"os"

	"zsh-pro/core/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./core/cli/ -v`
Expected: PASS (all five). The JSON/duplicate tests do not depend on `zsh` being installed; they assert exit code and output shape, which hold whether or not introspection succeeds.

- [ ] **Step 5: Remove the now-unused model import if `go vet` complains**

If `go vet ./...` flags the `var _ = model.Analysis{}` line as unnecessary, delete that line and the `model` import from `cli.go`. Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add core/cli/ core/cmd/zsh-pro/main.go
git commit -m "feat: analyze CLI with --json and exit-code contract"
```

---

### Task 9: Tier-1 test corpus + golden tests

**Files:**
- Create: `core/testdata/fixtures/clean_baseline.zsh`
- Create: `core/testdata/fixtures/installer_junk.zsh`
- Create: `core/testdata/fixtures/duplicate_aliases.zsh`
- Create: `core/testdata/fixtures/reassigned_env.zsh`
- Create: `core/testdata/fixtures/secrets_inline.zsh`
- Create: `core/testdata/fixtures/empty.zsh`
- Create: `core/testdata/fixtures/manifests.json`
- Test: `core/analyze/corpus_test.go`

**Interfaces:**
- Consumes: `analyze.Analyze`, `zsh.Provider`, `model.Analysis`.
- Produces: a golden corpus runner asserting each fixture's analysis against its manifest.

- [ ] **Step 1: Create the fixture files**

Create `core/testdata/fixtures/clean_baseline.zsh`:
```zsh
export EDITOR=nvim
export LANG=en_US.UTF-8
alias ll='ls -l'
alias gs='git status'
setopt AUTO_CD
```

Create `core/testdata/fixtures/installer_junk.zsh`:
```zsh
export EDITOR=nvim

# >>> conda initialize >>>
__conda_setup="$('/opt/conda/bin/conda' shell.zsh hook 2> /dev/null)"
eval "$__conda_setup"
# <<< conda initialize <<<

export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
eval "$(pyenv init -)"
```

Create `core/testdata/fixtures/duplicate_aliases.zsh`:
```zsh
alias gs='git status'
alias ll='ls -l'
alias gs='git switch'
```

Create `core/testdata/fixtures/reassigned_env.zsh`:
```zsh
export EDITOR=vim
export EDITOR=nvim
```

Create `core/testdata/fixtures/secrets_inline.zsh`:
```zsh
export GITHUB_TOKEN=ghp_example000000000000000000000000
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI
alias ll='ls -l'
```

Create `core/testdata/fixtures/empty.zsh`:
```zsh
```

- [ ] **Step 2: Create the ground-truth manifest**

Create `core/testdata/fixtures/manifests.json`:
```json
{
  "clean_baseline.zsh": {
    "min_blocks": 5,
    "issue_kinds": [],
    "has_secrets": false,
    "expect_categories": ["environment", "aliases", "options"]
  },
  "installer_junk.zsh": {
    "min_blocks": 4,
    "issue_kinds": [],
    "has_secrets": false,
    "expect_categories": ["environment", "plugins"]
  },
  "duplicate_aliases.zsh": {
    "min_blocks": 3,
    "issue_kinds": ["duplicate_alias"],
    "has_secrets": false,
    "expect_categories": ["aliases"]
  },
  "reassigned_env.zsh": {
    "min_blocks": 2,
    "issue_kinds": ["reassigned_env"],
    "has_secrets": false,
    "expect_categories": ["environment"]
  },
  "secrets_inline.zsh": {
    "min_blocks": 3,
    "issue_kinds": [],
    "has_secrets": true,
    "expect_categories": ["secrets", "aliases"]
  },
  "empty.zsh": {
    "min_blocks": 0,
    "issue_kinds": [],
    "has_secrets": false,
    "expect_categories": []
  }
}
```

- [ ] **Step 3: Write the failing golden test**

Create `core/analyze/corpus_test.go`:
```go
package analyze

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

type manifest struct {
	MinBlocks        int      `json:"min_blocks"`
	IssueKinds       []string `json:"issue_kinds"`
	HasSecrets       bool     `json:"has_secrets"`
	ExpectCategories []string `json:"expect_categories"`
}

func TestCorpusGolden(t *testing.T) {
	root := filepath.Join("..", "testdata", "fixtures")
	raw, err := os.ReadFile(filepath.Join(root, "manifests.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifests map[string]manifest
	if err := json.Unmarshal(raw, &manifests); err != nil {
		t.Fatal(err)
	}

	for name, m := range manifests {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			a := Analyze(zsh.Provider{}, src, filepath.Join(root, name))

			if a.BlockCount < m.MinBlocks {
				t.Errorf("blocks = %d, want >= %d", a.BlockCount, m.MinBlocks)
			}
			if a.HasSecrets != m.HasSecrets {
				t.Errorf("has_secrets = %v, want %v", a.HasSecrets, m.HasSecrets)
			}
			if a.OpaqueBlocks > 0 {
				t.Errorf("unexpected opaque blocks: %d (parser failed to understand real-world input)", a.OpaqueBlocks)
			}

			gotKinds := map[string]bool{}
			for _, is := range a.Issues {
				gotKinds[string(is.Kind)] = true
			}
			for _, want := range m.IssueKinds {
				if !gotKinds[want] {
					t.Errorf("missing expected issue kind %q (got issues %+v)", want, a.Issues)
				}
			}
			// No issue kinds beyond those declared in the manifest.
			declared := map[string]bool{}
			for _, k := range m.IssueKinds {
				declared[k] = true
			}
			for k := range gotKinds {
				if !declared[k] {
					t.Errorf("unexpected issue kind %q", k)
				}
			}

			gotCats := map[string]bool{}
			for _, c := range a.Categories {
				gotCats[string(c.Category)] = true
			}
			for _, want := range m.ExpectCategories {
				if !gotCats[want] {
					t.Errorf("missing expected category %q (got %v)", want, gotCats)
				}
			}
			_ = model.CatMisc // keep model import explicit
		})
	}
}
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run: `go test ./core/analyze/ -run TestCorpusGolden -v`
Expected: initially may FAIL on a fixture whose real classification differs from the manifest. For each failure, decide whether the **engine** is wrong (fix the relevant `core/shell/zsh` rule and re-run its unit test from Task 4/5) or the **manifest** is wrong (correct `manifests.json`). Iterate until PASS. Do not weaken assertions to force a pass — adjust the side that is actually incorrect.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./... -v`
Expected: all packages PASS (introspection-dependent tests skip if `zsh` is absent).

- [ ] **Step 6: Commit**

```bash
git add core/testdata/ core/analyze/corpus_test.go
git commit -m "test: Tier-1 fixture corpus with golden assertions"
```

---

## Self-Review

**Spec coverage:**
- Read-only analyze, categorize, report dup/shadow/conflict → Tasks 2–9. ✓
- Identity-based detection (not line-based) → `dupNameIssues`/`shadowIssues` key on names (Task 6). ✓
- Human + `--json` rendering → Task 7. ✓
- Go single module, monorepo `core/` + `ui/` → Tasks 1+ (`ui/` intentionally untouched). ✓
- Shell-agnostic `Provider` seam, zsh impl → Task 3 (interface) + Tasks 3–5 (zsh). ✓
- Static (mvdan/sh) + dynamic (zsh introspection), reconciled → Tasks 3, 5, 6. ✓
- Classifier B (rules + confidence + unsure bucket) → Task 4 (`ConfLow` + `CatMisc`). ✓
- Opaque-block fallback → Task 3. ✓
- Introspection degradation (never crash) → Task 5 (`Available:false`) + Task 6 (Notes). ✓
- Exit-code + `--json` contract → Tasks 7–8. ✓
- Two-tier corpus, Tier-1 golden tests → Task 9. (**Tier-2 real-world configs are a deliberate fast-follow, not in this plan** — noted in the spec.)
- Coverage taxonomy: order-sensitivity (PATH precedence, fpath/compinit) and structural edge cases (heredocs, multiline funcs) fixtures are **not yet** in the Task 9 corpus — they should be added as the corpus grows; the harness in Task 9 accepts new fixtures by adding a file + a manifest entry.

**Placeholder scan:** No TBD/TODO; every code step contains complete code; the only "verify against the library" steps (zsh `LangVariant` constant, `Word.Lit()`) are real external-API confirmations, not logic placeholders.

**Type consistency:** `model.*` types/constants are defined in Task 2 and used verbatim thereafter; `shell.Provider`'s four methods (`Parse`/`Classify`/`Introspect`/`Categories`) match the zsh implementations and the `mockProvider`; `Analysis.ExitCode()` is used consistently in render and cli.
