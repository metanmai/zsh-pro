---
phase: 05-runtime-loader-cli-bootstrap
plan: 01
subsystem: shell-runtime
tags: [zsh, runtime-loader, cli, emit, profiles]
requires:
  - phase: 03-git-backed-store
    provides: per-terminal profile store and branch validation
  - phase: 04-manifest-builder-emit
    provides: reversible apply/deactivate source generation
provides:
  - sourced zsh loader and five runtime verbs
  - CLI store/emitter seams with real Phase 4 emit integration
  - durable Phase 4/5 reconciliation and blocking re-diff gate
affects: [05-02-bootstrap-fail-open, phase-05-exit]
tech-stack:
  added: []
  patterns: [sourced-loader, cli-local-interfaces, explicit-nil-interface]
key-files:
  created:
    - core/shell/zsh/hook.go
    - core/shell/zsh/hook_test.go
    - core/shell/zsh/live_terminal_test.go
    - core/cli/store.go
    - core/cli/emitter.go
  modified:
    - core/shell/provider.go
    - core/cli/cli.go
    - core/cmd/zsh-pro/main.go
key-decisions:
  - "Phase 4 emit blocks are self-contained for scalar helpers; the loader owns only its named env helpers and terminal base state."
  - "The composition root injects the live Phase 4 runtime emitter because emit.go exists, while NotReadyEmitter remains a fail-closed fallback."
  - "Store initialization failure becomes a nil cli.Store interface so only store-backed verbs fail instead of panicking."
patterns-established:
  - "Runtime shell text stays exclusively in core/shell/zsh and crosses core/cli through HookScript()."
  - "Shell mutations are sourced functions; binary output is emitted through a narrow CLI seam."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: Sourced loader defines five verbs and mutates the current zsh with reversible profile state.
    requirement: BOOT-01
    verification:
      - kind: integration
        ref: core/shell/zsh/live_terminal_test.go#TestLiveTerminalLoaderSwitchesCurrentShellWithoutResidue
        status: pass
      - kind: unit
        ref: core/shell/zsh/hook_test.go#TestHookScriptIsParseableAndDefinesRuntimeSurface
        status: pass
    human_judgment: false
  - id: D2
    description: CLI hook/list/status/emit dispatch uses nil-safe injected seams and validates loader syntax end-to-end.
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: core/cli/cli_test.go#TestRuntimeVerbsUseInjectedSeams
        status: pass
      - kind: other
        ref: go run ./core/cmd/zsh-pro hook | zsh -n
        status: pass
    human_judgment: false
metrics:
  duration: 8min
  completed: 2026-07-27
status: complete
---

# Phase 05 Plan 01: Runtime Loader, CLI, and Bootstrap Summary

**A sourced zsh loader now exposes live profile verbs and reaches Phase 4's reversible emitter through nil-safe CLI seams.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-27T14:25:48Z
- **Completed:** 2026-07-27T14:33:30Z
- **Tasks:** 4/4
- **Files modified:** 11

## Accomplishments

- Recorded the actual Phase 4 emitter surface and a durable, blocking phase-exit re-diff obligation.
- Added a plain, `zsh -n`-clean loader with `checkout`, `activate`, `deactivate`, `list`, `status`, env helpers, per-terminal profile state, and base-PATH capture.
- Added CLI-local Store and Emitter interfaces, fail-closed emission, live Phase 4 profile-to-source adaptation, and explicit nil-interface store wiring.

## Task Commits

1. **Task 1: Phase 4/5 reconciliation contract** — `12a0530` (docs)
2. **Task 2: Runtime loader and Hooker seam** — `0230744` (feat)
3. **Task 3: CLI verbs and narrow seams** — `6b2bf1b` (feat)
4. **Task 4: Composition-root wiring** — `03c03a8` (feat)

## Files Created/Modified

- `core/shell/zsh/hook.go` — embedded sourced loader and runtime state helpers.
- `core/shell/provider.go` — Hooker seam composed into Provider.
- `core/cli/store.go` and `core/cli/emitter.go` — CLI-local abstractions and Phase 4 adapter.
- `core/cli/cli.go` — hook/list/status/emit dispatch with fail-closed handlers.
- `core/cmd/zsh-pro/main.go` — safe store and real emitter injection.

## Decisions Made

- Phase 4's emitted blocks define `zp_capture_scalar`/`zp_restore_scalar` themselves, so the loader does not duplicate speculative helpers.
- Since `emit.go` is present, main injects `NewRuntimeEmitter` rather than leaving a nonfunctional stub; `NotReadyEmitter` remains covered as the closed fallback.
- `err != nil` from `store.New` leaves `cliStore` as a literal nil interface, allowing hook/analyze to work and list/status to report a runtime error instead of panicking.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test bug] Function-definition assertion matched `activate()` inside `deactivate()`**
- **Found during:** Task 2
- **Fix:** Changed the assertion to an anchored function-definition regular expression.
- **Files modified:** `core/shell/zsh/hook_test.go`
- **Verification:** loader unit and live-terminal tests pass.
- **Committed in:** `0230744`

**2. [Rule 2 - Critical functionality] Reconciled to the shipped Phase 4 runtime emitter instead of retaining only the plan-time stub**
- **Found during:** Tasks 1, 3, and 4
- **Fix:** Added `NewRuntimeEmitter`, which validates/reads profiles, builds/diffs manifests, and calls the injected shell emitter. The stub remains fail-closed for unavailable dependencies.
- **Files modified:** `05-CONTRACT.md`, `core/cli/emitter.go`, `core/cmd/zsh-pro/main.go`
- **Verification:** CLI seam tests, `zsh-pro hook | zsh -n`, build, vet, tests, and `make check` pass.
- **Committed in:** `6b2bf1b`, `03c03a8`

**3. [Rule 3 - Blocking] Updated the analyzer test provider for the new composed Hooker interface**
- **Found during:** Task 2
- **Fix:** Added the harmless `HookScript` mock method so the existing analyzer test double still satisfies `shell.Provider`.
- **Files modified:** `core/analyze/analyze_test.go`
- **Verification:** `go test ./...` passes.
- **Committed in:** `0230744`

**Total deviations:** 3 auto-fixed (1 Rule 1, 1 Rule 2, 1 Rule 3). No scope outside the Phase 4/5 runtime boundary.

## TDD Gate Compliance

The RED commands failed as intended for Task 2 and Task 3 before implementation. Their standalone `test(...)` commits could not be created because the repository's mandatory pre-commit lint/typecheck rejects deliberately non-compiling tests, and `--no-verify` was prohibited. The passing tests were committed with their corresponding feature commits; both RED command failures are retained in execution evidence.

## Known Stubs

`NotReadyEmitter` intentionally returns `emit path not yet available` without shell output when dependencies are absent. The main composition root injects the real Phase 4 emitter in this checkout, so this fallback does not block Plan 01's delivered path.

## Next Phase Readiness

Plan 05-02 can add the installer, cached fail-open startup stub, one-block `zsh -n` validation, last-good fallback, and performance guardrails. Before Phase 5 completion, it must complete and check the `RE-DIFF` obligation in `05-CONTRACT.md`.

## Self-Check: PASSED

- Confirmed all loader, CLI, seam, and composition-root files exist.
- Confirmed task commits `12a0530`, `0230744`, `6b2bf1b`, and `03c03a8` exist.
- Confirmed focused tests, `go build ./...`, `go vet ./...`, `go test ./...`, and `make check` pass.
