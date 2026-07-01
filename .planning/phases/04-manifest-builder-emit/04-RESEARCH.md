# Phase 4: Manifest Builder + Emit — Research

**Researched:** 2026-07-01
**Domain:** Go domain-model + JSON serialization; zsh reverse-syntax codegen; injection-safe shell emission; sandboxed-zsh property testing
**Confidence:** HIGH (every load-bearing zsh/Go claim below was verified with a POC run under `zsh 5.9 (arm64-apple-darwin25.0)` / `go 1.25.7` in a throwaway scratch dir; POC commands + observed output are in the appendix)

> This is a HOW-only, high-rigor research run. It does not re-open the locked decisions
> D-01..D-17 or the Phase 1 carry-forwards; it verifies the mechanics those decisions
> assume and gives an adversarial review panel concrete, falsifiable material. Where a
> lock is contradicted by evidence, it is logged as an OQ (OQ-8..OQ-11 appended to
> `04-OPEN-QUESTIONS.md`), never silently overridden.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (D-01..D-17 — DO NOT re-open)

**Grey Area 1 — `model.Manifest` layout & JSON contract**
- **D-01:** `Manifest` + four parts live in a new `core/model/manifest.go`, stdlib-only, tagged directly with `json:` struct tags (no DTO indirection; `SecretRef` is the precedent). The Manifest IS the wire record.
- **D-02:** Field/JSON layout mirrors `01-MANIFEST-SHAPE.md` exactly: top-level `profile` + `schema` + `env []Scalar` + `lists []ListDelta` + `aliases NameSet` + `functions NameSet` + `options []OptionSet`. `Scalar.Original` is a `*string` with `omitempty` (nil→unset, &""→empty, &"vim"→value — the single load-bearing type choice). `ListDelta{Name, Additions, Deletions}`. `NameSet{Added, Shadowed}`. `OptionSet{Name, Enabled, WasOn (json:"was_on")}`.
- **D-03:** `schema` value is the literal `"v1"`; a forward-compat gate checked before reverse logic. No versioning machinery beyond the constant.
- **D-04:** JSON tags are the snake/lower keys from the validated shape (`profile`, `schema`, `name`, `applied`, `original`, `additions`, `deletions`, `added`, `shadowed`, `enabled`, `was_on`) — NOT the store DTO camelCase.

**Grey Area 2 — `core/activate` builder, diff, agnostic `Plan`**
- **D-05:** `core/activate` is a new package importing `core/model` only, never `core/shell/zsh`. Exposes builder (`Profile→Manifest`), differ (active+target `Manifest`→`Plan`), and the `Plan` value type.
- **D-06:** Builder consumes only `Entry.EffectiveManaged()==true`, classifies by `Category`+`Kind` reusing the `core/ir/route.go` routing shape. Fills declarative intent (`added`/`applied`/`enabled`/`additions`); leaves `shadowed`/`original`/`was_on` as runtime-reconciled slots (NOT authored by the builder). Imperative/`OverrideUnmanaged`/`Opaque` → no part.
- **D-07:** PATH/FPATH segmentation into `additions`/`deletions` happens in Phase 4's builder; the base is runtime state, not manifest data; the manifest carries the delta.
- **D-08:** `Plan` is a Go value type of ordered agnostic ops — never shell text. Deactivate side: `RestoreScalar`/`UnsetScalar`, `RebuildListFromBase`, `Unalias`/`RestoreShadowedAlias`, `UnsetFunc`/`RestoreShadowedFunc`, `RestoreOption`. Activate side: `SetScalar`, `ApplyListDelta`, `AddAlias`/`AddFunc`, `SetOption`. Ordering: all of A's reverse ops before all of B's apply ops. Grep-verified no zsh token in `core/activate`.
- **D-09:** Empty-target (pure deactivate), empty-active (pure activate), and A↔B all flow through the same `Plan` builder.

**Grey Area 3 — `emit.go` structure & single-emit-path invariant**
- **D-10:** `emit.go` renders a `Plan` to two zsh strings (apply + deactivate), both **plain** (no `emulate -L`/`LOCAL_OPTIONS`). Reached via a new `shell.Emitter` seam mirroring `shell.Regenerator`; wired at `main.go`.
- **D-11:** Reverse-op zsh tokens generated ONLY under `core/shell/zsh/emit.go`. `regen.go` stays forward-only. Tree-grep invariant test.
- **D-12:** Emitted apply captures the LIVE prior into per-terminal runtime undo state (`typeset -g` globals), NOT the manifest `original`. Deactivate is the drift-guarded, unset-vs-empty-correct reverse from the Phase 1 Loader Reference Snippet. Helper-call-vs-inline is discretion (OQ-5; default = emit calls to a fixed `zp_*` helper set).
- **D-13:** Injection safety = single-quote wrapping with the zsh `'\''` escape for every **static** user-controlled value; **dynamic** values (`Entry.Dynamic==true`) stay verbatim so zsh expands per-machine. Never `eval` raw user config. (OQ-6.)

**Grey Area 4 — introspect body-dump & property test**
- **D-14:** Extend `introspectScript` additively — new section(s) dumping `${aliases[name]}`/`${functions[name]}` bodies in a newline-safe delimited format; existing name sections untouched. Preserve `zsh -f -c` + 5s timeout + `Available:false`-on-error.
- **D-15:** Carry bodies in ADDITIVE companion fields on `model.IdentitySet` (`AliasBodies`/`FunctionBodies map[string]string`), leaving `Aliases`/`Functions map[string]bool` intact. (OQ-7.)
- **D-16:** Zero-residue property test in a zsh-requiring, `LookPath`-guarded test; ≥2 profiles → manifests → plans → emit → source under `zsh -f`; assert literal empty six-class snapshot diff over N≥20 random switch orders + `$#path` stable; a mutated emitter (append PATH) must flip it to fail.
- **D-17:** The property test is the SW-02 regression pin, runs in the default suite (skip when zsh absent), NOT behind a `spike` tag. N-scaling under `-short` is discretion (default fixed N=20).

### Claude's Discretion (research recommends, planner decides)
- Exact Go identifiers/internals for `core/activate` (ops as tagged-union structs vs enum+payload), provided agnostic-value + no-zsh-token + deactivate-then-activate hold.
- Exact `shell.Emitter` seam signature (single `Emit(Plan)(apply,deactivate string,err error)` vs two methods) and placement in `core/shell/provider.go`.
- Helper-call vs inline reverse logic (OQ-5; default = emit helper calls).
- Body-dump delimiter format + `IdentitySet` companion field names (OQ-7).
- Property-test internals: fixtures, `-short` scaling, mutated-emitter mechanism.
- Commit granularity within the phase.

### Deferred Ideas (OUT OF SCOPE)
- The sourced runtime loader, `checkout`/`activate`/`deactivate`/`list`/`status` verbs, per-terminal state wiring, `.zshrc` bootstrap — Phase 5.
- PATH base-capture PLACEMENT in the live loader (OQ-3) — Phase 5.
- Secret deref-on-switch (PROF-03 runtime half) — Phase 5 (seam may be defined here).
- Keybindings (`bindkey`) / hooks (`precmd_functions`/`chpwd_functions`) — master block (OQ-1).
- `compinit`/completion side effects — master block (only fpath-array membership admitted as a `FPATH ListDelta`).
- Real `~/.zshrc` end-to-end ingest — Phase 6. Other shells — out of the milestone.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| Req 1 | `model.Manifest` + four parts, lossless round-trip to `01-MANIFEST-SHAPE.md` JSON | Sub-area (a): tri-state `*string` verified (POC-J1); OQ-4 map-vs-array unmarshal proven both ways (POC-J2/J3/J4) |
| Req 2 | `core/activate` builder from resolved `Profile` (`EffectiveManaged` only) | Sub-area (b): classify by `route.go` shape; imperative→no part |
| Req 3 | Active-vs-target diff → agnostic deactivate-then-activate `Plan` | Sub-area (b): op vocabulary, ordering, empty cases; token-free grep gate |
| Req 4 | Single emit path renders `Plan` to zsh apply+deactivate | Sub-area (c): codegen mechanism; `zsh -n` gate verified (POC-Z7) |
| Req 5 | Zero-residue property test, N≥20, mutated emitter fails | Sub-area (e): full cycle byte-identical verified (POC-Z8d); append-PATH grows `$#path` (POC-Z6b) |
| Req 6 | Ownership-aware, drift-guarded restore | Sub-areas (c)/(e): `${(P)+var}==1` verified (POC-Z4a); PATH-from-base stable (POC-Z6a); `${path:#x}` deletions (POC-Z6c) |
| Req 7 | Shadow capture + byte-identical restore of aliases/functions | Sub-areas (c)/(d): body byte-identity round-trip (POC-Go-RT); **landmine**: `${(P)+literal}` and `-n` guards are wrong (POC-Z8b/Z8e → OQ-8) |
| Req 8 | Introspect extended to dump alias/function bodies, graceful degradation preserved | Sub-area (d): NUL-record encoding round-trips multi-line bodies (POC-Z3/Go-RT); env inheritance caveat (POC-Z9a) |
</phase_requirements>

