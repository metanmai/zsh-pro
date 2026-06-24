# Architecture Research

**Domain:** Read-only zsh-config analyzer CLI (Go) — integrating PATH-correctness changes into an existing, reviewed-clean single-pass pipeline
**Researched:** 2026-06-24
**Confidence:** HIGH (all claims grounded in the current source tree; extraction failures empirically reproduced; testgen import boundary verified via `go list -deps`)

> Scope note: This is **integration research for milestone v1.1**, not greenfield architecture. The job is to map *where each change lands* in the existing layered pipeline and *in what order* to build them without violating the documented layering. The generic "scaling / external services" sections of the template do not apply to a single-binary read-only analyzer and are reframed as **layering boundaries** and **wire-contract** sections, which is what the roadmap author actually needs.

---

## Standard Architecture (existing — study, don't re-derive)

### System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│  COMPOSITION ROOT      core/cmd/zsh-pro/main.go   (only zsh.Provider   │
│                                                    importer in prod)   │
├──────────────────────────────────────────────────────────────────────┤
│  CLI                   core/cli/cli.go  — flags, I/O, exit-code,       │
│                        --json agent contract                          │
├──────────────────────────────────────────────────────────────────────┤
│  ENGINE (shell-free)   core/analyze/                                   │
│   Analyzer.Analyze ──► Parse ─► Classify ─► Introspect ─► Reconcile    │
│   reconciler{}  (pure): duplicateNames · duplicatePaths · shadows      │
│         depends ONLY on  core/model  +  shell.Provider (interface)     │
├──────────────────────────────────────────────────────────────────────┤
│  RENDER                core/render/  human.go · json.go                │
│         model.Analysis ──(toDTO)──► dto.Envelope ──► bytes             │
├──────────────────────────────────────────────────────────────────────┤
│  SEAM                  core/shell/provider.go  (Parser/Classifier/     │
│                        Introspector ISP interfaces)                    │
│  ZSH IMPL              core/shell/zsh/  (parse·classify·introspect)    │
├──────────────────────────────────────────────────────────────────────┤
│  LEAVES (no internal deps except model)                                │
│   core/model (domain)   core/dto (wire)   core/buildinfo   core/util   │
├──────────────────────────────────────────────────────────────────────┤
│  TEST INFRA (imports ONLY core/model — verified)                       │
│   core/testgen/  graph · generator · render · oracle · mutate          │
└──────────────────────────────────────────────────────────────────────┘

Dependency direction (strictly inward):
  cmd → cli/testgen → analyze/render → shell(interface) → model/dto
```

### Component Responsibilities (only the ones v1.1 touches)

| Component | What it owns | v1.1 change |
|-----------|--------------|-------------|
| `core/model/issue.go` | `IssueKind` enum, `Issue` struct | **MODIFY** — add `IssueRelativePath` constant + `Severity` field + `Severity` enum |
| `core/model/analysis.go` | `Analysis`, `Analysis.ExitCode()` | **MODIFY** — `ExitCode()` must ignore advisory-severity issues |
| `core/analyze/reconciler.go` | pure static detectors (`duplicatePaths`, `pathSegRe`) | **MODIFY** — correct PATH extraction + canonicalization + new relative-path detector |
| `core/analyze/analyzer.go` | pipeline orchestration | **MODIFY (small)** — wire the new detector's issues into `a.Issues` |
| `core/dto/analysis.go` | wire `Issue` struct | **MODIFY** — add `severity` JSON field |
| `core/render/json.go` | `model.Analysis → dto.Envelope` map | **MODIFY** — map `Severity`; decide `issues_found` semantics |
| `core/render/human.go` | terminal report | **MODIFY** — surface severity in the ISSUES block |
| `core/testgen/graph.go` | `Node`, `NodeKind`, `ConfigGraph` | **MODIFY** — let a path node carry a "relative/unrooted" shape |
| `core/testgen/generator.go` | `GenParams`, `Build` | **MODIFY** — `RelativePaths` knob + planting logic + relative-dup pool |
| `core/testgen/render.go` | `Node.render()` (emits `.zsh`) | **MODIFY** — render relative entries verbatim (no `/` prefix) |
| `core/testgen/oracle.go` | `Expected()` (known-correct `Analysis`) | **MODIFY** — emit `IssueRelativePath` + canonicalized dup keys + severities |
| `core/testdata/fixtures/` + `manifests.json` | golden corpus | **ADD** — `duplicate_path.zsh`, `shadowed.zsh`, `relative_path.zsh`; corpus runner gains `issue_names`/`issue_lines` assertions |

**Nothing new is created as a package.** Every change is an edit to an existing file (plus new fixture data). This is the strongest signal that the layering already accommodates the feature.

---

## The five changes — where each lands (file-by-file)

### (a) New `Issue` severity field — full vertical slice

This is the **only change that cuts through every layer** (model → dto → both renderers → exit-code). Build it as one vertical slice so the wire contract is never half-applied.

**1. `core/model/issue.go` (MODIFY)** — add the severity type + field. Recommended shape (mirrors the existing `Confidence` iota enum in `block.go` for consistency):

```go
// Severity ranks an issue. Only SevActionable bumps the exit code; advisories
// surface in output but leave the exit-code signal clean.
type Severity int

