---
phase: 05
fixed_at: 2026-07-30T03:09:02Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 6
findings_in_scope: 3
fixed: 3
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T03:09:02Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 6

**Summary:**

- Findings in scope: 3
- Fixed: 3
- Skipped: 0

## Fixed Issues

### CR-01: The product never initializes or migrates the profile store

**Status:** fixed
**Files modified:** `core/cli/cli.go`, `core/cli/install.go`, `core/cli/install_test.go`, `core/cli/store.go`, `core/cmd/zsh-pro/main.go`, `core/cmd/zsh-pro/main_test.go`
**Commit:** 48397d6
**Applied fix:** Added an install-only store initializer at the composition root. `install` validates the user bootstrap first, then initializes or safely migrates the profile repository before either loader or `.zshrc` artifact is promoted; read-only verbs still never create storage. A real built-binary integration test covers fresh HOME fallback, XDG data home, explicit `ZSHPRO_HOME`, 0755 migration, ordinary list/emission, and descriptor-bound runtime list/emission.

### CR-02: Failed target cleanup loses its pointer and leaves resolved secret state orphaned

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/reverse_live_test.go`
**Commit:** 47bd7cd
**Applied fix:** A target reverse is registered in a dedicated recovery marker before apply runs. If apply or marker promotion fails, the loader attempts compensation but retains the marker and secret-bearing reverse function on failure. Subsequent `activate` or `checkout` calls are blocked before target emission; public `deactivate` retries recovery and removes both marker and function only after a successful reverse. Native-zsh coverage exercises normal, `ERR_EXIT`, and `ERR_RETURN` modes, checks secret-free diagnostics, proves a later activation is not emitted, and verifies retry scrubs the function and exported secret.

### CR-03: The portable Store initializer can chmod a substituted or foreign target

**Status:** fixed
**Files modified:** `core/store/store_root_private_unix.go`, `core/store/store_root_private_other.go`, `core/store/store_root_private_other_test.go`
**Commit:** 2402764
**Applied fix:** FreeBSD now uses the existing no-follow descriptor, `Fstat` ownership check, and descriptor `Fchmod` path. Platforms without those primitives fail closed without creating, inspecting, or chmodding a pathname target. The fallback regression test verifies a substituted symlink target remains mode 0755; its cross-target test binary compiles on Windows.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `make check` — passed (`gofmt`, `go vet`, `golangci-lint` with 0 issues, and all tests).
- `GOTOOLCHAIN=auto go test -count=1 ./core/cmd/zsh-pro -run '^TestBuiltBinaryInstallInitializesAndMigratesProfileStores$'` — passed.
- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run '^(TestLiveTerminalFailedTargetRecoveryBlocksSwitchAndRetries|TestLiveTerminalRuntimeTransportScrubsResolvedSource|TestLiveTerminalRuntimeHelperRejectsNonStickyWritableAncestor|TestLiveTerminalRuntimeHelperRejectsStickySymlinkReplacementRace|TestLiveTerminalPublicVerbsFailOpenUnderErrExitAndErrReturn)$'` — passed.
- `GOTOOLCHAIN=auto go test -count=1 ./core/store -run '^(TestInitIdempotent|TestInitRefusesUnsafeExistingStoreRoot)$'` — passed.
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli ./core/store ./core/shell/zsh` — passed.
- `GOOS=freebsd GOARCH=amd64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli ./core/store ./core/shell/zsh` — passed.
- `GOOS=windows GOARCH=amd64 GOTOOLCHAIN=auto go test -c ./core/store` — passed (cross-compiled fallback regression test).
- `hyperfine` is unavailable in this environment; the repository treats its performance harness as manual evidence rather than a release gate.

---

_Fixed: 2026-07-30T03:09:02Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 6_
