# Phase 1 Spike — Validated `Manifest` JSON Shape

**Phase:** 01-spike-zero-residue-live-hot-switch
**Role:** The reversible record held between activate and deactivate — the spike's real deliverable and **Phase 4's literal input** (it becomes `model.Manifest`, serialized into the per-profile activation state).
**Status:** **VALIDATED.** Two hand-written instances of this shape (`scratch/profile_a.json` work, `scratch/profile_b.json` personal) drove a byte-identical six-class round-trip and survived ≥5 A↔B cycles + the drift-guard sub-cases under sandboxed `zsh -f` (see `01-FINDINGS.md`). **No field is missing for any admitted class.**

Field names are confirmed by the fixtures (Assumption A1 resolved); the *shape* is load-bearing and is a direct adaptation of shadowenv's production-proven `undo::Data`, extended to the zsh classes, with **every field justified by an empirically-confirmed reverse operation**.

---

## The Validated Shape

```jsonc
{
  "profile": "work",            // == git branch name (informational here; the activation key in Phase 4)
  "schema": "v1",               // forward-compat version tag (shadowenv does this)

  // ── ENV SCALARS ── drift-guarded reverse (Pattern 3). The PRESENCE vs ABSENCE of
  // the "original" key encodes unset-vs-empty (Pattern 4) — the documented shape.
  // (NOTE: the proven loader trusts the LIVE prior captured at apply time, not this
  //  static "original"; see "Loader reality" below. "original" is the manifest's
  //  declarative record of intent, which the emitter reconciles against the live prior.)
  "env": [
    { "name": "EDITOR", "applied": "nvim", "original": "vim" },   // original present -> deactivate: export EDITOR=vim
    { "name": "WORK_TOKEN", "applied": "abc" }                    // original ABSENT  -> was unset -> deactivate: unset
  ],

  // ── PATH-LIKE LISTS ── add/delete deltas vs the captured base (Pattern 1). Deactivate
  // = rebuild from the captured base + re-apply the inverse. NEVER an absolute PATH;
  // NEVER `typeset -U` (Pitfall 2). FPATH is the fpath ARRAY only — `compinit` is NOT here.
  "lists": [
    { "name": "PATH",  "additions": ["/work/bin"],         "deletions": [] },
    { "name": "FPATH", "additions": ["/work/completions"], "deletions": [] }
  ],

  // ── ALIASES ── names this profile ADDED (-> unalias on deactivate) + prior bodies of
  // names it SHADOWED (-> restore via alias name='<prior body>') (Pattern 5).
  "aliases": {
    "added":    { "gs": "git status", "ga": "git add" },   // deactivate: unalias each
    "shadowed": { "ll": "ls -lh" }                          // deactivate: alias ll='ls -lh' (prior body)
  },

  // ── FUNCTIONS ── same model as aliases; bodies captured via ${functions[name]} (Pattern 5).
  "functions": {
    "added":    ["work_deploy"],                            // deactivate: unset -f each
    "shadowed": { "ff": "\tprint -r -- prior-ff" }          // deactivate: functions[ff]='<prior body>'
  },

  // ── OPTIONS ── applied state + prior on/off, restored EXACTLY (not a blind toggle).
  // The apply path MUST be a PLAIN fn or the option auto-reverts at return (Pitfall 1).
  "options": [
    { "name": "EXTENDED_GLOB", "enabled": true, "was_on": false }  // deactivate: unsetopt (restore was_on=false)
  ]
}
```

---

## Per-Field Justification (each tied to the reverse op it enables)

