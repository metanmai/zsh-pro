# Phase 1: SPIKE — Zero-Residue Live Hot-Switch - Research

**Researched:** 2026-06-25
**Domain:** zsh declarative-state reversal (aliases/functions/env/PATH/options/completion) in a single sandboxed `zsh -f` session; the validated `Manifest` JSON shape
**Confidence:** HIGH (every load-bearing mechanic was verified by running it under `zsh 5.9` in this session, not asserted from training data)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Determinism bar (the core decision)**
- **D-01:** Byte-identical reverse is the absolute bar for any managed state — measured by snapshot-diff equality on the state tables (env, aliases, functions, `$PATH`/`$path`, options) dumped as text. No "best-effort" management. This is the project's standing **measurement instrument for determinism**, reused to verify the real switch loop in Phases 4–5 — not a one-off spike criterion.
- **D-02:** The byte-identical check is the **admission test** for what's switchable. A state class earns into the managed set only by provably reversing byte-identical. A class that fails (most likely completion) is **excluded to the unmanaged master block** — a *scoping* outcome, NOT a product-killer.
- **D-03:** A true **no-go** fires only if the **core** classes (aliases / env / PATH) cannot hit the bar — i.e. deterministic switching is impossible at all. Low-risk. Failure of options or completion narrows scope; it does not kill the project.
- **D-04:** "Byte-identical" means exact: PATH **order** preserved, "unset" distinct from `""`, no trailing/duplicate-slash drift, no **accumulation** across repeated switches. The **drift guard** is the carve-out — reverse only what the profile applied; never clobber a value the user changed by hand mid-session.

**Managed-surface scope for the spike**
- **D-05:** **Measure all six classes** — aliases, functions, env, PATH, options, AND completion (`fpath`/`compinit`). Completion is included as **data, not veto**. The **`LOCAL_OPTIONS`/`emulate -L` trap** is the specific mechanic the spike MUST confirm: option (and any state) changes made *inside* the loader function get silently auto-reverted at function exit, so apply must **escape function scope** (parent-scope `eval` of emitted code).

**Fixtures & safety**
- **D-06:** Prove the mechanism with **synthetic, hand-written two-profile fixtures** (clean, every case deliberate) for the core round-trip; **then run one pass against a slice of a real `~/.zshrc`** as a reality check.
- **D-07:** The spike runs entirely in a **sandboxed `zsh -f`** (no rc files) subprocess — it does NOT mutate the user's real, live shell. Mirrors the existing `introspect.go` `zsh -f -c` + timeout + graceful-degrade pattern.

