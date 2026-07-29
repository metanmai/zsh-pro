---
phase: 05-runtime-loader-cli-bootstrap
plan: 06
subsystem: bootstrap-cli-safety
tags: [go, zsh, installer, transaction, fail-open, typed-nil]
requires:
  - phase: 05-03
    provides: exact managed-marker scanning and validated cached-loader promotion
  - phase: 05-05
    provides: shared nil-like dependency normalization at CLI composition boundaries
provides:
  - prepared managed-block replacement before runtime-cache mutation
  - regular-file, conditional cached-loader sourcing that survives hostile zsh error options
  - normal runtime diagnostics for literal and typed-nil shell Providers
affects: [phase-05-verification, BOOT-01, BOOT-02]
tech-stack:
  added: []
  patterns: [prepare-before-mutate installer transaction, conditional source-result consumption, constructor-and-dispatch nil guard]
key-files:
  created: []
  modified: [core/cli/install.go, core/cli/install_test.go, core/cli/cli.go, core/cli/cli_test.go]
key-decisions:
  - "Validate and prepare the complete .zshrc replacement before cache directory creation or loader promotion."
  - "Source only a readable regular cached loader, consume both source outcomes in conditional control flow, and keep the hot path free of command substitution or binary calls."
  - "Normalize nil-like Providers in CLI.New and use one availability guard at hook, install, and analyze dispatch points."
patterns-established:
  - "A user-owned structural validation failure must leave cache bytes, cache-directory existence, and bootstrap bytes unchanged."
  - "Every public injected dependency is normalized at construction and guarded before its first method dispatch."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: "Malformed marker rejection preserves both bootstrap bytes and cache state before any installer cache mutation."
    requirement: BOOT-01
    verification:
      - kind: integration
        ref: "core/cli/install_test.go#TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting"
        status: pass
      - kind: other
        ref: "direct binary malformed-marker install preservation probe"
        status: pass
    human_judgment: false
  - id: D2
    description: "A corrupt cached loader cannot abort hostile zsh startup, and literal or typed-nil Providers make hook, install, and analyze emit normal CLI runtime errors without source output, panics, or install writes."
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: "core/cli/install_test.go#TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders"
        status: pass
      - kind: other
        ref: "direct native zsh ERR_EXIT and ERR_RETURN corrupt-cache probes"
        status: pass
      - kind: unit
        ref: "core/cli/cli_test.go#TestTypedNilDependenciesFailClosedWithoutPanic"
        status: pass
      - kind: other
        ref: "go test ./core/cli -run 'TypedNil|Provider|RuntimeVerbs|Install' -count=1"
        status: pass
    human_judgment: false
metrics:
  duration: 12min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 06: Bootstrap Transaction and Provider Safety Summary

**The installer now validates `.zshrc` marker topology before cache promotion, contains corrupt cached-loader failures under hostile zsh options, and fails nil-like Providers through normal CLI diagnostics.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-07-29T22:29:38Z
- **Completed:** 2026-07-29T22:41:36Z
- **Tasks:** 2/2
- **Files modified:** 4

## Accomplishments

- Prepared the entire managed-block replacement in memory before any cache-directory creation, permission repair, or loader promotion; malformed exact markers preserve both user bootstrap bytes and known-good cache state.
- Replaced the unguarded cached-loader source with a regular/readable-file check and conditional source-result consumption, keeping `ZSHPRO_DISABLE`, the zero-subprocess startup path, and a final success status intact.
- Completed Provider nil-like handling at the CLI boundary: hook, install, and analyze now return the established human runtime error without panicking, emitting source, or touching an install target.

## Task Commits

1. **Task 1: Prepare the managed-block transaction before cache promotion and contain corrupt cached-loader startup failures** — `f6b91a6` (RED test), `aaeba5c` (implementation)
2. **Task 2: Normalize shell.Provider at the public CLI boundary** — `4ff1f49` (RED test), `14d9f23` (implementation)

## Files Created/Modified

- `core/cli/install.go` — validates marker structure before cache mutation, sources only a readable regular cache, and guards CLI install dispatch.
- `core/cli/install_test.go` — adds malformed-marker cache-preservation and native-zsh corrupt-cache hostile-option regressions.
- `core/cli/cli.go` — normalizes Provider injection and reuses one provider availability guard for hook and analyze.
- `core/cli/cli_test.go` — covers literal and typed-nil Provider behavior across hook, install, and analyze.

## Decisions Made

- Exact marker validation is a transaction-preparation step, not a post-cache-promotion check; a malformed user file can never replace a known-good loader.
- The startup stub consults only the cached loader file and safely consumes source failure, so it remains usable even when the cached script is corrupt.
- `isNilLike` is applied both in `CLI.New` and at provider dispatch, keeping constructor-boundary behavior robust without importing concrete zsh types into `core/cli`.

## Verification

- `go test ./core/cli -run 'Install|ManagedBlock|Marker|Stub|CachedLoader|TypedNil|Provider' -count=1` — PASS
- `go test ./core/cli -run 'TypedNil|Provider|RuntimeVerbs|Install' -count=1 && go vet ./core/cli` — PASS
- Direct native-zsh corrupt-cache probes — PASS: both `ERR_EXIT` and `ERR_RETURN` printed `SURVIVED` with status 0 after sourcing the installed `.zshrc`.
- Direct binary malformed-marker probe — PASS: `install` exited 1 with `rc_preserved=yes`, `loader_preserved=yes`, and the existing cache directory present.
- `go vet ./...` — PASS

## Known Downstream Issue

`go test ./core/cli -count=1` still fails only in `TestRuntimeEmitterLiveTransitionRemovesAOnlyState` and `TestRuntimeEmitterLiveCheckoutTransitionRemovesAOnlyState`. This is the pre-declared 05-08 emitter/reverse-contract gap; it is outside this plan and was intentionally not weakened, skipped, or repaired here.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Build hygiene] Removed superseded per-path install helpers after one-time path resolution.**
- **Found during:** Task 1 implementation commit.
- **Issue:** `runtimeDir` and `zshrcPath` became unused once `runInstall` correctly resolved both paths once before mutation, and the repository lint hook rejected the commit.
- **Fix:** Removed only the obsolete wrappers; `resolveInstallPaths` remains the single authoritative path-resolution routine.
- **Files modified:** `core/cli/install.go`
- **Verification:** Focused installer tests and the pre-commit `golangci-lint` hook passed.
- **Committed in:** `aaeba5c`

---

**Total deviations:** 1 auto-fixed (Rule 1 build hygiene).
**Impact on plan:** No scope expansion; removal is required by the plan's one-time-resolution transaction design.

## TDD Gate Compliance

- Task 1: `f6b91a6` (failing regression) -> `aaeba5c` (passing implementation)
- Task 2: `4ff1f49` (failing regression) -> `14d9f23` (passing implementation)

## Known Stubs

None. The changed runtime paths use concrete loader/cache state and direct native-zsh evidence.

## User Setup Required

None - no external service configuration is required.

## Next Phase Readiness

- Phase verification can now rely on a non-destructive malformed-marker installer boundary, hostile-option-safe cached-loader startup, and full Provider seam normalization.
- Plan 05-08 remains the owner of the two failing A-to-B runtime-emitter tests recorded above.

## Self-Check: PASSED

- Confirmed all four implementation/test artifacts and this summary exist.
- Confirmed TDD commits `f6b91a6`, `aaeba5c`, `4ff1f49`, and `14d9f23` exist in Git history.

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Completed: 2026-07-29*
