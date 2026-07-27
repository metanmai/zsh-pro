---
phase: 05-runtime-loader-cli-bootstrap
reviewed: 2026-07-27T15:13:51Z
reviewer_model: gpt-5.6-sol
depth: deep
files_reviewed: 15
files_reviewed_list:
  - core/shell/zsh/hook.go
  - core/shell/zsh/hook_test.go
  - core/shell/zsh/live_terminal_test.go
  - core/shell/provider.go
  - core/cli/cli.go
  - core/cli/store.go
  - core/cli/emitter.go
  - core/cli/install.go
  - core/cli/install_test.go
  - core/cli/cli_test.go
  - core/cmd/zsh-pro/main.go
  - scripts/perf-hyperfine.sh
  - core/shell/zsh/emit.go
  - core/activate/diff.go
  - core/store/store.go
supporting_files_traced:
  - core/activate/builder.go
  - core/store/secret.go
  - core/model/profile.go
findings:
  critical: 7
  warning: 4
  info: 0
  total: 11
status: issues_found
---

# Phase 5: Independent Sol Code Review

**Reviewed:** 2026-07-27T15:13:51Z  
**Depth:** deep  
**Files Reviewed:** 15 primary files plus 3 supporting call-chain files  
**Status:** issues_found

## Summary

Phase 5 is not ready to ship. The full Go suite passes, but independent probes
confirm all five blockers carried by `05-REVIEW.md` / `05-VERIFICATION.md`:

1. marker-like text in ordinary `.zshrc` content is deleted;
2. a handled verb failure still exits a shell using `ERR_EXIT`;
3. emitter and validator subprocesses are unbounded;
4. `activate A -> activate B` leaves A-only state behind; and
5. typed-nil dependencies panic.

The cache-mode warning also reproduces. The earlier xtrace warning does not
reproduce and should remain refuted.

This review found two additional shipping blockers. First, Phase 3's persisted
`SecretRef` entries are silently skipped by the Phase 5 activation path because
no runtime resolver is injected. Second, when `HOME`, `ZDOTDIR`, and
`ZSHPRO_HOME` are unset, `install` writes `.zshrc` and `.zsh-pro/loader.zsh`
relative to the process working directory rather than failing closed.

Four robustness warnings remain around cache permission repair, validation
ordering, staged-source temporary-file handling, and the optional performance
harness.

## Probe Results

These probes were run against the current checkout, not inferred from the prior
reports:

| Probe | Observed result |
| --- | --- |
| `go test ./...` | PASS in all packages |
| Install over quoted marker-like user lines | `ordinary-content-must-survive` was deleted |
| Direct failed `activate` with `setopt ERR_EXIT` | process exit 1; the command after `activate` did not run |
| Sleeping emitter under a 1-second outer watchdog | watchdog exit 124 |
| Sleeping `zsh -n` replacement under a 1-second outer watchdog | watchdog exit 124 |
| Production-shaped `activate A; activate B` fixture | exit 42 with `STALE_A_SURVIVED` |
| `var s *store.Store` injected as `cli.Store` | panic |
| Typed-nil pointer injected as `cli.Emitter` | panic |
| Existing `loader.zsh` mode 0644 followed by reinstall | remained 0644 |
| `HOME`, `ZDOTDIR`, and `ZSHPRO_HOME` all unset | created cwd-relative `.zshrc` and `.zsh-pro/loader.zsh` |
| Validator failure with an existing known-good cache | install returned 1 after replacing the old cache |
| Persisted-secret-shaped entry through `activate.Build` | manifest contained zero environment entries |
| Xtrace preservation probe | `XTRACE_PRESERVED` |
| Valid JSON containing `"mean": 0.100` through the harness regex | zero matches |

Passing tests are therefore not contrary evidence. The tests use fixtures that
do not model the failing production transitions and omit the relevant boundary
values.

## Narrative Findings (AI reviewer)

## Critical Issues (BLOCKER)

### CR-01: Marker substrings inside ordinary user text are treated as managed regions

**Status:** Confirmed  
**File:** `core/cli/install.go:97-125`

**Issue:** `replaceManagedBlock` locates `installBegin` and `installEnd` with
`bytes.Index`, so it recognizes the marker text anywhere in a byte stream. A
quoted `print '# >>> zsh-pro >>>'` line and a later quoted end marker become a
managed region. Everything between them is replaced. This is an ordinary
data-loss path and directly violates BOOT-01's byte-preservation guarantee.

