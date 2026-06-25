# Project Research Summary

**Project:** zsh-pro
**Domain:** Git-versioned, branchable shell-environment manager for zsh (each git branch = a live-switchable zsh profile; zero-residue hot-switch)
**Researched:** 2026-06-25
**Confidence:** HIGH

## Executive Summary

zsh-pro v2.0 turns the shipped read-only analyze engine into an **environment manager**: ingest `~/.zshrc` into a categorized, regenerable store, make each git branch an environment profile, and let `checkout <branch>` live-reload an already-open terminal into that profile with **zero residue**. This is a brownfield integration, not a greenfield build — the existing parse → classify → introspect front-end is reused as-is through the `Provider` seam, and the new manager surface (IR, git store, activation manifest, sourced runtime, partial-eval) bolts on without rewriting anything. The category that makes this product hard is not the Go plumbing; it is the runtime contract that **a tool which adds state to a live shell is only as good as its ability to remove that exact state later, on a machine it has never seen, without breaking the shell if anything goes wrong** — a failure surface a read-only analyzer never had.

All four research dimensions converge hard on the same mechanics, and the convergence is the headline finding. **(1)** The activation model is forced, not chosen: a child process cannot mutate its parent shell, so the runtime must be an **emit-and-source** loader (the binary prints eval-able shell code; a sourced function `eval`s it in the live shell) — direnv/conda/shadowenv/virtualenv all do exactly this. **(2)** Zero-residue restore is a **solved problem with a named reference design** — shadowenv's `undo::Data` / direnv's reverse-diff: record what you changed *and the prior value*, capture the base **before** any mutation, store PATH as a delta against that base (never a wholesale overwrite), and on deactivate reverse exactly what you applied, guarded by a drift check ("only restore if the live value still equals what I set"). **(3)** The **declarative-vs-imperative split is an upstream ingest gate**: only set/unset-reversible state (aliases/env/PATH/functions/options) is switchable; imperative run-once code (`eval "$(starship init)"`, daemons, `nvm use`) is irreversible by construction and must stay in a thin unmanaged `.zshrc` master block. **(4)** **Partial evaluation must stay static** — resolve only syntactically-constant literals; keep `$HOME`/`$(...)`/conditionals as verbatim late-bound text — because freezing them destroys the cross-machine portability that is the entire point of branches, and `eval`-ing config to "resolve" it is simultaneously an arbitrary-code-execution channel. **(5)** **Shell out to the `git` binary** (and reuse the existing `zsh -f` subprocess) — no new compiled dependency — mirroring the pattern the codebase already trusts.

The dominant risk is concentrated in two components — the **sourced loader** and the **manifest/switch loop** — and the unanimous, load-bearing recommendation across all four files is to **SPIKE the zero-residue live hot-switch first**, with a hand-written manifest and a hard-coded two-profile fixture, before building the IR, the git store, or the CLI. The verified shadowenv/direnv precedent says env-only zero-residue *is* feasible; the unproven delta is that zsh-pro also reverses **aliases, functions, and options** in a live, already-open terminal. If that loop cannot be made provably residue-free and path-independent, the product scope must change *before* an IR and a git store are built on top of it. The spike is ~1 phase and de-risks a possible full rewrite. After the spike, the **IR is the spine** (store, manifest, and regeneration all serialize it), so the build order is: spike → IR + partial-eval → git store → manifest builder + emit → runtime loader + CLI → polished ingest on-ramp.

## Key Findings

### Recommended Stack

**Zero net-new compiled dependencies.** Three of the four capability areas are already covered by what is in the tree plus shelling out to binaries the project already shells out to; the fourth (zsh builtins) is entirely stock zsh. The only explicit dependency decision the roadmap must make is git-binary-vs-`go-git`, and the recommendation is unambiguous: the **git binary**. `go-git` is a new module (violating the hard no-new-deps constraint) *and* is weakest at exactly the porcelain a branchable manager needs (its docs state it "lacks the main porcelain operations such as merges"; `Pull` is fast-forward-only). Shelling out adds zero modules and reuses the exact `exec.CommandContext` + timeout + composition-root seam the project already uses for `zsh -f`. See [STACK.md](STACK.md).

