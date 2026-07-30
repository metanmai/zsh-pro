---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T02:01:41Z
depth: deep
files_reviewed: 22
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/cli/runtime.go
  - core/cli/runtime_test.go
  - core/cli/runtime_root.go
  - core/cli/runtime_root_unix.go
  - core/cli/runtime_openat_linux.go
  - core/cli/store_root.go
  - core/cli/store_root_test.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/cli.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/secret.go
  - core/cmd/zsh-pro/main.go
  - core/cmd/zsh-pro/main_test.go
  - core/store/store.go
  - core/store/git.go
findings:
  critical: 2
  warning: 0
  info: 0
  total: 2
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-07-30T02:01:41Z
**Depth:** deep
**Files Reviewed:** 22
**Status:** issues_found

## Summary

The repaired descriptor-bound transport, secret scrubbing, StoreRoot precedence, installer transaction, and retained-reverse flow were reviewed adversarially against the full Phase 5 history and exercised with native zsh and built-CLI probes. Two release-blocking correctness defects remain. The default profile store is created with permissions that the new runtime capture always rejects, and a failed retained reverse can terminate the shell while irreversibly discarding its recovery state.

`make check`, focused uncached CLI/zsh/store tests, and `go vet ./...` pass. Those gates do not cover either reproduced failure. The absent `hyperfine` executable remains only the documented manual startup benchmark and is not a source finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] A normally initialized default profile store is rejected by every sourced runtime operation

**File:** `core/store/store.go:125-140`; `core/cli/runtime_root_unix.go:77-81`; `core/cmd/zsh-pro/main_test.go:117-120`

**Issue:** `Store.Init` creates the bare repository with `os.MkdirAll(s.dir, 0o755)`, but `secureRuntimeRoot` accepts the final store directory only when all group/other permission bits are zero. Consequently, a store created through the product's own initializer is mode 0755 under the normal umask, and `activate`, `checkout`, and `list` all fail at `runtime capture` before Git or the descriptor-bound emitter can run. The new composition integration test conceals the mismatch by calling `os.Chmod(root, 0o700)` immediately after `Init`, so it proves only an out-of-band repaired fixture.

**Reproduction:**

```text
mkdir -p "$HOME/.local/share/zsh-pro"
chmod 0755 "$HOME/.local/share/zsh-pro"
git init --bare -b main "$HOME/.local/share/zsh-pro"
env -u ZSHPRO_HOME -u XDG_DATA_HOME \
  zsh-pro runtime capture 2 -- zsh-pro list

MODE=755
RC=1
zsh-pro: runtime store root is unsafe
```

The same incompatibility applies to the `XDG_DATA_HOME/zsh-pro` and explicit `ZSHPRO_HOME` routes whenever the store was initialized by `Store.Init` and not subsequently chmodded by the installer or a test.

**Fix:**

Create and maintain the profile-store root at 0700 in `Store.Init`, including migration/repair of an existing user-owned store before runtime use. Validate ownership and directory type before chmodding. Add end-to-end tests for HOME fallback, XDG_DATA_HOME, and explicit ZSHPRO_HOME that call `Store.Init` and then invoke the real `runtime capture` without any test-only chmod.

### CR-02: [BLOCKER] A reverse failure terminates the shell and destroys the only recovery capability

**File:** `core/shell/zsh/hook.go:64-79`; `core/shell/zsh/hook.go:246-305`; `core/shell/zsh/hook.go:396-405`

**Issue:** `_zp_reverse_active_profile` clears `ZP_ACTIVE_PROFILE` and `ZSHPRO_PROFILE` before reversal, `_zp_run_retained_reverse` clears `ZP_ACTIVE_REVERSE_FN`, and `_zp_run_transient_reverse_payload` unsets the reverse function regardless of success. The scalar restore helper also performs fallible assignments and then unconditionally deletes its undo slots. If live state becomes non-restorable—for example, the user marks a profile-owned scalar readonly after activation—`deactivate` terminates native zsh, leaves the profile value applied, and has already discarded every marker/function needed to retry or diagnose the active ownership. This violates both the fail-open public-verb contract and partial-evaluation recovery.

**Reproduction:**

```text
source <(zsh-pro hook)
# A runtime payload applies FOO=applied and retains its generated reverse.
activate A
typeset -gr FOO
deactivate
print SURVIVED
```

Observed with a production-shaped emitted payload under `zsh -f`:

```text
ACTIVE_RC=0 FOO=applied MARKER=A STATUS=0
_zp_restore_scalar:9: read-only variable: FOO
```

The process exits 1 before `SURVIVED`, even without enabling `ERR_EXIT`; `FOO` remains applied. With `ERR_RETURN`, the same direct public call terminates before cleanup/diagnostic continuation.

**Fix:**

Make retained reversal commit-like: preflight known non-restorable state (including readonly scalar targets), retain active markers, undo slots, and the reverse function until the entire reverse succeeds, and consume them only after success. Every generated reverse operation/helper must explicitly propagate failure and must not erase its undo metadata after an unsuccessful restore. The public `deactivate`/switch boundary must turn a failed preflight or reverse into `ZP_LAST_RUNTIME_STATUS` while returning safely to hostile-option callers. Add native-zsh regressions for readonly scalar failure, reverse-command failure partway through a multi-operation profile, retry/recovery state retention, and both `ERR_EXIT` and `ERR_RETURN`.

---

_Reviewed: 2026-07-30T02:01:41Z_
_Reviewer: GPT-5.6 Sol (gsd-code-reviewer)_
_Depth: deep_
