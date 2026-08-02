# Phase 6: Ingest End-to-End - Context

**Gathered:** 2026-07-02
**Corrected:** 2026-08-02 (P6-011/P6-012 architecture decision)
**Status:** Ready for planning
**Mode:** Autonomous smart-discuss (`--auto`, unattended), followed by an authorized architecture correction. The correction is final: startup adoption is non-destructive, the store persists the complete redacted source-ordered Profile, and `EffectiveManaged` is only a projection.

<domain>
## Phase Boundary

Wire the already-built Phase 2/3/5 pieces into one real-file on-ramp: `zsh-pro` reads a real `~/.zshrc`, statically parses/classifies it, builds a complete source-ordered `model.Profile`, applies Store secret exclusion, commits the complete redacted Profile to `main`, and installs only the canonical Phase 5 loader region. Every eligible preexisting byte outside exact zsh-pro marker regions stays at its original location in its original order. First adoption appends the loader region; installed/re-ingest replaces or collapses only exact marker regions according to the landed Phase 5 topology rules. No ingest path rewrites ordinary startup source into an unmanaged complement and there is no final post-commit target rewrite.

Five deliverables remain in scope:

1. **End-to-end ingest orchestration + `ingest` CLI verb** — read the default or explicit path, call `provider.Parse` → `ir.Build`, and commit through the authenticated main-only Store transaction.
2. **Complete persisted Profile + logical projections** — commit managed and unmanaged entries in their complete redacted source order. `EffectiveManaged` selects activation/reporting only. Report `managed_entries` and `unmanaged_statements` so their sum accounts for source statements; physical startup bytes are proved independently.
3. **WithheldReport surfacing** — surface only withheld name/line metadata. Store secret exclusion runs before serialization, so literal secret values occur in no Git object.
4. **Non-destructive loader adoption + append warning** — use the Phase 5 installer once, preserve all outside-marker bytes exactly, and warn when ordinary nonblank/noncomment bytes remain after the canonical END marker without moving or deleting them.
5. **Three linked correctness assertions** — (a) `Store.Read(main)` returns the full redacted ordered Profile and `ir.Regenerate` preserves non-secret order, unmanaged `Text`, and managed semantics with an authority-safe SecretRef comparator; (b) `activate.Build`/the runtime emitter applies only `EffectiveManaged` entries, resolves SecretRefs, and leaves unmanaged execution canaries inert; (c) the built binary compares pristine source with the actual installed `.zshrc` under isolated `zsh -f`, including an order-sensitive managed-definition → imperative-use fixture, a literal-secret behavior check that never prints the value, outside-marker byte equality, allowed loader-symbol differences only, and zero startup subprocesses.

**In scope:** the ingest verb and value-free DTO; authenticated prepare/Begin/install/Commit/finalize orchestration; complete redacted Profile persistence; deterministic rerun behavior; logical managed/unmanaged accounting; WithheldReport rendering; the read-only post-END warning; actual-installed-startup and activation/regeneration proof; one concrete Store at the composition root.

**Not in scope:** a separate committed master file; a generated physical unmanaged complement; removal/reordering of ordinary startup source; a second/final post-commit `.zshrc` promotion; new classification rules; execution of input during ingest; parser/IR/store reinvention; multi-file config graphs; other shells; auto-activate on `cd`; remote sharing/trust.

</domain>

<decisions>
## Implementation Decisions

### Area 1 — `ingest` verb: interface, arguments, output

- **D-01: The verb is `ingest`; invocation is `zsh-pro ingest [path] [--json]`, defaulting to `~/.zshrc`.** At most one positional path is accepted; unknown flags or a second path are usage errors. `util.ExpandHome` runs once at the CLI read boundary, never in IR/store code.
- **D-02: Dispatch remains inside `CLI.Run` without changing its public signature.** Human/JSON output follows existing typed exit conventions: 0 complete or warning-only success, 1 runtime/conflict/recovery failure, 2 usage. JSON emits exactly one object on success or failure.
- **D-03: Accounting is statement-based and projection-only.** The DTO/user surface uses `managed_entries`, `unmanaged_statements`, `source_statements`, and `accounted_statements`; the invariant is `managed_entries + unmanaged_statements = accounted_statements = source_statements`. If useful, `unmanaged_source_lines` reports physical source lines separately, but physical lines never substitute for statement accounting. Do not expose a generated-block line field for an artifact that does not exist.
- **D-04: Results are value-free and deterministic.** Include commit/install/recovery state, the projection counts, withheld name/line metadata, and stable warnings. Do not put raw zsh text, paths, errors, Git IDs, backend identifiers, or secret values on the wire.

