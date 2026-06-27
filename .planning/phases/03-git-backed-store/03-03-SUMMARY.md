---
phase: 03-git-backed-store
plan: 03
subsystem: infra
tags: [secrets, secretref, keychain, secret-tool, vault, git-store, composition-root, prof-03, exclude-secrets]

# Dependency graph
requires:
  - phase: 02-ir-partial-evaluation
    provides: model.Profile/Entry IR with the already-classified Category (CatSecrets via secretRe) + the parser's Dynamic flag + verbatim Text/Value — the inputs excludeSecrets reads with zero new inspection; ir.Regenerate(p, shell.Regenerator) for profile.zsh
  - phase: 03-git-backed-store (Plan 01)
    provides: SecretRef{Kind,Key} model type, MarshalProfile/UnmarshalProfile (entryDTO carries Secret omitempty), errStore sentinels incl. ErrSecretBackendUnavailable, gitRunner plumbing
  - phase: 03-git-backed-store (Plan 02)
    provides: FINAL Store + New(dir, regen, kc) (3-arg), Commit(ctx, branch, p, msg) (WithheldReport, error) (body-edit only), the KeychainDriver interface (incl. Kind()), WithheldSecret/WithheldReport types, the Read(Commit(p)) round-trip pin
  - phase: 03-git-backed-store (Plan 03 / Task 1, pre-committed ac27b01)
    provides: concrete KeychainDriver backends (macOSKeychain/linuxKeychain/vaultKeychain) + NewOSKeychainDriver runtime selector + the .gitignore vault entry
provides:
  - "excludeSecrets(ctx, p, kc) (model.Profile, WithheldReport, error) — store-side literal-secret exclusion reusing the D-08 predicate over already-classified Entry fields (no new regex, no new AST walk); defensive copy, nil-backend guard, kc.Kind()-stamped SecretRef"
  - "Commit now runs exclusion in its body: literal secret -> value captured to the injected backend + SecretRef recorded + literal removed from BOTH committed blobs (profile.json text/value AND profile.zsh) + WithheldReport populated; already-dynamic secrets commit verbatim (D-08). Signature unchanged."
  - "Composition root (core/cmd/zsh-pro/main.go) wires store.New(storeDir(), zsh.Provider{}, NewOSKeychainDriver(dir)) — store constructed + injectable; store package still never imports core/shell/zsh"
  - "PROF-03 store-side half complete (D-07–D-10): secrets excluded by default + user told what was withheld via the WithheldReport"
affects: [04 (manifest builder reads the persisted Profile; sees Secret pointers not literals), 05 (loader derefs SecretRef on switch — reads the value back from the backend the kc.Kind() stamp names + emits the real assignment; consumes the WithheldReport to tell the user what was withheld), 06 (real ~/.zshrc end-to-end ingest closes PROF-03)]

# Tech tracking
tech-stack:
  added: []  # no new Go dependency — go.mod unchanged (PROF-01 honored); keychain via the security/secret-tool binaries (subprocess), vault via stdlib os
  patterns:
    - "Store-side secret exclusion reuses the SHIPPED classifier verdict (Entry.Category==CatSecrets) + the parser's Dynamic flag — D-08 predicate with ZERO new inspection logic; core/store stays shell-agnostic (never imports core/shell/zsh)"
    - "A literal secret's literal is removed from BOTH derived text fields (Entry.Text AND Entry.Value), not just Value — Text is the JSON \"text\" field AND the Regenerator's empty-Value fallback, so clearing only Value would still leak the literal into profile.json's text field and into profile.zsh (T-03-03). Both are replaced by an inert single-quoted kind:key placeholder; the Secret pointer is the authoritative record"
    - "SecretRef.Kind is stamped from the ACTIVE backend's kc.Kind() (keychain/keychain/file), never a hardcoded constant, so a machine without security/secret-tool produces a file-kind reference that Ph4/5 dereferences from the vault (T-03-09)"
    - "Composition-root-only injection of concrete drivers (zsh.Provider{} Regenerator + NewOSKeychainDriver keychain); store-init error is non-fatal so the read-only analyze path never crashes (store-backed CLI verbs are Phase 5)"

key-files:
  created:
    - core/store/secret.go
    - core/store/secret_test.go
  modified:
    - core/store/store.go
    - core/cmd/zsh-pro/main.go
    - .planning/REQUIREMENTS.md

