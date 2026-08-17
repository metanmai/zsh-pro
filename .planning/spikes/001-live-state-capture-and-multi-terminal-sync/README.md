---
spike: 001
name: live-state-capture-and-multi-terminal-sync
type: standard
validates: "Given two independent interactive zsh processes sharing one materialized profile, when either changes supported declarative state, then the shared semantic diff updates atomically and the other shell safely auto-applies it before its next command without whole-profile clobbering."
verdict: VALIDATED
related: []
tags: [zsh, hooks, worktree, concurrency, synchronization]
---

# Spike 001: Live-State Capture and Multi-Terminal Sync

## What This Validates

Given two independent interactive zsh processes sharing one materialized profile, when either changes supported declarative state, then:

- hook timing can capture the resulting semantic delta rather than infer intent from command text;
- environment variables, aliases, functions, PATH/FPATH, and supported options can be classified without volatile-shell noise;
- the shared worktree can be updated atomically and revision-aware;
- the second shell can auto-apply newer supported state at a safe boundary before its next command observes managed state;
- unrelated concurrent changes compose, while same-identity races have deterministic visible handling;
- disabling auto-apply leaves the shell visibly behind until an explicit sync;
- prompt/command-hook overhead remains bounded and the shell fails open.

## Kill Criteria

Phase 7 must not proceed with automatic capture/apply unchanged if any of these remain true after the spike:

1. A stale shell must publish a full snapshot and silently erase an unrelated newer change.
2. Applying shared state can interrupt or alter a foreground command already in progress.
3. Supported-state capture includes secrets, volatile process noise, or zsh-pro's own bookkeeping as ordinary committed values.
4. Failures in locking, parsing, generation, or validation can prevent the next interactive prompt.
5. The steady-state hook requires an unbounded Git/subprocess operation on every prompt or command.

## Experiments

1. **Hook lifecycle:** Observe `preexec` and `precmd` ordering across successful, failed, compound, sourced, and function-driven mutations.
2. **Semantic capture:** Snapshot and diff every supported category, including add/change/remove and quoting/newline edge cases; verify explicit exclusion of volatile identities.
3. **Two-shell convergence:** Drive two real interactive zsh processes against one temporary worktree and prove that Terminal B's next command observes Terminal A's published change when auto-apply is on.
4. **Manual mode:** Disable auto-apply in Terminal B, prove it remains unchanged but reports behind, then converge with `zsh-pro sync`.
5. **Concurrency:** Interleave unrelated and same-identity writes under a revision token and lock; reject or surface stale collisions without whole-profile replacement.
6. **Failure/performance:** Inject malformed state, lock contention, interrupted writes, and missing helpers; measure no-op hook overhead and prove fail-open behavior.

## Research

The experiment was grounded in the current zsh manual and then checked against zsh 5.9 in a real TTY:

