---
phase: 06-ingest-end-to-end
plan: 05
subsystem: ingest-e2e-verification
tags: [go, zsh, ingest, secrets, macos, e2e]
requires:
  - phase: 06-ingest-end-to-end
    plan: 04
    provides: complete-profile ingest controller with guarded startup promotion
provides:
  - built-binary proof that actual installed startup preserves source behavior and byte ownership
  - end-to-end complete-profile, activation-projection, secret-boundary, and no-execution coverage
  - exact-SHA native macOS runtime evidence with unedited committed output
  - complete convergence ledger with all P6-001 through P6-075 findings verified
affects: [PROF-03, phase-completion]
tech-stack:
  added: []
  patterns: [exact-name E2E selectors, exhaustive object database scanning, exact-SHA external evidence]
key-files:
  created:
    - core/cli/ingest_e2e_test.go
    - scripts/verify-phase06-macos-runtime.sh
    - scripts/verify-phase06-macos-runtime_test.sh
    - .planning/phases/06-ingest-end-to-end/06-MACOS-RUNTIME-EVIDENCE.md
  modified:
    - core/store/install_transaction.go
    - core/cli/install.go
    - core/cli/ingest.go
    - core/cmd/zsh-pro/main.go
key-decisions:
  - "Persist the complete source-ordered profile; use EffectiveManaged only for activation and reporting."
  - "Treat installed startup behavior as the built binary's actual target, not a separately generated artifact."
  - "Native macOS evidence must run the verifier at an earlier immutable implementation SHA and retain its unedited output."
requirements-completed: [PROF-03]
requirements-contributed: []
status: complete
---

# Phase 06 Plan 05: Final End-to-End Verification Summary

**The real ingest path now has executable proof of source-ordered profile persistence, secret exclusion, activation projection, actual installed startup behavior, and native macOS atomic cleanup.**

## Accomplishments

- Proved with the built `zsh-pro` binary that pristine and actually installed startup retain ordinary source behavior and order, preserve all outside-marker bytes, and launch no startup subprocess.
- Proved `Store.Read(main) → ir.Regenerate` preserves full non-secret profile order/text/semantics, while activation applies only `EffectiveManaged` entries and resolves authoritative `SecretRef`s.
- Exhaustively scanned primary and retained quarantine object databases, guarded source/expected-output privacy, and verified no source execution or forged SecretRef authority.
- Added exact-SHA macOS runtime evidence for implementation `1a83446b5911e65137f1c4eba37d84914dee96d9`: hosted macOS 14.8.7 arm64/APFS, local Go 1.25.0, all exchange/no-replace and cleanup rows PASS. The committed raw-output sidecar is byte-identical to the CI artifact.
- Closed the P6-001 through P6-075 convergence ledger; the final CodeRabbit pass raised no actionable finding.

## Validation Results

- Exact 12-selector end-to-end matrix, with one discovered copy of each selector and zero skips — PASS.
- `GOTOOLCHAIN=local go test ./core/model ./core/store ./core/cli ./core/cmd/zsh-pro -count=1` — PASS.
- Targeted race suite, `GOTOOLCHAIN=local go vet ./...`, and `GOTOOLCHAIN=local make check` — PASS.
- Linux amd64/arm64, Darwin amd64/arm64, and FreeBSD amd64 test-binary builds — PASS.
- Unsupported atomic-adapter tag, macOS verifier regression, plan structure, immutable module/dependency/layer/scope gates — PASS.
- [Native macOS CI run 31274061615](https://github.com/metanmai/zsh-pro/actions/runs/31274061615) — PASS at the exact implementation SHA above.
- Final CodeRabbit review against `557c818845f4239d5d50ecab02e37731bc117f10` — no actionable finding.

## Native Regression Resolution

Native macOS revealed that the Go descriptor-root API cannot open a dangling symlink under its mandatory no-follow policy. The initial fallback could miss a same-inode symlink replacement, so the final implementation records the no-follow target together with Lstat identity and verifies both before and after cleanup seams. Twenty consecutive replacement runs, cross-builds, and the exact-SHA native macOS run passed.

## User Setup Required

None.

## Self-Check: PASSED

- Confirmed implementation SHA precedes the later evidence record and no implementation/verifier file changed between them.
- Confirmed the committed macOS raw-output sidecar hash matches the downloaded CI artifact.
- Confirmed all Phase 6 plan summaries and evidence artifacts exist.
