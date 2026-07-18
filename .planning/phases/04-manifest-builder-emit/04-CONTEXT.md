# Phase 4: Manifest Builder + Emit - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning
**Mode:** Autonomous smart-discuss (`--auto`, unattended). Grey-area answers were auto-decided from prior phase decisions > codebase patterns > domain conventions > ROADMAP criteria. Medium/low-confidence calls are logged in `04-OPEN-QUESTIONS.md` (OQ-4+). No human was asked.

<domain>
## Phase Boundary

Turn a resolved `model.Profile` (Phase 2 IR, persisted by the Phase 3 store) into the reversible runtime record and the zsh code that applies/reverses it with **zero residue**:

1. **`model.Manifest`** — a shell-agnostic reversible record in `core/model` (no new deps) carrying exactly the four part types the ROADMAP names: `Scalar` (env), `ListDelta` (PATH/FPATH), `NameSet` (aliases/functions: `added` + `shadowed` prior bodies), `OptionSet` (`enabled` + `was_on`), plus a `schema` tag and `profile` (branch name). Round-trips losslessly to/from the Phase 1 `01-MANIFEST-SHAPE.md` JSON.
2. **`core/activate`** (new package) — builds a `Manifest` from a `Profile`'s `EffectiveManaged()` declarative entries (imperative entries excluded by construction), diffs active-vs-target, and produces a **shell-agnostic** ordered `Plan` value (deactivate-of-prior **then** activate-of-target) expressed in agnostic ops. Contains **no** zsh token.
3. **`core/shell/zsh/emit.go`** (new file) — the **sole** place apply/deactivate zsh syntax is generated (`unalias`/`unset -f`/`setopt`/`unsetopt`/PATH-rebuild), rendering a `Plan` to **plain** loader functions (no `emulate -L`/`LOCAL_OPTIONS`) a sourced loader `eval`s. Emitted code is `zsh -n`-valid and injection-safe (Phase 1 threat T-01-06).
4. **Introspection extension** — `introspectScript` + `model.IdentitySet` (additively) dump alias/function **bodies** so shadowed prior definitions are recoverable; graceful degradation (`Available:false`) preserved.
5. **Zero-residue property test** — N ≥ 20 random switch sequences under sandboxed `zsh -f`, asserting a byte-identical six-class snapshot regardless of order; a mutated emitter must make it fail (the SW-02 regression pin).

**In scope:** the `Manifest` + four parts; the builder; the agnostic diff/`Plan`; the single zsh emit path; drift-guarded env restore (`${(P)+var}==1`, reverse-only-if-`live==applied`); ownership-aware PATH-delta restore vs a captured base; shadow capture + byte-for-byte restore; body-dumping introspect; injection-safe emission; the property test.

**Not in scope (later phases):** the sourced runtime loader itself, `checkout`/`activate`/`deactivate`/`list`/`status` CLI verbs, per-terminal state wiring, and the `.zshrc` bootstrap block (Phase 5); the PATH base-capture *placement* in the live loader (Phase 5 — Phase 4 emits a plan assuming a base exists in runtime state, OQ-3); secret **deref-on-switch** (Phase 5 runtime half of PROF-03 — Phase 4 may define the seam only); keybindings/hooks/`compinit` (master block, OQ-1); real `~/.zshrc` end-to-end ingest (Phase 6); other shells (zsh-only milestone).

</domain>

<decisions>
## Implementation Decisions

### Grey Area 1 — `model.Manifest` Go field layout & JSON contract

