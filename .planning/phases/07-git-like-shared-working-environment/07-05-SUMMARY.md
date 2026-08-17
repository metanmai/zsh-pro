---
phase: 07-git-like-shared-working-environment
plan: 05
subsystem: git-worktree-persistence
tags: [committed-worktree, dto-compatibility, exact-git-objects, ref-cas, secret-redaction]
requires:
  - phase: 07-01-lossless-worktree-contract
    provides: model-owned committed worktree, live value, projection, and result contracts
  - phase: 07-02-admission-and-live-secret-boundary
    provides: pinned live-secret identity exclusion and fail-closed admission rules
  - phase: 07-04-exact-committed-projection
    provides: canonical complete-worktree zsh regeneration and tombstone semantics
provides:
  - Additive bidirectional legacy/current profile.json compatibility with exact committed projection presence
  - Direct commit-object reads with strict two-entry regular-blob tree authentication
  - Exact current-base branch creation and expected-base worktree publication CAS
  - Secret-redacted source persistence with visible conflict and committed recovery evidence
affects: [07-06-cli, 07-08-live-provider, 07-09-runtime-hooks, 07-10-e2e]
tech-stack:
  added: []
  patterns: [additive wire compatibility, direct object authentication, fixed-tree allowlist, prepared ref transaction, evidence-preserving recovery]
key-files:
  created: []
  modified:
    - core/store/dto.go
    - core/store/dto_test.go
    - core/store/git.go
    - core/store/git_test.go
    - core/store/store.go
    - core/store/store_test.go
key-decisions:
  - "Keep legacy source entries at the historical top-level wire location and add one presence-aware versioned worktree projection, so old readers retain complete Source while current readers fail closed on partial or unknown projection data."
  - "Authenticate caller revisions as direct commit object IDs, parse the complete root ls-tree -z result structurally, and read only the two authenticated blob OIDs; never peel tags or accept symbolic revision/path expressions."
  - "Preserve the source-compatible New signature by deriving the optional narrow shell.WorktreeRegenerator from the injected provider at construction; exact worktree methods fail closed when the seam is unavailable."
  - "Prepare and lock the expected ref transaction before applying secret backend writes, then preserve authoritative committed/recovery evidence when publication is proven after an uncertain response."
patterns-established:
  - "Exact durable worktrees contain only profile.json and profile.zsh as 100644 blobs under a direct commit object."
  - "New shared-worktree mutations carry an explicit current or expected base OID; process-local legacy adapters are deprecated and never consulted for authority."
requirements-completed: [WORK-01, WORK-03, SYNC-02]
coverage:
  - id: D1
    description: "Legacy source-only and additive current committed-worktree DTOs migrate bidirectionally without losing source, presence, ordered values, or tombstones"
    requirement: WORK-03
    verification:
      - kind: unit
        ref: "core/store/dto_test.go#TestDTOCompatibilityLegacyReaderAndWriterBoundary"
        status: pass
      - kind: unit
        ref: "core/store/dto_test.go#TestCommittedWorktreeDTOPresenceVersionAndShapeFailures"
        status: pass
    human_judgment: false
  - id: D2
    description: "Exact revision reads accept only direct commit objects with exactly profile.json and profile.zsh regular blobs whose regenerated source agrees byte-for-byte"
    requirement: WORK-03
    verification:
      - kind: integration
        ref: "core/store/store_test.go#TestCommitWorktreeReadWorktreeRevisionExactRoundTrip"
        status: pass
      - kind: unit
        ref: "core/store/git_test.go#TestReadWorktreeRevisionStrictTreeRecords"
        status: pass
      - kind: integration
        ref: "core/store/store_test.go#TestReadWorktreeRevisionRejectsBlobTreeTagAndSHAshapedInputs"
        status: pass
    human_judgment: false
  - id: D3
    description: "Branch creation uses the supplied exact current base and complete worktree commits publish one fixed tree with the expected parent through ref CAS"
    requirement: WORK-01
    verification:
      - kind: integration
        ref: "core/store/store_test.go#TestCreateFromUsesExactCurrentBaseAndNeverOverwrites"
        status: pass
      - kind: race
        ref: "core/store/store_test.go#TestCommitWorktreeConcurrentRefRaceHasOneWinner"
        status: pass
      - kind: integration
        ref: "core/store/store_test.go#TestCommitWorktreeSHA256ObjectFormat"
        status: pass
    human_judgment: false
  - id: D4
    description: "Secret literals never enter committed blobs or projections, ref mismatches stay visible, backend compensation remains guarded, and proven publication retains recovery evidence"
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: "core/store/store_test.go#TestCommitWorktreeSecretCanariesAreRedactedOrRejected"
        status: pass
      - kind: integration
        ref: "core/store/store_test.go#TestCommitWorktreeCompensatesSecretsWhenRefIsKnownUnchanged"
        status: pass
      - kind: integration
        ref: "core/store/store_test.go#TestCommitWorktreePreservesPublicationAndRecoveryEvidence"
        status: pass
    human_judgment: false
