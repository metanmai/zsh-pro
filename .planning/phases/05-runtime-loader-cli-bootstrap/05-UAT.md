---
status: testing
phase: 05-runtime-loader-cli-bootstrap
source:
  - 05-01-SUMMARY.md
  - 05-02-SUMMARY.md
  - 05-03-SUMMARY.md
  - 05-04-SUMMARY.md
  - 05-05-SUMMARY.md
  - 05-06-SUMMARY.md
  - 05-07-SUMMARY.md
  - 05-08-SUMMARY.md
  - 05-VERIFICATION.md
  - 05-REVIEW.md
started: 2026-07-30T20:31:57Z
updated: 2026-07-30T20:31:57Z
handoff_to: next-agent
test_count: 21
---

# Phase 5 End-to-End UAT Handoff

This is an execution handoff, not a record of prior success. Existing automated
evidence explains why each case exists, but every case below starts pending and
must be rerun against the current commit.

## Current Test

number: 1
name: Cold install and sourced-shell lifecycle
expected: |
  A freshly built binary installs entirely inside a disposable HOME/ZDOTDIR,
  defines all five sourced verbs, lists the main profile, activates and
  deactivates without error, and creates only the expected private artifacts.
awaiting: next-agent execution

## Next-Agent Operating Instructions

1. Read this file, `05-VERIFICATION.md`, and `05-REVIEW.md` before testing.
2. Record the starting commit, OS/architecture, Go version, zsh version, Git
   version, and Hyperfine version in `## Execution Environment`.
3. Use a new disposable root. Never point `HOME`, `ZDOTDIR`, `ZSHPRO_HOME`,
   `XDG_DATA_HOME`, `TMPDIR`, or the file vault at the operator's real home.
4. Build from source once at the start, then rebuild when a test explicitly
   checks a cold binary or cross-platform result.
5. Run cases in order. Later cases assume the baseline toolchain and native zsh
   checks from earlier cases work.
6. Capture command, exit status, stdout, stderr, elapsed time where relevant,
   artifact modes, and before/after hashes or tree manifests.
7. Do not use `set -x` in secret-bearing cases. Diagnostics and evidence must
   never contain the resolved fixture secret.
8. A prior passing Go test is not permission to mark a case passed without
   rerunning it. Use the named regression as the reproducible complex/edge-case
   driver and add the stated product-shaped observation.
9. On failure, preserve the disposable fixture, mark `result: issue`, quote the
   observed behavior verbatim, infer severity, and add a stable gap entry
   `G-05-{test-number}` under `## Gaps`.
10. Do not alter product source merely to make a test easier. Diagnose only
    after preserving evidence.

## Shared Setup

Run from `/home/metanmai/Code/zsh-pro`:

```bash
export E2E_REPO=/home/metanmai/Code/zsh-pro
export E2E_RUN_ROOT="$(mktemp -d)"
mkdir -p "$E2E_RUN_ROOT/bin" "$E2E_RUN_ROOT/evidence"
export E2E_BIN="$E2E_RUN_ROOT/bin/zsh-pro"

cd "$E2E_REPO"
git rev-parse HEAD | tee "$E2E_RUN_ROOT/evidence/commit.txt"
go version | tee "$E2E_RUN_ROOT/evidence/go-version.txt"
zsh --version | tee "$E2E_RUN_ROOT/evidence/zsh-version.txt"
git --version | tee "$E2E_RUN_ROOT/evidence/git-version.txt"
GOTOOLCHAIN=auto go build -o "$E2E_BIN" ./core/cmd/zsh-pro
```

Keep `E2E_RUN_ROOT` until the handoff report is complete. Cleanup is optional
only after confirming it is a disposable `mktemp` directory and no failure
evidence is needed.

## Execution Environment

commit: [record]
os_arch: [record]
go: [record]
zsh: [record]
git: [record]
hyperfine: [record or "locally staged"]
run_root: [record]

## Tests

### 1. Cold install and sourced-shell lifecycle

id: E2E-05-01
priority: P0
type: built-binary + native-zsh
risk_covered: A green build that does not produce a usable installed shell.

preconditions:

- Shared setup completed.
- No fixture state exists for this case.

instructions:

1. Create `"$E2E_RUN_ROOT/01/home"` and use it for both `HOME` and `ZDOTDIR`.
2. Run the built binary's `install` command.
3. Start `zsh -f`, explicitly source the fixture `.zshrc`, and run
   `whence -w checkout activate deactivate list status`.
4. Run `list`, `status`, `activate main`, `status`, `deactivate`, and `status`.
5. Source `.zshrc` a second time and confirm functions remain usable.
6. Record the fixture tree and modes of the store, cache directory, loader, and
   `.zshrc`.

commands: |

```bash
case_root="$E2E_RUN_ROOT/01"
mkdir -p "$case_root/home"
HOME="$case_root/home" ZDOTDIR="$case_root/home" PATH="$E2E_RUN_ROOT/bin:$PATH" \
  "$E2E_BIN" install
HOME="$case_root/home" ZDOTDIR="$case_root/home" PATH="$E2E_RUN_ROOT/bin:$PATH" \
  zsh -f -c '
    source "$ZDOTDIR/.zshrc"
    whence -w checkout activate deactivate list status
    list
    status
    activate main
    status
    deactivate
    status
    source "$ZDOTDIR/.zshrc"
    print UAT_SURVIVED
  '
```

