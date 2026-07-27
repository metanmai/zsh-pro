---
phase: 04-manifest-builder-emit
reviewed: 2026-07-27T11:20:00Z
depth: standard
files_reviewed: 9
files_reviewed_list:
  - core/activate/builder.go
  - core/model/profile.go
  - core/ir/regen.go
  - core/activate/builder_test.go
  - core/model/profile_test.go
  - core/ir/regen_test.go
  - core/store/dto_test.go
  - core/shell/zsh/pipeline_test.go
  - core/shell/zsh/residue_test.go
findings:
  critical: 2
  warning: 1
  info: 0
  total: 3
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-07-27T11:20:00Z
**Depth:** standard
**Files Reviewed:** 9
**Status:** issues_found

## Summary

The 04-14 reducer fixes the prior duplicate-delta loss for the static PATH/FPATH fixtures, and the four targeted declaration forms now remain verbatim and operation-free after DTO persistence. However, two persisted-profile paths still change shell semantics: canonicalizing a legacy dynamic list freezes its expansion, and a forced-managed append assignment is still lowered as an overwrite. The passing full suite, vet, build, and `make check` do not exercise either path.

## Critical Issues

### CR-01 [BLOCKER]: Legacy dynamic PATH/FPATH additions are frozen as literals

**File:** `core/activate/builder.go:190-208,291-297`

**Issue:** `pathDelta` accepts a legacy addition such as `$HOME/bin` (it is not rejected by `unsafeStatic`), but always creates `AdditionDynamic: []bool{false...}`. `composeLegacyList` then copies every addition into a token with `dynamic=false`. The new final delta has complete metadata, so `renderListDelta` trusts it and single-quotes that addition instead of using the former legacy heuristic. A stored legacy `PATH=$PATH:$HOME/bin` therefore emits `PATH=$ZP_BASE_PATH:'$HOME/bin'` and creates a literal `$HOME/bin`, violating late binding and changing the activated shell.

**Fix:** Preserve per-addition dynamic provenance while translating a legacy delta. Either add a shell-agnostic, strict compatibility classifier for only the legacy expansions the emitter previously accepted, or retain an explicit legacy-segment marker that the emitter resolves using its existing safe dynamic rule. Add DTO-to-`zsh -f` coverage for dynamic legacy PATH and FPATH, including a mixed semantic/legacy profile, with a different runtime `HOME`/parameter value.

### CR-02 [BLOCKER]: Persisted forced append assignments are still regenerated and activated as overwrites

**File:** `core/model/profile.go:124-138`, `core/ir/regen.go:27-30`, `core/activate/builder.go:35-63`

**Issue:** `DeclarationRepresentable` rejects only four declaration command names. The parse-time `Block.Append` flag is intentionally used by the router to keep `FOO+=bar` out of the templated path, but it is not present on `Entry` (nor copied by `ir.Build`). Once a persisted entry is marked `OverrideManaged`, it is considered representable. `ir.Regenerate` calls the zsh regenerator, which emits `FOO=bar`, and `Build` emits a `SetScalar{FOO, "bar"}`. Thus a persisted `export FOO+=bar` silently replaces the current value rather than appending, defeating the stated persisted representability boundary and mutating live shell state incorrectly.

**Fix:** Persist the structural fidelity markers needed after routing (at least `Append`; audit `Array` and alias flags too), replace the declaration-only predicate with one shared representability predicate, and require it in both regeneration and manifest construction. Until those markers are represented, forced-managed append entries must regenerate from `Text` and yield no manifest operation. Add a parse -> DTO round trip -> forced override -> regeneration/build/live-zsh test for `FOO+=bar` and `export PATH+=:/x`.

## Warnings

### WR-01 [WARNING]: New end-to-end cases omit both regression inputs

**File:** `core/shell/zsh/pipeline_test.go:245-364`

**Issue:** The legacy profile factory only covers static additions, and the persisted override table contains only the four declaration commands. Consequently the new pipeline tests pass while missing the dynamic legacy provenance loss and forced append override bypass above.

**Fix:** Extend the table with the inputs described in CR-01 and CR-02, assert the generated `AdditionDynamic` flags, and source the emitted code with runtime values that distinguish a real expansion from a quoted literal and an append from an overwrite.

---

_Reviewed: 2026-07-27T11:20:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
