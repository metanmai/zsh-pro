---
phase: 04-manifest-builder-emit
verified: 2026-07-27T08:38:33Z
status: gaps_found
score: 6/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 4/11
  gaps_closed:
    - "Repeated identities now reduce to final effective identities with collision-free, presence-aware undo state."
    - "Literal values, dynamic provenance, function bodies, introspection source failures, and transactional secret writes have dedicated code and passing regressions."
    - "The full-state zero-residue oracle now covers repeated scalar/function identities, punctuation-distinct names, PATH and FPATH, sentinel data, and renderer/state mutants."
  gaps_remaining: []
  regressions:
    - "The parser/IR route admits list and declaration shapes that the manifest builder cannot represent faithfully."
    - "The parser/IR route admits multi-name functions that Build silently omits."
gaps:
  - truth: "Only declarative profile entries that can be represented faithfully become PATH/FPATH manifest operations."
    status: failed
    reason: "A parsed list with nil ListValue falls back to raw source splitting; the fallback accepts PATH/FPATH base markers interchangeably and ignores ValueMode."
    artifacts:
      - path: "core/activate/builder.go"
        issue: "Lines 77-80 invoke pathDelta after semantic parsing rejected the list; lines 209-248 accept $FPATH/$fpath as a PATH base and vice versa."
      - path: "core/ir/route.go"
        issue: "Lines 46-59 mark the entry managed without requiring a valid ListValue."
    missing:
      - "Restrict raw pathDelta fallback to explicitly legacy entries and require a same-variable base marker."
      - "Add Parse -> IR -> Build regressions for PATH=$FPATH:/a, FPATH=$PATH:/a, and unsupported list syntax asserting no list operation."
  - truth: "Only attribute-free declarative scalar assignments are applied as reversible scalar operations."
    status: failed
    reason: "typeset, declare, local, and readonly declarations are recorded as KindAssignment without their declaration flags/attributes, routed managed, then emitted as plain export/typeset -g assignments."
    artifacts:
      - path: "core/shell/zsh/parse.go"
        issue: "Lines 133-205 collapse declaration forms to a normal assignment and preserve neither flags nor attributes."
      - path: "core/ir/route.go"
        issue: "Lines 46-59 admit every single-name non-array KindAssignment in the scalar categories."
      - path: "core/activate/builder.go"
        issue: "Lines 43-63 reduce the entry to Scalar{Name, Applied, Exported}, discarding integer, readonly, tied, and local semantics."
    missing:
      - "Route declaration/attribute-bearing forms to the imperative verbatim path until their semantics are modeled."
      - "Add parser-to-manifest tests for typeset -i, readonly, tied, and local declarations."
  - truth: "Every managed function declaration contributes a reversible function operation."
    status: failed
    reason: "The parser explicitly accepts zsh's function one two { ... } form and routeManaged marks every KindFuncDecl managed, but Build emits only when len(Names) == 1, dropping the declaration silently."
    artifacts:
      - path: "core/shell/zsh/parse.go"
        issue: "Lines 206-220 append every declared function name and capture one body."
      - path: "core/ir/route.go"
        issue: "Lines 44-45 admit all function declarations."
      - path: "core/activate/builder.go"
        issue: "Lines 92-106 require len(e.Names) == 1 and otherwise produce no manifest function."
    missing:
      - "Either route multi-name functions as imperative or emit one final function identity per declared name."
      - "Add a Parse -> IR -> Build -> Diff -> Emit live-zsh test that checks both names apply and restore."
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Build a versioned, shell-agnostic activation manifest from the declarative profile IR, diff it to executable operations, and emit safe reversible zsh activation/deactivation scripts while preserving late-bound values and zero residue.

