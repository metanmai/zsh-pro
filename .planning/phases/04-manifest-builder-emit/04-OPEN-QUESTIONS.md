---
phase: 04-manifest-builder-emit
updated: 2026-07-01T20:00:00Z
open_count: 23
---

# Open Questions — Phase 4 Manifest Builder + Emit

> Auto-decided under uncertainty while running unattended. Review and override as needed.
> Each item already has a tentative choice applied so downstream work could proceed.

## OQ-1: Keybindings / hooks class membership (deferred from Phase 1 A3)

- **Question:** Phase 1's spike deliberately did NOT admit or exclude `bindkey` state or hook arrays (`precmd_functions`/`chpwd_functions`) — it reported their presence as *data* and explicitly punted the manage-vs-master-block decision to Phase 4 (see `01-FINDINGS.md` "Keybinding / Hook Presence" and `01-MANIFEST-SHAPE.md` "Classes With NO Representation Here"). Should Phase 4's `model.Manifest` add a managed class for keybindings/hooks?
- **Tentative choice (applied):** NO — keybindings and hooks stay OUT of the managed set for v2.0. They route to the unmanaged master block, exactly like `compinit`. The `model.Manifest` has no field for them; SW-02's zero-residue property test covers only the six admitted classes (env scalars, PATH-like lists, aliases, functions, options — with FPATH as a list).
- **Alternatives:** (a) Add a `keybindings` NameSet-style part now and prove byte-reversibility in this phase; (b) add a reported-only "detected but unmanaged" advisory surface.
- **Why uncertain:** The spike measured `bindkey` at 144 entries and non-empty `precmd_functions` in a real inherited shell, so the class is real — but it was never put through the byte-identical round-trip that admitted the other six classes. Admitting it now would require its own reversibility proof, which is spike-territory, not builder-territory.
- **Impact:** Low for the core switch loop. Excluding keybindings/hooks does not affect SW-01/SW-02 zero-residue on the six admitted classes; it only limits which state a profile can switch. Reversible later (adding a class is additive to `model.Manifest`).
- **Confidence:** HIGH (safest, most reversible default; consistent with precision-over-recall and the Phase 1 completion exclusion). Recorded as an assumption, not a blocker.

## OQ-2: Manifest `env[].original` vs live-prior reconciliation

- **Question:** The validated shape carries a declarative `env[].original`, but the proven loader trusts the LIVE prior captured at apply time (`zp_capture_env`), not the static `original`. How does the emitted deactivate code reconcile the two?
- **Tentative choice (applied):** The manifest's `original` is the declarative record of intent; the emitted apply code captures the live prior into runtime undo state (`__ZP_ORIG_*`) at apply time, and the drift guard (`reverse only if live == applied`) governs restore. Deactivate restores to the captured *live* prior, using the manifest `original` only as the declarative expectation. (Directly follows the Phase 1 carry-forward.)
- **Alternatives:** (a) Trust the static `original` from the manifest (rejected by Phase 1 — breaks byte-identity under a base that differs from authored intent); (b) drop `original` from the manifest entirely (loses the declarative record and the unset-vs-empty encoding).
- **Why uncertain:** This is a design nuance between the manifest as a *record* and the loader as the *actor*; the exact code split lands in emit.go + core/activate during plan-phase.
- **Impact:** Medium — gets the drift guard (SW-02 criterion 3) right. Wrong choice reintroduces the clobber-a-hand-edit failure Phase 1 explicitly guarded against.
- **Confidence:** HIGH (dictated by the Phase 1 loader-reality finding). Locked in the SPEC constraints.

## OQ-3: Where the "captured base" for PATH lives across a switch

