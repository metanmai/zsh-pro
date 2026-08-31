# Requirements: zsh-pro

**Defined:** 2026-06-25
**Milestone:** v2.0 "Branchable Shell Environments"
**Core Value:** `checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses cleanly with zero residue, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).

## Requirements

Requirements for this milestone. Each maps to a roadmap phase.

### Ingest

- [x] **ING-01**: `zsh-pro` ingests `~/.zshrc` into a categorized, regenerable representation (env / aliases / functions / PATH / options) that round-trips back to behavior-equivalent zsh. Untouched statements are preserved verbatim (`Block.Text`); only rewritten declarative slices are templated.
- [x] **ING-02**: Ingest classifies each statement as **declarative** (set/unset-reversible → switchable) or **imperative** (run-once, side-effecting → unmanaged master block). A misclassified imperative line is a zero-residue violation by construction, so this gate is correctness-critical.

### Partial Evaluation

- [x] **EVAL-01**: The stored representation resolves syntactically-static values and keeps dynamic ones (`$HOME`, `${...}`, `$(...)`, conditionals) **late-bound** — "static" means syntactically constant, never resolved against the current machine's disk/env. Portability across machines is preserved.

### Profiles / Store

- [x] **PROF-01**: The representation is stored as a git-backed repo **via the `git` binary** (no new Go dependency; mirrors the existing `zsh -f` subprocess pattern) where each branch is an environment profile.
- [x] **PROF-02**: The user can create, list, and switch profiles (`checkout <branch>`); the active profile is tracked **per-terminal** (env-var-carried state), never a global file (avoids the conda concurrent-activation race class). *(This records the completed v2.0 activation model. Phase 7 supersedes the user-facing branch/worktree coordination with WORK-01 and SYNC-01 while keeping each running shell process independent.)*
- [x] **PROF-03**: Detected secrets are **excluded from the committed profile by default** (no encryption dependency); the user is told what was withheld. Reuses the existing secret detection from the ingest engine. *(Phase 3 introduced store-side exclusion: a literal secret becomes a resolver-agnostic `SecretRef` (`kind:key`), is captured to the OS keychain or git-ignored vault backend, stays out of committed blobs, and produces a withheld report; already-dynamic secrets commit verbatim. Phases 4/5 completed runtime dereference-on-switch, and Phase 6 completed the real-`~/.zshrc` end-to-end ingest proof.)*

### Switch / Activation

- [x] **SW-01**: A profile's declarative state is applied to the current shell via a **sourced loader that `eval`s emitted shell code** (a child process cannot mutate its parent shell). All zsh syntax lives in one emit path.
- [x] **SW-02**: Switching profiles **deactivates** the prior profile's managed state (reverse-diff manifest: `unalias`, `unset -f`, restore env to captured prior values, rebuild PATH from a captured base) then **activates** the new one — with **zero residue**: no leftover aliases/functions/options, no PATH growth, and base/unmanaged state left untouched (ownership-aware, Lmod-style — don't remove `/usr/local/bin` just because a profile also added it).
- [x] **SW-03**: *(Frontier)* Switching works **live in an already-open terminal**, not just new shells — validated by the Phase-1 spike asserting a byte-identical environment after `activate → switch → switch-back` on a no-op round-trip (env **and** aliases/functions/options).

### Bootstrap / Reliability

- [x] **BOOT-01**: A single **idempotent, BEGIN/END-marked `.zshrc` block** bootstraps the loader and preserves an unmanaged "master block" for imperative run-once code. Re-running the installer never duplicates the block.
- [x] **BOOT-02**: The loader is **fail-open and fast** — a broken, missing, or slow `zsh-pro` never locks the user out of a working shell (guarded sourcing, `zsh -n`-validated manifests, last-good fallback, a `ZSHPRO_DISABLE=1` escape hatch) and adds only a small, file-sourced startup cost (no git/subprocess on the hot path).

### Git-Like Working Environment

- [ ] **WORK-01**: `zsh-pro ingest` is a bootstrap operation that seeds one shared, materialized working profile and its baseline commit. After bootstrap, normal profile editing happens through live shell changes; the user does not re-ingest to update the current environment.
- [ ] **WORK-02**: After each command, zsh-pro detects changes to supported user-controlled declarative state (environment variables, aliases, functions, PATH/FPATH, and options), categorizes the resulting state delta rather than guessing from command text, filters volatile shell noise, preserves the existing secret boundary, and updates the shared working profile without an `add` or staging step.
- [ ] **WORK-03**: The shared working profile supports the basic Git-shaped workflow: `status`, categorized `diff`, `commit`, branch list/create, `checkout`, and an explicit destructive reset. New branches fork the current profile, the last checked-out branch remains current, and checkout is blocked while the worktree is dirty unless the user explicitly discards those changes.
- [ ] **SYNC-01**: Independent zsh processes observe the same branch and worktree. Changes captured in one terminal become visible to every terminal's `status`/`diff`; other terminals automatically apply newer supported state at safe between-command boundaries by default. Auto-apply is configurable, and `zsh-pro sync` performs the same convergence explicitly when it is disabled.
- [ ] **SYNC-02**: Shared-worktree writes are atomic and revision-aware. Concurrent terminals cannot silently replace unrelated changes or corrupt the profile; same-identity races produce deterministic, visible behavior established by the Phase 7 spike. Synchronization never interrupts a foreground command and remains fail-open.

## Future Requirements

Deferred to future work. Tracked but not in this roadmap.

### Profile Sharing

- **SHARE-01**: Pull/push profiles from a remote git repo, with a **trust gate** before applying a profile authored elsewhere (per-profile or per-hash approval).

### Ergonomics

- **AUTO-01**: Optional auto-activate on `cd` (direnv-style per-prompt `chpwd`/`precmd` hook), with a hard "no git subprocess per prompt" constraint and its own trust surface.

### Parked from v1.1 (analyzer milestone)

- The trustworthy PATH-extraction + notation-dedup work (PATH-01/02/03) re-scopes into **ING-01** (PATH is one ingest category needing correct split + canonicalization). The analyzer-reporting/oracle items (COV-01/02/03) remain parked — see `milestones/v1.1-ROADMAP.md`.

## Out of Scope

Explicitly excluded for this milestone. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Filesystem / live-`$HOME`/`$(...)` resolution | Resolving dynamic values against a specific machine freezes a profile to that machine and destroys cross-machine portability — the whole point of branches |
| Making the imperative startup surface switchable | Side-effecting run-once code (daemons, `eval`, version-manager init) can't be cleanly un-run; it stays in the unmanaged master block. Only declarative state is switchable |
| Other shells (bash, fish) | The activation model is zsh-specific (`zmodload zsh/parameter`, `unalias`/`unset -f`/`unsetopt`, `add-zsh-hook`) |
| Multi-file config graphs (sourced fragments, `*.zsh` beyond the entry point) | Single-entry ingest first; deep source-graph following is a later concern |
| Profile encryption / syncing secrets | Needs a crypto dependency (forbidden); secrets are excluded from the tree by default instead (PROF-03) |
| Remote profile sharing + trust gate | Future (SHARE-01) — v2.0 targets local profiles & branches |
| Auto-activate on `cd` | Future (AUTO-01) — v2.0 is explicit-`checkout` only |
| Git staging/index UX (`add`, partial commits) | The first working-tree UX commits all supported detected changes; staging can be introduced later if real use demands it |
| Merge, rebase, cherry-pick, and remote collaboration | Phase 7 intentionally ships only the basic local branch/commit/checkout workflow |
| Multiple zsh-pro worktrees | Phase 7 has one shared worktree; independent worktrees and their coordination are deferred |
| Terminal multiplexing or mirrored shell processes | Users who want two clients controlling one shell should use tmux; zsh-pro synchronizes declarative profile state across independent shells |
| Synchronizing `PWD`, jobs, command buffers, history, or arbitrary process-local state | These belong to each zsh process and are not part of the Git-backed environment profile |
| PATH ordering/precedence analysis · classifier precision overhaul | Captured design stances for later; independent of the manager's core switch loop |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SW-03 | Phase 1 — SPIKE: Zero-Residue Live Hot-Switch | Complete |
| ING-01 | Phase 2 — IR + Partial Evaluation | Complete |
| ING-02 | Phase 2 — IR + Partial Evaluation | Complete |
| EVAL-01 | Phase 2 — IR + Partial Evaluation | Complete |
| PROF-01 | Phase 3 — Git-Backed Store | Complete |
| PROF-02 | Phase 3 — Git-Backed Store | Complete |
| SW-01 | Phase 4 — Manifest Builder + Emit | Complete |
| SW-02 | Phase 4 — Manifest Builder + Emit | Complete |
| BOOT-01 | Phase 5 — Runtime Loader + CLI + Bootstrap | Complete |
| BOOT-02 | Phase 5 — Runtime Loader + CLI + Bootstrap | Complete |
| PROF-03 | Phase 3 (store-side exclusion + reference) → Phase 4/5 (runtime deref) → Phase 6 (end-to-end ingest) | Complete |
| WORK-01 | Phase 7 — Git-Like Shared Working Environment | Pending |
| WORK-02 | Phase 7 — Git-Like Shared Working Environment | Pending |
| WORK-03 | Phase 7 — Git-Like Shared Working Environment | Pending |
| SYNC-01 | Phase 7 — Git-Like Shared Working Environment | Pending |
| SYNC-02 | Phase 7 — Git-Like Shared Working Environment | Pending |

**Coverage:**

- Milestone requirements: 16 total (ING-01/02, EVAL-01, PROF-01/02/03, SW-01/02/03, BOOT-01/02, WORK-01/02/03, SYNC-01/02)
- Mapped to phase coverage: **16/16** — no orphans; requirements may span phases where their implementation crosses a durable boundary (for example, PROF-03).

---
*Requirements defined: 2026-06-25 (milestone v2.0)*
*Traceability populated: 2026-06-25 (roadmap creation)*
