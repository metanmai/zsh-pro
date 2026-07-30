---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T00:48:36Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 26
files_reviewed_list:
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
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/invariant_test.go
  - core/shell/zsh/live_terminal_test.go
  - scripts/perf-hyperfine.sh
findings:
  critical: 2
  warning: 0
  info: 0
  total: 2
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-30T00:48:36Z
**Depth:** deep
**Files Reviewed:** 26
**Status:** issues_found

## Summary

The two immediately preceding fixes are incomplete as a shipping boundary. Target B is now fully emitted, captured, and syntax-validated before active A is reversed, and the focused regressions preserve A for emitter failure, empty output, validation failure, and helper/capture failure. Partial B evaluation also reports potentially changed state and consumes B's transient reverse. The installer transaction tests and repository-wide quality gates pass.

Two independently actionable security defects remain:

1. the descriptor walk authenticates `ZSHPRO_HOME` only temporarily, then closes it and launches another `zsh-pro` process which reopens that same pathname for profile and secret reads; and
2. every successful activation leaves the complete emitted payload, including resolved secret literals, in the public global zsh parameter `REPLY`.

The generated apply function is removed, only the active reverse is retained, collision names use 128 bits from `crypto/rand`, failure cleanup consumes the transient reverse, and no shell pathname staging remains. Those improvements do not close the two exposures below.

Verification performed:

- focused runtime capture, active-A preflight, sticky-symlink, secret, and installer tests — PASS
- uncached `go test -count=1 ./...` — PASS
- `go vet ./...` — PASS
- `make check` — PASS
- native zsh secret-payload probe — reproduced `REPLY_LEAK_REPRODUCED`
- `hyperfine` is unavailable; the startup timing target remains a manual evidence item, not a code defect

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: The descriptor-authenticated runtime root is discarded before the emitter reopens it

**Classification:** BLOCKER

**Files:** `core/cli/runtime.go:59-79`, `core/cli/runtime_root_unix.go:17-76`, `core/cmd/zsh-pro/main.go:27-42`, `core/cmd/zsh-pro/main.go:50-57`, `core/store/git.go:38-52`

**Issue:** `runRuntimeCapture` calls `secureRuntimeRoot`, but that function closes every walked descriptor before returning. The helper then starts a fresh `zsh-pro emit` by pathname. That child resolves the unchanged `ZSHPRO_HOME` again in `storeDir`, constructs its store/keychain from the path, and later runs `git -C <path>` and secret retrieval through pathname-based APIs. An attacker who owns a replaceable link in sticky `/tmp` can therefore point it at the victim root during `secureRuntimeRoot`, replace it after the check, and redirect the child emitter to an attacker-controlled repository or vault.

The new race test does not exercise this production control flow: it deliberately uses an emitter shim which never reads `ZSHPRO_HOME`, and it asserts that the emitter was not started while the link happened to be a symlink during validation. It cannot detect replacement after validation followed by the production child's store reopen.

This defeats the claimed no-follow boundary. A substituted attacker profile is emitted, syntax-validates, and is evaluated in the interactive shell; a substituted file-backed secret store can also influence or expose resolved values.

**Reproducible scenario:**

1. Set `ZSHPRO_HOME=/tmp/attacker-owned-link`, initially targeting the victim's private profile repository.
2. Invoke `activate B` while another account repeatedly swaps its own sticky-directory link between the victim and attacker repositories.
3. Allow `secureRuntimeRoot` to finish while the victim target is installed, then swap before `exec.CommandContext(..., "zsh-pro", "emit", ...)` and the child's `git -C`.
4. The child follows the attacker-selected pathname even though the helper previously authenticated a different directory object.

**Fix:** Keep the final authenticated directory descriptor alive and make the emitter consume that exact object. On Linux, pass the descriptor through `ExtraFiles` and address the repository through `/proc/self/fd/<n>` only if every downstream operation preserves descriptor identity; otherwise move store/profile/secret reading and emission into the already-authenticated helper process and use descriptor-relative opens throughout. Do not re-export the unchecked pathname to a second composition-root initialization. Add a deterministic synchronization-hook test that pauses after descriptor validation, swaps the sticky-directory link, resumes emission, and proves the child reads neither attacker profiles nor attacker vault data.

### CR-02: Successful activation persists resolved secret source in global `REPLY`

**Classification:** BLOCKER

**Files:** `core/shell/zsh/hook.go:106-120`, `core/shell/zsh/hook.go:130-148`, `core/shell/zsh/hook.go:290-315`

**Issue:** `_zp_run_bounded` stores captured emitter output in zsh's global `REPLY`; `_zp_emit` returns the same payload through `REPLY`; `_zp_switch` copies it into local `block` but never clears the global. SecretRef resolution happens before source generation, so the payload contains literal resolved secrets. After a successful activation, any later shell code can read or print `$REPLY`, and it remains present independently of the active environment variable and generated apply-function cleanup.

This violates the intended secret lifetime and creates an unnecessary second plaintext copy in long-lived interactive shell state. The existing secret cleanup tests inspect generated functions and operation slots but do not assert that transport globals are scrubbed.

**Native-zsh reproduction:**

```zsh
source <(zsh-pro hook)
# With a transport fixture returning a payload containing phase5-secret-leak:
activate secret
[[ "$REPLY" == *phase5-secret-leak* ]] && print REPLY_LEAK_REPRODUCED
```

The probe on this checkout printed `REPLY_LEAK_REPRODUCED`.

**Fix:** Stop using the conventional global `REPLY` for secret-bearing transport. Return data through a caller-named private parameter or a dynamically scoped local, and clear every intermediate in an `always` block on success and every failure path. At minimum, `_zp_switch` must unset/overwrite `REPLY` immediately after copying it, and `_zp_emit`/`_zp_run_bounded` must not leave additional global copies. Add native-zsh tests for successful secret activation, emitter failure, validation failure, partial evaluation failure, switch, and deactivate; after each boundary assert that no global parameter or obsolete generated function contains the resolved fixture value, while the one active reverse capability retains only the state strictly required for safe reversal.

---

_Reviewed: 2026-07-30T00:48:36Z_
_Reviewer: the agent (gsd-code-reviewer; gpt-5.6-sol)_
_Depth: deep_