- **Question:** PATH is a delta vs a captured base. Is the base captured once at first-activate (session base) and reused across switches, or re-captured per switch?
- **Tentative choice (applied):** Base is captured ONCE at first activate into per-terminal runtime state (`ZP_BASE_PATH`-style), and every apply rebuilds `PATH` from that same base + the profile's additions/deletions — never `typeset -U`, never append. Deactivate rebuilds from the same base. (Phase 1 FM2 proved this keeps `$#path` stable across ≥5 cycles.)
- **Alternatives:** (a) Re-capture base on every switch (rejected — a switch would bake the prior profile's additions into the "base", causing PATH growth, the exact FM2 failure); (b) store base in the manifest (rejected — base is machine/session-specific runtime state, not portable profile data).
- **Why uncertain:** The precise capture *placement* (which loader entry point captures the base, and the re-capture guard) is called out in the ROADMAP research flags as "subtle" and is finalized in Phase 5's loader — Phase 4 emits the plan assuming a base exists in runtime state.
- **Impact:** Medium — wrong placement causes PATH accumulation (SW-02 criterion 2). The property test is the backstop.
- **Confidence:** MEDIUM-HIGH (the mechanism is locked by Phase 1; only the loader-side placement is a Phase 5 concern). Flagged so plan-phase treats base-capture placement as a Phase 4/5 seam contract.

## OQ-4: `NameSet.Added` shape — map vs array (validated-shape discrepancy)

- **Question:** `01-MANIFEST-SHAPE.md` shows `aliases.added` as a name→body **map** (`{"gs":"git status"}`) but `functions.added` as a bare **array** of names (`["work_deploy"]`). Should `model.NameSet` use one uniform Go type for both aliases and functions, or two different shapes matching the fixture exactly?
- **Tentative choice (applied):** ONE uniform `NameSet{ Added map[string]string; Shadowed map[string]string }` used for both aliases and functions. For functions, `Added` maps name→body (body may be `""` when only the name is needed for `unset -f`, since `unset -f` needs only the name). This keeps a single Go type, single marshal/unmarshal path, and a single builder branch.
- **Alternatives:** (a) Two distinct types — `AliasSet{Added map[string]string}` and `FuncSet{Added []string}` — matching the fixture byte-for-byte; (b) keep the fixture's `functions.added` array but store bodies in a parallel structure.
- **Why uncertain:** The uniform map diverges from the validated JSON fixture's `functions.added` array, so the round-trip acceptance test ("round-trips to/from the `01-MANIFEST-SHAPE.md` JSON with no field missing") may need the fixture re-expressed as a map, OR the acceptance is read as "no field missing for any admitted class" (semantic, not byte-identical JSON). A uniform map is the safest, most reversible internal shape; if a byte-identical fixture match is required, split into two types in plan-phase (additive, low-cost).
- **Impact:** Low-medium — internal type ergonomics + one acceptance-test interpretation. Both shapes carry the same information (`unset -f` needs only the name; the map's value is simply unused for function-add). Reversible: changing `[]string`↔`map[string]string` is a localized type edit.
- **Confidence:** MEDIUM (leans uniform-map for code simplicity; the fixture literally shows an array for functions). **Update (04-RESEARCH, POC-J2/J3):** a uniform `map[string]string` provably CANNOT unmarshal the fixture's `functions.added` array (`json: cannot unmarshal array into Go struct field ... of type map[string]string`); two distinct types match the fixture byte-for-byte. Research recommends the two-type split (see OQ-11). Resolve during plan-phase against the exact round-trip acceptance reading.

## OQ-5: Emitted reverse logic — call shared loader helpers vs inline per-op

- **Question:** Should `emit.go` emit calls to a small fixed set of runtime helper functions (`zp_capture_env`/`zp_restore_env`/PATH-rebuild/shadow-restore, defined once by the Phase 5 loader) or inline the full reverse logic into every emitted apply/deactivate block?
- **Tentative choice (applied):** Emit **calls to a small fixed helper set** (the `zp_*` functions from the Phase 1 reference snippet), so the per-plan emitted surface is minimal and the tricky drift-guard/`${(P)+var}` logic lives in one audited place. The helper definitions themselves are a Phase 4↔5 seam: Phase 4 emits code that calls them; whether Phase 4 also emits the helper definitions (self-contained block) or assumes the Phase 5 loader defines them is finalized in plan-phase. **See OQ-18 (pinned to bare calls, Hybrid dropped).**
- **Alternatives:** (a) Inline every reverse op fully (self-contained emitted block, no runtime dependency — heavier emitted text, logic duplicated per profile); (b) hybrid — emit helpers once per apply block, calls thereafter.
- **Why uncertain:** The split between "code Phase 4 emits" and "helpers the Phase 5 loader owns" is a seam boundary that firms up when the loader lands. For the Phase 4 property test to run standalone, the test harness must supply the helper definitions (or emit.go must emit a self-contained block) — a plan-phase detail.
- **Impact:** Medium — affects emitted-code size, the emit↔loader seam contract, and how self-contained the property test's `zsh -f` input is. Reversible: helper-call vs inline is an emit-strategy change, not a data-model change.
- **Confidence:** MEDIUM-HIGH (helper-calls match the Phase 1 reference snippet and keep the audited logic in one place). Flagged so plan-phase pins the Phase 4↔5 helper-ownership boundary. **See OQ-9 / OQ-18** for the self-contained-block question (now pinned to bare calls).

## OQ-6: Injection-safe escaping — static-quoted vs dynamic-verbatim split (T-01-06)

- **Question:** User-controlled values (env values, alias/function bodies) must be injection-safe in the `eval`'d emitted code, but Phase 2/EVAL-01 keeps dynamic values (`$HOME/go`, `$(...)`) late-bound and `regen.go` emits them verbatim (unquoted). How does emit.go reconcile "escape everything" (injection safety) with "emit dynamic verbatim" (portability)?
- **Tentative choice (applied):** Split on the `Entry.Dynamic` flag the IR already carries. **Static** values → hard single-quote with the zsh `'\''` escape (a value with `'`/`;`/`$(...)`/backtick/newline becomes an inert literal string, never command position). **Dynamic** values → emit verbatim so zsh expands per-machine at apply time; this is safe because the value came from a parsed, non-executed AST field (structured codegen), not from `eval` of raw user config. Never partial-eval user config.
- **Alternatives:** (a) Quote EVERYTHING including dynamic values (breaks EVAL-01 portability — `$HOME` would become the literal string `$HOME`); (b) an allowlist of permitted dynamic constructs (`$VAR`, `${...}`, `$(...)`) with everything else quoted — tighter, but needs its own parser and a proof that the allowlist can't be escaped.
- **Why uncertain:** This is the crux of threat T-01-06 (deferred from Phase 1 to here). Emitting a dynamic value verbatim means a maliciously-crafted `$(rm -rf ~)` in a profile WOULD execute at apply — but that value was authored by the profile owner and is exactly the late-binding EVAL-01 promises (equivalent to it being in their `.zshrc`). The residual risk is a value that is *classified* dynamic but should have been static/quoted, or a shared/imported profile (SHARE-01, a future milestone with its own trust gate). The safe default trusts the owner's own profile (same trust boundary as their `.zshrc`) and quotes everything not explicitly dynamic.
- **Impact:** HIGH (security) but the trust boundary is the profile owner's own config. A wrong split either breaks portability (over-quoting) or, in a future shared-profile world, is an injection vector (under-quoting) — SHARE-01's trust gate is the future backstop. For v2.0 (own profiles only), quote-static/verbatim-dynamic matches the `.zshrc`-equivalence trust model.
- **Confidence:** MEDIUM. **Update (04-RESEARCH, POC-Inj/Eval/Dyn):** the `'\''` escape is VERIFIED injection-safe against `'`, `;`, `$(...)`, backtick, newline, `'\''`-chains, and the kitchen-sink combo — under the real double-`eval` loader model, across scalar/alias/function-body contexts. The verbatim-dynamic `$HOME/go` expands as intended; the same value quoted stays literal. The remaining risk is purely a *misclassification* (a static value tagged `Dynamic`). Plan-phase must add a test asserting a static value with shell metacharacters is always quoted regardless of the flag path.

## OQ-7: Introspected function-body encoding for multi-line bodies

- **Question:** The existing `introspectScript` is line-oriented (one name per line, section-delimited). Function bodies (`${functions[name]}`) are multi-line by nature. How are multi-line alias/function bodies encoded so parsing stays robust and the existing name-only parse is untouched?
- **Tentative choice (applied):** Dump bodies in a NEW section (e.g. `##FUNC_BODIES##`) using a delimiter format that survives embedded newlines — e.g. a `name` line followed by a length-prefixed or sentinel-terminated body block, or NUL/record-separator delimiting — rather than the naive `name\tbody`-on-one-line (which a newline in the body would break). Verify the chosen encoding with a throwaway POC (a function with a newline + a `\t` + a `'` in its body) before finalizing the parser. Existing `##ALIASES##`/`##FUNCTIONS##` name sections stay exactly as-is (additive, D-14/D-15).
- **Alternatives:** (a) `name\tbody` single-line (simple, but breaks on any body newline — most function bodies have newlines, so this is likely wrong); (b) base64-encode each body (newline-safe, trivially parseable, but opaque in fixtures/diffs); (c) `print -rN` NUL-delimited records parsed by splitting on NUL.
- **Why uncertain:** zsh's `${functions[name]}` returns the body with its original newlines/indentation; the encoding must round-trip that byte-for-byte (the shadow-restore acceptance is "byte-identical prior body"). The exact zsh dump construct (`print -r`, `print -rN`, `typeset -f`) and the Go-side split need a POC to confirm no truncation/mangling.
- **Impact:** Medium — a broken encoding silently corrupts a shadowed function body, failing the byte-identical shadow-restore acceptance (SPEC req 7). Reversible: the encoding is internal to introspect+emit; changing it is localized.
- **Confidence:** MEDIUM → **RESOLVED-DIRECTION (04-RESEARCH, POC-Z3/Go-RT):** alternative (c), NUL-delimited `print -rN -- name body` records, is VERIFIED to round-trip a body containing `\n`+`\t`+`'`+`$`+`;` byte-for-byte (zsh dump → Go split-on-`\x00` → back into `functions[name]=`). NUL is safe because a zsh string cannot contain NUL. The naive `name\tbody` (alt a) provably breaks (embedded `\n`). Parser must slice the section by header/`##END##` offsets and split the enclosed bytes on NUL (not scan line-by-line, since bodies may contain `#`). **See OQ-17** for the section-boundary tightening (NUL-preceded sentinel, not a bare `\n##` scan).

## OQ-8: The Phase 1 Loader Reference Snippet's shadow-restore guards are incorrect (evidence-backed correction)

- **Question:** The `01-FINDINGS.md` "Loader Reference Snippet" — which `emit.go` is instructed to "faithfully reproduce" (D-12) — guards shadow restore with `[[ -n "$ZP_A_PRIOR_ALIAS_ll" ]]` and elsewhere implies `${(P)+ZP_..._ll}`. Are these guards correct?
- **Tentative choice (applied):** NO — reproduce the snippet's *structure* but CORRECT its two guard bugs. `emit.go` (and the property-test harness helpers) must use `${+name}` (a set-test on the literal slot name), NOT `-n` and NOT `${(P)+literalName}`.
- **Evidence (POCs run this session, `zsh 5.9`):**
  - **Bug 1 — `-n` conflates empty with unset.** A legitimately empty prior body (`alias x=''`) captured into a slot then guarded by `[[ -n "$SLOT" ]]` is SKIPPED — the empty alias is silently not restored (POC-Z8e: `-n` → `SKIPPED (empty prior silently lost)`; `${+SLOT}==1` → restores it, empty body preserved). This is the exact unset-vs-empty distinction Phase 1 carry-forward 2 already mandates for env vars, not yet applied to shadow bodies.
  - **Bug 2 — `${(P)+literalName}` indirects through the wrong parameter.** `(P)` treats its operand as a variable whose *value* is the parameter to test; writing the slot name literally inside `(P)` tests a parameter named by the *value* of `ZP_..._ll` (empty), returning `0` even when the slot is set (POC-Z8b: `(P)+ZP_..._ll` → `0`). The literal-named form must be `${+ZP_..._ll}` (no `(P)`), which returns `1` and restores correctly (POC-Z8c). A faithful reproduction of the snippet leaked a residual `ll` alias; the corrected form is byte-identical (POC-Z8d: `ZERO-RESIDUE: byte-identical`).
- **Alternatives:** (a) `local slot="ZP_..._ll"; [[ "${(P)+slot}" == "1" ]]` — also correct (indirection through a var holding the name); (b) capture into a zsh associative array keyed by name and test membership. `${+name}` is simplest because `emit.go` knows the literal name at codegen time.
- **Impact:** HIGH for correctness of Req 7 (shadow restore) — under the buggy guards, shadow restore silently no-ops for literal-named slots and drops empty-body shadows, failing the byte-identical shadow-restore acceptance. The mutated-emitter/property test (D-16) would catch a literal-`(P)` regression as residue.
- **Confidence:** HIGH (the fix and the failure are both POC-verified). Logged (not silently overridden) per the autonomy contract: this corrects a carry-forward *artifact*, consistent with the *intent* of Phase 1 carry-forward 2 (unset-vs-empty correctness).

## OQ-9: Should `emit.go` emit a self-contained helper block?

- **Question:** OQ-5 defaults to emitting *calls* to `zp_*` helpers the Phase 5 loader owns. But the Phase 4 property test (D-16) must run under `zsh -f` *without* the Phase 5 loader. Should `emit.go` optionally emit a self-contained block that also *defines* the `zp_*` helpers (Hybrid), so emitted code is testable and usable standalone?
- **Tentative choice (applied):** For Phase 4, the property-test harness supplies the `zp_*` helper definitions inline (verified working self-contained in POC-Z8d), so `emit.go` can emit bare calls per the OQ-5 default. Recommend the planner ALSO consider a Hybrid mode (emit a helper-definition preamble once per block) as a low-cost hedge that makes emitted code self-contained without waiting for Phase 5 — decide during plan-phase. **SUPERSEDED by OQ-18: pinned to bare calls; the Hybrid option is dropped for Phase 4.**
- **Alternatives:** (a) Bare calls only, harness supplies helpers (current default); (b) Hybrid — emit helper defs + calls; (c) fully inline every op (OQ-5 alt a).
- **Impact:** Medium — affects whether the emitted string is runnable in isolation and the emit↔loader seam contract. Reversible (emit-strategy, not data-model).
- **Confidence:** MEDIUM (a seam-boundary question that firms up when the Phase 5 loader lands; both shapes are proven runnable). **See OQ-18 (pinned).**

## OQ-10: Runtime undo-slot naming must produce valid zsh identifiers

- **Question:** The emitted apply code stores captured live priors in `typeset -g` globals whose names embed the profile name and the alias/function/var name (e.g. `ZP_<profile>_PRIOR_ALIAS_<name>`, `__ZP_ORIG_<var>`). A profile named `feature/x`, or an alias/var name containing characters illegal in a zsh identifier, would make `typeset -g` fail. How are slot names derived so they are always valid `[A-Za-z0-9_]` identifiers?
- **Tentative choice (applied):** `emit.go` sanitizes the `profile` + target name into a valid identifier before composing the slot name — e.g. replace every non-`[A-Za-z0-9_]` byte with `_`, or (to avoid collisions between `a/b` and `a_b`) append a short hash of the raw name. Env var names are already valid identifiers by construction (they come from `KindAssignment` names), so `__ZP_ORIG_<var>` is safe; the collision risk is in the profile-name and alias-name segments.
- **Alternatives:** (a) Store priors in a single associative array keyed by the raw (arbitrary) name — `typeset -gA ZP_PRIOR_ALIAS; ZP_PRIOR_ALIAS[$rawname]=...` — which sidesteps identifier rules entirely and is arguably cleaner; (b) reject/skip profiles with unsanitizable names (too restrictive).
- **Impact:** Medium — an unsanitized name aborts apply with a `typeset` error. Alternative (a), an assoc-array keyed by raw name, is likely the more robust design and worth the planner's consideration over slot-name-per-global.
- **Confidence:** MEDIUM (the hazard is real; the assoc-array alternative may be strictly better than name-sanitization — a plan-phase design call). **Cycle-1 review MEDIUM:** the injection corpus (adversarial names) must be run through the slot-name derivation path, not only through values — covered in Plan 04-02 Task 1 behavior + Task 2 slot-name sub-check.

## OQ-11: OQ-4 resolution direction — two distinct types vs uniform map

- **Question:** Given the POC evidence (OQ-4 update), which shape does `model.Manifest` use for `aliases`/`functions`?
- **Tentative choice (applied):** Two distinct part types — `AliasSet{Added map[string]string; Shadowed map[string]string}` and `FuncSet{Added []string; Shadowed map[string]string}` — matching `01-MANIFEST-SHAPE.md` byte-for-byte, because the round-trip acceptance names that fixture as the oracle and a uniform map provably cannot unmarshal `functions.added: ["work_deploy"]` (POC-J2).
- **Alternatives:** (a) Uniform `NameSet{Added map[string]string; Shadowed map[string]string}` for both — requires editing the fixture's `functions.added` to a map, changing what "round-trips to the shape" means (a defensible but explicit reading); (b) uniform type with a custom `UnmarshalJSON` accepting both shapes (POC-J4, over-engineered for v2.0).
- **Impact:** Low-medium — internal type ergonomics + the round-trip acceptance reading. Reversible.
- **Confidence:** MEDIUM (research recommends two types for byte-identical fixture match; the planner may keep a uniform map if it also edits the fixture and documents the semantic reading).

## OQ-12: Claim-validation-driven CONTEXT/RESEARCH corrections (RESOLVED, POC-backed)

- **Question:** Claim-validation pass 1 (24 claims, 12 REFUTED) surfaced load-bearing corrections to locked design facts. Were the locked CONTEXT decisions updated so the planner does not build a refuted mechanism?
- **Tentative choice (applied):** YES — auto-applied from POC evidence (see `04-EVIDENCE.md`), HIGH confidence:
  - **D-12 / canonical refs:** shadow-restore guards use `${+name}` set-tests, NOT the Phase 1 snippet's buggy `-n`/`${(P)+literalName}` guards (C1/OQ-8). The env-path `${(P)+var}` where `var` holds the name stays (C2/C21, correct).
  - **D-13:** single-quote wrapping applies to VALUE contexts (env values, alias bodies) only; **function bodies are live code**, restored by verbatim `functions[name]=$capturedBody`, never single-quote-wrapped-to-inert (C6).
  - **D-14/D-15:** body-dump uses NUL-delimited (or length-prefixed) framing, NOT a `name\tbody` line format (which truncates multi-line bodies; C14/C23); a NEW multi-line-safe reader parses it.
  - **D-08:** the "no zsh token in core/activate" check is defined precisely (case-sensitive, word-boundary, string-literals only, excluding Go identifiers/comments; C22).
  - **D-02:** `*string`+omitempty softened from "only shape" to "recommended shape" (C15).
- **Alternatives:** Leave decisions as-authored (rejected — would instruct the planner to reproduce a proven-buggy guard, a live-code injection misconception, and a truncating encoding).
- **Why uncertain:** Not uncertain — each correction is POC-proven. Logged for the returning user's audit trail.
- **Impact:** HIGH if NOT corrected (shadow restore silently no-ops; function-body "quoting" is a false safety claim; multi-line bodies truncate). Corrected → neutralized.
- **Confidence:** HIGH (POC-backed). Recorded as resolved, not open.

## OQ-13: Multi-managed-profile PATH co-ownership is OUT OF SCOPE for single-active v2.0 (from REVIEWS CH-1)

- **Question:** Cycle-1 review CH-1 (C11 refuted) flagged that `RebuildListFromBase{Additions}` carries only THIS profile's additions and `Diff(A,nil)` has no representation of OTHER active profiles, so two *managed* profiles co-owning a PATH entry under `typeset -U path` cannot be resolved by rebuild-from-base alone. Does Phase 4 need a reference-count / set-based ownership model?
- **Tentative choice (applied):** NO — v2.0 is **single-active-profile per terminal** (Phase 3 D-13: `ZSHPRO_PROFILE` names exactly one active profile). Two simultaneously-active *managed* profiles is not a runtime state, so the C11 two-managed-owner scenario does not arise at runtime. SPEC Req 6 criterion 3's `/usr/local/bin` example is **BASE-owned** (it lives in `ZP_BASE_PATH`); rebuild-from-base inherently preserves base entries because deactivate restores `PATH="$ZP_BASE_PATH"` which already contains it. The property test's ownership sub-check is therefore rescoped to **base-ownership**: base owns `/usr/local/bin`, the profile also adds it, deactivate preserves it via rebuild-from-base — NOT two active managed profiles. Multi-managed-profile co-ownership (reference-count / active-set model) is **deferred to a later share/loader concern** (SHARE-01 / Phase 5+, where the active set actually lives).
- **Alternatives:** (a) Build a reference-count ownership model now (rejected — no runtime state has two active managed profiles in v2.0; premature); (b) leave the ambiguity unaddressed (rejected — reintroduces C11 confusion).
- **Impact:** Medium — correctly scopes SW-02 Req 6 to the single-active model. A future multi-profile/share milestone must add the active-set ownership model before allowing concurrent managed profiles.
- **Confidence:** HIGH (dictated by Phase 3 D-13 single-active constraint). Addresses review concern CH-1.

## OQ-14: Deactivate cleanup of per-switch runtime undo globals vs snapshot filter (from REVIEWS CH-3)

- **Question:** Emitted apply creates per-switch runtime undo globals (`__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, sentinels). A full-env (CH-2) property-test snapshot taken after apply→deactivate would see them as newly SET → spurious residue fail, unless deactivate cleans them up OR the snapshot filters the namespace. Which?
- **Tentative choice (applied):** **Deactivate UNSETS its own per-switch undo slots** (`__ZP_ORIG_*`, `ZP_<profile>_PRIOR_*`, sentinels) as part of the reverse — the cleaner design (no silent filter hiding real residue). `ZP_BASE_PATH` is **session-persistent** by design (captured once at first activate, reused across switches per OQ-3) and stays — the property test either sets `ZP_BASE_PATH` BEFORE the baseline snapshot (so it is present in both baseline and post snapshots and nets to zero diff) OR excludes it via a single audited, documented `ZP_BASE_PATH`-only filter. The per-switch `ZP_`/`__ZP_` slots are unset by deactivate and so never appear as residue.
- **Alternatives:** (a) Blanket-filter the whole `ZP_`/`__ZP_` namespace from the snapshot (rejected — a silent filter could hide a genuinely-leaked per-switch slot, defeating the residue test); (b) leave the slots set and filter (rejected for the same reason).
- **Impact:** Medium — wrong choice either spuriously fails the residue test or silently hides residue. Chosen: deactivate cleans its per-switch slots; only `ZP_BASE_PATH` (session-persistent, single documented exception) is present-in-both / audited-filter.
- **Confidence:** MEDIUM-HIGH. Addresses review concern CH-3.

## OQ-15: SchemaV1 forward-compat gate — real runtime check vs bare const (from REVIEWS CH-8)

- **Question:** Cycle-1 CH-8: the threat register dispositions T-04-01 "mitigate" on the basis that `SchemaV1` is "a gate checked before reverse logic," but a bare `const SchemaV1 = "v1"` with only `grep 'const SchemaV1'` as acceptance enforces nothing. Add a real check or re-disposition the threat?
- **Tentative choice (applied):** **Add a REAL check.** The reverse-logic entry points `Diff` (and by extension the `Emit` path it feeds) refuse to operate on a manifest whose `Schema != model.SchemaV1` — `Diff` returns an error (or a sentinel) when either the `active` or `target` manifest carries an unknown schema, with a dedicated test asserting a schema-mismatch manifest is rejected. `Build` stamps `SchemaV1`; `Diff` validates it before generating any reverse op. T-04-01 is re-worded to reference the real check, not the const.
- **Alternatives:** (a) Re-disposition T-04-01 to deferred/Phase-5 and stop calling a bare const a mitigation (rejected — the check is cheap and closes the threat now); (b) check in `Build` only (weaker — the reverse-logic entry point is `Diff`, so the guard belongs there).
- **Impact:** Medium (security) — a future-versioned manifest is detectably rejected rather than silently mis-reversed.
- **Confidence:** HIGH. Addresses review concern CH-8.

## OQ-16: Builder PATH-split scope — full parse vs router-admitted shapes (from REVIEWS MEDIUM)

- **Question:** The builder splits a `CatPath` value like `$HOME/bin:$PATH` into additions vs the base marker (D-07). Cycle-1 MEDIUM: this Go-side string parse is unvalidated for edge cases (mid-list base, no base marker, `${PATH}` brace form). Full parse or narrow scope?
- **Tentative choice (applied):** **Narrow the builder to the router-admitted prepend/append shapes** — the builder handles exactly the shapes `core/ir/route.go` already admits as managed PATH assignments (a base self-reference `$PATH`/`$path`/`${PATH}`/`$FPATH`/`$fpath`/`${FPATH}` at the head or tail, colon-joined additions on the other side). Any `CatPath` value whose split is ambiguous (no base marker, base marker mid-list, or an unrecognized brace form) is NOT admitted as a `ListDelta` — it produces no part (routed imperative, honoring ING-02 precision-over-recall). The split must recognize both `$PATH` and `${PATH}` (and the lowercase/`fpath` forms). This is an UNVERIFIED claim (see EVIDENCE C26) — claim-validation pass 2 must prove the narrowed split against the mid-list-base, no-base-marker, and `${PATH}`-brace fixtures before execution.
- **Alternatives:** (a) Full general PATH-expression parser now (rejected — over-scoped, unvalidated, risks misclassification residue); (b) admit everything and hope (rejected — a bad split is a zero-residue violation).
- **Impact:** Medium — a mis-split bakes the wrong segment into `Additions`, breaking rebuild-from-base ownership. Narrowing + precision-over-recall is the safe default.
- **Confidence:** MEDIUM (pending pass-2 validation of the narrowed split). Addresses review MEDIUM (builder PATH split).

## OQ-17: NUL body-parser section boundary — offset/record-count vs `\n##` scan (from REVIEWS MEDIUM)

- **Question:** Cycle-1 MEDIUM: the body-parser slices "up to the next `\n##` marker," but a function body line legally starting with `##` (a comment) could be mistaken for a section header, corrupting the parse.
- **Tentative choice (applied):** Bound the body sections by a **NUL-preceded sentinel** rather than a bare `\n##` scan: the section is a run of `\x00`-framed `name\x00body\x00` records terminated by a distinguished end-of-section marker that is itself NUL-preceded (so a body line starting `##` — which is not NUL-preceded at a record boundary — cannot be mistaken for it), OR bound the section by a record-count/byte-offset. The parser locates the `##ALIASBODIES##`/`##FUNCTIONBODIES##` headers, then consumes NUL-framed records until the NUL-preceded section terminator — it never line-scans the body bytes for `##`.
- **Alternatives:** (a) Keep the `\n##`-scan (rejected — a `##`-prefixed body line fools it, C23-class corruption); (b) length-prefix each record (also acceptable; either offset/count or NUL-preceded sentinel satisfies the requirement).
- **Impact:** Medium — a fooled boundary corrupts every following section, silently dropping a shadowed body (fails byte-identical shadow restore).
- **Confidence:** MEDIUM-HIGH. Addresses review MEDIUM (NUL body-parser boundary).

## OQ-18: OQ-9 pinned — emit bare `zp_*` calls (drop Hybrid-preamble optionality) (from REVIEWS MEDIUM/simplicity)

- **Question:** OQ-9 left the Hybrid self-contained-helper-preamble as an optional low-cost hedge alongside the OQ-5 default (bare `zp_*` calls). Cycle-1 simplicity MEDIUM: the optionality is heavier than SW-01/SW-02 require.
- **Tentative choice (applied):** **PIN to bare `zp_*` calls.** `emit.go` emits bare calls to the `zp_*` helper set; the Phase 4 property test supplies the helper definitions inline (verified self-contained in POC-Z8d). Drop the Hybrid-preamble option for Phase 4 to reduce the emitted surface. The emit↔loader helper-ownership boundary firms up when the Phase 5 loader lands. Supersedes OQ-9's optionality.
- **Alternatives:** (a) Keep Hybrid as an option (rejected — extra surface for no SW-01/SW-02 benefit); (b) fully inline every op (rejected — logic duplication per profile).
- **Impact:** Low — emit-strategy only, reversible.
- **Confidence:** MEDIUM-HIGH. Addresses review MEDIUM (OQ-9 pinned).

## OQ-19: SetOption apply-side live-option capture (from REVIEWS cycle-2 CH-9)

- **Question:** CH-7 moved `was_on` to a runtime-captured fact and specified the DEACTIVATE side (`RestoreOption` reads the slot), but the APPLY per-op rendering (SetScalar/AddAlias/AddFunc capture rules) OMITTED `SetOption` — nothing captured the live option before `setopt`/`unsetopt`, so `RestoreOption` reads an unwritten slot and option restore silently fails. Add a `SetOption` apply-capture rule?
- **Tentative choice (applied):** YES — add a `SetOption` apply rule to emit.go (Plan 04-02 Task 1) mirroring SetScalar/AddAlias: a `${+slot}`-guarded idempotent capture of the LIVE option state via the zsh live-option test `[[ -o optname ]]` into a per-switch was_on slot, emitted BEFORE the `setopt`/`unsetopt`. Deactivate's `RestoreOption` reads that captured slot and restores the exact prior. A functional option-drift fixture is added to the residue property test (Task 2), and a NEW EVIDENCE claim C27 (live option capture/restore round-trip) is logged UNVERIFIED for pass-2.
- **Alternatives:** (a) leave option capture to the Phase 5 loader (rejected — RestoreOption is emitted here and would read an unwritten slot, a zero-residue violation on the options class shipped in this phase); (b) author was_on into the manifest (rejected — D-06/CH-7 forbid the builder authoring was_on).
- **Impact:** HIGH for the options class — without it, every option restore silently no-ops. Neutralized by the apply-capture rule + fixture + C27.
- **Confidence:** HIGH (the fix mirrors the proven SetScalar/AddAlias capture; the round-trip is C27 UNVERIFIED pending pass-2). Addresses review concern CH-9.

## OQ-20: Full-env snapshot self-stability — volatile-param allowlist + fd-capture (from REVIEWS cycle-2 CH-10)

- **Question:** The CH-2 full-env `${(@kv)parameters}` instrument iterates ALL params including volatile specials (SECONDS, RANDOM, LINENO, funcstack, pipestatus, `_`) and the harness's own snapshot/loop vars, so two identical snapshots differ (false-red) — or an unaudited filter re-opens the C19 false-green. How is the instrument made self-stable without blinding it?
- **Tentative choice (applied):** Define an AUDITED, documented exclusion allowlist of volatile/read-only special params + named harness vars (or snapshot only user-scope params via a `typeset +`-style filter), and capture each snapshot by REDIRECTING to a temp file / dedicated fd — NOT `$(...)` command-substitution (which itself forks and perturbs `_`/pipestatus/funcstack). A self-stability meta-test asserts two consecutive no-op snapshots diff to EMPTY under the SAME allowlist that still lets the leak/value-change meta-tests fire. NEW EVIDENCE claim C28 logged UNVERIFIED for pass-2.
- **Alternatives:** (a) blanket-filter the whole namespace (rejected — masks real residue, re-opens C19); (b) name-only snapshot (rejected — that IS the C19 false-green CH-2 exists to close).
- **Impact:** HIGH — a non-self-stable instrument false-reds every run; an over-broad allowlist false-greens a real leak. The dual meta-test (self-stability AND leak-fire under one allowlist) is the guard.
- **Confidence:** MEDIUM-HIGH (the exact allowlist membership is a pass-2 detail; the mechanism — audited allowlist + fd-capture — is sound). C28 UNVERIFIED. Addresses review concern CH-10.

## OQ-21: Deactivate = full rebuild-to-base; quoted-RHS element removal is reserved (from REVIEWS cycle-2 CH-11+CH-12)

- **Question:** CH-1c defined deactivate's list op as pure rebuild-to-base (`PATH="$ZP_BASE_PATH"`), but the plan ALSO mandated the quoted-RHS per-element removal loop (CH-6) and cited C25 + a metacharacter fixture against it. In single-active v2.0 deactivate has no `$target` to remove — the loop is dead code. Reconcile?
- **Tentative choice (applied):** State deactivate list reversal is a FULL rebuild-to-base and REMOVE the per-element quoted-RHS removal loop from the deactivate op rendering. DOCUMENT the quoted-RHS `[[ $e == "$target" ]]` form (C25 PROVEN) in an emit.go comment as the CORRECT form RESERVED for element-level removal WHEN needed (future `ListDelta.Deletions` / multi-managed) — currently unexercised, exactly like `ListDelta.Deletions` (04-01). With a runtime-VARIABLE `$target` and GLOB_SUBST off (zsh default), quoting is a robustness/GLOB_SUBST guard, NOT the sole barrier — do NOT present a functional test as pinning the C10 fix. Change the metacharacter residue sub-check to assert base-restore DROPS a profile-added `/opt/tool*` (an addition absent from base), which is what actually happens.
- **Alternatives:** (a) keep the removal loop on the deactivate path (rejected — dead code in single-active v2.0, and the CH-6 functional test was non-discriminating per CH-11); (b) drop the quoted-RHS form entirely (rejected — it is the correct reserved form for future deletion, worth documenting).
- **Impact:** Medium — removes dead code (simplicity) and corrects a non-discriminating test; the metacharacter fixture now asserts the real single-active behavior.
- **Confidence:** HIGH (dictated by single-active v2.0 + C25 PROVEN). Addresses review concerns CH-11, CH-12.

## OQ-22: Balanced switch-sequence generator (from REVIEWS cycle-2 CH-14)

- **Question:** If the residue harness generates two applies without an intervening deactivate (A then B both shadow `ll`), B captures A's `ll` (not base's) → deactivate-B restores A's `ll` → residue vs base (a harness artifact, not an emit bug). How is the harness constrained?
- **Tentative choice (applied):** The residue property-test generator produces only BALANCED sequences — model the real single-active checkout as deactivate-current-then-activate-next, so each apply is matched by its deactivate before a different profile applies (at most one active profile at any point). Add a generator meta-check (pure Go, no zsh) asserting the invariant on every generated sequence: two applies of different profiles never nest without an intervening deactivate.
- **Alternatives:** (a) guard emit's apply to re-capture correctly when a different profile is already active (rejected for Phase 4 — that is the multi-active concern deferred with OQ-13; single-active v2.0 never nests applies); (b) leave the generator unconstrained (rejected — false-red residue from a harness artifact).
- **Impact:** Medium — an unbalanced generator produces false-red residue; the balanced generator + invariant meta-check models the real runtime.
- **Confidence:** HIGH (single-active v2.0 checkout is inherently balanced). Addresses review concern CH-14.

## OQ-23: Base-ownership sub-check strengthened past tautology (from REVIEWS cycle-2 CH-15)

- **Question:** The rescoped base-ownership sub-check asserted `/usr/local/bin` (base + profile-added) "survives" deactivate — trivially true for ANY base entry regardless of whether the profile's addition was mishandled; it did NOT prove ownership-awareness. Strengthen?
- **Tentative choice (applied):** Strengthen the sub-check to assert, after apply→deactivate: (a) `$#path` is BYTE-IDENTICAL to baseline (guards strip AND duplicate) AND (b) `/usr/local/bin` appears EXACTLY once (guards strip and duplicate). Add a THIRD negative-control mutant (a renderList variant that strips base entries) that MUST fail this sub-check — so the sub-check is proven discriminating, not tautological.
- **Alternatives:** (a) keep the "still present" assertion (rejected — tautology); (b) assert only exactly-once (rejected — misses a compensating strip+duplicate that keeps count wrong).
- **Impact:** Medium — a tautological sub-check gives false confidence; the strengthened count+exactly-once + base-strip mutant makes it a real ownership guard.
- **Confidence:** HIGH. Addresses review concern CH-15.