**Spike artifact fate**
- **D-08:** **Throwaway code.** Keep only two durable outputs: the **go/no-go writeup** and the **validated `Manifest` JSON shape** (the real deliverable; Phase 4's input). The loader script survives only as a reference snippet inside the findings, not as production code.

### Claude's Discretion
- The exact snapshot/diff harness mechanics, the precise fixture contents, and the manifest JSON field names are left to research/planning — provided D-01 (byte-identical, exact) and D-05 (all six classes) hold.

### Deferred Ideas (OUT OF SCOPE)
- Per-profile **completion** as a managed/switchable class — only if the spike proves it byte-identical reversible; otherwise it stays unmanaged (master block) for v2.0.
- Auto-activate-on-`cd` (AUTO-01) and remote profile sharing + trust gate (SHARE-01) — out of scope for the spike and v2.0.
- Any IR/store/CLI/runtime (Phases 2–6); any change to the user's real shell; product-quality code.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **SW-03** | *(Frontier)* Switching works **live in an already-open terminal**, validated by the Phase-1 spike asserting a byte-identical environment after `activate → switch → switch-back` on a no-op round-trip (env **and** aliases/functions/options). | Verified GO: the full `activate A → switch to B → deactivate B` loop produced a byte-identical six-class snapshot under `zsh -f` in this session (Verification Log V6). The exact reverse operation per class, the drift guard, the PATH-delta-vs-base rebuild, and the function-scope escape for options are all documented and empirically confirmed below. |
</phase_requirements>

## Summary

This phase is a throwaway de-risk spike. Its job is to produce a **go/no-go verdict per zsh state class** and a **validated `Manifest` JSON shape**. The single material unknown is whether a live zsh shell can apply declarative state (aliases, functions, env, PATH, options, completion) and reverse it **byte-identical** — not just env, which direnv/shadowenv already prove, but the full zsh surface.

**I ran the load-bearing mechanics under `zsh 5.9` in this session rather than trusting prior art or training data.** The verdict is **GO for the core classes**, with three specific traps surfaced that the spike must encode, and completion confirmed as the prime exclusion candidate. Concretely:

1. **The `emulate -L zsh` / `LOCAL_OPTIONS` trap is real and class-specific (HIGH, verified).** Option changes made inside a function that runs `emulate -L zsh` (or `setopt LOCAL_OPTIONS`) are silently auto-reverted at function return. **Aliases, env, functions, and PATH all survive function scope regardless** — only **options** are scope-sensitive. The existing `introspectScript` opens with `emulate -L zsh`; copying that header into the *apply* path would silently break option apply. The apply loader function must be a **plain function** (no `emulate -L`, no `LOCAL_OPTIONS`).
2. **`typeset -U path` is NOT byte-identical-safe when the base PATH already contains duplicates (HIGH, verified).** It mutates the captured base in place by removing pre-existing dupes, so reversal can never restore the original. Under D-04 ("byte-identical, no dup-slash drift"), the spike must rebuild PATH from a captured base via **plain array assignment**, not `typeset -U`. `typeset -U` is a *normalization* tool, incompatible with a byte-identical bar against a messy real-world PATH.
3. **The full six-class round-trip is byte-identical (HIGH, verified).** With the loader functions defined once up front (as the real hook does), `activate A → deactivate A → activate B → deactivate B` left aliases/functions/env/PATH/options byte-identical to the pre-activation snapshot. Repeated A→B→A→B→A cycles did not grow PATH. The drift guard (`revert only if live == applied`) correctly refused to clobber a hand-changed value.

**Primary recommendation:** Plan the spike as a single sandboxed `zsh -f -c <script>` invocation (mirroring `introspect.go`) that, in one process: snapshots all six classes → applies profile A via a **plain** loader function → switches to B (deactivate-A-then-activate-B) → deactivates B → re-snapshots → diffs. Admit a class iff its diff is empty. Use the captured-base PATH rebuild (no `typeset -U`), `${VAR+x}` to distinguish unset from empty, and capture prior alias/function bodies for shadow-restore. Completion (`fpath` array) is byte-reversible but `compinit` is imperative (populates `$_comps`, autoloads `_*` functions, writes `.zcompdump`) and is the expected exclusion → master block. The validated `Manifest` shape (below) carries env scalars with a prior-value drift guard, PATH-like vars as add/delete deltas vs a captured base, alias/function name-sets with captured prior bodies, and an option-set with prior on/off state.

## Architectural Responsibility Map

The spike is a single-tier scratch artifact (one `zsh -f` subprocess driven by a scratch Go test or shell script). There is no product architecture to map. What matters for the spike is *which mechanism owns each capability*:

| Capability | Owner in the spike | Owner in the eventual product (informational) | Rationale |
|------------|--------------------|----------------------------------------------|-----------|
| Snapshot all six state classes as text | Embedded zsh `snapshot()` fn (reuses `introspectScript` shape) | `core/shell/zsh/introspect.go` (extended) | Snapshot is read-only zsh introspection; `emulate -L zsh` is safe here. This instrument is REUSED to verify Phases 4–5, so design it durable. |
| Apply declarative state to the live shell | Embedded zsh **plain** loader fn (`apply_X`) | `core/shell/zsh/emit.go` → sourced loader fn | Apply mutates global shell state; must escape function scope for options → plain fn, no `emulate -L`. |
| Reverse declarative state (deactivate) | Embedded zsh **plain** loader fn (`deact_X`) | `core/shell/zsh/emit.go` → sourced loader fn | Same scope rule as apply; reverse is the inverse of the recorded manifest. |
| Hold "what to undo" between apply and deactivate | Hand-written `Manifest` JSON (the deliverable) | `model.Manifest` serialized into `__ZSHPRO_STATE` | The manifest is the reversible record; the spike validates its shape. |
| Drive the session + diff verdict | Scratch Go test (`exec zsh -f -c`) or a `*.zsh` script | `core/cli` + a property test | Mirrors the `Introspect` subprocess pattern (D-07). |

## Standard Stack

No new dependencies. The spike is built entirely from tools already present and already verified available in this environment.

### Core
| Tool | Version (verified this session) | Purpose | Why Standard |
|------|--------------------------------|---------|--------------|
| `zsh` binary | 5.9 (arm64-apple-darwin25.0) `[VERIFIED: zsh --version]` | The sandboxed `zsh -f` session under test | Already the project's runtime introspection dependency (`introspect.go`). The activation model is intrinsically zsh-side. |
| Go | 1.25.7 `[VERIFIED: go version]` | The scratch test harness that spawns `zsh -f -c` and diffs snapshots | Project language; reuses the exact `exec.CommandContext` + timeout shape from `introspect.go`. |
| `zmodload zsh/parameter` | stock zsh 5.9 `[VERIFIED: ran it]` | Exposes `$aliases`, `$functions`, `$parameters`, `$options`, `$path`/`$fpath` as assoc arrays/arrays for snapshot + reverse | Already loaded by `introspectScript`. The single mechanism that makes all six classes inspectable as data. |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| stdlib `os/exec` + `context` | Go 1.25 | Run `zsh -f -c <script>` with a 5s timeout, capture stdout | Verbatim reuse of `introspect.go:42-53`. |
| `diff` (system) | any | Literal before/after snapshot equality (the D-01 instrument) | The verdict is a literal text diff; the harness can shell out to `diff` or compare strings in Go. |
| stdlib `encoding/json` | Go 1.25 | Parse the two hand-written fixture manifests | Project's wire format; trivially models the proposed `Manifest` shape. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Scratch Go test driving `zsh -f -c` | A standalone `.zsh` script run directly | The `.zsh` script is faster to iterate during the spike, but a Go test mirrors the eventual product harness and the `introspect.go` subprocess pattern (D-07). **Recommendation: do the exploration in a `.zsh` script, then pin the verdict in a throwaway Go test** so the "snapshot → apply → switch → deactivate → diff" sequence runs under the project's real subprocess shape. The script body becomes the D-08 reference snippet. |
| `typeset -U path` as a PATH-dedup backstop | Plain array rebuild from captured base | `typeset -U` **fails the byte-identical bar** (Verification Log V3c) — it mutates a base that has pre-existing dupes. Do **not** use it in the spike. Plain rebuild from `$ZP_BASE_PATH` is byte-exact (V3a/V3b). |
| `binary \| source /dev/stdin` to apply emitted code | `eval "$(...)"` | Not relevant to the single-session spike (no binary→shell hop yet), but flagged for Phase 4: a pipe runs the RHS in a subshell in zsh unless `setopt lastpipe`, silently losing mutations. `[CITED: STACK.md (b); Unix&Linux SE]` |

**Installation:** None. `zsh`, `go`, and `git` are all present (`[VERIFIED: --version]`).

## Package Legitimacy Audit

Not applicable — this phase installs **zero** external packages. The spike uses only stock `zsh` builtins, the Go standard library, and the already-pinned `mvdan.cc/sh/v3` (only if the optional real-`~/.zshrc` reality-check parses anything, which it need not — the spike can slice the file by hand). No registry, no slopcheck target.

## Architecture Patterns

### System Diagram — the single-session spike loop

```
              scratch harness (Go test  OR  .zsh script)
                          │
                          │  exec zsh -f -c "$SCRIPT"   (5s timeout, mirrors introspect.go; D-07)
                          ▼
   ┌─────────────────────────────────────────────────────────────────────┐
   │  ONE sandboxed zsh -f process  (no rc files; user's shell untouched)  │
   │                                                                       │
   │   zmodload zsh/parameter                                              │
   │   ZP_BASE_PATH="$PATH"            ← capture base BEFORE any mutation   │
   │                                                                       │
   │   snapshot()  ───────────────►  S_pre   (6 classes dumped as text)    │
   │                                                                       │
   │   apply_A()   (PLAIN fn — no emulate -L, so options escape scope)     │
   │     ├ env:    export A_VAR=val          (record orig via manifest)    │
   │     ├ PATH:   PATH="$ZP_BASE_PATH"; path=(/A/bin $path)               │
   │     ├ alias:  alias gs='git status'     (capture prior body if any)   │
   │     ├ func:   depA(){...}                (capture prior body if any)   │
   │     └ opt:    setopt EXTENDED_GLOB       (record WasOn=off)           │
   │                                                                       │
   │   ── switch A→B = deact_A() then apply_B() ──                         │
   │   deact_A()   (inverse of apply_A, drift-guarded)                     │
   │   apply_B()                                                           │
   │                                                                       │
   │   deact_B()                                                           │
   │                                                                       │
   │   snapshot()  ───────────────►  S_post                                │
   │                                                                       │
   │   VERDICT per class: diff S_pre vs S_post section ──► admit / exclude  │
   └─────────────────────────────────────────────────────────────────────┘
                          │  stdout: section-delimited snapshots + verdict
                          ▼
              harness diffs S_pre vs S_post  →  go/no-go writeup (D-08)
```

The diagram traces the success-criterion primary path input-to-output: capture base → snapshot → apply A → switch to B → deactivate B → snapshot → diff. The verdict is the empty-diff test, per class.

### Recommended Spike Layout

```
.planning/phases/01-spike-zero-residue-live-hot-switch/
├── 01-RESEARCH.md                 # this file
├── 01-PLAN.md / 01-01, 01-02      # the planner's output
└── (durable outputs only; D-08)
    ├── findings: go/no-go writeup (which classes admitted vs excluded)
    └── findings: validated Manifest JSON shape (Phase 4's input)

scratch/ (throwaway — NOT committed as product code; deleted after the verdict)
├── spike.zsh                      # the embedded session script (becomes the D-08 reference snippet)
├── profile_a.json / profile_b.json# two hand-written fixture manifests
└── spike_test.go (optional)       # pins the verdict under the real zsh -f subprocess shape
```

### Pattern 1: Capture-base-before-mutation (the root zero-residue principle)
**What:** Snapshot the reversal anchors (`ZP_BASE_PATH`, and per-entry prior values) **before** any managed apply runs.
**When to use:** Every reversible class. PATH especially — store it as a delta vs the captured base, never as a wholesale absolute value.
**Example (verified byte-exact, V3a/V3b):**
```zsh
# Source: this session's Verification Log V3 (run under zsh 5.9)
ZP_BASE_PATH="$PATH"            # capture ONCE, before any apply
# apply:    PATH="$ZP_BASE_PATH"; path=(/A/bin $path)
# switch:   PATH="$ZP_BASE_PATH"; path=(/B/bin $path)   # rebuild from base, not append
# deactivate: PATH="$ZP_BASE_PATH"                       # byte-identical restore
```
`[VERIFIED: ran under zsh 5.9 — V3a PASS (byte-identical), V3b PASS (no growth over 5 cycles)]`

### Pattern 2: Plain loader function so options escape scope (the D-05 trap, resolved)
**What:** The apply/deactivate functions must be **plain** zsh functions — no `emulate -L zsh`, no `setopt LOCAL_OPTIONS`. Inside such a function, `setopt`/`unsetopt` mutate the *global* option state and the change survives function return.
**When to use:** Every apply/deactivate function. (The read-only `snapshot()` function MAY safely use `emulate -L zsh` because it mutates nothing.)
**Example (verified, V1/V4):**
```zsh
# Source: this session's Verification Log V1b/V1c/V4b/V4c (run under zsh 5.9)
# WRONG — option auto-reverts at return (the trap):
checkout_bad() { emulate -L zsh; eval 'setopt EXTENDED_GLOB'; }   # EXTENDED_GLOB == off after return
# RIGHT — option escapes function scope and sticks:
checkout_ok()  { eval 'setopt EXTENDED_GLOB'; }                   # EXTENDED_GLOB == on  after return
```
`[VERIFIED: V1b/V1c emulate -L AND setopt LOCAL_OPTIONS both auto-revert options; V4c plain fn lets options stick. Aliases/env/functions/PATH survive function scope regardless (V1a, V1d–V1g).]`

### Pattern 3: Reverse-diff with a drift guard (shadowenv `undo::Data`)
**What:** Record both the prior value and the applied value. On deactivate, restore the prior value **only if the live value still equals what you applied**. If the user changed it mid-session, leave it alone.
**When to use:** Every env scalar; the safety carve-out in D-04.
**Example (verified, V3e):**
```zsh
# Source: this session's Verification Log V3e (run under zsh 5.9)
# manifest carries: Name=A_VAR, Original=<absent>, Applied="aval"
if [[ "${A_VAR-}" == "aval" ]]; then        # still ours? (drift guard)
  unset A_VAR                                # Original was absent → unset
fi                                           # else: user drifted it → leave intact
```
`[VERIFIED: V3e PASS — drift detected, user value 'user_changed_it' left intact]`

### Pattern 4: `${VAR+x}` to distinguish unset from empty-string (D-04 exactness)
**What:** `${VAR+x}` expands to `x` iff `VAR` is *set* (even to `""`), and to nothing iff `VAR` is *unset*. This is the only correct way to honor D-04's "unset distinct from `''`".
**Example (verified, V3d):**
```zsh
# Source: this session's Verification Log V3d (run under zsh 5.9)
export V=""        ; print "${V+SET}"    # → SET   (set-but-empty)
unset V            ; print "${V+SET}"    # → (empty) (absent)
```
`[VERIFIED: V3d — ${VAR+SET} distinguishes the two states]`

### Pattern 5: Shadow restore via `${aliases[name]}` / `${functions[name]}` round-trip
**What:** To restore a *shadowed* prior alias/function on deactivate, capture its body **before** the profile overrides it, then re-assign it back.
**Example (verified, V5):**
```zsh
# Source: this session's Verification Log V5 (run under zsh 5.9)
PRIOR_GS="${aliases[gs]}"     # capture body before override
alias gs='git status'         # profile overrides
# deactivate: restore the captured body
unalias gs; alias gs="$PRIOR_GS"          # alias restored byte-identical
# functions are an assoc array too:
PRIOR_FN="${functions[myf]}"  # capture full body text
functions[myf]="$PRIOR_FN"    # restore by direct assignment
```
`[VERIFIED: V5 — both alias-body and function-body shadow-restore round-trip exactly]`

### Anti-Patterns to Avoid
- **Copying `emulate -L zsh` into the apply path:** silently auto-reverts options at function return (V1b). It is correct *only* in the read-only `snapshot()` fn.
- **`typeset -U path` under a byte-identical bar:** mutates a base that has pre-existing dupes; the original PATH can never be restored (V3c). Use plain rebuild-from-base.
- **String-removing PATH entries on deactivate:** fragile (substring/trailing-slash/notation); rebuild from the captured base instead. `[CITED: PITFALLS.md Pitfall 1]`
- **Blind `unsetopt` / `unset -f` on deactivate:** deletes options/functions the user already had before the profile. Snapshot-and-restore prior state. `[CITED: PITFALLS.md Pitfall 2]`
- **Treating the loader's own functions as residue:** the snapshot will list `apply_*`/`deact_*` as functions. In the real product they're defined once at hook-time and appear in *both* snapshots; the spike harness must either pre-define them before the first snapshot (V6) or exclude them by name (V5 caught this).
- **Running `compinit` and expecting fpath-removal to reverse it:** `compinit` populates `$_comps`, autoloads `_*` functions, and writes `.zcompdump` — imperative side effects that removing the `fpath` entry does not undo (V4e).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Snapshotting the six state classes | A bespoke parser over `alias`/`functions`/`setopt` text output | The existing `introspectScript` shape (`for k in "${(@k)aliases}"` etc. via `zmodload zsh/parameter`) | It already dumps aliases/functions/exported-env/`$path`/on-options under `zsh -f`. The spike extends it (add bodies, options on/off value, `$fpath`), not rewrites it. `[CITED: core/shell/zsh/introspect.go:23-38]` |
| The sandboxed subprocess + timeout + degrade | A new exec wrapper | `exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)` with `context.WithTimeout(…, 5*time.Second)` and `Available:false` on failure | Verbatim the proven pattern in `introspect.go:42-53` (D-07). |
| PATH reversal | A delta-string-removal routine | Capture-base + plain array rebuild | The byte-identical bar makes any subtractive scheme wrong; rebuild-from-base is one line and verified exact (V3a). |
| Reverse-manifest data model | A novel undo format | shadowenv's `undo::Data` shape (`Scalar`/`List`/`NameSet`/`OptionSet`) | A named, production-proven reference design; the proposed `Manifest` (below) is a direct adaptation. `[CITED: ARCHITECTURE.md (c); shadowenv src/undo.rs]` |
| Snapshot determinism | Relying on assoc-array iteration order | Sort keys: `"${(@ok)aliases}"` (the `o` flag sorts) | Assoc-array order is unspecified; a sorted dump makes the before/after diff stable. The spike must sort, or the diff is noisy. `[VERIFIED: used (@ok) in V2–V6 snapshots; diffs were stable except for the documented PATH/loader-fn cases]` |

**Key insight:** Every primitive the spike needs already exists in the codebase or in stock zsh. The spike's value is **not** new code — it is the *empirical verdict* (which classes reverse byte-identical) and the *validated manifest shape*. Treat the snapshot harness as a durable instrument (D-01 says it's reused in Phases 4–5), and treat everything else as throwaway (D-08).