complex_cases:

- Separate command installation and sourced-shell execution.
- Re-source the installed bootstrap in the same shell.
- Exercise all five public verbs through the installed loader.

edge_cases:

- The fresh main profile contains little or no visible environment delta.
- `status` may report `main` both before activation and after deactivation;
  function availability and clean completion are the key baseline signals.

expected: |
  Install exits 0 and prints `zsh-pro: installed`. All five names report
  `function`; `list` contains exactly `main`; every lifecycle command returns;
  `UAT_SURVIVED` prints. The cache directory and store root are mode 0700 and
  the cached loader is mode 0600. No path outside the fixture changes.

evidence_to_capture:

- Full stdout/stderr and exit status.
- `find`/`stat` listing rooted at the fixture.
- Hash of `.zshrc` and cached loader after the second source.

result: [pending]

### 2. Store-root precedence and path validation matrix

id: E2E-05-02
priority: P0
type: built-binary matrix + integration regression
risk_covered: Ordinary CLI and sourced runtime select different repositories or accept unstable paths.

preconditions:

- Case 1 passed.

instructions:

1. Exercise three isolated configurations: HOME fallback, absolute
   `XDG_DATA_HOME`, and explicit absolute `ZSHPRO_HOME`.
2. Use a separate absolute `ZDOTDIR` in at least one configuration.
3. For each route, install, run ordinary `list`, source the loader, run sourced
   `list`, and compare outputs.
4. Run the named StoreRoot and composition-root regressions.
5. Try relative `HOME`, `XDG_DATA_HOME`, `ZSHPRO_HOME`, and `ZDOTDIR` values in
   fresh fixtures and verify rejection happens before mutation.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli \
  -run '^TestStoreRootUsesDocumentedPrecedenceAndRejectsUnsafeInputs$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/cmd/zsh-pro \
  -run '^TestRuntimeCaptureUsesCompositionStoreAndVaultForEveryLocation$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli \
  -run '^TestInstallRejectsUnstablePathInputsBeforeMutation$'
```

complex_cases:

- Cache and store are distinct under default HOME/XDG routes.
- Explicit `ZSHPRO_HOME` intentionally joins store and cache roots.
- `ZDOTDIR` may differ from HOME without changing store precedence.

edge_cases:

- Empty-but-set environment variables.
- Relative paths, a missing HOME, paths containing spaces, and pre-existing
  parent directories.

expected: |
  Ordinary and sourced listing/emission use the same profile store for every
  supported route. Precedence is explicit ZSHPRO_HOME, then XDG data home,
  then HOME/.local/share. Invalid or relative roots return a nonzero,
  zsh-pro-phrased error before creating or changing store, cache, or .zshrc.

evidence_to_capture:

- Per-route environment and resolved paths.
- Ordinary versus sourced `list` output.
- Before/after fixture tree for every invalid-path case.

result: [pending]

### 3. Installer idempotence, exact markers, and byte preservation

id: E2E-05-03
priority: P0
type: built-binary + byte-level integration
risk_covered: Reinstall duplicates or damages user-owned zsh configuration.

preconditions:

- Shared setup completed.

instructions:

1. Seed `.zshrc` with binary-safe unmanaged bytes before, between, and after
   marker-like text that is not an exact physical marker.
2. Install three times and compare complete file hashes after each run.
3. Verify exactly one managed BEGIN/END block remains.
4. Exercise balanced duplicate blocks and confirm they collapse while
   intervening unmanaged bytes survive.
5. Exercise unbalanced, reversed, nested, duplicate-BEGIN, duplicate-END,
   LF, and CRLF exact marker arrangements.
6. Run the named byte-preservation regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestInstallIdempotentPreservesUserContent|TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent|TestInstallCreatesAndRefusesUnbalancedMarkersWithoutWriting|TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting)$'
```

complex_cases:

- Balanced duplicate managed blocks with user content between them.
- Exact markers using both LF and CRLF physical lines.
- Marker text embedded inside comments, quotes, or longer lines.

edge_cases:

- Empty `.zshrc`, no trailing newline, non-default existing mode, and
  zero-length unmanaged regions.

expected: |
  Reinstall is byte-identical and never duplicates the managed block.
  Non-exact marker-like text is unmanaged. Valid duplicate blocks collapse
  without losing surrounding bytes. Every malformed exact ordering fails
  before store/cache/bootstrap mutation and preserves original bytes exactly.

evidence_to_capture:

- SHA-256 hashes and modes before/after each install.
- Marker counts and byte-level diff.
- Exit status and error for each malformed arrangement.

result: [pending]

### 4. Transactional install rollback across every failure stage

id: E2E-05-04
priority: P0
type: real built-binary failure matrix
risk_covered: Failed install reports an error but leaves a partial store, cache, or bootstrap.

preconditions:

- Shared setup completed.

instructions:

1. Run the built-binary failure fixtures for loader validation, runtime
   directory setup, staging, loader promotion, and `.zshrc` promotion.
2. Cover HOME fallback, XDG, and explicit `ZSHPRO_HOME`.
3. Cover a fresh store and a pre-existing 0755 user-owned bare store that
   requires privacy-mode migration.
