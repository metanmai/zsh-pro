---
phase: 05-runtime-loader-cli-bootstrap
verified: 2026-07-29T19:22:28Z
status: gaps_found
score: "0/4 must-haves verified"
behavior_unverified: 1
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 0/4
  gaps_closed:
    - "Exact physical-line marker scanning closes the prior quoted-marker substring deletion path."
    - "Non-main A-to-B transitions now emit a combined deactivate-then-apply source."
    - "Typed-nil Store, CLI Emitter, shell Emitter, and SecretResolver inputs are normalized."
  gaps_remaining:
    - "Current terminal and bootstrap fail-open behavior"
    - "Transactional installer safety"
    - "Profile transition/deactivation zero-residue behavior"
    - "Measured startup-budget evidence"
  regressions: []
gaps:
  - truth: "A corrupt readable cached loader never aborts an ERR_EXIT shell startup."
    status: failed
    reason: "The installed stub runs source as an unguarded simple command; a corrupt loader exits before the trailing true can consume the failure."
    artifacts:
      - path: core/cli/install.go
        issue: "renderInstallBlock emits an unguarded source at line 125."
    missing:
      - "Guard and consume the source result under ERR_EXIT, with a regression for a corrupt readable regular loader."
  - truth: "Every public sourced verb leaves an ERR_EXIT/ERR_RETURN terminal usable when the binary is missing, fails, or hangs."
    status: failed
    reason: "list bypasses _zp_run_bounded and directly executes command zsh-pro list."
    artifacts:
      - path: core/shell/zsh/hook.go
        issue: "list at lines 292-294 returns the binary's nonzero status directly."
    missing:
      - "Route list through the bounded, error-consuming public runtime boundary and cover failure, timeout, ERR_EXIT, and ERR_RETURN."
  - truth: "Switching from every activated profile to another profile removes prior-only state immediately."
    status: failed
    reason: "runtimeEmitter treats current == main as inactive even though main can be explicitly activated."
    artifacts:
      - path: core/cli/emitter.go
        issue: "The active-manifest condition at line 82 excludes main, so main-to-B output contains no zp_deactivate call."
    missing:
      - "Track active-versus-default state independently of the profile display name and add a real-store main-to-B-to-deactivate regression."
  - truth: "An install rejected for malformed managed markers leaves both the .zshrc and prior cached loader unchanged."
    status: failed
    reason: "runInstall promotes the loader before it reads and validates the .zshrc marker structure."
    artifacts:
      - path: core/cli/install.go
        issue: "Lines 39-48 create and replace the cache before lines 51-61 validate/prepare the .zshrc replacement."
    missing:
      - "Prepare and validate the .zshrc transaction before any cache-directory creation or cache promotion; test preservation of an existing cache."
  - truth: "Every public CLI dependency seam rejects typed-nil values through normal runtime errors rather than panicking."
    status: failed
    reason: "CLI.New normalizes Store and Emitter but retains a literal or typed-nil shell.Provider."
    artifacts:
      - path: core/cli/cli.go
        issue: "New at lines 33-40 does not normalize p; hook dereferences c.provider at line 57."
    missing:
      - "Normalize provider with isNilLike and fail provider-dependent commands before method dispatch; add typed-nil provider cases."
  - truth: "A successfully activated secret-bearing profile can always be deactivated without a fresh secret retrieval."
    status: failed
    reason: "Deactivate and active-profile transition paths re-resolve SecretRefs; a later resolver failure produces no reverse source."
    artifacts:
      - path: core/cli/emitter.go
        issue: "Target resolution at lines 60-67 occurs for deactivate, and active resolution at lines 83-90 occurs for a switch."
    missing:
      - "Retain activation-local reverse data/source after successful activation and use it for deactivation/switching without another resolver lookup."
  - truth: "Staged emitted shell source remains private and cannot be replaced through an untrusted TMPDIR."
    status: failed
    reason: "The loader accepts any writable TMPDIR, closes the exclusive file, then reopens the path by name for writing and validation."
    artifacts:
      - path: core/shell/zsh/hook.go
        issue: "_zp_private_temp accepts writable TMPDIR at lines 69-84; _zp_eval_block reopens tmp at lines 198 and 204."
    missing:
      - "Use an owner-controlled 0700 runtime directory and retain safe file ownership across write/validate/eval; add a non-sticky shared-directory race regression."
