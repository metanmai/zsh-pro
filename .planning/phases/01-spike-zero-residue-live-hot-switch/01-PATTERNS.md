# Phase 1: SPIKE — Zero-Residue Live Hot-Switch - Pattern Map

**Mapped:** 2026-06-25
**Files analyzed:** 4 scratch artifacts (1 reused-as-durable instrument, 3 throwaway)
**Analogs found:** 4 / 4

> **Framing — THROWAWAY SPIKE (D-08).** These are scratch artifacts under `scratch/`, deliberately NOT wired into the engine (CONTEXT "Integration Points: None yet"). The durable outputs are the go/no-go writeup and the validated `Manifest` JSON shape — not the harness code. The snapshot instrument is the one exception: D-01 says it is REUSED to verify Phases 4–5, so it should be written as a durable reference.
>
> **The dominant pattern signal here is DIVERGENCE, not imitation.** The spike reuses the *snapshot* shape from `introspectScript`, but the apply/deactivate paths must deliberately deviate from it in two empirically-confirmed ways (the `emulate -L zsh` option-scope trap; the `typeset -U path` byte-identical failure). Every Pattern Assignment below names both the analog AND the mandatory divergence.

## File Classification

| Scratch Artifact | Role | Data Flow | Closest Analog | Match Quality |
|------------------|------|-----------|----------------|---------------|
| `scratch/spike.zsh` → `snapshot()` fn | utility (read-only introspection) | transform (state → text) | `core/shell/zsh/introspect.go` `introspectScript` (`:23-38`) | exact (extend, don't rewrite) |
| `scratch/spike.zsh` → `apply_*` / `deact_*` plain loader fns | utility (state mutation) | transform (manifest → live shell) | `core/shell/zsh/introspect.go` `introspectScript` (shape only) + `core/testgen/render.go` `render()` (`:36-51`) | role-match — MUST DIVERGE (see below) |
| `scratch/profile_a.json`, `scratch/profile_b.json` (fixture manifests) | config (fixture data) | n/a (static JSON) | `core/analyze/corpus_test.go` `manifest` struct + `core/testdata/fixtures/manifests.json` | role-match (hand-written ground truth) |
| `scratch/spike_test.go` (optional verdict pin) | test | request-response (subprocess) | `core/shell/zsh/introspect_test.go` (`:10-49`) | exact |

## Pattern Assignments

### `scratch/spike.zsh` → `snapshot()` (utility, transform) — REUSE, extend

**Analog:** `core/shell/zsh/introspect.go`, `introspectScript` const (`core/shell/zsh/introspect.go:23-38`).

This is the **durable** instrument (D-01). Copy the section-delimited `zmodload zsh/parameter` + `for k in "${(@k)...}"` shape verbatim, then extend it for the spike's exactness bar.

**Snapshot shape to copy** (`core/shell/zsh/introspect.go:23-38`):
```zsh
emulate -L zsh
zmodload zsh/parameter 2>/dev/null
source "$1" >/dev/null 2>&1
print -r -- '##ALIASES##'
for k in "${(@k)aliases}"; do print -r -- "$k"; done
print -r -- '##FUNCTIONS##'
for k in "${(@k)functions}"; do print -r -- "$k"; done
print -r -- '##ENV##'
for k v in "${(@kv)parameters}"; do [[ "$v" == *export* ]] && print -r -- "$k"; done
print -r -- '##PATH##'
for p in $path; do print -r -- "$p"; done
print -r -- '##OPTIONS##'
for k in "${(@k)options}"; do [[ "${options[$k]}" == on ]] && print -r -- "$k"; done
print -r -- '##END##'
```

**Three extensions the spike must add** (per RESEARCH "Don't Hand-Roll" + Code Examples; each empirically confirmed):
1. **Sort keys for a stable diff** — change `"${(@k)...}"` → `"${(@ok)...}"` (the `o` flag sorts). Assoc-array iteration order is unspecified; without this the before/after diff is noisy. `[RESEARCH: "Don't Hand-Roll" → Snapshot determinism; VERIFIED V2–V6]`
2. **Dump alias/function BODIES, not just names** — `print -r -- "$k=${aliases[$k]}"` — required for shadow-restore verification (Pattern 5). The analog dumps names only; the spike needs bodies. `[RESEARCH: Code Examples → snapshot instrument; V5]`
3. **Add the two missing classes for D-05's six** — a `##FPATH##` section (`for p in $fpath`) and emit env/options as name+VALUE (not name only), plus PATH as the order-exact scalar `"$PATH"`. The analog covers aliases/functions/env/path/options (5); the spike adds `fpath` (the 6th) and value-exactness.

**SAFE to keep here:** the `emulate -L zsh` header (`core/shell/zsh/introspect.go:24`) is correct in `snapshot()` because it is **read-only** and mutates nothing. This is the ONLY place in the spike where that header belongs.

---

### `scratch/spike.zsh` → `apply_*` / `deact_*` (utility, transform) — DIVERGE from the analog

**Analog (shape only):** `core/shell/zsh/introspect.go` `introspectScript` (the embedded-zsh-string idiom) + `core/testgen/render.go` `render()` (`core/testgen/render.go:36-51`, the project's string-templated zsh-codegen precedent — informational for how Phase 4 will *emit* these, not how the spike writes them by hand).

There is **no existing apply/deactivate analog in the codebase** — the engine is read-only (`core/shell/zsh` only introspects). So this artifact has no positive pattern to copy; instead it is defined by two **mandatory divergences** from the snapshot analog, both verified under zsh 5.9.

**DIVERGENCE 1 — NO `emulate -L zsh` in the apply/deactivate path (the D-05 trap).**
The snapshot analog opens with `emulate -L zsh` (`core/shell/zsh/introspect.go:24`). Copying that header into an apply function silently auto-reverts every `setopt`/`unsetopt` at function return — a **false no-go on the options class**. The apply/deactivate functions MUST be **plain** (no `emulate -L`, no `setopt LOCAL_OPTIONS`).
```zsh
# Source: RESEARCH Pattern 2 / Verification Log V1b/V4c (zsh 5.9)
checkout_bad() { emulate -L zsh; setopt EXTENDED_GLOB; }   # WRONG — option off after return
checkout_ok()  {                  setopt EXTENDED_GLOB; }   # RIGHT — option sticks
```
`[RESEARCH: Pitfall 1; Pattern 2; VERIFIED V1b/V1c/V4c — aliases/env/functions/PATH survive scope regardless; ONLY options are scope-sensitive]`

**DIVERGENCE 2 — NO `typeset -U path`; rebuild PATH from a captured base.**
Prior-art research (FEATURES.md/STACK.md) suggested `typeset -U path` as a no-growth backstop. It **fails the D-04 byte-identical bar** against a real PATH that already contains dupes (it mutates the captured base). Use capture-base + plain array assignment:
```zsh
# Source: RESEARCH Pattern 1 / Verification Log V3a/V3b/V3c (zsh 5.9)
ZP_BASE_PATH="$PATH"                         # capture ONCE, before any apply
# apply:      PATH="$ZP_BASE_PATH"; path=(/A/bin $path)   # rebuild from base, not append
# deactivate: PATH="$ZP_BASE_PATH"                         # byte-identical restore
```
`[RESEARCH: Pitfall 2; Anti-Patterns; VERIFIED V3c FAIL for typeset -U, V3a PASS for rebuild]`

**The verified full loop to copy as the canonical reference** (RESEARCH Code Examples → "The verified full loop"; this becomes the D-08 reference snippet):
```zsh
# Loader fns defined ONCE up front so they cancel in the diff (Pitfall 3).
apply_A() { export A_VAR=aval; PATH="$ZP_BASE_PATH"; path=(/A/bin $path); alias gs='git status'; depA(){ echo A; }; setopt EXTENDED_GLOB; }
deact_A() { [[ "${A_VAR-}" == "aval" ]] && unset A_VAR; PATH="$ZP_BASE_PATH"; unalias gs 2>/dev/null; unset -f depA 2>/dev/null; unsetopt EXTENDED_GLOB; }
apply_B() { export B_VAR=bval; PATH="$ZP_BASE_PATH"; path=(/B/bin $path); alias ll='ls -la'; depB(){ echo B; }; setopt NO_CASE_GLOB; }
deact_B() { [[ "${B_VAR-}" == "bval" ]] && unset B_VAR; PATH="$ZP_BASE_PATH"; unalias ll 2>/dev/null; unset -f depB 2>/dev/null; unsetopt NO_CASE_GLOB; }

ZP_BASE_PATH="$PATH"
snapshot > S_pre
apply_A; deact_A; apply_B; deact_B          # activate A → switch to B → deactivate B
snapshot > S_post
diff S_pre S_post && echo "GO: byte-identical" || echo "RESIDUE — inspect per-section"
```
`[RESEARCH: Code Examples; VERIFIED V6 byte-identical six-class round-trip]`

---

### `scratch/profile_a.json` / `scratch/profile_b.json` (config, fixture data) — the real deliverable

**Analog:** `core/analyze/corpus_test.go` `manifest` struct (`core/analyze/corpus_test.go:24-30`) + the data-driven `core/testdata/fixtures/manifests.json` it loads. The precedent is "hand-written JSON ground truth, parsed with `encoding/json`, drives the test."

These two fixtures ARE the validated `Manifest` shape — the durable output. They are NOT modeled on an existing Go struct (no `model.Manifest` exists yet); they instantiate the **proposed shape in RESEARCH** ("Proposed Manifest JSON Shape", lines 368-414). The planner should lift that JSON shape directly into the two fixtures.

**Shape to instantiate** (RESEARCH lines 372-412 — field names are a recommendation per Assumption A1; the *shape* is load-bearing):
- `env`: array of `{name, applied, original?}` — **presence/absence of `original` encodes unset-vs-empty** (Pattern 4); drift-guarded reverse (Pattern 3).
- `lists`: array of `{name, additions[], deletions[]}` for PATH/FPATH — **add/delete deltas vs the captured base, never an absolute PATH** (Pattern 1). `compinit` is NOT represented here (master block).
- `aliases`: `{added:{name:body}, shadowed:{name:priorBody}}` — prior bodies for shadow-restore (Pattern 5).
- `functions`: `{added:[name], shadowed:{name:priorBodyText}}`.
- `options`: array of `{name, enabled, was_on}` — restore exact prior on/off, not a blind toggle.

**Fixtures must exercise** (per CONTEXT D-04/D-06 — the core round-trip + the three failure modes):
1. Core round-trip (env + alias + function + option apply/reverse).
2. PATH no-growth across repeated A→B→A switches (`lists` deltas vs base).
3. Drift guard (an `env` entry whose live value diverges from `applied` must be left intact).
4. Shadow-restore (a `shadowed` alias/function whose prior body is restored byte-identical).

Parse with `encoding/json` exactly as `corpus_test.go:38-40` does (`json.Unmarshal` into a typed struct).

---

### `scratch/spike_test.go` (test, request-response) — OPTIONAL verdict pin

**Analog:** `core/shell/zsh/introspect_test.go` (`core/shell/zsh/introspect_test.go:10-49`) — the only test in the codebase that directly shells out to zsh.

If the verdict is pinned in Go (RESEARCH recommends exploring in `.zsh`, then pinning in a throwaway Go test), copy this exact subprocess shape from `core/shell/zsh/introspect.go:42-53`:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)
var out bytes.Buffer
cmd.Stdout = &out
if err := cmd.Run(); err != nil {
    return model.IdentitySet{Available: false}, err   // graceful degrade
}
```

**Skip-guard to copy** (`core/shell/zsh/introspect_test.go:11-13`) — so the test no-ops where zsh is absent:
```go
if _, err := exec.LookPath("zsh"); err != nil {
    t.Skip("zsh not installed; skipping dynamic introspection test")
}
```

**Fixture-via-TempDir to copy** (`core/shell/zsh/introspect_test.go:14-19`) — write the spike script / `~/.zshrc` slice to `t.TempDir()`, never to a real path.

**Divergences for the spike test:** (a) the `script` is the full snapshot→apply→switch→deactivate→snapshot sequence, not `introspectScript`; (b) the assertion is a literal `diff S_pre S_post` (string equality), not field checks on `IdentitySet`; (c) per D-08 this test is **throwaway** and MUST NOT touch the `testgen` oracle property test (the standing regression pin).

## Shared Patterns

### Sandboxed subprocess (`zsh -f -c` + 5s timeout + graceful degrade)
**Source:** `core/shell/zsh/introspect.go:42-53`
**Apply to:** any Go driver (`scratch/spike_test.go`); also the conceptual shape of the `.zsh` script invocation.
**Honors:** D-07 (sandboxed `zsh -f`, never mutate the user's real shell). The `-f` flag suppresses rc-file *sourcing* only — **NOTE Pitfall 5**: exported env + PATH still **inherit** across the process boundary, so the synthetic-fixture pass must set a controlled clean base (`export PATH=/usr/bin:/bin`) before `ZP_BASE_PATH="$PATH"`, while the real-`~/.zshrc` pass asserts only the delta.

### Snapshot-and-restore prior state (NOT blind clear)
**Source:** RESEARCH Pattern 3 (drift guard) + Pattern 5 (shadow restore); no codebase analog (engine is read-only).
**Apply to:** every `deact_*` operation.
```zsh
# drift guard — restore only if live still equals what we applied (Pattern 3, V3e)
[[ "${A_VAR-}" == "aval" ]] && unset A_VAR
# shadow restore — recapture prior body before override, reassign on deactivate (Pattern 5, V5)
PRIOR="${aliases[gs]}"; alias gs='git status'   # ... later: unalias gs; alias gs="$PRIOR"
```

### Unset-vs-empty exactness (`${VAR+x}`)
**Source:** RESEARCH Pattern 4 (V3d). **Apply to:** every env scalar reverse and the snapshot's `##ENV##` dump.
Honors D-04 ("unset distinct from `''`"). `${VAR+x}` → `x` iff set (even to `""`), nothing iff unset.

### Loader-fn-cancellation (avoid self-residue)
**Source:** RESEARCH Pitfall 3 (V5→V6). **Apply to:** the harness sequencing.
Define all `apply_*`/`deact_*`/`snapshot` functions BEFORE taking `S_pre` (so they appear in both snapshots and cancel in the diff), OR filter them from `##FUNCTIONS##` by name prefix. Without this, the diff falsely reports function-class residue.

## No Analog Found

Files/behaviors with no close codebase match (the codebase is a read-only analyzer; mutation + reversal are entirely new). Planner should use RESEARCH patterns, not codebase patterns, for these:

| Artifact / Behavior | Role | Data Flow | Reason |
|---------------------|------|-----------|--------|
| `apply_*` / `deact_*` loader functions | utility | transform | No existing code mutates live shell state — engine only introspects. Pattern source is RESEARCH Patterns 1–5 + Verification Log, NOT the codebase. |
| `Manifest` JSON shape (the deliverable) | config | n/a | No `model.Manifest` type exists yet. Shape source is RESEARCH "Proposed Manifest JSON Shape" (adapted from shadowenv `undo::Data`). |
| PATH delta-vs-base rebuild | utility | transform | `core/model/identityset.go` captures resolved `Path []string` read-only; it has no reversal/delta concept. |
| `core/testgen/render.go` (`:36-51`) | — | — | Listed in canonical refs as the string-templated zsh-codegen precedent, but it is **informational for Phase 4's emitter**, NOT a pattern the hand-written spike copies. The spike writes zsh by hand, not via a templated renderer. |

## Metadata

**Analog search scope:** `core/shell/zsh/`, `core/testgen/`, `core/analyze/`, `core/model/`, all `*_test.go` shelling to zsh.
**Files scanned:** introspect.go, render.go, introspect_test.go, corpus_test.go, identityset.go (5 read in full; grep across all `core/**/*_test.go` for `exec`/`zsh` to confirm introspect_test.go is the sole direct-subprocess test).
**Pattern extraction date:** 2026-06-25
