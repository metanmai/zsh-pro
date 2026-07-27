---
phase: 04-manifest-builder-emit
verified: 2026-07-27T13:33:54Z
status: passed
score: 9/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 7/9
  gaps_closed:
    - "Persisted indexed assignments and declaration/export attributes now retain source-shape provenance and are rejected from lowering when not faithfully representable."
    - "Forced-managed indexed, multi-assignment, alias-query/multi-definition, option-control, and delimiter-list forms now remain verbatim and create no manifest operation."
  gaps_remaining: []
  regressions: []
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Turn a resolved profile into the reversible record the runtime applies, and generate the shell code from the single place zsh syntax may live.

**Verified:** 2026-07-27T13:33:54Z
**Status:** passed
**Re-verification:** Yes — after Plans 04-16 through 04-19

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | A versioned, shell-agnostic manifest is built from a resolved profile, and schema mismatches fail before a plan is emitted. | ✓ VERIFIED | `core/model/manifest.go` supplies the four manifest parts; `core/activate.Build` produces them and `Diff` rejects non-`SchemaV1` manifests. `go test -count=1 ./...` passed. |
| 2 | The activation layer produces a shell-agnostic deactivate-then-activate plan; generated zsh is owned by `core/shell/zsh/emit.go`. | ✓ VERIFIED | `go list -deps zsh-pro/core/activate` contains only `core/model` from this boundary, not `core/shell/zsh`; `core/activate/tokenfree_test.go` guards reverse-token ownership. `Provider.Emit` is the code-generation seam and live tests invoke its emitted functions in the same `zsh -f` process. |
| 3 | Only faithfully representable declarative source enters scalar/list/alias/option manifest operations; a forced override cannot erase source semantics. | ✓ VERIFIED | Parser markers (`Indexed`, `DeclarationFlags`, `AliasAssignment`, `OptionFlags`) copy through `ir.Build` and DTO v3. `Entry.Representable()` requires complete fidelity and rejects indexed, flagged/declaration, multi-assignment, alias query/multi-definition, and unsupported option controls. `TestPipelinePersistedRejectedSourceShapesPersisted` passed through Parse → IR → DTO re-save → forced override → Regenerate → Build → Diff → Emit → `zsh -f`. |
| 4 | Legacy and semantic PATH/FPATH additions retain literal versus late-bound provenance, mixed source order, runtime zero/one/many expansion, and captured-base reversal. | ✓ VERIFIED | `pathDelta`/`composeLegacyList` carry `AdditionDynamic` and `BaseIndex`; `renderListDelta` reconstructs from the captured base. `TestPipelineStoreRoundTripLegacyDynamic` and `TestPipelinePersistedDelimiterListAlias` passed under `zsh -f`, including `export --` PATH/FPATH forms. |
| 5 | Switching reverses managed state without residue: aliases/functions/options are restored, PATH/FPATH do not grow, and state is path-independent. | ✓ VERIFIED | `TestZeroResidueFullStateProperty` ran balanced randomized sequences for non-empty, empty, and unset list bases; it snapshots parameters, aliases, functions, options, PATH and FPATH. `TestResidueRendererMutants` also detected blind append, missing quoting, and base-stripping mutants. |
| 6 | Restore is ownership-aware: a user drift is not clobbered and a profile cannot remove unmanaged/base state merely because it also contains that value. | ✓ VERIFIED | `emit.go` captures applied/original/presence slots and restores only when the live state still matches the applied state; list deactivation rebuilds from its captured base. The full-state property and `TestPipelineRepeatedCollisionSentinelAndExportRestoration` passed in live zsh. |
| 7 | Shadowed alias/function bodies are captured before override and restored after deactivate, including empty/multiline and multi-name functions. | ✓ VERIFIED | NUL-framed body capture is implemented in `introspect.go`; `Diff` emits paired removal/restore operations and the emitter restores captured bodies. `TestIntrospectReadsAliasesAndFunctions`, `TestEmitAppliesAndRestoresShadowAndOption`, `TestEmitDefinesEmptyAndMultilineFunctions`, and `TestPipelineMultiNameFunctionRoundTrip` passed. |
| 8 | The accepted source-shape boundary is complete for the reported forms; unsupported forms remain verbatim and inert instead of being misrepresented. | ✓ VERIFIED | DTO v3 is presence-aware for all structural fields. The persisted live matrix proves supported plain/`export`/`export --`, empty assigned alias, and bare/`--`/`-o` options; it separately proves rejected `+o`/`-m`, flagged alias, arrays, appends, indexed/declaration-flagged and multi-assignment forms emit no manifest intent. |
| 9 | Emitted apply/deactivate code is syntactically valid and executes in the current zsh process, rather than relying on a child-process mutation. | ✓ VERIFIED | Live pipeline and emitter tests run the generated `zp_apply`/`zp_deactivate` functions in the same `zsh -f -c` shell and assert resulting state. Those suites run `zsh -n` on emitted code before execution (for example, `pipeline_test.go:74-77` and `emit_test.go:38-41`). The user-facing sourced loader itself is intentionally Phase 5 scope. |

