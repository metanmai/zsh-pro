---
phase: 03
slug: git-backed-store
status: secured
threats_open: 0
threats_closed: 10
asvs_level: 2
created: 2026-06-28
---

# Security Audit — Phase 3: Git-Backed Store

**Audited:** 2026-06-28
**Auditor:** gsd-security-auditor
**ASVS Level:** 2
**Block-on:** high
**Status:** SECURED — all declared mitigations verified present in implemented code

This audit verifies the threat register authored at plan time (across `03-01/02/03-PLAN.md` `<threat_model>` blocks). It does NOT scan for new vulnerabilities. Each `mitigate` threat is grounded in a specific `file:line` AND a passing adversarial test — not in the SUMMARY's claims. The `03-REVIEW.md` code review proved the first T-03-03 implementation leaked (CR-01/CR-02); this audit verifies the FIXED, fail-closed code.

---

## Verdict Summary

| Disposition | Count | Result |
|-------------|-------|--------|
| mitigate    | 8     | 8 CLOSED |
| accept      | 2     | 2 documented (see Accepted Risks) |
| **Total**   | **10**| **0 OPEN** |

Threats closed: **10/10**. No blockers. No unregistered flags.

---

## Threat Verification (mitigate)

| Threat ID | Category | Evidence (file:line + passing test) |
|-----------|----------|-------------------------------------|
| T-03-01 | Information Disclosure (git/keychain stderr leak) | `core/store/git.go:103-107` `mapGitError` discards `err`/`stderr`, returns `ErrGitCommand`; `core/store/keychain.go:334-336` `mapKeychainError` returns `ErrSecretBackendUnavailable` (no `SecKeychain`). `Read` clean probe → `ErrProfileNotFound`: `core/store/store.go:328-329`. Tests: `TestMapGitErrorNeverLeaksStderr` (asserts no `fatal:` / `not a git repository`), `TestErrStoreSentinelsArePhrased` (rejects `fatal:`/`git:`/`SecKeychain`/`security:`), `TestMacOSKeychainNotFoundIsPhrased` (ran on host — `security` present — asserts no `SecKeychain`). PASS. |
| T-03-02 | Tampering (branch-name argv injection / ref-path traversal; dynamic-value resolution at serialize) | `validBranchName` rejects empty, leading `-`, out-of-charset `[A-Za-z0-9._/-]`, AND per-`/`-component `..`/`.` (`core/store/store.go:353-371`, `isBranchRune:374-387`). Gates ALL four entry points: `Checkout:164`, `Create:178`, `Commit:212`, `Read:324`. Refs built only as `"refs/heads/"+name`; names passed as distinct argv via `gitRunner` (no shell string). DTO passes `Value` verbatim — no `util.ExpandHome` in `core/store/dto.go` (doc `:20-22`). Tests: `TestValidBranchName` (rejects `-rf`, `../etc`, `foo/../bar`, `a/./b`, `semi;colon`, `$(inject)`; accepts `v1.0`, `feature/x`), `TestMarshalNoHomeResolution` (`$HOME/go` literal survives, no `/Users/`). PASS. |
| T-03-03 | Information Disclosure (secret literal committed to the tree) — **CRITICAL, was found BROKEN (CR-01/CR-02), fixed fail-closed** | `excludeSecrets` (`core/store/secret.go:72-159`) runs BEFORE marshal/commit (`store.go:221`). **Single-name scalar literal** → captured to backend + `SecretRef{Kind:kc.Kind()}` + literal cleared from BOTH `Text` AND `Value` (the Rule-1 fix: clearing only `Value` leaked via the JSON `text` field + Regenerator fallback) (`secret.go:133-153`). **Multi-name / non-assignment** → `isExcludableSecretShape` false → `ErrUnsafeSecretShape`, Commit aborts, nothing written (`secret.go:93-101`, `189-193`). **Single-name array (empty `Value`, literal in `Text`)** → `ErrUnsafeSecretShape` (`secret.go:107-113`). Backend-Store failure or nil backend on a literal → abort (`secret.go:116-127`). Fix commit `c663e62` present. Tests: `TestCommitFailsClosedMultiNameSecretWithDynamicSibling` (CR-01: `export PATH=$HOME/bin API_KEY=sk-LEAKED…`), `TestCommitFailsClosedMultiNameSecretWrongKeyOrder` (CR-02: `export OTHER=foo API_KEY=…`), `TestCommitFailsClosedArraySecret` (`export SECRETS=(sk-arr-leak)`) — each asserts ref-unmoved + no `profile.json` blob + nothing captured; `TestCommitExcludesLiteralSecret` asserts literal absent from `profile.json` AND `profile.zsh` + recoverable from backend + reported + `SecretRef.Kind==file`. Vault `.gitignore`'d (`.gitignore:10`) + sibling-of-repo (`keychain.go:221-223`) + plumbing stages only profile.json/profile.zsh (`store.go:271-278`). All PASS. |
| T-03-04 | Tampering / Info Disclosure (shared-HEAD working-tree contention; conda race) | Bare repo by construction: `git init --bare -b main` (`store.go:110`); Commit moves refs via object DB only (temp index → write-tree → commit-tree → update-ref, `store.go:243-301`); Read via `git show <branch>:profile.json` (`store.go:337`). Zero `git status`/`checkout`/`reset` — grep `"(status\|checkout\|reset)"` in store.go → none. Test: `TestInitIdempotent` asserts `isBareRepo` true (no working tree). PASS. |
| T-03-06 | Information Disclosure (secret value to ps/argv during keychain store) | macOS `Store` pipes value via `cmd.Stdin` doubled form, arg list ends in `-w` with no value element (`keychain.go:88-94`); linux `Store` pipes via `cmd.Stdin` to `secret-tool store` (`keychain.go:151-157`); value never logged. Tests: `TestMacOSKeychainRoundTrip` (ran on host) Store/Retrieve/Delete via stdin. PASS. |
| T-03-07 | Tampering / Elevation (command injection via secret/entry value) | Values flow as data only: keychain via stdin (`keychain.go:94,157`), git argv as distinct elements via `gitRunner` (`git.go:46,61,80`); the store never `eval`s or shell-interpolates a value and never sources user config (deref/emit is Ph4/5). Commit message passed as distinct `-m` argv (`store.go:289-291`). No `exec` with a shell (`sh -c`) anywhere in `core/store`. Verified by reading every subprocess call site. CLOSED. |
| T-03-08 | Tampering (vault world-readable or inside repo) | `vaultKeychain.save` writes `0o600` (`keychain.go:323`); path is `filepath.Join(filepath.Dir(repoDir), vaultFileName)` — sibling, not inside the bare repo (`keychain.go:221-223`). Test: `TestVaultRoundTrip` asserts `Mode().Perm()==0o600` AND vault dir is the repo's sibling, not the repo dir. PASS. |
| T-03-09 | Information Disclosure (SecretRef stamped with wrong backend kind) | `excludeSecrets` builds `SecretRef{Kind: kc.Kind()}` from the ACTIVE driver, never a hardcoded constant (`secret.go:133`); `! grep 'Kind: model.SecretRefKeychain' secret.go` holds. Vault fallback `Kind()→SecretRefFile` (`keychain.go:227`), macOS/linux `Kind()→SecretRefKeychain` (`keychain.go:81,146`). Tests: `TestKindPerBackend` (vault→file, OS→keychain), `TestCommitExcludesLiteralSecret` step (d) asserts read-back `SecretRef.Kind==SecretRefFile` on the vault path. PASS. |