### Area 2 — Complete Profile persistence and non-destructive startup adoption

- **D-05: `EffectiveManaged()` is the activation/reporting projection, not a persistence filter.** `Store.CommitIngest` receives the complete source-ordered Profile after Store-owned redaction, including managed and unmanaged entries. `Store.Read(main)` must return that full Profile. `activate.Build` already ignores entries for which `!EffectiveManaged()` or `!Representable()`; Phase 6 pins that contract with unmanaged execution canaries and managed activation assertions.
- **D-06: “Master” is only a logical unmanaged projection.** There is no physical master block, generated complement, or separate committed master file. Prefer user-facing `unmanaged_statements`/`unmanaged_source_lines`. Ordinary `.zshrc` bytes remain exactly where the user authored them. Because atomic full-file exchange necessarily stages those bytes, the only permitted additional literal-bearing copy is the authenticated transaction peer below a current-EUID mode-0700 directory; it is never a profile/cache/output/test artifact, is removed after durable finalize or restore, and is retained only when deleting recovery evidence would be unsafe.
- **D-07: Reuse the landed Phase 5 installer exactly once.** Exact physical marker lines are `# >>> zsh-pro >>>` and `# <<< zsh-pro <<<`. No markers append the canonical loader region using landed separator rules; one balanced region is replaced; multiple balanced regions collapse to one while all surrounding/intervening ordinary bytes remain byte-identical; malformed topology fails before effects. Ingest prepares an independent install candidate from the original snapshot, promotes loader then `.zshrc` once, and never performs a later target rewrite.

### Area 3 — Secret exclusion and reruns

- **D-08: Ingest surfaces Store-owned secret exclusion.** Literal secret values are captured/redacted before Profile serialization and occur in no reachable, unreachable, packed, or retained-quarantine Git object. Output contains only withheld name and source line. Outside the installed target, a literal may exist only in the descriptor-authenticated transaction peer required by D-12; candidate evidence stores identity/digest/mode rather than source bytes. Successful finalize/restoration removes that peer durably, while recovery-required outcomes retain it privately and report recovery without printing its content.
- **D-09: Distinguish persisted SecretRef pass-through from source-literal re-ingest.** A programmatically supplied persisted `SecretRef` is validated structurally, calls backend `Kind` only when structurally valid, and never calls Retrieve/Store/Delete. Re-ingesting a literal that remains in the user's ordinary source may safely recapture and report it; tests must not claim the source parser saw a `SecretRef` or require zero Store on that source-literal rerun. Already-dynamic secret expressions remain verbatim and unreported.
- **D-10: Use the authenticated main-only transaction.** Begin against the exact baseline revision, commit the complete redacted Profile with fixed message `zsh-pro: ingest baseline`, and trust only typed commit/abort evidence. Store noncommit never becomes success and secret-handling failure stays fail-closed.

### Area 4 — Post-END warning and filesystem transaction

- **D-11: Post-END detection is read-only.** After canonicalizing exact marker regions, ordinary nonblank/noncomment content after END produces one stable warning and remains byte-identical at its original position. It is still part of the complete source Profile when eligible; warning classification must not silently remove it from persistence/accounting.
- **D-12: One journaled loader/install candidate owns the target transition.** Capability preflight precedes effects. `expectedTarget` is the original bounded snapshot and `expectedCandidate` comes only from independently durable digest/identity/mode evidence. The full-byte exchange peer is confined below the authenticated current-EUID mode-0700 transaction directory and opened only descriptor-relatively; no log, journal, evidence record, cache, or Store object contains its source bytes. Promote loader then target once before Store commit. On pre-commit or Store noncommit, filesystem restore/retain classification runs before `AbortIngest`, loader/cache rollback, and initializer rollback. A committed Store outcome durably removes authenticated transaction artifacts; recovery uncertainty retains them rather than risking data loss. No post-commit target promotion exists.

### Area 5 — Round-trip, activation, and actual-startup proof

