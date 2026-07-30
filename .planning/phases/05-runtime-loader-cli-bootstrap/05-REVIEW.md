---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T01:40:06Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 37
files_reviewed_list:
  - core/activate/diff.go
  - core/activate/plan.go
  - core/analyze/analyze_test.go
  - core/cli/cli.go
  - core/cli/cli_test.go
  - core/cli/dependencies.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/runtime.go
  - core/cli/runtime_openat_darwin.go
  - core/cli/runtime_openat_linux.go
  - core/cli/runtime_root.go
  - core/cli/runtime_root_other.go
  - core/cli/runtime_root_unix.go
  - core/cli/runtime_test.go
  - core/cli/secret.go
  - core/cli/store.go
  - core/cmd/zsh-pro/main.go
  - core/perf/hyperfine.go
  - core/perf/hyperfine_test.go
  - core/shell/provider.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/invariant_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/store/git.go
  - core/store/keychain.go
  - core/store/runtime_openat_darwin.go
  - core/store/runtime_openat_linux.go
  - core/store/runtime_vault_other.go
  - core/store/runtime_vault_unix.go
  - core/store/store.go
  - scripts/perf-hyperfine.sh
findings:
  critical: 1
  warning: 0
  info: 0
  total: 1
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-30T01:40:06Z
**Depth:** deep
**Files Reviewed:** 37
**Status:** issues_found

## Summary

The two latest security fixes materially close their reported defects. Runtime Git reads remain anchored to the authenticated repository descriptor through the real `git` subprocess, fallback-vault reads use the retained parent descriptor with `O_NOFOLLOW` and owner/mode checks, descriptors are closed on all helper returns, and unsupported platforms fail closed. The loader now transports emitted source through dynamically scoped locals and scrubs `REPLY` plus intermediate payload locals on success and failure; native-zsh coverage exercises successful activation, emitter failure, validation failure, partial evaluation, switch, and deactivate. Existing active-A preflight preservation, truthful partial-eval state, reverse/function cleanup, random helper collision avoidance, installer rollback, and hostile-option behavior remain green.

One shipping blocker remains: the descriptor-bound helper derives the authenticated repository from the loader-cache default (`$HOME/.zsh-pro`) instead of the store default used by the composition root (`$XDG_DATA_HOME/zsh-pro` or `$HOME/.local/share/zsh-pro`). Consequently normal activation fails whenever `ZSHPRO_HOME` is not explicitly set, including the documented default configuration.

Verification performed:

- focused runtime, loader, activation, store, and installer packages — PASS
- uncached `go test -count=1 ./...` — PASS
- `go vet ./...` — PASS
- `make check` — PASS (`golangci-lint`: 0 issues)
- Darwin arm64 cross-compilation of CLI, store, and zsh packages — PASS
- unsupported-platform (FreeBSD amd64) CLI cross-compilation — PASS
- built-binary default-path probe — reproduced ordinary `list` rc 0 versus descriptor-bound runtime `list` rc 1
- `hyperfine` remains unavailable; that benchmark is a manual performance evidence item, not a source defect

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Descriptor-bound runtime capture authenticates the loader cache instead of the default profile store

**Classification:** BLOCKER

**Files:** `core/cli/runtime.go:172-183`, `core/cmd/zsh-pro/main.go:27-29`, `core/cmd/zsh-pro/main.go:54-64`

**Issue:** Commit `22763bd` correctly makes the authenticated descriptors authoritative, but it binds them to the wrong path in the default configuration. `runtimeRoot()` returns `$ZSHPRO_HOME` when set and otherwise always returns `$HOME/.zsh-pro`. The composition root's `storeDir()` instead returns `$ZSHPRO_HOME`, then `$XDG_DATA_HOME/zsh-pro`, then `$HOME/.local/share/zsh-pro`. `$HOME/.zsh-pro` is the installer's cached-loader directory, not the default bare Git repository. `store.NewRuntime` therefore runs Git against the cache directory, so `activate`, `checkout`, and the loader's `list` fail even though ordinary store-backed CLI access succeeds.

This is masked by `TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors`, which always sets `ZSHPRO_HOME` to the test repository. It never exercises either documented default-store branch.

**Reproduction:**

```sh
review_home="$(mktemp -d)/home"
mkdir -p "$review_home/.local/share/zsh-pro" "$review_home/.zsh-pro"
git init --bare -b main "$review_home/.local/share/zsh-pro"
go build -o /tmp/zsh-pro-review ./core/cmd/zsh-pro

env -u ZSHPRO_HOME -u XDG_DATA_HOME HOME="$review_home" \
  /tmp/zsh-pro-review list
# rc=0

env -u ZSHPRO_HOME -u XDG_DATA_HOME HOME="$review_home" \
  PATH="/tmp:$PATH" /tmp/zsh-pro-review runtime capture 5 -- zsh-pro list
# rc=1: zsh-pro: runtime staging root is unsafe
```

The second command fails because it tries to authenticate the non-repository cache at `$HOME/.zsh-pro`; if that directory exists and is private, the later Git operation fails instead.

**Fix:** Define the store-location resolution once and use it in both composition-root construction and descriptor-bound runtime capture: `ZSHPRO_HOME` when explicitly set, otherwise absolute `XDG_DATA_HOME/zsh-pro`, otherwise `$HOME/.local/share/zsh-pro`. Keep the install/cache resolver separate because its default intentionally remains `$HOME/.zsh-pro`. Fail closed for empty or relative `HOME`, `ZSHPRO_HOME`, and `XDG_DATA_HOME`. Add real-store runtime capture tests for (1) unset `ZSHPRO_HOME`/unset `XDG_DATA_HOME`, (2) absolute `XDG_DATA_HOME`, and (3) explicit `ZSHPRO_HOME`, and assert both `list` and secret-bearing `emit apply` consume the same repository and vault selected by ordinary CLI construction.

---

_Reviewed: 2026-07-30T01:40:06Z_
_Reviewer: the agent (gsd-code-reviewer; gpt-5.6-sol)_
_Depth: deep_
