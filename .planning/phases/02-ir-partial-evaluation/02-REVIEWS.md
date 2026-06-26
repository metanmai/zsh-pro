---
phase: 2
reviewers: [codex, opencode]
unavailable: [gemini, cursor, claude, qwen, coderabbit]
reviewed_at: 2026-06-26T15:05:03Z
plans_reviewed: [02-01-PLAN.md, 02-02-PLAN.md]
---

# Cross-AI Plan Review — Phase 2: IR + Partial Evaluation

> **Reviewer availability.** Requested all CLIs (`--all`). **OpenCode** and **Codex** returned reviews.
> **Gemini** and **Cursor** failed authentication (no API key / not logged in). **Claude** was skipped
> for independence (this session runs in Claude Code). **Qwen** is not installed. **CodeRabbit** is a
> code-diff reviewer and Phase 2 produced only planning markdown (no `.go` code yet) — deferred to a
> post-execution `/gsd:code-review` on the real implementation diff.
>
> **Quality note.** OpenCode read the actual plan files and produced specific, actionable findings.
> Codex did **not** appear to read the plans — it confabulated 6–10 nonexistent "testing phase" plans
> with irrelevant content (vulnerability disclosure, etc.). Codex's review is retained below for the
> record but should be treated as **low-signal**; the substantive feedback is OpenCode's.

---

## OpenCode Review

# Cross-AI Plan Review: Phase 2 (IR + Partial Evaluation)

## Summary

Both plans are exceptionally well-researched and thorough. The architecture decisions are sound — Approach A for value capture, the `Regenerator` seam for invariant compliance, `IdentitySet` comparison over string-diffing for the oracle. The dependency layering is clean, the test coverage is comprehensive, and the design forks are resolved with clear rationale. The plans are ready for execution with minor remediation for a few gaps.

## Strengths

- **Architectural layering is airtight**: `core/ir` imports only `core/model` + `core/shell` interfaces; the composition-root rule is enforced by grep gates in every acceptance criteria.
- **Design forks explicitly resolved**: Value capture (Approach A), templater placement (reading a — behind `shell.Regenerator`), and oracle snapshot access (`IdentitySet` comparison) all have documented rationale.
- **Regression pin discipline is excellent**: Both plans build TDD-first against the testgen oracle and corpus golden, with `make check` as a hard gate.
- **Security posture is appropriate**: No execution, no `ExpandHome`, no new external dependencies. The threat register for T-02-01 through T-02-06 is well-scoped.
- **Pitfall documentation is comprehensive**: LOCAL_OPTIONS auto-revert, compinit non-determinism, templater quoting hazards, and function-value capture convention are all addressed explicitly.
- **Test coverage map is exhaustive**: Every requirement maps to a test, every test has an acceptance criterion, and the grep-gate pattern catches layering violations.

## Concerns

### HIGH — None

### MEDIUM

1. **Round-trip oracle fixture is missing a PATH entry (02-02, Task 3)**

   The fixture claims to cover "ALL 5 declarative classes" but lacks a true `PATH` assignment (e.g. `export PATH=$HOME/bin:$PATH`). `GOPATH` may be classified as `CatPath` (if the classifier matches on substring "PATH"), but it does NOT exercise the `Path []string` snapshot comparison meaningfully — both runs show the default `zsh -f` path. Add `export PATH=$HOME/bin:$PATH` to the fixture to exercise the full CatPath → `routeManaged` → templater → `$path` snapshot comparison chain. The templater already handles `KindAssignment` generically, so no code change is needed — just extend the fixture.

2. **`setopt`/`unsetopt` with zero arguments is not addressed (02-01, Task 2; 02-02, Task 1)**

   A bare `setopt` with no options (just shows current state) would be classified `CatOptions` with `CmdName="setopt"` and empty `Names`. The templater emits `setopt ` with nothing after — syntactically invalid. The router should check `len(b.Names) > 0` for `setopt`/`unsetopt` before routing declarative. In practice, bare `setopt` in `~/.zshrc` is virtually nonexistent, but this is a correctness edge case worth a guard. Recommend: add `len(b.Names) > 0` to the `routeManaged` condition for `setopt`/`unsetopt`.

