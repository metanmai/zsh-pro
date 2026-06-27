---
phase: 03-git-backed-store
reviewed: 2026-06-27T00:00:00Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - core/cmd/zsh-pro/main.go
  - core/model/profile.go
  - core/model/secretref.go
  - core/model/secretref_test.go
  - core/store/dto.go
  - core/store/dto_test.go
  - core/store/errors.go
  - core/store/git.go
  - core/store/git_test.go
  - core/store/keychain.go
  - core/store/keychain_test.go
  - core/store/roundtrip_test.go
  - core/store/secret.go
  - core/store/secret_test.go
  - core/store/store.go
  - core/store/store_test.go
findings:
  critical: 2
  warning: 3
  info: 2
  total: 7
status: resolved
resolved: 2026-06-27
---

# Phase 3: Code Review Report

**Reviewed:** 2026-06-27
**Depth:** standard
**Files Reviewed:** 16
**Status:** resolved (all findings fixed — see Resolution below)

## Resolution (2026-06-27)

All confirmed findings fixed and verified green (`make check`: 0 lint issues, full suite incl. Phase 2 oracle + round-trip pins). Each fix is backed by an adversarial test that fails pre-fix and passes post-fix.

| ID | Fix | Commit |
|----|-----|--------|
| CR-01 / CR-02 | `excludeSecrets` fails closed via `isExcludableSecretShape` — only single-name scalar `CatSecrets` assignments are excludable; any multi-name / non-scalar (incl. array `export ARR=(...)`, a leak vector found beyond the review) aborts the Commit with `ErrUnsafeSecretShape`, never leaking. Single-name literal still excluded; single-name dynamic still verbatim (D-08). | `c663e62` |
| WR-01 / WR-02 | Vault stores `key=base64(value)` (newline-safe) in sorted-key order (deterministic). | `d6b01e2` |
| WR-03 | `Read` distinguishes an absent branch (`ErrProfileNotFound`) from a present-but-uncommitted branch (empty `model.Profile{}`). | `ddcb8e0` |
| IN-02 | Shallow-copy comment corrected (no behavior change). | `9767e92` |
| IN-01 | Deferred (belt-and-suspenders post-commit blob scan) — the fail-closed CR fix is the substantive protection. | — |

Tests: `TestCommitFailsClosedMultiNameSecretWithDynamicSibling`, `TestCommitFailsClosedMultiNameSecretWrongKeyOrder`, `TestCommitFailsClosedArraySecret`, `TestVaultMultiLineRoundTrip`, `TestVaultSaveDeterministicOrder`, `TestReadDistinguishesAbsentFromUncommitted`.

---

### Original findings (as-found, now resolved)

## Summary

This phase delivers a git-backed, branchable profile store with literal-secret exclusion into an OS keychain / 0600 vault. The git plumbing (bare repo, hash-object → temp index → write-tree → commit-tree → update-ref), the determinism of the committed tree (fixed timestamp + struct-order JSON), the layering constraint (no `core/shell/zsh` import in non-test store files — verified by grep and golangci-lint), the argument-injection/path-traversal branch-name guard, the zsh-pro-phrased error mapping (no raw git/keychain stderr leak), secrets-via-stdin (never argv), and the 0600 vault perms are all implemented correctly and well-tested. Build, `go vet`, `golangci-lint` (0 issues), and the existing suite are green.

However, the central security guarantee of this phase — "no literal secret value enters the committed git tree" (threat T-03-03) — is **broken for multi-name assignments**, and I proved a live secret leak into both committed blobs against the real `Commit` path. The root cause is that `excludeSecrets`/`isLiteralSecret` reason per-`Entry` using `Names[0]`/`Value`/`Dynamic`, but the parser collapses a multi-name assignment so those fields describe the *whole* statement (the literal often lives only in `Entry.Text`, and `Dynamic` reflects any segment). This yields a secret leak in one shape and a wrong-key / data-loss capture in another. These are BLOCKERs because secret exclusion is the reason this phase exists.

Three lower-severity issues (vault map-iteration non-determinism, silent newline corruption of multi-line secrets in the vault, and a `Read`-vs-`Branches` "exists" inconsistency) and two info items round out the findings.

All test scaffolding I added to prove findings was removed; no source files were modified.

## Critical Issues

### CR-01: Literal secret in a multi-name assignment with a dynamic sibling leaks verbatim into the committed tree (T-03-03)

