---
phase: 05-runtime-loader-cli-bootstrap
verified: 2026-07-27T14:59:02Z
status: gaps_found
score: 0/4 must-haves verified
behavior_unverified: 1
overrides_applied: 0
gaps:
  - truth: "The installer preserves every byte of unmanaged .zshrc content outside real BEGIN/END marker lines."
    status: failed
    reason: "replaceManagedBlock recognizes marker text as an arbitrary substring, so quoted ordinary content is removed as though it were a managed block."
    artifacts:
      - path: "core/cli/install.go"
        issue: "bytes.Index at lines 98-99 is not line/marker anchored."
    missing:
      - "Replace the substring scan with a line-based exact-marker parser and add quoted-string and suffix-comment regression tests."
  - truth: "A broken, missing, or slow zsh-pro never locks the current terminal out of a working shell."
    status: failed
    reason: "The loader has no timeout around either emit or zsh -n; a one-second external watchdog killed both three-second hangs. A failed verb also exits a caller using ERR_EXIT before its next command can run."
    artifacts:
      - path: "core/shell/zsh/hook.go"
        issue: "_zp_emit and _zp_eval_block run unbounded subprocesses; their error result remains fatal to a direct ERR_EXIT caller."
    missing:
      - "Add a portable bounded-execution mechanism for emit and validation, clean staged files on every path, and test ERR_EXIT survivability plus emitter/validator timeout behavior."
  - truth: "activate, checkout, and deactivate leave the current terminal in exactly the selected profile state."
    status: failed
    reason: "activate B after activate A evaluates only zp_apply for B, leaving A-only state present."
    artifacts:
      - path: "core/shell/zsh/hook.go"
        issue: "activate at lines 84-92 does not transition through checkout or emit/deactivate the prior profile."
      - path: "core/cli/emitter.go"
        issue: "runtimeEmitter computes Diff(active,target) but returns only the apply half at lines 78-83."
    missing:
      - "Make active-profile activate use one combined deactivate-prior plus apply-target block (or delegate to checkout) and add a real-store A-only-state regression test."
  - truth: "Nil-like composition-root dependencies fail through cli.fail rather than panicking."
    status: failed
    reason: "A typed nil *store.Store satisfies the Store interface and bypasses c.store == nil, then panics in Store.Branches."
    artifacts:
      - path: "core/cli/cli.go"
        issue: "nil guards at runList/runStatus/runEmit only reject nil interfaces."
      - path: "core/cli/emitter.go"
        issue: "NewRuntimeEmitter also accepts a typed-nil Store."
    missing:
      - "Normalize/reject nil-like injected interfaces at construction (or use an internal reflection helper) and add typed-nil Store/Emitter tests."
behavior_unverified_items:
  - truth: "The file-sourced hot path adds less than the required startup budget, measured by hyperfine."
    test: "Run ZSHPRO_BIN=<built binary> scripts/perf-hyperfine.sh on a host with hyperfine installed."
    expected: "The script proves activate is sourced and reports added interactive-shell mean below 10 ms."
    why_human: "The mandatory structural no-subprocess check passed, but hyperfine was absent, so no timing measurement exercised the budget."
---

# Phase 5: Runtime Loader + CLI + Bootstrap Verification Report

**Phase Goal:** Wire the live terminal to the binary and ship the user-facing surface: a sourced loader, profile verbs, an idempotent fail-open bootstrap block, and a fast hot path.
**Verified:** 2026-07-27T14:59:02Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `checkout`/`activate`/`deactivate` change the current terminal into the selected profile state; `list`/`status` report state. | ✗ FAILED | Loader functions exist and basic synthetic test passes, but a live zsh repro of `activate A; activate B` leaves A-only alias state present (exit 42). |
| 2 | Installer is byte-idempotent and never changes unmanaged `.zshrc` content. | ✗ FAILED | Real `zsh-pro install` deleted `ordinary-content-must-survive` between quoted marker substrings. |
| 3 | Broken, missing, or slow runtime dependencies cannot lock out a working terminal. | ✗ FAILED | Both a sleeping emitter and sleeping validator were killed only by an external `timeout 1s` (exit 124); typed-nil store also panics. |
| 4 | The sourced startup hot path has zero subprocesses and meets the measured budget. | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Structural hot-path check and `hook | zsh -n` pass; `hyperfine` is unavailable, so the <10 ms runtime budget was not measured. |

