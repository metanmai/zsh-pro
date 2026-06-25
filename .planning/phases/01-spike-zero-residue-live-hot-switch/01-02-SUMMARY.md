---
phase: 01-spike-zero-residue-live-hot-switch
plan: 02
subsystem: infra
tags: [zsh, spike, activation, failure-modes, drift-guard, manifest, zero-residue, go-build-tag]

# Dependency graph
requires:
  - phase: 01-01
    provides: "Durable snapshot() instrument + plain apply_*/deact_* loaders + two Manifest-shape fixtures (profile_a/b.json) proving byte-identical round-trip"
provides:
  - "01-FINDINGS.md (durable, D-08): per-class admit/exclude verdict for all six classes + overall go/no-go GO + completion fpath-vs-compinit nuance + bindkey/hook presence note + embedded loader reference snippet"
  - "01-MANIFEST-SHAPE.md (durable, D-08): validated Manifest JSON shape with final field names and per-field reverse-op justification — Phase 4's literal input"
  - "Hardened spike harness: FM1 (no-op round-trip), FM2 (no accumulation over 5+ cycles + explicit no-surviving-state check), FM3 (conditional drift guard on a MANAGED var), real-~/.zshrc reality-check pass (in-sandbox, delta round-trip, bindkey/hook report, graceful skip)"
  - "Throwaway Go verdict pin (scratch/spike_test.go, //go:build spike) under the real introspect subprocess shape — invisible to go test ./..., runs under -tags spike"
affects: [02-ir-spine, 04-shell-integration-emit, 05-activation-runtime]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Failure-mode assertion harness: each FM emits a single PASS/FAIL line under sandboxed zsh -f -c; the diff is the verdict"
    - "Build-tag isolation for throwaway tests: //go:build spike as the FIRST line keeps a Go pin out of go test ./... and the testgen oracle"
    - "Reality-check under Pitfall 5: assert the slice's DECLARED-name VALUE delta round-trips (not a name-set diff, which zsh -f env inheritance makes spuriously zero)"
    - "Conditional drift guard proven on a managed var: 3a hand-edit survives, 3b untouched reverses"

key-files:
  created:
    - ".planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md (durable D-08 deliverable 1)"
    - ".planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md (durable D-08 deliverable 2)"
    - "scratch/spike_test.go (git-ignored throwaway; //go:build spike): Go verdict pin"
  modified:
    - "scratch/spike.zsh (git-ignored throwaway): +zp_seed_base, +fm1/fm2/fm3, +realzshrc_pass, +zp_dump_named helper, +spike_all aggregate driver (225 -> 453 lines)"

key-decisions:
  - "Overall verdict GO (D-03): core classes aliases/env/PATH reverse byte-identical; functions+options also admitted; completion's compinit excluded to master block (fpath array admittable)"
  - "FM3 built on the MANAGED var EDITOR (not the no-op A_VAR in the plan's literal verify) to actually exercise the conditional drift guard in both directions"
  - "Reality-check asserts the DECLARED-name VALUE delta (parsed from the slice), not a live env name-set diff — the honest measure under zsh -f inheritance (Pitfall 5)"

patterns-established:
  - "Each failure mode is a self-contained, named PASS/FAIL function reusable as a Phase 4-5 regression check on the real switch loop"
  - "//go:build spike isolation pattern: throwaway Go that shells to zsh -f -c without touching the production test graph"

requirements-completed: [SW-03]

# Metrics
duration: 7min
completed: 2026-06-25
---

# Phase 01 Plan 02: Failure-Mode Hardening + Go/No-Go Verdict & Validated Manifest Shape Summary

**Stress-tested the Plan 01-01 harness against the three trustworthiness failure modes (residue, PATH accumulation, clobbering hand edits) plus a real-`~/.zshrc` reality-check, all green under sandboxed `zsh -f`, and shipped the two durable deliverables: a per-class go/no-go verdict (GO) and the validated `Manifest` JSON shape — Phase 4's literal input.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-06-25T15:45:02Z
- **Completed:** 2026-06-25T15:52:16Z
- **Tasks:** 2
- **Files modified:** 2 tracked (the durable deliverables); 1 git-ignored scratch file extended + 1 git-ignored scratch file created (D-08)

## Accomplishments
- **All three failure modes pass under sandboxed `zsh -f -c`** (extended `scratch/spike.zsh`):
  - **FM1** — `activate A → switch to B → deactivate B` leaves the six-class snapshot **byte-identical** (empty diff).
  - **FM2** — after **5 A↔B cycles**, `$#path` is stable (no PATH growth), the six-class diff is empty, and an **explicit absence check** confirms no added alias (`gs`/`ga`/`gp`), function (`work_deploy`/`personal_sync`), or option (`extendedglob`/`nocaseglob`) survives.
  - **FM3** — the **conditional** drift guard, proven on a MANAGED var (`EDITOR`): a hand edit survives deactivate (3a), while an untouched value is reversed to its prior (3b).
