---
phase: 06-ingest-end-to-end
verified: 2026-08-08T19:30:23Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
---

# Phase 6: Ingest End-to-End Verification Report

**Phase Goal:** Polish the on-ramp last, against the final IR shape. Compose the already-built pieces into the full path: parse a real `~/.zshrc` → classify (declarative/imperative split) → partial-eval → commit the complete redacted, source-ordered profile to the baseline branch — while preserving every preexisting startup byte outside the canonical managed loader region and warning about post-END appends.

**Verified:** 2026-08-08T19:30:23Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Ingest commits the complete redacted, source-ordered Profile to `main`; `EffectiveManaged` is only a projection and no source statement is silently dropped or reordered. | ✓ VERIFIED | `CLI.Run` dispatches `ingest` to `runIngestCommand` (`core/cli/cli.go:74-75`); the controller prepares the original source, calls `provider.Parse` then `ir.Build`, and commits that full Profile (`core/cli/ingest.go:222-231,324-328`). The exact 12-selector real-provider/real-Store E2E matrix passed, including `TestIngestE2EStoreReadReturnsCompleteOrderedProfile`, `...RegeneratePreservesNonSecretOrderTextAndSemantics`, and the accounting/order tests. |
| 2 | Detected literal secrets are excluded from committed baseline objects by default and the user is told exactly what was withheld. | ✓ VERIFIED | `CommitIngest` invokes `prepareSecrets` before candidate persistence (`core/store/store.go:793-826`); redaction replaces literal entries with inert `SecretRef` metadata and records name/line only (`core/store/secret.go:83-199`). The DTO has no value field (`core/dto/ingest.go:3-25`) and the controller renders only name/line (`core/cli/ingest.go:363,471-475,583-584`). The E2E all-object and ambiguity-quarantine tests, plus built-binary secret-behavior test, passed. |
| 3 | Adoption alters only exact loader-marker regions, is idempotent, preserves every outside byte in order, and warns without clobbering post-END content. | ✓ VERIFIED | `prepareIngestInstallAt` supplies the parse-eligible source and canonical candidate before promotion (`core/cli/ingest.go:222-248`); promotion occurs once before Store commit and finalization never rewrites the target (`core/cli/ingest.go:288,324-376`). The built-binary matrix passed `TestMainIngestExpectedInstalledAndOutsideBytes`, `...Idempotent`, and `...AppendWarningPreservesAndPersists`. |
| 4 | Store-read regeneration, activation projection/SecretRef resolution, and actual built-binary `zsh -f` startup proof all hold; installed startup adds no subprocess. | ✓ VERIFIED | The real-Store E2E matrix passed regeneration, activation-only, resolver, and unmanaged-canary tests. The built-binary matrix passed `TestMainIngestActualInstalledStartupEquivalent`, `...OrderSensitiveDefinitionBeforeUse`, `...AllowsOnlyExactLoaderSymbols`, and `...StartupHasNoSubprocess`. Those tests build and invoke the binary (`core/cmd/zsh-pro/main_test.go:892,1530`), source pristine/installed targets with `zsh -f` (`:1645`), compare observable state, and fail on the stubbed subprocess counter (`:1657`). |
| 5 | **PROF-03:** end-to-end ingest reuses secret detection so committed profiles exclude literals and report withheld entries. | ✓ VERIFIED | The same tested Store redaction → typed `WithheldReport` → value-free CLI output path proves the requirement. `TestIngestE2ESecretAllObjectDatabases` covers reachable, unreachable, packed, and dynamically discovered retained-quarantine objects; `TestIngestE2ESecretAbsentAfterUpdateRefObservationAmbiguity` covers the recovery boundary; the actual binary test verifies behavior without disclosure. |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/cli/ingest.go` | Strict orchestration from source snapshot through full-Profile commit and one startup promotion | ✓ VERIFIED | Substantive controller with Parse → Build → Commit ordering, filesystem-first compensation, safe output, and no source execution path; wired from `CLI.Run`. |
| `core/store/store.go`, `core/store/install_transaction.go`, `core/store/secret.go`, `core/store/git.go` | Main-bound transaction, pre-object redaction, hermetic candidate Git environment | ✓ VERIFIED | `BeginIngest`/`AbortIngest` are intentionally implemented in `install_transaction.go:137,398`, while `CommitIngest` is in `store.go:793`; all are package methods on the same `Store` and are invoked by the controller. Hermetic Git environment disables system/global config and configures private objects/alternates (`git.go:115-126`). |
| `core/cli/install.go`, `core/cli/install_transaction.go` | Exact-marker candidate and authenticated one-promotion filesystem transition | ✓ VERIFIED | Called from ingest preparation/promotion and exercised by built-binary outside-byte/idempotence/append tests. |
| `core/cli/ingest_e2e_test.go` | Real provider/Store complete-profile, activation, privacy, and no-execution evidence | ✓ VERIFIED | Uses actual zsh provider, Git-backed Store, secret backend, controller, and dynamically generated literal; the exact 12-selector matrix passed. |
| `core/cmd/zsh-pro/main_test.go` | Built-binary actual-installed-startup oracle | ✓ VERIFIED | Builds the binary, runs `ingest`, then uses isolated `zsh -f` snapshots, an order-sensitive fixture, exact loader-symbol allowlist, and a subprocess-counter trap. Exact 13-selector matrix passed. |
| `scripts/verify-phase06-macos-runtime.sh` and `06-MACOS-RUNTIME-EVIDENCE.md` | Native Darwin atomic and cleanup evidence | ✓ VERIFIED | Regression script passed locally. GitHub Actions run `31274061615` succeeded on macOS 14 at workflow SHA `aa0f996`; its workflow explicitly checks out implementation `1a83446b`. Evidence commit `8774bc1` is later, with no `core`, module, or verifier-script diff from the implementation through `HEAD`; sidecar SHA-256 is `b35aa4d620e07e417bc038262a3dbdece745f383e84bd48165d1b2d445cf9b90`, matching the evidence record. |

`verify.artifacts` reported a static pattern miss for `core/store/store.go` because that plan listed `BeginIngest` in that file. Manual L1/L2/L3 inspection shows the public method was factored into the substantive companion `core/store/install_transaction.go` in the same package and is wired to the controller; this is not an implementation or behavior gap.

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `CLI.Run` | Parse → Build → `CommitIngest(main)` | `runIngestCommand` / `runIngestWithSeams` | ✓ WIRED | Dispatch at `core/cli/cli.go:74-75`; source parsing/build at `ingest.go:222-231`; complete Profile commit at `:324-328`. |
| Store redaction | Committed Git profile and user output | `prepareSecrets` + `WithheldReport` | ✓ WIRED | Redaction precedes commit (`store.go:820-826`), result transfers to DTO (`ingest.go:363,471-475`), and renderer emits name/line only. |
| Original snapshot | One promoted installed target | prepared candidate → `promote` → `CommitIngest` → `Finalize` | ✓ WIRED | Target promotion is before commit (`ingest.go:288,324`) and finalization follows committed status (`:365-376`); built-binary AST/runtime test verifies one promotion and no post-commit rewrite. |
| `Store.Read(main)` | Regenerate / activation / runtime emitter | full Profile and `EffectiveManaged` projection | ✓ WIRED | E2E tests pass the read profile through `ir.Regenerate`, `activate.Build`, and the real emitter, asserting order, inert unmanaged entries, and resolved SecretRefs. |
| Pristine target | Actual installed target | built binary + isolated `zsh -f` snapshots | ✓ WIRED | `TestMainIngestActualInstalledStartupEquivalent` and adjacent exact-symbol/no-subprocess tests passed against the actual installed temporary `.zshrc`. |

### Data-Flow Trace (Level 4)

| Artifact | Data | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| Ingest controller | `profile` | Real target bytes → `provider.Parse` → `ir.Build` | Yes — actual profile is committed then read from the Git Store in E2E | ✓ FLOWING |
| Secret path | `WithheldReport` / redacted Profile | `prepareSecrets` and Store backend | Yes — generated runtime literal is redacted before object creation and reflected as name/line output | ✓ FLOWING |
| Startup path | installed `.zshrc` | Original exact snapshot plus canonical loader candidate | Yes — built binary writes target and `zsh -f` sources that path | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Built binary preserves bytes/idempotence/startup behavior/order/symbol boundary/no subprocess | Exact 13-name `go test ./core/cmd/zsh-pro -run ... -count=1` selector from 06-05 | `ok` in 5.388s | ✓ PASS |
| Real provider/Store Profile, regeneration, activation, secret, and no-execution E2E | Exact 12-name `go test ./core/cli -run ... -count=1` selector from 06-05 | `ok` in 2.090s | ✓ PASS |
| Marker and Store-secret boundary regressions | Focused CLI and Store selectors | both `ok` | ✓ PASS |
| Touched packages and repository quality gates | `GOTOOLCHAIN=local go test ./core/model ./core/store ./core/cli ./core/cmd/zsh-pro -count=1`; `go vet ./...`; `make check` | all passed; lint reported `0 issues`; `make check` ran full `go test ./...` successfully | ✓ PASS |
| Native macOS atomic exchange and cleanup | CI run `31274061615`, verifier at exact implementation SHA | completed/success; all exchange, no-replace, capability, top-level/nested/symlink/replacement rows PASS | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — Phase 6 declares no `probe-*.sh` path and the repository has no conventional probe script. The native verifier is not a probe; its regression test and attributable CI execution were both verified above.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| PROF-03 | 06-01 through 06-05 | Literal secrets are excluded from the committed profile by default and the user is told what was withheld. | ✓ SATISFIED | Store-before-object redaction, value-free DTO rendering, exhaustive primary/retained-quarantine object scans, and built-binary no-disclosure behavior all passed. |

All five Phase 6 plans declare `PROF-03`; REQUIREMENTS.md maps no additional requirement solely to Phase 6, so there are no orphaned Phase 6 requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `scripts/verify-phase06-macos-runtime_test.sh` | 79 | `mktemp` test fixture directory | ℹ️ Info | Test-only isolated workspace; not a stub or debt marker. |

No `TBD`, `FIXME`, or `XXX` marker was found in the 40 Phase 6 changed `core/` and `scripts/` files. The only `placeholder` matches are intentional inert secret-reference strings and test fixtures, which are exercised by the secret-boundary tests.

### Human Verification Required

None. This phase has no visual or external interactive flow left unexercised: runtime state transitions are covered by named tests, and the normally unavailable native-Darwin requirement has attributable, exact-SHA CI evidence.

### Informational Notes

- `ROADMAP.md` still renders Phase 6 as 4/5 plans even though `06-05-PLAN.md`, `06-05-SUMMARY.md`, its code/tests, and its native evidence exist. This is planning-metadata drift, not a failed product truth; this verifier did not modify roadmap state.
- Plan key-link entries use semantic labels instead of source paths, so the generic `verify.key-links` helper cannot parse them. Each required connection above was traced manually in code and exercised by its named behavioral test.

### Gaps Summary

No goal-blocking gaps found. The Phase 6 goal and PROF-03 are achieved.

---

_Verified: 2026-08-08T19:30:23Z_
_Verifier: the agent (gsd-verifier)_
