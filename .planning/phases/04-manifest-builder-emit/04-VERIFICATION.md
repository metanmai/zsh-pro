---
phase: 04-manifest-builder-emit
verified: 2026-07-27T09:49:10Z
status: gaps_found
score: 7/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 6/9
  gaps_closed:
    - "Parsed unsupported/cross-list forms are inert; raw list fallback is restricted to ValueModeLegacy with a same-list base marker."
    - "Auto-routed typeset, declare, local, and readonly forms stay imperative, and valid multi-name functions now apply and restore."
  gaps_remaining:
    - "Legacy list fallbacks bypass source-ordered list composition."
    - "Persisted OverrideManaged declarations bypass the builder's faithful-representation boundary."
  regressions: []
gaps:
  - truth: "Every accepted legacy or semantic PATH/FPATH entry is composed in source order into one faithful reversible delta."
    status: failed
    reason: "Legacy fallback deltas are appended directly to Manifest.Lists rather than the semantic composition state; later operations rebuild from the captured base and discard earlier legacy additions."
    artifacts:
      - path: "core/activate/builder.go"
        issue: "Lines 77-80 append pathDelta directly, while lines 65-76 and 139-152 compose only semantic ListValue entries."
      - path: "core/shell/zsh/emit.go"
        issue: "Lines 181-190 capture one base slot, and renderList/renderListDelta each rebuild from that same base, so two PATH operations cannot preserve accumulated prior additions."
    missing:
      - "Convert a valid legacy pathDelta into listTokens and feed it through the same per-canonical-list source-order composition path as semantic lists."
      - "Add Parse/IR/store/Build/Diff/Emit live-zsh regressions for legacy+legacy and legacy+semantic PATH and FPATH in both source orders."
  - truth: "Only faithfully representable declarative assignments can cross a persisted profile into reversible scalar/list manifest operations."
    status: failed
    reason: "EffectiveManaged honors persisted OverrideManaged before Build inspects declaration semantics, so typeset/declare/local/readonly entries are emitted as ordinary scalars and lose attributes or scope."
    artifacts:
      - path: "core/model/profile.go"
        issue: "Lines 109-121 make OverrideManaged return true regardless of the router's auto verdict."
      - path: "core/activate/builder.go"
        issue: "Lines 34-48 accept every effective managed KindAssignment in environment/secrets categories without excluding declaration CmdName values."
      - path: "core/store/dto.go"
        issue: "Lines 31-43 and 103-146 persist and restore CmdName plus Override, making this reachable from profile.json."
    missing:
      - "At the builder boundary reject unmodeled declaration commands regardless of EffectiveManaged, and make regeneration retain their Text as a second guard."
      - "Add a Parse -> persisted profile -> Build regression for typeset -i, readonly, tied, and local forms with OverrideManaged, asserting no scalar/list operation and verbatim regeneration."
deferred:
  - truth: "A sourced loader evals emitted code into the parent terminal."
    addressed_in: "Phase 5"
    evidence: "Phase 5 goal explicitly wires the hook loader and eval into the live terminal; Phase 4 supplies the emitter seam."
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Build a versioned shell-agnostic manifest/diff/emitter that applies only faithfully representable declarative profile state, preserves late-bound values, and reverses with zero residue.