- **Real-`~/.zshrc` reality-check pass** (D-06/D-07): a 120-line slice copied to a `mktemp` temp file (read-only; never written back), sourced **in-sandbox only**. It reports the slice declares **4 exported env names** with **0 value changes** (all inherited identically — a live Pitfall-5 demonstration), asserts the declared-name **value delta round-trips**, **reports** bindkey (144 entries) + `precmd_functions=7` as data (A3, not a verdict), and **skips gracefully** (`REALZSHRC_SKIP`) when `~/.zshrc` is absent. The user's real shell and rc file are never mutated.
- **`01-FINDINGS.md` (durable deliverable 1, D-08):** six-row per-class admit/exclude table with diff evidence, overall **go/no-go GO** (core classes byte-identical, D-03), the completion fpath-vs-compinit nuance, the bindkey/hook presence note, and the embedded loader reference snippet (the surviving D-08 form).
- **`01-MANIFEST-SHAPE.md` (durable deliverable 2, D-08):** the validated `Manifest` JSON shape with final field names, each tied to the reverse op it enables, and an explicit table naming the **intentionally-absent** classes (compinit / keybindings / hooks → master block).
- **Throwaway Go verdict pin** (`scratch/spike_test.go`, first line `//go:build spike`): copies the `exec.CommandContext(ctx, "zsh", "-f", "-c", …)` + 5s-timeout shape (`introspect.go:42-53`) and the `LookPath` skip-guard (`introspect_test.go:11-13`), asserts a literal empty `diff S_pre S_post`. **`go test ./...` passes unchanged** (testgen oracle pin untouched); **`go test -tags spike ./scratch/...` runs the pin green**.

## Task Commits

1. **Task 1: three failure-mode assertions + real-`~/.zshrc` reality-check pass** — no tracked commit. All work landed in `scratch/spike.zsh`, which is git-ignored throwaway (D-08), exactly as Plan 01-01 Task 2. TDD gate (`tdd="true"`) satisfied **behaviorally**: RED (FM assertions absent → trustworthiness unproven) → GREEN (assertions added → FM1/FM2/FM3 + reality-check all PASS under `zsh -f -c`). No `test(...)`/`feat(...)` commit because the artifact is git-ignored.
2. **Task 2: durable deliverables + build-tag-isolated Go pin** — `5f2356c` (`docs(01-02): spike findings + validated manifest shape`). The two `.planning/` deliverables are tracked and committed; `scratch/spike_test.go` is git-ignored throwaway (D-08).

**Plan metadata:** this SUMMARY + STATE/ROADMAP updates commit.

_Note: per D-08 the spike is throwaway; the only durable, committed outputs are `01-FINDINGS.md`, `01-MANIFEST-SHAPE.md`, and this SUMMARY. The harness + Go pin are git-ignored and will be deleted after the verdict._

## Files Created/Modified
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-FINDINGS.md` (tracked) — the go/no-go writeup (deliverable 1)
- `.planning/phases/01-spike-zero-residue-live-hot-switch/01-MANIFEST-SHAPE.md` (tracked) — the validated Manifest shape (deliverable 2)
- `scratch/spike.zsh` (git-ignored) — extended from 225 → 453 lines: `zp_seed_base`, `fm1_no_op_roundtrip`, `fm2_no_accumulation`, `fm3_drift_guard`, `realzshrc_pass`, `zp_dump_named` helper, `spike_all` aggregate driver
- `scratch/spike_test.go` (git-ignored) — `//go:build spike` Go verdict pin (`TestSpikeByteIdenticalRoundTrip`, `TestSpikeFailureModes`)

