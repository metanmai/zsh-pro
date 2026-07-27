---
phase: 04-manifest-builder-emit
verified: 2026-07-27T11:10:46Z
status: gaps_found
score: 7/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 7/9
  gaps_closed:
    - "Repeated static legacy and mixed static legacy/semantic PATH and FPATH values now reduce to one source-ordered ListDelta."
    - "Persisted OverrideManaged typeset, readonly, tied, and local declarations regenerate verbatim and do not create manifest operations."
  gaps_remaining:
    - "Legacy dynamic list additions lose late-bound provenance during canonical reduction."
    - "Persisted OverrideManaged append assignments lose append semantics because the IR does not retain Append."
  regressions: []
gaps:
  - truth: "Legacy and semantic PATH/FPATH additions preserve source order and late-bound dynamic expansion through persistence, manifest construction, emission, and live zsh."
    status: failed
    reason: "The legacy compatibility parser accepts dynamic additions but stamps AdditionDynamic false; the canonical reducer copies that false provenance and emitter quotes the expression as a literal."
    artifacts:
      - path: "core/activate/builder.go"
        issue: "pathDelta lines 291-297 initializes every AdditionDynamic flag false; composeLegacyList lines 190-208 copies additions with dynamic=false."
      - path: "core/shell/zsh/emit.go"
        issue: "renderListDelta lines 41-58 uses explicit metadata and quotes entries whose dynamic flag is false, so a legacy $HOME/bin becomes a literal '$HOME/bin'."
      - path: "core/shell/zsh/pipeline_test.go"
        issue: "TestPipelineStoreRoundTripLegacy exercises static legacy entries only; its dynamic portion is always parser-semantic $EXTRA."
    missing:
      - "Preserve safe dynamic provenance for explicit legacy additions, without broadening the legacy grammar or freezing parameter expansion."
      - "Add DTO-to-zsh regressions for legacy dynamic PATH and FPATH and mixed semantic/legacy order with HOME and a simple parameter set at runtime."
  - truth: "A persisted forced-managed assignment is emitted only when its full source semantics are represented; += remains verbatim and creates no manifest operation."
    status: failed
    reason: "The parser records Block.Append, but model.Entry/DTO omit it. After OverrideManaged, DeclarationRepresentable permits the entry, regeneration emits = and Build emits SetScalar, changing append into overwrite."
    artifacts:
      - path: "core/model/block.go"
        issue: "Append is correctly recorded at line 39 but has no corresponding Entry field."
      - path: "core/ir/build.go"
        issue: "Lines 26-48 copy no Append/Array/Flagged structural-fidelity markers into the persisted Entry."
      - path: "core/model/profile.go"
        issue: "DeclarationRepresentable lines 124-138 rejects only declaration CmdName values, so a forced append assignment appears representable."
      - path: "core/ir/regen.go"
        issue: "Lines 27-30 route that forced entry to the templater, whose assignment path emits NAME=VALUE."
    missing:
      - "Persist the structural representability flags (at minimum Append; audit Array and Flagged) and use one shared predicate in regeneration and manifest construction."
      - "Add Parse -> DTO marshal/unmarshal -> OverrideManaged -> Regenerate/Build/live-zsh tests for FOO+=bar and export PATH+=:/x."
deferred:
  - truth: "A sourced loader evals emitted code into the parent terminal."
    addressed_in: "Phase 5"
    evidence: "Phase 5 explicitly owns the hook/loader integration; this Phase 4 verifier checks the manifest-to-emitter boundary."
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Build a versioned shell-agnostic manifest/diff/emitter that applies only faithfully representable declarative profile state, preserves late-bound values, and reverses with zero residue.

**Verified:** 2026-07-27T11:10:46Z  
**Status:** gaps_found  
**Re-verification:** Yes — after Plan 04-14

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Manifest v1 is versioned and shell-agnostic, and Diff rejects incompatible schemas. | ✓ VERIFIED | `model.Manifest` and `activate.Diff` remain substantive and covered by the current suite. |
| 2 | Only faithfully representable declarative state enters scalar/list manifest operations. | ✗ FAILED | A persisted forced-managed `+=` assignment is indistinguishable from plain `=` at the Entry/DTO/builder boundary. |
| 3 | `core/activate` provides a shell-agnostic deactivate-then-activate plan and zsh reverse syntax is confined to the emitter. | ✓ VERIFIED | `core/activate` imports only `core/model`; live zsh plan tests and reverse-token ownership checks remain green. |
| 4 | Supported operations preserve literal versus late-bound values and ownership-aware reversibility. | ✗ FAILED | Legacy dynamic PATH/FPATH values are converted to `AdditionDynamic:false` and emitted quoted, freezing their shell expressions. |
| 5 | Alias/function bodies are captured for shadow restoration and introspection fails closed. | ✓ VERIFIED | Existing NUL-framed introspection implementation and regressions remain wired. |
| 6 | Every supported multi-name function has a reversible operation for every valid name. | ✓ VERIFIED | Current builder atomically reduces all valid names; live-zsh multi-name round trip remains green. |
| 7 | The full-state residue property and renderer-mutant checks are active in the default suite. | ✓ VERIFIED | `TestZeroResidueFullStateProperty` and `TestResidueRendererMutants` ran and passed. |
| 8 | Profile DTO retains semantic state and store behavior remains transactional. | ✓ VERIFIED | Full uncached Go suite passed; current DTO tests confirm the declared persisted fields round-trip. |
| 9 | Every admitted legacy/semantic profile applies and deactivates path-independently without residue. | ✗ FAILED | The uncovered legacy-dynamic and forced-append profiles either apply the wrong live value or overwrite rather than append. |

