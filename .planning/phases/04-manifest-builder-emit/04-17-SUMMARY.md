---
phase: 04-manifest-builder-emit
plan: 17
subsystem: testing
tags: [go, zsh, persistence, manifest, residue]
requires:
  - phase: 04-manifest-builder-emit
    provides: "04-16 structural-fidelity v2 metadata and representability gate"
provides:
  - "Persisted live-zsh proof that indexed and semantic flagged declarations remain verbatim no-ops."
  - "Byte-identical full-state residue proof for a mixed persisted rejected profile."
affects: [phase-04-verification, phase-05-loader]
tech-stack:
  added: []
  patterns: ["Persisted source-shape tests traverse Parse, DTO re-save, OverrideManaged, Build, Diff, Emit, and zsh -f."]
key-files:
  created: []
  modified: [core/shell/zsh/pipeline_test.go, core/shell/zsh/residue_test.go, core/shell/zsh/parse.go]
key-decisions:
  - "The export delimiter is non-semantic, but export -- NAME=value must still preserve ordinary exported-scalar behavior."
  - "The full-state oracle cleans up emitted test loader helpers before comparing snapshots."
patterns-established:
  - "Rejected persisted source shapes use type/key/arithmetic sentinels plus no-operation manifests in live-zsh tests."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Persisted indexed and flagged declarations stay verbatim and create no activation operation while ordinary exports remain supported."
    requirement: SW-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/pipeline_test.go#TestPipelinePersistedOverrideManagedIndexedAndFlagged"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/pipeline_test.go#TestPipelineExportDelimiterControls"
        status: pass
    human_judgment: false
  - id: D2
    description: "A mixed persisted rejected profile leaves scalar, indexed, associative, and integer-export state byte-identical after apply and deactivate."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/residue_test.go#TestResiduePersistedRejectedStructuralNoop"
        status: pass
      - kind: integration
        ref: "GOTOOLCHAIN=auto go test -count=1 ./..."
        status: pass
    human_judgment: false
duration: 8min
completed: 2026-07-27
status: complete
---

# Phase 04 Plan 17: Persisted Source Fidelity and Residue Summary

**Persisted indexed and integer-export declarations now have live-zsh no-operation and byte-identical residue proof, including supported `export --` controls.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-27T12:35:11Z
- **Completed:** 2026-07-27T12:43:02Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Exercised `FOO[2]=bar`, `MAP[key]=bar`, and `export -i INTEGER=1` through Parse, IR, DTO re-save, persisted override, Regenerate, Build, Diff, Emit, and `zsh -f`.
- Proved numeric/associative type and key state, plus integer-export arithmetic (`3` versus scalar-export `12`), survive apply/deactivate because rejected profiles generate no manifest operations.
- Added the durable complete-snapshot proof for a mixed persisted rejected profile and retained ordinary plus delimiter export behavior.

## Task Commits

1. **Task 1: Exercise persisted rejected source shapes through Diff, Emit, and live zsh** — `a98f9e8` (test)
2. **Rule 1 follow-up: normalize semantic delimiter-export control expectation** — `f6bb54d` (fix)
3. **Rule 1 follow-up: support declaration-word delimiter assignments** — `8b99eb2` (fix)
4. **Rule 1 follow-up: preserve semantic delimiter export controls** — `5bf027f` (fix)
5. **Task 2: Pin mixed rejected-source zero residue with the full-state snapshot oracle** — `a8c89cc` (test)

## Files Created/Modified

- `core/shell/zsh/pipeline_test.go` — persisted production-path discriminators for indexed, flagged, ordinary, and delimiter export forms.
- `core/shell/zsh/residue_test.go` — byte-identical full-state no-residue proof for a mixed rejected persisted profile.
- `core/shell/zsh/parse.go` — handles `export -- NAME=value` as a semantic ordinary export without treating the delimiter as a value.

## Decisions Made

- `export --` is a non-semantic option delimiter, so regeneration may normalize it, but its following assignment must remain a supported exported scalar.
- Snapshot cleanup removes emitted loader helper definitions before comparison, keeping the no-residue oracle focused on persisted profile state.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Restored semantic parsing for `export -- NAME=value`**
- **Found during:** Task 2 full-suite verification.
- **Issue:** The naked delimiter was consumed as a declaration value, causing the following assignment to retain `ValueModeUnsupported` and produce no manifest scalar.
- **Fix:** Excluded naked declaration words from assignment-value capture while preserving real `NAME=value` assignment semantics.
- **Files modified:** `core/shell/zsh/parse.go`, `core/shell/zsh/pipeline_test.go`.
- **Verification:** Focused pipeline tests, complete uncached Go suite, build, vet, and `make check` passed.
- **Committed in:** `8b99eb2`, `5bf027f`.

**2. [Rule 1 - Bug] Removed emitter helper definitions before final snapshot comparison**
- **Found during:** Task 2 residue test execution.
- **Issue:** The test's emitted `zp_*` helper functions appeared only after the baseline snapshot, creating harness residue unrelated to the rejected profile.
- **Fix:** Reused the established cleanup pattern before the after snapshot.
- **Files modified:** `core/shell/zsh/residue_test.go`.
- **Verification:** `TestResiduePersistedRejectedStructuralNoop` and the default full-state property passed.
- **Committed in:** `a8c89cc`.

**Total deviations:** 2 auto-fixed Rule 1 issues. Both were required to make the plan's live-zsh controls discriminating and correct.

## TDD Gate Compliance

The new behavior tests passed immediately because Plan 04-16 had already delivered the source-fidelity implementation they validate. This plan therefore has test commits but no subsequent feature commit; the focused and full live-zsh verification is the acceptance evidence.

## Known Stubs

None. The changed production and test files contain no runtime placeholder or unwired data stub.

## Issues Encountered

`PLAIN+=2` initially triggered the existing drift guard during deactivation, so the control now restores its applied value before deactivation. This validates scalar arithmetic without incorrectly expecting the loader to overwrite a user mutation.

## User Setup Required

None — `zsh` was available and all checks ran locally.

## Next Phase Readiness

Plan 17’s required live-zsh and full-state evidence is complete. Phase status remains executing for the parent workflow’s separate phase-level verification and completion steps.

## Self-Check: PASSED

*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-27*