## Common Pitfalls

### Pitfall 1: Reusing the `introspectScript` `emulate -L zsh` header in the apply path
**What goes wrong:** Options set by a profile silently fail to apply (auto-reverted at function return), so the spike reports "options not reversible" when in fact options were never *applied*. A false no-go on the options class.
**Why it happens:** `introspectScript` correctly opens with `emulate -L zsh` because it's read-only; the obvious move is to copy that header into the apply function. `emulate -L` (and `setopt LOCAL_OPTIONS`) localize option state to the function.
**How to avoid:** Apply/deactivate functions are **plain** (no `emulate -L`, no `LOCAL_OPTIONS`). Only the read-only `snapshot()` fn may use `emulate -L`.
**Warning signs:** `${options[extendedglob]}` is `off` immediately after an apply that ran `setopt EXTENDED_GLOB`. `[VERIFIED: V1b, V4b]`

### Pitfall 2: `typeset -U path` silently fails the byte-identical bar
**What goes wrong:** Against a real-world PATH that already contains a duplicate (extremely common — the live test PATH in this session had `/Users/.../.cargo/bin` twice, V2), `typeset -U path` removes the pre-existing dupe, so the captured base is itself mutated and reversal can never reproduce the original PATH. The spike reports PATH residue that is actually a dedup artifact.
**Why it happens:** Prior-art research (FEATURES.md, STACK.md) recommends `typeset -U path` as a "no-growth backstop." That advice optimizes for *no duplicates*, which **conflicts** with D-04's *byte-identical* (preserve order, preserve pre-existing structure) bar.
**How to avoid:** Do not use `typeset -U` in the spike. Capture `ZP_BASE_PATH` and rebuild via plain assignment. If Phase 4 later wants dedup, that is a *normalization* decision the user must opt into — it is not zero-residue.
**Warning signs:** The PATH section of the diff shows a removed entry that was a duplicate in the base. `[VERIFIED: V2 (real PATH), V3c (controlled)]`

