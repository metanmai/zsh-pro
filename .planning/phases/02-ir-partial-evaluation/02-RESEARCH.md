# Phase 2: IR + Partial Evaluation - Research

**Researched:** 2026-06-26
**Domain:** Go AST-driven shell-config IR construction; static partial evaluation (no execution); behavior-equivalence round-trip testing under `zsh -f`
**Confidence:** HIGH (all seams verified against `core/` at file:line; mvdan/sh v3.13.1 API read from the vendored module source)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01: Entry keeps the verbatim source text.** Each `Entry` holds the original `Block.Text` byte-for-byte **plus** the Phase-2 derived fields (declarative/imperative verdict, static/dynamic tag, category, override). Regenerating an untouched/imperative entry is "print the stored text." (Rejected: a fresh transform that discards the Block.)
- **D-02: `Profile` stores a single ordered list in source order; category is a field, not the storage.** "Grouped by category" is a computed view/iteration over that list. (Rejected: per-category buckets as storage.)
- **D-03: PATH-like values are kept verbatim in Phase 2; segmentation is deferred to Phase 4.** A `PATH`/`FPATH` assignment is stored as one verbatim value (tagged dynamic when it contains `$HOME`/`${...}`/`$(...)`). EVAL-01 static/dynamic tagging still applies here.
- **D-04: Within `CatOptions`, the declarative set is Claude's discretion under a hard rule:** if a command's reversal was not proven byte-identical in the Phase 1 spike, it defaults to imperative (precision over recall). Documented lean: only `setopt`/`unsetopt` declarative; `zstyle`, `autoload`, `compinit`, `compdef`, `zmodload` → master block.
- **D-05: Declarative/imperative and static/dynamic are ORTHOGONAL axes.** A declarative entry whose value is dynamic (`export GOPATH=$HOME/go`) **stays declarative/switchable** and is **separately** tagged dynamic. A dynamic value is **never** grounds for banishing an entry to the imperative master block.
- **D-06: Maximize the switchable surface; route conservatively only when truly unrecognizable.** Default routing keys off `Block.Kind`/`Category`/`CmdName` membership in the 5 admitted reversible classes (env / PATH / aliases / functions / options). **Only `Opaque` blocks and genuinely-unknown categories default to imperative.** Confidence is **not** a blunt gate. The byte-identical reverse remains the correctness backstop.
- **D-07: The IR carries a per-entry override field now; the CLI/UX to set it is deferred.** `Entry` carries `ManagedOverride: auto | forced-managed | forced-unmanaged`. The auto-verdict is the default; a manual override wins and persists through round-trip into Ph3/Ph4.
- **D-08: The round-trip oracle executes and diffs state tables under sandboxed `zsh -f`, reusing the Phase 1 instrument.** Source original + regenerated `.zsh` each under `zsh -f`, snapshot state tables via the existing `introspectScript`, assert byte-identical. (Rejected: byte-identical source text; re-parse idempotency alone.)
- **D-09: Phase 2 regeneration emits in original source order — not regrouped by category.** Per-category emit is deferred to Phase 4.
- **D-10: Template declarative entries (rebuild from structured fields); emit everything else verbatim.** Declarative entries are emitted from structured fields so the oracle proves regeneration (not an echo tautology). Imperative/unmanaged/Opaque stay verbatim. Dynamic values are kept late-bound verbatim within the templated output.

### Claude's Discretion
- The exact `Profile`/`Entry` field names and Go types; package placement (within `core/model` + a new IR-build seam consistent with existing layering); the templater's internal structure — provided D-01, D-02, and the `core/model`-stays-dependency-free constraint hold.
- The precise declarative set inside `CatOptions` (D-04), under the stated hard rule.
- The static/dynamic detector's exact implementation, provided it triggers on `$HOME`/`${...}`/`$(...)`/backticks/conditionals and performs **no execution** and **no `util.ExpandHome`**.
- Whether imperative entries are represented as a distinct `Entry` kind/flag vs a category — provided round-trip and override semantics hold.

### Deferred Ideas (OUT OF SCOPE)
- CLI/UX to set the managed/unmanaged override (the IR field ships now; the command to set it is Ph5–6).
- PATH add/delete segmentation into `additions`/`deletions` vs a captured base — Phase 4.
- Per-category emit (category-grouped `.zsh` output) — Phase 4; Phase 2 emits in source order (D-09).
- Secret exclusion from the committed profile (PROF-03) — Phase 6; the IR may *tag* `CatSecrets` entries but does not implement exclusion.
- keybindings / hooks / completion (`compinit`) as future managed classes — stay in the master block for v2.0.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ING-01 | Ingest `~/.zshrc` into a categorized, regenerable representation (env/aliases/functions/PATH/options) that round-trips to behavior-equivalent zsh; untouched statements verbatim, rewritten declarative slices templated. | IR = ordered `[]Entry` wrapping `Block.Text` (D-01/D-02). Reuse `Provider.Parse`+`Classify` as the front-end (`core/shell/zsh/parse.go`, `classify.go`). Templater follows `core/testgen/render.go` pattern. Round-trip oracle reuses `introspectScript`. |
| ING-02 | Classify each statement declarative (switchable) vs imperative (unmanaged master block). Misclassification = zero-residue violation. | Routing gate keyed off `Block.Kind`/`Category`/`CmdName` membership in the 5 admitted classes (Phase 1 FINDINGS); only `Opaque` + unknown default imperative (D-06). `ManagedOverride` field is the safety valve (D-07). |
| EVAL-01 | Resolve syntactically-static values; keep dynamic ones (`$HOME`/`${...}`/`$(...)`/conditionals) late-bound. "Static" = syntactically constant, never resolved against the machine. No `util.ExpandHome` in the IR. | Detect via mvdan/sh `syntax.Word.Lit()` (non-empty ⇒ fully static) or walking `Word.Parts` for `*ParamExp`/`*CmdSubst`/`*DblQuoted`/`*ArithmExp` and `CmdSubst.Backquotes` (dynamic). No execution. |
</phase_requirements>