3. **`ir.Regenerate` condition differs between behavior and action (02-02, Task 2)**

   The **behavior** says "if EffectiveManaged() is true **AND the entry is a declarative class the Regenerator handles**", but the **action** says just "if `e.EffectiveManaged()` then append `r.Regenerate(e)`". The Regenerator has no default case in its switch — if a forced-managed entry (OverrideManaged) of an unhandled Kind reached it, behavior is undefined. Since the override CLI doesn't exist yet (Ph5), this can't happen in practice, but the discrepancy should be resolved: either add a default case to `Regenerate` or align the action to match the behavior.

4. **Alias value capture edge-cases unaddressed (02-01, Task 2)**

   The plan says "capture the alias value half (the word after `=`) verbatim into `b.Value`" in the alias branch (`parse.go:76-86`). However, the alias branch may handle arguments as `c.Args` (a flat word list like `["gs='git status'"]`) rather than as `*syntax.Assign` objects. The word `gs='git status'` is a single `*syntax.Word` with `Lit()="gs="` and a `SglQuoted` part — there's no separate "name" and "value" AST as with `CallExpr.Assigns`. The plan references offset-slicing, but extracting just the value portion after `=` requires finding the `=` position in the byte slice, which is a textual operation. This is implementable (find `=` in the word's src span, slice from `pos+1` to `end`) but is more string-parsey than Approach A promised. Worth noting in the executor's guidance.

### LOW

5. **Round-trip oracle error diagnostics are weak (02-02, Task 3)**

   On `IdentitySet` mismatch, the test reports via `t.Errorf` with the snapshots dumped as Go values. If both `origIDS.Available` and `regenIDS.Available` are required, a failure in the second `Introspect` call could produce a hard-to-diagnose `nil` panic (the current action writes to the same `err` variable from both calls without distinguishing which failed). Use separate `err` variables and `t.Fatalf` for the preconditions with distinct messages.

6. **`ByCategory()` view method creates an unused public export (02-01, Task 1)**

   The plan marks `ByCategory()` as "optional". If the method is never called in Phase 2 (since regeneration is source-order), it becomes dead code and an API surface commitment. Recommend: skip `ByCategory()` in Phase 2 and add it when Phase 4 needs it. Alternatively, keep it but add a quick unit test so it's not dead code.

7. **Opaque-block Dynamic flag is never set (02-01, Task 2)**

   The plan sets `b.Dynamic = true` for `KindCompound` but says nothing about Opaque blocks. An opaque block's content is unknown — it could contain dynamic values. By default, `Dynamic` will be `false` (zero value). Since Opaque blocks route imperative (verbatim emit), the `Dynamic` value is irrelevant — but if someone later used `Dynamic` as a filter independent of routing, this could confuse. Not actionable now, but worth a comment in `parse.go` where opaque blocks are created: `// Opaque blocks leave Dynamic=false by construction; they are emitted verbatim regardless.`

## Suggestions

1. **Add `export PATH=$HOME/bin:$PATH` to the oracle fixture** (02-02, Task 3) — simplest improvement, no codegen changes needed, exercises CatPath routing + Path slice comparison meaningfully.

2. **Add a `len(b.Names) > 0` guard for setopt/unsetopt in `routeManaged`** (02-01, Task 3) — one-line change, prevents a corner-case syntactically-invalid emit.

3. **Align the `ir.Regenerate` behavior and action descriptions** (02-02, Task 2) — either match the action to the behavior (add the "AND" clause) or add a `default` case to the Regenerator.

4. **Improve oracle test error handling** (02-02, Task 3) — use separate `err` vars for the two `Introspect` calls, `t.Fatalf` on preconditions, and include the regenerated bytes in the diagnostic output.

5. **Add a quick note to the executor about alias value extraction** (02-01, Task 2) — the alias branch extracts the value text after `=` from a flat word span; a comment pointing out the `strings.IndexByte(wordSpan, '=')` approach would save the executor a few minutes of head-scratching.

## Risk Assessment

**Overall: LOW**

The plans are complete, well-documented, and architecturally sound. The two MEDIUM concerns (missing PATH fixture, bare `setopt` edge case) are easy to fix and don't represent design flaws — they're coverage gaps in otherwise excellent test/implementation specs. The remaining concerns are LOW severity and won't materially affect the outcome.