- **D-13: The round-trip proof is three linked assertions, none substitutable for another.** First, `Store.Read(main)` returns the full redacted ordered Profile and `ir.Regenerate` preserves every non-secret entry's order, unmanaged `Text` verbatim, and managed semantics. A bounded SecretRef placeholder comparator may use the authoritative committed entry to validate the inert kind/key placeholder, but it must not forge authority or pretend reparsed source reconstructs `Secret`, `ValueMode`, `RuntimeValue`, or `StartLine`.
- **D-14: Activation and actual-startup fixtures are independently authored.** Activation tests prove only EffectiveManaged/Representable entries emit and SecretRefs resolve; unmanaged execution canaries remain inert. Built-binary E2E sources the pristine original and the actual installed `.zshrc` in isolated `zsh -f` processes, compares observable variables/order/aliases/functions/options/PATH, includes managed-definition → imperative-use ordering and a literal-secret equality assertion without printing the value, permits differences only for exact zsh-pro-owned loader symbols, proves zero startup subprocess, and compares every outside-marker byte exactly. Authored source/expected fixtures contain one reviewed non-secret placeholder; the test generates the literal at runtime, writes it only into the temp original source, and expands the independently authored expected template only in memory for comparison. Production code never generates expected bytes and no second literal-bearing expected file is written.
- **D-15: The concrete Store is injected once at `core/cmd/zsh-pro/main.go`.** The same pointer owns initialization ID issuance, main-only Begin/Commit/Abort, and existing store-backed verbs. `core/cli`, `core/ir`, and `core/store` do not import the concrete zsh provider.
- **D-16: Extend landed seams; do not create parallel ones.** Reuse Phase 5's installer/store injection and Phase 6's authenticated transaction types. No ingest-only writer, second Store, committed master file, or alternative renderer is introduced.

### Claude's Discretion

- Exact Go identifiers/file placement, provided the complete Profile reaches Store commit and the one-promotion order is explicit.
- Exact safe human wording and DTO field casing, provided `unmanaged_statements`/optional `unmanaged_source_lines` replace physical-master terminology and values/raw internals never cross output.
- One comprehensive fixture or a small family, provided the regeneration, activation, and actual-installed-startup assertions remain independently falsifiable and linked.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `core/ir/build.go` builds the ordered `model.Profile`; Phase 6 commits its complete redacted form rather than filtering it for persistence.
- `core/ir/regen.go` preserves unmanaged `Text` and regenerates managed entries; the round-trip assertion consumes the full Store-read Profile.
- `model.Entry.EffectiveManaged()` is the activation/reporting projection. It is not authorization to delete an entry from persistence or startup source.
- `activate.Build` already skips `!EffectiveManaged()` and `!Representable()` entries; Phase 6 adds direct activation/runtime proof rather than modifying this partition.
- Store secret exclusion and `WithheldReport` remain authoritative; literal exclusion runs before serialization.
- The landed Phase 5 installer owns the exact markers, duplicate collapse, outside-byte preservation, loader-before-target order, and fail-open/no-subprocess loader contract.
- `core/cmd/zsh-pro/main.go` remains the sole concrete composition root.

### Integration Points

- **Input:** exact original target snapshot → static `provider.Parse` → `ir.Build` complete Profile.
- **Filesystem:** original snapshot + landed canonical loader region → independent install candidate → one authenticated loader/target promotion.
- **Store:** exact Begin baseline → `CommitIngest` of the complete Profile after Store redaction → typed committed/noncommit/recovery result.
- **Output:** projection counts, value-free withheld metadata, and stable warnings.
- **Proof:** Store-read/regenerate structure, activation/emitter projection, and pristine-vs-actual-installed startup behavior/outside-byte equality.

</code_context>

<specifics>
## Specific Ideas

- Zero drop is proved logically by full Profile persistence and `managed_entries + unmanaged_statements = source_statements`; physical startup byte preservation is a separate exact-byte assertion.
- The original local `.zshrc` remains the startup authority for ordinary user code. Installing zsh-pro adds or canonicalizes only its exact marker region.
- The order-sensitive fixture must fail under the rejected complement architecture: a managed definition appears before an imperative use, and both pristine and installed startup must observe the same result.
- Ingest performs no source execution. The only source execution occurs later in isolated oracle subprocesses.

</specifics>

<deferred>
## Deferred Ideas

- A CLI to edit `ManagedOverride` values.
- Multi-file config graphs and sourced-fragment ingestion.
- Auto-activation on `cd`, sharing/trust, and other shells.
- Reinterpreting post-END warning content as a separate physical master artifact; the content remains ordinary source and is already represented in the full Profile.

</deferred>

---

*Phase: 06-ingest-end-to-end*
*Context corrected: 2026-08-02 — complete Profile persistence, non-destructive startup adoption, one filesystem promotion, and three-linked-assertion proof are locked.*
*Next step: execute 06-01 through 06-05 after plan-structure/index review and the P6-011/P6-012 post-document review gate.*
