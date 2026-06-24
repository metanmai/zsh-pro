# Phase 2: Issue Severity Tier - Research

**Researched:** 2026-06-24
**Domain:** Go enum design + additive vertical slice (model → dto → renderers → exit-code) in a read-only zsh-config analyzer CLI
**Confidence:** HIGH — every claim is grounded in the live source tree (read directly this session); no external library behavior is in question; the single dependency (`mvdan.cc/sh/v3`) is not touched by this phase.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Two-value tier only — `actionable` and `advisory`. `SevActionable` is the **zero value**, so the four existing issue kinds (`duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed`) keep their behavior with **no edits to their construction sites** (reconciler + testgen `Issue{}` literals). The enum stays extensible (add values later) without breaking the zero-value contract — do NOT add a 3+ scale speculatively.
- **D-02:** Severity is surfaced in `--json` as an **always-present (non-`omitempty`) self-describing string** (`"actionable"`/`"advisory"`) on every issue, so agents can switch on it unconditionally.
- **D-03:** The two tiers are named `actionable` / `advisory` (not `error`/`warning`). Self-describing for the agent contract; avoids clashing with the envelope's `ok`/error semantics.
- **D-04:** `Analysis.ExitCode()` and the envelope `issues_found` flag both count **actionable** issues only. An advisory-only config exits `0` with `issues_found: false`. Advisories never bump the exit code; exit 3 stays reserved for actionable issues. This is the SEV-02 wire-contract change — **confirmed in discussion**. It is inert for every existing config until Phase 3 introduces the first advisory (today all issues are actionable).
- **D-05:** Advisories render in the **single ISSUES list** with a distinct, softer marker (e.g. `~`) instead of the actionable `!`, so they read as non-blocking but stay inline. NOT a separate section.
- **D-06:** The human report **summary tallies advisories separately** from actionable issues — e.g. "N issues, M advisories" — so advisories are visible but clearly not counted as actionable "issues".

### Claude's Discretion
- The Go `Severity` type representation (`int` + `iota` + `String()` vs string constants) — the *wire encoding* is the string regardless (D-02).
- The exact softer marker character for advisories (`~`, `·`, …) and the precise summary-line wording, as long as advisories are visually + numerically distinguished from actionable issues (D-05/D-06).
- Whether to add a small helper (e.g. `Analysis.hasActionable()` / an actionable count) shared by `ExitCode()`, `issues_found`, and the summary.

### Deferred Ideas (OUT OF SCOPE)
- **Richer severity scale** (e.g. `error`/`warning`/`info`) — only if a real need appears; the enum is extensible. Not now.
- **Severity-based filtering** (e.g. `--severity advisory` CLI flag) — a new capability; its own future phase.
- **Classifier confidence surfacing** (PREC-01/02) — a separate axis (`Block.Conf` on blocks/categories, not `Issue.Severity`); separate future phase.
- The `relative_path_entry` advisory itself + CWE-427 (Phase 3); golden/oracle assertions on severity (Phase 4, COV-02/03).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SEV-01 | Every issue carries a severity (`actionable` or `advisory`), surfaced in both the human report and the `--json` envelope as a self-describing string. `actionable` is the default (zero value) so the four existing issue kinds are unchanged. | Severity type idiom (§ Standard Stack / Pattern 1) — `type Severity int` + `iota` with `SevActionable` first as zero value; `Severity.String()` in `core/model` mirroring `Category.Description()`; wire field on `dto.Issue` (non-omitempty); `toDTO` maps via `.String()`; human marker per D-05. |
| SEV-02 | The exit code and the `issues_found` flag reflect only actionable issues — a config whose only finding is an advisory exits `0` with `issues_found: false`. Advisories never bump the exit code; exit 3 stays reserved for genuine problems. | Shared actionable predicate (§ Pattern 3) — `Analysis.HasActionableIssues()` on `core/model/analysis.go` consumed by both `ExitCode()` and `json.go`'s `IssuesFound`; replaces today's `len(a.Issues) > 0` in two places. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

These are extracted from `./CLAUDE.md` and carry the same authority as locked decisions. The planner must not recommend approaches that violate them.

| Constraint | What it means for this phase |
|-----------|------------------------------|
| **No new dependencies** | `go.mod`/`go.sum` must not change. This phase is pure stdlib + existing `core/*` packages. (Verified: nothing here needs anything beyond `core/model`, `core/dto`, `core/render`, and the existing `fmt`/`strings`/`encoding/json` imports.) |
| **`core/analyze` stays shell-free** | The severity field touches `core/model` (leaf) + `core/render` + `core/dto`. The reconciler is **not** edited in this phase (D-01: zero-value keeps its `Issue{}` literals correct). No risk of reaching into `core/shell/zsh`. |
| **`core/model` is a leaf** | `Severity` type + `String()` method live in `core/model` and import nothing internal. `core/dto` stays string-only and imports nothing from `core/`. The int→string mapping is invoked at the render seam (`json.go`), exactly as `string(c.Category)` already is. |
| **model ↔ dto separation** | The `model.Severity` enum and the `dto.Issue.Severity string` wire field are threaded **manually** in `core/render/json.go` (`toDTO`). dto never imports model. |
| **One type per file, snake_case filenames; role-type methods** | The `Severity` type is added to the existing `core/model/issue.go` (it belongs with `Issue`/`IssueKind`, mirroring how `Confidence` lives in `block.go` next to `Block`). `Severity.String()` is a method on the typed enum, matching `Category.Description()` and the established "methods on role types" convention. No new file is required. |
| **`gofmt` / `go vet` / `golangci-lint` clean; TDD** | All edits must be fmt-clean and pass `make check` (fmt-check + vet + lint + test). Tests are written/updated alongside (TDD); the `testgen` 10-seed property test remains the primary regression pin and stays green by construction (D-01). |
| **`analyze --json` wire-contract change is accepted** | Adding `severity` to every issue and redefining `issues_found`/`exit_code` as actionable-only is the SEV-01/SEV-02 wire change, explicitly accepted (D-02, D-04). |

