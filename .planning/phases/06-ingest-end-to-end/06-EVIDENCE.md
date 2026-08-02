# Phase 6 Current-Source Evidence

**Refreshed:** 2026-08-02
**Status:** Planning evidence; executor must repeat the syscall audit before implementation
**Authority order:** Current source/read-first files > completed plan summaries > historical CONTEXT code-line commentary.

## Landed source snapshot

| Claim | Current evidence | Planning consequence |
| --- | --- | --- |
| Public dispatcher signature | `core/cli/cli.go`: `func (c *CLI) Run(args []string, stdout, stderr io.Writer) int` | 06-04 adds ingest dispatch with no signature/context change |
| Current injection seam | `cli.New(p, s, e)` and `NewWithStoreInitializer(p, s, e, initializer)`; `main.go` constructs `cliStore` but its initializer closure currently constructs a second Store | Extend the landed seam and remove the split; the exact `cliStore` pointer must issue initialization IDs and perform Begin/Commit/Abort |
| Store-neutral CLI boundary | `core/cli/store.go` defines `Store`, `StoreInitialization`, `StoreInitializer` | New transaction evidence stays in `core/model`; no concrete-store import |
| Store initialization placement | `runInstallWithStoreInitialization` calls initializer before target/cache preparation, then loader promotion before `.zshrc` promotion | Preserve shipped loader-before-target order, use that target promotion once for ingest, and make shared-root/rollback ordering explicit |
| Marker constants are landed | `core/cli/install.go` defines exact START/END constants | Historical claims that Phase 5 has not landed are stale |
| Balanced duplicates are repaired | `TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent`; `replaceManagedBlock` emits one replacement and removes later managed regions while preserving ordinary bytes | Multiple balanced regions are valid repair topology, not generic malformed/append topology |
| Malformed topology is rejected | Existing tests cover orphan END, unterminated START, nested START, and stray/interleaved END | Ingest must share the parser and fail before side effects |
| Current target promotion is pathname based | `preparedWrite.promote` calls `os.Rename`; rollback separately renames/removes | Replace with atomic exchange/exclusive creation and typed outcomes |
| Current installer compensation is filesystem-first | On `.zshrc` promotion failure, `runInstallWithStoreInitialization` calls target rollback, then loader rollback, then runtime cache/initializer cleanup; the exact shared-root helper prevents double ownership | Phase 6 controller must classify/restore the target first, then AbortIngest, then loader/cache, then initializer, with every uncertain result disarming later destructive steps |
| Current fresh-root cleanup is pathname recursive | `core/store/install_transaction.go` re-snapshots then calls `os.RemoveAll` | Phase 6 replaces transaction cleanup with a persistent per-canonical-root mode-0700 namespace, real descriptor-held Linux/Darwin advisory lock for cooperating processes, retained handles, no-follow recursive reauthentication, and descriptor-relative mutation through sync |
| Current Git execution inherits caller state and ambient repository discovery | `core/store/git.go` discards the absolute lookup result, invokes `git` with `-C`, and appends owned variables to `os.Environ()` without stripping existing `GIT_*` keys | Centralize all Store/ref/candidate commands on an absolute canonical bare `GIT_DIR`, disabled system/global config, absent work tree, sanitized Git environment, and typed non-worktree argv; hostile variables/decoy worktree tests prove confinement |
| Current initialization authority is caller-shaped | `StoreInitialization` exposes creation/path state rather than an opaque Store registry identity | `InitForInstall` issues an exact Store/root/baseline-bound `InstallInitializationID`; wrong/cross-Store IDs and mixed token outcomes cannot authorize rollback |
| Existing platform pattern | `cache_syscalls_{linux,darwin}.go` and `runtime_openat_{linux,darwin}.go` use build tags, raw syscalls/linkname, and no new dependency | New atomic adapters follow the same local pattern |
| Current command test build helper permits automatic toolchain selection | `core/cmd/zsh-pro/main_test.go` explicitly requests automatic mode; the Makefile has the same default | Phase 6 owns only `main_test.go` and its plan invocations: built-binary helpers and every independent gate force local Go 1.25.x, while the unchanged Makefile is invoked as `GOTOOLCHAIN=local make check` |

## Atomic API audit

The read-only planning audit inspected the active Go 1.25 tree under `go env GOROOT`, its vendored generated Unix syscall sources, installed Linux headers, and the repository's existing Darwin syscall wrappers. Execution must repeat this from `GOTOOLCHAIN=local go env GOROOT` only after local `go env GOVERSION` matches Go 1.25.x; automatic toolchain acquisition is not evidence.

