---
phase: 05
fixed_at: 2026-07-30T04:43:27Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 9
findings_in_scope: 1
fixed: 1
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T04:43:27Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 9

**Summary:**

- Findings in scope: 1
- Fixed: 1
- Skipped: 0

## Fixed Issues

### CR-01: [BLOCKER] Cached-loader installation follows symlinks and mutates unrelated targets

**Status:** fixed
**Files modified:** `core/cli/cache_directory.go`, `core/cli/cache_directory_unix.go`, `core/cli/cache_directory_other.go`, `core/cli/cache_syscalls_linux.go`, `core/cli/cache_syscalls_darwin.go`, `core/cli/install.go`, `core/cli/install_test.go`, `core/cmd/zsh-pro/main_test.go`
**Commit:** `87b92af`
**Applied fix:** Replaced the cache's path-based `Stat`/`Chmod`/symlink-resolution flow with descriptor-relative, `O_NOFOLLOW` traversal and writes on Linux and Darwin. Every cache-root component must be a real directory; an existing loader must be a regular non-symlink before the cache becomes writable. Candidate/rollback files are created, validated, promoted, restored, and removed relative to retained descriptors. The installer now validates the exact staged descriptor via `/dev/fd/3`; the user-owned `.zshrc` resolver remains deliberately symlink-compatible. Unsupported platforms reject cache installation before mutation. Rollback also checks inode/device identity before removing a newly created cache directory.

## Verification

- Direct cache regressions: `GOTOOLCHAIN=auto go test -count=1 -run 'TestInstallRejectsSymlinkedCachePathsWithoutMutation|TestCacheRollbackRefusesToRemoveAReplacedDirectory' -v ./core/cli` — passed root/loader symlinks for default and explicit caches, a symlinked ancestor, a non-directory ancestor, and substituted-directory rollback protection.
- Built-binary regressions: `GOTOOLCHAIN=auto go test -count=1 -run TestBuiltBinaryInstallRejectsSymlinkedCacheTargetsWithoutMutation -v ./core/cmd/zsh-pro` — passed HOME, XDG, and explicit-root reproductions. Each failure retained the symlink, target bytes/modes, `.zshrc`, and the relevant store/cache tree.
- Native zsh smoke: built binary `install`, then `zsh -f` source under `NO_UNSET ERR_EXIT ERR_RETURN` — passed and exposed `checkout`, `activate`, and `deactivate` from the cached loader.
- Full native suite: `GOTOOLCHAIN=auto go test -count=1 ./...` — passed, including native zsh tests.
- Static and project gates: `GOTOOLCHAIN=auto go vet ./...` and `make check` — passed; `golangci-lint` reported 0 issues.
- Cross-build checks: Darwin arm64 build plus `core/cli` test compilation, Linux arm64 build, and Windows amd64 build/test compilation — passed. Windows uses the fail-closed cache implementation.

---

_Fixed: 2026-07-30T04:43:27Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 9_