## Summary

Phase 2 builds the `model.Profile`/`model.Entry` IR spine from the parser's existing `[]model.Block`. The good news: the parse → classify front-end already exists and is reused as-is through the `shell.Provider` seam (`core/shell/provider.go:9-31`); the categorizer already buckets the five reversible classes correctly (`core/shell/zsh/classify.go:20-79`); and the round-trip oracle's measurement instrument — `introspectScript` + the `zsh -f -c` + 5s-timeout + skip-if-absent pattern — is sitting ready in `core/shell/zsh/introspect.go:23-53` and `introspect_test.go:11-13`. The string-templated codegen precedent for D-10 lives in `core/testgen/render.go:36-51`.

The **one genuine design fork** the planner must resolve: `model.Block` carries `Text`, `Names`, `Kind`, `CmdName`, `Exported` — but **NOT the assigned value or the alias/function body** (verified: `core/model/block.go:27-37` has no `Value`/`Body`/`Raw` field). Both the static/dynamic detector (EVAL-01) and the declarative templater (D-10) need the value. There are three viable approaches, ranked below; the recommended one extracts value/body at parse time into a new agnostic `Block` field via a small `core/shell/zsh/parse.go` enhancement, keeping `core/model` dependency-free and the detector AST-accurate.

**Primary recommendation:** Add `model.Profile`/`model.Entry` to `core/model` (zero deps). Build the IR in a **new `core/ir` package** that depends only on `core/model` + `core/shell` (interface) — never `core/shell/zsh`. Enhance `Provider.Parse` to capture the assignment value / alias body / option args as agnostic `Block` fields, then run the static/dynamic detector on that captured text using a tiny re-parse of *just the value word* (or carry a structured `[]Block`-level "dynamic" bool set at parse time). Route declarative/imperative off `Kind`+`Category`+`CmdName`. Template declarative entries from structured fields, emit everything else verbatim, in source order. Gate correctness with a new `zsh`-requiring round-trip oracle test that reuses `introspectScript`.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| `Profile`/`Entry` types | `core/model` (leaf, zero internal deps) | — | CLAUDE.md: model types stay dependency-free; Ph3/Ph4 serialize them. |
| Value/body extraction from AST | `core/shell/zsh` (parser) | — | Only the zsh provider touches the mvdan/sh AST; agnostic result lands on `Block`. |
| IR build (`[]Block` → `Profile`) | new `core/ir` (orchestration) | depends on `core/shell` interface + `core/model` | Mirrors how `core/analyze` composes the Provider seam without importing `core/shell/zsh`. |
| Declarative/imperative routing | `core/ir` | reuses `Block.Category` from `Classify` | Pure verdict over agnostic `Block` fields; shell-free. |
| Static/dynamic detection | `core/shell/zsh` (AST) OR `core/ir` (over captured text) | — | If done over AST → zsh package; if over a captured-value string → can be agnostic in `core/ir`. |
| Declarative templater | `core/shell/zsh/emit*` OR `core/ir` | — | If it emits zsh syntax, the milestone invariant says zsh-syntax codegen belongs to the zsh package (`emit.go`, per ROADMAP §invariants line 15). See Pitfall 5. |
| Round-trip oracle | external test pkg (`core/ir_test` or `core/analyze_test`) | wires `zsh.Provider{}` + `introspectScript` | Same pattern as `corpus_test.go` (external pkg wires the concrete provider). |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `mvdan.cc/sh/v3/syntax` | v3.13.1 | The zsh AST parser already in use; provides `Word`, `WordPart`, `ParamExp`, `CmdSubst` for static/dynamic detection | [VERIFIED: go.mod] — the project's **only** non-stdlib dependency; no new deps allowed |
| Go stdlib `strings`/`fmt` | go 1.25.0 | String-templated codegen for the declarative templater | [VERIFIED: go.mod] — matches `core/testgen/render.go` precedent |
| Go stdlib `testing` + `os/exec` | go 1.25.0 | Round-trip oracle subprocess + skip-guard | [VERIFIED] — exact shape in `introspect.go:42-53` / `introspect_test.go:11-13` |

**No new dependencies.** CLAUDE.md constraint: "single external dependency (`mvdan.cc/sh/v3`) — no new dependencies without explicit discussion." Phase 2 adds none.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| String-templated emit (`fmt.Fprintf`, per `render.go`) | mvdan/sh `syntax.Printer` to print AST nodes | CONTEXT canonical-refs explicitly steers to the `render.go` string-template precedent "rather than the mvdan/sh printer." The Printer would re-format and could lose the byte-late-bound dynamic value; string-templating from structured fields keeps `$HOME/go` verbatim. **Use the string template.** |
| Re-inspecting the AST in `core/ir` | Re-parse `Block.Text` value | `core/ir` cannot import `core/shell/zsh` (composition-root rule). Re-parsing in `core/ir` would need the parser via the interface, which only returns `[]Block` (no AST). This is why value/dynamic extraction belongs at parse time in the zsh package. |

**Installation:** None — no packages added.

**Version verification:**
```
$ go list -m mvdan.cc/sh/v3   → mvdan.cc/sh/v3 v3.13.1   [VERIFIED]
$ cat go.mod                  → go 1.25.0; require mvdan.cc/sh/v3 v3.13.1   [VERIFIED]
```

