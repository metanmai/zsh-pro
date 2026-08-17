# Phase 7: Git-Like Shared Working Environment - Research

**Researched:** 2026-08-17
**Domain:** Revisioned live-zsh state capture, shared materialized worktree, Git-plumbing branch workflow, and multi-process convergence
**Confidence:** HIGH

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| WORK-01 | `zsh-pro ingest` seeds one shared materialized working profile and baseline commit; subsequent editing comes from live shell changes. | Use ingest as the only bootstrap/materialization transaction, then persist a versioned worktree document whose base is the committed profile. `[VERIFIED: .planning/REQUIREMENTS.md]` |
| WORK-02 | Capture supported resulting state for environment variables, aliases, functions, PATH/FPATH, and options; categorize changes, filter noise and secrets, and update without staging. | Use framed semantic snapshots, an admitted-identity registry, per-identity deltas, an overlay/tombstone model, and direct worktree publication. `[VERIFIED: fresh Spike 001 verifier]` |
| WORK-03 | Provide status, categorized diff, commit, branch list/create, checkout, and destructive reset; fork the current profile, retain the last checkout, and block dirty checkout. | Put workflow policy in a worktree service over the existing Git-plumbing store; commit all dirty identities and guard checkout/reset under the same worktree lock. `[VERIFIED: codebase + .planning/REQUIREMENTS.md]` |
| SYNC-01 | Independent zsh processes share branch/worktree state, default to safe-boundary auto-apply, permit disabling it, and converge explicitly with `sync`. | Publish on `precmd`, pull/apply on `zle-line-finish`, keep a per-shell applied revision, and expose explicit sync through the same apply/acknowledge path. `[VERIFIED: fresh Spike 001 verifier]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Functions.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html]` |
| SYNC-02 | Writes are atomic and revision-aware; unrelated concurrent edits compose, same-identity races are visible/deterministic, and the foreground remains fail-open. | Serialize compare/apply/persist under the private store lock, compare keys changed since the shell's acknowledged revision, retain bounded history, and never acknowledge before validated application plus a fresh snapshot. `[VERIFIED: fresh Spike 001 verifier]` |
</phase_requirements>

## Project Constraints (from AGENTS.md)

- Phase 7 planning must use the project-local `spike-findings-zsh-pro` skill and its live-shell-state synchronization blueprint. `[VERIFIED: AGENTS.md]`

## Scope Constraints

- The validated spike decisions are constraints: ingest is bootstrap-only; there is one shared worktree; deltas represent resulting state rather than command intent; there is no staging; dirty checkout is blocked; auto-apply is default-on and configurable; `PWD`, jobs, command buffers, history, and other process-local state are excluded. `[VERIFIED: .codex/skills/spike-findings-zsh-pro/references/live-shell-state-synchronization.md]`
- The milestone adds no dependency and retains `git`/`zsh` subprocesses; generated mutation syntax such as `unalias`, `unset -f`, and `setopt` remains in `core/shell/zsh/emit.go`. `[VERIFIED: .planning/ROADMAP.md]`
- Merge, rebase, remotes, trust of externally authored profiles, and multiple worktrees are out of Phase 7 scope. `[VERIFIED: .planning/REQUIREMENTS.md]`

## Summary

The validated spike resolves the frontier questions: NUL-framed semantic snapshots, per-identity deltas, a global lock, atomic persistence, bounded revision history, deterministic same-key conflict handling, `precmd` publication, `zle-line-finish` convergence, and fail-open behavior work in two independent shells. A fresh run passed all eight verifier groups. Its 98 prompt-path samples measured p50 1.502 ms, p95 8.510 ms, and max 12.746 ms; the 50-cycle benchmark took 0.498 seconds, or about 9.97 ms/cycle. These are prototype observations, not a production SLA. `[VERIFIED: fresh Spike 001 verifier]`

Production integration is not a transplant of the spike. The existing store is a bare Git repository whose current branch is still per-terminal, `Store.Create` forks `main`, the CLI has no shared-worktree verbs, and `model.Profile` is a source-ordered ingest IR rather than an editable final-state map. The loader also retains a reverse function built when the profile was applied. If live synchronization changes state without rebuilding that reverse ownership record, a later checkout or deactivate can leave residue. `[VERIFIED: codebase]`

Implement a first-class, versioned shared worktree document and service between the CLI/runtime and the existing Git store. The document should retain the ingested profile as its committed source model, carry a category-aware final-state overlay plus tombstones, and record shared branch/base OID/revision. All mutation flows—publish, commit, checkout, reset, and conflict resolution—must go through that service. The shell emitter must produce both forward patches and a replacement reverse-ownership function; the loader must acknowledge a revision only after applying and re-snapshotting it. `[VERIFIED: codebase + fresh Spike 001 verifier]`

**Primary recommendation:** Build the durable worktree/identity/revision model first, integrate Git-shaped commands second, and add hooks only after the existing reverse-ownership lifecycle has an end-to-end `live edit → sibling sync → checkout/deactivate → zero residue` test. `[VERIFIED: codebase + fresh Spike 001 verifier]`

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Capture supported live state | Shell / sourced loader | CLI runtime helper | zsh exposes aliases, functions, parameters, options, PATH, and FPATH; the helper validates framing and limits. `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html]` `[VERIFIED: codebase]` |
| Identity admission and semantic diff | Worktree domain service | Model | Policy must be shell-independent and operate on typed identities/deltas rather than command strings. `[VERIFIED: fresh Spike 001 verifier]` |
| Revision arbitration and conflict handling | Worktree domain service | Durable storage | All processes need one compare/apply/persist critical section and the same deterministic overlap rule. `[VERIFIED: fresh Spike 001 verifier]` |
| Safe-boundary synchronization | Shell / sourced loader | Worktree runtime helper | `precmd` runs before prompts and `zle-line-finish` when ZLE finishes reading a line; hook helpers compose without replacing unrelated hooks. `[CITED: https://zsh.sourceforge.io/Doc/Release/Functions.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]` |
| Generated apply/reverse code | zsh emitter | Activate/model plan | The repository invariant already makes `core/shell/zsh/emit.go` the only zsh-code generation seam. `[VERIFIED: codebase + .planning/ROADMAP.md]` |
| Branch/ref history | Git store | Worktree service | Existing `commit-tree` and compare-and-swap `update-ref` plumbing provides the durable history boundary. `[VERIFIED: codebase]` `[CITED: https://git-scm.com/docs/git-commit-tree.html]` `[CITED: https://git-scm.com/docs/git-update-ref.html]` |
| Materialized state and per-shell acknowledgements | Private local storage | Worktree service | Shared branch/base/revision and shell-applied revisions are coordination state, not Git commits on every prompt. `[VERIFIED: fresh Spike 001 verifier]` |
| Secret values | OS keychain / ignored vault | Store secret preparation | Existing commits persist references and keep detected literal values outside Git; live capture must preserve that boundary. `[VERIFIED: codebase]` |

