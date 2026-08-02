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
| Store initialization placement | `runInstallWithStoreInitialization` calls initializer before target/cache preparation, then loader promotion before `.zshrc` promotion | Preserve shipped order and make shared-root/rollback ordering explicit |
| Marker constants are landed | `core/cli/install.go` defines exact START/END constants | Historical claims that Phase 5 has not landed are stale |
| Balanced duplicates are repaired | `TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent`; `replaceManagedBlock` emits one replacement and removes later managed regions while preserving ordinary bytes | Multiple balanced regions are valid repair topology, not generic malformed/append topology |
| Malformed topology is rejected | Existing tests cover orphan END, unterminated START, nested START, and stray/interleaved END | Ingest must share the parser and fail before side effects |
| Current target promotion is pathname based | `preparedWrite.promote` calls `os.Rename`; rollback separately renames/removes | Replace with atomic exchange/exclusive creation and typed outcomes |
| Current fresh-root cleanup is pathname recursive | `core/store/install_transaction.go` re-snapshots then calls `os.RemoveAll` | Abort quarantine cleanup requires retained handles + device/inode, descriptor-only recursion, and no final-check-then-pathname removal |
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

- Exchange is atomic but is not an inode-conditional compare-and-swap. A late replacement can be exchanged; postvalidation must detect it at the recovery basename.
- Guarded reverse exchange is safe only while target still equals candidate evidence and recovery still equals the displaced identity. Otherwise retain both and report recovery required.
- Existing-target exchange has no ENOENT interval: target is always the complete old or complete new file.
- Absent-target creation is a different state machine because exchange requires two names. No-replace preserves a concurrently created target.
- After no-replace succeeds, the audited adapters provide no identity-conditional inverse that atomically restores absence. Rollback therefore retains the installed target and journal as recovery-required evidence; it does not unlink after a check.
- Atomic namespace success does not prove crash durability. The journal must be durable before the syscall, the parent directory must be fsynced after it, and a failed/uncertain sync retains journal/artifacts.
- Recovery authenticates parent device/inode, journal descriptor identity/mode/uid/gid/digest/schema/transaction ID, requested-link topology, and basenames before identifying pre/post rows. It does not infer success from journal state alone.
- The target snapshot itself is narrower: existence, requested-link topology, regular-file type, device/inode, digest, and permission bits. Target uid/gid, timestamps, ACLs, xattrs, and platform flags are excluded; journal ownership is an independent authentication axis.
- All target/artifact namespace effects in `install.go` and `install_transaction.go` are confined to one authenticated descriptor-relative helper and guarded by a static AST mutation-surface test.

Planning-time evidence included a local Linux 6.18/ext4 probe that exchanged two inode identities exactly and relevant landed installer tests. Darwin amd64/arm64 cross-compilation was viable in the audit, but the repository has no implementation-real required macOS workflow. Cross-build is not runtime proof: Phase 6 completion is blocked until `scripts/verify-phase06-macos-runtime.sh` runs on a native macOS host at the exact implementation SHA and attributable unedited PASS evidence is recorded in `06-MACOS-RUNTIME-EVIDENCE.md`.

## Quarantine cleanup conclusions

- Pathname snapshot plus `RemoveAll` cannot authenticate the current quarantine pathname object at recursive-cleanup time.
- Public Begin validates the Store-issued initialization ID and reserves a main-bound provisional token under the Store mutex before any ref/profile/filesystem I/O. Every exit terminalizes that same immutable record; clean no-artifact failure remains no-publication/removed evidence, while uncertain setup retains the authenticated locator/handles and recovery requirement.
- Begin retains authenticated parent and quarantine handles and records device/inode/mode/owner from live handles. Abort and Begin unwind use the same recursive helper.
- Every file, symlink, nested directory, and top quarantine entry is captured no-follow relative to its authenticated parent and independently identity-bound. A final identity check followed by pathname unlink/rmdir is not accepted because substitution can still win afterward.
- Directory-entry removal is allowed only through a proven atomic capability that removes the exact authenticated opened child. If unavailable, or replacement occurs before or after the final-check seam at any depth, the replacement stays untouched and remaining tree/recovery evidence is retained; initializer rollback is unsafe.
- Initializer rollback is an aggregate over every reserved token under the exact Store-issued initialization ID; failed Begin attempts remain visible, and any provisional/active/finalizing, published, retained, or uncertain token makes deletion unsafe.

## Store commit and controller evidence conclusions

