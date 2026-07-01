---
phase: 04-manifest-builder-emit
type: test-strategy
created: 2026-07-02
requirements: [SW-01, SW-02]
covers_spec_reqs: [1, 2, 3, 4, 5, 6, 7, 8]
---

# Phase 4 — Testing Philosophy: Manifest Builder + Emit

This is the testing *philosophy* for Phase 4 — the reasoning about WHAT kind of test proves
each component and WHY, not the test code. The test code is written during execution (TDD,
per CLAUDE.md). The single non-negotiable output is a zero-residue **property test** that is
the SW-02 regression pin and can *actually detect residue* — a mutated emitter must fail it.

The whole phase divides cleanly into two test worlds separated by one axis: **does the
component's correctness depend on a running zsh?**

- **Pure Go (always runs).** The manifest types + JSON round-trip, and the entire
  `core/activate` builder/diff/`Plan`. These are deterministic value transformations; they
  are unit-tested TDD with table tests and `go-cmp`/`reflect.DeepEqual`, and they run on every
  `go test ./...` with no environment dependency.
- **zsh-requiring (skips cleanly when absent).** The emitted-code parse-validity, the extended
  introspect body-dump, and the zero-residue property test. These follow the existing
  `exec.LookPath("zsh")` skip-guard precedent (`core/shell/zsh/introspect_test.go`,
  `core/ir/roundtrip_test.go`, `core/store/roundtrip_test.go`) so they degrade to a skip — never
  a failure — where zsh is unavailable (CI without zsh, foreign platforms).

There is a hinge between the two worlds: `emit.go` codegen. Its *string shape* is asserted in
pure Go (no zsh), but its *behavioral correctness* — parse-validity, injection-inertness,
zero-residue — is only provable by feeding the emitted text to a real zsh. So `emit.go` gets
BOTH a pure-Go shape test and a zsh-requiring behavior test. See §"emit.go".

---

## Per-component decision: TDD / E2E / skip, and WHY

### 1. `model.Manifest` types + JSON round-trip (Plan 04-01 Task 1) — **PURE-GO TDD**

**Why TDD, why pure Go.** The manifest is a shell-agnostic value type in `core/model` (D-01),
stdlib-only, tagged directly with `json:`. Its correctness is entirely "does this Go struct
serialize to and deserialize from the validated `01-MANIFEST-SHAPE.md` JSON without losing a
field or conflating a state?" That is a deterministic function of the type definition — no zsh,
no subprocess, no I/O. It is the textbook case for table-driven TDD with `go-cmp`
(`reflect.DeepEqual` on the round-tripped value).

**What the tests must pin (invariants, not incidental bytes):**
- **Lossless round-trip.** Construct a `Manifest` with *all four part types populated*
  (env `Scalar`, PATH+FPATH `ListDelta`, `AliasSet`/`FuncSet` with added+shadowed, `OptionSet`),
  marshal → unmarshal, assert `DeepEqual`. The assertion is on the round-trip *identity*, not on
  a hand-copied expected JSON blob (that would be a circular/brittle test — see C17: `encoding/json`
  sorts map keys deterministically, so a byte-level oracle is *available* for the fixture-key
  check but the round-trip proof itself must assert value-identity, not a magic string).
- **Fixture-key match.** Marshal a `Manifest` literal mirroring `01-MANIFEST-SHAPE.md` and assert
  the emitted keys are exactly `profile/schema/env/lists/aliases/functions/options` and the part
  keys `name/applied/original/additions/deletions/added/shadowed/enabled/was_on` (D-04). This is
  the one place a *literal* JSON oracle is legitimate — the SPEC Req 1 acceptance is "round-trips
  to/from the validated shape," and the validated shape is the external contract.
- **Tri-state `Scalar.Original` (the single load-bearing type choice, D-02).** Three explicit
  cases: `nil` marshals with NO `original` key (was-unset); `&""` marshals `"original":""`
  (was-empty); `&"vim"` marshals `"original":"vim"` (was-value); all three unmarshal back losslessly.
  This is the assertion that a plain `string` would FAIL — it proves unset≠empty≠value are not
  conflated (threat T-04-02). Assert the *distinction is preserved*, not that a specific pointer
  value round-trips.