The Phase 5 threat model called literal marker collision an accepted low
residual, but that disposition conflicts with the phase requirement that
unmanaged content is never changed. The review and verification reports are
correct to treat this as blocking.

**Fix:** Parse the file line-by-line with a small marker state machine. Recognize
a marker only when the complete logical line equals the marker (accounting for
the chosen newline convention), reject malformed nesting, and preserve every
non-marker byte. Add quoted-string, suffix-comment, prefix-text, CRLF, duplicate
block, and no-final-newline cases.

### CR-02: The public verb's nonzero return still terminates `ERR_EXIT` shells

**Status:** Confirmed; prior fix guidance was incomplete  
**File:** `core/shell/zsh/hook.go:84-120`

**Issue:** A failed helper eventually makes `activate`, `checkout`, or
`deactivate` return nonzero. When a user invokes that verb as a simple command
with `ERR_EXIT` enabled, zsh exits before the next prompt command runs.

The earlier review focused on wrapping `zsh -n` and `eval` in `if` statements.
That is necessary so internal cleanup/reporting runs when helpers are invoked in
other contexts, but it is not sufficient: after the handler runs, the public
function still returns nonzero, which itself triggers `ERR_EXIT`. The independent
probe printed the validation diagnostic but never executed the following
`SURVIVED` command.

**Fix:** Decide and test the interactive contract explicitly. To satisfy the
phase's absolute fail-open promise, expected switch failures must be consumed by
the public interactive verb (diagnostic plus a zero return, with failure exposed
through a separate status variable/API), or the specification must be narrowed
to require conditional invocation. Also put emitter, staging, validator, eval,
cleanup, and history-stack operations in conditional/`always` forms so
`ERR_EXIT` and `ERR_RETURN` cannot skip handlers. Add direct-call tests with both
options enabled; checking only `activate bad || ...` masks the defect.

### CR-03: Emitter and validator subprocesses have no deadline

**Status:** Confirmed  
**Files:** `core/shell/zsh/hook.go:43-50`, `core/shell/zsh/hook.go:60-75`,
`core/cli/install.go:199-208`

**Issue:** `_zp_emit` synchronously waits for `zsh-pro emit`, and
`_zp_eval_block` synchronously waits for `zsh -n`. Neither has a deadline. The
install-time `validateZsh` path also uses `exec.Command` rather than
`exec.CommandContext`. A sleeping replacement for either runtime command
outlived a one-second budget and was terminated only by the outer watchdog.

This contradicts the explicit BOOT-02 requirement that a slow binary or
validator cannot block reaching a working prompt. It also diverges from the
repository's established five-second `context.WithTimeout` patterns in
`introspect.go`, `git.go`, and `keychain.go`.

**Fix:** Give install-time validation a Go context deadline. For sourced verbs,
choose one supported-platform watchdog design for both emission and validation;
do not assume GNU `timeout` exists on every supported host. Timeouts must take
the same no-export/no-last-good/cleanup path as other handled failures. Test both
commands with deterministic sleepers and assert bounded wall time.

### CR-04: `activate` does not perform a complete profile-to-profile transition

**Status:** Confirmed  
**Files:** `core/shell/zsh/hook.go:84-92`, `core/cli/emitter.go:69-83`,
`core/shell/zsh/emit.go:140-161`

**Issue:** `activate B` requests only `emit apply B` and calls only `zp_apply`.
`runtimeEmitter` correctly asks `activate.Diff(A, B)` for both halves, but then
returns only `apply` and discards the emitted deactivate half. State owned only
by A is therefore not represented in B's eventual deactivate plan and survives
the full `A -> B -> deactivate` sequence.

The current live test hides the bug: its generic deactivate shim removes all
test state, while the production emitter deactivates only identities owned by
the named profile.

**Fix:** Remove the duplicate transition semantics by making `activate` use the
same one-block switch path as `checkout`, or add an explicit switch emission
mode that returns a complete runnable transition. Add a real Store +
`runtimeEmitter` integration test in which A owns an alias/function/env entry
absent from B, then assert absence immediately after B and after final
deactivation.

### CR-05: Typed-nil interfaces bypass dependency checks and panic

**Status:** Confirmed  
**Files:** `core/cli/cli.go:33-35`, `core/cli/cli.go:67-101`,
`core/cli/emitter.go:36-40`

**Issue:** The guards compare interfaces directly to nil. An interface
containing `(*store.Store)(nil)` is non-nil, so `runList` and `runStatus` call a
method through a nil receiver. `NewRuntimeEmitter` accepts the same value and
constructs a broken adapter. A typed-nil pointer satisfying `cli.Emitter` also
bypasses `c.emitter == nil`.

