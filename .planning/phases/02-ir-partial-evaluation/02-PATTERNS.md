# Phase 2: IR + Partial Evaluation - Pattern Map

**Mapped:** 2026-06-26
**Files analyzed:** 7 (5 new, 2 modified) + 4 new test files
**Analogs found:** 11 / 11 (all in-repo; zero external precedent needed)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `core/model/profile.go` (NEW) | model | transform | `core/model/block.go` + `core/model/analysis.go` | exact |
| `core/model/block.go` (MODIFY: add `Value`, `Dynamic`) | model | transform | `core/model/block.go` (self — additive) | exact |
| `core/shell/zsh/parse.go` (MODIFY: capture value/body + dynamic flag in `describe()`) | provider/parser | transform | `core/shell/zsh/parse.go` (self — extend `describe()`) | exact |
| `core/shell/provider.go` (MODIFY: add `Regenerator` interface, see Open Q1) | seam | request-response | `core/shell/provider.go` (self — new narrow interface) | exact |
| `core/shell/zsh/regen.go` (NEW, if templater lives in zsh per Pitfall 5/A1) | provider/emit | transform | `core/testgen/render.go` | role+flow match |
| `core/ir/build.go` (NEW) | service/orchestration | transform | `core/analyze/analyzer.go` | exact |
| `core/ir/route.go` (NEW) | service (pure verdict) | transform | `core/shell/zsh/classify.go` + `core/analyze/reconciler.go` | role match |
| `core/ir/regen.go` (NEW, if templater lives in `core/ir` per Pitfall 5 reading b) | service/emit | transform | `core/testgen/render.go` | role+flow match |
| `core/ir/build_test.go` (NEW) | test (unit, pure Go) | — | (table-test conventions in repo) | role match |
| `core/ir/regen_test.go` (NEW) | test (unit, pure Go) | — | (table-test conventions) | role match |
| `core/ir/roundtrip_test.go` or `core/ir_test` (NEW) | test (integration, zsh) | request-response | `core/analyze/corpus_test.go` + `core/shell/zsh/introspect_test.go` | exact |
| `core/shell/zsh/dynamic_test.go` or extend `parse_test.go` (NEW) | test (unit, zsh-free) | — | `core/shell/zsh/parse_test.go` | role match |

**Note on the design fork (RESEARCH §"Central Design Fork"):** the planner must pick where value capture and templating live. PATTERNS records analogs for *both* defensible placements (zsh `regen.go` vs `core/ir/regen.go`); pick one, drop the other. RESEARCH recommends Approach A (capture at parse time) + templater behind a `shell.Regenerator` interface implemented in `core/shell/zsh`.

---

## Pattern Assignments

### `core/model/profile.go` (model, transform) — NEW

**Analog:** `core/model/block.go:1-37` (typed-enum + value-struct convention) and `core/model/analysis.go:1-21` (composite read-only result type).

**Package + zero-internal-import rule** — `core/model` imports nothing internal. Verified `core/model/category.go:1-2` (only `package model`) and `core/model/block.go` (no import block). Profile/Entry MUST stay stdlib-only (RESEARCH Anti-Pattern: "Adding internal imports to `core/model`").

**Typed-enum pattern to mirror for `ManagedOverride`** (from `core/model/block.go:6-15`):
```go
// BlockKind is the agnostic structural shape of a parsed block...
type BlockKind string

const (
	KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar
	KindAlias      BlockKind = "alias"      // alias gs=...
	KindFuncDecl   BlockKind = "func"       // foo() { ... }
	...
)
```
Apply the same shape per Open Question 3 (`type ManagedOverride string` with `Override*` constants — `OverrideAuto`, `OverrideManaged`, `OverrideUnmanaged`). Prefix derived from the type name, matching the `Cat`/`Kind`/`Conf`/`Exit`/`Issue` convention in CLAUDE.md.

**Value-struct + doc-comment pattern for `Entry`** (mirror `core/model/block.go:26-37`):
```go
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
```
`Entry` should hold the verbatim `Text` (D-01) plus the Phase-2 derived fields. Per RESEARCH Open Question 3, use **distinct fields**: `Managed bool` (auto verdict, D-06), `Override ManagedOverride` (D-07, default `OverrideAuto`), `Dynamic bool` (D-05 orthogonal axis). Keep `Category`, `Kind`, `Names`, `Value`, `Exported` so the templater can rebuild declarative entries from structure (D-10).