| Field | Type | Reverse operation it enables | Verified by |
|-------|------|------------------------------|-------------|
| `profile` | string | None at runtime — the activation key / git branch name. Informational in the spike. | fixtures |
| `schema` | string | Forward-compat gate so a future manifest version can be detected before reverse logic runs. | shadowenv precedent |
| `env[].name` | string | Identifies the scalar to restore/unset. | FM1/FM3 |
| `env[].applied` | string | The value the profile set — the **drift-guard comparand**: reverse only if `live == applied` (Pattern 3). | FM3a/FM3b |
| `env[].original` **(present)** | string | "was set to this prior value" → deactivate `export name=<original>`. Present-with-`""` encodes was-empty (Pattern 4). | `EDITOR` (restore case) |
| `env[].original` **(absent)** | — | Absence encodes "was unset" → deactivate `unset name`. The unset-vs-empty distinction (Pattern 4). | `WORK_TOKEN`/`PERSONAL_KEY` |
| `lists[].name` | string | The array-valued var (`PATH`, `FPATH`). | FM2 |
| `lists[].additions` | string[] | Entries to prepend after rebuilding from the captured base; the inverse is dropped on deactivate by rebuilding base. **Never an absolute PATH** (Pattern 1 / Pitfall 2). | FM2 (`$#path` stable across 5 cycles) |
| `lists[].deletions` | string[] | Entries the profile removed vs base (re-added on deactivate). Empty in both fixtures; the slot is validated as present and reversible. | shape coverage |
| `aliases.added` | map name→body | Each name `unalias`'d on deactivate. | FM1 (`gs`/`ga`/`gp` gone) |
| `aliases.shadowed` | map name→prior body | Prior body restored byte-for-byte via `alias name=$prior` (Pattern 5). | `ll` shadow-restore |
| `functions.added` | string[] | Each `unset -f`'d on deactivate. | `work_deploy`/`personal_sync` gone |
| `functions.shadowed` | map name→prior body | Prior body restored via `functions[name]=$prior` (Pattern 5). | `ff` shadow-restore |
| `options[].name` | string | The option to restore. | FM1 |
| `options[].enabled` | bool | The state the profile applied (the toggle comparand). Apply must run in a **plain** fn (Pitfall 1). | options round-trip |
| `options[].was_on` | bool | Prior on/off — deactivate restores this **exactly** (not a blind toggle): `was_on=false`→`unsetopt`, `was_on=true`→`setopt`. | options round-trip |

---

## Loader Reality vs the Manifest's `original` (the load-bearing finding)

Plan 01-01's Rule-1 fix established — and this plan's FM3 hardened — a distinction Phase 4's emitter **must** carry forward:

- The manifest's `env[].original` is the **declarative record** of what the profile believes the prior was (and is what `01-MANIFEST-SHAPE.md` documents as the shape).
- The **proven loader trusts the LIVE prior** captured at apply time (`zp_capture_env`), because the byte-identical bar requires reversing to whatever the base *actually* held, and uses `${(P)+var} == 1` (not a `-n` truthiness test) to distinguish unset from empty.

**Phase-4 implication:** the emitter records the live prior into the runtime undo state at apply time and reconciles it with the manifest's declarative `original`; the drift guard (`reverse only if live == applied`) is what reconciles a hand-edited value safely.

---

## Classes With NO Representation Here (intentional — the correct outcome)

Per D-02, a class excluded by the verdict simply has **no field** in the manifest; that absence is correct, not a gap:

| Excluded item | Why it is absent | Where it lives instead |
|---------------|------------------|------------------------|
| **`compinit`** (and its side effects: `$_comps`, autoloaded `_*` functions, `.zcompdump`) | Imperative / run-once — not byte-reversible by removing an fpath entry (Pitfall 4). Completion is admittable only as fpath-membership (the `FPATH` entry in `lists`), **without** a per-switch compinit. | The unmanaged `.zshrc` **master block** (run once at startup). |
| **keybindings (`bindkey`)** | Outside the six named classes (Assumption A3); the real-`~/.zshrc` pass *reports* their presence as data but the spike does not formally admit them, so they have no manifest field in v2.0. | Master block (or a future measured class, Phase 4 decision). |
| **hooks (`precmd_functions`/`chpwd_functions`)** | Same as keybindings — outside the six classes (A3); reported as data (the reality-check found `precmd_functions` non-empty in practice). | Master block (or future). |

The `FPATH` entry in `lists` **is** present (the fpath array is byte-reversible); only the imperative `compinit` invocation is excluded.

---

## Validation Evidence Summary

- Two hand-written instances (`profile_a.json`, `profile_b.json`) of this exact shape drove the loaders to a **byte-identical** six-class round-trip (empty diff) — every admitted class's reverse op is exercised by a field above.
- The four required env cases are covered: was-unset (`WORK_TOKEN`/`PERSONAL_KEY` — no `original`), restore (`EDITOR` — `original: "vim"`), plus the drift-guard hand-edit and untouched-reverse sub-cases (FM3).
- PATH `lists` delta via `additions` (no absolute PATH, no `compinit`); shadowed alias (`ll`) and shadowed function (`ff`) carry prior bodies.
- **No field was found missing** for any admitted class — which was the spike's job to confirm (A1). This shape is ready to become `model.Manifest` in the Phase 2 IR spine.