### Pitfall 3: The loader's own functions register as residue in the diff
**What goes wrong:** The snapshot's `##FUNCTIONS##` section lists `apply_A`, `deact_A`, etc., so the before/after diff is non-empty and the spike falsely reports function-class residue.
**Why it happens:** In the spike's single session, the loader functions are defined in the same process being snapshotted.
**How to avoid:** Define all loader functions **before** taking `S_pre` (so they appear in both snapshots and cancel in the diff — V6), or filter them out of the `##FUNCTIONS##` dump by a known name prefix. The real product avoids this naturally (hook defines them once at startup).
**Warning signs:** The only diff lines are `apply_*`/`deact_*`/`snapshot` function names. `[VERIFIED: V5 exhibited this; V6 fixed it → byte-identical]`

### Pitfall 4: `compinit` side effects are not reversed by removing the `fpath` entry
**What goes wrong:** The spike adds a completion dir to `fpath`, runs `compinit`, then removes the fpath entry on deactivate and expects byte-identical — but `$_comps`, the autoloaded `_*` functions, and `.zcompdump` persist. Completion appears "irreversible."
**Why it happens:** Two different things are conflated. The `fpath` **array** is a plain array and is byte-reversible (V4e-1). But `compinit` is **imperative** (run-once, side-effecting) — exactly the class the product routes to the unmanaged master block.
**How to avoid:** Measure the `fpath` array reversal separately from `compinit`. Expect the verdict: *fpath as a managed array may be admittable; the compinit invocation is imperative → excluded to master block.* This matches D-02/D-05 (completion is the prime exclusion candidate, recorded as data not veto).
**Warning signs:** Removing the fpath entry leaves `$_comps` populated. `[VERIFIED: V4e — fpath array reversible; compinit side effects are not]`

