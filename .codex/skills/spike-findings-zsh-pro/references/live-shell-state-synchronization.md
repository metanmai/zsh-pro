# Live Shell State Synchronization

## Requirements

- Capture the resulting supported shell state rather than attempting to parse command intent.
- Use one shared materialized worktree and treat ingest as bootstrap only.
- Admit identities from the materialized profile or an explicit safe admission path. Do not persist arbitrary inherited state.
- Publish per-identity deltas from each shell's last acknowledged revision. Never publish a stale whole-profile snapshot.
- Publish a completed command's state at `precmd`; pull newer shared state at `zle-line-finish` so the accepted line sees new aliases and other managed identities before parsing and expansion.
- Keep default-on auto-apply configurable and provide explicit `zsh-pro sync` recovery.
- Compose unrelated concurrent changes. For overlapping identities, the first lock acquirer wins and the loser receives a visible conflict.
- Bound all hook work, use atomic persistence, validate generated zsh before sourcing it, acknowledge only after application and a fresh snapshot, and fail open.
- Compact revision history. A shell behind the retained conflict window must reconcile cleanly or surface a history-gap conflict before it can publish.
- Exclude process-local state such as `PWD` and jobs; advanced Git operations and multiple worktrees remain out of scope.

## How to Build It

### 1. Derive an explicit identity registry

Build the managed identity set from the materialized profile and a deliberate admission operation for newly created variables, aliases, functions, arrays, and supported options. Store category and name as a stable identity key. The spike used `kind + NUL-like separator + name`; production should preserve the same category-aware semantics.

The prototype's `SPIKE_*` and `spike_*` namespaces are only a stand-in for this registry. Its exclusion rules demonstrate defense in depth for bookkeeping, volatile names, and likely secrets, but name filtering alone is not an admission policy.

### 2. Capture a semantic, framed snapshot

Read zsh's parameter, alias, function, option, `path`, and `fpath` tables. Preserve arrays as ordered arrays, and frame records so embedded newlines, quotes, spaces, empty elements, and colons cannot change their meaning. The validated prototype uses NUL-delimited records and rejects malformed, oversized, or excessive input.

Compare the new snapshot with the shell's acknowledged baseline to produce add, change, and explicit remove operations per identity. Unchanged identities do not enter the delta.

### 3. Keep shared and per-shell revision state

The shared state needs:

- a monotonic head revision;
- the current authoritative value of every admitted identity;
- a bounded suffix of revision events containing changed identities;
- the revision through which earlier events were compacted.

Each attached shell needs:

- its last successfully applied and acknowledged revision;
- its acknowledged semantic baseline;
- pending revision and behind state;
- auto-apply preference;
- a visible conflict or last recovered hook error.

This separation lets a stale shell publish only its local delta while detecting whether the same identities changed after its base revision.

### 4. Publish under a bounded lock

At `precmd`, snapshot the now-settled shell state and calculate a delta from its acknowledged baseline. Under a short-deadline advisory lock:

1. Load and validate the shared state.
2. Find shared changes after the shell's acknowledged revision.
3. If the local delta overlaps those identities, leave shared state untouched and record a visible conflict.
4. Otherwise apply the delta to the authoritative identity map, append one revision event, compact the retained suffix, and atomically replace the shared file.

The first process to acquire the lock wins an overlapping race; terminal identity or scheduling must not be used to manufacture a different winner. Disjoint deltas are safe to compose even when both publishers began from an older revision.

### 5. Pull only at safe shell boundaries

Install composable hooks instead of replacing user hook functions:

```zsh
autoload -Uz add-zsh-hook add-zle-hook-widget
add-zsh-hook precmd _zsh_pro_publish
add-zle-hook-widget line-finish _zsh_pro_pull
```

`precmd` owns publication of the previous foreground command's resulting state. `zle-line-finish` pulls only: it runs after ZLE accepts a line but early enough that synchronized aliases and other managed definitions affect parsing and expansion. Never asynchronously mutate a shell while a foreground command is running.