**Core technologies:**
- **Go 1.25 + `mvdan.cc/sh/v3` v3.13.1** (already pinned) — used for zsh **parsing only** (ingest front-end) and static AST inspection in partial-eval. Do **not** route codegen through its printer (zsh printer support is new and "not complete"; hand-built nodes are the documented source of printer panics).
- **`git` binary (shell-out)** — profile storage (`init`/`branch`/`checkout`/`commit`/`status`/`worktree`); new runtime dep of the same shape as `zsh`, degrade gracefully if absent. Parse only machine formats (`status --porcelain`, `for-each-ref --format`).
- **`zsh` binary (shell-out + sourced loader)** — existing `zsh -f` introspection (reused, extended to dump alias/function **bodies** for shadow restore) plus the new activate/deactivate runtime. All teardown primitives (`unalias`, `unset -f`, `unsetopt`, `typeset -U path`, `add-zsh-hook`, `zmodload zsh/parameter`, `emulate -L zsh`) are stock zsh.
- **Codegen = string templating** from the structured representation (the existing `core/testgen/render.go` pattern) — emit verbatim `Block.Text` for untouched statements, template only the rewritten declarative slices.
- **Eval mechanism = `eval "$(...)"`** (command substitution), **not** `binary | source /dev/stdin` — a pipe runs the RHS in a subshell in zsh unless `lastpipe` is set, silently losing env mutations.

### Expected Features

The feature landscape traces to verified prior-art mechanics: direnv (canonical reverse-diff restore), conda (snapshot-restore *and its documented failure mode*), Lmod (reference-counted PATH + env stack), chezmoi (three-state model, diff/apply preview, run-once-by-hash), home-manager (generations, atomic switch, verify-before-mutate), asdf/mise (shims-vs-activation, confirming live activation is required). See [FEATURES.md](FEATURES.md).

**Must have (table stakes):**
- **Ingest `~/.zshrc` into a categorized, regenerable representation** — reuses the shipped parser + `Cat*` classifier; the substrate for everything.
- **Sourced shell integration; parent never mutated directly** — `eval "$(zsh-pro hook)"` installs a `checkout` shell function that `eval`s emitted shell code.
- **`checkout <branch>` = deactivate(prev) → activate(next)** — the headline verb; full deactivate-then-activate, never a partial diff for v1.
- **Restore PATH to a captured base, deduped** — rebuild from base + profile entries, `typeset -U path`; never string-subtract.
- **Reverse env/alias/function changes cleanly (zero residue)** — the core promise; record-and-reverse with a drift guard.
- **Declarative/imperative split + thin bootstrap `.zshrc`** — only reversible state is switchable; imperative code stays unmanaged.
- **Partial evaluation (portability)** — constants resolved, `$HOME`/`$(...)`/conditionals late-bound. Non-negotiable; it *is* the Core Value.
- **`status` / `list`** — show active profile + managed set; enumerate branches (near-free once state-tracking exists).

**Should have (competitive differentiators):**
- **Git branch *is* the profile** — the defining bet; no competitor uses real git branches as the profile axis (branch/merge/diff/history/PR-review of environments for free).
- **Zero-residue *hot*-switch in an already-open terminal** — the single most defensible feature and the frontier; conda leaks, direnv only fires on `cd`, modules is HPC-batch.
- **Ingest an *existing, real* `~/.zshrc`** (the moat) — competitors make you rewrite config into their format; we adopt the messy file you already have via a real AST parser.
- **`diff` — preview a switch before applying** — chezmoi's headline UX; strong trust signal on the engine's existing structured output.
- **Reference-counted / ownership-aware restore** — "don't yank `/usr/local/bin` just because a profile also added it" (only Lmod does this).
- **Structured `--json` for switch/diff/status** — cheap differentiator on the existing `dto.Envelope` + exit-code rails.

