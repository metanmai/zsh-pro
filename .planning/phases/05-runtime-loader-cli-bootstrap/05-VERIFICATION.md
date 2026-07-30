---
phase: 05-runtime-loader-cli-bootstrap
verified: 2026-07-30T20:16:48Z
status: passed
score: "4/4 must-haves verified"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 0/4
  gaps_closed:
    - "Corrupt cached loader startup under ERR_EXIT/ERR_RETURN"
    - "Unbounded and error-propagating public list behavior"
    - "Incomplete active-profile transition and main-profile reversal"
    - "Malformed-marker cache mutation ordering"
    - "Typed-nil Provider and runtime dependency panics"
    - "Secret-bearing profile reversal after resolver loss"
    - "TMPDIR staging replacement and unset-sentinel collision"
  gaps_remaining: []
  regressions: []
behavior_verified_items:
  - truth: "The file-sourced startup hot path adds less than 10 ms of interactive-shell mean startup cost."
    test: "Built an absolute binary and ran ZSHPRO_BIN=<absolute-built-binary> scripts/perf-hyperfine.sh with Hyperfine 1.18.0 staged from the Ubuntu package in a disposable directory."
    result: "The harness verified activate is a sourced function, exited 0, and reported added startup mean: -6.734 ms."
---

# Phase 5: Runtime Loader + CLI + Bootstrap Verification Report

**Phase Goal:** Wire the live terminal to the binary and ship the user-facing surface: a sourced loader, profile verbs, an idempotent fail-open bootstrap block, and a fast hot path.

**Verified:** 2026-07-30T20:16:48Z
**Status:** passed — functional closure and the required startup timing measurement are verified.
**Re-verification:** Yes — after gap-closure Plans 05-03 through 05-08.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `checkout`/`activate`/`deactivate` mutate the current sourced zsh; `list` and `status` report terminal/profile state. | ✓ VERIFIED | `hook.go` defines all five sourced functions and routes explicit operations through `_zp_switch` / `_zp_run_bounded`. Uncached native-zsh tests passed for current-shell zero-residue switching, active-marker inheritance handling, public-verb failure survival, list, and exact argument arity. `TestRuntimeEmitterLiveTransitionRemovesAOnlyState` also exercised real Store/emitter-generated main→B→deactivate source. |
| 2 | The installer is safe and idempotent: one managed BEGIN/END block preserves unmanaged bytes, and failed installation leaves known-good state intact. | ✓ VERIFIED | `install.go` prepares marker replacement before cache mutation; its line scanner accepts only exact physical marker lines. Named uncached tests passed for byte-preserving idempotence, malformed-marker non-mutation, candidate-validation preservation, late `.zshrc` promotion rollback, symlink/mode policy, and built-binary trailing-argument non-mutation. |
| 3 | Missing, corrupt, slow, or failing runtime dependencies do not lock an interactive terminal; emitted code is validated before evaluation and runtime data stays private. | ✓ VERIFIED | The installed stub conditionally consumes a readable regular cached loader, and `hook.go` turns expected runtime failures into diagnostics plus a zero-return public call. Native-zsh tests passed for corrupt-cache `ERR_EXIT`/`ERR_RETURN`, invalid/empty emit, validator/emitter timeout, descriptor-root and shared-TMPDIR races, retained secret reversal, and typed-nil dependency failure. |
| 4 | The file-sourced hot path is subprocess-free and adds less than 10 ms of interactive-shell startup cost. | ✓ VERIFIED | `TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative` passed and `hook.go` keeps command execution inside verb bodies. With a freshly built absolute binary and staged Hyperfine 1.18.0, `scripts/perf-hyperfine.sh` verified `activate: function`, exited 0, and measured an added startup mean of -6.734 ms. |

**Score:** 4/4 truths verified

### Re-verification of the Previous Blockers