- **Fork A two-type split (OQ-11 supersedes D-02's tentative uniform-map).** `functions.added`
  unmarshals from a JSON *array* `["work_deploy"]` into `FuncSet.Added []string`; `aliases.added`
  unmarshals from a JSON *object* `{"gs":"git status"}` into `AliasSet.Added map[string]string`.
  C16 (PROVEN) is the provenance: a uniform `map[string]string` provably *cannot* unmarshal the
  array — so the two-type split is required, and the test that asserts the array parses is the
  guard that keeps someone from "simplifying" back to a uniform map.

**Not tested here.** The `SchemaV1` const is defined here but its *gate* is exercised in the
`Diff` schema test (§3) — a const alone proves nothing; the check does (CH-8).

### 2. `core/activate` builder (Plan 04-01 Task 2) — **PURE-GO TDD**

**Why.** The builder is a pure `Profile → Manifest` classification. It must never touch zsh
(D-05 — it imports `core/model` only). Every acceptance is a structural claim about which
entries become which parts, testable in-memory.

**What the tests must pin:**
- **One entry of each admitted class → exactly its part** (SPEC Req 2 acceptance). A `Profile`
  with a `CatEnvironment` scalar, a `CatSecrets` scalar, a `CatPath` assignment, a `KindAlias`, a
  `KindFuncDecl`, and a `setopt` yields a `Manifest` whose parts contain exactly those — and an
  imperative entry (`OverrideUnmanaged`, a non-`setopt` `KindCommand`, a rejected `Kind`) yields
  **no part**. This is the precision-over-recall backstop (threat T-04-04): assert the imperative
  entry contributes *nothing*, not merely "the managed ones are present."
- **Declarative-intent-only fill (D-06).** Assert `Shadowed`/`Original`/`WasOn`/`Deletions` are
  left nil/empty — the builder is FORBIDDEN to author `was_on` (CH-7); those are runtime-captured
  live facts. This is a negative assertion (no builder-authored `WasOn`) and it is load-bearing:
  if the builder authored a stale `was_on`, `RestoreOption` would restore the wrong state.
- **Narrowed PATH split (OQ-16 / C26 STATIC-VALIDATED).** Table fixtures (a)-(e) from the plan:
  head base `$HOME/bin:$PATH` → `Additions:["$HOME/bin"]`; tail base `$PATH:$HOME/bin` → same;
  `${PATH}` brace recognized as base; mid-list base and no-base-marker → **NO part** (ambiguous,
  routed imperative). Assert the invariant "unambiguous head/tail split OR nothing," never a magic
  segment count.
- **Injection-safety at the builder tier (first-class — see §"Injection-safety").** Option-name
  validation `^[A-Za-z_][A-Za-z0-9_]*$` (CH-17/C29); alias/func-name validation
  `^[A-Za-z0-9_][A-Za-z0-9_.-]*$` on the RAW extracted bytes (CH-19/C31); static PATH
  addition-segment drop-to-no-part (CH-24/C32). Hostile inputs → empty part; realistic inputs →
  populated. These are pure-Go tests (the validators are Go regexes) but they are *security* tests,
  not merely classification tests.

### 3. `core/activate` diff / `Plan` (Plan 04-01 Task 2) — **PURE-GO TDD + a structural grep test**

**Why.** The differ is a pure `(active, target) → Plan` transform over agnostic op values. The
`Plan` carries NO zsh text (D-08), so its correctness is entirely in-memory ordering + op-shape.

**What the tests must pin:**
- **Deactivate-then-activate ordering (SPEC Req 3 acceptance).** `Diff(A, B)`: all of A's reverse
  ops precede all of B's apply ops. `Diff(A, nil)` = pure deactivate; `Diff(nil, B)` = pure activate
  (D-09 empty cases first-class — the property test drives arbitrary sequences through these).
- **CH-1c distinct list-op semantics.** A PATH `ListDelta` yields a `RebuildListFromBase` on
  deactivate (rebuild TO base, carries NO additions to re-add) and a distinct `ApplyListDelta` on
  activate (re-adds). Assert they are *different op types* — proving deactivate does not resurrect
  the profile's additions.
- **CH-8 SchemaV1 gate is a REAL check.** `Diff` with either manifest at `Schema != SchemaV1`
  returns a non-nil error and generates ZERO ops (assert both bad-active and bad-target). This is
  the difference between a decorative const and a forward-compat gate (threat T-04-01). Test the
  *refusal*, not the const's value.
- **CH-13 Added-derived shadow restore.** A manifest with populated `Aliases.Added`/`Functions.Added`
  but an EMPTY `Shadowed` map yields N `RestoreShadowedAlias`/`RestoreShadowedFunc` ops (one per
  added name). This proves the diff derives restore ops from `Added`, NOT from the always-empty
  `Shadowed` map (D-06) — iterating `Shadowed` would silently emit zero restore ops and fail SPEC
  Req 7 with no visible error. This is a subtle-but-critical test: it guards against a plausible
  wrong implementation.
- **`Dynamic` propagation.** Apply ops carry the source entry's `Dynamic` bool so `emit.go` keys
  quote-vs-verbatim without re-deriving.
- **Token-free structural test (SPEC Req 3 acceptance, C22 — the PRECISE grep).** A test greps
  the `core/activate` non-test source and asserts NO zsh reverse-op token appears as a *string
  literal*: case-sensitive, word-boundary `\bunalias\b` / `\bunset -f` / `\bunsetopt\b` /
  PATH-array-rebuild, EXCLUDING Go identifier type names (`Unalias`, `UnsetFunc`, `SetOption`) and
  comments. C22 (REFUTED the naive grep) is the reason this must be precise: a naive `grep alias`
  false-matches Go identifiers and the forward `alias`/`export` that `regen.go` legitimately emits.
  Pair it with an import-graph assertion (`core/activate` imports only `zsh-pro/core/model`).

### 4. `core/shell/zsh/emit.go` codegen (Plan 04-02 Task 1) — **HYBRID: pure-Go shape TDD + zsh-requiring parse-validity/behavior**

`emit.go` is the phase's highest-risk file (T-01-06: a quoting bug here is a shell-injection bug).
It is tested on two independent axes because neither alone is sufficient.

**Axis A — pure-Go shape assertions (always run, TDD).** Assert the *emitted string shape*:
- Emitted apply/deactivate functions are **plain** — no `emulate -L`, no `LOCAL_OPTIONS`,
  no `setopt localoptions` (Pitfall 1 / C8). A pure-string grep of the emitted output. This is the
  cheapest guard against the single most likely regression (options auto-reverting at function
  return would silently defeat SetOption).
- A static value containing `'`/`;`/`$(...)`/backtick/newline is emitted single-quote-wrapped with
  the `'\''` escape; a dynamic value stays verbatim; the split keys strictly on the op's `Dynamic`
  bool (C5 PROVEN for the quote, C7 REFUTED the "regen already does this" — the Dynamic-keyed split
  is NEW behavior emit.go implements, so this test is the proof that it *does*).
- A function body is restored via `functions[name]=$captured`, NEVER single-quote-wrapped (C6
  REFUTED zquote-on-a-function-body — a body is live code). Shape test: the emitted restore for a
  function is the verbatim-reassign form, not a quoted literal.
- CH-11/CH-12: the deactivate list op emits a FULL rebuild-to-base with NO per-element removal
  loop; `${path:#pattern}` (C10 glob) is NEVER emitted; unquoted `== $target` is NEVER emitted; the
  quoted-RHS `[[ $e == "$target" ]]` literal-equality form (C25 PROVEN) appears ONLY as a
  documented/reserved (unexercised) comment for future deletion. Grep asserts the reserved form is
  inert and the glob form is absent.
- CH-5 (testability affordance): the PATH-rebuild and value-quoting logic are factored into
  package-level `renderList`/`renderValue` vars so the property test's mutants can substitute them
  (`Provider{}` is a zero-value struct with no fields to flip). Assert the vars exist. This is a
  *structural* requirement that exists to make the SW-02 mutants possible — see §"Mutants."

**Axis B — zsh-requiring parse-validity + behavior (skip-guarded).**
- **`zsh -n` parse-validity (SPEC Req 4 acceptance, C18 PROVEN).** Every emitted apply and
  deactivate block must pass `zsh -n` (syntax-valid). Critically (C18): run the gate on the RAW
  emitted block — an `eval`-wrapped malformed inner block would pass the OUTER `-n`, masking the
  bug. This is a behavior claim only a real zsh can make.
- **Injection inertness under double-eval** (the canary test — see §"Injection-safety"). This is
  behavioral and zsh-requiring; it is the payoff of Axis A's shape assertions.

**Single-emit-path invariant (Plan 04-02, `invariant_test.go`) — PURE-GO grep.** A tree-wide grep
asserts reverse zsh tokens (`unalias`/`unset -f`/`unsetopt`/PATH-array rebuild) are generated ONLY
under `core/shell/zsh/emit.go` (and its `_test.go` fixtures). This is the milestone invariant (D-11,
SPEC Req 4 acceptance) — it keeps the entire injection surface confined to one auditable file.

### 5. Extended introspect body-dump (Plan 04-01 Task 3) — **zsh-requiring TDD (LookPath-guarded)**

**Why zsh-requiring.** The body-dump is a change to `introspectScript` (zsh source) + its parser.
Its correctness — "does `${aliases[name]}`/`${functions[name]}` capture the right bytes, and does
the NUL-framed reader survive a multi-line body?" — is only provable by running a real `zsh -f`
against a fixture. It follows the exact `introspect_test.go` precedent: `zsh -f -c`, 5s timeout,
`LookPath("zsh")` skip.

**What the tests must pin:**
- **Body capture (SPEC Req 8 acceptance).** `Introspect` on a fixture with `alias gs='git status'`
  and `foo() { echo hi }` returns `AliasBodies["gs"]=="git status"` (RHS only, C13) and
  `FunctionBodies["foo"]` = the body without the `name(){` wrapper (C12 — note C12 REFUTED the
  "leading tab on every line" universal, so assert byte-identity of the captured body, not a tab
  pattern).
- **Paired co-population (SPEC Req 8 pairing).** ONE `Introspect` call on ONE fixture defining BOTH
  the alias and the function proves the name maps AND body maps co-populate from a single call —
  not only when each kind is tested in isolation.
- **Multi-line body round-trip (the load-bearing case, C14).** A function body containing
  newline+tab+single-quote+`$`+`;` round-trips byte-identical. C14 REFUTED the naive `name\tbody`
  line format (truncates multi-line bodies) and REFUTED the "NUL can never appear" justification
  (a scalar CAN hold NUL on zsh 5.9, but a reparsed function body never does) — so the encoding is
  NUL-framed and the test asserts byte-identity through embedded newlines.
- **OQ-17 boundary (`##`-prefixed body line).** A body containing a line starting `## not a header`
  must NOT truncate the section — proving the parser bounds sections by NUL-preceded framing, not a
  `\n##` line scan (a comment line would fool a `\n##` scan; C23). Test the adversarial framing case.
- **Additive / non-breaking (SPEC Req 8 acceptance).** The existing name-only Introspect tests still
  pass unchanged (C23 confirmed the name maps and `analyze`-reads-only-`Available` are untouched).
- **Graceful degradation preserved.** zsh-absent / error path still returns `Available:false` with
  nil body maps.

### 6. Zero-residue property test (Plan 04-02 Task 2) — **zsh-requiring; THE SW-02 regression pin**

This is the phase's cornerstone. It is a *property* test (invariant over N≥20 random sequences),
not an example test, and it is the SW-02 acceptance in full. See §"The property test" below for
its complete design — it is the largest single subject of this strategy.

---

## Test-quality principles (the rules every Phase-4 test obeys)

These are the standing quality bars. They are what separate a test suite that *pins* SW-02 from
one that *looks like* it does.

### P1 — No disabled tests on requirements
Every SPEC acceptance criterion maps to a test that RUNS (§"SPEC→test map"). The zsh-requiring
tests SKIP (not disable, not delete) when zsh is absent — a skip is honest ("not exercised here"),
a `t.Skip`/build-tag-disabled test on a requirement is a lie. The property test runs in the DEFAULT
suite (D-17) — NOT behind a `spike` build tag (that tag was Phase-1 throwaway isolation; this pin
is durable and must be in `make check`).

### P2 — No circular tests (the residue oracle must be independent of the emitter)
The single most dangerous failure mode for this phase: a test that re-derives its expected residue
FROM the emitter it is testing. The property test's snapshot instrument is the extended
`introspectScript` + a full-env `${(@kv)parameters}` dump — it reads live shell state
*independently* of the emit path. The emitter writes apply/deactivate code; the snapshot reads the
shell's actual `$aliases`/`$functions`/`$options`/`$path`/exported+non-exported params. The
comparison is pre-snapshot vs post-snapshot, both taken by the same *independent* instrument. The
emitter never supplies the expected value. If the snapshot were derived from the `Plan` or the
emitted strings, a bug in emit would be invisible (the "expected" would drift with the "actual").

### P3 — Expected-value provenance: assert the INVARIANT, not magic constants
C9's lesson, verbatim: "assert the invariant, not the 3/7 constants." The Phase-1 POC pinned
`$#path==3` after rebuild vs `7` after blind-append — but those numbers hold only for base=2+add=1.
The property test asserts the INVARIANT — "`$#path` after a full balanced cycle is byte-identical to
the baseline `$#path`" and "the sorted `$path` CONTENTS are byte-identical to baseline" (CH-25) —
not a hardcoded element count. Likewise the round-trip test asserts value-identity, not a copied
JSON string (except the one legitimate fixture-key oracle, which IS the external contract). Any test
that hardcodes a count, a byte offset, or a specific slot name is a provenance smell.

### P4 — Assertion strength: byte-identical snapshot string equality, not field-presence
The residue check is a LITERAL empty diff of the pre-vs-post snapshot STRINGS (the Phase-1 bar,
D-16) — NOT a set of `IdentitySet` field presence checks. C19 REFUTED the weaker form: snapshotting
`${(ok)functions}` NAMES + `typeset -x` (exported-only) SILENTLY MISSES a redefined-but-unrestored
function BODY and a leaked non-exported var. So the snapshot carries name+VALUE for ALL params
(exported AND non-exported), function BODIES, alias BODIES, option STATES, and the ordered `$path`
CONTENTS — and the assertion is string equality across the whole concatenation. Field-presence is a
false-green generator; byte-identity is not.

### P5 — The mutated-emitter negative checks are what make SW-02 falsifiable
A property test that only ever passes proves nothing about its own sensitivity. SW-02's acceptance
explicitly requires a mutated emitter to FAIL. Phase 4 ships THREE independent mutants (§"Mutants"),
each asserted in its OWN sub-test independent of the happy path. If a mutant does NOT fail, the pin
is broken — the mutant sub-tests are meta-tests of the pin itself. This is the operationalization of
P2/P4: the mutants prove the instrument is coupled to the shell's real state and strong enough to
catch residue.

### P6 — Self-stability meta-assert (the instrument must not lie in either direction)
The full-env snapshot is NOT naively self-stable (C28 REFUTED the named-scalar allowlist — two
no-op snapshots drift on ~40 module-backed `undefined`-typed associations, funcstack-family, and
lazily-materialized specials). A false-RED instrument (drifting on no-op) is as useless as a
false-GREEN one. So the instrument uses a TYPE-CLASS filter (skip `(*special*|*tied*|*hide*|undefined)`,
keep user-scope `scalar[-export]`/`array`/`association`/`integer`/`float`; never call `${(P)k}` on a
filtered key, killing lazy-init drift) + a reserved `ZP__` prefix for the instrument's own locals +
fd/temp-file capture (not `$(...)`, which forks and perturbs params). And this is META-ASSERTED:
two consecutive no-op snapshots diff to EMPTY (self-stable) AND the SAME filter still lets a
non-exported leak and an exported value-only change FIRE the diff. Both must hold with one filter —
the filter must not be so broad it masks a real leak (C28 PROVEN self-stable 5/5 with both
meta-asserts firing).

---

## Injection-safety testing as a first-class dimension

Injection safety is THE risk of this phase (ROADMAP line 214, threat T-01-06): the emitted code is
`eval`'d by a sourced loader, so a quoting/escaping bug is a shell-execution bug. It is tested as a
first-class dimension, not an afterthought, across FIVE injectable classes with a UNIFORM boundary:

| Injectable class | Evidence | Emit position | Defense |
|---|---|---|---|
| Env scalar / alias VALUE | C5 (zquote inert), C7 (Dynamic-keyed split) | value context | static → `'\''`-wrapped inert; dynamic → verbatim (portability) |
| Function BODY | C6 (REFUTED zquote — body is live code) | `functions[name]=` | verbatim capture-and-reassign, NUL-transported as DATA (never zquote'd) |
| Option NAME | C24 (slot), C29 (option name) | `[[ -o opt ]]`/`setopt`/`unsetopt`/slot | validate `^[A-Za-z_][A-Za-z0-9_]*$`, drop-to-no-part |
| Alias/function NAME | C31 (RAW metachar names) | `unalias`/`alias=`/`functions[]=`/`unset -f`/RestoreShadowed* | validate `^[A-Za-z0-9_][A-Za-z0-9_.-]*$` on RAW bytes, drop-to-no-part |
| PATH addition SEGMENT | C32 | `path=(<additions> $path)` array | static → zquote'd inert; dynamic self-reference → verbatim |

**The canary-file test proves inertness (the load-bearing mechanism).** Because the emitted code is
`eval`'d, the definitive proof that a value is INERT is: emit code containing an adversarial payload
(`x; touch $CANARY`, `/opt/x$(touch $CANARY)`, backtick-command, `gs;touch $CANARY`), source it under
`zsh -f` (the loader double-eval path), and assert the canary file was NOT created. A field-equality
check cannot prove this — only observing that arbitrary code did NOT execute can. Each class runs the
adversarial corpus through its emit path:
- The VALUE corpus (C5) through `renderValue` → the canary must not fire; the value appears as exact
  literal bytes.
- The NAME corpus (C29/C31) through BOTH the builder (drop-to-no-part, pure-Go — hostile → empty
  part) AND emit (defense-in-depth re-validation — hostile → inert). The builder is the first line;
  emit re-validates because it is the sole reverse-syntax codegen seam. C31 confirmed parse.go
  extracts names RAW (retaining the backslash in `gs\;x`), so the validator runs on the RAW bytes.
- The SEGMENT corpus (C32) through `renderList` per-segment → a static metacharacter segment is
  zquote'd inert (no canary), a dynamic `$HOME/bin` segment expands late-bound (the resulting `$path`
  holds the `$HOME`-derived dir, NOT the literal `$HOME/bin` — over-quoting would freeze it, a
  portability bug).

**Negative control (proves the quoting is load-bearing).** For the value and segment paths, an
UNQUOTED raw-splice control MUST fire the canary (C5/C32 both confirmed the unquoted control fires).
Without the negative control, a passing canary test could mean "the payload happened to be harmless"
rather than "the quoting neutralized it." The dropped-zquote mutant (§"Mutants") is this negative
control promoted to a first-class SW-02 mutant.

**The builder-tier validators are pure-Go security tests** (§2): hostile option/alias/func names and
unsafe static PATH segments → empty part; realistic inputs → populated. These run always (no zsh)
and are the FIRST line of the defense-in-depth; the emit-tier canary tests are the SECOND line and
prove end-to-end inertness under actual eval.

---

## zsh-dependency handling

| Test | zsh required? | Guard | Rationale |
|---|---|---|---|
| Manifest round-trip / tri-state / Fork A | No | — | pure Go value transform |
| `core/activate` build / diff / token-free / import-graph | No | — | pure Go; token-free is a source grep |
| `emit.go` shape (plain-fn, quote-form, reserved-removal, renderList/Value vars) | No | — | asserts emitted STRING shape |
| `emit.go` `zsh -n` parse-validity | **Yes** | `LookPath("zsh")` skip | syntax-validity is a zsh claim (C18) |
| `emit.go` injection inertness (canary, double-eval) | **Yes** | `LookPath("zsh")` skip | inertness is only provable by observing non-execution |
| introspect body-dump (capture, multi-line, `##`-boundary) | **Yes** | `LookPath("zsh")` skip | dumps live shell state |
| zero-residue property test (SW-02 pin) | **Yes** | `LookPath("zsh")` skip | sources emitted code, snapshots live state |

The skip-guard shape is copied verbatim from `core/shell/zsh/introspect_test.go` and
`core/store/roundtrip_test.go` (`if _, err := exec.LookPath("zsh"); err != nil { t.Skip(...) }`).
The pure-Go tests ALWAYS run — so on a zsh-less CI, the manifest/builder/diff/shape/token-free/
name-validation coverage is unaffected, and only the behavioral zsh tests skip. This means the
injection *builder-tier* defense (validators) and the *emitted-shape* discipline are verified even
without zsh; only the end-to-end inertness/residue proofs require it. The zsh-requiring tests all
run inside ONE sourced `zsh -f` process where they snapshot state (Pitfall 5 / C20: `zsh -f`
suppresses rc but inherits parent env, so a cross-process snapshot re-inherits stray env and gives
false diffs — the whole switch sequence + both snapshots must live in one sourced script).

---

## The property test (SW-02 regression pin) — full design

This is the deliverable that makes Phase 4 trustworthy. It is a property test: an invariant
(zero residue) over N≥20 random switch sequences, not a fixed example.

**Structure (single `zsh -f` process, C20 / Pitfall 5).** For each of N≥20 random BALANCED
sequences: Go builds ≥2 profiles → `activate.Build` → Manifest → `activate.Diff` → Plan →
`Provider{}.Emit` → apply/deactivate strings; the WHOLE sequence + both snapshots run in ONE sourced
`zsh -f` script. Baseline snapshot → source each emitted apply/deactivate in the random order →
post snapshot → assert literal empty diff.

**Balanced sequences (CH-14).** The generator models the REAL single-active checkout:
deactivate-current-then-activate-next, so at most one profile is active at any point and two applies
never nest without an intervening deactivate. WHY it matters: profile B applied over an
already-applied A would capture A's `ll` (not base's) as its shadow prior, and deactivate-B would
restore A's `ll` → a residue vs base that is a HARNESS ARTIFACT, not an emit bug. A pure-Go
generator meta-check (no zsh) asserts every generated sequence has ≤1 active profile at any point —
this keeps the test honest (a mis-generated sequence would produce a false failure).