4. Compare the complete pre/post tree including file contents, modes, and
   repository identity.
5. Confirm retries work after removing the injected failure.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cmd/zsh-pro -run \
  '^(TestBuiltBinaryInstallRollsBackFreshStoreAfterBootstrapFailures|TestBuiltBinaryInstallStagingFailureRollsBackStoreState|TestBuiltBinaryInstallRollsBackStoreOnPromotionFailures)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/store -run \
  '^TestInitForInstallRollsBackOnlyCreatedStoreState$'
```

complex_cases:

- Explicit root is shared by store and cache.
- A pre-existing bare repository is never deleted or rewound.
- A created parent is removed only when it remains owned and unchanged.

edge_cases:

- Failure after cache promotion but before `.zshrc` promotion.
- Replaced fixture path during rollback.
- Restore of a migrated mode without changing repository data.

expected: |
  Every injected failure exits nonzero and restores exact pre-install state.
  Fresh invocation-owned state is removed only when unchanged; pre-existing
  repositories remain byte-for-byte intact and regain their original mode.
  A subsequent normal install succeeds.

evidence_to_capture:

- Failure-stage matrix with exit statuses.
- Before/after tree manifests, modes, and Git refs.
- Retry result.

result: [pending]

### 5. Cache no-follow policy and `.zshrc` symlink compatibility

id: E2E-05-05
priority: P0
type: built-binary filesystem boundary
risk_covered: Generated cache overwrites unrelated files or strict cache handling breaks supported `.zshrc` symlinks.

preconditions:

- Shared setup completed.

instructions:

1. Point the cache root at a symlink to an unrelated mode-0755 directory.
2. Point `loader.zsh` at a symlink to an unrelated mode-0644 file.
3. Exercise symlinked and non-directory cache ancestors.
4. Confirm install fails without changing symlink, target bytes, target mode,
   store, or `.zshrc`.
5. Separately configure `.zshrc` as a user-owned symlink and verify install
   updates its intended target while preserving the symlink and target mode.
6. Run the direct and built-binary regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestInstallRejectsSymlinkedCachePathsWithoutMutation|TestCacheRollbackRefusesToRemoveAReplacedDirectory|TestInstallPreservesSymlinkAndModesAndWritesSecureCacheFirst)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/cmd/zsh-pro -run \
  '^TestBuiltBinaryInstallRejectsSymlinkedCacheTargetsWithoutMutation$'
```

complex_cases:

- Default cache, XDG-backed store, and explicit shared root.
- Intentional difference between generated cache and user-selected `.zshrc`.
- Directory replacement after validation but before rollback.

edge_cases:

- Dangling symlink, symlink loop, regular file where a directory is expected,
  directory where loader file is expected, and mode-repair attempt.

expected: |
  Cache traversal never follows a symlink or mutates an unrelated target.
  Unsafe cache paths fail before other install mutations. The supported
  `.zshrc` symlink remains a symlink, its intended target receives one managed
  block, and unrelated modes/bytes remain unchanged.

evidence_to_capture:

- Symlink targets and inode/device data before/after.
- Target hashes and modes.
- Fixture tree and installer diagnostic.

result: [pending]

### 6. Top-level CLI argument contract and mutation-free usage errors

id: E2E-05-06
priority: P0
type: built-binary command boundary
risk_covered: A typo such as `install --help` performs a real installation.

preconditions:

- Shared setup completed.

instructions:

1. Against both fresh and pre-populated HOME fixtures, invoke
   `install --help`, `install extra`, `hook extra`, `list extra`,
   `status extra`, `--version extra`, and `-v extra`.
2. Record exit status and diagnostics.
3. Compare the complete fixture tree byte-for-byte before and after.
4. Confirm valid no-extra forms still work.
5. Run the named built-binary and dependency-order regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestNoArgumentCommandsRejectTrailingArgumentsBeforeDependencies|TestBuiltBinaryInstallRejectsTrailingArgumentsWithoutMutation)$'
```

complex_cases:

- Pre-existing valid cache/store/`.zshrc` must remain unchanged.
- Usage validation must precede unavailable or typed-nil dependencies.

edge_cases:

- Empty-string argument, multiple extras, `--`, and an argument resembling a
  path or profile name.

expected: |
  Every invalid no-argument command exits 2 with command-specific usage.
  No dependency resolution or filesystem mutation occurs. Valid forms retain
  their existing behavior.

evidence_to_capture:

- Command/exit/output matrix.
- Before/after tree hashes for fresh and pre-populated fixtures.

result: [pending]

### 7. Bootstrap fail-open behavior for disabled, missing, unreadable, and corrupt loader states

id: E2E-05-07
priority: P0
type: installed native-zsh matrix
risk_covered: A broken installation prevents an interactive shell from starting.

preconditions:

- A disposable installed fixture exists.

instructions:

1. Source the installed `.zshrc` normally and confirm verbs exist.
2. Source with `ZSHPRO_DISABLE=1`; confirm no public verbs are defined and no
   activation occurs.
3. Repeat with the binary removed from PATH, loader missing, loader unreadable,
   loader a directory, and loader containing invalid zsh.
4. Run each state under ordinary zsh, `ERR_EXIT`, and `ERR_RETURN`.
5. Print a sentinel command immediately after source.
6. Run the installed-stub regression.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders$'
```

