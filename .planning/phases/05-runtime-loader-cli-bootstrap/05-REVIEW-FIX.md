---
phase: 05
fixed_at: 2026-07-30T02:37:15Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 5
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T02:37:15Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 5

**Summary:**

- Findings in scope: 2
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: A normally initialized default profile store is rejected by sourced runtime operations

**Status:** fixed
**Files modified:** `core/store/store.go`, `core/store/store_root_private_unix.go`, `core/store/store_root_private_other.go`, `core/store/store_test.go`, `core/cmd/zsh-pro/main_test.go`
**Commit:** af6c6b9
**Applied fix:** `Store.Init` now establishes the final store root as private before inspecting or initializing the bare repository. On Linux and Darwin it creates/repairs the root through a no-follow descriptor only after validating that it is a directory owned by the effective user; symlinks and non-directories are rejected without chmodding their targets. Existing current-user repositories at 0755 are safely migrated to 0700. Regression coverage verifies fresh and repaired roots, rejects unsafe existing roots, and exercises actual composition/runtime capture without fixture-only chmods for HOME fallback, XDG data home, and explicit `ZSHPRO_HOME`.

### CR-02: A failed retained reverse terminates the shell and destroys recovery state

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/emit.go`, `core/shell/zsh/residue_test.go`, `core/shell/zsh/reverse_live_test.go`
**Commit:** 9904a81
**Applied fix:** Generated reverses now use preflight, restore, and commit phases: known readonly/non-restorable values are rejected before mutation, and undo slots are removed only after all reverse operations succeed. The loader retains active markers, the retained reverse function, and undo metadata on failure; it consumes them only after a successful reverse. Native-zsh regression coverage invokes public `deactivate` and `activate B` directly under both `ERR_EXIT` and `ERR_RETURN`, tests readonly applied scalars and readonly undo slots, validates a mid-reverse failure preserves recovery state, and proves the retained reverse succeeds after the readonly state is released.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh` — passed.
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `make check` — passed (`gofmt`, `go vet`, `golangci-lint` with 0 issues, and all tests).
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli ./core/store ./core/shell/zsh` — passed.
- `GOOS=freebsd GOARCH=amd64 GOTOOLCHAIN=auto go build ./core/cmd/zsh-pro ./core/cli` — passed.

---

_Fixed: 2026-07-30T02:37:15Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 5_