key-decisions:
  - "Excluding a literal secret must remove the literal from Entry.Text AND Entry.Value (not just Value) — else it survives in profile.json's verbatim text field and, via the Regenerator's empty-Value fallback, in profile.zsh. Both fields get an inert single-quoted '<zsh-pro secret kind:key>' placeholder; the Secret pointer is authoritative (Rule 1 fix — the plan's clear-Value-only sketch leaked)"
  - "SecretRef.Kind = kc.Kind() from the live driver (not model.SecretRefKeychain hardcoded) so the vault fallback yields a file-kind reference (T-03-09)"
  - "excludeSecrets operates on a defensive copy of the Entries slice (caller's Profile untouched), guards a nil KeychainDriver on a literal secret with ErrSecretBackendUnavailable (never a nil-panic), and aborts the whole Commit if backend capture fails (never commit a half-excluded profile)"
  - "main.go stays the SOLE non-test importer of core/shell/zsh; storeDir() resolves D-04 ($ZSHPRO_HOME / $XDG_DATA_HOME/zsh-pro / ~/.local/share/zsh-pro); store-init error is non-fatal this phase (CLI verbs that consume it are Phase 5)"
  - "PROF-03 traceability records the Phase 3 start (store-side exclusion) without marking the requirement complete — it completes at the Ph6 end-to-end ingest"

patterns-established:
  - "D-08 literal-vs-dynamic secret split from already-populated Entry fields (CatSecrets verdict + Dynamic flag), no new regex/AST — the store consumes the ingest engine's verdict and stays shell-agnostic"
  - "Defense-in-depth secret exclusion: literal removed from both committed blobs + value off-argv into the backend + vault 0600 outside the bare repo + .gitignore'd + plumbing stages only profile.json/profile.zsh"
  - "Backend-kind-stamped SecretRef so the deref phase reads the correct backend (keychain vs file)"

requirements-completed: [PROF-02]  # PROF-03 store-side half landed but NOT closed (Ph4/5 deref + Ph6 end-to-end remain) — left unchecked in REQUIREMENTS.md by design

# Metrics
duration: 35min
completed: 2026-06-27
---

# Phase 3 Plan 03: Store-Side Secret Exclusion + Composition-Root Wiring Summary

**On Commit, a literal secret is now captured into a runtime-selected keychain/vault backend and replaced by a `kc.Kind()`-stamped `SecretRef` whose literal never enters profile.json OR profile.zsh, with a withheld-report returned; already-dynamic secrets commit verbatim (D-08); the store is wired at the sole composition root with `zsh.Provider{}` + `NewOSKeychainDriver` while staying shell-agnostic.**

## Performance

- **Duration:** ~35 min active (resume session; plan spanned ~1h wall including the post-Task-1 interruption gap)
- **Started:** 2026-06-27 (Task 1 backends pre-committed at `ac27b01`); resumed for Tasks 2–4
- **Completed:** 2026-06-27
- **Tasks:** 4 logical tasks (Task 1 pre-committed + verified reachable; secret exclusion, composition-root wiring, and the REQUIREMENTS traceability finished this session); `.gitignore` vault entry was already in `ac27b01`
- **Files modified:** 5 this session (2 created, 3 modified); `core/store/keychain.go`/`keychain_test.go`/`.gitignore` were in the pre-committed Task 1

## Resume Context

A prior executor committed Task 1 (`ac27b01`) more completely than the resume note implied: the three concrete `KeychainDriver` backends (`macOSKeychain`/`linuxKeychain`/`vaultKeychain`), `NewOSKeychainDriver`, the full `keychain_test.go`, AND the `.gitignore` vault entry (plan Task 5) were all already present and green. This session verified `ac27b01` reachable (`git merge-base --is-ancestor`), confirmed the working tree clean and the baseline `go test ./core/store/` green, then implemented the remaining work without redoing anything committed.

## Accomplishments