const (
    SevAdvisory Severity = iota // informational; does NOT affect exit code
    SevActionable               // a genuine problem; drives exit 3
)

type Issue struct {
    Kind     IssueKind
    Name     string
    Lines    []int
    Note     string
    Severity Severity // NEW
}
```

> Design call for the roadmap: **`SevAdvisory` should be the zero value, OR `SevActionable` should be.** Recommendation: make `SevActionable` the zero value (`SevActionable Severity = iota` first). Rationale: every existing `model.Issue{...}` literal in the reconciler (`duplicateNames`, `duplicatePaths`, `shadows`) and in the **oracle** omits `Severity`; if the zero value is `SevActionable`, those four existing kinds keep exit-3 behavior with **zero edits to their construction sites**, and only the new relative-path detector sets `Severity: SevAdvisory` explicitly. This minimizes churn and keeps the diff legible. (If `SevAdvisory` were zero instead, every existing `Issue{}` literal would need `Severity: SevActionable` appended — more churn, more chance of a missed site silently going advisory and breaking exit-3.)

**2. `core/model/analysis.go` (MODIFY)** — `ExitCode()` must stop counting advisories:

```go
func (a Analysis) ExitCode() ExitCode {
    for _, is := range a.Issues {
        if is.Severity == SevActionable {
            return ExitActionable
        }
    }
    return ExitClean
}
```
Current code is `if len(a.Issues) > 0 { return ExitActionable }` — that would wrongly return exit 3 for an advisory-only file. This is the load-bearing edit of the whole milestone.

**3. `core/dto/analysis.go` (MODIFY)** — add the wire field on `dto.Issue`:

```go
type Issue struct {
    Kind     string `json:"kind"`
    Name     string `json:"name"`
    Lines    []int  `json:"lines,omitempty"`
    Note     string `json:"note,omitempty"`
    Severity string `json:"severity"` // NEW — "actionable" | "advisory"
}
```
Emit a **string** ("actionable"/"advisory"), not the int, so the agent contract is self-describing and stable. Recommend **non-omitempty** (always present) so consumers can rely on the field existing on every issue. (See wire-contract delta below for the exact decision.)

**4. `core/render/json.go` (MODIFY)** — map it in `toDTO`. Needs a `model.Severity → string` mapping (put a `String()` method on `Severity` in `core/model` so both renderers share it and `core/dto` stays string-only with no import of `core/model`):

```go
out.Issues[i] = dto.Issue{
    Kind:     string(is.Kind),
    Name:     is.Name,
    Lines:    is.Lines,
    Note:     is.Note,
    Severity: is.Severity.String(), // NEW
}
```
**Critical second decision in this file:** `IssuesFound: len(a.Issues) > 0` (json.go:59). With advisories now living in `a.Issues`, an advisory-only file flips `issues_found` to `true` while `exit_code` stays `0`. **Recommend aligning `issues_found` with the exit-code signal** — i.e. `issues_found` should reflect *actionable* issues only, so `issues_found:false, exit_code:0` for an advisory-only file and the two fields never disagree. Suggested: add `Analysis.HasActionableIssues()` (or reuse `ExitCode() == ExitActionable`) and set `IssuesFound` from it. (Leaving it as `len > 0` is defensible but creates a confusing `issues_found:true / exit_code:0` envelope — flag for the user.)

**5. `core/render/human.go` (MODIFY)** — the ISSUES loop (lines 39–48) should mark advisories distinctly (e.g. a different glyph or a trailing `(advisory)` tag) so a human sees that a relative-path note is informational, not a hard problem. Low-risk, presentation-only.

### (b) New advisory issue kind — relative/unrooted PATH entries

**1. `core/model/issue.go` (MODIFY)** — add the constant alongside the other four:
```go
IssueRelativePath IssueKind = "relative_path"
```

**2. `core/analyze/reconciler.go` (MODIFY)** — this is where it's **detected**. The PATH entries come from the **same source** the (corrected) `duplicatePaths` uses: the `CatPath`-bucketed blocks, extracted from `Block.Text` by the new extractor (see (c)). Add a sibling pure method:
```go
func (reconciler) relativePaths(blocks []model.Block) []model.Issue
```
It walks the same extracted entries, flags any that are **not absolute** — i.e. does NOT begin with `/`, `$HOME`/`${HOME}`, or `~` after canonicalization — including **bare `.`** and **empty** (current-directory) entries, and emits `Issue{Kind: IssueRelativePath, Severity: SevAdvisory, ...}`. One issue per offending entry (with its line(s)); note text like `"relative PATH entry — resolves against the current directory"`.

**3. `core/analyze/analyzer.go` (MODIFY, ~1 line)** — append the new detector's output in the static-issue block (after `duplicatePaths`, line 70):
```go
a.Issues = append(a.Issues, az.rec.relativePaths(buckets[model.CatPath])...)
```
The existing `sort.SliceStable` at lines 91–96 already orders by `(Kind, Name)` and will fold the new kind in deterministically — **no sort change needed**.

> Layering note: both (a) and (b) stay entirely inside `core/analyze` + `core/model` + render. **`core/analyze` remains shell-free** — the new detector reads `model.Block.Text`/`.Category`, never anything from `core/shell/zsh`. No temptation to violate the seam here.

### (c) Corrected / canonicalizing PATH extraction — `core/analyze/reconciler.go`

This is the **root of the milestone** and the highest-care edit. Today `pathSegRe` (`reconciler.go:17`) does **regex segment-scraping over `Block.Text`**, which is empirically broken. Reproduced against the live regex:

| Input RHS | Current extraction | Bug |
|-----------|--------------------|-----|
| `export PATH="./scripts:$PATH"` | `["/scripts"]` | mis-named — leading `.` dropped |
| `export PATH="$PATH:.:$HOME/bin"` | `["$HOME/bin"]` | bare `.` blind spot |
| `export PATH=~/bin:${HOME}/bin:$PATH` | `["~/bin","/bin"]` | `${HOME}` not matched → `/bin`; `~` vs `${HOME}` not deduped |
| `export PATH="/usr/local/bin//:$PATH"` | `["/usr/local/bin//"]` | trailing slashes not normalized |
| `PATH=bin:$PATH` | `[]` | unrooted entry never seen |

**Recommended approach — replace regex scraping with split-based extraction:**

1. **Extract the RHS** of the PATH-family assignment from `Block.Text` (strip the `export PATH=` / `PATH=` prefix and surrounding quotes). The block is already classified `CatPath`, so the family membership is settled; the extractor only needs the value.
2. **Split on `:`** → ordered raw entries (verbatim, including `.`, empty strings, and relative names).
3. **Drop the `$PATH` / `${PATH}` self-reference** entry (and only that token) so the prepend/append idiom doesn't register as an "entry."
4. Keep the **verbatim** entry as `Issue.Name` (so `./scripts` reports as `./scripts`), but compute a separate **canonical key** for dedup.

**Canonicalization (notation-only, deterministic, NO filesystem/env):** a pure helper, e.g.
```go
func canonPathEntry(s string) string // ~ ⇒ $HOME ; ${HOME} ⇒ $HOME ; collapse // ; trim trailing /
```
- `~` ≡ `$HOME` ≡ `${HOME}` → fold to one token (string-level only; never read the real `$HOME`).
- collapse duplicate slashes, trim a trailing slash (but keep root `/`).
- Per PROJECT.md Out-of-Scope: **no** `..`/symlink/disk resolution, **no** live-`$HOME` lookup — keeps analysis read-only-pure and deterministic, and avoids the "`$HOME` reassigned mid-file" false positive.

`duplicatePaths` then keys on `canonPathEntry(entry)` (not the raw string), so `~/bin` and `${HOME}/bin` collide, and `/usr/local/bin` == `/usr/local/bin//`. `relativePaths` keys on the same extracted entries.