| Previous blocker | Current code and behavioral evidence | Status |
| --- | --- | --- |
| Corrupt cache aborts hostile shell startup | `renderInstallBlock` conditionally sources only a regular readable cache and consumes both outcomes; `TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders` passed under `ERR_EXIT` and `ERR_RETURN`. | ✓ CLOSED |
| `list` bypasses the fail-open boundary | `list()` calls `_zp_run_bounded`; `TestLiveTerminalListUsesBoundedFailOpenBoundary` and direct hostile-option matrix coverage pass. | ✓ CLOSED |
| A-to-B/main transition leaves residue | The emitter pairs target apply with retained target reverse; the loader reverses the active profile before applying the next payload. Real Store/native-zsh transition tests passed. | ✓ CLOSED |
| Malformed markers replace a known-good cache | `replaceManagedBlock` runs before cache directory creation/promotion; malformed-marker test cases prove both existing-cache and absent-cache preservation. | ✓ CLOSED |
| Typed-nil seams panic | `isNilLike` normalizes Provider, Store, emitter, and resolver seams before dispatch; the public typed-nil matrix passed without a panic or install mutation. | ✓ CLOSED |
| Resolver loss strands an active secret | Apply payloads retain the active reverse in the shell; secret-loss tests prove deactivate/switch do not re-resolve the old profile. | ✓ CLOSED |
| TMPDIR staging and sentinel collision | Runtime capture/validation use process-owned pipes plus descriptor-bound roots; environment presence is recorded separately from value. Shared-directory race and legacy-sentinel tests passed. | ✓ CLOSED |

## Required Artifacts

