---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T03:15:20Z
depth: deep
files_reviewed: 27
files_reviewed_list:
  - core/store/store.go
  - core/store/store_root_private_unix.go
  - core/store/store_root_private_other.go
  - core/store/store_root_private_other_test.go
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
  critical: 2
  warning: 0
  info: 0
  total: 2
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-07-30T03:15:20Z
**Depth:** deep
**Files Reviewed:** 27
**Status:** issues_found

## Summary

Phase 5 remains release-blocked by two actionable defects. The install-only initializer now makes fresh and migrated stores usable on the happy path, and failed-target compensation now retains a reachable recovery reverse under normal zsh, `ERR_EXIT`, and `ERR_RETURN`. However, store creation is outside the bootstrap rollback transaction, so a command that reports installation failure leaves a newly initialized repository behind. The portability repair also enables initialization on FreeBSD while the runtime helper still rejects every sourced `list` and `activate` there.

Fresh built-binary probes reproduced the partial-install defect. `GOOS=freebsd GOARCH=amd64 go build ./...` confirmed the mismatched FreeBSD branches compile together. Focused uncached tests, the uncached repository-wide suite, `go vet ./...`, and `make check` all passed despite these defects. Descriptor-root binding, `StoreRoot` precedence, runtime secret resolution and transport scrubbing, active A transition/reversal, failed-B recovery retention and retry blocking, marker/function cleanup, loader validation, and loader/`.zshrc` promotion rollback otherwise matched the current contracts in the paths reviewed. `hyperfine` remains unavailable manual evidence and is not a source finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] A failed install leaves a newly initialized profile store behind

**File:** `core/cli/install.go:56-60`, `core/cli/install.go:61-119`

**Issue:** `runInstallWithStoreInitialization` initializes or migrates the profile repository before preparing the `.zshrc` and loader writes, but none of the later failure paths rolls that store mutation back. On a fresh machine, loader validation, staging, runtime-directory setup, or either promotion can fail after `Store.Init` has created a private bare repository and seeded `main`. The command reports installation failure and correctly leaves no loader or `.zshrc`, yet persistent product state remains. This violates the installer’s prepared-before-mutation transaction contract and makes retry/cleanup behavior depend on a partial installation that the user was told failed. The defect applies to the HOME fallback, `XDG_DATA_HOME`, and explicit `ZSHPRO_HOME`; in the explicit case the partially created repository is also the configured loader-cache directory.

**Reproduction:**

```text
go build -o "$tmp/zsh-pro" ./core/cmd/zsh-pro
mkdir -p "$tmp/home" "$tmp/bin"
ln -s "$(command -v git)" "$tmp/bin/git"
ln -s /bin/false "$tmp/bin/zsh"
HOME="$tmp/home" PATH="$tmp/bin:/usr/bin:/bin" "$tmp/zsh-pro" install

# exit 1:
# zsh-pro: install cached loader: zsh -n: exit status 1
# loader exists: no
# .zshrc exists: no
# $tmp/home/.local/share/zsh-pro exists: yes
# git --git-dir=... rev-parse --is-bare-repository: true
# mode: 0700
```

**Fix:** Make store initialization part of the same explicit install transaction. Have the initializer return rollback state that distinguishes a newly created repository from a pre-existing store/migration, and invoke that rollback on every later failure. Do not delete or rewind a pre-existing repository; only remove state proven to have been created by this invocation. Alternatively prepare both bootstrap artifacts first, initialize the store immediately before promotion, and still retain a creation rollback for promotion failures. Add built-binary tests for loader-validation, runtime-directory, and second-promotion failure under fresh HOME, XDG, and explicit-store routes, asserting that all pre-install filesystem state is restored exactly.

### CR-02: [BLOCKER] FreeBSD installs a store that every sourced runtime command rejects

**File:** `core/store/store_root_private_unix.go:1`, `core/cli/runtime_root_other.go:1-10`

**Issue:** The latest portability fix adds `freebsd` to the descriptor-safe store initializer, so `install` can successfully create and migrate a FreeBSD profile store. But `secureRuntimeRoot` supports only Linux and Darwin; FreeBSD is compiled into the unconditional failure implementation. The installed loader routes `list` and emission through `zsh-pro runtime capture`, so every sourced `list`, `activate`, and `checkout` fails with “runtime store root is unsafe” on a platform the installer just accepted. Ordinary CLI `list` can still work, which makes the failure appear only in the primary live-shell workflow.

**Reproduction:**

```text
GOOS=freebsd GOARCH=amd64 go build ./...

# Build selection:
# core/store/store_root_private_unix.go: //go:build linux || darwin || freebsd
# core/cli/runtime_root_other.go:        //go:build !linux && !darwin
#
# Therefore FreeBSD Store.Init uses the supported descriptor implementation,
# while every `runtime capture` call returns the unsupported-platform error.
```

**Fix:** Keep the platform boundary consistent. Either implement and test FreeBSD descriptor-relative runtime traversal (including its `openat` seam) and include FreeBSD in `runtime_root_unix.go`, or remove FreeBSD from the initializer’s supported build tag so `install` fails before mutating any store/bootstrap state. Add build-tagged tests proving that every platform accepted by `Store.Init` also supports descriptor-bound runtime `list` and emission; unsupported platforms must fail install without mutation.

---

_Reviewed: 2026-07-30T03:15:20Z_
_Reviewer: GPT-5.6 Sol (gsd-code-reviewer)_
_Depth: deep_
