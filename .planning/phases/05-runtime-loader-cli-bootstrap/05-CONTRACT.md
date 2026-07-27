---
phase: 05-runtime-loader-cli-bootstrap
plan: 01
status: active
phase_exit_gate: true
---

# Phase 5 Runtime Loader and CLI Emit Contract

This is the durable Phase 4/5 boundary record. It was written before the
loader and CLI wiring so the implementation has one current source of truth.

## Reconciliation evidence

Reconciled on 2026-07-27 against the checkout's Phase 4 implementation:

| Probe | Result |
| --- | --- |
| `core/shell/zsh/emit.go` | present |
| `core/activate` | present |
| emitter implementation | `zsh.Provider.Emit(activate.Plan) (apply, deactivate string, err error)` |
| existing CLI emit command | absent; Phase 5 supplies `emit apply <name>` / `emit deactivate <name>` |

`emit.go` emits self-contained blocks: each block defines
`zp_capture_scalar`, `zp_restore_scalar`, and either `zp_apply` or
`zp_deactivate`. Those two scalar helpers are supplied inside the emitted
block, not by the loader. The loader must source/evaluate the block in the
current shell and must not duplicate, replace, or speculate about those
helpers.

## Part 1: loader helper and state surface

| Surface | Provider | Reconciled behavior |
| --- | --- | --- |
| `zp_capture_env <var>` | loader | Loader-owned compatibility helper. It captures a live export prior once into a sanitized derived slot or the unset sentinel. |
| `zp_restore_env <var> <applied>` | loader | Loader-owned, drift-guarded compatibility helper. It reverses only when the applied value is still live. |
| `ZP_UNSET_SENTINEL` | loader | Global marker distinguishing an unset prior from an empty prior. |
| `ZP_BASE_PATH` | loader shared eval path | Captured once before a path-mutating emitted block; `emit.go` reads it for PATH list rendering. |
| `ZP_BASE_FPATH` and `*_PRESENT` slots | emitted block | `emit.go` initializes these itself when a list delta needs them. Loader must preserve them across the block. |
| `ZP_ORIGINAL_*`, `ZP_PRESENT_*`, `ZP_EXPORTED_*`, `ZP_APPLIED_*`, `ZP_WAS_ON_OPTION_*` | emitted block | Actual Phase 4 state slots. `emit.go` derives identifier-safe names with hex encoding and reads/writes them through its embedded helpers. |
| alias/function prior slots | emitted block | Actual names are `ZP_ORIGINAL_ALIAS_<hex>`, `ZP_PRESENT_ALIAS_<hex>`, `ZP_ORIGINAL_FUNCTION_<hex>`, and `ZP_PRESENT_FUNCTION_<hex>`, not the plan-time illustrative profile-name slots. |

The loader sanitizes every name it derives before `typeset -g` or indirect
parameter access. It does not introduce `zp_rebuild_path` or `zp_shadow_*`:
those names are **not in contract unless a future emitter explicitly emits a
bare call to them**. Current `emit.go` has no such bare calls.

## Part 2: CLI-to-emit wiring

The user-facing loader invokes the binary subcommand surface below:

```
zsh-pro emit apply <name>
zsh-pro emit deactivate <name>
```

`core/cli` exposes this through a narrow CLI-local `Emitter` interface and
does not import `core/shell/zsh`. The composition root injects an adapter that
uses the Phase 4 provider, store read/build/diff path, and `Provider.Emit` to
produce stdout. A failed emission must go through `cli.fail` and emit no
partial shell source. The binary is the source of apply/deactivate text; the
loader never fabricates source itself.

The pre-Phase-4 fallback described by the plan is not active in this checkout:
Phase 4 is present. A temporary `NotReadyEmitter` remains useful only until
the real composition-root adapter is wired; it must fail closed with `emit
path not yet available` and never output fake apply code.

## Blocking phase-exit obligation

- [ ] RE-DIFF loader surface + CLI->emit subcommand surface against `emit.go` (BLOCKING phase-exit)

Before Phase 5 can be marked complete, repeat the exact probes above after
both Plan 05-01 and Plan 05-02 have landed, verify the loader's shared eval
path still satisfies every emitted state read, verify both binary subcommands
produce the expected phase-4 source, and change this checkbox to `[x]` with
the command output and commit evidence.