behavior_unverified_items:
  - truth: "The file-sourced startup hot path adds less than 10 ms of interactive-shell mean startup cost."
    test: "Run ZSHPRO_BIN=<absolute built binary> scripts/perf-hyperfine.sh on a host with hyperfine."
    expected: "The harness proves activate is sourced and reports added mean below 10 ms."
    why_human: "The mandatory structural test passes, but hyperfine is absent here, so the runtime budget was not exercised."
---

# Phase 5: Runtime Loader + CLI + Bootstrap Verification Report

**Phase Goal:** Wire the live terminal to the binary and ship the user-facing surface: a sourced loader, profile verbs, an idempotent fail-open bootstrap block, and a fast hot path.
**Verified:** 2026-07-29T19:22:28Z
**Status:** gaps_found
**Re-verification:** Yes — after gap-closure Plans 05-03 through 05-05

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `checkout`/`activate`/`deactivate` change the current terminal into the selected profile state; `list`/`status` report state. | ✗ FAILED | A direct `list` under `ERR_EXIT` exited 127 before `SURVIVED`; a current=`main` emitter probe generated B apply source without `zp_deactivate`; and a resolver that succeeds on activation then fails causes `deactivate` to emit no source. |
| 2 | The installer is safe and idempotent: its marked `.zshrc` block preserves unmanaged content and a rejected install has no runtime side effects. | ✗ FAILED | Exact marker scanning and idempotence tests pass, but a nested-marker real install exited 1 after replacing a known-good cached loader (`CACHE_PRESERVED=no`). |
| 3 | A broken, missing, or slow runtime dependency never locks a terminal out of a usable shell. | ✗ FAILED | A corrupt cached loader under `ERR_EXIT` exited 1 before `SURVIVED`; `list` with a missing binary exited 127; a typed-nil provider panics; and emitted source can be replaced through an untrusted staging directory. |
| 4 | The sourced startup hot path is zero-subprocess and meets the measured startup budget. | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | `TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative` passes, but `scripts/perf-hyperfine.sh` explicitly skipped because hyperfine is unavailable. |

**Score:** 0/4 truths verified (1 present, behavior-unverified)

