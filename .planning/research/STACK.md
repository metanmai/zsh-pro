# Stack Research

**Domain:** Branchable shell-environment manager (git-versioned zsh profiles with live activate/deactivate) on an existing Go analyzer base
**Researched:** 2026-06-25
**Confidence:** HIGH (every recommendation is buildable with the existing dependency set + shelling out to `git`/`zsh`; the one optional new dependency is flagged explicitly)

> Scope note: this is the **v2.0 (Branchable Shell Environments)** stack — it supersedes the prior v1.1 PATH-analysis STACK.md. It covers only the NEW environment-manager capabilities. The existing ingest engine (parse → classify → introspect, `mvdan.cc/sh` LangZsh, the `Provider` seam) is reused as-is and not re-researched.

---

## TL;DR for the roadmap

The v2.0 environment manager needs **zero new compiled dependencies**. Three of the four capability areas are covered by what is already in the tree plus shelling out to binaries the project already shells out to:

- **(a) Git storage** → **shell out to the `git` binary** (the project already shells out to `zsh`; same pattern, same composition-root seam). `go-git` is viable but is a *new dependency that violates the current no-new-deps rule* and is weakest at exactly the porcelain we need (merge). Recommend shell-out; flag go-git only if the project later wants to drop the runtime `git` requirement.
- **(b) zsh runtime** → a **sourced loader** (added once to `~/.zshrc`) that calls the binary and `eval`s its output; the binary emits eval-able `export`/`unset`/`alias`/`unalias`/`setopt` statements. Per-terminal active state is tracked with **env-var markers + a serialized reverse-manifest** (the direnv/shadowenv model). No new dependency.
- **(c) Codegen (struct → zsh source)** → **string templating from the structured representation**, which is *already the project's established pattern* (`core/testgen/render.go`). Do **not** route generation through the `mvdan/sh` printer (zsh printer support is new and explicitly "not complete"). No new dependency.
- **(d) zsh builtins** → all teardown/restore primitives (`unalias`, `unset -f`/`unfunction`, `unsetopt`, `typeset -U path`, `add-zsh-hook`) are stock zsh; the existing introspection already loads `zsh/parameter`. No new dependency.

The only **explicit dependency decision** the roadmap must make is (a) git-binary-vs-go-git — and the recommendation is the git binary.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.25.0 (toolchain 1.25.7 present) | Entire codebase | Already the project language; nothing changes. |
| `mvdan.cc/sh/v3` | v3.13.1 (already pinned; latest, 2025-04-06) | zsh **parsing** for ingest (front-end) | Already the sole dependency. v3.13.0 (2025-03) is where zsh parser+formatter support first landed — the project rides the first version that has it. Keep using it for **parsing only**. |
| `git` binary (shell-out) | system git (≥ 2.x) | Profile storage: `init`, `add`, `commit`, `branch`, `checkout`, `status`, `worktree` | Reuses the exact pattern the project already uses for `zsh` (`exec.CommandContext` + timeout at the composition-root seam). Zero new compiled deps. Bit-for-bit compatible with real git semantics (merge, conflict resolution, reflog) that a pure-Go lib only partially implements. |
| `zsh` binary (shell-out + sourced loader) | system zsh (≥ 5.x) | Introspection (existing) + the activate/deactivate runtime (new) | Already a runtime requirement (`Introspect` runs `zsh -f`). The activation model is intrinsically zsh-side (a child process cannot mutate the parent shell), so the runtime *must* be a sourced loader regardless of implementation language. |