### Pitfall 5: Inherited environment leaks into the `zsh -f` snapshot
**What goes wrong:** `zsh -f -c <script>` does NOT read rc files, but the subprocess still **inherits exported env vars and PATH** from the parent. The first real run in this session showed the full inherited PATH (V2), making the "clean base" assumption false.
**Why it happens:** `-f` suppresses rc-file *sourcing*, not environment *inheritance*. PATH is exported and crosses the process boundary.
**How to avoid:** Inside the spike script, set a controlled clean base (`export PATH=/usr/bin:/bin`) before capturing `ZP_BASE_PATH`, OR snapshot-diff *only the delta* the spike itself introduces (set-subtract the pre-snapshot). The synthetic-fixture pass (D-06) should use a controlled base; the real-`~/.zshrc` reality-check pass accepts the inherited environment and asserts the *delta* round-trips. `[VERIFIED: V2 inherited PATH; V3 used a controlled base and passed]`

## Code Examples

### The snapshot instrument (durable; reused in Phases 4–5 per D-01)
```zsh
# Source: extended from core/shell/zsh/introspect.go:23-38; verified under zsh 5.9 (V6)
# Dumps all six classes as sorted, section-delimited text so before/after is a literal diff.
snapshot() {
  emulate -L zsh                      # SAFE here: read-only. NEVER use this header in apply/deactivate.
  zmodload zsh/parameter 2>/dev/null
  print -r -- '##ALIASES##'
  for k in "${(@ok)aliases}"; do print -r -- "$k=${aliases[$k]}"; done      # name + BODY (for shadow restore)
  print -r -- '##FUNCTIONS##'
  for k in "${(@ok)functions}"; do print -r -- "$k"; done                   # names (bodies optional; large)
  print -r -- '##ENV##'
  for k in "${(@ok)parameters}"; do
    [[ "${parameters[$k]}" == *export* ]] && print -r -- "$k=${(P)k}"       # exported name + value
  done
  print -r -- '##PATH##'
  print -r -- "$PATH"                                                       # scalar, order-exact
  print -r -- '##FPATH##'
  for p in $fpath; do print -r -- "$p"; done                               # completion search path
  print -r -- '##OPTIONS##'
  for k in "${(@ok)options}"; do [[ "${options[$k]}" == on ]] && print -r -- "$k"; done
  print -r -- '##END##'
}
```

