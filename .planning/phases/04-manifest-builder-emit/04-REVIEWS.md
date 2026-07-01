# Adversarial Review — Phase 4 Manifest Builder + Emit

Panel of 5 independent Opus lens reviewers (correctness, risk, requirement-coverage, security, simplicity),
each told to REFUTE and to flag any load-bearing plan claim lacking a PROVEN/STATIC-VALIDATED EVIDENCE entry as HIGH.

## Cycle 1

Per-lens HIGH (from return lines): correctness=4, risk=4, requirement-coverage=3, security=3, simplicity=2 → **HIGH_COUNT=16** (raw sum; heavy cross-lens overlap).

### Consensus HIGH concerns (deduplicated → 8 distinct)

**CH-1 — Ownership-aware PATH restore: refuted mechanism (C11) shipped; scope confusion.** (risk, requirement-coverage, security, simplicity — 4 lenses)
`RebuildListFromBase{Additions}` carries only THIS profile's additions; `Diff(A,nil)` has no representation of other active profiles, and emit renders it as `PATH="$ZP_BASE_PATH"; path=(<additions> $path)`. Under `typeset -U path`, two *managed* profiles co-owning an entry can't be resolved. **Resolution direction (planner):** v2.0 is single-active-profile per terminal (Phase 3 D-13) — two simultaneously-active *managed* profiles is NOT a runtime state. SPEC Req 6 criterion 3's example (`/usr/local/bin`) is **base**-owned; rebuild-from-base inherently preserves base entries, satisfying it for the single-profile model. So: (a) rescope the property-test ownership sub-check to **base-ownership** (base owns `/usr/local/bin`, profile also adds it, deactivate preserves it via rebuild-from-base), NOT two active managed profiles; (b) document multi-managed-profile co-ownership as out-of-scope for single-active v2.0 (defer to a share/Phase-5+ concern where the active set lives); (c) ensure the deactivate list op has DISTINCT semantics from the activate op (rebuild-to-base, not re-add this profile's additions).

**CH-2 — Property-test snapshot instrument is exported-names-only (reintroduces C19 false-green).** (correctness, risk)
The plan reuses `introspectScript`'s `##ENV##` section, which is `[[ $v == *export* ]]`-filtered (exported vars, NAMES only). This is exactly the C19 false-green (a leaked non-exported var, or a clobbered value on an exported var, passes byte-identical). **Fix:** the property test must use a NEW full-env snapshot section iterating ALL `${(@kv)parameters}` (exported AND non-exported), emitting name+VALUE with multi-line-safe (NUL) framing; assert it would fail on a non-exported/value-only residue.

