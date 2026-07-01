# Phase 4: Manifest Builder + Emit — Specification

**Created:** 2026-07-01
**Ambiguity score:** 0.13 (gate: ≤ 0.20)
**Requirements:** 8 locked
**Mode:** `--auto` (interview skipped — initial ambiguity already below gate; derived from ROADMAP + REQUIREMENTS + Phase 1 spike outputs)

## Goal

A resolved `model.Profile` becomes a `model.Manifest` (the reversible record the runtime applies), `core/activate` diffs active-vs-target and produces a shell-agnostic deactivate-then-activate plan, and `core/shell/zsh/emit.go` renders that plan to zsh apply/deactivate code such that a sourced loader `eval`ing it applies and reverses declarative state with **zero residue** — verified byte-identical by a property test over N random switch sequences.

## Background

Phase 1 (SPIKE) proved zero-residue live hot-switch is feasible (verdict GO) and **fixed the exact `Manifest` JSON shape** — two hand-written instances of it drove a byte-identical six-class round-trip that survived ≥5 A↔B cycles plus the drift-guard sub-cases under sandboxed `zsh -f`. That shape (`01-MANIFEST-SHAPE.md`) is Phase 4's literal input: `env[]` scalars (drift-guarded, presence/absence of `original` encodes unset-vs-empty), `lists[]` (PATH/FPATH additions/deletions vs a captured base), `aliases`/`functions` (`added` + `shadowed` prior bodies), and `options[]` (`enabled` + `was_on`).

Phase 2 built the IR spine (`core/model.Profile`/`Entry`, `core/ir.Build`/`Regenerate`, the `shell.Regenerator` seam) and Phase 3 built the git store (`core/store`) whose `Read` returns a `model.Profile`. What does NOT exist today:
- `model.Manifest` and its parts (Scalar/ListDelta/NameSet/OptionSet) — no manifest type exists.
- `core/activate` — no package exists; nothing builds a manifest, diffs two, or produces an apply/deactivate plan.
- `core/shell/zsh/emit.go` — the file does not exist; `core/shell/zsh/regen.go` only emits *forward* declarative source for a single `Entry` (no `unalias`/`unset -f`/`setopt`/PATH-rebuild reverse code, no apply/deactivate loader functions).
- Alias/function **body** capture in introspection — `core/shell/zsh/introspect.go` dumps only the *names* present (`${(@k)aliases}`, `${(@k)functions}`); it never dumps `${aliases[name]}` / `${functions[name]}` bodies, so a shadowed prior definition cannot be restored.

Phase 1 recorded five mandatory loader carry-forwards for the emitter (capture the live prior, not a static `original`; `${(P)+var}==1` for unset-vs-empty; **plain** loader functions so options escape scope; rebuild PATH from a captured base, never `typeset -U`; solve the injection threat T-01-06). These are constraints below, not open questions.

## Requirements

1. **Manifest type + four parts**: `model.Manifest` carries the reversible record with exactly the four part types named in the ROADMAP.
   - Current: No `model.Manifest` type exists; only `model.Profile`/`Entry` and `model.IdentitySet`.
   - Target: `model.Manifest` (shell-agnostic, in `core/model`, no new deps) with `Scalar` (env name/applied/original — presence of `original` encodes was-set, absence encodes was-unset), `ListDelta` (name + additions + deletions for PATH/FPATH), `NameSet` (aliases/functions: `added` names + `shadowed` name→prior-body map), and `OptionSet` (name/enabled/was_on). A `schema` version tag and `profile` (branch name) are carried.
   - Acceptance: `model.Manifest` round-trips to/from the validated `01-MANIFEST-SHAPE.md` JSON with no field missing for any admitted class; a unit test constructs a manifest with all four part types populated and marshals/unmarshals it losslessly.

2. **Manifest builder from a resolved Profile**: `core/activate` builds a `model.Manifest` from a resolved `model.Profile`.
   - Current: Nothing turns a `Profile` into a `Manifest`; `core/store.Read` returns a `Profile` with no downstream builder.
   - Target: `core/activate` exposes a builder that consumes a `model.Profile` (only `EffectiveManaged()` declarative entries) and produces a `model.Manifest` classifying each entry into the right part (env scalar, PATH/FPATH list delta, alias/function name-set, option set); imperative/unmanaged entries are excluded by construction.
   - Acceptance: Building a manifest from a `Profile` containing at least one entry of each admitted class yields a manifest whose parts contain exactly those entries; an imperative entry (e.g. a `KindCompound` or forced-unmanaged) produces no manifest part.

