---
phase: 01-spike-zero-residue-live-hot-switch
verified: 2026-06-25T22:10:00Z
status: passed
score: 4/4 must-haves verified
overrides_applied: 0
---

# Phase 1: SPIKE — Zero-Residue Live Hot-Switch Verification Report

**Phase Goal:** De-risk the frontier before committing to any design. Prove that a live, already-open terminal can apply a profile's declarative state and reverse it with zero residue — reversing aliases, functions, and **options**, not just env — using a hand-written manifest and a hard-coded two-profile fixture. The output is a go/no-go decision and the validated shape of the `Manifest`.

**Verified:** 2026-06-25T22:10:00Z
**Status:** passed
**Re-verification:** No — initial verification

> Verification approach: this is a THROWAWAY de-risk SPIKE (D-08) that ships NO production `core/` source by design. The empirical proof lives in git-ignored throwaway code (`scratch/spike.zsh`, `scratch/spike_test.go`). Every PASS below was produced by **the verifier running the commands in its own process** — not by trusting SUMMARY.md PASS claims. zsh 5.9 (arm64-apple-darwin25.0) confirmed present.

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | In an already-open terminal, `activate A → activate B (auto-deactivates A) → deactivate B` leaves `$aliases`, `$functions`, `$PATH`, `$path`, exported env, and `$options` byte-identical to the pre-activation snapshot | ✓ VERIFIED | Plan's exact bare-base verify command printed `BYTE_IDENTICAL` (empty six-class diff). `go test -tags spike ./scratch/...` → `TestSpikeByteIdenticalRoundTrip PASS`. Direct `spike_all` → `FM1_PASS: no-op round-trip is byte-identical (empty diff)`. Snapshot emits all six markers: `##ALIASES## ##FUNCTIONS## ##ENV## ##PATH## ##FPATH## ##OPTIONS## ##END##`. |
| 2 | Repeated switch cycles do not grow `$PATH` (no duplicate/accumulated entries), and no alias/function/option from a prior profile survives a switch | ✓ VERIFIED | Plan 01-02 FM1+FM2 combined command (5 cycles) printed `FM12_PASS` (`$#path` pre==post). Direct `spike_all` → `FM2_PASS: no accumulation after 5 cycles ($#path stable at 2; no surviving alias/function/option)` — includes an explicit absence grep for `gs/ga/gp/work_deploy/personal_sync/extendedglob/nocaseglob`. `TestSpikeFailureModes PASS`. |
| 3 | The drift guard holds: when the user changes a managed env var by hand mid-session, deactivate does NOT clobber that change | ✓ VERIFIED | Plan's literal `A_VAR` sentinel command printed `DRIFT_PASS`. Direct `spike_all` proves BOTH directions on a MANAGED var (`EDITOR`): `FM3a_PASS` (hand edit survived deactivate) AND `FM3b_PASS` (untouched value reversed to prior `vim`) → `FM3_PASS: drift guard is conditional`. The guard is conditional, not a blanket skip — exactly the D-04 carve-out. |
| 4 | A written go/no-go decision records which zsh state classes are not cleanly reversible (narrowing the managed set), and the hand-written manifest JSON shape is captured as Phase 4's input | ✓ VERIFIED | `01-FINDINGS.md` (172 lines): overall **GO** verdict, 6-row per-class ADMITTED/EXCLUDED table with diff evidence, completion fpath-vs-compinit nuance, bindkey(129)/precmd_functions(6) presence note. `01-MANIFEST-SHAPE.md` (112 lines): validated `Manifest` JSON shape with all five class fields (env/lists/aliases/functions/options) + per-field reverse-op justification + the live-prior-vs-static-`original` carry-forward. |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `scratch/spike.zsh` | snapshot() six-class instrument + plain apply_*/deact_* loaders + drivers (FM1/FM2/FM3 + reality-check) | ✓ VERIFIED | 453 lines. `snapshot()` is the sole `emulate -L` site (read-only); loaders are plain. Sources cleanly under `zsh -f`. Git-ignored (D-08). |
| `scratch/profile_a.json` | Fixture A (work) instantiating the Manifest shape | ✓ VERIFIED | Valid JSON. Covers EDITOR (restore, `original:"vim"`), WORK_TOKEN (was-unset, no `original`), PATH `additions:["/work/bin"]`, shadowed alias `ll`, shadowed function `ff`, option EXTENDED_GLOB. |
| `scratch/profile_b.json` | Fixture B (personal), distinct switch target | ✓ VERIFIED | Valid JSON. Distinct env/PATH/alias/function/option set (EDITOR=code, PERSONAL_KEY, /personal/bin, ll/gp, personal_sync, NO_CASE_GLOB). |
| `scratch/spike_test.go` | Build-tag-isolated Go verdict pin | ✓ VERIFIED | First line `//go:build spike`. Invisible to `go test ./...` ("matched no packages" without tag). Runs green under `-tags spike`. Uses introspect.go's exec.CommandContext + 5s timeout + LookPath skip-guard. |
| `.planning/.../01-FINDINGS.md` | Go/no-go writeup (durable D-08 deliverable 1) | ✓ VERIFIED | 172 lines (≥40). GO verdict, six-class table, compinit nuance, bindkey/hook note, embedded loader reference snippet. |
| `.planning/.../01-MANIFEST-SHAPE.md` | Validated Manifest shape (durable D-08 deliverable 2) | ✓ VERIFIED | 112 lines (≥30). Full shape + per-field justification + intentionally-excluded-classes table. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `spike.zsh snapshot()` | `zmodload zsh/parameter` assoc arrays | sorted `(@ok)` section-delimited dump | ✓ WIRED | `zmodload zsh/parameter` at line 37; six markers confirmed emitted at runtime. |
| `spike.zsh apply_*/deact_*` | `$ZP_BASE_PATH` | plain array rebuild on apply, `PATH=$ZP_BASE_PATH` on deactivate | ✓ WIRED | Lines 152/171/190/202 rebuild from base. No executable `typeset -U` anywhere. |
| `spike_test.go` | `zsh -f -c <script>` | `exec.CommandContext` + 5s timeout + LookPath guard | ✓ WIRED | Both tests drive zsh and PASS. |
| `01-FINDINGS.md` | the six measured classes | per-class admit/exclude verdict | ✓ WIRED | 6-row table; `compinit` excluded, fpath admitted. |

