---
phase: 04-manifest-builder-emit
plan: 13
subsystem: shell activation
tags: [go, zsh, manifest, ir, zero-residue, regression]
requires:
  - phase: 04-manifest-builder-emit
    provides: semantic ValueMode/ListValue contracts and the activation emitter
provides:
  - Fail-closed declaration routing for unmodeled zsh declaration semantics
  - Legacy-only same-list PATH/FPATH fallback and atomic multi-name functions
  - Parse-to-live-zsh regression evidence for rejected list forms and restoration
affects: [phase-04-verification, phase-05-loader]
tech-stack:
  added: []
  patterns:
    - Declarative admission rejects source semantics not represented by the IR
    - Raw list parsing is a legacy compatibility path, never parsed-source recovery
key-files:
  created: []
  modified:
    - core/ir/route.go
    - core/activate/builder.go
    - core/shell/zsh/pipeline_test.go
key-decisions:
  - "typeset, declare, local, and readonly remain verbatim imperative because scope and attributes are not modeled."
  - "Only ValueModeLegacy may use raw list fallback, and it must reference its own canonical list."
  - "Function declarations are admitted atomically across every valid declared name when a body is present."
patterns-established:
  - "Use Parse -> IR -> Build -> Diff -> Emit -> zsh -f tests for source-fidelity regressions."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: Unmodeled declaration source is retained as imperative rather than a manifest scalar.
    requirement: SW-01
    verification:
      - kind: unit
        ref: core/ir/build_test.go#TestBuildDeclarationFormsStayImperative
        status: pass
    human_judgment: false
  - id: D2
    description: Parsed list gaps are inert, legacy fallback stays same-list, and multi-name functions apply and restore.
    requirement: SW-02
    verification:
      - kind: e2e
        ref: core/shell/zsh/pipeline_test.go#TestPipelineRejectsUnsupportedListForms and TestPipelineMultiNameFunctionRoundTrip
        status: pass
      - kind: other
        ref: GOTOOLCHAIN=auto go test -count=1 ./...
        status: pass
    human_judgment: false
duration: 3min
completed: 2026-07-27
status: complete
---

# Phase 04 Plan 13: Source Fidelity Gap Closure Summary

**Fail-closed source admission prevents lossy declarations and rejected lists from reaching emitted zsh, while multi-name functions now apply and restore every identity.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-27T09:37:50Z
- **Completed:** 2026-07-27T09:41:11Z
- **Tasks:** 3/3
- **Files modified:** 7

## Accomplishments

- Routed `typeset`, `declare`, `local`, and `readonly` assignments to the existing imperative/verbatim path while preserving ordinary assignment and export behavior.
- Restricted raw PATH/FPATH splitting to explicit legacy data with exactly one matching-list base reference; parser-rejected forms are now inert.
- Added all-or-nothing multi-name function reduction and real `zsh -f` proof that both names apply a profile body and regain distinct prior bodies.

## Task Commits

1. **Task 1: Route declarations with unrepresented semantics out of managed IR** - `7bb43e0` (test), `87d2ea8` (fix)
2. **Task 2: Fail closed on parsed list gaps and faithfully reduce multi-name functions** - `dc5dc5d` (test), `df7d8bd` (fix)
3. **Task 3: Pin source-to-live behavior for rejected forms and two-name function restoration** - `6bc797f` (test)

## Files Created/Modified

- `core/ir/route.go` - rejects declaration commands whose semantics cannot be faithfully represented.
- `core/ir/route_test.go`, `core/ir/build_test.go`, `core/shell/zsh/parse_test.go` - parser and IR admission regressions.
- `core/activate/builder.go`, `core/activate/builder_test.go` - legacy-only same-list fallback and atomic multi-name reduction.
- `core/shell/zsh/pipeline_test.go` - real Parse-to-Emit zsh behavior coverage.

## Decisions Made

- Declaration attributes and local scope remain imperative until the IR represents them.
- Parsed source never regains list semantics from raw `Value`; only explicit legacy profiles retain that compatibility path.
- A function declaration is accepted only when its body and every declared name are valid, preventing partial operations.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected an ineffectual map initialization in the narrowed legacy fallback.**
- **Found during:** Task 2
- **Issue:** The initial same-list base map declaration triggered the required linter's `ineffassign` check.
- **Fix:** Declared the map without an unused initial allocation before assigning the canonical-list map.
- **Files modified:** `core/activate/builder.go`
- **Verification:** Task test command, full Go suite, `go vet`, and `make check` passed.
- **Committed in:** `df7d8bd`

**Total deviations:** 1 auto-fixed (Rule 1).
**Impact on plan:** The correction was local to the planned fallback hardening and introduced no scope change.

## Issues Encountered

None.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./core/ir ./core/shell/zsh -run 'Test(RouteManaged|Build.*Declaration|Parse.*Declaration)'`
- `GOTOOLCHAIN=auto go test -count=1 ./core/activate -run 'TestBuild(.*Path.*|.*List.*|.*Function.*)'`
- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run 'TestPipeline(RejectsUnsupportedListForms|MultiNameFunctionRoundTrip)$' -v`
- `GOTOOLCHAIN=auto go test -count=1 ./...`
- `GOTOOLCHAIN=auto go build ./...`
- `GOTOOLCHAIN=auto go vet ./...`
- `make check`
- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run 'Test(ZeroResidueFullStateProperty|ReverseSyntaxHasSingleEmitHome)$' -v`

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 4 now has executable source-fidelity evidence for the verifier-identified declaration, semantic-list, and multi-name-function gaps. The existing zero-residue and reverse-syntax guards remain active and green.

## Self-Check: PASSED

All seven modified source/test files and all five task commits were found in the repository.

*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-27*
