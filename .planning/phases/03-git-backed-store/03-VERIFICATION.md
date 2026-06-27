---
phase: 03-git-backed-store
verified: 2026-06-27T00:00:00Z
status: passed
score: 11/11 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  note: initial verification (no prior VERIFICATION.md)
---

# Phase 3: Git-Backed Store Verification Report

**Phase Goal:** Make profiles real and switchable. Store the regenerated per-category `.zsh` as a git repo via the `git` binary (no new dependency), with branch = profile and the baseline branch holding the ingested `~/.zshrc`. Expose create / list / switch typed in terms of `model.Profile`, track the active profile per-terminal (never a shared global file), and keep detected secrets out of the committed tree by default.
**Verified:** 2026-06-27
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth (Success Criterion) | Status | Evidence |
|---|---------------------------|--------|----------|
| 1 | Store initializes a git repo via the `git` binary (subprocess + 5s timeout, mirroring `zsh -f`); no new Go module; absent git degrades with a clear error, not a crash | VERIFIED | `core/store/git.go:42-53` `gitRunner.run` = `context.WithTimeout(ctx, gitTimeout=5s)` + `defer cancel()` + `exec.CommandContext(ctx,"git",...)` + separate buffers (verbatim introspect.go shape). `newGitRunner` LookPath guard → `ErrGitAbsent` (git.go:31-36). `go.mod` unchanged (only `mvdan.cc/sh/v3 v3.13.1`). Independent harness: `New` with empty PATH returns `ErrGitAbsent` ("git is not installed…"), no panic. `Init` is idempotent (store.go:104). |
| 2 | User can create / list (= branches) / switch (checkout); Read/Commit round-trip a `model.Profile` | VERIFIED | `Create` forks main (store.go:177), `Branches` sorted via for-each-ref (132), `Checkout` validates existence (163), `Commit` plumbing-to-branch no checkout (211), `Read` via `git show` (323). `TestRoundTripReadCommit` PASS: `reflect.DeepEqual(Read(Commit(p)),p)` over all 5 declarative classes + an OverrideManaged. `TestRoundTripComposeOracle` PASS (RAN, not skipped — git+zsh present): regenerated profile.zsh re-sources byte-identical IdentitySet vs original, with anti-vacuity guards. |
| 3 | Active profile tracked per-terminal (env-var `ZSHPRO_PROFILE`), never a shared global file | VERIFIED | `Current()` = `os.Getenv("ZSHPRO_PROFILE")`, unset⇒"main" (store.go:151-156). No `os.Setenv` of it in non-test code (export is Phase 5). The only file writes in non-test store code are the secret vault (0600 sibling, git-ignored) and a temp git index — NO shared "current profile" file anywhere. `TestCurrent` PASS. |
| 4 | Detected secrets excluded from the committed tree by default; user told what was withheld | VERIFIED | `excludeSecrets` runs FIRST in `Commit` body (store.go:221); downstream marshals `excluded` not `p`. Single-name literal → captured to backend + SecretRef (Kind=kc.Kind()) + literal cleared from BOTH Text and Value → no leak in profile.json OR profile.zsh; `WithheldReport` names it. Independently re-confirmed (see Secret-Leak Fix below). |
| 5 | No new Go dependency added (PROF-01) | VERIFIED | `go.mod` still declares only `mvdan.cc/sh/v3 v3.13.1`. SUMMARYs' `tech-stack.added: []`. git/keychain via the binary (subprocess), vault via stdlib `os`. |
| 6 | `core/store` non-test files do NOT import `core/shell/zsh` (shell-agnostic seam) | VERIFIED | `go list -deps zsh-pro/core/store` → shell family is only `zsh-pro/core/shell` (the interface), NOT `core/shell/zsh`. `go list .Imports` per package: the SOLE non-test importer of `core/shell/zsh` is `zsh-pro/core/cmd/zsh-pro` (composition root). The grep hits in `core/store/secret.go` are comment mentions (lines 9, 12), confirmed not imports. |
| 7 | `model.SecretRef` is additive + dependency-free; serializes as `{kind,key}` | VERIFIED | `core/model/secretref.go` zero imports; `SecretRef{Kind,Key}` json-tagged kind/key; 3 kinds (keychain/file/cmd). `Entry.Secret *SecretRef` omitempty appended after Dynamic. `TestSecretRefMarshalShape`/`Kinds`/`RoundTrip`/`TestEntryOmitsSecretWhenNil` PASS. |
| 8 | `model.Profile`↔JSON serialization is lossless + deterministic via store-local DTO | VERIFIED | `core/store/dto.go` (entryDTO/profileDTO; imports only encoding/json + core/model). `TestRoundTripLossless`, `TestMarshalDeterministic`, `TestMarshalTrailingNewline`, `TestMarshalNoHomeResolution` ($HOME/go literal survives), `TestRoundTripSecretRef` all PASS. |
| 9 | Read(Commit(p)) round-trip pin present and composed with Phase 2 byte-identical oracle | VERIFIED | `core/store/roundtrip_test.go` (external `store_test`, the only store test wiring zsh.Provider). 3 properties pinned (full profile, empty profile, oracle compose). All 3 PASS and RAN (git+zsh present). |
| 10 | Phase 2 regression pins stay green (additive-only, not regressed) | VERIFIED | `TestRegenRoundTrip` (ir), `TestOracleProperty`+`TestFuzzSurvival` (testgen), `TestCorpusGolden` (analyze), `TestEffectiveManaged` (model) all PASS on fresh runs. |
| 11 | PROF-03 correctly marked "in progress / started Phase 3" in REQUIREMENTS.md (NOT done) | VERIFIED | REQUIREMENTS.md line 24 PROF-03 is `[ ]` unchecked with the D-07–D-10 narrative; traceability line 84 = "In progress (started Phase 3)". PROF-01/02 are `[x]` Complete. Exactly the required state. |

