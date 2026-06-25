# Pitfalls Research

**Domain:** Branchable shell-environment manager for zsh — a sourced activate/deactivate loader that owns part of the user's `~/.zshrc` startup and live-switches declarative state (aliases, env, PATH, functions, options) per git branch.
**Researched:** 2026-06-25
**Confidence:** HIGH (every critical pitfall is corroborated by a named prior-art post-mortem: conda, direnv, Environment Modules, virtualenv, NixOS, chezmoi)

> Scope note: This file is about the *new* v2.0 risk surface — the moment zsh-pro stops being a read-only analyzer and starts **mutating the live shell on every startup and every `checkout`**. The dominant theme across all prior art is: **a tool that adds state to a shell is only as good as its ability to remove that exact state later, on a machine it has never seen, without breaking the shell if anything goes wrong.** Read-only analysis has none of these failure modes; the manager has all of them.
>
> This file fully replaces the parked v1.1 ("Trustworthy PATH Analysis") pitfalls — that milestone's analyzer-reporting risk surface no longer drives the roadmap. PATH-extraction/canonicalization concerns from v1.1 survive only as they re-scope into the v2.0 ingest layer (Pitfall 1, Pitfall 5).

---

## Critical Pitfalls

### Pitfall 1: PATH accumulation / doubling on re-source and switch (the zero-residue problem, part 1)

**What goes wrong:**
Every `prepend`/`append` to `PATH` is non-idempotent. Re-sourcing `~/.zshrc` (Cmd-N new tab inheriting a dirty env, `exec zsh`, `source ~/.zshrc` after an edit) or switching branch A→B→A re-runs the prepends and `PATH` grows: `/work/bin:/work/bin:/usr/bin:...`. After a few switches the user has a 40-entry `PATH` with the same dirs 3–4 times, slowing every command resolution and producing wrong precedence. This is the single most reported failure of every environment switcher.

**Why it happens:**
Developers model PATH as "add my dir" (imperative) instead of "PATH should equal base + my dirs" (declarative). On deactivate they try to *string-remove* their entry — but string-removal is fragile (substring matches, trailing slashes, `~` vs `$HOME` notation, the entry appearing twice) and silently leaves residue. Conda hit exactly this: after `conda deactivate`, the env's `bin` dir was *still in PATH* because deactivate could not reliably reverse what activate (and user scripts) prepended ([conda#11021](https://github.com/conda/conda/issues/11021), [conda#8070](https://github.com/conda/conda/issues/8070)).

**How to avoid:**
- **Capture a base PATH snapshot once, before any managed mutation, and rebuild from it** — never string-remove. Deactivate = `PATH=$ZP_BASE_PATH`; activate = `PATH=$ZP_BASE_PATH:<branch dirs>`. This is the virtualenv `_OLD_VIRTUAL_PATH` model and the conda `CONDA_RESTORE_PATH` fix recommended in #8070: *save the value before, restore the value after.*
- **Make PATH a managed set, not an append target.** zsh gives you `typeset -U path` (dedup), but treat that as a *safety net*, not the primary mechanism — a dedup'd-but-rebuilt-from-dirty-base PATH is still wrong-precedence. Rebuild-from-captured-base is the real fix; `typeset -U` catches the leftover case.
- **Capture base PATH at loader install time / first managed shell, store it in a marked env var (e.g. `ZP_BASE_PATH`), and re-read it, never re-derive it** — see Pitfall 11 for *when* exactly to snapshot so you don't freeze a dirty PATH as the base.

**Warning signs:**
`echo $PATH | tr : '\n' | sort | uniq -d` returns anything after two switches; PATH length grows monotonically across `checkout`s; a command resolves to the wrong binary after switching back to a profile.

**Phase to address:** Activate/deactivate manifest phase (the switch loop). Pin with a property test: N random switch sequences → final PATH for profile X is byte-identical regardless of path taken.

---

### Pitfall 2: Leftover aliases / functions / options after switch (the zero-residue problem, part 2)

**What goes wrong:**
Branch A defines `alias gs='git status'` and a function `work_deploy`; branch B doesn't. After A→B the alias and function are *still live* because B's activation only *adds* B's state and never removed A's. The user runs `gs` in the "clean" personal profile and gets work behavior. Same for `setopt` flags and keybindings — `setopt EXTENDED_GLOB` set by A silently persists into B and changes how B's globs behave.

**Why it happens:**
There is no automatic teardown in a shell. Re-sourcing replaces *variables* (a later `FOO=` overwrites an earlier one) but **does not remove aliases, functions, or options that the new config simply doesn't mention** — they linger from the previous activation. Developers test "does B work?" not "is A *gone*?", so residue ships invisibly.

