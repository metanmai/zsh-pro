# Phase 4: Manifest Builder + Emit - Pattern Map

**Mapped:** 2026-07-01
**Files analyzed:** 8 (3 new-file targets, 4 existing-file extensions, 1 property test)
**Analogs found:** 8 / 8

> Every new/modified file in this phase has a strong existing analog in the tree.
> The phase is deliberately structured so that each deliverable copies a proven
> shape: the manifest copies `SecretRef`'s direct-json-tag precedent, the builder
> copies `routeManaged`'s classification gate, `emit.go` copies `regen.go`'s
> `fmt.Sprintf` codegen style (plus a NEW `Dynamic`-keyed quote branch that
> `regen.go` does NOT have — see C7), the introspect extension copies the existing
> `zsh -f -c` subprocess shape additively, and the property test copies the
> `LookPath`-guarded zsh-requiring test precedent (`roundtrip_test.go`).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `core/model/manifest.go` (NEW) | model | transform (domain record) | `core/model/secretref.go` | exact (direct json-tag, stdlib-only) |
| `core/activate/builder.go` (NEW) | service | transform (Profile→Manifest) | `core/ir/route.go` (`routeManaged`) + `core/shell/zsh/regen.go` (`switch e.Kind`) | role-match (agnostic classify) |
| `core/activate/diff.go` (NEW) | service | transform (Manifest×Manifest→Plan) | — (no diff exists) | no analog (see below) |
| `core/activate/plan.go` (NEW) | model | value type | `core/model/block.go` typed-enum + `secretref.go` | role-match (agnostic value + typed consts) |
| `core/shell/zsh/emit.go` (NEW) | utility | transform (Plan→zsh strings) | `core/shell/zsh/regen.go` | role-match (structured-field → zsh codegen) |
| `core/shell/zsh/introspect.go` (MODIFY) | utility | file-I/O + subprocess | itself (additive extension) | exact (extend in place) |
| `core/model/identityset.go` (MODIFY) | model | transform | itself (additive fields) | exact (add companion maps) |
| `core/shell/provider.go` (MODIFY) | config (seam) | request-response | `shell.Regenerator` interface (same file) | exact (mirror the seam) |
| `core/cmd/zsh-pro/main.go` (MODIFY) | config (wiring) | wiring | itself (Regenerator wiring) | exact (add Emitter injection) |
| `core/activate/roundtrip_test.go` or `core/shell/zsh/emit_test.go` (NEW) | test | subprocess property test | `core/store/roundtrip_test.go` + `core/shell/zsh/introspect_test.go` | exact (LookPath skip-guard) |

## Pattern Assignments

### `core/model/manifest.go` (model, transform) — NEW

**Analog:** `core/model/secretref.go` (D-01 names this the direct-json-tag precedent). The Manifest IS the wire record, so — unlike `model.Entry`, which is untagged and serialized via `core/store/dto.go` — it carries its own `json:` tags in-place.

**Direct-json-tag struct pattern** (`core/model/secretref.go:25-28`):
```go
type SecretRef struct {
	Kind SecretRefKind `json:"kind"` // resolver backend that holds the value
	Key  string        `json:"key"`  // name-scoped lookup key (e.g. "API_KEY")
}
```
Copy this discipline: `package model`, stdlib-only (no imports), one type cluster per file, exported doc comment on every type, short trailing comment per field.

**Typed-tag enum precedent for `schema`** (`secretref.go:6-18` — `SecretRefKind string` with `const` block). Mirror for the `schema` literal: a `const SchemaV1 = "v1"` (D-03) — the exact literal from `01-MANIFEST-SHAPE.md`, a forward-compat gate checked before reverse logic. This also matches `core/model/category.go` / `block.go` typed-string-enum convention.

