---
phase: 05-runtime-loader-cli-bootstrap
plan: 07
subsystem: shell-runtime-hardening
tags: [zsh, runtime-loader, err-exit, err-return, private-staging, environment-restoration]
requires:
  - phase: 05-04
    provides: bounded runtime subprocess boundary and fail-open public verbs
provides:
  - bounded, error-consuming public list execution under hostile zsh error options
  - owner-controlled mode-0700 runtime staging with mode-0600 transient files
  - presence-aware environment restoration without a magic unset value
affects: [phase-05-verification, phase-06-ingest, BOOT-01, BOOT-02]
tech-stack:
  added: []
  patterns: [private-per-operation staging child, fail-open read-only runtime verb, value-independent presence metadata]
key-files:
  created: []
  modified: [core/shell/zsh/hook.go, core/shell/zsh/hook_test.go, core/shell/zsh/live_terminal_test.go]
key-decisions:
  - "Explicit runtime verbs stage only below a verified zsh-pro cache root in a per-operation mode-0700 child; shared temporary directories are never trusted."
  - "list shares _zp_run_bounded and reports expected command failures through runtime status globals while returning zero to direct ERR_EXIT and ERR_RETURN callers."
  - "__ZP_ORIG_<safe>_PRESENT records original export presence separately, so all shell strings including the legacy marker remain ordinary data."
patterns-established:
  - "Use _zp_cleanup_private_temp to remove a 0600 file and only its private owner-controlled parent."
  - "Use a separate sanitized presence slot for reversible shell state rather than encoding absence in the value domain."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: "list is deadline-bounded and fail-open for success, missing, failing, and sleeping binaries under direct ERR_EXIT and ERR_RETURN callers."
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalListUsesBoundedFailOpenBoundary
        status: pass
      - kind: other
        ref: "direct zsh -f missing-binary probes under ERR_EXIT and ERR_RETURN"
        status: pass
    human_judgment: false
  - id: D2
    description: "Captured command output and secret-bearing emitted source use a private runtime child and cannot be raced through a mode-0777 shared directory."
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalStagingRejectsSharedTMPDIRReplacement
        status: pass
      - kind: other
        ref: "direct zsh -f private-staging probe with mode-0777 shared directory"
        status: pass
    human_judgment: false
  - id: D3
    description: "Legacy-marker, empty, unset, and user-drifted environment values restore with distinct correct outcomes."
    requirement: BOOT-01
    verification:
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalEnvRestorePreservesPresenceAndLegacyMarkerData
        status: pass
      - kind: other
        ref: "direct zsh -f sentinel, empty, unset, and drift restoration probe"
        status: pass
    human_judgment: false
metrics:
  duration: 19min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 07: Runtime Boundary and Restore Safety Summary

**Public list calls now survive hostile zsh error options, transient runtime source stays in a private cache child, and environment undo preserves every original value without a sentinel collision.**

## Performance

- **Duration:** 19 min
- **Started:** 2026-07-29T22:06:09Z
- **Completed:** 2026-07-29T22:25:19Z
- **Tasks:** 2/2
- **Files modified:** 3

## Accomplishments

- Routed `list` through the same timeout, output-capture, cleanup, and error-consuming boundary used by emit/validation work; successful output is emitted only after a successful child command.
- Replaced arbitrary temporary-directory staging with a verified absolute zsh-pro runtime root, a per-operation owner-controlled `0700` child, `0600` files, and parent-aware cleanup.
- Replaced the magic unset marker with a sanitized `__ZP_ORIG_<name>_PRESENT` slot, preserving literal legacy-marker data while keeping unset, empty, and drifted state distinct.
- Added live-zsh regressions that reproduce a non-sticky shared-directory replacement/read attempt and direct `ERR_EXIT`/`ERR_RETURN` callers.

## Task Commits

1. **Task 1: Route list through the fail-open boundary and stage runtime source only under an owner-controlled directory** — `bc49b55` (RED test), `bcf04b6` (implementation)
2. **Task 2: Replace value-sentinel restoration with explicit original-presence state** — `e6ae0cb` (RED test), `74bcd41` (implementation)

## Files Created/Modified

- `core/shell/zsh/hook.go` — private runtime-root validation and staging, bounded public list behavior, and presence-aware env restoration.
- `core/shell/zsh/hook_test.go` — structural loader contracts for private staging and independent presence metadata.
- `core/shell/zsh/live_terminal_test.go` — direct hostile-option, shared-directory race, cleanup, and restoration regressions.

## Decisions Made

- An explicit runtime operation may repair the permission mode of an owner-controlled cache root, but the loader source path remains filesystem- and subprocess-free.
- Runtime files are created under a unique private child and cleanup removes only that file and its owned parent; a writable shared directory is never a fallback.
- Existing state that lacks presence metadata is treated as data rather than applying an ambiguous historical magic-value interpretation.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test contract] Updated pre-existing live-loader fixtures to provision the newly required private runtime root.**
- **Found during:** Task 1 green verification.
- **Issue:** A few existing fixtures wrote the loader directly and therefore bypassed the test helper that creates the installer-owned runtime root; after removing the shared-directory fallback, those fixtures exercised an intentional handled staging failure instead of their original emit/validation behavior.
- **Fix:** Routed those fixtures through `writeLiveLoader` so they create the same private root as the runtime contract requires.
- **Files modified:** `core/shell/zsh/live_terminal_test.go`
- **Verification:** `go test ./core/shell/zsh -count=1` passed.
- **Committed in:** `bcf04b6`

---

**Total deviations:** 1 auto-fixed (Rule 1 test-contract correction).
**Impact on plan:** The correction keeps prior live-runtime regressions representative after the planned security boundary changed; no production scope expanded.

## Issues Encountered

- The local environment does not provide `hyperfine`; this plan intentionally did not treat that external timing tool as a blocker or alter its existing deferred evidence path.

## TDD Gate Compliance

Both task cycles recorded a failing RED test commit before their corresponding implementation commit:

- Task 1: `bc49b55` -> `bcf04b6`
- Task 2: `e6ae0cb` -> `74bcd41`

## Known Stubs

None. The changed loader paths have concrete runtime roots, commands, cleanup, and live data sources; the secret-like test payload is an intentional adversarial fixture only.

## Verification

- `go test ./core/shell/zsh -run 'LiveTerminal|HookScript' -count=1` — PASS
- `go test ./core/shell/zsh -count=1` — PASS
- `go vet ./...` — PASS
- `make check` — PASS (`gofmt` check, vet, `golangci-lint`, and full Go suite)
- Direct `zsh -f` probes — PASS: missing `zsh-pro` survives under both `ERR_EXIT` and `ERR_RETURN`; mode-0777 shared staging remains empty; legacy-marker, empty, unset, and drifted environment states restore correctly.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase verification can now rely on bounded public list behavior, an owner-controlled transient source boundary, and collision-free environment undo semantics.
- `hyperfine` measurement remains the existing external-tool evidence item; it was not changed by this plan.

## Self-Check: PASSED

- Confirmed all three implementation/test files and this summary exist.
- Confirmed RED and implementation commits `bc49b55`, `bcf04b6`, `e6ae0cb`, and `74bcd41` exist in Git history.

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Completed: 2026-07-29*