complex_cases:

- Direct source from a noninteractive `zsh -f` command stream.
- Corrupt loader under both hostile error options.
- Disable flag set before `.zshrc` source.

edge_cases:

- Empty loader, unreadable regular loader, directory at loader path, and
  previously defined user function with a public-verb name.

expected: |
  Every degraded state reaches the sentinel command and does not terminate the
  shell. `ZSHPRO_DISABLE=1` defines no verbs. Missing/unsafe loader states are
  silent or emit only the documented diagnostic, never partial activation.

evidence_to_capture:

- Option/state matrix and sentinel output.
- Function availability before/after source.
- Exit status and diagnostics.

result: [pending]

### 8. Public sourced-verb arity under restrictive shell options

id: E2E-05-08
priority: P0
type: native-zsh argument matrix
risk_covered: Missing or surplus arguments abort the shell or mutate active state.

preconditions:

- Hook can be sourced from the built binary.

instructions:

1. Test missing, explicitly empty, valid, and surplus arguments for
   `activate` and `checkout`.
2. Test valid zero-argument and surplus forms of `deactivate`, `list`, and
   `status`.
3. Repeat under ordinary options, `NO_UNSET`, `ERR_EXIT`, `ERR_RETURN`, and the
   combined restrictive set.
4. Seed active markers, reverse capability, a fixture secret, `REPLY`, and
   timeout state before invalid calls; verify all remain unchanged.
5. Place a sentinel command after every call.
6. Run the complete native matrix.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalNoArgumentVerbsFailOpenUnderNoUnset|TestLiveTerminalPublicVerbArityFailsOpenBeforeRuntimeWork)$'
```

complex_cases:

- Invalid call while a real profile is active.
- Combined `NO_UNSET ERR_EXIT ERR_RETURN`.
- Explicit empty argument is distinct from missing and valid non-empty input.

edge_cases:

- Newline, whitespace-only, glob-looking, and option-looking arguments.
- User-defined `REPLY` and pre-existing public function names.

expected: |
  Invalid calls print usage, set `ZP_LAST_RUNTIME_STATUS=2`, return safely, and
  reach the sentinel. They do not capture runtime output, reverse a profile,
  emit source, or change markers, secrets, REPLY, timeout, PATH, or last-good.

evidence_to_capture:

- All option/verb/arity combinations with exit/status/diagnostic.
- Before/after state snapshots.

result: [pending]

### 9. Bounded `list` and truthful `status`

id: E2E-05-09
priority: P0
type: native-zsh + child-process failure matrix
risk_covered: Read-only verbs bypass fail-open boundaries or print partial output.

preconditions:

- Disposable store contains at least main and one additional test branch where
  the named regression constructs it.

instructions:

1. Verify ordinary and sourced `list` return the same sorted branch set.
2. Verify `status` before activation, while active, and after deactivation.
3. Replace the invoked binary with shims that are missing, exit nonzero, emit
   partial output then fail, and sleep beyond the deadline.
4. Repeat direct calls under `ERR_EXIT` and `ERR_RETURN`.
5. Confirm successful output appears only after successful capture and failure
   status/diagnostic remain available.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^TestLiveTerminalListUsesBoundedFailOpenBoundary$'
```

complex_cases:

- Child emits valid-looking partial branch data before failing.
- Missing binary after the loader has already been sourced.
- Active marker differs from inherited `ZSHPRO_PROFILE`.

edge_cases:

- Empty branch list fixture, long branch name, and sleeping child.

expected: |
  `list` is bounded and fail-open; partial/failed output is suppressed.
  `status` truthfully reports the terminal-owned profile lifecycle. Direct
  hostile-option callers survive and retain a nonzero runtime status/error
  without a nonzero public return.

evidence_to_capture:

- Ordinary/sourced output comparison.
- Duration and status for each failure mode.
- Post-timeout child-process check.

result: [pending]

### 10. Complete A-to-B transition and zero-residue deactivation

id: E2E-05-10
priority: P0
type: real Store + real emitter + native-zsh
risk_covered: Profile switching layers state or fails to restore exact shell state.

preconditions:

- Native zsh available.
- Named integration fixture creates profiles A and B.

instructions:

1. Snapshot aliases, functions, scalar values and export presence, options,
   PATH/path, FPATH/fpath, and relevant loader globals.
