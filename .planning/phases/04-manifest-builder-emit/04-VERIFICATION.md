---
phase: 04-manifest-builder-emit
verified: 2026-07-18T19:26:57Z
status: gaps_found
score: 4/11 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: failed
  previous_score: 8/11
  gaps_closed:
    - "Quoted scalar and alias delimiters are now decoded into RuntimeValue and the ordinary Parse to IR to Build to Diff to Emit path is integration-tested."
    - "Function bodies now survive Parse to IR to Manifest to Diff and ordinary unique function declarations apply in live zsh."
    - "The fixed five-field residue loop was replaced by a deterministic 24-action, full-state qqqq-encoded oracle with sensitivity and renderer-mutant checks."
  gaps_remaining:
    - "Emitter bookkeeping is not collision-free or compositional for repeated identities, sanitized alias/function-name collisions, the __ZP_UNSET__ data value, or duplicate function declarations."
    - "PATH and FPATH deltas are derived from source spelling rather than semantic segments and repeated deltas reset to base instead of composing."
    - "Literal decoding preserves zsh escape syntax as data, and secret exclusion stores source spelling instead of RuntimeValue."
    - "Introspection can report success after source failure and its line scanner can misclassify exact section-marker lines inside bodies."
    - "Secret backend writes occur before fallible profile and Git work, without rollback."
    - "The residue and reverse-syntax invariants omit the adversarial shapes that expose these failures."
  regressions:
    - "The previously verified graceful introspection-failure truth is contradicted by a missing-source repro that exits zero and reports a snapshot."
    - "The previously verified general shadow/PATH restoration truths hold only for unique single-write fixtures, not for the accepted input space."
gaps:
  - truth: "Applying then deactivating an accepted manifest restores the exact prior scalar, alias, function, option, and list state."
    status: failed
    reason: "Runtime state slots are derived by a lossy sanitizer, use a valid data value as an absence sentinel, and preserve the first rather than final applied scalar. Duplicate declarations produce duplicate deactivate operations."
    artifacts:
      - path: "core/shell/zsh/emit.go"
        issue: "sanitizeSlot collisions, __ZP_UNSET__ ambiguity, and first-write-wins __ZP_APPLIED state make restoration non-injective and non-compositional."
      - path: "core/activate/builder.go"
        issue: "Repeated environment and function entries are appended instead of reduced to one final effective identity."
      - path: "core/activate/diff.go"
        issue: "Duplicate function identities produce repeated unset/restore pairs."
    missing:
      - "Use injective or separately keyed bookkeeping with explicit presence flags."
      - "Track the final applied value and reduce repeated declarations to the effective state before emitting deactivate operations."
  - truth: "PATH and FPATH are represented and applied as faithful base-relative deltas."
    status: failed
    reason: "pathDelta splits raw source spelling, accepts a base reference to the wrong variable, and Emit resets to base for every delta; valid quoted or repeated assignments therefore disappear or lose earlier additions."
    artifacts:
      - path: "core/activate/builder.go"
        issue: "pathDelta uses Entry.Value and a shared PATH/FPATH base-token set instead of semantic, same-variable segments."
      - path: "core/shell/zsh/emit.go"
        issue: "renderList resets the list to its base for each ApplyListDelta."
    missing:
      - "Carry semantic list segments from parsing, accept only same-variable self-reference, and compose repeated assignments to one final delta."
  - truth: "Literal runtime values remain exact across parsing, activation, and secret exclusion."
    status: failed
    reason: "decodeLiteralParts copies mvdan Lit.Value without interpreting zsh escape syntax; excludeSecrets ignores RuntimeValue and stores Entry.Value source spelling."
    artifacts:
      - path: "core/shell/zsh/parse.go"
        issue: "FOO=hello\\ world becomes runtime data hello\\ world instead of hello world after re-emission."
      - path: "core/store/secret.go"
        issue: "A quoted secret is stored with quote characters, while a valid empty literal is rejected."
    missing:
      - "Decode supported unquoted and double-quoted escapes according to zsh runtime semantics."
      - "Persist the semantic RuntimeValue exactly, including an explicitly present empty string."
  - truth: "Introspection fails gracefully and returns an exact, delimiter-safe identity snapshot."
    status: failed
    reason: "The sourced file's status is ignored, and the whole-output line scanner still recognizes section sentinels found on exact body lines."
    artifacts:
      - path: "core/shell/zsh/introspect.go"
        issue: "source failure is masked by later successful print commands; marker-shaped body lines can pollute identity sections."
    missing:
      - "Exit on source failure and frame every section so body bytes cannot be interpreted as protocol control lines."
  - truth: "A failed Commit leaves both Git and the secret backend unchanged."
    status: failed
    reason: "excludeSecrets writes to the keychain/vault before JSON regeneration and all Git plumbing; later failure has no snapshot or rollback path."
    artifacts:
      - path: "core/store/store.go"
        issue: "Commit calls excludeSecrets before its later fallible work."
      - path: "core/store/secret.go"
        issue: "excludeSecrets mutates the backend incrementally and cannot roll back prior writes."
    missing:
      - "Prevalidate every entry, stage or snapshot secret mutations, and restore/delete them if any later backend or Commit step fails."
  - truth: "The automated invariants are capable of detecting residue and reverse-zsh syntax across the accepted Phase 4 input space."
    status: failed
    reason: "The improved property fixtures still use unique scalar/alias/function names, one PATH delta, no FPATH, and no sentinel-valued originals; the reverse-syntax scan skips every zsh package file except regen.go."
    artifacts:
      - path: "core/shell/zsh/residue_test.go"
        issue: "The oracle is strong, but its generated input domain omits all independently reproduced break cases."
      - path: "core/shell/zsh/invariant_test.go"
        issue: "The zsh directory filter scans regen.go instead of scanning all non-emitter production files."
    missing:
      - "Generate repeated identities, colliding legal names, sentinel data, duplicate functions, PATH plus FPATH, and multiple deltas."
      - "Scan every relevant non-test production file while excluding only emit.go as the allowed reverse-syntax home."