## Standard Stack

### Core

| Library / Tool | Version | Purpose | Why Standard |
|----------------|---------|---------|--------------|
| Go standard library | 1.25.0 | Typed model/service, JSON, deadlines, descriptor-bound filesystem operations, subprocess orchestration | Already used throughout production; no new dependency is permitted. `[VERIFIED: go.mod + environment probe]` |
| Git CLI plumbing | 2.43.0 installed | Commit/tree/ref creation and branch history | Existing hardened `gitRunner` already allowlists argv, clears inherited `GIT_*`, uses `commit-tree`, and publishes refs with CAS. `[VERIFIED: codebase + environment probe]` |
| zsh | 5.9 installed | Semantic state capture, hook execution, and emitted apply/reverse code | The product runs in an already-open zsh and already uses `zsh/parameter`, `zsh -n`, and a sourced loader. `[VERIFIED: codebase + environment probe]` |
| `mvdan.cc/sh/v3` | 3.13.1 | Existing parse/render support | It is the module's sole non-standard Go dependency; this phase should not add another. `[VERIFIED: go.mod]` |

### Supporting

| Facility | Version | Purpose | When to Use |
|----------|---------|---------|-------------|
| `os.CreateTemp`, `File.Sync`, same-directory rename, directory sync | Go 1.25.0 | Crash-hardened replacement sequence for bounded JSON state | Use inside a locked private root for shared state, shell acknowledgements, config, and bounded events. `[CITED: https://pkg.go.dev/os]` `[VERIFIED: fresh Spike 001 verifier]` |
| `git update-ref` transactions/CAS | Git 2.43.0 | Ref publication without lost updates | Use for commit and branch creation; require the expected old OID. `[CITED: https://git-scm.com/docs/git-update-ref.html]` |
| `add-zsh-hook` / `add-zle-hook-widget` | zsh 5.9 | Cooperative hook installation | Use to add namespaced handlers without overwriting user/plugin handlers. `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Existing Git CLI driver | A Go Git library | Rejected: violates the locked no-new-dependency decision and duplicates hardened subprocess policy. `[VERIFIED: .planning/ROADMAP.md + codebase]` |
| Semantic snapshots | Parse the last command text | Rejected: aliases/functions/sourced scripts and indirect mutations make intent unequal to resulting state. `[VERIFIED: fresh Spike 001 verifier]` |
| Materialized overlay document | Rewrite `model.Profile.Entries` after every command | Rejected: the IR is source-ordered and can contain repeated assignments, so removing the last entry can reveal an earlier one rather than represent an unset. `[VERIFIED: codebase]` |
| Bounded revision events | Persist only the latest whole snapshot | Rejected: stale shells could silently overwrite unrelated newer identities. `[VERIFIED: fresh Spike 001 verifier]` |

**Installation:** No installation step. Use the existing module and system `git`/`zsh`. `[VERIFIED: codebase + environment probe]`

## Package Legitimacy Audit

Not applicable: Phase 7 installs no external packages, so the package-legitimacy gate is not triggered. `[VERIFIED: .planning/ROADMAP.md]`

## Architecture Patterns

### System Architecture Diagram

```text
zsh command completes
        |
        v
precmd capture (NUL-framed semantic snapshot, bounded)
        |
        v
runtime helper --> decode + admission/filter --> per-identity delta
        |                                         |
        |                           shared lock + revision arbitration
        |                                         |
        |                        +----------------+----------------+
        |                        |                                 |
        |                 no overlap                         same-key overlap
        |                        |                                 |
        |                  compose/persist                 visible conflict,
        |                        |                         first publisher wins
        |                        v                                 |
        |           worktree document + events + revision <-------+
        |                        |
        +------------------------+------------------------------+
                                                                 |
user starts next command --> zle-line-finish pull-only ----------+
                                  |
                         auto-apply enabled?
                           /             \
                         no               yes
                         |                 |
                 report shell behind   emitter creates validated
                                       forward + reverse-ownership patch
                                                |
                                       loader evals, re-snapshots,
                                       then acknowledges revision

explicit CLI workflow:
status/diff --------> worktree service
commit -------------> cleanly encode effective document -> commit-tree -> update-ref CAS
branch/create ------> current committed base OID -> new ref CAS
checkout/reset -----> dirty guard/policy -> materialize target -> revision event
```

The runtime helper should not invoke Git on each hook; hooks operate on bounded worktree state, while explicit commit/branch/checkout operations cross into the Git store. `[VERIFIED: fresh Spike 001 verifier + codebase]`

### Recommended Project Structure

```text
core/
├── model/
│   └── worktree.go          # typed identities, overlay/tombstones, deltas, revisions, conflicts
├── worktree/
│   ├── service.go           # publish/pull/ack/status/diff/commit/checkout/reset orchestration
│   ├── registry.go          # admitted identities and volatile/secret exclusions
│   ├── diff.go              # per-identity semantic comparison and categorized rendering model
│   ├── state.go             # versioned durable DTO, migrations, compaction
│   └── atomic_unix.go       # private-root locking and atomic persistence
├── store/
│   ├── store.go             # create-from-current, commit/reset/read projections
│   ├── git.go               # only required new allowlisted plumbing argv
│   └── dto.go               # backward-compatible profile/worktree encoding
├── cli/
│   ├── cli.go               # command dispatch
│   ├── store.go             # widened narrow interfaces
│   ├── worktree.go          # user-facing status/diff/commit/branch/checkout/reset/sync/config
│   └── runtime.go           # descriptor-bound publish/pull/ack helper subcommands
└── shell/
    ├── provider.go          # shell-neutral snapshot/patch contracts
    └── zsh/
        ├── hook.go          # namespaced precmd/ZLE hooks, fail-open shell lifecycle
        └── emit.go          # sole forward/reverse shell-code generator
