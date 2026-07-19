---
phase: 04-manifest-builder-emit
plan: 06
subsystem: shell-activation
tags: [go, zsh, manifest, restore, export-provenance, identity]
requires:
  - phase: 04-manifest-builder-emit
    plan: 05
    provides: complete residue oracle and production emitter regression harness
provides:
  - Final-effective identity reduction for scalar, function, and option declarations
  - Collision-free presence-aware zsh undo slots with exact export restoration
  - Source-to-live regressions for repeated, punctuation-distinct, sentinel, and export cases
affects: [04-07, 04-12, 05-runtime-loader, shell-switch-regression-suite]
tech-stack:
  added: []
  patterns: [final-occurrence identity reduction, injective hex undo slots, explicit scalar export provenance]
key-files:
  created: []
  modified:
    - core/model/manifest.go
    - core/activate/builder.go
    - core/activate/diff.go
    - core/shell/zsh/emit.go
    - core/shell/zsh/pipeline_test.go
key-decisions:
  - "Manifest ownership is keyed by final effective shell identity, not individual source statements or sanitized spellings."
  - "New scalar manifests carry explicit export provenance; only missing legacy provenance keeps historical export-on-apply behavior."
  - "Undo uses hex-encoded, class-separated slot names and independent presence, value, applied-value, and export-state slots."
patterns-established:
  - "Ordered manifest records reduce by final source occurrence; map-backed aliases retain exact-key lexicographic operation ordering."
  - "Live zsh restoration captures once, refreshes the final applied scalar after each write, and restores presence plus export attributes independently."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Repeated scalar, function, and option declarations reduce to one final effective identity, while legal punctuation-distinct names remain separate."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "core/activate/builder_test.go#TestBuildReducesRepeatedEffectiveIdentities"
        status: pass
      - kind: unit
        ref: "core/activate/builder_test.go#TestBuildPreservesDistinctPunctuationIdentities"
        status: pass
    human_judgment: false
  - id: D2
    description: "Diff and emitted zsh code use collision-free presence-aware slots, preserve former-sentinel data, track final scalar ownership, and restore scalar export state exactly."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestEmitRestoresPresenceSentinelAndExportState"
        status: pass
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestEmitRepeatedScalarTracksFinalAppliedValue"
        status: pass
      - kind: unit
        ref: "core/activate/diff_test.go#TestDiffNormalizesDuplicateFunctionIdentitiesAndExportProvenance"
        status: pass
    human_judgment: false
  - id: D3
    description: "The real parser-to-emitter pipeline applies final declarations and restores original scalar, alias, function, unset, empty, and export states under zsh -f."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/pipeline_test.go#TestPipelineRepeatedCollisionSentinelAndExportRestoration"
        status: pass
      - kind: other
        ref: "GOTOOLCHAIN=auto go test ./... -count=1 && GOTOOLCHAIN=auto go build ./..."
        status: pass
    human_judgment: false
duration: 74min
completed: 2026-07-19
status: complete
---

# Phase 4 Plan 06: Final Identity Restoration Summary

**Manifest activation now owns final shell identities exactly once, reverses collision-safe presence-aware state, and restores scalar export attributes through the live zsh pipeline.**

## Performance

- **Duration:** 74 min
- **Started:** 2026-07-19T05:33:26Z
- **Completed:** 2026-07-19T06:47:29Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments

- Reduced repeated scalar, function, and option declarations to their last effective source occurrence, preserving exact alias names and additive export provenance.
- Added `Scalar.Exported` provenance and propagated it through shell-agnostic activation operations while retaining legacy manifest behavior when provenance is absent.
- Replaced lossy restore-slot sanitization and sentinel absence checks with injective UTF-8 hex slots plus explicit prior presence and export state.
- Refreshed applied scalar ownership after every write, normalized legacy duplicate manifests in Diff, and restored aliases/functions independently across punctuation collisions.
- Added disposable `zsh -f` pipeline coverage for repeated declarations, former-sentinel values, punctuation-distinct identities, and unset/empty/exported scalar baselines.

## Task Commits

1. **Task 1: Reduce repeated declarations to deterministic final identities** — `26ed163` (feat)
2. **Task 2: Make runtime undo slots injective and presence-aware** — `5a31c32` (fix)
3. **Task 3: Prove identity ownership through the real source-to-live pipeline** — `5e0d66d` (test)

## Files Created/Modified

- `core/model/manifest.go` and `core/model/manifest_test.go` — additive scalar export provenance with legacy omission compatibility.
- `core/activate/builder.go`, `builder_test.go`, and `plan.go` — final-occurrence reducers and export-aware activation intent.
- `core/activate/diff.go` and `diff_test.go` — defensive manifest normalization and exported SetScalar propagation.
- `core/shell/zsh/emit.go` and `emit_test.go` — injective class-separated undo slots, independent presence/export capture, and exact restoration tests.
- `core/shell/zsh/pipeline_test.go` — real parse-to-live-zsh regressions for final-identity ownership.

## Decisions Made

- Treat final effective identity as the manifest, drift, and restoration key; source statements are input history only.
- Keep legacy scalar export inference only for omitted provenance, so explicit `false` is authoritative and wire-compatible.
- Capture scalar presence, original value, original export attribute, and applied value separately; user data is never an absence sentinel.

## Deviations from Plan

None - plan tasks executed as specified.

## Issues Encountered

- The initially installed `golangci-lint` was built with Go 1.24 and could not load this Go 1.25 module. The user approved and supplied the small lint-only prerequisite; its four pre-existing v2 findings were committed separately as `8eb64c4`. The refreshed tool passed on every task and final verification run.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None.

## Verification

- `GOTOOLCHAIN=auto go test ./core/model ./core/activate -run 'Manifest|Build|Repeated|Identity|Collision|Empty|Ordering|Export' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/activate ./core/shell/zsh -run 'Diff|Duplicate|Identity|Emit|Slot|Sentinel|RepeatedScalar|Shadow|Collision|Drift|Export' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Pipeline.*(Repeated|Collision|Sentinel|Duplicate|Export|Empty)|PipelinePreserves' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/activate ./core/shell/zsh -count=1`, `GOTOOLCHAIN=auto go test ./... -count=1`, and `GOTOOLCHAIN=auto go build ./...` — passed.
- `golangci-lint run` and `git diff --check` — passed.

## Next Phase Readiness

- Later Phase 4 plans can rely on final-identity manifests and exact scalar export restoration.
- The SW-02 edge-probe item remains intentionally reserved for the final real-zsh oracle in 04-12.

## Self-Check: PASSED

- All ten planned implementation/test files exist and the three task commits are present.
- Targeted tests, live-zsh pipeline tests, full suite, build, lint, and diff checks pass.
- Pre-existing `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` remain untouched and unstaged.

---
*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-19*