metrics:
  duration: 26min
  completed: 2026-08-17
status: complete
---

# Phase 07 Plan 05: Exact Git Worktree Persistence Summary

**Versioned source-plus-projection DTOs now round-trip through authenticated Git commits, canonical generated zsh, and expected-base ref CAS without weakening legacy reads or secret compensation.**

## Performance

- **Duration:** 26 min
- **Started:** 2026-08-17T12:39:06Z
- **Completed:** 2026-08-17T13:04:57Z
- **Tasks:** 2/2
- **Files created:** 0 implementation files
- **Files modified:** 6 implementation/test files

## Accomplishments

- Added a deterministic additive committed-worktree DTO that preserves exact source IR, projection value presence, ordered lists, options, and tombstones while keeping historical `UnmarshalProfile` reads intact in both migration directions.
- Added direct object authentication and strict `ls-tree -z` parsing: the revision must be an exact commit whose root contains exactly one `100644 blob` each for `profile.json` and `profile.zsh`, with no tag peeling, symbolic expressions, extra paths, malformed records, or mismatched generated source.
- Added `ReadWorktreeRevision`, `CreateFrom`, and `CommitWorktree`; publication stages only the two fixed blobs in a private temporary index, uses the explicit expected parent, and locks the expected ref transaction before applying backend changes.
- Proved SHA-1 and SHA-256 object formats, real-zsh quoted/multiline/present-empty/export/function/list/option/removal behavior, concurrent one-winner CAS, secret redaction/compensation, and committed recovery evidence.
- Deprecated `Current`, `Create`, `Checkout`, and `Commit` as source-compatible legacy adapters and pinned that process-local input cannot affect exact shared-worktree authority.

## Task Commits

1. **Task 1 RED: committed worktree DTO contract** — `8cf9a44` (test)
2. **Task 1 GREEN: exact additive committed-worktree DTO** — `c05aa20` (feat)
3. **Task 2 RED: exact Git worktree contracts** — `f5d4bda` (test)
4. **Task 2 GREEN: exact reads, current-base creation, and ref CAS publication** — `5d1d350` (feat)

## Files Created/Modified

- `core/store/dto.go` — Adds versioned committed source/projection encoding, strict current decoding, exact presence/value validation, and secret identity exclusion.
- `core/store/dto_test.go` — Pins legacy/current compatibility, deterministic exact values, malformed/partial data, defensive copies, and literal/dynamic secret canaries.
- `core/store/git.go` — Extends the exact argv allowlist for direct object type/blob reads and structurally validates the complete fixed root tree.
- `core/store/git_test.go` — Covers minimal Git authority and missing, duplicate, unexpected, wrong-mode/type/OID, malformed, and unterminated tree records.
- `core/store/store.go` — Adds exact revision read, current-base create, fixed-tree candidate construction, expected-ref transaction publication, compensation, recovery evidence, and legacy deprecations.
- `core/store/store_test.go` — Covers exact behavior through real Git/zsh, SHA-1/SHA-256, non-commit objects, derived-source mismatch, races, secret boundaries, compensation, recovery, input validation, and adapter isolation.

## Decisions Made

- Legacy `entries` remain the sole Source location. The additive `worktree` member carries only the current version and complete projection, allowing frozen legacy readers to ignore it without losing Source.
- Exact reads accept only caller-supplied hexadecimal object IDs. `cat-file -t`, complete root `ls-tree -z`, and `cat-file blob` operate on those direct IDs, so tags, refs, revision expressions, and path selectors cannot expand authority.
- `New` and `NewRuntime` retain source compatibility for historical `shell.Regenerator` callers. Providers that also satisfy `shell.WorktreeRegenerator` supply the narrow exact-worktree seam at construction; new exact methods reject a missing seam.
- Worktree commits reuse the hardened prepared `update-ref --no-deref --stdin` transaction. The expected ref is locked before secret writes, a clean non-publication restores backend priors under a no-change guard, and an observed candidate remains committed with recovery-required evidence.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Kept RED commits compatible with mandatory lint hooks**