**Shared extraction:** both `duplicatePaths` and `relativePaths` need the same `[]entry` list. Extract once into a small unexported helper on `reconciler` (e.g. `pathEntries(b model.Block) []string`) so the two detectors agree by construction and there's a single place the split/self-ref logic lives. `pathSegRe` is **deleted**.

> Risk flag — existing test churn: `core/analyze/analyze_test.go:176–208` asserts the dup key is `"$HOME/bin"` for `export PATH="$HOME/bin:$PATH"`. After canonicalization the **reported `Name`** is still `$HOME/bin` (verbatim survives), but if `$HOME` canonicalizes to a different token the assertion may need updating. Decide the canonical form's *display*: recommend **reporting the verbatim entry** as `Name` and keeping the canonical form internal — that keeps this existing test green and the human output faithful to what the user wrote.

### (d) Testgen changes — `core/testgen` (imports ONLY `core/model` — verified)

`go list -deps ./core/testgen` returns exactly `zsh-pro/core/model` (+ itself). **This boundary is sacred and easy to break**: the temptation will be to reuse the reconciler's `canonPathEntry` helper from `core/analyze`. **Do not import it.** The oracle must compute expected canonicalization **independently** (re-implement the tiny notation-fold inside `testgen`) — that independence is what makes the property test a real oracle rather than a tautology. (If testgen called the engine's canonicalizer, a bug in that canonicalizer would be invisible to the test.)

