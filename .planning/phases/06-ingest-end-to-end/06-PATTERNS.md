# Phase 6 Current Implementation Patterns

**Refreshed:** 2026-08-02
**Scope:** Current landed source plus the Phase 6 transaction interfaces the plans will add
**Precedence:** Current source and the executor's `read_first` files override historical Phase 5/6 prose when signatures or behavior differ.

## Landed composition contracts

### CLI surface

- `(*CLI).Run(args []string, stdout, stderr io.Writer) int` is the public dispatcher. Phase 6 adds `case "ingest"` without changing this signature.
- `cli.New(p shell.Provider, s Store, e Emitter) *CLI` and `cli.NewWithStoreInitializer(p, s, e, initializer) *CLI` are the landed injection seams.
- `core/cli.Store` is a narrow interface. `core/cli` must not import the concrete store or `core/shell/zsh`.
- Existing exit codes are integer forms of `model.ExitCode`: 0 clean, 1 runtime, 2 usage, 3 actionable.
- `runAnalyze` is the argument/path-boundary pattern: parse flags, expand `~` once at the CLI boundary, and pass concrete bytes/path downstream.

### Store/bootstrap order

- `runInstallWithStoreInitialization` reads and validates the target, invokes the injected store initializer, prepares the target, prepares/validates the cache loader, promotes the loader, then promotes `.zshrc`.
- Loader-before-target promotion is a shipped invariant. Phase 6 preserves it while moving initializer completion before any cache directory creation/open.
- `StoreInitialization` currently carries `Rollback` and caller-visible path/creation state. Phase 6 replaces boolean/path ownership authority with a Store-issued opaque `InstallInitializationID` bound to the exact Store instance, canonical root, and sealed baseline.
- `core/cmd/zsh-pro/main.go` is the sole concrete composition root, but the current initializer closure constructs a second `*store.Store` after `cliStore`. Phase 6 removes that split and routes initialization plus Begin/Commit/Abort through the exact `cliStore` pointer. `TestMainInitToBeginUsesExactCLIStore` is the real command-package proof: a second Store over the root cannot use the first pointer's initialization ID before effects.

## Landed managed-marker topology

The marker constants currently live in `core/cli/install.go`:

- START: `# >>> zsh-pro >>>`
- END: `# <<< zsh-pro <<<`

`replaceManagedBlock` is byte-oriented and recognizes only exact physical marker lines. Phase 6 factors its scan into one shared topology parser; it does not add a second marker implementation.

| Topology | Landed ordinary-install behavior | Phase 6 ingest behavior |
| --- | --- | --- |
| No markers | Append one managed region using landed separator/newline rules | Full-source first adoption |
| One balanced region | Replace that region; preserve all outside bytes | Installed-first-adoption or re-ingest from durable profile-object presence; eligible ordinary source is outside managed span |
| Multiple sequential balanced regions | Collapse to one region; preserve ordinary bytes before, between, and after regions | Run the same repair and rescan; every managed region is excluded, and a later balanced region is not generic appended content |
| Orphan END | Refuse before write | Malformed; refuse before initializer/cache/target effects |
| Unterminated START | Refuse before write | Malformed; refuse before effects |
| Nested or interleaved markers | Refuse before write | Malformed; refuse before effects |

Only ordinary nonblank/noncomment bytes after the canonical single END produce D-11's warning. Those bytes remain byte-identical and are not ingested.

## Current transaction gaps the plans replace

### Target promotion

- `preparedWrite.promote` currently calls pathname `os.Rename(temp, target)` and then syncs the parent directory.
- `writeRollback.restore` separately renames or removes by pathname.
- Validation and mutation are therefore not one conditional operation; move-aside plus link would also create an observable target absence.
- Phase 6 replaces both initial and final target mutation with one authenticated transaction protocol, not an ingest-only writer.

### Store/quarantine cleanup

