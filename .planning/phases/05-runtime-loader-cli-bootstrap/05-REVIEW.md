---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-27T14:52:18Z
depth: deep
files_reviewed: 15
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/provider.go
  - core/cli/cli.go
  - core/cli/store.go
  - core/cli/emitter.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/cli_test.go
  - core/cmd/zsh-pro/main.go
  - scripts/perf-hyperfine.sh
  - core/shell/zsh/emit.go
  - core/activate/diff.go
  - core/store/store.go
findings:
  critical: 5
  warning: 2
  info: 0
  total: 7
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-07-27T14:52:18Z
**Depth:** deep
**Files Reviewed:** 15
**Status:** issues_found

## Summary

The loader, installer, CLI seams, and their Phase 4 emitter/store callers were traced end-to-end. The checked Go suite passes, but it does not exercise real emitted profile-to-profile activation or hostile interactive zsh options. Five shipping blockers remain: a managed-marker data-loss path, an `ERR_EXIT` shell-exit path, unbounded explicit subprocesses, a stale-state `activate A -> activate B` transition, and typed-nil interface panics.

## Narrative Findings (AI reviewer)

## Critical Issues (BLOCKER)

### CR-01: Marker text inside ordinary user content is treated as a managed region and deleted

**File:** `core/cli/install.go:97`

**Issue:** `replaceManagedBlock` searches for `installBegin`/`installEnd` as arbitrary byte substrings. A user line such as `print '# >>> zsh-pro >>>'` followed later by `print '# <<< zsh-pro <<<'` is accepted as a complete managed region; the installer replaces it and silently removes every byte between the strings. This violates BOOT-01's promise to preserve all unmanaged content and is a data-loss path.

**Fix:** Scan line-by-line and recognize a marker only when the complete line is exactly the marker (allowing at most documented surrounding whitespace). Add regression cases for both markers inside quoted strings and comments with suffix text; both must remain byte-identical after install.

### CR-02: A syntax or runtime failure exits terminals using `setopt ERR_EXIT` before the fail-open handler runs

**File:** `core/shell/zsh/hook.go:65`

**Issue:** `command zsh -n "$tmp"` and `eval "$block"` are failing simple commands, followed only afterwards by `rc=$?`. Under the valid user option `setopt ERR_EXIT`, zsh exits at either failure and never reaches the recovery/report branch. Reproduction: `zsh -f -c 'setopt ERR_EXIT; f(){ command false; rc=$?; print recovery; }; f; print survived'` exits 1 without printing either marker. Thus a malformed emitted block can lock the user out of the current interactive shell, contrary to BOOT-02.

**Fix:** Execute every expected-to-fail command in a conditional context and capture the status in its `else` branch, including `zsh-pro emit`, `zsh -n`, and `eval`; add a live-loader test with `setopt ERR_EXIT` and invalid emitted source that asserts the shell continues after the failed verb.

```zsh
if command zsh -n -- "$tmp"; then
  :
else
  rc=$?
  command rm -f -- "$tmp"
  print -u2 -- 'zsh-pro: emitted shell source failed validation; shell state unchanged'
  return "$rc"
fi
```

### CR-03: A slow or hung emitter/validator can block the terminal indefinitely

**File:** `core/shell/zsh/hook.go:45`

**Issue:** Both `zsh-pro emit` and `zsh -n` are run synchronously with no timeout (the latter at line 65). A hung binary, a FIFO/slow `$TMPDIR` file, or a pathological validator invocation leaves the user at a blocked prompt. Phase 5 explicitly requires slow `zsh-pro` and validator errors/timeouts to fail open/closed without locking out the shell, but no bounded execution or timeout test exists.

**Fix:** Use a bounded subprocess mechanism appropriate for supported platforms (for example `command timeout 5s` after probing its availability, or a small helper with a watchdog) for both commands, treat timeout as a failed switch, clean the staged file, and add a shim that sleeps past the deadline.

### CR-04: `activate` over an active different profile never deactivates the old profile

**File:** `core/shell/zsh/hook.go:84`

**Issue:** `activate B` always emits and invokes only `zp_apply` for B. In `runtimeEmitter.Emit`, `Diff(A, B)` does calculate both deactivate-A and activate-B operations, but `Provider.Emit` returns them as separate strings and `runtimeEmitter` returns only the apply string (see `core/cli/emitter.go:69-83` and `core/shell/zsh/emit.go:140-161`). Consequently, an alias/function/option added only by A survives `activate A -> activate B -> deactivate`; B's deactivate plan has no operation for A-only state. The existing live test uses a shim whose deactivate restores the entire test state, so it cannot detect the production emitter behavior.

**Fix:** Make `activate` delegate to `checkout` when `ZSHPRO_PROFILE` names a different profile, or have the emitter return a combined deactivate+apply block for this path. Add an integration test using real Store profiles where A owns an alias absent from B and assert it is absent immediately after `activate B` and after deactivate.

### CR-05: Typed-nil store/emitter dependencies bypass the nil guards and panic

**File:** `core/cli/cli.go:68`

**Issue:** The guards use `c.store == nil` and `c.emitter == nil` only. A `(*store.Store)(nil)` stored in either interface is non-nil; `list`, `status`, or `emit` then invokes a method on a nil receiver and panics. The same problem is propagated by `NewRuntimeEmitter` at `core/cli/emitter.go:36-40`. This contradicts the phase's explicit typed-nil fail-closed requirement; current tests cover only a nil interface.

**Fix:** Reject nil-like interface values at construction (or use a small internal `isNilInterface` reflection helper for injected seams), return the existing runtime failure rather than retaining the dependency, and add a test passing `var s *store.Store` as the `Store` interface.

## Warnings

### WR-01: Reinstall does not repair an existing cached loader with unsafe permissions

**File:** `core/cli/install.go:165`

**Issue:** `runInstall` requests mode `0600`, but `atomicWrite` deliberately preserves an existing target's mode. An existing `loader.zsh` at `0644` remains world-readable after reinstall, so the stated `0600` cache-file invariant is not actually enforced. The fresh-install test misses this because it only asserts a newly created file.

**Fix:** Add an explicit `os.Chmod(loader, 0o600)` after the successful replacement (or an `enforceMode` option to `atomicWrite`) and test reinstall from an existing `0644` loader.

### WR-02: The loader permanently disables the user's xtrace option

**File:** `core/shell/zsh/hook.go:72`

**Issue:** `_zp_eval_block` runs `setopt NOXTRACE` to prevent secret leakage but never records or restores the caller's prior option state. Any successful switch changes unrelated terminal behavior, and a profile that intentionally controls `xtrace` is also overridden after the emitted code runs.

**Fix:** Capture whether `XTRACE` was enabled before the protected eval and restore it afterwards unless the emitted plan explicitly owns that option; add a live test that starts with xtrace enabled and verifies the promised post-switch option state.

---

_Reviewed: 2026-07-27T14:52:18Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: deep_
