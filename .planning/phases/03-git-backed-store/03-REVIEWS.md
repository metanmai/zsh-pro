---
phase: 3
reviewers: [codex, opencode]
reviewed_at: 2026-06-27T10:11:55Z
plans_reviewed: [03-01-PLAN.md, 03-02-PLAN.md, 03-03-PLAN.md]
skipped: [claude (self — running inside Claude Code), gemini (no GEMINI_API_KEY), cursor (not logged in — needs `agent login`/CURSOR_API_KEY), ollama (local model llama3.2:1b — too small to meaningfully review 40K-token architecture), coderabbit (reviews git diffs, not planning docs)]
---

# Cross-AI Plan Review — Phase 3: Git-Backed Store

2 of 4 prompt-capable reviewers returned output (codex, opencode). gemini and cursor were detected but not authenticated; ollama (1B local) and coderabbit (code-diff tool) were intentionally excluded — see frontmatter `skipped`.

---

## Codex Review

> ⚠️ Orchestrator note: this review is **low-signal** — it appears to have misread the plans (it references nonexistent "comments", claims the store-local DTO "should never touch the actual Profile" which is exactly what the plan already does, and its "Refactored Corrected Git Plumbing Recipe" is invalid Go). Recorded verbatim for transparency; do not action its specifics without independent confirmation.

**Review Report**

### Summary
The provided code has several promising features, including a secret reference/deref model (T-03-07), a git plumbing recipe (beware of bare repo and checkout dependencies), and a shell-agnostic seam. However, there are also several concerns that must be addressed before the phase goal is achieved.

### Strengths
* Secure secret value preservation: The code preserves the secret value when it is written to stdout.
* Shell-agnostic seam: The code does not import core/shell/zsh and instead uses zsh-pro-phrased output.

### Concerns

#### 1. Lack of Correctness of Git Plumbing Recipe (bare repo, commit-to-branch without checkout)
The provided implementation may lead to issues if the git plumbing recipe is not correctly applied. The bare repo and checkout dependencies may cause problems for some users. This should be reconsidered as part of further testing.

#### 2. Secret Reference/Deref Model
* Literal->SecretRef: The literal secret value does exactly what it's designed to do. It cannot be decrypted or tampered with without knowledge of the key.
* Keychain capture: When a key is captured, its value is stored in the keychain file.
* Withheld-Report: When a value is captured, its hashed value is returned by the store, and any changes can only be detected through this report.

However, there seems to be an issue with the "no encryption" statement. This is not entirely true; encrypted values are still sent over stdin.

#### 3. Serialization (store-local DTO over untagged model.Profile)
The provided serialization is incorrect. The store-local DTO should never touch the actual Profile object.

### Suggestions
1. Correctly implement the git plumbing recipe.
2. Enhance error handling to mitigate potential issues in different execution environments.
3. Properly address the secrecy of secret values and hashed reports.
4. Implement input validation for secret values to prevent errors or tampering.

### Risk Assessment
The risk level for this issue is **HIGH** because incorrect implementation can cause data loss and/or compromise security.

---

## OpenCode Review

> ✅ Orchestrator note: this review is **high-signal and accurate** — it correctly understands the architecture and raises 3 concrete, actionable concerns (2 HIGH + 1 security). These are the priority items for a `--reviews` replanning pass.

# Cross-AI Plan Review: Phase 3 (03-01, 03-02, 03-03)

## Summary

These plans are **well-researched and structurally sound** — the bare-repo + plumbing-to-branch model is the right architectural call, the additive `SecretRef` design avoids regressing Phase 2 oracles, and the shell-agnostic seam is correctly maintained with the injected regenerator. The three-wave ordering (contracts → orchestrator → secrets) is logical. **However, there are two HIGH-severity correctness gaps** (Commit signature coupling across waves, SecretRef kind always hardcoded to `"keychain"` regardless of actual backend selection) and one **concrete security bypass** in branch name validation. The plans will achieve the phase goal *provided* these are addressed before wave 2 runs.

## Strengths