3. **Active-vs-target diff → deactivate-then-activate plan (agnostic)**: `core/activate` diffs the active manifest against the target and emits a shell-agnostic ordered plan.
   - Current: No diff and no plan structure exist.
   - Target: `core/activate` produces an ordered `Plan` structure (a Go value type, NOT shell text) that is deactivate-of-prior **then** activate-of-target; the plan is expressed in agnostic operations (restore/unset scalar, rebuild list from base, unalias/restore-shadowed, unset-f/restore-shadowed, restore option) so `core/activate` never contains a zsh token.
   - Acceptance: A grep/test confirms `core/activate` emits no zsh syntax (no `unalias`/`unset`/`setopt`/`export`/`alias ` string literals); a plan built from active=A, target=B lists all of A's reverse ops ordered before all of B's apply ops.

4. **Single emit path renders plan to zsh (apply + deactivate)**: `core/shell/zsh/emit.go` is the sole place the apply/deactivate zsh code is generated.
   - Current: `emit.go` does not exist; `regen.go` emits only forward declarative source for one `Entry`; no reverse (`unalias`/`unset -f`/PATH rebuild/`setopt`) code is generated anywhere.
   - Target: `core/shell/zsh/emit.go` renders a `core/activate` `Plan` to zsh apply and deactivate code as **plain** loader functions (no `emulate -L`/`LOCAL_OPTIONS`, so options escape scope — Phase 1 carry-forward 3) that a sourced loader `eval`s; the milestone invariant holds — only `emit.go` writes `unalias`/`unset -f`/`setopt`/PATH-rebuild strings.
   - Acceptance: A test greps the whole tree and finds reverse-op zsh tokens (`unalias`, `unset -f`, `unsetopt`, PATH-array rebuild) generated only under `core/shell/zsh/emit.go`; emitted code is `zsh -n`-parseable (syntax-valid).

5. **Zero-residue under N random switch sequences (property test)**: switching leaves final state path-independent and dupe-free.
   - Current: No manifest/emit path exists to test; Phase 1's proof was hand-written throwaway (`scratch/`, git-ignored, deleted).
   - Target: A property test generates N random switch sequences over ≥2 profiles, applies the emitted apply/deactivate code under sandboxed `zsh -f`, and asserts the final `$aliases`/`$functions`/`exported env`/`$options`/`$PATH`/`$path` are byte-identical to the pre-activation snapshot regardless of switch order, with no PATH growth (element count stable) and no surviving alias/function/option from any prior profile.
   - Acceptance: The property test passes for N random sequences (N ≥ 20) with an empty snapshot diff; a deliberately mutated emitter (e.g. append PATH instead of rebuild) makes the test fail (the test actually detects residue).

6. **Ownership-aware, drift-guarded restore**: deactivate removes only what this profile added and only if the live value still equals what was applied.
   - Current: No restore logic exists.
   - Target: Emitted deactivate code (a) restores an env scalar only if the live value still equals the applied value (drift guard — a hand-edited managed var is left intact), using `${(P)+var}==1` for unset-vs-empty (Phase 1 carry-forwards 1+2); and (b) is ownership-aware for lists — it removes only entries this profile *added* vs the captured base and never strips a base/unmanaged entry the profile merely also added (e.g. `/usr/local/bin`).
   - Acceptance: A test where a managed env var is hand-edited after apply confirms deactivate does NOT clobber it, and an untouched managed var IS reversed; a test where two profiles both add `/usr/local/bin` confirms deactivating one does not remove it from PATH.

7. **Shadow capture + restore for aliases/functions**: a prior alias/function overridden by this profile is re-established on deactivate.
   - Current: Introspection dumps only names, not bodies; no shadow capture exists.
   - Target: The emitted apply code captures the live prior body of any alias/function name it is about to override (into per-terminal runtime undo state); deactivate `unalias`/`unset -f`s the names this profile *added* and restores each shadowed prior body byte-for-byte (`alias name=$prior` / `functions[name]=$prior`).
   - Acceptance: A test that activates a profile shadowing a pre-existing `ll` alias and `ff` function, then deactivates, confirms `ll` and `ff` are restored to their exact prior bodies (byte-identical), while names the profile *added* (not shadowed) are gone.

8. **Introspect extended to dump alias/function bodies**: introspection captures bodies so shadowed definitions are recoverable.
   - Current: `introspectScript` in `core/shell/zsh/introspect.go` dumps only names (`for k in "${(@k)aliases}"`); `model.IdentitySet` has `map[string]bool` for aliases/functions (no bodies).
   - Target: `introspectScript` additionally dumps each alias/function *body* (`${aliases[name]}` / `${functions[name]}`) in the section-delimited format, and `model.IdentitySet` (or an additive companion) carries the bodies; parsing degrades gracefully (`Available:false`) exactly as today when zsh is absent/times out.
   - Acceptance: `Introspect` on a fixture with a defined alias `gs='git status'` and a function `foo() { echo hi; }` returns the captured bodies (not just the names `gs`/`foo`); the existing name-only introspection tests still pass (additive, non-breaking).