**Target layout** (from RESEARCH §2, keyed to the validated `01-MANIFEST-SHAPE.md` shape, D-02):
```go
type Manifest struct {
	Profile   string       `json:"profile"`   // == git branch / ZSHPRO_PROFILE (Phase 3 D-13)
	Schema    string       `json:"schema"`    // literal "v1" (D-03)
	Env       []Scalar     `json:"env"`
	Lists     []ListDelta  `json:"lists"`
	Aliases   AliasSet     `json:"aliases"`   // OQ-4/OQ-11: Fork A default (see below)
	Functions FuncSet      `json:"functions"` // OQ-4/OQ-11: Fork A default (see below)
	Options   []OptionSet  `json:"options"`
}

type Scalar struct {
	Name     string  `json:"name"`
	Applied  string  `json:"applied"`
	Original *string `json:"original,omitempty"` // LOAD-BEARING tri-state (D-02): nil→unset, &""→empty, &"vim"→value
}

type ListDelta struct {
	Name      string   `json:"name"`      // "PATH" | "FPATH"
	Additions []string `json:"additions"`
	Deletions []string `json:"deletions"`
}

type OptionSet struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	WasOn   bool   `json:"was_on"` // note snake_case tag (D-04)
}
```

**The `*string` + `omitempty` tri-state is the single load-bearing type choice** (D-02, RESEARCH §2). Precedent for `*T` + `omitempty` in-tree: `Secret *model.SecretRef \`json:"secret,omitempty"\`` (`core/store/dto.go:38`). The pointer presence/absence encodes was-set-vs-was-unset; a present `&""` encodes was-empty.

**OQ-4/OQ-11 fork — RESEARCH recommends Fork A (two distinct types):** `AliasSet{Added map[string]string; Shadowed map[string]string}` + `FuncSet{Added []string; Shadowed map[string]string}` — matches the validated fixture byte-for-byte (`aliases.added` is a map, `functions.added` is an array). A uniform `map[string]string` for both (Fork B) FAILS to unmarshal the `functions.added` array unless the fixture is edited (proven, POC-J2). CONTEXT D-02 tentatively wrote "uniform map"; RESEARCH A1 flags the round-trip acceptance treats the fixture as a byte-identical oracle, favoring Fork A. This is a planner decision — resolve at plan time; both are reversible.

**JSON tags are snake/lower** (D-04): `profile`, `schema`, `name`, `applied`, `original`, `additions`, `deletions`, `added`, `shadowed`, `enabled`, `was_on` — NOT the store DTO's camelCase (`startLine`, `cmdName`).

---

### `core/model/manifest.go` marshal helpers (if any) — analog `core/store/dto.go`

**Deterministic marshal pattern** (`core/store/dto.go:51-61`) — if the manifest ships a `Marshal`/`Unmarshal` helper (or the round-trip test marshals directly), copy the deterministic two-space-indent + trailing-newline shape:
```go
b, err := json.MarshalIndent(dto, "", "  ")
if err != nil {
	return nil, err
}
return append(b, '\n'), nil
```
Maps sort keys deterministically in `encoding/json` (Go guarantee), so `aliases.added`/`shadowed` are diff-stable. Note: per D-01 the manifest tags itself directly and does NOT need a DTO indirection layer like `entryDTO` — this is the KEY divergence from `dto.go` (copy the marshal *style*, not the DTO layer).

---

### `core/activate/builder.go` (service, transform: Profile→Manifest) — NEW

**Analog:** `core/ir/route.go` `routeManaged` (D-06 — reuse the exact classification shape, do NOT re-derive the admitted set) + `core/shell/zsh/regen.go`'s `switch e.Kind` dispatch.

**Package-boundary constraint** (D-05): `core/activate` imports `core/model` ONLY — never `core/shell/zsh`. Copy the shell-agnostic discipline documented in `core/store/dto.go:1-7` package comment ("must never import the concrete zsh provider package").

**Classification gate to reuse** (`core/ir/route.go:35-71`). The builder consumes only `e.EffectiveManaged() == true` entries (`core/model/profile.go:42-51`) and classifies each by the SAME `Category`+`Kind` shape checks routeManaged proved:
```go
switch b.Kind {
case model.KindAlias:
	return len(b.Names) == 1 && !b.Flagged        // → aliases.Added[name] = Value
case model.KindFuncDecl:
	return true                                     // → functions.Added += name
case model.KindAssignment:
	if len(b.Names) != 1 || b.Append { return false }
	if b.Array { return false }
	return cat == model.CatEnvironment || cat == model.CatPath || cat == model.CatSecrets
	// CatEnvironment/CatSecrets → Scalar{Name, Applied: Value}
	// CatPath → ListDelta (PATH|FPATH by name); Value split into Additions vs runtime base (D-07)
case model.KindCommand:
	return cat == model.CatOptions &&
		(b.CmdName == "setopt" || b.CmdName == "unsetopt") &&
		len(b.Names) > 0                            // → OptionSet{Name, Enabled: CmdName=="setopt"}
}
```
Note: `routeManaged` operates on `model.Block`; the builder operates on `model.Entry` (which carries the same `Kind`/`Category`/`CmdName`/`Names`/`Value` fields, `core/model/profile.go:23-36`). The routing LOGIC is identical; the receiver type differs.