2. Activate A; verify all A-owned categories apply.
3. Activate B; verify A-only state disappears before B state applies.
4. Deactivate; compare the complete snapshot byte-for-byte.
5. Repeat using `checkout B` instead of `activate B`.
6. Repeat same-profile activation and explicit `main` transition.
7. Run both real-emitter transition regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestRuntimeEmitterLiveTransitionRemovesAOnlyState|TestRuntimeEmitterLiveCheckoutTransitionRemovesAOnlyState)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^TestLiveTerminalLoaderSwitchesCurrentShellWithoutResidue$'
```

complex_cases:

- A and B overlap some names and uniquely own others.
- Exported/unexported scalars, multiline functions, aliases, options, PATH and
  FPATH all participate in one transition.
- User changes an unrelated value while A is active.

edge_cases:

- Empty function body, duplicate PATH entry, `typeset -U`, empty scalar,
  pre-existing unset state, and same-profile reactivation.

expected: |
  A is reversed before B applies. No A-only identity remains under B.
  Deactivation restores the complete pre-A state exactly, without duplicate
  PATH/FPATH entries or loader bookkeeping residue.

evidence_to_capture:

- Encoded before/A/B/after snapshots.
- Per-category diff and command status.

result: [pending]

### 11. Inherited profile metadata versus terminal-owned activation

id: E2E-05-11
priority: P1
type: native-zsh lifecycle
risk_covered: An inherited environment variable is mistaken for a live reversible activation.

preconditions:

- Hook source available.

instructions:

1. Export `ZSHPRO_PROFILE=A` before sourcing the loader, without creating the
   private active marker.
2. Run `status` and `deactivate`; confirm no binary call or reverse occurs.
3. Activate a real profile and verify the non-exported active marker appears
   only in the current shell.
4. Spawn a child zsh and confirm it may inherit public profile metadata but not
   the terminal-owned active marker/reverse capability.
5. Run the activation-marker regression.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^TestLiveTerminalActivationMarkerDistinguishesInheritedProfile$'
```

complex_cases:

- Parent has inherited metadata, then performs a real activation.
- Child shell starts while parent profile is active.

edge_cases:

- Empty `ZSHPRO_PROFILE`, explicit `main`, and marker name pre-existing as a
  user export.

expected: |
  Inherited metadata never grants reversal authority. Marker-absent
  deactivation is a safe no-op. Only successful local activation owns and
  consumes a retained reverse; the private marker is not exported to children.

evidence_to_capture:

- Parent/child parameter export flags and values.
- Proof that marker-absent deactivate did not invoke the binary.

result: [pending]

### 12. Target preflight failures preserve the active profile

id: E2E-05-12
priority: P0
type: native-zsh failure matrix
risk_covered: Switching reverses active A before target B is known to be usable.

preconditions:

- Profile A is successfully active with observable state.

instructions:

1. Attempt B with an emitter error.
2. Attempt B with empty output.
3. Attempt B with syntactically invalid output.
4. Attempt B with capture/transport failure.
5. Attempt B with runtime-root/staging failure.
6. After each attempt, verify A's env, alias, function, option, PATH, markers,
   retained reverse, and last-good state are unchanged.
7. Verify a subsequent valid switch still succeeds.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalTargetPreflightFailuresPreserveActiveProfile|TestLiveTerminalTargetStagingFailurePreservesActiveProfile)$'
```

complex_cases:

- Failures occur at distinct preflight stages before any active reversal.
- Direct callers use `ERR_EXIT` and `ERR_RETURN`.

edge_cases:

- B output contains whitespace only.
- Validator emits its own diagnostic.
- Transport fails after A has accumulated user drift.

expected: |
  Every target preflight failure is consumed at the public boundary. A remains
  fully active and reversible, B applies nothing, no target reverse/helper
  leaks, and a later valid switch behaves normally.

evidence_to_capture:

- State snapshot after each failure.
- Runtime status/error and proof of no leaked target functions.

result: [pending]

### 13. Partial apply, failed compensation, retained recovery, and retry

id: E2E-05-13
priority: P0
type: adversarial native-zsh recovery
risk_covered: Partial target state becomes orphaned after apply or reverse failure.

preconditions:

- Native-zsh recovery regressions available.

instructions:

1. Trigger a target apply that mutates state and then fails.
2. Cover compensation success and compensation failure.
3. When compensation fails, verify the dedicated recovery pointer, markers,
   undo slots, and reverse function remain reachable.
4. Attempt another activation and verify it is blocked before target emission.
5. Repair the injected failure and call `deactivate` to retry recovery.
6. Mark a profile-owned scalar readonly and exercise reversal under ordinary,
   `ERR_EXIT`, and `ERR_RETURN`.
7. Trigger a mid-reverse multi-operation failure and verify cleanup is deferred.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalFailedEvalLeavesTruthfulInactiveStateAndRecovers|TestLiveTerminalReadonlyRetainedReverseFailsOpenAndRetries|TestLiveTerminalFailedTargetRecoveryBlocksSwitchAndRetries|TestLiveTerminalPartialRetainedReverseKeepsRecoveryState)$'
```

complex_cases:

- Target apply and its compensating reverse both fail.
- Reverse fails after some operations have restored.
- Retry occurs after removing readonly/protected state.

edge_cases:

- Direct public call under combined hostile options.
- Secret-bearing target reverse and obsolete helper scan.

expected: |
  No failure terminates the caller. Successful compensation leaves truthful
  inactive state. Failed compensation retains all recovery authority and
  blocks layering another profile. Recovery is consumed and scrubbed only
  after a successful retry.

evidence_to_capture:

- Apply/compensation/retry status timeline.
- Marker, pointer, undo-slot, function, and user-state snapshots.

result: [pending]

### 14. Runtime deadlines, child reaping, and cleanup

id: E2E-05-14
priority: P0
type: native-zsh timing/failure containment
risk_covered: Slow emitter, validator, or list command hangs the interactive shell or leaks children.

preconditions:

