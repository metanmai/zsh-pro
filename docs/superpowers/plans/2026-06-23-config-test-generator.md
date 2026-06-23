# Graph-Based Config Test Generator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A test-only subsystem that generates random zsh configs from a typed dependency graph, paired with a built-in oracle (the known-correct analysis), plus a mutation fuzz mode — so the `analyze` engine can be tested for correctness and robustness over thousands of seeds.

**Architecture:** A `core/testgen` library models a config as a `ConfigGraph` of typed `Node`s. `Build` (seeded) plants known defects; `RenderZsh` emits `.zsh` bytes and records each node's line; `Expected` derives the correct `model.Analysis`. An external-package property test closes the loop against the real engine. A `Mutator` corrupts rendered bytes for the fuzz mode. A `zsh-gen` CLI emits cases + a corpus-compatible manifest.

**Tech Stack:** Go (stdlib only — `math/rand`, `strings`, `sort`, `fmt`); `core/model` (leaf); the existing `analyze` + `shell/zsh` engine in tests. No new third-party dependency.

## Global Constraints

- **Dependency direction:** `core/testgen` imports **only `core/model`** (a leaf). It must NOT import `analyze`, `render`, `cli`, `shell`, or `shell/zsh`. Only the external test package (`testgen_test`) and `cmd/zsh-gen` wire the concrete engine/provider.
- **Zero new third-party deps.** Generation uses stdlib `math/rand`. No property-testing library.
- **No global mutable state.** The PRNG is injected via `New(rng *rand.Rand)` / `NewMutator(rng)` — never package-global. The seed is logged on any test failure for exact replay.
- **Behavior on role types.** `Generator`, `ConfigGraph`, `Node`, `Mutator` carry their logic as methods; no free-standing functions doing work. Immutable name pools / templates are unexported package-level `var`s.
- **One type per file; errors returned, never panicked.**
- **Oracle strict subset** (must always match): per-category counts, the issue set keyed by `(kind, name)`, `has_secrets`, and `opaque_blocks == 0`. Issue `lines` and the total `Lines` field are **gated** (a `const checkLines = false`) because they expose two known analyzer bugs (comment line mis-attribution; `Lines` off-by-one) that are out of scope to fix here.
- **Valid generated configs must parse with 0 opaque blocks.** A non-zero `OpaqueBlocks` on a clean generated config is a generator bug. Name pools are disjoint per node type and classification-safe (env names contain neither `PATH` nor any secret-regex token; path dirs are rooted so `pathSegRe` extracts them whole).
- **CLI manifest** matches the existing corpus schema exactly: `{min_blocks:int, issue_kinds:[]string, has_secrets:bool, expect_categories:[]string}`.
- **Verify after every task:** `GOTOOLCHAIN=auto go build ./... && GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./...` (green) and `gofmt -l core` (empty).

---

## File Structure

```
core/testgen/
  graph.go        NodeKind, Node, ConfigGraph + intrinsic methods (Add, DependOn)
  generator.go    Generator{rng}, GenParams, New, (*Generator).Build, name pools, pick
  render.go       (*ConfigGraph).RenderZsh() []byte ; (*Node).render() string
  oracle.go       (*ConfigGraph).Expected() model.Analysis + private grouping methods
  mutate.go       Mutator{rng}, NewMutator, (*Mutator).Corrupt([]byte) []byte
  graph_test.go        Task 1 (internal pkg)
  generator_test.go    Task 2 (internal pkg)
  render_test.go       Task 3 (external pkg testgen_test — wires zsh.Provider)
  oracle_test.go       Task 4 (internal pkg)
  property_test.go     Task 5 (external pkg testgen_test — wires analyze + zsh)
  fuzz_test.go         Task 6 (external pkg testgen_test)
core/cmd/zsh-gen/
  main.go         CLI composition root
  main_test.go    Task 7
```

---

### Task 1: Graph model

**Files:**
- Create: `core/testgen/graph.go`
- Test: `core/testgen/graph_test.go`

**Interfaces:**
- Produces: `NodeKind` (`NodeEnvVar, NodeAlias, NodeFunction, NodePathEntry, NodeCommand, NodeSecret`); `Node{Kind NodeKind; Name, Value, Comment string; Cat model.Category; DependsOn []int; Line int}`; `ConfigGraph{Nodes []*Node}`; `(*ConfigGraph).Add(*Node) int`; `(*ConfigGraph).DependOn(child, parent int)`.

- [ ] **Step 1: Write the failing test**