**Builder fills declarative intent ONLY** (D-06): `added`/`applied`/`enabled`/`additions`. Leave `Shadowed`/`Original`/`WasOn`/`Deletions` as runtime-reconciled slots (nil/empty) — those are LIVE facts the emitted apply code captures, NOT authored by the builder. Imperative / `OverrideUnmanaged` / `Opaque` → no part (acceptance: an imperative entry yields nothing).

**Constructor convention** (CLAUDE.md): if the builder is a type, `New(...) *T`; if it is a pure function, name it `Build` (noun-verb form, matching `ir.Build`, `Generator.Build`).

---

### `core/activate/diff.go` (service, transform: Manifest×Manifest→Plan) — NEW

**Analog:** none exists (no diff/plan structure in the tree today — SPEC Req 3 "Current: No diff and no plan structure exist"). Follow RESEARCH §3 recommendation and the general agnostic-transform discipline of `core/ir`.

**Ordering invariant** (D-08): `Plan.Deactivate` (all of A's reverse ops) fully precedes `Plan.Activate` (all of B's apply ops). Empty cases are first-class (D-09): `diff(A, nil)` = pure deactivate, `diff(nil, B)` = pure activate, `diff(A, B)` = the switch case — all through one builder. RESEARCH §3 warns AGAINST an "optimized" diff that skips shared names (correctness hazard) — use naive full-deactivate-then-full-activate.

**Token-free acceptance** (D-08/D-11, RESEARCH C22): a PRECISE grep test (case-sensitive, word-boundary `\bunalias\b`/`\bunset -f`/`\bunsetopt\b`, excluding Go identifier type names like `Unalias`) confirms `core/activate` contains no zsh reverse-op token as a string literal.

---

### `core/activate/plan.go` (model, value type) — NEW

**Analog:** `core/model/block.go` (typed-enum-const convention) + `core/model/secretref.go` (agnostic value struct).

**Op vocabulary** (D-08, RESEARCH §3 recommends a tagged-union of op structs in an ordered slice):
```go
type Plan struct {
	Deactivate []Op // A's reverse ops
	Activate   []Op // B's apply ops
}
// Op is a sealed interface; concrete ops carry ONLY agnostic data (no zsh tokens):
//   deactivate: RestoreScalar, UnsetScalar, RebuildListFromBase, Unalias,
//               RestoreShadowedAlias, UnsetFunc, RestoreShadowedFunc, RestoreOption
//   activate:   SetScalar{...,Dynamic bool}, ApplyListDelta, AddAlias{...,Dynamic bool},
//               AddFunc, SetOption
```
The `Dynamic bool` carried on `SetScalar`/`AddAlias` is what `emit.go` keys the quote-vs-verbatim split on. It is agnostic data (a bool), not a zsh token — `core/activate` stays token-free. Exact identifiers and whether ops are a tagged-union vs enum+payload is Claude's discretion (CONTEXT).

---

### `core/shell/zsh/emit.go` (utility, transform: Plan→zsh strings) — NEW

**Analog:** `core/shell/zsh/regen.go` — the forward-only templater. `emit.go` is its REVERSE-and-loader counterpart, a DISTINCT new file that shares NO code with regen.go (RESEARCH C7). regen.go stays forward-only (D-11).

**Codegen style to copy** (`core/shell/zsh/regen.go:24-72`) — method-on-`Provider{}`, `fmt.Sprintf`/`strings.Builder`, `switch` on the shape, total (default case never empty):
```go
func (Provider) Regenerate(e model.Entry) string {
	switch e.Kind {
	case model.KindAssignment:
		...
		if e.Exported {
			return fmt.Sprintf("export %s=%s", e.Names[0], e.Value)
		}
		return fmt.Sprintf("%s=%s", e.Names[0], e.Value)
	case model.KindAlias:
		return fmt.Sprintf("alias %s=%s", e.Names[0], e.Value)
	...
	}
}
```
RESEARCH §4 recommends `strings.Builder` + explicit `zquote()` per value-site (NOT `text/template`) precisely because injection safety depends on never missing an escape site, and an explicit `b.WriteString("export " + name + "=" + zquote(val))` makes each site auditable.