**Score:** 11/11 truths verified

### Secret-Leak Fix Verification (Success Criterion #4 — flagged critical)

The 03-REVIEW.md found and fixed 2 BLOCKER secret-leak bugs (CR-01 multi-name with dynamic sibling leaks; CR-02 wrong-key-order captures under the wrong key) via `isExcludableSecretShape` + `ErrUnsafeSecretShape` (commit `c663e62`). **Confirmed the fix HOLDS** — both by the committed adversarial tests AND by an independent verifier-authored harness (created, run, then removed; git tree left clean):

| Adversarial shape (genuine detected secret) | Required behavior | Committed test | Independent harness |
|---|---|---|---|
| `export PATH=$HOME/bin API_KEY=…` (multi-name, dynamic sibling — CR-01) | Fail closed `ErrUnsafeSecretShape`, nothing written, not captured | `TestCommitFailsClosedMultiNameSecretWithDynamicSibling` PASS | FAILED CLOSED, ref unmoved, not captured |
| `export OTHER=foo API_KEY=…` (wrong key order — CR-02) | Fail closed, not captured under wrong key | `TestCommitFailsClosedMultiNameSecretWrongKeyOrder` PASS | FAILED CLOSED, OTHER/API_KEY both absent from vault |
| `export API_KEYS=(…)` (array secret — leak vector beyond review) | Fail closed (literal hides in Text, empty Value) | `TestCommitFailsClosedArraySecret` PASS | FAILED CLOSED |
| `export API_KEY="sk-…"` (single literal) | Excluded; no literal in profile.json OR profile.zsh; captured; reported | `TestCommitExcludesLiteralSecret` PASS | No leak in either blob, captured, reported `[{API_KEY 1}]` |

