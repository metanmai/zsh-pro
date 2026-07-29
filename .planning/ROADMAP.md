# Roadmap: zsh-pro

## Milestones

- ✅ **v1.0 Trustworthy Line Numbers** — Phase 1 (shipped 2026-06-24) — detail: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ⏸️ **v1.1 Trustworthy PATH Analysis** — parked 2026-06-25 (Phase 2 shipped; 3-4 re-scoped into v2.0 ingest) — detail: [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- 🚧 **v2.0 Branchable Shell Environments** — Phases 1-6 (in progress)

## Overview

v2.0 turns the shipped read-only analyze engine into a **branchable shell-environment manager**: ingest `~/.zshrc` into a categorized, regenerable store, make each git branch an environment profile, and let `checkout <branch>` live-reload an already-open terminal into that profile with **zero residue**. This is a brownfield integration — the existing parse → classify → introspect front-end is reused as-is through the `Provider` seam, and the new manager surface bolts on without rewriting anything.

The journey opens with a throwaway **spike** that de-risks the one genuine unknown (reversing aliases/functions/options live, not just env) before any plumbing is built — because if zero-residue hot-switch is infeasible, the product scope must change first. It then proceeds in strict dependency order: the **IR is the spine** (everything serializes a `model.Profile`), so it lands next, carrying the declarative/imperative split and the static/dynamic portability tag. The **git store** follows (you can't `checkout` between profiles that don't exist), then the **manifest + emit** layer (the single place zsh syntax is generated, turning a profile into a reversible record), then the **runtime loader + CLI + bootstrap** (the live-terminal wiring, fail-open and fast). The polished **ingest on-ramp** comes last so it targets the final IR shape rather than chasing a moving target.

**Architectural invariants held throughout:** no new dependencies (git and zsh via subprocess, mirroring the existing `zsh -f` pattern); the new packages (`core/profile`, `core/store`, `core/activate`) compose with the `Provider` seam and never bypass it; only `core/shell/zsh/emit.go` ever writes `unalias`/`unset -f`/`setopt` strings; `model.Block.Text` is the round-trip anchor; partial-eval stays static (never `eval`s user config).

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

<details>
<summary>✅ v1.0 Trustworthy Line Numbers (Phase 1) — SHIPPED 2026-06-24 · ⏸️ v1.1 PATH Analysis (Phase 2) — PARKED 2026-06-25</summary>

Previous-milestone phases are archived. Full detail in [milestones/](milestones/). v2.0 below restarts phase numbering at 1.

</details>

### 🚧 v2.0 Branchable Shell Environments (In Progress)

**Milestone Goal:** Represent `~/.zshrc` as a categorized, regenerable, git-backed store where each branch is an environment profile, and let `checkout <branch>` live-reload an already-open terminal into that profile with zero residue.

- [x] **Phase 1: SPIKE — Zero-Residue Live Hot-Switch** - Prove (or disprove) that a live terminal can `activate → switch → switch-back` with a byte-identical environment, reversing aliases/functions/options, not just env — before building any IR, store, or CLI (completed 2026-06-25)
- [x] **Phase 2: IR + Partial Evaluation** - Build the regenerable `model.Profile` from parsed blocks, tag each entry declarative/imperative and static/dynamic, and regenerate behavior-equivalent per-category `.zsh` (completed 2026-06-26)
- [x] **Phase 3: Git-Backed Store** - Store the IR as a git repo (via the `git` binary) where each branch is a profile; create/list/switch profiles tracked per-terminal, with detected secrets excluded by default (completed 2026-06-27)
- [x] **Phase 4: Manifest Builder + Emit** - Turn a profile into a reversible `Manifest` (record-and-reverse with a drift guard; PATH as a delta vs captured base) and emit the apply/deactivate zsh code from the one place zsh syntax lives (completed 2026-07-27)
- [ ] **Phase 5: Runtime Loader + CLI + Bootstrap** - Wire the live terminal via a sourced emit-and-source loader and the `checkout`/`activate`/`deactivate`/`list`/`status` verbs, bootstrapped by an idempotent `.zshrc` block that is fail-open and fast
- [ ] **Phase 6: Ingest End-to-End** - Polish the on-ramp: parse a real `~/.zshrc` → classify → partial-eval → regenerate → commit to the baseline branch, against the final IR shape

## Phase Details

### Phase 1: SPIKE — Zero-Residue Live Hot-Switch

**Goal**: De-risk the frontier before committing to any design. Prove that a live, already-open terminal can apply a profile's declarative state and reverse it with zero residue — reversing aliases, functions, and **options**, not just env — using a hand-written manifest and a hard-coded two-profile fixture. The output is a go/no-go decision and the validated shape of the `Manifest`.
**Depends on**: Nothing (first phase)
**Requirements**: SW-03
**Success Criteria** (what must be TRUE):

  1. In an already-open terminal, `activate A → activate B (auto-deactivates A) → deactivate B` leaves `$aliases`, `$functions`, `$PATH`, `$path`, exported env, and `$options` (captured via `zmodload zsh/parameter`) **byte-identical** to the pre-activation snapshot.
  2. Repeated switch cycles do not grow `$PATH` (no duplicate or accumulated entries), and no alias/function/option from a prior profile survives a switch.
  3. The drift guard holds: when the user changes a managed env var by hand mid-session, deactivate does **not** clobber that change.
  4. A written go/no-go decision records which zsh state classes (if any) are not cleanly reversible, narrowing the managed set before the manifest is designed; the hand-written manifest JSON shape is captured as the input to Phase 4.

**Plans**: TBD

Plans:
**Wave 1**

- [x] 01-01: TBD (hand-written loader + two fixture manifests; live `activate → switch → switch-back` round-trip)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 01-02: TBD (zero-residue assertions across the three failure modes; go/no-go + manifest-shape writeup)

### Phase 2: IR + Partial Evaluation

**Goal**: Build the spine everything else serializes — a near-lossless, regenerable `model.Profile`/`Entry` constructed from the parser's existing `[]model.Block`, with each entry classified declarative-vs-imperative (the switchability gate) and tagged static-vs-dynamic for portability, then regenerated back to behavior-equivalent per-category `.zsh` anchored on verbatim `Block.Text`.
**Depends on**: Phase 1 (the spike defines what the manifest — and therefore the IR — must carry)
**Requirements**: ING-01, ING-02, EVAL-01
**Success Criteria** (what must be TRUE):

  1. `~/.zshrc` parses into a `model.Profile` whose entries are grouped by category (env / aliases / functions / PATH / options), and regenerating from it reproduces a behavior-equivalent `.zsh` — untouched statements emitted verbatim, only rewritten declarative slices templated.
  2. Each statement is classified **declarative** (set/unset-reversible → switchable) or **imperative** (run-once, side-effecting → unmanaged); when classification is uncertain, the entry defaults to unmanaged (precision over recall).
  3. A value containing `$HOME`, `${...}`, `$(...)`, backticks, or a conditional is tagged dynamic and kept **late-bound verbatim** — never resolved against the current machine; a value that is a syntactic constant is tagged static. A profile authored with `$HOME` round-trips with `$HOME` intact.
  4. Partial-eval performs **no execution** of user config (static AST inspection only); `util.ExpandHome` is never used inside the IR.

**Plans:** 3/3 plans complete

Plans:

**Wave 1**

- [x] 02-01-PLAN.md — model.Profile/Entry types + parser value/dynamic capture (Approach A) + ir.Build + declarative/imperative routing gate

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 02-02-PLAN.md — shell.Regenerator seam + declarative templater + ir.Regenerate (source order) + byte-identical round-trip oracle

**Gap closure** *(UAT array-assignment round-trip defect)*

- [x] 02-03-PLAN.md — detect array assignments (`name=(...)`) at parse time (additive Block.Array), route them imperative (routeManaged), oracle coverage + defensive empty-Value templater guard

### Phase 3: Git-Backed Store

**Goal**: Make profiles real and switchable. Store the regenerated per-category `.zsh` as a git repo via the `git` binary (no new dependency), with branch = profile and the baseline branch holding the ingested `~/.zshrc`. Expose create / list / switch typed in terms of `model.Profile`, track the active profile **per-terminal** (never a shared global file), and keep detected secrets out of the committed tree by default.
**Depends on**: Phase 2 (the store serializes a stable `model.Profile` type)
**Requirements**: PROF-01, PROF-02
**Success Criteria** (what must be TRUE):

  1. The store initializes a git repo via the `git` binary (subprocess + timeout, mirroring `zsh -f`); no new Go module is added, and an absent `git` degrades with a clear error rather than crashing.
  2. The user can create a profile, list profiles (= branches), and switch with `checkout <branch>`; `Read`/`Commit` round-trip a `model.Profile` through the per-category files.
  3. The active profile is tracked per-terminal (env-var-carried), so switching in one terminal never changes another and concurrent switches cannot corrupt a shared "current profile" file.
  4. Detected secrets (reusing the shipped secret detection) are excluded from the committed tree by default, and the user is told what was withheld.

**Plans**: 3 plans

Plans:

**Wave 1**

- [x] 03-01-PLAN.md — SecretRef contract (core/model) + lossless deterministic Profile<->JSON serialization + git subprocess driver/plumbing primitives + zsh-pro-phrased errors

**Wave 2** *(blocked on Wave 1)*

- [x] 03-02-PLAN.md — core/store orchestrator: Init (idempotent bare repo + main baseline) / Branches / Current (per-terminal ZSHPRO_PROFILE) / Create (forks main) / Checkout / Commit (plumbing-to-branch, no checkout) / Read (git show); Read(Commit(p)) round-trip pin composed with the Phase 2 oracle

**Wave 3** *(blocked on Wave 2)*

- [x] 03-03-PLAN.md — secret exclusion on Commit (literal->SecretRef + capture + withheld-report; already-dynamic verbatim) + KeychainDriver (security/secret-tool/0600 vault) + composition-root wiring + PROF-03 traceability (D-10)

### Phase 4: Manifest Builder + Emit

**Goal**: Turn a resolved profile into the reversible record the runtime applies, and generate the shell code from the single place zsh syntax may live. Build `model.Manifest` (`Scalar`/`ListDelta`/`NameSet`/`OptionSet`) and `core/activate` (build manifest, diff active-vs-target, produce a deactivate-then-activate plan as a shell-agnostic structure), with `core/shell/zsh/emit.go` rendering that plan to zsh apply/deactivate code. Extend the existing introspect to dump alias/function **bodies** so shadowed definitions can be restored.
**Depends on**: Phase 1 (the spike proved the design and fixed the manifest shape), Phase 2 (the IR resolves a profile)
**Requirements**: SW-01, SW-02
**Success Criteria** (what must be TRUE):

  1. Applying a profile's declarative state happens via emitted shell code that a sourced loader `eval`s (a child process never mutates the parent shell); all zsh syntax is generated in `core/shell/zsh/emit.go` only — `core/activate` emits a plan, never shell text.
  2. Switching deactivates the prior profile (reverse-diff: `unalias`, `unset -f`, restore env to captured prior values, rebuild PATH from a captured base) then activates the new one, with **zero residue** verified by a property test: N random switch sequences leave a profile's final env/aliases/functions/options/PATH path-independent and dupe-free.
  3. Restore is ownership-aware: deactivate removes only what this profile added (drift guard — restore a value only if the live value still equals what was applied) and never strips base/unmanaged state a profile merely also added (e.g. `/usr/local/bin`).
  4. A shadowed prior alias/function is captured before override and re-established on deactivate; PATH is stored as a delta against the captured base, never a wholesale overwrite.

**Plans**: 18/19 plans executed

Plans:

- [x] 04-ACCEPTANCE-TESTS.md — acceptance-test design executed by 04-UAT.md

**Wave 1**

- [x] 04-01-PLAN.md — `model.Manifest` + four parts (Fork A two-type split, tri-state `Original`, `SchemaV1`); `core/activate` builder + agnostic `Plan` + differ (token-free, deactivate-then-activate); additive NUL-framed introspect body-dump + `IdentitySet` companions (completed 2026-07-18)

**Wave 2** *(blocked on Wave 1)*

- [x] 04-02-PLAN.md — `core/shell/zsh/emit.go` reverse codegen (injection-safe `zquote`/verbatim-dynamic split, drift-guarded `${(P)+var}`, ownership-aware PATH rebuild-from-base, `${+name}` shadow guards, verbatim function-body restore); `shell.Emitter` seam + composition-root wiring; zero-residue property test (N≥20, mutated-emitter negative check)

**Wave 3** *(gap closure; blocked on Wave 2)*

- [x] 04-03-PLAN.md — explicit legacy/literal/dynamic/unsupported runtime-value contract; AST-only fail-closed decoding; empty/multiline/compound/redirected function-body capture; IR/store compatibility

**Wave 4** *(gap closure; blocked on Wave 3)*

- [x] 04-04-PLAN.md — backward-compatible manifest dynamic provenance + function body map; ValueMode-aware Build/Diff; empty-function emission; real source-to-live-zsh regression pipeline

**Wave 5** *(gap closure; blocked on Wave 4)*

- [x] 04-05-PLAN.md — deterministic balanced N≥20 zero-residue property; type-aware collision-safe full-state oracle; self-tests and three independent emitter mutants

**Wave 6** *(gap closure; blocked on Wave 5)*

- [x] 04-06-PLAN.md — reduce declarations to final effective shell identities; use collision-free presence-aware restore slots; preserve scalar export provenance and attributes
- [x] 04-07-PLAN.md — match zsh escape semantics exactly and capture secrets as semantic runtime values without freezing dynamic content
- [x] 04-10-PLAN.md — make introspection failures and framing explicit, fail-closed, and testable

**Wave 7** *(gap closure; blocked on Wave 6)*

- [x] 04-08-PLAN.md — carry PATH/FPATH as semantic list values with correct dynamic zero/one/many expansion
- [x] 04-11-PLAN.md — make secret storage and Git publication transactional, newline-safe, rollback-safe, and concurrency-safe

**Wave 8** *(gap closure; blocked on Wave 7)*

- [x] 04-09-PLAN.md — compose repeated PATH/FPATH statements while preserving an unset, empty, or non-empty base

**Wave 9** *(gap closure; blocked on Wave 8)*

- [x] 04-12-PLAN.md — expand the production-path residue oracle and enforce reverse-zsh ownership and test-discovery invariants (completed 2026-07-19)

**Wave 10** *(gap closure; blocked on Wave 9)*

- [x] 04-13-PLAN.md — fail closed on unsupported PATH/FPATH and declaration forms, and make multi-name functions apply and restore through the live zsh pipeline

**Wave 11** *(gap closure; blocked on Wave 10)*

- [x] 04-14-PLAN.md — compose legacy and semantic PATH/FPATH in source order and stop persisted forced declarations crossing the manifest boundary

**Wave 12** *(gap closure; blocked on Wave 11)*

- [x] 04-15-PLAN.md — preserve legacy dynamic PATH/FPATH provenance and fail closed for persisted structural syntax, including historical DTOs

**Wave 13** *(gap closure; blocked on Wave 12)*

- [x] 04-16-PLAN.md — preserve indexed-assignment and semantic export-flag source shape through the persisted representability boundary

**Wave 14** *(gap closure; blocked on Wave 13)*

- [x] 04-17-PLAN.md — prove indexed and semantic export state is inert and residue-free through the live zsh pipeline

**Wave 15** *(gap closure; blocked on Wave 14)*

- [x] 04-18-PLAN.md — preserve multi-assignment, alias, option, and delimiter export source shape through persistence

**Wave 16** *(gap closure; blocked on Wave 15)*

- [x] 04-19-PLAN.md — prove remaining source-shape forms stay inert and residue-free through live zsh

### Phase 5: Runtime Loader + CLI + Bootstrap

**Goal**: Wire the live terminal to the binary and ship the user-facing surface. Emit an embedded sourced loader from a `hook` subcommand (mirroring `introspectScript`), holding per-terminal active state in one env var and using `eval "$(...)"` to apply emitted code; add the `checkout`/`activate`/`deactivate`/`list`/`status` verbs; install an idempotent BEGIN/END `.zshrc` block that preserves the unmanaged master block; and make the loader fail-open and fast.
**Depends on**: Phase 4 (manifest + emit), Phase 3 (store)
**Requirements**: BOOT-01, BOOT-02
**Success Criteria** (what must be TRUE):

  1. `checkout`/`activate`/`deactivate` are sourced shell functions that `eval` the binary's emitted code and visibly change the **current** terminal (not just new shells); `list` and `status` report branches and the active profile.
  2. Re-running the installer leaves the `.zshrc` BEGIN/END block byte-identical (idempotent — never a duplicate block) and never touches content outside the markers; the unmanaged master block for imperative run-once code is preserved.
  3. A broken, missing, or slow `zsh-pro` never locks the user out: the stub guards sourcing (`command -v`, `[[ -r ]]`), generated manifests are `zsh -n`-validated with a last-good fallback, and `ZSHPRO_DISABLE=1` fully no-ops the loader.
  4. The hot path adds only a small, file-sourced startup cost — zero subprocesses (no `git`, no binary, no `$(...)`) on shell start — verified within budget via `hyperfine 'zsh -i -c exit'`.

**Plans**: 5/5 plans executed

Plans:
**Wave 1**

- [x] 05-01-PLAN.md
- [x] 05-01: TBD (embedded `loader.zsh` via `hook`; per-terminal `__ZSHPRO_STATE`; `checkout`/`activate`/`deactivate`/`list`/`status` CLI verbs)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 05-02-PLAN.md
- [x] 05-02: TBD (idempotent BEGIN/END `.zshrc` block writer; fail-open stub + `ZSHPRO_DISABLE`; `zsh -n` validation + last-good; hot-path perf budget)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 05-03-PLAN.md
- [x] 05-04-PLAN.md

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 05-05-PLAN.md

### Phase 6: Ingest End-to-End

**Goal**: Polish the on-ramp last, against the final IR shape. Compose the already-built pieces into the full path: parse a real `~/.zshrc` → classify (declarative/imperative split) → partial-eval → regenerate → commit to the baseline branch — with the idempotent managed-block install and detection/warning of installer appends that landed outside the managed block. This is the moat: adopt the messy file the user already has, never make them rewrite it.
**Depends on**: Phase 2 (IR), Phase 3 (store)
**Requirements**: PROF-03
**Success Criteria** (what must be TRUE):

  1. Running ingest on a real `~/.zshrc` produces a baseline-branch profile committed to the store, with declarative state captured and imperative lines routed to the unmanaged master block — never silently dropped.
  2. Detected secrets are excluded from the committed baseline by default and the user is told exactly what was withheld (reusing the shipped secret detection end-to-end).
  3. Re-running ingest/install is idempotent (the `.zshrc` managed block is byte-identical the second time), and installer appends that landed outside the managed block are detected and surfaced as a warning rather than clobbered.
  4. The committed baseline round-trips to a behavior-equivalent `.zshrc` (the round-trip oracle from Phase 2 holds against a real, end-to-end ingested file).

**Plans**: TBD

Plans:

- [ ] 06-01: TBD (end-to-end ingest path: real `~/.zshrc` → IR → baseline-branch commit; imperative-to-master-block routing)
- [ ] 06-02: TBD (secret-exclusion + withheld report on ingest; out-of-block installer-append detection/warning; end-to-end round-trip)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. SPIKE — Zero-Residue Live Hot-Switch | 2/2 | Complete   | 2026-06-25 |
| 2. IR + Partial Evaluation | 3/3 | Complete   | 2026-06-26 |
| 3. Git-Backed Store | 3/3 | Complete   | 2026-06-27 |
| 4. Manifest Builder + Emit | 19/19 | Complete    | 2026-07-27 |
| 5. Runtime Loader + CLI + Bootstrap | 5/5 | In Progress|  |
| 6. Ingest End-to-End | 0/2 | Not started | - |

## Requirement Coverage

All 11 milestone requirements mapped to exactly one phase (11/11, no orphans, no duplicates).

| Phase | Requirements |
|-------|--------------|
| 1. SPIKE — Zero-Residue Live Hot-Switch | SW-03 |
| 2. IR + Partial Evaluation | ING-01, ING-02, EVAL-01 |
| 3. Git-Backed Store | PROF-01, PROF-02 |
| 4. Manifest Builder + Emit | SW-01, SW-02 |
| 5. Runtime Loader + CLI + Bootstrap | BOOT-01, BOOT-02 |
| 6. Ingest End-to-End | PROF-03 |

## Research Flags

Phases likely needing a deeper research pass during planning (`/gsd:plan-phase --research-phase <N>`):

- **Phase 1 (SPIKE):** This *is* the research — plan it as a real spike with explicit kill-criteria. The unproven delta (reversing aliases/functions/**options** + any completion/keybinding/hook state live) has no direct prior-art guarantee; surface whatever zsh state turns out to be un-cleanly-reversible early.
- **Phase 4 (Manifest + Emit):** Shell-code emission is where quoting/escaping bugs become shell-injection bugs. The `${aliases[name]}` / `functions`-assoc-array body extraction and the exact deactivate ordering warrant verification against the zsh manual + the shadowenv source; the codegen round-trip oracle (`parse(generate(parse(src))) == parse(src)`) needs a design.
- **Phase 5 (Loader):** The fail-open stub, `ZSHPRO_DISABLE` recovery, idempotent BEGIN/END block writer, base-capture placement/re-capture guard, and the hot-path performance budget (`hyperfine`) are each subtle.

Phases with standard patterns (skip research-phase): **Phase 2** (reuses the shipped parser/classifier + the `render.go` codegen pattern; static-only partial-eval is well-specified), **Phase 3** (shelling out to `git` mirrors the `zsh -f` subprocess exactly; machine-format parsing is documented and stable), **Phase 6** (composes already-built pieces along a known data flow; the only subtlety is the idempotent managed-block, covered in Pitfall 6).