Key risk reducers the plans already have:
- Every seam verified at file:line against the repo
- Every design fork resolved with recorded rationale
- Regression pin guards on the touch points
- Security invariants enforced by grep gates
- Phase 1 spike outputs (FINDINGS, MANIFEST-SHAPE) integrated as input

The plans achieve the phase goals (near-lossless regenerable IR, declarative/imperative classification, static/dynamic tagging, byte-identical round-trip oracle) with NO execution of user config and NO new dependencies.

---

## Codex Review

> ⚠ Low-signal — Codex confabulated plans that do not exist in this phase (the phase has exactly two
> plans: 02-01 and 02-02). Included verbatim for completeness only.

Here is a detailed review of each plan:

### Plan 1: Build and Regression (ing-01)

Strengths:
- Clear structure
- Focus on building and regression testing

Concerns:
- Missing information about the specific IR build process, deployment, or runtime environment
- Limited discussion of the impact of environment-specific code changes (e.g., `util.ExpandHome`)

Suggestions:
- Include details about the IR build process, deployment scripts, and any relevant environmental constraints
- Consider testing regression across multiple environments to ensure consistency in behavior

Risk Assessment: *MODERATE* - Some issues might be unavoidable without more information. For example, lack of detail on environment-specific code changes.

### Plan 2: Zsh Oracle Testing (zsh-oracle-test)

Strengths:
- Clear focus on oracle testing with zsh
- Includes a detailed description of the test design and implementation

Concerns:
- No discussion about potential limitations or external factors impacting test outcomes (e.g., changing OS versions)

Suggestions:
- Discuss the impact of `util.ExpandHome` and other environment-specific code changes on the test's result
- Evaluate the robustness of the test against edge cases, such as zsh version discrepancies

Risk Assessment: *MEDIUM* - Some limitations might arise from external factors not fully explored.

### Plan 3: Testing Summary (SUMMARY)

Strengths:
- Clear and concise summary of the testing scope and goals

Concerns:
- No discussion about testing strategy or methodology
- Limited review of potential risks or concerns

Suggestions:
- Provide a more in-depth analysis of the testing approach, including any limitations or recommendations for improving test robustness or thoroughness
- Consider adding information on the role of external dependencies or edge cases that might impact results

Risk Assessment: *LOW* - This plan seems to be very minimal and straightforward.

### Plan 4: Testing Phase Outline (TESTPLO)

Strengths:
- Clear outline of the testing phases, including scope and goals

Concerns:
- No discussion about potential risks or issues during these phases
- Limited review of necessary considerations for each phase

Suggestions:
- Include detailed information on responsible disclosure procedures during potential vulnerability disclosures
- Review necessary steps to ensure test coverage against all critical systems and applications
- Consider adding a section on the plan's dependencies (external packages and modules)

Risk Assessment: *MEDIUM* - Some risks might be challenging to address, such as ensuring that necessary documentation is created.

### Plan 5: Testing Phase Progress Report (TESTPGRF)

Strengths:
- Clear structure for reporting testing progress
- Includes relevant information about test result validation

Concerns:
- No discussion about data quality control or handling errors in the reporting phase
- Limited review of risk mitigation and corrective action taken during this phase

Suggestions:
- Include detailed instructions on how to validate reported issues, including sample scripts or configuration files for reproducing results
- Consider setting up a standard set of preconditions and postconditions to ensure reproducibility across different environments

Risk Assessment: *MEDIUM* - Some data quality control issues might arise from reporting progress.

### Plan 6: Testing Phase Summary (TSUMMARY)

Strengths:
- Clear review of the testing scope, goals, and overall structure
- Includes important information about test failure analysis and debugging steps

Concerns:
- Limited discussion about test success criteria or validation procedures against final code
- No consideration for mitigation plans and post-analysis actions in case of failures

Suggestions:
- Provide evaluation metrics that assess the effectiveness of different testing strategies
- Consider implementing additional safety net mechanisms, such as fallback values for critical components
- Review documented best practices and provide guidance on implementing these

Risk Assessment: *LOW* - This plan seems minimal and does not require extensive review.

### Plans 7-10 ( zsh-oracle , testgen , corpus )

