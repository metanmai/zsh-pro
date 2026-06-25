# Feature Research

**Domain:** Git-versioned, branchable shell-environment manager (each git branch = a live-switchable zsh environment profile; zero-residue hot-switch)
**Researched:** 2026-06-25
**Confidence:** HIGH (prior-art mechanics verified against official docs + source for direnv, conda, Lmod, chezmoi, home-manager, mise)

> Supersedes the prior v1.1 "PATH-hygiene" feature research. The PATH extraction/dedup work from v1.1 is re-scoped here as a prerequisite of the ingest layer (see dependencies), not as the product.

---

## Prior-Art Mechanics (what we can borrow)

This section is the evidence base. Every table-stakes / differentiator / anti-feature decision below traces to one of these concrete mechanisms. The recurring problem in all of them is the same one our v2.0 must solve: **apply declarative state to a live shell, then reverse it with zero residue.**

### direnv — the canonical zero-residue reversal (MOST RELEVANT) — HIGH

direnv is the closest mechanical analog to our switch loop, even though its trigger is `cd` rather than `checkout`.

- **Activation:** Hooks into the shell via `eval "$(direnv hook zsh)"`, which registers functions in zsh's `precmd_functions` and `chpwd_functions`. Before every prompt it runs `direnv export zsh`, which loads `.envrc` **in a bash sub-process**, captures the exported-variable diff, and prints `export`/`unset` statements the parent shell `eval`s. Crucially, *the parent process is mutated only via `eval` of generated code* — direnv itself never mutates the parent. (This is exactly our "sourced manifest, no parent-process mutation" stance.)
- **State tracking:** Three bookkeeping env vars live in the shell:
  - `DIRENV_DIR` — the directory currently loaded.
  - `DIRENV_WATCHES` — file mtimes, so it knows when `.envrc` changed and must reload.
  - `DIRENV_DIFF` — a **base64-encoded, gzip-compressed JSON** snapshot of the environment *before and after* loading. Verified decode: `(printf '\x1f\x8b\x08\x00\x00\x00\x00\x00'; echo "$DIRENV_DIFF" | base64 -d) | gzip -dc | python -m json.tool`. The JSON holds prev/next environment maps.
- **Deactivation / restore (the key mechanic):** On leaving, direnv loads `DIRENV_DIFF` and computes `diff.Reverse().Patch(env)` — it *inverts the recorded diff* and re-applies it. This restores modified vars to their old values, removes vars it added, and re-adds vars it removed. This is materially smarter than conda's snapshot-restore (below): it is a **per-variable reverse-diff**, not a blanket PATH overwrite.
- **PATH helper:** `PATH_add bin` (and `source_up`) instead of raw `PATH=...`, so subdir profiles can layer on a parent without clobbering. (`DIRENV_BACKUP` was the old name for `DIRENV_DIFF`; renamed when the format changed.)

**Borrow:** the serialized before/after diff + reverse-and-patch model. This *is* our zero-residue deactivate, generalized beyond PATH to all managed env/aliases/functions.

### conda — snapshot-and-restore, and its documented failure mode — HIGH

