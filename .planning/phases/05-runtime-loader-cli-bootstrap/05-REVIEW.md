---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T02:43:31Z
depth: deep
files_reviewed: 27
files_reviewed_list:
  - core/store/store.go
  - core/store/store_root_private_unix.go
  - core/store/store_root_private_other.go
  - core/store/store_test.go
  - core/store/git.go
  - core/store/runtime_openat_linux.go
  - core/store/runtime_openat_darwin.go
  - core/store/runtime_vault_unix.go
  - core/store/runtime_vault_other.go
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/shell/zsh/residue_test.go
  - core/shell/zsh/reverse_live_test.go
  - core/cli/cli.go
  - core/cli/store.go
  - core/cli/store_root.go
  - core/cli/runtime.go
  - core/cli/runtime_root.go
  - core/cli/runtime_root_unix.go
  - core/cli/runtime_root_other.go
  - core/cli/emitter.go
  - core/cli/install.go
  - core/cmd/zsh-pro/main.go
  - core/cmd/zsh-pro/main_test.go
findings:
  critical: 3
  warning: 0
  info: 0
  total: 3
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-07-30T02:43:31Z
**Depth:** deep
**Files Reviewed:** 27
**Status:** issues_found

## Summary

The latest Store permission migration (`af6c6b9`) and transactional retained-reverse repair (`9904a81`) close their narrow regression tests, but Phase 5 remains release-blocked by three actionable defects. Production never invokes the repaired initializer, failed target cleanup still discards its recovery pointer and can retain a resolved secret, and the portable initializer performs a pathname-based chmod after a separate inspection.

Fresh built-binary and native-zsh probes reproduced the first two defects. Both focused uncached package tests and the repository-wide `make check` gate pass despite them. Store-root precedence, descriptor-bound reads, active-profile authority, target preflight ordering, public fail-open return behavior, global transport scrubbing, installer promotion rollback, option restoration, and successful reverse cleanup otherwise matched the current contracts in the paths exercised. `hyperfine` remains unavailable manual performance evidence only and is not a code finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] The product never initializes or migrates the profile store

**File:** `core/cmd/zsh-pro/main.go:36-56`; `core/store/store.go:125-159`; `core/cli/install.go:109-120`

**Issue:** The permission repair is reachable only through `Store.Init`, but no production code calls `Init`; every caller is a test. `newCLI` constructs a `Store` and injects it without initializing it, while `install` writes only `.zshrc` and the cached loader. A fresh installation therefore reports success but leaves the default `$HOME/.local/share/zsh-pro` absent. `list` then fails through the path-based store, and sourced `list`/`activate`/`checkout` fail the runtime-root capture before emission. An existing pre-repair 0755 store is likewise never migrated to 0700, so `af6c6b9` does not repair real users.

With explicit `ZSHPRO_HOME`, installation creates that directory as the loader cache (0700) but still does not make it a bare repository. This makes both documented root routes unusable from a normal fresh install.

**Reproduction:**

```text
go build -o "$tmp/zsh-pro" ./core/cmd/zsh-pro
HOME="$tmp/home" "$tmp/zsh-pro" install
# zsh-pro: installed
HOME="$tmp/home" "$tmp/zsh-pro" list
# zsh-pro: list profiles: zsh-pro: git operation failed
# $tmp/home/.local/share/zsh-pro does not exist

ZSHPRO_HOME="$tmp/explicit" HOME="$tmp/home" "$tmp/zsh-pro" install
ZSHPRO_HOME="$tmp/explicit" HOME="$tmp/home" "$tmp/zsh-pro" list
# zsh-pro: list profiles: zsh-pro: git operation failed
# explicit exists at 0700, but is not a bare repository
```

**Fix:** Add an explicit production initialization/migration lifecycle. At minimum, `install` must resolve `StoreRoot`, construct the store, call `Init`, and include store initialization in its error/transaction reporting; alternatively expose a dedicated bootstrap command and have install invoke it. Avoid silently mutating storage from unrelated read-only verbs. Add real-binary integration tests for fresh HOME fallback, XDG_DATA_HOME, explicit ZSHPRO_HOME, and migration of an existing 0755 bare store, followed by both ordinary and `runtime capture` list/emission.

### CR-02: [BLOCKER] Failed target cleanup loses its pointer and leaves resolved secret state orphaned

**File:** `core/shell/zsh/hook.go:305-325`; `core/shell/zsh/hook.go:360-417`

**Issue:** `_zp_run_payload` correctly attempts the new target's reverse when its apply function returns nonzero, but it ignores whether that reverse succeeded and then unconditionally unsets `ZP_ACTIVE_REVERSE_FN`. `_zp_run_transient_reverse_payload` retains a failed reverse function, so the random function (including static resolved secret literals in its body) remains in the shell while the sole pointer to it is destroyed. `_zp_switch` returns through the public fail-open boundary with no active marker. A later activate sees neither an active profile nor a stale reverse pointer and can layer another profile over the partially applied target. The user cannot retry cleanup, and a resolved secret can remain both exported and embedded in an orphan function.

This is distinct from the repaired active-profile reversal path: the new tests cover failure while reversing A, but not failure of B's compensating reverse after B's apply partially mutates the shell.

**Reproduction:**

```text
__zp_apply_probe() {
  export ZP_PROBE_SECRET='resolved-secret'
  return 73
}
__zp_reverse_probe() { return 74 }
_zp_run_payload __zp_apply_probe __zp_reverse_probe
```

Returned through `activate B` under normal zsh, `ERR_EXIT`, and `ERR_RETURN`:

```text
SURVIVED status=73 active=unset ptr=unset
secret=resolved-secret reverse_function_present=1
```

**Fix:** Treat failed apply compensation as retained recovery state. If the transient reverse fails, preserve its name in a dedicated recovery pointer (or `ZP_ACTIVE_REVERSE_FN` with an explicit partial/recovery marker), keep enough lifecycle state to force cleanup before any later activation, and report the cleanup failure rather than only the original apply status. Consume the function/pointer and scrub its secret-bearing source only after compensation succeeds. Add native-zsh tests for apply failure plus reverse failure, retry after repair, a later switch attempt, secret non-disclosure, and normal/ERR_EXIT/ERR_RETURN modes.

### CR-03: [BLOCKER] The portable Store initializer can chmod a substituted or foreign target

**File:** `core/store/store_root_private_other.go:13-29`

**Issue:** On every platform other than Linux and Darwin, `ensurePrivateStoreDir` performs `Lstat(dir)`, validates that pathname, and then calls `os.Chmod(dir, 0700)` in a separate pathname lookup. The final object can be swapped to a symlink or different directory between those calls, causing chmod to follow and modify the substituted target. This is exactly the symlink/foreign-target behavior the Unix implementation was added to prevent. The portable branch also performs no ownership validation before chmod. Runtime capture currently fails closed on these platforms, but `Store.Init` remains callable and destructive there, so that does not neutralize the initializer vulnerability.

**Fix:** Use a platform-supported no-follow descriptor open plus `Fstat`/ownership check and `Fchmod`, as on Linux/Darwin. If a target platform cannot provide that primitive, fail closed without chmod instead of implementing a check-then-use pathname fallback. Add platform-specific tests or an injected filesystem seam that swaps the final object between inspection and mutation and proves the substituted target is unchanged.

---

_Reviewed: 2026-07-30T02:43:31Z_
_Reviewer: GPT-5.6 Sol (gsd-code-reviewer)_
_Depth: deep_