**1. `core/testgen/graph.go` (MODIFY)** — give a path node a way to be relative/unrooted. Two options:
   - **(preferred)** add a bool field `Relative bool` on `Node` (only meaningful for `NodePathEntry`), so `Name` stays the directory and `render()`/oracle branch on it; **or**
   - reuse `Name` to already hold a relative string (e.g. `./scripts`, `.`, ``) and detect relativity in render/oracle by inspecting the string.

   The bool is cleaner and keeps the oracle's relativity test trivial and explicit.

**2. `core/testgen/generator.go` (MODIFY)** —
   - Add a **relative path pool** disjoint from `pathDirs`, e.g. `relPathDirs = []string{"./scripts", "bin", "../tools", ".", ""}` (covers relative, unrooted, parent-relative, bare-cwd, empty).
   - Add a `GenParams` knob: `RelativePaths int` (clean relative entries to plant) and optionally `DupRelativePaths int` for **relative duplicates** (the milestone explicitly wants "relative/unrooted dup paths" as the regression pin).
   - In `Build`, plant relative entries as `NodePathEntry` nodes with `Relative:true` from `relPathDirs`, partitioned out of the pool the same way base/dup names are partitioned today (so they never overlap).
   - To exercise **notation-equivalent dedup** in the oracle, also add a path-notation pair to the dup pool (e.g. plant `~/x` and `${HOME}/x` as a duplicate pair) so the oracle and engine must both canonicalize to agree.