**Verified:** 2026-07-27T08:38:33Z
**Status:** gaps_found
**Re-verification:** Yes — independent regression check after earlier gap closure.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Manifest v1 is a versioned, shell-agnostic record and Diff enforces SchemaV1 before plan generation. | ✓ VERIFIED | `core/model/manifest.go:3-69`; `core/activate/diff.go:11-45`; fresh model/activate and full-suite tests passed. |
| 2 | The parser/IR admits only profile entries the manifest can represent faithfully, preserving literal and late-bound values. | ✗ FAILED | PATH/FPATH semantic rejection is bypassed by `builder.go:77-80,209-248`; declaration attributes are discarded by `parse.go:133-205` before `route.go:46-59` admits them. |
| 3 | `core/activate` produces a shell-agnostic deactivate-then-activate plan, with reverse zsh syntax generated only by the zsh emitter. | ✓ VERIFIED | `Diff` builds deactivate first (`diff.go:38-45`); `TestReverseSyntaxHasSingleEmitHome` passed; manual lexical trace locates reverse commands only in `emit.go` (apart from non-reverse forward regen terminology). |
| 4 | Emitter operations are injected safely, preserve explicit dynamic provenance, and restore scalar/list/alias/function/option state for the supported manifest domain. | ✓ VERIFIED | Fresh live-zsh pipeline tests for values/functions, punctuation collisions/sentinel/export restoration, and composed PATH/FPATH all passed; `TestZeroResidueFullStateProperty` passed in its non-empty, present-empty, and unset-FPATH baselines. |
| 5 | Alias/function bodies are captured before override and introspection fails closed without interpreting body bytes as protocol controls. | ✓ VERIFIED | `introspect.go:18-97` exits on nonzero `source`, isolates line prefix before NUL body records, and the full suite passed the introspection regressions. |
| 6 | Every accepted managed function declaration reaches activation and deactivation. | ✗ FAILED | Multi-name function declarations are accepted by parser/route then suppressed by the single-name guard in `builder.go:92-106`; no test exercises this source form. |
| 7 | Repeated scalar/function/option/list identities, sentinel-valued data, PATH/FPATH presence, and renderer mutants are rejected by a full-state residue oracle. | ✓ VERIFIED | `residue_test.go` builds manifests through Parse -> IR -> Build -> Diff -> Emit; the named property and mutant checks passed under `zsh -f`. |
| 8 | Secrets are carried as semantic runtime values and Commit preserves backend/Git atomicity on failure. | ✓ VERIFIED | Fresh `core/store` transaction run passed `TestCommitRollsBackSecretWritesBeforeMovingRef` and `TestCommitCompensatesAmbiguousRefFailure`, plus literal/dynamic secret regressions. |
| 9 | Applying then deactivating any Phase-4 admitted declarative profile is reversible and zero-residue. | ✗ FAILED | Truths 2 and 6 show admitted source shapes that change semantics or disappear before emitting; green residue fixtures do not cover either shape. |

**Score:** 6/9 must-haves verified (0 present-but-behavior-unverified).

## Required Artifacts

All 40 declared artifacts exist and are substantive except for one plan-location mismatch: `verify.artifacts` marks `core/store/store_test.go` incomplete because its required `Rollback` pattern is absent. The transactional regressions actually live in `core/store/secret_test.go` and passed, so this is a documentation/test-placement warning rather than evidence that the transaction behavior is missing.

| Artifact group | Status | Details |
| --- | --- | --- |
| `core/model/manifest.go`, `core/activate/{builder,plan,diff}.go` | ⚠️ WIRED, incomplete | Manifest/diff are substantive and used by live tests; Builder has the two lossy admission paths above. |
| `core/shell/zsh/{emit,introspect,parse}.go` | ⚠️ WIRED, incomplete | Emit and introspection work for supported plans; parse/route do not preserve declaration attributes or constrain multi-name function admission. |
| `core/shell/zsh/{pipeline,residue,invariant}_test.go` | ✓ VERIFIED | Tests execute real zsh, trace the production path, and include property/mutant coverage. Their input domain omits the three failed source shapes. |
| `core/store/{secret,store,errors}.go` and transaction tests | ✓ VERIFIED | Side-effect-free preparation, rollback/compensation, and typed errors are wired and exercised by focused tests. |

## Key Link Verification

| From | To | Status | Evidence |
| --- | --- | --- | --- |
| zsh parser -> IR -> Build | ✗ PARTIAL | `ir.Build` copies semantic fields, but `routeManaged` admits unsupported lists/attribute declarations and Build loses those semantics. |
| Build -> Diff -> zsh Provider.Emit -> live zsh | ✓ WIRED | `pipeline_test.go` and `residue_test.go` call the production types and passed under `zsh -f`. |
| Introspection script -> `Provider.Introspect` | ✓ WIRED | `source_status` gate exits before output; `cmd.Run` returns the error. |
| Secret preparation -> Commit -> backend/Git compensation | ✓ WIRED | Focused transaction tests passed. |
| Composition root -> `shell.Emitter` | ⚠️ PARTIAL (expected Phase 5) | `main.go:21-23` assigns `zsh.Provider{}` to `shell.Emitter` but intentionally does not drive it; Phase 5 owns the sourced loader/CLI. |

## Data-Flow Trace