- **D-01: Manifest wire ownership** — `Manifest` and its four parts live in a new `core/model/manifest.go`, stdlib-only, tagged directly with `json:` struct tags. Unlike `model.Profile` (which is serialized via a store-local `entryDTO` because `Entry` is untagged, `core/store/dto.go`), the Manifest IS the wire record (Phase 1: "serialized into the per-profile activation state"), so it carries its own JSON tags. This is the safest, most direct match to the validated `01-MANIFEST-SHAPE.md` JSON and to `SecretRef` (which is itself directly `json:`-tagged in `core/model/secretref.go`). No DTO indirection layer for the Manifest.
- **D-02: Field/JSON layout mirrors `01-MANIFEST-SHAPE.md` exactly** — top-level `profile` (string) + `schema` (string) + `env []Scalar` + `lists []ListDelta` + `aliases NameSet` + `functions NameSet` + `options []OptionSet`. Part shapes:
  - `Scalar{ Name string \`json:"name"\`; Applied string \`json:"applied"\`; Original *string \`json:"original,omitempty"\` }` — **`Original` is a `*string`**, because presence-vs-absence of the key encodes was-set-vs-was-unset AND a present `""` encodes was-empty (Phase 1 Pattern 4). A `*string` with `omitempty` is the simplest/recommended Go shape that distinguishes all three states (nil→unset, &""→empty, &"vim"→value); a plain `string` cannot (C15: other shapes such as `json.RawMessage` also distinguish the three, but `*string`+`omitempty` is the recommended one). (This is the single load-bearing type choice in the manifest.)
  - `ListDelta{ Name string; Additions []string; Deletions []string }` — matches `lists[].name/additions/deletions`.
  - `NameSet{ Added map[string]string \`json:"added"\`; Shadowed map[string]string \`json:"shadowed"\` }` — one type for both aliases and functions. `Added` is name→body for aliases (shape shows `{"gs":"git status"}`) and, per the validated shape, functions' `added` is a JSON array of names. **Discrepancy resolved:** use `map[string]string` for both `Added` and `Shadowed` uniformly (functions' body in `Added` may be `""` when only the name is needed) — logged as **OQ-4** because the validated shape shows `functions.added` as an array; the safe reversible default (uniform map, body captured when available) is applied.
  - `OptionSet{ Name string; Enabled bool; WasOn bool }` — matches `options[].name/enabled/was_on` (`json:"was_on"`).
- **D-03: `schema` tag value is `"v1"`** — the exact literal in `01-MANIFEST-SHAPE.md`. It is a forward-compat gate checked before reverse logic runs (shadowenv precedent). No versioning machinery beyond the constant this phase.
- **D-04: JSON tags are the snake/lower keys from the validated shape** (`profile`, `schema`, `name`, `applied`, `original`, `additions`, `deletions`, `added`, `shadowed`, `enabled`, `was_on`) — NOT the store DTO's camelCase (`startLine`, `cmdName`). The manifest's contract is the Phase 1 shape, not the profile.json contract; matching the validated fixture keys is what the round-trip acceptance test asserts.

### Grey Area 2 — `core/activate` builder, diff, and the agnostic `Plan`