The repair commits did close narrower prior paths: exact marker-line recognition, non-main A-to-B composition, bounded emit/validator work, and several typed-nil seams. Those improvements are not enough to establish the four outcome-level truths above.

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/shell/zsh/hook.go` | Sourced loader, public verbs, bounded emit/validation path | ⚠️ WIRED, FUNCTIONALLY BLOCKED | 306 substantive lines; `HookScript()` flows through the CLI and cache. `list` bypasses the bounded path, staging trusts arbitrary writable `TMPDIR`, and public profile behavior still has residue/failure gaps. |
| `core/cli/install.go` | Exact-marker installer, cache writer, fail-open stub | ⚠️ WIRED, FUNCTIONALLY BLOCKED | 313 substantive lines; real line-aware scanner and validated cache writer exist. The cache is promoted before malformed `.zshrc` is rejected, and the stub's `source` result is unguarded. |
| `core/cli/emitter.go` | Store/profile-to-emitted transition adapter | ⚠️ WIRED, FUNCTIONALLY BLOCKED | 109 substantive lines; it reads profiles, resolves secrets, builds/diffs manifests, and calls the zsh emitter. It treats `main` as inactive and requires a new secret read for reverse source. |
| `core/cli/cli.go` | Hook/install/list/status/emit dispatch with dependency normalization | ⚠️ WIRED, FUNCTIONALLY BLOCKED | 169 substantive lines; all verbs dispatch, but `New` does not normalize `shell.Provider`, so `hook` panics with a typed-nil provider. |
| `core/cmd/zsh-pro/main.go` | Concrete provider/store/resolver injection | ✓ WIRED | Main constructs the real provider/store/keychain and injects `NewRuntimeEmitter` then `cli.New`. The defect is the public constructor boundary, not this normal composition path. |
| `core/perf/hyperfine.go` and `scripts/perf-hyperfine.sh` | Typed timing decoder and hermetic measured backstop | ⚠️ PRESENT, NOT MEASURED | Parser and harness are substantive and linked; the only local execution path is the documented hyperfine-absent skip. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `core/cli/cli.go` | `core/shell/zsh` loader | `c.provider.HookScript()` | ⚠️ PARTIAL | The normal link exists at `cli.go:57`, but a typed-nil provider crosses it and panics. |
| `core/cmd/zsh-pro/main.go` | CLI runtime seams | `NewRuntimeEmitter(...); cli.New(...)` | ✓ WIRED | Concrete provider, literal-nil store policy, and keychain resolver are injected at lines 27-44. |
| Loader switch verbs | `zsh-pro emit` and `zsh -n` | `_zp_emit` / `_zp_eval_block` | ⚠️ PARTIAL | The bounded switch link exists, but `list` is an unbounded direct command and reverse secret source can be unavailable. |
| Installed stub | Cached loader | guarded `command -v` + readable file + `source` | ⚠️ PARTIAL | The path is correct, but an unguarded source propagates a corrupt-loader error under `ERR_EXIT`. |
| Installer transaction | cache plus `.zshrc` | validated candidate then marker replacement | ✗ NOT TRANSACTIONAL | Cache replacement happens before the `.zshrc` marker validation that can reject the install. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| Loader switch | `REPLY` emitted source | `_zp_emit` → CLI `emit` → `runtimeEmitter` → Store/manifest/zsh emitter | Yes | ⚠️ FLOWING BUT INCOMPLETE: `Current()=="main"` suppresses the active manifest, so no main deactivation flows into B. |
| Secret reverse path | resolved active profile | `Store.Read` → `resolveSecretRefs` | Yes on first activation | ✗ DISCONNECTED ON RESOLVER LOSS: deactivation requires a new resolve and returns empty source after a later fault. |
| Loader staging | `tmp` path | `_zp_private_temp` using `TMPDIR` | Yes | ✗ UNSAFE FLOW: exclusive creation is followed by reopening the path by name in a directory that need only be writable. |
| Installer | candidate loader then `.zshrc` block | `writeValidatedLoader` then `replaceManagedBlock` | Yes | ✗ WRONG ORDER: a malformed marker rejection occurs after cache replacement. |

### Behavioral Spot-Checks

| Behavior | Command / setup | Result | Status |
| --- | --- | --- | --- |
| Focused CLI, zsh, and performance suites | `go test -count=1 ./core/cli ./core/shell/zsh ./core/perf && go vet ./... && bash -n scripts/perf-hyperfine.sh` | Passed | ✓ PASS — insufficient for the direct failures below. |
| Workspace regression suite | `go test -count=1 ./...` | All packages passed | ✓ PASS — does not exercise the missing cases. |
| Corrupt cached loader under hostile startup option | Built binary installed into a disposable HOME; cache replaced with malformed zsh; `setopt ERR_EXIT; source .zshrc; print SURVIVED` | Exit 1; `SURVIVED=0` | ✗ FAIL |
| Failed public `list` | Source valid loader; remove binary from `PATH`; `setopt ERR_EXIT; list; print SURVIVED` | Exit 127; `SURVIVED=0` | ✗ FAIL |
| Malformed-marker install transaction | Exact nested markers plus recognizable existing cache; run real `install` | Exit 1; cache bytes changed (`CACHE_PRESERVED=no`) | ✗ FAIL |
| Typed-nil provider | Disposable Go consumer calls `cli.New((*zsh.Provider)(nil), ...).Run(["hook"])` | `TYPED_NIL_PROVIDER_PANIC=true` | ✗ FAIL |
| Active `main` to B transition | Disposable Store reports `Current()=="main"`; emit `apply B` | `MAIN_TO_B_HAS_DEACTIVATE=false` | ✗ FAIL |
| Resolver loss after activation | Resolver succeeds once, then fails; emit apply then deactivate | Apply succeeds; deactivation errors with empty source | ✗ FAIL |
| Staging path replacement | Simulate an attacker swap after exclusive create in a mode-0777 non-sticky `TMPDIR` | `_zp_eval_block` followed the replacement symlink and overwrote the target (`FOLLOWED_REPLACED_PATH`) | ✗ FAIL |
| Sentinel collision | Original environment value equals `__zsh_pro_unset_7c5a0a15__` | Value was unset after restore | ⚠️ WARNING |
| Startup structural gate | `go test -count=1 ./core/shell/zsh -run TestHookScriptStartPathIsZeroSubprocessAndNonSpeculative -v` | Passed | ✓ PASS |

### Probe Execution

No phase-declared or conventional `scripts/*/tests/probe-*.sh` probes were found. The performance harness was independently executed:

| Probe | Command | Result | Status |
| --- | --- | --- | --- |
| Startup timing backstop | `ZSHPRO_BIN=<built binary> scripts/perf-hyperfine.sh` | `SKIP: hyperfine absent; structural zero-subprocess test is the gate` | ? SKIP |

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
| --- | --- | --- | --- |
| BOOT-01 | 05-01 through 05-04 | Safe idempotent bootstrap and current-terminal profile surface | ✗ BLOCKED | Marker recognition is now correct, but rejected malformed input still replaces the cached runtime loader, and an explicitly active `main` profile leaks state into B. |
| BOOT-02 | 05-01 through 05-05 | Fail-open, validated, dependency-safe, fast loader | ✗ BLOCKED | Corrupt cache and `list` can terminate `ERR_EXIT`; provider typed nil panics; resolver loss prevents deactivation; emitted source staging is replaceable. Timing remains unmeasured. |

No orphaned Phase 5 requirements were found: BOOT-01 and BOOT-02 are declared by the plans and mapped to Phase 5 in `REQUIREMENTS.md`.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- |
| `core/shell/zsh/hook.go` | 7, 31 | Fixed-string unset sentinel doubles as a legitimate data value | ⚠️ WARNING | A valid original environment value equal to the sentinel is removed on deactivation; direct probe reproduced it. |

No unreferenced `TBD`, `FIXME`, or `XXX` debt markers were found in the Phase 5 source files. The seven blocker paths are implementation/wiring failures, not placeholder code.

### Behavior Evidence Still Needed After Gap Closure

Run the hermetic `scripts/perf-hyperfine.sh` on a host with `hyperfine` after the blockers are fixed. It must first prove `activate` is sourced, then report added interactive startup mean below 10 ms. This remains a present-but-unverified behavior; it does not excuse the blocking runtime failures.

### Gaps Summary

Phase 5 is not ready to advance. Current code and fresh probes confirm seven blockers:

1. A corrupt readable cached loader aborts `ERR_EXIT` startup.
2. `list` directly exposes binary failure/timeout to `ERR_EXIT` and `ERR_RETURN` callers.
3. `activate main; activate B` omits main's deactivation.
4. A malformed `.zshrc` rejects only after the cached loader has changed.
5. A typed-nil shell provider panics through public CLI verbs.
6. Resolver loss after a secret-bearing activation makes deactivation/switching impossible and leaves live state in place.
7. An untrusted writable `TMPDIR` permits staged-source path replacement before validation/eval.

Phase 6 does not explicitly cover these runtime/bootstrap fixes, so none are deferred. The shared verification-status routing resolves this report to `gaps_found`: plan the fixes, execute them, then re-verify.

---

_Verified: 2026-07-29T19:22:28Z_
_Verifier: the agent (gsd-verifier)_
