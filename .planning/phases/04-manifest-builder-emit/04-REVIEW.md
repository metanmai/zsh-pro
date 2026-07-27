---
phase: 04-manifest-builder-emit
reviewed: 2026-07-27T12:00:22Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - core/model/profile.go
  - core/model/profile_test.go
  - core/store/dto.go
  - core/store/dto_test.go
  - core/ir/build.go
  - core/ir/build_test.go
  - core/ir/regen.go
  - core/ir/regen_test.go
  - core/activate/builder.go
  - core/activate/builder_test.go
  - core/shell/zsh/pipeline_test.go
  - core/shell/zsh/residue_test.go
findings:
  critical: 2
  warning: 0
  info: 0
  total: 2
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-07-27T12:00:22Z
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

Plan 04-15 closes the tested append, array-literal, flagged-alias, legacy-dynamic-list, and historical-DTO cases. However, its new structural-fidelity contract still declares two behavior-bearing zsh assignment forms representable without retaining their semantics. Both can enter regeneration and/or manifest emission, so the phase cannot claim faithful managed-state handling or zero-residue reversibility for those valid source forms.

Targeted package tests and `GOTOOLCHAIN=auto go test -count=1 ./...` / `go build ./...` pass, but neither blocker is exercised by the new matrix.

## Narrative Findings (AI reviewer)

The findings below are from direct source and live-zsh semantic tracing.

## Critical Issues

### CR-01: Indexed assignments are treated as ordinary scalar declarations

**File:** `core/model/profile.go:96-102`

**Issue:** The persisted structural-fidelity contract records only `Append`, `Array`, and `Flagged`; it has no marker for `syntax.Assign.Index`. `core/shell/zsh/parse.go:70-94` records a subscripted source such as `FOO[2]=bar` as `KindAssignment`, `Names:[FOO]`, and `Value:"bar"` without setting any existing marker. Consequently `Entry.Representable()` returns true at lines 152-166. The managed regeneration path turns it into `FOO=bar`, and the manifest builder can emit a scalar operation for it.

Those are not equivalent in zsh: under `zsh -f`, `FOO[2]=bar` creates an array (`${(t)FOO} == array`, element 2 is `bar`), whereas `FOO=bar` leaves a scalar. The live property tests do mutate array elements, but their profile fixtures never ingest an indexed assignment, so the bad promotion is invisible.

**Fix:** Add a presence-aware `Indexed` (or a richer assignment-shape enum) to `Block`, `Entry`, `structuralFidelityDTO`, and the IR/store copies. Set it from `a.Index != nil` in every parser assignment loop. Make `Representable()` return false for indexed assignments until the manifest/emitter has a reversible indexed-operation model. Add Parse → DTO re-save → forced-managed → Regenerate/Build → `zsh -f` tests for numeric and associative subscripts.

### CR-02: `export` attribute flags lose zsh variable type semantics

**File:** `core/model/profile.go:131-166`

**Issue:** `DeclarationRepresentable()` rejects only `typeset`, `declare`, `local`, and `readonly`; it treats every `export` assignment as an ordinary scalar. But the parser also does not retain flags passed to `export` (`core/shell/zsh/parse.go:133-163` skips flag words). A valid `export -i FOO=1` is therefore classified as a managed environment assignment with known `Append/Array/Flagged` markers. Regeneration emits `export FOO=1`, while activation emits an ordinary exported scalar, silently discarding the integer attribute even after `OverrideManaged`.

This changes observable behavior: in `zsh -f`, `export -i FOO=1; FOO+=2` yields `3` with `parameters[FOO] == integer-export`; the regenerated/plain-export equivalent yields `12` and `scalar-export`. The Plan 04-15 override matrix covers `typeset -i`, but not the equally valid `export -i` form, so the tests do not detect the loss.

**Fix:** Preserve declaration/export flags as part of the source-shape DTO and reject any flagged assignment from `Representable()` unless the exact attribute semantics are modeled. At minimum, fail closed for `export` invocations with flags other than an explicitly supported no-op terminator. Add a persisted forced-managed `export -i FOO=1` regression that verifies regeneration remains verbatim and Build produces no scalar operation.

---

_Reviewed: 2026-07-27T12:00:22Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
