---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T00:00:13Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 22
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/invariant_test.go
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
  critical: 2
  warning: 0
  info: 0
  total: 2
status: issues_found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-30T00:00:13Z
**Depth:** deep
**Files Reviewed:** 22
**Status:** issues_found

## Summary

Phase 05 is still not ready to ship. The four repair commits close the specific
runtime-evaluation cleanup, generated-function lifetime, ordinary late installer
promotion, and non-sticky writable-directory cases covered by their new tests.
The focused tests pass. Independent call-chain review and native-zsh probing
nevertheless found two remaining security/correctness blockers:

1. `_zp_switch` destroys the current profile before target emission and syntax
   validation succeed, so an ordinary missing target or failed binary reports
   "shell state unchanged" after actually deactivating the current profile; and
2. the staging ancestor check uses `zstat -L`, which follows symlinks. An
   attacker-owned link in a sticky shared ancestor can therefore pass validation
   based on its victim-owned target and then be replaced before the later
   pathname-based operations.

The generated function names are collision-resistant, the apply function is
scrubbed, only the active reverse is retained, failure/deactivation consume it,
and the reviewed collision fixtures preserve pre-existing functions. The
installer now prepares both targets and successfully rolls back the loader in
the tested late `.zshrc` promotion failure.

`hyperfine` remains absent. The structural zero-subprocess gate is present, but
the stated interactive startup measurement remains a manual/CI verification
item; tool absence is not a source defect.

Focused verification:

- repaired runtime/function/staging tests — PASS
- repaired emitter/installer transaction tests — PASS
- native-zsh active-A then failed-B-emitter probe — reproduced:
  `before active=A value=present`; then
  `after active=unset exported=unset value=unset status=7` while the diagnostic
  said `shell state unchanged`
- repository gates supplied independently for this integrated checkout:
  build, uncached tests, vet, and `make check` — PASS

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Target preflight failures deactivate the known-good current profile

**Classification:** BLOCKER

**Files:** `core/shell/zsh/hook.go:423-439`,
`core/shell/zsh/hook.go:275-283`

**Issue:** `_zp_switch` calls `_zp_reverse_active_profile` before `_zp_emit`
and before `_zp_eval_block` performs `zsh -n` validation. Consequently any
ordinary target-side preflight failure—missing profile, unavailable/failing
binary, empty emitter output, staging failure, or invalid generated
syntax—removes the current profile and clears both active markers even though no
target source has executed. `_zp_emit` then emits the explicitly false
diagnostic `shell state unchanged`.

This is distinct from the repaired partial-runtime-evaluation case. Once target
apply has begun, truthful inactive state plus target-specific reverse cleanup is
appropriate. Before target execution begins, however, the known-good current
state is still recoverable and must not be destroyed merely to discover that
the target cannot be emitted or parsed.

Reproduction with the real cached loader and a fake `zsh-pro` emitter:

```text
activate A
before active=A value=present status=0
activate B  # emitter exits 7
zsh-pro: emit apply failed; shell state unchanged
after active=unset exported=unset value=unset status=7
```

**Required corrective action:** Split target preparation from transition
execution. Capture and syntax-validate the target payload while A is still
active. Only after that succeeds should the loader clear markers, run A's
retained reverse, define/run the already-validated B payload, and apply the
existing partial-apply cleanup rules. Preserve A on every failure before the
first state-changing operation. Add native-zsh tests for emitter nonzero, empty
output, validator rejection, and staging failure while A is active; each must
assert A-owned env/alias/function/option/PATH, markers, and retained reverse are
unchanged.

### CR-02: Symlink-following staging validation leaves the checked root replaceable

**Classification:** BLOCKER

**Files:** `core/shell/zsh/hook.go:107-129`,
`core/shell/zsh/hook.go:137-170`,
`core/shell/zsh/hook_test.go:57-59`

**Issue:** `_zp_private_chain_safe` uses `zstat -L` for every path component.
In zsh, `-L` dereferences symbolic links; it does not perform the promised
no-follow check. The structural test compounds the error by requiring the
literal `zstat -L` while claiming that this validates ancestors "without
following symlinks."

The non-sticky-directory regression therefore covers only one replacement
mechanism. With `ZSHPRO_HOME=/tmp/victim-runtime`, an attacker can own the
`victim-runtime` symlink in root-owned sticky `/tmp` and initially point it at a
victim-owned mode-0700 directory. The dereferenced uid/mode checks pass.
Because sticky-bit rules protect the link's owner rather than the dereferenced
target's owner, the attacker may replace their own link after validation.
Subsequent `chmod`, `mkdir`, redirection, validation, and read operations reopen
the pathname and can be redirected to attacker-controlled content. Emitted
source may contain resolved credentials and is later evaluated, so this remains
a disclosure/source-substitution boundary.

**Required corrective action:** Reject every symlink component using no-follow
metadata (and validate the link object, not only its target), including the
configured root. Do not encode `zstat -L` as evidence of no-follow behavior.
Because separate pathname checks remain inherently raceable, prefer moving
staging/capture to a binary helper using descriptor-relative operations and
`O_NOFOLLOW`/`openat`-style traversal. At minimum, add a real permission-model
regression with an attacker-owned symlink in a sticky shared ancestor and race
replacement after each validation boundary; assert no attacker source is read
or evaluated and no secret-bearing bytes reach the attacker path.

---

_Reviewed: 2026-07-30T00:00:13Z_
_Reviewer: the agent (gsd-code-reviewer; gpt-5.6-sol)_
_Depth: deep_