**Net new compiled dependencies: 0.** New *runtime* requirement: `git` on `$PATH` (joins the existing best-effort `zsh` requirement). Like `zsh` today, absence should degrade gracefully (storage commands error cleanly; analyze still works).

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| stdlib `os/exec` | Go 1.25 | Run `git`/`zsh` subprocesses with `context.WithTimeout` | Already used in `core/shell/zsh/introspect.go:46`. Reuse verbatim for git. |
| stdlib `encoding/json` | Go 1.25 | Serialize the reverse-manifest / active-state record (per-terminal teardown info) | The activate step must persist "what I changed" so deactivate can reverse it. JSON is already the project's wire format (`core/dto`). |
| stdlib `text/template` *(optional)* | Go 1.25 | Render the sourced loader stub + per-branch activate scripts | Only if templating grows beyond `fmt.Sprintf`. The existing codegen (`core/testgen/render.go`) uses `fmt.Sprintf` and is sufficient to start. |
| `mvdan.cc/sh/v3/syntax` **Printer** | v3.13.1 | **NOT recommended for generation** — see "What NOT to Use" | Re-printing only verbatim, already-parsed nodes is safe; synthesizing zsh from scratch through it is not. Prefer string templating. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `git` (test fixtures) | Integration tests create throwaway repos in `t.TempDir()` | Mirrors the existing `corpus_test.go` / `zsh -f` integration-test style; gate behind a `git`-on-PATH check so unit tests stay hermetic. |
| existing `zsh-gen` + `core/testgen` | Generate `.zshrc` fixtures for round-trip (ingest → codegen → re-ingest) property tests | The oracle harness already proves "generate zsh → parse → expectations match." Extend it to pin **round-trip stability** of the new codegen. |
| `golangci-lint` v2 (existing) | Lint gate (`make check`) | No change. |

---

## Installation

```bash
# No new Go modules. The stack is: existing dependency + two system binaries.

# Runtime requirements (degrade gracefully if absent, like zsh does today):
#   - git  >= 2.x  on $PATH   (NEW: profile storage)
#   - zsh  >= 5.x  on $PATH   (EXISTING: introspection + activation runtime)

# go.mod stays:
#   require mvdan.cc/sh/v3 v3.13.1

# IF (and only if) the project later decides to drop the runtime git requirement
# and accept a new compiled dependency — this is the line, flagged for explicit sign-off:
#   go get github.com/go-git/go-git/v5@v5.19.1   # pure-Go git; see "Alternatives Considered"
```

---

## (a) Git-backed storage — shell-out vs. go-git (the explicit dependency decision)

**Recommendation: shell out to the `git` binary.** Reasons, in priority order:

1. **No-new-deps rule.** The project's hard constraint is "single external dependency; no new dependencies without explicit discussion" (CLAUDE.md, PROJECT.md §Constraints). `go-git` is a *new compiled dependency* (and a large transitive tree). Shelling out adds **zero** modules — it reuses the identical mechanism the project already trusts for `zsh` (`exec.CommandContext` + timeout + the `Provider` seam). This is the decisive factor.
2. **The operations we need are exactly go-git's weak spot.** A branchable profile manager leans on porcelain: `branch`, `checkout`, and eventually `merge`/conflict resolution. go-git's own docs state it "lacks the main porcelain operations such as merges," and `Pull` supports **fast-forward only**. The real `git` binary does all of this correctly and is what users already understand (they can `git log`/`git merge` the profile repo by hand).
3. **Fidelity + zero surprise.** Shelling out to git guarantees byte-identical behavior with the user's own git (hooks, config, reflog, `worktree`). A library reimplementation risks subtle divergence on edge cases.

**Tradeoffs of shell-out (honest accounting):**

| Concern | Shell-out to `git` | Mitigation |
|---------|--------------------|------------|
| Runtime dependency on `git` | Yes — git must be on `$PATH` | Same posture the project already accepts for `zsh`; degrade gracefully with a clear error, exactly like `Introspect` does (`IdentitySet{Available:false}`). |
| Process-spawn overhead | One short-lived process per git op | Profile ops are user-initiated (`checkout`, `commit`), not hot loops — overhead is irrelevant here (unlike the per-prompt activation path, which must NOT shell out to git). |
| Output parsing | Must parse `git status --porcelain`, branch lists | Use stable plumbing/porcelain formats (`status --porcelain`, `for-each-ref --format`), never the human UI. |
| Cross-version drift | git human output varies by version | Pin to `--porcelain` machine formats which are explicitly version-stable. |

