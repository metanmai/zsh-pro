---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T04:07:57Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 29
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/zsh/reverse_live_test.go
  - core/shell/zsh/emit.go
  - core/shell/zsh/emit_test.go
  - core/shell/zsh/pipeline_test.go
  - core/shell/zsh/residue_test.go
  - core/shell/provider.go
  - core/cli/cli.go
  - core/cli/cli_test.go
  - core/cli/dependencies.go
  - core/cli/store.go
  - core/cli/store_root.go
  - core/cli/store_root_test.go
  - core/cli/emitter.go
  - core/cli/emitter_test.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/runtime.go
  - core/cli/runtime_test.go
  - core/cli/runtime_root.go
  - core/cli/runtime_root_unix.go
  - core/cli/runtime_root_other.go
  - core/cmd/zsh-pro/main.go
  - core/cmd/zsh-pro/main_test.go
  - core/store/store.go
  - core/store/install_transaction.go
  - core/store/store_test.go
findings:
  critical: 1
  warning: 0
  info: 0
  total: 1
status: issues_found
---

# Phase 5: Final Independent Sol Functional Lifecycle Review

**Reviewed:** 2026-07-30T04:07:57Z
**Depth:** deep
**Files Reviewed:** 29
**Status:** issues_found

## Summary

Phase 5 is not clean. The previously reported no-argument failure under
`NO_UNSET` is fixed and its native-zsh regressions pass. Built-binary lifecycle
probes also passed for the HOME default, `XDG_DATA_HOME`, and explicit
`ZSHPRO_HOME`: `install -> source -> activate main -> deactivate -> list`
survived `NO_UNSET ERR_EXIT ERR_RETURN`, retained the expected terminal-lifetime
base/last-good values, and removed active/reverse markers on deactivation.
The complete uncached Go suite, vet, and build pass.

However, the installer applies the `.zshrc` symlink-following policy to the
security-sensitive cached-loader path. It accepts both a symlinked runtime
directory and a symlinked `loader.zsh`, so a successful install can chmod or
overwrite unrelated user-owned filesystem objects. This is an actionable
data-loss/path-redirection defect and blocks release.

`hyperfine` remains an unavailable manual timing item and is not a source
finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] Cached-loader installation follows symlinks and mutates unrelated targets

**Files:** `core/cli/install.go:297-303`, `core/cli/install.go:410-433`,
`core/cli/install.go:453-466`

**Issue:** The runtime-directory guard uses `os.Stat`, which follows a
`~/.zsh-pro` or `ZSHPRO_HOME` symlink, and then calls `os.Chmod` through that
same path. The loader writer subsequently calls the generic
`resolveWriteTarget`, whose deliberate `.zshrc` behavior is to follow a final
symlink with `filepath.EvalSymlinks`. That policy is unsafe for a generated
mode-0600 cache file.

Consequently, `zsh-pro install` can report success after changing an unrelated
directory's permissions and creating a loader inside it, or after replacing an
unrelated file with executable loader source. The store initializer's no-follow
checks do not protect the default cache because the cache defaults to
`$HOME/.zsh-pro` while the store defaults to
`$HOME/.local/share/zsh-pro`/`XDG_DATA_HOME`.

**Functional reproduction 1 — directory redirection:**

```sh
home="$(mktemp -d)"
victim="$(mktemp -d)"
chmod 0755 "$victim"
ln -s "$victim" "$home/.zsh-pro"
HOME="$home" zsh-pro install
stat -c '%a' "$victim"
test -f "$victim/loader.zsh"
```

Observed: install exited 0, the unrelated directory changed from `0755` to
`0700`, and `loader.zsh` was created inside it.

**Functional reproduction 2 — file overwrite:**

```sh
home="$(mktemp -d)"
mkdir -m 700 "$home/.zsh-pro"
victim="$(mktemp)"
printf 'DO NOT OVERWRITE\n' >"$victim"
chmod 0644 "$victim"
ln -s "$victim" "$home/.zsh-pro/loader.zsh"
HOME="$home" zsh-pro install
head -n 1 "$victim"
stat -c '%a' "$victim"
```

Observed: install exited 0; the unrelated file was replaced by the cached
loader and changed to mode `0600`, while the symlink remained.

**Fix:** Split target-resolution policies. Keep explicit symlink resolution only
for the user-selected `.zshrc` compatibility path. For the generated cache:

- walk/validate the runtime directory without following symlinks (the existing
  supported-platform descriptor traversal is the appropriate model);
- reject a symlink or non-directory at every runtime-root component that the
  installer owns;
- reject an existing symlink/non-regular `loader.zsh`;
- create and promote the cache relative to an authenticated directory
  descriptor with no-follow semantics, preserving the current validation and
  rollback transaction.

Add built-binary tests for both reproductions and assert failure leaves the
symlink, target bytes, target mode, store, and `.zshrc` unchanged.

## Verification Evidence

- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `GOTOOLCHAIN=auto go build ./...` — passed.
- Built-binary HOME, XDG, and explicit-root install/source/activate/deactivate
  probes under `NO_UNSET ERR_EXIT ERR_RETURN` — passed.
- Built-binary cache-directory and cache-file symlink probes — failed as
  documented in CR-01.
- `hyperfine` — unavailable manual timing item; not counted as a defect.

---

_Reviewed: 2026-07-30T04:07:57Z_
_Reviewer: GPT-5.6 Sol (independent deep functional reviewer)_
_Depth: deep_
