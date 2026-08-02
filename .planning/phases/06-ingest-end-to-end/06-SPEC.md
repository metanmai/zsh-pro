# Phase 6: Ingest End-to-End — Specification

**Created:** 2026-07-02
**Corrected:** 2026-08-02 (P6-011/P6-012)
**Ambiguity score:** 0.16 (gate: ≤ 0.20)
**Requirements:** 5 locked
**Mode:** `--auto`, followed by an authorized architecture correction grounded in the landed Phase 5 installer and Phase 2/3/4 code contracts.

## Goal

`zsh-pro` ingests a real `~/.zshrc` end-to-end without destructively rewriting the user's ordinary startup source. It statically parses/classifies the original source, commits the complete redacted source-ordered `model.Profile` to `main`, installs or canonicalizes only the exact Phase 5 loader marker region, surfaces secret withholding and post-END warnings, and proves three linked properties: the stored Profile round-trips, activation applies only its effective managed projection, and the actual installed startup remains behavior-equivalent to the pristine source.

## Background

- **IR:** `ir.Build` creates an ordered Profile. `ir.Regenerate` emits every Profile entry in order, preserving unmanaged `Text` and regenerating managed semantics. `EffectiveManaged()` is a projection used by activation/reporting; it is not a persistence filter.
- **Activation:** `activate.Build` already ignores `!EffectiveManaged()` and `!Representable()` entries. Phase 6 must prove unmanaged entries are inert and SecretRefs resolve on the managed path.
- **Store:** authenticated ingest transactions commit a `model.Profile`; Store-owned exclusion captures literal secrets and redacts them before serialization, returning value-free `WithheldReport` metadata.
- **Installer:** landed Phase 5 code appends one canonical marker region on an uninstalled file, replaces one exact balanced region, collapses multiple balanced regions while preserving all ordinary bytes, rejects malformed topology, promotes the loader before `.zshrc`, and sources no subprocess on startup.

The Phase 6 delta is orchestration and proof. It must not introduce a generated physical master/complement, a separate committed master file, or a second post-commit target promotion.

## Requirements

1. **Complete end-to-end ingest to baseline**: Ingesting a real `~/.zshrc` commits the complete redacted source-ordered Profile to `main`; managed and unmanaged source statements are never silently dropped or reordered.
   - Current: No single CLI flow connects original-source parsing, the authenticated main transaction, canonical loader installation, complete Profile commit, and result reporting.
   - Target: Read the exact original target snapshot, run `provider.Parse` → `ir.Build`, install the canonical loader region once, and pass the complete Profile (managed and unmanaged entries) to `CommitIngest`. `EffectiveManaged` selects activation/reporting only. Report `managed_entries` and `unmanaged_statements` such that `managed_entries + unmanaged_statements = accounted_statements = source_statements`; optional `unmanaged_source_lines` stays a separate physical-line measure.
   - Acceptance: `Store.Read(main)` returns the expected full ordered redacted Profile, including imperative, parser-opaque, forced-unmanaged, managed, comments/gaps represented by the existing IR contract, and SecretRef-redacted entries. An independently authored provenance table proves statement accounting and source order. Activation tests separately prove unmanaged entries are inert.

2. **Secret exclusion + truthful withheld/pass-through behavior**: Literal secrets are absent from every Git object and output; the user sees only what was withheld, while persisted references and source literals are not conflated.
   - Current: Store exclusion exists, but there is no ingest UI/E2E proof across final and retained object databases.
   - Target: Store exclusion runs before serialization. A literal secret is captured/redacted and reported by name/line; its value appears in no reachable, unreachable, packed, or retained-quarantine Git object. Atomic full-file exchange may place the literal only in its authenticated transaction peer below a current-EUID mode-0700 directory; identity evidence/journal/cache/output contain no source bytes. Durable finalize/restoration removes that peer, while recovery-required preserves it privately rather than destroying evidence. A programmatically supplied persisted `SecretRef` performs structural validation and Kind-only pass-through with zero Retrieve/Store/Delete. Re-ingesting a literal still present in ordinary source may recapture/report it and must not be described as parsing a SecretRef. Already-dynamic secret expressions stay verbatim and unreported.
   - Acceptance: Complete Git object enumeration finds no literal value; output/cache/journal/evidence scans find no value/raw backend data. A successful transaction leaves no literal-bearing transaction artifact; an injected recovery row retains it only beneath the authenticated private directory with no value disclosure. Dedicated tests separately cover programmatic persisted-reference Kind-only pass-through, source-literal re-ingest recapture/reporting, and already-dynamic preservation.

