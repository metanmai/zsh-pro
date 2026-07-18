---
phase: 04-manifest-builder-emit
verified: 2026-07-18T00:00:00Z
status: failed
score: 8/11
must_haves: 11
gaps:
  - "Profile values parsed from zsh source are not normalized before emit; quoted aliases and scalar assignments are emitted with quote characters as data."
  - "Function entries are reduced to names by activate.Build and AddFunc with an empty body emits no target function, so a parsed profile cannot apply its function definitions."
  - "The residue regression test is a fixed 20-cycle happy path with a five-field string snapshot; it is not random, does not snapshot full env/path contents/all six classes, and has no mutant tests."
---

# Phase 4: Manifest Builder + Emit — Verification

Verification was performed against the roadmap contract, both plan frontmatters, and the implementation. `SUMMARY.md` claims were treated as assertions, not evidence.

## Automated checks

- `GOTOOLCHAIN=auto go build ./...` — passed.
- `GOTOOLCHAIN=auto go test ./...` — passed.
- Targeted zsh tests (`TestEmit*`, `TestIntrospect*`, `TestReverseSyntaxHasSingleEmitHome`) — passed.
- Manual emitted-code checks under `zsh -f` — static/dynamic injection, drift guard, unset-vs-empty restoration, PATH base rebuild/ownership, and all three option prior-state cases passed.

## Roadmap success criteria

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | A resolved profile can be applied through emitted sourced/eval-able zsh, with reverse syntax confined to `core/shell/zsh/emit.go`. | **FAILED (BLOCKER)** | `Emit` exists, is syntactically valid, and reverse tokens are confined to the emitter. However, the actual parser → IR → `activate.Build` → `Emit` path is not behaviorally correct: parsing `alias gs='git status'` stores `Value: "'git status'"`; the emitted alias body becomes the literal `<'git status'>`, not `git status`. Parsed functions are also not applied (see truth 2). |
| 2 | Reverse-diff switching has zero residue under N random sequences and is path-independent/dupe-free. | **FAILED (BLOCKER)** | The only residue test is a fixed loop of 20 identical `zp_apply; zp_deactivate` cycles and compares `ZP_RESIDUE`, `$PATH`, one alias, one function, and one option. It has no random generator, full environment snapshot, ordered `$path` contents, dedicated option/alias sections, or mutated-emitter negative tests. A passing weak test cannot establish this roadmap truth. |
| 3 | Restore is drift- and ownership-aware. | **VERIFIED** | Manual `zsh -f` checks showed a hand-edited managed variable survives deactivate while an untouched variable restores; PATH rebuild retained a base-owned `/usr/local/bin` exactly once and removed profile-only `/opt/tool*`. `zp_restore_env` uses `${(P)+var}` and compares live value to applied value. |
| 4 | Shadowed aliases/functions are restored and PATH is represented as a base-relative delta. | **VERIFIED** | Emit captures prior alias/function bodies before overwrite with `${+slot}` guards; manual apply/deactivate restored prior alias/function bodies. Builder accepts only head/tail self-reference PATH forms and `Diff` emits distinct `RebuildListFromBase` and `ApplyListDelta` operations. |

## Plan must-haves and wiring

| Truth | Status | Evidence |
|---|---|---|
| Manifest v1 shape, Fork-A alias/function sets, and scalar unset/empty/value tri-state round-trip. | **VERIFIED** | `core/model/manifest.go`, `manifest_test.go`; `go test ./core/model` passed. |
| Builder uses `EffectiveManaged`, admits the intended categories, rejects unsafe names/PATH segments, and leaves runtime fields empty. | **VERIFIED** | `builder.go` and `builder_test.go`; hostile-name/path and forced-unmanaged tests passed. |
| Diff is shell-agnostic, schema-gated, and orders all deactivate operations before activate operations. | **VERIFIED** | `diff.go`, `plan.go`, schema/order tests passed; `core/activate` imports only `core/model`. |
| Introspection captures alias/function bodies with NUL framing, sorting, multiline support, and graceful zsh failure. | **VERIFIED** | `introspect.go` and body-boundary/integration tests passed. |
| Emit provides the sole reverse-zsh path, plain loader functions, injectable render seams, syntax validation, quoting, name validation, drift guard, shadow/option capture, slot cleanup, and PATH rebuild. | **VERIFIED** | `emit.go`; targeted syntax/injection/shadow tests passed; manual eval, drift, option, and PATH checks passed. Base capture placement is explicitly deferred to Phase 5. |
| `shell.Emitter` exists and is wired at the composition root. | **VERIFIED** | `core/shell/provider.go`, `core/shell/zsh/zsh.go`, and `core/cmd/zsh-pro/main.go`. |
| Parsed profile aliases/scalars preserve the semantics expected by emitted apply code. | **FAILED (BLOCKER)** | No integration test covers parser output into emitter. Reproduction: parser gives alias value `"'git status'"`; emitted apply sets `${aliases[gs]}` to `"'git status'"` including quote characters. Static quoted scalar assignments have the same issue. |
| Parsed function definitions are applied by the manifest/emitter path. | **FAILED (BLOCKER)** | `activate.Build` appends only function names to `FuncSet.Added`; `activate.AddFunc` has an optional body but `Diff` constructs it with only `Name`, and `emitActivate` emits no assignment when `Body == ""`. A parsed function therefore never reaches the target shell. |
| Zero-residue property test genuinely detects residue. | **FAILED (BLOCKER)** | `core/shell/zsh/residue_test.go` is 47 lines and contains no `rand`, `parameters`, `(@kv)`, `mutant`, or canary checks; it only compares a fixed five-field string. |

## Disconfirmation pass

The full suite passing is not sufficient evidence for the failed truths: the existing tests construct `activate.Plan` values directly with already-normalized alias bodies and manually supplied function bodies, bypassing the parser/IR/manifest integration that fails above. Likewise, the residue test exercises only one hand-written profile and cannot detect non-exported-variable, alias-body, option-state, PATH-order, or emitter-mutation residue.

## Verdict

**BLOCKED — Phase 4 is not achieved.** The emitter primitives and several safety invariants are present, but the profile-to-emitter path loses executable semantics for quoted source values and function bodies, and the required zero-residue property regression pin is absent. Phase 5 should not proceed until these gaps are closed and this verification is rerun.