## Boundaries

**In scope:**
- `model.Manifest` + the four part types (Scalar / ListDelta / NameSet / OptionSet) with a `schema` tag, in `core/model` (agnostic, no new deps).
- `core/activate` (new package): manifest builder from a `model.Profile`, active-vs-target diff, and a shell-agnostic ordered `Plan` (deactivate-then-activate).
- `core/shell/zsh/emit.go` (new file): renders a `Plan` to zsh apply + deactivate code as plain loader functions; the single place reverse zsh syntax lives.
- Drift-guarded env restore (`${(P)+var}==1`, reverse only if live==applied) and ownership-aware PATH-delta restore vs a captured base.
- Shadow capture (live prior body) + byte-for-byte restore of shadowed aliases/functions.
- Extended `introspectScript` + `model.IdentitySet` (or additive companion) to carry alias/function bodies.
- A zero-residue property test (N random switch sequences under sandboxed `zsh -f`) as the SW-02 regression pin.
- Injection-safe emission: user-controlled values escaped so they cannot break out of the emitted `eval`'d code (Phase 1 threat T-01-06, deferred here).

**Out of scope:**
- The sourced runtime loader itself, the `checkout`/`activate`/`deactivate`/`list`/`status` CLI verbs, per-terminal state wiring, and the `.zshrc` bootstrap block — that is Phase 5 (BOOT-01/02). Phase 4 emits the code a loader will `eval`; it does not install or invoke the loader.
- The base-capture *placement* in the live loader (which entry point captures `ZP_BASE_PATH` and the re-capture guard) — Phase 5's concern; Phase 4 emits a plan that assumes a base exists in runtime state (OQ-3).
- Secret **deref-on-switch** (resolving a `SecretRef` from the keychain/vault at apply time) — PROF-03 runtime half; Phase 4 may define the seam but end-to-end deref completes with the loader in Phase 5.
- Managing **keybindings** (`bindkey`) and **hooks** (`precmd_functions`/`chpwd_functions`) — deferred to the master block per OQ-1 (Phase 1 A3 punted the admit/exclude decision; safest default is exclude, additive to re-admit later).
- `compinit`/completion side effects — already excluded to the master block by Phase 1 (fpath array membership is admitted as a `ListDelta`; the imperative `compinit` invocation is not).
- Real `~/.zshrc` end-to-end ingest — Phase 6.
- Other shells (bash/fish) — the whole milestone is zsh-only.

## Constraints

- **No new dependencies** — `model.Manifest` and `core/activate` are stdlib-only; zsh emission stays within the existing `mvdan.cc/sh` + subprocess-`zsh` footprint.
- **Single zsh-syntax emit path** — only `core/shell/zsh/emit.go` may write `unalias`/`unset -f`/`setopt`/`unsetopt`/PATH-rebuild strings. `core/activate` stays shell-agnostic (emits a `Plan` value, never shell text) and must not import the concrete `core/shell/zsh` provider (single composition root preserved; reached via the `shell` seam).
- **Plain loader functions** — emitted apply/deactivate functions must NOT use `emulate -L` or `LOCAL_OPTIONS`, or `setopt`/`unsetopt` auto-reverts at function return (Phase 1 Pitfall 1 / carry-forward 3).
- **PATH is a delta, never a wholesale overwrite** — rebuild from a captured base + apply additions/deletions; never `typeset -U`, never blind append (Phase 1 Pitfall 2 / carry-forward 4; keeps `$#path` stable across cycles).
- **Trust the live prior, not the static `original`** — the emitted apply captures the live prior into runtime undo state; the manifest's `env[].original` is the declarative record the drift guard reconciles against (Phase 1 carry-forward 1; OQ-2).
- **Unset-vs-empty correctness** — use `${(P)+var}==1` (not a `-n` truthiness test) to distinguish an unset var from an empty one (Phase 1 carry-forward 2).
- **Injection safety** — user-controlled values (alias bodies, env values, function bodies) must be emitted so they cannot escape the `eval`'d code into arbitrary shell execution (Phase 1 threat T-01-06 is explicitly deferred to and must be solved in this phase). This is the phase where quoting/escaping bugs would become shell-injection bugs (ROADMAP research flag).
- **Never `eval` user config** — emission is static codegen from structured fields; no partial-eval executes user config (the milestone invariant).
- **Introspection stays best-effort** — extending `introspectScript` must preserve graceful degradation (`Available:false` when zsh is absent/times out); the change is additive to `model.IdentitySet`.
- **Testing: TDD.** The zero-residue property test is the SW-02 regression pin and must be able to *detect* residue (a mutated emitter fails it). Emitted code is `zsh -n`-validated. `make check` (fmt-check + vet + lint + test) stays green.