**The six-class byte-identical snapshot (P4).** The pre-vs-post comparison string is the
CONCATENATION of, ALL byte-identical across the N-sequence compare (CH-21/CH-25):
1. Type-class-filtered full-env `${(@kv)parameters}` — ALL params, exported AND non-exported,
   name+VALUE, NUL-framed (CH-10/C28; the self-stable instrument of P6). Covers env scalars.
2. SORTED function-BODY section (`${(@ok)functions}` — CH-22; sorted so it is byte-stable, reused
   from the extended introspect body-dump). Covers function bodies.
3. Dedicated PATH section carrying BOTH `$#path` AND the ordered `$path` array CONTENTS (CH-25 — a
   count-preserving reorder/swap is caught by the contents, not by `$#path` alone; PATH is
   order-significant so emitted in `$path` order, not re-sorted). Covers PATH.
4. Dedicated SORTED option-state section (`${(@ok)options}` → `optname=<on|off>` — CH-16/CH-20).
5. Dedicated SORTED alias-body section (`${(@ok)aliases}` → `name<NUL>body` — CH-16/CH-20).
   Sections 4/5 are DEDICATED because `aliases`/`options` are `*special*`-typed and thus EXCLUDED by
   the C28 type-class filter — without them, a leaked alias or unrestored option would pass
   byte-identical (a C19-class false-green on 2 of 6 classes). This is a direct consequence of P4:
   the filter that makes the env section self-stable also hides two classes, so those classes need
   filter-independent dedicated sections.

