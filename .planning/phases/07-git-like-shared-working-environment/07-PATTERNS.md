# Phase 7: Git-Like Shared Working Environment - Pattern Map

**Mapped:** 2026-08-17
**Files analyzed:** 23 production files expected to be created or modified
**Analogs found:** 23 / 23
**Source basis:** Current dirty working tree, Phase 7 research, requirements, roadmap, and the validated Spike 001 blueprint

The current working tree is authoritative. In particular, keep the uncommitted typed-nil runtime-store guard, descriptor-close hardening, NUL-framed introspection boundaries, representability tests, and secret-transport error distinction. Do not plan from HEAD-only versions of those seams.

## File Classification

| New/Modified File | Role | Data Flow | Closest Current Analog | Match Quality |
|---|---|---|---|---|
| `core/model/worktree.go` | model | event-driven / transform | `core/model/profile.go`, `core/activate/plan.go` | role + contract |
| `core/activate/plan.go` | model | transform | `core/activate/plan.go:4-92` | exact (modify in place) |
| `core/activate/builder.go` | service | transform | `core/activate/builder.go:15-155` | exact (modify in place) |
| `core/activate/diff.go` | utility | transform | `core/activate/diff.go:9-73` | exact (modify in place) |
| `core/worktree/service.go` | service | event-driven / CRUD | `core/store/store.go:107-176`, `core/store/store.go:790-988` | role + flow |
| `core/worktree/registry.go` | utility | transform | `core/ir/route.go:35-77`, `core/store/secret.go:83-203` | policy match |
| `core/worktree/diff.go` | utility | transform | `core/activate/diff.go:64-190` | exact role + flow |
| `core/worktree/state.go` | model / config | file-I/O | `core/store/dto.go:24-125` | exact role + flow |
| `core/worktree/atomic_unix.go` | utility | file-I/O / concurrency | `core/store/store_root_private_unix.go:39-120`, `core/cli/cache_directory_unix.go:281-345` | exact role + flow |
| `core/store/store.go` | service | CRUD / request-response | `core/store/store.go` | exact (modify in place) |
| `core/store/git.go` | service | request-response / subprocess | `core/store/git.go` | exact (modify in place) |
| `core/store/dto.go` | model / utility | transform | `core/store/dto.go` | exact (modify in place) |
| `core/cli/cli.go` | controller | request-response | `core/cli/cli.go:58-109` | exact (modify in place) |
| `core/cli/store.go` | provider / interface | request-response | `core/cli/store.go:9-25` | exact (modify in place) |
| `core/cli/ingest.go` | controller | file-I/O / CRUD | `core/cli/ingest.go:136-376` | exact (modify in place) |
| `core/cli/worktree.go` | controller | request-response | `core/cli/ingest.go:19-53`, `core/cli/cli.go:119-161` | role + flow |
| `core/cli/runtime.go` | controller | event-driven / request-response | `core/cli/runtime.go:25-176` | exact (modify in place) |
| `core/cli/emitter.go` | service | transform | `core/cli/emitter.go:47-169` | exact (modify in place) |
| `core/shell/provider.go` | provider | request-response | `core/shell/provider.go:12-68` | exact (modify in place) |
| `core/shell/zsh/introspect.go` | service | streaming / transform | `core/shell/zsh/introspect.go:23-135` | exact framing seam |
| `core/shell/zsh/emit.go` | utility | transform | `core/shell/zsh/emit.go` | exact (modify in place) |
| `core/shell/zsh/hook.go` | component | event-driven | `core/shell/zsh/hook.go` | exact lifecycle seam |
| `core/cmd/zsh-pro/main.go` | config / provider | request-response | `core/cmd/zsh-pro/main.go:1-93` | exact (modify in place) |

The recommended project structure in research names 20 files directly. `core/activate/plan.go` is additionally implied by the required shell-neutral forward-plus-reverse live patch contract; `core/cli/emitter.go` and `core/activate/{builder,diff}.go` are explicit existing seams that the research says must be extended.