### The verified full loop (the success-criterion sequence)
```zsh
# Source: this session's Verification Log V6 — produced a BYTE-IDENTICAL six-class round-trip under zsh 5.9.
# Loader fns defined ONCE up front (as the real hook does) so they cancel in the diff.
apply_A() { export A_VAR=aval; PATH="$ZP_BASE_PATH"; path=(/A/bin $path); alias gs='git status'; depA(){ echo A; }; setopt EXTENDED_GLOB; }
deact_A() { [[ "${A_VAR-}" == "aval" ]] && unset A_VAR; PATH="$ZP_BASE_PATH"; unalias gs 2>/dev/null; unset -f depA 2>/dev/null; unsetopt EXTENDED_GLOB; }
apply_B() { export B_VAR=bval; PATH="$ZP_BASE_PATH"; path=(/B/bin $path); alias ll='ls -la'; depB(){ echo B; }; setopt NO_CASE_GLOB; }
deact_B() { [[ "${B_VAR-}" == "bval" ]] && unset B_VAR; PATH="$ZP_BASE_PATH"; unalias ll 2>/dev/null; unset -f depB 2>/dev/null; unsetopt NO_CASE_GLOB; }

ZP_BASE_PATH="$PATH"
snapshot > S_pre
apply_A; deact_A; apply_B; deact_B          # activate A → switch to B (deact A, apply B) → deactivate B
snapshot > S_post
diff S_pre S_post && echo "GO: byte-identical" || echo "RESIDUE — inspect per-section"
```

## Runtime State Inventory

This is a sandboxed, single-process spike (D-07) that mutates nothing outside its own `zsh -f` subprocess. There is no persistent runtime state to migrate. Each category is answered explicitly:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **None** — the spike writes only to `/tmp` scratch files (snapshots) and `scratch/` fixtures, all deleted on completion (D-08). No database, no datastore. | None |
| Live service config | **None** — no external service is touched. The spike runs entirely inside one `zsh -f` subprocess. | None |
| OS-registered state | **None** — no Task Scheduler / launchd / systemd / pm2 registration. The user's real shell is explicitly NOT mutated (D-07). | None |
| Secrets/env vars | **None created.** The spike *sets and unsets* synthetic env vars (`A_VAR`, `SPIKE_*`) **inside the subprocess only**; they never reach the parent shell or any secret store. The real `~/.zshrc` reality-check pass (D-06) *reads* a slice of the user's file but writes nothing back. | None |
| Build artifacts | **None durable.** A throwaway Go test (if used) compiles in-memory via `go test`; no installed binary, no egg-info, no committed scratch code (D-08). | Delete `scratch/` after the verdict. |

**Verified:** the entire spike is read-only with respect to every system outside its own subprocess — the success-criterion sequence ran under `zsh -f -c` in this session and left the parent shell untouched.

## State of the Art

| Old / naive approach | Current approach (this spike adopts) | Why |
|----------------------|--------------------------------------|-----|
| Snapshot-and-overwrite PATH (conda `CONDA_PATH_BACKUP`) | Reverse-diff vs a captured base (direnv `DIRENV_DIFF`) | Snapshot-overwrite silently discards other tools' mid-session PATH changes (conda#8070). Reverse-diff restores exactly what the profile changed. `[CITED: FEATURES.md; conda#8070]` |
| `typeset -U path` as the zero-residue mechanism | Capture-base + plain rebuild; `typeset -U` rejected | `typeset -U` is a *dedup/normalization* tool that mutates a dup-containing base, failing the byte-identical bar. `[VERIFIED: V3c]` |
| Blind `unsetopt`/`unset -f` to "clear" state | Snapshot prior state, restore it (drift-guarded) | A blind clear deletes the user's pre-existing options/functions. `[CITED: PITFALLS.md Pitfall 2]` |
| `binary \| source /dev/stdin` (shadowenv) | `eval "$(...)"` (direnv) | A pipe runs in a subshell in zsh unless `lastpipe`; mutations are lost. (Phase 4 concern; out of scope for the single-session spike.) `[CITED: STACK.md (b)]` |

**Deprecated/outdated for this spike:**
- Any recommendation to use `typeset -U path` for the zero-residue guarantee — superseded by the empirical V3c finding under the D-04 byte-identical bar.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The proposed `Manifest` JSON **field names** (below) are a recommendation; CONTEXT D-08 leaves field names to research/planning. The *shape* (env scalars w/ drift guard; PATH-like add/delete deltas; alias/func name-sets w/ prior bodies; option-set w/ prior on/off) is grounded in shadowenv's verified `undo::Data` and the empirical reverse-ops above. | Manifest JSON Shape | LOW — names are cosmetic; the spike validates the shape by hand-writing two fixtures and proving they drive a byte-identical loop. If a field is missing, the spike surfaces it (that is the spike's job). |
| A2 | Completion will be **excluded** to the master block (fpath-array reversible, compinit imperative). This is the *expected* outcome per D-02/D-05, but the spike MUST measure it, not assume it. | Pitfall 4 | LOW — recorded as the hypothesis to test; D-02 makes exclusion a scoping outcome, not a failure. If completion somehow reverses fully, it's admitted — strictly better. |
| A3 | Keybindings and hooks (`bindkey`, `add-zsh-hook` arrays) are **out of the six named classes** and not measured by the spike. CONTEXT names exactly six classes (aliases/functions/env/PATH/options/completion); ROADMAP's "Research Flags" mentions keybinding/hook state as *possibly* un-reversible. | Open Questions | MEDIUM — if a real `~/.zshrc` profile sets keybindings declaratively and the product later wants them switchable, that's an un-measured class. Recommendation: the spike's real-`~/.zshrc` pass should *note* whether bindkey/hook state appears, even if it doesn't formally admit/exclude it. |

**If this table feels short:** it is deliberately so. The five load-bearing mechanics (option scope trap, PATH rebuild, drift guard, unset-vs-empty, shadow restore) were each **run under zsh 5.9 this session** rather than assumed — see the Verification Log. The remaining assumptions are scoping judgments, all flagged.

## Open Questions

