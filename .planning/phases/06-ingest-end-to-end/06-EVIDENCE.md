# Phase 6 Current-Source Evidence

**Refreshed:** 2026-08-02
**Status:** Planning evidence; executor must repeat the syscall audit before implementation
**Authority order:** Current source/read-first files > completed plan summaries > historical CONTEXT code-line commentary.

## Landed source snapshot

| Claim | Current evidence | Planning consequence |
| --- | --- | --- |
| Public dispatcher signature | `core/cli/cli.go`: `func (c *CLI) Run(args []string, stdout, stderr io.Writer) int` | 06-04 adds ingest dispatch with no signature/context change |
| Current injection seam | `cli.New(p, s, e)` and `NewWithStoreInitializer(p, s, e, initializer)` | Extend the landed seam; one concrete store pointer at `main.go` |
| Store-neutral CLI boundary | `core/cli/store.go` defines `Store`, `StoreInitialization`, `StoreInitializer` | New transaction evidence stays in `core/model`; no concrete-store import |
| Store initialization placement | `runInstallWithStoreInitialization` calls initializer before target/cache preparation, then loader promotion before `.zshrc` promotion | Preserve shipped order and make shared-root/rollback ordering explicit |
| Marker constants are landed | `core/cli/install.go` defines exact START/END constants | Historical claims that Phase 5 has not landed are stale |
| Balanced duplicates are repaired | `TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent`; `replaceManagedBlock` emits one replacement and removes later managed regions while preserving ordinary bytes | Multiple balanced regions are valid repair topology, not generic malformed/append topology |
| Malformed topology is rejected | Existing tests cover orphan END, unterminated START, nested START, and stray/interleaved END | Ingest must share the parser and fail before side effects |
| Current target promotion is pathname based | `preparedWrite.promote` calls `os.Rename`; rollback separately renames/removes | Replace with atomic exchange/exclusive creation and typed outcomes |
| Current fresh-root cleanup is pathname recursive | `core/store/install_transaction.go` re-snapshots then calls `os.RemoveAll` | Abort quarantine cleanup requires retained handles + device/inode and descriptor-only recursion |
| Existing platform pattern | `cache_syscalls_{linux,darwin}.go` and `runtime_openat_{linux,darwin}.go` use build tags, raw syscalls/linkname, and no new dependency | New atomic adapters follow the same local pattern |

## Atomic API audit

The read-only planning audit inspected the active Go 1.25 tree under `go env GOROOT`, its vendored generated Unix syscall sources, installed Linux headers, and the repository's existing Darwin syscall wrappers.

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
- Atomic namespace success does not prove crash durability. The journal must be durable before the syscall, the parent directory must be fsynced after it, and a failed/uncertain sync retains journal/artifacts.
- Recovery identifies pre/post rows from recorded identities and basenames; it does not infer success from journal state alone.

Planning-time evidence included a local Linux 6.18/ext4 probe that exchanged two inode identities exactly and relevant landed installer tests. Darwin amd64/arm64 cross-compilation was viable in the audit; direct Darwin runtime tests remain the tagged execution gate on a macOS runner.

## Quarantine cleanup conclusions

- Pathname snapshot plus `RemoveAll` cannot authenticate the current quarantine pathname object at recursive-cleanup time.
- Begin must retain the authenticated parent and quarantine handles and record device/inode/mode/owner from the live quarantine handle.
- Abort must compare parent-relative no-follow basename identity to the retained handle before recursion, invoke the deterministic race seam, repeat the identity check, recurse only through the retained handle, recheck before non-recursive directory removal, then fsync the parent.
- Replacement before cleanup or after the first identity check leaves the replacement bytes/inode/mode/descendants untouched and returns recovery required with initializer rollback unsafe.

## Planned new files and symbols

These do not exist in the refreshed source snapshot; the named plan creates them:

| Artifact | Planned by | Key symbols/contract |
| --- | --- | --- |
| `core/model/ingest_transaction.go` | 06-01 | exact baseline + typed commit/abort/recovery evidence |
| `core/cli/install_transaction.go` | 06-03 | topology/layout snapshot, journal, `promotionOutcome`, recovery matrix |
| `core/cli/atomic_rename.go` | 06-03 | `atomicRenameMode`, `atomicRenameAt`, `ErrAtomicRenameUnsupported` |
| `core/cli/atomic_rename_linux.go` | 06-03 | audited Linux exchange/no-replace adapter |
| `core/cli/atomic_rename_darwin.go` | 06-03 | audited Darwin swap/exclusive adapter |
| `core/cli/atomic_rename_other.go` | 06-03 | fail-closed unsupported-platform adapter |
| `core/cli/atomic_rename_test.go` | 06-03 | direct platform/identity/no-replace/unsupported tests |
| `core/cli/ingest.go` | 06-04 | transactional ingest controller |
| `core/dto/ingest.go` | 06-04 | value-free human/JSON result fields |

## Execution gates derived from evidence

1. Capture immutable `06-EXECUTION-BASE` before Go edits.
2. Repeat and record exact syscall/trap/flag audit before writing `atomic_rename_*`.
3. Require exact-name preflight for every focused test selector.
4. Run landed balanced-duplicate and malformed-marker tests unchanged.
5. Run deterministic quarantine replacement before/after-identity-check tests.
6. Run direct adapter identity/no-replace/unsupported tests and existing/absent crash matrices.
7. Cross-compile Linux and Darwin amd64/arm64 with CGO disabled; execute tagged runtime tests on their native hosts.
8. Prove `go.mod`/`go.sum` and forbidden dependency/layering surfaces are unchanged from the execution base.