**File:** `core/store/secret.go:130-132` (with root in the parser's multi-name `Value`/`Dynamic` collapse)

**Issue:** `isLiteralSecret` is `Category==CatSecrets && !e.Dynamic && e.Value != "" && len(e.Names) > 0`. For a multi-name assignment where any segment is dynamic, the parser sets `Entry.Dynamic = true` for the *entire* entry, so the `!e.Dynamic` guard is false and the entry bypasses exclusion entirely — even though a *different* segment is a static secret literal. The literal then commits verbatim via the `Managed=false` → verbatim-`Text` path into **both** `profile.json` and `profile.zsh`, and the `WithheldReport` is empty (the user is never told).

Proven against the real `s.Commit` path:

Input: `export PATH=$HOME/bin API_KEY=sk-LEAKED-SECRET-123`
- Parser yields `Names=[PATH, API_KEY]`, `Value="sk-LEAKED-SECRET-123"`, `Dynamic=true` (because of `$HOME`), `Category=secrets`, `Managed=false`.
- `isLiteralSecret` → false (the `!Dynamic` guard fires) → no exclusion.
- Committed `profile.json` contains `"text": "export PATH=$HOME/bin API_KEY=sk-LEAKED-SECRET-123"` and committed `profile.zsh` contains the same line. `WithheldReport=[]`.

This is exactly threat T-03-03 (a literal secret in the committed git tree), the defect this phase exists to prevent.

**Fix:** Secret exclusion cannot safely reason at `Entry` granularity for multi-name assignments, because `Value`/`Dynamic` are whole-statement properties while the literal can live in any segment of `Text`. Options, in order of robustness:
1. Treat any `CatSecrets` assignment that is NOT exactly one name + one scalar value (i.e. anything `routeManaged` would NOT manage as a clean `NAME=VALUE`) as **un-excludable** and refuse the Commit with a clear `zsh-pro: cannot safely store a secret in a multi-assignment; split it into its own statement` error rather than committing the literal. This fails closed and never leaks.
2. Or push secret detection/segmentation up to the parser/IR so a multi-name assignment is decomposed into per-name segments each carrying its own `Value`/`Dynamic`, then exclude per segment.

At minimum, add the guard so the leak path fails closed:
```go
// In Commit (or excludeSecrets), before trusting Value/Dynamic per entry:
func isExcludableSecretShape(e model.Entry) bool {
    // Only a single-name, scalar assignment has a Value/Dynamic that faithfully
    // describes the secret segment. Anything else may hide a literal in Text.
    return e.Category == model.CatSecrets &&
        e.Kind == model.KindAssignment &&
        len(e.Names) == 1
}
// A CatSecrets assignment that is NOT this shape must abort the Commit
// (return a zsh-pro-phrased error) — never fall through to a verbatim commit.
```

### CR-02: Secret value captured under the wrong key (and benign sibling destroyed) for `OTHER=foo SECRET=...` ordering

**File:** `core/store/secret.go:65,99-105,130-132`

**Issue:** `excludeSecrets` uses `key := e.Names[0]` as the keychain/vault account. For a static multi-name secret assignment whose first segment is *not* the secret (`export OTHER=foo API_KEY=sk-REAL-SECRET`), the parser yields `Names=[OTHER, API_KEY]`, `Value="sk-REAL-SECRET"` (last segment), `Dynamic=false`, `Category=secrets`. `isLiteralSecret` returns true, so:
- The real secret `sk-REAL-SECRET` is stored under the **wrong key** `OTHER` (`Names[0]`), not `API_KEY`.
- `WithheldReport` reports `{Name: "OTHER", ...}` — the report lies about what was withheld.
- `Entry.Text` is rewritten to `export OTHER='<zsh-pro secret file:OTHER>'`, **destroying** the benign `OTHER=foo` assignment as well as the real secret name `API_KEY`.
- A Phase 4/5 deref of `API_KEY` finds nothing; a deref of `OTHER` surprisingly returns the secret.

Proven against the real `s.Commit` path: `WithheldReport=[{OTHER 1}]`, `vault[OTHER]="sk-REAL-SECRET"`, `vault[API_KEY]` not found.

This is data corruption (wrong-key capture + lost sibling value) and a confidentiality hazard (the secret becomes resolvable under an unrelated name).

**Fix:** Same root cause and remedy as CR-01 — restrict exclusion to single-name scalar `CatSecrets` assignments and fail closed on any other shape. If multi-name secret support is wanted, segment the assignment so each name carries its own value and pick the key from the secret-matching name (re-run `secretRe` per name), not `Names[0]`.

## Warnings

### WR-01: Multi-line secret values are silently corrupted by the vault backend

**File:** `core/store/keychain.go:229-235,273-283`

**Issue:** `vaultKeychain.Store` does `entries[key] = strings.ReplaceAll(value, "\n", "")` and `load` splits on `"\n"` with a `key=value` line format. A multi-line secret value (e.g. a PEM key — and `PRIVATE_KEY` is one of `secretRe`'s patterns) is stored with all newlines stripped, concatenating the lines into an unusable blob with no error. Proven: storing `-----BEGIN KEY-----\nLINE1\nLINE2\n-----END KEY-----` retrieves `-----BEGIN KEY-----LINE1LINE2-----END KEY-----`. The value is unrecoverable, and the comment "Values never contain a newline in practice" is incorrect for quoted multi-line assignments.

This is WARNING (not BLOCKER) because it affects only the vault fallback path and most secrets are single-line; but when it hits, it is silent and irreversible.

**Fix:** Either reject multi-line values explicitly (return a zsh-pro-phrased error from `Store` so the caller knows capture failed and can fall through to "cannot exclude → abort Commit"), or use a newline-safe vault encoding (e.g. one `key=base64(value)` line per secret, decoding on `load`). A base64 line keeps the line-per-secret format while preserving arbitrary bytes:
```go
// Store
entries[key] = base64.StdEncoding.EncodeToString([]byte(value))
// load
raw, err := base64.StdEncoding.DecodeString(val)
if err != nil { continue } // skip malformed
entries[k] = string(raw)
```

### WR-02: Vault file is rewritten in non-deterministic map-iteration order

**File:** `core/store/keychain.go:289-301`

**Issue:** `save` iterates `for k, val := range entries` over a Go map, whose iteration order is randomized. Every rewrite of the vault file may reorder the lines (confirmed: 4 distinct orders in 20 passes for a 5-key map). The phase's stated determinism goal and the "stable-diff" framing used throughout the store are violated for the vault artifact, churning the on-disk file across otherwise-identical operations and complicating debugging / external diffing.

This is WARNING, not BLOCKER, because the vault is `.gitignore`'d and never enters the committed tree, so it cannot affect commit-SHA stability.

**Fix:** Sort keys before writing:
```go
keys := make([]string, 0, len(entries))
for k := range entries {
    keys = append(keys, k)
}
sort.Strings(keys) // or reuse the package-local sortStrings
for _, k := range keys {
    buf.WriteString(k); buf.WriteByte('='); buf.WriteString(entries[k]); buf.WriteByte('\n')
}
```

### WR-03: `Read` returns `ErrProfileNotFound` for a branch that demonstrably exists

**File:** `core/store/store.go:311-324` (interaction with `Init` at `:116-124` and `Branches`/`Checkout`)

**Issue:** After `Init`, `main` is a real branch — `Branches(ctx)` returns `["main"]` and `Checkout(ctx, "main")` succeeds — but `Read(ctx, "main")` returns `ErrProfileNotFound` because the baseline root commit is over the empty tree and has no `profile.json`. The same is true of a freshly `Create`d fork before its first `Commit`. Proven: `Read(main)` on fresh init → `zsh-pro: profile not found`; `Read(work)` freshly forked → same. The error conflates "branch absent" with "branch present but never committed to," which will surprise the Phase 5 verb caller (e.g. `checkout main` validates fine, but reading its profile errors).

**Fix:** Distinguish the two states. Either (a) seed `main` (and forks) with an empty `profile.json` blob at creation so `Read` returns `model.Profile{}` for a never-written-but-existing branch, or (b) have `Read` first probe `refs/heads/<branch>` existence and return `model.Profile{}` (empty) when the branch exists but `:profile.json` does not, reserving `ErrProfileNotFound` for a genuinely absent branch. Document whichever contract Phase 5 should rely on.

## Info

### IN-01: `WithheldReport` does not surface the leaked/un-excluded secret, masking CR-01/CR-02 from the user

**File:** `core/store/store.go:211-303`, `core/store/secret.go:45-109`

**Issue:** When a `CatSecrets` entry bypasses exclusion (CR-01) or is mis-captured (CR-02), `WithheldReport` is either empty or names the wrong key. Since Phase 5 relies on this report to tell the user "what was withheld" (success-criterion #4), there is no signal that a secret-named variable was committed in the clear. Independent of the CR fixes, a defense-in-depth post-commit assertion would catch regressions: after building `jsonBytes`/`zshBytes`, verify no `CatSecrets` entry's pre-exclusion literal substring survives in the blobs, and fail the Commit if it does.

**Fix:** Add a final guard in `Commit` that scans the about-to-be-committed blobs for any retained secret literal and aborts (zsh-pro-phrased) rather than writing the tree — a belt-and-suspenders net beneath the per-entry logic.

### IN-02: `excludeSecrets` makes a shallow copy; `Names`/`Secret` are shared with the caller

**File:** `core/store/secret.go:53-54`

**Issue:** `entries := make(...); copy(entries, p.Entries)` is a shallow copy: each copied `Entry` shares the caller's `Names` backing array and `Secret` pointer. The current code only writes `e.Secret` (a new pointer), `e.Text`, and `e.Value` (new strings), and only reads `Names[0]`, so the caller is not corrupted today and `TestExcludeSecretsDefensiveCopy` passes. This is a latent hazard, not a present bug: any future edit that mutates `e.Names[i]` in place or mutates `*e.Secret` would silently corrupt the caller's profile. The doc comment calls this a "defensive copy," which overstates the guarantee.

**Fix:** Either deep-copy `Names` (and document that `Secret` is replaced, never mutated in place) in the loop, or downgrade the comment to "shallow copy of the entry slice; the loop only replaces value-typed fields and new pointers." Reuse `cloneNames` from `dto.go` if a deep copy is chosen.

---

_Reviewed: 2026-06-27_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