**Defer (v2.x / post-PMF):**
- `rollback` / `checkout -`, secret-at-commit warnings, verify-before-mutate guard (after the core loop validates).
- **Anti-features to actively refuse:** freezing dynamic values, blanket snapshot-overwrite of PATH (conda's documented bug), shims instead of live activation, auto-switch on `cd` (conflates project with identity axis), multi-shell (bash/fish), deep multi-file/framework ingest, a full transaction/daemon engine.

### Architecture Approach

A brownfield integration that adds **three new packages** (`core/profile` = the IR + partial-eval + regeneration; `core/store` = git-as-database; `core/activate` = the switch-loop orchestration), **modifies four** (`core/model` additively with `Profile`/`Entry`/`Manifest`; `core/shell/zsh` with a new `emit.go` + body-dumping introspect; `core/cli` with new verbs; `main.go` with store wiring), and **rewrites nothing**. The IR is *additive*: today's `model.Analysis` is a lossy report (category → `[]string` of names) and cannot regenerate `.zshrc`; the new `model.Profile` is a near-lossless store anchored on the verbatim `Block.Text` the parser already captures, with structured fields layered on for diffing and a static/dynamic portability tag. The non-negotiable boundary: **only `core/shell/zsh/emit.go` ever writes `unalias`/`unset -f`/`setopt` strings** — the orchestration layer stays shell-agnostic (operates on `Manifest`), exactly as `core/analyze` stays shell-free behind the `Provider` interface, and `core/profile` is imported nowhere via the concrete provider (single composition root preserved). See [ARCHITECTURE.md](ARCHITECTURE.md).

**Major components:**
1. **`core/profile` (NEW)** — builds `model.Profile` from parsed `Block`s, runs the partial-eval pass (static/dynamic tagging, no execution), regenerates per-category `.zsh`. Pure Go over the AST; depends on `Parser`/`Classifier` interfaces only.
2. **`core/store` (NEW)** — git-as-database via the `git` binary; branch = profile, files = categories (`aliases.zsh`/`env.zsh`/`path.zsh`/`functions.zsh`/`options.zsh`/`unmanaged.zsh`). `Read`/`Commit` typed in terms of `model.Profile`.
3. **`core/activate` (NEW)** — builds a reversible `Manifest` from a target profile, diffs against the active manifest, produces a deactivate-then-activate plan. Emits a plan, never shell text.
4. **`core/shell/zsh/emit.go` (NEW, in the existing concrete provider)** — the *only* place that turns a `Manifest` into zsh apply/deactivate shell code.
5. **Sourced loader (NEW, embedded `.zsh`)** — emitted by a `hook` subcommand (mirroring the existing `introspectScript` constant); holds per-terminal active state in one env var (`__ZSHPRO_STATE`), calls the binary, `eval`s output.
6. **`model.Manifest` (NEW)** — the reversible undo record (shadowenv's `undo::Data` adapted): `Scalar{Name, Original, Applied}` for env, `ListDelta{Additions, Deletions}` for PATH-like vars, `NameSet{Added, Shadowed}` for aliases/functions, `OptionSet{WasOn}` for options.

### Critical Pitfalls

All eleven critical pitfalls are corroborated by named prior-art post-mortems (conda, direnv, Environment Modules, virtualenv, NixOS, chezmoi). The top cluster: see [PITFALLS.md](PITFALLS.md).

1. **PATH accumulation/doubling on re-source and switch** — never string-remove. Capture a base PATH **once, before any managed mutation**, store it in `ZP_BASE_PATH`, and rebuild from it on every switch; `typeset -U path` is the backstop, not the mechanism. (conda#11021, #8070.)
2. **Leftover aliases/functions/options after switch** — generate an explicit deactivate manifest that is the *inverse* of activate; switch = full deactivate(prev) → activate(next), never a partial diff for v1; snapshot-and-restore options (blind `unsetopt` deletes options the user already had); capture prior function/alias bodies before overriding (blind `unset -f` deletes the base version).
3. **Trying to mutate the parent shell from a child process** — architectural invariant: `checkout` is a *sourced shell function* that `eval`s the binary's stdout; the binary only ever prints shell code. Fix in design, not code review.
4. **Imperative side-effecting startup code that can't be un-run** — make the declarative/imperative split a hard classification gate at ingest; default to "unmanaged" when unsure (precision over recall); a misclassified imperative line in the switchable set is a zero-residue violation by construction.
5. **Freezing late-bound values destroys portability** — partial-eval resolves only provably-static constants; `$HOME`/`$(...)`/`~`/conditionals stay verbatim. Treat them as a "do not touch" set; do **not** reuse `util.ExpandHome` inside the IR. Verify with a two-machine (`$HOME`/`$USER`) round-trip test.
6. **Loader slowness on the hot path** — the loader runs on *every* shell start; pre-compile the active profile to a static `activate.zsh` and `source` it. **Zero subprocesses on the hot path** (no `git`, no binary, no `$(...)`); the binary runs only at cold `checkout`/regenerate time. Budget: < ~15–20ms over bare zsh; measure with `hyperfine`.
7. **Loader/tool failure breaks the user's shell** — the stub must be defensive and **fail-open** (`command -v ... || return`, `[[ -r $manifest ]] || return`, `zsh -n`-validate generated manifests, keep a last-good fallback, ship a `ZSHPRO_DISABLE=1` escape hatch). A broken loader on the startup path can lock the user out of a working terminal.
8. **Safety of running/regenerating arbitrary config** — keep partial-eval *static* (never `eval` user config — it's an ACE channel and a portability hazard); adopt a direnv-style trust/allow gate before `eval`-ing a profile whose content changed or came from a `git pull`; hash manifests; keep `zsh -f` + timeout for any introspection.

(Also critical: **per-shell state, not a global file** — active profile lives in `__ZSHPRO_STATE` in *that* shell, never a shared `~/.config/.../active` (cross-terminal drift, write races, hybrid new-tab envs); **own one BEGIN/END-marked region of `.zshrc`** idempotently, never the whole file (conda#8703 append bug); **capture the base at the right moment** — early, once per base shell, guarded against re-capture so a child never promotes a dirty inherited PATH to "base".)

## Implications for Roadmap

Based on the convergent research, the suggested phase structure. The IR is the spine; the spike comes before it because it can invalidate scope and informs the manifest shape. This mirrors the build order all four files independently arrive at.

### Phase 0: SPIKE — Zero-Residue Live Hot-Switch
**Rationale:** The frontier risk PROJECT.md explicitly calls for de-risking. Independent of all Go plumbing; the one thing that can invalidate the whole product. Verified shadowenv/direnv precedent covers env-only; the unproven delta is reversing aliases/functions/options in a *live* terminal.
**Delivers:** Two hand-written `Manifest` JSON files + a hand-written loader (`activate`/`deactivate`/`checkout` + `__ZSHPRO_STATE`). Proves `activate A → activate B (auto-deactivates A) → deactivate B` in an already-open terminal, asserting **byte-identical** `$aliases`/`$functions`/`$PATH`/`$path`/env/`$options` before-and-after (via `zmodload zsh/parameter`). Stresses the three failure modes: PATH growth, stale aliases/functions, drifted env (drift guard must not clobber).
**Addresses:** Zero-residue hot-switch (frontier).
**Avoids:** Pitfalls 1, 2, 3, 11 (and validates the design before any commitment to the manifest format).

### Phase 1: IR + Partial Evaluation
**Rationale:** The store, manifest, and regeneration all serialize a profile, so the IR lands right after the spike validates *what the manifest must contain*. Lowest-risk Go work (front-end already exists); a natural new oracle target.
**Delivers:** `model.Profile`/`Entry`, `core/profile/profile.go` (build IR from `Block`s), `parteval.go` (static/dynamic tagging — no execution), `regenerate.go` (per-category `.zsh` anchored on verbatim `Raw`).
**Uses:** `mvdan.cc/sh` AST (static inspection only), the reused `Parser`/`Classifier` seam.
**Implements:** The `core/profile` component.
**Avoids:** Pitfall 5 (portability — keep dynamics late-bound), Pitfall 9 (static-only, no `eval`), Pitfall 4 (classification routes imperative → unmanaged bucket).

### Phase 2: Git-Backed Store
**Rationale:** `checkout` is meaningless without profiles to switch between; the store must serialize *something*, so it follows the IR. Isolated subprocess-over-`git`, integration-tested against a temp repo.
**Delivers:** `core/store` (`Init`/`Branches`/`Current`/`Checkout`/`Read`/`Commit`); repo layout (branch=profile, category files, baseline branch = ingested `~/.zshrc`); secret-exclusion default (keep `CatSecrets` out of the synced tree).
**Uses:** `git` binary via `os/exec` + timeout (the decided no-new-deps choice).
**Implements:** The `core/store` component.
**Avoids:** Pitfall 6 (own one marked region of `.zshrc` idempotently; never write profile bodies back into it).

### Phase 3: Manifest Builder + Emit
**Rationale:** Turns a profile into the reversible record the runtime applies. The spike has proved the design; the IR resolves a profile. Extends the existing introspect to dump alias/function bodies (for shadow restore).
**Delivers:** `model.Manifest` (+ `Scalar`/`ListDelta`/`NameSet`/`OptionSet`), `core/activate` (build manifest, diff active-vs-target, deactivate-then-activate plan), `core/shell/zsh/emit.go` (manifest → zsh shell code — the *only* place zsh syntax lives), extended `introspect.go`.
**Uses:** Reused `zsh -f` introspection (extended); the shadowenv `undo::Data` reference design.
**Implements:** `core/activate`, the emit boundary.
**Avoids:** Pitfalls 1, 2 (delta-vs-base PATH, captured-prior-value reverse with drift guard), Anti-Pattern 6 (no zsh strings in the orchestration layer).

### Phase 4: Runtime Loader + CLI Commands
**Rationale:** Composes everything; by now the manifest design is proven and profiles exist. The user-facing verb surface and the live-terminal wiring.
**Delivers:** Embedded `loader.zsh` emitted by `hook`; CLI verbs (`hook`, `checkout`, `activate`, `deactivate`, `list`, `status`); per-terminal `__ZSHPRO_STATE`; fail-open stub with `ZSHPRO_DISABLE`; `zsh -n` validation of generated manifests + last-good fallback; pre-compiled static `activate.zsh` for the hot path.
**Uses:** `eval "$(...)"` loader pattern; `add-zsh-hook` (only if a precmd re-assert is later added).
**Implements:** The sourced runtime + CLI command group.
**Avoids:** Pitfalls 3, 7, 8, 10, 11 (parent-mutation invariant, hot-path performance budget, fail-open, per-shell state + normalize-on-startup, base-capture placement/guard).

### Phase 5: Ingest End-to-End (the on-ramp)
**Rationale:** Best last — it is the polished on-ramp, and doing it last means it targets the *final* IR shape rather than chasing a moving target. (`~/.zshrc` → IR → baseline branch committed.)
**Delivers:** The full ingest path: parse real `~/.zshrc` → classify (declarative/imperative split) → partial-eval → regenerate → commit to baseline branch; idempotent managed-block install; out-of-block installer-append detection/warning.
**Addresses:** Ingest & categorize (table stakes), the moat (adopt the real file).
**Avoids:** Pitfall 6 (idempotent block writer — install twice → identical file), Pitfall 4 (imperative lines routed to the master block, never silently dropped).

### Phase Ordering Rationale

- **Spike first** because the zero-residue live loop is the single risk that can invalidate the product, it is independent of all Go plumbing, and it *defines* the manifest contents — building the IR/store/manifest before proving the loop risks a full rewrite (all four files agree).
- **IR is the spine** — store, manifest, and regeneration all serialize `model.Profile`, so it lands immediately after the spike. The alternative (store before IR) was explicitly considered and rejected: `Store.Read`/`Commit` are typed in terms of `model.Profile`, so building git plumbing first means designing against a placeholder and reworking it.
- **Store before manifest-emit** — `checkout` needs profiles to switch *between* before there's anything to make reversible.
- **Runtime last among the build phases** because it composes everything; by then the design is spike-proven and profiles exist.
- **Two upstream gates are correctness-critical and must land before the manifest is designed:** declarative/imperative classification (Phase 1, Pitfall 4) and partial-eval staticness/portability (Phase 1, Pitfalls 5/9) — if either is wrong, the switch loop is *unfixable downstream*.
- **Risk is concentrated in two components:** the sourced loader (Pitfalls 3, 7, 8, 11) and the manifest/switch loop (Pitfalls 1, 2, 10) — which is precisely why the spike targets both before commitment.

### Research Flags

Phases likely needing deeper research during planning (`/gsd:plan-phase --research-phase <N>`):
- **Phase 0 (SPIKE):** This *is* the research — but plan it as a real spike with explicit kill-criteria. The unproven delta (reversing aliases/functions/**options** + completion/keybinding/hook state in a live terminal) has no direct prior-art guarantee; surface whatever zsh state turns out to be un-cleanly-reversible early.
- **Phase 3 (Manifest + Emit):** Shell-code emission is where quoting/escaping bugs become shell-injection bugs. The `${aliases[name]}` / `functions`-assoc-array body extraction and the exact deactivate ordering warrant verification against the zsh manual + the shadowenv source during planning. Round-trip stability of codegen (`parse(generate(parse(src))) == parse(src)`) needs an oracle-extension design.
- **Phase 4 (Loader):** The fail-open stub, `ZSHPRO_DISABLE` recovery, idempotent BEGIN/END block writer, and the hot-path performance budget (`hyperfine`) are each subtle; the trust/allow gate for pulled profiles (Pitfall 8/security) may need its own design pass if shared/team stores are in view.

Phases with standard patterns (skip research-phase):
- **Phase 1 (IR + partial-eval):** Reuses the shipped parser/classifier and the established `render.go` codegen pattern; the partial-eval rule is well-specified (static-only). Pure-Go, oracle-testable.
- **Phase 2 (Git store):** Shelling out to `git` mirrors the existing `zsh -f` subprocess pattern exactly; machine-format parsing (`--porcelain`, `for-each-ref --format`) is well-documented and version-stable.
- **Phase 5 (Ingest):** Composes already-built pieces along a known data flow; the only new subtlety (idempotent managed-block) is covered in Pitfall 6.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every recommendation buildable with the existing dep set + shelling out; the one optional new dep (`go-git`) is flagged with explicit rejection rationale. Versions verified against upstream releases (mvdan/sh v3.13.1, go-git v5.19.1). |
| Features | HIGH | Prior-art mechanics verified against official docs + source for direnv, conda, Lmod, chezmoi, home-manager, mise. Every table-stakes/differentiator/anti-feature traces to a concrete mechanism. |
| Architecture | HIGH | Existing engine read directly from source; runtime mechanics verified against shadowenv/direnv/chezmoi source and the zsh manual. The IR/manifest/emit boundaries follow the codebase's own established discipline. |
| Pitfalls | HIGH | All 11 critical pitfalls corroborated by named prior-art post-mortems (conda issues, direnv issues, Modules, NixOS, chezmoi). Each carries a verification test and a phase mapping. |

**Overall confidence:** HIGH

### Gaps to Address

- **Zero-residue feasibility for the alias/function/option/completion delta** — the *single material unknown*. shadowenv/direnv prove env-only zero-residue in a live terminal, but zsh-pro adds aliases, functions, options (and possibly completion/keybinding/hook state). **Handle in Phase 0:** the spike must explicitly snapshot all of `$aliases`/`$functions`/`$options` (+ keybindings/completion if reachable) and assert byte-identical restore; if some state class is un-cleanly-reversible, narrow the managed set *before* designing the manifest. Define kill-criteria up front.
- **`$path` ordering on removed-then-readded entries** — shadowenv documents that re-insertion *position* of a delta-restored PATH entry is best-effort. Likely acceptable (exact position rarely load-bearing). **Handle in Phase 3:** decide and document the ordering contract; assert dedup + presence, not exact index, in tests.
- **Secret handling in the synced tree** — no-new-deps forbids age-style encryption, so the safe v2.0 default is *exclude* `CatSecrets` from the committed tree (git-ignored sidecar or `--include-secrets` gate). **Handle as a Phase 2 roadmap decision** — confirm the default with the user; reuse the shipped secret detection.
- **Trust model for shared/`git pull`'d profiles** — a git-backed store *will* pull other people's code, crossing a trust boundary; `eval`-ing a pulled manifest is an ACE channel. **Handle in Phase 4 (or defer if v2.0 is single-user-only):** decide whether a direnv-style trust-on-first-use gate is in v2.0 scope or a later milestone — flag for the requirements step.
- **`Block` AST exposure for partial-eval** — partial-eval needs to know "does this value have dynamic parts?", which the current `Block` doesn't surface. **Handle in Phase 1:** choose between (option 1) computing a static/dynamic flag in `parse.go` where the AST is in scope, or (option 2) re-parsing each entry's `Raw` in `core/profile`. Recommendation leans option 1 for the flag, option 2 only if deeper structural extraction is needed.

## Sources

### Primary (HIGH confidence)
- **Existing codebase** (read directly) — `core/shell/zsh/{parse,classify,introspect}.go`, `core/model/{block,analysis,category,identityset}.go`, `core/analyze/analyzer.go`, `core/cli/cli.go`, `core/shell/provider.go`, `core/testgen/render.go`, `core/util/path.go`, `core/cmd/zsh-pro/main.go`. The seam, the verbatim `Block.Text`, the `zsh -f` subprocess + graceful degradation, the string-templating codegen.
- **Shopify shadowenv** (source) — the reference reversible-manifest design: `src/undo.rs` (`Scalar`/`List`/`Data`), `src/shadowenv.rs` (`unshadow` drift guard), `sh/shadowenv.zsh.in` (emit-and-source hook), schema versioning. Closest structural analog. https://github.com/Shopify/shadowenv
- **direnv** — reverse-diff restore (`DIRENV_DIFF` = base64+gzip JSON; `diff.Reverse().Patch(env)`), per-shell hook, `eval "$(direnv hook zsh)"`, `PATH_add`. https://direnv.net/ , https://github.com/direnv/direnv
- **`mvdan/sh`** (Context7 + releases) — v3.13.1 latest; zsh parser+formatter landed v3.13.0 ("support is not complete"); printer position/nil panics fixed across v3.2.2/v3.9.0. https://github.com/mvdan/sh/releases
- **go-git** (releases + pkg.go.dev) — v5.19.1 stable, v6 alpha; "lacks the main porcelain operations such as merges"; `Pull` fast-forward-only. https://github.com/go-git/go-git
- **zsh manual** — `zmodload zsh/parameter` (`$aliases`/`$functions`/`$options`), `${aliases[name]}` body, `unalias`/`unset -f`/`unsetopt`, `typeset -U path`, `add-zsh-hook`, `emulate -L zsh`. https://zsh.sourceforge.io/Doc/Release/
- **conda** — activation deep-dive (shell-fn wrapper, `# >>> conda initialize >>>` block), and the documented failure post-mortems: #11021 (PATH not reset / capture-before-mutate), #8070 (`CONDA_RESTORE_PATH`), #8703 (non-idempotent init block), #9911 (concurrent-activation race), #13439 (`CONDA_*` leak). https://docs.conda.io/projects/conda/en/latest/dev-guide/deep-dives/activation.html
- **Environment Modules / Lmod** — load/unload reversal, reference counting (`__MODULES_SHARE_*` / `LMOD_DUPLICATE_PATHS`), `pushenv` value stack, named collections. https://modules.readthedocs.io/ , https://lmod.readthedocs.io/
- **chezmoi** — three-state model, `diff`/`status`/`apply` preview, `run_once_`/`run_onchange_`/`before_`/`after_` by content-hash, late-binding templates for portability. https://www.chezmoi.io/
- **home-manager** — generations as symlink chain, atomic switch, `--rollback`, activation DAG + `writeBoundary`/`checkLinkTargets` (verify-before-mutate). https://home-manager.dev/

### Secondary (MEDIUM confidence)
- **asdf / mise** — shims-vs-activation; mise docs confirm only activation reflects live env (validates the live-activation choice over shims). https://mise.jdx.dev/direnv.html
- **virtualenv / venv** — `_OLD_VIRTUAL_*` snapshot-and-restore; `deactivate` self-removal. https://docs.python.org/3/library/venv.html
- **direnv issues** — malformed `.envrc`/hook breaking the shell (#50, #1084); security/allow model (`.envrc` is arbitrary code). https://direnv.com/is-direnv-safe-to-use/
- **NixOS declarative-vs-imperative** — only pure/declarative config is reversible/reproducible; imperative → drift.
- **zsh startup performance** — vanilla ~50–100ms, < ~150ms budget, `zprof`/`hyperfine` profiling; asdf-direnv per-prompt-hook caching. https://blog.openreplay.com/zsh-slow-startup-fix/
- **`eval` vs `source /dev/stdin`** — pipe runs in a subshell unless `lastpipe` (env lost); favor `eval "$(...)"`.

### Tertiary (LOW confidence)
- **claude-code#4014** — concurrent shell-snapshot state corruption from a shared state dir (illustrative of the global-state-across-terminals footgun). https://github.com/anthropics/claude-code/issues/4014

---
*Research completed: 2026-06-25*
*Ready for roadmap: yes*