---

## 1. Summary

Phase 4 has **four deliverables** (a shell-agnostic `model.Manifest` + parts; a `core/activate` builder/differ/`Plan`; a single `core/shell/zsh/emit.go` reverse-codegen path; an additive introspect body-dump) and **one regression pin** (a zero-residue property test). Every locked decision is mechanically sound — I verified the load-bearing zsh and Go facts by running POCs, not by asserting from prior art.

**The three load-bearing risks, in priority order:**

1. **Injection (T-01-06) — HIGH severity, but SOLVED by a one-line escape.** Single-quote wrapping with the zsh `'\''` idiom makes any static value an inert literal that survives even a double `eval` layer (`loader eval "$block"`) across scalar/alias-body/function-body contexts. Verified against `'`, `;`, `$(...)`, backtick, newline, `'\''`-chains, and the kitchen-sink combo (POC-Inj, POC-Eval). The residual risk is a *misclassified* dynamic value emitted verbatim (OQ-6), not the escape function itself.

2. **The Phase 1 Loader Reference Snippet has TWO restore-guard bugs the emitter must NOT reproduce (new finding → OQ-8).** (i) It restores shadowed aliases/functions with `[[ -n "$SLOT" ]]`, which silently drops a legitimately-empty prior body (`alias x=''`); the correct test is `${+name}` (set-test), the same unset-vs-empty distinction the env path already respects. (ii) A `${(P)+literalName}` guard writes the slot name *literally inside `${(P)+...}`*, but `(P)` indirects through the *value* of that name — so a literal-named slot must use `${+name}` (no `(P)`), or a `local slot=NAME; ${(P)+slot}`. The corrected `${+name}` full cycle is byte-identical (POC-Z8c/8d). **Failure DIRECTION (C4 refinement):** a `${(P)+literalName}` guard fails to restore the shadow, leaving it UNSET (the prior is DROPPED, not left behind); the `-n` scalar guard drops an empty-body prior only. In both failures the residue is an ABSENCE (a dropped prior), NOT a surviving/residual alias — `unalias` runs first, so the shadow body never persists (POC-Z8b/8e).

3. **PATH must be rebuilt from a captured base every apply, never appended.** Rebuild-from-base holds `$#path` stable at 3 across 5 cycles; blind prepend grows it to 7 (POC-Z6). This is exactly the mutation the property test must detect.

**Primary recommendation:** Model the manifest exactly per D-02 with `Scalar.Original *string`; resolve OQ-4 with **two distinct part types** (`aliasSet` map + `funcSet` array-of-names) to match the validated fixture byte-for-byte, since the round-trip acceptance names the fixture as the oracle (a uniform map cannot unmarshal the `functions.added` array — proven). Emit via `strings.Builder` + a single `zquote()` escape applied at every static value site (not `text/template`), and emit **calls to a fixed `zp_*` helper set** that the property-test harness supplies inline. Use **NUL-delimited `name\0body\0` records** for the introspect body dump. Correct the two Phase 1 restore-guard bugs (OQ-8) in `emit.go` and in the harness helpers.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Reversible record type (`Manifest`) | `core/model` (agnostic domain) | — | Domain type, stdlib-only, no shell knowledge (D-01/D-05) |
| `Profile→Manifest` build + classify | `core/activate` (agnostic logic) | `core/model` | Reuses `route.go` shape; produces a value, no shell text (D-06) |
| Active-vs-target diff → `Plan` | `core/activate` | — | Pure value transformation; deactivate-then-activate ordering (D-08) |
| `Plan → zsh apply/deactivate code` | `core/shell/zsh/emit.go` (NEW file) | `shell.Emitter` seam | The SOLE reverse-syntax codegen site (D-11). **Distinct from the existing `regen.go`** — emit.go is ACTIVATION codegen (apply/deactivate); `regen.go` is ingest-regeneration (forward). They share no code (C7) |
| Static-vs-dynamic emission split (Dynamic-keyed) | `core/shell/zsh/emit.go` (NEW behavior) | — | **NEW behavior emit.go must implement** — `regen.go` emits `Value` VERBATIM in every branch and never reads `Dynamic` (verified, C7). emit.go must add the `Dynamic`-keyed static-zquote / dynamic-verbatim branch |
| Injection-safe quoting (scalar/alias VALUE contexts) | `core/shell/zsh/emit.go` | — | Escaping is a zsh-syntax concern; lives with codegen (D-13). Applies to scalar/alias values, NOT function bodies (C6) |
| Live-prior capture + drift-guarded restore | Emitted runtime code (Phase 5 executes) | `emit.go` shapes it | The manifest records intent; the loader is the actor (D-12) |
| Alias/function body introspection | `core/shell/zsh/introspect.go` | `core/model.IdentitySet` (additive) | zsh-specific dump; agnostic companion fields (D-14/D-15) |
| Zero-residue snapshot instrument | `core/shell/zsh` (reuse `introspectScript`) | property test | The snapshot tool already exists; extend it (D-16) |

---

## 2. Sub-area (a): `model.Manifest` — types, JSON, tri-state, OQ-4

### Recommended type layout (per D-01/D-02, verified)

```go
// core/model/manifest.go — stdlib-only, directly json-tagged (D-01).
type Manifest struct {
    Profile   string       `json:"profile"`   // == git branch / ZSHPRO_PROFILE (Phase 3 D-13)
    Schema    string       `json:"schema"`    // literal "v1" (D-03)
    Env       []Scalar     `json:"env"`
    Lists     []ListDelta  `json:"lists"`
    Aliases   AliasSet     `json:"aliases"`   // see OQ-4 default below
    Functions FuncSet      `json:"functions"` // see OQ-4 default below
    Options   []OptionSet  `json:"options"`
}

type Scalar struct {
    Name     string  `json:"name"`
    Applied  string  `json:"applied"`
    Original *string `json:"original,omitempty"` // LOAD-BEARING tri-state (D-02)
}

type ListDelta struct {
    Name      string   `json:"name"`      // "PATH" | "FPATH"
    Additions []string `json:"additions"`
    Deletions []string `json:"deletions"`
}

type OptionSet struct {
    Name    string `json:"name"`
    Enabled bool   `json:"enabled"`
    WasOn   bool   `json:"was_on"`
}
```

### The `*string` tri-state — VERIFIED (D-02, the single load-bearing type choice)

- `Original == nil` → key omitted → "was unset" → deactivate `unset`. **Verified:** marshals to `{"name":"WORK_TOKEN","applied":"abc"}` (no `original` key).
- `Original == &""` → `"original":""` → "was empty" → deactivate restore to empty. **Verified:** marshals to `{"name":"X","applied":"y","original":""}`.
- `Original == &"vim"` → `"original":"vim"` → "was set to vim". **Verified.**
- All three round-trip losslessly through `json.Unmarshal` (POC-J1). A plain `string` cannot distinguish nil from `""`. The `*string`+`omitempty` is the **simplest/recommended shape** for the tri-state — not the only one: `json.RawMessage`+`omitempty` and a `*struct{V string}` / custom `json.Marshaler` with a present-flag also distinguish all three (compiled counterexample, C15). `*string` is the idiomatic minimal choice; the tri-state behavior is what's load-bearing.

### OQ-4 forks — `NameSet.Added` map-vs-array (VERIFIED both ways)

The validated fixture shows `aliases.added` as a **map** (`{"gs":"git status"}`) but `functions.added` as a bare **array** (`["work_deploy"]`). This is a real JSON-shape divergence.

