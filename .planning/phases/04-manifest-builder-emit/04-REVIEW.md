---
phase: 04-manifest-builder-emit
reviewed: 2026-07-18T19:13:24Z
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
  critical: 9
  warning: 4
  info: 0
  total: 13
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-07-18T19:13:24Z  
**Depth:** standard  
**Files Reviewed:** 31  
**Status:** issues_found

## Narrative Findings (AI reviewer)

## Summary

The full Go suite passes, but direct zsh checks reproduced data loss and value corruption that the current fixtures do not cover. The highest-risk defects are non-injective restoration slots, repeated-identity handling, a data-colliding unset sentinel, incorrect decoding of unquoted escapes, and secret storage of source spelling rather than runtime data.

## Critical Issues

### CR-01: Unquoted escapes are retained as runtime data

**File:** `core/shell/zsh/parse.go:318-339`  
**Issue:** `decodeLiteralParts` copies every `syntax.Lit.Value` byte unchanged. mvdan/sh retains unquoted escape backslashes in that value, so `FOO=hello\ world` is decoded as `hello\ world` instead of the live zsh value `hello world`. `core/shell/zsh/dynamic_test.go:137-138` currently asserts the incorrect backslash-bearing result. The manifest then safely quotes the wrong value, making the corruption persistent. A direct zsh comparison produced `$'hello world'` for the source assignment and `$'hello\\ world'` for the decoded/emitted spelling.  
**Fix:** Decode unquoted `Lit` backslash pairs according to zsh rules (including line continuations), while keeping single-quoted text verbatim and applying the narrower double-quote escape rules inside `DblQuoted`. If a spelling is not modeled, return `ValueModeUnsupported`. Replace the tests with assertions against live zsh runtime values for escaped space, quote, backslash, glob, and newline cases.

### CR-02: Literal secrets are stored with shell quotes and escapes

**File:** `core/store/secret.go:104-129`  
**Issue:** The new semantic contract provides the decoded literal in `RuntimeValue`, but exclusion calls `kc.Store(key, e.Value)`, where `Value` is source spelling. For `export API_KEY="sk-abc"`, the backend receives `"sk-abc"` including quote characters; escaped literals are similarly stored with escape syntax, and `API_KEY=''` stores two quotes rather than an empty value. The current test at `core/store/secret_test.go:118-119` only checks `Contains`, so it masks this corruption.  
**Fix:** Require a valid literal contract (`ValueModeLiteral` with non-nil `RuntimeValue`) and store `*RuntimeValue`. Keep a narrowly documented legacy fallback only for old entries. Add exact-equality tests for single/double-quoted, escaped, multiline, and present-empty secrets.

### CR-03: Repeated scalar assignments cannot restore the original value

**File:** `core/shell/zsh/emit.go:148-150`  
**Issue:** `__ZP_APPLIED_<name>` is written only for the first `SetScalar`. A normal profile containing `FOO=one` followed by `FOO=two` leaves the applied slot at `one`; deactivation sees live `two`, treats it as drift, clears the capture slots, and leaves `FOO=two` behind. The builder emits every scalar entry (`core/activate/builder.go:38-44`), so this is reachable from ordinary source. A direct execution of the emitted helper logic ended with `$'two'` instead of the original `$'base'`.  
**Fix:** Capture the original value only once, but update the applied-value slot after every successful assignment so it always represents the profile's final owned value. Add a source-to-live-zsh test with repeated static and dynamic assignments to the same variable.

### CR-04: Sanitized alias/function slot names collide and drop shadows

**File:** `core/shell/zsh/emit.go:73-85,156-169,213-229`  
**Issue:** `sanitizeSlot` maps every punctuation character to `_`, while valid alias/function names may contain `.`, `-`, and `_`. Thus `foo-bar`, `foo.bar`, and `foo_bar` share one prior-state slot. Applying two such aliases captures only the first shadow; deactivation restores the first and loses the second. Direct zsh execution of the generated pattern restored `foo-bar=old-dash` but left `foo.bar` missing. Functions have the same collision.  
**Fix:** Use an injective identifier encoding (for example, hex-encode every name byte) or store prior bodies in associative maps keyed by safely quoted original names. Add collision pairs covering `.`, `-`, and `_` for both aliases and functions.

### CR-05: The unset sentinel destroys a legitimate original value

**File:** `core/shell/zsh/emit.go:95-115`  
**Issue:** `__ZP_UNSET__` is used both as user data and as the internal absence marker. If a variable originally equals that valid string, deactivate unsets it. The same data collision exists for alias and function shadows at `core/shell/zsh/emit.go:161-168,218-229`. A direct helper execution with `FOO=__ZP_UNSET__` ended with `FOO` absent.  
**Fix:** Represent presence separately from value (dedicated boolean slots or associative presence maps). Never encode absence as a string that can occur in user state. Add exact sentinel-valued scalar, alias, and function fixtures.

### CR-06: PATH deltas parse source spelling instead of shell semantics