## Pattern Assignments

### `core/model/worktree.go` (model, event-driven / transform)

**Analogs:** `core/model/profile.go:80-118` for plain domain values and presence-aware fields; `core/activate/plan.go:4-92` for a closed typed-operation contract; `core/model/category.go:4-25` for string-backed enums.

**Copy this shape:**

```go
// core/activate/plan.go:4-11
type Plan struct {
    Deactivate []Op
    Activate   []Op
}

type Op interface{ activationOp() }
```

Define typed `Identity`, live value variants, add/change/remove `LiveChange`, overlay entries/tombstones, revision events, conflicts, the exact private Attach/Publish/Prepare/Acknowledge/Resolve requests, and status/diff results as shell-agnostic values. Use pointer or explicit presence fields wherever unset and present-empty differ; that is already the model convention in `core/model/profile.go:24-61` and `core/model/manifest.go:17-40`.

Keep JSON tags, locks, filesystem paths, Git refs, and zsh source out of this file. `core/worktree/state.go` owns durable encoding.

### `core/activate/plan.go` (model, transform)

**Analog:** the file itself, especially the one-operation-per-semantic-action pattern at `core/activate/plan.go:13-92`.

Add only shell-neutral live patch operations that the zsh emitter needs. A Phase 7 plan must represent both:

- forward final-state mutations, including explicit removes; and
- replacement reverse ownership for the shell's new acknowledged baseline.

Do not place zsh commands such as `unset`, `unalias`, or `unsetopt` in this package. Preserve the private marker-interface pattern so callers cannot supply arbitrary operations.

### `core/activate/builder.go` (service, transform)

**Analog:** `Build` at `core/activate/builder.go:15-155`.

Reuse its admission and final-source-occurrence reduction:

```go
// core/activate/builder.go:35-49
for _, e := range p.Entries {
    if !e.EffectiveManaged() || !e.Representable() {
        continue
    }
    switch e.Kind {
    case model.KindAssignment:
        if len(e.Names) != 1 {
            continue
        }
        // ...
    }
}
```

Project the effective worktree document into a manifest/patch input; do not mutate `Profile.Entries` as though it were a map. Preserve final occurrence, export provenance, function-body presence, PATH/FPATH source order, and the existing name validators at `core/activate/builder.go:9-13`.

### `core/activate/diff.go` (utility, transform)

**Analog:** `Diff` and normalization at `core/activate/diff.go:9-126`.

Copy the "validate the complete target before producing any operation" behavior:

```go
// core/activate/diff.go:11-45
func Diff(active, target *model.Manifest) (Plan, error) {
    // schema and target metadata validation first
    // normalization second
    // only then append deactivate/activate operations
}
```

For live patches, normalize each identity to its final value, reject malformed category/value metadata before emitting a partial plan, and preserve deterministic category/name order.

### `core/worktree/service.go` (service, event-driven / CRUD)

**Analogs:** injected stateful service at `core/store/store.go:107-176`; claim/validate/mutate/terminalize transaction flow at `core/store/store.go:318-414` and `core/store/store.go:790-988`.

Use a constructor-injected service with no package-global singleton. Its dependencies should be narrow interfaces for durable state, Git projections, clock/ID generation if needed, and shell-neutral patch planning.

Every mutating operation—materialize, publish, acknowledge, commit, branch creation, checkout, reset, conflict resolution—must enter the same bounded worktree lock. Within it:

1. load and validate the authoritative state;
2. compare the shell's acknowledged revision with retained events;
3. decide disjoint acceptance, overlap conflict, or history gap;
4. update the authoritative identity map and bounded events;
5. atomically persist; and
6. return a typed result with no raw captured values in errors/events.

Match the existing transaction convention: distinguish conflict, clean non-mutation, committed mutation, and recovery-required/uncertain outcomes rather than collapsing them into one error.

### `core/worktree/registry.go` (utility, transform)