All listed artifacts passed existence, substantive-content, and wiring inspection. The automated artifact checker had false negatives for a few plan entries written as component prose rather than file paths/patterns; those links were traced manually below rather than accepted on file existence alone.

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/shell/zsh/hook.go` | Embedded loader, public verbs, error boundary, retained reverse lifecycle | ✓ VERIFIED | 571 substantive lines. `HookScript()` returns the constant; public functions validate arity, consume expected failures, and use non-exported `ZP_ACTIVE_PROFILE`. |
| `core/cli/cli.go`, `core/shell/provider.go`, `core/cli/store.go` | CLI commands and interface seams | ✓ VERIFIED | `hook`, `install`, `list`, `status`, `emit`, and private `runtime` dispatch are present; Provider/Store availability is guarded before use. |
| `core/cli/install.go`, `core/cli/cache_directory_unix.go` | Transactional bootstrap/cache installer | ✓ VERIFIED | Exact line marker scanner, symlink-resolved `.zshrc` target, descriptor-safe 0700/0600 cache handling, candidate validation, and compensation paths are implemented. |
| `core/cli/runtime.go`, `runtime_root_unix.go`, `store_root.go` | Bound runtime transport and storage-root security | ✓ VERIFIED | Capture validates an absolute private root with no-follow descriptor traversal, builds a descriptor-bound Store, keeps source in pipes, and bounds calls with context. |
| `core/cli/emitter.go`, `core/cli/secret.go`, `core/cli/dependencies.go` | Target-only source emission, secret resolution, nil safety | ✓ VERIFIED | Target is validated/read/resolved before `Build`/`Diff`; apply includes retained reverse source; errors return no partial output. |
| `core/cmd/zsh-pro/main.go` | Concrete composition root | ✓ VERIFIED | Injects `zsh.Provider`, Store initialization, descriptor-bound runtime Store factory, and the secret resolver without importing concrete zsh into production `core/cli`. |
| `core/*/*_test.go` Phase 5 suites | Behavioral regression evidence | ✓ VERIFIED | Named tests above run native `zsh`, real Store/emitter fixtures, hostile options, race scenarios, and a built process where the command boundary matters. |
| `core/perf/hyperfine.go`, `scripts/perf-hyperfine.sh` | JSON-correct timing harness | ✓ VERIFIED | Decoder test and `bash -n` pass. The actual Hyperfine branch ran with a freshly built absolute binary and measured -6.734 ms added startup mean. |

## Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `core/cmd/zsh-pro/main.go` | `core/cli` | `NewRuntimeEmitterWithRuntimeStore` and `NewWithStoreInitializer` | ✓ WIRED | Composition root injects concrete provider, Store factory, and resolver. |
| `CLI.Run("hook")` | `zsh.Provider.HookScript()` | Provider interface dispatch | ✓ WIRED | Provider availability guard precedes source output; typed-nil public tests pass. |
| Loader `activate`/`checkout`/`list` | `zsh-pro runtime capture` | `_zp_run_bounded` → descriptor-bound Store/emitter/list | ✓ WIRED | The helper accepts only emit/list shapes, authenticates the root, and returns source/output over process pipes. |
| Loader source | `zsh-pro runtime validate` | pipe-fed `zsh -n` before `eval` | ✓ WIRED | Invalid source, timeout, and empty output paths are covered by named native-zsh tests. |
| Runtime emitter | Store → SecretResolver → `activate.Build`/`Diff` → zsh runtime emitter | target-only apply plus retained reverse payload | ✓ WIRED | Resolver test confirms live assignment without persistent-profile mutation; resolver-loss and main→B tests prove reverse order. |
| Installer | cached loader + managed `.zshrc` | prepared writes, validation, promotion/rollback | ✓ WIRED | Malformed markers stop before cache creation; a later `.zshrc` promotion failure restores the cache. |

## Data-Flow Trace (Level 4)

| Artifact / behavior | Data variable | Source | Produces real data | Status |
| --- | --- | --- | --- | --- |
| Profile switch | captured emitted block | Loader → `runtime capture` → descriptor-bound Store → `runtimeEmitter.Emit` → pipe | Yes — real profile Store and resolved SecretRef paths are exercised. | ✓ FLOWING |
| Reversal | `ZP_ACTIVE_REVERSE_FN` | Target-specific reverse emitted with apply and retained in sourced shell | Yes — main transition and resolver-loss tests invoke the retained function without a fresh active-profile read. | ✓ FLOWING |
| Branch list | captured listing | Loader → `runtime capture` → descriptor-bound `Branches` | Yes — output prints only after a successful bounded call. | ✓ FLOWING |
| Bootstrap | candidate loader and prepared `.zshrc` bytes | exact marker scan → prepared writes → promotion/compensation | Yes — byte-level installation/rollback tests exercise real files. | ✓ FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Built binary rejects invalid installer arguments without mutation | `go test -count=1 ./core/cli -run '^TestBuiltBinaryInstallRejectsTrailingArgumentsWithoutMutation$' -v` | Fresh and preexisting HOME fixtures passed; exit 2 and no store/cache/`.zshrc` change. | ✓ PASS |
| Bootstrap idempotence and transaction safety | Named uncached `TestInstallIdempotentPreservesUserContent`, `TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting`, `TestInstallInvalidCandidatePreservesKnownGoodCacheAndCleansStaging`, and `TestInstallRollsBackLoaderWhenPreparedZshrcCannotBePromoted` | All passed. | ✓ PASS |
| Current-terminal transitions and source validation | Named uncached `TestRuntimeEmitterLiveTransitionRemovesAOnlyState`, `TestLiveTerminalLoaderSwitchesCurrentShellWithoutResidue`, and `TestLiveTerminalLoaderRejectsInvalidEmitWithoutChangingLastGood` | All passed in native zsh. | ✓ PASS |
| Hostile shell behavior and deadlines | Named uncached `TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders`, `TestLiveTerminalPublicVerbsFailOpenUnderErrExitAndErrReturn`, and `TestLiveTerminalTimeoutsAreBoundedAndCleanedUp` | All passed; timeout cases completed in about one second each without the outer watchdog. | ✓ PASS |
| Runtime-root and source confidentiality | Named uncached `TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors`, `TestRuntimeCaptureUsesPrivatePipeAndRejectsUnsafeRoots`, `TestLiveTerminalRuntimeTransportIgnoresSharedTMPDIRRace`, and `TestLiveTerminalRuntimeHelperRejectsStickySymlinkReplacementRace` | All passed. | ✓ PASS |
| Secret and dependency boundaries | Named uncached resolver, resolver-failure, retained-secret-reverse, typed-nil, and public-arity tests | All passed. | ✓ PASS |
| Repository quality gate | `GOTOOLCHAIN=auto go build ./... && GOTOOLCHAIN=auto make check` | Build, gofmt check, vet, golangci-lint (0 issues), and full suite passed. | ✓ PASS |
| Cross-platform compilation | Linux amd64, Darwin amd64/arm64, FreeBSD amd64, Windows amd64 `go build ./...` | All five builds passed. Unsupported runtime transport intentionally fails closed at runtime. | ✓ PASS |
| Startup timing | `ZSHPRO_BIN=<fresh absolute binary> scripts/perf-hyperfine.sh` with staged Hyperfine 1.18.0 | Verified `activate: function`, exited 0, and reported `added startup mean: -6.734 ms`. | ✓ PASS |

## Probe Execution

No declared or conventional `scripts/*/tests/probe-*.sh` probes exist for this phase. The only phase timing harness was run directly and skipped only because `hyperfine` is not installed.

## Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| BOOT-01 | 05-01 through 05-08 | Idempotent managed bootstrap plus live profile surface | ✓ SATISFIED | Loader/current-shell transition, exact-marker byte preservation, pre-mutation validation, rollback, and public command arity all have passing behavioral evidence. |
| BOOT-02 | 05-01 through 05-08 | Fail-open, validated, dependency-safe, fast loader | ✓ SATISFIED | Fail-open/error/timeout/privacy/secret/dependency assertions are verified, and the actual Hyperfine harness measured -6.734 ms added interactive startup cost. |

No orphaned Phase 5 requirements were found. Phase 6 addresses end-to-end ingest/PROF-03, not this phase's timing measurement, so the timing item is not deferred.

## Negative Checks and Plan Reconciliation

| Negative property | Evidence | Status |
| --- | --- | --- |
| Malformed managed markers do not mutate known-good cache or unmanaged bootstrap bytes | Direct malformed-marker preservation test passed for seeded and absent cache states. | ✓ VERIFIED |
| Explicit `main` activation does not leave state after switch/deactivate | Real Store/native-zsh transition test passed. | ✓ VERIFIED |
| Resolver loss does not strand secret-bearing state after a successful activation | Stateful resolver and native-zsh retained-reverse tests passed. | ✓ VERIFIED |
| Shared writable TMPDIR cannot substitute or read staged source | Non-sticky shared-directory and sticky symlink-replacement regressions passed. | ✓ VERIFIED |

Plan 01's old literal `ZP_UNSET_SENTINEL` detail is intentionally superseded by Plan 07's stronger presence-metadata design. `hook.go` no longer encodes absence in a value sentinel, and `TestLiveTerminalEnvRestorePreservesPresenceAndLegacyMarkerData` proves literal legacy-marker, empty, and unset values remain distinct. This improves rather than reduces the roadmap's zero-residue behavior; it is not a functional gap. `05-CONTRACT.md` still describes the retired sentinel and should be refreshed as planning-document hygiene, but source and tests follow the later, safer contract.

## Anti-Patterns Found

No unreferenced `TBD`, `FIXME`, or `XXX` markers were found in the Phase 5 source/test files. No placeholder implementation, hardcoded rendered data path, or orphaned runtime artifact was found. The sole textual `placeholder` match is a local test variable for a temporary race-fixture path, not product behavior.

## Disconfirmation Pass

| Check | Finding | Disposition |
| --- | --- | --- |
| Timing requirement | The timing budget was measured with the repository's actual Hyperfine harness against a freshly built absolute binary. | Passed: added mean -6.734 ms, below the strict 10 ms budget. |
| Potentially misleading passing test | `TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative` inspects loader top-level text; by itself it cannot establish millisecond performance. | Counterbalanced by the explicit Hyperfine harness, which now provides the actual timing measurement. |
| External benchmark path | The branch of `scripts/perf-hyperfine.sh` that invokes real Hyperfine ran in a disposable extracted-package directory. | Passed with exit 0 and the recorded -6.734 ms added mean. |

## Completed Timing Verification

The required command was run with a freshly built absolute binary and Hyperfine 1.18.0 staged from the local package candidate into a disposable extraction directory:

```bash
ZSHPRO_BIN=<absolute-built-binary> scripts/perf-hyperfine.sh
```

The harness verified `activate: function`, exited 0, and reported `added startup mean: -6.734 ms`, satisfying the strict `<10 ms` budget.

## Verdict

There are no remaining implementation, runtime, transaction, storage-root, argument-contract, source/package-quality, or timing gaps blocking the Runtime Loader + CLI + Bootstrap goal. All Phase 5 requirements are closed with current code and fresh behavioral evidence. The canonical GSD status is **`passed`**.

---

_Verified: 2026-07-30T20:16:48Z_
_Verifier: the agent (gsd-verifier)_
