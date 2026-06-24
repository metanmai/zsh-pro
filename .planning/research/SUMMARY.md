# Project Research Summary

**Project:** zsh-pro
**Domain:** Read-only static analysis of a zsh config file — correct PATH-entry extraction, notation-only canonicalization, rootedness classification, and a new advisory severity tier (Go CLI)
**Researched:** 2026-06-24
**Milestone:** v1.1 "Trustworthy PATH Analysis"
**Confidence:** HIGH

## Executive Summary

This is a focused **correctness milestone on a brownfield, reviewed-clean Go analyzer** — not greenfield work. The four research tracks converge tightly: there is **no new dependency**, **no new package**, and **no new architectural layer**. Every change is an edit inside an existing file (plus new test fixtures), which is the strongest possible signal that the current layering already accommodates the feature. The job is to replace one broken regex (`pathSegRe` in `core/analyze/reconciler.go`) with structural, split-based extraction; add a notation-only canonicalizer for duplicate keying; add one advisory issue kind for relative/unrooted/cwd PATH entries; and introduce the first `Issue` severity tier so that advisory surfaces **without** bumping the exit code. The testgen oracle (10 seeds) is the regression pin that proves it end-to-end.

The recommended approach is unanimous across all four researchers: extract the PATH right-hand side from the **`mvdan.cc/sh` AST** (`*syntax.Assign.Value` rendered with `syntax.Printer.Print`, which does NOT resolve `$HOME`/`~`), then split with `strings.Split(rhs, ":")` (which preserves empty fields — the cwd foot-gun), drop only the `$PATH`/`${PATH}` self-reference token, and classify each entry by leading-token prefix. Canonicalization for dedup is **string/notation-only**: fold bare `~` ≡ `$HOME` ≡ `${HOME}`, normalize slashes — never touch the filesystem, never read `os.Getenv("HOME")`, never call `filepath.Clean`/`Abs`/`EvalSymlinks`. The new advisory is recommended as `relative_path_entry` at `warning` severity (does not affect exit code), with the empty/`.`/colon cwd sub-case carrying a `CWE-427` security reference. This naming and severity is grounded in Lynis, CIS Benchmarks, CWE-427, and Go's own stdlib PATH-security policy — it is not invented.

The dominant risk is **re-creating the original bug class** by reaching for the obvious-but-wrong tool: a smarter regex, `path.Clean`, `filepath.IsAbs`, `os.ExpandEnv`, or `strings.FieldsFunc` (which silently drops the empty/cwd entries that are the headline finding). The second-order risk is **breaking the agent contract** — either by letting advisories bump the exit code (`ExitCode()` currently counts `len(a.Issues)`, not actionable issues), or by leaving `issues_found` inconsistent with `exit_code`. The third risk is **making the testgen oracle circular** by importing the engine's canonicalizer; the oracle's value is that it re-implements canonicalization independently. The recommended phase order — **A (Severity tier) → B (PATH extraction + dedup + relative detector) → C (Oracle + golden coverage)** — sequences these risks so each phase is independently testable and no wrong intermediate state (advisory-only configs exiting 3) ever exists. One critical design note: the oracle's **two-field node split** (rendered notation vs. oracle dedup key) must be designed *before* the engine canonicalizer is written, because retrofitting it is the single hardest recovery in the research.

## Recommended Decisions Already Converged (Adopt as Default)

These were independently reached by multiple researchers and should be carried into requirements/roadmap as defaults unless a phase deliberately overrides them.