- Native zsh and process inspection available.

instructions:

1. Use sleeping emitter, validator, and list shims that exceed the configured
   runtime timeout.
2. Test minimum, normal, maximum, invalid, and absent timeout environment
   values.
3. Invoke public verbs directly under `ERR_EXIT` and `ERR_RETURN`.
4. Measure wall-clock duration and inspect for remaining child/watchdog
   processes.
5. Inspect runtime directories and descriptors for cleanup.
6. Run the bounded-runtime regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalTimeoutsAreBoundedAndCleanedUp|TestLiveTerminalConsumesAllExpectedRuntimeFailures|TestLiveTerminalListUsesBoundedFailOpenBoundary)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^TestRuntimeCaptureBoundsEmitter$'
```

complex_cases:

- Child and watchdog exit in different orders.
- Output arrives before the child later hangs.
- Timeout occurs while another profile remains active.

edge_cases:

- Timeout values 0, negative, nonnumeric, 1, and 99.
- Child ignores a normal termination signal.

expected: |
  Each operation returns within its documented bound, sets visible runtime
  status/error, and returns safely from the public verb. No partial output,
  child, watchdog, temporary artifact, or state transition remains.

evidence_to_capture:

- Per-case elapsed time.
- Process list before/after and runtime-root tree.
- Status/error and active-state snapshot.

result: [pending]

### 15. Exact environment presence, export state, drift, and readonly recovery

id: E2E-05-15
priority: P0
type: native-zsh state fidelity
risk_covered: Unset, empty, literal sentinel-like, or drifted values restore incorrectly.

preconditions:

- Loader and real emitted profile fixture available.

instructions:

1. Cover a variable originally unset, set empty, set to the retired
   sentinel-looking literal, exported, and unexported.
2. Activate a profile that changes each variable.
3. Change one value manually while active to create user drift.
4. Deactivate and inspect value presence and export flags.
5. Repeat with a readonly applied scalar, release readonly state, and retry.
6. Run the presence and reverse regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalEnvRestorePreservesPresenceAndLegacyMarkerData|TestLiveTerminalReadonlyRetainedReverseFailsOpenAndRetries)$'
```

complex_cases:

- Presence metadata is separate from arbitrary user data.
- User drift should not be confused with the exact applied value.
- Readonly failure retains recovery metadata.

edge_cases:

- Empty string versus unset.
- Newline/backslash/metacharacter content.
- Export flag changes without value changes.

expected: |
  Original presence, value, and export state restore exactly where reversal is
  safe. Literal sentinel-looking data remains ordinary data. A readonly failure
  is fail-open and retryable; undo metadata is not consumed prematurely.

evidence_to_capture:

- `${(P)+name}`, quoted value, and export-flag snapshots at each stage.
- Retry state before/after readonly release.

result: [pending]

### 16. Secret resolution, redaction, lifetime, and resolver-loss reversal

id: E2E-05-16
priority: P0
type: real Store + file vault + native-zsh
risk_covered: Secrets leak to persisted profiles, diagnostics, globals, helpers, or become impossible to reverse.

preconditions:

- Use only a disposable file-vault fixture.
- Choose a unique secret token and never enable shell xtrace.

instructions:

1. Persist a profile containing a `SecretRef`; verify stored profile data stays
   redacted.
2. Activate through the real resolver and verify the live assignment receives
   the fixture value.
3. Scan stdout/stderr, global parameters, `REPLY`, generated functions, and
   runtime paths for the token.
4. Remove or disable the resolver and binary, then deactivate using the
   retained reverse.
5. Exercise missing key, backend-kind mismatch, nil resolver, resolver error,
   failed switch, and partial-evaluation recovery.
6. Run the secret lifecycle regressions.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestRuntimeEmitterResolvesSecretRefBeforeBuild|TestRuntimeEmitterSecretResolverFailuresEmitNothing|TestRuntimeEmitterSwitchDoesNotResolveAnActiveSecretAgain|TestRuntimeEmitterPreservesUserFunctionsAndScrubsResolvedSecrets|TestRuntimeEmitterFailedSwitchScrubsResolvedSecretPayload)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalRetainedSecretReverseSurvivesUnavailableBinary|TestLiveTerminalRuntimeTransportScrubsResolvedSource|TestLiveTerminalFailedTargetRecoveryBlocksSwitchAndRetries)$'
```

complex_cases:

- Resolver disappears after successful activation.
- Apply fails after resolving a secret and compensation also fails.
- Dynamic value needs reversal state while static secret copies must disappear.

edge_cases:

- Empty secret, multiline secret, shell metacharacters, missing key, wrong
  backend kind, and typed-nil resolver.

expected: |
  Persistent profile remains redacted. Resolution happens only in an activation
  copy. The secret reaches only the intended live state and necessary retained
  recovery capability. It never appears in diagnostics, REPLY, avoidable
  globals, obsolete functions, or shared files. Deactivation/retry works after
  resolver and binary loss and removes the secret.

evidence_to_capture:

- Redacted stored profile excerpt.
- Token scan counts without printing the token itself.
- Resolver-loss deactivation state and function/global inventory.

result: [pending]

### 17. Generated-function collision resistance and user-function preservation

id: E2E-05-17
priority: P1
type: native-zsh collision matrix
risk_covered: Runtime helpers overwrite user functions or leave executable source behind.

preconditions:

- Hook source and real runtime emitter available.

instructions:

1. Define user functions using historical/generic helper names before sourcing
   and activation.
2. Exercise successful activation, failed activation, switch, recovery, and
   deactivation.
3. Verify every user function body remains byte-identical.
4. Inventory generated functions and retained reverse pointers after each
   boundary.
5. Run collision and secret-scrubbing regressions repeatedly.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=10 -v ./core/cli -run \
  '^(TestRuntimePayloadCollisionLeavesPreexistingFunctionUntouched|TestRuntimeEmitterPreservesUserFunctionsAndScrubsResolvedSecrets)$'
```