Verified call model:

- Linux existing target: `renameat2(parentFD, candidate, parentFD, target, RENAME_EXCHANGE)`.
- Linux absent target: the same syscall with `RENAME_NOREPLACE`.
- Darwin existing target: descriptor-relative `renameatx_np(parentFD, candidate, parentFD, target, RENAME_SWAP)`, the at-variant of the `renamex_np` family.
- Darwin absent target: the same call with `RENAME_EXCL`.
- Linux flag values: no-replace 1, exchange 2.
- Darwin flag values: swap 2, exclusive 4.
- Darwin raw trap in the audited Go/XNU tables: 488.

Go 1.25 does not expose a uniform public helper for these exact operations on the required targets. In particular, linux/amd64 does not expose `syscall.SYS_RENAMEAT2`, and Darwin does not expose `SYS_RENAMEATX_NP` or a public extended-rename wrapper. The implementation therefore needs audited build-tagged `syscall.Syscall6` adapters; importing `golang.org/x/sys` is forbidden by the immutable-module decision.

Audited Linux `renameat2` trap table:

| GOARCH | Trap |
| --- | ---: |
| amd64 | 316 |
| 386 | 353 |
| arm | 382 |
| arm64, loong64, riscv64 | 276 |
| mips, mipsle | 4351 |
| mips64, mips64le | 5311 |
| ppc64, ppc64le | 357 |
| s390x | 347 |

The executor must repeat this audit against its actual Go 1.25 toolchain before copying the table. An absent/mismatched architecture or syscall definition is `ErrAtomicRenameUnsupported`; it is not permission to invent a trap or use a weaker namespace sequence.

## Atomicity and durability conclusions

- Exchange is atomic but is not an inode-conditional compare-and-swap. A late replacement can be exchanged; postvalidation must detect it as displacedObserved at the same `exchangePeerBasename` that held candidate bytes before exchange.
- Guarded reverse exchange is safe only while target still equals expectedCandidate and the exchange peer still equals displacedObserved. After reverse that same peer holds the candidate again; otherwise retain both names and report recovery required. There is no additional recovery-only basename or relocation.
- Existing-target exchange has no ENOENT interval: target is always the complete old or complete new file.
- Absent-target creation is a different state machine because exchange requires two names. No-replace preserves a concurrently created target.
- After no-replace succeeds, the audited adapters provide no identity-conditional inverse that atomically restores absence. Rollback therefore retains the installed target and journal as recovery-required evidence; it does not unlink after a check.
- Atomic namespace success does not prove crash durability. Retain the private transaction-directory FD through finalize/recovery. Candidate/evidence files are fsynced/closed and then the transaction directory is fsynced; the prepared-journal file is fsynced/closed and the transaction directory fsynced again before any namespace syscall; every transition repeats journal-file plus transaction-directory fsync; every directory changed by forward/reverse/no-replace is synced. Any failure retains journal/artifacts with recovery required.
- The three exact directory barriers are independently faulted: artifact-directory sync before prepared journal, prepared-journal directory sync before namespace, and transaction-directory sync for each journal transition. The first two make zero target namespace calls and leave the target unchanged; transition failure retains authenticated recovery state.
- Recovery authenticates target-parent and transaction-directory descriptor identities, journal descriptor identity/mode/uid/gid/digest/schema/transaction ID, requested-link topology, exchangePeerBasename, candidateEvidenceBasename, and journalBasename before identifying pre/post/reversed rows. It does not infer success from journal state alone and defines no additional displaced-occupant artifact.
- The identity axes are independent: expectedTarget is the bounded original source snapshot (existence, requested-link topology, regular-file type, device/inode, digest, and permission bits), while expectedCandidate comes from independently durable canonical loader/install candidate evidence. Before the one exchange target/peer match expectedTarget/expectedCandidate; afterward target matches expectedCandidate and peer/displacedObserved is compared with expectedTarget. Target uid/gid, timestamps, ACLs, xattrs, and platform flags are excluded; journal ownership is another authentication axis.
- Capability preflight first performs a side-effect-free OS/architecture/mode adapter check, then proves exchange and no-replace on authenticated random private entries on the actual target filesystem. Both precede Store initialization, cache, loader, and target effects. Unsupported capability leaves those resources unchanged; uncertain probe cleanup retains authenticated private evidence and authorizes no later effect.
- All target/artifact namespace effects in `install.go` and `install_transaction.go` are confined to one authenticated descriptor-relative helper and guarded by a static AST mutation-surface test. Cooperating zsh-pro instances addressing the same canonical target root additionally hold one real exclusive advisory lock from fresh private-journal authentication through effect, postvalidation, journal transition, and parent sync.
- That lock is a cooperation boundary, not hostile target exclusion. Arbitrary external `.zshrc` writers remain in scope: exchange may move a late occupant to the authenticated exchange-peer basename, but the protocol must never destroy/lose/relocate it and must guarded-reverse through that same peer or retain both names with recovery-required evidence.

