---
phase: 06-ingest-end-to-end
verified: 2026-08-08T20:54:52Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
---

# Phase 6: Ingest End-to-End Verification Report

**Phase Goal:** Polish the on-ramp last, against the final IR shape. Compose the already-built pieces into the full path: parse a real `~/.zshrc` → classify (declarative/imperative split) → partial-eval → commit the complete redacted, source-ordered profile to the baseline branch — while preserving every preexisting startup byte outside the canonical managed loader region and warning about post-END appends.

**Verified:** 2026-08-08T20:54:52Z  
**Status:** passed  
**Re-verification:** No — the prior report contained no `gaps:` section, so this is an independent initial-mode check.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Ingest commits the complete redacted, source-ordered Profile to `main`; `EffectiveManaged` is only a projection, with no dropped or reordered source statement. | ✓ VERIFIED | `CLI.Run` dispatches `ingest` to `runIngestCommand` ([`core/cli/cli.go:74`](../../core/cli/cli.go)); the controller prepares the original source, calls `provider.Parse` then `ir.Build`, and passes that complete Profile to `CommitIngest` ([`core/cli/ingest.go:222`](../../core/cli/ingest.go), [`:227`](../../core/cli/ingest.go), [`:231`](../../core/cli/ingest.go), [`:324`](../../core/cli/ingest.go)). The independently authored-provenance real-provider/real-Store E2E matrix passed, including `TestIngestE2EStoreReadReturnsCompleteOrderedProfile` and `TestIngestE2ERegeneratePreservesNonSecretOrderTextAndSemantics`. |
| 2 | Literal secrets are excluded from committed baseline objects by default and the user is told exactly what was withheld. | ✓ VERIFIED | `CommitIngest` invokes `prepareSecrets` before candidate creation ([`core/store/store.go:793`](../../core/store/store.go), [`:816`](../../core/store/store.go)); redaction replaces literal `Text`, `Value`, and `RuntimeValue` with inert `SecretRef` metadata and only retains name/line report fields ([`core/store/secret.go:100`](../../core/store/secret.go), [`:185`](../../core/store/secret.go), [`:199`](../../core/store/secret.go)). The E2E object scan and the built-binary no-disclosure test both passed. |
| 3 | Adoption changes only exact loader-marker regions, is idempotent, preserves all outside bytes in order, and warns rather than clobbers post-END content. | ✓ VERIFIED | `prepareIngestInstallAt` derives both eligible source and the canonical candidate from the same original snapshot ([`core/cli/install_transaction.go:213`](../../core/cli/install_transaction.go)); the controller promotes loader then target once before Store commit and finalizes that transaction without another target write ([`core/cli/ingest.go:283`](../../core/cli/ingest.go), [`:288`](../../core/cli/ingest.go), [`:324`](../../core/cli/ingest.go), [`:365`](../../core/cli/ingest.go)). Named built-binary tests passed for outside bytes, idempotency, exact one-promotion ordering, and post-END persistence/warning. |
| 4 | Store-read regeneration, effective-managed activation/SecretRef resolution, and actual installed `zsh -f` startup equivalence all hold; startup adds no subprocess. | ✓ VERIFIED | `ir.Regenerate` keeps stored entry order and emits unmanaged text verbatim ([`core/ir/regen.go:22`](../../core/ir/regen.go)); `activate.Build` filters only ineffective/unrepresentable entries ([`core/activate/builder.go:16`](../../core/activate/builder.go)). The exact E2E matrix passed activation/SecretRef/inert-canary tests, and the built-binary matrix passed actual-installed startup, order-sensitive definition-before-use, exact loader-symbol, and no-subprocess tests. |
| 5 | **PROF-03:** end-to-end ingest reuses secret detection so committed profiles exclude literals and report withheld entries. | ✓ VERIFIED | This is exercised end-to-end rather than inferred from types: Store-owned pre-object redaction, value-free DTO/rendering ([`core/dto/ingest.go:3`](../../core/dto/ingest.go)), exhaustive final/retained-quarantine object scans, and the built-binary secret-behavior test all passed. |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/cli/ingest.go` | Source → complete Profile → one promotion → Store transaction controller | ✓ VERIFIED | Exists and is substantive (597 lines); wired from `CLI.Run`; its target/source/Profile/Store data flow is exercised by the exact CLI E2E matrix. |
| `core/store/store.go`, `core/store/install_transaction.go`, `core/store/secret.go`, `core/store/git.go` | Main-bound commit, redaction before objects, and authenticated transaction state | ✓ VERIFIED | `CommitIngest` is in `store.go`; `BeginIngest`/`AbortIngest` are intentionally factored into the substantive companion `install_transaction.go` ([`core/store/install_transaction.go:137`](../../core/store/install_transaction.go), [`:398`](../../core/store/install_transaction.go)) and are reached through the injected transaction interface. |
| `core/cli/install.go`, `core/cli/install_transaction.go`, `core/cli/atomic_rename*.go` | Exact-marker candidate and one guarded filesystem promotion | ✓ VERIFIED | Present, substantive, and reached by the controller. Named built-binary and transaction tests demonstrate marker fidelity, idempotence, and target-promotion cardinality. |
| `core/cmd/zsh-pro/main.go` | One concrete Store/provider composition root | ✓ VERIFIED | `cliStore` is constructed once and captured by the initializer ([`core/cmd/zsh-pro/main.go:67`](../../core/cmd/zsh-pro/main.go), [`:74`](../../core/cmd/zsh-pro/main.go), [`:92`](../../core/cmd/zsh-pro/main.go)); the real composition test is included in the passing full suite. |
| `core/cli/ingest_e2e_test.go`, `core/cmd/zsh-pro/main_test.go` | Real Store/provider and built-binary behavioral evidence | ✓ VERIFIED | Both are substantive, runnable tests. The exact 12-selector and 13-selector matrices were independently enumerated and run uncached. |
| `scripts/verify-phase06-macos-runtime.sh` plus macOS evidence record | Native Darwin atomic/cleanup proof at implementation SHA | ✓ VERIFIED | The local shell regression passed; the evidence binds a PASS native macOS 14.8.7 arm64 run at `2443a0e`, records all atomic/cleanup rows, and its sidecar SHA-256 matches (`6cff…d9e9b`). `2443a0e` is an ancestor of the later evidence commit `405f8d0`; no implementation or verifier-script file changed between them. |

`verify.artifacts` found 35/36 static artifacts. Its sole miss was the Plan 06-01 text pattern `BeginIngest` in `core/store/store.go`; the public method is deliberately in the companion file above, in the same Store package, is implemented and wired. This is a plan-path pattern mismatch, not a missing implementation.

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `CLI.Run` | Parse → Build → `CommitIngest(main)` | `runIngestCommand` / `runIngestWithSeams` | ✓ WIRED | Dispatch, static parsing, full Profile build, and commit call are directly present and exercised by real provider/Store E2E tests. |
| Store redaction | Git objects and user output | `prepareSecrets` + value-free `WithheldReport`/DTO | ✓ WIRED | Redaction precedes candidate preparation; all-object scans prove literals absent, while output tests prove only name/line metadata can render. |
| Original snapshot | One installed target transition | canonical candidate → loader promotion → target promotion → commit → finalize | ✓ WIRED | The controller order is explicit and `TestMainIngestPromotesTargetExactlyOnce` verifies one target `promote`, then commit, then finalize. |
| `Store.Read(main)` | Regenerate / activation / runtime emission | full Profile then `EffectiveManaged` projection | ✓ WIRED | Regeneration, activation, resolver, and inert-canary named tests passed against the Store-read Profile. |
| Pristine target | Actual installed target | built binary + isolated `zsh -f` snapshots | ✓ WIRED | The test sources the actual modified target, compares its structured runtime snapshot to pristine, checks order-sensitive behavior, exact outside bytes, and subprocess counter. |

The generic key-link helper reports zero parseable links because all Phase 6 plan links use semantic component names rather than source-file paths. The connections above were therefore traced manually in source and exercised behaviorally.

### Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Produces real data | Status |
| --- | --- | --- | --- | --- |
| Ingest controller | `profile` | original target bytes → eligible source → `provider.Parse` → `ir.Build` | Real zsh provider and Git Store in passing E2E test | ✓ FLOWING |
| Secret path | redacted Profile and `WithheldReport` | Store `prepareSecrets` before object creation | Runtime-generated literal is absent from scanned objects/output, but name/line render | ✓ FLOWING |
| Startup path | installed `.zshrc` | exact original snapshot plus independently authored expected candidate | Actual built binary mutates and then `zsh -f` sources target | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Real provider/Store full-profile, regeneration, activation, privacy, and no-execution behaviors | Exact 12-name `GOTOOLCHAIN=local go test ./core/cli -run '^(…)$' -count=1` selector from 06-05 | `ok zsh-pro/core/cli 1.310s` | ✓ PASS |
| Built-binary ingest, bytes, idempotency, actual startup, order, secret, and subprocess behaviors | Exact 13-name `GOTOOLCHAIN=local go test ./core/cmd/zsh-pro -run '^(…)$' -count=1` selector from 06-05 | `ok zsh-pro/core/cmd/zsh-pro 5.813s` | ✓ PASS |
| Workspace quality gate | `GOTOOLCHAIN=local make check` | `go vet ./...`, `golangci-lint run` (0 issues), and `go test ./...` passed | ✓ PASS |
| Native-evidence regression and binding | `scripts/verify-phase06-macos-runtime_test.sh`; SHA/ancestry/source-diff checks | Script PASS; sidecar hash matched; source unchanged from `2443a0e` through `405f8d0` | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no conventional `scripts/**/tests/probe-*.sh` exists and no Phase 6 PLAN/SUMMARY declares a probe. The native verifier is a separately tested runtime-evidence script, not a probe.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| PROF-03 | 06-01 through 06-05 | Detected secrets are excluded from the committed profile by default and the user is told what was withheld. | ✓ SATISFIED | Store-before-object redaction, name/line-only DTO, all reachable/unreachable/packed/retained-quarantine scans, and built-binary no-disclosure behavior passed. |

Every Phase 6 plan declares `PROF-03`; `REQUIREMENTS.md` maps no additional requirement solely to Phase 6, so there are no orphaned Phase 6 requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `core/store/secret.go` | 180 | `placeholder` | ℹ️ Info | Intentional inert SecretRef placeholder; the redaction/object-scan tests exercise it. |
| `core/cli/ingest_e2e_test.go` | 153 | `placeholder` | ℹ️ Info | Independently authored runtime-secret test placeholder; not persisted as the generated literal. |

No `TBD`, `FIXME`, or `XXX` marker exists in the 51 Phase 6 changed `core/` and `scripts/` implementation files. Empty-return scan hits are normal Go success/error returns, not user-visible stubs; full and named behavioral tests exercise the affected paths.

### Human Verification Required

None. All behavior-dependent roadmap truths have passing named behavioral tests. The otherwise external Darwin requirement has attributable exact-SHA CI evidence with a matching unedited sidecar and an implementation/evidence commit-boundary check.

### Gaps Summary

No goal-blocking gaps found. The Phase 6 goal and PROF-03 are achieved at source SHA `2443a0efbff263c27e0dc2db1bd075af18879d3a`; `HEAD` `405f8d0ee6e707f785d7efed811654a0b2e25657` changes only the macOS evidence files.

---

_Verified: 2026-08-08T20:54:52Z_  
_Verifier: the agent (gsd-verifier)_