**3. `core/testgen/render.go` (MODIFY)** — `Node.render()` case `NodePathEntry` currently always does `export PATH=%q` with `n.Name+":$PATH"`. For a relative node it must emit the entry **verbatim with no `/` root** (e.g. `export PATH="./scripts:$PATH"`, `export PATH=".:$PATH"`, `export PATH=":$PATH"` for empty). Branch on `Relative` (or the string shape). This is what produces the `.zsh` the engine then mis-handled pre-fix.

**4. `core/testgen/oracle.go` (MODIFY)** — `Expected()` must now:
   - Emit `IssueRelativePath` (Severity `SevAdvisory`) for each planted relative/unrooted/`.`/empty entry, with the correct `Node.Line`(s). Add a `relativePathIssues()` method mirroring `dupNameIssues`/`shadowIssues`.
   - For duplicate paths, key on the **testgen-local canonicalizer** (re-implemented, not imported) so notation-equivalent pairs (`~/x` vs `${HOME}/x`) register as one `IssueDuplicatePath`.
   - Set `Severity` on the four existing issue kinds. **If `SevActionable` is the zero value (recommended in (a)), no edit is needed here** — the existing `model.Issue{...}` literals in `dupNameIssues`/`shadowIssues` stay correct. Only the new `relativePathIssues` sets `Severity: SevAdvisory`.

**5. `core/testgen/property_test.go` (MODIFY)** — `propParams()` gains the new knob(s) (e.g. `RelativePaths: 1, DupRelativePaths: 1`). `assertStrict` already compares the full **issue (kind,name) set** and per-issue **lines** across 10 seeds; once the oracle emits the advisory and the engine detects it, this pins the new behavior for free. Optionally extend `assertStrict` to compare `Severity` per issue (cheap, and locks the exit-code-neutrality of advisories at the oracle level). `checkLines` is already `true`.

> The property test (`property_test.go`, 10 seeds) is the **primary regression pin** per the Constraints. It already exercises `duplicate_path`/`shadowed` line slices; extending it to relative/dup-relative paths is the milestone's success gate.

### Golden-fixture coverage (corpus) — `core/testdata/fixtures/` + `core/analyze/corpus_test.go`

PROJECT.md's fifth Active requirement: close the `duplicate_path` and `shadowed` golden gaps and assert `issue_names`/`issue_lines`.

- **ADD fixtures:** `duplicate_path.zsh`, `shadowed.zsh`, `relative_path.zsh` (the last covers the new advisory: a relative entry, a bare `.`, an empty entry).
- **ADD manifest entries** in `manifests.json` for each. The corpus runner (`corpus_test.go`) currently asserts `min_blocks`, `issue_kinds` (set equality), `has_secrets`, `expect_categories`, optional `lines`. To satisfy "asserts `issue_names`/`issue_lines`," **extend the `manifest` struct + runner** with optional `issue_names []string` and `issue_lines map[string][]int` (mirroring the optional-`Lines` pointer pattern already there) so the two formerly-untested kinds get name+line assertions. This is a **test-only** change in the external `analyze_test` package — no production touch.
- `relative_path.zsh`'s manifest is the place to assert that an advisory-only fixture yields the advisory issue kind but (if the runner also checks exit code) **exit 0** — a clean end-to-end proof of the severity tier.

---

## Data Flow — what changes

### Analysis request flow (PATH entries, post-fix)

```
src bytes
  └► Provider.Parse  → []Block (Block.Text holds `export PATH="...:$PATH"`)
       └► Provider.Classify → Block.Category = CatPath          (UNCHANGED)
            └► Analyzer.Analyze buckets[CatPath]                (UNCHANGED)
                 └► reconciler.pathEntries(block)               (NEW helper)
                      split RHS on ':' · drop $PATH · verbatim entries
                       ├► duplicatePaths   keys on canonPathEntry  → IssueDuplicatePath (SevActionable)
                       └► relativePaths    flags non-absolute      → IssueRelativePath  (SevAdvisory)  [NEW]
                 └► a.Issues (sorted by Kind,Name)               (sort UNCHANGED)
                      └► Analysis.ExitCode()  counts SevActionable only   [CHANGED]
                           └► render: human (severity glyph) · json (severity field + issues_found align)
```