**Composite read-only result pattern for `Profile`** (mirror `core/model/analysis.go:10-21` — a value type holding an ordered slice, with derived views as methods, e.g. `ExitCode()` at `analysis.go:39-44`):
```go
// Analysis is the complete read-only result the renderers consume.
type Analysis struct {
	Path         string
	...
	Categories   []CategorySummary
	...
}
```
Per D-02, `Profile` stores ONE ordered `[]Entry` in source order; "grouped by category" is a computed view/method over that list (analog: `Analysis.HasActionableIssues()` is a derived method, not stored state — `analysis.go:27-34`). Do NOT use per-category buckets as storage.

---

### `core/model/block.go` (model, transform) — MODIFY (add `Value`, `Dynamic`)

**Analog:** self — additive field extension.

**What to add** (RESEARCH Approach A, the recommended fork resolution):
- `Value string` — verbatim assignment value / alias body / option-args text captured from the AST.
- `Dynamic bool` — set true when the value's `*syntax.Word` contains a non-literal part.

Both are agnostic scalar types, so `core/model` stays dependency-free. **Critical (Pitfall 2):** additive only — must NOT change `Kind`/`Names`/`Category` assignment. The testgen oracle pin (`core/testgen/property_test.go:32-42`) and corpus golden (`core/analyze/corpus_test.go:32-96`) assert category counts, issue (kind,name) set, `has_secrets`, zero opaque blocks, and exact line numbers — adding read-only fields must leave all green. Run `make check` after the change.

---

### `core/shell/zsh/parse.go` (provider/parser, transform) — MODIFY `describe()`

**Analog:** self — extend the existing `describe()` AST-handling method (`parse.go:60-139`).

**Where to hook** — the assignment branches already iterate `c.Assigns` to pull names. Capture the value text + dynamic flag in the same loop. Existing pure-assignment branch (`parse.go:64-72`):
```go
if len(c.Args) == 0 {
	b.Kind = model.KindAssignment
	for _, a := range c.Assigns {
		if a.Name != nil {
			b.Names = append(b.Names, a.Name.Value)
		}
	}
	return
}
```
The `export/typeset/...` `CallExpr` branch (`parse.go:87-105`) and the `*syntax.DeclClause` branch (`parse.go:109-121`) also iterate `c.Assigns` / `c.Args` — add value capture in each.

**Verbatim-value capture pattern** — follow the existing `wordLitPrefix` precedent (`parse.go:144-154`) which already reasons about `Word.Lit()` being empty for quoted parts:
```go
// wordLitPrefix returns the leading literal text of a word. For a word like
// `gs='git status'` (a *Lit followed by a quoted part) Word.Lit() is empty,
// but the name we care about lives in the first *Lit part ("gs=").
func (Provider) wordLitPrefix(w *syntax.Word) string {
	if lit := w.Lit(); lit != "" {
		return lit
	}
	if len(w.Parts) > 0 {
		if l, ok := w.Parts[0].(*syntax.Lit); ok {
			return l.Value
		}
	}
	return ""
}
```
For the *value* (not just the name prefix), slice it verbatim from `src` using the word's positions (`a.Value.Pos().Offset()` .. `a.Value.End().Offset()`) — the same offset-slicing already done for `Block.Text` at `parse.go:43-49` — so quoting (`'git status'`) and dynamic literals (`$HOME/go`) are preserved byte-for-byte (Pitfall 3 / D-05). Note `Parse` already imports `mvdan.cc/sh/v3/syntax` (`parse.go:7`).

**Dynamic-detection helper** (new private func in this package, EVAL-01 / RESEARCH Pattern 3) — `syntax.Walk` exists at `syntax/walk.go:16`:
```go
func wordIsDynamic(w *syntax.Word) bool {
	dyn := false
	syntax.Walk(w, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst:
			dyn = true
			return false
		}
		return true
	})
	return dyn
}
```
`KindCompound` blocks (if/for/case — `parse.go:133-135`) are conditionals: treat as dynamic by construction and route imperative (not one of the 5 declarative classes). No execution, no `util.ExpandHome` (EVAL-01 SC4).

---

### `core/shell/provider.go` (seam) — MODIFY (add `Regenerator`, per Open Q1 reading a)

**Analog:** self — `core/shell/provider.go:9-31` is a textbook ISP-segregated seam.

**Existing narrow-interface convention to mirror:**
```go
// Classifier owns the category space and assigns a block to it.
type Classifier interface {
	Classify(b model.Block) (model.Category, model.Confidence)
	Categories() []model.Category
}
```
If the planner picks the strict-invariant reading (RESEARCH Pitfall 5 / Open Q1 recommendation a — templater lives in `core/shell/zsh`), add a narrow interface here:
```go
// Regenerator emits behavior-equivalent zsh source for a declarative entry.
type Regenerator interface {
	Regenerate(e model.Entry) string
}
```
and compose it into `Provider` alongside `Parser`/`Classifier`/`Introspector` (`provider.go:27-31`). `core/ir` then depends on this interface, never on `core/shell/zsh` (composition-root rule). If the planner picks reading b (forward templating in `core/ir`), this interface is unnecessary — record the choice.

