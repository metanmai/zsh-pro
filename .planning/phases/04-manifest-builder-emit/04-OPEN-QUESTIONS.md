---
phase: 04-manifest-builder-emit
updated: 2026-07-01T18:00:00Z
open_count: 7
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
- **Confidence:** MEDIUM (leans uniform-map for code simplicity; the fixture literally shows an array for functions). Resolve during plan-phase against the exact round-trip acceptance reading.

## OQ-5: Emitted reverse logic — call shared loader helpers vs inline per-op

- **Question:** Should `emit.go` emit calls to a small fixed set of runtime helper functions (`zp_capture_env`/`zp_restore_env`/PATH-rebuild/shadow-restore, defined once by the Phase 5 loader) or inline the full reverse logic into every emitted apply/deactivate block?
- **Tentative choice (applied):** Emit **calls to a small fixed helper set** (the `zp_*` functions from the Phase 1 reference snippet), so the per-plan emitted surface is minimal and the tricky drift-guard/`${(P)+var}` logic lives in one audited place. The helper definitions themselves are a Phase 4↔5 seam: Phase 4 emits code that calls them; whether Phase 4 also emits the helper definitions (self-contained block) or assumes the Phase 5 loader defines them is finalized in plan-phase.
- **Alternatives:** (a) Inline every reverse op fully (self-contained emitted block, no runtime dependency — heavier emitted text, logic duplicated per profile); (b) hybrid — emit helpers once per apply block, calls thereafter.
- **Why uncertain:** The split between "code Phase 4 emits" and "helpers the Phase 5 loader owns" is a seam boundary that firms up when the loader lands. For the Phase 4 property test to run standalone, the test harness must supply the helper definitions (or emit.go must emit a self-contained block) — a plan-phase detail.
- **Impact:** Medium — affects emitted-code size, the emit↔loader seam contract, and how self-contained the property test's `zsh -f` input is. Reversible: helper-call vs inline is an emit-strategy change, not a data-model change.
- **Confidence:** MEDIUM-HIGH (helper-calls match the Phase 1 reference snippet and keep the audited logic in one place). Flagged so plan-phase pins the Phase 4↔5 helper-ownership boundary.

## OQ-6: Injection-safe escaping — static-quoted vs dynamic-verbatim split (T-01-06)

- **Question:** User-controlled values (env values, alias/function bodies) must be injection-safe in the `eval`'d emitted code, but Phase 2/EVAL-01 keeps dynamic values (`$HOME/go`, `$(...)`) late-bound and `regen.go` emits them verbatim (unquoted). How does emit.go reconcile "escape everything" (injection safety) with "emit dynamic verbatim" (portability)?
- **Tentative choice (applied):** Split on the `Entry.Dynamic` flag the IR already carries. **Static** values → hard single-quote with the zsh `'\''` escape (a value with `'`/`;`/`$(...)`/backtick/newline becomes an inert literal string, never command position). **Dynamic** values → emit verbatim so zsh expands per-machine at apply time; this is safe because the value came from a parsed, non-executed AST field (structured codegen), not from `eval` of raw user config. Never partial-eval user config.
- **Alternatives:** (a) Quote EVERYTHING including dynamic values (breaks EVAL-01 portability — `$HOME` would become the literal string `$HOME`); (b) an allowlist of permitted dynamic constructs (`$VAR`, `${...}`, `$(...)`) with everything else quoted — tighter, but needs its own parser and a proof that the allowlist can't be escaped.
- **Why uncertain:** This is the crux of threat T-01-06 (deferred from Phase 1 to here). Emitting a dynamic value verbatim means a maliciously-crafted `$(rm -rf ~)` in a profile WOULD execute at apply — but that value was authored by the profile owner and is exactly the late-binding EVAL-01 promises (equivalent to it being in their `.zshrc`). The residual risk is a value that is *classified* dynamic but should have been static/quoted, or a shared/imported profile (SHARE-01, a future milestone with its own trust gate). The safe default trusts the owner's own profile (same trust boundary as their `.zshrc`) and quotes everything not explicitly dynamic.
- **Impact:** HIGH (security) but the trust boundary is the profile owner's own config. A wrong split either breaks portability (over-quoting) or, in a future shared-profile world, is an injection vector (under-quoting) — SHARE-01's trust gate is the future backstop. For v2.0 (own profiles only), quote-static/verbatim-dynamic matches the `.zshrc`-equivalence trust model.
- **Confidence:** MEDIUM (the static/dynamic split is well-grounded in the existing `Dynamic` flag and EVAL-01, but the exact escape function and its test vectors — `'`, `;`, `$(...)`, newline, `'\''` chains — must be verified against the zsh manual + shadowenv source, ROADMAP research flag). Plan-phase must write explicit injection test vectors.

## OQ-7: Introspected function-body encoding for multi-line bodies

- **Question:** The existing `introspectScript` is line-oriented (one name per line, section-delimited). Function bodies (`${functions[name]}`) are multi-line by nature. How are multi-line alias/function bodies encoded so parsing stays robust and the existing name-only parse is untouched?
- **Tentative choice (applied):** Dump bodies in a NEW section (e.g. `##FUNC_BODIES##`) using a delimiter format that survives embedded newlines — e.g. a `name` line followed by a length-prefixed or sentinel-terminated body block, or NUL/record-separator delimiting — rather than the naive `name\tbody`-on-one-line (which a newline in the body would break). Verify the chosen encoding with a throwaway POC (a function with a newline + a `\t` + a `'` in its body) before finalizing the parser. Existing `##ALIASES##`/`##FUNCTIONS##` name sections stay exactly as-is (additive, D-14/D-15).
- **Alternatives:** (a) `name\tbody` single-line (simple, but breaks on any body newline — most function bodies have newlines, so this is likely wrong); (b) base64-encode each body (newline-safe, trivially parseable, but opaque in fixtures/diffs); (c) `print -rN` NUL-delimited records parsed by splitting on NUL.
- **Why uncertain:** zsh's `${functions[name]}` returns the body with its original newlines/indentation; the encoding must round-trip that byte-for-byte (the shadow-restore acceptance is "byte-identical prior body"). The exact zsh dump construct (`print -r`, `print -rN`, `typeset -f`) and the Go-side split need a POC to confirm no truncation/mangling.
- **Impact:** Medium — a broken encoding silently corrupts a shadowed function body, failing the byte-identical shadow-restore acceptance (SPEC req 7). Reversible: the encoding is internal to introspect+emit; changing it is localized.
- **Confidence:** MEDIUM (NUL/sentinel-delimited is the clearly-correct direction; the exact construct needs a POC). Flagged so plan-phase POCs the multi-line body round-trip before wiring the parser.