**Analogs:** the conservative reversible-shape gate at `core/ir/route.go:35-77` and store-side reuse of an existing secret verdict at `core/store/secret.go:3-14`, `core/store/secret.go:103-159`.

Copy the policy layering:

- seed ownership from the materialized profile's `EffectiveManaged() && Representable()` identities;
- treat PATH/FPATH and supported options as explicit typed categories;
- auto-admit only identities absent at shell attachment and passing safe-name, bookkeeping, volatile, and secret gates;
- preserve valid existing `SecretRef` identities;
- reject a new literal secret before any worktree/event/conflict persistence.

Do not import `core/shell/zsh` or add a second secret regex. Reuse/expose the existing classification result; events may carry category/name/reason but never value.

### `core/worktree/diff.go` (utility, transform)

**Analog:** `core/activate/diff.go:64-190`.

Use final-occurrence normalization and explicit add/change/remove kinds. Sort by the accepted user-facing category order, then identity name, so two terminals render byte-stable status/diff results. Do not compare serialized JSON or command text; compare typed semantic values, preserving ordered arrays for PATH/FPATH.

The categorized result should be a model returned to the CLI, not terminal text. Secret values must be structurally unrepresentable in this result.

### `core/worktree/state.go` (model / config, file-I/O)

**Analog:** store-local DTO ownership in `core/store/dto.go:15-25` and deterministic marshal/unmarshal at `core/store/dto.go:84-125`.

```go
// core/store/dto.go:94-104
func MarshalProfile(p model.Profile) ([]byte, error) {
    // map domain values to a store-local DTO
    b, err := json.MarshalIndent(dto, "", "  ")
    if err != nil {
        return nil, err
    }
    return append(b, '\n'), nil
}
```

Give the durable document an explicit schema version and stable fields for branch, base OID, head revision, compaction floor/baseline, authoritative identities, bounded events, conflicts, and persisted auto-apply default. Give per-shell state a separate DTO keyed by an opaque shell ID.

Decode unknown/incomplete versions fail-closed. Copy slices/maps/pointers defensively as `core/store/dto.go:127-290` does. Keep nil, present-empty, and tombstone distinct. Marshal deterministically with one trailing newline.

### `core/worktree/atomic_unix.go` (utility, file-I/O / concurrency)

**Analogs:** authenticated cross-process lock at `core/store/store_root_private_unix.go:39-120` and durable same-directory staged replacement at `core/cli/cache_directory_unix.go:281-345`.

Copy these invariants:

```go
// core/store/store_root_private_unix.go:99-119
if err := lockStoreRootTransaction(ctx, lock); err != nil {
    // preserve cancellation/deadline; otherwise map to bounded lock unavailable
}
guard.valid = true
if !guard.authenticationValid() {
    return ErrStoreTransactionLockUnavailable
}
callbackErr := fn(guard)
if callbackErr != nil {
    return callbackErr
}
if !guard.authenticationValid() {
    return ErrStoreTransactionLockUnavailable
}
```

```go
// core/cli/cache_directory_unix.go:308-319,332-345
if n, err := file.Write(content); err != nil {
    cleanup()
    return "", nil, err
} else if n != len(content) {
    cleanup()
    return "", nil, io.ErrShortWrite
}
if err := file.Sync(); err != nil {
    cleanup()
    return "", nil, err
}
// same-directory rename, then directory.Sync()
```

Use a current-EUID private 0700 root, a no-follow regular 0600 lock, nonblocking `flock` retries under a short deadline, descriptor-relative mutation, same-directory random temp, file chmod/write/fsync/close, rename, then directory fsync. Abandoned temp files are ignored/cleaned; readers never accept them as state. Preserve context cancellation separately from ordinary fail-open hook contention.

### `core/store/store.go` (service, CRUD)

**Analog:** the file itself.

Reuse `Branches` (`core/store/store.go:249-265`), exact-ref `Read` (`core/store/store.go:1108-1145`), branch validation (`core/store/store.go:1147-1189`), and the existing commit-tree/update-ref transaction.