**Why concatenate into the GENERAL compare (CH-21/CH-25), not only isolated sub-checks.** Binding
the alias/option/PATH-content classes into the general N-sequence byte-identical string means a
leaked alias body, an unrestored option, OR a count-preserving PATH corruption makes the GENERAL
compare non-empty under arbitrary switch order — not merely a hand-picked fixture. The isolated
sub-checks (below) additionally pin *specific* mechanisms.

**Isolated sub-checks (each in its OWN single-sourced `zsh -f` baseline, OQ-26 — so a targeted
failure is never conflated with a sequence-order residue diff):**
- **Drift guard (SPEC Req 6, C21).** TWO explicit assertions: (i) a hand-edited managed env var
  SURVIVES deactivate (live≠applied → left intact, `${(P)+var}==1` for unset-vs-empty — where `var`
  HOLDS the name, the correct indirect form per C2/C21); (ii) an untouched managed var IS reversed to
  its prior.
- **Ownership-aware PATH (SPEC Req 6, C11 resolution / CH-15).** Base owns `/usr/local/bin`; a
  profile also adds it. After apply→deactivate: `$#path` byte-identical to baseline AND
  `/usr/local/bin` appears EXACTLY ONCE (guards both strip and duplicate — NOT merely "still
  present," which is trivially true for any base entry). C11 REFUTED naive per-addition subtraction
  under `typeset -U`; the single-active v2.0 answer is rebuild-from-base, and `/usr/local/bin` is
  BASE-owned so it survives. Multi-managed co-ownership is OUT OF SCOPE (OQ-13, documented in a
  comment).
- **Metacharacter PATH element (CH-12d).** A profile-added `/opt/tool*` (absent from base) is DROPPED
  by rebuild-to-base on deactivate — the correct single-active behavior. NOT a "quoted comparison
  spared it" assertion (there is NO per-element comparison on the deactivate hot path; C10 glob
  hazard is not on this path).
