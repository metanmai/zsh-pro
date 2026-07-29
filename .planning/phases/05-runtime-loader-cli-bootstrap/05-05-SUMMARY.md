---
phase: 05-runtime-loader-cli-bootstrap
plan: 05
subsystem: runtime-secret-resolution
tags: [go, zsh, cli, secretref, keychain, runtime-emission, typed-nil]
requires:
  - phase: 05-04
    provides: complete runtime transition emission and fail-open loader handling
  - phase: 03-git-backed-store
    provides: redacted SecretRef records and concrete keychain/vault drivers
provides:
  - SecretRef resolution on an activation-only profile copy before manifest construction
  - generic fail-closed resolver diagnostics with no partial emitted source
  - typed-nil normalization for Store, CLI Emitter, shell Emitter, and SecretResolver injection seams
  - composition-root injection of the existing concrete keychain driver into runtime emission
affects: [phase-05-verification, phase-06-ingest, PROF-03]
tech-stack:
  added: []
  patterns: [activation-only secret profile copy, resolver-kind validation, constructor-boundary nil-like normalization]
key-files:
  created: [core/cli/secret.go, core/cli/dependencies.go]
  modified: [core/cli/emitter.go, core/cli/emitter_test.go, core/cli/cli.go, core/cli/cli_test.go, core/cmd/zsh-pro/main.go]
key-decisions:
  - "The CLI owns a narrow SecretResolver seam and validates its backend kind against each persisted SecretRef before retrieval."
  - "Secret values are installed only as temporary RuntimeValue data on an activation copy; persisted Text and Value remain redacted."
  - "Reflection-based nil-like normalization runs at public constructor boundaries so CLI verbs use existing unavailable-dependency errors instead of dereferencing typed nils."
patterns-established:
  - "Resolve every SecretRef before Build, Diff, or shell emission, so a resolver fault can return empty source atomically."
  - "Normalize nilable interface payloads once at injection boundaries instead of scattering unsafe interface-equality checks through command handlers."
requirements-completed: [BOOT-02]
coverage:
  - id: D1
    description: "A persisted SecretRef is resolved only in an activation copy, reaches a live zsh assignment, and leaves the stored profile redacted."
    requirement: BOOT-02
    verification:
      - kind: integration
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterResolvesSecretRefBeforeBuild"
        status: pass
      - kind: unit
        ref: "go test ./core/cli -run 'RuntimeEmitter.*Secret|SecretResolver' -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: "Missing, mismatched, and failing resolvers fail before shell emission and do not disclose fixture data."
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: "core/cli/emitter_test.go#TestRuntimeEmitterSecretResolverFailuresEmitNothing"
        status: pass
    human_judgment: false
  - id: D3
    description: "Typed-nil Store, CLI Emitter, shell Emitter, and SecretResolver dependencies return normal CLI runtime errors without panics."
    requirement: BOOT-02
    verification:
      - kind: unit
        ref: "core/cli/cli_test.go#TestTypedNilDependenciesFailClosedWithoutPanic"
        status: pass
      - kind: other
        ref: "go test ./core/cli -run 'TypedNil|RuntimeVerbs|SecretResolver' -count=1 && go build ./core/cmd/zsh-pro"
        status: pass
    human_judgment: false
metrics:
  duration: 9min
  completed: 2026-07-29
status: complete
---

# Phase 05 Plan 05: Runtime Secret Resolution and Dependency Safety Summary

**Persisted SecretRefs now resolve safely into live zsh activation state while every tested typed-nil runtime dependency fails through normal CLI errors rather than a panic.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-07-29T18:41:47Z
- **Completed:** 2026-07-29T18:51:10Z
- **Tasks:** 2/2
- **Files modified:** 7

## Accomplishments

- Added a CLI-local `SecretResolver` that validates a stored reference's backend kind, retrieves only into an activation copy, and never wraps retrieved values in diagnostics.
- Resolved both target and active profiles before manifest construction, preventing partial source whenever a resolver is absent, mismatched, missing a key, or fails.
- Normalized typed-nil runtime dependencies once at public constructors, while the composition root now injects its already-created keychain/vault driver into the runtime emitter.

## Task Commits

1. **Task 1: Resolve SecretRef values before runtime manifest construction** — `d6e9d32` (RED test), `0a694b9` (implementation)
2. **Task 2: Normalize typed-nil dependencies at CLI and composition boundaries** — `18b1848` (RED test), `8a56f13` (implementation)

## Files Created/Modified

- `core/cli/secret.go` — narrow resolver contract and redaction-preserving profile-copy resolution.
- `core/cli/emitter.go` — resolves references before build/diff/emission and accepts the resolver injection seam.
- `core/cli/dependencies.go` — shared reflection-based nil-like dependency normalizer.
- `core/cli/cli.go` — turns typed-nil Store and CLI Emitter values into the existing unavailable paths.
- `core/cmd/zsh-pro/main.go` — injects the one concrete keychain/vault driver into runtime emission.
- `core/cli/emitter_test.go` and `core/cli/cli_test.go` — real-zsh secret-value, redaction, resolver-fault, and typed-nil panic regressions.

## Decisions Made

- The runtime seam uses only `Retrieve` and `Kind`, so the CLI neither owns backend mutation nor imports the concrete store implementation.
- A resolver error is intentionally collapsed to a generic runtime error: no reference value, backend error text, or generated shell source is returned on failure.
- Nil-like checks cover all reflection-nilable payload kinds and are applied at constructors, preserving existing `cli.fail` behavior for the public verbs.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. The RED tests exposed the intended missing behavior, and normal pre-commit hooks accepted every atomic commit.

## TDD Gate Compliance

Both tasks recorded a failing RED test commit before their matching Green implementation commit:

- Task 1: `d6e9d32` -> `0a694b9`
- Task 2: `18b1848` -> `8a56f13`

## Known Stubs

None. The existing `NotReadyEmitter` remains an intentional fail-closed dependency state, not a rendered-data placeholder.

## Verification

- `go test ./core/cli -run 'RuntimeEmitter.*Secret|SecretResolver' -count=1` — PASS
- `go test ./core/cli -run 'TypedNil|RuntimeVerbs|SecretResolver' -count=1` — PASS
- `go build ./core/cmd/zsh-pro` — PASS
- `go test ./...` — PASS
- `go vet ./...` — PASS
- `make check` — PASS (`golangci-lint` plus the full Go suite)
- Composition-root probe with `git` absent from `PATH`: `hook` exited 0 and `list` exited 1 with the expected unavailable-store CLI error — PASS.

## User Setup Required

None - no external service configuration is required.

## Next Phase Readiness

- Phase 5's runtime secret dereference seam and typed-nil safety gap are closed for the Phase 6 end-to-end ingest path.
- The optional `hyperfine` startup measurement remains a tool-availability check recorded by Plan 05-03; it does not block this resolver/safety plan.

## Self-Check

PASSED

- Confirmed all seven implementation/test artifacts and this summary exist.
- Confirmed TDD commits `d6e9d32`, `0a694b9`, `18b1848`, and `8a56f13` exist in Git history.

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Completed: 2026-07-29*
