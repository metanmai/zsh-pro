---
phase: 05
fixed_at: 2026-07-29T23:54:39Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-29T23:54:39Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 1

**Summary:**

- Findings in scope: 4
- Fixed: 4
- Skipped: 0

## Fixed Issues

### CR-01: Failed profile switches leave a false active marker and an unrecoverable mixed shell

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** 1c3f003
**Applied fix:** Clears lifecycle markers before reversal, retains target-specific cleanup only while active, and consumes a failed target's reverse before returning inactive. Native zsh coverage asserts environment, alias, function, option, PATH, marker, and generated-function cleanup through failed switch, recovery, and deactivation.

### CR-02: Generated runtime helpers collide with user functions and retain resolved secrets

**Status:** fixed: requires human verification
**Files modified:** `core/cli/emitter.go`, `core/cli/emitter_test.go`, `core/shell/provider.go`, `core/shell/zsh/emit.go`, `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`, `core/shell/zsh/invariant_test.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** 23159ba
**Applied fix:** Emits random internal apply/reverse names, rejects collisions before definition, keeps scalar support loader-owned, retains only the active reverse, and scrubs generated functions plus resolved-secret bodies after failure or deactivation. Native zsh tests preserve pre-existing user functions and inspect the live function table for secret retention.

### CR-03: Runtime staging trusts writable ancestors during check-then-reopen

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/hook_test.go`, `core/shell/zsh/live_terminal_test.go`
**Commit:** 708f99b
**Applied fix:** Validates every lexical runtime-root ancestor with `zsh/stat`, rejects symlinks, untrusted owners, and non-sticky group/other-writable directories, then revalidates after securing the root. The live regression races root and stage replacement beneath a non-sticky `0777` ancestor and verifies emitted source is never evaluated.

### WR-01: A reported install failure can still replace the active cached loader

**Status:** fixed: requires human verification
**Files modified:** `core/cli/install.go`, `core/cli/install_test.go`
**Commit:** 22e3c57
**Applied fix:** Prepares and fsyncs both final-target temporaries plus rollback copies before either rename; on loader or `.zshrc` promotion failure, restores prior targets and cache-directory mode. Tests cover dangling target resolution, a valid marker in an unwritable target directory, and a late post-prepare `.zshrc` promotion failure with loader rollback.

## Verification

- `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run 'TestLiveTerminal(FailedEvalLeavesTruthfulInactiveStateAndRecovers|RetainedSecretReverseSurvivesUnavailableBinary|StagingRejectsNonStickyWritableAncestorReplacement)$'` — passed
- `GOTOOLCHAIN=auto go test -count=1 ./core/cli -run 'TestRuntimeEmitter(PreservesUserFunctionsAndScrubsResolvedSecrets|FailedSwitchScrubsResolvedSecretPayload|PayloadCollisionLeavesPreexistingFunctionUntouched)$|TestInstall(ZshrcPreparationFailurePreservesKnownGoodCache|RollsBackLoaderWhenPreparedZshrcCannotBePromoted)$'` — passed
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed
- `GOTOOLCHAIN=auto go vet ./...` — passed
- `make check` — passed

## Remaining Manual Verification

`hyperfine` is unavailable in this environment, so the source-path performance benchmark remains unverified.

---

_Fixed: 2026-07-29T23:54:39Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 1_