| Fork | Correctness | Reversibility | Complexity | Verdict |
|------|-------------|---------------|------------|---------|
| **(A) Two distinct types** — `AliasSet{Added map[string]string}` + `FuncSet{Added []string}` | Matches the fixture byte-for-byte; `unset -f` needs only the name so `[]string` loses nothing | Full — carries exactly what each reverse op needs | Two builder branches, two marshal paths | **RECOMMENDED DEFAULT** — the round-trip acceptance names the fixture as the oracle |
| (B) Uniform `map[string]string` for both | **BREAKS** the fixture: `json: cannot unmarshal array into Go struct field ... of type map[string]string` (POC-J2). Requires re-expressing `functions.added` as a map `{"work_deploy":""}` in the fixture | Full (bodies are `""` for functions) | One type, one branch — but diverges from validated JSON | Only viable if the acceptance is read as *semantic* ("no field missing"), not byte-identical |
| (C) Uniform type + custom `UnmarshalJSON` | Accepts EITHER shape into a uniform map (array→keys with `""` bodies) — verified (POC-J4) | Full | Custom un/marshaler; asymmetric marshal (must re-emit array for functions) | Over-engineered for v2.0; keep in reserve |

**Recommended default: Fork (A), two distinct types.** The CONTEXT applied a "uniform map" tentative default (Fork B) *pending* the round-trip acceptance reading; the fixture is the acceptance oracle and it literally shows an array for functions, so the byte-identical reading favors (A). Fork (A) is additive/low-cost and the safest against the acceptance test as written. This is a **planner decision** — logged as **OQ-4 resolution guidance** (not a new OQ; OQ-4 already exists). Both are reversible.

> **Note for the panel:** if the planner keeps Fork (B) (uniform map), it must also edit the round-trip test fixture's `functions.added` to a map, which changes what "round-trips to the `01-MANIFEST-SHAPE.md` JSON" means. That is a defensible reading but should be an explicit, logged choice, not silent.

### Marshal precedent
Follow `core/store/dto.go`'s `json.MarshalIndent(v, "", "  ")` + trailing `\n` for deterministic, diff-stable output. Maps sort keys deterministically in `encoding/json` (Go guarantee), so `aliases.added`/`shadowed` are stable.

---

## 3. Sub-area (b): `core/activate` — builder / diff / `Plan`

### Builder (`Profile → Manifest`), per D-06

Consume only `e.EffectiveManaged() == true`. Classify each entry by the **same shape checks `core/ir/route.go` already proved** (reuse, do not re-derive):

| Entry shape (from `route.go`) | Manifest part |
|-------------------------------|---------------|
| `KindAssignment`, 1 name, non-append, non-array, `CatEnvironment`/`CatSecrets` | `Scalar{Name, Applied: Value}` (leave `Original` nil — runtime-reconciled, D-06) |
| `KindAssignment`, `CatPath` | a `ListDelta` entry: `PATH` or `FPATH` by name; the `Value` split into `Additions` vs the runtime base (D-07) |
| `KindAlias`, 1 name, non-flagged | `aliases.Added[name] = Value` |
| `KindFuncDecl` | `functions.Added += name` (Fork A) |
| `KindCommand` `setopt`/`unsetopt`, ≥1 name | `OptionSet{Name, Enabled: (CmdName=="setopt")}` (leave `WasOn` runtime-reconciled) |
| anything else / `Opaque` / `OverrideUnmanaged` | **no part** (acceptance: imperative entry yields nothing) |

**The builder does NOT author `shadowed`/`original`/`was_on`** — those are LIVE facts the emitted apply code captures at runtime (D-06/D-12). The builder fills declarative intent only.

**PATH segmentation (D-07):** a `CatPath` assignment like `export PATH=$HOME/bin:$PATH` is *dynamic* (`Entry.Dynamic==true`, verified it carries `$HOME`/`$PATH`). The additions the profile *introduces* are the literal segments; `$PATH`/`$path` self-references are the base marker. The manifest carries the addition segment(s) as `Additions`; the emitted code rebuilds `path=(<additions> $path)` from the runtime `ZP_BASE_PATH`. Deletions surface only if a delta model exposes them (empty in both Phase 1 fixtures; the slot is validated present). **Landmine:** a dynamic PATH segment (`$HOME/bin`) is an *addition that must stay verbatim* (dynamic split, §4) — do not hard-quote it or `$HOME` freezes to a literal.