**`go-git` profile (the rejected-for-now alternative):** pure Go, no runtime git, great for cross-compiled single binaries; used by Gitea/Flux/Pulumi. Latest stable **v5.19.1 (2025-05-18)**, module `github.com/go-git/go-git/v5`; v6 is **alpha** (`v6.0.0-alpha.4`) — not production-ready. Reconsider go-git **only** if the project later prioritizes a self-contained binary with no host `git` over the no-new-deps rule, AND can live with fast-forward-only merges (or shell out to git just for merge). That is a deliberate constraint change requiring explicit sign-off — do not adopt it silently.

**`git2go`/libgit2 (rejected):** cgo bindings to a C library — adds a native build dependency and cross-compilation pain. Strictly worse than both options for this project.

**Integration point:** add a `store` package behind a narrow interface (mirror the `shell.Provider` ISP seam). The concrete `git`-shelling implementation is wired only at the composition root (`core/cmd/zsh-pro/main.go`), keeping `core/analyze` and the engine git-free — exactly the layering rule in PROJECT.md §Constraints.

---

## (b) The zsh shell-integration runtime (the activation mechanism — concrete)

A child process **cannot** mutate its parent shell's environment, aliases, or functions. Every comparable tool (direnv, conda, virtualenv, asdf, shadowenv) solves this the same way: ship a **sourced loader** that runs *in* the user's shell and `eval`s output the binary prints to stdout. This is the only mechanism that works; it is not a stylistic choice.

### What the loader is (added once to `~/.zshrc` — the "master block")

A tiny stub the user adds once (the thin bootstrapping `.zshrc` from PROJECT.md):

```zsh
# zsh-pro loader (add once, lives in the unmanaged master block)
eval "$(zsh-pro shell-init zsh)"
```

`zsh-pro shell-init zsh` prints the integration function + (optional) hook registration to stdout; `eval` installs it into the live shell. This is the direnv pattern verbatim (`eval "$(direnv hook zsh)"`).

### What the integration emits and how hot-switch works

`checkout <branch>` is a shell **function** (installed by the loader), not the bare binary — because only a function running in-shell can apply env changes. The function calls the binary and evals the result:

```zsh
zp() {                       # the user-facing command, runs in the live shell
  eval "$(command zsh-pro checkout "$1")"
}
```

The **binary** prints eval-able teardown-then-setup to stdout, e.g.:

```zsh
# --- deactivate previous branch (reverse-manifest, from saved state) ---
unalias gs 2>/dev/null
unset -f work_deploy 2>/dev/null
export PATH='<captured base PATH>'        # rebuild from captured base, not string-strip
unset CLIENT_TOKEN
# --- activate new branch (declarative state only) ---
alias gs='git status'
work_deploy() { ... }
export EDITOR=nvim
typeset -U path; path=(/new/bin $path)
# --- persist new active-state marker for the next deactivate ---
export ZSHPRO_ACTIVE=client-x
```

**Eval mechanism — concrete choice:** use `eval "$(...)"` (command substitution), **not** `binary | source /dev/stdin`. A pipe (`|`) runs the right-hand side in a **subshell** in zsh unless `setopt lastpipe` is set, so env mutations would be lost. direnv uses `eval "$(...)"` for exactly this reason; shadowenv's `| source /dev/stdin` only works because of zsh-specific pipe handling and is more fragile. **Pick `eval "$(...)"`.** (Gotcha verified — see Sources.)

### Tracking per-terminal vs. global active state

Two-layer state, matching how the field does it:

