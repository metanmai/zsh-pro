---
phase: 03-git-backed-store
plan: 01
subsystem: infra
tags: [git-plumbing, subprocess, encoding-json, serialization, secretref, store]

# Dependency graph
requires:
  - phase: 02-ir-partial-evaluation
    provides: model.Profile/Entry IR (verbatim Text, ManagedOverride, static/dynamic tag); ir.Regenerate seam; the byte-identical round-trip oracle
provides:
  - "core/model.SecretRef (kind:key) — additive, dependency-free secret pointer; Entry gains one omitempty *SecretRef field"
  - "core/store DTO: lossless deterministic model.Profile<->profile.json serialization (MarshalProfile/UnmarshalProfile) — the Read/Commit payload format"
  - "core/store gitRunner: git-binary subprocess driver (5s timeout + LookPath guard + graceful degrade) mirroring introspect.go"
  - "git plumbing primitives Plan 02 composes: hashObject, catFileExists, show, forEachRef, revParse, updateRef, isBareRepo, runCommit (deterministic GIT_* env)"
  - "core/store errStore typed sentinels (D-11) — zsh-pro-phrased, never raw git stderr"
affects: [03-02 (Init/Read/Commit/Branches compose these primitives + payload), 03-03 (secret exclusion populates SecretRef), 04 (manifest reads persisted Profile), 05 (loader derefs SecretRef)]

# Tech tracking
tech-stack:
  added: []  # no new Go dependency — git via the binary (PROF-01 honored); go.mod unchanged
  patterns:
    - "Store-local DTO mapping (entryDTO/profileDTO) carries json tags so model.Entry stays a pure domain type (decision #1, option b)"
    - "git-binary subprocess driver mirrors core/shell/zsh/introspect.go verbatim (context.WithTimeout 5s + defer cancel + exec.CommandContext + separate out/errb buffers)"
    - "All store errors are errStore typed sentinels; mapGitError discards raw stderr (D-11)"
    - "core/store is fully shell-agnostic — transitive dep set is core/model + stdlib only (no shell package at all)"

key-files:
  created:
    - core/model/secretref.go
    - core/store/dto.go
    - core/store/git.go
    - core/store/errors.go
    - core/model/secretref_test.go
    - core/store/dto_test.go
    - core/store/git_test.go
  modified:
    - core/model/profile.go

key-decisions:
  - "SecretRef placement = Entry.Secret *SecretRef (omitempty) with a cleared Value when set — NOT a kind:key string replacing Value (critical decision #2 / RESEARCH Q1)"
  - "Serialization via a store-local DTO, not json tags on model.Entry (critical decision #1, option b — keeps the Phase 2 IR struct untouched)"
  - "git driver mirrors introspect.go subprocess shape verbatim; no go-git, no new dependency (critical decision #4 foundation, PROF-01)"
  - "mapGitError maps every git failure to ErrGitCommand and never embeds raw stderr (D-11; threat T-03-01)"

patterns-established:
  - "Store-local DTO for lossless deterministic IR serialization (json.MarshalIndent + single trailing newline)"
  - "git-binary subprocess driver with LookPath-guarded construction + 5s-timeout per-call runners (run/runStdin/runCommit)"
  - "zsh-pro-phrased errStore sentinels as the only error vocabulary leaving core/store"

requirements-completed: [PROF-01]

# Metrics
duration: 9min
completed: 2026-06-27
---

# Phase 3 Plan 01: Git-Backed Store Foundation Summary

**Additive `SecretRef` contract in core/model, a lossless+deterministic store-local DTO for `model.Profile`↔`profile.json`, and a git-binary subprocess driver (mirroring `introspect.go`) with its zsh-pro-phrased error vocabulary and the plumbing primitives Plan 02 composes — no new dependency, store fully shell-agnostic.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-27T11:19:53Z
- **Completed:** 2026-06-27T11:28Z
- **Tasks:** 3
- **Files modified:** 8 (7 created, 1 modified)

## Accomplishments