3. **Non-destructive, idempotent startup adoption**: Ingest/install changes only exact zsh-pro loader marker regions and performs one target promotion.
   - Current: Phase 5 supplies the safe installer, but Phase 6 needs to compose it transactionally with Store ingest.
   - Target: First adoption appends the canonical loader region while preserving every preexisting byte. Installed/re-ingest replaces or collapses only exact balanced marker regions according to the landed topology contract, preserving every byte before, between, and after them in original order. The authenticated journal uses `expectedTarget=original snapshot` and `expectedCandidate=independent install candidate`, promotes loader then target once before Store commit, and finalizes after a committed outcome. There is no final post-commit target rewrite.
   - Acceptance: An independently authored expected-installed template matches the actual file after exactly one in-memory substitution of a runtime-generated secret placeholder; no literal-bearing expected file is written. Outside-marker bytes match the original exactly. A second unchanged ingest is byte-identical with one marker pair. Pre-commit and Store-noncommit rows restore/retain filesystem state before Store Abort and loader/cache/initializer compensation.

4. **Out-of-block installer-append detection**: Ordinary content after the canonical END marker is warned about and remains byte-identical without being excluded from the full Profile merely because it is post-END.
   - Current: No ingest-facing warning exists.
   - Target: Exact-line topology scanning distinguishes malformed/duplicate marker regions from ordinary post-END content. Valid post-END content emits one stable non-failing warning and stays in place. When eligible for parsing it remains part of complete Profile persistence/accounting; the warning is not a persistence filter.
   - Acceptance: An installed fixture with post-END lines exits 0, emits exactly one warning, keeps the complete post-END bytes unchanged and ordered, and persists their eligible entries in the full redacted Profile.

5. **Linked round-trip, activation, and installed-startup equivalence**: No single regenerated-file assertion stands in for real startup behavior.
   - Current: Phase 2 has a synthetic round-trip oracle; Phase 4 activation and Phase 5 startup behavior are not yet linked to a real ingest.
   - Target: Prove all three:
     1. `Store.Read(main)` returns the full ordered redacted Profile, and `ir.Regenerate` preserves every non-secret entry's order, unmanaged `Text` verbatim, and managed semantics. The SecretRef comparator validates only the inert placeholder against authoritative Store data; it does not forge authority or claim source reconstructs `Secret`, `ValueMode`, `RuntimeValue`, or `StartLine`.
     2. `activate.Build` and the runtime emitter apply only EffectiveManaged/Representable entries and resolve SecretRefs; unmanaged execution canaries are inert.
     3. A built-binary E2E sources the pristine original and the **actual installed `.zshrc`** under isolated `zsh -f`, compares user-observable values/order/aliases/functions/options/PATH, includes a managed-definition → imperative-use ordering fixture and a literal-secret equality assertion without printing the value, permits only exact zsh-pro-owned loader symbols, proves no startup subprocess, and proves every outside-marker byte exact.
   - Acceptance: All three assertions pass from independently authored fixtures/expected startup bytes. Ingest itself executes no input; only the explicit isolated oracle stage sources files.

## Boundaries

**In scope:** strict `ingest [path] [--json]`; complete Profile persistence; Store-owned redaction/reporting; logical managed/unmanaged accounting; one journaled loader/install target promotion; typed filesystem-first compensation; exact post-END warning; regeneration, activation, and actual-installed-startup E2E; one Store at the composition root.

**Out of scope:** generated unmanaged complements; physical master-block output; a separate committed master file; deletion/reordering of ordinary startup source; a final post-commit target rewrite; new parser/classifier/regenerator/store internals except the transaction seams already planned in 06-01/06-02; execution during ingest; multi-file graphs; other shells; auto-cd and sharing.