- **Shadow capture/restore (SPEC Req 7).** Both profiles override a pre-existing `ll` alias and `ff`
  function; after deactivate they are restored byte-for-byte via the CH-4-captured prior, and
  profile-ADDED (non-shadowed) names are gone.
- **Option drift/restore (CH-9, C27).** `extendedglob` live-ON before apply, profile
  `unsetopt extendedglob`; after deactivate restored ON via the captured `was_on` slot. Verified
  through the dedicated option-state section (options are `*special*`-typed, excluded from the
  env section).
- **PATH addition-segment injection (CH-24, C32).** A static metacharacter/command-sub segment
  through the ApplyListDelta emit path fires NO canary; a dynamic `$HOME/bin` segment expands
  late-bound.
- **`ZP_BASE_PATH` persistence (CH-23).** After the full sequence, `ZP_BASE_PATH` equals its baseline
  — deactivate cleans per-switch slots (`__ZP_ORIG_*`, shadow-priors, sentinels, `was_on`) but NEVER
  unsets the session-persistent base.

### Mutants (P5 — three, each independently asserted)
The SW-02 acceptance requires a mutated emitter to fail. Substitute/restore the package-level
`renderList`/`renderValue` vars (CH-5):
1. **Blind-append PATH** — `renderList` → `path=(<add> $path)` with no rebuild-from-base → the
   general snapshot diff is non-empty / `$#path` grows (C9/C19 mechanism).