- `installInitializationState.rollback` currently re-snapshots a pathname and calls `os.RemoveAll(s.dir)` for a fresh store root.
- That implementation is historical context, not the Phase 6 quarantine-cleanup pattern.
- Current Git command setup appends owned variables to `os.Environ()` and uses ambient repository discovery, so hostile inherited Git directory/work-tree/config/index/object variables can redirect effects. Phase 6 centralizes every Store command behind one typed non-worktree plumbing builder: strip inherited `GIT_*`, bind absolute canonical bare `GIT_DIR`, set `GIT_CONFIG_NOSYSTEM=1` and `GIT_CONFIG_GLOBAL=os.DevNull`, omit `GIT_WORK_TREE`, then add private candidate index/object/alternate values only where required. Checkout/reset/switch/restore/clean, `read-tree -u`, repository/work-tree overrides, and equivalent worktree-writing forms are rejected before process creation; the long-lived update-ref process reuses this exact runner.
- Each canonical Store root maps to one deterministic persistent sibling transaction namespace beneath its authenticated parent, current-EUID mode 0700 with a fixed no-follow regular mode-0600 lock entry. On Linux/Darwin, every cooperating zsh-pro process acquires a descriptor-held exclusive advisory OS lock before authenticating a private random mode-0700 quarantine and holds it through descriptor-relative mutation, postvalidation, and parent sync. The stable sibling namespace/lock entry is not removed with a token quarantine or fresh Store-root rollback, and authentication evidence never survives unlock/reacquire.
- `AbortIngest` retains authenticated parent/quarantine handles and recorded device/inode identity, recurses only through those handles, and reauthenticates every file, symlink, nested directory, and the top entry immediately before descriptor-relative unlink/rmdir while that root lock is held. Lock open/acquire failure, discarded lock descriptor, observed replacement at either seam, or cleanup/durability uncertainty stops further mutation and retains the remaining tree/recovery evidence. Advisory locking coordinates cooperating processes only; arbitrary non-cooperating same-UID/privileged mutation of private transaction internals is outside the product guarantee, though every observed mismatch still retains/requires recovery.
- Public `BeginIngest(ctx, installInitializationID)` is structurally main-only. Under the Store mutex it validates the ID and reserves a provisional token before ref/profile/filesystem I/O; every exit terminalizes that same immutable record, including clean no-artifact failure and uncertain retained setup. Initializer rollback is safe only when every reserved token beneath the exact initialization ID is terminal, no-publication, and cleanup removed.
- The legacy generic `Store.Commit(ctx, branch, ...)` never calls public Begin. It uses private `beginTransactionForRef` under internal non-removable authority so arbitrary validated branch B is preserved and main remains untouched.

## Atomic namespace pattern

The repository already uses build-tagged raw syscall wrappers and retained descriptors in:

- `core/cli/cache_syscalls_linux.go`
- `core/cli/cache_syscalls_darwin.go`
- `core/cli/runtime_openat_linux.go`
- `core/cli/runtime_openat_darwin.go`

Phase 6 follows that pattern with no new module dependency:

| File | Planned responsibility |
| --- | --- |
| `core/cli/atomic_rename.go` | `atomicRenameMode`, strict sibling-basename validation, `atomicRenameAt`, `ErrAtomicRenameUnsupported` |
| `core/cli/atomic_rename_linux.go` | Raw `renameat2`: exchange for existing targets, no-replace for absent targets |
| `core/cli/atomic_rename_darwin.go` | Raw descriptor-relative `renameatx_np`: swap for existing targets, exclusive create for absent targets |
| `core/cli/atomic_rename_other.go` | Fail-closed non-Linux/non-Darwin adapter |
| `core/cli/atomic_rename_test.go` | Platform-neutral validation, per-call injection isolation, and Go 1.25 Linux ABI/fallback AST audit |
| `core/cli/atomic_rename_supported_test.go` | `linux || darwin` direct exchange/no-replace and injected unsupported-error tests |
| `core/cli/atomic_rename_unsupported_test.go` | `!linux &amp;&amp; !darwin` native unsupported/zero-call tests |

Rules:

- Existing target: candidate and target are atomically exchanged. The target pathname always names the old or new complete file; candidate/recovery names the displaced occupant after the syscall.
- Absent target: use exclusive/no-replace creation, never exchange.
- There is no `os.Rename`, hard-link sequence, delete-then-rename sequence, or weaker retry when the exact mode is unavailable.
- `atomicRenameAtWithSyscall(call syscall6Fn,...)` receives its raw dependency per invocation; production passes `syscall.Syscall6` directly and tests use independent closures. Supported attempts make exactly one raw call, validation rejection makes zero, and no mutable package global controls the syscall.
- `TestAtomicRenameLinuxGo125ABIAndFallbackSurface` derives the complete Linux architecture set from local Go 1.25 `go tool dist list`, parses local syscall sources and production AST, and pins every trap, flags 1/2, Syscall6 ABI order, one-call surface, and absence of retry/ordinary rename/link/unlink/remove fallbacks.
- A substitution exchanged after prevalidation is detected by postvalidation. Reverse only through another guarded exchange while both identities still match; otherwise retain both and require recovery.
- A successful namespace syscall followed by directory-sync uncertainty is recovery required. Generic pathname rollback is disarmed.
- Target snapshot equality is deliberately bounded to existence, requested-link topology, regular-file type, device/inode, content digest, and permission bits. Target uid/gid, timestamps, ACLs, xattrs, and platform flags are outside equality; private journal owner authentication is separate.
- The private journal binds parent device/inode, journal descriptor identity/mode/uid/gid/digest/schema/transaction ID, requested-link topology, and authenticated basenames. Every mutation and cleanup reauthenticates those fields.
- Candidate bytes and independent evidence are fsynced and closed before the prepared journal is written/fsynced and before `atomicRenameAt`; candidate/evidence failure advances no journal, performs no namespace call, and leaves the target unchanged.
- Each canonical target root has the same persistent private-namespace/advisory-lock pattern as Store cleanup for cooperating zsh-pro instances. The lock spans fresh journal authentication through effect, postvalidation, journal transition, and directory fsync, but it does not exclude arbitrary external `.zshrc` writers. Those remain in scope through exchange/no-replace, displaced-identity postvalidation, guarded reverse, or retained recovery.
- `install.go` and `install_transaction.go` route every namespace mutation through one audited descriptor-relative helper. A static AST test rejects direct rename/remove/recursive-remove/link/root mutation outside it.
- Once absent-target no-replace succeeds, this phase has no audited identity-conditional inverse to restore absence. A later rollback retains target+journal and returns recovery required rather than unlinking by name.

## Store publication and controller result pattern

- `RefPresent`, optional `ExpectedRevision`, and `ProfileObjectPresent` are separate evidence. `ExpectedRevision` is absent if and only if RefPresent is false. Absent main performs only ref-existence resolution, zero exact-revision/profile/object/tree/parent reads, seeds an empty private index, and commits without a parent. Present main captures one expected object ID; Init-only profile absence, committed-empty profile presence, and operational probe/read failure remain distinct. Commit rejects any presence invariant mismatch before starting update-ref.
- `CommitIngest` atomically claims Store + initialization ID + token. Commit, Abort, and terminal cleanup serialize on that registry state, while private quarantine cleanup additionally uses the real cooperating-process Store-root lock; caller paths never select artifacts.
- RefPresent/ExpectedRevision selects one of two package-private byte-exact update-ref encoders only after validation: absent uses create; present uses update(expected). Callers cannot provide a raw ref verb, and invalid state starts no process.
- Persisted SecretRef validation is structural-first: unsupported/malformed rows call backend Kind zero times; a structurally valid wrong-backend-kind row and valid pass-through call Kind once; every persisted-reference row keeps Retrieve/Store/Delete zero. Git counters reset after Begin prove malformed CommitIngest performs no candidate object writes, final publication, ref-process start, or ref change.
- Publication truth and quarantine cleanup truth are independent: committed/not-committed is preserved even when cleanup is removed, retained, or uncertain. Only a lost Git update-ref commit response enters observation ambiguity; backend, object-publication, and cleanup failures remain directly classified.
- `IngestResult` counts `managed_entries`, `master_statements`, and physical `master_block_lines` separately. The invariant is `managed_entries + master_statements = accounted_statements = source_statements`; multiline master statements may span multiple block lines.
- Every post-initial-promotion/pre-Commit failure uses one `abortAndCompensatePreCommit` helper. `ir.Build` is total; parse, duplicate identity, accounting, candidate preparation, and snapshot validation are the actual pre-Commit failure seams.
- The ingest failure renderer accepts only a value-free result plus a closed safe reason. JSON preserves status flags, all counts, withheld name/line metadata, and warnings without receiving a raw error.