Main currently converts one specific store-construction failure to a literal nil
interface, but the public constructors do not enforce the invariant they rely
on. Both store and emitter probes panic.

**Fix:** Reject nil-like injected interfaces at construction, including pointer,
map, slice, function, channel, and interface kinds where applicable, or replace
the loose constructor with explicit validated dependency wrappers. Apply the
same policy in `NewRuntimeEmitter`. Add typed-nil Store and Emitter tests; the
current literal-nil tests are insufficient.

### CR-06: Persisted `SecretRef` entries are silently omitted during activation

**Status:** Confirmed; additional finding  
**Files:** `core/store/secret.go:158-188`, `core/activate/builder.go:35-48`,
`core/activate/builder.go:222-242`, `core/cli/emitter.go:28-40`

**Issue:** Secret exclusion deliberately rewrites a captured literal to a
`SecretRef`, clears `RuntimeValue`, and sets `ValueModeUnsupported`.
`activate.Build` calls `activationValue`, which returns `ok=false` for
unsupported values, so the entry is skipped. `runtimeEmitter` has no secret
resolver dependency and cannot turn the reference back into activation data.

The result is not merely deferred end-to-end ingestion: the Phase 5
`OQ-05-02` contract says this phase wires and fixture-tests the runtime
resolution seam while Phase 6 owns the real `.zshrc` round trip. That seam is
absent. A persisted-secret-shaped fixture produced a manifest with zero
environment entries.

**Fix:** Inject a narrow runtime `SecretResolver` at the composition root,
resolve supported `SecretRef` values before manifest construction, and fail the
whole emission if resolution fails. Add a Phase 5 fixture test covering
`SecretRef -> resolver -> emitted assignment -> live value`, while leaving the
real ingest round-trip to Phase 6 as planned.

### CR-07: Missing home configuration makes `install` mutate the working directory

**Status:** Confirmed; additional finding  
**File:** `core/cli/install.go:65-77`

**Issue:** `runtimeDir` joins an empty `HOME` with `.zsh-pro`, and `zshrcPath`
joins an empty base with `.zshrc`. With `HOME`, `ZDOTDIR`, and `ZSHPRO_HOME`
unset, a successful install creates `.zshrc` and `.zsh-pro/loader.zsh` relative
to the current directory. A configuration installer must not silently reinterpret
an unknown home as the caller's repository or arbitrary working directory.

**Fix:** Resolve a non-empty home through one shared checked helper and fail
before any write when no stable absolute base is available. Validate or
canonicalize configured `ZSHPRO_HOME` / `ZDOTDIR` values so installer and startup
cannot disagree because of cwd-relative paths. Add unset, empty, and relative
environment cases that assert no filesystem mutation on failure.

## Warnings

### WR-01: Reinstall preserves an insecure cached-loader mode

**Status:** Confirmed  
**File:** `core/cli/install.go:152-168`

**Issue:** `runInstall` requests mode 0600, but `atomicWrite` preserves the mode
of every existing target. Reinstalling over a 0644 `loader.zsh` leaves it 0644.
The implementation therefore does not enforce its documented cache invariant.

**Fix:** Split the policies: preserve mode for `.zshrc`, enforce 0600 for the
cache. Add a reinstall-from-0644 case, not only a fresh-create assertion.

### WR-02: Validation occurs after the known-good cache has already been replaced

**Status:** Confirmed  
**File:** `core/cli/install.go:33-40`

**Issue:** `runInstall` atomically replaces `loader.zsh` and only then validates
the in-memory loader string. If validation fails, install returns an error but
the previous known-good cache is already gone. The independent probe began with
a recognizable old cache, forced validation failure, observed exit 1, and found
the cache had already been replaced.

**Fix:** Validate the complete candidate string before the rename, then install
the already-validated bytes atomically. If validation itself needs a staged
file, validate the same temporary file and rename that file only on success.
Test that a validation failure preserves the old cache byte-for-byte.

### WR-03: Emitted source staging is hand-rolled and not interruption-safe

**Status:** Confirmed by code path  
**File:** `core/shell/zsh/hook.go:54-80`

**Issue:** `_zp_eval_block` constructs a filename from `$RANDOM` and `$$`, opens
it with ordinary redirection, and removes it only on the normal validation path.
There is no exclusive-create guarantee and no `always`/signal cleanup. A name
collision or interruption can reuse or leave staged source behind. This is also
one more cleanup path that hostile shell options can skip.