---

## Accepted Risks (accept)

| Threat ID | Category | Disposition | Justification verified |
|-----------|----------|-------------|------------------------|
| T-03-05 | Tampering (non-deterministic commits) | accept (low) | Fixed `commitTS = "1700000000 +0000"` (`store.go:31`) stamped via `runCommit`'s `GIT_AUTHOR_DATE`/`GIT_COMMITTER_DATE` + fixed author/committer identity (`git.go:81-90`). Tests assert on CONTENT (`Read`→`reflect.DeepEqual`), not SHAs; `TestGitCommitToBranch` additionally pins that a fixed timestamp yields a byte-stable SHA across two repos. **Accept is reasonable**: residual risk is only that two semantically-different commits could share metadata, which has no security impact (content is the contract). CLOSED as documented accepted risk. |
| T-03-SC | Tampering (npm/pip/cargo / new Go modules) | accept (N/A) | `go.mod` declares only `mvdan.cc/sh/v3 v3.13.1`; no new module added (last `go.mod`/`go.sum` change predates Phase 3 — commits `69dda7d`/`9d97de7`). No package-manager installs in any Phase 3 file. **Accept (N/A) is correct.** CLOSED as documented accepted risk. |

---

## Architecture Invariant Checks (corroborating, not separate threats)

- **Shell-agnostic store (seam):** `go list -deps ./core/store/` shows **0** transitive deps on `core/shell/zsh` — the production store package is genuinely shell-free. The `grep -rl 'core/shell/zsh' core/store/*.go` substring hits in `secret.go` are **doc-comment references only** (lines 9, 12), not imports — confirmed by reading the import block (`secret.go:28-33` imports only `context`, `fmt`, `core/model`). The only real `core/shell/zsh` importers in `core/store` are the two sanctioned external/internal test files (`roundtrip_test.go`, `secret_test.go`) that wire `zsh.Provider{}` solely to build authentic fixtures.
- **Sole composition root:** `core/cmd/zsh-pro/main.go` is the only non-test file that imports both `core/shell/zsh` and the concrete store drivers, wiring `store.New(storeDir(), zsh.Provider{}, NewOSKeychainDriver(dir))` (`main.go:19-28`); store-init error is non-fatal so the read-only analyze path never crashes.
- **Full suite green:** `go test ./...` passes (Phase 2 regression pins — `TestRegenRoundTrip`/`TestOracleProperty`/`TestCorpusGolden`/`TestEffectiveManaged` — not regressed); `go vet ./core/store/` clean. Host has `security`, so the macOS keychain tests RAN (not skipped).

---

## Unregistered Flags

None. No `## Threat Flags` section exists in any Phase 3 SUMMARY, and no new attack surface was found during verification that lacks a threat mapping. The plan-time cross-AI review concerns (`03-REVIEWS.md` #1 Commit signature, #2 SecretRef.Kind, #3 branch-name traversal) were all folded into the plans and are verified implemented (see T-03-02 and T-03-09 above; `Commit` is `(WithheldReport, error)` from Plan 02 with no cross-wave signature evolution: `store.go:211`).

---

## Notes for Downstream Phases

- T-03-03 is mitigated **fail-closed**: a multi-name or array-valued `CatSecrets` assignment aborts the Commit with `ErrUnsafeSecretShape` (the user must split the secret into its own `NAME=value` statement). Phase 5's CLI must surface this error clearly, and the `WithheldReport` (produced here, consumed in Ph5) to tell the user what was withheld.
- SecretRef deref (Ph4/5) MUST honor `SecretRef.Kind` — a `file`-kind reference resolves from the vault, a `keychain`-kind from the OS backend. The stamp is authoritative; do not assume `keychain`.
