---
phase: 04-manifest-builder-emit
reviewed: 2026-07-27T12:49:25Z
depth: deep
files_reviewed: 17
files_reviewed_list:
  - core/model/block.go
  - core/model/profile.go
  - core/model/profile_test.go
  - core/store/dto.go
  - core/store/dto_test.go
  - core/ir/build.go
  - core/ir/regen.go
  - core/ir/route.go
  - core/ir/roundtrip_test.go
  - core/activate/builder.go
  - core/activate/plan.go
  - core/activate/diff.go
  - core/shell/zsh/parse.go
  - core/shell/zsh/regen.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/pipeline_test.go
  - core/shell/zsh/residue_test.go
findings:
  critical: 4
  warning: 0
  info: 0
  total: 4
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-07-27T12:49:25Z
**Depth:** deep
**Files Reviewed:** 17
**Status:** issues_found

## Summary

Plans 04-16 and 04-17 correctly close the earlier indexed-assignment and flagged-`export` failures: those source shapes now persist through DTO v2 and fail closed. The final code still has four loss-of-semantics paths for valid zsh input. They are not caught by the green test suite because its multi-name coverage stays on the auto-imperative route and its delimiter coverage exercises only ordinary environment scalars.

The full uncached Go suite, build, vet, and `make check` pass. That is not sufficient evidence here: each issue crosses the persisted `OverrideManaged`/manifest boundary or the normal managed option path with behavior not covered by those tests.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: A persisted forced override collapses multi-assignment source to one scalar

**File:** `/home/metanmai/Code/zsh-pro/core/model/profile.go:154-171`

**Issue:** `routeManaged` correctly keeps `A=one B=two`, `export A=one B=two`, and `export -- A=one B=two` imperative (`core/ir/route.go:46-66`). A persisted `OverrideManaged` bypasses that route, however, and `Representable()` has no exact-one-name guard. The parser stores all names but only one `Value` slot; on the second assignment `captureWordSemantics` marks the value unsupported while `b.Value` becomes the final value (`core/shell/zsh/parse.go:70-95`, `154-175`). `ir.Regenerate` then calls the zsh regenerator, which writes `Names[0]=Value` (`core/ir/regen.go:24-33`, `core/shell/zsh/regen.go:24-47`). Thus `A=one B=two` persists and regenerates as `A=two`; the export variants become `export A=two`. The activation builder happens to produce no scalar because the value mode is unsupported, but the stored profile has already been corrupted on regeneration.

**Fix:** Make exact-one-name a shared representability requirement for assignments, not only an auto-routing rule. Add a defensive `len(e.Names) != 1` verbatim fallback in `Provider.Regenerate`. Cover Parse -> DTO re-save -> `OverrideManaged` -> Regenerate -> Build -> Diff/Emit with plain, `export`, and `export --` multi-assignment inputs.

### CR-02: Alias queries and forced multi-alias definitions are regenerated as a different alias

**File:** `/home/metanmai/Code/zsh-pro/core/ir/route.go:40-43`

**Issue:** A single-name alias query, `alias ll`, has one parsed name and no assignment value. It is automatically marked managed by line 43; `Representable()` also accepts it (`core/model/profile.go:164-170`). `core/shell/zsh/regen.go:48-52` therefore turns the query into `alias ll=`, which creates an empty alias instead of querying an existing one. Separately, a persisted forced-managed `alias a=one b=two` bypasses the otherwise-safe two-name auto route and becomes `alias a=two`, because the parser's sole `Value` field is overwritten by the last alias word (`core/shell/zsh/parse.go:101-133`).

The live control shows the distinction: `alias zpprobe` only queries the alias, whereas `alias zpprobe=` defines it with an empty body.

**Fix:** Record whether an alias argument contained `=` (or derive admission from a present, supported semantic value), and require exactly one assignment-form alias before either regeneration or manifest lowering. Route alias queries and multi-alias forms verbatim even under `OverrideManaged`; add direct defensive guards to `Provider.Regenerate`. Add persisted pipeline cases for `alias ll`, `alias a=one b=two`, and empty-but-assigned `alias ll=` so the valid empty definition remains supported.

### CR-03: `setopt`/`unsetopt` flags are discarded, reversing or inventing option changes

**File:** `/home/metanmai/Code/zsh-pro/core/shell/zsh/parse.go:177-190`

**Issue:** The option parser drops words beginning with `-`, but treats `+o` and the following option as ordinary names. `activate.Build` subsequently ignores the invalid `+o` name and emits a `SetOption` for the real name using only the command verb (`core/activate/builder.go:120-136`). This changes valid zsh commands: under `zsh -f`, `setopt +o extendedglob` leaves `extendedglob` off, but the manifest emits `setopt extendedglob` and turns it on; `unsetopt +o extendedglob` has the inverse error. Query form `setopt -m extendedglob` also leaves the option unchanged, but parsing drops `-m` and activation sets it.

**Fix:** Persist option invocation syntax/polarity as source fidelity and admit only forms with a modeled `SetOption.Enabled` meaning. The conservative fix is to mark any option-command flag other than explicitly proved equivalent forms (`--` and `-o`) unrepresentable, forcing verbatim/no-operation even with `OverrideManaged`. Add persisted live-zsh pipeline tests for `setopt +o`, `unsetopt +o`, and `setopt -m` with pre-seeded option state.

### CR-04: `export -- PATH` / `FPATH` loses its list-delta contract and silently becomes a no-op

**File:** `/home/metanmai/Code/zsh-pro/core/shell/zsh/parse.go:154-176`

**Issue:** The newly supported delimiter assignment is represented as a declaration word, not a `syntax.Assign` (lines 161-170). The parser captures its name and scalar value but invokes `captureListValue` only with `c.Assigns` (line 176), which is empty for `export -- PATH=$PATH:$EXTRA`. After DTO re-save, the entry remains structurally known and dynamic but has `ListValue:nil`. `activate.Build` accepts the path category, then declines both `composeList` and the legacy fallback because its mode is not legacy (`core/activate/builder.go:66-83`). The source regenerates as the equivalent `export PATH=...`, but its persisted original profile creates no `ListDelta`, so the loader never applies or reverses the requested PATH/FPATH change.

**Fix:** Feed delimiter-form assignment words through the same AST-derived list decoder as ordinary assignments (or fail them closed until that representation exists). Add Parse -> DTO re-save -> Build -> Diff/Emit -> `zsh -f` cases for `export -- PATH=$PATH:$EXTRA` and FPATH, asserting late-bound expansion, source order, and captured-base restoration just as for their non-delimiter controls.

---

_Reviewed: 2026-07-27T12:49:25Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: deep_
