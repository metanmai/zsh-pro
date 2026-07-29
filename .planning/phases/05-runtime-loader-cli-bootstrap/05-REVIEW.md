---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-29T23:38:00Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 21
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/zsh/emit.go
  - core/shell/provider.go
  - core/cli/store.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/cli.go
  - core/cli/cli_test.go
  - core/cli/secret.go
  - core/cli/dependencies.go
  - core/cmd/zsh-pro/main.go
  - core/perf/hyperfine.go
  - core/perf/hyperfine_test.go
  - core/activate/builder.go
  - core/activate/diff.go
  - core/store/store.go
  - scripts/perf-hyperfine.sh
findings:
  critical: 3
  warning: 1
  info: 0
  total: 4
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-29T23:38:00Z
**Depth:** deep
**Files Reviewed:** 21
**Status:** issues_found

## Summary

Phase 5 is not ready to ship. The 05-06 through 05-08 repairs close the prior
typed-nil, malformed-installer, bounded-list, sentinel-collision, target-only
emission, and resolver-loss findings, and the full repository gate passes.
However, direct sourced-zsh probes expose three remaining blockers:

1. a failed A-to-B apply leaves the shell in a mixed state while the authoritative
   marker still claims A is active;
2. generated apply/reverse helper functions overwrite user functions and persist
   after deactivation, including resolved secret literals; and
3. private staging trusts a path beneath an unchecked writable ancestor, so its
   check-then-reopen sequence remains raceable by another user controlling that
   ancestor.

The installer also still changes the cached loader before discovering ordinary
write failures at the `.zshrc` target, so an install that reports failure can
nevertheless change the loader used by an existing bootstrap.

Verification run against the reviewed tree:

- `GOTOOLCHAIN=auto go test -count=1 ./core/cli ./core/shell/zsh ./core/perf` — PASS
- `GOTOOLCHAIN=auto go vet ./...` — PASS
- `make check` — PASS
- direct native-zsh failed-transition probe — reproduced mixed state:
  `marker=A exported=A A=unset B=1 runtime=9`
- the same probe confirmed both generated functions remain defined:
  `functions=1:1`

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Failed profile switches leave a false active marker and an unrecoverable mixed shell

**Classification:** BLOCKER

**Files:** `core/shell/zsh/hook.go:298-322`,
`core/shell/zsh/live_terminal_test.go:117-153`

**Issue:** `_zp_switch` prepends the currently retained `zp_deactivate` to the
new target payload and evaluates the concatenated block. If the old reverse
succeeds but the new `zp_apply` later fails, `_zp_eval_block` returns an error
and `_zp_switch` deliberately leaves `ZP_ACTIVE_PROFILE` and
`ZSHPRO_PROFILE` unchanged. That marker no longer describes reality: the old
profile has already been removed, the target may be partially applied, and the
target payload has already replaced `zp_deactivate`.

This is not only cosmetic. A subsequent `activate A` takes the same-profile fast
path at lines 300-303 and does not restore A, so the user cannot recover by
activating the profile that `status` says is active. The existing
`TestLiveTerminalFailedEvalKeepsActivationMarkerAndProfile` asserts only the
two marker strings and therefore locks in the inconsistency without checking
the actual old/target shell state or the retained reverse identity.

The direct probe activated A, then evaluated a B payload whose apply function
set `B_PART=1` and returned 9. The observed state was:
`marker=A exported=A A=unset B=1 runtime=9`.

**Required corrective action:** Make lifecycle state follow the last completed
transition, not the requested transition. Before executing the old reverse,
clear/transition the active marker so a failed target apply cannot claim the
old profile is live. On failure, invoke the target-specific reverse when it was
successfully defined to remove partial target state, and leave the shell
truthfully inactive unless the old apply can actually be replayed. Remove the
same-profile shortcut when state is not known-good. Extend the live test to
assert A-owned and B-owned env, alias, function, option, and PATH state after
the failure and after the next recovery command.

### CR-02: Runtime helper functions persist, overwrite user state, and retain resolved secrets

**Classification:** BLOCKER

**Files:** `core/cli/emitter.go:88-98`, `core/shell/zsh/emit.go:137-162`,
`core/shell/zsh/hook.go:310-315`, `core/shell/zsh/hook.go:345-352`

**Issue:** Every apply payload globally defines `zp_apply`,
`zp_deactivate`, `zp_capture_scalar`, and `zp_restore_scalar`. The loader calls
these functions but never unsets them or restores pre-existing functions after
a successful apply, switch, failure, or deactivate. This violates zero-residue
behavior and silently destroys any user functions with those names.