```go
package testgen

import (
	"testing"

	"zsh-pro/core/model"
)

func TestGraphAddAndDepend(t *testing.T) {
	var g ConfigGraph
	a := g.Add(&Node{Kind: NodeEnvVar, Name: "EDITOR", Value: "vim", Cat: model.CatEnvironment})
	b := g.Add(&Node{Kind: NodeAlias, Name: "ll", Value: "ls -la", Cat: model.CatAliases})
	if a != 0 || b != 1 {
		t.Fatalf("Add returned indices %d,%d; want 0,1", a, b)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("len(Nodes)=%d; want 2", len(g.Nodes))
	}
	g.DependOn(b, a) // alias depends on the env var (must follow it)
	if got := g.Nodes[b].DependsOn; len(got) != 1 || got[0] != a {
		t.Fatalf("DependsOn=%v; want [%d]", got, a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestGraphAddAndDepend`
Expected: FAIL — `undefined: ConfigGraph` (package does not compile yet).

- [ ] **Step 3: Write minimal implementation**

```go
// Package testgen builds random zsh configs from a typed dependency graph,
// together with the known-correct analysis they should produce. It is test
// infrastructure: it imports only core/model (a leaf) and never the engine.
package testgen

import "zsh-pro/core/model"

// NodeKind is the shell-entity type of a graph node.
type NodeKind int

const (
	NodeEnvVar NodeKind = iota
	NodeAlias
	NodeFunction
	NodePathEntry
	NodeCommand
	NodeSecret
)

// Node is one shell entity. Identity for duplicate/shadow detection is Name
// (for PathEntry, Name is the rooted directory; for Command, the leading word).
type Node struct {
	Kind      NodeKind
	Name      string
	Value     string         // RHS / body / args, Kind-dependent
	Comment   string         // optional leading comment (no leading '#')
	Cat       model.Category // intended classification — the oracle's source of truth
	DependsOn []int          // indices of nodes that must be emitted before this one
	Line      int            // 1-based statement line; filled by RenderZsh
}

// ConfigGraph is a generated config. Insertion order is a valid topological
// order: generators only ever record DependsOn edges to lower-index nodes, so
// no separate topological sort is needed when rendering.
type ConfigGraph struct {
	Nodes []*Node
}

// Add appends a node and returns its index.
func (g *ConfigGraph) Add(n *Node) int {
	g.Nodes = append(g.Nodes, n)
	return len(g.Nodes) - 1
}

// DependOn records that the child node must be emitted after the parent.
// Callers pass parent < child to preserve the topological-order invariant.
func (g *ConfigGraph) DependOn(child, parent int) {
	g.Nodes[child].DependsOn = append(g.Nodes[child].DependsOn, parent)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestGraphAddAndDepend`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/testgen/graph.go core/testgen/graph_test.go
git commit -m "feat(testgen): typed config graph model"
```

---

### Task 2: Seeded generator

**Files:**
- Create: `core/testgen/generator.go`
- Test: `core/testgen/generator_test.go`

**Interfaces:**
- Consumes: `ConfigGraph`, `Node`, `NodeKind` (Task 1).
- Produces: `Generator{rng}`; `New(rng *rand.Rand) *Generator`; `GenParams{EnvVars, Aliases, Functions, PathEntries, Commands, DupAliases, ReassignedEnv, DupPaths, Shadows, Secrets int}`; `(*Generator).Build(p GenParams) *ConfigGraph`.

**Design notes (read before coding):**
- Defects use **freshly drawn names distinct from the clean base**, so no accidental compound defects (a dup alias that is also a shadow). Each `DupAliases` plants two alias nodes sharing one fresh name; each `ReassignedEnv` two env nodes; each `DupPaths` two path nodes sharing a dir; each `Shadows` one alias node + one function node sharing a fresh name (drawn from the alias pool, which is disjoint from the function pool, so it cannot collide with a clean function).
- `Cat` is stamped at generation from the node kind (`NodeCommand` carries its own intended category in `Value`'s template — for v1, Commands are limited to `bindkey`→keybindings, `setopt`→options, `source`→plugins). Env/secret/path/alias/func map 1:1 to their categories.
- `pick` shuffles a copy of a pool and returns the first n — deterministic for a given rng.

- [ ] **Step 1: Write the failing test**

```go
package testgen

import (
	"math/rand"
	"reflect"
	"testing"
)