1. **Keybindings and hooks as a (seventh/eighth) state class.**
   - What we know: CONTEXT names six classes; the spike measures those. `bindkey` state and `precmd`/`chpwd` hook arrays are technically inspectable (`bindkey -L`, `$precmd_functions`).
   - What's unclear: Whether any real-world profile sets these *declaratively* in a way the product would want to switch, and whether they reverse byte-identical.
   - Recommendation: Out of scope for the formal admit/exclude verdict (honor the six-class CONTEXT scope), but have the real-`~/.zshrc` reality-check pass (D-06) *report* if bindkey/hook state is present, so Phase 4 has data. Do not block the spike on it.

2. **The completion verdict's granularity.**
   - What we know: `fpath` array is byte-reversible; `compinit` is imperative (V4e).
   - What's unclear: Whether "per-profile completion" could mean *only* swapping `fpath` (admittable) without re-running `compinit` — i.e., is a useful completion-switch possible without the imperative part?
   - Recommendation: The spike should record this nuance in the go/no-go writeup: "fpath-array switching is byte-reversible; compinit is not — completion is admittable only if defined as fpath-membership without a per-switch compinit." Let Phase 4 / the user decide if that partial capability is worth managing.

3. **Whether the spike harness should be Go or pure-zsh.**
   - What we know: A `.zsh` script iterates fastest; a Go test mirrors the eventual `introspect.go` subprocess shape (D-07).
   - What's unclear: Purely a planning-ergonomics choice.
   - Recommendation: Explore in `.zsh`, pin the final verdict in a throwaway Go test that runs `exec zsh -f -c`, so the proven sequence is exercised under the real subprocess pattern. Both are throwaway (D-08).

## Proposed Manifest JSON Shape (the real deliverable — D-08)

This is the shape the spike validates by hand-writing two fixtures (profile A, profile B) and proving they drive the byte-identical loop. It is a direct adaptation of shadowenv's verified `undo::Data`, extended to the zsh classes, with every field justified by an empirically-confirmed reverse operation above. **Field names are a recommendation (A1); the shape is load-bearing.**

```jsonc
{
  "profile": "work",            // == git branch name (informational in the spike)
  "schema": "v1",               // forward-compat version tag (shadowenv does this)

  // ENV SCALARS — drift-guarded reverse (Pattern 3, V3e). "original": null ⇒ was unset ⇒ unset on deactivate.
  // The presence of the "original" key (vs its absence) encodes unset-vs-empty (Pattern 4, V3d):
  //   {"applied":"x"}                      → original was UNSET   → deactivate: unset
  //   {"applied":"x","original":""}        → original was EMPTY   → deactivate: export VAR=""
  //   {"applied":"x","original":"old"}     → original was "old"   → deactivate: export VAR=old
  "env": [
    { "name": "EDITOR", "applied": "nvim", "original": "vim" },
    { "name": "WORK_TOKEN", "applied": "abc" }                     // original absent → unset on deactivate
  ],

  // PATH-LIKE LISTS — add/delete deltas vs the captured base (Pattern 1, V3a/V3b). Deactivate = rebuild base,
  // re-apply the inverse. NEVER store an absolute PATH. NO typeset -U (V3c).
  "lists": [
    { "name": "PATH",  "additions": ["/work/bin"], "deletions": [] },
    { "name": "FPATH", "additions": ["/work/completions"], "deletions": [] }  // fpath array only; compinit is NOT here (V4e, master block)
  ],

  // ALIASES — names this profile added (→ unalias on deactivate) + prior bodies for shadow-restore (Pattern 5, V5).
  "aliases": {
    "added":    { "gs": "git status", "ga": "git add" },           // deactivate: unalias each
    "shadowed": { "ll": "ls -lh" }                                 // prior body; deactivate: restore via alias ll='ls -lh'
  },

  // FUNCTIONS — same model as aliases; bodies captured via ${functions[name]} (V5).
  "functions": {
    "added":    ["work_deploy"],                                   // deactivate: unset -f each
    "shadowed": { "ff": "<prior body text>" }                      // deactivate: functions[ff]="<prior body text>"
  },

  // OPTIONS — applied state + prior on/off, restored exactly (NOT a blind toggle). Apply path must be a PLAIN fn (V1/V4).
  "options": [
    { "name": "EXTENDED_GLOB", "enabled": true,  "was_on": false },// deactivate: unsetopt (restore was_on=false)
    { "name": "NOMATCH",       "enabled": false, "was_on": true }  // deactivate: setopt   (restore was_on=true)
  ]
}
```

