---
phase: 01-spike-zero-residue-live-hot-switch
plan: 01
subsystem: infra
tags: [zsh, spike, activation, snapshot, manifest, zero-residue, zmodload]

# Dependency graph
requires: []
provides:
  - "Durable snapshot() instrument (D-01) — sorted, section-delimited six-class dump (aliases/functions/env/PATH/fpath/options) with alias+function bodies; reused to verify the real switch loop in Phases 4-5"
  - "Validated Manifest JSON shape (the real deliverable) instantiated as two hand-written fixtures (profile_a.json/profile_b.json)"
  - "Empirical GO verdict for SW-03: the live activate-A -> switch-to-B -> deactivate-B round-trip is byte-identical across all six classes under sandboxed zsh -f"
  - "Reference plain-function apply/deact loader snippet encoding both mandatory divergences (no emulate -L in loaders; rebuild-from-base PATH, no typeset -U)"
affects: [02-ir-spine, 04-shell-integration-emit, 05-activation-runtime]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Snapshot-diff determinism instrument: dump six state classes as sorted section-delimited text; byte-identical diff is the admission test"
    - "Plain-function apply/deact (options escape function scope) vs read-only snapshot() (emulate -L safe)"
    - "PATH rebuild from a captured ZP_BASE_PATH via plain array assignment (never typeset -U)"
    - "Drift-guarded env reverse capturing the LIVE prior at apply time; unset-vs-empty via ${(P)+var} == 1"

key-files:
  created:
    - "scratch/spike.zsh (git-ignored, D-08 throwaway): snapshot() + plain apply_*/deact_* loaders + spike_run driver"
    - "scratch/profile_a.json (git-ignored): fixture manifest A (work)"
    - "scratch/profile_b.json (git-ignored): fixture manifest B (personal)"
  modified:
    - ".gitignore: added /scratch/ so throwaway spike artifacts are never committed"

key-decisions:
  - "GO verdict: core classes (env/aliases/functions/PATH) plus options reverse byte-identical — milestone v2.0's single material unknown (SW-03) is retired"
  - "Loaders trust the LIVE prior env value captured at apply time, not the fixture's static `original` field — required for byte-identical reversal regardless of base state"
  - "Snapshot filters its own harness functions (snapshot/apply_*/deact_*/spike_run/zp_*) from ##FUNCTIONS## by name prefix (Pitfall 3 resolution)"

patterns-established:
  - "D-01 snapshot instrument: sorted (@ok) six-class section-delimited dump with bodies"
  - "Two mandatory divergences from the introspectScript analog: plain loaders (no emulate -L) + rebuild-from-base PATH (no typeset -U)"

requirements-completed: [SW-03]

# Metrics
duration: 22min
completed: 2026-06-25
---

# Phase 01 Plan 01: Spike Measurement Instrument + Byte-Identical Round-Trip Summary

**Proved SW-03 GO: a live `zsh -f` session applies declarative state (env/aliases/functions/PATH/options) and reverses it byte-identical across all six classes, via a durable sorted snapshot-diff instrument and two hand-written Manifest-shape fixtures.**

## Performance

- **Duration:** 22 min
- **Started:** 2026-06-25T20:58:00Z
- **Completed:** 2026-06-25T21:05:00Z
- **Tasks:** 2
- **Files modified:** 1 tracked (`.gitignore`); 3 git-ignored scratch artifacts created (D-08)

## Accomplishments
- **Durable `snapshot()` instrument (D-01, D-05):** dumps all six state classes as sorted, section-delimited text — `##ALIASES## ##FUNCTIONS## ##ENV## ##PATH## ##FPATH## ##OPTIONS## ##END##` — with alias AND function bodies (name=body) for shadow-restore verification, env as name+VALUE (`${(P)k}`), PATH as the order-exact scalar, options name-only-when-on. Extends `introspectScript` (core/shell/zsh/introspect.go:23-38) with the three required extensions (sorted `(@ok)`, bodies, the 6th `##FPATH##` class + value exactness).
- **Two hand-written fixture manifests** instantiating the proposed Manifest JSON shape. Together they cover the four required cases: was-unset env (`WORK_TOKEN`, `PERSONAL_KEY` — no `original` key), restore env (`EDITOR` with `original: "vim"`), PATH `lists` delta via `additions` (`/work/bin`, `/personal/bin` — no absolute PATH), and a shadowed alias (`ll`) + shadowed function (`ff`) carrying prior bodies. No `compinit` in any `lists`.
- **Byte-identical round-trip proven (SC1):** `apply_A -> deact_A -> apply_B -> deact_B` under a controlled clean base (`export PATH=/usr/bin:/bin`) yields an EMPTY six-class diff. Verified via both the plan's exact verify command (bare base) and the `spike_run` driver (which seeds the shadowed prior state to exercise Pattern 5).
- **Both mandatory divergences encoded and empirically confirmed:** (1) plain loaders — option `EXTENDED_GLOB` set inside plain `apply_A` is still `on` after the function returns (escaped scope); `emulate -L` appears ONLY in the read-only `snapshot()`. (2) PATH rebuilt from `$ZP_BASE_PATH` equals the base byte-for-byte; no `typeset -U` anywhere.
- **Regression pin untouched:** `go build ./...` and the full `go test ./...` suite (including the `core/testgen` oracle property test) pass unchanged; `scratch/` contains zero Go files and is git-ignored (D-08, threat T-01-02 mitigated).

