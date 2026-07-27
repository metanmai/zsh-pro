---
phase: 04-manifest-builder-emit
verified: 2026-07-27T12:04:55Z
status: gaps_found
score: 7/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 7/9
  gaps_closed:
    - "A persisted legacy PATH or FPATH addition containing a safe late-bound parameter expands at activation, preserves mixed source order, and deactivates to the captured base."
    - "Historical absent, partial, and unsupported Append/Array/Flagged fidelity records remain unknown after DTO re-save and cannot create manifest operations."
    - "Current forced-managed append, array, flagged-alias, and the existing declaration forms regenerate verbatim and create no manifest operation."
  gaps_remaining:
    - "Indexed assignments are promoted to ordinary scalar entries because the structural-fidelity contract has no Index marker."
    - "export attribute flags are discarded, allowing export -i to regenerate as a plain export."
  regressions: []
gaps:
  - truth: "Only faithfully representable declarative state enters scalar/list manifest operations, and a forced override cannot erase source semantics."
    status: failed
    reason: "Indexed assignments and export attribute flags are valid parsed source shapes but are not retained by Block, Entry, or the versioned DTO. They therefore pass Representable() as ordinary assignments after OverrideManaged."
    artifacts:
      - path: "core/model/block.go"
        issue: "Block tracks Append, Array, and Flagged only; it has no marker for syntax.Assign.Index or declaration/export flags."
      - path: "core/model/profile.go"
        issue: "Representable() rejects only Append, Array, and flagged aliases, so indexed and flagged-export assignments are treated as fully known plain declarations."
      - path: "core/store/dto.go"
        issue: "structuralFidelityDTO version 1 serializes only append, array, and flagged."
      - path: "core/shell/zsh/parse.go"
        issue: "Assignment loops never inspect a.Index; export CallExpr flags are skipped rather than preserved."
    missing:
      - "Persist an indexed-assignment source-shape marker from every parser assignment path through Entry and a presence-aware DTO version; reject it at the shared representability gate until an indexed manifest operation exists."
      - "Persist declaration/export flags (or a conservative declaration-shape marker) and reject flagged export assignments at the shared representability gate unless their exact attributes are modeled."
  - truth: "Every admitted profile applies and deactivates path-independently without residue and without changing source semantics."
    status: failed
    reason: "The manifest pipeline changes numeric indexed assignment into a scalar operation, fails against an existing associative array, and regeneration changes export -i integer arithmetic into plain string concatenation. No live-zsh regression covers these forms."
    artifacts:
      - path: "core/ir/regen.go"
        issue: "EffectiveManaged plus Representable() selects templating for both missing shapes."
      - path: "core/activate/builder.go"
        issue: "The same predicate admits indexed assignment to scalar lowering."
      - path: "core/shell/zsh/pipeline_test.go"
        issue: "The forced-managed matrix covers append, array literal, flagged alias, and declaration commands, but no numeric/associative index or export -i case."
    missing:
      - "Add Parse -> DTO marshal/unmarshal -> OverrideManaged -> Regenerate/Build/Diff/Emit -> zsh -f regressions for FOO[2]=bar and a declared associative MAP[key]=bar. Verify verbatim/no-operation plus unchanged sentinels after apply and deactivate."
      - "Add the same forced-managed pipeline regression for export -i FOO=1, asserting verbatim regeneration, no scalar manifest operation, integer-export type, arithmetic behavior, and no residue."
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Turn a resolved profile into the reversible record the runtime applies, and generate shell code from the single place zsh syntax may live.