### Oracle flow (testgen, independent)

```
Generator.Build(params incl. RelativePaths/DupRelativePaths)
  └► ConfigGraph with NodePathEntry{Relative:true/false}        [NEW field]
       └► RenderZsh → .zsh source (relative entries verbatim)   [render branch NEW]
            └► Expected() :
                 dup paths keyed on testgen-LOCAL canon          [NEW, must NOT import analyze]
                 relativePathIssues → IssueRelativePath/SevAdvisory  [NEW]
                 (4 existing kinds: Severity = zero = SevActionable)
```

---

## --json wire-contract delta (spell it out exactly)

The milestone is an **accepted breaking change** to `analyze --json` (PROJECT.md Constraints). Exact deltas:

**1. New issue kind value.** `analysis.issues[].kind` gains a sixth enumerated value:
```
"relative_path"   (joins duplicate_alias, reassigned_env, duplicate_path, shadowed)
```

**2. New field on every issue object** — `analysis.issues[].severity`:
```json
{
  "kind": "relative_path",
  "name": "./scripts",
  "lines": [12],
  "note": "relative PATH entry — resolves against the current directory",
  "severity": "advisory"
}
```
Values: `"actionable"` | `"advisory"`. **Recommendation: emit it on every issue (no `omitempty`)** so agents can switch on it unconditionally. The four pre-existing kinds carry `"severity":"actionable"`.

**3. Corrected `name` / `lines` values for PATH issues.** Same field shapes, but **different data**:
   - `duplicate_path` entries that were mis-named (`/scripts`) now report verbatim (`./scripts`); notation-equivalent dups (`~/x` + `${HOME}/x`) now collapse to a **single** `duplicate_path` issue instead of two/none.
   - Previously-missed unrooted/`.`/empty entries now appear (as `relative_path`).

**4. Exit-code / `issues_found` semantics.**
   - `exit_code`: an advisory-only file now returns **0** (was: would have been 3 under the old `len>0` rule had the advisory existed). Genuine dups/shadows still return **3**.
   - `issues_found` (`dto.Envelope`): **recommend** redefining as "actionable issues present" so it agrees with `exit_code` (advisory-only ⇒ `issues_found:false, exit_code:0`). If left as `len(issues)>0`, an advisory-only envelope is `issues_found:true, exit_code:0` — internally inconsistent; **flag this decision for the user.**

**5. Unchanged:** envelope shape (`tool/version/command/ok/issues_found/exit_code/analysis`), category rollups, `has_secrets`, `introspected`, `notes`, `lines`/`blocks`. No field is removed or renamed.

**Tests asserting the snapshot that will move:** `core/render/render_test.go` (`exit_code==3`, `issues_found==true` for a dup-alias sample — still valid, dup-alias stays actionable), `core/cli/cli_test.go` (envelope `exit_code` must equal process code — still valid). New tests should cover the **advisory-only** envelope (`exit_code:0`, `severity:"advisory"`).

---

## Recommended phase build order

Order is driven by **dependency direction** (leaves first) and by keeping each phase independently testable. Three phases, each a coherent vertical or horizontal slice.