## Phase 6 interface map

| Plan | Planned symbols/artifacts | Consumers |
| --- | --- | --- |
| 06-01 | `InstallInitializationID`, optional `ExpectedRevision`, main-only `IngestBaseline`, `IngestAbortOutcome`, `BeginIngest`, `AbortIngest`, pre-I/O reservation, hermetic canonical bare-Git state, persistent per-root transaction lock, replayable recursive cleanup | 06-02, 06-03, 06-04 |
| 06-02 | `IngestCommitOutcome`, fixed-main `CommitIngest`, private branch-aware legacy constructor, exact ExpectedRevision/RefPresent wire table, atomic registry claim, independent publication/cleanup axes, value-free withheld report | 06-03 contract extension, 06-04 controller |
| 06-03 | shared marker topology, `prepareIngestInstallAt`, `installSnapshot`, `promotionOutcome`, per-call `atomicRenameAt`, tagged platform tests, target-root transaction lock, candidate/evidence-before-journal durability | 06-04 controller |
| 06-04 | `runIngest`, strict `CLI.Run` dispatch, ingest DTO, one-store composition root | 06-05 E2E |
| 06-05 | built-binary, real store/provider, privacy, no-execution, round-trip, exact-base/dependency gates, native macOS atomic plus real store-cleanup verifier/evidence | phase verification |

## Testing conventions

- Every focused verify command first enumerates exact test names with `GOTOOLCHAIN=local go test -list` and fails if any name is absent.
- Before any independent verifier does other work, `GOTOOLCHAIN=local go env GOVERSION` must match Go 1.25.x. Focused/full tests, vet, syscall/GOROOT audit, cross-builds, dependency queries, built-binary builds, the final `GOTOOLCHAIN=local make check`, and native macOS evidence all run in local mode; an automatic toolchain download cannot satisfy a Phase 6 gate.
- Race/crash tests use per-instance seams, never mutable package globals that can cross parallel tests.
- File assertions compare the explicit bounded snapshot fields, authenticated journal/parent/link state, and retained artifacts; target owner/timestamps/ACL/xattr/flags are not silently folded into equality.
- Platform-neutral atomic tests always run; exact local-GOOS selection runs the `linux || darwin` supported list or the `!linux &amp;&amp; !darwin` unsupported list. Linux/Darwin amd64/arm64 are cross-compiled with `CGO_ENABLED=0 GOTOOLCHAIN=local`, but cross-build is compile evidence only. Darwin support requires external exact-SHA native macOS runtime evidence for both atomic operations and production Store cleanup across top-level, nested file/directory, symlink, and before/after replacement rows, recording `go_toolchain: local` and exact Go 1.25.x `go_env_goversion` in 06-05.
- Landed ordinary-install marker and rollback suites remain unchanged and run after focused tests.
- `06-EXECUTION-BASE` contains exactly reviewed commit `557c818845f4239d5d50ecab02e37731bc117f10`; `go.mod`/`go.sum` remain byte-identical to it across HEAD, index, and worktree.
- Production dependency lists for `core/cli`, `core/ir`, and `core/store` are inspected under explicit Linux/Darwin amd64/arm64 build contexts; every context rejects the concrete zsh provider and both exact `golang.org/x/sys` and every `golang.org/x/sys/*` subpackage.