- **Store-side secret exclusion (`core/store/secret.go`, `37041e1`):** `excludeSecrets` applies the D-08 predicate — `Category==CatSecrets && !Dynamic && Value != "" && len(Names)>0` => LITERAL — using ONLY the already-classified Entry fields (the shipped `secretRe`/`CatSecrets` verdict + the parser's `Dynamic` flag); no new regex, no new AST walk, no `core/shell/zsh` import. Each literal's value is captured into the injected backend via `kc.Store`, the entry gets a `SecretRef{Kind: kc.Kind(), Key: Names[0]}` (authoritative), and the literal is removed from both `Entry.Text` and `Entry.Value`. Already-dynamic secrets (`export TOKEN=$(op read ...)`) pass through verbatim (D-08). Operates on a defensive copy; nil-backend on a literal => `ErrSecretBackendUnavailable`; a backend failure aborts the Commit.
- **Commit body wired (`core/store/store.go`, `37041e1`):** exclusion runs first; `MarshalProfile`/`ir.Regenerate`/hash/stage/commit all operate on the post-exclusion profile; the terminal `return nil, nil` is now `return report, nil`. Signature unchanged; no Plan 02 test edited; the 03-02 round-trip pins stay green (exclusion is a no-op for the secret-free fixture).
- **Test coverage (`core/store/secret_test.go`, `37041e1`):** builds fixtures through the real `zsh.Provider{}` + `ir.Build` (authentic classification), then pins: a literal secret is absent from BOTH `profile.json` AND `profile.zsh`, captured (recoverable via the vault), reported (`{API_KEY, line 2}`), and read back with `SecretRef.Kind==file` (vault fallback, T-03-09); a dynamic secret commits verbatim and is not reported; `excludeSecrets` never mutates the caller's Profile; a nil backend guards a literal but no-ops a secret-free profile.
- **Composition root wired (`core/cmd/zsh-pro/main.go`, `8b1ec95`):** `store.New(storeDir(), zsh.Provider{}, NewOSKeychainDriver(dir))` with a `storeDir()` D-04 resolver. main.go is the verified SOLE non-test importer of `core/shell/zsh`; `core/store/*.go` non-test files import none of it. Store-init error is non-fatal (the read-only `analyze` path never crashes; store-backed verbs are Phase 5).
- **PROF-03 traceability (`.planning/REQUIREMENTS.md`, `8b1ec95`):** records the Phase 3 store-side start (→ Ph4/5 deref → Ph6 end-to-end) without marking PROF-03 complete.
- **`make check` fully green:** fmt-check + `go vet ./...` + `golangci-lint` (0 issues) + `go test ./...`. The Phase 2 oracles and the 03-02 round-trip pins are not regressed; `core/store` ran fresh (3.146s).

## Task Commits

1. **Task 1: Concrete KeychainDriver backends (security/secret-tool/0600 vault) + .gitignore vault entry** — `ac27b01` (feat) — *pre-committed by the prior executor; verified reachable this session*
2. **Tasks 2–3: Secret exclusion wired into Commit + secret_test.go** — `37041e1` (feat)
3. **Task 4: Composition-root wiring + PROF-03 traceability** — `8b1ec95` (feat)

**Plan metadata:** _(this SUMMARY + STATE/ROADMAP — see final docs commit)_

## Files Created/Modified

- `core/store/secret.go` (created) — `excludeSecrets` + the `isLiteralSecret` D-08 predicate + the `secretRefValue` inert placeholder; shell-agnostic (imports only `context`, `fmt`, `core/model`)
- `core/store/secret_test.go` (created) — internal `package store` tests; wires `zsh.Provider{}`/`ir.Build` only to build authentic fixtures (the sanctioned test-wiring exception)
- `core/store/store.go` (modified) — `Commit` body runs `excludeSecrets` first and returns the populated report; signature unchanged
- `core/cmd/zsh-pro/main.go` (modified) — store wired at the composition root + `storeDir()` D-04 helper; remains the sole `core/shell/zsh` importer
- `.planning/REQUIREMENTS.md` (modified) — PROF-03 traceability records the Phase 3 start (not marked complete)

## Decisions Made

Followed the plan's locked decisions exactly, with one correctness fix to the plan's exclusion sketch:
- **D-08 reuse, no new inspection:** literal-vs-dynamic from `Entry.Category`/`Entry.Dynamic`/`Entry.Value`/`Entry.Names`; the store never re-runs `secretRe` and never imports `core/shell/zsh`.
- **kc.Kind()-stamped SecretRef (T-03-09):** the vault fallback yields a `file`-kind reference; a keychain backend yields `keychain` — so Ph4/5 derefs the right backend.
- **Defensive copy + fail-closed capture:** the caller's Profile is untouched; a nil backend on a literal is `ErrSecretBackendUnavailable`; a `kc.Store` failure aborts the Commit before any ref moves (never commit a half-excluded profile).
- **Composition-root invariant:** main.go alone imports `core/shell/zsh`; the store stays shell-agnostic; store-init failure is non-fatal this phase.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Literal secret leaked via `Entry.Text` (and `profile.zsh`) — clearing only `Value` was insufficient**
- **Found during:** Tasks 2–3 (the `TestCommitExcludesLiteralSecret` profile.json no-leak assertion)
- **Issue:** The plan's exclusion sketch (echoed in 03-PATTERNS) said to "set `Entry.Secret` + clear `Entry.Value`". But `Entry.Text` carries the verbatim source statement (`export API_KEY="sk-abc123"`) and is serialized as the JSON `"text"` field — so the literal survived in `profile.json` even with `Value` cleared. Worse, `ir.Regenerate` calls the Regenerator for a managed secret entry, and `zsh.Provider.Regenerate`'s `KindAssignment` branch falls back to `e.Text` when `Value==""` (its empty-Value guard) — so the literal ALSO leaked into the committed `profile.zsh`. Both are the T-03-03 "literal never enters the tree" violation the threat model mandates be mitigated.
- **Fix:** In `excludeSecrets`, replace BOTH `Entry.Text` and `Entry.Value` with an inert single-quoted `'<zsh-pro secret kind:key>'` placeholder (built by `secretRefValue`), keeping the `SecretRef` pointer as the authoritative record. The non-empty placeholder Value also stops the Regenerator falling back to Text, so `profile.zsh` emits a literal-free `export API_KEY='<zsh-pro secret file:API_KEY>'`. This is store-side DATA substitution (a placeholder string), not zsh codegen — `core/store` stays shell-agnostic; the real deref/emit syntax remains Ph4/5's job (the placeholder is never sourced in Phase 3). This matches 03-RESEARCH Pitfall 6 ("for secret-converted entries the committed form intentionally differs — compare the post-exclusion profile").
- **Files modified:** `core/store/secret.go` (added the dual-field rewrite + `secretRefValue` helper)
- **Verification:** `TestCommitExcludesLiteralSecret` asserts `sk-abc123` is absent from BOTH `git show main:profile.json` AND `git show main:profile.zsh`, while the SecretRef key + the recoverable backend value are present; all store tests + `make check` green.
- **Committed in:** `37041e1` (part of the secret-exclusion task commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 bug — a real secret-leak the plan's clear-Value-only sketch would have shipped).
**Impact on plan:** The fix is essential for the plan's own threat-model mitigation (T-03-03) and success criterion #4. No scope creep — no signature changed, no Plan 02 test edited, no new dependency, and the deref runtime stays Ph4/5. The placeholder representation is store-side data, not new shell codegen.

## Issues Encountered

- On this development machine `security` is present, so `NewOSKeychainDriver` selects `macOSKeychain` and `TestMacOSKeychainRoundTrip` ran (not skipped). The secret-exclusion tests deliberately construct `newVaultKeychain` directly, so they exercise the file-kind (vault) path and its `SecretRef.Kind==file` assertion regardless of the host keychain — the T-03-09 distinction is pinned on every machine.
- Two of my own initial test assertions assumed `Value` would be cleared to empty; after the Rule 1 fix set it to the placeholder, I updated them to assert the literal is absent (not that the field is empty) and added the `profile.zsh` no-leak check. Caught and fixed within the same task before commit.

## User Setup Required

None — no external service configuration required. The keychain backend is auto-selected at runtime (`security`/`secret-tool`/vault fallback); the vault file needs no setup. The `ZSHPRO_PROFILE` export-on-switch and the runtime `SecretRef` deref are Phase 5.

## Next Phase Readiness

- **Phase 4 (manifest builder)** reads the persisted Profile via `Read`; it now sees `Secret` pointers (and the placeholder text/value) instead of literals for excluded secrets.
- **Phase 5 (loader)** derefs `SecretRef` on switch: it reads the value back from the backend named by `SecretRef.Kind` (the `kc.Kind()` stamp guarantees the right backend) and emits the real assignment, and consumes the `WithheldReport` to tell the user what was withheld.
- **Phase 6** closes PROF-03 with the real `~/.zshrc` end-to-end ingest into `main`.
- No blockers. `go.mod` unchanged (PROF-01 honored). All Phase 2 regression pins + the 03-02 round-trip pin green; the composition-root + shell-agnostic invariants hold. **Phase 3 is complete** (all 3 plans).

## Self-Check: PASSED

- `core/store/secret.go`, `core/store/secret_test.go` present; `core/store/store.go`, `core/cmd/zsh-pro/main.go`, `.planning/REQUIREMENTS.md` carry the changes.
- All three plan commits exist in git history: `ac27b01`, `37041e1`, `8b1ec95`.
- `core/store/*.go` non-test files do NOT import `core/shell/zsh`; `core/cmd/zsh-pro/main.go` is the sole non-test importer.
- `make check` green (fmt-check + vet + lint 0 issues + full `go test ./...`); `core/store` ran fresh (not cached).

---
*Phase: 03-git-backed-store*
*Completed: 2026-06-27*