```
Phase A — Severity tier (vertical slice through every layer)
  model.Issue.Severity + Severity enum  →  Analysis.ExitCode() (count actionable)
  →  dto.Issue.severity  →  render/json (map + issues_found decision)  →  render/human (glyph)
  Tests: ExitCode unit test (advisory ⇒ 0; actionable ⇒ 3); render_test advisory-only envelope.
  WHY FIRST: introduces the Severity type the other phases reference. With SevActionable as
  the zero value, this phase leaves all FOUR existing kinds' exit-3 behavior byte-identical —
  pure additive, lowest blast radius. No reconciler change yet.

Phase B — PATH extraction + dedup + relative-path detector (engine, shell-free)
  reconciler.pathEntries() (split-on-':' + drop $PATH)  →  canonPathEntry() (notation-only)
  →  duplicatePaths keys on canon  →  relativePaths() emits IssueRelativePath/SevAdvisory
  →  analyzer.go: append relativePaths(buckets[CatPath]); delete pathSegRe
  Tests: reconciler unit tests for the empirically-broken cases above; update analyze_test.go
         dup-path assertion if the canonical display shifts.
  DEPENDS ON: Phase A (relativePaths sets Severity: SevAdvisory; IssueRelativePath kind).
  STAYS shell-free — reads model.Block only.

Phase C — Oracle + golden coverage (test infra, model-only)
  testgen: Node.Relative + relPathDirs pool + GenParams.RelativePaths/DupRelativePaths
  →  render.go relative-entry branch  →  oracle relativePathIssues() + LOCAL canon for dup keys
  →  property_test propParams + (optional) Severity comparison
  →  fixtures: duplicate_path.zsh, shadowed.zsh, relative_path.zsh + manifest issue_names/issue_lines
  DEPENDS ON: Phase B (engine must actually produce the new/ corrected issues for the property
             test to pass) and Phase A (IssueRelativePath/Severity exist in model).
  CRITICAL: testgen re-implements canonicalization locally — MUST NOT import core/analyze.
```

**Dependency graph:** `A → B → C` (strict). A is self-contained; B needs A's `Severity`/`IssueRelativePath`; C needs B's runtime behavior to assert against and A's model symbols. C is also the regression pin that proves A+B end-to-end.

> Alternative ordering considered: doing extraction (B) before severity (A). Rejected — B's `relativePaths` detector needs `SevAdvisory` to exist, and wiring an advisory kind into `a.Issues` *before* `ExitCode()` is severity-aware would transiently make advisory-only configs exit 3 (a wrong intermediate state). A-first avoids that window.

---

## Anti-Patterns to avoid (layering traps specific to this milestone)

### Trap 1: testgen importing the engine's canonicalizer
**What people do:** reuse `analyze.canonPathEntry` from `core/testgen/oracle.go` to avoid duplicating the notation-fold.
**Why it's wrong:** breaks the verified `testgen → core/model only` boundary, AND turns the property test into a tautology (a canon bug becomes invisible).
**Do instead:** re-implement the tiny notation-fold inside `testgen`. The duplication is the point — two independent implementations agreeing is the oracle's value.

### Trap 2: doing PATH family-detection in the reconciler instead of using the classifier
**What people do:** re-inspect `Block.Text` for `PATH=`/`fpath` inside `reconciler.duplicatePaths` to decide which blocks are PATH manipulations.
**Why it's wrong:** duplicates the classifier's job and risks the engine reaching toward shell-specifics.
**Do instead:** trust `buckets[model.CatPath]` (already classified). The reconciler only **extracts entries** from text it's already been told is PATH-family. (Note: classifier PATH over-capture is explicitly **out of scope** — PROJECT.md — so don't "fix" it here.)

### Trap 3: filesystem/env resolution sneaking into canonicalization
**What people do:** resolve `~`/`$HOME` against the real environment, or `..`/symlinks against disk, to "really" dedup.
**Why it's wrong:** makes a read-only static analyzer env-dependent and non-deterministic; reintroduces the "`$HOME` reassigned mid-file" false positive. Explicitly Out-of-Scope.
**Do instead:** string-level notation fold only. `~` ≡ `$HOME` ≡ `${HOME}` as **tokens**, slash normalization — nothing more.

### Trap 4: advisories bumping the exit code
**What people do:** leave `ExitCode()` as `len(a.Issues) > 0` after adding advisories to `a.Issues`.
**Why it's wrong:** defeats the entire severity tier — relative-path notes would return exit 3 and pollute the agent signal.
**Do instead:** `ExitCode()` (and ideally `issues_found`) gate on `Severity == SevActionable`.