---

### `core/shell/zsh/regen.go` OR `core/ir/regen.go` (emit, transform) — NEW

**Analog:** `core/testgen/render.go:36-51` — the project's string-templated zsh codegen precedent (CONTEXT canonical-ref explicitly steers here, NOT the mvdan/sh printer).

**Templating pattern to copy** (`render.go:36-51`):
```go
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
**Critical divergence from the analog (Pitfall 3 / D-05):** `render.go` uses `%q` and hard-coded single quotes because it generates *static literal* values. The Phase-2 templater must emit the **captured value text verbatim** (e.g. `export %s=%s` with the byte-for-byte `Value`), NOT re-quote — because re-quoting `$HOME/go` into `'$HOME/go'` would freeze the dynamic value (single quotes suppress expansion). Preserve the original quoting captured from the AST value (Approach A), or use `syntax.Quote(s, syntax.LangZsh)` (`syntax/quote.go:48`) only for proven-static values. The whole-file regeneration loop (in `core/ir/build.go` or here) mirrors `RenderZsh`'s source-order, per-entry iteration (`render.go:12-33`) — but emits in **source order** (D-09), not topological/category order, and prints `Entry.Text` verbatim for imperative/Opaque/unmanaged entries (D-01).

---

### `core/ir/build.go` (service/orchestration, transform) — NEW

**Analog:** `core/analyze/analyzer.go:14-58` — the Provider-composition orchestrator. This is the closest structural match in the repo: agnostic package, takes the shell seam via interface, iterates `[]model.Block` classifying each, never imports `core/shell/zsh`.

**Package doc + dependency-direction pattern** (`analyzer.go:1-11`):
```go
// Package analyze drives a shell.Provider and reconciles the static and dynamic
// views into one model.Analysis. It depends only on core/model and core/shell.
package analyze

import (
	"bytes"
	"sort"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)