- **Found during:** Tasks 1 and 2 RED
- **Issue:** Go cannot compile tests against absent functions and methods, while the normal pre-commit hook runs repository lint and may not be bypassed.
- **Fix:** Added minimal compile-safe failing scaffolds in each RED commit; the tests failed on the intended unimplemented behavior and the GREEN commits replaced every scaffold.
- **Files modified:** `core/store/dto.go`, `core/store/git.go`, `core/store/store.go`
- **Verification:** Stub scan is clean and both final exact plan suites pass.
- **Commits:** `8cf9a44`, `f5d4bda`

**2. [Rule 1 - Security Bug] Closed a direct DTO secret bypass**

- **Found during:** Task 2 GREEN threat self-check
- **Issue:** Store publication prepared and redacted literal secrets, but a direct `MarshalCommittedWorktree` caller could supply an unredacted literal `CatSecrets` source; dynamic secret identities also were not pinned out of projection.
- **Fix:** Reject every unredacted non-dynamic secret source at the committed DTO boundary and pin every exact source secret identity, including safe late-bound dynamic source, out of states and tombstones.
- **Files modified:** `core/store/dto.go`, `core/store/dto_test.go`
- **Verification:** The new canary test failed with the literal visible before the fix, then both Task 1 and Task 2 exact suites passed after the fix.
- **Commit:** `5d1d350`

**Total deviations:** 2 auto-fixed (1 blocking, 1 security bug). **Impact:** Both preserve the planned public architecture and strengthen normal-hook and T-07-19 guarantees without adding a new subsystem.

## TDD Gate Compliance

- Each task has a committed RED test before its GREEN implementation commit.
- Task 1 RED failed on the explicit committed-worktree DTO scaffold; Task 2 RED failed on the exact allowlist/tree/parser and Store method scaffolds.
- The security regression added during GREEN was separately observed failing with the literal canary in DTO bytes before its fix.
- Both exact plan verification commands, full package tests, race tests, vet, lint, and normal hooks pass.

## Verification

- Exact Task 1 plan regex suite — passed.
- Exact Task 2 plan regex suite — passed.
- `GOTOOLCHAIN=local go test -race ./core/store -run 'Test(.*CommitWorktree|.*UpdateRef)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/store -count=1` — passed.
- `GOTOOLCHAIN=local go vet ./core/store` — passed.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed across the repository.
- `GOTOOLCHAIN=local go vet ./...` — passed across the repository.
- `golangci-lint run` — passed with 0 issues.
- `git diff --check` — passed; `go.mod` and `go.sum` are unchanged; production `core/store` does not import the concrete zsh provider.
- Every task commit passed the normal pre-commit hook; no hook bypass was used.

## Known Stubs

None. Empty strings, empty lists, all-zero expected refs, and injected lost-response errors in tests are intentional semantic/security fixtures; no placeholder implementation remains.

## Threat Review

- T-07-19 is mitigated by pure source-secret preparation, committed DTO fail-closed redaction checks, pinning of literal and dynamic secret identities, fixed-blob canary tests, backend compensation under a prepared ref lock, and canonical regeneration.
- T-07-20 is mitigated by explicit schema and presence wrappers, deterministic encoding, strict current decoding, exact value validation, partial/unknown rejection, and frozen bidirectional compatibility fixtures.
- T-07-21 is mitigated by the existing scrubbed Git environment and deadlines plus a minimally extended exact argv allowlist that accepts only direct object IDs for new reads.
- T-07-22 is mitigated by explicit current/expected base OIDs, exact commit parents, prepared expected-ref transactions, real concurrent one-winner tests, and visible conflict results.
- T-07-22A is mitigated by direct commit type checks, complete structural `ls-tree -z` parsing, exact two-path `100644 blob` enforcement, authenticated blob-OID reads, and blob/tree/tag/malformed rejection matrices.
- No unplanned network, authentication, external service, or schema trust boundary was introduced.

## Issues Encountered

None unresolved. As in prior Phase 7 plans, the current milestone `REQUIREMENTS.md` has no `WORK-01`, `WORK-03`, or `SYNC-02` rows; completion is recorded in this SUMMARY's required frontmatter and coverage matrix without inventing requirement records.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for Plan 07-06 to replace legacy Store adapter call paths with exact branch/base/revision contracts while preserving visible CAS conflict and recovery evidence.

## Self-Check: PASSED

- All six declared implementation/test files and this SUMMARY exist.
- All four TDD task commits exist in RED then GREEN order.
- Exact DTO/Git gates, real-zsh behavior, SHA-1/SHA-256, full tests, race, vet, lint, secret/threat scans, stub scans, and normal hooks pass.