**CRITICAL DIVERGENCE from regen.go — the `Dynamic`-keyed quote branch is NEW behavior** (RESEARCH C7): regen.go emits `e.Value` VERBATIM in every branch and NEVER reads `Dynamic`. emit.go must ADD the split (D-13, OQ-6):
```go
if op.Dynamic { write(value) /* verbatim, stays late-bound */ } else { write(zquote(value)) /* injection-safe */ }
```

**The injection escape** (D-13, T-01-06, verified safe in RESEARCH §4):
```go
// zquote wraps s as a zsh single-quoted literal, escaping embedded ' with '\''.
func zquote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
```
Applies to scalar-export and alias-body VALUE contexts ONLY. Function bodies are the EXCEPTION (C6): a function body is live code and CANNOT be made inert by single-quoting — restore it by verbatim capture-and-reassign `functions[name]=$capturedBody` (body round-trips as DATA via the NUL body dump), never zquote'd.

**Plain-function constraint** (D-10, Pitfall 1): emitted `apply_*`/`deact_*` functions MUST be plain — NO `emulate -L`, NO `LOCAL_OPTIONS`, NO `setopt localoptions`, or `setopt`/`unsetopt` auto-revert at return. This is the EXACT OPPOSITE of `introspectScript`'s `emulate -L zsh` line (`introspect.go:24`) — do NOT copy that line into emit.go.

**Reached via a NEW `shell.Emitter` seam** (D-10) — see provider.go below.

**Shadow-guard correction (OQ-8, from RESEARCH — do NOT reproduce the Phase 1 snippet verbatim):** the Loader Reference Snippet's shadow-restore guards are buggy. Use `${+name}` set-tests (NOT `-n` truthiness, NOT `${(P)+literalName}`). The `(P)` indirection form is correct ONLY for the ENV path where the variable holds the NAME (`${(P)+var}==1`). RESEARCH Pitfalls 3/4 and §Don't Hand-Roll detail both bugs.

---

### `core/shell/zsh/introspect.go` (utility, subprocess) — MODIFY (additive, D-14)

**Analog:** itself — extend the existing `introspectScript` const and `parseIntrospect` ADDITIVELY. Preserve the exact `zsh -f -c` + 5s-timeout + `Available:false`-on-error shape (`introspect.go:42-53`):
```go
func (p Provider) Introspect(path string) (model.IdentitySet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, buildinfo.Name, path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return model.IdentitySet{Available: false}, err
	}
	return p.parseIntrospect(out.String()), nil
}
```
Do NOT change this subprocess shape (graceful degradation is a hard constraint).

**Existing name-dump pattern to extend** (`introspect.go:27-30`):
```go
print -r -- '##ALIASES##'
for k in "${(@k)aliases}"; do print -r -- "$k"; done
```
Add NEW body sections AFTER the name sections (D-14, RESEARCH §5). CRITICAL (C23): body sections must NOT reuse the line-oriented `##DELIMITER##` framing — use NUL-delimited `print -rN -- name body` records, because a multi-line function body's newlines split a line-parser and a heredoc line reading `##END##` collides with the line switch:
```zsh
print -r -- '##ALIASBODIES##'
for k in "${(@k)aliases}"; do print -rN -- "$k" "${aliases[$k]}"; done
print -r -- ''
print -r -- '##FUNCTIONBODIES##'
for k in "${(@k)functions}"; do print -rN -- "$k" "${functions[$k]}"; done
print -r -- ''
print -r -- '##END##'
```

**`parseIntrospect` line-switch to preserve + extend** (`introspect.go:55-90`): the existing `strings.Split(s, "\n")` + section-marker switch stays UNCHANGED for the name sections. Add a DEDICATED non-line body parser that slices the body section by known header/marker offsets and splits enclosed bytes on `\x00` — it must NOT scan line-by-line inside body bytes (C23). Name-only tests must keep passing (acceptance).

---

### `core/model/identityset.go` (model, transform) — MODIFY (additive, D-15)