- **`model.SecretRef` (critical decision #2):** new dependency-free `core/model/secretref.go` defining `SecretRef{Kind,Key}` + `SecretRefKind` (keychain/file/cmd), serializing to the locked `{"kind":...,"key":...}` shape (D-09). `Entry` gains exactly one additive `Secret *SecretRef` omitempty field appended after `Dynamic` — Phase 2 declaration order and the regen oracle are untouched (Pitfall 5).
- **Lossless deterministic serialization (critical decision #1):** `core/store/dto.go` maps `model.Profile`↔JSON via a store-local `entryDTO`/`profileDTO` (the DTO carries the json tags, not `model.Entry`). `MarshalProfile`/`UnmarshalProfile` round-trip byte-for-byte; output is deterministic with a single trailing newline; dynamic values (`$HOME/go`) pass through verbatim (no `util.ExpandHome`, threat T-03-02).
- **git subprocess driver (critical decision #4 foundation):** `core/store/git.go` `gitRunner` mirrors `introspect.go` verbatim (5s `context.WithTimeout` + `defer cancel` + `exec.CommandContext` + separate `out`/`errb` buffers). `newGitRunner` guards `exec.LookPath("git")`→`ErrGitAbsent` (D-06). Plumbing primitives (`hashObject`, `catFileExists`, `show`, `forEachRef`, `revParse`, `updateRef`, `isBareRepo`, deterministic-commit `runCommit`) are exposed for Plan 02.
- **zsh-pro error vocabulary (D-11):** `core/store/errors.go` `errStore` typed sentinels; `mapGitError` maps every failure to `ErrGitCommand` and never embeds raw git stderr (threat T-03-01).
- **`core/store` is fully shell-agnostic:** its entire transitive dependency set is `core/model` + stdlib — no shell package at all (verified via `go list -deps`).
- **Phase 2 oracles not regressed:** `make check` green (fmt + vet + lint 0 issues + full `go test ./...`); `TestRegenRoundTrip`/`TestOracleProperty`/`TestCorpusGolden`/`TestEffectiveManaged` all pass.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the additive SecretRef contract to core/model** — `dfcd479` (feat, TDD: RED test→GREEN impl folded into one atomic commit)
2. **Task 2: Lossless deterministic Profile<->JSON serialization** — `541310b` (feat, TDD: RED test→GREEN impl)
3. **Task 3: git subprocess driver + plumbing primitives + zsh-pro errors** — `b7bc802` (feat)

**Plan metadata:** _(this SUMMARY + STATE/ROADMAP/REQUIREMENTS — see final docs commit)_

## Files Created/Modified

- `core/model/secretref.go` (created) — `SecretRef`/`SecretRefKind` value type; zero imports (leaf-package constraint)
- `core/model/profile.go` (modified) — added one `Secret *SecretRef` field to the end of `Entry`
- `core/model/secretref_test.go` (created) — pins kind:key wire shape, the three kinds, round-trip, and the omitempty additive contract
- `core/store/dto.go` (created) — `entryDTO`/`profileDTO` + `MarshalProfile`/`UnmarshalProfile`; imports only `encoding/json` + `core/model`
- `core/store/dto_test.go` (created) — lossless round-trip across all 5 declarative classes + both overrides, determinism, trailing newline, SecretRef round-trip, stable lowercase keys, non-aliased Names
- `core/store/git.go` (created) — `gitRunner` subprocess driver, `mapGitError`, `emptyTreeSHA`, and the plumbing primitives
- `core/store/errors.go` (created) — `errStore` typed zsh-pro-phrased sentinels (D-11)
- `core/store/git_test.go` (created) — stderr-never-leaked mapping, phrased sentinels, empty-tree pin, absence guard, git-guarded bare-repo primitives test, and a full commit-to-branch plumbing + determinism test

## Decisions Made

Followed the plan's resolved critical decisions exactly:
- **Critical decision #1 → store-local DTO** (option b): serialization tags live on `entryDTO`, not `model.Entry`, so the Phase 2 domain struct is never touched for serialization concerns. Documented in the `dto.go` package comment.
- **Critical decision #2 → `Entry.Secret *SecretRef` (omitempty) + cleared Value:** the literal never round-trips; the reference does. SecretRef is global-by-name (RESEARCH Q2).
- **Critical decision #4 foundation → git via the binary, mirroring `introspect.go`:** no `go-git`, no new Go module (`go.mod` unchanged, PROF-01).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Wrong BlockKind constants in the dto_test fixture**
- **Found during:** Task 2 (serialization round-trip test)
- **Issue:** The test fixture referenced `model.KindFunction` (does not exist) and used `KindCommand` for the alias entry. The real constants are `KindFuncDecl` and `KindAlias` (`core/model/block.go`). Build failed to compile.
- **Fix:** Changed the function entry to `model.KindFuncDecl` and the alias entry to `model.KindAlias`. (The DTO round-trip is field-agnostic, but the fixture should use real constants.)
- **Files modified:** core/store/dto_test.go
- **Verification:** `go test ./core/store/` compiles and all 7 dto tests pass.
- **Committed in:** `541310b` (Task 2 commit)

**2. [Rule 3 - Blocking] golangci-lint `unused` blocked the Task 3 commit on the as-yet-uncomposed plumbing primitives**
- **Found during:** Task 3 (git driver — pre-commit hook)
- **Issue:** `runCommit`, `forEachRef`, `revParse`, and `updateRef` are unexported methods the plan explicitly asks to expose for Plan 02 to compose, but they have no caller in this package yet — the `make hooks` pre-commit golangci-lint `unused` check failed the commit. The architecture guardrails require `make check` (incl. lint) to pass.
- **Fix:** Added `TestGitCommitToBranch` to `git_test.go` that exercises the full commit-to-a-non-checked-out-branch path end-to-end (`hashObject`→temp-index `read-tree`/`update-index`/`write-tree`→`runCommit` commit-tree→`updateRef`→`revParse`/`show`/`forEachRef`). This both satisfies the linter via real callers AND strengthens verification: it pins Pattern 1 / D-12 (the load-bearing recipe Plan 02 reuses) and proves commit determinism (Pitfall 2) — chosen over a `//nolint` suppression or a `var _` sink, which would add no value.
- **Files modified:** core/store/git_test.go
- **Verification:** `golangci-lint run ./core/store/` → 0 issues; the new test passes; `make check` fully green.
- **Committed in:** `b7bc802` (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both necessary to make the planned code compile and pass the mandated `make check` gate. The Rule 3 fix de-risks Plan 02 by proving the commit-to-branch plumbing recipe works end-to-end ahead of when Plan 02 composes it. No scope creep — no Init/Read/Commit orchestration was pulled forward.

## Issues Encountered

- The `! grep -q 'core/shell/zsh' <file>` acceptance gates initially "failed" because doc comments referenced `introspect.go` by its full path. Reworded the comments to name the file without the import-path token, so the grep gates (file-level and the `<verification>` package-wide `! grep -rq`) are clean and a reviewer's grep cannot false-positive. The actual import graph never contained the concrete provider (confirmed via `go list -deps`: `core/store` depends on `core/model` + stdlib only).

## User Setup Required

None — no external service configuration required. (The git binary is already present; secret-backend/keychain wiring and the `ZSHPRO_PROFILE`/`ZSHPRO_HOME` env contract are Plan 03 / Ph4-5.)

## Next Phase Readiness

- **Plan 02 (Init/Read/Commit/Branches)** can compose directly against fixed contracts: `MarshalProfile`/`UnmarshalProfile` (the payload), the `gitRunner` primitives (incl. the verified commit-to-branch recipe), `emptyTreeSHA`, and the `errStore` sentinels. The bare-repo init + plumbing recipe is proven by `TestGitCommitToBranch`.
- **Plan 03 (secret exclusion)** has the `SecretRef` type + the round-trip-proven cleared-Value serialization to populate on `Commit`.
- **Ph4/5** have the locked `kind:key` SecretRef shape (global-by-name) to dereference.
- No blockers. `go.mod` unchanged (PROF-01 honored). All Phase 2 regression pins green.

## Self-Check: PASSED

- All 7 created files present + `core/model/profile.go` carries the new `Entry.Secret *SecretRef` field.
- All 3 task commits exist in git history: `dfcd479`, `541310b`, `b7bc802`.
- `make check` green (fmt + vet + lint 0 issues + full `go test ./...`); `core/store` transitive deps = `core/model` + stdlib only (shell-agnostic verified).

---
*Phase: 03-git-backed-store*
*Completed: 2026-06-27*