### Mandatory Divergence Verification (D-05 correctness gates)

| Divergence | Requirement | Status | Evidence |
|-----------|-------------|--------|----------|
| DIVERGENCE 1 | NO `emulate -L`/`LOCAL_OPTIONS` in apply_*/deact_* (options must escape function scope) | ✓ VERIFIED | awk-scoped scan of loader bodies (apply_A 144-164, deact_A 166-182, apply_B 185-197, deact_B 199-208) found ZERO matches. `emulate -L` only in `snapshot()` (line 36) + comments. |
| DIVERGENCE 2 | NO `typeset -U` (rebuild PATH from captured base) | ✓ VERIFIED | No `typeset -U` in any executable line (the two grep hits at lines 87/270 are comment prose). PATH rebuilt from `$ZP_BASE_PATH`. |

### D-08 Isolation Invariant (regression safety)

| Check | Status | Evidence |
|-------|--------|----------|
| Default `go test ./...` green | ✓ VERIFIED | All packages `ok`, exit 0. |
| `core/testgen` oracle property test untouched | ✓ VERIFIED | Explicit `TestOracleProperty` run: 10 seeds (1..89) all PASS, exit 0. |
| Spike pin invisible without `-tags spike` | ✓ VERIFIED | `go test -list ./scratch/...` → "matched no packages". |
| `go build ./...` clean | ✓ VERIFIED | Exit 0. |
| `scratch/` git-ignored & untracked | ✓ VERIFIED | `git check-ignore` matches all 4 files; `git status --porcelain scratch/` empty; `.gitignore:5 /scratch/`. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| SC1 byte-identical round-trip (bare base) | `zsh -f -c '... apply_A;deact_A;apply_B;deact_B; diff ...'` | `BYTE_IDENTICAL` | ✓ PASS |
| SC2 no-accumulation over 5 cycles | Plan 01-02 FM1+FM2 verify command | `FM12_PASS` | ✓ PASS |
| SC3 drift guard | Plan's `A_VAR` sentinel command | `DRIFT_PASS` | ✓ PASS |
| Full failure-mode + reality-check driver | `zsh -f -c 'source scratch/spike.zsh; spike_all'` | FM1/FM2/FM3a/FM3b/FM3 + REALZSHRC all PASS, exit 0 | ✓ PASS |
| Snapshot six markers | `zsh -f -c 'source scratch/spike.zsh; snapshot'` | all 7 section markers present | ✓ PASS |

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| Spike verdict pin (build-tagged Go) | `GOTOOLCHAIN=auto go test -tags spike ./scratch/... -count=1 -v` | `TestSpikeByteIdenticalRoundTrip PASS`, `TestSpikeFailureModes PASS`, exit 0 | PASS |

