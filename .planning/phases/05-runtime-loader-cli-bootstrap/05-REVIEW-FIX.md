---
phase: 05
fixed_at: 2026-07-30T01:54:13Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 4
findings_in_scope: 1
fixed: 1
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T01:54:13Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 4

**Summary:**

- Findings in scope: 1
- Fixed: 1
- Skipped: 0

## Fixed Issues

### CR-01: Descriptor-bound runtime capture authenticates the loader cache instead of the default profile store

**Status:** fixed: requires human verification
**Files modified:** `core/cli/runtime.go`, `core/cli/store_root.go`, `core/cli/store_root_test.go`, `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-pro/main_test.go`
**Commit:** f67c44d
**Applied fix:** Added the shared `cli.StoreRoot()` resolver and used it for both ordinary composition-root construction and descriptor-bound runtime capture. It selects explicit absolute `ZSHPRO_HOME`, then absolute `XDG_DATA_HOME/zsh-pro`, then `$HOME/.local/share/zsh-pro`; explicitly empty or relative selected inputs fail closed. The installer's `$HOME/.zsh-pro` loader-cache resolution remains separate.

The end-to-end regression constructs real bare stores and file vaults through the composition root for all three supported locations: HOME fallback, absolute XDG data home, and explicit `ZSHPRO_HOME`. For each it proves ordinary `list` equals descriptor-bound runtime `list`, and ordinary/runtime `emit apply` both contain the same profile identity and vault-resolved secret. When zsh is available, it also sources every captured runtime payload in native zsh and verifies the resulting environment.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 -run 'TestStoreRootUsesDocumentedPrecedenceAndRejectsUnsafeInputs|TestRuntimeCaptureUsesCompositionStoreAndVaultForEveryLocation' -v ./core/cli ./core/cmd/zsh-pro` — passed (all invalid-input and three real-store cases)
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed
- `GOTOOLCHAIN=auto go vet ./...` — passed
- `make check` — passed (`golangci-lint`: 0 issues)
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli ./core/store ./core/shell/zsh` — passed
- `GOOS=freebsd GOARCH=amd64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli` — passed (descriptor capture remains fail-closed on unsupported platforms)
- Built-binary HOME-default smoke test — passed: ordinary `list` and `runtime capture ... list` both returned `main`
- `GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run '^TestLiveTerminal'` — passed, including retained-secret transport, fail-open hostile-option behavior, bounded runtime list, and descriptor-race coverage

## Remaining Manual Verification

`hyperfine` remains an unrun manual timing item; it is not part of this source repair.

---

_Fixed: 2026-07-30T01:54:13Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 4_
