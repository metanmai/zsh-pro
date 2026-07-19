---
phase: 04-manifest-builder-emit
plan: 12
subsystem: testing
tags: [zsh, residue, manifest, property-test, mutation-test]
requires:
  - phase: 04-06
    provides: final effective identity and collision-free restoration ownership
  - phase: 04-09
    provides: composed PATH/FPATH list deltas and base-presence restoration
  - phase: 04-11
    provides: atomic secret and storage regressions
provides:
  - source-derived Parse to IR to Build to Diff to Emit zero-residue oracle
  - exact PATH/FPATH base-presence, order, multiplicity, and dynamic-cardinality coverage
  - mutation-resistant state-bookkeeping checks and reverse-zsh ownership scan
affects: [04-UAT, phase-05-loader]
tech-stack:
  added: []
  patterns: [source-derived fixtures, qqqq shell snapshots, discovery-first regression gates]
key-files:
  created: []
  modified: [core/shell/zsh/residue_test.go, core/shell/zsh/invariant_test.go]
key-decisions:
  - "The oracle owns source fixtures and invokes the full production pipeline; it does not duplicate manifest reduction or list composition."
  - "Internal ZP_BASE bookkeeping is excluded from the external shell snapshot while PATH/FPATH observable presence and array content remain byte-visible."
requirements-completed: [SW-01, SW-02]
coverage:
  - id: D1
    description: Source-derived repeated identities and balanced cross-profile sequences restore a byte-identical shell snapshot.
    requirement: SW-02
    verification:
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestZeroResidueFullStateProperty
        status: pass
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestEffectiveIdentitySourcePipeline
        status: pass
    human_judgment: false
  - id: D2
    description: Dynamic PATH/FPATH expansion matches direct zsh for empty and multi-element values, and bookkeeping mutants are rejected.
    requirement: SW-01
    verification:
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestDynamicCardinalitySourcePipeline
        status: pass
      - kind: integration
        ref: core/shell/zsh/residue_test.go#TestResidueStateBookkeepingMutants
        status: pass
    human_judgment: false
  - id: D3
    description: Reverse zsh code construction is confined to emit.go and every final regression gate is discovered before execution.
    requirement: SW-01
    verification:
      - kind: unit
        ref: core/shell/zsh/invariant_test.go#TestReverseSyntaxHasSingleEmitHome
        status: pass
      - kind: other
        ref: discovery-first Phase 04 regression matrix
        status: pass
    human_judgment: false
duration: 31min
completed: 2026-07-19
status: complete
---

# Phase 04 Plan 12: Final Oracle Summary

**The final Phase 04 oracle derives adversarial profiles from accepted zsh source and proves their emitted activation leaves no observable shell residue.**

## Accomplishments

- Replaced hand-built residue manifests with source fixtures that traverse `Provider.Parse -> ir.Build -> activate.Build -> Diff -> Provider.Emit`.
- Covered repeated scalar/function identities, `foo-bar`/`foo.bar`/`foo_bar`, marker-valued prior state, export attributes, and PATH/FPATH unset, empty, and non-empty bases with exact qqqq-encoded snapshots.
- Added direct dynamic `$EXTRA` comparisons against zsh source for empty and multi-element PATH/FPATH expansion, plus targeted stale-applied, export, slot-collision, sentinel, FPATH-presence, and dynamic-cardinality mutants.
- Reworked the ownership invariant to recursively inspect non-test Go string literals under all relevant core directories, excluding only `core/shell/zsh/emit.go`.
- Ran a discovery-first matrix across every Phase 04 break-case package/regex before uncached targeted execution and the full repository suite.

## Task Commits

1. **Task 1: Broaden the production-path zero-residue oracle** - `d60192f` (test)
2. **Task 2: Prove oracle sensitivity and enforce reverse-zsh ownership** - `d60192f` (test)
3. **Task 3: Run the complete discovery-first gap-closure gate** - verification recorded below

## Verification

- `GOTOOLCHAIN=auto go test ./core/shell/zsh -run 'ZeroResidue|EffectiveIdentity|SlotCollision|Sentinel|Export|Path.*Multiplicity|FPATH|BasePresence|DynamicCardinality|Snapshot|ResidueRendererMutants|ResidueStateBookkeepingMutants|ReverseSyntaxHasSingleEmitHome' -count=1` — passed
- Discovery-first matrix for identity/export, escape semantics, list IR/DTO/live behavior, introspection, secret transactions, and final oracle — every row listed at least one test and passed uncached.
- `GOTOOLCHAIN=auto go test ./... -count=1` — passed
- `GOTOOLCHAIN=auto go build ./...` — passed
- `GOTOOLCHAIN=auto go vet ./...` — passed
- `make lint` — passed

## Deviations from Plan

None - the production ownership fixes in `386f5cb` and `50e72ef` were preserved; this plan only added the promised source-derived adversarial oracle and final gates.

## User Setup Required

None.

## Next Phase Readiness

Phase 04's adversarial gap matrix is covered by production-path tests. The remaining phase work is the existing UAT/phase verification workflow before Phase 5 consumes the emitter boundary.

## Self-Check: PASSED

- The source-derived property, mutation suite, ownership invariant, discovery matrix, full tests, build, vet, and lint all passed.