- **Activation:** A shell **function wrapper** (`conda`) intercepts `conda activate`/`conda deactivate`, calls Python to generate shell code, and `eval`s it. Activation prepends the env's `bin` to `PATH` and sources `$CONDA_PREFIX/etc/conda/activate.d/*`.
- **State tracking:** `CONDA_PREFIX` (active env path), `CONDA_DEFAULT_ENV` (name), `CONDA_SHLVL` (nesting depth), `CONDA_PREFIX_1` / `CONDA_PREFIX_2` … (the activation **stack** for nested envs), and `CONDA_PATH_BACKUP` (the snapshot of `PATH` taken at activate time).
- **Deactivation / restore:** Sources `deactivate.d/*`, then **restores `PATH` wholesale from `CONDA_PATH_BACKUP`**.
- **Documented failure mode (anti-pattern for us):** Because restore is a *blanket overwrite from a snapshot*, any `PATH` change made by *another* tool **after** activate but **before** deactivate is silently lost on deactivate (conda issues #8070, #2914, #3915). Also `CONDA_*` vars notoriously leak after deactivate (#13439). This is precisely the residue we promise to avoid — and the reason to prefer direnv-style reverse-diff over conda-style snapshot-overwrite.

**Borrow:** the explicit active-state vars (`*_PREFIX`, `*_SHLVL`, a numbered stack). **Avoid:** blanket snapshot-overwrite of PATH; leaking bookkeeping vars.

### environment-modules / Lmod — reference-counted PATH + a real env stack — HIGH

The HPC `module load` / `module unload` model is the most rigorous treatment of "two sources touched the same variable."

- **Activation:** `module load X` runs a modulefile whose `prepend_path("PATH", "/X/bin")` / `append_path(...)` / `setenv(...)` mutate the environment. `module unload X` **reverses each operation**: the entry prepended on load is removed on unload; `setenv` vars are unset.
- **State tracking + reference counting:** With the default `LMOD_DUPLICATE_PATHS=no`, Lmod keeps a **reference count per PATH entry**. If modules A and B both add `/A/bin`, it's stored once with refcount 2; unloading A drops it to 1 and the entry **stays** until B also unloads (refcount 0). This prevents both duplicate growth *and* premature removal of a still-needed entry. The module table + counts are serialized into the environment so they survive subshell/load cycles.
- **Env stack:** `pushenv(VAR, val)` maintains a **stack** of prior values, so nested loads restore the exact previous value on unload, not just "unset."
- **Collections:** `module save <name>` / `module restore <name>` persist a named set of loaded modules (≈ a profile). Caveat learned: a restored collection loads exactly the recorded list and **ignores the modulefiles' own `load()` dependencies** — it trusts the captured snapshot, not re-derivation.

**Borrow:** reference-counting / "who put this here" accounting for PATH, and a `pushenv`-style value stack for env vars. This is the principled answer to "the user (or their master block) already had `/usr/local/bin` on PATH — don't remove it just because a profile also added it."

### chezmoi — three-state model, diff/apply preview UX, imperative escape hatch — HIGH

chezmoi manages *files* not a live shell, but its **state model and UX are the template** for our declarative/imperative split and our reviewability.

- **Three states:** *source state* (the git repo, `~/.local/share/chezmoi`) → rendered into *target state* → reconciled against *destination state* (actual `~`). `chezmoi apply` computes the **minimum changes** to make destination match target.
- **UX commands:** `add`, `edit`, `diff` (preview target-vs-destination before touching anything), `status` (porcelain summary), `apply`, `update` (`git pull --autostash --rebase` then `apply`), `cd`, `managed`, `forget`, `re-add`. The **diff-before-apply** loop is the headline ergonomic.
- **Imperative escape hatch (directly maps to our master block):** declarative file management is the default; *scripts* are the explicit exception. Prefixes encode intent + ordering: `run_once_` (run only if this script's SHA256 hasn't succeeded before — stored in a `scriptState` bucket), `run_onchange_` (run when contents change), `before_`/`after_` (ordering relative to file application). chezmoi's own docs say *"Scripts break chezmoi's declarative approach and should be used sparingly"* and *"all scripts should be idempotent."*

**Borrow:** the three-state vocabulary, the `diff`/`status`/`apply` preview loop, and run-once-by-content-hash for the imperative bootstrap.

### Nix home-manager — generations, atomic switch, rollback, ordered activation — HIGH

home-manager is the reference for the **generation model** and **safe atomic switching**.

- **Generations as a symlink chain:** Each successful `home-manager switch` builds a new generation (numbered 1, 2, 3 …) and **atomically** repoints the `home-manager` profile symlink (via `nix-env`) to it, keeping the `home-manager-N-link` history.
- **Rollback:** `home-manager switch --rollback` runs `nix-env --rollback` then re-runs the generation's activation script. `home-manager generations` lists them with timestamps. GC roots (`~/.local/state/home-manager/gcroots/{current-home,new-home}`) pin the live + in-flight generation.
- **Ordered, safe activation:** the activation script is a **DAG** of named blocks (`hm.dag.entriesAfter`, …). A special `writeBoundary` block separates *verify-only* steps from *mutating* steps: `checkLinkTargets` runs **before** the boundary and **aborts the switch** if a managed target collides with an unmanaged file. Mutating blocks run after the boundary. A `clobber`/force option governs whether an existing target is unconditionally replaced.

**Borrow:** atomic switch with retained history, one-command rollback, and the **verify-before-mutate** guard (refuse to switch if it would stomp something unmanaged / dirty).

### asdf / mise — shims vs. activation (confirms our architecture) — HIGH

- **asdf:** routes commands through **shims** (`~/.asdf/shims/ruby` resolves the version at call time). No live env mutation; the cost is per-invocation shim overhead and a non-real `PATH`.
- **mise:** supports both shims and a **direnv-style activation** (`mise activate`, `MISE_USE_SHIMS=false`) that injects/withdraws env on `cd`. mise docs explicitly note that only the activation path gives *"real-time reflection of environment variable changes in the interactive shell"* — shims alone cannot. mise also manages per-directory env vars that are removed when you leave.

**Borrow:** confirms our architectural choice — *live activation (sourced manifest), not shims*, is required for a true env profile (aliases, functions, options can't be shimmed). Shims are an anti-feature for us.

### Cross-cutting zsh mechanics (for zero-residue) — HIGH

- **`typeset -U path`** ties the `$path` array to `$PATH` and **auto-dedupes**, so re-sourcing never grows PATH. The single most important zsh primitive for our "no PATH growth" promise, and the simplest backstop against re-activation residue.
- **PATH-growth-on-re-source is a known, classic zsh footgun** — sourcing a config twice appends duplicates unless guarded by `typeset -U` or an idempotent `add_to_path` helper. Our hot-switch re-enters this hazard *every* switch, so it must be designed against from line one.
- **zsh removal primitives we depend on:** `unalias`, `unset -f <fn>`, `unset <var>`, and option restore via `setopt`/`unsetopt` (snapshot `$options` from `zmodload zsh/parameter`). These are the deactivate verbs; our existing introspection already loads `zsh/parameter`, so the live tables are in reach.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = the product feels broken or untrustworthy.

| Feature | Why Expected | Complexity | Notes / Prior art / Ingest-engine dependency |
|---------|--------------|------------|-------|
| **`checkout <branch>` switches the active profile** | The entire pitch. Every tool here has a one-verb activate (`direnv` auto, `conda activate`, `module load`, `home-manager switch`). | HIGH | Drives a generated activate/deactivate manifest. **Depends on** ingest (must know the managed set) + git-backed profiles. |
| **Sourced shell integration; parent never mutated directly** | Universal pattern: direnv/conda generate shell code the parent `eval`s/sources. A binary can't mutate its parent's env. | MEDIUM | `eval "$(zsh-pro shell-init zsh)"` in `.zshrc` + a sourced function that `eval`s emitted `export`/`unalias`/`unset -f`. Mirrors `direnv hook zsh`. |
| **Deactivate-then-activate ordering on switch** | Switching B→C must remove B's state *before* applying C's, or residue accumulates (the conda-leak complaint, #13439). | MEDIUM | Manifest = `deactivate(prev) ; activate(next)`. Matches PROJECT.md's committed model. |
| **Restore PATH to a captured base, deduped** | Naïve append grows PATH on every switch — the #1 zsh footgun. Users expect PATH to look identical after a round-trip. | MEDIUM | Capture base PATH at hook-init; rebuild from base + profile additions each switch; `typeset -U path`. Borrows conda's `*_BACKUP` idea but **rebuild, don't blanket-overwrite**. **Depends on** PATH ingest (re-scoped v1.1 extraction/canonicalization). |
| **Reverse env/alias/function changes cleanly (zero residue)** | The core promise. direnv reverses a recorded diff; Lmod reverses each op; users will diff their env before/after and expect equality. | HIGH | Record what the profile set; on deactivate `unset`/`unalias`/`unset -f` exactly those, restoring prior values where they existed (direnv reverse-diff > conda snapshot). **Depends on** ingest categories (env/aliases/functions). |
| **Declarative/imperative split + thin bootstrap `.zshrc`** | Run-once/side-effecting init (daemons, `eval "$(starship init)"`) isn't safely reversible; every tool fences it off (chezmoi `run_once_`, conda activate.d). | MEDIUM | A minimal master block (hook + loader) stays unmanaged; only declarative state (env/alias/fn/PATH/options) is switchable. Already in PROJECT.md scope. **Depends on** ingest's declarative-vs-imperative classification. |
| **Ingest `~/.zshrc` into a categorized, regenerable representation** | Can't manage what you can't model. The existing engine's job, now the front door. | MEDIUM | **Reuses** existing parser + `Cat*` classifier directly. The manager's substrate. |
| **`status` / `current` — show active profile + what it manages** | conda shows `(envname)`; modules has `module list`; chezmoi has `status`. Users must see active state + managed set. | LOW | Read the bookkeeping env vars (active branch, managed keys) the hook sets. Cheap once state-tracking exists. |
| **`list` / `branch` — enumerate available profiles** | `conda env list`, `home-manager generations`, `module avail`. Discovery is assumed. | LOW | Thin wrapper over git branches. |
| **`diff` — preview what a switch would change before applying** | chezmoi's headline UX; switching a live shell blind is scary. Users expect "show me what this does first." | MEDIUM | Compare current managed state vs target profile's manifest. Strong trust signal; pairs with the engine's existing structured output. |
| **Graceful, reversible failure on switch (never half-applied)** | A switch dying mid-way, leaving a Frankenstein shell, is worse than not switching. home-manager verifies before `writeBoundary`. | HIGH | Either complete or roll back to prior profile; verify-before-mutate. Hard in a live shell (no transactions) → spike candidate. |
| **Portability: dynamic values stay late-bound** | A profile frozen with one machine's `$HOME`/`$(...)` is useless elsewhere. The Core Value and a stated boundary. | MEDIUM | Partial eval: resolve constants, leave `$HOME`/`$(...)`/conditionals unevaluated in the manifest. **Depends on** ingest partial-eval. |

### Differentiators (Competitive Advantage)

Features that set us apart. Aligned to the Core Value: *git-branch-as-live-profile with zero-residue hot-switch.*

| Feature | Value Proposition | Complexity | Notes / Prior art |
|---------|-------------------|------------|-------|
| **Git branch *is* the profile (not a parallel config format)** | No tool here uses real git branches as the profile axis. conda/modules use named dirs; chezmoi/home-manager use git only as *backing store* for a single linear target. We get branch/merge/diff/history/PR-review of environments for free. | HIGH | The defining bet. Each branch = a categorized store; `checkout` = switch. Git becomes the generation model (vs home-manager's hand-rolled symlink chain). |
| **Zero-residue *hot*-switch in an already-open terminal** | conda leaks `CONDA_*`; direnv only fires on `cd`; modules is HPC-batch. Live, mid-session, reversible switching with provable no-residue is the frontier none of them nail. | HIGH | PROJECT.md "frontier"; de-risk with a spike. Borrow direnv reverse-diff + Lmod refcounting + `typeset -U`. **The single most defensible feature.** |
| **Reference-counted / ownership-aware restore** | "I (or my master block) already had `/usr/local/bin`; don't yank it when I leave a profile that also added it." Only Lmod does this; direnv/conda don't. Makes zero-residue *correct*, not just *aggressive*. | HIGH | Track who-added-what (base vs profile). Reverse-diff gets most of this; refcounting handles overlap. Borrows Lmod. |
| **Ingest an *existing, real* `~/.zshrc` into branchable profiles** | direnv/chezmoi/home-manager make you *rewrite* config into their format. We adopt the messy file you already have via a real AST parser + classifier. Near-zero onboarding. | MEDIUM | **The existing engine is the moat.** Parser + classifier + secret detection already shipped & oracle-tested. |
| **Structured `diff`/`status` as an agent contract (`--json`)** | The engine already emits a typed `--json` envelope with exit codes. Extending switch/diff/status to JSON makes profiles scriptable/agent-drivable — unique here. | LOW–MEDIUM | Reuses `dto.Envelope` + exit-code contract. Cheap differentiator on existing rails. |
| **Honest classification of unsafe-to-switch entries** | "This line is imperative/side-effecting; it stays in your master block, not the profile." Surfacing *why* something isn't switchable builds trust no competitor offers. | MEDIUM | Leans on the committed classifier-precision stance (precision over recall; explicit "uncertain"). **Depends on** classifier work. |
| **One-command rollback / `checkout -` to previous profile** | home-manager has `--rollback`; for a live shell, "put it back the way it was" is gold after a bad switch. | MEDIUM | Git's own previous-ref + the deactivate manifest. Natural once switch + state-tracking exist. |
| **Secret-aware profiles** | Engine already flags `has_secrets`. Warn before committing a secret into a branch / leaking across profiles — a real footgun direnv has (people commit `.envrc` secrets). | LOW | Reuses existing secret detection at commit/ingest time. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create disproportionate problems. Documented to prevent scope creep.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Freeze/resolve dynamic values (`$HOME`, `$(...)`) at ingest** | "Make profiles fully self-contained / deterministic." | Destroys portability — the Core Value. Freezes a profile to one machine's disk/user (explicitly Out of Scope in PROJECT.md). | Partial eval: resolve constants only; keep dynamic values late-bound in the manifest. |
| **Blanket snapshot-and-overwrite of PATH on deactivate (conda-style)** | Simplest possible "restore." | Documented conda failure: silently discards PATH changes other tools made during the session (#8070); risks PATH growth. | direnv-style **reverse-diff** of only what the profile changed + refcount for overlap + `typeset -U`. |
| **Manage the imperative startup surface (make all init switchable)** | "Switch *everything*, including `eval`/daemons/plugin bootstraps." | Side-effecting run-once code isn't reversible; reversing it = undefined behavior / broken shell. Out of Scope in PROJECT.md; chezmoi explicitly fences scripts off. | Thin unmanaged master block runs imperative bootstrap once; only declarative state is branch-switchable. |
| **Shims (asdf-style) instead of live activation** | "Avoid mutating the live shell; cleaner isolation." | Shims can't represent aliases, functions, options, or arbitrary env — only executables. A *shell environment* profile fundamentally needs live activation (mise docs confirm shims can't reflect live env). | Sourced activate/deactivate manifest (the chosen model). |
| **Auto-switch profile on `cd` (direnv's trigger)** | "It's magic, like direnv." | Conflates *project* (per-directory) with *identity/environment* (work vs personal) — orthogonal axes. Surprising mid-session env changes; re-introduces direnv's parent/child `.envrc` PATH-clobber bugs. | Explicit `checkout`. (Per-dir auto-checkout could be a *much later, opt-in* layer.) |
| **Multi-shell support (bash/fish) in v2.0** | "Support my shell too." | The activation model is zsh-specific (`zmodload zsh/parameter`, `unalias`/`unset -f`, `typeset -U`, `$options`). Generalizing now dilutes the zero-residue core. Out of Scope in PROJECT.md. | zsh-only first; the `shell.Provider` seam already exists to add shells later if validated. |
| **Deep multi-file config-graph ingest (follow every `source`/`*.zsh`)** | "My config is split across 20 files / a framework (oh-my-zsh)." | Explodes the ingest surface and partial-eval/ordering complexity before the core switch loop is proven. Out of Scope in PROJECT.md. | Single entry-point ingest first; sourced-file following is a later milestone. |
| **Full env transaction/rollback engine with a daemon** | "Guarantee atomicity like a database." | A live interactive shell has no transaction primitive; a daemon/IPC layer is heavy and brittle for a single static-binary CLI (constraint: one external dep). | Verify-before-mutate + best-effort reverse-on-failure + `checkout -` rollback. Spike to find the achievable guarantee. |
| **Resolve/lint PATH precedence & overhaul classifier precision inside v2.0** | "While you're parsing PATH, also fix ordering and tighten categories." | Explicitly deferred in PROJECT.md as independent of the switch loop; bundling stalls the manager. | Keep as separate future phases; ingest needs only *correct extraction + notation dedup* (re-scoped v1.1 PATH work). |

---

## Feature Dependencies

```
[Ingest & categorize ~/.zshrc]   (existing parser + Cat* classifier — the front door)
    └──requires──> nothing new; reuses shipped engine
         │
         ├──enables──> [Partial evaluation]  (resolve constants, keep $HOME/$(...) late-bound)
         │                   └──requires──> [Ingest & categorize]
         │
         ├──enables──> [Declarative vs imperative split]  (what is switchable vs master block)
         │                   └──requires──> [Ingest & categorize] (+ classifier precision for honesty)
         │
         └──enables──> [Git-backed profiles]  (store representation; branch = profile)
                             └──requires──> [Ingest & categorize] + [Partial evaluation]
                                  │
                                  └──enables──> [checkout switches profile]
                                          └──requires──> [Sourced shell integration / hook]
                                          └──requires──> [State tracking: active branch + managed keys + base PATH]
                                                  │
                                                  ├──requires──> [Restore PATH from captured base + typeset -U]
                                                  │                   └──requires──> PATH ingest (re-scoped v1.1)
                                                  │
                                                  ├──requires──> [Reverse env/alias/fn diff on deactivate]
                                                  │
                                                  ├──enables──> [diff: preview a switch]
                                                  ├──enables──> [status / current / list]
                                                  ├──enables──> [rollback / checkout -]
                                                  │
                                                  └──hardened-by──> [Zero-residue hot-switch (FRONTIER)]
                                                          ├──enhanced-by──> [Reference-counted / ownership-aware restore]
                                                          └──de-risked-by──> a SPIKE before commit

[Shims]             ──conflicts──> [Sourced live activation]   (mutually exclusive; live activation chosen)
[Auto-switch on cd] ──conflicts──> [Explicit checkout as the identity axis]   (different axes; defer)
[Freeze dynamic values] ──conflicts──> [Partial evaluation / portability]
```

### Dependency Notes

- **Everything requires Ingest:** the manager can only manage the categorized set the existing engine produces. Ingest is the spine and the lowest-risk phase (code already exists).
- **`checkout` requires both the hook and state-tracking:** without a sourced hook there's no way to mutate the live shell; without bookkeeping vars (active branch + managed keys) deactivate can't know what to reverse. These two must land together with (or just before) the first real switch.
- **Restore-PATH requires PATH ingest:** the re-scoped v1.1 PATH extraction/canonicalization is a hard prerequisite for deduped, base-relative PATH rebuild — schedule PATH ingest before the switch loop.
- **Zero-residue hot-switch *hardens* `checkout`; it is not a separate feature you can ship instead.** A basic switch can work; making it provably residue-free is the frontier layer the reverse-diff + refcount + `typeset -U` mechanics buy you. PROJECT.md correctly flags a spike first.
- **`diff`/`status`/`rollback` all sit on top of state-tracking** and are individually cheap once the manifest + bookkeeping exist — they reuse the engine's structured output.
- **Conflicts:** shims vs live activation (mutually exclusive — we chose live); auto-`cd`-switch vs explicit `checkout` (orthogonal axes — defer auto); freeze-dynamic vs portability (freezing breaks the Core Value).

---

## MVP Definition

### Launch With (v2.0 core)

Minimum to validate "git branch = live, zero-residue zsh profile."

- [ ] **Ingest & categorize `~/.zshrc`** — reuses shipped parser/classifier; nothing to manage without it.
- [ ] **Partial evaluation (constants resolved, dynamics late-bound)** — portability is the Core Value; non-negotiable.
- [ ] **Git-backed profiles (branch = profile)** — the defining mechanism.
- [ ] **Declarative/imperative split + thin bootstrap `.zshrc`** — required to make switching *safe* (only reversible state is switchable).
- [ ] **Sourced hook + state-tracking (active branch, managed keys, captured base PATH)** — the substrate for any switch/restore.
- [ ] **`checkout <branch>` = deactivate(prev) → activate(next), PATH restored from base (`typeset -U`), env/alias/fn reverse-applied** — the headline.
- [ ] **`status` / `list`** — users must see and discover profiles (near-free once state exists).

### Add After Validation (v2.x)

Once the core switch loop is proven residue-free in real shells.

- [ ] **`diff` (preview a switch)** — trigger: users hesitate to switch blind; high trust payoff, modest cost.
- [ ] **Zero-residue hot-switch hardening (reverse-diff + refcount)** — trigger: the spike confirms the approach; promote from frontier to guaranteed.
- [ ] **`rollback` / `checkout -`** — trigger: first "that switch broke my shell" report.
- [ ] **Structured `--json` for switch/diff/status** — trigger: agent/scripting demand; cheap on existing rails.
- [ ] **Secret-at-commit warnings** — trigger: first near-miss committing a secret into a branch.
- [ ] **Verify-before-mutate guard (refuse to switch over a dirty/unmanaged state)** — trigger: first half-applied-switch incident.

### Future Consideration (post-PMF)

- [ ] **Multi-shell (bash/fish)** — defer: activation model is zsh-specific; only after zsh value is proven and the `Provider` seam is exercised.
- [ ] **Multi-file / framework (oh-my-zsh) ingest** — defer: large ingest-surface increase; single-entry-point must prove out first.
- [ ] **Optional per-directory auto-checkout** — defer: re-introduces direnv's parent/child clobber hazards; only as an opt-in layer atop a solid explicit switch.
- [ ] **PATH precedence/ordering analysis + classifier-precision overhaul** — defer: explicitly independent of the switch loop per PROJECT.md.
- [ ] **Profile merge/compose (layer base + overlay branches)** — defer: powerful (git merge of environments) but needs single-branch switch rock-solid first.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Ingest & categorize `~/.zshrc` | HIGH | LOW (reuses engine) | P1 |
| Partial evaluation (portability) | HIGH | MEDIUM | P1 |
| Git-backed profiles (branch=profile) | HIGH | MEDIUM | P1 |
| Declarative/imperative split + master block | HIGH | MEDIUM | P1 |
| Sourced hook + state-tracking | HIGH | MEDIUM | P1 |
| `checkout` switch (deactivate→activate, PATH restore) | HIGH | HIGH | P1 |
| `status` / `list` | MEDIUM | LOW | P1 |
| `diff` (preview switch) | HIGH | MEDIUM | P2 |
| Zero-residue hot-switch hardening (reverse-diff) | HIGH | HIGH | P2 (spike → P1 candidate) |
| Reference-counted / ownership-aware restore | MEDIUM | HIGH | P2 |
| `rollback` / `checkout -` | MEDIUM | MEDIUM | P2 |
| Structured `--json` for switch/diff/status | MEDIUM | LOW | P2 |
| Honest "unsafe-to-switch" classification | MEDIUM | MEDIUM | P2 |
| Secret-at-commit warnings | MEDIUM | LOW | P2 |
| Verify-before-mutate guard | MEDIUM | HIGH | P2 |
| Multi-shell (bash/fish) | LOW (now) | HIGH | P3 |
| Multi-file / framework ingest | MEDIUM | HIGH | P3 |
| Per-directory auto-checkout | LOW | MEDIUM | P3 |
| Profile merge/compose | MEDIUM | HIGH | P3 |

**Priority key:** P1 = must have for v2.0 launch · P2 = add after core validates · P3 = post-PMF / future.

---

## Competitor Feature Analysis

| Capability | direnv | conda | Lmod / env-modules | chezmoi | home-manager | asdf/mise | **Our approach** |
|------------|--------|-------|--------------------|---------|--------------|-----------|------------------|
| Activation trigger | auto on `cd` | `conda activate` | `module load` | `apply` (files) | `switch` | shims / `mise activate` | explicit **`checkout <branch>`** |
| Live shell mutation | `eval` of export diff | `eval` via shell-fn wrapper | shell evals modulefile output | n/a (files) | n/a (files/services) | shims / `cd` injection | **sourced manifest**, parent eval only |
| Restore mechanism | **reverse recorded diff** (`DIRENV_DIFF`) | snapshot overwrite (`CONDA_PATH_BACKUP`) | **per-op reverse + refcount** | re-apply target state | re-run activation / `--rollback` | drop injected env / shim | **reverse-diff + refcount + `typeset -U`** |
| Active-state tracking | `DIRENV_DIR/DIFF/WATCHES` | `CONDA_PREFIX/SHLVL/PREFIX_n` stack | encoded module table + refcounts + `pushenv` stack | `scriptState` persistent bucket | symlink generation chain + GC roots | env vars / shim dir | bookkeeping env vars (active branch, managed keys, base PATH) |
| Profile/generation model | per-dir `.envrc` | named env dirs | named collections (`module save`) | single git source state | numbered symlink generations | `.tool-versions`/`.mise.toml` | **git branches** = profiles (history/diff/merge free) |
| Preview before change | (load is the action) | no | `module --dry-run` | **`diff` / `status`** | build is the preview | no | **`diff` / `status` (+ `--json`)** |
| Imperative escape hatch | arbitrary bash in `.envrc` | `activate.d`/`deactivate.d` scripts | arbitrary Lua in modulefile | **`run_once_`/`run_onchange_`/`before_`/`after_`** | activation DAG + `writeBoundary` | n/a | **thin unmanaged master block** (declarative-only switch) |
| Ingest existing real config | no (write `.envrc`) | no | no (write modulefiles) | `add` imports files verbatim | no (write Nix) | no | **AST-parse + classify the real `~/.zshrc`** (the moat) |
| Rollback | leave dir | `conda deactivate` | `module restore` | re-`apply` prior commit | **`switch --rollback`** | edit version file | **`checkout -`** + git history |
| Safety guard | authorize (`direnv allow`) | none notable | refcount prevents over-removal | minimal-change apply | **`checkLinkTargets` before `writeBoundary`** | n/a | verify-before-mutate (P2) |

---

## Sources

direnv (activation, `DIRENV_DIFF`/`DIRENV_DIR`/`DIRENV_WATCHES`, reverse-diff restore, `PATH_add`/`source_up`):
- https://direnv.net/docs/hook.html
- https://direnv.net/man/direnv.1.html
- https://github.com/direnv/direnv/blob/master/test/show-direnv-diff.sh
- https://github.com/direnv/direnv/blob/master/internal/cmd/config.go
- https://direnv.net/CHANGELOG.html
- https://kyan.com/insights/managing-a-project-specific-path-with-direnv

conda (shell-fn wrapper, `CONDA_PREFIX`/`CONDA_SHLVL`/`CONDA_PATH_BACKUP`, snapshot-restore failure mode, var leakage):
- https://docs.conda.io/projects/conda/en/stable/dev-guide/deep-dives/activation.html
- https://docs.conda.io/projects/conda/en/latest/user-guide/tasks/manage-environments.html
- https://github.com/conda/conda/issues/8070
- https://github.com/conda/conda/issues/2914
- https://github.com/conda/conda/issues/13439

environment-modules / Lmod (load/unload reversal, reference counting for PATH, `pushenv` stack, collections):
- https://lmod.readthedocs.io/en/latest/010_user.html
- https://lmod.readthedocs.io/en/latest/077_ref_counting.html
- https://lmod.readthedocs.io/en/latest/015_writing_modules.html
- https://lmod.readthedocs.io/en/latest/050_lua_modulefiles.html

chezmoi (three-state model, diff/apply/status UX, `run_once_`/`run_onchange_`/`before_`/`after_` scripts):
- https://www.chezmoi.io/user-guide/daily-operations/
- https://www.chezmoi.io/user-guide/frequently-asked-questions/design/
- https://www.chezmoi.io/user-guide/use-scripts-to-perform-actions/
- https://deepwiki.com/twpayne/chezmoi/3.1-source-state-processing

home-manager (generations as symlink chain, atomic switch, `--rollback`, activation DAG / `writeBoundary` / `checkLinkTargets`):
- https://deepwiki.com/nix-community/home-manager/2.5-generation-and-profile-management
- https://home-manager.dev/manual/25.11/
- https://github.com/nix-community/home-manager/blob/master/modules/home-environment.nix
- https://github.com/nix-community/home-manager/blob/master/modules/files.nix

asdf / mise (shims vs activation; only activation reflects live env):
- https://mise.jdx.dev/direnv.html
- https://github.com/asdf-community/asdf-direnv/blob/master/README.md

zsh zero-residue primitives (`typeset -U`, PATH-growth-on-re-source footgun):
- https://tech.serhatteker.com/post/2019-12/remove-duplicates-in-path-zsh/
- https://dev.to/deni_sugiarto_1a01ad7c3fb/how-to-remove-duplicate-paths-in-zsh-on-macos-3l68
- https://paiml.github.io/bashrs/config/purifying.html

---
*Feature research for: git-versioned, branchable shell-environment manager (zsh; zero-residue hot-switch)*
*Researched: 2026-06-25*