| Artifact | Data source | Status | Evidence |
| --- | --- | --- | --- |
| Scalar/alias runtime data | AST `RuntimeValue`/`ValueMode` -> IR -> Build -> Diff -> Emit | ✓ FLOWING for supported forms | Live pipeline value test passed. |
| PATH/FPATH data | AST `ListValue` -> IR -> Build -> emitted tied-scalar assignment | ⚠️ HOLLOW on rejected forms | Valid semantic lists compose correctly; nil `ListValue` falls into raw `pathDelta`, reopening rejected/cross-list forms. |
| Function body data | AST `FunctionBody` -> IR -> `FuncSet.Bodies` -> `AddFunc` -> emitter | ⚠️ PARTIAL | Unary definitions are live-tested; a multi-name declaration has a body but is dropped by Builder. |
| Secret data | `RuntimeValue` -> prepare -> backend mutation -> CAS Git ref | ✓ FLOWING | Exact-value and rollback/ambiguous-ref tests passed. |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Whole current workspace | `GOTOOLCHAIN=auto go test -count=1 ./...` | all packages passed | ✓ PASS |
| Real emitter pipeline | `go test -count=1 ./core/shell/zsh -run 'TestPipeline(PreservesValuesAndFunctionsInLiveZsh|RepeatedCollisionSentinelAndExportRestoration|ComposedPathAndFPathDynamicExpansion)$' -v` | three `zsh -f` integrations passed | ✓ PASS |
| Full-shell residue and reverse-syntax invariant | `go test -count=1 ./core/shell/zsh -run 'TestZeroResidueFullStateProperty|TestReverseSyntaxHasSingleEmitHome|TestParseListValueRejectsAmbiguousBase' -v` | all passed | ✓ PASS, but exposes a coverage disconnect: parser rejects cross-list inputs while Builder re-admits them |
| Transaction rollback/compensation | `go test -count=1 ./core/store -run 'Test.*(Rollback|Commit|Secret|Transaction)' -v` | all listed transaction/secret tests passed | ✓ PASS |
| Invalid emitter operation disposition | `go test -count=1 ./core/shell/zsh -run '^TestEmitRejectsHostileNamesWithoutOutput$' -v` | passes by asserting no error and empty output | ⚠️ WARNING — final validation silently omits invalid operations |

## Requirements Coverage

| Requirement | Source Plans | Status | Evidence |
| --- | --- | --- | --- |
| SW-01 — apply declarative state through one emitted zsh path | 04-01, 02, 03, 04, 06, 07, 08, 09, 11, 12 | ✗ BLOCKED | The single emitter seam is real, but Build can create a semantically wrong list operation and lose declaration attributes/multi-name functions. The sourced loader itself is explicitly Phase 5 work, not counted as this Phase-4 gap. |
| SW-02 — deactivate/reverse with zero residue | 04-01, 02, 04, 05, 06, 09, 10, 12 | ✗ BLOCKED | Property evidence proves supported fixtures only; the admitted source paths above are not reversible and have no regression tests. |

All declared Phase 4 requirement IDs are accounted for in `.planning/REQUIREMENTS.md`; no orphaned requirement IDs were found.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `core/activate/builder.go` | 77-80, 209-248 | Raw list fallback after semantic rejection | 🛑 BLOCKER | Re-admits unsupported/cross-list list expressions. |
| `core/shell/zsh/parse.go` | 133-205 | Attribute-bearing declarations collapsed to scalar entries | 🛑 BLOCKER | Integer/readonly/tied/local meaning is not preserved. |
| `core/activate/builder.go` | 92-106 | Managed multi-name function silently omitted | 🛑 BLOCKER | Accepted profile declaration does not activate. |
| `core/shell/zsh/emit.go` | 166-210, 226-277 | Invalid operation returns nil after omission | ⚠️ WARNING | Corrupt/manual manifests can appear to activate successfully with missing operations. |
| `core/activate/builder.go` | 15-28 | `Build(model.Profile)` cannot set `Manifest.Profile` | ⚠️ WARNING | Call sites must remember to assign identity; pipeline/residue tests do it manually. |
| Phase-modified production files | — | `TBD`/`FIXME`/`XXX` scan | ✓ CLEAN | No unresolved debt-marker blocker found. |

## Probe Execution

No declared or conventional `scripts/*/tests/probe-*.sh` probes exist for this Go phase. The focused Go/zsh tests above are the runnable verification surface.

## Human Verification Required

None. The blocking failures are deterministically observable from parser, router, builder, and existing test-domain evidence; human UAT cannot resolve them.

## Gaps Summary

Phase 4 is **not achieved**. The implementation is strong for the supported fixture domain: the fresh full suite, real-zsh pipeline, full-state residue oracle, reverse-syntax invariant, and secret transaction checks all pass. However, three accepted source forms are not faithfully represented before emission. That invalidates both SW-01 and SW-02 for the declared profile IR, regardless of the green fixture suite.

Next command: `gsd-plan-phase 4 --gaps`

---

_Verified: 2026-07-27T08:38:33Z_
_Verifier: the agent (gsd-verifier)_