> No conventional `scripts/*/tests/probe-*.sh` exists in this repo; the phase-declared verification driver is the build-tagged Go pin, which the verifier ran directly (exit 0).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SW-03 | 01-01, 01-02 | (Frontier) Switching works live in an already-open terminal — byte-identical after activate→switch→switch-back (env AND aliases/functions/options) | ✓ SATISFIED | All four SCs verified empirically; REQUIREMENTS.md line 30 is `[x]`, traceability table line 74 marks it `Complete`. No orphaned requirements for this phase. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | None | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any phase-modified file (spike.zsh, spike_test.go, 01-FINDINGS.md, 01-MANIFEST-SHAPE.md). |

### Human Verification Required

None. Every success criterion is a programmatically observable byte-identity/diff assertion, all of which the verifier executed directly under sandboxed `zsh -f`. No visual/UX/real-time/external-service surface in this spike.

### Gaps Summary

No gaps. All four ROADMAP success criteria are VERIFIED by independent command execution (not SUMMARY trust):

- **SC1** (byte-identical six-class round-trip): proven via the plan's bare-base command, the build-tagged Go pin, and `FM1_PASS`.
- **SC2** (no PATH accumulation, no surviving prior-profile state): proven via `FM12_PASS` and `FM2_PASS` with an explicit per-name absence check.
- **SC3** (conditional drift guard): proven in BOTH directions (`FM3a_PASS` hand-edit survives, `FM3b_PASS` untouched reverses).
- **SC4** (go/no-go decision + validated Manifest shape): both durable committed docs are present, substantive, and content-gated (GO verdict, six-class table, completion nuance, bindkey/hook note, full manifest field set with per-field justification).

Both mandatory D-05 divergences are encoded and verified (plain loaders; rebuild-from-base PATH). The D-08 isolation invariant holds: default suite green, the `core/testgen` oracle property pin untouched (10 seeds PASS), the spike invisible without `-tags spike`, and all `scratch/` artifacts git-ignored and untracked. The deliberate absence of `core/` source is correct for a throwaway spike, not a gap.

**Recorded carry-forwards (NOT gaps):** (1) the loader trusts the LIVE prior env value, not the fixture's static `original` (Rule-1 finding); (2) the reality-check measures the declared-name VALUE delta rather than a live env name-set diff under `zsh -f` env inheritance (Pitfall 5, documented Rule-1 deviation). Both are explicitly captured in 01-FINDINGS.md / 01-MANIFEST-SHAPE.md as Phase-4 inputs for `core/shell/zsh/emit.go`.

---

_Verified: 2026-06-25T22:10:00Z_
_Verifier: Claude (gsd-verifier)_
