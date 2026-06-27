---
phase: 03-git-backed-store
plan: 02
subsystem: infra
tags: [git-plumbing, bare-repo, store, branch-as-profile, roundtrip-oracle, zshpro-profile, secretref]

# Dependency graph
requires:
  - phase: 02-ir-partial-evaluation
    provides: model.Profile/Entry IR (verbatim Text, ManagedOverride, static/dynamic tag); ir.Regenerate(p, shell.Regenerator) seam; the byte-identical round-trip oracle (core/ir/roundtrip_test.go pattern)
  - phase: 03-git-backed-store (Plan 01)
    provides: gitRunner plumbing primitives (hashObject/catFileExists/show/forEachRef/revParse/updateRef/isBareRepo/runCommit), MarshalProfile/UnmarshalProfile, errStore sentinels, emptyTreeSHA
provides:
  - "core/store.Store + New(dir, regen, kc) — the FINAL 3-arg constructor (Plan 03 wires the concrete keychain behind the existing field; no signature change)"
  - "Init (idempotent bare repo + main baseline root commit), Branches (sorted), Current (per-terminal ZSHPRO_PROFILE, unset => main), Create (forks main), Checkout (validate-only)"
  - "Commit(ctx, branch, p, msg) (WithheldReport, error) — FINAL 2-value signature; writes profile.json + derived profile.zsh to a branch purely via plumbing (temp index -> write-tree -> commit-tree -> update-ref), no checkout"
  - "Read(ctx, branch) (model.Profile, error) — git show <branch>:profile.json -> UnmarshalProfile, with a catFileExists probe -> ErrProfileNotFound"
  - "KeychainDriver interface (Store/Retrieve/Delete/Kind() model.SecretRefKind) + WithheldSecret/WithheldReport types — declared at first use in FINAL form (Plan 03 fills bodies only)"
  - "validBranchName — V5 + per-/-component path-traversal guard (T-03-02)"
  - "The Read(Commit(p)) round-trip pin (incl. empty-profile edge) composed with the Phase 2 byte-identical oracle — the Phase 3 correctness gate"
affects: [03-03 (secret exclusion fills Commit body + populates WithheldReport; concrete KeychainDriver impls), 04 (manifest builder reads the persisted Profile via Read), 05 (loader exports ZSHPRO_PROFILE on checkout + derefs SecretRef)]

# Tech tracking
tech-stack:
  added: []  # no new Go dependency — git via the binary (PROF-01 honored); go.mod unchanged
  patterns:
    - "Bare-repo plumbing-to-branch: Commit moves refs in the object DB only (temp GIT_INDEX_FILE -> read-tree -> update-index --cacheinfo -> write-tree -> commit-tree -> update-ref), zero working-tree verbs — the conda concurrent-activation race avoided by construction (D-12, critical decision #4)"
    - "Read via `git show <branch>:profile.json` from the object DB, gated by a catFileExists existence probe so a missing profile surfaces ErrProfileNotFound, never a raw `fatal:` (D-11/Pitfall 4)"
    - "Public seam fixed in FINAL form one wave early (New/Commit signatures, KeychainDriver, WithheldReport) so Plan 03 changes no signature and edits no test in this plan"
    - "External-package round-trip oracle (package store_test) is the only store test wiring the concrete zsh.Provider{} — the sanctioned end-to-end-wiring exception, mirroring core/ir/roundtrip_test.go and core/analyze/corpus_test.go; production store.go stays shell-agnostic"

key-files:
  created:
    - core/store/store.go
    - core/store/store_test.go
    - core/store/roundtrip_test.go
  modified:
    - core/store/dto.go
    - core/store/errors.go
    - core/store/git_test.go

key-decisions:
  - "Commit stays plumbing-only — no git checkout/status/reset anywhere; a bare repo has no working tree, so cross-terminal contention is impossible by construction (D-12, T-03-04)"
  - "Active profile is per-terminal via ZSHPRO_PROFILE (name only; unset => main); never a shared global file. Checkout validates the branch exists but does NOT export — the export is Phase 5's sourced loader"
  - "New/Commit signatures + KeychainDriver + WithheldReport declared in FINAL form here so the public seam never evolves into Plan 03 (root-cause fix the reviewer asked for)"
  - "UnmarshalProfile decodes an entry-less profile to the zero value model.Profile{} (nil Entries), not a non-nil empty slice — the same nil-preserving choice cloneNames makes — so the empty round-trip is reflect.DeepEqual to its input (Rule 1 fix)"