- Public `BeginIngest(ctx, installInitializationID)` has no branch input and resolves only `refs/heads/main`. Legacy arbitrary-branch `Store.Commit` uses a private internally authorized branch-aware transaction constructor rather than widening or calling public Begin.
- Optional `ExpectedRevision` is absent if and only if `RefPresent` is false, independently of `ProfileObjectPresent`. Absent main performs zero exact-revision/tree/object/profile/parent reads, seeds an empty private index, and later uses create/zero-old with no commit parent. Present main captures one exact expected ID; Init-only, committed-empty, and operational probe failure are distinct. Commit rejects an invariant mismatch before ref-process startup.
- `CommitIngest` claims Store + initialization ID + token atomically, derives paths only from the registry, and serializes Commit, Abort, and final cleanup.
- Baseline probes, candidate plumbing, and the long-lived update-ref process all use the same canonical bare-store runner: inherited `GIT_*` removed, external config disabled, `GIT_WORK_TREE` absent, and worktree-mutating argv rejected before exec.
- Publication status and quarantine-cleanup status are independent durable axes. Cleanup failure after committed or clean-not-committed preserves publication truth and retains evidence.
- Only loss of the Git update-ref commit response is an ambiguous observation boundary. Backend, final-object publication, directory-sync, and cleanup failures have direct typed classifications.
- Valid persisted SecretRefs are proven at CommitIngest level: exact entry pass-through, Kind once, Retrieve/Store/Delete zero, and no second withheld record; malformed shapes fail before ref processing.
- The legacy `Store.Commit` adapter has an exact committed/conflict/not-committed/recovery mapping with typed errors and value-free report behavior.
- CLI accounting separates `master_statements` from physical `master_block_lines`; `managed_entries + master_statements = accounted_statements = source_statements`, including a multiline fixture where line and statement counts differ.
- Every failure after initial promotion and before Commit uses one `abortAndCompensatePreCommit` path. `ir.Build` is total, so the real injected seams are parse, duplicate identity, accounting, candidate preparation, and snapshot validation.
- The ingest-specific failure renderer accepts only a value-free `IngestResult` and closed safe reason; stale/recovery JSON retains status flags/counts/withheld/warnings without raw errors.

## Planned new files and symbols

These do not exist in the refreshed source snapshot; the named plan creates them:

| Artifact | Planned by | Key symbols/contract |
| --- | --- | --- |
| `core/model/ingest_transaction.go` | 06-01 | main-only baseline, optional ExpectedRevision invariant, and typed begin/commit/abort/recovery evidence |
| `core/cli/install_transaction.go` | 06-03 | topology/layout snapshot, journal, `promotionOutcome`, recovery matrix |
| `core/cli/atomic_rename.go` | 06-03 | `atomicRenameMode`, `atomicRenameAt`, `ErrAtomicRenameUnsupported` |
| `core/cli/atomic_rename_linux.go` | 06-03 | audited Linux exchange/no-replace adapter |
| `core/cli/atomic_rename_darwin.go` | 06-03 | audited Darwin swap/exclusive adapter |
| `core/cli/atomic_rename_other.go` | 06-03 | fail-closed unsupported-platform adapter |
| `core/cli/atomic_rename_test.go` | 06-03 | direct platform/identity/no-replace/unsupported tests |
| `core/cli/ingest.go` | 06-04 | transactional ingest controller |
| `core/dto/ingest.go` | 06-04 | value-free human/JSON result fields |
| `scripts/verify-phase06-macos-runtime.sh` | 06-05 | native-Darwin-only exact-SHA, local-Go-1.25.x exchange/no-replace runtime verifier |
| `06-MACOS-RUNTIME-EVIDENCE.md` | 06-05 | attributable native macOS toolchain/GOVERSION/platform/filesystem/command/output PASS record |

## Execution gates derived from evidence

1. Before Go edits, create `06-EXECUTION-BASE` with exactly `557c818845f4239d5d50ecab02e37731bc117f10` plus LF; verify the commit exists and never replace it from execution-time HEAD.
2. Repeat and record exact syscall/trap/flag audit before writing `atomic_rename_*`.
3. Require every independent verifier to first reject non-local or non-Go-1.25.x execution, then exact-name-preflight every focused test selector. No Phase 6 gate may select automatic mode or download a toolchain.
4. Run landed balanced-duplicate and malformed-marker tests unchanged.
5. Run exact main-only/optional-revision tests; pre-I/O reservation/failed-Begin aggregate tests; hostile canonical bare-Git environment and rejected worktree-argv tests; and deterministic per-child file/symlink/nested-directory/top substitution tests at both final-check seams. No record is erased and no pathname deletion closes the last race.
6. Run direct adapter identity/no-replace/unsupported-before-effects tests, static mutation-surface audit, changed parent/journal/link authentication, and eight separately named existing/absent crash rows; absent post-create rollback remains recovery required.
7. Run controller accounting mismatch and multiline statement/line tests plus the parse/duplicate/candidate-preparation common-compensation matrix; never fabricate an ir.Build error.
8. Cross-compile Linux and Darwin amd64/arm64 with CGO disabled under local Go 1.25.x, then block phase completion on attributable native macOS exchange/no-replace/late-target evidence for the exact implementation SHA recording `go_toolchain: local` and exact `go_env_goversion`.
9. Prove `go.mod`/`go.sum` match the literal reviewed base across HEAD/index/worktree and, under explicit local-toolchain Linux/Darwin amd64/arm64 build contexts, reject the concrete zsh provider, exact `golang.org/x/sys`, and every x/sys subpackage from cli/ir/store dependency lists; finish with `GOTOOLCHAIN=local make check`.