| Layer | Mechanism | Used by (precedent) | Role here |
|-------|-----------|---------------------|-----------|
| **Per-terminal "what's active"** | An exported env-var marker, e.g. `ZSHPRO_ACTIVE=<branch>` | direnv `DIRENV_DIR`, virtualenv `VIRTUAL_ENV`, conda `CONDA_DEFAULT_ENV` | Each terminal carries its own marker (env vars are per-process); switching one terminal does not touch another. This is what makes profiles per-terminal. |
| **Per-terminal "how to undo"** | A serialized **reverse-manifest** the activate step records | direnv `DIRENV_DIFF` (stores the env *diff*, reverses it on leave); shadowenv `__shadowenv_data` | On deactivate, replay the inverse: `unalias` the aliases it added, `unset -f` the functions, `unset`/restore the env vars, rebuild PATH from the captured base. This is the **zero-residue** guarantee. |
| **Global "what exists"** | The git repo (branches) on disk | — | Branches/profiles are global (shared across terminals); only *activation* is per-terminal. |

Two design schools for the reverse-manifest, both proven — pick per the zero-residue spike:

1. **Diff/reverse model (direnv, shadowenv) — recommended for env + PATH.** Record the changes; on deactivate apply the inverse. Robust because it reverses *exactly* what was applied, even across nested switches. Store as JSON in the env marker or a per-terminal state file keyed by `$$`/`$TTY`.
2. **`_OLD_*` snapshot model (virtualenv, conda).** Save specific originals (`_OLD_VIRTUAL_PATH`) before mutating; restore on deactivate. Simpler but per-variable and brittle for aliases/functions. Fine as a fallback for a small fixed set; not enough alone for arbitrary declarative state.

**PATH specifically:** never reverse PATH by string-subtracting what you added (entries shift, duplicates accumulate — the classic foot-gun). Instead **capture a base PATH** at first activation and on switch **rebuild** PATH from that base + the new profile's entries, then `typeset -U path` to dedupe. This is the "rebuild PATH from a captured base" requirement in PROJECT.md, and it is the correct one.

**Critical performance constraint for the roadmap:** the *switch* (`checkout`) runs the binary once and is fine. But if the design ever puts zsh-pro on a `precmd`/`chpwd` hook (auto-activate on `cd`, like direnv), that hook fires **before every prompt** — it must be near-instant and must **not** shell out to `git` on each prompt. Cache aggressively (compare a cheap fingerprint; only invoke the binary when the active branch actually changed). For v2.0's explicit `checkout` model this is avoidable; flag it the moment auto-activation is proposed.

---

## (c) Regenerating zsh source from a structured representation (codegen)

**Recommendation: string templating from the structured representation — the project's existing pattern.** Do **not** route generation through the `mvdan/sh` printer.

**Why string templating:**

- The codebase **already does this** in `core/testgen/render.go` (`fmt.Sprintf("export %s=%q", ...)`, `alias %s='%s'`, function bodies). It is proven, deterministic, and the oracle harness already pins it. Extending the same approach to the ingest store is the lowest-risk path and stays consistent with house style.
- The parser already hands you everything needed: per statement you get `Kind`, `Names`, `Exported`, `CmdName`, **and verbatim `Text`** (`core/shell/zsh/parse.go:48-51`, `core/model/block.go`). So generation is mostly **"emit `Text` for unmodified statements, template only the fields you rewrite."**

**Why NOT the `mvdan/sh` Printer for *generation* (the load-bearing gotcha):**

- zsh **printer** support is brand new (landed v3.13.0, 2025-03) and the maintainer's own release note says zsh "support is not complete ... should work for many use cases." Round-tripping arbitrary zsh-specific constructs through it risks silent corruption of the very config you are managing.
- The printer's safe, supported use is **re-printing nodes it parsed** (positions intact). **Synthesizing nodes from scratch** (building `&syntax.Assign{...}`/`&syntax.CallExpr{...}` with zero-value positions) is the historical source of printer **panics** — the changelog shows repeated position/nil-pointer panic fixes (v3.2.2 "avoid comment position panic in the printer"; v3.9.0 "don't panic when pattern words are nil"). Constructing zsh by hand-building AST nodes is exactly the fragile path.

