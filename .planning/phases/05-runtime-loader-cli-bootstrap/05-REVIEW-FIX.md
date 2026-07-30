---
phase: 05
fixed_at: 2026-07-30T00:42:19Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 2
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T00:42:19Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 2

**Summary:**

- Findings in scope: 2
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: Target preflight failures deactivate the known-good current profile

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** 3c4db78
**Applied fix:** Splits target validation from evaluation and captures/emits/validates B before clearing A's markers or invoking A's retained reverse. Native zsh regressions keep A's environment, alias, function, option, PATH, both markers, and retained reverse unchanged for a nonzero emitter, empty output, validator rejection, and a real helper staging/capture failure.

### CR-02: Symlink-following staging validation leaves the checked root replaceable

**Status:** fixed
**Files modified:** `core/cli/cli.go`, `core/cli/runtime.go`, `core/cli/runtime_openat_darwin.go`, `core/cli/runtime_openat_linux.go`, `core/cli/runtime_root_other.go`, `core/cli/runtime_root_unix.go`, `core/cli/runtime_test.go`, `core/cli/emitter_test.go`, `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** cad4d49
**Applied fix:** Replaces shell pathname staging with a private-pipe runtime helper. The helper walks every root component with descriptor-relative `openat` and `O_NOFOLLOW`, rejects final and intermediate symlinks plus non-sticky writable ancestors, and validates pipe-fed source without reopening a stage file. Native tests use the compiled helper to race an attacker-owned sticky-`/tmp` symlink, assert the emitter and attacker sink never receive the source, and preserve active A; direct tests retain non-sticky ancestor protection.

## Verification

- Native zsh preflight, non-sticky ancestor, shared-TMPDIR, and real sticky-symlink-race regressions — passed
- Runtime helper unit tests for safe sticky roots, final/intermediate symlinks, non-sticky ancestors, bounded emitters, and pipe validation — passed
- `GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go test -c -o /tmp/zsh-pro-cli-darwin-test ./core/cli` — passed
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed
- `GOTOOLCHAIN=auto go vet ./...` — passed
- `make check` — passed

## Remaining Manual Verification

`hyperfine` remains unavailable in this environment, so the existing interactive startup benchmark is still a manual/CI verification item; it is unrelated to these two fixes.

---

_Fixed: 2026-07-30T00:42:19Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 2_
