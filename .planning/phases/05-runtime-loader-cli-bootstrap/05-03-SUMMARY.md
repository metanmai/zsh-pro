---
phase: 05-runtime-loader-cli-bootstrap
plan: 03
subsystem: installer-reliability
tags: [zsh, installer, filesystem, validation, hyperfine, performance]
requires:
  - phase: 05-02
    provides: cached-loader bootstrap, atomic write primitive, and hermetic performance harness
provides:
  - exact physical-line marker replacement that preserves unmanaged .zshrc bytes
  - fail-closed validated cached-loader promotion with private permissions
  - Bash-3.2-compatible hyperfine harness backed by typed Go JSON decoding
affects: [05-04, 05-05, phase-05-verification]
tech-stack:
  added: []
  patterns: [physical-line marker scanner, checked install-path resolver, validate-before-rename, typed hyperfine JSON decoder]
key-files:
  created: [core/perf/hyperfine.go, core/perf/hyperfine_test.go]
  modified: [core/cli/install.go, core/cli/install_test.go, scripts/perf-hyperfine.sh]
key-decisions:
  - "Only complete physical lines, with an optional CRLF carriage return, delimit zsh-pro managed regions."
  - "Installer path bases fail closed unless HOME and supplied ZDOTDIR/ZSHPRO_HOME values are non-empty absolute paths."
  - "A private candidate loader is zsh-validated with a five-second context deadline before atomic promotion."
  - "The optional performance harness delegates JSON parsing and named-result comparison to dependency-free Go instead of Bash regexes."
patterns-established:
  - "Preserve user-owned bytes by scanning line offsets and replacing only balanced exact marker regions."
  - "Keep a known-good cache until a same-directory staged candidate validates and is atomically renamed."
requirements-completed: [BOOT-01, BOOT-02]
coverage:
  - id: D1
    description: Exact physical-line marker scanning preserves unmanaged .zshrc bytes and rejects malformed exact marker ordering.
    requirement: BOOT-01
    verification:
      - kind: unit
        ref: core/cli/install_test.go#TestReplaceManagedBlockOnlyRecognizesExactPhysicalMarkerLines
        status: pass
      - kind: unit
        ref: core/cli/install_test.go#TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting
        status: pass
    human_judgment: false
  - id: D2
    description: Installer rejects unstable path bases, repairs cache permissions, and preserves known-good files on validation failures.
    requirement: BOOT-01
    verification:
      - kind: unit
        ref: core/cli/install_test.go#TestInstallRejectsUnstablePathInputsBeforeMutation
        status: pass
      - kind: unit
        ref: core/cli/install_test.go#TestInstallValidationDeadlinePreservesKnownGoodCache
        status: pass
    human_judgment: false
  - id: D3
    description: Hyperfine result parsing is whitespace-safe and the harness remains Bash-3.2 syntax compatible.
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: core/perf/hyperfine_test.go#TestAddedStartupMillisecondsDecodesWhitespaceAndResultOrder
        status: pass
      - kind: other
        ref: bash -n scripts/perf-hyperfine.sh
        status: pass
    human_judgment: false
  - id: D4
    description: Added interactive startup mean remains below 10 ms on a hyperfine-equipped host.
    requirement: BOOT-02
    verification:
      - kind: manual_procedural
        ref: ZSHPRO_BIN=<built-binary> scripts/perf-hyperfine.sh
        status: pass
    human_judgment: true
    rationale: "Follow-up evidence recorded the real Hyperfine branch with Hyperfine 1.18.0 and a freshly built absolute binary: the harness verified activate was sourced and measured -6.734 ms, below the 10 ms budget."
metrics:
  duration: 19min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 03: Installer and Performance Reliability Summary

**The installer now treats only exact marker lines as managed content, promotes a private validated cache without risking the prior loader, and measures startup through typed Hyperfine JSON decoding.**

## Performance

- **Duration:** 19 min
- **Started:** 2026-07-29T17:34:13Z
- **Completed:** 2026-07-29T17:53:01Z
- **Tasks:** 3/3
- **Files modified:** 5

## Accomplishments