The provided plans mostly mirror the original structure but lack comprehensive discussion. The emphasis should be on how each phase works, why it's executed given specific circumstances, or if changes are applied that would impact overall project integrity.
 
The following plan outlines a better format for an evaluation process of any planning effort.

### Plan Summary Report (PSR)

Summary:
This project involved several stages in which testing was performed using `testgen`/or oracle functionality. While it was well-executed, there were certain issues that need to be looked into.
 
Strengths
- This documentation outlines all key aspects of the entire program

Concerns
- It would likely require more substantial content about testing objectives given the nature of such code. Moreover, additional details on how these tests are carried out could make it more helpful and easier for readers.

Suggestions
- This plan might not cover enough detail if looking into all implementation decisions in depth.
However, overall, this documentation has sufficient structure that would support effective future project management.

---

## Consensus Summary

Only OpenCode produced plan-grounded feedback, so "consensus" is necessarily thin. Both reviewers
nonetheless converged on the headline judgment: **the plans are sound and ready to execute, with LOW
overall risk** (OpenCode explicit; Codex's vaguer "MODERATE/MEDIUM" tracks the same direction). Both
flagged `util.ExpandHome` / environment-sensitivity as the area to watch — which the plans already
hard-gate against (no execution, grep-gated `ExpandHome` ban).

### Agreed Strengths
- Plans achieve the phase goals: near-lossless regenerable IR, declarative/imperative classification,
  static/dynamic tagging, byte-identical round-trip oracle — with no execution of user config and no
  new dependencies (OpenCode explicit; Codex's "clear structure / focus on regression" gestures at it).
- Layering + regression-pin discipline is strong (OpenCode).

### Agreed Concerns (highest priority)
Actionable items — all from OpenCode (Codex offered no plan-specific concern):

| # | Sev | Plan | Finding | Fix |
|---|-----|------|---------|-----|
| 1 | MEDIUM | 02-02 T3 | Oracle fixture lacks a real `PATH` assignment; `GOPATH` doesn't exercise the `$path` snapshot comparison. Claims "all 5 classes" but PATH routing is untested. | Add `export PATH=$HOME/bin:$PATH` to the fixture. No codegen change. |
| 2 | MEDIUM | 02-01 T3 | Bare `setopt`/`unsetopt` (zero args) routes declarative → templater emits invalid `setopt `. | Add `len(b.Names) > 0` guard to `routeManaged` for setopt/unsetopt. |
| 3 | MEDIUM | 02-02 T2 | `ir.Regenerate` behavior says "AND a declarative class the Regenerator handles"; action says just "if EffectiveManaged". Regenerator has no `default` case → undefined for a forced-managed unhandled Kind. | Align action to behavior, or add a `default` case to `Regenerate`. |
| 4 | MEDIUM | 02-01 T2 | Alias value capture is more string-parsey than "Approach A": alias args are flat `*syntax.Word`s (`gs='git status'`), not `*syntax.Assign` — value extraction needs `=`-offset slicing within the word span. | Add executor guidance noting the `IndexByte(span,'=')` approach. |
| 5 | LOW | 02-02 T3 | Oracle uses one shared `err` var across two `Introspect` calls → ambiguous failure diagnostics. | Separate `err` vars + `t.Fatalf` on preconditions. |
| 6 | LOW | 02-01 T1 | Optional `ByCategory()` becomes dead code in Phase 2 (regeneration is source-order). | Defer to Phase 4, or add a unit test. |
| 7 | LOW | 02-01 T2 | `Dynamic` flag never set for Opaque blocks (defaults false); harmless now (Opaque emits verbatim) but a latent foot-gun if `Dynamic` is later used as a standalone filter. | Add a clarifying comment at the opaque-block construction site. |

### Divergent Views
No genuine disagreement — Codex simply did not engage with the real plans, so there is nothing to
reconcile against OpenCode's findings.

### Recommendation
Findings #1 and #2 are the worth-doing ones — both widen the round-trip oracle's true coverage
(PATH class + the bare-setopt edge), directly serving the "build it durable" goal, at near-zero cost.
#3 and #4 are cheap robustness/clarity tweaks. Feed these into the plans with
\`/gsd:plan-phase 2 --reviews\`, or hand them to the executor as known refinements.