## Task Commits

1. **Task 1: snapshot() instrument + two fixture manifests** — committable change `4c27c62` (chore: git-ignore scratch/). The snapshot + fixtures themselves are git-ignored throwaway artifacts (D-08), so the only tracked change is the `.gitignore` entry that isolates them.
2. **Task 2: plain apply/deact loaders + byte-identical round-trip** — no new tracked files (all artifacts are intentionally git-ignored per D-08). TDD gate satisfied empirically: RED (loaders absent → no real apply/reverse) → GREEN (loaders added → byte-identical round-trip). The Rule-1 fix below was applied in the same throwaway file.

**Plan metadata:** this SUMMARY commit.

_Note: Because the spike is throwaway (D-08), the conceptually durable outputs — the snapshot section layout and the fixture JSON shape — live in this SUMMARY and the scratch files, not in committed product code._

## Files Created/Modified
- `.gitignore` — added `/scratch/` (throwaway spike isolation; the only tracked change)
- `scratch/spike.zsh` (git-ignored) — `snapshot()` (durable D-01 instrument) + plain `apply_A`/`deact_A`/`apply_B`/`deact_B` loaders + `zp_capture_env`/`zp_restore_env` helpers + `spike_run` driver. 225 lines.
- `scratch/profile_a.json` / `scratch/profile_b.json` (git-ignored) — the validated Manifest-shape fixtures (work / personal profiles).

## Final Snapshot Section Layout

```
##ALIASES##     name=body  (sorted (@ok); bodies for shadow-restore)
##FUNCTIONS##   name=body  (sorted; harness fns snapshot/apply_*/deact_*/spike_run/zp_* filtered out)
##ENV##         name=VALUE (exported only, via ${(P)k}; sorted)
##PATH##        $PATH      (order-exact scalar)
##FPATH##       one path per line (the 6th class; fpath array)
##OPTIONS##     name       (name-only when on; sorted)
##END##
```

## Fixture Coverage (which cases each covers)

| Case | Where | Detail |
|------|-------|--------|
| Core round-trip (env+alias+func+option) | A and B | Both profiles carry env scalars, added aliases, added functions, and one option each |
| PATH `lists` delta via `additions` | A: `/work/bin`; B: `/personal/bin` | Add-delta vs captured base; NO absolute PATH; NO compinit |
| Env was-unset (no `original` key) | A: `WORK_TOKEN`; B: `PERSONAL_KEY` | Deactivate unsets |
| Env restore (`original` present) | A/B: `EDITOR` (`original: "vim"`) | Deactivate restores prior |
| Shadowed alias w/ prior body | A: `ll` (`ls -lh`) | Deactivate restores prior body byte-identical |
| Shadowed function w/ prior body | A: `ff` | Deactivate restores prior body via `functions[ff]=...` |

## Byte-Identical Diff Result (the core round-trip)

`diff S_pre S_post` is **EMPTY** for the full `apply_A; deact_A; apply_B; deact_B` sequence under the controlled clean base — both via the plan's exact verify command and the seeded `spike_run` driver. Verdict printed: `BYTE_IDENTICAL` / `GO: byte-identical`.

## Both Divergences Confirmed In Place

- **DIVERGENCE 1 (no `emulate -L` in loaders):** `grep -E 'emulate -L|typeset -U'` over the extracted `apply_*`/`deact_*` bodies returns nothing; `emulate -L` is present only in `snapshot()` (read-only) and in header comments. Empirical proof: `${options[extendedglob]}` == `on` immediately after plain `apply_A` returns.
- **DIVERGENCE 2 (no `typeset -U`; rebuild from base):** no `typeset -U` in the file; after the round-trip `$PATH == $ZP_BASE_PATH` byte-for-byte (`/usr/bin:/bin`). (Note: `typeset -g` is used for global scoping in the helper functions — this is unrelated to the forbidden `typeset -U` dedup flag.)