**PATH ownership & element removal (C10/C11 — CORRECTED):** Two mechanisms the emitter must get right:
- **Element deletion must be LITERAL-equality, not `${path:#pattern}`.** `${path:#/opt/x}` is a GLOB/pattern subtraction (C10): a PATH element (or the target) containing `? * [ ]` over-matches and deletes sibling entries (`${path:#/opt/tool?}` deleted `/opt/toolX`/`/opt/toolY`). Remove a specific element by rebuilding the array with a string-equality compare (`for e in $path; do [[ $e == $target ]] || newpath+=($e); done`, or otherwise disable pattern semantics), NOT `${path:#target}`.
- **Ownership-aware restore needs a dedup-correct model.** With `typeset -U path` active (the project's own PATH-no-growth backstop), co-owned entries collapse to ONE physical entry — so a naive "remove one occurrence per addition" strips a shared entry another active profile/base still owns (C11: deactivating A removes the sole `/usr/local/bin` while B is active). Ownership-aware restore requires a reference-count or set-based ownership model (remove an entry only when NO other active profile/base owns it), or the rebuild-from-base + re-apply-other-actives approach — never per-addition string subtraction.

### `Plan` value type + op vocabulary (D-08)

Recommended internal shape — a **tagged-union of op structs** in a single ordered slice (cleaner than an enum+payload for exhaustive `switch` in `emit.go`):

```go
type Plan struct {
    Deactivate []Op // A's reverse ops
    Activate   []Op // B's apply ops
}
// Op is a sealed interface; concrete ops carry only agnostic data (no zsh tokens):
//   deactivate: RestoreScalar{Name,Applied,Original}, UnsetScalar{Name,Applied},
//               RebuildListFromBase{Name,Additions,Deletions},
//               Unalias{Name}, RestoreShadowedAlias{Name}, UnsetFunc{Name},
//               RestoreShadowedFunc{Name}, RestoreOption{Name,WasOn}
//   activate:   SetScalar{Name,Applied,Dynamic}, ApplyListDelta{Name,Additions,Deletions},
//               AddAlias{Name,Body,Dynamic}, AddFunc{Name,Body},
//               SetOption{Name,Enabled}
```

Note the `Dynamic bool` carried on `SetScalar`/`AddAlias` — this is what `emit.go` keys the quote-vs-verbatim split on (§4). It is agnostic data (a bool), not a zsh token, so `core/activate` stays token-free.

### Ordering & empty cases (D-08/D-09)

- **Ordering:** `Plan.Deactivate` (all of A's reverse ops) fully precedes `Plan.Activate` (all of B's apply ops). Within each side, a stable order (scalars → lists → aliases → functions → options, or the reverse for deactivate) is sufficient; there is no cross-op dependency within a side because each op targets a distinct name.
- **`diff(A, nil)`** → pure deactivate (Phase 5 `deactivate` verb).
- **`diff(nil, B)`** → pure activate (first `checkout`).
- **`diff(A, B)`** → the switch case.
- All three via one builder so the property test drives arbitrary sequences (D-16).

**Diff subtlety for the panel:** a naive diff would deactivate *all* of A then activate *all* of B, re-touching names both profiles share. That is correct and safe for zero-residue (deactivate reverses to base, activate re-applies), and it is what Phase 1 validated (`apply_A; deact_A; apply_B; deact_B`). An "optimized" diff that skips shared names would be a **correctness hazard** (it would leave A's value where B's differs). Recommend the naive full-deactivate-then-full-activate; do NOT optimize.

### Token-free acceptance (D-08/D-11) — PRECISE CHECK (C22 refinement)
A grep test asserts `core/activate` contains no reverse-op zsh syntax. The check must be **precise**, not a naive case-insensitive grep, because (a) the op *type names* (`Unalias`, `RestoreOption`, `SetOption`) are Go identifiers that a naive substring match false-hits, and (b) `alias`/`export`/`setopt`/`unsetopt` are FORWARD tokens that `regen.go` legitimately emits today, so any tree-wide gate for them false-positives on `regen.go`. Define the check as:
- **case-sensitive** (`Unalias` ≠ `unalias`),
- **word-boundary anchored** (`\bunalias\b`, `\bunset -f`, `\bunsetopt\b`, PATH-array-rebuild pattern),
- **scoped to genuinely reverse tokens** (`unalias`, `unset -f`, `unsetopt`, PATH-array rebuild) — NOT the forward tokens `alias`/`export`/`setopt` which regen.go owns,
- **excluding** Go identifier type names (`Unalias`, `SetOption`, …) and comments.

The planner writes the grep against zsh *syntax* literals under those constraints, so it does not self-trigger on the Go op-type names, on comments, or on `regen.go`'s forward emission.

---

## 4. Sub-area (c): `emit.go` — codegen, injection quoting, plain-fn, static-vs-dynamic

### Codegen mechanism — RECOMMENDED: `strings.Builder` + a single `zquote()` (not `text/template`)

| Fork | Injection safety | Complexity | Verdict |
|------|------------------|------------|---------|
| **`strings.Builder` + `fmt`/manual, `zquote()` at every value site** | Escape is explicit and local at each interpolation; auditable | Low; matches `regen.go`'s `fmt.Sprintf` style | **RECOMMENDED** — the escape must be applied per-value-site, which a builder makes explicit |
| `text/template` with a custom `zq` func | Safe IF every `{{.X}}` uses `{{zq .X}}` — but a single forgotten `{{.X}}` is a silent injection hole | Medium; template + funcmap | Rejected — templates make it *easy to forget* the escape at one site; the failure is invisible until an adversarial value hits it |
| `text/template` with auto-escaping | `text/template` has NO context-aware auto-escaping for shell (that is `html/template` only) | — | Rejected — no shell escaper exists in stdlib |

Both `strings.Builder` and `text/template` are stdlib (no new dep — constraint satisfied). The recommendation is `strings.Builder` precisely because injection safety here depends on *never missing an escape site*, and an explicit `b.WriteString("export " + name + "=" + zquote(val))` makes each site visible to a reviewer.

### The injection quoting function — VERIFIED SAFE (D-13, T-01-06)

```go
// zquote wraps s as a zsh single-quoted literal, escaping embedded ' with '\''.
// A single-quoted zsh string has NO expansion of $, `, (), etc. — the only
// metacharacter is the closing '. So the result is always an inert literal.
func zquote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
```

**Adversarial vectors — all SAFE (POC-Inj, POC-Eval), no canary fired, byte-literal round-trip:**

| Vector | Result |
|--------|--------|
| `it's a value` | literal `it's a value` |
| `foo; touch $CANARY` | literal (no command ran) |
| `x$(touch $CANARY)y` | literal (no cmdsub) |
| `` a`touch $CANARY`b `` | literal (no backtick eval) |
| `line1\ntouch $CANARY\nline3` | literal (newline is data) |
| `a'\''b` (an escaped-sq chain) | literal |
| `'; touch $CANARY; $(...) \`...\` \n rm -rf /tmp/x` (kitchen sink) | literal |

Crucially, this holds **under the real threat model** — the loader does `eval "$emitted_block"`, a *double* layer — for the **scalar `export`** and **`alias name=<body>`** value contexts (POC-Eval, C5). The escape is applied once in the emitted source; the `eval` re-parse still sees an inert single-quoted literal.

**IT DOES NOT apply to `functions[name]=<body>` shadow-restore (C6 — CORRECTED).** A zsh function BODY is executable code by nature: wrapping it in single quotes only makes the edge bytes literal, and the body still runs when the function is called (`eval "functions[fv]='x; touch CF'"; fv` FIRED the canary); an adversarial single-quote in the body produces "invalid function definition"/"unmatched '". You **cannot** make a function body an inert literal with zquote.
- **Function restore is verbatim capture-and-reassign:** `functions[name]=$capturedBody`, where the body round-trips as DATA via the NUL-delimited capture (§5, C14). The captured body is trusted live code by construction (it was already a live function in the user's environment — the same trust boundary as their `.zshrc`), not attacker-controlled emitted quoting.
- The static-quote/dynamic-verbatim split (below, OQ-6) applies to scalar and alias VALUE contexts only — **not** to function-body reassignment.

### The static-vs-dynamic split — VERIFIED behavior, NEW mechanism (D-13, OQ-6, C7)

The same value `$HOME/go`:
- emitted **verbatim** → expands to `/Users/poc/go` (POC-Dyn). Correct for `Entry.Dynamic==true` (EVAL-01 late binding).
- emitted **zquote'd** → stays literal `$HOME/go` (POC-Dyn). Correct for static values (injection-safe).

`emit.go` keys the choice on the `Dynamic bool` carried on the op (§3). **Rule:** `if op.Dynamic { write(value) /* verbatim */ } else { write(zquote(value)) }`.

**This is NEW behavior emit.go must implement — it is NOT provided by existing regen infrastructure (C7 — CORRECTED).** The existing `Provider.Regenerate` (`core/shell/zsh/regen.go`, delegated from `core/ir/regen.go`) emits the captured `Value` **VERBATIM in every branch** and never reads any `Dynamic` bool — two `model.Entry` values with identical `Value="$HOME/go"` but opposite `Dynamic` produce byte-identical output (verified). In the ingest round-trip, the static case stays literal only because the surrounding single-quotes were captured as part of `Value` (verbatim source-span capture in the parser), NOT because emission zquotes on `Dynamic`. `regen.go`'s own doc comment says it emits verbatim and diverges from the `%q` used in test-only `render.go`. So the emit.go static-literal-vs-dynamic-verbatim split is genuinely new codegen; do not assume the existing regen path already keys emission on `Dynamic`.

**Precondition (still true):** the split is portable only if the value reaching emit.go is the UNEXPANDED literal `$HOME/go` (dollar preserved from the AST). If `$HOME` was eagerly expanded upstream, both branches collapse to the machine-specific path and portability breaks. The Dynamic bool is necessary but not sufficient — the pipeline must carry the raw unexpanded token (Pitfall 6 / "never freeze dynamic values").

**Residual risk (OQ-6, HIGH severity, trust-bounded):** a value *classified* dynamic but authored maliciously (`export X=$(rm -rf ~)`) WILL execute at apply — but that is the profile owner's own config, the same trust boundary as their `.zshrc` (EVAL-01). The real hazard is a **misclassification** (a value that should be static gets `Dynamic==true` and is emitted verbatim). Recommend the planner add a property/unit test asserting that a *static* value with shell metacharacters is always quoted, independent of the `Dynamic` flag path. A future shared-profile milestone (SHARE-01) needs its own trust gate — out of scope here.

### Plain loader functions — VERIFIED mandatory (D-10, Pitfall 1)

`setopt EXTENDED_GLOB` inside a **plain** function persists after return; inside `emulate -L zsh` OR `setopt LOCAL_OPTIONS` it auto-reverts at return (POC-Z5). The emitted `apply_*`/`deact_*` functions MUST be plain. **Contrast landmine:** `introspectScript` uses `emulate -L zsh` — correct THERE (introspection wants isolation) but the exact opposite of what the loader needs. Do not copy the `emulate -L` line into `emit.go`.

### Helper-call vs inline (OQ-5) — RECOMMENDED: emit calls to a fixed `zp_*` set

| Fork | Zero-residue correctness | Complexity | Verdict |
|------|--------------------------|------------|---------|
| **Emit calls to `zp_capture_env`/`zp_restore_env`/etc. (fixed set)** | The tricky drift-guard/`${(P)+var}` logic lives in ONE audited place | Emitted surface is minimal; the property-test harness must supply the helper defs | **RECOMMENDED DEFAULT** (matches Phase 1 snippet; D-12) |
| Inline every reverse op fully | Self-contained emitted block (no runtime dependency) | Heavier text, logic duplicated per profile → more surface to get the guards wrong | Reserve for a "portable export" feature later |
| Hybrid (emit helper defs once per block, calls thereafter) | Same as helper-call, self-contained | Medium | Good option if the property test should run without harness-supplied helpers |

**Phase 4↔5 seam:** `emit.go` emits *calls* to `zp_*`; the helper *definitions* are Phase 5's loader (OQ-3/OQ-5). For the Phase 4 property test to run standalone, the harness supplies the helper defs inline (verified working self-contained in POC-Z8d). The planner should pin whether `emit.go` also emits a self-contained helper block (Hybrid) so the emitted code is testable without the loader — a low-cost hedge.

---

## 5. Sub-area (d): introspect body-dump — encoding, multi-line round-trip, additive IdentitySet

### Body format facts — VERIFIED

- `${functions[name]}` returns the body **WITHOUT** the `name() {` wrapper and with **no trailing newline** in the value itself (POC-Z1a/1c). A single-line `ff() { echo hi }` yields exactly `\techo hi`. Body lines are normally tab-indented, but **a leading tab is NOT a universal per-line invariant** (C12 refinement): heredoc content and terminator lines have NO leading tab. Do not rely on "leading tab on each line" — rely only on the no-trailing-newline + byte-identical-reassign facts.
- `${aliases[name]}` returns the **RHS only** (`git status`), not `gs=git status` (POC-Z2a).
- Both re-establish **byte-identical** via `functions[name]=$cap` / `alias name=$cap` (POC-Z1d/2b). This capture-and-reassign is also how a function BODY is restored (it is live code, not zquote'd — C6).

### Encoding fork (OQ-7) — RECOMMENDED: NUL-delimited `name\0body\0` records

Function bodies are multi-line by nature, so a naive `name\tbody` single-line dump is **broken** — the embedded newlines split one record into several lines (POC-Z3a, `od -c` shows the `\n`s).

| Encoding | Newline-safe | Parse complexity | Diff-readable | Verdict |
|----------|-------------|------------------|---------------|---------|
| `name\tbody` single line | **NO** (breaks on any body newline) | trivial | yes | Rejected — most function bodies have newlines |
| **`print -rN -- name body` (NUL-delimited records)** | YES (NUL cannot appear in a zsh string) | split on `\x00` in Go | no (binary) | **RECOMMENDED** — verified byte-identical round-trip (POC-Go-RT) |
| base64 each body | YES | decode per record | no (opaque) | Reserve; heavier, opaque fixtures |
| length-prefixed | YES | count bytes | no | Works but more fragile than NUL |

**Verified round-trip (POC-Go-RT):** a function body containing a newline + tab + single-quote + `$` + `;` dumped via `print -rN`, parsed in Go by splitting the section on `\x00`, and fed back into `functions[name]=` is **byte-identical** to the original. **Why NUL is safe (C14 — CORRECTED justification):** a FUNCTION BODY is reparsed by zsh, so `${functions[name]}` never carries a raw 0x00 — the delimiter is unambiguous for body records. It is NOT true that "a zsh string can never contain NUL": on zsh 5.9 an arbitrary scalar value CAN hold a literal NUL (`v=$'a\0b'` → `${#v}`=3), which would break a Go 0x00 splitter — so this encoding is scoped to reparsed function/alias bodies, not to arbitrary scalar values.

### Additive `introspectScript` extension (D-14) — recommended shape (C23 — CORRECTED)

Add NEW sections after the existing name sections, leaving the name lines untouched. The body sections **must NOT reuse the existing line-oriented `##DELIMITER##` framing** — they must use a multi-line-safe (NUL-delimited, consistent with C14) encoding:

```zsh
print -r -- '##ALIASBODIES##'
for k in "${(@k)aliases}"; do print -rN -- "$k" "${aliases[$k]}"; done
print -r -- ''                       # newline so the ##...## marker starts a line
print -r -- '##FUNCTIONBODIES##'
for k in "${(@k)functions}"; do print -rN -- "$k" "${functions[$k]}"; done
print -r -- ''
print -r -- '##END##'
```

**WHY the framing must differ (C23):** the existing `parseIntrospect` is strictly **line-oriented** — it does `strings.Split(s, "\n")` and switches on the full line matching a `##…##` marker (`core/shell/zsh/introspect.go:64-85`). Reusing that line reader for body sections breaks two ways:
1. **Multi-line body truncation** — a multi-line function body's continuation lines are not `name -> body` records, so a line parser drops everything after the first line.
2. **Delimiter collision** — a heredoc inside a function body emits UNINDENTED lines at column 0 (zsh preserves heredoc content verbatim). A body line that happens to read `##PATH##`/`##END##` is mis-read by the line switch as a real section switch, routing subsequent bytes into the wrong section and corrupting every following ENV/PATH/OPTIONS section.

So `parseIntrospect` gains a **dedicated, non-line body parser** for the new sections: slice the body section by the *known offsets* of the header line and the next `\n##` marker, then split the enclosed bytes on `\x00` — it must NOT scan line-by-line for `##END##` inside body bytes. The existing line-oriented switch for `##ALIASES##`/`##FUNCTIONS##`/etc. is unchanged (D-14/D-15 additive; existing name-only tests keep passing).

**Confirmed additive sub-claims (C23 — these stay true):** the name maps stay `map[string]bool` (untouched — verified `core/model/identityset.go`); `analyze` consumes only `ids.Available` (verified `core/analyze/analyzer.go:84`) so no caller breaks; a zsh-absent/timeout run still returns `IdentitySet{Available:false}`; all existing `IdentitySet{}` literals are keyed, so adding fields is source-compatible. Only the framing of the NEW body sections needed correcting.

### Additive `IdentitySet` companion (D-15)

```go
type IdentitySet struct {
    Aliases        map[string]bool   // unchanged
    Functions      map[string]bool   // unchanged
    Env            map[string]bool   // unchanged
    Path           []string          // unchanged
    Options        map[string]bool   // unchanged
    Available      bool              // unchanged
    AliasBodies    map[string]string // NEW — name→RHS body
    FunctionBodies map[string]string // NEW — name→body (leading tab preserved)
}
```

Additive: `analyze` consumes only `Available` today (verified at `introspect.go:86-90`), so no existing caller breaks. Graceful degradation is preserved — the new sections are populated only on the success path; a zsh-absent/timeout still returns `IdentitySet{Available:false}`.

### Env-inheritance caveat for the snapshot instrument (POC-Z9a)
`zsh -f` suppresses rc *sourcing* but NOT env *inheritance* (Pitfall 5 — a parent `FOO=hello` is visible in the `-f` child). This does not affect body-dumping, but it constrains the property test (§6): the harness must snapshot within a controlled base, not compare across processes with divergent inherited env.

---

## 6. Sub-area (e): zero-residue property-test harness

### Structure (D-16, D-17)

- `LookPath("zsh")` skip-guard (like `introspect_test.go:11-13`, `roundtrip_test.go:149`). Runs in the default suite; NOT a `spike` tag (D-17).
- Build ≥2 profiles → `activate.Build` → `Manifest` → `activate.Diff` → `Plan` → `emit.Emit` → apply/deactivate zsh strings.
- Drive N≥20 random switch sequences (default fixed N=20; `-short` may reduce but must keep the mutated-emitter negative check — D-17).
- Snapshot **all six classes with full FIDELITY** via the (extended) `introspectScript` — assert **literal string equality** of pre-vs-post snapshots (the Phase 1 bar, NOT `IdentitySet` field checks), plus `$#path` element-count stable.
- **The snapshot must capture function BODIES, not just names, and the FULL env (exported AND non-exported), or body-level and non-exported residue passes silently (C19 — CORRECTED).** A name-only functions snapshot (`${(ok)functions}`) does not catch a redefined-but-not-restored function BODY; an exported-only env snapshot (`typeset -x`) does not catch a leaked plain shell var. The zero-residue guarantee is a byte-identical snapshot across all six classes at BODY level — enumerate: alias bodies, function bodies, env (all vars in scope), options, PATH scalar, `$path` array + `$#path` count.
- Full end-to-end zero-residue verified self-contained in POC-Z8d (byte-identical after `apply_work; deact_work` including a shadowed `ll`).

### Where the snapshot must happen — the process-boundary decision (from POC-Z9a)

Because `zsh -f` inherits parent env, the harness has two viable shapes:

| Fork | Correctness | Verdict |
|------|-------------|---------|
| **Single `zsh -f` process: set base → snapshot → apply → snapshot → deactivate → snapshot, all inline** | Base is controlled; no cross-process env drift; matches the Phase 1 driver | **RECOMMENDED** — the whole sequence + helper defs + emitted apply/deactivate + snapshot function run in one sourced script |
| Spawn a fresh `Introspect` subprocess per snapshot | Each subprocess inherits the harness's env, which may differ from the base under test → false diffs | Rejected — the inheritance caveat makes cross-process snapshots unreliable |

So the property test writes ONE zsh script per sequence: it defines the `zp_*` helpers (harness-supplied, OQ-5), sets `ZP_BASE_PATH`, sources the emitted apply/deactivate blocks, and runs an inline snapshot function (reusing the `introspectScript` body verbatim). This makes the emitted code testable **without** the Phase 5 loader.

### The mutated-emitter negative check (D-16, first-class acceptance) — VERIFIED the mutation is detectable

The test must FAIL when the emitter appends PATH instead of rebuilding from base. **Verified:** rebuild-from-base holds `$#path` at 3 across 5 cycles; blind prepend grows it to 7 (POC-Z6a/6b). Recommended mechanism (planner's discretion per D-17): an **in-test emitter variant** (a function value or a flag on the emitter) that swaps `PATH="$ZP_BASE_PATH"; path=(<add> $path)` for a blind `path=(<add> $path)`, asserted to produce a NON-empty diff / grown `$#path`. An in-test injection is cleaner than a build tag (no separate compile) and keeps the negative check in the default suite.

### Fixtures
Two profiles exercising all six classes with at least one shadow collision (both profiles override the same pre-existing `ll`/`ff`) and one shared PATH addition (`/usr/local/bin` in both — the ownership-aware criterion, Req 6). A dynamic value (`export GOPATH=$HOME/go`) in at least one profile to exercise the verbatim path through the full cycle.

---

## Runtime State Inventory

> This phase EXTENDS runtime-facing code (introspect dump format; emitted apply/deactivate assumes runtime undo state). No stored data is being renamed. The relevant inventory is the Phase 4↔5 runtime-state *seam* the emitted code assumes.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — the `Manifest` is a NEW serialized record; no existing datastore keys change. `profile.json` (Phase 3) is unchanged. | None |
| Live service config | None — no external service. | None |
| OS-registered state | None. | None |
| Secrets/env vars | Emitted code assumes per-terminal runtime globals exist at apply/deactivate time: `ZP_BASE_PATH` (PATH base), `__ZP_ORIG_<var>` (captured live prior env), `ZP_<profile>_PRIOR_ALIAS_<name>` / `ZP_<profile>_PRIOR_FUNC_<name>` (captured shadow bodies), `ZP_UNSET_SENTINEL`. **These are written by the emitted apply code and read by the emitted deactivate code — the Phase 4↔5 seam (OQ-3/OQ-5).** Phase 4 emits code shaped for this contract; Phase 5 owns base-capture *placement*. | Phase 4: define the naming contract in `emit.go`. Phase 5: place the capture. Harness supplies them inline (§6). |
| Build artifacts | None — new package + new file; no stale artifacts. Phase 1's `scratch/` is git-ignored throwaway, unrelated. | None |

**Naming-collision landmine:** the shadow-prior slot name embeds the profile name (`ZP_<profile>_PRIOR_ALIAS_<name>`). A profile named with characters illegal in a zsh identifier (e.g. `feature/x`) would produce an invalid `typeset -g` target. Recommend `emit.go` sanitize the profile+name into a valid identifier (e.g. replace non-`[A-Za-z0-9_]` with `_`, or hash) — **logged as OQ-10.**

---

## Common Pitfalls

### Pitfall 1: `emulate -L`/`LOCAL_OPTIONS` in the emitted loader functions
**What goes wrong:** `setopt`/`unsetopt` auto-revert at function return; options never actually apply. **Verified** (POC-Z5). **Avoid:** emit PLAIN functions (D-10). **Warning sign:** `[[ -o extendedglob ]]` is false immediately after `apply_*` returns.

### Pitfall 2: PATH blind-append instead of rebuild-from-base
**What goes wrong:** `$#path` grows every cycle (7 after 5 cycles vs stable 3) — residue. **Verified** (POC-Z6). **Avoid:** `PATH="$ZP_BASE_PATH"; path=(<add> $path)`. **Warning sign:** the property test's `$#path`-stability assertion fails.

### Pitfall 3: `-n` guard for shadow restore (NEW — the Phase 1 snippet has this bug → OQ-8)
**What goes wrong:** `[[ -n "$SLOT" ]] && alias name="$SLOT"` silently DROPS a legitimately-empty prior body (`alias x=''`). **Verified** (POC-Z8e). **Avoid:** use `${+SLOT}` (set-test), matching the env unset-vs-empty discipline. **Warning sign:** an empty-body shadowed alias is not restored on deactivate.

### Pitfall 4: `${(P)+literalName}` for a slot whose name is written literally (NEW → OQ-8)
**What goes wrong:** `(P)` indirects through the *value* of `literalName`, not the name itself — so a literal-named slot with `(P)` tests the wrong parameter and the restore silently no-ops. **Verified** (POC-Z8b): my faithful reproduction of the Phase 1 snippet leaked a residual `ll` alias. **Avoid:** `${+ZP_..._name}` directly (no `(P)`) since `emit.go` knows the literal name; or `local slot=NAME; ${(P)+slot}`. Corrected form is byte-identical (POC-Z8c/8d). **Warning sign:** shadow restore silently does nothing.

### Pitfall 5: `zsh -f` inherits parent env (snapshot instrument)
**What goes wrong:** cross-process snapshots see the harness's inherited env, producing false diffs. **Verified** (POC-Z9a). **Avoid:** run the full apply/snapshot/deactivate/snapshot sequence in ONE sourced `zsh -f` script (§6). **Warning sign:** the property test diffs on env vars the profiles never touched.

### Pitfall 6: dynamic value hard-quoted (over-quoting) or static value emitted verbatim (under-quoting)
**What goes wrong:** over-quoting freezes `$HOME` to a literal (breaks EVAL-01 portability); under-quoting is the injection vector. **Verified both directions** (POC-Dyn). **Avoid:** key strictly on `Entry.Dynamic`; add a test that a static value with metacharacters is always quoted regardless of flag path (OQ-6). **Warning sign:** `$HOME` appears literally in a resolved path, or a metacharacter-laden static value expands.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Shell single-quote escaping (scalar/alias VALUE contexts only) | A custom char-by-char escaper with `\` sequences | The `'\''` idiom via one `strings.ReplaceAll(s,"'",`'\''`)` | Verified injection-safe against all vectors under double `eval` for scalar-export and alias-body values (POC-Inj/Eval, C5); a `\`-based escaper is wrong inside single quotes. **NOT for function bodies** — see the function-body row below (C6) |
| Function-body restore | zquote-to-inert-literal (a function body cannot be made inert — it is live code by nature; calling it runs the body, and adversarial `'` breaks the definition: "invalid function definition") | Verbatim capture-and-reassign: `functions[name]=$capturedBody` (body round-trips as DATA via the NUL-delimited capture) | C6: `eval "functions[fv]='…; touch CF'"` FIRED the canary — single-quoting only makes the edge bytes literal, the body still executes. The trusted body is captured live at ingest; it is not attacker-supplied emitted quoting |
| Unset-vs-empty set-test for a LITERAL-named slot | `-n`/`-z` truthiness, OR `${(P)+literalSlot}` | `${+name}` (NO `(P)` flag) | C1: `-n` conflates unset with empty; `${(P)+FOO}` indirects through the VALUE of `FOO` (tests the wrong parameter, always 0 for a real literal slot). `${+FOO}` is 0/1/1 (unset/empty/set). Use `(P)` ONLY when the slot NAME is stored in another variable (env path — C21) |
| Multi-line body encoding | `name\tbody` single-line, or ad-hoc sentinels, or the existing line-oriented `##DELIMITER##` framing | NUL-delimited `print -rN` records with a dedicated (non-line) parser | Verified byte-identical for bodies with `\n`+`\t`+`'` (C14). Safe because a FUNCTION BODY is reparsed and never carries a raw 0x00 (NOT because "a zsh string can never contain NUL" — an arbitrary scalar value can). The line-oriented parser truncates multi-line bodies and mis-reads heredoc lines as section switches (C23) |
| PATH ownership-aware element removal | `${path:#/opt/x}` pattern subtraction, OR naive "remove one occurrence per addition" | LITERAL-equality element rebuild (`[[ $e == $target ]]` string-compare) + a reference-count / set-based ownership model correct under `typeset -U` | C10: `${path:#PATTERN}` is a GLOB — an element with `? * [ ]` over-matches and deletes siblings. C11: per-addition string subtraction strips a co-owned entry under `typeset -U` (co-owners collapse to one physical entry). Rebuild-from-base + re-apply other actives is the safe model |
| PATH dedup/rebuild | Blind append | Rebuild from captured base each apply | Append causes growth residue (Pitfall 2). Note `typeset -U` is the runtime no-growth backstop, but it collapses co-owned entries — so ownership-aware REMOVAL must not assume one-occurrence-per-owner (C11) |
| Shell codegen templating with auto-escape | `html/template` (wrong domain) or trusting `text/template` auto-escape | `strings.Builder` + explicit `zquote()` per site | stdlib has no shell context-aware escaper; explicit per-site escaping is auditable |

**Key insight:** every "clever" shortcut in this phase (a `-n` guard, a blind append, a one-line body dump, a `\`-escaper) is a *silent* correctness or security bug that only surfaces on an adversarial or edge value. The verified primitives above are boring and correct; use them.

---

## State of the Art

| Old Approach | Current Approach | Source | Impact |
|--------------|------------------|--------|--------|
| Phase 1 snippet: `[[ -n "$PRIOR" ]] && alias ...` | `[[ "${+PRIOR}" == "1" ]] && alias ...` | POC-Z8e (this run) | Empty-body shadows now restored (OQ-8) |
| Phase 1 snippet: `[[ "${(P)+ZP_..._ll}" == "1" ]]` (literal name in `(P)`) | `[[ "${+ZP_..._ll}" == "1" ]]` (no `(P)`) | POC-Z8b/8c (this run) | Shadow restore actually fires (OQ-8) |
| Introspect dumps only names | Additive NUL-record body dump | D-14/D-15 + POC-Go-RT | Shadow restore becomes possible (Req 7/8) |

No external library or version is in play — the phase is stdlib + existing `mvdan.cc/sh/v3 v3.13.1` only. `mvdan.cc/sh` is NOT needed for emit (emit is structured-field codegen, not parsing); it remains only in the ingest path.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The round-trip acceptance treats `01-MANIFEST-SHAPE.md` JSON as a byte-identical oracle (favoring OQ-4 Fork A, two types) | §2 | If read semantically, Fork B (uniform map) is fine but needs the fixture edited — a logged choice either way |
| A2 | The Phase 5 loader will define the `zp_*` helpers `emit.go` calls; the Phase 4 property test supplies them inline | §4/§6 | If Phase 5 changes the helper contract, emitted calls break — pinned as the OQ-5 seam |
| A3 | `Entry.Dynamic` correctly classifies every value that must stay verbatim vs quoted | §4 | A misclassified static value with metacharacters would be emitted verbatim (injection) — mitigated by the OQ-6 static-quoting test |
| A4 | Profile names are valid zsh identifier fragments (or will be sanitized) for slot naming | Runtime Inventory | An invalid `typeset -g` target aborts apply — logged as OQ-10 |

---

## Open Questions

The four medium/low-confidence DECISIONS surfaced by this research are appended to
`04-OPEN-QUESTIONS.md` as **OQ-8, OQ-9, OQ-10, OQ-11** (OQ-1..OQ-7 untouched). Summary:

1. **OQ-8 (HIGH confidence in the fix):** the Phase 1 Loader Reference Snippet's shadow-restore guards (`-n` test; `${(P)+literalName}`) are BOTH wrong; `emit.go` and the harness must use `${+name}` set-tests. Evidence: POC-Z8b/8c/8e. *This is a correction to a carry-forward artifact, logged (not silently overridden) per the autonomy contract.*
2. **OQ-9 (MEDIUM):** should `emit.go` emit a self-contained helper block (Hybrid, §4) so emitted code is testable/usable without the Phase 5 loader, or emit bare `zp_*` calls?
3. **OQ-10 (MEDIUM):** how are runtime undo-slot names derived from `profile`+`name` so they are always valid zsh identifiers (sanitize vs hash)?
4. **OQ-11 (MEDIUM):** OQ-4 resolution — two distinct types (Fork A, matches fixture) vs uniform map (Fork B, needs fixture edit). Research recommends Fork A.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| zsh | introspect extension; property test; all emit POCs | ✓ | 5.9 (arm64-apple-darwin25.0) at `/bin/zsh` | Tests `LookPath`-skip when absent (graceful) |
| Go toolchain | entire codebase | ✓ | 1.25.7 | — |
| `mvdan.cc/sh/v3` | ingest only (not this phase's emit path) | ✓ | v3.13.1 (in go.mod) | — |
| git | property-test precedent uses `store` (indirect) | ✓ | present | `LookPath`-skip |

**Missing dependencies with no fallback:** none.
**No new external dependency is introduced** (constraint satisfied — stdlib + existing `mvdan.cc/sh`).

---

## Security Domain

> The phase's central threat is injection (T-01-06). `nyquist_validation` is `false`, so the
> formal Validation Architecture section is omitted; this security section is retained because
> injection is the phase's primary risk.

### Applicable threat controls for zsh code emission

| ASVS-ish Category | Applies | Standard Control (verified) |
|-------------------|---------|-----------------------------|
| V5 Output/Injection encoding | **yes** | `zquote()` single-quote `'\''` escaping at every static value site (POC-Inj/Eval) |
| V5 Input trust boundary | yes | Values originate from a parsed (non-executed) AST; dynamic values stay verbatim as EVAL-01-equivalent to the owner's `.zshrc` (D-13) |
| V6 Cryptography | no | No crypto in this phase; secret *deref* is Phase 5 |

### Known threat patterns for zsh emit

| Pattern | STRIDE | Standard Mitigation (verified) |
|---------|--------|--------------------------------|
| Shell injection via unescaped value in `eval`'d block | Elevation of Privilege / Tampering | `zquote()` — survives double `eval` across scalar/alias/function contexts (POC-Eval) |
| Command execution via verbatim dynamic value | (accepted risk, owner-trust) | Only `Entry.Dynamic` values emitted verbatim; equivalent to the owner's own `.zshrc` (OQ-6); SHARE-01 future gate |
| State residue → environment confusion | Tampering | PATH rebuild-from-base; drift-guarded restore; `${+name}` set-tests (POC-Z6/Z8) |

---

## Verified-Claims Appendix

Every claim below was produced by running the command under `zsh 5.9`/`go 1.25.7` in a
throwaway scratch dir (`.../scratchpad/poc`, not in the repo). Output shown is observed.

**POC-Z1a — `${functions[name]}` excludes the `name(){` wrapper, no trailing newline (body lines usually tab-prefixed, but NOT universally — C12):**
`zsh -f -c 'ff() { echo hi }; print -r -- "${functions[ff]}"' | od -c` → `\t e c h o   h i` (body only, 8 bytes, no trailing newline; the `\n` under `print` is added by print). Heredoc content/terminator lines have NO leading tab, so "leading tab per line" is not a universal invariant.

**POC-Z1d — a captured function body re-establishes byte-identically:**
`functions[ff]=$cap; [[ "${functions[ff]}" == "$cap" ]]` prints `BYTE-IDENTICAL` for a multi-line body.

**POC-Z2a — `${aliases[name]}` is the RHS only:**
`alias gs='git status'; print -r -- "${aliases[gs]}"` → `git status` (not `gs=git status`).

**POC-Z3a — a naive `name\tbody` single-line dump breaks on a multi-line body:**
`print -r -- "ff\t${functions[ff]}" | od -c` shows embedded `\n`s inside the record.

**POC-Go-RT — NUL-record encoding round-trips a multi-line body byte-for-byte (zsh→Go→zsh):**
`go test -run TestFuncBodyRoundTrip` PASS; parsed `ff` body = `"\techo one\n\techo 'has a $quote and ; semicolon'\n\techo two"`; re-established identical.

**POC-Z4a — set-test for a LITERAL-named slot is `${+name}` (NO `(P)`) — C1 CORRECTED:**
`${+FOO}`: `unset FOO`→`0`; `FOO=""`→`1`; `FOO=bar`→`1` (distinguishes unset/empty/set). **`${(P)+FOO}` is WRONG for a literal slot** — `(P)` indirects through the VALUE of `FOO` (`FOO=bar; ${(P)+FOO}`→`0`, tests a param named `bar`). Use `(P)` only when the slot NAME is stored in another var (`name=FOO; ${(P)+name}`→matches FOO). `-n`/`-z` cannot distinguish unset from empty (both truthy-false for `""`).

**POC-Z4b — `${(P)var}` indirects through the name in `$var`:**
`EDITOR=nvim; var=EDITOR; print ${(P)var}` → `nvim`.

**POC-Z5 — plain fn lets `setopt` escape; `emulate -L` / `LOCAL_OPTIONS` auto-revert:**
plain → `EXTENDED_GLOB ON after return`; `emulate -L` → `reverted at return`; `LOCAL_OPTIONS` → `reverted at return`.

**POC-Z6a/6b — PATH rebuild-from-base stable, blind append grows:**
5 rebuild-from-base applies → `#path=3` (stable); 5 blind prepends → `#path=7` (residue).

**POC-Z6c — CORRECTED (C10):** `${path:#/opt/x}` is a GLOB/pattern subtraction, NOT literal-equality. It removes metacharacter-free elements cleanly, but a PATH element (or target) containing `? * [ ]` OVER-matches and deletes siblings (`${path:#/opt/tool?}` deleted `/opt/toolX`/`/opt/toolY`). Ownership-aware element removal must use LITERAL-equality (`[[ $e == $target ]]` rebuild), not `${path:#pattern}`. And under `typeset -U` a co-owned entry collapses to one, so per-addition subtraction strips a shared entry (C11) — needs a reference-count/set-based ownership model.

**POC-Z7 — `zsh -n` gate:** valid script → exit 0; unterminated-quote script → exit 1.

**POC-Inj — `zquote` survives all adversarial vectors (direct source):**
`go test -run TestInjectionVectors` PASS for `'`, `;cmd`, `$(...)`, backtick, newline+cmd, `'\''`-chain, `$HOME`-literal, kitchen-sink — no canary, byte-literal round-trip.

**POC-Eval — CORRECTED (C6):** `zquote` survives the double `eval` layer (`loader eval "$block"`) for the **scalar-export** and **alias-body** contexts only — no canary, value literal. It DOES NOT protect the **function-body** context: a function body is live code, so `eval "functions[fv]='…; touch CF'"; fv` FIRES the canary and an adversarial `'` gives "invalid function definition". Function bodies are restored by verbatim capture-and-reassign (`functions[name]=$capturedBody`, NUL-captured — POC-Go-RT), not zquote.

**POC-Dyn — the static/dynamic tension:** verbatim `x=$HOME/go` → `/Users/poc/go` (expands); `x='$HOME/go'` (zquote'd) → `$HOME/go` (literal). `go test -run TestDynamicVerbatimExpands` PASS.

**POC-J1 — `*string`+`omitempty` tri-state marshals/unmarshals losslessly:**
nil → key omitted; `&""` → `"original":""`; `&"vim"` → `"original":"vim"`. `TestScalarTriState` PASS.

**POC-J2 — a uniform `map[string]string` CANNOT unmarshal the fixture's `functions.added` array:**
`json: cannot unmarshal array into Go struct field nameSetMap.added of type map[string]string`. `TestFunctionsAddedArrayIntoMapFails` PASS.

**POC-J3 — two distinct types (`aliasSet` map + `funcSet` array) match the fixture:**
`TestTwoTypeShapeMatchesFixture` PASS.

**POC-J4 — a custom `UnmarshalJSON` accepts both shapes into a uniform map:**
`TestFlexUnmarshalBothShapes` PASS (array→`{"work_deploy":""}`, map→`{"gs":"git status"}`).

**POC-Z8b/8c — `${(P)+literalName}` tests the wrong parameter; `${+name}` (no P) is correct:**
`(P)+ZP_..._ll` → `0` (wrong); `${+ZP_..._ll}` → `1`, restores `ll`=`ls -lh`.

**POC-Z8d — full apply→deactivate cycle with corrected guards is byte-identical:**
`ZERO-RESIDUE: byte-identical` (includes a shadowed `ll` restore).

**POC-Z8e — `-n` guard silently drops an empty-body shadowed alias; `${+SLOT}` restores it:**
`-n` → `SKIPPED (empty prior silently lost)`; `${+SLOT}==1` → restores `aliases[emptyalias]` (empty body preserved).

**POC-Z9a — `zsh -f` inherits parent env (Pitfall 5):**
`FOO_INHERITED=hello zsh -f -c 'print ${FOO_INHERITED-<unset>}'` → `hello`.

**POC-Z9c/9d — options snapshot stable across a setopt/unsetopt no-op; exported var shows `scalar-export` in `${(@kv)parameters}`** (matches the existing `*export*` glob).

---

## Risks / Landmines for the Planner

1. **Correct the Phase 1 restore guards (OQ-8).** The plan MUST specify `${+name}` set-tests for shadow restore, not the snippet's `-n` / `${(P)+literalName}`. Otherwise empty-body shadows and (worse) *all* literal-named-slot restores silently no-op. This is the single most dangerous copy-paste trap in the phase.
2. **Do not optimize the diff.** Full-deactivate-A-then-full-activate-B is the zero-residue-safe order Phase 1 validated; skipping shared names re-introduces residue.
3. **Apply `zquote` at EVERY static value site** — scalar `applied`, alias `Added` body, function shadow body. A single missed site is a silent injection hole; prefer `strings.Builder` over `text/template` so each site is visible (§4). Add an explicit static-metacharacter-always-quoted test (OQ-6).
4. **Key quote-vs-verbatim strictly on `Entry.Dynamic`** and carry that bool onto the `Plan` op — never re-derive it in `emit.go` from string inspection.
5. **Sanitize runtime slot names (OQ-10)** — a profile named `feature/x` yields an invalid `typeset -g` target.
6. **Snapshot inside one `zsh -f` process** (Pitfall 5) — the property test must not compare across subprocesses with divergent inherited env.
7. **The property test must supply the `zp_*` helpers inline** (OQ-5/OQ-9) so it runs without the Phase 5 loader; pin whether `emit.go` also emits a self-contained helper block.
8. **The token-free grep for `core/activate` must match zsh *syntax* string literals** (`"unalias "`, `"unset -f"`, `"setopt "`), not the Go op-type identifiers (`Unalias`, `SetOption`), or it self-triggers a false positive (§3).
9. **`emulate -L zsh` is correct in `introspectScript` but forbidden in `emit.go`** — do not copy the isolation line into the loader functions (Pitfall 1).

---

## Sources

### Primary (HIGH confidence — verified by POC this session)
- Live `zsh 5.9` + `go 1.25.7` POC runs — all claims in the Verified-Claims Appendix (commands + observed output).
- `core/shell/zsh/introspect.go`, `core/shell/zsh/regen.go`, `core/shell/provider.go`, `core/model/{identityset,profile,secretref}.go`, `core/ir/route.go`, `core/store/{dto,roundtrip_test}.go`, `core/cmd/zsh-pro/main.go` — read this session.
- `01-MANIFEST-SHAPE.md`, `01-FINDINGS.md` (Loader Reference Snippet), `04-SPEC.md`, `04-CONTEXT.md`, `04-OPEN-QUESTIONS.md` — the locked grounding.

### Secondary (established, not re-verified)
- zsh 5.9 parameter-expansion semantics (`(P)`, `${+name}`, `${(@k)...}`) — behavior confirmed by POC rather than doc citation.

### Tertiary
- None — no unverified web claim is load-bearing in this report.

---

## Metadata

**Confidence breakdown:**
- Manifest types / JSON / tri-state / OQ-4: **HIGH** — every marshal/unmarshal behavior verified in Go (POC-J1..J4).
- `core/activate` builder/diff/Plan: **HIGH** on structure (reuses proven `route.go` shape); **MEDIUM** on the exact op-struct layout (discretion, D-08).
- emit.go injection quoting: **HIGH** — verified against adversarial vectors under double `eval` (POC-Inj/Eval).
- emit.go restore guards: **HIGH** — the two Phase 1 bugs found and the fixes verified (POC-Z8, OQ-8).
- introspect body-dump: **HIGH** — NUL-record round-trip verified byte-identical (POC-Go-RT).
- property-test harness: **HIGH** on the zero-residue mechanics (POC-Z8d); **MEDIUM** on the mutated-emitter mechanism (discretion, D-17).

**Research date:** 2026-07-01
**Valid until:** stable — pinned to zsh 5.9 / Go 1.25 semantics and locked D-01..D-17; re-verify only if the zsh version or the manifest shape changes.
