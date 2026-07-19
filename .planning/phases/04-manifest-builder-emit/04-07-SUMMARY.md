---
phase: 04-manifest-builder-emit
plan: 07
subsystem: ingest
tags: [go, zsh, escape-decoding, secrets, runtime-value]
requires:
  - phase: 04-manifest-builder-emit
    plan: 06
    provides: final-identity restoration contract
provides:
  - Context-aware zsh literal escape decoding
  - Exact RuntimeValue-based literal secret persistence
affects: [04-08, 04-12, 05-runtime-loader]
tech-stack:
  added: []
  patterns: [AST-only escape decoding, semantic secret-value selection]
key-files:
  created: []
  modified: [core/shell/zsh/parse.go, core/shell/zsh/dynamic_test.go, core/store/secret.go, core/store/secret_test.go]
key-decisions:
  - "Literal secret capture uses RuntimeValue only; source Value remains regeneration data."
  - "Unsupported or malformed semantic contracts fail closed before backend mutation."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: "Supported unquoted and double-quoted escapes match live zsh without parser execution."
    requirement: SW-01
    verification:
      - kind: integration
        ref: "core/shell/zsh/dynamic_test.go#TestParseRuntimeValueEscapesMatchLiveZsh"
        status: pass
    human_judgment: false
  - id: D2
    description: "Literal secrets, including present-empty values, are stored as exact RuntimeValue bytes and redacted afterward."
    requirement: SW-02
    verification:
      - kind: integration
        ref: "core/store/secret_test.go#TestExcludeSecretsStoresLiteralRuntimeValueExactly"
        status: pass
    human_judgment: false
duration: 3min
completed: 2026-07-19
status: complete
---

# Phase 4 Plan 07: Semantic Escape and Secret Summary

**The parser now decodes the supported zsh literal escape subset by quote context, and secret storage persists exact semantic RuntimeValue bytes without freezing dynamic source.**

## Accomplishments

- Modeled unquoted and double-quoted backslash behavior while retaining fail-closed unsupported syntax and verbatim source `Value`.
- Compared parser runtime values against disposable `zsh -f` assignments, including continuation and concatenated quote contexts.
- Routed literal secret storage through present `RuntimeValue`, including empty strings; dynamic and narrowly defined legacy values remain explicit.
- Rejected malformed literal/unsupported contracts before backend writes and preserved Text/Value/RuntimeValue redaction.

## Task Commits

1. **Task 1: Decode modeled zsh escape semantics without executing source** — `a8454df`
2. **Task 2: Store semantic secret values exactly, including present-empty** — `c913466`

## Verification

- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'Dynamic|RuntimeValue|Escape|Literal|Unsupported' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/store -run 'Secret|RuntimeValue|Literal|Empty|Legacy|Dynamic' -count=1` — passed.
- `GOTOOLCHAIN=auto go test ./core/shell/zsh ./core/store -count=1`, `GOTOOLCHAIN=auto go test ./... -count=1`, `GOTOOLCHAIN=auto go build ./...`, and `golangci-lint run` — passed.

## Deviations from Plan

None - plan executed exactly as written.

## Next Phase Readiness

Semantic literal bytes now remain authoritative through parsing and secret capture; later Phase 4 plans can consume RuntimeValue without source-escape leakage.

## Self-Check: PASSED

- Both task commits and all four planned files exist.
- Protected `.planning/config.json`, `.planning/graphs/`, and `graphify-out/` remain unstaged.