**Verified:** 2026-07-27T12:04:55Z
**Status:** gaps_found  
**Re-verification:** Yes — after Plan 04-15

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Manifest v1 is versioned and shell-agnostic, and `Diff` rejects incompatible schemas. | ✓ VERIFIED | `model.Manifest`, `activate.Build`, and `activate.Diff` remain substantive; the full uncached suite passed. |
| 2 | Only faithfully representable declarative state enters scalar/list manifest operations; `OverrideManaged` cannot erase source semantics. | ✗ FAILED | `FOO[2]=bar` traverses Parse -> DTO -> OverrideManaged as `Known:true, Append:false, Array:false, Flagged:false`, so `Representable()` admits it and Build produces `SetScalar(FOO, bar)`. `export -i FOO=1` loses `-i` and regenerates as `export FOO=1`. |
| 3 | `core/activate` emits a shell-agnostic deactivate-then-activate plan; zsh rendering remains in `core/shell/zsh/emit.go`. | ✓ VERIFIED | Production `core/activate` imports only `core/model`; its plan is consumed by the zsh emitter. The active/deactivate plan and renderer tests pass. |
| 4 | Legacy and semantic PATH/FPATH additions preserve literal versus safe late-bound values, source order, and captured-base reversal. | ✓ VERIFIED | `pathDelta` now derives per-addition dynamic provenance; `TestBuildPreservesLegacyListDynamicProvenanceAndSourceOrder` and `TestPipelineStoreRoundTripLegacyDynamic` passed under `zsh -f`. |
| 5 | Alias/function bodies are captured for shadow restoration and malformed emitted names fail closed. | ✓ VERIFIED | Existing NUL-framed introspection, name validation, shadow capture/restore, and pipeline coverage remain wired and pass in the full suite. |
| 6 | Every supported multi-name function has a reversible operation for every valid name. | ✓ VERIFIED | Builder atomically reduces valid function names and the existing live-zsh multi-name round trip remains green. |
| 7 | Full-state zero-residue and renderer-mutant checks are active in the default suite. | ✓ VERIFIED | Full `go test -count=1 ./...` passed, including `TestZeroResidueFullStateProperty` and `TestResidueRendererMutants`. |
| 8 | Profile DTO preserves the currently modeled semantic state and historical incomplete Append/Array/Flagged data fails closed. | ✓ VERIFIED | `TestStructuralFidelityDTOCompatibilityMatrix` passed for absent, each partial marker, and unsupported version; re-save leaves fidelity unknown rather than inferring false. |
| 9 | Every admitted source form applies and deactivates without semantic loss or residue. | ✗ FAILED | Valid indexed/declaration-flagged forms are admitted by the shared predicate despite their missing source-shape fidelity. Numeric indexed assignments silently become scalars; associative ones error when a real association is present; `export -i` loses integer behavior on regeneration. |