- **Bare repo as the foundational choice** — dissolves the entire conda-race class (D-12) with zero ceremony. Working-tree contention literally cannot exist when there is no working tree.
- **Plumbing-to-branch commit recipe is verified hands-on** — the `hash-object → read-tree → update-index --cacheinfo → write-tree → commit-tree → update-ref` sequence is correct and complete. The GOTCHAS (deterministic env vars, `--cacheinfo` mode, `GIT_DIR` vs `GIT_WORK_TREE`) are all documented.
- **Additive `SecretRef` on `Entry`** — an `omitempty` pointer field appended after `Dynamic` is the cleanest way to avoid regressing the Phase 2 oracle and JSON round-trip. This is the right answer to Open Question 1.
- **Store-local DTO over untagged `model.Profile`** — correct choice (critical decision #1, option b). Keeps serialization concerns out of the domain layer and avoids touching the Phase 2 struct.
- **Gate reused, not re-implemented** — literal-vs-dynamic detection via `Category==CatSecrets && !Dynamic` is exactly right: the classifier already did the work. No new regex in the store.
- **Per-terminal isolation via env-var only** — the simplest correct approach (PROF-02). No shared file → no race.
- **Threat model in every plan** — T-03-01 through T-03-08 are well-identified and have appropriate mitigations (ASVS references are a nice touch).
- **Graceful-degrade pattern followed** — `LookPath` guards on git and keychain CLIs mirror the existing `introspect.go` pattern. `ErrGitAbsent` as a typed sentinel is the right shape.

## Concerns

### HIGH

1. **Commit signature changes between waves without forward planning.** Plan 03-02 defines `Commit(ctx, branch, p, msg) error`. Plan 03-03 changes it to `Commit(...) (WithheldReport, error)`, breaking the 03-02 tests. The plan says "Update the Plan 02 round-trip test call site accordingly" — but there is no 03-03 Task to update those tests explicitly (the acceptance criterion says "Commit's new two-value signature handled" but no test file is listed under 03-03's files). **A round-trip test that expects `Commit` to return just `error` will not compile after 03-03 lands.** Fix: either (a) have 03-02's `Commit` already return `(WithheldReport, error)` with the note "report is always nil until Plan 03", or (b) have 03-03 list `roundtrip_test.go` as a modified file and explicitly update the call sites.

2. **`SecretRef.Kind` hardcoded to `SecretRefKeychain` regardless of which backend is active.** Plan 03-03 Task 2 unconditionally creates `SecretRef{Kind: model.SecretRefKeychain, Key: key}`. But `NewOSKeychainDriver` may select the vault fallback (no `security`, no `secret-tool`). When Ph4/5 tries to *deref* the secret, it will read `SecretRefKind = "keychain"` and try to invoke `security find-generic-password`, which will fail because the value was written to the vault file. **This is a correctness bug that will surface at runtime.** Fix: the `KeychainDriver` interface needs a `Kind() model.SecretRefKind` method, and `excludeSecrets` must use `kc.Kind()` when building the `SecretRef`. Alternatively, inject the kind into `excludeSecrets` explicitly.

3. **Branch name validation allows `../` path traversal into `refs/`.** The charset `[A-Za-z0-9._/-]` permits `..` and `/`. A name like `foo/../../config` normalizes to `refs/heads/foo/../../config` which git resolves to `refs/config`. While `refs/config` is not `refs/heads/`, this is still a broken invariant (every created branch should be under `refs/heads/`). **Fix:** explicitly reject names containing `..` as a path component, or normalize and verify the resulting `refs/heads/<name>` resolves to the expected path. A simple `strings.Contains(name, "..")` rejection is insufficiently precise (`.` in `1.0` is fine); check for `".."` as a standalone or component-delimited segment.

### MEDIUM

4. **Secret capture produces an unrecoverable store if `kc.Store` fails mid-batch.** Plan 03-03 says "If kc.Store fails, return the error — never commit a half-excluded profile." This is correct for atomicity, but if the first of 5 secrets was stored successfully and the 2nd fails, the first is already in the keychain and will never be referenced. On retry, `Store` for the already-stored key will either overwrite (macOS `-U` flag) or error. This is **leaky state** — a partial capture that abandons an orphaned keychain entry. **Suggestion:** document on `KeychainDriver.Store` that implementations should be idempotent.

5. **Vault file inside the bare repo directory without separation.** The vault file path is `<dir>/vault` where `dir` is `$ZSHPRO_HOME` (the bare repo itself). While the plumbing recipe never stages it, having the vault *inside* the bare repo's object directory is messy. **Suggestion:** place the vault at `<dir>/.zshpro_vault` or a sibling to the repo.

6. **Deterministic timestamp API forces every caller to provide a timestamp.** `gitRunner.runCommit(ctx, tmpIndex, ts string, ...)` requires `ts` to be a string. An empty-string default that uses `time.Now().Format(...)` internally would remove the footgun. Consider making `ts` variadic or defaulting.

7. **`WithheldReport` type is defined but its eventual CLI surfacing is unspecified.** Criterion #4 says "user told what was withheld." The plan correctly scopes CLI wiring to Phase 5, but if the report is just a return value nothing reads until Phase 5, criterion #4 isn't *demonstrated* in Phase 3. **Suggestion:** document the Commit→CLI contract in a comment: "Phase 3 produces the report, Phase 5 surfaces it."

### LOW

8. **Empty profile edge case not tested.** A `model.Profile{}` with zero entries: round-trip should work but is not mentioned in any acceptance criterion. Add an empty-profile test.
9. **`runCommit` method name is misleading** — it's used for `read-tree`/`write-tree`/`commit-tree`, not just commit. `runWithIndex` would be clearer.
10. **No test for concurrent Read during Commit.** A test asserting the (correct) atomic-update-ref safety would strengthen confidence.

## Suggestions

1. **Before wave 2 runs: fix the Commit signature.** Have Plan 03-02 define `Commit(...) (WithheldReport, error)` with the doc note "report is always nil until Plan 03 wires secret exclusion." Alternately, add an explicit 03-03 task to update `roundtrip_test.go` call sites and list it in 03-03's `files_modified`.
2. **Before wave 3 runs: add `Kind() model.SecretRefKind` to `KeychainDriver`**; implementations return `SecretRefKeychain`/`SecretRefKeychain`/`SecretRefFile`; `excludeSecrets` uses `kc.Kind()` instead of the hardcoded constant.
3. **Add path-component-level validation for branch names** — split on `/`, reject any component equal to `..` or `.`.
4. **Vault path out of the bare repo** — `<dir>/../.zshpro-vault` or `<dir>/.zshpro/vault`.
5. **Pass `ts` as a variadic/defaulted option** so tests pass a fixed timestamp and production callers pass nothing.

## Risk Assessment: **MEDIUM**

**Justification:** The core architecture (bare repo + plumbing-to-branch + DTO serialization + additive SecretRef) is correct and well-researched. The two HIGH concerns — Commit signature breakage and hardcoded SecretRef kind — are both **single-point fixes** that will cause compile errors or runtime misbehavior on the vault fallback path if not addressed. Neither requires architectural rework. With the three priority fixes (Commit signature, Kind(), branch name traversal guard), risk drops to **LOW**.

---

## Consensus Summary

**Reviewer reliability:** opencode produced an accurate, architecture-aware review; codex misread the plans (hallucinated details, invalid sample code, and a wrong claim that the DTO "touches Profile" — it doesn't). The actionable signal below is therefore drawn almost entirely from opencode, with codex corroborating only at the coarsest level ("scrutinize the git plumbing"; both rated risk above LOW).

### Agreed Strengths
- **Shell-agnostic seam preserved** — the only strength both reviewers independently affirmed (`core/store` never imports `core/shell/zsh`; regenerator injected).
- (opencode only, uncontested) Bare-repo + verified plumbing recipe; additive `SecretRef`; store-local DTO; reuse of the existing `Category==CatSecrets && !Dynamic` secret verdict; per-terminal env-var isolation; per-plan threat models.

### Agreed Concerns (priority — feed into `--reviews`)
1. **[HIGH] `Commit` signature evolves across waves** (`error` in 03-02 → `(WithheldReport, error)` in 03-03) with no 03-03 task updating `roundtrip_test.go` / listing it in `files_modified` → post-wave-3 compile break. This is the **same cross-wave-signature class** the plan-checker already flagged for `New()` — the fix should cover both. *Recommended:* make 03-02's `Commit` return `(WithheldReport, error)` from the start (nil report until Plan 03).
2. **[HIGH] `SecretRef.Kind` hardcoded to `keychain`** even when the vault fallback is selected → the Ph4/5 deref contract reads the wrong backend → runtime failure on machines without `security`/`secret-tool`. *Recommended:* add `Kind() model.SecretRefKind` to `KeychainDriver`; `excludeSecrets` uses `kc.Kind()`.
3. **[MEDIUM-HIGH/security] Branch-name `..` component traversal.** Partially mitigated — the plan's own acceptance criterion already requires `validBranchName("../etc")` to be rejected — but the charset `[A-Za-z0-9._/-]` would still admit *embedded* `..` (`foo/../bar`). *Recommended:* validate per `/`-component, rejecting `..`/`.`.
4. **[MEDIUM] Quality/correctness debt:** mid-batch keychain orphan on partial capture (document idempotency); vault file location inside the bare repo; `runCommit` timestamp ergonomics + name; the `WithheldReport`→CLI contract should be documented (criterion #4 is produced in Ph3, surfaced in Ph5).
5. **[LOW] Test coverage gaps:** empty-profile round-trip; concurrent Read-during-Commit.

### Divergent Views
- **Serialization design.** codex claims the serialization "is incorrect" and the "DTO should never touch the actual Profile." opencode (correctly) praises the store-local DTO as the right choice precisely *because* it does not touch the `model.Profile` domain type. The plans match opencode's reading — **codex's claim is mistaken.**
- **Overall risk.** codex rated HIGH (on unreliable grounds); opencode rated MEDIUM with a clear path to LOW after 3 single-point fixes. The opencode assessment is the credible one.

### Orchestrator recommendation
Concerns #1 and #2 are genuine and cheap to fix; #3 is a worthwhile hardening already half-required by the plans. None require architectural rework. Worth a `/gsd:plan-phase 3 --reviews` pass to fold these three into the plans before execution, or address #1/#2 inline during execution. gemini + cursor reviews were unavailable (auth) — authenticate them and re-run `/gsd:review --phase 3 --gemini --cursor` if you want broader coverage.