## Package Legitimacy Audit

> Not applicable — **this phase installs zero external packages.** All work uses the already-pinned `mvdan.cc/sh/v3 v3.13.1` (in `go.sum`) and the Go standard library. No registry lookup or slopcheck needed.

| Package | Registry | Disposition |
|---------|----------|-------------|
| (none added) | — | N/A — phase adds no dependency |

## Architecture Patterns

### System Architecture Diagram

```
  ~/.zshrc bytes
        │
        ▼
  ┌──────────────────────────────────────────────┐
  │ shell.Provider seam (core/shell/provider.go)  │   ← reused as-is, do not re-implement
  │  Parse(src) → []model.Block                   │   (core/shell/zsh/parse.go)
  │  Classify(block) → (Category, Confidence)     │   (core/shell/zsh/classify.go)
  └──────────────────────────────────────────────┘
        │  []model.Block  (Text, Kind, CmdName, Names, Exported, Opaque,
        │                   + NEW: captured Value / Body / option-args)
        ▼
  ┌──────────────────────────────────────────────┐
  │ core/ir  (NEW package — depends on            │
  │           core/model + core/shell interface)  │
  │                                                │
  │  Build([]Block, Classifier) → model.Profile   │
  │    for each block, in SOURCE ORDER:           │
  │      1. category := Classify(block)           │
  │      2. managed := routeManaged(block,cat) ───┼──▶ ING-02 gate (D-04/D-06)
  │      3. dynamic := isDynamic(block) ──────────┼──▶ EVAL-01 tag (D-05, orthogonal)
  │      4. Entry{ Text, Category, Managed,       │
  │               Dynamic, Override:auto, ... }   │   ← D-01 verbatim + derived
  │    Profile{ Entries: ordered []Entry }        │   ← D-02 single ordered list
  └──────────────────────────────────────────────┘
        │  model.Profile
        ▼
  ┌──────────────────────────────────────────────┐
  │ Regenerate(Profile) → []byte .zsh             │
  │   for each Entry, in SOURCE ORDER (D-09):     │
  │     if managed && declarative:                │
  │        template from structured fields (D-10) │   ← env/alias/func/option/PATH
  │        (dynamic values emitted verbatim)      │
  │     else: print Entry.Text verbatim (D-01)    │   ← imperative/Opaque/unmanaged
  └──────────────────────────────────────────────┘
        │  regenerated .zsh
        ▼
  ┌──────────────────────────────────────────────┐
  │ Round-trip oracle (zsh-requiring test, D-08)  │
  │   introspectScript under `zsh -f` on BOTH:    │
  │     snapshot(original) vs snapshot(regen)     │
  │   assert byte-identical state tables          │
  └──────────────────────────────────────────────┘
```

### Recommended Project Structure
```
core/
├── model/
│   ├── profile.go      # NEW: Profile, Entry, ManagedOverride, EntryKind (zero deps)
│   └── block.go        # existing — EXTEND with captured value/body fields (still zero deps)
├── shell/zsh/
│   └── parse.go        # EXTEND describe() to capture Assign.Value text / alias body /
│                       #       option args + set the dynamic flag from the AST
├── ir/                 # NEW package (depends on core/model + core/shell interface only)
│   ├── build.go        # Build([]model.Block, shell.Classifier) → model.Profile
│   ├── route.go        # declarative/imperative routing (ING-02 / D-04 / D-06)
│   └── regen.go        # Regenerate(Profile) → []byte; templater (D-10)
└── ir/ir_test.go OR analyze_test-style external test for the zsh round-trip oracle
```

### Pattern 1: Reuse the Provider seam, never re-parse
**What:** `core/ir.Build` takes `[]model.Block` (already parsed) + a `shell.Classifier` (interface), exactly as `core/analyze.Analyzer` does (`core/analyze/analyzer.go:14-43`). The concrete `zsh.Provider{}` is injected only at the composition root / in external tests.
**When to use:** Always for the IR build.
**Example:**
```go
// Source: pattern lifted from core/analyze/analyzer.go:24-43 [VERIFIED]
func Build(blocks []model.Block, c shell.Classifier) model.Profile {
    var p model.Profile
    for i := range blocks {
        cat, conf := c.Classify(blocks[i])     // reuse existing classifier
        blocks[i].Category = cat
        blocks[i].Conf = conf
        e := model.Entry{
            Text:     blocks[i].Text,           // D-01 verbatim
            Category: cat,
            Managed:  routeManaged(blocks[i], cat),  // ING-02 (D-04/D-06)
            Dynamic:  blocks[i].Dynamic,        // EVAL-01 (D-05), set at parse time
            Override: model.OverrideAuto,       // D-07
        }
        p.Entries = append(p.Entries, e)        // D-02 ordered list, source order
    }
    return p
}
```

### Pattern 2: String-templated declarative regeneration (D-10)
**What:** Rebuild env/alias/function/option/PATH entries from structured fields, not from `Text`. Follow `core/testgen/render.go`'s `fmt.Sprintf` style.
**When to use:** Managed + declarative entries only.
**Example:**
```go
// Source: precedent core/testgen/render.go:36-51 [VERIFIED]
func renderEntry(e model.Entry) string {
    switch e.Kind {
    case model.KindAssignment:
        // dynamic value (e.g. "$HOME/go") emitted VERBATIM, never resolved (D-05)
        if e.Exported { return fmt.Sprintf("export %s=%s", e.Names[0], e.Value) }
        return fmt.Sprintf("%s=%s", e.Names[0], e.Value)
    case model.KindAlias:
        return fmt.Sprintf("alias %s=%s", e.Names[0], e.Value)
    // ... functions, setopt/unsetopt
    }
}
```
**Critical:** the templated output must reproduce *behavior*, not bytes. The oracle (D-08) tolerates whitespace/quoting differences; it asserts the *state tables* match. But quoting still matters for behavior — `alias gs='git status'` vs `alias gs=git status` differ. See Pitfall 3.