The current `Create` is the exact code to replace, not to preserve:

```go
// core/store/store.go:293-309 — Phase 7 changes the hard-coded main base
func (s *Store) Create(ctx context.Context, name string) error {
    // ...
    mainSHA, err := s.git.revParse(ctx, "refs/heads/main")
    // ...
    return s.git.updateRefCAS(ctx, "refs/heads/"+name, mainSHA, strings.Repeat("0", len(mainSHA)))
}
```

Add a create-from-expected-current-OID operation. Commit the worktree's effective profile with `base_oid` as the parent/expected ref. A ref mismatch is a visible conflict. Store remains responsible for Git objects/refs and secret-reference persistence, not prompt snapshots or per-shell acknowledgements.

### `core/store/git.go` (service, request-response / subprocess)

**Analog:** the file itself.

Preserve all four gates:

- scrub inherited `GIT_*` and own `GIT_DIR` (`core/store/git.go:93-129`);
- allowlist exact argv before process creation (`core/store/git.go:174-270`);
- use context deadlines and map raw stderr to typed zsh-pro errors (`core/store/git.go:307-459`);
- encode typed CAS/ref transactions, never caller-authored frames (`core/store/git.go:659-695`, `core/store/git.go:759-933`).

Add only the minimum plumbing wrappers needed to read a target exact revision or create/reset a ref from the worktree base. Do not add porcelain checkout/reset or shell command strings. Keep `commit-tree <tree> -p <expected>` and `update-ref` expected-old semantics.

### `core/store/dto.go` (model / utility, transform)

**Analog:** the file itself.

Extend through store-local DTOs, not JSON tags on `model.Profile`. Preserve old profile decoding and source-order fields. If the effective worktree projection is encoded into Git, make additions presence/version-aware and fail closed rather than inferring missing semantic data. Reuse deterministic marshal, defensive copies, and additive compatibility tests from `core/store/dto_test.go:214-246` and `core/store/dto_test.go:360-405`.

Shared branch/revision/shell acknowledgement coordination does not belong in committed `profile.json`.

### `core/cli/cli.go` (controller, request-response)

**Analog:** strict dispatcher and exit-code boundary at `core/cli/cli.go:58-109`.

Add public binary dispatch for categorized `diff`, `commit`, branch list/create, shared `checkout`, `reset --hard`, and `config set auto-apply true|false`. Validate arity/flags before dependency or filesystem work, as `noArgumentUsage` does at `core/cli/cli.go:111-117`. Direct binary `sync` and `sync --resolve shared` must fail before dependency access because both are sourced-loader parent-shell flows.

Replace `runStatus`'s process-local `Store.Current()` output (`core/cli/cli.go:133-142`) with the worktree service's shared branch/base/revision/dirty/conflict plus current-shell applied/behind/auto-apply view.

### `core/cli/store.go` (provider / interface, request-response)

**Analog:** narrow consumer-owned interfaces at `core/cli/store.go:9-25`.

Keep interface segregation. Prefer focused interfaces such as read-only worktree status, workflow mutation, and runtime synchronization over exposing concrete `*store.Store` or `*worktree.Service`. Carry model-owned request/result types across the boundary. Ordinary read-only commands must not initialize or repair storage implicitly unless the service returns an explicit idempotent recovery-needed result prescribed by research.

### `core/cli/ingest.go` (controller, file-I/O / CRUD)

**Analog:** `runIngestWithSeams` at `core/cli/ingest.go:136-376`.

The worktree materialization participant belongs after a baseline commit is authoritatively reported committed (`core/cli/ingest.go:323-363`), before the command can report complete (`core/cli/ingest.go:364-376`). Preserve publication truth: if Git committed but materialization fails, return recovery-required with `ProfileCommitted=true`; do not pretend the commit rolled back.

Materialization must be idempotently reconstructible from the committed DTO/exact OID so a later `status` or `sync` can repair it before hooks operate.