---

# Phase 4: Manifest Builder + Emit Verification Report

**Phase Goal:** Turn a resolved profile into a shell-agnostic, reversible plan and generate all apply/deactivate zsh through `core/shell/zsh/emit.go`.
**Verified:** 2026-07-18T19:26:57Z
**Status:** gaps_found
**Re-verification:** Yes — all three prior gaps were rechecked, then previously passing truths received a regression/disconfirmation pass.

## Cause

The phase models source statements, but the emitter must own effective shell identities. `Build` can retain repeated entries, while `Emit` stores prior/applied state in global variables derived from lossy names and assumes one write per identity. That mismatch makes restore state ambiguous or stale for repeated assignments, legal alias/function names that sanitize to the same slot, duplicate function declarations, and the valid string `__ZP_UNSET__`.

The second cause is that the new semantic-value contract is only partially consumed. Activation uses `RuntimeValue` for ordinary scalars, but PATH construction still parses raw `Entry.Value`, the literal decoder preserves some zsh escape syntax, and secret exclusion writes raw `Entry.Value`. Tests establish strong comparison mechanics over a narrow fixture domain, so the full suite passes without exercising these accepted inputs.

## Goal Achievement

### Re-verification of the three previous gaps

| Previous gap | Result | Evidence |
|---|---|---|
| Quoted aliases/scalars included source quote delimiters as data. | **CLOSED for the reported quote forms** | `RuntimeValue` is present through model/IR/store; `TestPipelinePreservesValuesAndFunctionsInLiveZsh` passes under real `zsh -f`. A broader escape-decoding gap remains. |
| Parsed function bodies did not reach `AddFunc`. | **CLOSED for ordinary unique functions** | `FunctionBody` is carried into `Manifest.Functions.Bodies`; `Diff` requires and emits bodies; the live pipeline test passes. Duplicate declarations expose a new restoration gap. |
| Residue test was fixed, shallow, and non-falsifiable. | **CLOSED mechanically, insufficient behaviorally** | The replacement runs 24 deterministic actions, records type-aware `${(qqqq)}` state, includes eight sensitivity checks and three renderer mutants. Its fixture generator omits every newly reproduced failure shape. |