```
Mirror exactly: `core/ir` depends ONLY on `core/model` and `core/shell` (interface). `Package ir` doc states the same dependency boundary.

**Classify-each-block loop to mirror** (`analyzer.go:34-43`):
```go
buckets := map[model.Category][]model.Block{}
for i := range blocks {
	cat, conf := az.provider.Classify(blocks[i])
	blocks[i].Category = cat
	blocks[i].Conf = conf
	if blocks[i].Opaque {
		a.OpaqueBlocks++
	}
	buckets[cat] = append(buckets[cat], blocks[i])
}
```
For `core/ir.Build`, replace the bucket accumulation with source-order `Entry` construction (D-02 — do NOT bucket by category for storage; the analyzer buckets because its *output* is per-category rollups, which is exactly what D-02/D-09 say NOT to do for the IR). Target shape (RESEARCH Pattern 1):
```go
func Build(blocks []model.Block, c shell.Classifier) model.Profile {
	var p model.Profile
	for i := range blocks {
		cat, _ := c.Classify(blocks[i])     // reuse existing classifier seam
		e := model.Entry{
			Text:     blocks[i].Text,         // D-01 verbatim
			Category: cat,
			Kind:     blocks[i].Kind,
			Names:    blocks[i].Names,
			Value:    blocks[i].Value,        // captured at parse time (Approach A)
			Exported: blocks[i].Exported,
			Managed:  routeManaged(blocks[i], cat), // ING-02 (route.go)
			Dynamic:  blocks[i].Dynamic,       // EVAL-01 (D-05), parse-time flag
			Override: model.OverrideAuto,      // D-07
		}
		p.Entries = append(p.Entries, e)       // D-02 single ordered list, source order
	}
	return p
}
```
Take the `shell.Classifier` (narrowest interface needed), constructor-injected — mirror `analyze.New(p shell.Provider)` (`analyzer.go:19-20`). The concrete `zsh.Provider{}` is wired only at the composition root / external tests.

---

### `core/ir/route.go` (service, pure verdict) — NEW

**Analog:** `core/shell/zsh/classify.go:20-79` (the `Kind`/`CmdName` switch-driven verdict) + `core/analyze/reconciler.go` (pure stateless detector methods).

**Switch-on-Kind/CmdName pattern to mirror** (`classify.go:26-78`):
```go
switch b.Kind {
case model.KindAlias:
	return model.CatAliases, model.ConfHigh
case model.KindFuncDecl:
	return model.CatFunctions, model.ConfHigh
case model.KindAssignment:
	...
case model.KindCommand:
	switch b.CmdName {
	case "setopt", "unsetopt", "zstyle", "autoload", "compinit", "compdef", "zmodload":
		return model.CatOptions, model.ConfHigh
	...
	}
}
```
**Routing verdict (ING-02 / D-04 / D-06)** — `routeManaged(b, cat)` returns managed=true iff the entry is in one of the 5 admitted reversible classes. Key off `Kind`/`Category`/`CmdName` membership, NOT confidence (RESEARCH Anti-Pattern: "Using confidence as a routing gate"). The 5 declarative classes map to: `KindAssignment` → `CatEnvironment`/`CatPath`/`CatSecrets`; `KindAlias` → `CatAliases`; `KindFuncDecl` → `CatFunctions`; and within `CatOptions`, **only `CmdName ∈ {setopt, unsetopt}`** is declarative (D-04 hard rule). The grab-bag at `classify.go:46` lumps `zstyle, autoload, compinit, compdef, zmodload` into `CatOptions` — these route **imperative** (verbatim master block, D-04 lean / Pitfall 4). Default-imperative ONLY for `Opaque` blocks and genuinely-unknown categories (`CatMisc`, `CatKeybindings`, `CatPlugins`, `CatLocal`) — D-06.

**Pure-stateless-method convention** — if `routeManaged` is grouped under a receiver, follow `reconciler{}`'s zero-value-struct pattern (CLAUDE.md: "pure methods on a zero-value `reconciler{}`"); otherwise a plain package function is fine since CLAUDE.md permits both for stateless logic.

---

### `core/ir/build_test.go` + `core/ir/regen_test.go` (test, unit, pure Go) — NEW

**Analog:** table-driven unit tests; closest in-repo is `core/shell/zsh/classify_test.go` / `parse_test.go` (co-located `<subject>_test.go`, per CLAUDE.md naming). These run WITHOUT zsh (pure-Go IR build/route/regen logic).

**Coverage (RESEARCH Wave-0 gaps + Test Map):**
- `build_test.go`: ING-02 routing (env/alias/func/option/PATH → managed; Opaque/unknown → imperative) + `ManagedOverride` wins-and-persists (D-07).
- `regen_test.go`: ING-01 verbatim emit for imperative/Opaque entries + templated emit for declaratives; assert dynamic value survives verbatim (`$HOME/go` stays `$HOME/go`) — D-05.
- Static grep guard (RESEARCH Test Map EVAL-01): `! grep -rn ExpandHome core/ir core/model`.

---

### `core/ir/roundtrip_test.go` (or external `core/ir_test`) (test, integration, zsh) — NEW

**Analog (two combined):** `core/analyze/corpus_test.go:1-18` (external-test-package wiring of the concrete `zsh.Provider{}`) + `core/shell/zsh/introspect_test.go:10-13` (the `exec.LookPath("zsh")` skip-guard) + `core/shell/zsh/introspect.go:42-53` (the `exec.CommandContext` + 5s-timeout subprocess shape).

**External-package wiring rationale to copy** (`corpus_test.go:1-18`):
```go
package analyze_test

// ...lives in the EXTERNAL test package on purpose: it is the only place that
// wires the concrete zsh.Provider to the agnostic analyze.Analyzer, so the
// production analyze package stays shell-agnostic.

import (
	...
	"zsh-pro/core/analyze"
	"zsh-pro/core/shell/zsh"
)
```
The round-trip oracle wires `zsh.Provider{}` + `ir.Build`/`Regenerate` the same way (external `core/ir_test` keeps `core/ir` shell-agnostic).

**Skip-guard to copy verbatim** (`introspect_test.go:11-13`):
```go
if _, err := exec.LookPath("zsh"); err != nil {
	t.Skip("zsh not installed; skipping ...")
}
```

**Subprocess + snapshot instrument to reuse** (`introspect.go:42-53`) — the oracle sources both original and regenerated `.zsh` under `zsh -f` and snapshots the state tables:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, buildinfo.Name, path)
var out bytes.Buffer
cmd.Stdout = &out
if err := cmd.Run(); err != nil {
	return model.IdentitySet{Available: false}, err
}
```
**Snapshot-access decision (RESEARCH Open Question 2):** `introspectScript` is an unexported const (`introspect.go:23`). The oracle (external pkg) cannot read it. Two options the planner must pick: (a) add an exported `Provider.SnapshotState(srcPath) (string, error)` (or compare the already-returned `model.IdentitySet` from `Introspect` — `introspect.go:42`, fields at `identityset.go:4-11`: `Aliases`/`Functions`/`Env`/`Path`/`Options`); or (b) inline an equivalent throwaway script in the test (what the Phase-1 spike did). RESEARCH recommends (a) — comparing `IdentitySet` equality is structured and already available — D-08's "byte-identical state tables" maps cleanly onto `IdentitySet` map/slice equality.