Non-interactive, non-ZLE, disabled-auto-apply, and recovery paths use the same pull/apply mechanism through explicit `zsh-pro sync`. Status must expose the head revision, applied revision, behind state, and conflict.

### 6. Render, validate, apply, then acknowledge

Reduce all shared events since the shell's revision to the final change for each identity. Render those changes as safely quoted zsh, including explicit removals and array element boundaries. The apply sequence is strict:

1. Write the generated patch to a temporary file and atomically rename it.
2. Run `zsh -n` against the patch within the hook deadline.
3. Source it only at the safe boundary.
4. Capture a fresh semantic snapshot.
5. Acknowledge the new baseline and applied revision only if that snapshot succeeds.

An apply or acknowledgement failure stays visible for diagnosis, but the hook returns success so the user's prompt or entered command continues.

### 7. Handle compacted history explicitly

Retain a bounded revision suffix for overlap detection and reduce it efficiently by revision. If a shell's acknowledged revision is older than the compaction floor:

- a clean shell may reconcile against the current authoritative identity map;
- a shell with an unacknowledged local delta must stop and expose a history-gap conflict.

It must not publish until the user or a defined resolution policy reconciles the gap.

### 8. Verify behavior through independent shells

Keep unit coverage for framing, semantic diffs, removals, quoting, generated patch syntax, history compaction, and history-gap reconciliation. Add retained interactive testing with two independent `zsh -f` processes sharing one temporary state root. Prove:

- every supported category converges, including multiline and removal cases;
- the receiving shell's next accepted command sees a newly synchronized alias;
- manual mode remains visibly behind until explicit sync;
- disjoint stale writes compose and an overlapping race yields one visible loser conflict;
- a foreground command is not changed mid-run;
- missing helpers, lock contention, malformed snapshots, and abandoned partial writes fail open;
- excluded values never appear in shared state or forensic logs;
- no-op hook latency remains within the production budget.

The copied verifier is the executable baseline:

```bash
cd .codex/skills/spike-findings-zsh-pro/sources/001-live-state-capture-and-multi-terminal-sync
./verify.sh
```

## What to Avoid

- Do not pull only at `precmd`; a shell waiting at a prompt would run one stale command.
- Do not rely on `preexec` for pull; alias expansion has already happened.
- Do not apply from a background watcher while a foreground command is active.
- Do not persist every inherited environment variable, function, alias, or option discovered in the process.
- Do not use whole-snapshot last-writer-wins or let a stale shell replace current shared state.
- Do not predetermine race winners by terminal name; the first bounded lock holder owns the overlapping publication.
- Do not advance the shell's applied revision before validation, sourcing, and post-apply acknowledgement succeed.
- Do not replay an unbounded history or run Git on every prompt/accepted line.
- Do not let lock, helper, parsing, validation, or persistence errors block the user's shell.

## Constraints

- `zle-line-finish` is interactive-ZLE-specific, so explicit sync is a required fallback.
- The prototype caps snapshots at 2 MiB and 10,000 records, retains 128 revision events, and bounds helper calls to 250 ms. Production values may change only with measured justification; the bounds themselves are mandatory.
- Atomic writes require temporary files in the destination filesystem, flush-before-rename behavior, and readers that ignore abandoned partial files.
- Same-identity conflict behavior is deterministic after lock acquisition, but process scheduling determines which terminal acquires the lock first.
- A production identity registry must be derived from profile materialization; the prototype namespaces are not sufficient policy.
- Forensic events should record identity names, categories, counts, revisions, durations, exclusion reasons, conflicts, and recovered errors without leaking excluded values.
- Runtime synchronization covers declarative managed state only. It does not synchronize `PWD`, jobs, process trees, terminal UI, or other process-local state.

## Origin

- Spike: `001-live-state-capture-and-multi-terminal-sync`
- Verdict: VALIDATED
- Original findings: `.planning/spikes/001-live-state-capture-and-multi-terminal-sync/README.md`
- Copied runnable evidence: `../sources/001-live-state-capture-and-multi-terminal-sync/`
