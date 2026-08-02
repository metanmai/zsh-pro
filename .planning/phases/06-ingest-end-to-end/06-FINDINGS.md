# Phase 6 Findings Ledger

**Updated:** 2026-08-02
**Scope:** CodeRabbit, plan-checker, and source-audit findings for Phase 6

Status meanings:

- `OPEN` — actionable and not yet fixed.
- `FIXED` — a concrete change exists but has not passed the required independent verification.
- `VERIFIED` — the fix passed its focused checks and the relevant review gate.
- `DISMISSED WITH EVIDENCE` — non-actionable, duplicate, speculative, or out of scope, with exact evidence recorded.

A `VERIFIED` finding is reopened only when new code, test, runtime, or review evidence contradicts it.

| ID | Finding | Status | Evidence |
| --- | --- | --- | --- |
| P6-001 | Master derivation positive-selected recognized imperative forms and could drop parser-opaque or gap bytes. | VERIFIED | Commit `86b56b1` makes exact byte extents authoritative and retains imperative, opaque, comment, and gap bytes; plan structure, stale-term, and CodeRabbit rerun gates passed. This routing design is superseded only where P6-011/P6-012 explicitly revise physical startup composition. |
| P6-002 | Pre-commit compensation could abort Store state before classifying/restoring the filesystem. | VERIFIED | Commit `86b56b1` fixes the order to filesystem classify/restore-or-retain, then AbortIngest, loader/cache rollback, initializer rollback; later gates are disarmed on uncertainty. |
| P6-003 | Exchange recovery used ambiguous candidate/recovery basenames. | VERIFIED | Commit `86b56b1` defines one `exchangePeerBasename`, one independent `candidateEvidenceBasename`, and no second displaced-occupant artifact. |
| P6-004 | Atomic capability failure could occur after Store/cache/loader/target effects. | VERIFIED | Commit `86b56b1` requires a side-effect-free adapter check and authenticated same-filesystem exchange/no-replace probe before product effects. |
| P6-005 | Expected target identity and candidate evidence were conflated. | VERIFIED | Commit `86b56b1` separates immutable `expectedTarget` from independent `expectedCandidate` with distinct pre/post comparisons and tests. |
| P6-006 | 06-02 Task 1 claimed full model/store tests and vet but ran only focused selectors. | VERIFIED | Commit `bbd98dd` adds `go test ./core/model ./core/store` and `go vet ./core/model ./core/store`; exact selector parity and all five plan-structure checks passed. |
| P6-007 | `git update-ref --stdin` could write `commit` before backend/object publication durability. | VERIFIED | Commit `bbd98dd` stages start/mutation/prepare, waits for acknowledgements, completes backend publication plus fanout/root fsync, then writes commit. Focused ordering/failure selectors and an independent ledger audit passed. |
| P6-008 | Candidate, evidence, and journal file fsyncs omitted owning transaction-directory durability. | VERIFIED | Commit `bbd98dd` adds retained transaction-directory identity, file-then-directory barriers, per-transition directory fsync, all changed-directory syncs, and three exact failure selectors. Independent selector/evidence parity audit passed. |
| P6-009 | Unsupported native Darwin cleanup could be recorded as phase-complete PASS. | VERIFIED | Commit `bbd98dd` requires `TestDarwinQuarantineCleanupCapabilitySupported` plus every owned removal/sync row; unsupported-retained emits BLOCKED/nonzero and cannot complete Phase 6. Seven-selector parity and evidence wording passed independent audit. |
| P6-010 | A prepare mismatch was incorrectly required to receive an explicit `abort` after Git had already aborted/exited. | VERIFIED | Commit `bbd98dd` distinguishes Git's automatic prepare-rejection abort/exit from explicit abort after successful prepare; stale scans and independent re-review returned PASS. |
| P6-011 | Committing only EffectiveManaged entries makes `Store.Read(main) -> ir.Regenerate` unable to reproduce full source order. | FIXED | `06-CONTEXT.md` D-05/D-13 and `06-SPEC.md` Requirements 1/5 now require the complete redacted source-ordered Profile in main, projection-only EffectiveManaged, an authority-safe SecretRef comparator, and direct activation/emitter inertness proof. `06-04-PLAN.md` Task 3 commits the complete Profile; `06-05-PLAN.md` Task 2 links Store-read/regenerate with activation tests. Independent review remains required before VERIFIED. |
| P6-012 | Replacing `.zshrc` with the unmanaged complement plus a loader-only EOF block can run dependent imperative code before removed managed declarations. | FIXED | `06-CONTEXT.md` D-06/D-07/D-12/D-14 and `06-SPEC.md` Requirements 3/5 now preserve all ordinary outside-marker bytes, prepare one canonical loader/install candidate, prohibit post-commit target rewrite, and require built-binary pristine-vs-actual-installed `zsh -f` proof with an order-sensitive definition/use fixture. `06-03-PLAN.md` owns the one promotion; `06-05-PLAN.md` replaces the expected complement artifact with independently authored `expected-installed.zshrc`. Independent review remains required before VERIFIED. |
| P6-013 | The no-local-secret-copy claim contradicted the required atomic full-file exchange and a literal-bearing expected-installed fixture would create another copy itself. | FIXED | The contract now names the unavoidable secret-bearing transaction peer as a bounded exception: it is confined below an authenticated current-EUID mode-0700 directory, never serialized or emitted, removed after durable finalize/restore, and retained only when recovery safety forbids deletion. Test fixtures contain a reviewed placeholder; a runtime-generated literal is written only to the temp original source, and expected bytes are expanded only in memory. Independent review remains required before VERIFIED. |

## Current convergence gate

P6-011 through P6-013 are document-fixed but must pass the independent post-document review gate before becoming `VERIFIED` and before Phase 6 execution. Validate all plan structures and the phase plan index, run one pre-execution CodeRabbit pass, fix or source-dismiss every actionable result, then mark only independently confirmed findings VERIFIED.