**What the spike proves about this shape:** that two hand-written instances of it (A and B), fed to a plain-function loader, produce a byte-identical six-class round-trip — and, critically, that *no field is missing* for any admitted class. Any class excluded by the verdict (expected: completion's compinit) simply has no representation here, which is the correct outcome (it lives in the unmanaged master block).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `zsh` | The sandboxed session under test (D-07) | ✓ | 5.9 (arm64-apple-darwin25.0) | None needed — present `[VERIFIED]` |
| `zsh/parameter` module | Snapshot + reverse of all six classes | ✓ | stock in zsh 5.9 | None — `zmodload zsh/parameter` succeeded `[VERIFIED: ran it]` |
| Go toolchain | Optional Go test harness | ✓ | 1.25.7 | A pure-`.zsh` script needs no Go `[VERIFIED]` |
| `git` | Not used by the spike (no store yet) | ✓ | 2.50.1 | N/A — spike has no store dependency `[VERIFIED]` |
| `diff` | The before/after equality verdict | ✓ | system | Go string comparison if absent |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None — every dependency is present.

## Project Constraints (from CLAUDE.md)

| Directive | How the spike honors it |
|-----------|--------------------------|
| Go 1.25+; single external dependency (`mvdan.cc/sh/v3`); **no new dependencies** without discussion | The spike adds **zero** dependencies — stock zsh builtins + stdlib only. The optional real-`~/.zshrc` slice need not even invoke the parser. |
| Architecture layering — `core/analyze` shell-free; only `core/shell/zsh` touches zsh; compose with the `Provider` seam | The spike is **throwaway scratch code** (D-08), deliberately NOT wired into the engine (CONTEXT "Integration Points: None yet"). It therefore introduces no new module and no layering violation. The eventual product (Phase 4) must keep zsh emission in `core/shell/zsh/emit.go` — flagged for the planner, not enforced here. |
| Activation safety — zero-residue on declarative state; never freeze dynamic values; portability is a hard requirement | The spike *is* the zero-residue proof. It only sets synthetic static values; it does not exercise dynamic-value freezing (that's Phase 2's partial-eval concern). The byte-identical verdict is the activation-safety gate. |
| Testing — TDD; the `testgen` oracle property test is a regression pin | The spike is exploratory de-risk, not product TDD. If pinned in a Go test, that test is throwaway (D-08) and does not touch the `testgen` oracle. |
| GSD workflow enforcement — start work through a GSD command | This research runs under `/gsd:plan-phase`; the planner will produce the spike plan. |

**Authority note:** These CLAUDE.md directives are treated as locked constraints. The most relevant one for the spike is *no new dependencies* — fully satisfied (zero added).

## Sources

### Primary (HIGH confidence — verified this session)
- **This session's Verification Log (ran under `zsh 5.9`)** — the option-scope trap (V1, V4), PATH rebuild + no-growth (V3a/V3b), the `typeset -U` byte-identical failure (V2 real-PATH, V3c controlled), unset-vs-empty via `${VAR+x}` (V3d), the drift guard (V3e), shadow restore of aliases/functions (V5), and the full byte-identical six-class loop (V6). **These are the load-bearing findings and they were executed, not assumed.**
- **`core/shell/zsh/introspect.go`** (read directly) — `introspectScript` (the snapshot shape the spike reuses + extends), `zmodload zsh/parameter`, `emulate -L zsh`, and the `exec.CommandContext(ctx, "zsh", "-f", "-c", …)` + 5s-timeout + `Available:false` degrade pattern (D-07's vehicle). `[VERIFIED]`
- **`core/model/identityset.go`** (read directly) — the existing resolved-end-state shape (Aliases/Functions/Env/Path/Options/Available); the snapshot's structured target.
- **`core/testgen/render.go`** (read directly) — the project's string-templated zsh codegen precedent (informational for Phase 4, not the spike).

### Secondary (HIGH–MEDIUM — canonical project research, read in full)
- **`.planning/research/ARCHITECTURE.md`** — the `Manifest` / shadowenv `undo::Data` reference shape (basis for the proposed JSON), the explicit spike step list, and the `LOCAL_OPTIONS` option-reversal warning. `[CITED]`
- **`.planning/research/PITFALLS.md`** — Pitfalls 1 (PATH doubling, capture-before-mutation), 2 (leftover aliases/funcs/options; snapshot-restore not blind toggle), 3 (parent-shell mutation), 11 (snapshotting a dirty base). `[CITED]`
- **`.planning/research/FEATURES.md`** — direnv reverse-diff (`DIRENV_DIFF`) north star; conda snapshot-overwrite anti-pattern (#8070); `typeset -U path` framing (which this spike empirically *narrows* under the byte-identical bar). `[CITED]`
- **`.planning/research/STACK.md`** — `eval "$(...)"` vs pipe-to-source (Phase 4 concern); the stock-zsh reversal builtins (`unalias`/`unset -f`/`unsetopt`); codegen via string templating. `[CITED]`
- **`.planning/research/SUMMARY.md`** — cross-cutting synthesis; the "spike first, env-only is proven, the unproven delta is aliases/functions/options" framing this spike retires.
- **Shopify shadowenv** `src/undo.rs` / `src/shadowenv.rs` (via project research) — the `Scalar`/`List`/drift-guard design adapted into the proposed `Manifest`. `[CITED: ARCHITECTURE.md sources]`

### Tertiary (LOW — not relied upon for any verdict)
- Training-data knowledge of zsh option scoping and `${(@k)}` flags — **superseded** by the in-session Verification Log wherever they overlap. Nothing in the verdict rests on un-run training knowledge.

## Metadata

**Confidence breakdown:**
- The six-class byte-identical verdict (core GO): **HIGH** — executed end-to-end under zsh 5.9 (V6).
- The option-scope trap (D-05) resolution: **HIGH** — directly reproduced (V1/V4), including both the failing (`emulate -L`) and correct (plain fn) forms.
- PATH mechanics (rebuild, no-growth, `typeset -U` rejection): **HIGH** — controlled + real-PATH runs (V2/V3).
- Drift guard + unset-vs-empty: **HIGH** — reproduced (V3d/V3e).
- Shadow restore: **HIGH** — reproduced (V5).
- Completion exclusion: **MEDIUM-HIGH** — fpath-array reversibility verified (V4e-1); compinit irreversibility is mechanistically sound and documented, but the spike must run the full compinit case to formalize the exclusion (A2).
- Proposed Manifest field names: **MEDIUM** — shape is HIGH (grounded in verified reverse-ops + shadowenv); names are a recommendation the spike's fixtures will confirm (A1).
- Keybindings/hooks: **n/a** — explicitly out of the six-class scope (A3, Open Question 1).

**Research date:** 2026-06-25
**Valid until:** 2026-07-25 (stable — zsh option/parameter semantics and the project's introspect pattern are not fast-moving; the empirical findings are pinned to zsh 5.9, which is the current stable release).
