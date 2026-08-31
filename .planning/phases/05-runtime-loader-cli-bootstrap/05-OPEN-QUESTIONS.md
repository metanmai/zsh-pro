---
phase: 05-runtime-loader-cli-bootstrap
updated: 2026-07-27T00:00:00Z
open_count: 0
---

# Resolved Questions — Phase 5 Runtime Loader + CLI + Bootstrap

All former questions are resolved below. A listed execution-time gate is an adopted
phase-exit condition, not an unresolved design choice. Sources of truth are the
locked Phase 5 decisions, the reviewed Phase 5 plans, and `05-SOL-REVIEW.md`.

## OQ-05-01: `zp_*` runtime helper ownership

**Resolved decision:** The embedded `core/shell/zsh` loader owns the helper bodies and
runtime state. It definitively supplies `zp_capture_env`, `zp_restore_env`,
`ZP_UNSET_SENTINEL`, `ZP_BASE_PATH`, and the documented reverse-operation slots.
**Execution-time gate:** 05-01 Task 1 must reconcile the exact emitted Phase 4 helper
calls and state reads before phase exit; it adds only helpers that the actual emitter
uses.

## OQ-05-02: Secret dereference on switch

**Resolved decision:** Phase 5 resolves a persisted `SecretRef` through a narrow
composition-root-injected resolver before runtime manifest construction, failing the
whole emission without exposing the value. 05-05 Task 1 proves that seam with a
fixture. Phase 6 alone owns real-home ingest and the end-to-end PROF-03 round trip.

## OQ-05-03: Install verb and managed markers

**Resolved decision:** The verb is `zsh-pro install`; exact marker lines are
`# >>> zsh-pro >>>` and `# <<< zsh-pro <<<`. 05-03 Task 1 recognizes markers only
as complete physical lines and refuses malformed marker ordering before any write.

## OQ-05-04: Startup budget

**Resolved decision:** Added interactive startup mean remains under 10 ms. The
structural zero-subprocess check is mandatory in every environment; the timing result
is the optional-tool CI backstop described by OQ-05-12 and OQ-05-14.

## OQ-05-05: Phase 4 helper-call surface

**Resolved decision:** Do not predeclare PATH or shadow helpers that Phase 4 did not
emit. **Execution-time gate:** 05-01 Task 1 records the completed Phase 4 surface in
`05-CONTRACT.md`, maps every emitted bare helper/state read to the loader, and checks
the blocking RE-DIFF obligation only after the mapping is complete.

## OQ-05-06: Emit subcommand shape

**Resolved decision:** The CLI contract is `zsh-pro emit <apply|deactivate> <name>`.
**Execution-time gate:** 05-01 Task 1 compares the completed Phase 4 public emitter
surface to that contract. Any necessary synchronized adaptation is made through the
CLI `Emitter` seam and recorded in `05-CONTRACT.md`; phase exit is blocked until it is
reconciled.

## OQ-05-07: `zsh -n` validation location

**Resolved decision:** The sourced runtime verb validates the complete captured source
immediately before eval. 05-04 Task 1 gives both emission and validation portable
bounded execution, conditional failure handling, cleanup, and direct-call hostile
shell-option coverage.

## OQ-05-08: Last-good payload and runtime failure

**Resolved decision:** `ZP_LAST_GOOD_PROFILE` stores only the successful profile name.
On a runtime eval failure, preserve it, set explicit loader-visible error state, report
the recovery command, and return safely to the direct interactive caller; do not
automatically re-apply a possibly stale payload. 05-02 Task 2 and 05-04 Task 1 test
this report-only recovery contract.

## OQ-05-09: Store injection shape

**Resolved decision:** `core/cli` declares narrow Store and Emitter interfaces; the
composition root injects concrete dependencies. 05-05 Task 2 normalizes typed-nil
interfaces at public constructor boundaries so unavailable dependencies fail closed
without panics.

## OQ-05-10: Loader provider seam

**Resolved decision:** `core/shell/zsh.Provider.HookScript() string` exposes the
loader through the composite provider seam. `core/cli` prints that value and contains
no loader text or concrete zsh import.

## OQ-05-11: Structural zero-subprocess boundary

**Resolved decision:** The required structural test examines the installed stub and
cached-loader top level while excluding verb function bodies. It asserts the source
path has no command substitutions, backticks, `git` invocation, or binary command
execution; explicit verb bodies remain separately tested. This test always runs even
when zsh itself is unavailable.

## OQ-05-12: Hermetic performance fixture

**Resolved decision:** Use sibling disposable `ZDOTDIR` fixtures: one with the
installed stub and cached loader, one empty. The harness proves `activate` is actually
defined before comparing startup, never mutates the real home configuration, and uses
portable JSON parsing as implemented by 05-03 Task 3.

## OQ-05-13: Durable helper-contract reconciliation

**Resolved decision:** This is the durable form of OQ-05-05. 05-01 Task 1 writes
`05-CONTRACT.md` before loader work and keeps its RE-DIFF checkbox as a blocking
phase-exit gate. The contract covers both helper/state mapping and the CLI-to-emit
subcommand seam.

## OQ-05-14: Local absence of `hyperfine`

**Resolved decision:** Missing `hyperfine` may only produce the documented timing
skip; it does not skip the structural loader check. When the executable is present,
05-03 Task 3 builds/runs the current binary, validates fixture sourcing, and treats any
harness failure or over-budget result as a failure.

## OQ-05-15: CLI-to-emitter boundary

**Resolved decision:** Use the dedicated narrow `cli.Emitter` interface rather than
reconstructing emit orchestration from Store.Read in `core/cli`. Its exact completed
Phase 4 adapter is subject to the OQ-05-06/13 reconciliation gate; a missing adapter
fails with a clear no-source runtime error rather than emitting synthetic shell code.

## OQ-05-16: Runtime recovery depth

**Resolved decision:** Adopt explicit report-only recovery rather than automatic
last-good re-application. A failed eval may have partially changed shell state, so the
loader preserves last-good, exposes error state, reports the recovery command, and
keeps the interactive caller alive. 05-04 Task 1 verifies the direct ERR_EXIT and
ERR_RETURN behavior.

## OQ-05-17: `command -v zsh-pro` placement

**Resolved decision:** Keep the builtin `command -v zsh-pro` guard in the installed
stub, before cached-loader sourcing, and invoke the binary only from explicit verb
bodies. This satisfies both the fail-open requirement and the hot-path
zero-subprocess invariant.

## OQ-05-18: Durable contract location

**Resolved decision:** `05-CONTRACT.md` is the immediate Task-1 output and the
read-first source for dependent implementation tasks; the phase summary only links to
it. The checked reconciliation obligation is required before Phase 5 can complete.

## Resolution Check

- `open_count` is zero.
- OQ-05-01 through OQ-05-18 each has an adopted decision or a named, bounded
  execution-time gate.
- No deferred Phase 6 work has been pulled into Phase 5 beyond the runtime
  SecretRef seam and its fixture coverage.