**Fix:** Use a platform-supported exclusive temporary-file primitive, retain the
returned exact path, and centralize removal in an `always`/trap cleanup path.
Test pre-existing-name handling and interrupted validation without inspecting
real user temporary directories.

### WR-04: The optional performance harness is not robustly portable or JSON-safe

**Status:** Confirmed by source and fixture  
**File:** `scripts/perf-hyperfine.sh:25-27`

**Issue:** The script uses `mapfile`, which is unavailable in Bash 3.2, despite
the repository containing an explicit macOS runtime path. It also parses JSON
with a regex requiring no whitespace after `"mean":`; a valid fixture using
`"mean": 0.100` produced zero matches. Because `hyperfine` is absent locally,
the shipped skip path never exercises either failure.

**Fix:** Parse the JSON with a real JSON decoder (a small Go helper/test keeps the
repository dependency-free), or use a documented machine-readable field with a
portable reader. Remove `mapfile` or state and enforce the minimum Bash version.
Add a checked-in representative result fixture so parsing is always tested even
when `hyperfine` is unavailable.

## Refuted Prior Finding

### Existing WR-02: “The loader permanently disables xtrace”

**Status:** Refuted; do not carry forward

The direct zsh probe preserved `XTRACE`, matching `05-VERIFICATION.md`. The
warning in `05-REVIEW.md` should not be fixed as written. Any future change to
option scoping still needs a regression test, but the current code did not
reproduce permanent xtrace loss.

## Conditional Observations (Not Counted as Findings)

- `atomicWrite` renames at `core/cli/install.go:188` and then directory-syncs at
  lines 191-196. A directory-sync failure therefore reports an error after the
  visible target has changed. That can be an honest “not durably confirmed”
  result, but callers and tests must not interpret every returned error as “no
  mutation occurred.”
- Relative `ZSHPRO_HOME` and `ZDOTDIR` values are cwd-dependent. The unset-home
  case is confirmed as CR-07; the exact policy for explicitly relative values is
  a product decision and should be settled before implementing that fix.
- The startup budget remains unmeasured because `hyperfine` is absent. This is an
  unverified acceptance criterion, not evidence of a performance defect.

## Test Coverage Gaps

The green suite misses each production failure for a specific reason:

| Finding | Missing regression shape |
| --- | --- |
| CR-01 | markers embedded in quoted text or suffixed comments |
| CR-02 | direct public verb call under `ERR_EXIT` / `ERR_RETURN`, followed by a survival assertion |
| CR-03 | deterministic sleeping emitter and sleeping validator with a wall-time bound |
| CR-04 | real Store/emitter A-only and B-only ownership transition |
| CR-05 | typed-nil pointer values, not literal nil interfaces |
| CR-06 | persisted `SecretRef` fixture crossing the runtime resolver seam |
| CR-07 | all home/path environment bases unset or relative |
| WR-01 | existing 0644 loader before reinstall |
| WR-02 | old cache remains byte-identical after validation failure |
| WR-03 | exclusive staging collision and interruption cleanup |
| WR-04 | parser fixture independent of `hyperfine` availability and Bash 3.2 execution |

## Prioritized Complete Remediation

1. **Prevent destructive installer targeting first:** replace substring marker
   parsing (CR-01) and reject unresolved home/path bases before any write
   (CR-07).
2. **Make switching terminal-safe:** define the public `ERR_EXIT` failure
   contract (CR-02), add bounded emission/validation (CR-03), and centralize
   exclusive staging plus unconditional cleanup (WR-03).
3. **Make transitions semantically complete:** route `activate` through the same
   full transition as `checkout`, then prove it with the real emitter (CR-04).
4. **Harden dependency construction:** reject typed nils in both CLI and runtime
   emitter constructors (CR-05).
5. **Wire the promised runtime secret seam:** inject resolution before
   `activate.Build` and add the Phase 5 `SecretRef` fixture (CR-06).
6. **Finish installer cache invariants:** validate before replacement (WR-02)
   and enforce cache mode 0600 independently from `.zshrc` mode preservation
   (WR-01).
7. **Repair the verification backstop:** use portable shell constructs and a
   real JSON decoder for hyperfine output (WR-04), then run the actual timing
   gate on a host with `hyperfine`.
8. Re-run `go test ./...`, the new focused live-zsh tests, the installer
   destructive-boundary fixtures, and Phase 5 verification. Do not mark the
   phase complete merely because the pre-existing suite remains green.

---

_Reviewed: 2026-07-27T15:13:51Z_  
_Reviewer: GPT-5.6 Sol (independent deep review)_  
_Depth: deep_