It is also a credential-disclosure bug. `resolveSecretRefs` converts a
`SecretRef` into a literal runtime value before emission. That literal is
embedded in `zp_apply` and in the retained `zp_deactivate` comparison source.
Even after `deactivate` removes the environment variable and clears the profile
marker, both function bodies remain readable through zsh's `functions` table
or `functions zp_apply zp_deactivate`. The private staging-file cleanup therefore
does not remove the secret from the live shell.

The direct native-zsh probe confirmed `${+functions[zp_apply]}` and
`${+functions[zp_deactivate]}` both remain 1 after transition evaluation; code
inspection confirms production secret literals occupy those same bodies.

**Required corrective action:** Do not use persistent generic global functions
as the transport. Generate collision-resistant internal function names, capture
and restore any pre-existing definitions, unset the one-shot apply function and
scalar helpers immediately after evaluation, and unset/scrub the retained
reverse immediately after it runs. Only the current target reverse may remain
while a profile is active. Failure cleanup must remove newly defined functions
without destroying the prior retained reverse. Add a real secret-profile live
test that inspects the `functions` table/source after deactivate and asserts the
secret and all generated helper definitions are absent, plus a fixture with
pre-existing functions of the current generic names.

### CR-03: The private staging directory remains replaceable through a writable ancestor

**Classification:** BLOCKER

**File:** `core/shell/zsh/hook.go:73-117`, `core/shell/zsh/hook.go:146-181`,
`core/shell/zsh/hook.go:242-255`

**Issue:** `_zp_private_root` validates only the final `ZSHPRO_HOME` directory
with `-d`, `! -L`, and `-O`, then returns its pathname. It does not validate the
ownership/mode of any ancestor and does not retain a directory descriptor.
`_zp_private_temp` and both consumers subsequently reopen descendants by path.

If `ZSHPRO_HOME` is an absolute victim-owned directory beneath a non-sticky
attacker-writable parent, another user who controls the parent can rename the
validated root and replace it with a symlink after line 87. The attacker can
then rename/replace the per-operation child after its line 106 checks. Later
redirection at line 151 or line 248 and the read at line 176 follow the
replacement path. This recreates the disclosure/source-substitution boundary
that 05-07 intended to close; emitted source can contain resolved credentials
and is subsequently evaluated.

The existing shared-`TMPDIR` regression does not exercise this case because it
places the runtime root in a trusted test directory and attacks a path that the
new implementation no longer uses.

**Required corrective action:** Resolve staging to a runtime location whose
entire ancestor chain is not writable by another user, and fail closed when the
configured root is beneath an unsafe ancestor. Prefer an OS-created per-user
runtime directory and descriptor-relative/no-follow operations; if shell-only
implementation cannot hold safe descriptors, move staging and bounded command
capture into a small binary subcommand that uses `openat`/`O_NOFOLLOW`-style
semantics. Add a multi-user or permission-model regression with a victim-owned
root under a mode-0777 non-sticky parent and race root/stage replacement between
validation, write, validation, and read.

## Warnings

### WR-01: A reported install failure can still replace the active cached loader

**Classification:** WARNING

**File:** `core/cli/install.go:35-63`

**Issue:** The malformed-marker transaction ordering is fixed: `.zshrc` is read
and its replacement is prepared before cache mutation. But the code does not
resolve or prove the `.zshrc` write target is replaceable before promoting the
loader. `writeValidatedLoader` renames the cache at line 58; only afterward does
`atomicWrite` resolve the `.zshrc` symlink/target, create its sibling temporary
file, and rename it. A dangling symlink, unwritable target directory, or
late path-resolution error therefore returns a failed install after changing
the cache. If a prior bootstrap is already present, the changed loader becomes
active on the next shell start despite the failure report.

**Required corrective action:** Prepare both target writes before promoting
either: resolve both final targets, validate the loader candidate, and create/
fsync both sibling temporary files first. Then promote in an explicitly
documented order with rollback of the first rename if the second fails, or
define a recoverable transaction journal. Add a valid-marker fixture whose
`.zshrc` target cannot be replaced and assert the existing loader bytes remain
unchanged when install returns an error.

---

_Reviewed: 2026-07-29T23:38:00Z_
_Reviewer: the agent (gsd-code-reviewer; gpt-5.6-sol)_
_Depth: deep_
