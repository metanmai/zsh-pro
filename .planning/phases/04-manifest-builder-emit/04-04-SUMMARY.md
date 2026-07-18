---
phase: 04-manifest-builder-emit
plan: 04
subsystem: activation
tags: [go, zsh, manifest, provenance, integration, tdd]
requires:
  - phase: 04-manifest-builder-emit
    plan: 03
    provides: explicit parser-to-store value and function-body semantics
provides:
  - Backward-compatible manifest provenance for scalar and alias activation values
  - Presence-aware function bodies through Build, Diff, and Emit
  - Real source-to-live-zsh regression coverage for quoted values and functions
affects: [04-05-zero-residue-oracle, 05-runtime-loader, 06-ingest]
tech-stack:
  added: []
  patterns: [explicit provenance with legacy fallback, map-key presence for empty values, source-to-live-shell pipeline testing]
key-files:
  created:
    - core/shell/zsh/pipeline_test.go
  modified:
    - core/model/manifest.go
    - core/model/manifest_test.go
    - core/activate/builder.go
    - core/activate/builder_test.go
    - core/activate/diff.go
    - core/activate/diff_test.go
    - core/shell/zsh/emit.go
    - core/shell/zsh/emit_test.go
key-decisions:
  - "Only absent manifest provenance uses containsDynamic; explicit false remains authoritative even when the decoded value contains a dollar sign."
  - "Function body map-key presence distinguishes a valid empty function from missing activation data."
patterns-established:
  - "New scalar and alias manifest parts always carry explicit dynamic provenance; legacy manifests alone use heuristic fallback."
  - "Target activation validates every function body before constructing any plan operations."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Manifest, Build, and Diff preserve explicit literal-versus-dynamic runtime intent while remaining compatible with legacy JSON."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "go test ./core/model ./core/activate -run 'Manifest|Build|Diff|Dynamic|Runtime|Quoted|Function|Legacy|Unsupported' -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: "Present-empty and multiline function bodies reach emitted assignments and execute correctly in zsh."
    requirement: SW-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/emit_test.go#TestEmitDefinesEmptyAndMultilineFunctions"
        status: pass
    human_judgment: false
  - id: D3
    description: "Real zsh source traverses Parse, IR, Manifest, Diff, and Emit with exact quoted values, aliases, and four function body forms."
    requirement: SW-02
    verification:
      - kind: e2e
        ref: "core/shell/zsh/pipeline_test.go#TestPipelinePreservesValuesAndFunctionsInLiveZsh"
        status: pass
      - kind: other
        ref: "go test ./... -count=1 && go build ./..."
        status: pass
    human_judgment: false
duration: 12min
completed: 2026-07-18
status: complete
---

# Phase 4 Plan 04: Manifest Semantic Pipeline Summary

**Explicit value provenance and presence-aware function bodies now survive the complete source-to-live-zsh activation pipeline without changing the v1 manifest fields.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-07-18T18:23:36Z
- **Completed:** 2026-07-18T18:35:12Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments

- Added optional scalar/alias dynamic provenance and function body maps to the manifest while preserving old JSON and the validated `functions.added` array.
- Made `activate.Build` consume the explicit 04-03 semantic contract, drop unsupported or malformed entries, and preserve present-empty function bodies.
- Made `activate.Diff` trust explicit provenance, use heuristic fallback only for legacy manifests, and reject incomplete target functions before producing operations.
- Made the emitter assign valid empty function bodies and added a real Parse → IR → Build → Diff → Emit test under disposable `zsh -f`.
- Proved literal `$HOME` data stays literal while dynamic `$HOME` expands under a controlled child HOME; aliases and empty, multiline, subshell, and redirected functions retain exact behavior and deactivate cleanly without firing a canary.

## Task Commits

Each task was committed through RED and GREEN TDD gates:

1. **Task 1 RED: manifest semantic contract tests** — `a2c964e`
2. **Task 1 GREEN: manifest provenance and function bodies** — `a070360`
3. **Task 2 RED: source-to-live-zsh pipeline tests** — `95d51b3`
4. **Task 2 GREEN: present-empty function emission** — `8c5c83e`

## Files Created/Modified

- `core/model/manifest.go` — additive scalar/alias provenance and function body fields.
- `core/model/manifest_test.go` — populated round-trip, explicit-false, legacy decode, and omission coverage.
- `core/activate/builder.go` — ValueMode-aware selection and fail-closed function body construction.
- `core/activate/builder_test.go` — literal, dynamic, unsupported, malformed, legacy, empty, and multiline cases.
- `core/activate/diff.go` — explicit provenance consumption and target function prevalidation.
- `core/activate/diff_test.go` — legacy fallback/deactivation and function body presence tests.
- `core/shell/zsh/emit.go` — unconditional assignment for validated AddFunc operations.
- `core/shell/zsh/emit_test.go` — callable empty and multiline function coverage.
- `core/shell/zsh/pipeline_test.go` — complete production pipeline exercised in live zsh.

## Decisions Made

- A non-nil scalar `Dynamic` pointer and alias `Dynamic` map-key are authoritative even when false; only absence identifies legacy data eligible for `containsDynamic` fallback.
- `Functions.Bodies[name]` presence, not body non-emptiness, is the activation validity test.
- Target function validation runs before any deactivate operations are constructed, so malformed activation data returns an empty plan.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Tooling] Repository pre-commit linter cannot load the Go 1.25 module**

- **Found during:** Task 1 RED commit
- **Issue:** The installed golangci-lint binary was built with Go 1.24 and rejects the module's Go 1.25 target before analysis.
- **Fix:** After the normal hook reproduced the known incompatibility, task commits bypassed only that hook. Formatting, `go vet`, targeted tests, the complete test suite, and the full build were run independently.
- **Files modified:** None
- **Verification:** `gofmt -l`, `go vet ./...`, `go test ./... -count=1`, and `go build ./...` all passed.
- **Committed in:** All task commits used the documented local-tool workaround.

**Total deviations:** 1 auto-handled tooling blocker. **Impact:** no production behavior or validation scope changed; no dependency was added.

## Issues Encountered

- The local pre-commit toolchain mismatch described above remains an environment issue, not a repository failure.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None.

## Next Phase Readiness

- Plan 04-05 can now build its full-state random residue oracle on a behaviorally correct source-to-live activation pipeline.
- No implementation blockers remain in this plan.

## Self-Check: PASSED

- All nine planned files exist and all four RED/GREEN commits are present in order.
- Targeted model/activate/zsh tests, the full Go suite, build, vet, formatting, and diff checks pass.
- The live-zsh pipeline uses the built Manifest and Plan directly and proves apply plus deactivate behavior without a corrected hand-built operation.
- Unrelated `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` changes remain untouched and unstaged.

---
*Phase: 04-manifest-builder-emit*
*Completed: 2026-07-18*