**Pitfall 1 (LOCAL_OPTIONS auto-revert):** keep the regenerated file *sourced at top level* (`source "$1"` — `introspect.go:26`), NOT wrapped in `emulate -L`/a function, so `setopt` changes stick and are observable.

**Pitfall 4 (compinit non-determinism):** Phase-2 oracle fixtures should avoid `compinit`; imperative lines emit verbatim in both runs so their side effects cancel in the original-vs-regen diff.

---

### `core/shell/zsh/dynamic_test.go` (or extend `parse_test.go`) (test, unit, zsh-free) — NEW

**Analog:** `core/shell/zsh/parse_test.go` (co-located unit test for `describe()` output — same package, no zsh subprocess).

**Coverage (EVAL-01):** assert `wordIsDynamic` / the parse-time `Block.Dynamic` flag: `$HOME`, `${...}`, `$(...)`, backtick `` `...` ``, `$((...))`, and `KindCompound` conditionals → dynamic; bare literals and single-quoted strings (`'literal $HOME'`) → static. This pins the EVAL-01 detector independently of the zsh subprocess.

---

## Shared Patterns

### Dependency-direction / composition-root rule
**Source:** CLAUDE.md "Dependency Injection at the Composition Root" + `core/analyze/analyzer.go:1-20` + `core/shell/zsh/introspect.go:15` (`var _ shell.Provider = Provider{}`).
**Apply to:** `core/ir/build.go`, `core/ir/route.go`, `core/ir/regen.go` (if in ir).
`core/ir` imports ONLY `core/model` + `core/shell` (interface). The concrete `zsh.Provider{}` is wired at the composition root (`core/cmd/zsh-pro/main.go`) and in external test packages (`core/ir_test`). Mirror the `analyze` package boundary verbatim (`analyzer.go:3-11`).

### Typed-enum + doc-comment convention
**Source:** `core/model/block.go:6-15` (`BlockKind` + `Kind*`), `core/model/category.go:5-18` (`Category` + `Cat*`), CLAUDE.md "Typed-enum constants".
**Apply to:** `model.ManagedOverride` (`type ManagedOverride string`, `Override*` constants). Every exported type/const cluster gets a doc comment (CLAUDE.md "Comments").

### String-templated zsh codegen (NOT the mvdan/sh printer)
**Source:** `core/testgen/render.go:36-51`; CONTEXT canonical-refs line 67.
**Apply to:** the declarative templater (`regen.go`). Use `fmt.Sprintf`; emit captured `Value` verbatim for dynamic preservation (D-05).

### Sandboxed-subprocess + skip-guard + graceful-degrade
**Source:** `core/shell/zsh/introspect.go:42-53` + `core/shell/zsh/introspect_test.go:11-13`.
**Apply to:** the round-trip oracle test. `exec.CommandContext` with 5s timeout, `exec.LookPath("zsh")` skip, no panic on failure.

### Regression-pin protection (the non-negotiable guard)
**Source:** `core/testgen/property_test.go:32-42` (oracle pin) + `core/analyze/corpus_test.go:32-96` (corpus golden).
**Apply to:** every change in `core/shell/zsh/parse.go` and `core/model/block.go`. Both pins assert category counts / issue (kind,name) set / `has_secrets` / zero opaque / exact lines. Run `make check` after the parser/model edit. TDD: write IR/oracle tests first, then make the additive parser change, then confirm both pins stay green (Pitfall 2).

---

## No Analog Found

None. Every Phase-2 file has a strong in-repo analog — the parse→classify front-end, the snapshot instrument, the codegen precedent, the Provider-composition orchestrator, and both test patterns (pure-Go unit + zsh-requiring external) all ship today. The only *new* logic with no direct precedent is the static/dynamic AST walk (`wordIsDynamic`), but its building block (`syntax.Walk` over the already-imported `mvdan.cc/sh/v3/syntax`) and the offset-slicing pattern (`parse.go:43-49`) both exist; RESEARCH Pattern 3 supplies the exact node-type switch.

## Metadata

**Analog search scope:** `core/model/`, `core/shell/`, `core/shell/zsh/`, `core/analyze/`, `core/testgen/` (all cited at file:line).
**Files scanned:** 11 source/test files read in full (all ≤ 154 lines; single-pass each).
**Pattern extraction date:** 2026-06-26