complex_cases:

- Pre-existing function matches a historical generated helper name.
- Failed target leaves a retryable reverse but no unreferenced helper.
- Repeated runs exercise random capability names.

edge_cases:

- Empty and multiline user functions.
- User function names that are valid but visually similar to generated names.

expected: |
  User functions are never replaced or removed. Generated apply helpers are
  consumed; only a deliberately retained reverse/recovery capability remains
  while active or recovering, and it is removed after successful cleanup.

evidence_to_capture:

- Function body hashes before/after.
- Generated-function inventory and retained pointer at each stage.

result: [pending]

### 18. Descriptor-bound runtime root and shared-directory replacement resistance

id: E2E-05-18
priority: P0
type: reproducible filesystem-race integration
risk_covered: Runtime capture reopens mutable paths or stages source in a shared directory.

preconditions:

- Run only inside disposable fixture directories.
- Do not use or inspect another user's data.

instructions:

1. Verify runtime capture uses a private pipe and validated absolute store root.
2. Exercise unsafe modes, symlinked final component, symlinked ancestor,
   non-sticky writable ancestor, and owner mismatch where the fixture supports
   it.
3. Set `TMPDIR` to a mode-0777 shared fixture and verify runtime transport does
   not create source there.
4. Run the deterministic post-validation symlink replacement fixture using the
   real Store, emitter, and file vault.
5. Confirm captured profile and vault data stay bound to the validated
   descriptor object.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/cli -run \
  '^(TestRuntimeCaptureUsesPrivatePipeAndRejectsUnsafeRoots|TestRuntimeCaptureBindsProfileAndVaultToValidatedDescriptors)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^(TestLiveTerminalRuntimeTransportDoesNotTouchTMPDIR|TestLiveTerminalRuntimeTransportIgnoresSharedTMPDIRRace|TestLiveTerminalRuntimeHelperRejectsNonStickyWritableAncestor|TestLiveTerminalRuntimeHelperRejectsStickySymlinkReplacementRace)$'
```

complex_cases:

- Replacement occurs after descriptor validation but before emission proceeds.
- Both profile Git reads and fallback-vault reads use validated objects.
- Shared TMPDIR is actively inspected during the run.

edge_cases:

- Sticky and non-sticky shared directories.
- Final symlink, intermediate symlink, non-directory, unsafe mode, and
  substituted target.

expected: |
  Unsafe roots fail before emission/evaluation. The real emitter reads only the
  descriptor-authenticated profile and vault. No secret-bearing source or
  command output appears in shared TMPDIR, and substituted paths remain
  unchanged.

evidence_to_capture:

- Fixture topology/modes before and after.
- Child invocation count, output identity, and shared-directory inventory.

result: [pending]

### 19. Store permission migration and supported-platform boundary

id: E2E-05-19
priority: P1
type: built-binary + cross-build matrix
risk_covered: Product-created stores are rejected by runtime, unsafe stores are chmodded, or unsupported platforms install unusable state.

preconditions:

- Cross-compilation toolchain available through Go.

instructions:

1. Create a fresh store and confirm mode 0700 after install.
2. Create a current-user 0755 bare store and verify safe migration to 0700.
3. Try symlink, non-directory, substituted target, and unsafe existing root;
   confirm no target chmod or mutation.
4. Verify Linux/Darwin accepted paths align with descriptor runtime support.
5. Cross-build FreeBSD and Windows and run/compile their fallback tests,
   confirming install fails closed before mutation on unsupported platforms.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/store -run \
  '^(TestInitRefusesUnsafeExistingStoreRoot|TestInitForInstallRollsBackOnlyCreatedStoreState)$'
GOTOOLCHAIN=auto go test -count=1 -v ./core/cmd/zsh-pro -run \
  '^TestBuiltBinaryInstallMatchesDescriptorRuntimePlatformBoundary$'
GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go build ./...
GOOS=freebsd GOARCH=amd64 GOTOOLCHAIN=auto go build ./...
GOOS=windows GOARCH=amd64 GOTOOLCHAIN=auto go build ./...
```

complex_cases:

- Existing valid repository requires mode migration.
- Unsupported platform reaches Store initialization before any bootstrap write.
- Platform build tags must agree across store, installer, and runtime capture.

edge_cases:

- Symlinked root, foreign owner where available, missing root, regular file,
  and current-user directory that is not a bare repository.

expected: |
  Supported product-created stores work with ordinary and sourced runtime.
  Safe current-user migration changes only the final directory mode. Unsafe
  targets are not chmodded or modified. Unsupported platforms compile but fail
  installation before store/cache/`.zshrc` mutation.