## Decisions Made
- **GO verdict for SW-03.** The core classes plus options reverse byte-identical; milestone v2.0's single material de-risk unknown is retired. Completion's `compinit` remains the expected exclusion (imperative → master block) and is deliberately not represented in `lists` — formal completion measurement (fpath-array vs compinit) is Plan 01-02's job.
- **Loaders trust the LIVE prior, not the fixture `original`.** The byte-identical bar requires reversing to whatever the base actually held, so `zp_capture_env` records the live value (or unset sentinel) at apply time; the fixture's static `original` documents the *intended* manifest shape (the deliverable) but is not the reversal source. This is the rigorous form of Patterns 3/4.
- **Harness self-residue filtered by name prefix** (Pitfall 3): `snapshot()` skips `snapshot|apply_*|deact_*|spike_run|zp_*` in the `##FUNCTIONS##` dump, so the spike's own functions never register as function-class residue regardless of definition order.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Env reverse produced set-but-empty residue for was-unset vars; and hardcoded `original` mismatched a bare base**
- **Found during:** Task 2 (byte-identical round-trip)
- **Issue:** Two related defects surfaced by running the plan's own verify command. (a) The first loader draft restored `EDITOR` to a hardcoded `vim`, which is only correct if the base actually held `EDITOR=vim`; under the plan's bare clean base (`EDITOR` unset) this left `EDITOR=vim` as residue. (b) After switching to capture-the-live-prior, `zp_capture_env` tested `[[ -n "${(P)+var}" ]]` — but `${(P)+var}` is `"0"` (non-empty) when unset, so the "set" branch always won and was-unset vars were restored as `export VAR=""` (set-but-empty residue: `WORK_TOKEN=`, `PERSONAL_KEY=`, `EDITOR=`).
- **Fix:** (a) Replaced the static-`original` reverse with `zp_capture_env`/`zp_restore_env` helpers that record the LIVE prior value (or an `__ZP_UNSET__` sentinel) at apply time and drift-guard the restore. (b) Corrected the set-ness test to `[[ "${(P)+var}" == "1" ]]` so unset vars take the sentinel branch and are `unset` on deactivate (honoring Pattern 4 unset-vs-empty exactly).
- **Files modified:** `scratch/spike.zsh` (git-ignored throwaway)
- **Verification:** Both the plan's exact Task-2 verify command (bare base) and the seeded `spike_run` driver now print byte-identical; the residue lines are gone. `go test ./...` still green.
- **Committed in:** N/A — the file is git-ignored per D-08; the fix lives only in the throwaway scratch artifact and is documented here + reproduced in the Plan 01-02 reference snippet.

---

**Total deviations:** 1 auto-fixed (1 bug, Rule 1)
**Impact on plan:** The fix was essential to meet SC1 (byte-identical) and is itself a load-bearing finding for Phase 4: a real emitter must capture the live prior env value, not trust a statically-recorded `original`, and must use `${(P)+var} == 1` (not a `-n` truthiness test) to distinguish unset from empty. No scope creep — all changes confined to the throwaway spike.

## Issues Encountered
- **`zsh -f` inherits the parent env/PATH (Pitfall 5).** Confirmed: the snapshot showed the full inherited exported environment despite `-f`. The round-trip is still byte-identical because the inherited env is identical in S_pre and S_post (it cancels in the diff); the controlled clean base (`export PATH=/usr/bin:/bin`) anchors the PATH/FPATH classes. Resolved by setting the clean base inside the driver before capturing `ZP_BASE_PATH`, exactly as the plan specifies.

## TDD Gate Compliance
This plan's Task 2 (`tdd="true"`) is an empirical shell spike, not Go product TDD. The RED/GREEN gate was satisfied behaviorally (RED: loaders absent, no real apply/reverse; GREEN: loaders added, byte-identical round-trip), but the artifacts are git-ignored throwaway (D-08), so there are no `test(...)`/`feat(...)` git commits to validate. This is expected and consistent with the plan's throwaway-spike framing. The standing `core/testgen` oracle pin remains untouched and green.

## Next Phase Readiness
- **SW-03 retired (GO).** Plan 01-02 reuses this harness for the failure-mode assertions (the residue cases this plan's instrument already detects) and the go/no-go writeup, plus the real-`~/.zshrc` reality-check and the completion (fpath vs compinit) verdict.
- **Manifest shape validated** for the admitted classes — ready to become `model.Manifest` in the Phase 2 IR spine. Phase 4's emitter must carry forward the Rule-1 finding (capture live prior; `${(P)+var} == 1`).
- **No blockers.** scratch/ is git-ignored and will be deleted after the verdict; nothing leaks into product code.

---
*Phase: 01-spike-zero-residue-live-hot-switch*
*Completed: 2026-06-25*