**Analog:** itself — add companion fields, leaving existing `map[string]bool` maps intact (`core/model/identityset.go:4-11`):
```go
type IdentitySet struct {
	Aliases        map[string]bool   // unchanged
	Functions      map[string]bool   // unchanged
	Env            map[string]bool   // unchanged
	Path           []string          // unchanged
	Options        map[string]bool   // unchanged
	Available      bool              // unchanged
	AliasBodies    map[string]string // NEW — name→RHS body
	FunctionBodies map[string]string // NEW — name→body
}
```
Additive is non-breaking: `analyze` consumes only `ids.Available` today (`core/analyze/analyzer.go:84`, verified in RESEARCH), and all existing `IdentitySet{}` literals are keyed, so adding fields is source-compatible. Populate the new maps only on the success path in `parseIntrospect`; a zsh-absent/timeout run still returns `IdentitySet{Available:false}`.

---

### `core/shell/provider.go` (config/seam, request-response) — MODIFY (D-10)

**Analog:** the `shell.Regenerator` interface in the SAME file (`core/shell/provider.go:33-43`) — the new `Emitter` seam mirrors it exactly:
```go
type Regenerator interface {
	Regenerate(e model.Entry) string
}

type Provider interface {
	Parser
	Classifier
	Introspector
	Regenerator
}
```
Add an `Emitter` interface alongside `Regenerator`, with a doc comment explaining it is the SOLE reverse-syntax codegen seam (mirror the Regenerator doc's "single place ... zsh syntax is generated" language). Exact signature is Claude's discretion (single `Emit(Plan) (apply, deactivate string, err error)` vs two methods, CONTEXT). The `Emitter` takes a `core/activate.Plan` — note this makes `shell` import `core/activate` (an agnostic value package), which is acceptable since both are agnostic; OR the Plan type lives such that no cycle forms (planner resolves placement). Whether `Emitter` joins the composite `Provider` interface or stands alone is the planner's call — keep the ISP discipline (narrowest interface per caller).

---

### `core/cmd/zsh-pro/main.go` (config/wiring) — MODIFY (D-10)

**Analog:** the Regenerator wiring in the SAME file (`core/cmd/zsh-pro/main.go:16-36`) — the sole composition root, the only package that imports `core/shell/zsh`:
```go
func main() {
	provider := zsh.Provider{}          // is the Parser/Classifier/Introspector AND Regenerator
	...
	s, err := store.New(dir, provider, kc)  // provider injected as the Regenerator seam
	_, _ = s, err
	os.Exit(cli.New(provider).Run(os.Args[1:], os.Stdout, os.Stderr))
}
```
Wire the new Emitter here exactly like the Regenerator: `zsh.Provider{}` (or a new emitter type on it) satisfies `shell.Emitter`, injected into whatever consumer needs it. Preserve the package doc's single-composition-root invariant statement (`main.go:1-5`). Note the phase's downstream consumer (the loader/CLI verbs) is Phase 5 — like the store, the Emitter may be constructed + injectable now but not yet driven by a verb (mirror the `_, _ = s, err` non-fatal wiring comment at `main.go:29-34`).

---

### Property test — `core/shell/zsh/emit_test.go` or `core/activate/roundtrip_test.go` (test, subprocess) — NEW (D-16/D-17)

**Analogs:** `core/store/roundtrip_test.go` (the zsh-requiring, `LookPath`-guarded, external-test-package precedent) + `core/shell/zsh/introspect_test.go` (the skip-guard shape).

**Skip-guard pattern to copy** (`introspect_test.go:11-13`, `roundtrip_test.go:149-151`):
```go
if _, err := exec.LookPath("zsh"); err != nil {
	t.Skip("zsh not installed; skipping ...")
}
```
Runs in the DEFAULT suite (NOT behind a `spike` build tag — D-17); skips cleanly when zsh is absent.

**Full-cycle round-trip structure to copy** (`roundtrip_test.go:147-219`): build ≥2 profiles → `activate.Build` → `Manifest` → `activate.Diff` → `Plan` → `emit.Emit` → apply/deactivate strings → source under `zsh -f` → snapshot via the (extended) `introspectScript` → assert `reflect.DeepEqual` (RESEARCH §6 prefers literal STRING equality of the six-class snapshot, the Phase 1 bar, NOT `IdentitySet` field checks) across N ≥ 20 random switch orders + `$#path` stable.

**Fixture style to copy** (`roundtrip_test.go:43-48`): the `roundTripSrc` const covering all declarative classes. RESEARCH §6 wants ≥2 profiles with at least one shadow collision (both override the same pre-existing `ll`/`ff`), one shared PATH addition (`/usr/local/bin` — the ownership criterion), and one dynamic value (`export GOPATH=$HOME/go`).

**Single-process snapshot** (RESEARCH §6, Pitfall 5): run the whole set-base→snapshot→apply→snapshot→deactivate→snapshot sequence in ONE sourced `zsh -f` script (the harness supplies the `zp_*` helper defs inline, OQ-5), because `zsh -f` inherits parent env and cross-process snapshots produce false diffs.

**Mutated-emitter negative check** (D-16, first-class acceptance): an in-test emitter variant that swaps `PATH="$ZP_BASE_PATH"; path=(<add> $path)` for a blind `path=(<add> $path)` MUST flip the test to fail (grows `$#path`). RESEARCH §6 recommends in-test injection over a build tag.

## Shared Patterns

### Agnostic-package boundary (no `core/shell/zsh` import)
**Source:** `core/store/dto.go:1-7` package comment; `core/cmd/zsh-pro/main.go:1-5`.
**Apply to:** `core/activate` (all files) and `core/model/manifest.go`.
`core/activate` and `core/model` import `core/model` / stdlib only; the concrete emitter is reached through the `shell.Emitter` interface, wired at `main.go`. This is the single-composition-root invariant.

### Direct json-tagging (no DTO indirection) for wire records
**Source:** `core/model/secretref.go:25-28`; contrast `core/store/dto.go:26-39` (which DOES use a DTO because `Entry` is untagged).
**Apply to:** `core/model/manifest.go`.
The Manifest IS the wire record → tag it directly (D-01). Use snake/lower keys (D-04), `*string`+`omitempty` for tri-state (D-02).

### `switch e.Kind` / `switch b.Kind` classification dispatch
**Source:** `core/ir/route.go:35-71` (routeManaged) and `core/shell/zsh/regen.go:24-72` (Regenerate).
**Apply to:** `core/activate/builder.go` (classify entries into parts) and `core/shell/zsh/emit.go` (dispatch ops to zsh). Reuse the routeManaged shape checks; do not re-derive the admitted set.

### Sandboxed subprocess + graceful degradation
**Source:** `core/shell/zsh/introspect.go:42-53`.
**Apply to:** the introspect extension (preserve the exact shape) and the property-test harness (`zsh -f`, `LookPath` skip-guard).
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", script, ...)
if err := cmd.Run(); err != nil {
	return model.IdentitySet{Available: false}, err
}
```

### Interface seam mirroring (ISP)
**Source:** `core/shell/provider.go:33-43` (`Regenerator` + composite `Provider`).
**Apply to:** the new `shell.Emitter` seam. Narrow interface, doc-commented as the single reverse-codegen seam, wired at the composition root.

### Deterministic JSON marshal
**Source:** `core/store/dto.go:51-61`.
**Apply to:** any manifest marshal helper + the round-trip test. `json.MarshalIndent(v, "", "  ")` + trailing `\n`.

### `LookPath`-guarded zsh-requiring test in an external test package
**Source:** `core/store/roundtrip_test.go` (whole file) + `core/shell/zsh/introspect_test.go:10-13`.
**Apply to:** the zero-residue property test.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `core/activate/diff.go` | service | transform (Manifest×Manifest→Plan) | No diff/plan structure exists in the tree (SPEC Req 3). Follow RESEARCH §3 (naive full-deactivate-then-full-activate, ordered slice of ops). The agnostic-transform discipline of `core/ir` is the closest structural precedent, but there is no existing two-input diff to copy. |

> The `Op` sealed-interface / tagged-union shape in `plan.go` also has no exact
> in-tree precedent (the model uses typed-string enums, not sealed interfaces).
> RESEARCH §3 recommends the tagged-union; the planner should treat this as new
> design guided by RESEARCH, not a copy of an analog.

## Metadata

**Analog search scope:** `core/model/`, `core/activate/` (new), `core/shell/`, `core/shell/zsh/`, `core/ir/`, `core/store/`, `core/cmd/zsh-pro/`
**Files scanned:** 11 (secretref.go, profile.go, identityset.go, provider.go, route.go, regen.go, introspect.go, dto.go, main.go, roundtrip_test.go, introspect_test.go) + category.go/block.go const grep + 01-MANIFEST-SHAPE.md
**Pattern extraction date:** 2026-07-01