- [`precmd` runs before each prompt, while `preexec` runs after a command has been read and is about to execute](https://zsh.sourceforge.io/Doc/Release/Functions.html). The documented `preexec` arguments already include an alias-expanded command, so it is too late to guarantee a newly synchronized alias affects the accepted line.
- [`zle-line-finish` runs when the line editor finishes reading a line](https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html). A direct tmux probe proved a variable installed there was visible to expansion of the line that had just been accepted.
- [`add-zsh-hook` and `add-zle-hook-widget` compose with existing hook owners](https://zsh.sourceforge.io/Doc/Release/User-Contributions.html), avoiding replacement of a user's existing `precmd` or special ZLE widget.
- [`zsh/parameter` exposes semantic tables for options, aliases, functions, and parameter metadata](https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html). In particular, function definitions are writable through the `functions` associative array and options have explicit `on`/`off` values.
- [Zsh arrays preserve element boundaries and order](https://zsh.sourceforge.io/Doc/Release/Parameters.html), so PATH/FPATH are framed as arrays rather than split/rejoined colon strings.

| Approach | Pros | Cons | Status |
|----------|------|------|--------|
| `precmd` capture and pull only | Simple; always after the prior foreground command | A terminal already sitting at a prompt can stay stale until after its next command | Rejected |
| `preexec` pull | Runs immediately before execution | Alias expansion has already happened; too late for the complete supported state surface | Rejected |
| Background watcher applying into the shell | Lower apparent latency | Cannot safely mutate an executing foreground shell; introduces asynchronous ZLE/process coupling | Rejected |
| `precmd` publish + `zle-line-finish` pull | Captures resulting state, applies after line acceptance but before parsing/expansion, and never mutates a running command | Interactive-ZLE-specific; still needs an explicit sync fallback | Chosen |
| Full-snapshot last-writer-wins | Easy persistence model | A stale shell can erase unrelated newer changes | Rejected |
| Per-identity delta + revision/lock | Unrelated stale changes compose; same-key races are visible | Requires revision history and explicit conflict resolution | Chosen |

The prototype deliberately admits only experiment-owned identities (`SPIKE_*`, `spike_*`), PATH/FPATH, and an option allowlist. That namespace stands in for the real materialized profile's identity policy. Persisting every inherited environment variable or function merely because it exists would violate the secret/noise kill criterion.

## How to Run

Run the complete repeatable verification:

```bash
cd .planning/spikes/001-live-state-capture-and-multi-terminal-sync
./verify.sh
```

Start a retained interactive two-pane demo:

```bash
./demo.sh --detached
tmux attach -t zsh-pro-spike-001
```

Pane A and Pane B are independent `zsh -f` processes. Try this in A:

```zsh
export SPIKE_COLOR=amber
alias spike_hi='print -r -- synced'
```

Then enter this in B; the `line-finish` boundary pulls before zsh parses the line:

```zsh
print -r -- "$SPIKE_COLOR"
spike_hi
```

Use `_zp_spike_set_auto_apply off`, `_zp_spike_sync`, `_zp_spike_status`, and `_zp_spike_resolve_shared` to exercise manual mode and conflict resolution.

## What to Expect

- Environment, alias, function, PATH/FPATH, and supported option add/change/remove operations converge semantically.
- B's newly entered command observes A's published state, including aliases that must exist before parsing.
- With auto-apply off, B prints its old state and `status` reports `behind=true`; `_zp_spike_sync` converges it.
- Disjoint stale writes both survive. A same-identity race selects the first locked writer and records a visible conflict on the loser.
- Missing helpers and lock contention set diagnostic status but do not block the entered command.
- `events.jsonl` contains timestamps, categories, revisions, event counts, durations, exclusions, conflicts, and recovered failures without excluded values.

## Observability

Each demo prints its temporary root. The important artifacts are:

- `state/state.json` — authoritative revision, admitted shared entries, and per-revision deltas;
- `state/shells/A.json` and `B.json` — local applied revision, baseline fingerprints/state, behind flag, and conflict;
- `state/events.jsonl` — append-only forensic event export;
- `spike-sync status --root <state-root> [--json]` — current convergence/conflict view;
- `spike-sync summary --root <state-root>` — event counts and p50/p95/max cycle durations.

## Investigation Trail

- 2026-08-17: Spike defined from user-facing Docker UAT and the decision to use one shared worktree with default-on configurable auto-apply. No implementation experiment has run yet.
- 2026-08-17: A real tmux/zsh probe established that `zle-line-finish` state changes are visible to expansion of the accepted command. This also avoids the documented `preexec` alias-expansion problem.
- 2026-08-17: The first prototype used whole local semantic snapshots as baselines, NUL framing for multiline-safe values, a locked JSON revision log, atomic fsync+rename writes, syntax-validated generated patches, and post-apply acknowledgement.
- 2026-08-17: Environment, aliases, functions, PATH, options, multiline quoting, and removals converged. Function application initially created a literal `$'spike_fn'` key; changing the generated patch to assign through a temporary associative-array subscript variable fixed it and gained a regression test.
- 2026-08-17: Manual mode produced `behind=true` and required explicit sync. Stale disjoint changes composed; a barrier-released same-key race produced one first-lock winner and one visible conflict, then explicit shared resolution converged the loser.
- 2026-08-17: A foreground `sleep` did not receive a concurrently published value until its next safe boundary. Missing-helper and lock-contention paths returned 127/124 while the entered commands still ran. A malformed snapshot was rejected without a revision change, and an abandoned partial file did not affect the atomically written state.
- 2026-08-17: The capture policy was tightened from arbitrary live process state to admitted experiment identities. This is a required production constraint: the materialized profile/explicit admission policy must define which identities are eligible, while secret/bookkeeping identities remain excluded.
- 2026-08-17: Revision replay was capped at 128 events. Active shells use a binary-searched recent suffix; a shell older than the compaction floor may full-reconcile only when clean, while an unsynced local delta stops with an explicit history-gap conflict.
- 2026-08-17: Final fresh-session verification passed all eight groups. Fifty no-op cycles took 0.449063 seconds wall-clock (~8.98 ms each); 98 helper samples measured p50 1.427 ms, p95 8.653 ms, and max 14.624 ms.

## Results

**Verdict: VALIDATED.** Two independent interactive zsh processes can safely publish and apply admitted semantic deltas using `precmd` plus `zle-line-finish`, without Git or an unbounded subprocess in the steady-state helper.

Evidence from the final clean run:

- all eight verifier groups passed with shared revision 8 and both shells applied at revision 8;
- 8 semantic publications, 15 patch acknowledgements, one deliberate conflict, one explicit conflict resolution, and two recovered fail-open errors were recorded;
- secret/bookkeeping values never appeared in shared state or logs; only the exclusion name/reason was recorded once;
- no remote update was applied during an active foreground command;
- unrelated stale changes composed per identity, while same-identity overlap stopped and surfaced a conflict instead of replacing the shared snapshot;
- the helper validates a 2 MiB/10,000-record cap, caps replay history at 128 revisions, uses an advisory lock, fsyncs temporary files, atomically renames, and requires syntax validation plus an acknowledgement before advancing the shell's applied revision.

### Constraints for Phase 7

1. The production identity set must come from the materialized profile and an explicit safe admission path for new identities. Never persist arbitrary inherited shell state.
2. Publish semantic per-identity deltas against the shell's last acknowledged revision. Never let a stale terminal replace the full profile.
3. Pull at `zle-line-finish` for entered interactive commands and use `precmd` to publish the prior command's resulting state. Keep explicit `sync` for non-ZLE/manual recovery.
4. Apply only at safe boundaries, validate generated source before sourcing, and acknowledge only after a fresh post-apply snapshot.
5. Preserve default-on configurable auto-apply, visible behind/conflict state, bounded deadlines, atomic writes, and fail-open hook returns.
6. The first lock acquirer wins a same-key race; scheduling does not predetermine which terminal wins, but the loser must deterministically stop with a visible conflict.
7. Compact revision and forensic history in production. A shell older than the retained conflict window must never publish until it cleanly reconciles or explicitly resolves its history-gap conflict.
