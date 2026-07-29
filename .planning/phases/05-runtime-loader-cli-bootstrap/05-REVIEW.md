---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-29T19:08:20Z
depth: standard
files_reviewed: 15
files_reviewed_list:
  - core/cli/cli.go
  - core/cli/cli_test.go
  - core/cli/dependencies.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/secret.go
  - core/cmd/zsh-pro/main.go
  - core/perf/hyperfine.go
  - core/perf/hyperfine_test.go
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - scripts/perf-hyperfine.sh
findings:
  critical: 7
  warning: 1
  info: 0
  total: 8
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-29T19:08:20Z  
**Depth:** standard  
**Files Reviewed:** 15  
**Status:** issues_found

## Summary

The prior remediation fixed the originally reported marker, cache-mode, bounded-emission, A-to-B, resolver, and typed-nil cases. However, current code still has seven ship-blocking runtime, security, and transaction failures.

Two fresh live probes confirm failures independent of prior reports:

- With a syntactically corrupt cached loader, `setopt ERR_EXIT; source ~/.zshrc; print SURVIVED` exits 127 before `SURVIVED`.
- With `zsh-pro` returning failure, `setopt ERR_EXIT; list; print SURVIVED` exits 1 before `SURVIVED`.

`go test -count=1 ./...`, `go vet ./...`, `bash -n scripts/perf-hyperfine.sh`, and focused CLI/zsh/perf suites all pass. Those green checks do not cover the paths below.

## Narrative Findings (AI reviewer)

## Critical Issues (BLOCKER)

### CR-01: A corrupt cached loader still terminates `ERR_EXIT` shell startup

**Classification:** BLOCKER  
**File:** `core/cli/install.go:123-129`

**Issue:** The installed block invokes `source` as an unguarded simple command. A syntax or runtime error in an otherwise readable `loader.zsh` therefore propagates out of `.zshrc`; under `ERR_EXIT`, zsh exits before the trailing `true` executes. This violates BOOT-02's fail-open startup promise. The current corrupt-cache test runs without `ERR_EXIT`, so it misses this path.

**Fix:** Consume the source result in an `if` body and add an `ERR_EXIT` corrupt-cache regression. Also require a regular readable file before sourcing it.

```zsh
if [[ -f "${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh" ]]; then
  if source "${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh"; then :; else :; fi
fi
```

### CR-02: `list` bypasses the bounded, fail-open runtime boundary

**Classification:** BLOCKER  
**File:** `core/shell/zsh/hook.go:292-294`

**Issue:** `list()` directly executes `command zsh-pro list`. A missing or failing binary makes the function return nonzero and exits a direct `ERR_EXIT` caller; a hung replacement is also unbounded. This is a public sourced-loader verb, so it contradicts the guarantee that a broken or slow binary cannot strand the terminal.

**Fix:** Route `list` through `_zp_run_bounded`, print `REPLY` only on success, report a runtime error on failure/timeout, and always return zero to the interactive caller. Add direct `ERR_EXIT`/`ERR_RETURN` and sleeping-list regressions.

### CR-03: Switching away from an activated `main` profile leaves its state behind

**Classification:** BLOCKER  
**Files:** `core/cli/emitter.go:81-106`, `core/shell/zsh/hook.go:248-258`

**Issue:** `_zp_switch` exports `ZSHPRO_PROFILE=main` after `activate main`, but `runtimeEmitter.Emit` deliberately excludes `current == "main"` when deciding whether an active manifest needs deactivation. `main` is a real, readable branch/profile, not merely an inactive sentinel. Thus `activate main; activate B` emits B's apply half without main's deactivate half, leaving main-only aliases, environment variables, functions, options, or PATH entries in the terminal.

**Fix:** Preserve whether a profile is actually active separately from its display default. For example, expose an `(activeName, active bool)` store seam based on `LookupEnv`, and build an active manifest whenever `active` is true and the name differs from the target. Add a real-store `main -> B -> deactivate` zero-residue test.

### CR-04: A malformed `.zshrc` aborts install only after replacing the cached loader

**Classification:** BLOCKER  
**File:** `core/cli/install.go:35-61`

