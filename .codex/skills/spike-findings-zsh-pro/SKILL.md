---
name: spike-findings-zsh-pro
description: Use when implementing zsh-pro live shell state capture, shared semantic diffs, revision-aware terminal synchronization, safe auto-apply hooks, or related Phase 7 behavior. Contains validated requirements, implementation patterns, constraints, and runnable source evidence from project spikes.
---

# Spike Findings for zsh-pro

Use these findings when planning, implementing, reviewing, or testing the live shared-worktree behavior validated by the project's spikes. Treat the reference as an implementation blueprint and the copied source as executable evidence, not as production-ready code to transplant wholesale.

## Project Context

zsh-pro should feel like a local Git worktree after one bootstrap ingest: supported live-shell changes become a shared categorized diff, basic branch and commit operations work without staging, and independent terminals synchronize declarative state without becoming mirrored shell processes.

## Requirements

- `zsh-pro ingest` is bootstrap, not the routine update command.
- One shared materialized worktree is used in this phase; multiple worktrees are deferred.
- Capture resulting supported shell state, not command-text intent.
- There is no `add` or staging layer; commit includes every supported detected change.
- Dirty checkout is blocked until commit or explicit destructive reset.
- Independent terminals share status/diff and auto-apply newer supported state by default at safe boundaries.
- Auto-apply is configurable and has an explicit `zsh-pro sync` fallback.
- zsh-pro does not multiplex terminals or synchronize process-local state such as `PWD` and jobs.
- Merge, rebase, remotes, and other advanced Git operations are deferred.
- Live capture operates only on identities admitted by the materialized profile or an explicit safe admission path; arbitrary inherited shell state is never persisted.
- Runtime publication uses per-identity deltas against the last acknowledged revision; a stale shell never publishes a whole replacement snapshot.
- Interactive capture publishes at `precmd`, while default-on pull runs at `zle-line-finish`; explicit sync remains available for manual/non-ZLE recovery.
- Same-identity concurrency is first-lock-wins with a visible loser conflict; unrelated changes compose.
- Runtime hooks are deadline-bounded, atomically persist shared state, validate generated zsh before applying, acknowledge after apply, and fail open.
- Revision history is compacted; a stale shell older than the retained conflict window must cleanly reconcile or surface a history-gap conflict before publishing.

## Findings Index

| Feature area | Implementation blueprint | Runnable evidence |
|---|---|---|
| Live shell state synchronization | [references/live-shell-state-synchronization.md](references/live-shell-state-synchronization.md) | [sources/001-live-state-capture-and-multi-terminal-sync/README.md](sources/001-live-state-capture-and-multi-terminal-sync/README.md) |

## Processed Spikes

- `001-live-state-capture-and-multi-terminal-sync` — VALIDATED