### `core/cli/worktree.go` (controller, request-response)

**Analogs:** closed argument parsing at `core/cli/ingest.go:19-53`, typed rendering at `core/cli/ingest.go:552-589`, and simple command failure handling at `core/cli/cli.go:119-161`.

Put user-facing command parsing/output here; delegate dirty checks, categorized diff, concurrency, secrets, and Git policy to services. Use typed requests and typed results. Do not pass raw errors, object IDs, captured values, or helper stderr to renderers unless a command's documented output explicitly requires a non-secret value.

`checkout` must reject dirty worktree; `reset --hard` is the only ordinary discard path; `commit` includes every supported dirty identity; `branch <name>` forks the current committed base without switching. `sync` and `sync --resolve shared` are deliberately absent from direct binary dispatch and reuse the runtime prepare/apply/verify/reverse/fresh-capture/ack flow only through the sourced `zsh-pro()` dispatcher.

### `core/cli/runtime.go` (controller, event-driven / request-response)

**Analog:** the private allowlist and descriptor-bound timeout boundary at `core/cli/runtime.go:25-128`, plus exact-byte validation at `core/cli/runtime.go:131-176`.

Extend the private command allowlist with exactly five operations: Attach, Publish, Prepare, Acknowledge, and Resolve. Prepare is not Pull/query. Argv contains only the non-authorizing operation tag and the existing integer outer watchdog; shell capability, shell ID, operation ID, revisions/tokens, and operation payload travel in one `ZPWT` version-1 stdin envelope. Cap the complete envelope at exactly 2,101,248 bytes and 10,016 records, read it once with a plus-one overflow sentinel, require an explicit final record followed by EOF, and reject partial lengths, early EOF, duplicates, unknown/out-of-order fields, operation-tag mismatch, trailing bytes, and cap-plus-one before Service access. Never fall back to argv, environment, pathname, or a second read.

Keep finite context deadlines, in-memory/pipe output, descriptor-bound roots, raw diagnostic suppression, and typed-nil guards. All five runtime operations forward the decoded credential unchanged to Service, which alone checks the verifier under the canonical transaction. They must never accept arbitrary paths, refs, Git argv, shell commands, or output destinations. Attach allocation is the sole credential-less frame mode and returns its generated pair through a separate bounded private response after descriptor authentication.

### `core/cli/emitter.go` (service, transform)

**Analog:** `runtimeEmitter.Emit` at `core/cli/emitter.go:78-139` and descriptor-bound construction at `core/cli/emitter.go:141-169`.

Reuse the production chain:

```text
typed state -> resolve valid SecretRefs -> effective manifest/patch plan
            -> shell.RuntimeEmitter -> exact in-memory zsh payload
```

Add the live-patch method beside, not around, this chain. It must return forward mutations and a replacement retained reverse function together. Preserve the current dirty-tree typed-nil check at `core/cli/emitter.go:158-169`. Never render shell in CLI/worktree code.

### `core/shell/provider.go` (provider, request-response)

**Analog:** interface segregation and composition at `core/shell/provider.go:12-68`.

Add the smallest shell-neutral interfaces required for semantic snapshot decoding and live patch emission. Consumers should depend on those narrow interfaces, not the broad `Provider`. Types belong in `core/model`/`core/activate`; this package declares behavior only. Keep concrete zsh and persistence policy out.

### `core/shell/zsh/introspect.go` (service, streaming / transform)

**Analog:** the current dirty-tree NUL framing at `core/shell/zsh/introspect.go:23-46` and boundary-safe parser at `core/shell/zsh/introspect.go:64-135`.

Reuse one compatible framed semantic format for live values/attributes. Full live records must be NUL-framed; newline section framing is insufficient for multiline functions, quotes, empty values, and ordered list elements. Validate terminal markers, record arity, category/name syntax, maximum records, and maximum bytes before returning a snapshot.