**Verified:** 2026-07-27T09:49:10Z  
**Status:** gaps_found  
**Re-verification:** Yes — after Plan 04-13

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Manifest v1 is a versioned, shell-agnostic reversible record; Diff rejects incompatible schemas. | ✓ VERIFIED | `core/model/manifest.go` supplies the v1 model; `core/activate/diff.go` validates schema before building operations. |
| 2 | Only faithfully representable declarative state reaches manifest operations. | ✗ FAILED | Persisted `OverrideManaged` declarations bypass the parser/router guard and Build lowers them to ordinary scalars. |
| 3 | `core/activate` creates a shell-agnostic deactivate-then-activate plan; reverse zsh syntax is owned by the emitter. | ✓ VERIFIED | `core/activate` imports only `core/model`; `Diff` orders deactivate before activate; reverse-token ownership remains in `core/shell/zsh/emit.go`. |
| 4 | Supported scalar/list/alias/function/option operations preserve literal versus late-bound provenance and reverse ownership-aware state. | ✓ VERIFIED | Focused live-zsh pipeline tests passed for values, function bodies, cross-list rejection, multi-name restoration, and semantic PATH/FPATH expansion. |
| 5 | Introspection captures shadowable alias/function bodies and fails closed on source/framing failure. | ✓ VERIFIED | `core/shell/zsh/introspect.go` uses NUL-framed body records and propagates source errors; prior focused regression suite remains present. |
| 6 | Every accepted function declaration has a reversible operation for every valid declared name. | ✓ VERIFIED | Builder atomically validates/reduces all names (`builder.go:92-118`); `TestPipelineMultiNameFunctionRoundTrip` passed under `zsh -f`. |
| 7 | The residue oracle exercises source-derived profiles and rejects renderer/state mutants. | ✓ VERIFIED | `residue_test.go` uses Parse -> IR -> Build -> Diff -> Emit under `zsh -f`; its property and invariant are wired into the standard package tests. |
| 8 | Secret values retain semantic treatment and storage commits are transactional. | ✓ VERIFIED | Store DTO preserves semantic fields; focused store round-trip/transaction tests passed. |
| 9 | Applying and deactivating any admitted legacy or semantic profile is path-independent and zero-residue. | ✗ FAILED | Repeated/mixed legacy PATH/FPATH additions are accepted but earlier additions are lost at the builder/emitter boundary; existing property fixtures do not cover that domain. |

**Score:** 7/9 truths verified (0 present-but-behavior-unverified).

## Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/model/manifest.go`, `core/activate/{plan,diff}.go` | Versioned shell-agnostic manifest and plan | ✓ VERIFIED | Substantive and wired through live pipeline tests. |
| `core/activate/builder.go` | Faithful source-order reduction to manifest parts | ✗ FAILED | Two boundary failures below make an otherwise substantive/wired artifact lossy. |
| `core/shell/zsh/{emit,introspect}.go` | Sole zsh emitter and recoverable live-state capture | ✓ VERIFIED for one operation per list | A second list operation resets to the same captured base, exposing the Builder's duplicate-delta defect. |
| `core/store/dto.go` | Lossless persisted Profile fields | ✓ VERIFIED, security-relevant | It correctly persists `CmdName` and `Override`; that proves the forced-managed declaration gap is reachable, not a synthetic in-memory-only case. |
| `core/shell/zsh/{pipeline,residue,invariant}_test.go` | Real shell regression/property coverage | ⚠️ INCOMPLETE | Tests are substantive and pass, but omit mixed/repeated legacy-list and persisted forced-managed declaration paths. |

## Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- |
| Parser -> IR router -> Builder | Declarative entry admission | ✗ PARTIAL | Router rejects declarations automatically, but `EffectiveManaged()` re-admits a persisted override before Build's unguarded scalar branch. |
| Legacy/semantic list entry -> Builder composition -> Emit | One source-ordered PATH/FPATH delta | ✗ NOT WIRED faithfully | Semantic entries flow through `lists`; legacy `pathDelta` entries bypass it into `m.Lists`, then emitter resets from the same captured base for every op. |
| Build -> Diff -> zsh Provider.Emit -> `zsh -f` | Supported manifest operations | ✓ WIRED | Current focused pipeline tests passed. |
| Introspection -> emitter restore slots | Shadow body capture/restore | ✓ WIRED | NUL-framed body data feeds the supported function restore path. |

## Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Produces real data | Status |
| --- | --- | --- | --- | --- |
| List manifest | `m.Lists` | `ListValue` -> `lists` map, versus legacy `pathDelta` -> direct append | No for repeated/mixed legacy paths | ✗ HOLLOW/LOSSY |
| Declaration scalar | `m.Env` | persisted `Entry{CmdName, Override, RuntimeValue}` -> `EffectiveManaged()` -> scalar branch | Yes, but with semantics discarded | ✗ HOLLOW/LOSSY |
| Functions | `Functions.Bodies` | parsed function body -> IR -> Build -> Emit | Yes | ✓ FLOWING |
| Secrets | semantic entry -> DTO/store transaction | DTO + transactional store code | Yes | ✓ FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Semantic list composition, legacy same-list admission, multi-name reduction | `go test -count=1 ./core/activate -run 'TestBuild(RequiresSemanticListOrLegacySameList|ComposesSemanticListsInSourceOrder|ReducesFunctionNamesAtomically)$' -v` | PASS | ✓ PASS — also demonstrates the missing mixed/repeated-legacy cases. |
| Auto-routed declarations remain imperative | `go test -count=1 ./core/ir -run 'TestBuildDeclarationFormsStayImperative$' -v` | PASS | ✓ PASS — no `OverrideManaged` persistence case is exercised. |
| Production zsh pipelines | `go test -count=1 ./core/shell/zsh -run 'TestPipeline(ComposedPathAndFPathDynamicExpansion|RejectsUnsupportedListForms|MultiNameFunctionRoundTrip)$' -v` | PASS under `zsh -f` | ✓ PASS — only semantic list composition is covered. |
| DTO profile persistence | `go test -count=1 ./core/store -run 'Test(RoundTrip|Marshal|ListValueDTO)' -v` | PASS | ✓ PASS — confirms profile field persistence but not the forced-managed declaration boundary. |

## Probe Execution

No declared or conventional `scripts/*/tests/probe-*.sh` probes exist. Go and live-zsh focused tests are the runnable Phase 4 surface.

## Requirements Coverage

| Requirement | Source Plans | Status | Evidence |
| --- | --- | --- | --- |
| SW-01 — apply declarative state through one emitted zsh path | 04-01 through 04-13 | ✗ BLOCKED | One emitter seam exists, but it receives a scalar that no longer represents forced-managed declaration semantics and can receive an incomplete legacy list result. |
| SW-02 — reverse prior profile with zero residue | 04-01 through 04-13 | ✗ BLOCKED | Reverse works for covered supported fixtures, not every admitted legacy/semantic profile; repeated/mixed legacy list input loses additions before reversal. |

All Phase 4 plan requirement IDs are SW-01/SW-02 and both appear in `.planning/REQUIREMENTS.md`; no orphaned Phase 4 requirements were found.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `core/activate/builder.go` | 77-80 | Legacy delta bypasses composition | 🛑 BLOCKER | Earlier PATH/FPATH additions can disappear. |
| `core/activate/builder.go` | 34-48 | Effective override crosses unmodeled declaration boundary | 🛑 BLOCKER | `typeset`/`declare`/`local`/`readonly` meaning can be silently changed. |
| `core/shell/zsh/{pipeline,residue}_test.go` | — | Coverage gap | ⚠️ WARNING | Green property/pipeline tests do not exercise either blocker path. |
| Phase-modified production files | — | `TBD`/`FIXME`/`XXX` scan | ✓ CLEAN | No unresolved debt-marker blocker found. |

## Deferred Items

The parent-shell sourced loader is specifically Phase 5 work. Phase 4 has a wired emitter/provider seam but does not itself install the loader; this is explicitly scheduled by the Phase 5 roadmap goal and is not counted as a Phase 4 gap.

## Gaps Summary

Phase 4 is **not achieved**. Plan 04-13 correctly closed the prior parser-list, auto-routed declaration, and multi-name-function failures, but its new legacy compatibility path is not composed with semantic list state, and its router-only declaration guard is bypassable through persisted `OverrideManaged` entries. These are deterministic source-level defects despite the green uncached suite.

Next command: `gsd-plan-phase 4 --gaps`

---

_Verified: 2026-07-27T09:49:10Z_  
_Verifier: the agent (gsd-verifier)_
