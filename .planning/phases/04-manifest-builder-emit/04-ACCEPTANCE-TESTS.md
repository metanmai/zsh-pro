---
type: uat-plan
phase: 04-manifest-builder-emit
status: draft
created: 2026-07-02
source:
  - 04-SPEC.md (Req 1-8, Acceptance Criteria)
  - 04-01-PLAN.md (must_haves + acceptance_criteria)
  - 04-02-PLAN.md (must_haves + acceptance_criteria)
  - ROADMAP.md Phase 4 success_criteria (1-4)
  - 04-EVIDENCE.md (C1-C32 claims ledger)
requirements: [SW-01, SW-02]
execution: NONE — this is a test PLAN, not a run. No code is written or executed here.
---

# Phase 4: Manifest Builder + Emit — UAT TEST PLAN

**Read before running:** this is the plan a verifier follows AFTER Phase 4 is executed. It lists,
in order, every observable behavior a verifier checks and the artifacts/truths that must exist.

## Scope note — what Phase 4 is (and is NOT)

Phase 4 is a **backend / engine phase**. It delivers exactly four things:

1. `model.Manifest` + four part types (Scalar / ListDelta / AliasSet+FuncSet / OptionSet), in `core/model`.
2. `core/activate` — the shell-agnostic manifest **builder**, active-vs-target **differ**, and ordered
   deactivate-then-activate **`Plan`** value type (no zsh tokens).
3. `core/shell/zsh/emit.go` — the **sole** place zsh apply/deactivate code is generated, reached via a
   new `shell.Emitter` seam wired at the composition root.
4. The **zero-residue property test** (SW-02 regression pin) + additive body-dumping introspect.

Phase 4 does **NOT** deliver the user-facing `checkout`/`activate`/`deactivate`/`list`/`status` verbs, the
sourced runtime loader, per-terminal state wiring, or the `.zshrc` bootstrap — **all of that is Phase 5**
(BOOT-01/BOOT-02). Phase 4 emits the code a loader will `eval`; it never installs or invokes a loader.

**Consequence for UAT:** the milestone's user-visible promise — "`checkout <branch>` yields a different,
trustworthy shell environment" — is **not observable in Phase 4**. Every Phase-4 acceptance criterion is
verified by **automated Go tests** (many gated on a `zsh` binary via `exec.LookPath` skip-guards), including
end-to-end apply/deactivate behavior exercised under sandboxed `zsh -f` inside the property test. There is
**no interactive/manual UAT surface** in this phase (see the Human-Verification Required section).

---

## How a verifier runs this plan

Preconditions (all automated; run from the repository root):

- `GOTOOLCHAIN=auto go build ./...` succeeds.
- `GOTOOLCHAIN=auto go test ./...` passes (`-count=1` to defeat cache for the property test).
- `make check` (fmt-check + vet + lint + test) is green.
- `zsh` is on `$PATH` (5.9+ per evidence) — the emit/introspect/residue tests skip cleanly if absent, but a
  real verification MUST run with `zsh` present so the `zsh -n` and zero-residue checks actually execute.
- `go.mod` require block is unchanged (still only `mvdan.cc/sh/v3`) — **no new dependency**.

---

## Ordered UAT test list

Each item = **name** + the **precise observable a verifier checks**. Ordered to build from the agnostic
foundation (Wave 1: manifest/builder/plan/introspect) up to the zsh emit path and the zero-residue pin
(Wave 2). Derived from SPEC Req 1-8, the two plans' acceptance_criteria, ROADMAP success_criteria 1-4, and
the PROVEN claims in 04-EVIDENCE.md.

### 1. Cold-start build & full test smoke
**Expected:** From a clean state, `GOTOOLCHAIN=auto go build ./...` and `GOTOOLCHAIN=auto go test -count=1 ./...`
both succeed. No new dependency appears in `go.mod` (still only `mvdan.cc/sh/v3`). `make check` is green;
`gofmt -l` on the new/changed files (`core/model/manifest.go`, `core/activate/*.go`, `core/shell/zsh/emit.go`,
`core/shell/zsh/introspect.go`) returns nothing. (ROADMAP overall; SPEC Constraint "make check stays green".)