**Codegen approach — concrete recommendation:**

1. **Preserve-verbatim by default.** For any statement the manager doesn't rewrite, emit its captured `Block.Text` unchanged. This sidesteps both the printer's zsh gaps and any reformatting churn, and it directly serves the "imperative run-once code stays in the master block, untouched" requirement.
2. **Template the declarative slices.** For the categorized, regenerable state (aliases / env / PATH / functions / options), emit canonical lines via `fmt.Sprintf`/`text/template`, the same way `render.go` already does.
3. **Partial evaluation = string-level, not eval-level.** Keep dynamic values (`$HOME`, `$(...)`, conditionals) as their **literal source text** in the representation; only "resolve" static constants by recognizing them as plain literals. The parser already distinguishes a quoted literal value from a word containing `*ParamExp`/`*CmdSubst` parts (it reads `Word.Parts` — `wordLitPrefix` in `parse.go:144`), so "is this value static or dynamic?" is answerable structurally **without ever executing anything**. This is the portability guarantee in PROJECT.md (never freeze `$HOME`/`$(...)`).

**Codegen gotchas to pin with tests:**

- **Quoting/escaping.** `%q` produces a Go-style quoted string, not always a zsh-faithful one (e.g. `$`, backticks, `!` history expansion in double quotes). Emit single-quoted where the value is a literal; for embedded single quotes use the `'\''` idiom. Pin with round-trip tests (`ingest → codegen → re-ingest` must be a fixed point).
- **Round-trip stability.** Make `parse(generate(parse(src)))` structurally equal to `parse(src)` for the managed slices — extend the `core/testgen` oracle (10 seeds) to assert this, the same way it pins line numbers today.
- **Ordering.** Declarative reordering can change semantics (a PATH prepend before a later override). Preserve original statement order for anything not deliberately canonicalized (the `Block` slice is already ordered).

---

## (d) zsh builtins/modules for activate/deactivate (all stock; no new dep)

Everything the teardown/restore loop needs is core zsh. The existing introspection already proves the project can drive these (`emulate -L zsh; zmodload zsh/parameter` in `introspect.go:24-25`).