## Summary

This phase is a **textbook additive vertical slice** through a small, reviewed-clean Go codebase: add a `Severity` type + field to `model.Issue`, surface it as a self-describing string on every `--json` issue, and make `ExitCode()` + `issues_found` + the human summary count only `actionable` issues. There is **no new dependency, no new package, no new file** — every change is an edit inside an existing file. The single most important design lever is **making `SevActionable` the zero value** (`SevActionable Severity = iota` first): this is what lets all four existing `Issue{}` construction sites (in `core/analyze/reconciler.go` and `core/testgen/oracle.go`) stay **byte-identical with zero edits**, which is exactly D-01.

The codebase already contains the precedent for every decision this phase needs. The most idiomatic representation is **`type Severity int` with `iota` + a `Severity.String() string` method living in `core/model`** — this directly mirrors two existing patterns: `Confidence int` + `iota` in `core/model/block.go` (the enum shape), and `func (c Category) Description() string` in `core/model/category.go` (a model-layer method returning a human string that the renderers call). The int→string mapping therefore lives as a **method on the model type**, and the render seam (`json.go` `toDTO`) calls `is.Severity.String()` exactly the way it already calls `string(c.Category)`. `core/dto` stays string-only and imports nothing from `core/model`, preserving the model↔dto separation.

The key risk flagged in the milestone research — **golden byte-stability** — turns out to be **non-existent for this phase**. The golden corpus runner (`core/analyze/corpus_test.go`) asserts against the `model.Analysis` struct field-by-field (block count, has-secrets, optional line count, issue *kinds* as set equality, categories). **It never serializes to JSON and never inspects `Name`, `Lines`, `Note`, or `Severity`.** Adding a `severity` field changes no bytes it observes. Likewise the testgen property test (`assertStrict`) compares only `(Kind, Name)` keys and `Lines` — never severity — so it stays green. The only tests that touch `exit_code`/`issues_found` (`render_test.go`, `cli_test.go`) all use an **actionable** issue (`duplicate_alias`) and therefore remain correct under the new actionable-only rule with **no edits**. There is genuinely no fixture or snapshot churn in this phase.

**Primary recommendation:** `type Severity int` + `const ( SevActionable Severity = iota; SevAdvisory )` + `func (s Severity) String() string` (returns `"actionable"`/`"advisory"`) — all in `core/model/issue.go`; add `Severity Severity` to `model.Issue`; add `Analysis.HasActionableIssues()` to `core/model/analysis.go` and consume it from both `ExitCode()` and `json.go`'s `IssuesFound`; add `Severity string \`json:"severity"\`` (non-omitempty) to `dto.Issue` and map it in `toDTO` via `.String()`; update the human ISSUES loop to use `~` for advisories and add a "N issues, M advisories" tally. Touch **no** construction site, **no** fixture, **no** existing test.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| `Severity` type + `String()` mapping | `core/model` (domain leaf) | — | Domain enum; the int→string mapping is a model method (precedent: `Category.Description()`). dto must stay string-only, so the mapping cannot live there; render must stay a thin mapper, so it calls the method rather than owning a lookup table. |
| `Issue.Severity` field | `core/model` (domain leaf) | — | The field is a property of the domain `Issue`. |
| Actionable predicate (`HasActionableIssues`) | `core/model` (domain leaf) | — | Both `ExitCode()` (model) and `IssuesFound` (render) must agree; the single source of truth belongs on the domain type both can call. `ExitCode()` already lives on `Analysis`, so co-locating the predicate avoids any layering break. |
| Exit-code derivation | `core/model` (`Analysis.ExitCode()`) | — | Already there; edit in place to consult the predicate. |
| `severity` wire field | `core/dto` (wire leaf) | `core/render` (mapping) | dto declares the JSON shape; render maps `model.Severity` → wire string. |
| `issues_found` wire flag | `core/render` (`json.go` `toDTO`) | `core/model` (predicate) | The envelope flag is set in `toDTO`; its *value* comes from the model predicate so it can never disagree with `exit_code`. |
| Human report marker + advisory tally | `core/render` (`human.go`) | — | Presentation-only; reads `Issue.Severity`. |

## Standard Stack

### Core
This phase introduces no libraries. The "stack" is the existing language + project facilities.

| Facility | Version | Purpose | Why Standard |
|----------|---------|---------|--------------|
| Go stdlib (`fmt`, `strings`, `encoding/json`) | Go 1.25.7 | enum `String()`, human formatting, JSON marshal | Already imported by the touched files (`human.go` imports `fmt`/`strings`; `json.go` imports `encoding/json`). `[VERIFIED: go version → go1.25.7 darwin/arm64]` |
| `iota` typed-int enum + `String()` method | language feature | the `Severity` type | Direct precedent in this repo: `Confidence int` + `iota` (`core/model/block.go:18-24`) for the enum shape; `func (c Category) Description() string` (`core/model/category.go:29`) for a model-layer string-mapping method the renderers call. `[VERIFIED: codebase read]` |