**Score:** 9/9 truths verified (0 present-but-behavior-unverified).

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/model/manifest.go` | Versioned `Manifest` and scalar/list/name/option records | ✓ VERIFIED | Substantive typed model, consumed by `activate.Build` and `activate.Diff`; covered by manifest and schema tests. |
| `core/model/profile.go` | Shared persisted representability gate | ✓ VERIFIED | Complete source-fidelity fields and fail-closed `Representable()` are substantive and used by both regeneration and builder. |
| `core/store/dto.go` | Presence-aware source-fidelity persistence | ✓ VERIFIED | DTO v3 serializes every marker, preserves nil/empty controls, and marks absent/v1/v2/partial/unsupported records unknown. |
| `core/ir/{build,regen}.go` | Parser provenance transfer and verbatim fallback | ✓ VERIFIED | Copies markers without aliasing and avoids lowering rejected entries. |
| `core/activate/{builder,diff,plan}.go` | Manifest construction and shell-free plan | ✓ VERIFIED | Build checks `EffectiveManaged()` plus `Representable()`; Diff validates schema/list invariants and emits ordered operations. |
| `core/shell/zsh/{parse,regen,emit,introspect}.go` | Parser, fallback, sole zsh renderer, body capture | ✓ VERIFIED | Wired through production pipeline tests; `regen.go` provides defense-in-depth verbatim fallback for known incomplete fidelity. |
| `core/shell/zsh/{pipeline,residue}_test.go` | Live source-to-zsh and full-state residue proof | ✓ VERIFIED | Focused named tests and the uncached full suite passed using locally installed `zsh`. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| parser source shape | `Block` → `Entry` → DTO v3 → `Representable()` | all assignment, alias, and option markers | ✓ WIRED | Indexed/declaration/export, alias, option-control, and delimiter markers are present at every layer; partial/historical DTOs fail closed. |
| `Representable()` | `ir.Regenerate` and `activate.Build` | shared fail-closed admission | ✓ WIRED | Both consumers test the same predicate; the zsh provider repeats the known-incomplete rejection at its own seam. |
| legacy PATH/FPATH | list reducer → `ListDelta` → emitted list code | `BaseIndex` + `AdditionDynamic` | ✓ WIRED | Targeted live tests confirm source order, late binding and base restoration. |
| `Diff` plan | `Provider.Emit` | typed operations, not shell strings | ✓ WIRED | Activate has no zsh package dependency; emitted functions pass `zsh -n` then run under `zsh -f`. |
| alias/function identity | introspection bodies → shadow slots → reverse operations | NUL body framing plus plan operations | ✓ WIRED | Live restoration, empty/multiline definitions, and multi-name functions are exercised. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces faithful data | Status |
| --- | --- | --- | --- | --- |
| Structural-fidelity DTO | marker fields and control slices | AST parser → IR copy → JSON v3 → decoded Entry | Yes — every current marker is presence-aware; incomplete history is unknown | ✓ FLOWING |
| PATH/FPATH emission | `Additions`, `AdditionDynamic`, `BaseIndex` | semantic/legacy list reduction → manifest → plan → emitter | Yes — actual runtime `HOME`/`EXTRA` expansion and captured-base reversal asserted | ✓ FLOWING |
| Shadow restoration | alias/function bodies | zsh NUL dump → identity set → runtime capture slots → emitter | Yes — bodies are restored in live zsh | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Persisted source-shape admission, dynamic lists, options, rejected no-ops, residue and renderer mutants | `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run '^(TestPipelineStoreRoundTripLegacyDynamic|TestPipelinePersistedRejectedStructuralNoop|TestPipelinePersistedRejectedSourceShapesPersisted|TestPipelinePersistedOptionSyntaxMatrixOption|TestPipelinePersistedDelimiterListAlias|TestResiduePersistedSourceShapeNoop|TestZeroResidueFullStateProperty|TestResidueRendererMutants)$' -v` | PASS under `zsh -f` | ✓ PASS |
| Alias/function bodies, shadow restore, export collision, and multi-name functions | `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run '^(TestPipelinePreservesValuesAndFunctionsInLiveZsh|TestPipelineRepeatedCollisionSentinelAndExportRestoration|TestPipelineMultiNameFunctionRoundTrip|TestEmitAppliesAndRestoresShadowAndOption|TestEmitDefinesEmptyAndMultilineFunctions|TestIntrospectReadsAliasesAndFunctions|TestEffectiveIdentitySourcePipeline)$' -v` | PASS under `zsh -f` | ✓ PASS |
| Workspace regression suite | `GOTOOLCHAIN=auto go test -count=1 ./...` | PASS; all packages green | ✓ PASS |

### Probe Execution

No Phase 4 probe scripts are declared or present. The applicable executable evidence is the focused source-to-live-zsh pipeline and residue tests above.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| SW-01 | 04-01 through 04-19 | Apply faithfully representable declarative state through a single emitted-zsh boundary. | ✓ SATISFIED | Shell-agnostic Build/Diff plans feed only the zsh emitter; exhaustive persisted admission tests prove supported forms lower and incomplete forms remain verbatim/inert. |
| SW-02 | 04-01 through 04-19 | Reverse managed state without residue, PATH growth, or base/unmanaged-state removal. | ✓ SATISFIED | Live zsh property, drift/collision, shadow, dynamic-list and renderer-mutant tests cover env, aliases, functions, options, PATH and FPATH. |

No Phase 4 requirement is orphaned. Phase 5 owns the user-facing sourced loader/CLI wiring; it is not a deferred defect in this manifest-and-emitter phase.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| Phase-04 source files | — | `TBD` / `FIXME` / `XXX` scan | ✓ CLEAN | No unresolved debt marker in the verified implementation paths. |
| `core/shell/zsh/*_test.go` | — | `zsh not available` skip guards | ℹ️ INFO | The local verifier has zsh, so the behavior tests executed rather than skipped. |

## Gaps Summary

None. The previous persisted source-fidelity gaps are closed in current code and exercised through the live zsh pipeline. No human verification remains for this library/emitter phase.

---

_Verified: 2026-07-27T13:33:54Z_
_Verifier: the agent (gsd-verifier)_