## Constraints

- **No new dependencies.**
- **Static ingestion only.** Input is never sourced/evaled/command-substituted by ingest; dynamic syntax remains late-bound.
- **Complete Profile persistence.** Secret exclusion may redact values, but `EffectiveManaged` must not filter entries from `main`.
- **Non-destructive local source.** Only exact zsh-pro marker regions may be added/replaced/collapsed; all outside bytes remain exact and ordered.
- **Bounded secret-bearing transaction state.** Atomic full-file promotion may duplicate source bytes only at the authenticated exchange peer below the current-EUID mode-0700 transaction directory. The journal/evidence/cache/output/store never contain those bytes; success/restoration removes the peer durably, while uncertainty retains it privately for recovery.
- **One authenticated target transition.** Capability preflight happens before effects; expected target/candidate evidence stays independent; loader then `.zshrc` promote once; committed outcome finalizes rather than rewriting the target.
- **Filesystem-first compensation.** After target promotion and before Store commit, or after Store noncommit, restore/retain classification precedes AbortIngest, loader/cache rollback, and initializer rollback. Uncertainty stops later destructive compensation.
- **No secret authority forgery.** Source parsing cannot mint a trusted SecretRef or reconstruct metadata not encoded by source.
- **Independent oracles.** Expected startup/outside bytes and behavior assertions are authored independently of production output.
- **Single composition root and unchanged layering.**

## Acceptance Criteria

- [ ] `Store.Read(main)` returns the complete redacted source-ordered Profile; managed plus unmanaged statements equal source statements, and activation proves unmanaged entries inert.
- [ ] Literal secrets occur in no Git object or output; persisted SecretRef pass-through, source-literal re-ingest, and already-dynamic behavior are tested as distinct cases.
- [ ] First adoption appends and installed/re-ingest canonicalizes only exact marker regions; actual installed bytes match an independently authored expected startup and every outside-marker byte equals the original.
- [ ] The controller performs one loader/install target promotion, commits the full Profile, finalizes on commit, and uses filesystem-first restore/retain before Store/loader/cache/initializer compensation on noncommit.
- [ ] Valid post-END content emits one warning, exits 0, stays byte-identical, and remains represented in the full Profile when eligible.
- [ ] Store-read/regenerate, activation/runtime projection, and pristine-vs-actual-installed `zsh -f` behavior all pass, including order-sensitive dependency, literal-secret-without-printing, allowed-loader-symbol-only, no-startup-subprocess, and no-ingest-execution checks.
- [ ] CLI dispatch and composition-root layering remain exact; JSON emits one value-free object.

## Ambiguity Report

| Dimension | Score | Min | Status | Notes |
| --- | ---: | ---: | --- | --- |
| Goal Clarity | 0.92 | 0.75 | ✓ | Complete Profile persistence and non-destructive startup are explicit. |
| Boundary Clarity | 0.91 | 0.70 | ✓ | No physical complement/master and no second promotion. |
| Constraint Clarity | 0.92 | 0.65 | ✓ | One transaction, secret authority, and exact-byte boundaries are locked. |
| Acceptance Criteria | 0.93 | 0.70 | ✓ | Three linked assertions plus transaction/secret/outside-byte gates are falsifiable. |
| **Ambiguity** | **0.08** | **≤0.20** | ✓ | P6-011/P6-012 correction removes the conflicting architecture. |

## Decision Log Addendum

| Date | Finding | Decision locked |
| --- | --- | --- |
| 2026-08-02 | P6-011 | Persist the complete redacted ordered Profile; EffectiveManaged is projection-only; prove unmanaged activation inertness. |
| 2026-08-02 | P6-012 | Preserve ordinary startup bytes and add/canonicalize only the Phase 5 loader region; one pre-commit target promotion; prove the actual installed startup. |

---

*Phase: 06-ingest-end-to-end*
*Spec corrected: 2026-08-02*
*Next step: execute the corrected 06-01 through 06-05 plan set after plan-index/structure and independent review gates pass.*
