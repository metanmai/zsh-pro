---
phase: 04-manifest-builder-emit
plan: 08
subsystem: ingest
tags: [go, zsh, path, fpath, semantic-list, dto]
requires:
  - phase: 04-manifest-builder-emit
    plan: 07
    provides: AST-decoded literal runtime values
provides:
  - Presence-aware shell-agnostic PATH/FPATH semantic list contract
  - AST-only same-list segmentation with scalar-context dynamic source
  - Backward-compatible list-value profile persistence
affects: [04-09, 04-12, 05-runtime-loader]
tech-stack:
  added: []
  patterns: [semantic list segments, same-list self marker, scalar-context dynamic source, optional DTO]
key-files:
  created: []
  modified:
    - core/model/block.go
    - core/model/profile.go
    - core/ir/build.go
    - core/ir/build_test.go
    - core/shell/zsh/parse.go
    - core/shell/zsh/parse_test.go
    - core/shell/zsh/dynamic_test.go
    - core/store/dto.go
    - core/store/dto_test.go
key-decisions:
  - "A semantic list is present only with exactly one same-list Self marker; nil remains the legacy or unsupported state."
  - "Dynamic list additions retain validated simple-parameter source in scalar context and never claim a fixed element cardinality before zsh evaluates them."
requirements-completed: [SW-01]
coverage:
  - id: D1
    description: "Model and IR preserve valid ordered literal, dynamic, and self list segments without aliasing and reject malformed contracts."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "core/ir/build_test.go#TestBuildCopiesListValueContract"
        status: pass
    human_judgment: false
  - id: D2
    description: "The zsh parser accepts only same-list self references, decodes quoted and escaped literal list data, and preserves dynamic scalar source without execution."
    requirement: SW-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/dynamic_test.go#TestParseListDynamicSegmentsMatchLiveZsh"
        status: pass
    human_judgment: false
  - id: D3
    description: "Profile JSON round-trips ordered list contracts deterministically, retains self-only presence, and omits malformed or legacy list data."
    requirement: SW-01
    verification:
      - kind: unit
        ref: "core/store/dto_test.go#TestRoundTripListValueContract"
        status: pass
    human_judgment: false
duration: 18min
completed: 2026-07-19
status: complete
---

# Phase 4 Plan 08: Semantic PATH and FPATH List Summary

**PATH and FPATH assignments now carry ordered, shell-agnostic semantic list data from zsh AST parsing through IR and deterministic profile JSON, while dynamic additions remain late-bound scalar expressions.**

## Accomplishments

- Added `ListValue` and `ListSegment` with presence-aware Self, literal, and dynamic-source states; IR rejects malformed combinations and deep-copies valid contracts.
- Derived same-list PATH/FPATH segments from the zsh AST without source execution, preserving quoted/escaped runtime bytes and rejecting cross-list, repeated, missing, or complex references.
- Confirmed `$EXTRA` dynamic additions retain zero/one/many tied-list behavior in live `zsh -f` for both PATH and FPATH.
- Persisted optional list contracts through profile JSON with explicit false flags, deterministic ordering, deep copies, self-only presence, legacy omission, and malformed-dynamic omission.

## Task Commits

1. **Task 1: Define presence-aware semantic list types and IR copying** — `cfa05af`
2. **Task 2: Derive same-variable list segments from the zsh AST** — `84c1ed6`
3. **Task 3: Persist semantic list data with legacy compatibility** — `ab19862`

## Checks

- `GOTOOLCHAIN=auto go test ./core/ir -run 'Build|ListValue|ListSegment|DeepCopy|Empty' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Parse|ListValue|PathSemantic|CrossVariable|QuotedPath|Dynamic' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/store -run 'DTO|ListValue|ListSegment|Legacy|Deterministic' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/model ./core/ir ./core/store ./core/shell/zsh -count=1`, `GOTOOLCHAIN=auto go test ./... -count=1`, `GOTOOLCHAIN=auto go build ./...`, and `golangci-lint run` — passed.
- Source scan of `core/shell/zsh/parse.go` found no process, environment, or filesystem expansion API.

## Deviations from Plan

None - plan executed exactly as written.

## Next Phase Readiness

04-09 can consume `Entry.ListValue` to compose repeated PATH/FPATH deltas without reparsing source text or prematurely splitting dynamic scalar expansions.

## Self-Check: PASSED

- All three task commits and the nine planned source/test files exist.
- Protected `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` remain unstaged.