**File:** `core/activate/builder.go:104-143`  
**Issue:** `pathDelta` splits the verbatim `Entry.Value`. A common `PATH="$HOME/bin:$PATH"` has quote characters around the first/last pieces, so no base marker matches and the managed PATH change is silently omitted. The single shared base-marker map also accepts `$PATH` as the base for `FPATH` (and `$FPATH` for `PATH`), then emits a delta against the wrong captured base.  
**Fix:** Add an AST-derived path-segment contract that distinguishes expansion segments from literal data and records the exact self-reference for the assigned variable. Build PATH only from PATH/path self-references and FPATH only from FPATH/fpath; reject cross-variable bases. Cover fully quoted, mixed quoted, and cross-variable cases in the production pipeline test.

### CR-07: Multiple PATH assignments discard earlier profile additions

**File:** `core/activate/builder.go:46-49`  
**Issue:** Every PATH statement becomes an independent `ListDelta`, while `renderList` resets the list to `ZP_BASE_*` for every delta (`core/shell/zsh/emit.go:21-35`). Source `PATH=/a:$PATH; PATH=/b:$PATH` evaluates to `/b:/a:<base>`, but the emitted apply path resets twice and produces `/b:<base>`, dropping `/a`.  
**Fix:** Compose same-list deltas in source order into one final delta, or reject sequences that cannot be faithfully composed. Add head/head, head/tail, and PATH/FPATH repeated-assignment integration cases.

### CR-08: Duplicate function declarations remove the restored prior function

**File:** `core/activate/builder.go:60-64`  
**Issue:** Repeated declarations append the same name multiple times to `Functions.Added` while `Bodies[name]` keeps only the last body. Deactivation therefore emits `UnsetFunc` plus `RestoreShadowedFunc` more than once (`core/activate/diff.go:54-56`): the first pair restores the pre-profile function and clears its slot, and the second pair unsets the just-restored function with no slot left to recover it.  
**Fix:** Keep `Functions.Added` unique while preserving the final body for each name, or make deactivation operations identity-unique. Add a profile with two declarations of the same function over a pre-existing shadow.

### CR-09: Secret backend mutation is not transactional with profile commit

**File:** `core/store/secret.go:88-129`  
**Issue:** Each secret is written to the global-by-name backend immediately. A later unsafe entry or backend failure returns an error after earlier keys have already been overwritten; failures later in the Git commit path have the same effect. The branch remains unchanged while an existing committed `SecretRef` can now resolve to the attempted new value, violating fail-closed behavior and potentially destroying the prior secret.  
**Fix:** Prevalidate the entire profile before any backend write, snapshot prior backend values, and roll back every write/delete if any later backend or Git operation fails. Prefer an explicit staged secret transaction owned by `Store.Commit`. Add a test with an existing value, one valid replacement, then a later forced failure, asserting both the branch and backend remain unchanged.

## Warnings

### WR-01: Introspection reports success when the target cannot be sourced

**File:** `core/shell/zsh/introspect.go:23-56`  
**Issue:** `source "$1"` is followed by dump commands regardless of its status. A missing or failing file therefore commonly exits the overall script successfully and returns `Available:true` with the shell's default state, contrary to the `shell.Introspector` contract. `core/shell/zsh/introspect_test.go:39-48` explicitly permits this false-success behavior.  
**Fix:** Gate the dump on successful sourcing and exit nonzero on failure; assert `Available:false` and a non-nil error for missing and syntactically failing files.

### WR-02: Exact sentinel lines inside function bodies corrupt identity sections

**File:** `core/shell/zsh/introspect.go:68-93`  
**Issue:** Bodies are correctly decoded separately with NUL framing, but the line scanner still interprets every exact `##ENV##`, `##PATH##`, or similar line inside the body region as a real section header. Subsequent body lines can then be inserted into `Env`, `Path`, or `Options`. The boundary test uses `## not a header`, not an actual sentinel.  
**Fix:** Parse the line-framed identity prefix only up to `##ALIASBODIES##`, then parse both body sections exclusively through the NUL-framed parser. Add heredoc/body lines equal to every sentinel.

### WR-03: The residue property omits the states that break restoration

**File:** `core/shell/zsh/residue_test.go:216-255`  
**Issue:** The two manifests use unique scalar names, one simple alias/function name, no sentinel-valued prior state, and no FPATH. Consequently the full-state oracle passes while CR-03 through CR-08 still leave residue or lose state. The meta-tests prove snapshot sensitivity, but they do not exercise these emitter inputs.  
**Fix:** Add repeated scalar/function identities, punctuation-collision aliases/functions, sentinel-valued prior state, repeated PATH deltas, and FPATH to the real-emitter sequence. Add focused mutants for slot collision and stale applied-value capture.

### WR-04: The reverse-syntax ownership test skips almost every zsh source file

**File:** `core/shell/zsh/invariant_test.go:10-35`  
**Issue:** For `core/shell/zsh`, the filter keeps only `regen.go`; it does not scan other non-emitter zsh files. Reverse tokens can therefore move into `parse.go`, `introspect.go`, or a new file without failing the claimed single-home invariant.  
**Fix:** Scan every non-test Go file under `core/shell/zsh` except `emit.go`, plus the agnostic directories already covered.

---

_Reviewed: 2026-07-18T19:13:24Z_  
_Reviewer: the agent (gsd-code-reviewer)_  
_Depth: standard_