### Trap 5: forgetting `issues_found` when only fixing `ExitCode()`
**What people do:** make `ExitCode()` severity-aware but leave `IssuesFound: len(a.Issues) > 0` in json.go.
**Why it's wrong:** emits a self-contradictory envelope (`issues_found:true, exit_code:0`).
**Do instead:** derive both from the same actionable-issue check. Flag the precise semantics to the user before implementing.

---

## Integration Points (summary table for the roadmap author)

| File | New / Modified | Integration point | Layer |
|------|----------------|-------------------|-------|
| `core/model/issue.go` | MODIFY | `Severity` enum + field; `IssueRelativePath` const; `Severity.String()` | leaf (domain) |
| `core/model/analysis.go` | MODIFY | `ExitCode()` counts `SevActionable` only | leaf (domain) |
| `core/analyze/reconciler.go` | MODIFY | `pathEntries()` (split/`$PATH`-drop), `canonPathEntry()`, `relativePaths()`; delete `pathSegRe`; `duplicatePaths` re-keys | engine (shell-free) |
| `core/analyze/analyzer.go` | MODIFY (~1 line) | append `relativePaths(buckets[CatPath])` | engine (shell-free) |
| `core/dto/analysis.go` | MODIFY | `Issue.Severity string` + json tag | leaf (wire) |
| `core/render/json.go` | MODIFY | map `Severity`; align `IssuesFound` with actionable | render |
| `core/render/human.go` | MODIFY | severity marker in ISSUES block | render |
| `core/testgen/graph.go` | MODIFY | `Node.Relative bool` | test infra (model-only) |
| `core/testgen/generator.go` | MODIFY | `relPathDirs` pool; `GenParams.RelativePaths`/`DupRelativePaths`; planting | test infra (model-only) |
| `core/testgen/render.go` | MODIFY | relative-entry render branch | test infra (model-only) |
| `core/testgen/oracle.go` | MODIFY | `relativePathIssues()`; LOCAL canon for dup keys; severities | test infra (model-only) |
| `core/testgen/property_test.go` | MODIFY | `propParams()` knobs; optional `Severity` compare | test infra |
| `core/analyze/analyze_test.go` | MODIFY | dup-path assertion may shift; add broken-case unit tests | external test |
| `core/analyze/corpus_test.go` | MODIFY | extend `manifest` + runner with `issue_names`/`issue_lines` | external test |
| `core/testdata/fixtures/*.zsh` + `manifests.json` | ADD | `duplicate_path.zsh`, `shadowed.zsh`, `relative_path.zsh` | test data |
| `core/render/render_test.go` | MODIFY/ADD | advisory-only envelope (`exit_code:0`, `severity`) | test |

**No new packages. No new dependencies.** Every layering constraint in PROJECT.md is honored by edits inside existing boundaries: `core/analyze` reads only `model`/interface; `core/testgen` keeps its `model`-only dependency (verified via `go list -deps`); `core/model` (domain) and `core/dto` (wire) stay separate and mapped solely in `core/render/json.go`.

## Sources

- Live source tree (HIGH): `core/analyze/{reconciler,analyzer}.go`, `core/model/{issue,block,analysis,exitcode,category}.go`, `core/dto/{analysis,envelope}.go`, `core/render/{json,human}.go`, `core/testgen/{graph,generator,render,oracle,property_test}.go`, `core/analyze/{analyze_test,corpus_test}.go`, `core/shell/zsh/{parse,classify}.go`, `core/testdata/fixtures/manifests.json`
- `.planning/PROJECT.md` — v1.1 milestone scope, Key Decisions, Out-of-Scope (HIGH)
- `go list -deps ./core/testgen` — empirically confirms testgen depends only on `zsh-pro/core/model` (HIGH)
- Empirical reproduction of `pathSegRe` extraction failures via a throwaway Go program (HIGH) — confirms `./scripts→/scripts`, bare `.`/unrooted blind spots, `${HOME}` miss, trailing-slash non-normalization

---
*Architecture research for: zsh-pro v1.1 (Trustworthy PATH Analysis) — integration mapping*
*Researched: 2026-06-24*