func sampleParams() GenParams {
	return GenParams{
		EnvVars: 3, Aliases: 3, Functions: 2, PathEntries: 2, Commands: 2,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	g1 := New(rand.New(rand.NewSource(42))).Build(sampleParams())
	g2 := New(rand.New(rand.NewSource(42))).Build(sampleParams())
	if !reflect.DeepEqual(g1, g2) {
		t.Fatal("same seed produced different graphs")
	}
}

func TestBuildPlantsDefects(t *testing.T) {
	g := New(rand.New(rand.NewSource(7))).Build(sampleParams())

	aliasCount := map[string]int{}
	envCount := map[string]int{}
	pathCount := map[string]int{}
	isAlias := map[string]bool{}
	isFunc := map[string]bool{}
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeAlias:
			aliasCount[n.Name]++
			isAlias[n.Name] = true
		case NodeEnvVar, NodeSecret:
			envCount[n.Name]++
		case NodePathEntry:
			pathCount[n.Name]++
		case NodeFunction:
			isFunc[n.Name] = true
		}
	}
	dups := func(m map[string]int) int { c := 0; for _, v := range m { if v > 1 { c++ } }; return c }
	if dups(aliasCount) != 1 {
		t.Errorf("duplicate-alias pairs = %d; want 1", dups(aliasCount))
	}
	if dups(envCount) != 1 {
		t.Errorf("reassigned-env pairs = %d; want 1", dups(envCount))
	}
	if dups(pathCount) != 1 {
		t.Errorf("duplicate-path pairs = %d; want 1", dups(pathCount))
	}
	shadows := 0
	for name := range isAlias {
		if isFunc[name] {
			shadows++
		}
	}
	if shadows != 1 {
		t.Errorf("shadows = %d; want 1", shadows)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestBuild`
Expected: FAIL — `undefined: GenParams` / `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

```go
package testgen

import (
	"fmt"
	"math/rand"

	"zsh-pro/core/model"
)

// Name pools — disjoint per kind so classification is deterministic and the
// oracle never mis-predicts a category. Env names contain neither "PATH" nor a
// secret-regex token; path dirs are rooted so the analyzer's pathSegRe extracts
// each whole. Pools must be large enough for base + planted-defect draws.
var (
	envNames    = []string{"EDITOR", "PAGER", "LANG", "LESS", "TERM", "COLORTERM", "TMPDIR", "LSCOLORS", "MANWIDTH", "GREP_OPTIONS"}
	aliasNames  = []string{"ll", "la", "gs", "gd", "gco", "kc", "tf", "dco", "vi", "rl"}
	funcNames   = []string{"mkcd", "extract", "backup", "up", "ports", "serve", "gclone", "tre"}
	pathDirs    = []string{"/usr/local/bin", "/opt/bin", "/opt/homebrew/bin", "$HOME/.local/bin", "$HOME/go/bin", "/snap/bin", "$HOME/.cargo/bin"}
	secretNames = []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "API_KEY", "NPM_TOKEN", "CLIENT_SECRET", "AUTH_TOKEN"}
)

// commandTmpl is one renderable command with its known category.
type commandTmpl struct {
	word string
	args string
	cat  model.Category
}

var commandTmpls = []commandTmpl{
	{"bindkey", `"^X^E" edit-command-line`, model.CatKeybindings},
	{"setopt", "EXTENDED_GLOB", model.CatOptions},
	{"source", "/etc/zsh/zshrc.local", model.CatPlugins},
}

// Generator builds random config graphs from an injected PRNG.
type Generator struct{ rng *rand.Rand }

// New returns a Generator driven by rng.
func New(rng *rand.Rand) *Generator { return &Generator{rng: rng} }

// GenParams says how many of each node type to create and how many defects to
// plant. Defect counts must fit within the relevant pool (caller's responsibility).
type GenParams struct {
	EnvVars, Aliases, Functions, PathEntries, Commands int
	DupAliases, ReassignedEnv, DupPaths, Shadows, Secrets int
}

// pick shuffles a copy of pool and returns its first n names (deterministic).
func (gen *Generator) pick(pool []string, n int) []string {
	cp := append([]string(nil), pool...)
	gen.rng.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp[:n]
}

// Build constructs a graph: clean base nodes from disjoint pools, then planted
// defects on fresh, non-overlapping names.
func (gen *Generator) Build(p GenParams) *ConfigGraph {
	g := &ConfigGraph{}

	// Partition each pool into clean-base names and defect names up front so the
	// two never overlap.
	aliasN := gen.pick(aliasNames, p.Aliases+p.DupAliases+p.Shadows)
	baseAlias, dupAlias, shadowName := aliasN[:p.Aliases], aliasN[p.Aliases:p.Aliases+p.DupAliases], aliasN[p.Aliases+p.DupAliases:]
	envN := gen.pick(envNames, p.EnvVars+p.ReassignedEnv)
	baseEnv, dupEnv := envN[:p.EnvVars], envN[p.EnvVars:]
	pathN := gen.pick(pathDirs, p.PathEntries+p.DupPaths)
	basePath, dupPath := pathN[:p.PathEntries], pathN[p.PathEntries:]
	funcN := gen.pick(funcNames, p.Functions)
	secretN := gen.pick(secretNames, p.Secrets)

	for _, name := range baseEnv {
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "1", Cat: model.CatEnvironment})
	}
	for _, name := range baseAlias {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo " + name, Cat: model.CatAliases})
	}
	for _, name := range funcN {
		g.Add(&Node{Kind: NodeFunction, Name: name, Value: "echo " + name, Cat: model.CatFunctions})
	}
	for _, dir := range basePath {
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
	}
	for i := 0; i < p.Commands; i++ {
		t := commandTmpls[i%len(commandTmpls)]
		g.Add(&Node{Kind: NodeCommand, Name: t.word, Value: t.args, Cat: t.cat})
	}
	for _, name := range secretN {
		g.Add(&Node{Kind: NodeSecret, Name: name, Value: "xxxxxxxx", Cat: model.CatSecrets})
	}

	// Planted defects (fresh names, distinct from base).
	for _, name := range dupAlias {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo first", Cat: model.CatAliases})
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo second", Cat: model.CatAliases})
	}
	for _, name := range dupEnv {
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "first", Cat: model.CatEnvironment})
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "second", Cat: model.CatEnvironment})
	}
	for _, dir := range dupPath {
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
	}
	for _, name := range shadowName {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo alias", Cat: model.CatAliases, Comment: fmt.Sprintf("%s alias", name)})
		g.Add(&Node{Kind: NodeFunction, Name: name, Value: "echo func", Cat: model.CatFunctions})
	}
	return g
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestBuild`
Expected: PASS (both subtests)

- [ ] **Step 5: Commit**

```bash
git add core/testgen/generator.go core/testgen/generator_test.go
git commit -m "feat(testgen): seeded generator that plants known defects"
```

---

### Task 3: Renderer

**Files:**
- Create: `core/testgen/render.go`
- Test: `core/testgen/render_test.go` (external package `testgen_test` — wires the real `zsh.Provider` to prove output parses cleanly)

**Interfaces:**
- Consumes: `ConfigGraph`, `Node` (Task 1).
- Produces: `(*ConfigGraph).RenderZsh() []byte` (sets each `Node.Line`); `(*Node).render() string` (statement text, no trailing newline, may contain internal newlines).

- [ ] **Step 1: Write the failing test**

```go
package testgen_test

import (
	"math/rand"
	"strings"
	"testing"

	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

func TestRenderParsesCleanly(t *testing.T) {
	g := testgen.New(rand.New(rand.NewSource(99))).Build(testgen.GenParams{
		EnvVars: 3, Aliases: 3, Functions: 2, PathEntries: 2, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	})
	src := g.RenderZsh()

	// Every node got a line, and that line in the output starts the statement.
	lines := strings.Split(string(src), "\n")
	for _, n := range g.Nodes {
		if n.Line < 1 || n.Line > len(lines) {
			t.Fatalf("node %q has bad Line %d", n.Name, n.Line)
		}
	}

	// The real parser must understand every block — no opaque fallback.
	blocks, _ := zsh.Provider{}.Parse(src)
	opaque := 0
	for _, b := range blocks {
		if b.Opaque {
			opaque++
		}
	}
	if opaque != 0 {
		t.Errorf("rendered config produced %d opaque blocks; generator must emit valid zsh\n%s", opaque, src)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestRenderParsesCleanly`
Expected: FAIL — `g.RenderZsh undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package testgen

import (
	"fmt"
	"strings"
)

// RenderZsh emits the graph as .zsh source in node (topological) order,
// emitting each node's optional leading comment, then its statement, then a
// blank separator line. It sets each Node.Line to the 1-based line of that
// node's statement. Call RenderZsh before Expected (Expected reads Node.Line).
func (g *ConfigGraph) RenderZsh() []byte {
	var b strings.Builder
	line := 1
	for _, n := range g.Nodes {
		if n.Comment != "" {
			fmt.Fprintf(&b, "# %s\n", n.Comment)
			line++
		}
		n.Line = line
		stmt := n.render()
		b.WriteString(stmt)
		b.WriteByte('\n')
		line += strings.Count(stmt, "\n") + 1
		b.WriteByte('\n') // blank separator
		line++
	}
	return []byte(b.String())
}

// render returns the zsh statement text for a node (no trailing newline).
func (n *Node) render() string {
	switch n.Kind {
	case NodeEnvVar, NodeSecret:
		return fmt.Sprintf("export %s=%q", n.Name, n.Value)
	case NodeAlias:
		return fmt.Sprintf("alias %s='%s'", n.Name, n.Value)
	case NodeFunction:
		return fmt.Sprintf("%s() {\n  %s\n}", n.Name, n.Value)
	case NodePathEntry:
		return fmt.Sprintf("export PATH=%q", n.Name+":$PATH")
	case NodeCommand:
		return strings.TrimSpace(n.Name + " " + n.Value)
	default:
		return "# unknown node"
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestRenderParsesCleanly`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/testgen/render.go core/testgen/render_test.go
git commit -m "feat(testgen): render graph to valid zsh with line tracking"
```

---

### Task 4: Oracle

**Files:**
- Create: `core/testgen/oracle.go`
- Test: `core/testgen/oracle_test.go` (internal package)

**Interfaces:**
- Consumes: `ConfigGraph`, `Node` (Task 1); `RenderZsh` must have run so `Node.Line` is set.
- Produces: `(*ConfigGraph).Expected() model.Analysis` — fills `Categories` (counts only, no Items), `Issues` (kind+name+correct lines, sorted by kind then name), `HasSecrets`. Mirrors engine grouping: `reassigned_env` spans env+secret nodes; `duplicate_path` keys on the dir.

- [ ] **Step 1: Write the failing test**

```go
package testgen

import (
	"math/rand"
	"testing"

	"zsh-pro/core/model"
)

func TestExpectedMatchesPlantedDefects(t *testing.T) {
	g := New(rand.New(rand.NewSource(7))).Build(sampleParams())
	g.RenderZsh() // sets Node.Line
	exp := g.Expected()

	kinds := map[model.IssueKind]int{}
	for _, is := range exp.Issues {
		kinds[is.Kind]++
	}
	for _, want := range []model.IssueKind{
		model.IssueDuplicateAlias, model.IssueReassignedEnv,
		model.IssueDuplicatePath, model.IssueShadowed,
	} {
		if kinds[want] != 1 {
			t.Errorf("expected exactly 1 %s issue, got %d", want, kinds[want])
		}
	}
	if !exp.HasSecrets {
		t.Error("HasSecrets=false; sampleParams plants 1 secret")
	}
	// Categories carry counts.
	total := 0
	for _, c := range exp.Categories {
		total += c.Count
	}
	if total != len(g.Nodes) {
		t.Errorf("category counts sum to %d; want %d (one per node)", total, len(g.Nodes))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestExpectedMatchesPlantedDefects`
Expected: FAIL — `g.Expected undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package testgen

import (
	"sort"

	"zsh-pro/core/model"
)

// Expected derives the known-correct analysis from the graph. It requires
// RenderZsh to have run (it reads Node.Line). The line numbers it records are
// the CORRECT statement lines; the engine may differ on a first occurrence that
// follows a comment (a known bug), so the property test compares issues by
// (kind, name) and gates the line comparison.
func (g *ConfigGraph) Expected() model.Analysis {
	var a model.Analysis
	counts := map[model.Category]int{}
	for _, n := range g.Nodes {
		counts[n.Cat]++
		if n.Kind == NodeSecret {
			a.HasSecrets = true
		}
	}
	for _, cat := range model.Categories() {
		if c := counts[cat]; c > 0 {
			a.Categories = append(a.Categories, model.CategorySummary{Category: cat, Count: c})
		}
	}

	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodeAlias}, model.IssueDuplicateAlias)...)
	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodeEnvVar, NodeSecret}, model.IssueReassignedEnv)...)
	a.Issues = append(a.Issues, g.dupNameIssues([]NodeKind{NodePathEntry}, model.IssueDuplicatePath)...)
	a.Issues = append(a.Issues, g.shadowIssues()...)

	sort.SliceStable(a.Issues, func(i, j int) bool {
		if a.Issues[i].Kind != a.Issues[j].Kind {
			return a.Issues[i].Kind < a.Issues[j].Kind
		}
		return a.Issues[i].Name < a.Issues[j].Name
	})
	return a
}

