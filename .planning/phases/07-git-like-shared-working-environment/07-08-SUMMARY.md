---
phase: 07-git-like-shared-working-environment
plan: 08
subsystem: shell-runtime
tags: [zsh, live-state, nul-framing, worktree, descriptor-binding, acknowledgement]
requires:
  - phase: 07-02
    provides: narrow live capture, decode, and patch interfaces
  - phase: 07-03
    provides: credential-authenticated descriptor-bound worktree Service and durable pending transitions
  - phase: 07-04
    provides: typed live patch construction and snapshot fingerprinting
provides:
  - bounded current-zsh semantic capture and all-or-none NUL decoder
  - concrete-zsh live apply/replacement-reverse source ownership
  - five-method credential-forwarding RuntimeWorktree adapter
  - descriptor-bound runtime factory with explicit duplicate-descriptor close ownership
  - private transition payload and exact emitter-owned acknowledgement footer
  - durable uint64 reply-handle recovery to the Service's opaque resolution token
affects: [07-09-runtime-hooks, shared-worktree-sync, parent-shell-acknowledgement]
tech-stack:
  added: []
  patterns:
    - sourced-shell NUL records with byte and record caps
    - validate-completely-before-emitting exact shell source
    - descriptor-bound late factory construction
    - private source and fixed-size public metadata split
    - durable opaque-token lookup behind a fixed-width parent-shell handle
key-files:
  created: []
  modified:
    - core/shell/zsh/introspect.go
    - core/shell/zsh/introspect_test.go
    - core/shell/zsh/emit.go
    - core/shell/zsh/emit_test.go
    - core/cli/emitter.go
    - core/cli/emitter_test.go
key-decisions:
  - "Capture the current zsh in place with a sourced builtin-only function; retain the existing child-zsh path only for file introspection."
  - "Keep patch source in an unexported CLI payload and expose only uint64 values plus a fixed SHA-256 fingerprint as metadata."
  - "Let concrete zsh own the complete acknowledgement assignment footer and return zero bytes on every validation failure."
  - "Represent the opaque Service resolution token as a deterministic uint64 reply handle, then recover it from descriptor-bound durable pending state before Service performs authoritative acknowledgement checks."
metrics:
  duration: 30m
  completed: 2026-08-17
  tasks: 2
  files: 6
status: complete
---

# Phase 7 Plan 8: Live Shell Capture and Runtime Transition Summary

**Bounded current-zsh snapshots now flow through a five-operation descriptor-bound adapter into exact reversible zsh programs with private source and durable parent-shell acknowledgement metadata.**

## Performance

- **Duration:** 30 minutes
- **Started:** 2026-08-17T13:09:47Z
- **Completed:** 2026-08-17T13:39:25Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added a sourced, builtin-only current-shell capture function for exported environment scalars, aliases, multiline functions, exact PATH/FPATH arrays, and option booleans. Its versioned NUL decoder enforces the 2 MiB and 10,000-record caps and returns a zero snapshot on malformed input.
- Added concrete-zsh `EmitLivePatch` and `EmitRuntimeTransition` ownership. Apply and replacement-reverse functions cover every live category, hostile or malformed inputs yield zero bytes, and the final reply footer has one fixed protocol/revision/token/fingerprint/complete order.
- Added the exact five credential-bearing runtime operations: Attach, Publish, Prepare, Acknowledge, and Resolve. Each decodes a bounded frame and forwards the supplied `ShellCredential` unchanged to the freshly descriptor-bound Service.
- Added late `RuntimeWorktreeFactory.Bind` construction over the authenticated `RuntimeRoot`, with independent StateStore/Registry/Service values, no pathname reopen, explicit idempotent close ownership, and preservation of the caller-owned root descriptor.
- Closed the 07-08 to 07-09 token handoff: the decimal reply token is a fixed-width SHA-256 handle, while a later factory binding recovers the original opaque token from the canonical pending transition before `Service.Acknowledge` rechecks the credential and token under its transaction.

## Task Commits

Each task used RED/GREEN TDD commits with normal repository hooks:

1. **Task 1: Capture and decode bounded current-shell semantic state**
   - `d323280` — `test(07-08): define bounded live capture contract`
   - `6abbc58` — `feat(07-08): capture bounded current-shell state`
2. **Task 2: Render and transport exact patches with executable acknowledgement metadata**
   - `b255b35` — `test(07-08): define runtime worktree transition contract`
   - `cc74035` — `feat(07-08): emit descriptor-bound live transitions`
   - `cbd9e80` — `fix(07-08): bind reply token handles durably`

## Files Created/Modified

