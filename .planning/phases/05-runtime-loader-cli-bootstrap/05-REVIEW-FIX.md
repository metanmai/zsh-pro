---
phase: 05
fixed_at: 2026-07-30T05:07:11Z
review_path: .planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md
iteration: 10
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-30T05:07:11Z
**Source review:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-REVIEW.md`
**Iteration:** 10

**Summary:**

- Findings in scope: 2
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: [BLOCKER] `install` ignores trailing arguments and performs unintended filesystem mutation

**Status:** fixed: requires human verification
**Files modified:** `core/cli/cli.go`, `core/cli/cli_test.go`, `core/cli/install_test.go`
**Commit:** `e7fee67`
**Applied fix:** The top-level dispatcher now rejects trailing arguments for `install`, `hook`, `list`, `status`, `--version`, and `-v` before provider/store resolution or install-path work, with command-specific usage and exit code 2. The built-binary regression runs both `install --help` and an arbitrary extra argument against fresh and pre-populated HOME fixtures, snapshots their full trees byte-for-byte, and proves no `.zshrc`, cached loader, or profile store mutation.

### WR-01: [WARNING] Sourced verbs silently ignore extra positional arguments

**Status:** fixed: requires human verification
**Files modified:** `core/shell/zsh/hook.go`, `core/shell/zsh/live_terminal_test.go`
**Commits:** `bb2ed25`, `cc22815`
**Applied fix:** `activate` and `checkout` now require exactly one non-empty argument; `deactivate`, `list`, and `status` require zero. Validation occurs at each public function boundary before local capture, emit, reversal, or state work; invalid calls emit usage, set `ZP_LAST_RUNTIME_STATUS=2`, and return 0. The native-zsh matrix covers missing, explicitly empty, and surplus forms across normal, `NO_UNSET`, `ERR_EXIT`, and `ERR_RETURN` configurations while preserving active markers, retained reverse function, secret, `REPLY`, and timeout state.

## Verification

- Focused CLI regression: `GOTOOLCHAIN=auto go test -count=1 ./core/cli -run '^(TestNoArgumentCommandsRejectTrailingArgumentsBeforeDependencies|TestBuiltBinaryInstallRejectsTrailingArgumentsWithoutMutation)$' -v` — passed.
- Focused native-zsh matrix: `GOTOOLCHAIN=auto go test -count=1 ./core/shell/zsh -run '^TestLiveTerminalPublicVerbArityFailsOpenBeforeRuntimeWork$' -v` — passed all 54 missing, explicitly empty, surplus, and shell-option cases.
- Full uncached suite: `GOTOOLCHAIN=auto go test -count=1 ./...` — passed.
- Static/project gates: `GOTOOLCHAIN=auto go vet ./...`, `GOTOOLCHAIN=auto go build ./...`, and `GOTOOLCHAIN=auto make check` — passed; `golangci-lint` reported 0 issues.
- Cross-builds: `GOOS=linux GOARCH=amd64`, `GOOS=darwin GOARCH=amd64`, and `GOOS=darwin GOARCH=arm64` with `go build ./...` — passed.
- Direct probes: a freshly built binary returned exit 2 for `install --help` without creating store/cache/`.zshrc`; `zsh -f` sourced-loader probes under `NO_UNSET ERR_EXIT ERR_RETURN` rejected both surplus and explicitly empty `activate` arguments while preserving active state and returning to the caller.
- Performance harness syntax: `bash -n scripts/perf-hyperfine.sh` — passed. `hyperfine` is absent on this host, so the measured startup-budget check remains a manual/CI gate.

---

_Fixed: 2026-07-30T05:07:11Z_
_Fixer: the agent (gsd-code-fixer)_
_Iteration: 10_