```

These are recommended ownership boundaries, not a requirement to use every filename literally. Keep the composition root in `core/cmd/zsh-pro/main.go`. `[VERIFIED: codebase]`

### Existing Production Seams and Exact Symbols

| File / Symbol | Current responsibility | Phase 7 extension |
|---------------|------------------------|-------------------|
| `core/cli/cli.go`: `CLI`, `New`, `Run`, `runStatus` | Dispatches public commands; status currently reports only the current profile. `[VERIFIED: codebase]` | Add Git-like command parsing/output and delegate all worktree policy to a narrow service interface. |
| `core/cli/store.go`: `Store` | Exposes only `Branches`, `Current`, `Checkout`, and `Read` to the CLI. `[VERIFIED: codebase]` | Replace/widen the interface with focused read/workflow capabilities; avoid making the CLI depend on concrete `store.Store`. |
| `core/cli/ingest.go`: `runIngestCommand`, `runIngestWithSeams` | Coordinates the existing multi-step bootstrap transaction. `[VERIFIED: codebase]` | Add materialized-worktree publication/recovery as an explicit participant after the committed baseline is known. |
| `core/cli/runtime.go`: `runRuntime`, `runRuntimeCapture`, `runRuntimeValidate` | Allows descriptor-bound bounded capture and validation for loader-only commands. `[VERIFIED: codebase]` | Extend the private allowlist with bounded publish/pull/ack operations; do not expose arbitrary file or command execution. |
| `core/cli/emitter.go`: `NewRuntimeEmitterWithRuntimeStore` | Builds descriptor-bound runtime emitters and resolves secrets. `[VERIFIED: codebase]` | Reuse its private-pipe pattern for exact worktree patch bytes and typed-nil/error hardening. |
| `core/store/store.go`: `Store.Current`, `Checkout`, `Create`, `Commit`, `Read`, `CommitIngest` | Owns bare-repository branch/profile operations; `Current` is environment-carried and `Create` forks `main`. `[VERIFIED: codebase]` | Add shared-current/base operations, create from an expected current OID, effective-document commit, and materialization reads while preserving ingest recovery behavior. |
| `core/store/git.go`: `gitRunner`, `updateRefSession` | Enforces argv allowlists, deadlines, clean `GIT_*` environment, and ref transactions. `[VERIFIED: codebase]` | Add only the minimal plumbing argv required for ref-from-current/reset reads; keep all expected-old-OID checks here. |
| `core/model/profile.go`: `Profile` and `core/model/identityset.go`: `IdentitySet` | Represent source-ordered ingest entries and introspected identity summaries. `[VERIFIED: codebase]` | Add separate live identity/value/overlay contracts; do not overload `IdentitySet`, which lacks the full values/attributes needed by synchronization. |
| `core/activate/builder.go`: `Build` and `core/activate/diff.go`: `Diff` | Build manifests and full deactivate/activate plans. `[VERIFIED: codebase]` | Add/project an effective worktree manifest and define identity-patch ownership semantics without bypassing manifest drift guards. |
| `core/shell/provider.go`: `Emitter`, `RuntimeEmitter`, `Provider` | Defines shell-neutral parse/introspect/regenerate/emit seams. `[VERIFIED: codebase]` | Add the smallest shell-neutral snapshot and live-patch contracts; keep worktree policy out of the concrete provider. |
| `core/shell/zsh/introspect.go`: `Provider.Introspect`, `parseIntrospect` | Captures/parses semantic identities; the working tree has hardened NUL-delimited framing. `[VERIFIED: codebase + git diff]` | Reuse/extend that framing for bounded live values and attributes rather than adding a second incompatible snapshot parser. |
| `core/shell/zsh/emit.go`: `Provider.Emit`, `EmitRuntime` | Sole generator of apply/deactivate zsh with retained reverse semantics. `[VERIFIED: codebase]` | Generate live forward mutations and the replacement reverse-ownership payload here. |
| `core/shell/zsh/hook.go`: `Provider.HookScript`, `_zp_switch`, `ZP_ACTIVE_REVERSE_FN` lifecycle | Applies/reverses profiles and owns fail-open loader markers. `[VERIFIED: codebase]` | Add idempotent publish/pull hooks and integrate acknowledgement with the existing recovery/reverse slots. |
| `core/cmd/zsh-pro/main.go` | Constructs concrete provider/store/emitter/CLI dependencies. `[VERIFIED: codebase]` | Construct the worktree service and descriptor-bound runtime factory here; no package-global singleton. |

### Pattern 1: Versioned Baseline Plus Effective-State Overlay

**What:** Store the committed source-ordered `model.Profile` and a versioned overlay keyed by typed identity. Overlay values represent final supported state; tombstones represent removal. Store `branch`, `base_oid`, `revision`, bounded revision events, and conflicts beside that document. `[VERIFIED: codebase + fresh Spike 001 verifier]`

**Why:** A source IR and a live semantic state map answer different questions. The overlay prevents repeated source assignments from reappearing when a live value is removed and gives `status`/`diff` a deterministic category/identity ordering. `[VERIFIED: codebase]`

**Commit rule:** Under the worktree lock, project the effective document to the store DTO/generated profile, create a commit with `base_oid` as expected parent, publish the current branch with ref CAS, then advance the worktree `base_oid` and clear only the committed overlay. A ref mismatch is a visible conflict, never an overwrite. `[VERIFIED: codebase]` `[CITED: https://git-scm.com/docs/git-update-ref.html]` `[CITED: https://git-scm.com/docs/git-commit-tree.html]`

### Pattern 2: Admission Registry, Not Ambient-Shell Import

**What:** Seed the registry from identities the materialized profile owns, plus PATH/FPATH and the supported option set. Automatically admit a new env/alias/function identity only when it was absent at shell attachment and passes bookkeeping, volatile, unsafe-name, and secret filters. Never import arbitrary inherited values. `[VERIFIED: .codex/skills/spike-findings-zsh-pro/references/live-shell-state-synchronization.md]`

**Why:** Whole-shell snapshots contain session/framework noise and unmanaged parent state. Persisting all of it would violate ownership, portability, and secret boundaries. `[VERIFIED: fresh Spike 001 verifier + codebase]`

### Pattern 3: Per-Identity Optimistic Concurrency

**What:** Each shell sends its last acknowledged revision and delta. Under the global lock, inspect keys changed after that revision. Disjoint keys compose. Overlap uses deterministic first-publisher-wins, records a visible conflict for the loser, and leaves the shared value unchanged. `[VERIFIED: fresh Spike 001 verifier]`

**History gap:** If the shell's base precedes retained history, perform a three-way per-identity comparison against a retained/compacted baseline. If no safe baseline exists, refuse publication and require explicit refresh/resolution. Never apply a stale whole snapshot. `[VERIFIED: .codex/skills/spike-findings-zsh-pro/references/live-shell-state-synchronization.md]`

### Pattern 4: Prepare, Apply, Verify, Acknowledge

**What:** Pull returns a bounded typed change plan. The emitter renders it and a replacement reverse-ownership function. Validate generated zsh, apply it in the current shell, capture a fresh snapshot, compare the managed target, then persist that shell's acknowledgement. `[VERIFIED: codebase + fresh Spike 001 verifier]`

**Why:** Acknowledging before shell application makes a failed shell appear converged. Rebuilding reverse ownership is necessary because the current loader's retained reverse function otherwise describes the pre-sync profile. `[VERIFIED: codebase]`

### Pattern 5: Split Publish From Pull

**What:** `precmd` may capture and publish settled local changes. `zle-line-finish` must be pull-only, so it cannot publish while a user command buffer is transitioning to execution. Explicit `sync` should use the same prepare/apply/verify/ack path and should publish first only when invoked at a prompt boundary. `[VERIFIED: fresh Spike 001 verifier]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Functions.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html]`

**Hook composition:** Install namespaced handlers with zsh's hook helpers and make installation/removal idempotent across re-sourcing. `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]`

### Component Responsibilities

| Component | Extend | Must Not Own |
|-----------|--------|--------------|
| `core/model` | Typed identity, live value, delta, overlay, tombstone, revision/conflict contracts | Locks, filesystem paths, zsh strings. `[VERIFIED: codebase]` |
| `core/worktree` | Policy, revision arbitration, category diff, status, state migration/compaction | Shell syntax or raw Git subprocesses. `[VERIFIED: codebase + spike]` |
| `core/store` | Git commit/tree/ref operations, current-branch ref creation, DTO compatibility, secret-reference persistence | Prompt-cycle snapshots or per-shell hook state. `[VERIFIED: codebase]` |
| `core/shell/zsh` | Semantic capture framing, hook glue, forward/reverse generation, validation | Shared conflict policy or Git workflow. `[VERIFIED: codebase + spike]` |
| `core/cli` | Command parsing, output, bounded runtime helper, dependency composition | Duplicate diff/concurrency/secret logic. `[VERIFIED: codebase]` |

### Anti-Patterns to Avoid

- **Transplanting the spike's patch renderer:** It directly emits shell mutations outside the production emitter and does not maintain Phase 5 reverse ownership. Port the behavior through `core/shell/zsh/emit.go`. `[VERIFIED: codebase + spike source]`
- **Mutating `Profile.Entries` as a final-state dictionary:** Source ordering and repeated assignments make delete/update semantics incorrect. Use an explicit overlay/tombstone model. `[VERIFIED: codebase]`
- **Using `ZSHPRO_PROFILE` as the shared current branch:** That variable is process-local. Shared branch/base/revision must be durable worktree state; keep the variable only as the shell's applied marker. `[VERIFIED: codebase + .planning/REQUIREMENTS.md]`
- **Git on every prompt:** It creates an unbounded external-process hot path. Prompt hooks should touch bounded local state only. `[VERIFIED: fresh Spike 001 verifier]`
- **Whole-snapshot last-writer-wins:** It silently destroys unrelated changes from stale shells. `[VERIFIED: fresh Spike 001 verifier]`
- **Persistent patch files sourced by pathname:** A replaceable path increases tampering/staleness risk. Prefer the existing descriptor/pipe runtime pattern and validate the exact bytes applied. `[VERIFIED: codebase]`
- **A second secret classifier in `worktree`:** Centralize or expose admission/exclusion policy from the existing classification seam; do not let regexes drift. `[VERIFIED: codebase]`

## Recommended Plan Boundaries

1. **Domain and ownership contract:** Add typed identity/overlay/revision models, admission rules, semantic diffing, and the forward-plus-reverse patch contract. Prove repeated-source-entry removal and `live edit → sync → deactivate` before persistence. `[VERIFIED: codebase]`
2. **Durable worktree engine:** Add private-root state, lock/CAS critical section, atomic replace, bounded revision/event history, per-shell acknowledgements, history-gap handling, conflict records, and migrations. `[VERIFIED: fresh Spike 001 verifier]`
3. **Bootstrap and Git-shaped workflow:** Materialize successful ingest; add shared current branch, status/diff/commit/branch/create/checkout/reset; change branch creation from hard-coded `main` to current committed base; enforce dirty checkout and commit CAS. `[VERIFIED: codebase + .planning/REQUIREMENTS.md]`
4. **Runtime and loader integration:** Add bounded publish/pull/ack runtime commands, generated apply/reverse code, `precmd` and pull-only ZLE hooks, explicit sync, and default-on persistent configuration with a per-shell override. `[VERIFIED: codebase + fresh Spike 001 verifier]`
5. **Cross-process proof and hardening:** Run real independent-zsh races, secret/noise exclusions, injected write/apply failures, stale-history reconciliation, branch/reset flows, zero-residue lifecycle checks, and measured hot-path tests. `[VERIFIED: fresh Spike 001 verifier]`

Do not combine boundaries 1–2 with hook installation: the hooks would expose an unproven ownership/persistence format to live shells and make later correction migration-heavy. `[VERIFIED: codebase + spike blueprint]`

## Workflow Semantics the Plan Should Lock

| Command / Event | Prescribed behavior |
|-----------------|---------------------|
| `ingest` | On successful baseline commit, atomically materialize a versioned clean worktree at that exact commit. A post-commit materialization failure is recovery-required and idempotently repairable; do not pretend the published Git commit rolled back. `[VERIFIED: codebase]` |
| `status` | Show shared branch/base/revision, dirty counts by category, conflict count, and current shell applied/behind/auto-apply state. `[VERIFIED: .planning/REQUIREMENTS.md + spike]` |
| `diff` | Deterministically show add/change/remove grouped by env, alias, function, PATH, FPATH, and option. User-requested non-secret values may be shown; secret literals must never be present in the worktree. `[VERIFIED: .planning/REQUIREMENTS.md + codebase]` |
| `commit -m` | Commit all shared supported changes directly—no index/staging UX—and become clean only after Git ref CAS and worktree-base update both succeed. `[VERIFIED: .planning/REQUIREMENTS.md + codebase]` |
| `branch <name>` | Point the new branch at the current committed base, not `main`; leave the shared worktree/branch unchanged. `[VERIFIED: .planning/REQUIREMENTS.md + codebase]` |
| `checkout <name>` / `checkout -b <name>` | Under the worktree lock, refuse dirty state, validate target, materialize it, update shared branch/base, and publish a revision describing the identity changes. Running shells converge at safe boundaries. `[VERIFIED: .planning/REQUIREMENTS.md + spike]` |
| `reset --hard` | The sole ordinary destructive discard path: restore current branch's base commit, clear its dirty overlay through a revisioned event, and report what was discarded. `[VERIFIED: .planning/REQUIREMENTS.md]` |
| `sync` | Use the same bounded pull/apply/verify/ack pipeline as auto-apply; it must work when auto-apply is disabled. `[VERIFIED: .planning/REQUIREMENTS.md + spike]` |
| `config set auto-apply true|false` | Persist a user-local default (initially true); allow a shell-local override without changing other shells. `[VERIFIED: accepted Phase 7 note + spike blueprint]` |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Git object/ref consistency | Custom repository or branch database | Existing `gitRunner`, `commit-tree`, and `update-ref` CAS/transactions | Git already defines object/ref integrity and the repository has a hardened driver. `[VERIFIED: codebase]` |
| Zsh parsing/escaping | String concatenation in CLI/worktree code | Existing typed plan plus `core/shell/zsh/emit.go`, followed by `zsh -n` | Shell quoting errors are execution vulnerabilities and violate the single-emitter invariant. `[VERIFIED: codebase + .planning/ROADMAP.md]` |
| Secret encryption | New encryption/file format | Existing OS keychain / ignored vault with `SecretRef` | Phase 3 already owns resolution and persistence; cryptography must not be improvised. `[VERIFIED: codebase]` |
| Command timeout wrapper | GNU `timeout` dependency | Existing Go `context` deadlines/runtime helper | The installed/runtime platform need not have the external utility; production already has bounded subprocess infrastructure. `[VERIFIED: codebase]` |
| State conflict resolution | Whole-profile overwrite or timestamps | Revisioned per-identity overlap detection | Disjoint changes compose and same-key races remain deterministic/visible. `[VERIFIED: fresh Spike 001 verifier]` |
| Hook multiplexing | Assign directly to `precmd_functions`/ZLE widgets | `add-zsh-hook` and `add-zle-hook-widget` | These helpers compose and clean up named handlers. `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]` |

**Key insight:** The difficult part is not detecting that bytes changed; it is maintaining identity ownership, causal revision state, secret boundaries, and reversible live-shell behavior across processes. Those are one integrated transaction protocol. `[VERIFIED: codebase + fresh Spike 001 verifier]`

## Common Pitfalls

### Pitfall 1: Reverse Ownership Becomes Stale

**What goes wrong:** A synced alias/function/scalar is applied, but the loader's retained reverse function still describes the originally activated profile; checkout or deactivate then skips or incompletely removes it. `[VERIFIED: codebase]`

**How to avoid:** Make every apply plan replace the retained reverse record using the shell's prior acknowledged state and ownership slots. Test new identity, changed identity, removed identity, PATH/FPATH, and option lifecycle through switch-back/deactivate. `[VERIFIED: codebase]`

### Pitfall 2: Source IR Is Mistaken for a Mutable State Map

**What goes wrong:** Updating/removing an entry exposes earlier repeated assignments or cannot express `unset`/`unalias`/`unfunction` correctly. `[VERIFIED: codebase]`

**How to avoid:** Keep the source profile and store a typed final-state overlay with tombstones; project an effective activation/commit view explicitly. `[VERIFIED: codebase]`

### Pitfall 3: Secret Literals Cross the Worktree Boundary

**What goes wrong:** A newly assigned token-like variable enters snapshot JSON, conflict logs, patch files, or committed blobs before store-side secret preparation runs. `[VERIFIED: codebase]`

**How to avoid:** Filter before persistence; preserve known `SecretRef`s, exclude new secret-like literals with value-free name/reason reporting, and never include raw values in events/errors. `[VERIFIED: codebase + spike blueprint]`

### Pitfall 4: Acknowledging Before Verified Application

**What goes wrong:** A render, validation, or eval failure leaves the shell behind while its applied revision says current. `[VERIFIED: fresh Spike 001 verifier]`

**How to avoid:** Persist acknowledgement only after validated eval and a fresh managed-state snapshot matches the target. Fail open with visible diagnostics and retry later. `[VERIFIED: spike blueprint]`

### Pitfall 5: Incomplete Atomicity

**What goes wrong:** Locking only writes or renaming without syncing permits races, partial coupled-state updates, or crash ambiguity. `[VERIFIED: fresh Spike 001 verifier]`

**How to avoid:** Hold one private-root lock across read/compare/apply/persist, write bounded same-directory temporary files at mode 0600, sync file, rename, sync directory, and test fault points. Go's `Rename` durability semantics vary by platform, so keep platform-specific tests rather than asserting universal crash guarantees. `[CITED: https://pkg.go.dev/os]`

### Pitfall 6: History Compaction Makes Old Shells Unsafe

**What goes wrong:** A long-idle shell's acknowledged revision predates retained events, so overlap detection cannot establish causality. `[VERIFIED: spike blueprint]`

**How to avoid:** Retain a compacted baseline sufficient for three-way comparison; otherwise refuse publication and require explicit refresh. `[VERIFIED: spike blueprint]`

### Pitfall 7: Hook Reentrancy and Foreground Interference

**What goes wrong:** Sync hooks invoke themselves, publish during line acceptance, overwrite plugin hooks, or surface noisy stderr in every prompt. `[VERIFIED: spike blueprint + zsh official docs]`

**How to avoid:** Use a process-local recursion guard, namespaced cooperative hook registration, publish only in `precmd`, pull-only in `zle-line-finish`, bounded deadlines, and fail-open diagnostics stored in namespaced globals. `[VERIFIED: spike blueprint]` `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]`

### Pitfall 8: Unbounded Forensic or Prompt State

**What goes wrong:** Revision/event logs grow forever, snapshots accept arbitrary records, or a helper waits without a deadline. `[VERIFIED: spike blueprint]`

**How to avoid:** Start with the proven 2 MiB/10,000-record snapshot caps, 128 revision events, and 250 ms helper deadline; change them only after production measurement. Cap/rotate value-free forensic events as well. `[VERIFIED: fresh Spike 001 verifier]`

## Testing Strategy

Nyquist validation is explicitly disabled in `.planning/config.json`, so no planner-generated Validation Architecture/Wave 0 contract is required. Phase 7 still needs the following implementation tests because the requirements explicitly demand real two-shell and failure behavior. `[VERIFIED: .planning/config.json + .planning/REQUIREMENTS.md]`

| Layer | Required proof | Suggested command / harness |
|-------|----------------|-----------------------------|
| Model/worktree unit | Identity ordering, add/change/remove, repeated-source overlay, tombstones, volatile/secret exclusion, disjoint/overlap/history-gap arbitration | `go test -count=1 ./core/model ./core/worktree` `[VERIFIED: codebase conventions]` |
| Store integration | Create-from-current, commit CAS, dirty guard, reset, DTO backward compatibility, atomic fault injection, permissions/symlink defenses | `go test -count=1 ./core/store` `[VERIFIED: codebase conventions]` |
| Emitter/loader | Forward plus reverse update, quoting, `zsh -n`, idempotent hooks, fail-open apply, no lifecycle residue | `go test -count=1 ./core/shell/zsh ./core/activate ./core/cli` `[VERIFIED: codebase conventions]` |
| Real two-shell E2E | Auto on/off, explicit sync, unrelated race, same-key conflict/resolution, stale shell, checkout/reset, `PWD`/history/buffer/jobs untouched | Extend the retained Spike 001 two-zsh harness to run the production binary and real loader. `[VERIFIED: fresh Spike 001 verifier]` |
| Performance | Prompt-cycle p50/p95/max, helper count, timeout behavior, bounded state sizes | Keep the spike sampler in CI-smoke or a dedicated repeatable benchmark; use `hyperfine` only when available. `[VERIFIED: fresh Spike 001 verifier + environment probe]` |

The current uncommitted production tree passes `go test -count=1 ./core/store ./core/cli ./core/shell/zsh ./core/activate ./core/model`. `[VERIFIED: fresh test run]`

## Code Examples

### Typed Publication Contract

```go
// Recommended contract derived from the production model/store seams and Spike 001.
type PublishRequest struct {
	ShellID            string
	AcknowledgedRevision uint64
	Delta              []model.LiveChange
}

type PublishResult struct {
	SharedRevision uint64
	Accepted       []model.Identity
	Conflicts      []model.Conflict
}
```

The service should validate identity/value limits before acquiring the lock, then re-read authoritative state and decide acceptance under the lock. `[VERIFIED: fresh Spike 001 verifier]`

### Git Ref Compare-and-Swap

```text
git commit-tree <tree> -p <expected-base-oid>
git update-ref refs/heads/<current> <new-commit-oid> <expected-base-oid>
```

`commit-tree` creates a commit from a tree and parent; the old OID argument to `update-ref` verifies the ref before changing it. Use the existing argv-safe driver rather than a shell command string. `[CITED: https://git-scm.com/docs/git-commit-tree.html]` `[CITED: https://git-scm.com/docs/git-update-ref.html]` `[VERIFIED: codebase]`

### Safe Hook Registration Shape

```zsh
autoload -Uz add-zsh-hook add-zle-hook-widget
add-zsh-hook precmd _zp_worktree_publish
add-zle-hook-widget line-finish _zp_worktree_pull
```

Use namespaced recursion guards and idempotent removal/re-addition around this shape. `precmd` runs before each prompt and `line-finish` is called when ZLE finishes reading a line. `[CITED: https://zsh.sourceforge.io/Doc/Release/Functions.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]`

## State of the Art

| Old / Current Product Approach | Phase 7 Approach | When Changed | Impact |
|--------------------------------|------------------|--------------|--------|
| Per-terminal `ZSHPRO_PROFILE` selects a Git branch; loader functions perform switch locally. `[VERIFIED: codebase]` | Durable shared branch/base/revision; per-shell variable is only applied-state metadata. `[VERIFIED: .planning/REQUIREMENTS.md]` | Planned Phase 7 | Two terminals see one Git-like worktree while remaining independent processes. |
| Ingest commits the whole parsed profile. `[VERIFIED: codebase]` | Ingest bootstraps once; live per-identity overlay becomes the dirty worktree. `[VERIFIED: .planning/REQUIREMENTS.md]` | Planned Phase 7 | Normal edits no longer require re-ingest. |
| `Store.Create` forks `main`. `[VERIFIED: codebase]` | Branch creation forks the current committed base. `[VERIFIED: .planning/REQUIREMENTS.md]` | Planned Phase 7 | Branch semantics match the accepted Git mental model. |
| Activate diff is full deactivate-then-activate. `[VERIFIED: codebase]` | Sync applies an identity patch while regenerating reverse ownership. `[VERIFIED: codebase + spike blueprint]` | Planned Phase 7 | Safe convergence without clobbering unrelated/process-local state. |

**Deprecated for the Phase 7 user workflow:** routine re-ingest, per-terminal user-facing current branch, and `Store.Create` hard-coded to `main`. Preserve backward compatibility only where migration/recovery needs it. `[VERIFIED: .planning/REQUIREMENTS.md + codebase]`

## Current Dirty-Tree Compatibility

Research and tests used the working tree, not HEAD-only code. Relevant uncommitted changes harden runtime descriptor handling, typed-nil emitter behavior, NUL-framed zsh introspection, token-free activation tests, model representability, and keychain transport errors. Phase 7 plans must build on these versions and must not revert or duplicate them. `[VERIFIED: git diff + fresh test run]`

Files currently modified in the affected seams include `core/cli/emitter.go`, `core/cli/runtime_root_unix.go`, `core/cli/runtime_test.go`, `core/shell/zsh/introspect.go`, `core/shell/zsh/introspect_test.go`, `core/shell/zsh/emit_test.go`, `core/activate/tokenfree_test.go`, `core/model/profile_test.go`, `core/store/errors.go`, `core/store/keychain.go`, and `core/store/keychain_os_test.go`. `[VERIFIED: git status]`

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| — | None. Prescriptive choices are derived from locked roadmap/requirements, current code, the validated spike, or official documentation. | — | — |

## Open Questions (RESOLVED)

1. **How should a pre-existing unmanaged identity become managed?**
   - What we know: Automatically importing all inherited shell state is forbidden; new absent-at-attach identities can be admitted safely. `[VERIFIED: spike blueprint]`
   - What's unclear: The accepted command surface has no explicit `track` verb for an inherited alias/function/variable that the user wants to start managing. `[VERIFIED: accepted Phase 7 note]`
   - **RESOLVED:** Phase 7 auto-admits only safe identities that were absent at shell attachment. Identities already present and unmanaged at attachment remain excluded; no pre-existing admission command is added. `[VERIFIED: spike blueprint + selected Phase 7 plan decision]`

2. **How should ingest publication and first materialization report a split failure?**
   - What we know: The current ingest publishes a real Git commit through a hardened transaction; a published commit cannot honestly be treated as absent. `[VERIFIED: codebase]`
   - What's unclear: There is no existing recovery UX for “commit published, materialized worktree write failed.” `[VERIFIED: codebase]`
   - **RESOLVED:** Materialization is idempotently reconstructed from the exact committed worktree DTO and OID. A published-commit/materialization split reports committed plus recovery-required evidence, and the next status/sync repairs from that exact OID before hooks operate. `[VERIFIED: codebase + selected Phase 7 plan decision]`

3. **Should `config set auto-apply` be global-only or support a documented shell override?**
   - What we know: Auto-apply defaults on, is configurable, and a disabled shell must remain visibly behind until explicit sync. `[VERIFIED: .planning/REQUIREMENTS.md]`
   - What's unclear: The requirements do not prescribe configuration scope. `[VERIFIED: .planning/REQUIREMENTS.md]`
   - **RESOLVED:** Persist a user-local default and permit the exact shell-local `ZSHPRO_AUTO_APPLY=true|false` override. `status` displays the effective value and whether it came from the persisted default or the current shell override. `[VERIFIED: spike blueprint + selected Phase 7 plan decision]`

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go | Build/tests/runtime helper | ✓ | 1.25.0 | — `[VERIFIED: environment probe]` |
| Git | Store/branch/commit | ✓ | 2.43.0 | None; required product dependency. `[VERIFIED: environment probe + .planning/REQUIREMENTS.md]` |
| zsh | Capture/apply/E2E | ✓ | 5.9 | None; required product shell. `[VERIFIED: environment probe]` |
| GNU Make | Existing task entrypoints | ✓ | 4.3 | Direct `go test` commands. `[VERIFIED: environment probe]` |
| Docker | Optional isolated multi-shell UAT | ✓ | 29.1.5 | Local independent zsh processes. `[VERIFIED: environment probe]` |
| tmux | Optional interactive two-shell UAT | ✓ | 3.7b | Local process harness/Docker. `[VERIFIED: environment probe]` |
| jq | Spike/harness inspection | ✓ | 1.7 | Go JSON helpers. `[VERIFIED: environment probe]` |
| golangci-lint | Repository lint | ✓ | 2.12.2 | `go test`/`go vet` while unavailable. `[VERIFIED: environment probe]` |
| hyperfine | Optional benchmark presentation | ✗ | — | Existing spike sampler and `/usr/bin/time`; do not block implementation. `[VERIFIED: environment probe]` |

**Missing dependencies with no fallback:** None. `[VERIFIED: environment probe]`

**Missing dependencies with fallback:** `hyperfine`; use the retained Go/shell timing harness unless the tool is installed for final benchmark reporting. `[VERIFIED: environment probe]`

## Security Domain

Security enforcement is enabled because `.planning/config.json` does not set it to `false`. `[VERIFIED: .planning/config.json]`

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | no | Local single-user process; no authentication boundary is introduced. `[VERIFIED: project scope]` |
| V3 Session Management | no | Per-shell revision state is synchronization metadata, not an authenticated web session. `[VERIFIED: project scope]` |
| V4 Access Control | yes | Private store-root ownership/mode checks, descriptor-bound operations, and same-user file permissions. `[VERIFIED: codebase]` |
| V5 Input Validation | yes | Typed framed records, name/value/count/size validation, strict branch names, DTO versions, and bounded messages before mutation. `[CITED: https://cornucopia.owasp.org/taxonomy/asvs-5.0/01-encoding-and-sanitization/02-injection-prevention]` |
| V6 Cryptography | no new control | Reuse OS keychain/ignored vault and existing secret references; never hand-roll encryption. `[VERIFIED: codebase]` |

### Known Threat Patterns for the Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Shell-code injection through names/bodies/values | Tampering / Elevation | Validate typed identities, emit only in `core/shell/zsh/emit.go`, validate exact generated bytes with `zsh -n`, and do not build shell command strings. `[VERIFIED: codebase]` `[CITED: https://cornucopia.owasp.org/taxonomy/asvs-5.0/01-encoding-and-sanitization/02-injection-prevention]` |
| Worktree path/symlink replacement | Tampering | Reuse private-root canonicalization/ownership checks, descriptor anchoring, generated fixed filenames, no-follow behavior, and same-directory atomic replace. `[VERIFIED: codebase]` `[CITED: https://cornucopia.owasp.org/taxonomy/asvs-5.0/05-file-handling/03-file-storage]` |
| Stale shell overwrites unrelated edits | Tampering | Per-identity revision overlap detection and visible history-gap refusal. `[VERIFIED: fresh Spike 001 verifier]` |
| Secret literal in state/log/error | Information Disclosure | Exclude before persistence, retain only `SecretRef`, use value-free event metadata, and avoid raw helper stderr in prompt diagnostics. `[VERIFIED: codebase + spike blueprint]` `[CITED: https://cornucopia.owasp.org/taxonomy/asvs-5.0/13-configuration/03-secret-management]` `[CITED: https://cornucopia.owasp.org/taxonomy/asvs-5.0/16-security-logging-and-error-handling/02-general-logging]` |
| Snapshot/event/helper resource exhaustion | Denial of Service | Bounded frames/records/value sizes/history/events, deadlines, recursion guards, and failure without blocking the prompt. `[VERIFIED: fresh Spike 001 verifier]` |
| Invisible same-key loss | Repudiation / Tampering | First-publisher-wins plus durable value-free conflict metadata surfaced in status/diff. `[VERIFIED: fresh Spike 001 verifier]` |

## Sources

### Primary (HIGH confidence)

- Current working-tree code under `core/model`, `core/activate`, `core/store`, `core/cli`, `core/shell/zsh`, and `core/cmd/zsh-pro` — exact seams, invariants, and uncommitted hardening. `[VERIFIED: codebase]`
- `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md`, `.planning/STATE.md`, and the accepted Phase 7 note — scope, workflow, and locked constraints. `[VERIFIED: project planning artifacts]`
- Project-local `spike-findings-zsh-pro` skill, synchronization blueprint, and Spike 001 source/README — validated implementation constraints. `[VERIFIED: project skill]`
- Fresh execution of Spike 001 `verify.sh` — all eight groups and current performance observations. `[VERIFIED: fresh Spike 001 verifier]`
- Fresh targeted Go tests against the uncommitted working tree. `[VERIFIED: fresh test run]`

### Secondary (MEDIUM confidence)

- [zsh Functions](https://zsh.sourceforge.io/Doc/Release/Functions.html) — `precmd`/`preexec` timing. `[CITED: https://zsh.sourceforge.io/Doc/Release/Functions.html]`
- [zsh Line Editor](https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html) — `zle-line-finish` timing. `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html]`
- [zsh User Contributions](https://zsh.sourceforge.io/Doc/Release/User-Contributions.html) — cooperative hook helpers. `[CITED: https://zsh.sourceforge.io/Doc/Release/User-Contributions.html]`
- [zsh Modules](https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html) and [Parameters](https://zsh.sourceforge.io/Doc/Release/Parameters.html) — semantic state tables and tied PATH/FPATH arrays. `[CITED: https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html]` `[CITED: https://zsh.sourceforge.io/Doc/Release/Parameters.html]`
- [git update-ref](https://git-scm.com/docs/git-update-ref.html) and [git commit-tree](https://git-scm.com/docs/git-commit-tree.html) — transactional/CAS ref updates and commit creation. `[CITED: https://git-scm.com/docs/git-update-ref.html]` `[CITED: https://git-scm.com/docs/git-commit-tree.html]`
- [Go `os` package](https://pkg.go.dev/os) — temporary file, sync, and rename APIs. `[CITED: https://pkg.go.dev/os]`
- [OWASP ASVS 5.0](https://owasp.org/www-project-application-security-verification-standard/) — applicable input, file, secret, and logging control categories. `[CITED: https://owasp.org/www-project-application-security-verification-standard/]`

### Tertiary (LOW confidence)

- None. All factual planning claims were checked against project artifacts, executable evidence, or official documentation.

## Metadata

**Confidence breakdown:**

- Standard stack: HIGH — existing versions and dependencies were verified locally; no package addition is recommended. `[VERIFIED: codebase + environment probe]`
- Architecture: HIGH — derived from current production seams and a freshly passing project-owned prototype. `[VERIFIED: codebase + fresh Spike 001 verifier]`
- Pitfalls: HIGH — the concurrency/prompt-path cases are executable spike cases, while the reverse-ownership and source-IR hazards are directly visible in current code. `[VERIFIED: codebase + fresh Spike 001 verifier]`
- Official API details: MEDIUM — verified against primary project documentation sites through the research seam. `[CITED: official documentation URLs above]`

**Graph note:** `.planning/graphs/graph.json` was about 766 hours old and 349 commits behind; three discovery queries returned no nodes, so no architecture claim relies on it. `[VERIFIED: graphify status/query]`

**Research date:** 2026-08-17
**Valid until:** 2026-09-16 for the stable local stack; re-run environment and spike measurements if implementation begins later.