- Replaced substring marker matching with a single-pass physical-line scanner that recognizes only complete LF/CRLF BEGIN and END lines, preserves all unrelated bytes, and rejects malformed exact marker sequences before writing.
- Made installation path selection fail closed, repaired insecure cache modes on reinstall, and staged a 0600 loader candidate for bounded `zsh -n` validation before atomic promotion.
- Reworked the optional timing backstop to use Bash-3.2-compatible shell syntax and a dependency-free Go decoder that compares named Hyperfine result means safely.

## Task Commits

1. **Task 1: exact physical-line marker scanner** — `f8a4497` (RED test), `0a9d055` (implementation)
2. **Task 2: fail-closed paths, cache, and validation** — `f888238` (RED test), `9bb86e0` (implementation)
3. **Task 3: portable JSON-correct Hyperfine backstop** — `eb5990c` (RED test), `1b5ebe3` (implementation)

## Files Created/Modified

- `core/cli/install.go` — exact marker scanning, checked install-path resolution, and private candidate validation/promotion.
- `core/cli/install_test.go` — byte-level hostile-marker, path, mode, validation-failure, deadline, and cleanup regressions.
- `core/perf/hyperfine.go` — standard-library Hyperfine decoder and named-result added-startup calculator command.
- `core/perf/hyperfine_test.go` — whitespace, ordering, and missing-result parsing coverage.
- `scripts/perf-hyperfine.sh` — portable hermetic fixture harness that invokes the typed decoder instead of `mapfile`/regex parsing.

## Decisions Made

- Marker recognition is deliberately exact at the physical-line boundary; marker-looking content in strings, comments, prefixes, or suffixes remains user-owned data.
- An explicitly supplied empty or relative `ZDOTDIR`/`ZSHPRO_HOME` is unsafe configuration rather than a fallback request; only an unset optional variable falls back to the checked absolute `HOME` base.
- Cache validation requires a real `zsh` executable and completes before the cache rename, so an unavailable, invalid, or timed-out validator cannot displace known-good bytes.
- The Hyperfine decoder is a small Go command under `core/perf` so the shell harness can parse arbitrary legal JSON without adding a dependency or relying on Bash 4 features.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Reconciled stale Phase 5 state tracking after the SDK advance landed behind completed summaries**
- **Found during:** Plan metadata update
- **Issue:** STATE.md still pointed at Plan 1 despite completed 05-02 and 05-03 summaries; its frontmatter percentage remained 67 although the progress handler reported 94.
- **Fix:** Advanced the missed completed-plan positions to the real next plan (05-04) and aligned the recorded percentage and activity description with the SDK's calculated progress.
- **Files modified:** `.planning/STATE.md`
- **Verification:** Summary count is 3/5, Current Position is Plan 4 of 5, and both state progress displays report 94%.

---

**Total deviations:** 1 auto-fixed (Rule 1 planning-state consistency).
**Impact on plan:** No production scope change; the execution record now points to the actual next pending plan.

## Issues Encountered

- The pre-commit typecheck rejects undefined symbols in a RED commit. Task 3 therefore used a deliberately failing test-local placeholder, removed by the Green implementation, so the RED behavior remained committed without bypassing hooks.
- The environment blocks cleanup traps containing `rm -f`; verification was rerun without that syntax and did not create project files.

## TDD Gate Compliance

All three tasks recorded a failing RED test commit before their corresponding Green implementation commit. Task 3's placeholder returned the wrong values during RED, so assertions failed while the repository hook could still typecheck the test package.

## Known Stubs

None. The absent-`hyperfine` branch is intentional and explicit; the harness itself remains available for a timing-equipped host.

## Threat Flags

None - the only new filesystem and subprocess surfaces are the installer/cache boundaries covered by this plan's threat model.

## Next Phase Readiness

- Plans 05-04 and 05-05 can rely on non-destructive installer boundaries, validated cached-loader replacement, and a portable performance harness.
- Follow-up timing evidence ran the real Hyperfine branch with a freshly built absolute binary and measured -6.734 ms, satisfying the <10 ms budget.

## User Setup Required

None - no external service configuration is required.

## Self-Check: PASSED

- Confirmed all five implementation/test artifacts and this summary exist.
- Confirmed RED and Green commits `f8a4497`, `0a9d055`, `f888238`, `9bb86e0`, `eb5990c`, and `1b5ebe3` exist in Git history.

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Completed: 2026-07-29*
