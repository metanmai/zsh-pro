---
phase: 07-git-like-shared-working-environment
plan: 02
subsystem: live-admission
tags: [worktree, admission, secret-policy, interface-segregation, zsh]
requires:
  - phase: 07-01-lossless-worktree-contract
    provides: validated live identities, snapshots, values, exclusions, and committed projection contracts
provides:
  - Narrow optional live-secret, worktree regeneration, capture, decode, and patch-emission interfaces
  - Classifier-owned production secret verdict for validated live identities
  - Exact materialized ownership, SecretRef pinning, attachment exclusions, and absent-only admission
affects: [07-03-state-service, 07-04-activation-patch, 07-05-store-dto, 07-07-composition, 07-08-live-provider, 07-09-runtime-hooks]
tech-stack:
  added: []
  patterns: [classifier-owned policy injection, category-aware exact identity registry, defensive attachment results, fail-closed typed-nil policy]
key-files:
  created:
    - core/worktree/registry.go
    - core/worktree/registry_test.go
  modified:
    - core/shell/provider.go
    - core/shell/zsh/classify.go
    - core/shell/zsh/classify_test.go
key-decisions:
  - "Live environment secret decisions construct the ingest-equivalent assignment shape and delegate to zsh.Provider.Classify, keeping secretRe as the sole classifier authority."
  - "Registry seeding reduces to the final source occurrence and owns only EffectiveManaged plus Representable identities; SecretRefs retain ownership metadata but fail the single live-value eligibility gate."
  - "Attachment results retain only managed non-secret baseline values and value-free exclusion metadata; safe admission requires an attachment issued by the same registry and exact kind/name absence."
  - "A missing or typed-nil LiveSecretPolicy fails closed with a stable policy-missing reason rather than admitting an unclassified value."
requirements-completed: [WORK-02, SYNC-02]
coverage:
  - id: D1
    description: "Narrow optional shell interfaces preserve the existing broad Provider composite while production live-secret classification reuses the existing zsh classifier"
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/shell/zsh/classify_test.go#TestLiveSecretUsesClassifierPolicy"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/classify_test.go#TestLiveInterfacesStayNarrow"
        status: pass
      - kind: unit
        ref: "core/shell/zsh/classify_test.go#TestLiveSecretImportBoundaries"
        status: pass
    human_judgment: false
  - id: D2
    description: "Registry ownership derives only from final managed representable profile identities and retains SecretRef identity metadata without live-value eligibility"
    requirement: WORK-02
    verification:
      - kind: unit
        ref: "core/worktree/registry_test.go#TestRegistrySeedOwnsOnlyFinalManagedRepresentable"
        status: pass
      - kind: unit
        ref: "core/worktree/registry_test.go#TestPinnedSecretNeverEntersAttachmentValues"
        status: pass
    human_judgment: false
  - id: D3
    description: "Attachment and admission exclude inherited, volatile, bookkeeping, unsafe, unsupported, pinned, and classifier-secret identities while admitting only safe post-attach identities"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "core/worktree/registry_test.go#TestAdmissionOnlyAllowsSafeAbsentAtAttach"
        status: pass
      - kind: integration
        ref: "core/worktree/registry_test.go#TestAttachAndAdmissionWithRealZshProvider"
        status: pass
      - kind: unit
        ref: "core/worktree/registry_test.go#TestAdmissionFailsClosedWithoutAttachmentOrSecretPolicy"
        status: pass
    human_judgment: false
metrics:
  duration: 13min
  completed: 2026-08-17
status: complete
---

# Phase 07 Plan 02: Admission and Live-Secret Boundary Summary

**A classifier-owned secret policy and category-aware registry now admit only exact, safe post-attachment identities while keeping SecretRef and ambient values outside every live value-bearing path.**

## Performance

- **Duration:** 13 min
- **Started:** 2026-08-17T11:02:45Z
- **Completed:** 2026-08-17T11:15:54Z
- **Tasks:** 2/2
- **Files created:** 2
- **Files modified:** 3

## Accomplishments

- Added optional `LiveSecretPolicy`, `WorktreeRegenerator`, `LiveCaptureSource`, `LiveSnapshotDecoder`, and `LivePatchEmitter` seams without widening the legacy broad `shell.Provider` composite.
- Implemented `zsh.Provider.IsLiveSecretIdentity` by validating exact live identities and delegating classifier-equivalent blocks to the existing `Classify`/`secretRe` policy.
- Added a registry that seeds final materialized ownership, pins valid SecretRefs, filters attachment baselines before values escape, and records immutable value-free exclusions.
- Enforced exact absent-at-attach admission with stable reasons for inherited, volatile, bookkeeping, unsafe, unsupported, secret, pinned, missing-policy, and unattached cases.
- Proved the privacy boundary with both fake policies and the concrete zsh provider, including inherited and pinned resolved-literal canaries.

## Task Commits