- **D-05: `core/activate` is a new package; it imports `core/model` only** — never `core/shell/zsh` (single-composition-root invariant, `core/cmd/zsh-pro/main.go`). It exposes a builder (`Profile → Manifest`), a differ (active `Manifest` + target `Manifest` → `Plan`), and the `Plan` value type. Mirrors how `core/ir` and `core/store` stay shell-agnostic and reach zsh only through an injected seam.
- **D-06: The builder consumes only `Entry.EffectiveManaged() == true` entries and classifies each by `Category` + `Kind` into a part** (reusing the exact routing the Phase 2 `core/ir/route.go` already proved): `CatEnvironment`/`CatSecrets` scalar assignment → `Scalar`; `CatPath` assignment → a `ListDelta` entry (PATH or FPATH by name); `KindAlias` → `aliases.Added`; `KindFuncDecl` → `functions.Added`; `setopt`/`unsetopt` `KindCommand` → `OptionSet`. Imperative/`OverrideUnmanaged`/`Opaque` entries produce **no** part (acceptance: an imperative entry yields nothing). Building the manifest does NOT capture `shadowed` prior bodies or `was_on` — those are **live** facts captured at apply time by the emitted code, not authored into the manifest by the builder (Phase 1 loader-reality; OQ-2). The builder fills `added`/`applied`/`enabled`/`additions` (declarative intent) and leaves `shadowed`/`original`/`was_on` as the runtime-reconciled slots.
- **D-07: PATH/FPATH segmentation into `additions`/`deletions` happens HERE** (deferred to Phase 4 by Phase 2 D-03). Phase 4's builder is where the "captured base" concept exists; a `CatPath` entry's value is split into additions (and, if a delta model surfaces deletions, deletions) vs the base. The base itself is **runtime state**, not manifest data (OQ-3 / Phase 1: `ZP_BASE_PATH` is machine/session-specific). The manifest carries the *delta*; the emitted code rebuilds from the runtime-held base.
- **D-08: The `Plan` is a Go value type of ordered agnostic operations — never shell text.** Op vocabulary is agnostic verbs, e.g. `RestoreScalar`/`UnsetScalar`, `RebuildListFromBase`, `Unalias`/`RestoreShadowedAlias`, `UnsetFunc`/`RestoreShadowedFunc`, `RestoreOption` (deactivate side) and `SetScalar`, `ApplyListDelta`, `AddAlias`/`AddFunc`, `SetOption` (activate side). Ordering: **all of active(A)'s reverse ops before all of target(B)'s apply ops** (deactivate-then-activate). Acceptance: a **precise** check (case-sensitive, word-boundary, string-literals only — excluding Go identifier type names like `Unalias`/`UnsetFunc` and comments; C22) confirms `core/activate` contains no zsh reverse-op token as an emitted string literal.
- **D-09: The empty-target and empty-active cases are first-class** — `diff(active=A, target=nil/empty)` is a pure deactivate (used by Phase 5's `deactivate` verb); `diff(active=nil/empty, target=B)` is a pure activate (first `checkout`). The switch case is `diff(A, B)`. All three flow through the same `Plan` builder so the property test can drive arbitrary sequences.

### Grey Area 3 — `core/shell/zsh/emit.go` function structure & the single-emit-path invariant

- **D-10: `emit.go` renders a `core/activate.Plan` to two zsh strings** (or one block with two functions): an **apply** function body and a **deactivate** function body, both **plain** (no `emulate -L`, no `LOCAL_OPTIONS`, no `setopt localoptions`) so `setopt`/`unsetopt` escape function scope (Phase 1 Pitfall 1 / carry-forward 3). It reaches the `Plan` via a new seam on the `shell` package (an `Emitter` interface, mirroring `shell.Regenerator`), so `core/activate` and any caller depend on the interface, not the concrete zsh package. Wiring lands at the composition root (`main.go`), exactly like the Regenerator (`core/cmd/zsh-pro/main.go`).
- **D-11: Reverse-op zsh tokens are generated ONLY under `core/shell/zsh/emit.go`.** `regen.go` stays forward-only (its doc comment already scopes it to forward syntax). The milestone invariant test greps the whole tree for `unalias`/`unset -f`/`unsetopt`/PATH-array-rebuild and asserts they appear only in `emit.go` (and its `_test.go` fixtures). `core/activate` and `core/model` stay token-free.
- **D-12: The emitted apply code captures the LIVE prior into per-terminal runtime undo state** (`__ZP_ORIG_*` / `ZP_*_PRIOR_*`-style globals, `typeset -g`), NOT the manifest's `original` (Phase 1 carry-forward 1 / OQ-2). The emitted deactivate code is the drift-guarded, unset-vs-empty-correct reverse from the Phase 1 reference snippet (`01-FINDINGS.md` §"Loader Reference Snippet"), reproduced **with the OQ-8 correction, NOT verbatim** — the snippet's shadow-restore guards are buggy: `zp_capture_env`/`zp_restore_env` using `${(P)+var}==1` **where `var` holds the env NAME** (this indirect form IS correct — C2/C21 — because `(P)` dereferences value-as-name); shadow restore of aliases/functions guarded by a literal-name set-test **`${+name}` — NOT the snippet's `-n` or `${(P)+literalName}` guards** (which silently drop an empty prior / test the wrong parameter, OQ-8/C1); `alias name=$prior` / `functions[name]=$prior` to re-establish the captured prior (a function body is live code, restored by verbatim capture-and-reassign, never single-quote-wrapped — C6); `unalias`/`unset -f` for added names; `PATH="$ZP_BASE_PATH"; path=(<additions> $path)` for the list rebuild. Phase 4 emits the code shaped for that runtime contract; the runtime helpers' *placement* is Phase 5 (OQ-3). Whether `emit.go` emits calls to shared `zp_*` helpers or inlines them per-op is **Claude's discretion** (logged as OQ-5; safe default = emit calls to a small fixed helper set that the loader defines, minimizing per-plan emitted surface).
- **D-13: Injection safety uses single-quote wrapping with the zsh `'\''` escape for every user-controlled VALUE context** (env scalar values and alias bodies) — the standard shell-safe quoting for a value going into `eval`'d code. **Function bodies are the exception (C6):** a zsh function body is executable code by nature and CANNOT be rendered as an inert single-quoted literal (calling the function runs the body; adversarial single-quotes break the definition with "invalid function definition"). Function restore is therefore verbatim capture-and-reassign — `functions[name]=$capturedBody`, where the captured body round-trips as *data* via the NUL-delimited body dump (D-14/C14) — a distinct mechanism, not single-quote wrapping. A value containing `'`, `;`, `$(...)`, backtick, or newline is wrapped as `'val'\''more'` so it can only ever be a literal string, never break out into command position. Dynamic-by-intent values are the tension here: Phase 2/EVAL-01 keeps `$HOME/go`/`$(...)` late-bound and `regen.go` emits them **verbatim** (unquoted) on purpose. **Resolution:** the manifest/emit path distinguishes the two using the `Dynamic` flag the IR already carries — a **static** value is hard-single-quoted (injection-safe); a **dynamic** value stays verbatim so zsh expands it per-machine (the value came from a parsed, non-executed AST, so it is codegen from a structured field, not `eval` of user config). This is the crux of T-01-06 and is logged as **OQ-6** (medium confidence on the exact static-vs-dynamic escaping split) with the safe default applied: quote static, verbatim dynamic, and never `eval` raw user config.

### Grey Area 4 — Introspection body-dump & the zero-residue property test harness

- **D-14: Extend `introspectScript` additively** — after each `##ALIASES##`/`##FUNCTIONS##` name dump, also dump the body via `${aliases[name]}` / `${functions[name]}`. Emit bodies in a **new section-delimited block** (`##ALIAS_BODIES##` / `##FUNC_BODIES##`) using a **multi-line-safe framing — NUL-delimited `print -rN -- name body` records (or a length-prefixed encoding), NOT a `name\tbody` line format** (C14/C23: a line/tab format truncates multi-line function bodies — an embedded newline splits one record across physical lines and the existing line-oriented `parseIntrospect` reader mis-parses it). The body block is parsed by a NEW multi-line-safe reader, NOT the existing line reader, so the existing name-only parse and its tests keep passing unchanged (acceptance: name-only tests still pass). Preserve the exact `zsh -f -c` + 5s-timeout + `Available:false`-on-error shape (`core/shell/zsh/introspect.go`).
- **D-15: Carry bodies in an ADDITIVE companion on `model.IdentitySet`** — add `AliasBodies map[string]string` and `FunctionBodies map[string]string` fields (leaving the existing `Aliases`/`Functions map[string]bool` presence maps intact) rather than changing the existing map types. Additive is non-breaking (`analyze` consumes only `Available` today, `core/shell/zsh/introspect.go:86-90`); no existing caller breaks. The multi-line function body newline hazard is handled by the NUL-delimited (or length-prefixed) body encoding (D-14 — a `\t`-delimited line format was REFUTED as truncating multi-line bodies, C14/C23); logged as **OQ-7** with the safe default = a NUL-delimited framing that survives embedded newlines/tabs/quotes, POC-confirmed (C14) before the parser is finalized.
- **D-16: The zero-residue property test lives in a zsh-requiring test** that follows `core/store`/`core/ir` roundtrip precedent: `exec.CommandContext(ctx, "zsh", "-f", ...)` + `exec.LookPath("zsh")` skip-guard (skips cleanly when zsh is absent, like `corpus_test.go`/`introspect_test.go`). It builds ≥2 profiles → manifests → plans, renders apply/deactivate via `emit.go`, sources them under `zsh -f`, snapshots the six classes via the (extended) `introspectScript`, and asserts a **literal empty diff** of pre-vs-post snapshots across N ≥ 20 random switch orders, with `$#path` element count stable. It asserts on the snapshot **string equality** (the Phase 1 bar), not on `IdentitySet` field checks. A deliberately mutated emitter (append PATH instead of rebuild) must flip it to fail — a `//go:build`-tagged or in-test mutation proves the test detects residue (SW-02 acceptance).
- **D-17: The property test is the SW-02 regression pin and must not regress `make check`.** It composes alongside (never replaces) the `core/testgen` oracle pin. It runs in the default suite (skipping when zsh is absent), like the existing zsh-requiring tests — NOT behind a `spike` build tag (that tag was Phase 1 throwaway isolation; this is a durable pin). Whether N is a fixed constant vs a `testing.Short()`-scaled value is Claude's discretion (safe default: fixed N=20, honoring `-short` by reducing but keeping the mutated-emitter negative check).

### Claude's Discretion

- Exact Go identifiers and package internals for `core/activate` (builder/differ/`Plan`/op-type names and whether ops are a tagged union of structs vs an enum+payload), provided the agnostic-value + no-zsh-token + deactivate-then-activate-ordering constraints hold (D-05/D-08).
- The exact `shell.Emitter` seam signature (single `Emit(Plan) (apply, deactivate string, err error)` vs two methods) and where it sits in `core/shell/provider.go`, provided the single-emit-path invariant (D-11) and composition-root wiring (D-10) hold.
- Whether `emit.go` emits calls to a small fixed set of runtime helper functions (`zp_capture_env`/`zp_restore_env`/etc., Phase-5-defined) or inlines the reverse logic per op — provided the emitted code is plain, `zsh -n`-valid, drift-guarded, and injection-safe (OQ-5; default = emit helper calls).
- The exact body-dump section-delimiter format and `IdentitySet` companion field names (D-14/D-15), provided it is additive, name-only tests still pass, and multi-line function bodies survive the encoding (OQ-7).
- Property-test internals: profile fixtures used, N-scaling under `-short`, and the mutated-emitter mechanism (in-test injection vs build tag), provided N ≥ 20 and the mutation genuinely fails the test (D-16).
- Commit granularity and message conventions within the phase (project uses the GSD `gsd-sdk query commit` flow).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase contract
- `.planning/phases/04-manifest-builder-emit/04-SPEC.md` — the 8 locked requirements, boundaries, constraints, and acceptance criteria (HOW is this CONTEXT's job; WHAT/WHY are locked there — do not re-open).
- `.planning/ROADMAP.md` §"Phase 4: Manifest Builder + Emit" (line 40, 125) — goal + SW-01/SW-02; §"Cross-Cutting" line 214 (injection / shadowenv-source verification research flag); line 215 (base-capture placement is Phase 5).
- `.planning/REQUIREMENTS.md` — **SW-01** (record-and-reverse manifest w/ drift guard; PATH as delta vs captured base) and **SW-02** (zero-residue under N switch sequences — the property-test pin).

### The literal input (load-bearing — read first)
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md` — the **validated `Manifest` JSON shape** that becomes `model.Manifest` verbatim; the round-trip acceptance test asserts against these exact keys. Note the `functions.added` = array vs `aliases.added` = map discrepancy (OQ-4) and the "Loader Reality vs `original`" finding (OQ-2).
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md` — the go/no-go; the 5 carry-forwards; the **Loader Reference Snippet** (`zp_capture_env`/`zp_restore_env`, plain fns, PATH-from-base, drift guard, `${(P)+var}==1`) that `emit.go` reproduces **with the OQ-8 shadow-guard correction** (shadow restore uses `${+name}` set-tests, not the snippet's buggy `-n`/`${(P)+literal}` guards).

### Inherited IR + store contracts
- `.planning/phases/02-ir-partial-evaluation/02-CONTEXT.md` — D-03 (PATH segmentation deferred *to this phase*), D-05 (declarative⊥dynamic — a dynamic value stays switchable and late-bound), D-06 (maximize managed surface), D-07 (`ManagedOverride` persists), D-10 (verbatim dynamic values).
- `.planning/phases/03-git-backed-store/03-CONTEXT.md` — D-10 (secret *deref* is Ph4/5; Phase 4 may define the seam only), D-13 (`ZSHPRO_PROFILE` per-terminal env var carries the profile name → the manifest's `profile` field / activation key), D-01 (the persisted Profile the builder reads via `store.Read`).

### Reusable engine code
- `core/model/profile.go` — `Profile`/`Entry`/`EffectiveManaged()`/`ManagedOverride` (the builder's input; entries carry `Dynamic`, `Category`, `Kind`, `Names`, `Value`, `Exported`, `Secret`).
- `core/ir/route.go` — the proven `routeManaged` declarative/imperative gate; the builder classifies by the same `Category`+`Kind` shapes (reuse the logic, don't re-derive the admitted set).
- `core/shell/zsh/regen.go` — the **forward-only** Regenerator; `emit.go` is its reverse-and-loader counterpart, NOT a modification of it. Note its verbatim-dynamic-value emission (the D-13/OQ-6 escaping tension).
- `core/shell/zsh/introspect.go` — `introspectScript` + `parseIntrospect` + the `zsh -f -c` subprocess shape (extend additively per D-14); `core/model/identityset.go` (add body companions per D-15).
- `core/shell/provider.go` — the ISP seam + the `Regenerator` interface `emit.go`'s new `Emitter` seam mirrors.
- `core/cmd/zsh-pro/main.go` — the sole composition root; the new `Emitter` wiring lands here alongside the existing `zsh.Provider{}` injection.
- `core/store/dto.go` — the JSON-serialization precedent (though the manifest tags itself directly per D-01, not via a DTO).
- `core/store/roundtrip_test.go`, `core/ir/roundtrip_test.go`, `core/shell/zsh/introspect_test.go` — the zsh-requiring, `LookPath`-guarded test precedent the property test (D-16) follows.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`core/ir/route.go` `routeManaged`** — the exact declarative/imperative gate (admitted `Category`+`Kind` shapes, with all the BL-01/BL-02/WR-01/WR-02/array/bare-command guards). The manifest builder classifies managed entries into parts using the same shape checks; reuse this logic rather than re-deriving which entries are admitted.
- **`core/shell/zsh/introspect.go` `introspectScript` + `parseIntrospect`** — the section-delimited dump + `zsh -f -c` + 5s-timeout + `Available:false`-on-error pattern. Extend additively for alias/function bodies (D-14) and reuse verbatim as the property test's snapshot instrument (D-16).
- **`01-FINDINGS.md` Loader Reference Snippet** — the durable reference `emit.go` reproduces: plain apply/deactivate fns, `zp_capture_env`/`zp_restore_env` with `${(P)+var}==1` (env path, `var` holds the name), PATH-from-base rebuild, shadow-body capture/restore (with `${+name}` guards per OQ-8, not the snippet's `-n`/`${(P)+literal}`), exact-`was_on` option reverse.
- **`core/shell/zsh/regen.go`** — the forward templater; precedent for structured-field → zsh-string codegen and for the verbatim-dynamic-value discipline (do NOT re-quote dynamic values).
- **`core/store` roundtrip/marshal tests + `core/store/dto.go`** — the deterministic-JSON + lossless-round-trip test pattern the `Manifest` marshal/unmarshal acceptance test follows.

### Established Patterns
- **Single composition root:** only `core/cmd/zsh-pro/main.go` imports `core/shell/zsh`. `core/activate` and `core/model` stay shell-agnostic; the concrete emitter is reached through a new `shell.Emitter` interface wired in `main.go` (mirrors `shell.Regenerator` / the store's injected regenerator, D-03).
- **`core/model` is dependency-free, stdlib-only.** `model.Manifest` adds no dependency; `SecretRef`'s direct `json:` tagging is the precedent for tagging the Manifest in-place (D-01).
- **Sandboxed subprocess with graceful degradation** (`zsh -f -c`, 5s timeout, `LookPath` skip-guard) is the pattern for the introspect extension and the property test.
- **TDD; the `testgen` oracle is the standing regression pin.** The new zero-residue property test is the SW-02 pin and composes alongside it; `make check` (fmt-check + vet + lint + test) stays green.
- **`*string` + `omitempty` for tri-state presence** — the `Scalar.Original` shape (nil/unset vs &""/empty vs &"val"/value) is the Go idiom for the Phase 1 unset-vs-empty encoding; no bool-flag companion needed.

### Integration Points
- **Input:** a `model.Profile` from `store.Read(<branch>)` (Phase 3) — the manifest builder's source. The `profile` manifest field = the branch name / `ZSHPRO_PROFILE` value (Phase 3 D-13).
- **Output consumed downstream (Phase 5):** the emitted apply/deactivate zsh code is what the sourced loader `eval`s; the `Plan`/`Manifest` are the values the loader's `checkout`/`activate`/`deactivate` verbs drive. Phase 4 emits the code and the plan; Phase 5 installs and invokes the loader. The `SecretRef` deref seam may be defined here (PROF-03 runtime half) but completes in Phase 5.
- **Runtime-state contract (Phase 4↔5 seam):** the emitted code assumes per-terminal runtime state exists — a captured `ZP_BASE_PATH` (base for PATH rebuild) and `__ZP_ORIG_*`/shadow-prior slots (written by apply, read by deactivate). Phase 4 emits code shaped for this contract; Phase 5 owns the base-capture *placement* and re-capture guard (OQ-3).

</code_context>

<specifics>
## Specific Ideas

- **Trust the live prior, not the static `original`** (Phase 1 carry-forward 1 / OQ-2): the manifest's `env[].original` is the declarative record of intent; the emitted apply captures the LIVE prior into runtime undo state, and the drift guard (`reverse only if live == applied`) reconciles a hand-edited value safely. This governs D-06 (builder does not author `shadowed`/`was_on`) and D-12 (emit captures live).
- **Injection safety is THE risk of this phase** (ROADMAP line 214, threat T-01-06): shell-code emission is where a quoting bug becomes a shell-injection bug. Static values are hard-single-quoted (`'\''`-escaped); dynamic values (`$HOME`/`$(...)`) stay verbatim because they are codegen from a non-executed AST field, not `eval` of user config — never partial-eval the user's config (D-13 / OQ-6). Verify quoting against the zsh manual + the shadowenv source (ROADMAP research flag).
- **PATH is a delta, never a wholesale overwrite** (Phase 1 Pitfall 2 / carry-forward 4): rebuild from a captured base + additions/deletions; never `typeset -U`, never blind append. The property test's `$#path`-stability check is the backstop (D-16).
- **Plain loader functions** (Phase 1 Pitfall 1 / carry-forward 3): apply/deactivate must NOT use `emulate -L`/`LOCAL_OPTIONS` or `setopt`/`unsetopt` auto-reverts at function return. This is a hard emit constraint (D-10), directly contradicting the `emulate -L zsh` line in `introspectScript` (which is correct THERE — introspection wants isolation; the loader wants escape).
- **The mutated-emitter negative test** is a first-class acceptance criterion: the property test must FAIL when the emitter appends PATH instead of rebuilding — i.e. it genuinely detects residue, not just asserts a happy path (SW-02, D-16).

</specifics>

<deferred>
## Deferred Ideas

- **The sourced runtime loader, `checkout`/`activate`/`deactivate`/`list`/`status` verbs, per-terminal state wiring, `.zshrc` bootstrap block** — Phase 5 (BOOT-01/02). Phase 4 emits the code a loader `eval`s; it does not install or invoke it.
- **PATH base-capture PLACEMENT in the live loader** (which entry point captures `ZP_BASE_PATH`, the re-capture guard) — Phase 5 (OQ-3). Phase 4 emits a plan assuming a base exists in runtime state.
- **Secret deref-on-switch** (resolving a `SecretRef` from keychain/vault at apply time and setting/unsetting the env var) — Phase 5 runtime half of PROF-03. Phase 4 may define the seam; end-to-end deref completes with the loader.
- **Keybindings (`bindkey`) and hooks (`precmd_functions`/`chpwd_functions`)** — routed to the unmanaged master block for v2.0 (OQ-1); no `model.Manifest` field. Re-admitting is additive later (requires its own byte-reversibility proof).
- **`compinit` / completion side effects** — the imperative `compinit` invocation stays in the master block; only fpath-array membership is admitted (as a `FPATH ListDelta`), per Phase 1.
- **Real `~/.zshrc` end-to-end ingest** into the `main` baseline — Phase 6 (PROF-03 end-to-end).
- **Other shells (bash/fish)** — out of scope; the whole milestone is zsh-only.

</deferred>

---

*Phase: 04-manifest-builder-emit*
*Context gathered: 2026-07-01 (autonomous smart-discuss, unattended)*
*Next step: /gsd:plan-phase 4 — Manifest Go field layout finalized, emit.go function structure, property-test harness shape, injection-safe quoting strategy per the decisions above; resolve OQ-4..OQ-7 during planning if they surface.*