Planning-time evidence included a local Linux 6.18/ext4 probe that exchanged two inode identities exactly and relevant landed installer tests. Darwin amd64/arm64 cross-compilation was viable in the audit, but the repository has no implementation-real required macOS workflow. Cross-build is not runtime proof: Phase 6 completion is blocked until `scripts/verify-phase06-macos-runtime.sh` runs on a native macOS host at the exact implementation SHA and attributable unedited PASS evidence is recorded in `06-MACOS-RUNTIME-EVIDENCE.md`.

## Quarantine cleanup conclusions

- Pathname snapshot plus `RemoveAll` cannot authenticate the current quarantine pathname object at recursive-cleanup time.
- Public Begin validates the Store-issued initialization ID and reserves a main-bound provisional token under the Store mutex before any ref/profile/filesystem I/O. Every exit terminalizes that same immutable record; clean no-artifact failure remains no-publication/removed evidence, while uncertain setup retains the authenticated locator/handles and recovery requirement.
- Each canonical Store root maps to one deterministic persistent sibling transaction namespace beneath its authenticated parent, current-EUID mode 0700 with a fixed no-follow regular mode-0600 lock entry. On Linux/Darwin, all cooperating zsh-pro processes acquire its descriptor-held exclusive advisory lock before authenticating private transaction state and hold it through descriptor-relative mutation, postvalidation, and parent sync. The stable sibling namespace/lock entry is not removed with a token quarantine or fresh Store-root rollback, and no authentication evidence survives unlock/reacquire.
- Begin retains authenticated parent and quarantine handles and records device/inode/mode/owner from live handles. Abort, Begin unwind, and Commit cleanup use the same recursive helper under a freshly acquired root lock.
- Every file, symlink, nested directory, and top quarantine entry is captured no-follow relative to its authenticated parent and reauthenticated immediately before descriptor-relative unlink/rmdir while the lock is held. Open/acquire failure, discarded lock descriptor, replacement before or after the final-check seam, or cleanup/durability uncertainty stops further mutation and retains recovery evidence; initializer rollback is unsafe.
- Advisory locking does not honestly exclude arbitrary non-cooperating same-UID/privileged mutation of private internals; that is outside the product boundary. There is no portable mid-interval lock-loss oracle, so a discarded/closed descriptor invalidates prior evidence and permits no subsequent mutation rather than claiming detection. Every mismatch that is observed still leaves replacements untouched and retained/recovery-required.
- Initializer rollback is an aggregate over every reserved token under the exact Store-issued initialization ID; failed Begin attempts remain visible, and any provisional/active/finalizing, published, retained, or uncertain token makes deletion unsafe.

## Store commit and controller evidence conclusions

