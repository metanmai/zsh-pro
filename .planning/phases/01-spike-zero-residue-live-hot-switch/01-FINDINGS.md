# Phase 1 Spike — Findings: Go/No-Go for Zero-Residue Live Hot-Switch

**Phase:** 01-spike-zero-residue-live-hot-switch
**Requirement:** SW-03 (frontier — live in-terminal switching, byte-identical after activate → switch → switch-back)
**Date:** 2026-06-25
**Verdict source:** every result below was produced by running the throwaway harness (`scratch/spike.zsh`) under sandboxed `zsh -f -c` on `zsh 5.9 (arm64-apple-darwin25.0)` — not asserted from prior art. The harness is git-ignored throwaway (D-08); this writeup and `01-MANIFEST-SHAPE.md` are the only durable outputs.

---

## Overall Verdict: **GO**

**go/no-go: GO** (core classes aliases/env/PATH reverse byte-identical → D-03 gate met; no NO-GO condition fires).

The core classes (**aliases, env, PATH**) reverse **byte-identical** after a full `activate A → switch to B → deactivate B` round-trip, and survive ≥5 repeated A↔B cycles with **no accumulation** and **no surviving prior-profile state**. Per D-03, GO requires only that the core classes hit the byte-identical bar; they do. **Functions** and **options** also reverse byte-identical and are admitted. **Completion** splits: the `fpath` array is byte-reversible but `compinit` is imperative → completion's compinit invocation is EXCLUDED to the unmanaged master block (a scoping outcome per D-02, not a product-killer).

The milestone-v2.0 single material unknown (SW-03) is **retired GO** — already established in Plan 01-01 and now hardened against the three failure modes that decide trustworthiness in a real, long-lived session.

---

## Per-Class Admission Verdict (D-02 / D-03)

The admission test (D-01/D-02): a class is **ADMITTED → MANAGED (switchable)** iff its section of the snapshot diff is **empty** after the full apply→switch→deactivate round-trip; a class that cannot reverse byte-identical is **EXCLUDED → unmanaged master block**.

| # | Class | Snapshot-diff evidence | Verdict | Reason |
|---|-------|------------------------|---------|--------|
| 1 | **aliases** | `##ALIASES##` section empty after round-trip; added `gs`/`ga`/`gp` gone, shadowed `ll` restored to its prior body byte-for-byte | **ADMITTED → MANAGED** | `unalias` the added; restore shadowed prior body via `alias name=$PRIOR` (Pattern 5). Core class. |
| 2 | **functions** | `##FUNCTIONS##` section empty; added `work_deploy`/`personal_sync` gone, shadowed `ff` restored via `functions[ff]=$PRIOR` | **ADMITTED → MANAGED** | `unset -f` the added; restore shadowed prior body via the `functions` assoc array (Pattern 5). |
| 3 | **env** | `##ENV##` section empty; `WORK_TOKEN`/`PERSONAL_KEY` unset (were absent), `EDITOR` restored to its captured prior | **ADMITTED → MANAGED** | Drift-guarded restore to the LIVE prior captured at apply (Pattern 3); unset-vs-empty via `${(P)+var} == 1` (Pattern 4). Core class. |
| 4 | **PATH** | `##PATH##` scalar identical; `$#path` element count stable at the pre-activation value across 5 cycles (no growth) | **ADMITTED → MANAGED** | Rebuild from captured `ZP_BASE_PATH` via plain array assignment; **never** `typeset -U`, **never** append (Pattern 1 / Pitfall 2). Core class. |
| 5 | **options** | `##OPTIONS##` section empty; `EXTENDED_GLOB`/`NO_CASE_GLOB` enabled by a profile are off again after deactivate | **ADMITTED → MANAGED** | Restore exact prior on/off (`was_on`), not a blind toggle. **Requires a PLAIN loader fn** — `emulate -L`/`LOCAL_OPTIONS` would auto-revert the apply at function return (Pattern 2 / Pitfall 1). |
| 6 | **completion** | `fpath` **array** round-trips byte-identical (plain array, same mechanism as PATH); but `compinit` populates `$_comps`, autoloads `_*` functions, and writes `.zcompdump` — none reversed by removing the fpath entry (Pitfall 4) | **SPLIT: fpath ADMITTED (as array membership); `compinit` EXCLUDED → master block** | Completion is admittable **only** as fpath-array membership *without* a per-switch `compinit`. The imperative compinit invocation is the expected exclusion (D-05, recorded as data not veto). |

**Core-class gate (D-03):** aliases ✓ env ✓ PATH ✓ — all byte-identical → **GO**. No core class failed, so no NO-GO condition fires. Options ✓ and functions ✓ are admitted on top. Completion's compinit exclusion narrows scope; it does not kill the project.

---

## Failure-Mode Results (the three trustworthiness conditions)

Each failure mode is the condition under which a live switcher becomes **unusable**. All three pass under sandboxed `zsh -f -c` (`scratch/spike.zsh`):