evidence_to_capture:

- Mode/owner/repository checks.
- Cross-build outputs and fallback-test result.

result: [pending]

### 20. Startup zero-subprocess structure and measured Hyperfine budget

id: E2E-05-20
priority: P0
type: structural + measured performance
risk_covered: Correct loader adds hidden shell-start subprocesses or exceeds the <10 ms budget.

preconditions:

- Quiet host.
- Hyperfine available on PATH or locally staged without system installation.

instructions:

1. Run the structural start-path test.
2. Build a fresh absolute binary path.
3. Run `scripts/perf-hyperfine.sh` three times, preserving each terminal output
   and exported JSON while practical.
4. Confirm each run verifies `activate: function`.
5. If Hyperfine reports outliers, quiet the host and repeat rather than choosing
   only the best run.
6. Record means, standard deviations, ranges, and added startup result.

commands: |

```bash
GOTOOLCHAIN=auto go test -count=1 -v ./core/shell/zsh -run \
  '^TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative$'

perf_root="$(mktemp -d)"
GOTOOLCHAIN=auto go build -o "$perf_root/zsh-pro" ./core/cmd/zsh-pro
ZSHPRO_BIN="$perf_root/zsh-pro" scripts/perf-hyperfine.sh
```

complex_cases:

- With-loader and without-loader shells use isolated HOME/ZDOTDIR fixtures.
- A negative added mean is valid measurement noise but must still be recorded.
- Reruns distinguish stable budget compliance from one noisy sample.

edge_cases:

- Hyperfine absent, outlier warning, system load spike, non-absolute binary,
  and loader install failure.

expected: |
  Structural test proves no startup command execution in the file-sourced hot
  path. Every valid measured run exits 0, verifies `activate: function`, and
  reports added startup mean strictly below 10.000 ms. An absent Hyperfine
  executable is `blocked`, not a pass.

evidence_to_capture:

- Hyperfine version and full output for every run.
- Added means and any outlier warnings.
- Absolute binary path and commit.

result: [pending]

### 21. Final repository, cross-platform, and artifact consistency gate

id: E2E-05-21
priority: P0
type: release gate
risk_covered: Narrow E2E cases pass while the repository or planning evidence is stale/incomplete.

preconditions:

- Cases 1-20 have definitive results.

instructions:

1. Run build, uncached full tests, vet, and `make check`.
2. Run Linux amd64, Darwin amd64/arm64, FreeBSD amd64, and Windows amd64
   cross-builds.
3. Confirm `05-REVIEW.md` is `clean` with zero findings.
4. Confirm `05-VERIFICATION.md` is `passed`, score 4/4, and records a current
   Hyperfine result.
5. Run GSD phase completeness and confirm 8 plans, 8 summaries, no errors or
   warnings.
6. Check the worktree and distinguish test artifacts from pre-existing user
   planning changes; do not stage or remove unrelated files.

commands: |

```bash
GOTOOLCHAIN=auto go build ./...
GOTOOLCHAIN=auto go test -count=1 ./...
GOTOOLCHAIN=auto go vet ./...
make check
GOOS=darwin GOARCH=amd64 GOTOOLCHAIN=auto go build ./...
GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=auto go build ./...
GOOS=freebsd GOARCH=amd64 GOTOOLCHAIN=auto go build ./...
GOOS=windows GOARCH=amd64 GOTOOLCHAIN=auto go build ./...
node /home/metanmai/.codex/gsd-core/bin/gsd-tools.cjs verify phase-completeness 5
git status --short
```

complex_cases:

- Full uncached native-zsh suite runs after all prior fixtures.
- Planning state and current commit are checked separately from code gates.
- Cross-build success is not confused with runtime support on unsupported OSes.

edge_cases:

- Generated Graphify output or pre-existing planning changes in the worktree.
- A clean review with stale verification, or passed verification with missing
  plan summaries.

expected: |
  Build, uncached tests, vet, lint, and all cross-builds pass. Sol review is
  clean. Verification is passed 4/4 with measured timing. GSD reports complete
  true, 8 plans, 8 summaries, and no errors/warnings. No unexpected source or
  test artifact remains in the worktree.

evidence_to_capture:

- Complete gate logs.
- Review/verification frontmatter and phase-completeness JSON.
- Final `git status --short` with every remaining path classified.

result: [pending]

## Traceability

| Requirement / risk | Covered by cases |
| --- | --- |
| BOOT-01 sourced verbs and current-shell mutation | 1, 8-13, 15 |
| BOOT-01 installer idempotence and byte preservation | 1-6 |
| BOOT-02 fail-open startup/runtime | 7-9, 12-14 |
| BOOT-02 validated/private runtime transport | 12, 14, 16-19 |
| BOOT-02 zero-subprocess and <10 ms startup | 20 |
| Secret resolution and cleanup | 13, 16, 17 |
| Store/cache root and platform consistency | 2, 4, 5, 18, 19 |
| CLI/sourced argument contracts | 6, 8 |
| Repository-wide release evidence | 21 |

## Summary

total: 21
passed: 0
issues: 0
pending: 21
skipped: 0
blocked: 0

## Gaps

[none yet]