2. **Dropped-zquote** — `renderValue` → emit a static metacharacter value verbatim → the canary
   FIRES / the value is not literal (the negative control of §"Injection-safety").
3. **Base-strip** — `renderList` → a variant that omits/over-removes base entries → the strengthened
   base-ownership sub-check FAILS (non-baseline `$#path` or missing `/usr/local/bin`, CH-15).

Each mutant is asserted in its own sub-test, separate from the happy path. If any mutant does NOT
fail, the pin is broken — these are the meta-tests of the pin's own sensitivity.

### Instrument meta-asserts (P6)
- Self-stability: two consecutive no-op snapshots → empty diff (type-class filter + `ZP__` prefix +
  fd-capture neutralize volatility; C28 PROVEN self-stable 5/5).
- Leak detection: the SAME filter still fires on a non-exported var leak (`typeset localonly=x`) and
  an exported value-only change (`export EXISTING=changed`) — proving the filter is not so broad it
  masks residue (CH-2/CH-10).
- Populated-map guard (CH-16): `zmodload zsh/parameter` in the preamble + assert a known user scalar
  appears in the baseline full-env snapshot — a silently-empty `parameters` map would be an
  all-classes false-green.

### N and `-short`
Fixed N=20 (D-17, honoring the SW-02 N≥20 floor). `testing.Short()` MAY reduce N but MUST keep all
three mutant negative-checks — the mutants are the falsifiability guarantee and cannot be shorted
away. Composes ALONGSIDE (never replaces) the `core/testgen` oracle property test — both are
standing regression pins; `make check` stays green with both.