### 2. Manifest round-trips to/from the fixed `01-MANIFEST-SHAPE.md` JSON, byte-identical, all six classes
**Expected:** A `model.Manifest` constructed with **all four part types populated** — `Scalar` env entries,
`ListDelta` (PATH/FPATH additions), `AliasSet` (`added` map + `shadowed` map), `FuncSet` (`added` array +
`shadowed` map), and `OptionSet` (`name`/`enabled`/`was_on`) — marshals to and unmarshals from the exact
Phase-1 `01-MANIFEST-SHAPE.md` JSON **losslessly, with no field missing for any admitted class**. Deterministic
marshal (sorted map keys, trailing newline) yields byte-identical output across runs (C17). (SPEC Req 1;
04-01 acceptance; ROADMAP-4 "PATH stored as a delta".)

### 3. Tri-state `Original` distinguishes unset / empty / value
**Expected:** `Scalar.Original` is `*string` with `omitempty`. A round-trip test proves the three states are
never conflated: `nil` → **was-unset** (field absent in JSON), `&""` → **was-empty** (`"original":""`),
`&"vim"` → **was-value** (`"original":"vim"`). A plain-`string` shape would fail this (would clobber a
was-unset var to empty on restore). (SPEC Req 1; 04-01 Task 1; threat T-04-02.)

### 4. Manifest type shape is exactly Fork A (two-type alias/func split) and stdlib-only
**Expected:** `AliasSet.Added` is `map[string]string`; `FuncSet.Added` is `[]string` (a uniform map cannot
unmarshal the fixture's `functions.added` array — C16). `const SchemaV1 = "v1"` is present (the exact fixture
literal). `OptionSet.WasOn` is doc-commented as runtime-captured / builder-forbidden. `ListDelta.Deletions` is
doc-commented validated-present-but-unexercised. `core/model/manifest.go` imports **no** `zsh-pro/core/*`
package (agnostic, stdlib-only). (SPEC Req 1; 04-01 acceptance; CH-7.)

### 5. Builder classifies each admitted class into the right part; imperative entries produce no part
**Expected:** `activate.Build(profile)` over a `model.Profile` containing **one entry of each admitted class**
(CatEnvironment scalar; CatSecrets scalar; CatPath assignment; KindAlias; KindFuncDecl; `setopt`) yields a
`Manifest` whose `Env` / `Lists` / `Aliases.Added` / `Functions.Added` / `Options` contain **exactly those
entries** — no more, no fewer. An imperative/unmanaged entry (`OverrideUnmanaged`, a non-`setopt` `KindCommand`,
or a Kind the gate rejects) produces **no manifest part**. The builder consumes only `EffectiveManaged()` entries.
(SPEC Req 2; ROADMAP-1; 04-01 acceptance; threat T-04-04.)

### 6. Builder fills declarative intent only — Shadowed / Original / WasOn / Deletions left empty
**Expected:** The built manifest carries only `added`/`applied`/`enabled`/`additions`. `Shadowed`, `Original`,
`WasOn`, and `Deletions` are left nil/empty — these are runtime-reconciled (D-06). No `OptionSet` carries a
builder-authored non-zero `WasOn`. (SPEC Req 2; 04-01 acceptance; CH-7.)

### 7. Narrowed, precision-over-recall PATH split
**Expected (per EVIDENCE C26, STATIC-VALIDATED):** the builder recognizes a base self-reference
(`$PATH`/`$path`/`${PATH}`/`$FPATH`/`$fpath`/`${FPATH}`) at head **or** tail with colon-joined additions on the
other side → `ListDelta{Additions:[non-base segments]}`. Fixtures:
(a) `export PATH=$HOME/bin:$PATH` → `Additions:["$HOME/bin"]`;
(b) `export PATH=$PATH:$HOME/bin` (tail) → `Additions:["$HOME/bin"]`;
(c) `${PATH}` brace form recognized as the base marker;
(d) mid-list base `$HOME/bin:$PATH:$HOME/go/bin` → **NO part** (ambiguous → imperative);
(e) no-base-marker `$HOME/bin:/usr/bin` → **NO part**.
(SPEC Req 2; 04-01 acceptance; OQ-16.)

