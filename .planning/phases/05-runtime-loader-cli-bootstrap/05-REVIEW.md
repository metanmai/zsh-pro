---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T20:01:11Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 37
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
  - core/cli/cache_directory.go
  - core/cli/cache_directory_unix.go
  - core/cli/cache_directory_other.go
  - core/cli/cache_syscalls_linux.go
  - core/cli/cache_syscalls_darwin.go
  - core/cmd/zsh-pro/main.go
  - core/cmd/zsh-pro/main_test.go
  - core/store/store.go
  - core/store/install_transaction.go
  - core/store/store_test.go
  - core/activate/builder.go
  - core/activate/diff.go
  - scripts/perf-hyperfine.sh
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 5: Final Bounded Independent Sol Release Review

**Reviewed:** 2026-07-30T20:01:11Z
**Depth:** deep
**Files Reviewed:** 37
**Status:** clean

## Summary

No actionable Phase 5 functional correctness defect remains in the bounded
acceptance lifecycle.

The current dispatcher rejects surplus arguments before dependency or
filesystem resolution. A built-binary matrix proves that `install --help` and
an arbitrary trailing argument return usage exit 2 without creating or changing
the profile store, cached loader, or `.zshrc`, for both fresh and pre-existing
homes. The other no-argument CLI commands and version flags use the same
exact-cardinality boundary.

The sourced loader enforces exactly one non-empty argument for `activate` and
`checkout`, and zero arguments for `deactivate`, `list`, and `status`. Native-zsh
coverage exercises missing, empty, and surplus forms under ordinary operation,
`NO_UNSET`, `ERR_EXIT`, `ERR_RETURN`, and their restrictive combinations. These
calls remain fail-open and preserve active markers, retained reverse functions,
secrets, `REPLY`, timeout state, base PATH, and last-good state without invoking
runtime capture or reversal.

Installer and runtime coverage exercises fresh and pre-existing HOME, XDG data
roots, explicit `ZSHPRO_HOME`, separate `ZDOTDIR`, supported and unsupported
platform paths, store initialization compensation, cache and `.zshrc` rollback,
symlink and replacement rejection, retained reverse recovery, failed target
cleanup, profile transitions, and marker/function/secret lifecycle. Cached
loader resolution and profile-store resolution remain intentionally distinct:
the cache defaults to `$HOME/.zsh-pro`, the store defaults through
`XDG_DATA_HOME` or `$HOME/.local/share`, and explicit `ZSHPRO_HOME` joins them.
The sourced runtime consumes the store root through the same `StoreRoot`
contract as the ordinary CLI.

All reviewed files meet the bounded Phase 5 functional quality standard. No
issues found.

## Narrative Findings (AI reviewer)

None.

## Verification Evidence

- `GOTOOLCHAIN=auto go test -count=1 ./core/cli ./core/shell/zsh ./core/cmd/zsh-pro` — passed.
- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `GOTOOLCHAIN=auto go build ./...` — passed.
- `GOTOOLCHAIN=auto make check` — passed, including `golangci-lint` with zero issues.
- Cross-builds for Linux amd64, Darwin amd64/arm64, FreeBSD amd64, and Windows amd64 — passed.
- `bash -n scripts/perf-hyperfine.sh` — passed.
- `hyperfine` is unavailable on this host. The less-than-10-ms startup
  measurement remains manual timing evidence only and is not a source finding.

---

_Reviewed: 2026-07-30T20:01:11Z_
_Reviewer: GPT-5.6 Sol (independent bounded functional reviewer)_
_Depth: deep_