Do not run the existing child `zsh -f` introspection subprocess on every prompt. The sourced loader captures its current shell; the bounded helper decodes/validates the frame.

### `core/shell/zsh/emit.go` (utility, transform)

**Analog:** the only production zsh code generator.

Copy:

- name validation and safe quoting at `core/shell/zsh/emit.go:61-106`;
- caller-selected runtime function names at `core/shell/zsh/emit.go:184-220`;
- complete-plan construction before return at `core/shell/zsh/emit.go:222-260`;
- typed operation switches with unsupported-operation errors at `core/shell/zsh/emit.go:262-470`.

Add live add/change/remove rendering here only. Emit explicit scalar unset, alias removal, function removal, PATH/FPATH replacement/delta, and option state changes from typed operations. Produce the replacement reverse ownership payload in the same call and validate hostile names before any source escapes.

### `core/shell/zsh/hook.go` (component, event-driven)

**Analog:** current bounded fail-open transport (`core/shell/zsh/hook.go:142-240`), protected eval (`core/shell/zsh/hook.go:243-283`), retained reverse lifecycle (`core/shell/zsh/hook.go:285-415`), and switch transaction (`core/shell/zsh/hook.go:417-479`).

Preserve these behaviors:

- generated source goes through the descriptor-bound capture and exact-byte `zsh -n` path;
- xtrace/history protections wrap validation/eval;
- a failed apply retains diagnosable recovery and returns control to the shell;
- public shell functions consume errors and return success to fail open;
- resolved source is cleared from dynamic locals/`REPLY` in `always` blocks.

Add idempotent namespaced handlers with the Spike 001 shape:

```zsh
autoload -Uz add-zsh-hook add-zle-hook-widget
add-zsh-hook precmd _zp_worktree_publish
add-zle-hook-widget line-finish _zp_worktree_pull
```

`precmd` captures and publishes only after the foreground command settles. `line-finish` pulls only; do not publish there. `sync` and `sync --resolve shared` are sourced-loader flows and use the same Prepare/Resolve -> render/validate/eval/replacement-reverse/fresh-snapshot/Acknowledge path. Update `ZP_ACTIVE_REVERSE_FN` ownership before acknowledging. Acknowledge only after the fresh managed snapshot matches the target. Build private frames with shell builtins under private locals and restored xtrace/history protections; the persistent non-exported marker is the only long-lived capability copy, while every call-local credential/control/frame/response copy and descriptor is cleared on all exits. Use a recursion guard and bounded helper deadline; store diagnostics in namespaced globals without blocking the prompt.

The shared current branch is durable worktree state. `ZSHPRO_PROFILE`/`ZP_ACTIVE_PROFILE` remain this shell's applied markers, not shared authority.

### `core/cmd/zsh-pro/main.go` (config / provider, request-response)

**Analog:** the file's sole-composition-root contract at `core/cmd/zsh-pro/main.go:1-15` and concrete wiring at `core/cmd/zsh-pro/main.go:50-93`.

Construct distinct scoped Services over one canonical durable `worktree.json` generation: public/materializer commands receive a path-bound Service from `OpenStateStore(path)`, while each private runtime operation receives a fresh authenticated-descriptor-bound Service from `NewStateStoreFromAuthenticatedRoot` after validation and RuntimeRoot authentication. Authority is canonical-generation identity, never Service pointer identity. Do not reopen a path, reuse the public path-bound store inside runtime, fall back to another authority, or add a package-global singleton; preserve caller-input versus StateStore-duplicate close ownership.

## Shared Patterns

### Ownership Boundaries

| Concern | Owner | Reused Current Pattern |
|---|---|---|
| Typed identities, values, deltas, revisions, conflicts | `core/model` | Plain domain structs and presence-aware fields |
| Admission, revision arbitration, status/diff, compaction | `core/worktree` | Conservative routing + typed store transaction |
| Git objects, exact revision reads, branch/ref CAS, SecretRef persistence | `core/store` | Hardened `gitRunner` and `CommitIngest` transaction |
| Shell capture framing, zsh mutations, forward/reverse code | `core/shell/zsh` | NUL parser + sole emitter + retained reverse |
| User/runtime command parsing and value-free output | `core/cli` | Strict dispatch, narrow interfaces, bounded runtime helper |
| Concrete dependency construction | `core/cmd/zsh-pro` | Sole composition root |