### FM1 — no-op round-trip is byte-identical (residue)
`activate A → switch to B (deact A; apply B) → deactivate B` leaves the six-class snapshot **byte-identical** to the pre-activation snapshot (empty `diff`). Restates SC1 as the zero-residue invariant on a clean base.
```
FM1_PASS: no-op round-trip is byte-identical (empty diff)
```

### FM2 — no accumulation across ≥5 A↔B switch cycles (PATH growth / leaked state)
After 5 repeated A↔B cycles **before** the final deactivate: `$#path` equals the pre-activation element count (PATH rebuilt from base each apply, never appended), the full six-class diff is empty, and an **explicit absence check** confirms no added alias (`gs`/`ga`/`gp`), no added function (`work_deploy`/`personal_sync`), and no profile-enabled option (`extendedglob`/`nocaseglob`) survives the final deactivate.
```
FM2_PASS: no accumulation after 5 cycles ($#path stable at 2; no surviving alias/function/option)
```

### FM3 — conditional drift guard (clobbering a hand edit)
The D-04 carve-out, proven in two sub-cases on a **managed** var (`EDITOR`, which `apply_A` actually sets):
- **3a:** the user hand-edits `EDITOR` after apply → deactivate leaves the hand value intact (drift guard fires: `live != applied` ⇒ skip restore).
- **3b:** `EDITOR` left untouched after apply → deactivate **does** restore the prior (`vim`) — proving the guard is *conditional*, not a blanket skip.
```
FM3a_PASS: hand-changed managed var survived deactivate (drift guard refused to clobber)
FM3b_PASS: untouched managed var correctly reversed to its prior (guard is conditional)
FM3_PASS: drift guard is conditional (preserves hand edits, reverses untouched)
```

---

## Real-`~/.zshrc` Reality-Check Pass (D-06 / D-07)

A **slice** (first 120 lines) of the user's real `~/.zshrc` was copied to a `mktemp` temp file (read-only source — **never written back**) and sourced **inside the sandboxed `zsh -f` process only**. The user's real shell and rc file were never mutated.