### Pattern 3: Static/dynamic detection over the AST (EVAL-01)
**What:** A value is **static** iff its `*syntax.Word` is composed only of `*Lit` and `*SglQuoted` parts (single-quotes suppress expansion). It is **dynamic** if any part is `*ParamExp` (`$HOME`, `${...}`), `*CmdSubst` (`$(...)`, and `.Backquotes==true` for `` `...` ``), `*ArithmExp` (`$((...))`), or a `*DblQuoted` whose `.Parts` contain any of those.
**When to use:** At parse time in `core/shell/zsh/parse.go` (the only place with the AST), recording the result as `Block.Dynamic bool`.
**Example:**
```go
// Source: mvdan/sh v3.13.1 syntax/nodes.go:509-620 [VERIFIED: vendored module]
// Word.Lit() returns "" for any word containing a non-literal part (nodes.go:516-540),
// so the cheap path is: Lit()=="" with non-empty/non-single-quoted parts ⇒ dynamic.
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
Note: `syntax.Walk(node, func(Node) bool)` exists (`syntax/walk.go:16` [VERIFIED]). `CmdSubst.Backquotes` distinguishes legacy backticks (`syntax/nodes.go:591-604` [VERIFIED]) — both are `*CmdSubst`, so the type switch already catches them. Compound blocks (`KindCompound` — `if/for/case`) are conditionals: treat the whole entry as dynamic by construction (and route imperative anyway — they are not in the 5 declarative classes).

### Anti-Patterns to Avoid
- **Importing `core/shell/zsh` from `core/ir`:** Forbidden by the composition-root rule (CLAUDE.md; enforced in `core/cli`, `core/analyze`). `core/ir` sees only the `shell.Provider`/`shell.Classifier` interface.
- **Adding internal imports to `core/model`:** `Profile`/`Entry` go in `core/model` only if they stay stdlib-only. Verified `core/model/block.go:1` imports nothing internal.
- **Re-grouping by category in Phase 2 emit:** D-09 — emit in source order; per-category emit is Phase 4. Re-grouping introduces the cross-category load-order hazard D-02 was designed to defuse.
- **Resolving dynamic values:** `util.ExpandHome` must never appear in `core/ir`. EVAL-01 / Success Criterion 4. A `$HOME` value round-trips with `$HOME` intact.
- **Using confidence as a routing gate:** D-06 explicitly forbids dumping medium-confidence declaratives into the master block. The classifier's `Conf` is informational; routing keys off `Kind`/`Category`/`CmdName` membership.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Detecting `$VAR`/`$(...)`/backticks in a value | A regex over `Block.Text` | mvdan/sh AST walk on the value `*Word` | A regex mis-handles `'literal $HOME'` (single-quoted = static), escaped `\$`, and nested quoting. The AST already distinguishes `*Lit`/`*SglQuoted` (static) from `*ParamExp`/`*CmdSubst` (dynamic). [VERIFIED: nodes.go:542-560] |
| Re-parsing zsh source | A hand-rolled tokenizer | The existing `Provider.Parse` (`parse.go:15`) | The parser, opaque-fallback, and comment-attachment are already shipped and corpus-tested. |
| Running the config to see its state | Any `eval`/subprocess in the IR build | Static AST inspection only | EVAL-01 SC4: "no execution of user config." The only subprocess is in the *oracle test*, not the IR. |
| State-table snapshot for the oracle | A new introspection harness | Reuse `introspectScript` (`introspect.go:23-38`) | The spike validated this exact dump (aliases/functions/env/`$path`/options) as the byte-identity comparand. Don't reinvent. |
| zsh codegen | mvdan/sh `syntax.Printer` | `fmt.Sprintf` string templates (`render.go`) | CONTEXT canonical-refs steers explicitly away from the printer; the printer reformats and risks freezing dynamic values. |

**Key insight:** Almost everything Phase 2 needs already exists — the parser, classifier, the categorizer's correct bucketing of the 5 classes, the snapshot instrument, and the codegen pattern. The *new* code is (1) two model types, (2) the routing verdict, (3) the dynamic flag (a small parser enhancement), (4) the templater, (5) the oracle test. Resist re-implementing the front-end.

## The Central Design Fork (planner MUST resolve)

`model.Block` has no field for the assigned **value** or alias/function **body** — verified `core/model/block.go:27-37` (fields: `Text, StartLine, Kind, CmdName, Names, Exported, Opaque, Category, Conf`). Both EVAL-01 detection and D-10 templating need the value. Three approaches:

| # | Approach | How | Pros | Cons | Confidence |
|---|----------|-----|------|------|-----------|
| **A (recommended)** | Capture at parse time | Extend `describe()` in `parse.go` to record `Assign.Value` text (`syntax/nodes.go:288-294` — `Assign.Value *Word`), alias body, option args into new agnostic `Block` fields (e.g. `Value string`, `Dynamic bool`). | AST-accurate dynamic detection; `core/model` stays dep-free; `core/ir` stays shell-free; the value is structured for the templater. | Touches `parse.go` (the shipped parser) — must not break the testgen oracle pin or corpus. | HIGH |
| B | Re-parse the value in `core/ir` | Can't — `core/ir` can't import `syntax` (only via the zsh provider, which returns `[]Block`). | — | Violates layering; the interface returns no AST. | N/A — rejected |
| C | String-parse `Block.Text` in `core/ir` | Split on `=`, scan for `$`/`(`/backtick in the value substring. | No parser change. | Fragile vs quoting/escaping (the very thing the AST solves); re-introduces the regex pitfall above; brittle for multi-line functions. | LOW |

**Recommendation: Approach A.** The parser is the only place with the AST; capturing value + a `Dynamic bool` there is the clean seam. The new `Block` fields are agnostic strings/bools, so `core/model` stays dependency-free. Verify the change against the testgen oracle pin (`go test ./core/testgen/`) and corpus (`go test ./core/analyze/`) — both assert category/issue/line invariants that adding fields must not disturb.

## Common Pitfalls

### Pitfall 1: LOCAL_OPTIONS / `emulate -L` auto-revert in the oracle harness
**What goes wrong:** If the oracle's snapshot script (or any apply function) runs under `emulate -L zsh` or sets `LOCAL_OPTIONS`, `setopt`/`unsetopt` changes auto-revert at function return — the options diff would falsely pass (or fail) regardless of the regenerated code.
**Why it happens:** zsh scopes options to the enclosing function when `LOCAL_OPTIONS` is set; `emulate -L` implies it. The spike proved the loader must be a **plain** function (`01-FINDINGS.md` Pitfall 1 / Pattern 2; reference snippet lines 121-133).
**How to avoid:** For Phase 2's oracle, the regenerated `.zsh` is *sourced* (not run inside a wrapper fn) under `zsh -f`, exactly as `introspectScript` does (`source "$1"` at top level — `introspect.go:26`). Note `introspectScript` itself opens with `emulate -L zsh` (`introspect.go:24`) — but it `source`s the target and *reads* `$options` afterward; the sourced file's top-level `setopt` is **not** inside the harness function, so it sticks. This is the same mechanism the spike used. Keep the regenerated file sourced at top level; do not wrap apply in `emulate -L`.
**Warning signs:** Options section of the diff is always empty even when you intentionally corrupt the regenerated `setopt`.

### Pitfall 2: Breaking the testgen oracle regression pin
**What goes wrong:** Adding fields to `Block` or changing `parse.go` silently changes category counts, issue detection, or line numbers, breaking `TestOracleProperty` (`core/testgen/property_test.go:32`) or `TestCorpusGolden` (`core/analyze/corpus_test.go:32`).
**Why it happens:** The oracle pin asserts strict category-count equality, the issue (kind,name) set, `has_secrets`, zero opaque blocks, and exact line numbers across 10 seeds. The corpus asserts the same over real fixtures.
**How to avoid:** Approach A only *adds* fields to `Block` and *reads* more of the AST in `describe()`; it must not change `Kind`/`Names`/`Category` assignment. Run `make check` (fmt-check + vet + lint + test, per `Makefile:33`) after the parser change. TDD: write the IR/oracle tests first, then make the parser change, then confirm the pin stays green.
**Warning signs:** Any failure in `core/testgen` or `core/analyze` after a `parse.go` edit.

### Pitfall 3: Templater quoting changes behavior (not just bytes)
**What goes wrong:** Templating `alias gs='git status'` as `alias gs=git status` (dropping quotes) changes what zsh stores — the oracle's `${aliases[gs]}` would differ, failing byte-identity.
**Why it happens:** The templater rebuilds from structured fields (D-10); if it doesn't preserve the quoting that affects the stored value, the regenerated state diverges.
**How to avoid:** Either (a) preserve the value sub-string verbatim from the captured AST value (Approach A captures `Assign.Value` text including its original quoting), or (b) use `syntax.Quote(s, syntax.LangZsh)` (`syntax/quote.go:48` [VERIFIED]) to re-quote safely. For **dynamic** values, you MUST preserve them verbatim (D-05) — never re-quote `$HOME` into `'$HOME'` (single quotes would suppress expansion and freeze the value to the literal string). The safest path: emit the captured value text verbatim within the template (`export NAME=<captured-value-text>`), which keeps `$HOME/go` working and `'git status'` quoted.
**Warning signs:** Oracle diff shows an alias/env value present but with a different body.

### Pitfall 4: compinit / completion side effects leak into the oracle
**What goes wrong:** A real `~/.zshrc` calls `compinit`, which populates `$_comps`, autoloads `_*` functions, and writes `.zcompdump` — none reversible (`01-FINDINGS.md` class 6). If the oracle snapshots `$functions`, the autoloaded `_*` functions appear and could cause spurious diffs between original and regenerated runs.
**Why it happens:** `compinit` is imperative and non-deterministic across runs (filesystem-dependent).
**How to avoid:** Phase 2 emits in source order and routes `compinit`/`compdef`/`autoload`/`zstyle`/`zmodload` to the **imperative** verdict (D-04 lean), so they are emitted *verbatim* in both original and regenerated files — meaning their side effects are identical in both snapshots and cancel in the diff. The oracle compares original-vs-regenerated (not vs a clean base), so as long as imperative lines are byte-verbatim, their side effects match. **Do not** template these. (Phase 2 fixtures for the oracle should ideally avoid `compinit` to keep the test deterministic; real-`~/.zshrc` round-trip is a Phase 6 concern per ROADMAP line 155.)
**Warning signs:** `$functions` section shows `_git`, `_brew`, etc. differing between the two runs.

### Pitfall 5: Where does the templater live? (zsh-syntax codegen invariant)
**What goes wrong:** Putting zsh-string emission (`export ...`, `alias ...`, `setopt ...`) in `core/ir` may conflict with the milestone invariant "only `core/shell/zsh/emit.go` ever writes `unalias`/`unset -f`/`setopt` strings" (ROADMAP line 15).
**Why it happens:** That invariant is written for the **reversal/activation** emit (Phase 4's `emit.go` — `unalias`, `unset -f`, `setopt` deactivate code). Phase 2's templater emits **forward** declarative source (`export`, `alias`, `setopt` apply form) for the round-trip — arguably different. This is an ambiguity the planner should resolve explicitly.
**How to avoid:** Two defensible readings: (a) Phase 2's templater lives in `core/shell/zsh` (a new `regen.go` or proto-`emit.go`), keeping all zsh-syntax generation in the zsh package, and `core/ir` calls it via a new `shell` interface method (e.g. `Regenerate(Entry) string`); or (b) Phase 2 templates simple forward declarations in `core/ir` since they are not the reversal codegen the invariant targets, and Phase 4's `emit.go` owns the reversal codegen. **Recommendation:** lean (a) for strict invariant compliance — add a narrow `Regenerator` interface to `core/shell`, implemented in `core/shell/zsh`, so `core/ir` stays shell-syntax-free. This also positions the templater next to where Phase 4's `emit.go` will live. The planner should pick one and record it.
**Warning signs:** A plan that writes `fmt.Sprintf("export %s=...")` inside `core/ir` — flag against the invariant.

### Pitfall 6: PATH/FPATH value tagged dynamic incorrectly, or segmented prematurely
**What goes wrong:** Splitting a PATH assignment into segments, or resolving `$HOME/bin` in a PATH value, in Phase 2.
**Why it happens:** Temptation to do the Phase 4 delta work early.
**How to avoid:** D-03 — PATH/FPATH is stored as ONE verbatim value like any other env entry, tagged dynamic if it contains `$HOME`/`${...}`/`$(...)`. Segmentation (additions/deletions vs captured base) is Phase 4. The classifier already routes `*PATH*` names to `CatPath` (`classify.go:37-42`); treat `CatPath` as a declarative class (it's one of the 5 admitted), template it as a single assignment.
**Warning signs:** A Phase 2 plan that touches `lists[].additions/deletions` or imports the captured-base concept.

## Code Examples

### Detecting dynamic value at parse time (EVAL-01)
```go
// Source: extend core/shell/zsh/parse.go describe(); mvdan/sh nodes.go:288-294,542-560 [VERIFIED]
// In the *syntax.CallExpr / *syntax.DeclClause assignment branch:
for _, a := range c.Assigns {
    if a.Name != nil {
        b.Names = append(b.Names, a.Name.Value)
    }
    if a.Value != nil {
        // capture verbatim value text via positions, and the dynamic flag
        b.Value = literalValueText(src, a.Value)   // verbatim sub-slice (Approach A)
        if wordIsDynamic(a.Value) {                // AST walk (Pattern 3)
            b.Dynamic = true
        }
    }
}
```

### Round-trip oracle skeleton (D-08) — zsh-requiring, skips when absent
```go
// Source: pattern from introspect.go:42-53 + introspect_test.go:11-13 + corpus_test.go [VERIFIED]
func TestRegenRoundTrip(t *testing.T) {
    if _, err := exec.LookPath("zsh"); err != nil {
        t.Skip("zsh not installed; skipping round-trip oracle")
    }
    src := []byte("export EDITOR=nvim\nexport GOPATH=$HOME/go\nalias gs='git status'\nsetopt EXTENDED_GLOB\n")
    p := zsh.Provider{}
    blocks, _ := p.Parse(src)
    profile := ir.Build(blocks, p)
    regen := ir.Regenerate(profile)

    orig := snapshotUnderZsh(t, src)     // writes temp file, runs introspectScript via zsh -f
    got := snapshotUnderZsh(t, regen)
    if orig != got {
        t.Errorf("round-trip not byte-identical:\n--- original ---\n%s\n--- regen ---\n%s\ndiff: orig=%q got=%q", src, regen, orig, got)
    }
}
// snapshotUnderZsh reuses the exact exec.CommandContext(ctx,"zsh","-f","-c",introspectScript,...)
// shape; since introspectScript is unexported, either export a test helper from package zsh or
// inline an equivalent script in the test (the spike inlined its own throwaway script).
```
**Note:** `introspectScript` is an unexported const in package `zsh` (`introspect.go:23`). The oracle (in an external test package) cannot read it directly — either (a) add an exported `Provider.IntrospectString(src)` helper, or (b) inline an equivalent snapshot script in the test (what the spike did). The planner should choose; option (a) keeps one source of truth for the snapshot format.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| v1.0/v1.1 read-only analyzer (`Block` → `Analysis` report) | v2.0 regenerable IR (`Block` → `Profile` → `.zsh`) | This phase | The same parsed `Block` now feeds a *write-back* path, not just a report. Verbatim `Text` (D-01) is the round-trip anchor. |
| Byte-identical *source text* as the equivalence bar | Byte-identical *runtime state tables* (D-08) | Phase 1 spike | Equivalence is measured by behavior under `zsh -f`, tolerating safe whitespace/quoting differences. |

**Deprecated/outdated:** None relevant. `mvdan.cc/sh/v3` v3.13.1 is current and pinned; the syntax API used (`Word`, `WordPart`, `Walk`, `Quote`) is stable v3 surface.

## Runtime State Inventory

> Phase 2 is greenfield code (new IR types + a parser enhancement) with **no rename/refactor/migration**. No stored data, live services, OS registrations, secrets, or build artifacts carry a renamed string. Omitting the full inventory.

- **Stored data:** None — Phase 2 produces an in-memory `Profile`; serialization to a git store is Phase 3.
- **Build artifacts:** Adding `core/ir` and fields to `core/model.Block` requires a normal `go build ./...`; no stale artifacts (Go has no egg-info equivalent). Verified by `make build` (`Makefile:12`).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase 2's *forward* declarative templater is distinct from Phase 4's *reversal* `emit.go`, so the "only emit.go writes setopt strings" invariant may not strictly bind it. | Pitfall 5 | If the invariant is read strictly, a templater in `core/ir` violates it — mitigated by recommending the templater live in `core/shell/zsh` behind a `Regenerator` interface. The planner must pick. |
| A2 | The recommended `Block` enhancement (add `Value`/`Dynamic`) will not perturb the testgen oracle or corpus assertions (they assert category/issue/line, not value/dynamic). | Central Design Fork / Pitfall 2 | If a field addition somehow shifts classification, the pin breaks — mitigated by TDD: run `make check` after the change. Low risk: fields are additive and read-only in `describe()`. |
| A3 | `introspectScript`'s top-level `source "$1"` makes the sourced file's `setopt` stick (not auto-reverted), so the oracle can observe regenerated options. | Pitfall 1 | If wrong, the options section would always be empty — detectable by a deliberate-corruption test. Confidence HIGH (matches spike mechanism + `introspect.go:26,36`). |

## Open Questions

1. **Should the declarative templater live in `core/ir` or `core/shell/zsh`?**
   - What we know: the milestone invariant pins *reversal* zsh codegen to `emit.go`; Phase 2 emits *forward* declarations.
   - What's unclear: whether the invariant binds forward emission too.
   - Recommendation: put it in `core/shell/zsh` behind a new narrow `shell.Regenerator` interface; `core/ir` calls it. Strictly invariant-safe and pre-positions Phase 4's `emit.go`. (See Pitfall 5 / A1.)

2. **How does the oracle access the snapshot format given `introspectScript` is unexported?**
   - What we know: the const is package-private (`introspect.go:23`); the spike inlined its own throwaway script.
   - Recommendation: add an exported `Provider.SnapshotState(srcPath) (string, error)` (or reuse `Introspect` and compare `IdentitySet` fields) so the oracle and production share one snapshot definition. Comparing `model.IdentitySet` (aliases/functions/env/path/options maps) is arguably cleaner than string-diffing raw script output and is already returned by `Introspect`. The planner should weigh string-diff (D-08's literal reading) vs `IdentitySet`-equality (structured, already available via `introspect.go:42-52`).

3. **Is `EntryKind`/`Managed`/`Dynamic` represented as fields or a sub-enum?**
   - What we know: D-07 wants `ManagedOverride: auto|forced-managed|forced-unmanaged`; D-05 wants an orthogonal dynamic tag.
   - Recommendation: distinct fields — `Managed bool` (auto verdict), `Override ManagedOverride` (the D-07 enum, default auto), `Dynamic bool`. The effective managed state = `Override` if set, else `Managed`. Keeps the two axes orthogonal (D-05) and the override's win-and-persist semantics explicit (D-07). Naming follows `core/model` conventions (`type ManagedOverride string` with `Override*` constants, mirroring `BlockKind`/`Kind*`).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All build/test | ✓ | go 1.25.0 (go.mod) | — |
| `zsh` on `$PATH` | Round-trip oracle test only (D-08) | ✓ (assumed dev machine) | zsh 5.9 used in spike | Test skips cleanly via `exec.LookPath("zsh")` guard (like `introspect_test.go:11`) |
| `mvdan.cc/sh/v3` | Parser + dynamic detector | ✓ | v3.13.1 (go.sum, vendored) | — |
| `git` | NOT used in Phase 2 | — | — | Phase 3 concern |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** `zsh` — the oracle test skips when absent, exactly like the existing `corpus`/`introspect` tests. Pure-Go IR build/regen tests run without zsh.

## Validation Architecture

> `.planning/config.json` not read for an explicit `nyquist_validation` flag; including this section (absent ⇒ enabled).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (no external test deps) |
| Config file | none (standard `go test`) |
| Quick run command | `go test ./core/ir/... ./core/model/...` |
| Full suite command | `make check` (fmt-check + vet + lint + test — `Makefile:33`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ING-01 | Parse → Profile → regenerate produces behavior-equivalent `.zsh` | integration (zsh) | `go test ./core/ir/ -run RoundTrip` | ❌ Wave 0 |
| ING-01 | Untouched/imperative entries emitted verbatim | unit | `go test ./core/ir/ -run Verbatim` | ❌ Wave 0 |
| ING-02 | env/alias/func/option/PATH → managed; Opaque/unknown → imperative | unit | `go test ./core/ir/ -run Route` | ❌ Wave 0 |
| ING-02 | `ManagedOverride` wins over auto verdict and persists | unit | `go test ./core/ir/ -run Override` | ❌ Wave 0 |
| EVAL-01 | `$HOME`/`${...}`/`$(...)`/backtick/conditional → dynamic; literal → static | unit | `go test ./core/shell/zsh/ -run Dynamic` | ❌ Wave 0 |
| EVAL-01 | `$HOME` value round-trips with `$HOME` intact (no resolution) | integration (zsh) | `go test ./core/ir/ -run RoundTrip` | ❌ Wave 0 |
| EVAL-01 | No `util.ExpandHome` reachable from `core/ir` | static/grep | `! grep -rn ExpandHome core/ir core/model` | ❌ Wave 0 |
| (regression) | testgen oracle pin stays green after parser change | property | `go test ./core/testgen/` | ✅ exists (`property_test.go`) |
| (regression) | corpus golden stays green | golden | `go test ./core/analyze/` | ✅ exists (`corpus_test.go`) |

### Sampling Rate
- **Per task commit:** `go test ./core/ir/... ./core/model/... ./core/shell/zsh/...`
- **Per wave merge:** `make check` (includes the testgen oracle pin + corpus)
- **Phase gate:** Full suite green + round-trip oracle byte-identical before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `core/ir/build_test.go` — covers ING-02 routing + override (pure Go, no zsh)
- [ ] `core/ir/regen_test.go` — covers ING-01 verbatim + templated emit (pure Go)
- [ ] `core/ir/roundtrip_test.go` (or external `core/ir_test`) — covers ING-01/EVAL-01 byte-identical oracle (zsh-requiring, skip-guarded)
- [ ] `core/shell/zsh/dynamic_test.go` (or extend `parse_test.go`) — covers EVAL-01 static/dynamic detector
- [ ] Decide oracle snapshot access (Open Question 2): export a snapshot helper from package `zsh`, OR compare `model.IdentitySet`
- Framework install: none — Go `testing` is built in

*(The two existing regression pins — `core/testgen/property_test.go`, `core/analyze/corpus_test.go` — must stay green; they are the guard that the `parse.go` enhancement is non-breaking.)*

## Security Domain

> `security_enforcement` not found explicitly in config; including a scoped note. Phase 2 is in-memory IR construction with **no untrusted input execution** (EVAL-01: no `eval` of user config). The injection threat surface (escaping user values into emitted `eval`'d code, tracked as T-01-06 in the spike) is **deferred to Phase 4** per `01-FINDINGS.md` carry-forward line 170.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | partial | The parser already produces an `Opaque` fallback for unparseable input (`parse.go:21-28`); Phase 2 routes Opaque → imperative verbatim (no interpretation). |
| V6 Cryptography | no | No crypto; secrets are *tagged* (`CatSecrets`) but exclusion is Phase 6. |
| Code injection (forward) | deferred | Templating dynamic values verbatim (D-05) means a value like `$(rm -rf ~)` is preserved as-is — but it is **emitted to a file that the round-trip oracle sources under `zsh -f` in a sandbox**, not eval'd into the user's live shell. Live-shell injection hardening is Phase 4 (`emit.go`). |

### Known Threat Patterns for {Go IR + zsh codegen}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malicious value frozen/escaped into emitted code | Elevation/Tampering | Phase 2: emit value verbatim, never resolve; oracle runs sandboxed `zsh -f`. Real mitigation (quoting/escaping for live `eval`) is Phase 4 per spike carry-forward (T-01-06). |
| compinit non-determinism poisoning the oracle | (test integrity) | Route `compinit`/`compdef` imperative-verbatim so side effects are identical in both snapshots and cancel (Pitfall 4). |

## Sources

### Primary (HIGH confidence)
- `core/model/block.go:27-37` — `Block` fields (confirms NO value/body field)
- `core/model/category.go:7-26` — `Category` taxonomy + `Categories()` load order
- `core/shell/zsh/classify.go:20-79` — current classifier; `CatOptions` grab-bag at lines 44-47 (D-04)
- `core/shell/zsh/parse.go:15-154` — parser, opaque fallback (21-28), `describe()` AST handling, `wordLitPrefix` (144-154)
- `core/shell/zsh/introspect.go:23-53` — `introspectScript` + `zsh -f -c` + 5s timeout (the oracle instrument)
- `core/shell/zsh/introspect_test.go:11-13` — `exec.LookPath("zsh")` skip-guard
- `core/shell/provider.go:9-31` — `Parser`/`Classifier`/`Introspector`/`Provider` ISP seam
- `core/analyze/analyzer.go:14-43` — the Provider-composition pattern `core/ir` mirrors
- `core/analyze/corpus_test.go` — external-test-package wiring of `zsh.Provider{}` (oracle pattern)
- `core/testgen/render.go:36-51` — string-templated codegen precedent for D-10
- `core/testgen/property_test.go:32-108` — the oracle regression pin (must stay green)
- mvdan/sh v3.13.1 `syntax/nodes.go:288-294,509-620` — `Assign.Value`, `Word`, `WordPart`, `ParamExp`, `CmdSubst.Backquotes` (dynamic detection)
- mvdan/sh v3.13.1 `syntax/walk.go:16` — `Walk(node, func(Node) bool)`; `syntax/quote.go:48` — `Quote(s, LangZsh)`
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md` — 5 admitted classes, byte-identical bar, LOCAL_OPTIONS trap, completion split, Phase-4 carry-forwards
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md` — validated Manifest shape the IR must be able to feed (Phase 4 input)
- `go.mod` — go 1.25.0, single dep `mvdan.cc/sh/v3 v3.13.1`; `Makefile:12,15,33` — build/test/check targets

### Secondary (MEDIUM confidence)
- (none — all claims verified against repo code or the vendored module source)

### Tertiary (LOW confidence)
- (none)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new deps; all APIs verified against go.mod + vendored module source.
- Architecture / seams: HIGH — every seam cited at file:line; mirrors the shipped `core/analyze` composition pattern.
- Static/dynamic detector: HIGH — mvdan/sh AST node types and `Walk`/`Quote` read directly from v3.13.1 source.
- Central design fork (value capture): HIGH on the diagnosis (Block has no value field — verified), MEDIUM on which approach the planner picks (A recommended; B rejected on layering; C inferior).
- Templater placement: MEDIUM — genuine invariant-reading ambiguity (Open Question 1 / Pitfall 5); recommendation given.
- Oracle: HIGH — reuses the spike-validated instrument; one access-detail open question (snapshot const is unexported).

**Research date:** 2026-06-26
**Valid until:** ~2026-07-26 (30 days — stable internal codebase; `mvdan.cc/sh` pinned, no fast-moving external surface)