patterns-established:
  - "Bare-repo plumbing-to-branch commit recipe (temp index + commit-tree + update-ref, parented-or-root) with no checkout"
  - "catFileExists existence probe before `git show` so reads return a zsh-pro-phrased ErrProfileNotFound"
  - "Per-terminal ZSHPRO_PROFILE active-profile contract (unset => main)"
  - "Read(Commit(p)) round-trip pin composed with the Phase 2 byte-identical oracle (Phase 3 correctness gate), skip-guarded on git + zsh"

requirements-completed: [PROF-01, PROF-02]

# Metrics
duration: 9min
completed: 2026-06-27
---

# Phase 3 Plan 02: core/store Orchestrator + Round-Trip Pin Summary

**The `core/store` orchestrator over a BARE git repo — `Init`/`Branches`/`Current`/`Create`/`Checkout` plus `Commit` (plumbing-to-branch, no checkout) and `Read` (`git show`) — with `New`/`Commit`/`KeychainDriver`/`WithheldReport` declared in their FINAL form, pinned by the `Read(Commit(p))` round-trip composed with the Phase 2 byte-identical oracle.**

## Performance

- **Duration:** ~9 min active (resume session; plan spanned ~2h wall including the mid-Task-2 interruption gap)
- **Started:** 2026-06-27 (Task 1 skeleton at 79f2bc3); resumed mid-Task-2
- **Completed:** 2026-06-27
- **Tasks:** 2 (Task 1 pre-committed and verified reachable; Task 2 finished this session)
- **Files modified:** 6 (3 created, 3 modified) across the plan

## Resume Context

This plan was executed in two sessions. A prior executor committed Task 1 (`79f2bc3`, the Store skeleton + `New`/`Init`/`Branches`/`Current`/`Create`/`Checkout` + the `KeychainDriver` seam + `store_test.go`) and left Task 2 partially written but **uncommitted** in the working tree. This resume session verified `79f2bc3` was reachable (`git merge-base --is-ancestor`), reviewed the uncommitted `Read`/`Commit`/`WithheldReport` additions against the plan (correct — committed atomically), then wrote the round-trip pin and finished the plan. No completed work was redone.

## Accomplishments

- **Bare-repo orchestrator (Task 1, pre-committed `79f2bc3`):** `Store` with exactly four injected fields (`dir`, `git`, `regen`, `keychain`); `New(dir, regen, kc)` in FINAL form; idempotent `Init` (re-run is a no-op via `isBareRepo`; seeds `main` with an empty-tree root commit); `Branches` (sorted via `forEachRef`); `Current` (reads `ZSHPRO_PROFILE`, unset => `main`, D-13); `Create` (forks `main` via `revParse`+`updateRef`); `Checkout` (validate-only — existence check, no export). `validBranchName` rejects empty / leading-`-` / out-of-charset / any `..` or `.` `/`-component (T-03-02).
- **`Commit` — plumbing-to-branch, FINAL 2-value signature (this session, `cf116e3`):** `(WithheldReport, error)`. Implements the Plan-01-proven Pattern 1 with NO checkout: `MarshalProfile` -> `ir.Regenerate(p, s.regen)` -> `hashObject` both blobs -> temp `GIT_INDEX_FILE` via `os.CreateTemp` (+ deferred `os.Remove`) -> `read-tree <branch>` (parented) or `read-tree --empty` (root commit) -> `update-index --add --cacheinfo` for `profile.json` + `profile.zsh` only -> `write-tree` -> `commit-tree [-p parent]` (msg via `-m`) -> `update-ref`. Returns `nil, nil` (secret exclusion deferred to Plan 03). Every git call goes through `gitRunner`, so errors are zsh-pro-phrased.
- **`Read` — object-DB read (this session, `cf116e3`):** validates the branch, probes `catFileExists(branch+":profile.json")` -> `ErrProfileNotFound` (never a raw `git show` fatal — D-11/T-03-01), then `show` -> `UnmarshalProfile`. No checkout, no re-parse (store stays shell-agnostic).
- **FINAL public seam declared at first use (this session, `cf116e3`):** `WithheldSecret{Name, StartLine}` and `WithheldReport []WithheldSecret` with the documented producer/consumer contract (produced here on `Commit`, surfaced to the user in Phase 5). `KeychainDriver` (with `Kind() model.SecretRefKind`) was already declared in Task 1. Plan 03 changes none of these signatures.
- **The Phase 3 correctness gate — `Read(Commit(p))` round-trip composed with the Phase 2 oracle (this session, `381fb09`):** `core/store/roundtrip_test.go` (`package store_test` — the only store test wiring `zsh.Provider{}`). Three pins: (1) a full profile (all 5 declarative classes + an explicit `OverrideManaged`) reads back `reflect.DeepEqual` with an empty `WithheldReport`; (2) the empty-profile edge `Read(Commit(model.Profile{})) == model.Profile{}`; (3) regenerating `profile.zsh` from the read-back profile sources to byte-identical `IdentitySet` tables (Aliases/Functions/Env/Options/Path) vs the original under `zsh -f`. Skip-guarded on git (1/2) and additionally on zsh (3).
- **Phase 2 oracles not regressed:** `make check` fully green (fmt-check + `go vet ./...` + `golangci-lint` 0 issues + `go test ./...`); `TestRegenRoundTrip`, `TestOracleProperty`, `TestCorpusGolden`, `TestEffectiveManaged` all pass.