**How to avoid:**
- **Generate an explicit deactivate manifest per branch that is the inverse of its activate manifest:** every `alias x=...` pairs with `unalias x 2>/dev/null`; every `function f {...}` pairs with `unset -f f 2>/dev/null`; every `setopt OPT` pairs with the captured prior state of `OPT` (not a blind `unsetopt` — see below).
- **Switch = full deactivate(prev) → activate(next), never a partial diff** for v1. (A diff-based "only change what differs" optimization is a *later* performance refinement, and a notorious source of residue bugs — Environment Modules needed reference-counting in `__MODULES_SHARE_*` to make diffing safe ([Modules docs](https://modules.readthedocs.io/en/latest/modulefile.html)). Don't start there.)
- **For options, snapshot-and-restore, don't toggle.** `setopt` has no clean inverse if the user already had the option on. Capture `setopt` state (or the relevant flags) into the base snapshot and restore it, mirroring the PATH-base approach.
- **For functions, capture the prior definition before overriding.** If A overrides a function that existed in the base shell, a blind `unset -f` on deactivate *deletes the base version too*. Save it with `typeset -f name` before overriding and restore it on deactivate ([typeset -f / unset -f semantics](https://www.gnu.org/software/bash/manual/html_node/Shell-Functions.html)).

**Warning signs:**
An alias/function from the previous profile still responds after switching; `alias` / `functions` / `setopt` output differs depending on which profile you switched *from* rather than which you're *in*; `setopt` flags drift across switches.

**Phase to address:** Activate/deactivate manifest phase. This is the core correctness contract. Verification: after switch A→B, assert `alias`, `functions`, and `setopt` snapshots match a cold `zsh -f` + B-only baseline (path-independence).

---

### Pitfall 3: Trying to mutate the parent shell from a child process (exec vs source)

**What goes wrong:**
`checkout` is implemented as a normal binary/subcommand that sets env vars, modifies PATH, defines aliases — and **none of it sticks**, because a child process cannot mutate its parent's environment. The user runs `zsh-pro checkout work`, sees "switched to work," and the shell is unchanged. Aliases/functions especially can never cross a process boundary; they are shell constructs, not env vars.

**Why it happens:**
This is the foundational constraint of every shell-environment tool, and it is counterintuitive: people expect a CLI to change "the shell." It cannot. virtualenv, conda, direnv, and Environment Modules **all** solve this the same way — the binary only *prints/returns shell code as a string*, and a thin shell function in the user's rc **`eval`s or `source`s** that string in-process. Conda's docs state it explicitly: *"The conda shell functions only write shell code and don't execute it themselves — the shell must eval or source these strings in-session"* ([conda activation deep-dive](https://docs.conda.io/projects/conda/en/latest/dev-guide/deep-dives/activation.html)).

**How to avoid:**
- **Architect the switch as: Go binary emits a manifest of shell statements on stdout → a sourced zsh function `eval`s it in the current shell.** The user-facing `checkout` must be a *shell function* (installed by the loader), not the raw binary. Plan the binary's stdout to be a clean, evaluable manifest (the agent-contract discipline from v1 transfers well here).
- **Keep the binary pure (compute the manifest) and the shell glue thin (apply it).** All the hard logic (compute deactivate(prev)+activate(next)) stays in testable Go; the shell side is a minimal `eval "$(zsh-pro __emit-switch B)"`.
- **Never `exec` the new env** as a "fix" — `exec zsh` throws away the current shell's history/jobs/cwd state and is not a switch, it's a restart.

**Warning signs:**
Switch "works" when you run it as `source <(...)` in a test but "does nothing" when invoked as a plain command; aliases never appear; design docs describe `checkout` as "a command" without a sourced-function wrapper.

**Phase to address:** The sourced-loader / shell-integration phase. This is an *architectural invariant* — fix it in the design, not in code review. The roadmap should make "checkout is a shell function that evals binary output" a Phase-0 constraint of the loader.

---

### Pitfall 4: Imperative / side-effecting startup code that cannot be un-run

**What goes wrong:**
A profile's config contains run-once imperative code: `eval "$(starship init zsh)"`, `nvm use 18`, `ssh-add ~/.key`, starting a daemon, `compinit`, `source $(brew --prefix)/.../something`, network calls, `git config --global ...`. The manager treats these as "state to apply," activates them on switch — but on deactivate there is **no inverse**. You can't "un-eval" a prompt init; you can't un-start a daemon by removing a line; `nvm use` mutates PATH in ways the manager didn't record. Switching becomes lossy and the shell accumulates irreversible side effects.

**Why it happens:**
The line between *declarative state* (an alias, an env value, a PATH entry — a fact that can be set and unset) and *imperative effect* (running a program that does arbitrary things) is invisible in a flat `.zshrc`. Everything looks like "a line in my config." NixOS's entire thesis is that **only pure/declarative configuration is safely reproducible and reversible; imperative steps produce drift and can't be rolled back** ([NixOS declarative vs imperative](https://medium.com/thenixos/what-is-declarative-configuration-in-nixos-understanding-declarative-vs-imperative-approaches-d24d4d144df6)).

**How to avoid:**
- **Make the declarative/imperative split a hard classification boundary, enforced by the ingest layer.** Only `Cat*` categories that are *set/unset-reversible* (aliases, env assignments, PATH entries, functions, options) are branch-switchable. Everything else (eval, command substitution at top level with side effects, plugin init, daemon starts) is **NOT managed** — it stays in the thin unmanaged `.zshrc` "master block" that runs once at shell start and is never switched. PROJECT.md already commits to this; the pitfall is *letting imperative code leak into the managed set* via an over-eager classifier.
- **Default to "unmanaged" when unsure.** This mirrors the v1 classifier principle already in PROJECT.md's Key Decisions ("precision over recall; a silent false positive is worse than an honest 'unsure'"). A misclassified imperative line in the switchable set is a *zero-residue violation by construction* — it can be activated but never deactivated.
- **Detect side-effect markers during ingest** — top-level `eval`, `$(...)`/backticks used as statements (not assignments), known init idioms (`* init zsh`, `nvm use`, `compinit`, `ssh-add`) — and route them to the master block, never to a branch manifest.

**Warning signs:**
A profile's deactivate manifest contains a no-op or a guess for some entries; switching leaves a daemon running / a prompt half-changed; the same `eval` line runs twice after a round-trip switch; "switch" feels like it half-works for prompt/version-manager state.

**Phase to address:** Ingest & categorize phase (the declarative/imperative classifier is the gate) *and* the manifest phase (the manifest generator must refuse to emit a deactivate it can't actually perform). Verification: every entry in an activate manifest has a real, tested inverse in the deactivate manifest, or it isn't in the managed set.

---

### Pitfall 5: Freezing late-bound values destroys cross-machine portability

**What goes wrong:**
Partial evaluation over-evaluates. The ingest layer resolves `export PROJECT="$HOME/work"` to `export PROJECT="/Users/alice/work"`, or resolves `eval "$(brew shellenv)"` / `$(go env GOPATH)` to a literal captured on the authoring machine. The profile now works only on that one machine; on a laptop where `$HOME` is `/Users/bob` or brew lives elsewhere, the profile points at paths that don't exist. The branch is no longer a *portable* environment — it's a snapshot of one machine, which defeats the entire value proposition ("`checkout` gives you a different *trustworthy* environment" — and trustworthy means it works where you check it out).

**Why it happens:**
"Partial evaluation" tempts you to resolve as much as possible for a clean, static store. But shell config is deliberately late-bound: `$HOME`, `$(...)`, `$USER`, conditionals on `$OSTYPE`, and `~` are *meant* to be evaluated at shell-start on the target machine. This is the exact mistake dotfile users make by hardcoding absolute paths instead of `~`/`$HOME`; the fix the whole dotfiles ecosystem converged on is **templating / late binding** (chezmoi's `.chezmoi.homeDir`, GNU Stow relative links) so values expand on the target machine, not the author's ([chezmoi templating](https://www.chezmoi.io/reference/commands/add/), [portable dotfiles](https://www.jeffyang.io/posts/configuring-portable-dotfiles/)).

**How to avoid:**
- **Partial evaluation resolves only provably-static, machine-independent constants; everything containing `$VAR`, `$(...)`/backticks, `~`, or a conditional stays *unresolved* and is re-emitted verbatim into the manifest** to be expanded at activation time on the live machine. PROJECT.md already makes this an Out-of-Scope guarantee ("dynamic values stay late-bound and unresolved") — the pitfall is a partial-eval pass that quietly crosses that line.
- **Treat `$HOME`, `$(...)`, command substitution, and tilde as a "do not touch" set in the evaluator.** When in doubt, keep it dynamic. The cost of leaving a constant unresolved is ~nothing; the cost of freezing a dynamic value is a profile that's silently broken on every other machine.
- **Test portability explicitly:** generate a profile with `HOME=/Users/alice`, then activate it under `HOME=/Users/bob` and assert the resulting env/PATH point at bob's home — i.e., the manifest contains `$HOME`, not `/Users/alice`.

**Warning signs:**
The stored representation contains absolute home paths or machine-specific brew/go/python prefixes; a profile authored on one machine references nonexistent paths on another; grep of the store finds a literal `/Users/<name>` or `/home/<name>`.

**Phase to address:** Partial-evaluation phase (this *is* the phase's central safety property). Verification: a "portability" test that round-trips a profile across two different `$HOME`/`$USER` values and asserts dynamic values survived as references.

---

### Pitfall 6: Dual source of truth — `.zshrc` drifts from the managed store

**What goes wrong:**
The manager stores profiles in its git-backed store, but `~/.zshrc` is *also* edited — by the user adding a line by hand, and especially by **installers that blindly append** (`conda init`, `nvm`, `rbenv`, `brew`, `pyenv`, the next tool's "add this to your .zshrc"). Now there are two sources of truth that disagree: the store says profile X, but `.zshrc` has an extra `export` or an `eval` that the manager doesn't know about. On the next switch the manager either clobbers the hand-edit (data loss, user fury) or ignores it (the store is a lie). Conda's own init block is *not idempotent* and appends/duplicates on repeated `conda init` ([conda#8703](https://github.com/conda/conda/issues/8703), [conda#7781](https://github.com/conda/conda/issues/7781)).

**Why it happens:**
`.zshrc` is a shared, written-by-every-tool file. No installer asks permission; they all `>> ~/.zshrc`. A manager that *owns* startup but doesn't *defend a boundary* in the file will be silently corrupted by the ecosystem, and users will (reasonably) keep hand-editing the file they've edited for years.

**How to avoid:**
- **Own exactly one clearly-marked region of `.zshrc`, never the whole file.** Use BEGIN/END sentinel markers — the conda `# >>> conda initialize >>>` / `# <<< conda initialize <<<` pattern is the de-facto standard ([conda init deep-dive](https://docs.conda.io/projects/conda/en/latest/dev-guide/deep-dives/activation.html)). Everything outside the markers is the user's; the manager only ever rewrites *between* the markers, and that rewrite must be **idempotent** (re-running install replaces the block, never appends a second one — the explicit bug conda#8703 documents).
- **The managed block should be a tiny loader stub, not the profile content.** Put `source <manager-loader>` (plus the captured base snapshot) inside the markers; keep actual profile state in the store. This shrinks the drift surface to one line.
- **On ingest, treat `.zshrc` as input you read, not state you own**; on switch, never write profile bodies back into `.zshrc`. Detect and warn on installer appends that landed *outside* your block ("`pyenv` added 3 lines to your .zshrc after the zsh-pro block — import them into a profile?").
- **Reconcile, don't assume.** Before a switch, check whether `.zshrc`'s managed block still matches what the manager wrote; if a human/installer touched it, surface the conflict instead of silently overwriting.

**Warning signs:**
`git diff` of the store and the actual `.zshrc` content disagree; a second copy of your loader block appears after an OS/tool update; a user reports "my hand-added alias disappeared after switching"; installer lines accumulate.

**Phase to address:** Git-backed store / ingest phase (define ownership boundary + idempotent block writer) and the loader-install phase. Verification: run the install/regenerate path twice → `.zshrc` is byte-identical the second time (idempotency test), and content outside the markers is never modified.

---

### Pitfall 7: Loader slowness — it runs on *every* shell, so it is felt constantly

**What goes wrong:**
The loader does real work at shell startup: spawns the Go binary (process fork + Go runtime init), reads/parses the store, computes a manifest, maybe shells out to `git`. Even 80–150ms per shell is *immediately perceptible* on every new tab, every `tmux` pane, every subshell, every script that starts an interactive-ish zsh. A `git` call or a per-prompt subprocess is far worse. Users abandon tools that make their terminal feel sluggish — this is the #1 reason zsh setups get torn out.

**Why it happens:**
A read-only analyzer runs *on demand*; a loader runs *on the hot path of every shell start*, a context developers under-weight. Spawning a subprocess (especially a Go binary that re-reads/re-parses state) per shell, or calling `git` per startup, blows the budget. Reference points: vanilla zsh starts in ~50–100ms; a well-tuned config targets **under ~150ms median**, and anything over ~500ms means "something specific is wrong" ([zsh startup profiling](https://blog.openreplay.com/zsh-slow-startup-fix/), [optimizing zsh init with zprof](https://www.mikekasberg.com/blog/2025/05/29/optimizing-zsh-init-with-zprof.html)). direnv's per-prompt hook is a known startup-cost target that people cache to mitigate ([asdf-direnv perf discussion](https://github.com/asdf-community/asdf-direnv/discussions/57)).

**How to avoid:**
- **Pre-compile the active profile to a flat, ready-to-source `activate.zsh` manifest; the startup hot path should `source` a static file, not invoke the Go binary or `git`.** The binary runs at `checkout`/regenerate time (cold path), writes the manifest; shell start just sources it. This is the single most important performance decision.
- **Zero subprocesses on the hot path.** No `git`, no `zsh-pro ...`, no `$(...)` in the loader stub beyond reading a cached file. Switching (cold) can be as slow as it needs; *starting a shell* (hot, every time) must be near-free.
- **Set and defend a startup budget** (e.g., loader adds < 15–20ms over bare zsh). Measure with `zmodload zsh/zprof` and `/usr/bin/time zsh -i -c exit` in CI-ish checks, the way the zsh community profiles ([zsh profiling + hyperfine](https://how2.sh/posts/how-to-trace-slow-shell-startup-zsh-profiling-hyperfine/)).
- **Don't re-parse on every shell.** Parsing `.zshrc` with the AST parser is an *ingest-time* (cold) operation, never a startup operation.

**Warning signs:**
New tabs feel laggy; `zprof` shows the loader/binary near the top; `time zsh -i -c exit` regresses after install; the loader stub contains `$(...)` or `git`; opening many tmux panes is visibly slow.

**Phase to address:** Loader / shell-integration phase, with a performance budget as an explicit success criterion. Verification: `hyperfine 'zsh -i -c exit'` with vs without the loader stays within budget; assert the hot path spawns zero subprocesses.

---

### Pitfall 8: Loader or tool failure breaks the user's shell

**What goes wrong:**
The loader stub in `.zshrc` errors — the manager binary is missing (uninstalled, `$PATH` not yet set, on a fresh machine, mid-upgrade), the store is corrupt, the manifest has a syntax error, or an `eval` of bad generated code throws. Because this runs during `.zshrc`, a hard failure **aborts the rest of `.zshrc` and/or drops the user into a broken or non-interactive shell**. Worst case: the user can't open a working terminal to *fix* it — a catastrophic, trust-destroying failure for a tool on the startup path. direnv has multiple reports of malformed `.envrc` / hook errors freezing or breaking the shell ([direnv freezing #50](https://github.com/direnv/direnv/issues/50), [direnv hang #1084](https://github.com/direnv/direnv/issues/1084)).

**Why it happens:**
A loader naively assumes its binary exists, its store is valid, and its generated code is correct — none guaranteed across machines, upgrades, and partial installs. Generated shell code that's `eval`'d is *arbitrary code that can have syntax errors*. There's no try/catch in `eval`; a bad manifest just breaks the shell.

**How to avoid:**
- **The loader stub must be defensive and fail-open.** Guard everything: `command -v zsh-pro >/dev/null 2>&1 || return` before touching it; `[[ -r $manifest ]] || return` before sourcing; wrap the apply so a failure logs a warning and *continues to an interactive shell* rather than aborting `.zshrc`. The ecosystem-standard guard is the `[ -f file ] && source file` idiom ([safe rc sourcing](https://rc-docs.northeastern.edu/en/latest/best-practices/shell_environment.html)).
- **Generated manifests must be validated before they're trusted as the active profile.** Parse/`zsh -n` the generated `activate.zsh` at *generation time* (cold path) and only promote it to "active" if it's syntactically valid — never let a never-validated blob become what every new shell sources.
- **Always keep a known-good fallback.** Keep the previous valid manifest; if the current one fails validation, the loader sources the last-good one (or nothing) and warns. The user always gets a working shell.
- **Recovery must not require a working shell.** Provide an escape hatch that works even if the loader is broken: e.g., `ZSHPRO_DISABLE=1` env var that the stub checks first to no-op itself, documented prominently. (direnv users disable the hook to recover; bake that in.)

**Warning signs:**
A missing binary or bad manifest produces anything worse than a warning + working shell; no `command -v` / `[[ -r ]]` guards in the stub; generated manifests are sourced without prior `zsh -n` validation; no documented "disable the loader" recovery path.

**Phase to address:** Loader / shell-integration phase (graceful-degradation is a hard requirement, mirroring v1's "introspection degrades gracefully" principle already in the codebase). Verification: delete the binary / corrupt the store / inject a syntax error into the manifest → a new shell still opens interactively with a warning; `ZSHPRO_DISABLE=1` fully no-ops the loader.

---

### Pitfall 9: Safety of running / regenerating arbitrary config

**What goes wrong:**
To ingest or partially-evaluate, the manager runs or sources *arbitrary shell code from the user's config* (e.g., to resolve a value it executes the surrounding snippet), and the regenerated manifest is `eval`'d into the live shell on every switch. If a profile's content is attacker-influenced (a shared/team store, a profile pulled from a colleague's machine, a malicious commit in the git-backed store, or simply a typo that becomes `rm -rf`), switching to it **runs that code with the user's full privileges**. This is the direnv threat model exactly: an `.envrc` is arbitrary code, so direnv refuses to run one until the user explicitly `direnv allow`s it ([Is direnv safe](https://direnv.com/is-direnv-safe-to-use/), [direnv allow/whitelist](https://man.archlinux.org/man/extra/direnv/direnv.toml.1.en)).

**Why it happens:**
Shell config *is* code; there's no safe "data-only" subset once you `source`/`eval` it. A manager that generates-and-evals on switch has effectively created an arbitrary-code-execution channel keyed on `checkout`. Once profiles can be shared or git-synced (the whole point of a branchable, git-backed store), they cross a trust boundary.

**How to avoid:**
- **Prefer static ingest (AST parse) over execution wherever possible.** zsh-pro already *parses* rather than executes (the v1 engine uses `mvdan.cc/sh` AST, with `zsh -f` introspection as best-effort and sandboxed via `zsh -f` + a 5s timeout). Keep partial-evaluation *static*: resolve constants by AST inspection, never by `eval`-ing user code. Executing config to "resolve" it is both a portability hazard (Pitfall 5) and a security hazard — avoid it.
- **Adopt a direnv-style trust/allow gate for switching to a profile whose content changed or came from elsewhere.** First switch to a new/modified profile prompts (or requires `zsh-pro allow`) before its manifest is eval'd. Hash the manifest; re-prompt when the hash changes. This is the proven model for "I'm about to run code you might not have written."
- **Never auto-source a manifest the user hasn't approved**, especially on `git pull` of a shared store. The git-backed design *will* pull other people's code; treat pulled profiles as untrusted until allowed.
- **Keep `zsh -f` (no-rcs) for any unavoidable introspection** so the user's own rc/plugins don't amplify a malicious snippet, and keep the timeout.

**Warning signs:**
Partial-eval works by `eval`-ing user snippets; switching to a freshly `git pull`'d profile runs its code with no approval step; manifests are sourced without an integrity check; introspection runs without `zsh -f`/timeout.

**Phase to address:** Partial-evaluation phase (keep it static — no exec) and the switch/store phases (trust-on-first-use gate before eval; integrity hash). Verification: a profile containing a side-effecting payload is *not* executed on ingest, and *not* eval'd on switch until explicitly allowed.

---

### Pitfall 10: State-tracking incorrectness across many concurrent terminals

**What goes wrong:**
The user has 20 terminals open. "Active profile" is stored as **global** state (a single file like `~/.config/zsh-pro/active`), so `checkout work` in terminal 1 changes the global pointer — and terminal 2 (which is on "personal") now disagrees with the global state. Worse, two terminals switching at once race on that file and corrupt it; or a new tab opens, reads the global "active = work," and applies work's manifest *on top of* the personal env it inherited from its parent — producing a hybrid, residue-laden environment that matches neither profile. Conda has a documented concurrent-activation race ([conda#9911](https://github.com/conda/conda/issues/9911)); Claude Code had concurrent shell-snapshot state corruption from shared state dirs ([claude-code#4014](https://github.com/anthropics/claude-code/issues/4014)).

**Why it happens:**
"Which profile is active" feels like one global fact, but it is intrinsically **per-shell** — each terminal is an independent process with its own environment. Modeling it globally creates drift (terminals disagree), races (concurrent writes), and inheritance bugs (new shells inherit a dirty parent env, then layer a profile on top). Shared mutable state across concurrent shells is a classic race surface requiring `flock`/atomic writes ([concurrent shell state](https://www.mindfulchase.com/explore/troubleshooting-tips/programming-languages/troubleshooting-subshell-and-concurrency-issues-in-bash-scripts.html)).

**How to avoid:**
- **Active profile is per-shell state, carried in the shell's own environment** (e.g., `ZP_ACTIVE_PROFILE=work` exported in *that* shell), not a single global file that all terminals share. Each terminal owns its truth; switching one never affects another.
- **Make activation idempotent against a dirty inherited env.** A new tab inherits the parent's `ZP_ACTIVE_PROFILE` *and* its dirty PATH/aliases. The loader must **deactivate-to-base then activate** on startup (using the captured `ZP_BASE_PATH`), so a child shell lands cleanly on its profile regardless of what it inherited — this is what kills the "hybrid env in a new tab" bug and ties back to Pitfalls 1–2.
- **If any on-disk state is shared (e.g., the store, a default-profile pointer), make every write atomic** (write-temp-then-rename, or `flock`) so 20 terminals can't corrupt it. But keep *active-profile* out of shared state entirely — that's what causes the drift.
- **Decide and document the new-tab policy:** does a new shell inherit the parent's profile or start from a configured default? Either is fine; *silently inheriting a dirty env and not re-normalizing* is the bug.

**Warning signs:**
Switching in one terminal changes another's behavior; a new tab's env matches neither the parent nor any clean profile; the active-profile file is a single global path written by all shells; concurrent `checkout`s corrupt state; `ZP_ACTIVE_PROFILE` says one thing but the actual aliases/PATH say another.

**Phase to address:** Activate/deactivate manifest phase (per-shell state model + deactivate-to-base-on-startup) and store phase (atomic writes for anything genuinely shared). Verification: parallel-switch stress test (N shells switching concurrently) leaves each shell internally consistent and shared state uncorrupted; a new tab spawned from a "dirty" parent normalizes to a clean profile.

---

### Pitfall 11: Capturing the base snapshot at the wrong moment (snapshotting a dirty state)

**What goes wrong:**
The "capture base PATH/env once" fix (Pitfalls 1, 2, 10) is captured **after** other tools have already mutated the shell — so the "base" itself contains conda's bin dir, an already-doubled PATH, or a previous profile's leftovers. Every future "restore to base" restores *to that contamination*. The zero-residue guarantee silently rests on a polluted baseline, and residue is baked in permanently.

**Why it happens:**
Ordering inside `.zshrc` is subtle and tools fight over it. If the manager's snapshot runs late (after other initializers, or after a previous activation), it captures their effects as "base." This is the *same root cause* as conda#11021/#8070 — capturing/relying on state at the wrong point in the sequence — applied to the snapshot step itself.

**How to avoid:**
- **Capture the base snapshot as early as possible and exactly once per *base* shell, before any managed activation and ideally before other tools' init.** Persist it (`ZP_BASE_PATH`, base options/functions) and **guard against re-capture**: if `ZP_BASE_PATH` is already set (inherited by a child shell), do *not* re-snapshot — reuse it, so a child never promotes a dirty inherited PATH to "base."
- **Place the snapshot at the top of the managed block** in `.zshrc`, accepting that some pre-existing tool state may be in the base on first install — and document a "re-baseline from a clean `zsh -f`" command for users who want a pristine base.
- **Treat the base as immutable once captured.** Switching rebuilds from it but never rewrites it.

**Warning signs:**
`ZP_BASE_PATH` contains a profile's or another tool's dirs; restoring to base still leaves residue; a child shell's base differs from its parent's; re-baselining changes behavior (proves the old base was dirty).

**Phase to address:** Loader / shell-integration phase (snapshot placement + re-capture guard), co-designed with the manifest phase. Verification: in a child shell, `ZP_BASE_PATH` equals the parent's (no re-snapshot); a from-`zsh -f` install produces a base PATH with no managed/tool dirs.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| String-remove PATH entries on deactivate instead of rebuild-from-base | Feels simpler, no snapshot needed | Fragile (`~` vs `$HOME`, trailing slash, dup entries) → permanent PATH residue; the exact conda#11021 bug | **Never** — rebuild-from-captured-base from day one |
| Diff-based partial switch ("only change what differs between A and B") | Faster switching | Residue bugs; needs reference-counting (Modules `__MODULES_SHARE_*`) to be correct | Only after full deactivate→activate is correct and proven, as a measured optimization |
| Invoke the Go binary / `git` on the startup hot path | Always-fresh manifest, no cache to invalidate | Every shell start pays fork+parse+git cost; tool gets torn out for being slow | **Never** on the hot path; binary runs at checkout-time, shell sources a static manifest |
| Resolve `$HOME`/`$(...)` during partial-eval for a "clean static store" | Tidy, fully-resolved store | Profiles freeze to one machine; portability (the core value) is dead | Never for dynamic values; only provably-static constants |
| Store "active profile" in one global file shared by all terminals | Simple "what's active?" query | Cross-terminal drift, write races, hybrid new-tab envs | Never for active-profile; per-shell env var instead |
| Blind `unsetopt`/`unset -f` on deactivate | One-line inverse | Deletes base-shell options/functions the user had before activating | Never — snapshot-and-restore prior state |
| Skip `zsh -n` validation of generated manifests | Faster generation | A syntax error in generated code breaks every new shell | Never — validate at generation, keep last-good fallback |
| Eval'ing user config to "resolve" values during ingest | Easy way to get computed values | ACE channel + portability loss | Never — static AST resolution only |

## Integration Gotchas

Common mistakes when connecting to the surrounding ecosystem.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `~/.zshrc` (shared with every installer) | Owning the whole file / appending non-idempotently (conda#8703) | Own one marked BEGIN/END region; idempotent rewrite of only that region; never touch outside |
| Other tools' init (conda/nvm/pyenv/brew) | Treating their `eval`/`use` lines as switchable declarative state | Classify as imperative → route to unmanaged master block; never into a branch manifest |
| `git` (the profile store backend) | Calling `git` on shell startup; auto-applying `git pull`'d profiles | `git` only at cold checkout/sync time; trust-gate pulled profiles before eval |
| zsh startup ordering | Capturing base snapshot after other tools mutate the shell | Snapshot at top of managed block, once, with a re-capture guard |
| Child shells / tmux / subshells | Assuming a fresh env; inheriting parent's dirty PATH + active-profile | Deactivate-to-base then activate on startup; reuse (don't re-capture) inherited base |
| `eval` of generated manifest | Sourcing unvalidated generated shell code | `zsh -n` validate at generation; fail-open with last-good fallback |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Subprocess (binary/`git`) per shell start | New tabs lag; `zprof` shows binary on top | Static pre-compiled `activate.zsh`; zero subprocesses on hot path | Immediately — felt on *every* shell, worse with many tmux panes |
| Re-parsing `.zshrc` (AST) at startup | Startup time scales with config size | Parse only at ingest (cold); startup sources a flat manifest | As config grows; large `.zshrc` → 100ms+ just to parse |
| Re-snapshotting base in every child shell | Slower nested shells; dirty bases | Guard on `ZP_BASE_PATH`; reuse inherited base | With deep shell nesting / many subshells |
| Full deactivate→activate on a huge profile every switch | Switch feels heavy | Acceptable for switch (cold path); only optimize with measured diff later | Only at very large profiles; switch is cold so budget is generous |
| Per-prompt hooks (direnv-style) for "auto-detect profile" | Every prompt pays a cost | If added later, cache aggressively (the evalcache pattern); prefer explicit `checkout` | If a precmd hook spawns work each prompt |

Reference budget: bare zsh ≈ 50–100ms; target loader overhead < ~15–20ms; investigate hard if shell start exceeds ~150–200ms ([zsh startup baselines](https://blog.openreplay.com/zsh-slow-startup-fix/)).

## Security Mistakes

Domain-specific security issues — these exist *because* the tool runs code in the user's shell.

| Mistake | Risk | Prevention |
|---------|------|------------|
| `eval`-ing config to resolve values during ingest | Arbitrary code execution at ingest time | Static AST resolution only; never execute user config |
| Auto-eval'ing a `git pull`'d / shared profile on switch | ACE from a teammate's or attacker's commit | direnv-style trust/allow gate; hash manifest; re-prompt on change ([Is direnv safe](https://direnv.com/is-direnv-safe-to-use/)) |
| Sourcing a manifest with no integrity check | Tampered manifest runs silently | Hash + verify before eval; lock the active manifest |
| Introspecting with the user's full rc/plugins loaded | A malicious snippet is amplified by plugins | Use `zsh -f` (no rcs) + timeout for any introspection (already the v1 pattern) |
| Treating the store as trusted because it's "local git" | A synced/shared store crosses a trust boundary | Trust-on-first-use per profile; never auto-run pulled content |
| Secrets resolved/frozen into the store or manifest | Secrets land in git history / shared profiles | Keep secret-bearing lines unresolved/late-bound; reuse v1 secret detection to refuse to freeze them |

## UX Pitfalls

Common user-experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Silent residue ("switched!" but old aliases remain) | User trusts a profile that's actually a hybrid → wrong/dangerous commands | Verify zero-residue; offer `zsh-pro doctor` that diffs live shell vs expected profile |
| Switch only affects new shells, not the current one | "It didn't work" confusion | Sourced function applies to the current shell live (Pitfall 3); state clearly which shells are affected |
| Clobbering hand-edited `.zshrc` lines on switch/regenerate | Lost customizations, broken trust | Own only the marked block; warn on out-of-block installer appends; never overwrite user content |
| No recovery path when the loader breaks the shell | User can't open a terminal to fix it | `ZSHPRO_DISABLE=1` escape hatch checked first; fail-open loader; documented recovery |
| Freezing machine-specific paths into a "portable" profile | Profile silently broken on the next machine | Keep dynamic values late-bound; portability test before shipping a profile |
| Surprising new-tab behavior (inherits dirty parent vs default) | Inconsistent env between tabs | Pick and document a policy; always normalize to a clean profile on startup |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **Switch:** Looks done if B works — verify **A is fully *gone*** (aliases, functions, options, PATH) and switching is *path-independent* (A→B→A == fresh B-then-A).
- [ ] **Deactivate manifest:** Looks done if it exists — verify **every** activate entry has a *real, tested* inverse (no no-ops, no guesses), especially options (snapshot-restore not blind toggle) and overridden base functions.
- [ ] **PATH handling:** Looks done if PATH is right once — verify **no growth/dupes after N round-trips**, and the *base* snapshot itself is clean (captured early, re-capture guarded).
- [ ] **Partial eval:** Looks done if the store is tidy — verify **dynamic values survived as references** (`$HOME`/`$(...)` not frozen) via a two-machine portability test.
- [ ] **Loader install:** Looks done if a shell opens — verify it's **idempotent** (install twice → identical `.zshrc`) and **fails open** (missing binary / bad manifest → working shell + warning, plus `ZSHPRO_DISABLE`).
- [ ] **Performance:** Looks done if startup feels fine on your machine — verify with `hyperfine 'zsh -i -c exit'` against budget, and confirm **zero subprocesses on the hot path**.
- [ ] **Concurrency:** Looks done with one terminal — verify with **many terminals**: switching one doesn't affect others; a new tab from a dirty parent normalizes; concurrent switches don't corrupt shared state.
- [ ] **Safety:** Looks done if your own profiles work — verify a **side-effecting / pulled profile is not run** until explicitly allowed; manifests are validated and integrity-checked.
- [ ] **Imperative split:** Looks done if declarative state switches — verify **imperative lines are detected and routed to the master block**, never silently dropped into a manifest they can't deactivate.

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| PATH doubled / residue accumulated | LOW | `PATH=$ZP_BASE_PATH` + re-activate; `typeset -U path` as backstop; ship `zsh-pro doctor --fix` to rebuild from base |
| Loader broke the shell (can't get a working terminal) | MEDIUM | `ZSHPRO_DISABLE=1` env (or `zsh -f` to bypass rc) → fix manifest/binary → re-enable; fall back to last-good manifest automatically |
| `.zshrc` clobbered / duplicate blocks | MEDIUM | Restore user content outside markers from git/backup; idempotent re-write collapses duplicate blocks; back up `.zshrc` before first modification |
| Profile frozen to one machine (portability lost) | MEDIUM | Re-ingest with dynamic values preserved; replace literal home/prefixes with `$HOME`/`$(...)`; add the line to the late-bound set |
| Imperative side effect left running (daemon/prompt) | HIGH | Often un-reversible in-session → require `exec zsh`/new shell to clear; *prevent* by classification, recover by documenting "open a fresh shell" |
| Concurrent-switch state corruption | MEDIUM | Atomic-rewrite the shared store from last-good; move active-profile to per-shell env so it can't recur |
| Ran an untrusted pulled profile | HIGH | Audit what executed; add trust gate retroactively; rotate any exposed secrets; hash-pin manifests going forward |

## Pitfall-to-Phase Mapping

Phase names are inferred from PROJECT.md's v2.0 target features (Ingest → Partial-eval → Git-backed store → Activate/deactivate manifest + loader → frontier zero-residue hot-switch spike). The roadmap may rename/reorder; the *prevention obligation* travels with the named phase.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. PATH doubling on re-source/switch | Activate/deactivate manifest | N-switch property test → PATH path-independent & dupe-free |
| 2. Leftover aliases/functions/options | Activate/deactivate manifest | Post-switch `alias`/`functions`/`setopt` == cold `zsh -f` + target-only baseline |
| 3. Parent-shell mutation (source not exec) | Sourced loader / shell integration (architectural invariant) | `checkout` is a sourced function eval'ing binary output; live shell changes |
| 4. Imperative side-effects can't be un-run | Ingest & categorize (classifier gate) + manifest | Every activate entry has a real inverse; imperative lines routed to master block |
| 5. Freezing late-bound values (portability) | Partial evaluation | Two-machine (`$HOME`/`$USER`) round-trip keeps dynamic values as references |
| 6. Dual-source drift (`.zshrc` vs store) | Git-backed store / ingest + loader install | Idempotent block writer (install twice → identical file); out-of-block content untouched |
| 7. Loader slowness (hot path) | Sourced loader / shell integration | `hyperfine 'zsh -i -c exit'` within budget; zero subprocesses on hot path |
| 8. Loader/tool failure breaks shell | Sourced loader / shell integration | Missing binary / bad manifest / syntax error → working shell + warning; `ZSHPRO_DISABLE` works |
| 9. Running/regenerating arbitrary config | Partial evaluation (static-only) + switch/store (trust gate) | Side-effect payload not executed on ingest; not eval'd on switch until allowed |
| 10. Concurrent-terminal state correctness | Activate/deactivate manifest (per-shell state) + store (atomic writes) | Parallel-switch stress test; new-tab-from-dirty-parent normalizes; no shared-state corruption |
| 11. Snapshotting a dirty base | Sourced loader / shell integration (snapshot placement) | Child `ZP_BASE_PATH` == parent's (no re-capture); from-`zsh -f` base has no managed dirs |

**Cross-cutting note for roadmap ordering:** Pitfalls 3, 7, 8, 11 are all properties of the *sourced loader*, and pitfalls 1, 2, 10 are all properties of the *manifest/switch loop* — these two components carry the bulk of the new risk and deserve the **frontier spike** PROJECT.md already plans for "zero-residue live hot-switch." The spike should de-risk, specifically: (a) capture-base-before-mutation ordering, (b) full deactivate→activate path-independence, (c) fail-open loader, and (d) per-shell state across concurrent tabs — *before* committing to the manifest format. Ingest-phase classification (Pitfall 4) and partial-eval staticness/portability (Pitfalls 5, 9) are upstream gates that, if wrong, make the switch loop *unfixable downstream* — so their correctness must land before the manifest is designed.

## Sources

- conda#11021 — PATH not correctly reset after deactivate (capture-before-mutate root cause): https://github.com/conda/conda/issues/11021
- conda#8070 — can't modify PATH in deactivate scripts; two-phase deactivate / `CONDA_RESTORE_PATH`: https://github.com/conda/conda/issues/8070
- conda#8703 — conda init blocks in rc files are not idempotent (managed-block append bug): https://github.com/conda/conda/issues/8703
- conda#9911 — race condition during concurrent conda activation: https://github.com/conda/conda/issues/9911
- conda activation deep-dive — "shell functions only write code; the shell must eval/source it"; `# >>> conda initialize >>>` managed block: https://docs.conda.io/projects/conda/en/latest/dev-guide/deep-dives/activation.html
- direnv — DIRENV_DIFF snapshot/diff model, "only backs up the diff of environments": https://direnv.net/ and https://github.com/direnv/direnv
- direnv#50 / #1084 — malformed `.envrc` / hook errors freezing/breaking the shell: https://github.com/direnv/direnv/issues/50 , https://github.com/direnv/direnv/issues/1084
- direnv security / allow model — `.envrc` is arbitrary code; trust gate: https://direnv.com/is-direnv-safe-to-use/ , https://man.archlinux.org/man/extra/direnv/direnv.toml.1.en
- asdf-direnv perf discussion — per-prompt hook startup cost / caching: https://github.com/asdf-community/asdf-direnv/discussions/57
- Environment Modules — prepend/append reverse on unload; `__MODULES_SHARE_*` reference counting for safe diffing: https://modules.readthedocs.io/en/latest/modulefile.html
- Python venv — activate prepends PATH / sets VIRTUAL_ENV; `deactivate` restores saved values: https://docs.python.org/3/library/venv.html
- NixOS declarative vs imperative — only pure/declarative config is reversible/reproducible; imperative → drift: https://medium.com/thenixos/what-is-declarative-configuration-in-nixos-understanding-declarative-vs-imperative-approaches-d24d4d144df6
- chezmoi templating / portable dotfiles — late binding (`.chezmoi.homeDir`) vs hardcoded paths: https://www.chezmoi.io/reference/commands/add/ , https://www.jeffyang.io/posts/configuring-portable-dotfiles/
- zsh startup performance — vanilla 50–100ms, budgets, `zprof`/hyperfine profiling: https://blog.openreplay.com/zsh-slow-startup-fix/ , https://www.mikekasberg.com/blog/2025/05/29/optimizing-zsh-init-with-zprof.html , https://how2.sh/posts/how-to-trace-slow-shell-startup-zsh-profiling-hyperfine/
- zsh `typeset -U path` — dedups PATH (re-source doubling backstop): https://dev.to/deni_sugiarto_1a01ad7c3fb/how-to-remove-duplicate-paths-in-zsh-on-macos-3l68
- bash/zsh `typeset -f` / `unset -f` — capture/remove function definitions (snapshot-restore): https://www.gnu.org/software/bash/manual/html_node/Shell-Functions.html
- safe rc sourcing / fail-open guards: https://rc-docs.northeastern.edu/en/latest/best-practices/shell_environment.html
- concurrent shell state / race conditions (flock, atomic writes): https://www.mindfulchase.com/explore/troubleshooting-tips/programming-languages/troubleshooting-subshell-and-concurrency-issues-in-bash-scripts.html
- claude-code#4014 — concurrent shell-snapshot state corruption (shared state dir): https://github.com/anthropics/claude-code/issues/4014

---
*Pitfalls research for: branchable shell-environment manager for zsh (sourced activate/deactivate loader on the startup path)*
*Researched: 2026-06-25*