**Score:** 0/4 truths verified (1 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `core/shell/zsh/hook.go` | Sourced loader and five shell verbs | ⚠️ WIRED, FUNCTIONALLY BLOCKED | 136 substantive lines; returned by `HookScript()` and loaded by the installer. Its active-profile transition and unbounded command paths fail live repros. |
| `core/cli/install.go` | Safe idempotent installer and cached loader | ⚠️ WIRED, FUNCTIONALLY BLOCKED | Cache-first atomic-write/symlink logic is substantive and dispatch-wired, but marker recognition deletes ordinary quoted content. |
| `core/cli/cli.go` | Runtime verb dispatch and dependency guards | ⚠️ WIRED, FUNCTIONALLY BLOCKED | `hook/install/list/status/emit` cases are wired; nil-interface guards do not protect typed nils. |
| `core/cli/emitter.go` | Store/profile-to-emitted-zsh adapter | ⚠️ WIRED, FUNCTIONALLY BLOCKED | It reads profiles, builds/diffs manifests, and calls `Provider.Emit`, but discards the computed deactivate half on the apply path. |
| `core/cmd/zsh-pro/main.go` | Concrete provider/store/emitter injection | ⚠️ PARTIAL | Main uses a literal nil interface on `store.New` error, but CLI construction still accepts typed-nil dependencies from other composition paths. |
| `core/shell/zsh/live_terminal_test.go` | Runtime behavior coverage | ⚠️ INSUFFICIENT | Existing shim always overwrites the same alias and its deactivate resets all test state; it cannot expose A-only residue after `activate A -> activate B`. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `core/cli/cli.go` | `core/shell/zsh` loader | `provider.HookScript()` | ✓ WIRED | `hook` prints `HookScript()` and installer caches the same value. |
| `core/cmd/zsh-pro/main.go` | CLI runtime seams | `cli.New(provider, cliStore, emitter)` | ⚠️ PARTIAL | Injection exists; typed-nil interface inputs are not fail-closed. |
| Loader verbs | `zsh-pro emit` | `_zp_emit` -> `command zsh-pro emit` -> `_zp_eval_block` | ⚠️ WIRED, UNSAFE | Exit/empty guards exist, but emitter and validator calls are unbounded; state transition is incomplete. |
| Installer stub | cached loader | guarded `command -v` + `[[ -r ]]` + `source` | ✓ WIRED | Missing/corrupt/disabled basic fail-open tests pass. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| Loader | `REPLY` / emitted block | `_zp_emit` invokes CLI `emit` | Yes — `runtimeEmitter` validates/reads Store profiles then calls `Provider.Emit` | ⚠️ FLOWING BUT INCORRECT | On `activate` it receives only `zp_apply`; `runtimeEmitter` returns only `apply` even when `Diff(active,target)` generated deactivation operations. |
| Loader status | `ZSHPRO_PROFILE` | current terminal environment | Yes | ✓ FLOWING | No shared active-profile file; unset reports `main`. |

### Behavioral Spot-Checks

| Behavior | Command / setup | Result | Status |
| --- | --- | --- | --- |
| Repository regression suite | `go test ./...` | All packages passed. | ✓ PASS — not sufficient for the adversarial gaps below. |
| Loader syntax | `<built zsh-pro> hook \| zsh -n` | Exit 0. | ✓ PASS |
| Startup structure | loader top-level structural scan | No `$(`, backticks, `git`, or `zsh-pro` invocation. | ✓ PASS |
| CR-01 marker preservation | Temp HOME/ZDOTDIR with quoted marker strings; real `zsh-pro install` | `ordinary-content-must-survive` removed; resulting file spliced the install block into the quoted text. | ✗ FAIL |
| CR-02 ERR_EXIT | `setopt ERR_EXIT; source loader; activate bad; print survived`, malformed emission | Exit 1; validation handler printed, but `survived` did not run. | ✗ FAIL |
| CR-03 emitter timeout | Sleeping fake `zsh-pro`; outer `timeout 1s` | Exit 124. | ✗ FAIL |
| CR-03 validator timeout | Sleeping fake `zsh`; outer `timeout 1s` | Exit 124. | ✗ FAIL |
| CR-04 active transition | Fake production-shaped A-only/B-only emit blocks; `activate A; activate B` | `STALE_A_SURVIVED`, exit 42. | ✗ FAIL |
| CR-05 typed nil | `var s *store.Store; cli.New(..., s, ...).Run(["list"])` | Panic in `(*Store).Branches`, exit 1. | ✗ FAIL |
| WR-01 cache permission repair | Pre-create `loader.zsh` mode 0644, then real install | Remained mode 0644. | ⚠️ WARNING |
| WR-02 xtrace preservation | `setopt XTRACE; activate trace; [[ -o XTRACE ]]` | `XTRACE_PRESERVED`, exit 0. | ℹ️ REFUTED — this warning does not reproduce on zsh 5.9; `NOXTRACE` is function-scoped here. |

### Probe Execution

| Probe | Command | Result | Status |
| --- | --- | --- | --- |
| Performance backstop | `ZSHPRO_BIN=<built binary> scripts/perf-hyperfine.sh` | `SKIP: hyperfine absent` | ? SKIP — see Human Verification |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| BOOT-01 | 05-01, 05-02 | Marked idempotent `.zshrc` block preserving unmanaged content plus live runtime surface | ✗ BLOCKED | CR-01 proves data loss from quoted marker substrings; CR-04 proves `activate` does not reliably transition an active terminal. |
| BOOT-02 | 05-01, 05-02 | Fail-open/fast loader with guarded sourcing, validation, last-good path, and escape hatch | ✗ BLOCKED | CR-02 direct ERR_EXIT caller exits; CR-03 hangs indefinitely; CR-05 panics on typed nil. Structural startup check passes but timing is unmeasured. |

No orphaned Phase 5 requirements were found: both declared plan IDs (`BOOT-01`, `BOOT-02`) map to Phase 5 in `REQUIREMENTS.md`.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `core/cli/install.go` | 98-99 | Raw `bytes.Index` marker substring search | 🛑 BLOCKER | Deletes unmanaged user `.zshrc` content. |
| `core/shell/zsh/hook.go` | 45, 65 | Unbounded emitter and validator subprocesses | 🛑 BLOCKER | A slow dependency blocks the terminal indefinitely. |
| `core/shell/zsh/hook.go` | 84-92 | Active `activate` path omits prior deactivation | 🛑 BLOCKER | Stale prior-profile state survives a transition. |
| `core/cli/cli.go` | 68, 81, 99 | Interface equality used as typed-nil protection | 🛑 BLOCKER | Nil pointer panic instead of `cli.fail`. |
| `core/cli/install.go` | 165-168 | Existing cached-loader mode is preserved | ⚠️ WARNING | Reinstall does not repair an insecure 0644 cache. |

No untracked `TBD`, `FIXME`, or `XXX` debt marker was found in Phase 5 source files.

### Human Verification Required

### 1. Startup performance budget

**Test:** Install a built binary into a disposable `HOME`/`ZDOTDIR` on a host with `hyperfine`, then run `ZSHPRO_BIN=<absolute binary> scripts/perf-hyperfine.sh`.

**Expected:** The installer-produced loader is sourced and added interactive-startup mean is below 10 ms.

**Why human:** `hyperfine` is not installed in this verification environment; static no-subprocess evidence cannot measure startup latency.

### Gaps Summary

Phase 5 is not ready to advance. The implementation has substantive, normally-wired artifacts, but the decisive runtime/reliability properties are false: it can delete ordinary `.zshrc` content, block indefinitely, leave profile residue on `activate` transitions, and panic on typed-nil dependencies. The all-green Go suite is not contrary evidence: its current live-loader fixture does not model the A-only/B-only production transition, and it has no marker-substring, timeout, or typed-nil cases.

The performance check remains unmeasured rather than failed. It does not defer any blocker: no later roadmap phase specifically covers these Phase 5 bootstrap/reliability defects.

---

_Verified: 2026-07-27T14:59:02Z_
_Verifier: the agent (gsd-verifier)_