**CH-3 — Runtime undo globals pollute the full-env snapshot; no deactivate cleanup spec.** (correctness)
Emitted apply creates `ZP_BASE_PATH`, `__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, sentinels. A full-env (CH-2) snapshot after apply→deactivate sees them as newly SET → spurious fail, or a silent filter hides real residue. **Fix:** decide+state explicitly — either deactivate unsets its own undo slots (add cleanup ops + test), OR the snapshot excludes the `ZP_`/`__ZP_` namespace via an audited, documented filter.

**CH-4 — Shadow prior is never captured at apply time.** (correctness)
`AddAlias`/`AddFunc` emission says nothing about capturing the pre-existing shadowed body BEFORE overwrite; only `SetScalar` captures the live prior. Without capture-before-overwrite (guarded by `${+name}`), `$prior` is never populated and shadow restore cannot pass. **Fix:** specify that `AddAlias`/`AddFunc` emit a `${+name}`-guarded capture-before-overwrite into the shadow-prior slot; add an apply-side capture test.

**CH-5 — Mutated-emitter negative check incompatible with stateless `Provider{}`; too narrow; not independently asserted.** (correctness, requirement-coverage, security)
The negative check needs a flag/variant, but `Emit` is a method on zero-value `Provider struct{}` (no fields to flip). **Fix:** factor the PATH-rebuild (and value-quoting) rendering into an injectable package-level func the test can substitute; add a SECOND mutant that drops zquote on a static metacharacter value (injection-class, not only PATH-append); add an explicit assertion/meta-test that each mutant produces a non-empty diff / fired canary, independent of the happy path.

**CH-6 — `[[ $e == $target ]]` is glob matching, not literal-equality (reintroduces the C10 bug).** (security)
zsh pattern-matches the unquoted RHS of `==` inside `[[ ]]`: `[[ /opt/toolX == /opt/tool* ]]` MATCHES. A PATH element containing `? * [ ]` over-matches — the exact C10 collateral-deletion hazard. **Unvalidated claim** (POC-Z6c only tested `${path:#}`). **Fix:** quote the RHS — `[[ $e == "$target" ]]` — add a POC/EVIDENCE entry proving the quoted form is literal, and a residue-test PATH element containing a metacharacter.

**CH-7 — `RestoreOption{Name, WasOn bool}` built from data the builder is forbidden to author.** (requirement-coverage)
D-06 says `was_on` is a runtime-captured LIVE fact, never authored into the manifest — yet `Diff` builds `RestoreOption{WasOn}` from the manifest where `WasOn` is structurally always the zero value, so "exact was_on restore" (emit) can't be honored from Go-side data. **Fix:** specify that option-reverse reads the runtime-captured `was_on` slot (like scalars/shadows), and mark the op's `WasOn` field non-authoritative (or drop it).

**CH-8 — SchemaV1 "forward-compat gate" is only a `const`, not enforced (T-04-01 unmitigated).** (security, risk)
The threat register dispositions T-04-01 "mitigate" on the basis that SchemaV1 is "a gate checked before reverse logic," but no code reads `Manifest.Schema` and rejects a mismatch — the only acceptance is `grep 'const SchemaV1'`. **Unvalidated claim.** **Fix:** either add a real check (`Build`/`Diff`/`Emit` errors when `Schema != SchemaV1`) with a test, or re-disposition T-04-01 to deferred/Phase-5 and stop calling a bare const a mitigation.

### Notable MEDIUM (address where cheap; several cluster on CH-1/CH-5)
- Builder PATH split (`$HOME/bin:$PATH` → additions vs base marker) is an unvalidated Go-side string-parse; edge cases (mid-list base, no base marker, `${PATH}` brace form). Add a POC/EVIDENCE entry or narrow the builder to the router-admitted prepend/append shapes and route the rest imperative.
- NUL body-parser "slice up to next `\n##` marker" can be fooled by a body line starting `##`; bound the section by record-count/offset or a NUL-preceded sentinel, not a `\n##` scan.
- `ListDelta.Deletions` / `RebuildListFromBase.Deletions` are always empty this phase — note explicitly as validated-present-but-unexercised (SPEC Req 1 "additions+deletions" half-covered).
- Partial-apply / interrupted-switch failure mode unaddressed (plain functions, no transaction/rollback) — add a threat-register row + document the recovery contract (Phase 5 seam).
- Cross-terminal undo-slot re-capture on double-`checkout` overwrites the captured prior with already-applied state — specify a `${+slot}`-guarded capture-idempotency (only capture on clean base).
- Injection corpus not run through the slot-name derivation path (OQ-10/C24 name→slot) — add adversarial name cases.
- (simplicity) 13-type Op union and the self-contained "Hybrid helper-def preamble" (OQ-9) are heavier than SW-01/SW-02 require; pin OQ-9 to bare `zp_*` calls (the test supplies inline helpers); optionally collapse paired restore/unset ops. Not blocking.

### Confirmed sound (held up under attack)
`zquote` inert for value contexts (C5); verbatim function-body restore (C6); `${+name}` shadow guards (C1/OQ-8); `${(P)+var}` env drift guard where var holds the name (C2/C21); NUL-framed body dump (C14/C23); plain loader functions (C8); tri-state `*string`+omitempty (C15); two-type AliasSet/FuncSet split essential per C16.

**HIGH_COUNT = 16 → not converged. Proceed to replan (cycle 1 revision).**
