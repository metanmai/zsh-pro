---
phase: 04-manifest-builder-emit
reviewed: 2026-07-27T08:33:12Z
depth: standard
files_reviewed: 31
files_reviewed_list:
  - core/activate/builder.go
  - core/activate/builder_test.go
  - core/activate/diff.go
  - core/activate/diff_test.go
  - core/activate/plan.go
  - core/activate/schema_test.go
  - core/activate/tokenfree_test.go
  - core/cmd/zsh-pro/main.go
  - core/ir/build.go
  - core/ir/build_test.go
  - core/model/block.go
  - core/model/identityset.go
  - core/model/manifest.go
  - core/model/manifest_test.go
  - core/model/profile.go
  - core/shell/provider.go
  - core/shell/zsh/dynamic_test.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/shell/zsh/introspect.go
  - core/shell/zsh/introspect_test.go
  - core/shell/zsh/invariant_test.go
  - core/shell/zsh/parse.go
  - core/shell/zsh/parse_test.go
  - core/shell/zsh/pipeline_test.go
  - core/shell/zsh/residue_test.go
  - core/shell/zsh/zsh.go
  - core/store/dto.go
  - core/store/dto_test.go
  - core/store/secret.go
  - core/store/secret_test.go
findings:
  critical: 3
  warning: 2
  info: 0
  total: 5
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-07-27T08:33:12Z
**Depth:** standard
**Files Reviewed:** 31
**Status:** issues_found

## Summary

`GOTOOLCHAIN=auto go test -count=1 ./...` passes, but the implementation still admits source forms it cannot represent faithfully and then either changes their semantics or drops them. The three blockers below prevent the manifest/emit path from being reversible for accepted zsh input.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01 [BLOCKER]: PATH fallback re-admits rejected and cross-list expressions

**File:** `core/activate/builder.go:77-80,209-248`
**Issue:** When `parse.go` deliberately leaves `Entry.ListValue` nil, `Build` falls back to splitting raw source spelling. The fallback's `base` set accepts both PATH and FPATH references for either target list. Consequently `PATH=$FPATH:/a` is rejected by the semantic parser (`parse_test.go:360-374`) but is re-admitted here as a PATH delta based on `ZP_BASE_PATH`, changing the source's meaning. The same fallback runs for unsupported list syntax (the branch does not check `ValueMode`), so syntax which the semantic parser cannot model can be emitted as a malformed or altered list assignment rather than safely excluded.

**Fix:** Restrict `pathDelta` to explicitly legacy entries only, and require the sole base marker to be the canonical form of the list currently being built. For parsed entries, build only from a valid `ListValue`; otherwise leave the path entry unmanaged/no-part. Add end-to-end tests showing `PATH=$FPATH:/a` and unsupported parameter forms produce no manifest list operation, while valid PATH and FPATH self-references retain their direct zsh behavior.

### CR-02 [BLOCKER]: Attribute-bearing declarations are managed after their attributes are discarded

**File:** `core/shell/zsh/parse.go:133-163`
**Issue:** `typeset`, `declare`, `local`, and `readonly` are parsed as ordinary assignments, but their declaration flags and attributes are not stored in `model.Block`/`model.Entry`. The routing gate nevertheless treats every single-name `KindAssignment` in an environment/path/secret category as managed, and `activate.Build` reduces it to a scalar that `emit.go` applies with plain `typeset -g` or `export`. For example, `typeset -i COUNT=2` loses its integer attribute and `readonly TOKEN=value` is either silently made mutable in a fresh activation shell or fails when an existing readonly value is assigned. That is not behavior-equivalent or reversible.

**Fix:** Admit only plain assignments and `export NAME=value` until declaration attributes are modeled. Record a declaration/flag marker in `Block` and have `routeManaged` route `typeset`, `declare`, `local`, and `readonly` forms (including their flags) through the imperative/verbatim path. Add parser-to-manifest tests for integer, readonly, tied, and local declarations.

### CR-03 [BLOCKER]: Multi-name function declarations are accepted then silently disappear

**File:** `core/shell/zsh/parse.go:211-215` and `core/activate/builder.go:92-106`
**Issue:** The parser explicitly supports zsh's `function one two { ... }` form and records both names. `routeManaged` admits every `KindFuncDecl`, but the manifest builder requires exactly one name and emits no function operation when there are two. Thus an accepted, managed declaration yields an empty manifest contribution and is not applied at all.

**Fix:** Either route multi-name function declarations as imperative until their exact semantics are modeled, or emit one `FuncSet` entry per declared name with the same captured body and verify apply/deactivate for both names. Add a source-to-live-zsh regression test; do not silently skip the entry.

## Warnings

### WR-01 [WARNING]: Invalid plan operations are silently omitted instead of rejected

**File:** `core/shell/zsh/emit.go:166-169,181-184,191-194,199-202,207-210,226-239,251-258,263-277`
**Issue:** The emitter returns `nil` after encountering an invalid environment, alias/function, or option name. Callers therefore receive a syntactically valid loader and no error even though one or more requested operations were not rendered. This hides corrupted/manually persisted manifests and makes a failed activation indistinguishable from success; `TestEmitRejectsHostileNamesWithoutOutput` currently codifies that silent success.

**Fix:** Return a contextual error for every invalid operation/name and make the caller abort activation. Keep the builder's defensive filtering for parser input, but treat the emitter as the final validation boundary. Update the hostile-name test to require an error and verify no partial loader is returned.

### WR-02 [WARNING]: The builder cannot populate the manifest's required profile identity

**File:** `core/activate/builder.go:15-28` and `core/model/profile.go:124-131`
**Issue:** `Manifest.Profile` is part of the persisted wire record, but `model.Profile` contains only entries and `activate.Build` has no profile-name argument. Every production call must manually assign `manifest.Profile` afterward (as the pipeline and residue tests do), so a normal call to `Build` returns a record with an empty profile identity. This is easy to omit when persistence/runtime wiring is added.

**Fix:** Add an explicit validated profile-name parameter to `activate.Build`, or make the profile identity part of `model.Profile` and copy it into the manifest. Test that a normal builder call emits the expected non-empty `profile` JSON field.

---

_Reviewed: 2026-07-27T08:33:12Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
