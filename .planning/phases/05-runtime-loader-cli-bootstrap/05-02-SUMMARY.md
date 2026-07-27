---
phase: 05-runtime-loader-cli-bootstrap
plan: 02
subsystem: shell-runtime
tags: [zsh, installer, loader, atomic-write, fail-open, hyperfine]
requires:
  - phase: 05-01
    provides: sourced runtime loader, CLI emit surface, and Phase 4 emitter wiring
provides:
  - idempotent safe .zshrc bootstrap installer and cached loader
  - fail-closed emitted-code evaluation with last-good tracking
  - zero-subprocess startup check and hermetic hyperfine backstop
affects: [phase-05-exit, phase-06-ingest]
tech-stack:
  added: []
  patterns: [balanced-marker-refusal, symlink-safe-atomic-replace, cached-loader-first, one-block-zsh-validation]
key-files:
  created: [core/cli/install.go, core/cli/install_test.go, scripts/perf-hyperfine.sh]
  modified: [core/cli/cli.go, core/shell/zsh/hook.go, core/shell/zsh/hook_test.go, core/shell/zsh/live_terminal_test.go, .planning/phases/05-runtime-loader-cli-bootstrap/05-CONTRACT.md]
key-decisions:
  - "The runtime cache follows ZSHPRO_HOME when set and otherwise uses HOME/.zsh-pro, matching the installed stub; the existing store's XDG default is left untouched."
  - "A cached loader carries a version marker; upgrades require zsh-pro install to refresh it, while startup remains fail-open if it is stale or unavailable."
  - "Runtime errors are reported as best-effort recovery rather than falsely claimed atomic rollback."
patterns-established:
  - "Refuse malformed marker pairs before any .zshrc write; only balanced regions are safe to replace."
  - "Validate one complete emitted shell block before evaluation; only successful application advances last-good."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: Safe, idempotent .zshrc installer with secure cached loader and fail-open stub.
    requirement: BOOT-01
    verification:
      - kind: unit
        ref: core/cli/install_test.go#TestInstallCreatesAndRefusesUnbalancedMarkersWithoutWriting
        status: pass
      - kind: integration
        ref: core/cli/install_test.go#TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders
        status: pass
    human_judgment: false
  - id: D2
    description: Emitted code is gated, one-block syntax validated, and runtime failure is reported without advancing last-good.
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalLoaderRejectsInvalidEmitWithoutChangingLastGood
        status: pass
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalLoaderGatesEmptyEmitAndReportsRuntimeFailure
        status: pass
    human_judgment: false
  - id: D3
    description: Shell startup stays subprocess-free, with a hermetic optional timing backstop.
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: core/shell/zsh/hook_test.go#TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative
        status: pass
      - kind: other
        ref: scripts/perf-hyperfine.sh
        status: pass
    human_judgment: false
metrics:
  duration: 10min
  completed: 2026-07-27
status: complete
---

# Phase 05 Plan 02: Runtime Bootstrap Safety Summary

**A secure cached-loader installer now preserves user `.zshrc` content, fails open at shell start, and fail-closes profile switches before unsafe emitted code can run.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-07-27T14:36:10Z
- **Completed:** 2026-07-27T14:46:05Z
- **Tasks:** 3/3
- **Files modified:** 8

## Accomplishments

- Added `zsh-pro install`: cached-loader-first, `0o700` directory / `0o600` loader, install-time syntax validation, and fsync-backed symlink-safe `.zshrc` replacement.
- Made malformed BEGIN/END marker configurations fail without writing; balanced duplicates collapse while surrounding user content remains byte-for-byte preserved.
- Hardened the sourced loader with emit exit/empty gates, one-block `zsh -n` validation, xtrace/history protection, last-good tracking, and an honest runtime-failure report.
- Added the always-run zero-subprocess start-path test and a hermetic `hyperfine` script that skips with a note when the optional tool is unavailable.

## Task Commits

1. **Task 1: safe installer and cache** — `43539a9` (feat)
2. **Task 2 RED: emitted-code validation test** — `2d60b73` (test)
3. **Task 2: loader evaluation hardening** — `c587aad` (feat)
4. **Task 3: hermetic perf backstop** — `c304512` (test)
5. **Rule 1 follow-up: preserve emitter invariant** — `0e438bc` (fix)