### Eleven merged must-haves

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | Manifest v1 has the required scalar/list/alias/function/option shape and tri-state scalar representation. | **VERIFIED** | `core/model/manifest.go`; model tests and the full suite pass. |
| 2 | Builder turns accepted entries into one semantically faithful reversible manifest. | **FAILED** | `builder.go:35-75,104-143` appends repeated identities and derives PATH from raw source spelling with a cross-variable base set. |
| 3 | Diff is shell-agnostic, schema-gated, and places all deactivate operations before activate operations. | **VERIFIED** | `diff.go:9-32`; package imports remain shell-agnostic; tests pass. |
| 4 | Introspection captures exact bodies and degrades unavailable on source failure. | **FAILED** | NUL body capture works, but `introspect.go:26` ignores `source` status and `:71-93` interprets exact body marker lines as protocol sentinels. |
| 5 | Emit is the sole reverse-zsh path and its primitives are reversible over accepted names/values. | **FAILED** | Reverse tokens are currently confined to `emit.go`, but `emit.go:73-85,92-115,148-175,187-235` has colliding slots, ambiguous sentinels, and first-write applied state. |
| 6 | `shell.Emitter` exists and is injected at the composition root. | **VERIFIED** | `core/shell/provider.go`, `core/shell/zsh/zsh.go`, and `core/cmd/zsh-pro/main.go`. |
| 7 | Parsed literal scalar/alias values retain zsh runtime semantics through emit. | **FAILED** | `parse.go:318-339` copies `syntax.Lit.Value`; direct zsh comparison proves `hello\\ world` is preserved as a backslash-bearing value. |
| 8 | Parsed ordinary function bodies reach live zsh. | **VERIFIED** | `TestPipelinePreservesValuesAndFunctionsInLiveZsh` passes; direct source trace reaches `AddFunc.Body`. |
| 9 | The zero-residue property genuinely establishes path-independent cleanup for the accepted domain. | **FAILED** | Oracle mechanics are strong, but `residueManifests` uses unique identities, one list delta, PATH only, and non-sentinel data. Six direct adversarial cases fail while the property stays green. |
| 10 | Restoration is drift- and ownership-aware for accepted manifests. | **FAILED** | The ordinary single-write drift guard remains, but repeated scalar applied-state and repeated list ownership are incorrect. |
| 11 | Alias/function shadows restore exactly and PATH/FPATH are base-relative deltas. | **FAILED** | Legal name collisions, sentinel bodies, duplicate function declarations, quoted PATH, and repeated list deltas violate this truth. |

**Score:** 4/11 must-haves verified.

## Independent behavioral disconfirmation

One combined `zsh -f` harness used the real emitted helper semantics and produced:

```text
repeated_scalar=two
sentinel_scalar_present=0
alias_collision_dash=old-dash dot=<missing>
duplicate_function_present=0
repeated_path=/b:/base
source_escape=$'hello world'
decoded_escape=$'hello\\ world'
```

These observations independently establish six failures:

- two assignments to one scalar leave the profile's final value after deactivate;
- an original scalar whose value is `__ZP_UNSET__` is deleted;
- `foo-bar` and `foo.bar` collide in `ZP_PRIOR_ALIAS_foo_bar`, losing one shadow;
- duplicate declarations of the same function restore on the first pair and are removed by the second pair;
- two valid PATH assignments reset from base independently and lose `/a`;
- an unquoted zsh escape is emitted as data rather than decoded runtime content.