## Acceptance Criteria

- [ ] `model.Manifest` with Scalar / ListDelta / NameSet / OptionSet round-trips to/from the `01-MANIFEST-SHAPE.md` JSON with no field missing for any admitted class.
- [ ] `core/activate` builds a `model.Manifest` from a resolved `model.Profile` (managed declarative entries only; imperative entries excluded).
- [ ] `core/activate` produces an ordered deactivate-then-activate `Plan` value and contains **no** zsh syntax (grep/test-verified).
- [ ] `core/shell/zsh/emit.go` is the only place reverse zsh tokens (`unalias`/`unset -f`/`unsetopt`/PATH-rebuild) are generated; emitted code passes `zsh -n`.
- [ ] Emitted apply/deactivate functions are **plain** (no `emulate -L`/`LOCAL_OPTIONS`).
- [ ] A zero-residue property test over N ≥ 20 random switch sequences leaves the six-class snapshot byte-identical (empty diff), PATH element count stable, and no surviving prior-profile alias/function/option — and a mutated emitter (append PATH) makes it fail.
- [ ] The drift guard holds: a hand-edited managed env var survives deactivate; an untouched one is reversed to its prior (using `${(P)+var}==1`).
- [ ] Ownership-aware restore: deactivating a profile does not remove a base/unmanaged PATH entry the profile merely also added.
- [ ] A shadowed prior alias and function are captured before override and restored byte-for-byte on deactivate; profile-added names are gone.
- [ ] `introspectScript` + `model.IdentitySet` capture alias/function bodies; absent-zsh still degrades to `Available:false`; existing name-only tests still pass.
- [ ] User-controlled values (env/alias/function bodies) are emitted injection-safe (a value containing `'`, `;`, `$(...)`, or a newline cannot execute arbitrary code when the emitted block is `eval`'d).

## Ambiguity Report

| Dimension          | Score | Min  | Status | Notes                                                                 |
|--------------------|-------|------|--------|-----------------------------------------------------------------------|
| Goal Clarity       | 0.90  | 0.75 | ✓      | 4 explicit ROADMAP success criteria; exact deliverables named          |
| Boundary Clarity   | 0.85  | 0.70 | ✓      | In/out explicit; keybindings/hooks default logged as OQ-1 (assumption) |
| Constraint Clarity | 0.88  | 0.65 | ✓      | No new deps; single emit path; 5 Phase-1 carry-forwards; injection threat named |
| Acceptance Criteria| 0.85  | 0.70 | ✓      | Zero-residue property test + ownership + shadow — all falsifiable       |
| **Ambiguity**      | 0.13  | ≤0.20| ✓      | Below gate; interview skipped under --auto (Step 3)                    |

Status: ✓ = met minimum, ⚠ = below minimum (planner treats as assumption)

**No dimension is below minimum.** Three sub-decisions were auto-resolved to safe/reversible defaults and logged in `04-OPEN-QUESTIONS.md` (OQ-1 keybindings/hooks → master block; OQ-2 `original` vs live-prior reconciliation → trust live prior; OQ-3 PATH base-capture placement → once-at-first-activate, Phase-5 seam). None block planning.

## Interview Log

| Round | Perspective    | Question summary                          | Decision locked                                                        |
|-------|----------------|-------------------------------------------|------------------------------------------------------------------------|
| —     | (auto-derived) | Initial ambiguity ≤ 0.20 → interview skipped per Step 3 | SPEC derived from ROADMAP 4 success criteria + SW-01/SW-02 + Phase 1 `01-MANIFEST-SHAPE.md`/`01-FINDINGS.md` |
| auto  | Researcher     | What exists vs the Phase-4 delta?         | Manifest/emit/activate all absent; introspect dumps names not bodies; regen.go is forward-only |
| auto  | Simplifier     | Irreducible core?                         | 8 requirements: manifest type, builder, agnostic diff/plan, single emit path, zero-residue property test, drift/ownership restore, shadow capture, body-dumping introspect |
| auto  | Boundary Keeper| What is NOT this phase?                   | Loader/CLI/bootstrap → Ph5; base-capture placement → Ph5; secret deref runtime → Ph5; keybindings/hooks → master block (OQ-1); real ingest → Ph6 |
| auto  | Failure Analyst| What invalidates zero-residue?            | PATH append vs rebuild; blind option toggle vs was_on; static `original` vs live prior; clobbering a drifted var; injection via unescaped values (T-01-06) — each pinned as a constraint/criterion |

---

*Phase: 04-manifest-builder-emit*
*Spec created: 2026-07-01*
*Next step: /gsd:discuss-phase 4 — implementation decisions (Manifest Go field layout, emit.go function structure, property-test harness shape, injection-safe quoting strategy)*
