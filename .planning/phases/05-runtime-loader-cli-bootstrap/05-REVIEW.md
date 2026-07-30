---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T04:49:07Z
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
  critical: 1
  warning: 1
  info: 0
  total: 2
status: issues_found
---

# Phase 5: Final Independent Sol Functional Lifecycle Review

**Reviewed:** 2026-07-30T04:49:07Z
**Depth:** deep
**Files Reviewed:** 37
**Status:** issues_found

## Summary

Phase 5 is not clean. The latest descriptor-relative cached-loader repair closes
the previously reported symlink redirection defect, and the complete uncached Go
suite plus vet pass. The reviewed lifecycle now has coherent distinct defaults
for the cached loader (`$HOME/.zsh-pro`) and profile store
(`$XDG_DATA_HOME/zsh-pro` or `$HOME/.local/share/zsh-pro`), with explicit
`ZSHPRO_HOME` joining them. Existing built-binary/native-zsh coverage exercises
transactional install rollback, supported/unsupported platform boundaries,
restrictive shell options, retained reversal/retry, and runtime marker/function
cleanup.

One release blocker remains: top-level commands that take no arguments ignore
all trailing arguments. A fresh built binary executed `zsh-pro install --help`
as a successful real installation, creating the store, cache, and `.zshrc`.
The sourced verbs have the same cardinality defect and can silently activate a
profile despite an accidental extra token.

`hyperfine` remains an unavailable manual timing item and is not a source
finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] `install` ignores trailing arguments and performs unintended filesystem mutation

**File:** `core/cli/cli.go:65-83`

**Issue:** The dispatcher validates arguments only for `emit` and `runtime`.
`install`, `hook`, `list`, `status`, and the version flags execute without
checking `len(args)`. This is correctness-sensitive for `install`: a typo or the
conventional help request `zsh-pro install --help` is silently treated as
authorization to initialize the profile store, write the cached loader, and
rewrite `.zshrc`. It exits 0 and prints `zsh-pro: installed`, so callers cannot
detect that their requested argument was ignored.

**Simple reproduction:**

```sh
home="$(mktemp -d)"
HOME="$home" ./zsh-pro install --help
find "$home" -maxdepth 4 -print
```

Observed with a fresh build: exit 0, and new `$home/.local/share/zsh-pro`,
`$home/.zsh-pro/loader.zsh`, and `$home/.zshrc` were created.

**Fix:** Enforce exact top-level cardinality before dispatch. `install`, `hook`,
`list`, `status`, `--version`, and `-v` must require `len(args) == 1`; otherwise
print command-specific usage and return `model.ExitUsageErr` without resolving
paths or initializing storage. Add built-binary regressions for `install
--help` and arbitrary extra arguments that assert exit 2 and byte-for-byte
absence/preservation of store, cache, and `.zshrc`.

## Warnings

### WR-01: [WARNING] Sourced verbs silently ignore extra positional arguments

**File:** `core/shell/zsh/hook.go:481-555`

**Issue:** `activate` and `checkout` validate only that `$1` is non-empty, while
`deactivate`, `list`, and `status` do not validate argument count at all.
Commands such as `activate work accidental` therefore activate `work` and
report success. This disagrees with the documented one-profile/no-argument
surfaces and makes shell typos state-changing rather than diagnosable.

**Simple reproduction:**

```zsh
source <(zsh-pro hook)
activate main accidental
status
```

With an initialized store this activates `main`; the extra token is ignored.

**Fix:** At each public function boundary validate `$#` before any capture,
emit, reversal, or output. Require exactly one argument for `activate` and
`checkout`, and zero for `deactivate`, `list`, and `status`. Preserve the
fail-open contract by setting `ZP_LAST_RUNTIME_STATUS=2`, printing usage, and
returning 0. Cover the checks under `NO_UNSET`, `ERR_EXIT`, and `ERR_RETURN`,
asserting shell state and retained markers are unchanged.

## Verification Evidence

- `go test -count=1 ./...` — passed.
- `go vet ./...` — passed.
- `bash -n scripts/perf-hyperfine.sh` — passed.
- Fresh built-binary `HOME=<empty> zsh-pro install --help` probe — failed as
  CR-01 documents: exit 0 and full install side effects.
- Current native-zsh lifecycle suites cover missing arguments, restrictive
  options, transition failures, retry/deactivation, secret transport, and
  marker/function cleanup; they do not cover surplus arguments.
- `hyperfine` — unavailable manual timing item; not counted as a defect.

---

_Reviewed: 2026-07-30T04:49:07Z_
_Reviewer: GPT-5.6 Sol (independent deep functional reviewer)_
_Depth: deep_