**Score:** 7/9 truths verified (0 present-but-behavior-unverified).

## Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/activate/builder.go` | One source-ordered list reducer and a faithful builder boundary | ✗ FAILED | Static composition is fixed, but legacy dynamic flags are discarded and forced append can reach scalar lowering. |
| `core/model/profile.go` | Persisted representation/representability contract | ✗ FAILED | Declaration guard works for four command forms only; it cannot see a discarded Append flag. |
| `core/ir/{build,regen}.go` | Preserve fidelity state and choose template/verbatim correctly | ✗ FAILED | Build drops `Block.Append`; regen consequently templates a forced append as overwrite. |
| `core/shell/zsh/emit.go` | Safely render explicit list provenance | ✓ VERIFIED for valid metadata | It correctly obeys metadata; the Builder supplies false provenance for accepted legacy dynamic input. |
| `core/shell/zsh/{pipeline,residue}_test.go` | Real persistence-to-zsh proof | ⚠️ INCOMPLETE | Green tests cover static legacy composition and four declaration commands, not either failing input. |

## Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- |
| Legacy list Entry -> canonical token reducer -> `ListDelta.AdditionDynamic` -> emitter | Dynamic PATH/FPATH late binding | ✗ PARTIAL | `pathDelta` produces all-false metadata, `composeLegacyList` carries it as static, and `renderListDelta` quotes it. |
| Parser Block -> IR Entry -> DTO -> regenerate/Build | Forced append fidelity | ✗ NOT WIRED | `Block.Append` is intentionally parsed, but no Entry/DTO field carries it across persistence. |
| Semantic list Entry -> reducer -> emitter -> `zsh -f` | Source order and dynamic cardinality | ✓ WIRED | Semantic `$EXTRA` cases and static mixed legacy cases passed. |
| Parser -> IR -> Build -> Diff -> Emit | Reject cross-list, invalid list, multi-name/function/declaration regressions | ✓ WIRED | Focused suite passed after Plan 04-14. |

## Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Produces faithful data | Status |
| --- | --- | --- | --- | --- |
| Legacy list | `AdditionDynamic` | legacy `Value` -> `pathDelta` -> `composeLegacyList` | No — every flag false | ✗ HOLLOW/LOSSY |
| Forced append | `Entry` fidelity fields | parsed `Block.Append` -> IR -> DTO | No — marker disappears before persistence | ✗ DISCONNECTED |
| Semantic list | `ListValue.Segments` | parser semantic contract -> reducer -> emitter | Yes | ✓ FLOWING |
| Declaration command forms | `CmdName` | parse -> DTO -> representability gate | Yes for typeset/declare/local/readonly | ✓ FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Legacy/semantic reducer tests | `go test -count=1 ./core/activate -run 'TestBuild(Composes.*Legacy.*|Composes.*Semantic.*|RequiresSemanticListOrLegacySameList)$' -v` | PASS | ✓ PASS — static-only legacy coverage. |
| Store-to-live-zsh, residue, renderer mutants | `go test -count=1 ./core/shell/zsh -run 'Test(PipelineStoreRoundTripLegacy|PipelinePersistedOverrideManaged|ResidueStoreRoundTripLegacy|ZeroResidueFullStateProperty|ResidueRendererMutants)$' -v` | PASS under `zsh -f` | ✓ PASS — both failed inputs omitted. |
| Whole workspace | `GOTOOLCHAIN=auto go test -count=1 ./...` | PASS | ✓ PASS — not evidence of untested behavior. |
| Build | `GOTOOLCHAIN=auto go build ./...` | PASS | ✓ PASS |

## Probe Execution

No Phase 4 probe scripts are declared or present. The focused live-zsh tests above are the runnable verification surface.

## Requirements Coverage

| Requirement | Status | Evidence |
| --- | --- | --- |
| SW-01 — only faithfully representable declarative state is applied through emitted zsh | ✗ BLOCKED | Legacy dynamic additions freeze and forced append is represented as a different operation. |
| SW-02 — activation/deactivation leaves zero residue and base state intact | ✗ BLOCKED | The affected input domain is admitted but not faithfully applied; green residue fixtures do not include it. |

No Phase 4 requirement is orphaned; all Plan 04-01 through 04-14 requirement declarations map to SW-01 and/or SW-02.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- |
| `core/activate/builder.go` | 190-208, 291-297 | Legacy additions hard-coded static | 🛑 BLOCKER | Late-bound expressions are frozen as literal paths. |
| `core/ir/build.go` | 26-48 | Does not persist Append/Array/Flagged fidelity markers | 🛑 BLOCKER | Override can turn a rejected append into an overwrite. |
| `core/shell/zsh/pipeline_test.go` | 245-364 | Missing critical input cases | ⚠️ WARNING | All listed Plan 04-14 pipelines pass without exercising the two blockers. |
| Phase-modified production files | — | `TBD`/`FIXME`/`XXX` scan | ✓ CLEAN | No unresolved debt marker found. |

## Deferred Items

The parent-shell sourced loader is Phase 5 work. It is explicitly scheduled and does not absorb either Phase 4 builder-boundary blocker.

## Gaps Summary

Phase 4 is **not achieved**. Plan 04-14 closed the originally observed static-list and declaration-command cases, but current code still changes behavior for two persisted profile shapes: dynamic legacy list additions are made literal, and forced-managed append assignments become overwrites. These are deterministic code-path failures; the fresh full suite and build pass because neither path is exercised.

Next command: `gsd-plan-phase 4 --gaps`

---

_Verified: 2026-07-27T11:10:46Z_  
_Verifier: the agent (gsd-verifier)_
