---
phase: 04-manifest-builder-emit
plan: 10
subsystem: ingest
tags: [go, zsh, introspection, protocol-framing]
requires:
  - phase: 04-manifest-builder-emit
    plan: 05
    provides: identity introspection protocol
provides:
  - Fail-closed source-status reporting for zsh introspection
  - Prefix-only identity parsing with NUL-framed body preservation
affects: [04-12, 05-runtime-loader]
tech-stack:
  added: []
  patterns: [source-status gate, prefix-body protocol partition, NUL record framing]
key-files:
  created: []
  modified:
    - core/shell/zsh/introspect.go
    - core/shell/zsh/introspect_test.go
key-decisions:
  - "A nonzero sourced configuration status exits before any protocol output, so Available cannot mask a source failure."
  - "Only the bytes preceding the first body header enter the line-oriented identity parser; body payloads remain NUL-framed."
requirements-completed: [SW-02]
coverage:
  - id: D1
    description: "Missing, syntax-invalid, and explicitly failing sourced files return an error and an unavailable identity set, while an empty valid file remains available."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/shell/zsh/introspect_test.go#TestIntrospectFailureAndEmpty"
        status: pass
    human_judgment: false
  - id: D2
    description: "All exact protocol sentinels inside a function body round-trip without creating identity records outside the body."
    requirement: SW-02
    verification:
      - kind: unit
        ref: "core/shell/zsh/introspect_test.go#TestParseIntrospectSentinelBodyDoesNotPolluteIdentityPrefix"
        status: pass
    human_judgment: false
duration: 4min
completed: 2026-07-19
status: complete
---

# Phase 4 Plan 10: Honest Introspection and Framing Summary

**Introspection now fails closed when the requested zsh configuration cannot be sourced, and marker-shaped body data cannot corrupt line-framed identity parsing.**

## Accomplishments

- Captured the source command status immediately and exited before protocol emission on any nonzero result.
- Replaced permissive missing-file behavior with unavailable-plus-error assertions and covered syntax, explicit-return, and valid-empty inputs.
- Partitioned identity prefix parsing from NUL-framed alias/function body payloads at the first body header.
- Added an adversarial body fixture containing every exact protocol sentinel and confirmed byte-identical recovery without unrelated identity pollution.

## Task Commits

1. **Task 1: Fail introspection immediately when source fails** — `24eaac3`
2. **Task 2: Isolate line identity parsing from NUL-framed body bytes** — `b9fc335`

## Checks

- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Introspect.*(Missing|Failure|Syntax|Empty|Unavailable)' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Introspect|Body|Sentinel|Framing|Marker' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Introspect|Body|Sentinel|Failure' -count=1`, `GOTOOLCHAIN=auto go test ./... -count=1`, and `GOTOOLCHAIN=auto go build ./...` — passed.
- Both task commits ran the repository pre-commit `golangci-lint run` hook with zero issues.

## Deviations from Plan

None - plan executed exactly as written.

## Next Phase Readiness

The introspection protocol now exposes failure honestly and keeps body bytes isolated; later restoration work can rely on availability and body records without marker collisions.

## Self-Check: PASSED

- Both task commits and the two planned source files exist.
- Protected `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` remain unstaged.
