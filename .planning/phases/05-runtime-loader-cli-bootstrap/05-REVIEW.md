---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-30T03:52:09Z
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

# Phase 5: Final Independent Sol Code Review

**Reviewed:** 2026-07-30T03:52:09Z
**Depth:** deep
**Files Reviewed:** 29
**Status:** issues_found

## Summary

Phase 5 is close, but it is not clean. The final installer/store transaction
repairs are present: unsupported platforms fail during store initialization
before bootstrap writes; fresh-store failures remove the unchanged created
tree; existing-store mode migration is reversible; HOME, XDG, and explicit
roots are covered by built-binary tests. The retained reverse-function design
also gives successful transitions, failed-apply cleanup, retryable cleanup,
deactivation, and secret scrubbing a coherent lifecycle.

One public-shell boundary still violates the option-heavy fail-open contract.
`activate` and `checkout` expand an absent positional parameter before their
usage checks. Under `NO_UNSET`, a no-argument invocation aborts the remaining
sourced command stream, so the documented diagnostic/status path is never
reached.

The complete uncached Go suite, vet, build, lint, and `make check` pass. Those
gates do not exercise this no-argument plus `NO_UNSET` combination.
`hyperfine` remains an unavailable manual timing item and is not a source
finding.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: [BLOCKER] Missing verb arguments abort `NO_UNSET` shells before fail-open handling

**Files:** `core/shell/zsh/hook.go:482`, `core/shell/zsh/hook.go:496`

**Issue:** Both public functions begin with `local name="$1"`. With
`setopt NO_UNSET`, expanding an absent `$1` raises `parameter not set` before
the subsequent `[[ -z "$name" ]]` usage branch. In a sourced/noninteractive
command stream, zsh skips everything after the call. This is incorrect
user-visible behavior for a routine usage error and contradicts Phase 5's
requirement that option-heavy shells return safely. The functions' later
`return 0` cannot help because execution never reaches it.

**Functional reproduction:**

```sh
zsh -fc '
  setopt NO_UNSET
  source <(go run ./core/cmd/zsh-pro hook)
  print BEFORE
  activate
  print AFTER
'
```

Observed output:

```text
BEFORE
activate:1: 1: parameter not set
```

`AFTER` is absent. Replacing `activate` with `checkout` reproduces the same
failure. Adding `ERR_EXIT` does not restore the promised handled path.

**Fix:**

```zsh
activate() {
  local name="${1-}"
  # existing handled usage branch
}

checkout() {
  local name="${1-}"
  # existing handled usage branch
}
```

Add native-zsh regression cases for both verbs with no argument under
`NO_UNSET`, `NO_UNSET ERR_EXIT`, and `NO_UNSET ERR_RETURN`. Assert that the
following command runs, `ZP_LAST_RUNTIME_STATUS == 2`, the usage diagnostic is
present, and no profile/reverse state changes.

## Verification Evidence

- `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- `GOTOOLCHAIN=auto go vet ./...` — passed.
- `GOTOOLCHAIN=auto go build ./...` — passed.
- `make check` — passed (`golangci-lint`: 0 issues; all tests passed).
- Native `zsh -f` no-argument `NO_UNSET` probes — failed as documented in
  CR-01.
- Current installer tests cover new/existing store rollback and HOME,
  `XDG_DATA_HOME`, and explicit `ZSHPRO_HOME`; current built-binary tests cover
  the supported Linux/Darwin selection and unsupported-platform build paths.
- `hyperfine` is unavailable; manual startup timing remains outside this source
  verdict.

---

_Reviewed: 2026-07-30T03:52:09Z_
_Reviewer: GPT-5.6 Sol (gsd-code-reviewer)_
_Depth: deep_
