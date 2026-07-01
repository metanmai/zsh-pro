---
phase: 04-manifest-builder-emit
updated: 2026-07-01T14:13:44Z
open_count: 12
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
- **Tentative choice (applied):** Emit **calls to a small fixed helper set** (the `zp_*` functions from the Phase 1 reference snippet), so the per-plan emitted surface is minimal and the tricky drift-guard/`${(P)+var}` logic lives in one audited place. The helper definitions themselves are a Phase 4↔5 seam: Phase 4 emits code that calls them; whether Phase 4 also emits the helper definitions (self-contained block) or assumes the Phase 5 loader defines them is finalized in plan-phase.
- **Alternatives:** (a) Inline every reverse op fully (self-contained emitted block, no runtime dependency — heavier emitted text, logic duplicated per profile); (b) hybrid — emit helpers once per apply block, calls thereafter.
- **Why uncertain:** The split between "code Phase 4 emits" and "helpers the Phase 5 loader owns" is a seam boundary that firms up when the loader lands. For the Phase 4 property test to run standalone, the test harness must supply the helper definitions (or emit.go must emit a self-contained block) — a plan-phase detail.
- **Impact:** Medium — affects emitted-code size, the emit↔loader seam contract, and how self-contained the property test's `zsh -f` input is. Reversible: helper-call vs inline is an emit-strategy change, not a data-model change.
- **Confidence:** MEDIUM-HIGH (helper-calls match the Phase 1 reference snippet and keep the audited logic in one place). Flagged so plan-phase pins the Phase 4↔5 helper-ownership boundary. **See OQ-9** for the self-contained-block question surfaced during research.

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
- **Confidence:** MEDIUM → **RESOLVED-DIRECTION (04-RESEARCH, POC-Z3/Go-RT):** alternative (c), NUL-delimited `print -rN -- name body` records, is VERIFIED to round-trip a body containing `\n`+`\t`+`'`+`$`+`;` byte-for-byte (zsh dump → Go split-on-`\x00` → back into `functions[name]=`). NUL is safe because a zsh string cannot contain NUL. The naive `name\tbody` (alt a) provably breaks (embedded `\n`). Parser must slice the section by header/`##END##` offsets and split the enclosed bytes on NUL (not scan line-by-line, since bodies may contain `#`).

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
- **Tentative choice (applied):** For Phase 4, the property-test harness supplies the `zp_*` helper definitions inline (verified working self-contained in POC-Z8d), so `emit.go` can emit bare calls per the OQ-5 default. Recommend the planner ALSO consider a Hybrid mode (emit a helper-definition preamble once per block) as a low-cost hedge that makes emitted code self-contained without waiting for Phase 5 — decide during plan-phase.
- **Alternatives:** (a) Bare calls only, harness supplies helpers (current default); (b) Hybrid — emit helper defs + calls; (c) fully inline every op (OQ-5 alt a).
- **Impact:** Medium — affects whether the emitted string is runnable in isolation and the emit↔loader seam contract. Reversible (emit-strategy, not data-model).
- **Confidence:** MEDIUM (a seam-boundary question that firms up when the Phase 5 loader lands; both shapes are proven runnable).

## OQ-10: Runtime undo-slot naming must produce valid zsh identifiers

- **Question:** The emitted apply code stores captured live priors in `typeset -g` globals whose names embed the profile name and the alias/function/var name (e.g. `ZP_<profile>_PRIOR_ALIAS_<name>`, `__ZP_ORIG_<var>`). A profile named `feature/x`, or an alias/var name containing characters illegal in a zsh identifier, would make `typeset -g` fail. How are slot names derived so they are always valid `[A-Za-z0-9_]` identifiers?
- **Tentative choice (applied):** `emit.go` sanitizes the `profile` + target name into a valid identifier before composing the slot name — e.g. replace every non-`[A-Za-z0-9_]` byte with `_`, or (to avoid collisions between `a/b` and `a_b`) append a short hash of the raw name. Env var names are already valid identifiers by construction (they come from `KindAssignment` names), so `__ZP_ORIG_<var>` is safe; the collision risk is in the profile-name and alias-name segments.
- **Alternatives:** (a) Store priors in a single associative array keyed by the raw (arbitrary) name — `typeset -gA ZP_PRIOR_ALIAS; ZP_PRIOR_ALIAS[$rawname]=...` — which sidesteps identifier rules entirely and is arguably cleaner; (b) reject/skip profiles with unsanitizable names (too restrictive).
- **Impact:** Medium — an unsanitized name aborts apply with a `typeset` error. Alternative (a), an assoc-array keyed by raw name, is likely the more robust design and worth the planner's consideration over slot-name-per-global.
- **Confidence:** MEDIUM (the hazard is real; the assoc-array alternative may be strictly better than name-sanitization — a plan-phase design call).

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