1. **Task 1 RED: live classifier and interface contract** — `b7e9464` (test)
2. **Task 1 GREEN: classifier-owned live-secret policy** — `a07a94f` (feat)
3. **Task 2 RED: registry ownership and admission contract** — `59481a6` (test)
4. **Task 2 GREEN: materialized registry and absent-only admission** — `bb42bf4` (feat)

## Files Created/Modified

- `core/shell/provider.go` — Declares the narrow optional Phase 7 shell capabilities while leaving `shell.Provider` compatible.
- `core/shell/zsh/classify.go` — Implements the production live-secret verdict through the existing classifier.
- `core/shell/zsh/classify_test.go` — Pins real-provider behavior, broad-interface compatibility, and concrete-zsh import boundaries.
- `core/worktree/registry.go` — Owns materialized identities, SecretRef pinning, attachment filtering, and admission decisions.
- `core/worktree/registry_test.go` — Covers final-occurrence seeding, immutable exclusions, canary privacy, fake policy tables, and real-provider integration.

## Decisions Made

- Reused the classifier's exact assignment/category precedence for live identities; no second regex, Store-side reclassification, or value inspection was introduced.
- Kept pinned SecretRefs owned for identity metadata but made `AllowsLiveValue` the single false gate for baseline, overlay, event, patch, and projection consumers.
- Bound `AttachmentResult` to its issuing registry and kept baseline, exclusions, and present-at-attach state private with defensive-copy accessors.
- Normalized typed-nil policies to missing and failed closed, because admitting before a production secret verdict would violate the plan's privacy boundary.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Made RED scaffolds lint-compatible without bypassing hooks**

- **Found during:** Task 2 RED commit
- **Issue:** The normal pre-commit `golangci-lint` hook rejected private RED scaffold fields that the intentionally unimplemented accessors did not yet read.
- **Fix:** Implemented defensive-copy/presence accessors in the scaffold while leaving ownership and admission behavior unimplemented, re-ran the intended failing suite, and committed through the normal hook.
- **Files modified:** `core/worktree/registry.go`
- **Verification:** The RED suite still failed on ownership/admission behavior; the hook then reported `0 issues`.
- **Commit:** `59481a6`

**2. [Rule 2 - Missing Critical] Failed closed for missing and typed-nil secret policies**

- **Found during:** Task 2 GREEN security review
- **Issue:** A nil-like injected policy could otherwise panic or permit a value without the required production secret verdict.
- **Fix:** Normalized nil-like policies, added the stable `policy-missing` exclusion, required a same-registry attachment, and pinned both paths with tests.
- **Files modified:** `core/worktree/registry.go`, `core/worktree/registry_test.go`
- **Verification:** `TestAdmissionFailsClosedWithoutAttachmentOrSecretPolicy` passes under targeted, scoped, and full repository runs.
- **Commit:** `bb42bf4`

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing critical). **Impact:** Both changes preserve the normal-hook TDD history and strengthen the planned privacy boundary without widening scope.

## TDD Gate Compliance

- Both tasks have RED commits preceding their GREEN commits.
- Each RED run failed on the intended missing classifier or registry behavior, not on syntax/import errors.
- Both GREEN implementations pass their exact task verification commands; no separate refactor commit was necessary.

## Verification

- `GOTOOLCHAIN=local go test ./core/shell/zsh -run 'Test(Classify|LiveSecret|LiveInterfaces)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/worktree ./core/shell/zsh -run 'Test(Registry|Admission|Attach|PinnedSecret|LiveSecret)' -count=1` — passed.
- `GOTOOLCHAIN=local go test ./core/worktree ./core/shell/zsh -count=1` — passed.
- `GOTOOLCHAIN=local go vet ./core/worktree ./core/shell/zsh` — passed.
- `golangci-lint run ./core/worktree/... ./core/shell/zsh/...` — passed with 0 issues.
- `GOTOOLCHAIN=local go test ./... -count=1` — passed across the repository.
- `GOTOOLCHAIN=local go vet ./...` — passed across the repository.
- Structural checks confirmed `secretRe` has one definition and shell-agnostic production packages do not import `core/shell/zsh`.
- All four RED/GREEN commits passed the normal pre-commit hook; no `--no-verify` bypass was used.

## Known Stubs

None. Both compile-safe RED scaffolds were replaced by the GREEN implementations; remaining empty/nil returns are validated absence/error semantics rather than placeholders.

## Issues Encountered

None beyond the resolved normal-hook scaffold lint issue. As in Plan 07-01, the current milestone `REQUIREMENTS.md` has no `WORK-02` or `SYNC-02` rows; completion is therefore recorded in this SUMMARY's required frontmatter and coverage matrix without inventing requirement records.

## Next Phase Readiness

Ready for Plan 07-03 to persist attachment exclusions, pinned ownership, and per-shell revision state against the exact registry contract.

## Self-Check: PASSED

- All five declared implementation/test files and this SUMMARY exist.
- All four RED/GREEN task commits are present in repository history.
- Coverage metadata validates and the plan-scoped tests/vet pass after SUMMARY creation.
