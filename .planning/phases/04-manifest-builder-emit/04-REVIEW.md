---
phase: 04-manifest-builder-emit
reviewed: 2026-07-27T09:45:47Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - core/ir/route.go
  - core/ir/route_test.go
  - core/ir/build_test.go
  - core/shell/zsh/parse_test.go
  - core/activate/builder.go
  - core/activate/builder_test.go
  - core/shell/zsh/pipeline_test.go
findings:
  critical: 2
  warning: 0
  info: 0
  total: 2
critical: 2
warning: 0
info: 0
total: 2
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-07-27T09:45:47Z
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

The new parser-to-live-zsh tests correctly prove that newly parsed cross-list and unsupported list forms are inert, and that a valid two-name function applies and restores both names. The targeted tests, full Go suite, build, vet, and `make check` all pass. However, the builder still loses list additions for a supported legacy profile with repeated or mixed list entries, and persisted forced-managed declarations can still cross the manifest boundary despite the new router guard.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Repeated legacy PATH/FPATH assignments overwrite earlier profile state

**File:** `core/activate/builder.go:77-80`

**Issue:** Each legacy fallback is appended directly to `m.Lists` instead of entering the source-ordered `lists` composition state. Two valid legacy entries such as `PATH=$PATH:/a` followed by `PATH=$PATH:/b` therefore produce two legacy `ListDelta`s. During emission, the first delta captures `ZP_BASE_PATH`; the second sees that slot already exists and rebuilds from the original base, yielding `BASE:/b` and silently dropping `/a`. A legacy entry mixed with a semantic `ListValue` has the same loss/order problem because the semantic result is appended only after the loop. This violates the promised legacy compatibility and makes activation apply a different PATH/FPATH than the stored profile.

**Fix:** Fold a successfully parsed legacy fallback into the same per-canonical-list composition state used by semantic lists, preserving the self-marker position and dynamic/static provenance, then emit one final `ListDelta` per list. Add real-zsh regression coverage for two legacy PATH assignments and for legacy-plus-semantic PATH entries in both source orders.

### CR-02: A persisted forced-managed declaration bypasses the new fail-closed router

**File:** `core/activate/builder.go:35-48`

**Issue:** The router correctly marks `typeset`, `declare`, `local`, and `readonly` imperative, but `Entry.EffectiveManaged()` lets persisted `OverrideManaged` re-admit any rejected entry. `Build` then ignores `CmdName` and turns, for example, `typeset -i COUNT=2` into an ordinary scalar operation. `CmdName` and `Override` are both serialized in `profile.json`, so this is reachable from a stored profile rather than only an in-memory test shape. The integer, readonly, tied, or local semantics are again lost, contradicting the closure requirement that unmodeled declarations remain imperative.

**Fix:** Treat unmodeled declaration commands as non-manifestable at the builder boundary regardless of `EffectiveManaged()` (and have the zsh regenerator fall back to `Text` for those commands as a second guard). Add a Parse → persisted-profile → Build regression with `OverrideManaged` for each declaration class, asserting no scalar/list manifest operation is created and verbatim source remains the regeneration path.

---

_Reviewed: 2026-07-27T09:45:47Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