`excludeSecrets` (secret.go:72-159) fails closed at three points: non-excludable shape → `ErrUnsafeSecretShape` (line 100); single-name with empty scalar Value (array) → `ErrUnsafeSecretShape` (line 112); nil backend on a literal → `ErrSecretBackendUnavailable` (line 121); a `kc.Store` failure aborts (line 126). The literal is cleared from BOTH `Entry.Text` AND `Entry.Value` (lines 147-153) — the deeper leak path (Text is the JSON "text" field and the Regenerator's empty-Value fallback). `Commit` returns `nil, err` before any ref moves on abort. No regression — the goal-failure scenario the prompt flagged is not present.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `core/model/secretref.go` | SecretRef value type, additive, zero imports | VERIFIED | Zero imports; SecretRef + SecretRefKind + 3 kinds |
| `core/model/profile.go` | One additive `Secret *SecretRef` omitempty field | VERIFIED | Appended after Dynamic; Phase 2 order preserved (oracles green) |
| `core/store/dto.go` | Lossless deterministic Profile↔JSON | VERIFIED | entryDTO/profileDTO; nil-Entries preserved (empty round-trip); imports only encoding/json + core/model |
| `core/store/git.go` | gitRunner subprocess driver + plumbing primitives | VERIFIED | 5s timeout, LookPath guard, mapGitError no-stderr, emptyTreeSHA, all primitives present |
| `core/store/errors.go` | zsh-pro-phrased typed sentinels incl. ErrUnsafeSecretShape | VERIFIED | errStore type; all sentinels start "zsh-pro:"; ErrUnsafeSecretShape present |
| `core/store/store.go` | Store + final New(dir,regen,kc) + Init/Branches/Current/Checkout/Create/Commit(2-value)/Read + KeychainDriver + WithheldReport + validBranchName | VERIFIED | 398 lines; all present; Commit calls excludeSecrets; no checkout/status/reset |
| `core/store/secret.go` | excludeSecrets (D-08 over Entry fields, kc.Kind() stamp, fail-closed) | VERIFIED | 194 lines; no regex/AST; isExcludableSecretShape gate; dual-field clear |
| `core/store/keychain.go` | 3 concrete drivers + Kind() each + LookPath selector + 0600 vault | VERIFIED | macOS/linux=keychain, vault=file; value via stdin (off argv); vault base64 (WR-01) + sorted (WR-02) + 0600 sibling; NewOSKeychainDriver never nil |
| `core/store/roundtrip_test.go` | Read(Commit(p)) + empty + oracle compose | VERIFIED | external store_test; 3 props PASS |
| `core/cmd/zsh-pro/main.go` | Composition-root store wiring | VERIFIED | store.New(storeDir(), provider, NewOSKeychainDriver(dir)); storeDir() D-04; sole core/shell/zsh importer; store-init non-fatal |
| `.gitignore` | Vault-file ignore | VERIFIED | `.zsh-pro-vault` entry present with comment |
| `.planning/REQUIREMENTS.md` | PROF-03 started-Phase-3 traceability | VERIFIED | PROF-03 `[ ]` + "In progress (started Phase 3)" |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `store.go` Commit | `secret.go` excludeSecrets | first body step before marshal | WIRED | store.go:221; downstream uses `excluded` not `p` |
| `store.go` Commit | `git.go` plumbing | hash-object/read-tree/update-index/write-tree/commit-tree/update-ref | WIRED | store.go:228-301; no working-tree verbs |
| `store.go` Commit | `ir.Regenerate` | profile.zsh derived view | WIRED | store.go:232 `ir.Regenerate(excluded, s.regen)` |
| `store.go` Current | `ZSHPRO_PROFILE` env | os.Getenv, unset⇒main | WIRED | store.go:152 |
| `secret.go` | `keychain.go` KeychainDriver | kc.Store + kc.Kind() | WIRED | secret.go:123,133 |
| `main.go` | store.New + zsh.Provider + keychain | composition root | WIRED | main.go:26-28 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| Commit → committed tree | profile.json/profile.zsh blobs | MarshalProfile(excluded) + ir.Regenerate(excluded) | Yes — real serialized IR + regenerated zsh, written via hash-object to object DB | FLOWING |
| Read → model.Profile | profile.json bytes | git show <branch>:profile.json → UnmarshalProfile | Yes — round-trip pin proves DeepEqual | FLOWING |
| excludeSecrets → vault | captured secret value | kc.Store(key, value) | Yes — independent harness Retrieve returns captured literal | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command / Method | Result | Status |
|----------|------------------|--------|--------|
| Whole module compiles | `go build ./...` | exit 0 | PASS |
| Full suite + lint | `make check` | go vet ok, golangci-lint 0 issues, go test all ok | PASS |
| core/store fresh (not cached) | `go test -count=1 ./core/store/` | ok 4.291s | PASS |
| Binary runs with store wired (non-fatal) | built + `zsh-pro analyze --json` | valid JSON, exit 0, no crash | PASS |
| Absent-git degrade | harness New with empty PATH | ErrGitAbsent, no panic | PASS |
| Secret no-leak (single literal) | harness Commit + git show both blobs | literal absent from json AND zsh, captured | PASS |
| Fail-closed (multi-name/wrong-order/array detected secrets) | harness Commit | ErrUnsafeSecretShape, ref unmoved, not captured | PASS |
| No-deps invariant | `go list -deps core/store \| grep shell/zsh` | 0 (only core/shell interface) | PASS |

### Probe Execution

No conventional `scripts/*/tests/probe-*.sh` and no probe declarations in the PLANs. This phase is Go-test driven; behavioral spot-checks above substitute. (`find scripts -path '*/tests/probe-*.sh'` → none.)

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PROF-01 | 03-01, 03-02 | git-backed via the git binary, no new dep, branch = profile | SATISFIED | go.mod unchanged; gitRunner mirrors introspect.go; `[x]` Complete in REQUIREMENTS.md |
| PROF-02 | 03-02, 03-03 | create/list/switch; per-terminal env-var state, never a global file | SATISFIED | Create/Branches/Checkout/Current; ZSHPRO_PROFILE read-only, no global file; `[x]` Complete |
| PROF-03 | 03-03 | secrets excluded by default, told what was withheld; ELEVATED to *start* in Ph3 | SATISFIED (correctly partial) | Store-side exclusion landed; `[ ]` unchecked with "In progress (started Phase 3)" — completes Ph4/5/Ph6 by design, not a gap |

No orphaned requirements: every ID in the three plans' frontmatter (PROF-01, PROF-02, PROF-03) is accounted for in REQUIREMENTS.md with the expected status.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX debt markers in any phase-modified non-test file | — | Debt-marker gate clean |
| (none) | — | No TODO/HACK/PLACEHOLDER markers; no empty-impl stubs; no panic() in non-test store code | — | The `placeholder` symbol in secret.go is a real secret-substitution feature, not a stub |

### Human Verification Required

None. All four success criteria were verifiable programmatically against the codebase (git/zsh/security all present on this machine, so the round-trip oracle, macOS keychain, and secret-exclusion tests RAN rather than skipped). The remaining runtime surface (env-var export-on-switch, SecretRef deref, CLI verbs) is explicitly Phase 4/5/6 scope per CONTEXT D-10/D-13 and is not part of this phase's contract.

### Gaps Summary

No gaps. All 11 must-haves VERIFIED. The build, full test suite, and lint are green on the current `main` checkout (`33dc13b`). No new Go dependency was added. `core/store` is shell-agnostic (verified via `go list -deps`, not grep). The Phase 2 byte-identical oracle and the Read(Commit(p)) round-trip pin are present and green. Most importantly, the 2 FIXED secret-leak blockers (CR-01/CR-02, plus the array vector found beyond the review) were independently re-confirmed to fail closed with `ErrUnsafeSecretShape` and to never leak a literal into profile.json or profile.zsh — the `TestCommitFailsClosed*` tests exist and pass. PROF-03 is correctly tracked as "in progress / started Phase 3", not falsely marked complete.

---

_Verified: 2026-06-27_
_Verifier: Claude (gsd-verifier)_