### Error Handling

Use typed domain outcomes for conflict/history gap/behind/dirty/recovery-required. Infrastructure errors may wrap context cancellation/deadline, but must not include raw Git stderr, captured shell values, secret values, or generated source. Hook-facing failures return a nonzero internal result for status tracking, then the zsh hook itself returns success so the prompt/command continues.

Sources to copy:

- Git error mapping: `core/store/git.go:452-460`.
- Runtime source diagnostic suppression: `core/cli/runtime.go:131-176`.
- Loader fail-open diagnostics: `core/shell/zsh/hook.go:129-140`.

### Validation

Validate at every authority boundary:

- CLI: arity/flags before dependencies.
- Runtime: exact five-operation allowlist, duplicated operation tag, version-1 stdin record grammar, shell ID/capability/revision/token shape, 2,101,248-byte and 10,016-record envelope bounds, one-shot read, final-record/EOF enforcement, and no trailing data or alternate credential channel.
- Model/worktree: category/name/value shape, admitted ownership, event/history bounds.
- Git: validated branch/ref and exact argv.
- Filesystem: private owner/mode, no-follow descriptors, authenticated lock and state entry.
- Shell: safe identity names, exact generated bytes through `zsh -n`.

The spike's starting bounds are 128 revision events and a 250 ms whole-transition default. Changing them needs retained production measurement; removing, disabling, resetting per stage, or making them user-unbounded is not an option.

### Private Capability Transport and Deadline Ownership

The raw `ShellCapability` is a bearer and must never appear in process argv or environment. The sourced loader writes the exact versioned envelope directly to helper stdin using zsh builtins in byte-counting locale, with xtrace/history locally disabled and restored. Keep the valid stable marker non-exported across re-source, but clear every call-local credential/control/frame/response copy and temporary descriptor in `always` on success, malformed input, timeout, signal, and lost response. A live blocked-helper test must inspect cmdline/environ before Service invocation, then prove shell-ID-only and wrong shell-ID/capability pairing cannot access receipts or mutate state. At rest, only Plan 07-03's domain-separated verifier exists.

`core/cli/runtime.go` owns the unexported monotonic `transitionClock`/`transitionBudget` seam and the finite 250 ms production default. Production construction is sealed to Go monotonic time; only package-private tests inject a fake clock, and no CLI/environment/public config can override the clock or create a zero/negative/unbounded budget. `core/shell/zsh/hook.go` owns the cleanup-bound parent transition guard from Prepare/Resolve through protected eval, reply parsing, replacement reverse, fresh capture, and Acknowledge. Exact cumulative 249 ms admission versus 251 ms timeout is proven with fake time, including final consumption at late stages and no real boundary sleep. Active production-path fail-open tests use 25 ms well-below-budget and 500 ms clearly-over-budget stalls for authentication, bind, helper read/execute, transport, protected eval, reverse replacement, fresh capture, and acknowledgement; every timeout retains the old acknowledged revision and usable prompt.

### Determinism and Concurrency

Sort user-visible diffs by category then name. Retain ordered PATH/FPATH elements. Under one lock, unrelated stale-shell identity deltas compose; overlapping identities are first-lock-wins with a visible loser conflict. A shell older than the compaction floor must reconcile cleanly or surface a history-gap conflict before publishing. Never publish a stale whole snapshot.

### Testing