**Issue:** `runInstall` creates and promotes `loader.zsh` before it reads and validates the managed-marker structure of `.zshrc`. A nested, stray, or unterminated exact marker makes `replaceManagedBlock` fail, but a known-good cached loader has already been replaced and a new cache directory may already exist. This violates the stated no-write-on-malformed-input transaction boundary and leaves a failed install with observable runtime changes.

**Fix:** Resolve both paths once, read and validate/prepare the `.zshrc` replacement before `MkdirAll` or cache promotion, then write the cache and `.zshrc`. Extend malformed-marker tests to assert the prior loader bytes and cache-directory state are unchanged.

### CR-05: The public CLI constructor still panics on a typed-nil shell provider

**Classification:** BLOCKER  
**Files:** `core/cli/cli.go:33-40`, `core/cli/cli.go:56-60`, `core/cli/cli.go:118-139`

**Issue:** `CLI.New` normalizes only `Store` and CLI `Emitter`; it accepts a literal or typed-nil `shell.Provider`. `hook` then calls `c.provider.HookScript()`, `install` passes it to `runInstall`, and `analyze` passes it to `analyze.New`. For example, an interface containing `(*zsh.Provider)(nil)` panics when its value-receiver method is dispatched. This leaves one public dependency seam outside the Phase 5 typed-nil safety guarantee.

**Fix:** Normalize `p` with `isNilLike` too, and make provider-dependent verbs return the normal runtime-unavailable error before invoking it. Add literal-nil and `*zsh.Provider` typed-nil cases for `hook`, `install`, and `analyze`.

### CR-06: Secret-bearing profiles cannot be deactivated if the resolver later fails

**Classification:** BLOCKER  
**Files:** `core/cli/emitter.go:60-67`, `core/cli/emitter.go:81-90`, `core/shell/zsh/hook.go:281-289`

**Issue:** Both `emit deactivate` and the active half of an A-to-B switch call `resolveSecretRefs` again before producing the reverse source. If a profile's secret was successfully activated but its keychain/vault is later unavailable, missing, or locked, `deactivate` emits nothing. The loader consumes the error and leaves `ZSHPRO_PROFILE` plus the secret-bearing shell state live. This is both a zero-residue failure and a credential-exposure risk precisely when the user is trying to deactivate it.

**Fix:** On a successful activation, retain an activation-local reverse manifest/source (including the resolved comparison values) for the active profile and use it for later deactivation/switching; do not require a fresh secret retrieval merely to remove live state. Add an activation-success followed by resolver-failure deactivation test that proves the secret variable and profile state are removed.

### CR-07: `TMPDIR` races can replace or disclose staged emitted source

**Classification:** BLOCKER  
**File:** `core/shell/zsh/hook.go:69-80, 106, 131, 192-198`

**Issue:** `_zp_private_temp` atomically creates a 0600 file but closes it immediately. Later code reopens the path by name for command output and for writing the emitted block. It only checks that `$TMPDIR` is writable; a shared non-sticky directory lets another user unlink and replace the name between those operations. An attacker can make generated source (including resolved secrets) readable, redirect writes through a symlink, or race a replacement source that passes `zsh -n` and is then `eval`'d in the victim shell.

**Fix:** Do not stage in an arbitrary writable `TMPDIR`. Create/use a mode-0700, owner-controlled runtime subdirectory (or validate owner/sticky semantics), keep file descriptors open where possible, and never reopen a path that an untrusted directory can replace. Add a race-oriented regression using a non-sticky shared fixture directory.

## Warnings

### WR-01: The fixed unset sentinel can corrupt a legitimate original environment value

**Classification:** WARNING  
**File:** `core/shell/zsh/hook.go:7, 17-37`

**Issue:** `zp_capture_env` represents an originally unset variable with the literal `__zsh_pro_unset_7c5a0a15__`. If a user originally has that exact string as a managed variable's value, `zp_restore_env` treats it as unset and removes it after deactivation. The result is a deterministic zero-residue/data-preservation violation for a valid shell value.

**Fix:** Store presence separately from the original value (for example, a private `__ZP_ORIG_<name>_WAS_SET` flag) instead of using a magic data sentinel. Add a round-trip regression whose original value equals the current sentinel.

---

_Reviewed: 2026-07-29T19:08:20Z_  
_Reviewer: the agent (gsd-code-reviewer)_  
_Depth: standard_