- The slice **declares 4 exported env names**; **0 changed value** in-sandbox because `zsh -f` already **inherits** those names from the parent (a live demonstration of Pitfall 5 — `-f` suppresses rc *sourcing*, not env *inheritance*).
- Because of that inheritance, the honest assertion is on the **introduced delta** (the declared names' values), not a clean-base byte-identity: the declared-name delta **round-tripped** cleanly back to the pre-source values.
```
REALZSHRC: slice declares 4 exported env name(s); 0 changed value in-sandbox (rest were inherited identically — Pitfall 5)
REALZSHRC_PASS: the introduced env delta (4 declared name(s)) round-tripped to the pre-source values
```
- **Graceful skip:** if `~/.zshrc` is absent the pass prints `REALZSHRC_SKIP` and does not fail the spike.

### Keybinding / Hook Presence (reported as DATA, not a verdict — A3 / Open Question 1)
Keybindings and hooks are **outside** the six named classes (Assumption A3), so the pass *reports* their presence for Phase 4 rather than admitting/excluding them:
```
REALZSHRC_REPORT: bindkey entries present after slice: 144 (reported as data, not admitted/excluded)
REALZSHRC_REPORT: precmd_functions=7 chpwd_functions=0 (hook state — data only)
```
**Finding for Phase 4:** the inherited interactive environment carries substantial `bindkey` state and **non-empty `precmd_functions`** (hook state is real in practice). These reflect the *inherited* base (they vary by invoking shell), not the slice — but they confirm A3's relevance: if a future profile wants keybindings/hooks switchable, that is an **unmeasured seventh/eighth class** the spike deliberately did not formally admit/exclude. Recommend Phase 4 decide whether to manage them or route them to the master block.

---

## Completion Nuance (D-05 / Open Question 2), stated explicitly

> **fpath-array switching is byte-reversible; the `compinit` invocation is imperative → completion is EXCLUDED to the unmanaged master block.**

Two different things are conflated under "completion":
1. The **`fpath` array** is a plain zsh array and reverses byte-identical via the same capture-base + rebuild mechanism as PATH. As pure fpath-membership, completion is *admittable*.
2. **`compinit`** is run-once and side-effecting — it populates `$_comps`, autoloads `_*` functions, and writes `.zcompdump`. Removing an fpath entry on deactivate does **not** undo any of that. This is exactly the imperative class the product routes to the unmanaged master block.

**Resolution (Open Question 2):** completion is admittable **only** as fpath-membership *without* a per-switch `compinit`. Whether that partial capability is worth managing is a Phase-4 / user decision; the spike's verdict is that the imperative compinit part is excluded.

---

## Loader Reference Snippet (the D-08 surviving snippet)

The `scratch/` code is **throwaway** and git-ignored; it will be deleted after this verdict. The conceptually durable loader form is preserved here as the reference for Phase 4's `core/shell/zsh/emit.go`. It encodes **both mandatory divergences** (plain loaders so options escape scope; PATH rebuilt from a captured base, no `typeset -U`) and the **drift-guarded, unset-vs-empty-correct** env reverse that Plan 01-01's Rule-1 fix established.

```zsh
# Capture the LIVE prior value (or an unset sentinel) at apply time. ${(P)+var} is
# "1" iff $var is SET (even to ""), else "0" — the only correct unset-vs-empty test.
zp_capture_env() {
  local var="$1" slot="__ZP_ORIG_$1"
  if [[ "${(P)+var}" == "1" ]]; then typeset -g "$slot"="${(P)var}"
  else typeset -g "$slot"="$ZP_UNSET_SENTINEL"; fi
}

# Drift-guarded restore: reverse ONLY if the live value still equals what we applied.
zp_restore_env() {
  local var="$1" applied="$2" slot="__ZP_ORIG_$1" prior
  [[ "${(P)+var}" == "1" && "${(P)var}" == "$applied" ]] || return 0   # user drifted it -> leave intact
  prior="${(P)slot}"
  if [[ "$prior" == "$ZP_UNSET_SENTINEL" ]]; then unset "$var"          # was unset -> unset
  else export "$var"="$prior"; fi                                       # was set (incl "") -> restore exact
}

# apply_* / deact_* are PLAIN functions — NO `emulate -L`, NO `LOCAL_OPTIONS`
# (DIVERGENCE 1: options set inside must escape function scope).
apply_A() {
  zp_capture_env EDITOR; zp_capture_env WORK_TOKEN
  export EDITOR=nvim; export WORK_TOKEN=abc
  PATH="$ZP_BASE_PATH"; path=(/work/bin $path)          # DIVERGENCE 2: rebuild from base, never typeset -U
  ZP_A_PRIOR_ALIAS_ll="${aliases[ll]}"                  # capture shadowed prior body (Pattern 5)
  alias gs='git status'; alias ga='git add'; alias ll='ls -lh'
  ZP_A_PRIOR_FUNC_ff="${functions[ff]}"
  work_deploy() { print -r -- work_deploy; }
  functions[ff]=$'\tprint -r -- profile-a-ff'
  setopt EXTENDED_GLOB                                  # escapes scope (plain fn)
}
deact_A() {
  zp_restore_env EDITOR nvim; zp_restore_env WORK_TOKEN abc
  PATH="$ZP_BASE_PATH"                                  # byte-identical PATH restore
  unalias gs 2>/dev/null; unalias ga 2>/dev/null; unalias ll 2>/dev/null
  [[ -n "$ZP_A_PRIOR_ALIAS_ll" ]] && alias ll="$ZP_A_PRIOR_ALIAS_ll"   # restore shadowed prior
  unset -f work_deploy 2>/dev/null
  if [[ -n "$ZP_A_PRIOR_FUNC_ff" ]]; then functions[ff]="$ZP_A_PRIOR_FUNC_ff"; else unset -f ff 2>/dev/null; fi
  unsetopt EXTENDED_GLOB                                # restore was_on=false
}

# Driver (the verified success-criterion sequence):
ZP_BASE_PATH="$PATH"
snapshot > S_pre
apply_A; deact_A; apply_B; deact_B    # activate A -> switch to B -> deactivate B
snapshot > S_post
diff S_pre S_post && echo "GO: byte-identical" || echo "RESIDUE"
```

---

## Verdict Pin (throwaway Go test, build-tag isolated — D-08)

The verdict is pinned under the project's real subprocess shape by `scratch/spike_test.go`, whose **first line is `//go:build spike`**. This isolates it from the default suite and the `core/testgen` oracle property pin:

```
go test ./...                  # spike file invisible; testgen oracle pin runs unchanged (verified green)
go test -tags spike ./scratch/...   # runs the pin: TestSpikeByteIdenticalRoundTrip + TestSpikeFailureModes (verified PASS)
```

The pin copies the `exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)` + 5s-timeout shape from `core/shell/zsh/introspect.go:42-53` and the `exec.LookPath("zsh")` skip-guard from `core/shell/zsh/introspect_test.go:11-13`, and asserts a **literal empty `diff S_pre S_post`** (string equality), not `IdentitySet` field checks. No `_test.go` was added to any `core/...` package.

---

## What This Retires, and What Carries Forward

- **SW-03 retired GO.** Live in-terminal switching is byte-identical across the core classes and survives repeated cycles + a hand-edit drift guard. The single material milestone risk is gone.
- **Carry-forward to Phase 4 (`core/shell/zsh/emit.go`):** (1) capture the **live prior** env value, not a static `original`; (2) use `${(P)+var} == 1` for unset-vs-empty; (3) emit **plain** loader functions (no `emulate -L`/`LOCAL_OPTIONS`); (4) rebuild PATH from a captured base, never `typeset -U`; (5) the injection threat (escaping arbitrary user values into emitted `eval`'d code, T-01-06) is deferred to Phase 4 and must be solved there.
- **Validated Manifest shape:** see `01-MANIFEST-SHAPE.md` (the second durable deliverable, Phase 4's literal input).
- **Throwaway confirmed:** `scratch/` is git-ignored; `go test ./...` and the testgen oracle pin are untouched.