## Files Created/Modified

- `core/cli/install.go` — secure cached-loader writer and marker-safe installer.
- `core/cli/install_test.go` — installer, stub, hostile-option, and fail-open coverage.
- `core/shell/zsh/hook.go` — gated validation, last-good bookkeeping, and runtime failure reporting.
- `core/shell/zsh/hook_test.go` / `live_terminal_test.go` — structural and live terminal coverage.
- `scripts/perf-hyperfine.sh` — throwaway-ZDOTDIR timing backstop that proves `activate` loaded first.

## Decisions Made

- Cached-loader skew is deliberately visible through a version comment; `zsh-pro install` refreshes the cache after an upgrade. Startup does not execute the binary to self-refresh, preserving the zero-subprocess guarantee.
- The installer honors `ZSHPRO_HOME` or defaults to `$HOME/.zsh-pro`; this is intentionally separate from the existing XDG-based store default so the rendered stub and installed cache always agree.
- Runtime failures report possible partial state and the last-good profile rather than claiming an unavailable general rollback.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected the loader top-level extractor for nested zsh braces**
- **Found during:** Task 2 structural startup test.
- **Issue:** The original extractor treated every `}` inside a function body as the function terminator, causing function-body text to be inspected as shell-start code.
- **Fix:** Track brace depth through nested group blocks before applying the zero-subprocess grep.
- **Files modified:** `core/shell/zsh/hook_test.go`
- **Verification:** `TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative` and full `make check` pass.
- **Committed in:** `c587aad`

**2. [Rule 1 - Bug] Kept loader xtrace hardening compatible with the Phase 4 reverse-syntax invariant**
- **Found during:** Full suite after Task 3.
- **Issue:** `unsetopt xtrace` in the loader violated the invariant reserving reverse emitter syntax to `emit.go`.
- **Fix:** Use equivalent `setopt NOXTRACE` syntax in the runtime loader.
- **Files modified:** `core/shell/zsh/hook.go`
- **Verification:** `TestReverseSyntaxHasSingleEmitHome`, `go test ./...`, and `make check` pass.
- **Committed in:** `0e438bc`

**3. [Rule 2 - Critical functionality] Chose one shared runtime cache location for stub and installer**
- **Found during:** Task 1.
- **Issue:** The existing store's XDG default differs from the plan's required `$HOME/.zsh-pro/loader.zsh` bootstrap location; using either alone would make one side unable to find the other.
- **Fix:** The installer and rendered stub both use `ZSHPRO_HOME` when set, otherwise `$HOME/.zsh-pro`.
- **Files modified:** `core/cli/install.go`
- **Verification:** installer path and cache tests pass.
- **Committed in:** `43539a9`

**Total deviations:** 3 auto-fixed (2 Rule 1, 1 Rule 2). All are localized correctness fixes.

## TDD Gate Compliance

Task 1's initial RED test could not be committed because the pre-commit typecheck rejects intentionally undefined installer symbols; it failed as expected before implementation. Task 2 has a committed RED test (`2d60b73`) that failed before `zsh -n` validation was added, followed by its implementation commit.

## Known Stubs

None. The optional `hyperfine` measurement is intentionally SKIP-with-note when absent; the structural start-path test remains the mandatory gate.

## Threat Flags

None — the installer, cache, and eval boundaries were present in the plan threat model and received the specified mitigations.

## Reconciliation

The Phase 4 `emit.go` re-diff was performed against the live checkout and the blocking checkbox in `05-CONTRACT.md` is checked. The emitter supplies its own scalar helpers, consumes `ZP_BASE_PATH`, and has no speculative loader helper calls.

## Self-Check: PASSED

- Confirmed installer, loader, live tests, and perf harness exist.
- Confirmed commits `43539a9`, `2d60b73`, `c587aad`, `c304512`, and `0e438bc` exist.
- Passed focused tests, `go build ./...`, `go vet ./...`, `go test ./...`, and `make check`; `hyperfine` is absent, so its harness skipped cleanly after the structural gate passed.