| New/Changed Area | Existing Test Pattern to Copy |
|---|---|
| Domain round-trip, presence, schema compatibility | `core/model/manifest_test.go:10-53`, `core/model/manifest_test.go:97-138` |
| Final identity normalization and fail-before-plan | `core/activate/diff_test.go:30-58`, `core/activate/diff_test.go:113-181` |
| Lock retry, cancellation, deadline, modes | `core/store/store_root_private_unix_test.go:15-163` |
| Deterministic DTO and compatibility | `core/store/dto_test.go:214-246`, `core/store/dto_test.go:360-405` |
| Git allowlist, CAS conflict, real plumbing | `core/store/git_test.go:128-152`, `core/store/git_test.go:191-230`, `core/store/git_test.go:307-401` |
| Runtime typed-nil, descriptor binding, bounded stdin capability transport, fake monotonic deadline, source redaction | `core/cli/runtime_test.go:19-29`, `core/cli/runtime_test.go:42-154`, `core/cli/runtime_test.go:394-409` |
| Snapshot multiline/sentinel framing | `core/shell/zsh/introspect_test.go:74-122` |
| Emitter hostile names, syntax, apply/reverse | `core/shell/zsh/emit_test.go:18-113`, `core/shell/zsh/emit_test.go:115-246` |
| Loader surface, no source-time subprocess, fail-open boundary | `core/shell/zsh/hook_test.go:10-65` |
| Current-shell zero-residue E2E harness | `core/shell/zsh/live_terminal_test.go:13` and helpers beginning `core/shell/zsh/live_terminal_test.go:1441` |
| Exact composition-store identity | `core/cmd/zsh-pro/main_test.go:32` and `core/cmd/zsh-pro/main_test.go:141` |

Add a retained production-binary two-`zsh -f` harness derived from Spike 001 for auto on/off, next-command alias visibility, sourced-only explicit sync/resolve, disjoint/overlap races, history gaps, dirty checkout/reset, foreground non-interference, secret/noise exclusion, malformed/partial/trailing stdin frames, live helper cmdline/environ capability absence, deterministic/active deadline behavior, abandoned partial writes, fail-open helper loss, and `live edit -> sibling sync -> checkout/deactivate -> zero residue`.

## No Analog Found

No planned production file lacks a useful current codebase analog.

One subpattern is genuinely new to production: cooperative `precmd`/`zle-line-finish` registration and per-shell revision acknowledgement. Use the validated blueprint at `.codex/skills/spike-findings-zsh-pro/references/live-shell-state-synchronization.md:65-67` for hook registration and line 130 for initial bounds. Do not transplant the spike's direct patch renderer; route its behavior through `core/activate` and `core/shell/zsh/emit.go`.

## Planner-Critical Do/Do-Not List

**Do:**

- build the model/projection and reverse-ownership contract before installing hooks;
- put all shared mutations under one authenticated bounded lock;
- commit with expected base/ref CAS, then update the worktree base;
- materialize ingest idempotently from the exact committed baseline;
- acknowledge only after validate, eval, and fresh semantic snapshot;
- test real independent shells and zero residue.

**Do not:**

- rewrite `Profile.Entries` as the live state map;
- use `ZSHPRO_PROFILE` as shared current branch;
- run Git on each prompt or accepted line;
- persist ambient inherited shell state;
- add staging, merge/rebase/remotes, multiple worktrees, or process-local state;
- add a second secret classifier;
- apply from a watcher during a foreground command;
- source replaceable persistent patch paths;
- acknowledge before shell state is verified.

## Metadata

**Analog search scope:** `core/model`, `core/activate`, `core/worktree` (currently absent), `core/store`, `core/cli`, `core/shell`, `core/shell/zsh`, `core/cmd/zsh-pro`

**Files scanned:** 149 tracked Go files by path/signature search; 39 current production/test analog files opened fully or in targeted non-overlapping sections

**Dirty-tree source files explicitly preserved:** `core/cli/emitter.go`, `core/cli/runtime_root_unix.go`, `core/shell/zsh/introspect.go`, `core/store/errors.go`, `core/store/keychain.go`, plus their current uncommitted tests and the unrelated planning edits

**Pattern extraction date:** 2026-08-17
