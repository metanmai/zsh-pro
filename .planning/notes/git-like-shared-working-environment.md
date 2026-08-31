---
title: Git-Like Shared Working Environment
date: 2026-08-17
context: User-driven Docker UAT exposed that repeated ingest and per-terminal snapshots do not match the intended Git mental model.
status: accepted
---

# Git-Like Shared Working Environment

## Decision

zsh-pro manages a shared, declarative shell-environment worktree; it does not multiplex terminals or attempt to make independent zsh processes share process memory.

`zsh-pro ingest` is a bootstrap operation. It imports the existing startup configuration, seeds the baseline branch, installs the loader, and materializes the initial working profile. Routine edits then come from supported changes made directly in a live shell.

## User Model

| Git concept | zsh-pro concept |
|-------------|-----------------|
| Repository | The existing Git-backed profile store |
| Branch / HEAD | The current environment profile |
| Working tree | One shared materialized working profile |
| File edit | A supported live-shell state change |
| `git status` | `zsh-pro status` |
| `git diff` | Categorized `zsh-pro diff` |
| `git commit -am` | `zsh-pro commit -m` with every supported change; no staging |
| `git checkout -b` | Create a profile branch from current committed state and switch to it |
| Clean-checkout rule | Checkout blocks while the shared worktree is dirty |
| Destructive discard | An explicit reset command restores the current commit |

## Canonical State

The bare Git repository remains committed history. Phase 7 adds one materialized working-profile location, conceptually:

```text
~/.local/share/zsh-pro/
├── repo.git/                 # commits, trees, refs, branches
└── worktree/
    ├── profile.json          # canonical structured uncommitted state
    ├── profile.zsh           # deterministic generated view
    └── revision              # worktree concurrency/version token
```

The exact paths remain an implementation decision. `profile.json` is authoritative; `profile.zsh` is generated. Git does not monitor commands itself: zsh-pro shell hooks capture state deltas and atomically update the materialized profile, while Git supplies comparison and history.

## Capture, Do Not Parse Command Intent

Command text is diagnostic context, not the source of truth. `source setup.zsh`, `eval`, a function call, or a plugin can change many identities without describing those changes in the command string. zsh-pro therefore compares supported live state before and after a command and records semantic operations keyed by category and identity:

- environment variable set/change/unset;
- alias add/change/remove;
- function add/change/remove;
- PATH/FPATH list changes;
- supported zsh option changes.

Volatile shell internals such as `PWD`, `OLDPWD`, `SHLVL`, `_`, random values, terminal metadata, zsh-pro bookkeeping, jobs, history, and editor buffers are excluded. Unsupported or irreversible state remains unmanaged. Existing secret detection and `SecretRef` behavior remain mandatory at the worktree and commit boundaries.

## Multi-Terminal Synchronization

Each terminal remains an independent zsh process. They share the branch and working profile, so `status` and `diff` report the same state everywhere.

Capture and apply are separate:

1. After a command finishes, the originating shell computes its supported semantic delta and publishes that delta atomically to the shared worktree.
2. Before another command observes managed state, a shell checks the worktree revision and applies newer state at a safe between-command boundary.
3. Auto-apply is enabled by default and configurable. With it disabled, zsh-pro reports that the shell is behind and waits for `zsh-pro sync`.
4. No synchronization runs in the middle of a foreground command.

Because a sibling process cannot rewrite another shell's memory, an idle terminal converges when its next hook boundary runs. This is intentionally different from tmux, where multiple clients may control one shell process.

## Concurrency Contract

Writers publish semantic deltas under an inter-process lock and against an observed worktree revision; they must not replace the entire snapshot derived from one stale terminal. Unrelated identities should compose. The Phase 7 spike must select and prove deterministic behavior for simultaneous changes to the same identity before implementation planning.

## Basic Command Surface

The intended first release is limited to:

```text
zsh-pro ingest
zsh-pro status
zsh-pro diff
zsh-pro commit -m <message>
zsh-pro branch
zsh-pro branch <name>
zsh-pro checkout <name>
zsh-pro checkout -b <name>
zsh-pro sync
zsh-pro reset --hard
zsh-pro config set auto-apply true|false
```

Exact spelling is finalized during Phase 7 discussion. There is no `add`/staging layer, merge, rebase, cherry-pick, remote collaboration, or multiple worktrees in this phase.

## Superseded Behavior

The completed per-terminal activation model remains valuable runtime machinery, but it is no longer the final user-facing model for current branch and dirty state. Phase 7 introduces one shared branch/worktree while retaining process-local reverse manifests so applying synchronized state remains reversible and residue-free inside each shell.