// dupNameIssues reports names appearing on more than one node of the given kinds.
func (g *ConfigGraph) dupNameIssues(kinds []NodeKind, kind model.IssueKind) []model.Issue {
	want := map[NodeKind]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	lines := map[string][]int{}
	order := []string{}
	for _, n := range g.Nodes {
		if !want[n.Kind] {
			continue
		}
		if _, seen := lines[n.Name]; !seen {
			order = append(order, n.Name)
		}
		lines[n.Name] = append(lines[n.Name], n.Line)
	}
	var out []model.Issue
	for _, name := range order {
		if ls := lines[name]; len(ls) > 1 {
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: kind, Name: name, Lines: ls})
		}
	}
	return out
}

// shadowIssues reports a name defined as BOTH an alias and a function.
func (g *ConfigGraph) shadowIssues() []model.Issue {
	aliasLine := map[string]int{}
	funcLine := map[string]int{}
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeAlias:
			if _, ok := aliasLine[n.Name]; !ok {
				aliasLine[n.Name] = n.Line
			}
		case NodeFunction:
			if _, ok := funcLine[n.Name]; !ok {
				funcLine[n.Name] = n.Line
			}
		}
	}
	var out []model.Issue
	for name, al := range aliasLine {
		if fl, ok := funcLine[name]; ok {
			ls := []int{al, fl}
			sort.Ints(ls)
			out = append(out, model.Issue{Kind: model.IssueShadowed, Name: name, Lines: ls})
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestExpectedMatchesPlantedDefects`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/testgen/oracle.go core/testgen/oracle_test.go
git commit -m "feat(testgen): oracle deriving the known-correct analysis"
```

---

### Task 5: Oracle property test (closes the loop against the real engine)

**Files:**
- Create: `core/testgen/property_test.go` (external package `testgen_test`)

**Interfaces:**
- Consumes: `testgen.New/Build/RenderZsh/Expected`; `analyze.New(zsh.Provider{}).Analyze`; `model`.
- Produces: `TestOracleProperty` + an `assertStrict` helper. The strict subset is per-category counts, the `(kind, name)` issue set, `has_secrets`, and `opaque_blocks == 0`. A `const checkLines = false` gates the issue-line / total-`Lines` comparison (exposes the two known analyzer bugs; flip to `true` once they're fixed).

- [ ] **Step 1: Write the failing test**

```go
package testgen_test

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"zsh-pro/core/analyze"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

const checkLines = false // gated: exposes known line mis-attribution + Lines off-by-one

func propParams() testgen.GenParams {
	return testgen.GenParams{
		EnvVars: 4, Aliases: 4, Functions: 3, PathEntries: 3, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

func TestOracleProperty(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 5, 8, 13, 21, 34, 55, 89} {
		seed := seed
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			g := testgen.New(rand.New(rand.NewSource(seed))).Build(propParams())
			src := g.RenderZsh()
			want := g.Expected()
			got := analyze.New(zsh.Provider{}).Analyze(src, fmt.Sprintf("gen-%d.zsh", seed))
			assertStrict(t, seed, want, got, src)
		})
	}
}

func assertStrict(t *testing.T, seed int64, want, got model.Analysis, src []byte) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Errorf(format+"\n--- seed %d, source ---\n%s", append(args, seed, src)...)
	}

	if got.OpaqueBlocks != 0 {
		fail("opaque_blocks = %d; want 0", got.OpaqueBlocks)
	}
	if got.HasSecrets != want.HasSecrets {
		fail("has_secrets = %v; want %v", got.HasSecrets, want.HasSecrets)
	}

	wantCat := map[model.Category]int{}
	for _, c := range want.Categories {
		wantCat[c.Category] = c.Count
	}
	gotCat := map[model.Category]int{}
	for _, c := range got.Categories {
		gotCat[c.Category] = c.Count
	}
	for cat, n := range wantCat {
		if gotCat[cat] != n {
			fail("category %q count = %d; want %d", cat, gotCat[cat], n)
		}
	}
	for cat, n := range gotCat {
		if _, ok := wantCat[cat]; !ok {
			fail("unexpected category %q (count %d)", cat, n)
		}
	}

	key := func(is model.Issue) string { return string(is.Kind) + "/" + is.Name }
	wantKeys, gotKeys := []string{}, []string{}
	for _, is := range want.Issues {
		wantKeys = append(wantKeys, key(is))
	}
	for _, is := range got.Issues {
		gotKeys = append(gotKeys, key(is))
	}
	sort.Strings(wantKeys)
	sort.Strings(gotKeys)
	if fmt.Sprint(wantKeys) != fmt.Sprint(gotKeys) {
		fail("issue set = %v; want %v", gotKeys, wantKeys)
	}

	if checkLines {
		// Enable once comment line mis-attribution + Lines off-by-one are fixed.
	}
}
```

- [ ] **Step 2: Run to verify it passes (no production code changes)**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestOracleProperty -v`
Expected: PASS for all 10 seeds. If any seed fails the strict subset, that is a real analyzer bug — capture the logged source and STOP (report it); do not weaken the assertion to make it pass.

- [ ] **Step 3: Commit**

```bash
git add core/testgen/property_test.go
git commit -m "test(testgen): oracle property test across seeds"
```

---

### Task 6: Fuzz mode

**Files:**
- Create: `core/testgen/mutate.go`
- Create: `core/testgen/fuzz_test.go` (external package `testgen_test`)

**Interfaces:**
- Consumes: `ConfigGraph.RenderZsh`; `analyze` + `render.JSONRenderer` + `zsh.Provider`.
- Produces: `Mutator{rng}`; `NewMutator(rng *rand.Rand) *Mutator`; `(*Mutator).Corrupt(src []byte) []byte` (applies one random corruption op). Survival property: analyze never panics; the JSON renderer always emits valid JSON.

- [ ] **Step 1: Write the failing test**

```go
package testgen_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"zsh-pro/core/analyze"
	"zsh-pro/core/render"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

// fuzzIters is a deliberate, logged bound — each iteration spawns `zsh -f`, so
// we keep it modest rather than silently capping a huge run.
const fuzzIters = 40

func TestFuzzSurvival(t *testing.T) {
	t.Logf("fuzzing %d corrupted configs (bounded: each spawns zsh -f)", fuzzIters)
	for i := 0; i < fuzzIters; i++ {
		seed := int64(1000 + i)
		rng := rand.New(rand.NewSource(seed))
		g := testgen.New(rng).Build(propParams())
		corrupt := testgen.NewMutator(rng).Corrupt(g.RenderZsh())

		out, ok := analyzeNoPanic(corrupt, fmt.Sprintf("fuzz-%d.zsh", seed))
		if !ok {
			t.Fatalf("analyze panicked on seed %d\n%s", seed, corrupt)
		}
		if !json.Valid(out) {
			t.Fatalf("JSON renderer emitted invalid JSON on seed %d\n%s\n--- json ---\n%s", seed, corrupt, out)
		}
	}
}

func analyzeNoPanic(src []byte, path string) (out []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	a := analyze.New(zsh.Provider{}).Analyze(src, path)
	b, err := render.JSONRenderer{}.Render(a)
	if err != nil {
		return nil, false
	}
	return b, true
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestFuzzSurvival`
Expected: FAIL — `testgen.NewMutator undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package testgen

import "math/rand"

// Mutator corrupts rendered config bytes to test engine robustness.
type Mutator struct{ rng *rand.Rand }

// NewMutator returns a Mutator driven by rng.
func NewMutator(rng *rand.Rand) *Mutator { return &Mutator{rng: rng} }

// Corrupt applies one random corruption operator to a copy of src.
func (m *Mutator) Corrupt(src []byte) []byte {
	out := append([]byte(nil), src...)
	if len(out) == 0 {
		return out
	}
	switch m.rng.Intn(5) {
	case 0: // truncate at a random point
		return out[:m.rng.Intn(len(out))]
	case 1: // inject a random raw byte
		i := m.rng.Intn(len(out))
		out[i] = byte(m.rng.Intn(256))
		return out
	case 2: // insert an unbalanced quote
		i := m.rng.Intn(len(out))
		return append(out[:i], append([]byte{'"'}, out[i:]...)...)
	case 3: // inject an invalid UTF-8 byte
		i := m.rng.Intn(len(out))
		return append(out[:i], append([]byte{0xff}, out[i:]...)...)
	default: // drop a closing brace if present, else duplicate a chunk
		for i, c := range out {
			if c == '}' {
				return append(out[:i], out[i+1:]...)
			}
		}
		return append(out, out[:m.rng.Intn(len(out))]...)
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/testgen/ -run TestFuzzSurvival`
Expected: PASS. If analyze panics or emits invalid JSON, that is a real robustness bug — STOP and report with the logged seed/source.

- [ ] **Step 5: Commit**

```bash
git add core/testgen/mutate.go core/testgen/fuzz_test.go
git commit -m "feat(testgen): mutation fuzz mode + survival property test"
```

---

### Task 7: `zsh-gen` CLI

**Files:**
- Create: `core/cmd/zsh-gen/main.go`
- Create: `core/cmd/zsh-gen/main_test.go`

**Interfaces:**
- Consumes: `testgen.New/Build/RenderZsh/Expected`; `model`.
- Produces: a binary with flags `-n int` (cases, default 5), `-seed int64` (default 1), `-out string` (target dir, required). For each case it writes `gen_<i>.zsh` and accumulates a manifest entry; it writes `manifests.json` in the exact corpus schema (`min_blocks` = node count; `issue_kinds` / `has_secrets` / `expect_categories` from `Expected`). Generation logic stays in `testgen`; `main` is flags + I/O only.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateWritesCorpus(t *testing.T) {
	dir := t.TempDir()
	if err := generate(3, 1, dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifests.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifests map[string]manifestEntry
	if err := json.Unmarshal(raw, &manifests); err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 3 {
		t.Fatalf("manifest has %d entries; want 3", len(manifests))
	}
	for name, m := range manifests {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("manifest names %q but file missing: %v", name, err)
		}
		if m.MinBlocks <= 0 {
			t.Errorf("%s: min_blocks = %d; want > 0", name, m.MinBlocks)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=auto go test ./core/cmd/zsh-gen/`
Expected: FAIL — `undefined: generate` / `undefined: manifestEntry`.

- [ ] **Step 3: Write minimal implementation**

```go
// Command zsh-gen emits random generated zsh configs plus a corpus-compatible
// manifests.json, for inspection or promotion into the golden corpus.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"zsh-pro/core/testgen"
)

// manifestEntry matches the golden-corpus schema (see core/analyze/corpus_test.go).
type manifestEntry struct {
	MinBlocks        int      `json:"min_blocks"`
	IssueKinds       []string `json:"issue_kinds"`
	HasSecrets       bool     `json:"has_secrets"`
	ExpectCategories []string `json:"expect_categories"`
}

func genParams() testgen.GenParams {
	return testgen.GenParams{
		EnvVars: 4, Aliases: 4, Functions: 3, PathEntries: 3, Commands: 3,
		DupAliases: 1, ReassignedEnv: 1, DupPaths: 1, Shadows: 1, Secrets: 1,
	}
}

func generate(n int, seed int64, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	manifests := map[string]manifestEntry{}
	for i := 0; i < n; i++ {
		g := testgen.New(rand.New(rand.NewSource(seed + int64(i)))).Build(genParams())
		src := g.RenderZsh()
		exp := g.Expected()

		name := fmt.Sprintf("gen_%d.zsh", i)
		if err := os.WriteFile(filepath.Join(outDir, name), src, 0o644); err != nil {
			return err
		}

		kinds := []string{}
		seen := map[string]bool{}
		for _, is := range exp.Issues {
			if k := string(is.Kind); !seen[k] {
				seen[k] = true
				kinds = append(kinds, k)
			}
		}
		cats := []string{}
		for _, c := range exp.Categories {
			cats = append(cats, string(c.Category))
		}
		manifests[name] = manifestEntry{
			MinBlocks:        len(g.Nodes),
			IssueKinds:       kinds,
			HasSecrets:       exp.HasSecrets,
			ExpectCategories: cats,
		}
	}
	raw, err := json.MarshalIndent(manifests, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "manifests.json"), raw, 0o644)
}

func main() {
	n := flag.Int("n", 5, "number of cases to generate")
	seed := flag.Int64("seed", 1, "base PRNG seed")
	out := flag.String("out", "", "output directory (required)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: zsh-gen -out DIR [-n N] [-seed S]")
		os.Exit(2)
	}
	if err := generate(*n, *seed, *out); err != nil {
		fmt.Fprintf(os.Stderr, "zsh-gen: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote %d cases + manifests.json to %s\n", *n, *out)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=auto go test ./core/cmd/zsh-gen/`
Expected: PASS

- [ ] **Step 5: Verify the emitted corpus actually validates end-to-end**

```bash
GOTOOLCHAIN=auto go run ./core/cmd/zsh-gen -out /tmp/gen-corpus -n 3
GOTOOLCHAIN=auto go build -o /tmp/zp ./core/cmd/zsh-pro
for f in /tmp/gen-corpus/gen_*.zsh; do /tmp/zp analyze "$f" --json >/dev/null && echo "ok $f (exit $?)"; done
```
Expected: each generated config analyzes without error (exit 0 or 3).

- [ ] **Step 6: Commit**

```bash
git add core/cmd/zsh-gen/main.go core/cmd/zsh-gen/main_test.go
git commit -m "feat(zsh-gen): CLI emitting generated cases + corpus manifest"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** graph model (T1) · seeded generator + defects (T2) · render + line tracking (T3) · oracle (T4) · oracle property test vs real engine (T5) · fuzz mode (T6) · CLI + corpus manifest (T7). All spec sections map to a task.
- **Type consistency:** `Node`/`ConfigGraph`/`GenParams`/`Generator`/`Mutator` signatures are identical everywhere they appear; `model` field names (`BlockCount`, `OpaqueBlocks`, `CategorySummary{Category,Count,Items}`, `Issue{Kind,Name,Lines,Note}`) verified against source.
- **Leaf rule:** `core/testgen` (graph/generator/render/oracle/mutate) imports only `core/model`. The engine is wired only in external `testgen_test` files and `cmd/zsh-gen`.
- **Known-bug honesty:** `checkLines = false` gates the line/`Lines` assertions; the strict subset still fully exercises detection. A strict-subset failure is to be reported as an analyzer bug, never silenced.
```