- `core/shell/zsh/introspect.go` — Current-shell capture source, bounded NUL reader, canonical record validation, and all-or-none semantic decoding.
- `core/shell/zsh/introspect_test.go` — Real-zsh exact round trips, byte/record boundaries, malformed frames, and child/process-local exclusion tests.
- `core/shell/zsh/emit.go` — Exact live operation rendering, reserved-name rejection, caller-named patch functions, and sole reply-footer generation.
- `core/shell/zsh/emit_test.go` — All-category apply/reverse execution, syntax validation, metadata-only output, and zero-source hostile-input tests.
- `core/cli/emitter.go` — Fixed-size metadata, private payloads, exact five-method runtime interface, descriptor-bound factory, Service adapters, and durable token-handle recovery.
- `core/cli/emitter_test.go` — Method/credential reflection, private-source serialization separation, credential forwarding, no-op/metadata-only behavior, typed nils, path replacement, close ownership, and cross-binding acknowledgement.

## Decisions Made

- The live capture function intentionally does not use `emulate -L` or local option scoping because the caller's actual option booleans are part of the captured semantic state.
- Public transition metadata contains only `uint64` values and one `[32]byte` fingerprint. Executable source and attach credentials remain in unexported payloads that cannot serialize through the public metadata shape.
- The numeric reply token is not treated as the Service's token. It is a deterministic fixed-width handle resolved only through the descriptor-bound pending transition; the opaque token remains durable and is still verified by the Service.
- Runtime binding owns the duplicated StateStore descriptor only. Closing the returned closer never closes `RuntimeRoot`, and replacing the authenticated pathname cannot redirect later binds or operations.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Added durable numeric-to-opaque resolution-token binding**

- **Found during:** Task 2 cross-plan 07-09 contract audit
- **Issue:** Plan 07-09 explicitly feeds the canonical decimal reply token back to Acknowledge, but Plan 07-03 persists a non-reversible opaque string token. A one-way numeric projection alone could not acknowledge after the prepare helper exited.
- **Fix:** Acknowledge now accepts the decimal `uint64` handle and resolves it against the current descriptor-bound durable pending transition. It then forwards the recovered opaque token and unchanged credential to `Service.Acknowledge`, which remains the authoritative transactional verifier.
- **Files modified:** `core/cli/emitter.go`, `core/cli/emitter_test.go`
- **Commit:** `cbd9e80`

## TDD Gate Compliance

- RED gate: `d323280`, `b255b35`
- GREEN gate: `6abbc58`, `cc74035`
- Security/cross-plan correction: `cbd9e80`
- Normal hooks remained enabled for every commit. Task 2's RED commit included compile-only error-returning seams because the repository pre-commit hook type-checks test packages; the behavioral tests still failed on those seams before GREEN.

## Verification

- `GOTOOLCHAIN=local go test ./... -count=1` — passed
- `GOTOOLCHAIN=local go vet ./...` — passed
- `GOTOOLCHAIN=local go build ./...` — passed
- `golangci-lint run` — passed with 0 issues
- Both plan-scoped exact test commands — passed
- Real `zsh -f` capture, apply/reverse, syntax, and acknowledgement-envelope execution — passed
- AST/token/stub/secret/threat scans — passed: five reply assignments exist only in concrete zsh production code; live capture has no child shell or process-local kind; no new resolver call, placeholder, logging, shell command, endpoint, schema, or persistent patch path was introduced.

## Known Stubs

None. The modified production diff contains no TODO, FIXME, placeholder, coming-soon, panic, or not-implemented path.

## Auth Gates

None.

## Tracking Notes

- `state.advance-plan` moved the sequential execution-position counter from 6/10 to 7/10. This counter records completion order, so it is correct even though the completed plan ID is 07-08.
- The plan declares WORK-02, SYNC-01, and SYNC-02, but those IDs do not exist in the current `.planning/REQUIREMENTS.md`; the required `requirements.mark-complete` call reported all three as `not_found` and made no file change. No replacement requirement IDs were invented.

## Threat Model Coverage

- **T-07-31:** Complete validation and safe concrete-zsh quoting precede all source return; hostile identities return zero bytes.
- **T-07-32 / T-07-34A:** Source stays in an unexported payload, public metadata is fixed-size/value-free, and errors do not include capture or patch bytes.
- **T-07-33:** Factory tests replace the authenticated pathname and prove canonical descriptor authority plus caller-owned root lifetime.
- **T-07-34:** Capture enforces exact byte and record caps and never launches a child zsh.
- **T-07-34B:** The zsh emitter owns exactly five ordered assignments, rejects the reserved namespace, and emits metadata even for pending zero-mutation transitions.
- **T-07-34C:** Reflection and behavioral tests freeze exactly five credential-bearing methods and prove unchanged credential forwarding into Service.

No new unplanned network, authentication, schema, or external file-access surface was introduced.

## Next Phase Readiness

Plan 07-09 can bind the factory after RuntimeRoot authentication, validate the entire private source with `zsh -n`, eval it in the parent, parse canonical decimal metadata, and acknowledge through a fresh binding without retaining in-process Service state.

---
*Phase: 07-git-like-shared-working-environment*
*Completed: 2026-08-17*

## Self-Check: PASSED

All six modified implementation/test files and the summary exist; all five task/deviation commits are present in repository history; required frontmatter marks the plan complete.