**Score:** 7/9 truths verified (0 present-but-behavior-unverified).

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/model/profile.go` | Shared persisted structural-fidelity/representability contract | ⚠️ PARTIAL | Correctly handles known-vs-unknown and Append/Array/Flagged, but has no indexed or declaration-flag state. |
| `core/store/dto.go` | Presence-aware versioned fidelity representation | ⚠️ PARTIAL | Version 1 explicitly stores only Append, Array, Flagged; source-shape loss is permanent across a re-save. |
| `core/ir/{build,regen}.go` | Carry parser fidelity and choose verbatim versus template | ⚠️ PARTIAL | Carries the three declared markers and protects them; missing shapes are stamped as known ordinary declarations and templated. |
| `core/activate/builder.go` | Source-ordered list reduction and representability-gated manifest construction | ⚠️ PARTIAL | Plan 15 fixed dynamic legacy PATH/FPATH provenance, but the gate admits indexed entries. |
| `core/shell/zsh/pipeline_test.go` | Persisted DTO-to-live-zsh proof | ⚠️ INCOMPLETE | Green Plan 15 matrices omit numeric/associative indexes and `export -i`. |
| `core/shell/zsh/emit.go` | The sole zsh renderer | ✓ VERIFIED | Renderer correctly consumes the list dynamic metadata supplied by Build; the blocked forms arise before emission. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `Block.Append/Array/Flagged` | `Entry` -> DTO -> `Representable()` | current parser/IR/store fidelity link | ✓ WIRED | Complete marker set is copied and Plan 15 raw historical compatibility matrix passes. |
| legacy `PATH`/`FPATH` Value | `pathDelta` -> `AdditionDynamic` -> `renderListDelta` | source-ordered list reduction | ✓ WIRED | Dynamic `$HOME`/`$EXTRA` additions remain late-bound in the live pipeline. |
| `syntax.Assign.Index` | `Block` -> `Entry` -> DTO -> `Representable()` | indexed-assignment fidelity | ✗ NOT WIRED | Parser never reads `a.Index`; no model or DTO field exists. |
| `export` flag words | `Block` -> `Entry` -> DTO -> `Regenerate` | declaration-attribute fidelity | ✗ NOT WIRED | Parser skips words beginning `-`; no flag marker exists and `DeclarationRepresentable()` permits `export`. |
| `Representable()` | `Regenerate` and `activate.Build` | shared forced-managed boundary | ⚠️ PARTIAL | Both consumers use the common predicate, but it returns true for the two missing shapes. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces faithful data | Status |
| --- | --- | --- | --- | --- |
| Legacy list reducer | `ListDelta.AdditionDynamic` | legacy Value -> `pathDelta` -> `composeLegacyList` -> emitter | Yes | ✓ FLOWING |
| Current structural fidelity | Append/Array/Flagged | parser Block -> Entry -> DTO -> shared predicate | Yes for the three modeled markers | ✓ FLOWING |
| Indexed assignment | index/subscript kind | `syntax.Assign.Index` -> parser -> Entry/DTO | No — never read | ✗ DISCONNECTED |
| `export -i` | declaration flag/attribute | CallExpr args -> parser -> Entry/DTO | No — `-i` is skipped | ✗ DISCONNECTED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Legacy dynamic PATH/FPATH persistence and live expansion | `go test -count=1 ./core/shell/zsh -run 'TestPipelineStoreRoundTripLegacyDynamic$' -v` | PASS under `zsh -f`; runtime HOME/EXTRA expansion and base restoration asserted. | ✓ PASS |
| Historical/current Plan 15 forced-managed structural forms | `go test -count=1 ./core/shell/zsh -run 'TestPipeline(PersistedOverrideManaged|LegacyStructuralFidelityMatrix)$' -v` | PASS; append, array, flagged alias, existing declaration and historical rows remain verbatim/no-operation. | ✓ PASS |
| Numeric index fidelity | Independent Parse -> DTO -> OverrideManaged -> Regenerate/Build/Diff/Emit -> `zsh -f` trace | FAIL: `FOO[2]=bar` becomes regenerated `FOO=bar`; manifest contains scalar `FOO=bar`; emitted plan leaves scalar `FOO=bar`. Direct source produces `array` with element 2 `bar`. | ✗ FAIL |
| Associative index fidelity | Independent trace and `zsh -f` check | FAIL: `MAP[key]=bar` becomes `MAP=bar`; direct source after `typeset -A MAP` is `association` with `MAP[key]=bar`, while applying the scalar plan over that association exits nonzero (`inconsistent type for assignment`). | ✗ FAIL |
| Export integer attribute fidelity | `zsh -f -c 'export -i FOO=1; FOO+=2; ...'` versus plain export; independent forced-managed persistence trace | FAIL: source is `integer-export`, value `3`; regenerated `export FOO=1` is `scalar-export`, value `12`. Build currently drops it only because its unsupported value mode happens to block activation, but regeneration is already lossy. | ✗ FAIL |
| Workspace regression suite | `GOTOOLCHAIN=auto go test -count=1 ./...` | PASS in 3.8s; `go build ./...` also passed. | ✓ PASS — missing cases are not covered |

### Probe Execution

No Phase 4 probe scripts are declared or present. The relevant executable surface is the focused `zsh -f` pipeline tests above.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| SW-01 | 04-01 through 04-15 | Apply only faithfully representable declarative state through the emitted zsh boundary. | ✗ BLOCKED | The shared persisted-fidelity boundary admits indexed assignments and flagged `export` declarations without their behavior-bearing source fields. |
| SW-02 | 04-01 through 04-15 | Reverse managed state without residue and leave base/unmanaged state untouched. | ✗ BLOCKED | Indexed scalar lowering changes the activated type/semantics; associative state can make activation fail. The random residue property does not exercise these profiles. |

No Phase 4 requirement is orphaned. No later phase explicitly owns fixing Profile/DTO/parser source-shape fidelity, so neither blocker is deferred to Phase 5 or 6.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `core/model/block.go` | 39-42 | Structural-fidelity marker set omits index and declaration flags | 🛑 BLOCKER | Valid source semantics are treated as known plain assignments. |
| `core/store/dto.go` | 49-56 | Versioned DTO omits the same fields | 🛑 BLOCKER | A re-save cannot recover the lost source semantics. |
| `core/shell/zsh/parse.go` | 70-94, 133-163, 189-205 | Assignment subscripts ignored; export flags skipped | 🛑 BLOCKER | Loss originates before profile persistence. |
| `core/shell/zsh/pipeline_test.go` | 367-470 | Forced-managed coverage misses indexed and `export -i` inputs | ⚠️ WARNING | Full-suite green result is non-discriminating for both blockers. |
| Phase-15-modified files | — | `TBD`/`FIXME`/`XXX` scan | ✓ CLEAN | No unresolved debt markers found. |

## Gaps Summary

Plan 04-15 genuinely closed the prior legacy-dynamic-list and Append/Array/Flagged historical-DTO holes. It did **not** make persisted structural fidelity complete: the new versioned representation defines "known" using only those three markers. Two valid zsh assignment forms therefore still cross Parse -> DTO -> OverrideManaged as ordinary scalar state even though their behavior cannot be regenerated or emitted faithfully.

This is a deterministic code-path failure, not a visual or external-service uncertainty, so no human verification can close it. Add the two missing source-shape dimensions, fail closed at the shared predicate, and pin both with the full live-zsh pipeline before re-verifying.

Next command: `gsd-plan-phase 4 --gaps`

---

_Verified: 2026-07-27T12:04:55Z_
_Verifier: the agent (gsd-verifier)_