| Builtin / module | Role in activate/deactivate | Key flags / gotchas |
|------------------|------------------------------|----------------------|
| `add-zsh-hook` | Register `precmd`/`chpwd`/`preexec` hooks **if** auto-activation is added | `autoload -Uz add-zsh-hook` first. Add idempotently (check array membership, like direnv does with `(I)`); `add-zsh-hook -d` removes. Hooks fire per-prompt — keep them cheap (see (b) perf note). Not required for the explicit-`checkout` model. |
| `precmd` / `chpwd` (function arrays) | The hook arrays themselves (`precmd_functions`, `chpwd_functions`) | `typeset -ag` to declare; guard against double-registration. |
| `unalias` | Remove aliases on deactivate | `unalias name`; `unalias -a` clears ALL (too broad — only remove the profile's own). Silently succeeds if absent, so `2>/dev/null` is safe. `-s` (suffix), `-m` (pattern) exist if needed. |
| `unset -f` / `unfunction` | Remove functions on deactivate | `unset -f name` or `unfunction name`. Pair with the `Names` the FuncDecl parse already captured (`parse.go:122-132` handles `function a b {}` multi-name). |
| `unset` | Remove env vars the profile added | `unset VAR`. For vars the profile *overrode* (not added), restore the captured prior value instead of unsetting. |
| `typeset` / `local` | Declare/restore typed params; PATH array handling | `typeset -U path` dedupes the tied `$path`/`$PATH` array (`-U` = unique, keeps first occurrence). `$path` (array) and `$PATH` (scalar) are tied — mutate either. `local` inside the activate function auto-unsets on return (useful for scratch vars). Watch `TYPESET_TO_UNSET`. |
| `export` | Set/restore env on activate; rebuild PATH | `export PATH='<captured base>'` then re-add — rebuild, don't string-subtract. |
| `setopt` / `unsetopt` | Apply/reverse shell options per profile | `setopt name` / `unsetopt name` (or `setopt noname`). **Gotcha:** a bad option name does NOT abort subsequent code (unlike `set -o`), so failures are silent — validate option names at ingest. To snapshot/restore the full option set, capture `setopt` output (or the `$options` assoc array from `zsh/parameter`) at base and diff. |
| `zmodload zsh/parameter` | Exposes `$aliases`, `$functions`, `$parameters`, `$options`, `$path` assoc/arrays | Already used by introspection. Lets the loader read live state to compute an accurate reverse-manifest at activation time. `2>/dev/null` guard (already present). |
| `emulate -L zsh` | Sandbox the activate/deactivate function from the user's options | Already used in the introspect script; reuse so the teardown logic isn't perturbed by user `setopt`s. `-L` makes it local to the function. |

**Deactivate ordering that avoids residue:** (1) restore/`unset` env vars from the reverse-manifest → (2) `unalias` the profile's aliases → (3) `unset -f` the profile's functions → (4) `unsetopt`/restore options → (5) rebuild PATH from captured base + `typeset -U path` → (6) clear the `ZSHPRO_ACTIVE` marker and reverse-manifest. Then apply the new profile in the reverse-friendly order and write the new marker.

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Shell out to `git` | `go-git/v5` (v5.19.1) | Only if the project deliberately drops the no-new-deps rule to ship a self-contained binary with no host `git`, and accepts fast-forward-only merges (or shells out just for merge). Requires explicit sign-off. |
| Shell out to `git` | `git2go`/libgit2 (cgo) | Essentially never for this project — adds a native C build + cross-compile burden, worse than both alternatives. |
| String templating for codegen | `mvdan/sh` Printer | Only for re-printing nodes the project itself just parsed (positions intact). Never for synthesizing zsh from hand-built AST nodes. |
| `eval "$(...)"` loader | `binary \| source /dev/stdin` | Only under `setopt lastpipe`; otherwise the piped side runs in a subshell and env mutations are lost. Prefer `eval "$(...)"`. |
| Diff/reverse manifest (per-terminal state) | `_OLD_*` snapshot vars | Acceptable for a small fixed set of scalars; insufficient alone for arbitrary aliases/functions. |
| Sourced loader function (`checkout` is a shell function) | Bare binary `zsh-pro checkout` | A bare binary **cannot** mutate the parent shell — not an option for live hot-switch; the function wrapper is mandatory. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `mvdan/sh` Printer to **generate** zsh from scratch | zsh printer support is new + "not complete" (v3.13.0); hand-built nodes with zero positions are the documented source of printer panics | String templating from the structured rep (the existing `render.go` pattern) + emit verbatim `Block.Text` for untouched statements |
| Adding `go-git` (or any module) silently | Violates the project's hard no-new-deps constraint; go-git is also weakest at the porcelain (merge) this product needs | Shell out to the `git` binary via the composition-root seam |
| Reversing PATH by string-subtracting added entries | Entries shift; duplicates accumulate; non-deterministic residue (the exact foot-gun v2.0 must avoid) | Capture a base PATH; rebuild from base + profile entries; `typeset -U path` to dedupe |
| `binary \| source /dev/stdin` for the loader | Pipe runs in a subshell in zsh (unless `lastpipe`); env changes silently lost | `eval "$(zsh-pro ...)"` (command substitution runs in the current shell) |
| Shelling out to `git` inside a `precmd`/`chpwd` hook | Hook fires before every prompt; a git subprocess per prompt is a visible latency tax | Cache; only invoke the binary when a cheap fingerprint shows the active branch changed (and avoid hooks entirely for the explicit-`checkout` model) |
| Resolving `$HOME`/`$(...)` at ingest (`os.ExpandEnv`, `mvdan.cc/sh/v3/expand`, `shell.Expand`) | Freezes a profile to one machine — destroys the portability that makes branches useful (explicit Out-of-Scope in PROJECT.md) | Keep dynamic values as literal source text (late-bound); only fold genuinely-static literals; the printer renders `$HOME` as a token, never its value |
| `unalias -a` / clearing all functions on deactivate | Nukes the user's unmanaged aliases/functions, not just the profile's | Track the profile's own `Names` (already captured at parse) and remove exactly those |

---

## Stack Patterns by Variant

**If v2.0 ships the explicit-`checkout`-only model (PROJECT.md target):**
- Loader installs a `checkout` shell function that runs `eval "$(zsh-pro checkout …)"`.
- No `precmd`/`chpwd` hook needed → no per-prompt latency concern → shelling out to `git` on switch is completely fine.
- Per-terminal state = `ZSHPRO_ACTIVE` env marker + a serialized reverse-manifest.

**If auto-activate-on-`cd` is added later (direnv-style):**
- Register `__zshpro_hook` on `precmd_functions` + `chpwd_functions` via `add-zsh-hook` (idempotent).
- The hook MUST be near-instant and MUST NOT shell out to git per prompt — gate the binary call behind a cheap changed-branch fingerprint.
- This is a meaningful complexity step (security/trust of auto-run, like shadowenv's `trust`); treat as its own phase.

**If the project later prioritizes a single self-contained binary over the no-new-deps rule:**
- Swap the `store` seam's git-shelling impl for `go-git/v5` (v5.19.1) — but accept fast-forward-only merges or shell out solely for merge. Explicit sign-off required.

---

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `mvdan.cc/sh/v3 v3.13.1` | Go 1.25.0 | Already in `go.mod`/`go.sum`; latest release; first line with zsh printer support (use printer only for re-printing parsed nodes). |
| `git` binary ≥ 2.x | any Go version | Parse only machine formats (`status --porcelain`, `for-each-ref --format`) for version stability. |
| `zsh` binary ≥ 5.x | any Go version | `add-zsh-hook`, `zsh/parameter`, `typeset -U`, `emulate -L` all available; `lastpipe`/`TYPESET_TO_UNSET` are option-gated — don't depend on user settings, `emulate -L zsh` in the activate function. |
| `go-git/v5 v5.19.1` *(only if adopted)* | Go 1.25 | Stable line; v6 is alpha (`v6.0.0-alpha.4`) — avoid in production. New dependency — flagged, requires sign-off. |

---

## Integration Points with the Existing Engine

- **Ingest reuses the front-end as-is.** `Provider.Parse` already yields ordered `Block`s with `Kind`/`Names`/`Exported`/`CmdName` + verbatim `Text`, and `Classify` assigns `Cat*`. The categorized, regenerable store is a new consumer of those existing outputs — no engine change required for ingest.
- **New `store` seam mirrors `shell.Provider`.** Define a narrow storage interface; wire the concrete git-shelling implementation only at the composition root (`core/cmd/zsh-pro/main.go`), keeping `core/analyze` git-free (PROJECT.md §Constraints layering rule).
- **`shell-init` / `checkout` are new CLI verbs** on the existing `CLI` (`core/cli/cli.go`), reusing the typed exit-code + `--json` envelope contract.
- **Activation snapshot can reuse introspection.** The existing `zsh -f` introspection (resolved `IdentitySet`: aliases/functions/env/path/options) is precisely the data needed to compute an accurate reverse-manifest — wiring those already-captured tables into activation is the backlogged enrichment noted in `introspect.go:86-90`.
- **Codegen extends the existing `render.go` pattern**, and the `core/testgen` oracle extends to pin round-trip stability — the same harness that pins line numbers today.

---

## Sources

- `/mvdan/sh` (Context7, 496 snippets, High reputation) — `Printer.Print` accepts any `Node` (File/Stmt/Word/Assign/Command/WordPart); AST nodes are constructible programmatically (`&syntax.BinaryArithm{...}`); package supports Zsh variant. HIGH.
- https://github.com/mvdan/sh/releases — latest **v3.13.1 (2025-04-06)**; **v3.13.0 (2025-03-09)** introduced zsh parser+formatter, "support is not complete"; printer position/nil panics fixed across v3.2.2 and v3.9.0. HIGH.
- https://github.com/go-git/go-git/releases — latest stable **v5.19.1 (2025-05-18)**, module `…/go-git/v5`; v6 is **alpha**; used by Gitea/Flux/Pulumi. HIGH.
- https://pkg.go.dev/github.com/go-git/go-git/v5 + go-git README (via WebSearch) — pure-Go, no native git; "lacks main porcelain operations such as merges"; `Pull` is fast-forward-only. MEDIUM-HIGH.
- https://github.com/direnv/direnv/blob/master/internal/cmd/shell_zsh.go (via WebFetch) — zsh hook template: `_direnv_hook` registered on `precmd_functions`+`chpwd_functions` (idempotent via `(I)`), `eval "$(direnv export zsh)"`; export emits `export KEY=VALUE;` / `unset KEY;`. HIGH.
- https://direnv.net/ + CHANGELOG + `internal/cmd/config.go` (via WebSearch) — `DIRENV_DIFF` stores the env *diff*; `Revert()` applies `diff.Reverse().Patch(env)` to restore. HIGH.
- https://github.com/Shopify/shadowenv (+ `sh/shadowenv.zsh.in`, `docs/shadowlisp.md`) — reversible structured env shadowing; `__shadowenv_data` tracks reverse state; `eval`/`source /dev/stdin` loader; `env/set` preserves previous value for reactivation; `shadowenv trust` security model. HIGH (closest structural analog).
- https://docs.conda.io/.../deep-dives/activation.html — conda obtains shell code from an activator and `eval`s it (temp script + source where no eval); deactivate restores/erases vars; PATH set after custom scripts. HIGH.
- https://gist.github.com/andy-zam/041d57c768d9aa65c2d2794c2d899654 + Recurse Center "no magic: virtualenv" (via WebSearch) — `_OLD_VIRTUAL_PATH`/`_OLD_VIRTUAL_*` snapshot-and-restore pattern; `unset -f deactivate` self-removal. MEDIUM-HIGH.
- https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html — `unalias` (`-a`/`-s`/`-m`, silent success), `unset -f`/`unfunction`, `typeset`/`local` scoping + `TYPESET_TO_UNSET`, `setopt`/`unsetopt` (bad name does not abort). HIGH.
- https://github.com/zsh-users/zsh/blob/master/Functions/Misc/add-zsh-hook + zsh hooks docs (via WebSearch) — `add-zsh-hook` adds/removes from `precmd`/`chpwd`/`preexec` arrays; `autoload -Uz`; `-d` removes; `-L` lists. HIGH.
- https://tech.serhatteker.com/post/2019-12/remove-duplicates-in-path-zsh/ + til.hashrocket (via WebSearch) — `$path`(array) tied to `$PATH`(scalar); `typeset -U path` dedupes keeping first occurrence. HIGH.
- Unix&Linux SE "eval vs source /dev/stdin" (via WebSearch) — pipe to `source /dev/stdin` runs in a subshell (env lost) unless `lastpipe`; favor `eval "$(...)"` for in-shell env mutation. MEDIUM-HIGH.
- Existing codebase (read directly): `core/shell/zsh/introspect.go` (shell-out pattern, `zmodload zsh/parameter`, `emulate -L zsh`), `core/shell/zsh/parse.go` (structured `Block` extraction + verbatim `Text`), `core/testgen/render.go` (string-templating codegen already in house). HIGH.

---
*Stack research for: branchable shell-environment manager (git-versioned zsh profiles) on an existing Go analyzer base*
*Researched: 2026-06-25*
