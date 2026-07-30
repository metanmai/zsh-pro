---
phase: 05
fixed_at: 2026-07-30T03:45:46Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 7
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T03:45:46Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 7

**Summary:**

- Findings in scope: 2
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: [BLOCKER] A failed install leaves a newly initialized profile store behind

**Status:** fixed
**Files modified:** `core/cli/install.go`, `core/cli/install_test.go`, `core/cli/store.go`, `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-pro/main_test.go`, `core/store/install_transaction.go`, `core/store/store_root_private_other.go`, `core/store/store_root_private_unix.go`, `core/store/store_test.go`
**Commit:** `5085ac6`
**Applied fix:** `Store.InitForInstall` now returns an idempotent install-transaction rollback. Fresh roots are snapshotted and removed only when their complete current tree still matches this invocation; newly created empty parents are then removed. Existing initialized stores are never deleted or rewound, while an install-time privacy migration restores its original descriptor-verified mode. The installer invokes this compensation after every later bootstrap failure, including runtime-directory setup, loader validation/staging/promotion, and `.zshrc` promotion; explicit `ZSHPRO_HOME` correctly orders shared-root rollback.

### CR-02: [BLOCKER] FreeBSD installs a store that every sourced runtime command rejects

**Status:** fixed
**Files modified:** `core/cmd/zsh-pro/main_test.go`, `core/store/store_root_private_other.go`, `core/store/store_root_private_other_test.go`, `core/store/store_root_private_unix.go`
**Commit:** `5f06145`
**Applied fix:** Private-store initialization now supports exactly Linux and Darwin, matching descriptor-bound runtime support. FreeBSD and every other unsupported platform use the fail-closed implementation before a store or bootstrap artifact can be created. The built-binary test proves accepted platforms can install and use both ordinary and descriptor-bound runtime list/emission; the fallback test verifies `InitForInstall` leaves an unsupported-platform root absent. Linux, Darwin, FreeBSD, and Windows package/test compilation confirms the intended build selection.

## Verification

- Focused built-binary rollback coverage passed for loader validation, runtime-directory setup, staging, first loader promotion, and second `.zshrc` promotion across HOME fallback, `XDG_DATA_HOME`, and explicit `ZSHPRO_HOME`.
- `GOTOOLCHAIN=auto go test -count=1 ./core/store ./core/cli ./core/cmd/zsh-pro` — passed.
- Native-zsh recovery, `ERR_EXIT`/`ERR_RETURN`, secret-scrubbing, runtime-transport, and runtime-root-confinement regressions — passed.
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` and `GOTOOLCHAIN=auto go build ./...` — passed.
- `make check` — passed (`go vet`, `golangci-lint` with 0 issues, and all tests).
- Linux/amd64, Darwin/arm64, FreeBSD/amd64, and Windows/amd64 builds plus `core/store` and `core/cmd/zsh-pro` test-binary compilation — passed.
- A separate isolated built-binary `zsh -f` smoke test installed, sourced `.zshrc`, listed `main`, and deactivated successfully.
- `hyperfine` is unavailable in this environment; performance measurement remains the documented manual benchmark, not a release gate.

## Limitations

- FreeBSD behavior was verified by build selection and cross-compiled tests, not by executing a FreeBSD runtime locally. It is intentionally unsupported until a descriptor-safe `openat` runtime traversal is implemented and tested.

---

_Fixed: 2026-07-30T03:45:46Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 7_