### Supporting
None. No supporting library is needed or permitted (CLAUDE.md: no new dependencies).

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `type Severity int` + `iota` + `String()` | `type Severity string` with `SevActionable Severity = "actionable"`, `SevAdvisory = "advisory"` (mirrors `IssueKind string`) | **String-typed is viable and also satisfies D-01/D-02**, but it is the weaker fit here. (a) The zero value of a string-typed `Severity` is `""`, not `"actionable"` — to honor D-01 ("`SevActionable` is the zero value") you would have to either accept that the zero value is the empty string (and then special-case `"" → actionable` in the wire mapping and exit logic, which is exactly the "default un-set severity backwards" foot-gun the milestone PITFALLS #6 warns about) or audit every `Issue{}` literal to set it explicitly (violating D-01's "no edits to construction sites"). (b) `int`+`iota` makes the zero value *genuinely* `SevActionable` with no empty-string ambiguity, which is the cleaner expression of the invariant. (c) The wire encoding is a string **either way** (D-02) — with `int`+`iota` you get it via `String()`; the in-repo precedent (`Confidence int` + `iota`, `Category.Description()`) already establishes the int-enum-with-string-method pattern, so `int`+`iota` is the more consistent choice. **Recommendation: `int` + `iota`.** |
| `Severity.String()` method on the model type | a `map[Severity]string` lookup table in `core/render/json.go` | A render-layer map would (a) duplicate knowledge the human renderer also needs (forcing two maps or an exported one), and (b) break the established convention that model types own their human/string projections (`Category.Description()`). The method keeps `core/dto` string-only and lets *both* renderers share one mapping. **Recommendation: method on the model type.** |
| Reuse `ExitCode() == ExitActionable` for `issues_found` | a dedicated `HasActionableIssues()` predicate | Reusing `ExitCode()` works and is defensible (it is already actionable-aware after the edit). A named boolean predicate reads more clearly at the `IssuesFound:` call site and at the human-summary tally, and decouples "is there an actionable issue" from the exit-code enum. **Either is acceptable (this is explicit Claude's-discretion); a named predicate is marginally cleaner. Pick one and use it in all three places (`ExitCode`, `IssuesFound`, human summary) so they cannot drift.** |

**Installation:** None. `go.mod`/`go.sum` unchanged. `[VERIFIED: go.mod read — module zsh-pro, go 1.25.0, single require mvdan.cc/sh/v3 v3.13.1]`

## Package Legitimacy Audit

> Not applicable — this phase installs **zero external packages**. `go.mod` is unchanged. The only dependency in the project (`mvdan.cc/sh/v3 v3.13.1`) is already present, already pinned in `go.sum`, and is not touched by this phase (the reconciler/parser are not edited here). No slopcheck/registry audit is required because nothing is added.

## Architecture Patterns

### System Architecture Diagram

The phase is a vertical slice; this shows how a `Severity` value flows from where it is set (a construction site in the engine) to where it is observed (exit code + both renderers). **In Phase 2 every issue is `SevActionable` (zero value), so the advisory branch is dormant until Phase 3 plants the first `SevAdvisory`.**

```
                                  reconciler.go / oracle.go
                                  model.Issue{...}            ← Severity OMITTED ⇒ zero ⇒ SevActionable
                                        │                       (NO edit to these sites — D-01)
                                        ▼
                          model.Analysis.Issues []model.Issue
                                        │
            ┌───────────────────────────┼───────────────────────────────┐
            ▼                           ▼                                ▼
   Analysis.ExitCode()        Analysis.HasActionableIssues()      render layer
   (model/analysis.go)        (model/analysis.go)  ◄── NEW           │
        │                           │  shared predicate              │
        │ consults predicate        │                                │
        ▼                           │                  ┌─────────────┴──────────────┐
   ExitActionable(3) iff            │                  ▼                            ▼
   ∃ issue.Severity==               │           render/json.go               render/human.go
   SevActionable                    │           toDTO():                      ISSUES loop:
        │                           │            Severity: is.Severity         marker = "!" if actionable
        ▼                           │              .String()  ◄── NEW          else "~"  (D-05)
   process exit code                └─────────►   IssuesFound: a.              summary: "N issues,
   (cli.go: a.ExitCode())                          HasActionableIssues()        M advisories" (D-06)
                                                    ◄── CHANGED (was            │
                                                        len(a.Issues)>0)        ▼
                                                       │                   human report bytes
                                                       ▼
                                                  dto.Envelope ──► one JSON object
                                                  (dto.Issue.Severity string,
                                                   non-omitempty — D-02)
```

Data-flow trace for the primary use case (an `analyze --json` run): source bytes → `Analyzer.Analyze` builds `Issues` (all `SevActionable` today) → `JSONRenderer.toDTO` maps each issue and sets `Severity: is.Severity.String()` and `IssuesFound: a.HasActionableIssues()` and `ExitCode: int(a.ExitCode())` → one `dto.Envelope` marshaled to stdout. The component-to-file mapping is in Component Responsibilities below.

### Component Responsibilities (files this phase MODIFIES)

| Component | File | Change | Risk |
|-----------|------|--------|------|
| `Severity` type + `String()` + field | `core/model/issue.go` | ADD `type Severity int`, `const (SevActionable Severity = iota; SevAdvisory)`, `func (s Severity) String() string`, and `Severity Severity` field on `Issue` | LOW — leaf package, additive |
| Exit-code + shared predicate | `core/model/analysis.go` | ADD `func (a Analysis) HasActionableIssues() bool`; CHANGE `ExitCode()` from `if len(a.Issues) > 0` to consult the predicate | LOW — but **load-bearing** (SEV-02); covered by a new unit test |
| Wire field | `core/dto/analysis.go` | ADD `Severity string \`json:"severity"\`` to `dto.Issue` (non-omitempty) | LOW — leaf, additive |
| Mapping + `issues_found` | `core/render/json.go` | ADD `Severity: is.Severity.String()` in `toDTO`'s issue loop; CHANGE `IssuesFound: len(a.Issues) > 0` → `IssuesFound: a.HasActionableIssues()` | LOW — single mapping site |
| Human marker + tally | `core/render/human.go` | CHANGE the `! %-16s` per-issue line to branch the glyph on `is.Severity` (`!` actionable / `~` advisory — D-05); ADD an advisory count to the report (D-06) | LOW — presentation only |

**Files this phase does NOT touch (and why):**
- `core/analyze/reconciler.go`, `core/analyze/analyzer.go` — **no edit.** Zero-value=actionable (D-01) keeps the four `Issue{}` literals correct; no advisory is produced until Phase 3. `[VERIFIED: reconciler.go:54,77,134 — three Issue{} literals, none set Severity]`
- `core/testgen/*` — **no edit.** Oracle `Issue{}` literals (oracle.go:71, oracle.go:98) omit `Severity` ⇒ zero ⇒ `SevActionable`, matching the engine. `[VERIFIED: oracle.go read]`
- `core/testdata/fixtures/*` + `manifests.json` — **no edit.** Corpus runner never observes severity or JSON bytes (see Pitfall: Golden byte-stability). PATH/shadowed fixtures are Phase 4 (COV-01).

### Recommended Project Structure
No structural change. All edits land in existing files:
```
core/
├── model/
│   ├── issue.go      # + Severity type, String(), Issue.Severity field
│   └── analysis.go   # + HasActionableIssues(); ExitCode() consults it
├── dto/
│   └── analysis.go   # + dto.Issue.Severity (json:"severity", non-omitempty)
└── render/
    ├── json.go       # toDTO: map Severity via String(); IssuesFound via predicate
    └── human.go      # ISSUES loop: glyph by severity (D-05); advisory tally (D-06)
```

### Pattern 1: Typed-int enum with zero-value default + `String()` projection (the Severity type)
**What:** Represent `Severity` as `type Severity int` with `iota` constants where the **first** constant is the default-by-omission value, plus a `String()` method that returns the self-describing wire string. This is the exact shape of `Confidence` in `block.go`, combined with the model-method-returns-string shape of `Category.Description()`.
**When to use:** For this `Severity` type (SEV-01).
**Why this keeps the invariant cleanly:** Because `SevActionable` is declared first, it is `0`. Any `model.Issue{Kind: ..., Name: ...}` literal that omits `Severity` gets `Severity == SevActionable` for free — which is precisely D-01's "no edits to construction sites." A string-typed enum's zero value would be `""`, forcing either an empty-string special-case or edits to every literal.
**Example (recommended shape — mirrors existing `Confidence` + `Category.Description`):**
```go
// Source: pattern composed from core/model/block.go:18-24 (Confidence iota enum)
//         and core/model/category.go:29 (model method returning a human string).
// File: core/model/issue.go

// Severity ranks an issue. SevActionable is the zero value: an Issue literal
// that omits Severity is actionable, so the existing issue kinds keep their
// exit-3 behavior with no construction-site edits. Only an advisory opts in.
type Severity int

const (
	SevActionable Severity = iota // genuine problem; drives exit 3 and issues_found
	SevAdvisory                   // informational; never bumps the exit code
)

// String is the self-describing wire/report label ("actionable" | "advisory").
// Mirrors Category.Description() — a model type owning its string projection so
// both renderers (and toDTO) share one mapping and core/dto stays string-only.
func (s Severity) String() string {
	switch s {
	case SevAdvisory:
		return "advisory"
	default: // SevActionable (and any future-unset value) reads as actionable
		return "actionable"
	}
}

type Issue struct {
	Kind     IssueKind
	Name     string
	Lines    []int
	Note     string
	Severity Severity // NEW — zero value (SevActionable) for the four existing kinds
}
```
> Note on the `default:` arm: returning `"actionable"` for the default case is deliberate and matches D-01 (un-set ⇒ actionable). It also means a hypothetical future-added constant that a renderer forgot to handle degrades to the *safe* (exit-bumping, visible) label rather than silently disappearing.

### Pattern 2: Wire field threaded manually through `toDTO` (model↔dto separation)
**What:** `dto.Issue` gains a `Severity string` field with a JSON tag; `core/render/json.go`'s `toDTO` maps `model.Severity` → string by calling `.String()`. dto never imports model.
**When to use:** For the SEV-01 wire surface (D-02).
**Why:** This is the established mapping seam — `toDTO` already does `Kind: string(is.Kind)` and `Category: string(c.Category)`. Adding `Severity: is.Severity.String()` is the same move. Non-omitempty tag ⇒ the field is present on every issue (D-02).
**Example:**
```go
// Source: core/dto/analysis.go (existing dto.Issue) + core/render/json.go:46-51 (existing toDTO loop)

// core/dto/analysis.go
type Issue struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Lines    []int  `json:"lines,omitempty"`
	Note     string `json:"note,omitempty"`
	Severity string `json:"severity"` // NEW — always present (D-02): "actionable" | "advisory"
}

// core/render/json.go — inside toDTO's `for i, is := range a.Issues` loop
out.Issues[i] = dto.Issue{
	Kind:     string(is.Kind),
	Name:     is.Name,
	Lines:    is.Lines,
	Note:     is.Note,
	Severity: is.Severity.String(), // NEW
}
```

### Pattern 3: Single actionable predicate shared by exit-code and issues_found (SEV-02)
**What:** One boolean predicate on `model.Analysis` is the sole source of truth for "are there actionable issues?", consumed by `ExitCode()` (model), `IssuesFound` (render), and the human summary tally. Prevents the self-contradictory `issues_found:true / exit_code:0` envelope (PITFALLS #5/Trap 5).
**When to use:** For SEV-02.
**Why here and not in render:** `ExitCode()` already lives on `Analysis` in `core/model`. Putting the predicate beside it keeps the exit-code logic in the model (where it is today) and lets the render layer *call* it rather than re-deriving the actionable count — so the two can never disagree, and no layering boundary is crossed (render already imports `core/model`).
**Example:**
```go
// Source: core/model/analysis.go:25-30 (existing ExitCode) + render/json.go:59 (existing IssuesFound)

// core/model/analysis.go
// HasActionableIssues reports whether any issue is actionable. It is the single
// source of truth for both the exit code and the envelope's issues_found flag,
// so the two can never disagree (an advisory-only analysis is clean: exit 0,
// issues_found:false).
func (a Analysis) HasActionableIssues() bool {
	for _, is := range a.Issues {
		if is.Severity == SevActionable {
			return true
		}
	}
	return false
}

func (a Analysis) ExitCode() ExitCode {
	if a.HasActionableIssues() {
		return ExitActionable
	}
	return ExitClean
}

// core/render/json.go — in the returned dto.Envelope
IssuesFound: a.HasActionableIssues(), // CHANGED from: len(a.Issues) > 0
```

### Pattern 4: Severity-aware human report (D-05 + D-06)
**What:** Keep the single ISSUES list; choose the per-issue glyph by severity (`!` actionable, `~` advisory); add an advisory count to the report so advisories are numerically visible but not counted as actionable "issues".
**When to use:** SEV-01 human surface + D-05/D-06.
**Example (illustrates the shape; exact marker char and wording are Claude's discretion):**
```go
// Source: core/render/human.go:34-48 (existing ISSUES loop) — current code uses a fixed "!".
// The marker is currently hardcoded:  line := fmt.Sprintf("   ! %-16s %s", is.Kind, is.Name)

// Count actionable vs advisory up front (or reuse a.HasActionableIssues()).
actionable, advisory := 0, 0
for _, is := range a.Issues {
	if is.Severity == model.SevAdvisory {
		advisory++
	} else {
		actionable++
	}
}
// ... in the per-issue loop, pick the glyph:
marker := "!"
if is.Severity == model.SevAdvisory {
	marker = "~" // softer, non-blocking (D-05) — exact char is discretionary
}
line := fmt.Sprintf("   %s %-16s %s", marker, is.Kind, is.Name)
// ... and a tally line for the summary, e.g.:
//   "  %d issues, %d advisories\n"  (D-06) — exact wording is discretionary
```
> The existing human-report top line is `"%d lines, %d blocks"` (human.go:17). D-06's "N issues, M advisories" tally can sit either near that header or as a closing line of the ISSUES section — placement/wording is discretionary, as long as advisories are numerically distinguished from actionable issues. Note the current empty-issues early-return (`human.go:35-37`, "none found — nice and clean") should still read correctly when the only finding is an advisory; if you want the summary tally to appear even with advisories present, ensure the advisory path doesn't hit the `len(a.Issues) == 0` early return (it won't — advisories are in `a.Issues`).

### Anti-Patterns to Avoid
- **Making `SevAdvisory` the zero value** (or using a string enum whose zero value is `""`) — would silently turn an un-set issue advisory, making genuine duplicates non-actionable (exit-3 regression), OR force edits to every `Issue{}` literal (violates D-01). Always declare `SevActionable` first. (PITFALLS #6.)
- **Leaving `ExitCode()` severity-aware but `IssuesFound` as `len(a.Issues) > 0`** — emits a self-contradictory `issues_found:true / exit_code:0` envelope. Both must come from the one predicate. (PITFALLS #5 / Trap 5.)
- **Putting the int→string map in `core/render`** — duplicates a mapping the human renderer also needs and breaks the model-owns-its-string-projection convention (`Category.Description()`). Use a `String()` method on `Severity`.
- **Importing `core/model` into `core/dto`** — breaks the leaf/wire separation. dto stays string-only; the mapping happens in render.
- **Editing reconciler / oracle / fixtures "to be safe"** — unnecessary and risky. Zero-value=actionable means they are already correct; an edit risks accidentally flipping a kind's severity. (PITFALLS Technical-Debt row.)
- **Any `fmt.Print`/log to stdout in JSON mode** — the agent contract is exactly one JSON object on stdout (success and `fail` paths). The human marker/tally is human-mode only; it must never leak into the JSON path. (PITFALLS #7.)

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| enum → wire string | a `map[Severity]string` in render, or inline string literals at each call site | a `Severity.String()` method on the model type | One source of truth; matches `Category.Description()`; keeps dto string-only and both renderers consistent |
| "are there actionable issues?" computed in two places | duplicate `for` loops in `ExitCode()` and in `toDTO` | one `Analysis.HasActionableIssues()` predicate called by both (and the human tally) | They can never disagree; this is the whole point of SEV-02's consistency requirement |
| JSON marshaling of the envelope | manual string building | the existing `json.MarshalIndent` in `JSONRenderer.Render` (unchanged) | Already correct; just add the field to the DTO |

**Key insight:** This phase has essentially nothing to hand-roll — it is wiring an existing pattern (typed enum + model string-method + manual dto mapping) into one more field. The discipline is *reuse the existing seams*, not invent new ones.

## Runtime State Inventory

> Not a rename/refactor/migration phase in the runtime-state sense — but the milestone *does* call SEV-02 an accepted **wire-contract** change, so the closest analog (what consumers downstream see change) is worth stating explicitly.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — the analyzer is read-only and stateless; no database, no cache, no persisted analysis. (Verified: CLAUDE.md "No `.env`… No global mutable state"; `analyze` reads a file and emits output.) | none |
| Live service config | None — single-binary CLI; no services. | none |
| OS-registered state | None. | none |
| Secrets/env vars | None changed. (`analyze` *detects* secrets in the user's config but stores nothing.) | none |
| Build artifacts | None — no package rename, no egg-info/binary-name change. `go.mod` module name unchanged. | none |
| **Wire-contract consumers (analog)** | Every `analyze --json` issue object gains a `"severity"` field (always present, D-02). `issues_found`/`exit_code` redefined to actionable-only (D-04). **Inert for all existing configs in Phase 2** (every issue is actionable today; the flip only becomes observable when Phase 3 adds the first advisory). | Documented as accepted change (CLAUDE.md, D-02/D-04). No data migration; agents that read `issues[].severity` will simply find it newly present. |

## Common Pitfalls

### Pitfall 1: Severity tier bumps or suppresses the exit code (the load-bearing SEV-02 trap)
**What goes wrong:** `ExitCode()` is `len(a.Issues) > 0` today. If left unchanged after a future advisory is added (Phase 3), an advisory-only file exits 3. The mirror mistake: make the zero value advisory and silently turn genuine duplicates non-actionable.
**Why it happens:** Advisories correctly reuse the `Issue` type, but `ExitCode()` counts *issues*, not *actionable* issues; zero-value handling is easy to get backwards.
**How to avoid:** `SevActionable` is the zero value (Pattern 1); `ExitCode()` and `issues_found` both consult `HasActionableIssues()` (Pattern 3). Add the table test below.
**Warning signs:** an advisory-only `Analysis` returns `ExitActionable`; a duplicate-only `Analysis` returns `ExitClean`; `ExitCode()` still reads `len(a.Issues)`.
**Phase-2 note:** Because no advisory exists yet, the *only* way to exercise the advisory branch in Phase 2 is a hand-constructed `model.Issue{Severity: SevAdvisory}` in a unit test (see Test Plan). The behavior change is real but dormant on real configs until Phase 3.

### Pitfall 2: Forgetting `issues_found` when only fixing `ExitCode()`
**What goes wrong:** `ExitCode()` becomes severity-aware but `json.go:59` still sets `IssuesFound: len(a.Issues) > 0`, producing `issues_found:true / exit_code:0` for an advisory-only file.
**Why it happens:** The two live in different files (model vs render) and are edited separately.
**How to avoid:** Derive both from `HasActionableIssues()` (Pattern 3). Grep for `len(a.Issues) > 0` after the change — it should appear **zero** times (today it's in `analysis.go:26` and `json.go:59`; both get replaced).
**Warning signs:** a grep for `len(a.Issues) > 0` still matches; an advisory-only envelope disagrees between the two fields.

### Pitfall 3: Forgetting to thread the field through dto / breaking the one-JSON-object contract
**What goes wrong:** Adding `Severity` to `model.Issue` but not `dto.Issue` (silent omission from `--json`); or leaking the human marker/tally into JSON mode.
**Why it happens:** The model→dto map is manual; the human/JSON split is implicit.
**How to avoid:** Thread through `toDTO` (Pattern 2). The human glyph/tally is in `human.go` only — `json.go` never prints anything but the marshaled envelope. The existing agent-contract tests (`cli_test.go` `TestRunJSONEmitsOneObject`, `TestRunMissingFileJSONErrorOnStdout`) already pin "exactly one JSON object on stdout" and still pass (they decode into `dto.Envelope`, which tolerates the new field).
**Warning signs:** `severity` present in the human report but absent from `--json`; `json.Decode` finds trailing data.

### Pitfall 4: Golden byte-stability — **NOT a risk in this phase** (the key finding)
**What the milestone research warned:** "adding `severity` to every issue changes those bytes → update golden expectations." This is the headline risk the additional-context question #4 asks to scope.
**What is actually true here:** The golden corpus test asserts **per-field against the `model.Analysis` struct, not a JSON snapshot.** `[VERIFIED: core/analyze/corpus_test.go read]` It checks: `BlockCount >= MinBlocks`, `HasSecrets`, optional `Lines` (pointer), `OpaqueBlocks == 0`, issue **kinds** as set equality, and **categories**. It **never** serializes to JSON, and **never** inspects `Issue.Name`, `Issue.Lines`, `Issue.Note`, or `Issue.Severity`. Therefore **adding a `severity` field changes no bytes or values it observes — there is zero fixture/manifest churn in Phase 2.**
**Corroborating facts:**
- The current fixtures contain **no** PATH or shadowed fixtures and **no** advisory-producing config; every fixture's `issue_kinds` is either `[]` or a single actionable kind. `[VERIFIED: manifests.json read — clean_baseline/installer_junk/secrets_inline/empty have [], duplicate_aliases→duplicate_alias, reassigned_env→reassigned_env]`
- The testgen property test `assertStrict` compares only the `(Kind, Name)` issue-key set and per-issue `Lines` — **not** `Severity`. `[VERIFIED: property_test.go:76-106]` So it stays green; you *may* optionally extend it to compare severity (cheap, locks advisory exit-neutrality) but it is **not required** for Phase 2 and is explicitly a Phase 4 concern (COV-02/03).
- The two tests that *do* assert `exit_code`/`issues_found` (`render_test.go` `TestJSONIsOneObjectWithContract` expecting `exit_code==3, issues_found==true`; `cli_test.go` `TestRunDuplicateExitsThree`/`TestRunJSONEmitsOneObject`) both use a **`duplicate_alias`** issue, which stays `SevActionable`. Under the new actionable-only rule these expectations are **still correct — no edit needed.** `[VERIFIED: render_test.go:11-22, cli_test.go:38-90]`
**How to avoid the (non-)problem:** Do not initialize `a.Issues` to a non-nil empty slice (would flip JSON `null`→`[]`) — but nothing in this phase touches that, since the reconciler/analyzer are not edited. Simply add the field and the predicate; run `go test ./...` to confirm the suite is green.
**Warning signs (would indicate you went out of scope):** any diff to `manifests.json` or a fixture; any edit to `corpus_test.go`; a golden test failing on `null`↔`[]`.

### Pitfall 5: Over-engineering the scale or the marker
**What goes wrong:** Adding `error`/`warning`/`info` "while we're here," or a configurable marker, or a `--severity` filter.
**How to avoid:** D-01 (two values only), Deferred Ideas (richer scale, filtering are out of scope). Two constants, one glyph swap, one tally. Stop there.

## Code Examples

All "code examples" for this phase are the four patterns above (Pattern 1–4), each sourced from a concrete existing file in the repo:
- **Severity type** ← `core/model/block.go:18-24` (`Confidence`+`iota`) + `core/model/category.go:29` (`Category.Description()` model-method-returns-string). `[VERIFIED: codebase read]`
- **dto mapping** ← `core/render/json.go:46-51` (existing `toDTO` issue loop) + `core/dto/analysis.go:20-25` (existing `dto.Issue`). `[VERIFIED]`
- **shared predicate / ExitCode** ← `core/model/analysis.go:25-30` (existing `ExitCode`) + `core/render/json.go:59` (existing `IssuesFound`). `[VERIFIED]`
- **human marker/tally** ← `core/render/human.go:34-48` (existing ISSUES loop, fixed `!`). `[VERIFIED]`

No external/Context7 examples are needed — this phase introduces no library API.

## State of the Art

| Old Approach (current code) | New Approach (this phase) | Why |
|--------------|------------------|--------|
| `ExitCode()` = `len(a.Issues) > 0 → ExitActionable` | `ExitCode()` = `HasActionableIssues() → ExitActionable` | SEV-02: advisories must not bump exit code |
| `IssuesFound: len(a.Issues) > 0` (json.go) | `IssuesFound: a.HasActionableIssues()` | SEV-02: keep `issues_found` consistent with `exit_code` |
| `dto.Issue` has no severity | `dto.Issue.Severity string` (`json:"severity"`, non-omitempty) | SEV-01 / D-02: self-describing severity on every issue |
| human ISSUES line: fixed `!` glyph | glyph by severity (`!`/`~`) + advisory tally | SEV-01 / D-05 / D-06 |

**Deprecated/outdated:** nothing is removed or renamed. `pathSegRe` and the reconciler PATH logic are a **Phase 3** concern — explicitly not touched here.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The exact advisory marker char (`~`) and the precise summary wording ("N issues, M advisories") are Claude's discretion. | Pattern 4 / D-05–D-06 | None — CONTEXT.md explicitly marks these discretionary; any visually + numerically distinct choice satisfies D-05/D-06. |
| A2 | A named `HasActionableIssues()` predicate is preferable to reusing `ExitCode() == ExitActionable` for `issues_found`. | Pattern 3 / Alternatives | Low — both are correct; CONTEXT.md marks the helper choice discretionary. If the planner prefers reusing `ExitCode()`, that is equally valid as long as one source is used in all three places. |

**Note:** Both assumptions are explicitly delegated to Claude's discretion by CONTEXT.md, so neither needs user confirmation before execution. Every other claim in this research is `[VERIFIED: codebase read]` against the live source tree this session. The table is intentionally minimal because the milestone's locked decisions (D-01..D-06) already resolved the design space.

## Open Questions

None blocking. The design space is fully constrained by D-01..D-06 and verified against the source. Two micro-decisions are explicitly discretionary (A1, A2 above) and do not need resolution before planning. The Phase-3/Phase-4 open questions in STATE.md (extraction location, intra-statement duplicates, oracle two-field split) are **not** Phase 2 concerns — Phase 2 produces no advisory and touches no reconciler/oracle logic.

## Environment Availability

> Skipped — this phase is a pure code change in an existing Go module. The only toolchain requirement is the already-present Go 1.25.x. `[VERIFIED: go version → go1.25.7 darwin/arm64; go.mod declares go 1.25.0]` No external tool, service, runtime, or package is introduced or required. `golangci-lint` v2 is a dev-only check (run via `make lint`/`make check`) and is already part of the project's workflow per CLAUDE.md.

## Validation Architecture

> `workflow.nyquist_validation` is `false` in `.planning/config.json`, so the formal Validation Architecture section is **not required**. The user optionally asked for testable invariants; the concrete test plan below serves that purpose and can seed a VALIDATION.md later if desired.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (Go 1.25.7); no external test runner. `[VERIFIED: go.mod, *_test.go files]` |
| Config file | none — standard `go test ./...` |
| Quick run command | `go test ./core/model/... ./core/render/...` |
| Full suite command | `go test ./...` (and `make check` = fmt-check + vet + lint + test) |

### Test Plan (specific tests to add/update for Phase 2)

These map directly to additional-context question #5. **Net new tests are small and localized; no existing test needs editing** (all existing exit-code/JSON tests use actionable issues and stay correct).

| Test | Where | Asserts | Requirement |
|------|-------|---------|-------------|
| `TestExitCodeAdvisoryOnlyIsClean` | `core/model/` (new `analysis_test.go` or extend existing) | `Analysis{Issues: []Issue{{Kind: IssueDuplicatePath, Severity: SevAdvisory}}}.ExitCode() == ExitClean` | SEV-02 |
| `TestExitCodeActionableIsThree` | `core/model/` | `Analysis{Issues: []Issue{{...}}}.ExitCode() == ExitActionable` (zero-value Severity ⇒ actionable) | SEV-01/02 |
| `TestExitCodeMixedIsThree` | `core/model/` | one actionable + one advisory ⇒ `ExitActionable` | SEV-02 |
| `TestExitCodeCleanIsClean` | `core/model/` | no issues ⇒ `ExitClean` | SEV-02 |
| `TestHasActionableIssues` | `core/model/` | predicate true iff ≥1 actionable; false for advisory-only and empty | SEV-02 |
| `TestSeverityString` | `core/model/` | `SevActionable.String()=="actionable"`, `SevAdvisory.String()=="advisory"`, zero-value ⇒ `"actionable"` | SEV-01 |
| `TestJSONIssueHasSeverityOnEveryIssue` | `core/render/` (extend `render_test.go` or new) | rendered JSON: every `issues[].severity` present and non-empty; an actionable issue ⇒ `"actionable"`; a hand-set advisory ⇒ `"advisory"` | SEV-01/D-02 |
| `TestJSONAdvisoryOnlyEnvelope` | `core/render/` | advisory-only `Analysis` ⇒ envelope `issues_found:false`, `exit_code:0`, and the issue carries `severity:"advisory"` | SEV-02/D-04 |
| `TestHumanAdvisoryMarkerAndTally` | `core/render/` | human output uses the advisory marker (`~`) for an advisory and the actionable marker (`!`) for an actionable; the summary shows the advisory count distinctly | D-05/D-06 |
| (agent contract — already exists, **re-run only**) | `core/cli/` | `TestRunJSONEmitsOneObject` + `TestRunMissingFileJSONErrorOnStdout` still pass (one JSON object, clean stderr) with the new field | D-02 invariant |

### Invariants worth pinning (for an eventual VALIDATION.md)
- **INV-SEV-1:** `Issue{}` with `Severity` omitted ⇒ `SevActionable` (zero-value contract). *(Compile-time + `TestExitCodeActionableIsThree`.)*
- **INV-SEV-2:** `ExitCode()` and `IssuesFound` agree on every `Analysis` (both derive from `HasActionableIssues()`). *(Property: for any issue set, `(ExitCode()==ExitActionable) == HasActionableIssues()`.)*
- **INV-SEV-3:** Advisory-only ⇒ `exit_code:0 ∧ issues_found:false`. Actionable-present ⇒ `exit_code:3 ∧ issues_found:true`.
- **INV-SEV-4:** `--json` emits exactly one object; `issues[].severity` present on every issue. *(Already pinned by `cli_test.go`; severity-presence extends it.)*
- **INV-SEV-5 (regression pin, unchanged):** the testgen 10-seed property test stays green with no severity comparison — proving the four existing kinds' `(Kind,Name,Lines)` behavior is byte-stable.

### Sampling / gates
- **Per task commit:** `go test ./core/model/... ./core/render/...`
- **Per phase gate:** `go test ./...` green, then `make check` (fmt + vet + lint + test) green before `/gsd:verify-work`.

## Security Domain

> `security_enforcement` is not present in `.planning/config.json`'s `features` map (it is `{}`), and the milestone's `security_enforcement` is not set — treat as the default. This phase introduces **no** new input parsing, no auth, no crypto, no session, no network, and no file writes. It is a pure additive enum + presentation change over data the engine already produced.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | n/a — no auth surface |
| V3 Session Management | no | n/a — stateless CLI |
| V4 Access Control | no | n/a — read-only local file analysis |
| V5 Input Validation | no (for this phase) | The severity field is set internally (enum), never parsed from untrusted input. Input parsing (the zsh AST) is unchanged and is a Phase 3 concern. |
| V6 Cryptography | no | n/a — never hand-rolls crypto; none involved |
| V7/V8/V9+ (errors, data protection, comms) | no | n/a |

### Known Threat Patterns for this phase's surface

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Misleading exit signal (advisory treated as actionable, or vice-versa) corrupts an agent's downstream decision | Tampering / Repudiation (signal integrity) | The single `HasActionableIssues()` predicate + the SEV-02 table tests guarantee the exit/`issues_found` signal exactly reflects actionable issues — this *is* the security-relevant property of the phase (a trustworthy agent contract). |
| Stray output breaks the one-JSON-object agent contract | Tampering (output stream) | Human marker/tally is human-mode only; JSON path emits exactly one marshaled envelope; existing `cli_test.go` contract tests re-run as a guard. |

> Note: the genuinely security-flavored work of this *milestone* (CWE-427 cwd-in-PATH advisory) is **Phase 3**, not here. Phase 2 only builds the severity mechanism that Phase 3's advisory will use.

## Sources

### Primary (HIGH confidence)
- **Live source tree, read this session** — `core/model/{issue.go, analysis.go, block.go, category.go, exitcode.go}`, `core/dto/{analysis.go, envelope.go}`, `core/render/{json.go, human.go, render_test.go}`, `core/analyze/{reconciler.go, analyzer.go, corpus_test.go}`, `core/cli/{cli.go, cli_test.go}`, `core/testgen/{oracle.go, property_test.go, oracle_test.go}`, `core/testdata/fixtures/manifests.json`. Every `[VERIFIED: codebase read]` tag refers to these.
- **`go version`** → `go1.25.7 darwin/arm64`; **`go.mod`** → module `zsh-pro`, `go 1.25.0`, single `require mvdan.cc/sh/v3 v3.13.1`.
- **`go list -deps ./core/testgen`** → `zsh-pro/core/model` + `zsh-pro/core/testgen` only (confirms the model-only boundary; relevant to Phase 4, not edited here).
- **`.planning/phases/02-issue-severity-tier/02-CONTEXT.md`** — locked decisions D-01..D-06 (authoritative).
- **`.planning/REQUIREMENTS.md`** — SEV-01, SEV-02.
- **`./CLAUDE.md`** — project constraints (no new deps; layering; conventions; TDD).

### Secondary (MEDIUM confidence)
- **`.planning/research/{SUMMARY.md, ARCHITECTURE.md, PITFALLS.md}`** — milestone-level integration map and pitfall catalogue. Used to cross-check the per-file change set and the exit-code/`issues_found` traps; all of their Phase-2-relevant claims were independently re-verified against the live source this session (so effectively HIGH for the parts cited).

### Tertiary (LOW confidence)
- None. This phase required no web/Context7 lookup; there is no external library behavior in question.

## Metadata

**Confidence breakdown:**
- Standard stack (Severity type idiom): HIGH — two concrete in-repo precedents (`Confidence`+`iota`, `Category.Description()`) directly establish the recommended shape; verified by reading the files.
- Architecture (which sites change / don't): HIGH — every construction site and every exit-code/`issues_found`/JSON site was read and enumerated; zero-value invariant confirmed against the actual `Issue{}` literals.
- Golden byte-stability (the key risk): HIGH — corpus runner read in full; confirmed it asserts struct fields (kinds/categories/blocks/lines), never JSON bytes or severity, so the phase has no fixture churn.
- Pitfalls / test plan: HIGH — derived from the read source and the milestone PITFALLS, mapped to specific files and assertions.

**Research date:** 2026-06-24
**Valid until:** ~30 days (stable — internal Go refactor, no fast-moving external dependency; only invalidated if the source files change materially before planning).