A separate direct probe of `zsh -f` with a nonexistent sourced file printed `AFTER_SOURCE` and exited with `STATUS:0`, confirming that the introspection script masks source failure.

## Additional critical data-flow findings

| Finding | Evidence | Impact |
|---|---|---|
| Secret semantic value is bypassed. | `secret.go:109-125` tests/stores `Entry.Value`, while `builder.go:81-99` correctly treats `RuntimeValue` as authoritative. | `export API_KEY="sk-abc"` stores quote characters; an explicitly empty secret is rejected. |
| Secret writes are not transactional. | `Commit` calls the mutating `excludeSecrets` at `store.go:216-224`, before marshal/regeneration and Git plumbing at `:226-301`. | A later failure can leave new/overwritten keychain or vault state even though no profile ref moves. |
| PATH data flow is split across incompatible contracts. | `builder.go:47` calls `pathDelta(name, e.Value)` while scalar activation uses `activationValue(e)`. | Supported quoting and semantic normalization are silently lost only for list variables. |

## Required artifacts and links

The artifact verifier found every declared artifact in plans 04-01 through 04-05 (18/18 declarations passed existence/substance checks). Manual data-flow tracing confirms these links exist:

| From | To | Via | Status |
|---|---|---|---|
| Parser/model semantic fields | Manifest builder | `ir.Build` copies `ValueMode`, `RuntimeValue`, and `FunctionBody`; `activate.Build` consumes them | **PARTIAL** — ordinary scalar/function flow works; PATH and secrets bypass semantic fields. |
| Manifest | Plan | `activate.Diff(active,target)` | **WIRED** — schema and ordering work; duplicate identities are not normalized. |
| Plan | live zsh | `zsh.Provider.Emit` | **WIRED BUT INCORRECT** — generated code runs, but the reproduced restoration cases fail. |
| Composition root | `shell.Emitter` | `zsh.Provider{}` | **WIRED**. |
| Residue oracle | real emitter | `Provider.Emit` plus scoped renderer mutants | **WIRED BUT UNDER-SAMPLED**. |

The automated key-link metadata checker could not resolve several plan links because their `from` fields contain component names or directories rather than relative file paths; those links were traced manually above.

## Automated checks

| Check | Result |
|---|---|
| `GOTOOLCHAIN=auto go test -count=1 ./...` | **PASS** — all packages; run once for this verification. |
| Named pipeline/introspection/residue/mutant tests | **PASS** under fresh execution. |
| Plan artifact verifier, 04-01 through 04-05 | **PASS**, 18/18 declarations. |
| Reverse-token source scan | Current production reverse tokens appear only in `core/shell/zsh/emit.go`; the invariant test itself is too narrow because it skips all zsh files except `regen.go`. |
| Human/external checks | None required; every failed behavior is locally reproducible. |

The green suite does not contradict the failed verdict: it proves the ordinary unique-name path and the oracle's sensitivity to its chosen mutants, not coverage of the emitter's accepted identity/value domain.

## Requirements

| Requirement | Status | Blocking evidence |
|---|---|---|
| SW-01 — switch/apply changes are generated and sourced for the selected profile. | **BLOCKED** | The emit path exists, but accepted profile values and repeated declarations are not applied/restored with faithful semantics. |
| SW-02 — switching/deactivation restores prior state with zero residue. | **BLOCKED** | Direct live-zsh repros leave a scalar and PATH residue and lose prior alias/function/scalar state. |

No orphan Phase 4 requirement IDs were found: both roadmap IDs are represented in `.planning/REQUIREMENTS.md`.

## Verdict

**GAPS FOUND — Phase 4 is not achieved.** The three originally reported gaps were materially addressed, but the stronger integration and property work exposed a common underlying defect: runtime ownership is tracked per emitted statement with lossy global slots instead of per final shell identity. Correct that state model first, then fix semantic decoding/PATH/secret consumers and broaden the property input domain. Phase 5 should not rely on Phase 4 switching until SW-01 and SW-02 re-verify against the reproduced cases above.