- Public `BeginIngest(ctx, installInitializationID)` has no branch input and resolves only `refs/heads/main`. Legacy arbitrary-branch `Store.Commit` uses a private internally authorized branch-aware transaction constructor rather than widening or calling public Begin.
- Optional `ExpectedRevision` is absent if and only if `RefPresent` is false, independently of `ProfileObjectPresent`. Absent main performs zero exact-revision/tree/object/profile/parent reads, seeds an empty private index, and later uses create/zero-old with no commit parent. Present main captures one exact expected ID; Init-only, committed-empty, and operational probe failure are distinct. Commit rejects an invariant mismatch before ref-process startup.
- `CommitIngest` claims Store + initialization ID + token atomically, derives paths only from the registry, and serializes Commit, Abort, and final cleanup.
- Separate typed update-ref mutation encoders are selected only after RefPresent/ExpectedRevision validation: absent emits create and present emits update(expected). The session writes start and waits OK, writes mutation+prepare and waits prepare OK, performs backend/object publication plus all changed fanout/object-root fsyncs, and writes commit only afterward. Prepare rejection self-aborts/exits with no explicit abort write; every later pre-commit failure writes abort. Both paths keep commit-write count zero; callers cannot provide raw verbs and invalid state starts no ref process.
- Baseline probes, candidate plumbing, and the long-lived update-ref process all use the same canonical bare-store runner: inherited `GIT_*` removed, external config disabled, `GIT_WORK_TREE` absent, and worktree-mutating argv rejected before exec.
- Publication status and quarantine-cleanup status are independent durable axes. Cleanup failure after committed or clean-not-committed preserves publication truth and retains evidence.
- Only loss of the Git update-ref response after the separately staged commit write is an ambiguous observation boundary. Backend, final-object publication, fanout/root-directory sync, and cleanup failures occur before commit or on independent cleanup axes and have direct typed classifications.
- Programmatically supplied persisted SecretRefs are proven at CommitIngest level with a fixed call/effect matrix: structural/unsupported-kind rejection has Kind=0; structurally valid wrong-backend-kind and valid pass-through have Kind=1; every persisted-reference row has Retrieve/Store/Delete=0. Counters reset after Begin prove malformed rows perform zero candidate object writes, final publication, ref-process starts, and ref changes. This does not describe re-ingesting a literal that remains in ordinary source: that source parse does not contain a SecretRef and safe recapture/reporting is allowed.
- The legacy `Store.Commit` adapter has an exact committed/conflict/not-committed/recovery mapping with typed errors and value-free report behavior.
- CLI accounting separates `unmanaged_statements` from optional physical `unmanaged_source_lines`; `managed_entries + unmanaged_statements = accounted_statements = source_statements`, including a multiline fixture where line and statement counts differ. There is no generated-block DTO field.
- CommitIngest receives the complete redacted source-ordered Profile. EffectiveManaged is activation/reporting-only: `activate.Build` and the runtime emitter skip ineffective/unrepresentable entries, while Store.Read/ir.Regenerate retain imperative, parser-opaque, forced-unmanaged, and other Profile entries in source order with unmanaged Text verbatim.
- Atomic full-file exchange necessarily duplicates ordinary source bytes at the exchange peer. That peer is the only permitted additional literal-bearing file and is confined below the authenticated current-EUID mode-0700 transaction directory; candidate evidence stores digest/identity/mode only. Durable success/restoration removes authenticated artifacts, while recovery uncertainty retains the peer privately. Runtime-secret test values are injected into the temp original source only; independently authored fixtures keep a placeholder and expected bytes are expanded only in memory.
- Every failure after the one loader/install target promotion and before Commit, plus every Store noncommit, uses one `compensatePreCommitFilesystemFirst` path. Its exact order is filesystem classify/restore-or-unchanged -> AbortIngest when applicable -> loader/cache rollback -> initializer rollback. Only unchanged/restored-durable advances; recovery-required, sync-uncertain, unsafe-reverse, or absent-post-create evidence retains target/journal/Store quarantine/cache/initializer and keeps later counters zero. A committed Store outcome finalizes the already-promoted candidate; no post-commit target rewrite occurs.
- The ingest-specific failure renderer accepts only a value-free `IngestResult` and closed safe reason; stale/recovery JSON retains status flags/counts/withheld/warnings without raw errors.

## Planned new files and symbols

These do not exist in the refreshed source snapshot; the named plan creates them:

| Artifact | Planned by | Key symbols/contract |
| --- | --- | --- |
| `core/model/ingest_transaction.go` | 06-01 (create); 06-02 Task 1 (extend) | 06-01 IDs/baseline/Begin/Abort/common cleanup evidence; 06-02 commit/ref/backend/object/publication axes |
| `core/cli/install_transaction.go` | 06-03 | topology/layout snapshot, journal, `promotionOutcome`, recovery matrix |
| `core/cli/atomic_rename.go` | 06-03 | `atomicRenameMode`, `atomicRenameAt`, `ErrAtomicRenameUnsupported` |
| `core/cli/atomic_rename_linux.go` | 06-03 | audited Linux exchange/no-replace adapter |
| `core/cli/atomic_rename_darwin.go` | 06-03 | audited Darwin swap/exclusive adapter |
| `core/cli/atomic_rename_other.go` | 06-03 | fail-closed unsupported-platform adapter |
| `core/cli/atomic_rename_test.go` | 06-03 | common validation, per-call injection isolation, and Go 1.25 Linux ABI/fallback audit |
| `core/cli/atomic_rename_supported_test.go` | 06-03 | `linux || darwin` direct exchange/no-replace and injected unsupported-error tests |
| `core/cli/atomic_rename_unsupported_test.go` | 06-03 | `!linux &amp;&amp; !darwin` native unsupported and zero-call tests |
| `core/store/install_transaction_darwin_test.go` | 06-01 | exact native production-helper capability test plus cleanup tests for top-level, nested file/directory, symlink, and both replacement seams |
| `core/cli/ingest.go` | 06-04 | transactional ingest controller |
| `core/dto/ingest.go` | 06-04 | value-free human/JSON result fields |
| `core/cli/testdata/ingest/expected-installed.zshrc` | 06-05 | independently authored actual-installed template with one reviewed runtime-secret placeholder; comparison expands it only in memory and outside-marker equality is checked directly against original source |
| `scripts/verify-phase06-macos-runtime.sh` | 06-05 | native-Darwin-only exact-SHA, local-Go-1.25.x atomic plus production Store-cleanup runtime verifier |
| `06-MACOS-RUNTIME-EVIDENCE.md` | 06-05 | attributable native macOS toolchain/GOVERSION/platform/filesystem/command/output atomic, supported-capability, and six owned cleanup-row PASS record; unsupported-retained evidence is BLOCKED/nonzero |

## Execution gates derived from evidence

1. Before Go edits, create `06-EXECUTION-BASE` with exactly `557c818845f4239d5d50ecab02e37731bc117f10` plus LF; verify the commit exists and never replace it from execution-time HEAD.
2. Repeat and record exact syscall/trap/flag audit before writing `atomic_rename_*`.
3. Require every independent verifier to first reject non-local or non-Go-1.25.x execution, then exact-name-preflight every focused test selector. No Phase 6 gate may select automatic mode or download a toolchain.
4. Run landed balanced-duplicate and malformed-marker tests unchanged.
5. Run exact main-only/optional-revision tests; pre-I/O reservation/failed-Begin aggregate tests; hostile canonical bare-Git environment/rejected worktree-argv tests; real cooperating-process root-lock helper tests; deterministic per-child file/symlink/nested-directory/top substitution tests at both final-check seams; and the SecretRef call/effect matrix plus byte-exact typed create/update wire tests.
6. Run common/per-platform atomic selectors, `TestAtomicRenameLinuxGo125ABIAndFallbackSurface`, per-call parallel counters, direct adapter identity/no-replace/unsupported-before-effects tests, candidate/evidence-fsync-before-journal failure, exact `TestPromoteGuardedArtifactDirectoryFsyncFailureBeforePreparedJournal`, exact `TestPromoteGuardedPreparedJournalDirectoryFsyncFailureBeforeNamespace`, exact `TestPromotionJournalTransitionDirectoryFsyncFailureRetainsRecovery`, target-root lock tests, static mutation-surface audit, changed parent/journal/link authentication, and eight separately named existing/absent crash rows; absent post-create rollback remains recovery required.
7. Run controller full-Profile, unmanaged-projection accounting, one-promotion/finalize, Store-noncommit filesystem-first compensation, persisted-SecretRef versus source-literal, and multiline statement/line tests; never fabricate an ir.Build error.
8. Cross-compile Linux and Darwin amd64/arm64 with CGO disabled under local Go 1.25.x, then block phase completion on attributable native macOS atomic plus production Store cleanup evidence for the exact implementation SHA. Exact `TestDarwinQuarantineCleanupCapabilitySupported` and every top-level, nested file/directory, symlink, and both replacement-seam removal/sync row must PASS with `cleanup_capability: supported`. Unsupported-retained output is safety evidence only: record it as `status: BLOCKED` with a nonzero verifier exit, and do not complete Phase 6 until a supported native run passes the full contract. Record `go_toolchain: local` and exact `go_env_goversion`.
9. Prove the three linked assertions: full Store.Read/regenerate structure with authority-safe SecretRef comparison, EffectiveManaged-only activation with SecretRef resolution and inert unmanaged canaries, and pristine-vs-actual-installed `zsh -f` behavior/outside-byte/no-startup-subprocess equivalence from independently authored expected startup bytes.
10. Prove `go.mod`/`go.sum` match the literal reviewed base across HEAD/index/worktree and, under explicit local-toolchain Linux/Darwin amd64/arm64 build contexts, reject the concrete zsh provider, exact `golang.org/x/sys`, and every x/sys subpackage from cli/ir/store dependency lists; finish with `GOTOOLCHAIN=local make check`.