## Decisions Made
- **Overall verdict: GO (D-03).** The core classes (aliases/env/PATH) reverse byte-identical, so no NO-GO condition fires. Functions and options are also admitted byte-identical. Completion splits: the `fpath` array is byte-reversible (admittable as fpath-membership) but `compinit` is imperative → **EXCLUDED to the master block** (the expected D-02/D-05 scoping outcome, not a product-killer).
- **FM3 tests a MANAGED var, not the plan's literal `A_VAR`.** The plan's `<automated>` DRIFT command uses `A_VAR` (a sentinel the loaders never touch), which passes trivially. To actually exercise the *conditional* guard, the FM3 function drifts `EDITOR` (which `apply_A` sets) and asserts both directions: hand-edit survives (3a) AND untouched reverses (3b). This builds directly on Plan 01-01's `zp_capture_env`/`zp_restore_env` drift-guard behavior, as the orchestrator directed.
- **Reality-check measures the VALUE delta of the slice's declared names, not the live env name-set diff.** Under `zsh -f` env inheritance (Pitfall 5), an `export FOO=…` for an already-inherited `FOO` changes its value, not the name-set — a name-set diff reports a spurious zero. Parsing the slice's declared names and round-tripping their values is the honest measure.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Reality-check name-set delta was spuriously zero under `zsh -f` env inheritance**
- **Found during:** Task 1 (real-`~/.zshrc` reality-check pass)
- **Issue:** The first draft asserted the *introduced env NAME-set* delta (names present after sourcing the slice but not before). Because `zsh -f` inherits the parent's exported env (Pitfall 5), the slice's `export ZSH/LANG/LC_*` lines re-set names that were **already inherited**, so the name-set delta was **0** — making the "delta round-trips" assertion vacuous (it round-tripped because there was nothing to round-trip). That is weak evidence and misframes the actual behavior.
- **Fix:** Switched to parsing the **declared** names directly from the slice text and round-tripping their **VALUES** (via a new `zp_dump_named` helper that emits `name=VALUE` only for set names, distinguishing unset). The pass now reports "slice declares 4 names; N changed value in-sandbox (rest inherited identically — Pitfall 5)" and asserts the declared-name value delta returns to the pre-source values. Also widened the slice from 80 → 120 lines to span the user's export block.
- **Files modified:** `scratch/spike.zsh` (git-ignored throwaway)
- **Verification:** `realzshrc_pass` now prints `REALZSHRC_PASS` with a substantive "4 declared names" finding; the graceful-skip path (`REALZSHRC_SKIP`) verified by running with `HOME=/nonexistent`. `go test -tags spike ./scratch/...` green; `go test ./...` unchanged.
- **Committed in:** N/A — the fix lives only in the git-ignored scratch harness (D-08); it is documented here and reflected in `01-FINDINGS.md`'s reality-check section as a load-bearing Pitfall-5 finding for Phase 4.

---

**Total deviations:** 1 auto-fixed (1 bug, Rule 1)
**Impact on plan:** The fix turned a vacuous reality-check into a substantive one and surfaced a concrete Phase-4 finding (under `zsh -f` env inheritance, measure value deltas, not name-set deltas). No scope creep — confined to the throwaway harness; all plan success criteria met.

## Issues Encountered
- **`zsh -f` inherits the parent env/PATH/bindkey/hooks (Pitfall 5), reconfirmed.** The reality-check's bindkey count (144) and `precmd_functions=7` reflect the *inherited* interactive base, not the slice — so they vary by invoking shell and are correctly reported as **data, not a verdict** (A3). This is itself the finding: hook state is real in practice, so Phase 4 must decide whether to manage keybindings/hooks or route them to the master block.

## TDD Gate Compliance
Task 1 (`tdd="true"`) is an empirical shell spike, not Go product TDD. The RED/GREEN gate was satisfied **behaviorally** (RED: failure-mode assertions absent → trustworthiness unproven; GREEN: assertions added → FM1/FM2/FM3 + reality-check all PASS under sandboxed `zsh -f -c`), but the artifacts are git-ignored throwaway (D-08), so there are no `test(...)`/`feat(...)` git commits to validate — consistent with the plan's throwaway-spike framing and Plan 01-01's precedent. The standing `core/testgen` oracle property pin is untouched and green (verified via `go test ./...` and `go test ./core/testgen/...`).

## Next Phase Readiness
- **SW-03 retired GO and hardened.** The single material milestone-v2.0 risk (zero-residue *live* hot-switch) is gone: byte-identical across the core classes, no accumulation over repeated cycles, and a conditional drift guard that preserves hand edits.
- **Manifest shape validated and documented** (`01-MANIFEST-SHAPE.md`) with no missing field for any admitted class — ready to become `model.Manifest` in the Phase 2 IR spine.
- **Phase-4 carry-forwards (for `core/shell/zsh/emit.go`):** capture the LIVE prior env value (not the static `original`); use `${(P)+var} == 1` for unset-vs-empty; emit PLAIN loader functions (no `emulate -L`/`LOCAL_OPTIONS`); rebuild PATH from a captured base (no `typeset -U`); decide keybinding/hook handling (data shows they are present); and solve the deferred injection threat (T-01-06) when emitting `eval`'d code from parsed user values.
- **No blockers.** `scratch/` (harness + Go pin) is git-ignored and will be deleted after the verdict; nothing leaks into product code. The two durable deliverables + this SUMMARY are the phase's surviving outputs.

## Self-Check: PASSED

- Files verified present: `01-FINDINGS.md`, `01-MANIFEST-SHAPE.md`, `scratch/spike.zsh`, `scratch/spike_test.go`, `01-02-SUMMARY.md`.
- Commit verified present: `5f2356c` (docs(01-02): spike findings + validated manifest shape).
- `go test ./...` (incl. testgen oracle pin) green; `go test -tags spike ./scratch/...` green; `scratch/` is git-ignored (D-08 holds). All FM1/FM2/FM3 + real-`~/.zshrc` (PASS and graceful-SKIP) verified under sandboxed `zsh -f -c`.

---
*Phase: 01-spike-zero-residue-live-hot-switch*
*Completed: 2026-06-25*