| # | Decision | One-line rationale |
|---|----------|--------------------|
| 1 | **Extract via the AST, not regex.** Take `*syntax.Assign.Value` (and `Array.Elems` for `path=(...)`), render with `syntax.Printer.Print`, then `strings.Split(rhs, ":")`. Splitter is `strings.Split` (preserves empty fields); drop only the `$PATH`/`${PATH}` self-reference by exact token match; trim surrounding quotes per entry. | The `pathSegRe` regex anchoring on `/`/`~`/`$HOME` is the documented root cause of BOTH bugs (mis-names `./scripts`, blind to bare-relative/empty); the parser already produced the value, and `Printer.Print` renders `$HOME`/`~` as tokens without resolving — keeping it deterministic. |
| 2 | **Canonicalization is NOTATION-ONLY.** Fold bare `~` ≡ `$HOME` ≡ `${HOME}`; normalize trailing/duplicate slashes (guard the root `/`). Do NOT fold `~user`/`~+`/`~-`/`~N` (named home ≠ `$HOME`). Never touch the filesystem or `os.Getenv`; never `filepath.Clean`/`Abs`/`EvalSymlinks`. | Catches real notational duplicates of the same dir while staying deterministic and read-only-pure; avoids the "`$HOME` reassigned mid-file" false positive and the false-equating of `~root/bin` with `~/bin` (verified locally: `~root`→`/var/root` ≠ `~`→`$HOME`). |
| 3 | **One new advisory issue kind, `relative_path_entry`, severity = advisory/warning** (does NOT bump exit code). The cwd sub-case (`.`, empty field, leading/trailing/double colon) carries a `CWE-427` security reference. | Every authority (Lynis=warning, CIS=L1 "correct or justify", Go=opt-in) treats relative/cwd-in-PATH as a review item that may be intentional; exit-3 conflation would corrupt the agent signal. CWE-427 (not CWE-426) is the correct weakness for cwd/empty/relative. |
| 4 | **Severity field: `SevActionable` is the ZERO value** (existing 4 kinds unchanged); BOTH `Analysis.ExitCode()` AND the `issues_found` flag switch to counting **actionable** issues only; thread the field through both `model.Issue` and `dto.Issue` (manual `toDTO`). | Zero-value-actionable keeps all four existing `Issue{}` literals (reconciler + oracle) byte-identical with zero edits; only the new detector sets `SevAdvisory`. Aligning `issues_found` with `exit_code` prevents a self-contradictory `issues_found:true / exit_code:0` envelope. |
| 5 | **Wire encoding:** severity emitted as a self-describing string (`"actionable"` / `"advisory"`), present on every issue (non-omitempty); duplicate-path `Name` reported **verbatim** (not canonical). | A string is agent-friendlier and stable than an int; always-present lets agents switch unconditionally. Verbatim `Name` keeps human output faithful (`./scripts` reads as `./scripts`) and keeps the existing `analyze_test.go` dup-key assertion green. |
| 6 | **Oracle independence:** each testgen path node carries TWO independent fields — rendered notation (what `render.go` writes) vs. oracle dedup key (the dir's intended identity, set by construction). testgen re-implements canonicalization locally and stays `core/model`-only. | If the oracle called the engine's `canonPathEntry`, the property test becomes a tautology and a real dedup bug is invisible; two independent implementations agreeing is the oracle's entire value. |

## Open Questions (Still Need a Human/Plan Decision)

These are **not** resolved by research and must be decided during requirements/planning. Do not treat them as settled.

1. **Extraction LOCATION — provider vs. reconciler-local split.**
   - **Option A (preferred by STACK/ARCHITECTURE):** the zsh provider (`core/shell/zsh`) populates a structured, agnostic `[]string` on `model.Block` (e.g. `PathEntries`) via the AST recipe during `describe()`. The reconciler then canonicalizes/dedups/classifies that agnostic slice with `strings` only — **keeps `core/analyze` shell-free and keeps the `mvdan.cc/sh` import where it belongs** (honors the layering constraint exactly).
   - **Option B (acceptable fallback):** keep extraction in the reconciler but replace the regex with `strings.Split` over a provider-exposed RHS string. Smaller diff, but leaves PATH-tokenizing (a shell-flavored concern) in the agnostic layer.
   - **Layering implication:** Option A is the only one that fully satisfies "`core/analyze` stays shell-free"; Option B is a mild, knowingly-accepted layering smell. The stdlib choices are identical either way — this is purely a *where does the code live* decision. **PITFALLS adds nuance:** word-reconstruction (walking `*syntax.Word.Parts` to handle `${PATH:+:$PATH}` brace-aware splitting) is zsh-AST-specific and argues for keeping that part behind the shell seam regardless.

2. **Intra-statement duplicates (`PATH="/a:/a:$PATH"`).** When one statement lists the same dir twice, is it a `duplicate_path`? Three candidate behaviors, each defensible: (a) **report it** with `Lines` legitimately containing a repeated line number; (b) **dedup the line list** so the repeated line appears once; (c) **ignore** intra-statement repeats and only flag cross-statement duplicates. Must be decided and fixtured (`PATH="/a:/a:$PATH"`). Cross-*statement*/cross-line dedup is settled (accumulate canonical-key → []line across all PATH blocks, preserving current behavior); only the intra-statement case is open.

## Key Findings

### Recommended Stack

No new dependency is warranted, and none is recommended — the entire feature is ~40 lines of `strings` logic over an AST field the project already parses. The "stack" decision is really *which stdlib facilities to use, which to deliberately avoid, and one already-present `mvdan.cc/sh` facility to use for clean extraction.* All findings were verified empirically against the exact pinned toolchain (Go 1.25.7, `mvdan.cc/sh/v3 v3.13.1`), so confidence is HIGH and there is **zero `go.mod`/`go.sum` change** for this milestone.

**Core technologies:**
- **Go stdlib `strings`** (already imported by the reconciler): split RHS on `:` (`strings.Split` — preserves empty fields), canonicalize via `HasPrefix`/`TrimPrefix`/`ReplaceAll`/`TrimRight`, classify rootedness via `HasPrefix(e, "/")` — zero hidden path semantics, exactly the "notation-only, read-only-pure" contract the milestone demands.
- **`mvdan.cc/sh/v3/syntax`** (already the project's only external dep): `*syntax.Assign.Value` gives the RHS as a `*syntax.Word`; `syntax.Printer.Print` renders it back to source **without** parameter expansion (verified: `${HOME}` prints as `$HOME`, nothing resolved). This deletes the `pathSegRe` regex outright.
- **Deliberately AVOIDED (the obvious-but-wrong tools):** `path.Clean`/`filepath.Clean` (collapse `..`, rewrite `./x`→`x` and `""`→`"."` — destroy the exact signals the advisory must surface); `filepath.IsAbs`/`path.IsAbs` (OS-coupled and blind to `~`/`$HOME`/empty); `os.ExpandEnv` / `mvdan.cc/sh/v3/expand` / `shell.Expand` (resolve against the live env — non-deterministic, read-only-purity violation); `strings.FieldsFunc`/`Fields` (drop empty fields — silently eat the cwd entry); any new third-party "path/shellwords" module.

See **STACK.md** for the full empirical verification and the per-API rationale.

### Expected Features

The ecosystem is bifurcated: **shell linters** (ShellCheck/shfmt) lint *script syntax* and have **no** relative/cwd-PATH check (SC2123 is accidental-clobber, a different problem); **system auditors** (Lynis/CIS) check the *live* root PATH against the *real filesystem*. Nobody does **static, deterministic, per-statement PATH-content hygiene on a config file with trustworthy line numbers and a machine envelope.** That empty intersection is zsh-pro's differentiator, and the milestone's naming/severity deliberately mirror Lynis + CIS + CWE-427 so the output reads as familiar.

**Must have (table stakes):**
- **Correct per-entry extraction** — split on `:`, keep empty fields, drop only the `$PATH` self-reference. (Keystone — everything depends on it.)
- **Duplicate detection across notations** — `~` ≡ `$HOME` ≡ `${HOME}`, slash-normalized; report the *later* occurrence (matches `typeset -U` first-wins). Stays **actionable** (exit 3).
- **Flag current-directory-in-PATH** — `.`, empty field, leading/trailing/double colon (empty field ≡ `.` per CWE-427/POSIX). The highest-value finding.
- **Human + `--json` parity** for the new finding, and **a severity that does not bump exit code**.

**Should have (competitive differentiators):**
- Static config-file PATH hygiene with **no live env/FS** (the whole niche — determinism is the feature).
- **Notation-equivalent dedup as a first-class finding** (`~/bin` and `$HOME/bin` reported as the same duplicate).
- **CWE-427-tagged security reference** on the cwd finding (machine-readable weakness mapping).
- **Trustworthy line attribution** for each PATH finding (inherits the v1.0 guarantee).

**Defer (v1.x / v2+ — explicitly out of scope here):**
- System-audit mode (live FS: dir exists/writable/ownership/mode) — Lynis/CIS territory; non-deterministic.
- PATH ordering / precedence ("entry X shadows entry Y") — separate order-sensitivity milestone.
- Configurable severity / suppression comments; multi-file PATH tracing; `fix`/`doctor` auto-remediation.

**Anti-features (do NOT build):** resolving `~`/`$HOME`/`..`/symlinks against live env or disk; flagging relative entries as *errors* (exit 3); re-implementing ShellCheck SC2123 (FP magnet here); position-based suppression of `.` at end-of-PATH ("slightly safer" still executes attacker code).

See **FEATURES.md** for the full competitor matrix and severity justification.

### Architecture Approach

This is **integration research, not greenfield** — the task is mapping *where each change lands* in the existing single-pass pipeline (`Parse → Classify → Introspect → Reconcile → Render`) and *in what order* to build without violating the documented layering. **Nothing new is created as a package; no new dependency.** The severity field is the only change that cuts through every layer (model → dto → both renderers → exit-code) and must be built as one vertical slice so the wire contract is never half-applied. Everything else stays inside `core/analyze` + `core/model` + render (shell-free) or inside `core/testgen` (`model`-only, verified via `go list -deps`).

**Major components touched (all MODIFY except fixtures):**
1. **`core/model/issue.go` + `analysis.go`** — add `Severity` enum/field + `IssueRelativePath` const + `Severity.String()`; `ExitCode()` counts `SevActionable` only (the load-bearing edit).
2. **`core/analyze/reconciler.go`** — `pathEntries()` helper (split + drop `$PATH`), `canonPathEntry()` (notation-only), `relativePaths()` detector; `duplicatePaths` re-keys on canon; **delete `pathSegRe`**. One-line append in `analyzer.go`.
3. **`core/dto/analysis.go` + `core/render/{json,human}.go`** — `severity` wire field (string), `toDTO` mapping, `issues_found` aligned with actionable, severity glyph in human output.
4. **`core/testgen/{graph,generator,render,oracle,property_test}.go`** — `Node.Relative` field + two-field node split, `relPathDirs` pool, `GenParams.RelativePaths`/`DupRelativePaths`, relative-render branch, `relativePathIssues()` + LOCAL canon for dup keys.
5. **`core/testdata/fixtures/*.zsh` + `manifests.json` + corpus runner** — ADD `duplicate_path.zsh`, `shadowed.zsh`, `relative_path.zsh`; extend `manifest` + runner with `issue_names`/`issue_lines`.

See **ARCHITECTURE.md** for the file-by-file integration table and the exact `--json` wire-contract delta.

### Critical Pitfalls

1. **Re-extracting PATH with another regex over `Block.Text`** (repeating the original bug) — a regex can't know where the value starts/ends, what's quoted, or what `:` separators are; it scoops tokens from comments and the self-reference. *Avoid:* extract from structure (`*syntax.Assign`), not text.
2. **Splitting on `:` inside a parameter expansion** (`${PATH:+:$PATH}`, `${VAR:-/a:/b}`) — naive `strings.Split` shreds the `:` inside `${...}` into garbage entries. *Avoid:* split on parsed `Word.Parts` (each `*ParamExp`/`*DblQuoted` is opaque), or use a brace/quote-depth-0 scanner. **This is the single most likely correctness regression.**
3. **Resolving the filesystem or live `$HOME` during canonicalization** — `filepath.Clean`/`Abs`/`EvalSymlinks`/`os.Getenv` make output machine-dependent and violate the read-only/deterministic constraint. *Avoid:* pure string notation-fold; add a guard test asserting byte-identical JSON across two `$HOME` values; grep-forbid `os`/`filepath.Abs`/`EvalSymlinks` in `core/analyze`.
4. **Canonicalizing a named tilde `~user` as if it were `~`** — `~root`/`~+`/`~-`/`~N` are NOT `$HOME` (verified: `~root`→`/var/root`). *Avoid:* fold only a **bare** leading `~` (followed by `/` or end); keep named-tilde verbatim and opaque.
5. **Mis-classifying empty entries (the cwd foot-gun)** — leading/trailing/double colon = current directory = the headline advisory; `strings.Fields`/`FieldsFunc`/zsh `(s.:.)` all silently drop empties. *Avoid:* `strings.Split` (preserves empties); drop the self-reference *before* counting empties.
6. **Severity tier bumping (or suppressing) the exit code** — `ExitCode()` counts `len(a.Issues)` today; appending an advisory would exit 3. The mirror mistake: defaulting un-set severity to advisory and silently making genuine duplicates non-actionable. *Avoid:* `SevActionable` as zero value; `ExitCode()` filters by severity; table-test advisory-only=0 / mixed=3 / dup-only=3.
7. **Breaking the "exactly one JSON object on stdout" contract / forgetting DTO threading** — stray prints in JSON mode, or adding `severity` to `model.Issue` but not `dto.Issue`. *Avoid:* thread through `toDTO`; extend the agent-contract test (parse stdout, assert one object) for clean/dup-only/advisory-only/`fail` paths.
8. **Making the testgen oracle circular** — reusing the engine's `canonPathEntry` turns the property test into a tautology. *Avoid:* re-implement the notation-fold locally; two-field node (rendered notation + oracle key); grep-forbid `core/testgen` importing `core/analyze`/`core/shell`. **Design the two-field split before writing the canonicalizer — retrofit is HIGH recovery cost.**

Moderate pitfalls to carry forward: quoted/array/`+=` forms (`path=(...)`, `path+=(...)`, `typeset -U path`) parsed inconsistently (#9); self-reference over/under-dropping (`$PATH_BACKUP` must survive; never substring-replace) (#10); slash/trailing/root over-collapse (`/` must stay `/`) (#11); `CatPath` classifier *under*-capture of the array form starving the reconciler (#12 — a seam check, not a classifier rewrite, since over-capture is out of scope); cross-line dedup + correct line attribution (#13, ties to the intra-statement open question). See **PITFALLS.md** for the full Critical → Minor ordering, the "Looks Done But Isn't" checklist, and recovery costs.

## Implications for Roadmap

Based on combined research, the suggested phase structure is **A → B → C (strict dependency order)** — matching ARCHITECTURE's recommended build order. The order is driven by dependency direction (leaves first) and by keeping each phase independently testable and free of wrong intermediate states.

### Phase A: Severity tier (vertical slice through every layer)
**Rationale:** Introduces the `Severity` type that B and C reference. With `SevActionable` as the zero value, this phase leaves all FOUR existing kinds' exit-3 behavior byte-identical — pure additive, lowest blast radius, no reconciler change yet. Doing B before A would transiently make advisory-only configs exit 3 (a wrong intermediate state) — explicitly rejected.
**Delivers:** `model.Issue.Severity` + enum + `String()`; `Analysis.ExitCode()` counts actionable only; `dto.Issue.severity` (string); `render/json` mapping + `issues_found` aligned; `render/human` severity glyph.
**Addresses (FEATURES):** "a severity that does not bump exit code" (table stakes; gates the advisory).
**Avoids (PITFALLS):** #6 (exit-code bump), #7 (DTO threading / one-JSON-object contract), #14 (nil-vs-empty golden stability).

### Phase B: PATH extraction + dedup + relative detector (engine, shell-free)
**Rationale:** The root of the milestone and the highest-care edit. Depends on A (the `relativePaths` detector sets `SevAdvisory`; the `IssueRelativePath` kind must exist). Stays shell-free — reads `model.Block` only (modulo the open extraction-location question, which may move part of this into the provider).
**Delivers:** `pathEntries()` (split-on-`:` + drop `$PATH`), `canonPathEntry()` (notation-only), `duplicatePaths` re-keyed on canon, `relativePaths()` emitting `IssueRelativePath`/`SevAdvisory`; `pathSegRe` deleted; one-line append in `analyzer.go`.
**Uses (STACK):** `strings.Split`/`HasPrefix`/`TrimPrefix`; AST `Assign.Value` + `Printer.Print` (location per open question #1).
**Implements (ARCHITECTURE):** changes (b) + (c) — the relative-path detector and corrected extraction.
**Avoids (PITFALLS):** #1 (regex re-extraction), #2 (`:`-split inside `${...}`), #3 (FS/`$HOME` resolution), #4 (named tilde), #5 (empty/cwd entries), #9 (array/`+=` forms), #10 (self-reference dropping), #11 (slash/root collapse), #12 (classifier under-capture seam check), #13 (cross-line dedup + attribution).

### Phase C: Oracle + golden coverage (test infra, model-only)
**Rationale:** The regression pin that proves A+B end-to-end. Depends on B (the engine must actually produce the new/corrected issues) and A (model symbols). The milestone's success gate per the TDD constraint.
**Delivers:** `Node.Relative` + `relPathDirs` pool + `GenParams.RelativePaths`/`DupRelativePaths`; relative-entry render branch; `relativePathIssues()` + **LOCAL** canon for dup keys; `property_test` propParams + optional `Severity` compare; golden fixtures (`duplicate_path.zsh`, `shadowed.zsh`, `relative_path.zsh`) + manifest `issue_names`/`issue_lines`.
**Addresses (FEATURES):** "Coverage" table-stakes item (the regression pin; milestone exit criterion).
**Avoids (PITFALLS):** #8 (circular oracle — the load-bearing trap of this phase), #14 (golden byte-stability).
**CRITICAL:** testgen re-implements canonicalization locally and MUST NOT import `core/analyze`. **The oracle's two-field node split (rendered notation vs. dedup key) must be DESIGNED before the Phase B canonicalizer is written** — retrofitting it is the highest recovery cost in the research.

### Phase Ordering Rationale
- **A → B → C is strict.** A is self-contained; B needs A's `Severity`/`IssueRelativePath`; C needs B's runtime behavior to assert against and A's model symbols.
- **A-first avoids a wrong intermediate state:** wiring an advisory into `a.Issues` before `ExitCode()` is severity-aware would make advisory-only configs exit 3.
- **Cross-cutting design note that breaks the linear order:** the oracle's two-field node split (Phase C) must be *designed* before the Phase B canonicalizer, even though it's *implemented* last — otherwise the property test risks circularity. Treat this as a Phase B planning prerequisite.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase B:** the `${PATH:+:$PATH}` / `${VAR:-/a:/b}` brace-aware splitting (Pitfall #2) and the full enumeration of assignment shapes (scalar / array / `+=` / `typeset -U`, Pitfall #9) are the trickiest. Most are already mapped in PITFALLS/STACK with verified behavior, so this is **light** research — mainly resolving the open extraction-location question (#1) and confirming each shape classifies `CatPath` (Pitfall #12 seam check). A `--research-phase` pass is *optional* but reasonable for the brace-depth scanner design.

Phases with standard patterns (skip research-phase):
- **Phase A:** mechanically specified end-to-end in ARCHITECTURE (exact struct shapes, exit-code edit, wire field). Standard vertical-slice; no research needed.
- **Phase C:** the oracle two-field pattern is fully described in ARCHITECTURE + PITFALLS; the only risk (circularity) is a discipline issue, not an unknown. No research needed — but design the node split up front.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every API choice verified empirically against the exact pinned toolchain (Go 1.25.7, `mvdan.cc/sh/v3 v3.13.1`); over-normalization and `IsAbs` blind spots proven by execution. |
| Features | HIGH | Naming + severity grounded in ShellCheck wiki, CWE-427 (4.20), CIS Benchmarks, Lynis source, UPenn CETS, and Go's stdlib security policy — not inferred. |
| Architecture | HIGH | All claims grounded in the live source tree; extraction failures empirically reproduced; the `core/testgen → core/model`-only boundary verified via `go list -deps`. |
| Pitfalls | HIGH | zsh PATH semantics (empty-field=cwd, `~root`≠`$HOME`, `${(s.:.)}` empty-elision, `path`↔`PATH` tie) reproduced locally in zsh 5.9 and cross-checked against the official zsh Expansion manual. |

**Overall confidence:** HIGH

### Gaps to Address

- **Extraction location (provider vs. reconciler)** — *open question #1.* Decide during Phase B planning; Option A (provider populates agnostic `[]string`) is preferred for the layering constraint, but word-reconstruction for brace-aware splitting is zsh-AST-specific and may need to stay behind the shell seam regardless. The stdlib logic is identical either way, so this can be decided late without rework risk to the algorithm.
- **Intra-statement duplicates (`PATH="/a:/a:$PATH"`)** — *open question #2.* No research-derived "right" answer; pick one of {report with repeated line / dedup line list / ignore}, document it in PROJECT.md as a contract decision, and fixture it. Resolve during Phase B.
- **`issues_found` semantics** — research *recommends* aligning with actionable-only (decision #4), but flags it as a deliberate wire-contract change the user must confirm (an advisory-only envelope flips from `issues_found:true/exit_code:0` to `false/0`). Confirm with the user before Phase A ships.
- **`CatPath` array-form classification** — Pitfall #12 is a *seam check*, not a known failure: verify (don't assume) that `path=(...)`/`path+=(...)`/`typeset -U path` classify `CatPath` before relying on the reconciler. If any miss, the minimal in-scope fix is recognizing the path-family lowercase names — without touching the out-of-scope over-capture overhaul.

## Sources

### Primary (HIGH confidence)
- **Empirical probes** (Go 1.25.7, `mvdan.cc/sh/v3 v3.13.1`) — RHS extraction via `Assign.Value` + `Printer.Print` (no resolution), `strings.Split` empty-field semantics, `path.Clean`/`filepath.Clean` over-normalization, `IsAbs` blind spots. (STACK.md)
- **Live source tree** — `core/analyze/{reconciler,analyzer}.go`, `core/model/{issue,block,analysis,exitcode,category}.go`, `core/dto/*`, `core/render/{json,human}.go`, `core/testgen/{graph,generator,render,oracle,property_test}.go`, `core/analyze/{analyze_test,corpus_test}.go`, `core/shell/zsh/{parse,classify}.go`. (ARCHITECTURE.md, PITFALLS.md)
- **`go list -deps ./core/testgen`** — confirms testgen depends only on `zsh-pro/core/model`. (ARCHITECTURE.md)
- **`mvdan.cc/sh/v3@v3.13.1` source** — `syntax/nodes.go` (`Assign{Name,Value,Array}`, `Word`, `WordPart`), `syntax/printer.go` (`Printer.Print` accepts `*Word`/`WordPart`). (STACK.md)
- **CWE-427 Uncontrolled Search Path Element (4.20)** — empty PATH element = cwd = untrusted search element; no fixed CVSS (confirms advisory, not error). (FEATURES.md)
- **zsh official Expansion manual** + **local reproduction in zsh 5.9** — bare `~`=`$HOME` vs `~name`=named-directory; `${(s.:.)}` empty-field elision; `path`<->`PATH` tie; `typeset -U` first-wins. (PITFALLS.md)
- **`.planning/PROJECT.md`** — v1.1 milestone scope, Key Decisions, Out-of-Scope.

### Secondary (MEDIUM confidence)
- **Lynis (CISOfy) source + docs** — "relative path in PATH" / "Suspicious location in PATH" as a *warning*; reads live system PATH. (FEATURES.md)
- **CIS Benchmark "Ensure root PATH Integrity" (L1)** — enumerates empty (`::`), trailing colon, cwd (`.`); "correct or justify". (FEATURES.md)
- **Go stdlib security policy ("Command PATH security in Go")** — cwd-in-PATH rejection is opt-in/policy-scoped, not a blanket error. (FEATURES.md)
- **UPenn CETS "What's wrong with having '.' in your $PATH?"** — empty dir name "equivalent" to `.`; end-placement does not eliminate risk. (FEATURES.md)
- **ShellCheck wiki (SC2123, SC2155)** — confirms ShellCheck has *no* relative/cwd-PATH check (SC2123 = accidental clobber). (FEATURES.md)

### Tertiary (LOW confidence)
- **`typeset -U path` community write-ups** — corroborate the dedup-keeps-first-occurrence convention (already cross-checked against local zsh reproduction, so effectively MEDIUM). (FEATURES.md)

---
*Research completed: 2026-06-24*
*Ready for roadmap: yes*