---

## Cold-start / smoke test

**N/A by construction — justified.** Phase 4 adds NO startup path, NO database, NO migration, NO
service boot. It produces value types (`model.Manifest`, `activate.Plan`) and emitted zsh strings;
it does NOT install or invoke a loader (that is Phase 5 / BOOT-01/02, explicitly out of scope). There
is nothing to "cold-start."

The nearest analog to an integration smoke — the full switch loop `Profile → Build → Manifest →
Diff → Plan → Emit → source under zsh → snapshot` — IS the zero-residue property test. It exercises
the entire Phase-4 pipeline end-to-end in one `zsh -f` process. So the property test doubles as the
integration smoke for the switch loop; no separate smoke test is warranted. The composition-root
wiring (the `shell.Emitter` seam in `main.go`, Plan 04-02) is verified by `go build ./...` compiling
— it is injectable-but-not-yet-driven (Phase 5 drives it), mirroring the existing Regenerator
`_, _ = s, err` pattern, so a runtime smoke of an un-invoked seam would test nothing.

---

## SPEC acceptance criterion → test-kind map

| # | SPEC Req (acceptance) | Test kind | Where | zsh? |
|---|---|---|---|---|
| 1 | Manifest round-trips to/from `01-MANIFEST-SHAPE.md`; all four parts; tri-state Original | Pure-Go TDD table + `go-cmp` round-trip + fixture-key oracle + tri-state cases (P3) | `core/model/manifest_test.go` | No |
| 2 | Builder: managed declarative → parts; imperative → no part | Pure-Go TDD: one-of-each-class + imperative-yields-nothing (T-04-04 precision) | `core/activate/builder_test.go` | No |
| 3 | Diff: ordered deactivate-then-activate Plan; NO zsh syntax | Pure-Go TDD ordering + empty cases + PRECISE token-free grep (C22) + import-graph | `core/activate/diff_test.go`, `tokenfree_test.go` | No |
| 4 | emit.go sole reverse-token source; emitted code passes `zsh -n` | Pure-Go tree-grep invariant (D-11) + zsh-requiring `zsh -n` on RAW block (C18) | `invariant_test.go`, `emit_test.go` | grep No / `-n` Yes |
| 4 (plain) | Emitted fns plain (no `emulate -L`/`LOCAL_OPTIONS`) | Pure-Go emitted-string grep (C8) | `emit_test.go` | No |
| 5 | Zero-residue over N≥20; six-class byte-identical; PATH stable; mutant fails | zsh-requiring PROPERTY test (the SW-02 pin) + 3 mutants + meta-asserts (P2/P4/P5/P6) | `residue_test.go` | Yes |
| 6 (drift) | Hand-edited managed var survives; untouched reversed (`${(P)+var}==1`) | zsh-requiring sub-check, two explicit assertions (C21) | `residue_test.go` | Yes |
| 6 (ownership) | Deactivating one profile does not strip a shared base PATH entry | zsh-requiring sub-check: `$#path` identical + exactly-once (CH-15/C11) | `residue_test.go` | Yes |
| 7 | Shadowed alias+function restored byte-for-byte; added names gone | zsh-requiring sub-check (CH-4 capture/restore) | `residue_test.go` | Yes |
| 8 | introspect captures bodies; absent-zsh → Available:false; name-only tests pass | zsh-requiring TDD (capture, multi-line C14, `##`-boundary OQ-17) + additive-non-breaking | `introspect_test.go` | Yes (name-only + degradation subsumed) |
| Injection | env/alias/func value injection-safe under eval | Builder validators (pure-Go, drop-to-no-part) + emit canary/double-eval (C5/C6/C7/C29/C31/C32) | `builder_test.go`, `emit_test.go`, `residue_test.go` | validators No / canary Yes |

Every SPEC acceptance criterion has a running test. The pure-Go rows run everywhere; the zsh rows
skip cleanly when zsh is absent (P1). The property test (Req 5) is the pin whose mutants (P5) make
it falsifiable and whose independent instrument (P2) makes it trustworthy.

---

*Phase: 04-manifest-builder-emit — Test strategy (philosophy, not code)*
*Grounded in 04-SPEC (8 reqs), 04-CONTEXT (D-01..D-17), the two plans (04-01/04-02), and the 32
validated claims of 04-EVIDENCE (12 PROVEN pass-1, C25-C32 PROVEN/STATIC-VALIDATED passes 2-5).*
*Next: /gsd:execute-phase 4 — TDD per the plans; the property test is the SW-02 regression pin.*