## Task Commits

1. **Task 1: Store orchestrator skeleton — Init/Branches/Current/Create/Checkout + KeychainDriver seam** — `79f2bc3` (feat) — *pre-committed by the prior executor; verified reachable this session*
2. **Task 2 (impl): Commit (plumbing-to-branch) + Read + WithheldReport contract** — `cf116e3` (feat)
3. **Task 2 (test + fix): Read(Commit(p)) round-trip pin composed with the Phase 2 oracle** — `381fb09` (test; includes the Rule 1 `UnmarshalProfile` nil-preservation fix)

**Plan metadata:** _(this SUMMARY + STATE/ROADMAP/REQUIREMENTS — see final docs commit)_

## Files Created/Modified

- `core/store/store.go` (created Task 1; `Commit`/`Read`/`WithheldReport` added this session) — the orchestrator; does NOT import `core/shell/zsh` (verified shell-agnostic)
- `core/store/store_test.go` (created Task 1) — internal `package store` tests: `TestInitIdempotent`, `TestCurrent`, `TestCreate`, `TestCheckout`, `TestValidBranchName` (the unexported guard's real caller)
- `core/store/roundtrip_test.go` (created this session) — external `package store_test`; the only store test importing `core/shell/zsh`; the round-trip + empty-profile + compose-oracle pins
- `core/store/dto.go` (modified this session) — `UnmarshalProfile` now preserves nil `Entries` for an entry-less profile (Rule 1 fix)
- `core/store/errors.go` (modified Task 1) — added `ErrInvalidProfileName` (V5 + path-traversal guard message)
- `core/store/git_test.go` (modified Task 1) — minor adjustment alongside the skeleton

## Decisions Made

Followed the plan's locked decisions exactly:
- **D-12 / critical decision #4 → bare repo, plumbing-to-branch:** `Commit` never checks out; a bare repo has no working tree, so the conda concurrent-activation race (T-03-04) is impossible by construction. Verified by the absence of `"status"`/`"checkout"`/`"reset"` argv and the `isBareRepo` assertion in `TestInitIdempotent`.
- **D-13 → per-terminal `ZSHPRO_PROFILE` (name only; unset => main):** `Current` reads the env var; `Checkout` validates the target exists but does NOT export (the export is Phase 5's sourced loader).
- **FINAL public seam this wave:** `New(dir, regen, kc)`, `Commit(...) (WithheldReport, error)`, `KeychainDriver` (with `Kind()`), and `WithheldReport` are all declared here so Plan 03 introduces no signature change.
- **D-01 fidelity at the no-entries edge:** an empty profile must round-trip to the zero value, mirroring `cloneNames`' nil-preserving intent.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `UnmarshalProfile` broke the empty-profile round-trip (non-nil empty slice vs nil)**
- **Found during:** Task 2 (the empty-profile round-trip pin, `TestRoundTripEmptyProfile` — the reviewer's LOW #8 edge the test exists to catch)
- **Issue:** `UnmarshalProfile` always did `make([]model.Entry, len(dto.Entries))`, so an entry-less profile decoded to `model.Profile{Entries: []model.Entry{}}` (non-nil empty slice). The input was `model.Profile{}` (nil `Entries`), so `reflect.DeepEqual` returned `false` and `Read(Commit(model.Profile{})) == model.Profile{}` failed.
- **Fix:** Short-circuit to `return model.Profile{}, nil` when `len(dto.Entries) == 0`, preserving nil-ness — the same deliberate choice `cloneNames` already makes for `Names`. The fix is in a Plan-01 file (`core/store/dto.go`), but the round-trip fidelity it guarantees is squarely Plan 02's correctness gate.
- **Files modified:** `core/store/dto.go`
- **Verification:** `TestRoundTripEmptyProfile` passes; all existing dto tests (`TestRoundTripLossless`, `TestMarshalDeterministic`, …) still pass (they use non-empty `Entries`).
- **Committed in:** `381fb09`

**2. [Rule 1 - Bug] `errcheck` lint failures in the uncommitted `Commit` (pre-commit hook blocked the commit)**
- **Found during:** Task 2 (committing the uncommitted `Read`/`Commit` — the `make hooks` pre-commit `golangci-lint` gate)
- **Issue:** The interrupted executor's `Commit` had `idxFile.Close()` and `defer os.Remove(idx)` with unchecked return values. `errcheck` failed the commit; the architecture guardrails require `make check` (incl. lint) green.
- **Fix:** Discarded both returns with `_ =` (`_ = idxFile.Close()`; `defer func() { _ = os.Remove(idx) }()`) — the package's established idiom (`mapGitError` already uses `_ = err`/`_ = stderr`), with an explanatory comment. Best-effort temp-index cleanup; a leftover is harmless.
- **Files modified:** `core/store/store.go`
- **Verification:** `golangci-lint run ./core/store/` -> 0 issues; the commit's pre-commit hook passed.
- **Committed in:** `cf116e3`

---

**Total deviations:** 2 auto-fixed (both Rule 1 bugs).
**Impact on plan:** Both were necessary to make the planned code pass the mandated `make check`/`make hooks` gate and the plan's own correctness pin. No scope creep — no Plan 03 surface (secret exclusion / concrete keychain) was pulled forward; the FINAL signatures were declared exactly as the plan specifies.

## Issues Encountered

- The uncommitted Task 2 code built and passed `go test ./core/store/` as left by the prior executor, but did NOT pass the `errcheck` lint gate that the pre-commit hook enforces — surfaced only on the commit attempt (deviation #2). The empty-profile fidelity bug (deviation #1) was latent until the round-trip pin was written, exactly as designed (the test is the gate).

## User Setup Required

None — no external service configuration required. (Concrete keychain/secret-backend wiring and the `ZSHPRO_PROFILE` export-on-switch are Plan 03 / Phase 5.)

## Next Phase Readiness

- **Plan 03 (secret exclusion)** composes directly against the FINAL seam: it inserts an `excludeSecrets` step at the top of `Commit`'s body and changes `return nil, nil` to a populated `WithheldReport`, and wires concrete `KeychainDriver` backends behind the existing field — **no signature change, no test edit in this plan**. The `SecretRef` cleared-Value serialization (Plan 01) and the round-trip pin are in place to validate it.
- **Phase 4 (manifest builder)** reads the persisted `Profile` via `Read`.
- **Phase 5 (loader)** exports `ZSHPRO_PROFILE` on checkout and derefs `SecretRef`.
- No blockers. `go.mod` unchanged (PROF-01 honored). All Phase 2 regression pins green; the Phase 3 correctness gate (`Read(Commit(p))` composed with the Phase 2 oracle) is now pinned.

## Self-Check: PASSED

- `core/store/store.go`, `core/store/store_test.go`, `core/store/roundtrip_test.go` all present; `core/store/dto.go` carries the nil-preserving `UnmarshalProfile`.
- All three plan commits exist in git history: `79f2bc3`, `cf116e3`, `381fb09`.
- `core/store/store.go` does NOT import `core/shell/zsh`; `roundtrip_test.go` is the only store test that does.
- `make check` green (fmt-check + vet + lint 0 issues + full `go test ./...`); `core/store` package tests ran (not cached).

---
*Phase: 03-git-backed-store*
*Completed: 2026-06-27*