### 8. Ordered deactivate-then-activate `Plan`; distinct reverse vs apply list ops
**Expected:** `Diff(A, B)` returns a `Plan` whose **entire `Deactivate` slice (all of A's reverse ops) precedes
the entire `Activate` slice (all of B's apply ops)**. A PATH `ListDelta` produces a `RebuildListFromBase` op on
the deactivate side (rebuild **to** base, carrying **no** additions to re-add) and a **distinct** `ApplyListDelta`
op on the activate side (re-adds additions) — asserted as different op types with the same list name.
`Diff(A, nil)` = pure deactivate; `Diff(nil, B)` = pure activate. Shared names are **not** optimized away (naive
full-deactivate-then-full-activate). (SPEC Req 3; ROADMAP-2; 04-01 acceptance; CH-1c.)

### 9. Shadow-restore ops derived from `Added`, not the builder-empty `Shadowed` map
**Expected:** A manifest with populated `Aliases.Added` (e.g. `{"ll":"ls -la"}`) and `Functions.Added`
(e.g. `["ff"]`) but an **empty** `Shadowed` map yields, on the deactivate side, **one `Unalias`+`RestoreShadowedAlias`
pair per added alias** and **one `UnsetFunc`+`RestoreShadowedFunc` pair per added function** — N added → N restore
ops, regardless of `Shadowed` being empty. Proves the diff does not consult the always-empty `Shadowed` map (which
would silently yield zero restore ops → SPEC Req 7 failure). (04-01 acceptance; CH-13; threat T-04-15.)

### 10. SchemaV1 forward-compat gate is a real check
**Expected:** `Diff` with an `active` **or** `target` manifest whose `Schema != model.SchemaV1` returns a non-nil
error and generates **zero ops** — asserted in **both** directions (bad active, bad target). A matching-SchemaV1
pair succeeds. This is a real conditional in `diff.go`, not a decorative const. (04-01 acceptance; CH-8; threat T-04-01.)

### 11. `core/activate` contains no zsh token — PRECISE check
**Expected:** A **precise** grep/test (case-sensitive, word-boundary, string-literal only) finds **no** reverse-op
zsh syntax (`\bunalias\b`, `\bunset -f`, `\bunsetopt\b`, PATH-array-rebuild pattern) as a string literal in
non-test `core/activate` source — explicitly **excluding** Go type identifiers (`Unalias`, `UnsetFunc`,
`SetOption`, `RebuildListFromBase`) and comments. `core/activate` imports `zsh-pro/core/model` **only** (never
`core/shell/zsh`), verified by the import graph. (SPEC Req 3; ROADMAP-1; 04-01 acceptance; C22; threat T-04-03.)

### 12. Builder rejects hostile OPTION names (drop-to-no-part)
**Expected:** An option name conforming to `^[A-Za-z_][A-Za-z0-9_]*$` (`extendedglob`, `no_case_glob`) yields an
`OptionSet`; a hostile/non-conforming name (`foo; touch $CANARY`, `x$(touch $CANARY)y`, `foo -o bar`, `ext glob`)
produces **no `OptionSet` part** (Options slice empty for hostile fixtures, populated for conforming). (04-01
acceptance; CH-17; threat T-04-17; C29 PROVEN.)

### 13. Builder rejects hostile ALIAS / FUNCTION names on RAW extracted bytes (drop-to-no-part)
**Expected:** A name conforming to `^[A-Za-z0-9_][A-Za-z0-9_.-]*$` (`gs`, `ll`, `git-foo`, `_helper`, `foo.bar`)
is authored into `Aliases.Added`/`Functions.Added`; a hostile name carrying a shell metacharacter — including the
**backslash forms parse.go retains** (`gs\;x`) plus `gs;touch $CANARY`, `x$(...)y`, backtick-command, `a|b`,
`a b`, `a\nb` — produces **no part**. Validation runs on the **raw extracted bytes** (parse.go keeps the
backslash). (04-01 acceptance; CH-19; threat T-04-18; C31 PROVEN.)

### 14. Builder drops unsafe STATIC PATH addition segments; keeps dynamic self-reference verbatim
**Expected:** `export PATH=$HOME/bin:$PATH` and `export PATH=$PATH:/opt/tool*` (a legitimate static glob element)
each yield a `ListDelta`; but `export PATH=/opt/x$(touch $CANARY):$PATH` and a static segment containing
`;`/backtick/newline/space produce **no part** (whole entry dropped, precision-over-recall). A **dynamic**
self-reference segment (`$HOME/bin`) stays **verbatim** in `Additions` (not hard-quoted — portability); a static
glob (`/opt/tool*`) stays as a plain Addition for emit to zquote. (04-01 acceptance; CH-24; threat T-04-19; C32 PROVEN.)

### 15. Introspect dumps alias/function BODIES additively (name maps intact)
**Expected:** `Introspect` on a fixture defining `alias gs='git status'` and `foo() { echo hi }` returns
`AliasBodies["gs"] == "git status"` (RHS only) and `FunctionBodies["foo"]` = the body **without** the `name(){`
wrapper. A single `Introspect` call on one fixture co-populates the alias-NAME map AND function-NAME map AND
`AliasBodies` AND `FunctionBodies`. The existing name-only tests still pass unchanged (additive/non-breaking).
(SPEC Req 8; ROADMAP-4; 04-01 acceptance; C13/C23.)

### 16. Body-dump survives multi-line + `##`-prefixed body lines (NUL-framed, sentinel-bounded)
**Expected:** `Introspect` on a function whose body contains a newline + tab + single-quote + `$` + `;` returns
`FunctionBodies[name]` **byte-identical** to the original. A function body with a line that starts `##` (e.g.
`## not a header`) round-trips byte-identically and does **not** truncate/corrupt the section or the following
`##END##` marker — proving the parser bounds sections by NUL-preceded record framing, **not** a `\n##` scan.
Body-dump loops use the `o` sort flag (`${(@ok)aliases}` / `${(@ok)functions}`) for byte-stable ordering.
(SPEC Req 8; 04-01 acceptance; OQ-17; C23; threat T-04-05.)

### 17. Introspect degrades gracefully when zsh is absent
**Expected:** The zsh-absent / error / timeout path still returns `IdentitySet{Available:false}` with nil body
maps, and the 5s `context.WithTimeout` subprocess shape is unchanged. (SPEC Req 8; 04-01 acceptance; C23.)

### 18. `emit.go` is the SOLE place reverse zsh tokens are generated (single-emit-path invariant)
**Expected:** A tree-grep test (`invariant_test.go`) confirms reverse-op zsh tokens (`\bunalias\b`, `\bunset -f\b`,
`\bunsetopt\b`, PATH-array rebuild) are generated **only** under `core/shell/zsh/emit.go` (+ its `_test.go`) —
nowhere else in the tree. `regen.go` stays forward-only. (SPEC Req 4; ROADMAP-1; 04-02 acceptance; threat T-04-03.)

### 19. Emitted apply AND deactivate pass `zsh -n`
**Expected:** For representative Plans, `Emit` produces apply and deactivate strings that each pass `zsh -n`
(syntax-valid, exit 0). Malformed emission would fail nonzero. The check runs on the **raw** block (an
eval-wrapped malformed inner block would pass the outer `-n`). (SPEC Req 4; 04-02 acceptance; C18.)

### 20. Emitted loader functions are PLAIN (no `emulate -L` / `LOCAL_OPTIONS`)
**Expected:** Emitted apply/deactivate functions contain **no** `emulate -L` and **no** `LOCAL_OPTIONS` — so
`setopt`/`unsetopt` persist past function return (they auto-revert under local-option scope). (SPEC Req 4/Constraint;
ROADMAP-1; 04-02 acceptance; C8; Phase-1 carry-forward 3.)

### 21. Static values emitted injection-safe (zquote); dynamic values stay verbatim — the split keys on `Dynamic`
**Expected:** A **static** user-controlled value containing `'`, `;`, `$(...)`, backtick, or a newline is emitted
single-quote-wrapped (`'\''` escape) so it is an **inert literal** under `eval` (no canary fires under `zsh -f`,
value appears as exact literal bytes). A **dynamic** value (`$HOME/go`) stays **verbatim** so zsh expands it
per-machine (late-bound). The split keys strictly on the op's `Dynamic` bool. Function bodies are restored
**verbatim** via `functions[name]=$captured` (never zquote'd — a body is live code, C6). (SPEC Req 4/AC; ROADMAP-1/3;
04-02 acceptance; threat T-01-06; C5/C6/C7 PROVEN.)

### 22. Adversarial names/values/segments fire NO canary through the emitted double-eval — the uniform injection boundary
**Expected:** Every injectable class, run through the emitted code under a **loader double-eval** (`eval` of the
emitted string), fires no canary while legitimate inputs round-trip:
- **VALUES** (scalar/alias) — static metacharacter/command-sub payloads inert; dynamic self-reference verbatim (C5/C6/C7).
- **NAMES** — emit.go **re-validates** every alias/func name against `^[A-Za-z0-9_][A-Za-z0-9_.-]*$` (CH-19/C31) and
  every option name against `^[A-Za-z_][A-Za-z0-9_]*$` (CH-17/C29) before emitting it unquoted into
  `unalias`/`alias=`/`functions[]=`/`unset -f`/`[[ -o ]]`/`setopt`/`unsetopt`/RestoreShadowed* positions; hostile names
  are rejected (no part), realistic names (`gs`/`ll`/`git-foo`/`_helper`/`foo.bar`, `extendedglob`) round-trip. This is
  **defense in depth** over the 04-01 builder drop. Slot names are sanitized/assoc-keyed so the name→slot path cannot
  inject (C24).
- **PATH ADDITION SEGMENTS** — `renderList` runs **each** segment through the same static-zquote/dynamic-verbatim split
  (never raw splicing): a static metacharacter/command-sub segment is zquote'd inert, a `$HOME/bin`-style dynamic
  segment stays verbatim and expands late-bound (C32/CH-24).
(SPEC Req 4/AC "injection-safe"; ROADMAP-1; 04-02 acceptance; threats T-01-06/T-04-17/T-04-18/T-04-19.)

### 23. Drift-guarded env restore (`${(P)+var}==1`): hand-edited var survives, untouched var reversed
**Expected:** Under `zsh -f`, deactivate restores an env scalar **only if the live value still equals the applied
value** (drift guard). Two distinct assertions: (i) a managed env var **hand-edited after apply survives** deactivate
(left intact, live != applied); (ii) an **untouched** managed var **is reversed** to its prior. Unset-vs-empty is
distinguished via `${(P)+var}==1` (the operand is a var holding the name — C2/C21 — not `-n`, which mis-classifies
unset as empty). (SPEC Req 6; ROADMAP-3; 04-02 acceptance; C21 PROVEN.)

### 24. Ownership-aware PATH restore: base/shared entry survives, profile-added entry dropped
**Expected:** Deactivate reversal of a list is a **full rebuild-to-base** (`RebuildListFromBase`) with **no
per-element removal loop** on the deactivate hot path (single-active v2.0). Consequences verified under `zsh -f`:
(a) a **base-owned** entry (`/usr/local/bin` in `ZP_BASE_PATH`) that a profile **also** added is **not stripped**
by deactivating that profile — after apply→deactivate, `$#path` is byte-identical to baseline AND `/usr/local/bin`
appears **exactly once** (guards both strip and duplicate, not merely "still present");
(b) a **profile-added** element **absent from base** and containing a glob metacharacter (`/opt/tool*`) is **dropped**
by rebuild-to-base on deactivate (no per-element glob comparison — the C10 hazard is off this path). The reserved
quoted-RHS literal-equality form `[[ $e == "$target" ]]` (C25) appears only in a comment as the future
element-deletion form; `${path:#pattern}` and unquoted `== $target` are **never** emitted. (SPEC Req 6;
ROADMAP-2/3; 04-02 acceptance; CH-11/CH-12/CH-15; C25 PROVEN.)

### 25. Shadowed alias/function captured before override, restored byte-identically; added names gone
**Expected:** Emitted apply **captures the live prior body** of any alias/function it overrides (into a
`${+name}`-guarded, idempotent runtime slot — a double-checkout does not clobber the clean prior). After activating a
profile that shadows a pre-existing `ll` alias and `ff` function, then deactivating, `ll` and `ff` are restored to
their **exact prior bodies (byte-identical)** — alias via `alias name=$prior`, function via `functions[name]=$prior`
verbatim. Names the profile **added** (not shadowed) are **gone** after deactivate. Restore guards use `${+name}`
(not `-n`, not `${(P)+literalName}` — C1/C3/C4). (SPEC Req 7; ROADMAP-4; 04-02 acceptance; C3/C4 PROVEN; threat T-04-08.)

### 26. Option restore round-trips via runtime-captured `was_on` (was-on / was-off / was-toggled)
**Expected:** Emitted `SetOption` apply captures the **live** option state via `[[ -o optname ]]` into a per-switch
`${+slot}`-guarded `was_on` slot **before** `setopt`/`unsetopt`. `RestoreOption` reads that captured slot (no
authoritative Go-side `WasOn`) and restores exactly. Three cases round-trip: was-ON→profile-OFF→restored-ON;
was-OFF→profile-ON→restored-OFF; was-ON→profile-ON→restored-ON. A double-apply does not clobber the captured
was_on. (SPEC Req 6/7; ROADMAP-3; 04-02 acceptance; CH-7/CH-9; C27 PROVEN.)

### 27. Deactivate cleans its own per-switch undo slots but never unsets `ZP_BASE_PATH`
**Expected:** The end of the emitted deactivate `unset`s every per-switch slot it created (`__ZP_ORIG_*`,
`ZP_<profile>_PRIOR_*`, sentinels, the captured `was_on` slot) so no slot survives as residue — but it does
**NOT** unset `ZP_BASE_PATH` (session-persistent). A post-sequence assertion confirms `ZP_BASE_PATH` equals its
baseline value after a full switch run. (04-02 acceptance; CH-3/CH-23; OQ-3/OQ-14.)

### 28. Zero-residue property test — N≥20 balanced sequences, byte-identical six-class snapshot
**Expected (the SW-02 regression pin):** a default-suite, `LookPath`-guarded property test drives **N≥20 random
BALANCED switch sequences** (every apply matched by its deactivate before a different profile applies — at most
one profile active at any point; a pure-Go generator meta-check asserts this invariant). All sequences run in
**one sourced `zsh -f` process** (cross-process would re-inherit stray env). After each sequence the pre-vs-post
snapshot is **byte-identical (empty diff)**, where the compared snapshot is the CONCATENATION of:
- the **type-class-filtered** full-env `${(@kv)parameters}` section (exported AND non-exported, name+VALUE, NUL-framed;
  skips any param whose type matches `(*special*|*tied*|*hide*|undefined)`, keeping concrete user-scope types);
- the **sorted function-BODY** section (`${(@ok)functions}`);
- a dedicated **PATH** section carrying BOTH `$#path` AND the **ordered `$path` array CONTENTS** (a count-preserving
  reorder/swap must be caught, not only a count change — CH-25);
- the two dedicated **sorted, filter-independent** sections: **option-state** (`${(@ok)options}` → `optname=<on|off>`)
  and **alias-body** (`${(@ok)aliases}` → `name<NUL>body`) — because `aliases`/`options` are `*special*`-typed and
  excluded by the type-class filter (CH-16/CH-20).
`$#path` is stable across the sequence; no alias/function/option from any prior profile survives.
(SPEC Req 5; ROADMAP-2; 04-02 acceptance; C19/C28/C30/C32 corrections applied.)

### 29. The property test genuinely DETECTS residue — three independent mutants each FAIL
**Expected:** Each of three emitter mutants, substituted via the injectable package-level `renderList`/`renderValue`
vars, is asserted in its **own** sub-test to make the pin FAIL:
1. **blind-append** (`renderList` → `path=(<add> $path)` with no rebuild-from-base) → non-empty diff / `$#path` grows;
2. **dropped-zquote** (`renderValue` emits a static metacharacter value verbatim) → the canary FIRES / value not literal;
3. **base-strip** (`renderList` omits/over-removes base entries) → the base-ownership sub-check FAILS (non-baseline
   `$#path` or missing `/usr/local/bin`).
(SPEC Req 5 AC "a mutated emitter makes the test fail"; ROADMAP-2; 04-02 acceptance; CH-5/CH-15.)

### 30. The snapshot INSTRUMENT itself is self-stable and catches what the old names-only instrument missed
**Expected:** Meta-tests on the instrument (same type-class filter throughout): (a) two consecutive no-op snapshots
diff to **empty** (self-stable — the type-class filter + fd/temp-file capture, not `$(...)`, neutralize volatility);
(b) an injected **non-exported var leak** (`typeset localonly=x`) fires the diff; (c) an injected **exported
value-only change** (`export EXISTING=changed`) fires the diff; (d) an injected **option-drift** makes the
option-state section diff non-empty; (e) an injected **alias-body-change** makes the alias-body section diff
non-empty; (f) a **known user scalar** (`zmodload zsh/parameter` preamble) appears in the baseline full-env
snapshot (guards against a silently-empty `parameters` map → all-classes false-green). (04-02 acceptance;
CH-2/CH-10/CH-16; C28/C30 PROVEN.)

### 31. `shell.Emitter` seam exists and is wired at the sole composition root
**Expected:** `shell.Emitter` interface exists in `core/shell/provider.go` (doc-commented as the sole reverse-syntax
codegen seam, mirroring `Regenerator`), taking a `core/activate.Plan`. `zsh.Provider{}` satisfies it and is wired
in `core/cmd/zsh-pro/main.go` (constructed + injectable now, un-driven until Phase 5). No import cycle forms.
(SPEC Req 4; 04-02 acceptance/key_links; D-10.)

---

## Must-haves a verifier checks (from plan frontmatter)

### Truths (must be TRUE after execution)

From **04-01** (Wave 1):
- A `model.Manifest` with all four parts round-trips to/from `01-MANIFEST-SHAPE.md` JSON losslessly. *(→ test 2)*
- `core/activate.Build` builds a manifest from `EffectiveManaged()` entries only; imperative/forced-unmanaged →
  no part; ambiguous CatPath base → no part; unsafe STATIC addition segment → no part (drop-to-no-part); dynamic
  self-reference segment stays verbatim. *(→ tests 5, 7, 14)*
- `Diff` produces an ordered deactivate-then-activate `Plan`; `RebuildListFromBase` (deactivate) is semantically
  DISTINCT from `ApplyListDelta` (activate); the package contains no zsh reverse-op string literal. *(→ tests 8, 11)*
- `Diff` refuses a manifest whose `Schema != SchemaV1` before generating any op. *(→ test 10)*
- `RestoreOption` carries no builder-authored `WasOn` (builder forbidden — D-06). *(→ tests 6, 26)*
- Builder validates option names (`^[A-Za-z_][A-Za-z0-9_]*$`) and alias/func names
  (`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`, on RAW bytes) — hostile → no part. *(→ tests 12, 13)*
- Introspect captures alias/function BODIES via NUL-framed, sentinel-bounded dump; multi-line body round-trips;
  name-only tests still pass; zsh-absent → `Available:false`. *(→ tests 15, 16, 17)*

From **04-02** (Wave 2):
- `emit.go` renders a `Plan` to PLAIN apply+deactivate zsh (no `emulate -L`/`LOCAL_OPTIONS`) that passes `zsh -n`;
  it is the SOLE reverse-token generator; rendering factored into injectable package-level `renderList`/`renderValue`.
  *(→ tests 18, 19, 20, 29)*
- Static value → single-quote-wrapped inert literal; dynamic value → verbatim; split keys on `Dynamic`. *(→ test 21)*
- `renderList` applies the per-segment static-zquote/dynamic-verbatim discipline to EACH PATH/FPATH addition segment.
  *(→ tests 14, 22)*
- `AddAlias`/`AddFunc` apply captures the shadowed prior body (`${+name}`-guarded, idempotent) before overwrite.
  *(→ test 25)*
- `SetOption` apply captures live option state (`[[ -o optname ]]`) into a `${+slot}`-guarded `was_on` slot before
  toggling. *(→ test 26)*
- `SetOption` / alias-func-name ops re-validate the emitted NAME against the identifier/safe grammar before emitting
  it unquoted (defense in depth). *(→ tests 12, 13, 22)*
- Deactivate is ownership-aware + drift-guarded (`${(P)+var}==1`); PATH reversal = full rebuild-to-base; function
  bodies restored verbatim; deactivate unsets its own slots but keeps `ZP_BASE_PATH`. *(→ tests 23, 24, 25, 27)*
- The zero-residue property test (N≥20 balanced, self-stable full-env + dedicated sections snapshot, three failing
  mutants, self-stability + leak meta-asserts, PATH-segment injection sub-test, ZP_BASE_PATH-persistence) is the
  SW-02 pin. *(→ tests 28, 29, 30)*

### Artifacts (must EXIST)
- `core/model/manifest.go` — `Manifest` + `Scalar`/`ListDelta`/`AliasSet`/`FuncSet`/`OptionSet`; `SchemaV1` const.
- `core/model/identityset.go` — additive `AliasBodies`/`FunctionBodies map[string]string`.
- `core/activate/builder.go` (`func Build`), `plan.go` (`type Plan`), `diff.go` (`func Diff`).
- `core/activate/{builder_test,diff_test,schema_test,tokenfree_test}.go`.
- `core/shell/zsh/introspect.go` — additive NUL-framed body-dump sections (`print -rN`) + non-line body parser.
- `core/shell/zsh/emit.go` — `Emit`; package-level `var renderList` / `var renderValue`; `zquote`.
- `core/shell/provider.go` — `Emitter interface`.
- `core/cmd/zsh-pro/main.go` — `zsh.Provider{}` wired as `Emitter`.
- `core/shell/zsh/{emit_test,residue_test,invariant_test}.go`.

### Key links (must HOLD)
- `core/activate.builder.go` reuses `core/ir/route.go`'s Kind+Category classification shape (`EffectiveManaged`) —
  does not re-derive the admitted set.
- `core/activate` imports `zsh-pro/core/model` **only** — never `core/shell/zsh`.
- `core/shell/zsh/emit.go` consumes `activate.Plan`.
- `core/cmd/zsh-pro/main.go` injects `zsh.Provider{}` as `shell.Emitter` at the sole composition root.
- `residue_test.go` uses a NEW full-env `${(@kv)parameters}` snapshot (not the exported-only `##ENV##` section),
  reused inline in one `zsh -f` process.

---

## Human-Verification Required

**Status: effectively N/A for Phase 4 — deferred to Phase 5 runtime.**

Phase 4 is a pure infrastructure/engine phase. Every acceptance criterion above is verified by **automated Go
tests** — and where behavior is genuinely shell-level (apply/deactivate, drift guard, shadow restore, option
round-trip, zero residue, injection inertness), it is exercised **end-to-end under sandboxed `zsh -f` inside the
automated property/emit tests**, not by a human at a live prompt. There is:

- **No CLI verb** to run by hand — `checkout`/`activate`/`deactivate`/`list`/`status` do not exist yet (Phase 5).
- **No runtime loader** to source — Phase 4 emits the code a Phase-5 loader will `eval`; it does not install,
  source, or invoke one.
- **No `.zshrc` bootstrap** and no per-terminal state wiring (Phase 5).

**Why there is no manual item:** the milestone's user-observable promise — *"`checkout <branch>` yields a
different, trustworthy shell environment; declarative state applies and reverses with zero residue while dynamic
values stay late-bound"* — becomes observable only once the Phase-5 runtime loader wires the emitted code into a
live, already-open terminal. In Phase 4, the emitted apply/deactivate strings are exercised in a **child** `zsh -f`
by the property test; a human sitting at their own terminal cannot yet see a profile switch because nothing sources
the emitted code into the **parent** shell.

**Deferred to Phase 5 human/manual UAT (NOT in scope here):**
- In an already-open terminal, `checkout A → checkout B → deactivate` visibly changes the **current** shell and
  leaves it byte-identical to the pre-activation state (the live analog of automated test 28).
- The drift guard, shadow restore, and PATH ownership behave correctly against the user's **real** aliases/env/PATH
  in a live session (live analogs of tests 23-25).
- `list`/`status` report branches and the active profile; the loader is fail-open and fast.

Recording these here so the Phase-5 UAT picks them up; they are **not** Phase-4 verification gaps — the Phase-4
deliverables (the manifest, the agnostic plan, the sole emit path, the SW-02 property pin) are fully and correctly
verified by the automated suite above.

---

## Pass/fail summary template (to be filled by the verifier run)

```
total:   31
passed:  __
failed:  __
skipped: __   (zsh-gated tests when zsh absent — a real verification must run WITH zsh present)
```

A phase PASS requires: all 31 automated items pass with `zsh` present, `make check` green, no new dependency,
and the three property-test mutants each demonstrably FAIL the pin (test 29).
