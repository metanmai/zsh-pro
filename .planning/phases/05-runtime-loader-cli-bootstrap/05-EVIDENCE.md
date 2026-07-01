---
phase: 05-runtime-loader-cli-bootstrap
updated: 2026-07-01T20:02:52Z
proven: 14
static_validated: 0
refuted: 1
unverifiable: 1
unverified: 0
---

# Claims Ledger — Phase 5 Runtime Loader + CLI + Bootstrap

Record of every falsifiable technical claim in this phase's design docs (05-SPEC / 05-CONTEXT /
05-RESEARCH) and how it was validated. POCs backing PROVEN/REFUTED verdicts are throwaway (isolated
scratch under `/tmp/gsd-docs-poc/05/`, OUTSIDE the repo) and are NOT committed — this file is the
durable evidence (snippet, command, observed output). Production `.go`/`.zsh`/`.zshrc` files were
never edited (READ only).

Claim-validation **pass 1 (2026-07-01)** extracted 16 load-bearing claims (C1..C16) and INDEPENDENTLY
POC-proved each under `zsh 5.9 (arm64-apple-darwin25.0)` / `go 1.25.7` — the research doc's
self-reported E1..E16 verifications were **re-run from scratch, not trusted**. The load-bearing claims
(fail-open guards, idempotent writer, current-shell mutation, zero-subprocess, `zp_*` contract) were
adversarially probed. Verdict: **14 PROVEN, 1 REFUTED, 1 UNVERIFIABLE**.

The one REFUTED claim (**C16**) is a documentation/contract mismatch, NOT a broken zsh semantic: Phase 5's
CONTEXT/RESEARCH assert the loader "defines exactly the helper set Phase 4's `emit.go` emits **bare calls**
to" and enumerate `zp_capture_env`/`zp_restore_env`/`zp_rebuild_path`/shadow-capture/shadow-restore as that
set — but Phase 4's OWN evidence (04-EVIDENCE / 04-CONTEXT D-12/OQ-5) shows (a) `emit.go` does not exist on
disk yet, (b) the helper-vs-inline decision is **Claude's discretion (Phase 4 OQ-5), not locked**, and (c)
Phase 4 only commits to `zp_capture_env`/`zp_restore_env` as named helpers — shadows, PATH-rebuild, and
options are described as INLINE reverse ops (`RestoreShadowedAlias/Func`, `PATH="$ZP_BASE_PATH"; path=(…)`,
`SetOption`/`RestoreOption` with an inline `[[ -o opt ]]`→`was_on` capture), NOT a `zp_rebuild_path`/
`zp_shadow_*` helper set. See `## Details` and the RETURN block at the bottom.

The one UNVERIFIABLE claim (**C15**) is the `hyperfine 'zsh -i -c exit'` < 10 ms startup budget — `hyperfine`
is ABSENT on this machine (a dev/CI tool, not a Go dep). The load-bearing structural guarantee (zero
subprocess on the start path, **C14**) IS proven; the millisecond budget was measured only via an
in-process `EPOCHREALTIME` proxy (~0.01 ms added, ~900× under budget), which is a proxy, not the stated gate.

| id | claim | source | status | evidence |
|----|-------|--------|--------|----------|
| C1 | A function/alias/env defined in an `eval`'d string persists in the CURRENT interactive shell (not a subshell); the same body run inside `$(...)` does NOT leak to the parent | 05-RESEARCH E1 / sub-area (a) | PROVEN | `zsh -f C1/poc.zsh`: `eval 'foo(){…}'; eval "alias myal=…"; eval 'export MYENV=…'` → `foo` is a live function, `${aliases[myal]}`=`echo aliasbody`, `$MYENV`=`envval`. Contrast: `bar` defined inside `$(…)` → `NOTDEFINED` in parent; `SUBONLY` → `UNSET`. Persistence holds only for `eval`, not `$(…)`. |
| C2 | `$(cmd)` runs in a subshell — its `cd`/`export` do NOT affect the parent; `eval` of the same body DOES change the current shell | 05-RESEARCH E2 / sub-area (b) spine | PROVEN | `zsh -f C2/poc.zsh`: `X=$(cd /etc; export SUBVAR=…)` → parent `PWD` unchanged, `SUBVAR=UNSET`. `eval "cd /etc; export EVALVAR=…"` → parent `PWD=/etc`, `EVALVAR=fromeval`. This is the entire reason the mutating verbs are shell functions, not the bare binary. |
| C3 | A sourced shell function that runs `eval "$(<binary>)"` mutates the CURRENT terminal (alias + env + PATH + `ZSHPRO_PROFILE` become live in the parent) | 05-SPEC Req 2 / 05-CONTEXT D-05 / E4 | PROVEN | `zsh -f C3/poc.zsh`: sourced `activate()` runs `eval "$(fakebin)"` where fakebin prints `alias gs='git status'; export EMITTED_VAR=applied; path=(/profileA/bin $path)`, then `export ZSHPRO_PROFILE=$1` → in the parent: `${aliases[gs]}`=`git status`, `EMITTED_VAR=applied`, `path[1]=/profileA/bin`, `ZSHPRO_PROFILE=A`. |
| C4 | Full `activate A → activate B → deactivate` through the eval-of-emitted-code path with the `zp_*` helpers + base guard is zero-residue; `$#path` does not grow across switches | 05-SPEC Req 2 / 05-RESEARCH E15 / Phase 4 zero-residue property | PROVEN | `zsh -f C4/poc.zsh`: PRE `#path=2 EDITOR=vim gs=[echo none]` → A `#path=3 EDITOR=nvim gs=[git status]` → B `#path=3 EDITOR=emacs gs=[echo none]` → POST `#path=2 EDITOR=vim gs=[echo none]` == PRE exactly. `zp_capture_env`/`zp_restore_env` (drift-guarded), `${slot+x}` shadow restore, rebuild-from-`ZP_BASE_PATH`. `$#path` stable at 2. |
| C5 | `ZP_BASE_PATH` is captured ONCE at first activate per terminal; the re-capture guard `[[ "${ZP_BASE_PATH+x}" == "x" ]] \|\| typeset -g ZP_BASE_PATH="$PATH"` prevents folding a prior profile's additions into the base | 05-SPEC Req 2 / 05-CONTEXT D-08 / E8 | PROVEN | `zsh -f C5/poc.zsh`: base `/usr/bin:/bin`; 1st capture → `ZP_BASE_PATH=/usr/bin:/bin`; after A pollutes PATH, 2nd capture → STILL `/usr/bin:/bin` (guard blocked re-capture); after B rebuild, 3rd capture → STILL `/usr/bin:/bin`; deactivate rebuild-to-base → `#path=2` == base. |
| C6 | `[[ "${(P)+var}" == "1" ]]` distinguishes set(incl. empty) from unset when the operand `var` HOLDS the env NAME (`(P)` dereferences value-as-name) — the correct `zp_capture_env`/`zp_restore_env` env guard | 05-CONTEXT D-01 / 05-RESEARCH E5 / Don't-Hand-Roll | PROVEN | `zsh -f C6/poc.zsh` (operand holds NAME, as in the emitted helper call): `EDITOR=vim`→`set` value `[vim]`; `EMPTYONE=""`→`set` value `[]`; `UNSETONE` unset→`unset`. Distinguishes all three states. (This is the C21-confirmed indirect form; distinct from the C1-refuted `${(P)+literalName}` in the Phase 4 ledger.) |
| C7 | Shadow-restore must guard with `[[ -n "${slot+x}" ]]` (set-test), NOT `[[ -n "$slot" ]]` — the `-n` form silently DROPS a valid empty prior body, leaving the shadow as residue | 05-CONTEXT D-01 (OQ-8) / 05-RESEARCH E6 / Pitfall 3 | PROVEN | `zsh -f C7/poc.zsh`: prior alias body is `""`. Buggy `[[ -n "$slot" ]]` → restore SKIPPED, alias left as `echo shadowed` (residue). Correct `${slot+x}` set-test → alias restored to empty body (`set?=x body=[]`). The `-n` form is verified WRONG for an empty prior. |
| C8 | Slot names must be sanitized to `[A-Za-z0-9_]` (`${name//[^A-Za-z0-9_]/_}`): a raw `/` is a hard `typeset` failure AND a raw `$(...)` in a derived name EXECUTES at name-build time (injection) | 05-CONTEXT D-02 / 05-RESEARCH E12/E16 / Security V5 | PROVEN | Isolated `zsh -f -c` subprocesses: (1) `typeset -g "__ZP_ORIG_feature/x"=v` → `zsh:typeset: not valid in this context` rc=1 (fatal). (2) `${(e)}` of a name containing `$(echo PWNED > canary)` FIRED the canary. (3) `${n//[^A-Za-z0-9_]/_}` maps `feature/x`→`feature_x`, `a b`→`a_b`, `ver.1`→`ver_1`, `x$(…)y`→`x__echo_…_y`, all valid `typeset` targets, no injection. |
| C9 | Idempotent BEGIN/END `.zshrc` writer (Go, `strings.Index` find-region-then-replace + atomic rename): byte-identical on re-run, `grep -c` BEGIN == 1, content outside markers preserved byte-for-byte, dup blocks collapse, missing file created | 05-SPEC Req 4 / 05-CONTEXT D-10 / 05-RESEARCH E-INSTALL | PROVEN | Go POC (`go build -C`, stdlib only, module `poc` — NEVER imports zsh-pro). Scenario 1: 2 installs → `diff` empty, BEGIN count 1, user `export MY_VAR=1`/`alias myls` survive. Scenario 2: 2 dup blocks + between-content → collapses to 1, `USER_BETWEEN` preserved. Scenario 3: missing file → created with 1 block. Adversarial (adv_test.sh): content DIRECTLY adjacent to markers preserved + stale body gone + re-run byte-identical; mid-file block keeps header+tail; 3 scattered dups collapse to 1 with all U1-U4 kept; no-trailing-newline file appended cleanly; installed stub is `zsh -n` clean. All PASS. |
| C10 | `command -v <binary>` / `[[ -r <path> ]]` guards are a silent no-op when the binary/loader is absent/unreadable; the shell reaches an interactive prompt with no shell-aborting non-zero exit | 05-SPEC Req 5 / 05-CONTEXT D-14 / 05-RESEARCH E11 | PROVEN | `zsh -f -c 'source C10/stub.zsh; …'`: `command -v zsh-pro-DEFINITELY-ABSENT` no-op; `[[ -r "$HOME/.zsh-pro/NOSUCH_loader.zsh" ]] && source …` skipped; script prints `REACHED_PROMPT_EQUIVALENT` and `source` returns exit 0. No abort, no error. |
| C11 | `zsh -n` rejects a syntax error with a non-zero exit WITHOUT executing the file/string; works on a file operand AND a here-string (`zsh -n <<< "$code"`) | 05-SPEC Req 1/6 / 05-CONTEXT D-15 / 05-RESEARCH E3/E3b | PROVEN | `zsh -n C11/bad.zsh` (unterminated quote, `print SHOULD_NOT_RUN`) → rc=1, `unmatched '`, and `SHOULD_NOT_RUN` NEVER appeared in output. `zsh -n C11/good.zsh` → rc=0. Here-string: bad → rc=1, good → rc=0. The verb can validate a captured string in-place. |
| C12 | `ZSHPRO_DISABLE=1` is a full early-return no-op: the stub defines NO verbs and performs no activation; unset → verbs defined | 05-SPEC Req 5 / 05-CONTEXT D-14 / 05-RESEARCH E10 | PROVEN | Stub first line `[[ -n "$ZSHPRO_DISABLE" ]] && return 0`. `ZSHPRO_DISABLE=1 zsh -f -c 'source stub; whence -w activate'` → `activate defined? NO`. Unset → `activate defined? activate: function`. Full no-op confirmed. |
| C13 | `zsh -n`-validated manifests with atomic last-good: validate the ENTIRE emitted block up front; a switch whose code fails `zsh -n` is refused BEFORE any `eval` (no half-apply), the live shell keeps the prior good state, and `ZP_LAST_GOOD_PROFILE` stays intact | 05-SPEC Req 6 / 05-CONTEXT D-15/D-16 / 05-RESEARCH E3 | PROVEN | `zsh -f C13/poc.zsh`: good switch to A → `EDITOR=nvim gs=[git status]`, last-good=A. Bad switch to B (valid first half + unterminated quote) → `safe_switch` runs `zsh -n <<< "$code"` first, it fails, `eval` is NOT reached → shell STILL `EDITOR=nvim gs=[git status]` (A's good state, deactivate-half NOT applied), `ZP_LAST_GOOD_PROFILE`=A. Refuse-all-on-failure is atomic. |
| C14 | The shell-START hot path (stub + the cached loader's DEFINITION lines) contains no `$(...)`, no `git`, and no `zsh-pro` command; verb-function BODIES carry `$(zsh-pro emit …)` but those run only on explicit invocation | 05-SPEC Req 7 / 05-CONTEXT D-13 / 05-RESEARCH E13 | PROVEN | Structural grep: stub has `$(`×0, backtick×0, `git `×0, `zsh-pro`-in-command-position×0 (appears only in the marker comment). Loader start-path lines (verb bodies excluded via awk) have `$(`×0, `git `×0, `zsh-pro`×0. Sanity: the 5 verb BODIES DO carry `zsh-pro emit`×3 — those fire only on explicit `activate`/`checkout`/etc. `$'\x1f…'` (a quote form) is correctly NOT counted as `$(` command-sub. |
| C15 | The added startup cost of sourcing the loader is within the < 10 ms budget, verified via `hyperfine 'zsh -i -c exit'` with the block installed vs not | 05-SPEC Req 7 / 05-CONTEXT D-17 / OQ-05-04 | UNVERIFIABLE | `hyperfine` is ABSENT on this machine (`which hyperfine` → not found; a dev/CI tool, not a Go dep). The STATED gate (`hyperfine 'zsh -i -c exit'`) cannot be run here. In-process `EPOCHREALTIME` PROXY over N=2000: baseline (source empty) 0.054 ms/iter, source pure-def loader 0.064 ms/iter → **~0.01 ms added** (~900× under 10 ms). This is a proxy, NOT the `hyperfine` interactive-startup gate. The load-bearing guarantee (C14 zero-subprocess) IS proven; the ms budget needs `hyperfine` installed in CI to close. |
| C16 | The loader's `zp_*` helper signatures match Phase 4's emitted **bare calls** — the loader "defines exactly the helper set Phase 4's `emit.go` emits bare calls to" (`zp_capture_env`, `zp_restore_env`, `zp_rebuild_path`/PATH-rebuild, shadow-capture, shadow-restore) | 05-SPEC Req 1 / 05-CONTEXT D-01/D-20 / 05-RESEARCH sub-area (a) helper table | REFUTED | (1) `core/shell/zsh/emit.go` and `core/activate` do NOT exist on disk (`ls` → No such file); no `zp_*` bare call is emitted anywhere in `core/` yet. So the claim cannot be verified against real code — matching 05-RESEARCH's own OQ-05-05 caveat. (2) Reconciled against Phase 4's OWN evidence (04-EVIDENCE / 04-CONTEXT D-12/OQ-5): Phase 4 leaves helper-vs-inline as **Claude's discretion (OQ-5), NOT locked** ("Whether emit.go emits calls to shared `zp_*` helpers or inlines them per-op is Claude's discretion"). Phase 4 commits to `zp_capture_env`/`zp_restore_env` as named helpers ONLY; shadows (`RestoreShadowedAlias`/`RestoreShadowedFunc` → `alias name=$prior`/`functions[name]=$prior`), PATH (`PATH="$ZP_BASE_PATH"; path=(<additions> $path)`), and options (`SetOption`/`RestoreOption` with inline `[[ -o opt ]]`→`was_on`) are described as INLINE ops, NOT a `zp_rebuild_path`/`zp_shadow_*` helper set. Phase 5 CONTEXT line 32 explicitly names a `zp_rebuild_path` helper that Phase 4 never commits to. The claim's word "exactly … bare calls to [a helper set]" overstates a NOT-YET-LOCKED, largely-INLINE Phase 4 contract. See `## Details` + RETURN. |

## Details

Only REFUTED and UNVERIFIABLE claims are detailed below (PROVEN claims are locked as-stated per the table).

### C16 — `zp_*` helper contract reconciliation (REFUTED — contract mismatch, not a broken semantic)

- **Claim (as stated in 05 docs):** The loader "defines exactly the helper set Phase 4's `emit.go` emits
  bare calls to" — enumerated across 05-SPEC Req 1, 05-CONTEXT D-01, and the 05-RESEARCH sub-area (a)
  helper table as `zp_capture_env`, `zp_restore_env`, a PATH-rebuild helper (05-CONTEXT line 32 names it
  `zp_rebuild_path`), shadow-capture, and shadow-restore for aliases + functions.

- **What actually happened (verified 2026-07-01):**
  1. **`emit.go` is not on disk.** `ls core/shell/zsh/emit.go core/activate` → `No such file or directory`;
     `grep -rn 'zp_capture_env\|zp_restore_env\|zp_rebuild_path\|zp_base' core/` → nothing. There is no
     literal Phase-4 bare-call surface to match against yet. (05-RESEARCH already flags this as OQ-05-05:
     "Phase 4 is documented but not yet on disk … diff the loader's defined helpers against emit.go's
     literal bare calls at plan/execute time." The SPEC/CONTEXT wording ("defines EXACTLY the helper set …
     emits bare calls to") reads as a settled fact rather than a pending gate.)
  2. **Phase 4 does NOT lock a `zp_*` helper set — it is Claude's discretion (Phase 4 OQ-5).**
     04-CONTEXT D-12 (line 50): "Whether `emit.go` emits calls to shared `zp_*` helpers or inlines them
     per-op is **Claude's discretion** (logged as OQ-5; safe default = emit calls to a small fixed helper
     set …)." 04-CONTEXT line 64 lists only "`zp_capture_env`/`zp_restore_env`/**etc.**" — no committed
     names for PATH/shadow/option ops.
  3. **Phase 4's committed emitted vocabulary is mostly INLINE, not a `zp_*` helper set.** From 04-CONTEXT
     D-08 (op vocabulary) and D-12, and 04-EVIDENCE C6/C21/C27/C32: env is `zp_capture_env`/`zp_restore_env`
     (named); shadows are `RestoreShadowedAlias`/`RestoreShadowedFunc` → `alias name=$prior` /
     `functions[name]=$prior` guarded by `${+name}` (inline reverse ops); PATH is
     `PATH="$ZP_BASE_PATH"; path=(<additions> $path)` (inline); options are `SetOption`/`RestoreOption`
     with an inline `[[ -o opt ]]`→`was_on` capture (04-EVIDENCE C27, inline). There is exactly ONE
     `zp_`-named helper reference in 04-EVIDENCE (line 147, `zp_restore_env`, C21). A `zp_rebuild_path`
     helper (named in 05-CONTEXT line 32) appears NOWHERE in Phase 4's docs.

- **Correct fact:** The loader must provide the runtime bodies for whatever call surface Phase 4's `emit.go`
  actually emits, and that surface is (a) not yet on disk and (b) not locked to a fixed `zp_*` helper set —
  Phase 4's default is "emit calls to a small fixed helper set" but it commits by name only to
  `zp_capture_env`/`zp_restore_env`; shadow/PATH/option reverse logic is documented as INLINE. Phase 5 must
  therefore (1) treat the helper set as a plan-time reconciliation gate (OQ-05-05), NOT a settled "exactly
  matches" fact, and (2) either define ONLY the helpers Phase 4 chooses to emit as bare calls, or — if Phase 4
  inlines — provide only `ZP_BASE_PATH`/`ZP_UNSET_SENTINEL`/`__ZP_ORIG_*`/`ZP_*_PRIOR_*`/`was_on` runtime
  STATE (the slots the inline code reads/writes) plus `zp_capture_env`/`zp_restore_env`, and drop the
  assumption that `zp_rebuild_path`/`zp_shadow_capture`/`zp_shadow_restore` are part of the contract.

- **Action (for the owning docs — 05-SPEC Req 1, 05-CONTEXT D-01/line 32, 05-RESEARCH sub-area (a) table):**
  Reword the helper-contract claim from a settled "defines EXACTLY the helper set Phase 4's emit.go emits
  bare calls to" to a **plan-time reconciliation gate**: "Phase 4's emit-vs-inline shape is Claude's
  discretion (Phase 4 OQ-5) and `emit.go` is not yet on disk; at plan/execute time, diff the loader's defined
  helpers/state against `emit.go`'s ACTUAL bare-call surface and provide exactly those. Phase 4 commits by
  name only to `zp_capture_env`/`zp_restore_env` (env); shadows/PATH/options may be emitted INLINE (reading
  the `ZP_BASE_PATH`/`__ZP_ORIG_*`/`ZP_*_PRIOR_*`/`was_on` runtime STATE the loader provides), so a
  `zp_rebuild_path`/`zp_shadow_*` helper is NOT guaranteed to be in the contract." Remove or conditionalize
  the `zp_rebuild_path` name in 05-CONTEXT line 32 (Phase 4 never commits to it).

- **Note:** This is a DOC-CONSISTENCY refutation. Every underlying zsh SEMANTIC the helpers rely on is
  independently PROVEN here (C4/C5/C6/C7/C8) and in Phase 4's ledger (C9/C21/C24/C27/C32). The loader's
  *runtime state contract* (`ZP_BASE_PATH` once-captured, `__ZP_ORIG_*`/`ZP_*_PRIOR_*` slots, sanitized slot
  names, `${(P)+var}` env guard, `${slot+x}` shadow guard) is correct and consistent with Phase 4. Only the
  *shape* claim — "a fixed `zp_*` helper set that emit.go emits bare calls to, matched exactly" — overstates
  a not-yet-locked, largely-inline Phase 4 contract. It does not block planning; it must be reworded to a gate
  so the executor reconciles against real `emit.go` rather than assuming a helper set Phase 4 may not emit.

### C15 — `hyperfine` startup budget (UNVERIFIABLE — tool absent)

- **Claim:** The added interactive-startup cost is within the < 10 ms budget, verified via
  `hyperfine 'zsh -i -c exit'` with the managed block installed vs not (05-SPEC Req 7, 05-CONTEXT D-17,
  OQ-05-04).
- **Why unverifiable:** `hyperfine` is not installed (`which hyperfine` → not found). It is a dev/CI tool,
  not a Go module (confirmed by 05-RESEARCH Environment Availability table and the SPEC "no new deps"
  constraint). The STATED gate — `hyperfine 'zsh -i -c exit'` comparing ZDOTDIR-with-block vs
  ZDOTDIR-without-block — requires the tool and cannot be run in this environment.
- **Proxy measured (NOT the gate):** In-process `zmodload zsh/datetime` + `EPOCHREALTIME` over N=2000
  source-iterations: baseline (source empty file) 0.054 ms/iter; source the pure-function-def loader
  0.064 ms/iter → **~0.01 ms added per shell-start**, ~900× under the 10 ms budget. Pure definitions are
  effectively free, consistent with 05-RESEARCH E14's ~0.03 ms proxy.
- **Action:** Keep the < 10 ms budget as the SPEC criterion but record that the `hyperfine` gate is a
  CI-machine concern (install `hyperfine` via `brew`); the load-bearing guarantee is the C14 zero-subprocess
  structural grep, with the EPOCHREALTIME proxy as an interim backstop. No doc correction needed — 05-RESEARCH
  already documents the tool as absent with a proxy fallback; this ledger records the budget as UNVERIFIABLE
  here so planning does not treat the ms number as confirmed on this machine.

## Status meanings
- **PROVEN** — a POC/smoke test was run and confirmed the claim. Locked.
- **STATIC-VALIDATED** — confirmed without running (type-check / source inspection / dry compile). Locked.
- **REFUTED** — claim is false/overstated; owning doc's needed correction recorded in the row's evidence +
  the `## Details` Action + the RETURN block.
- **UNVERIFIABLE** — could not be checked here (needs an absent tool/hardware); assumed-but-flagged.
- **UNVERIFIED** — extracted but not yet validated. NONE remaining.

## RETURN — corrections the orchestrator must apply before planning

**1 REFUTED claim requires a doc correction (C16). 1 UNVERIFIABLE (C15) is a CI-tool gap, no doc edit.**

- **C16 (REFUTED) — owning docs: 05-SPEC Req 1, 05-CONTEXT D-01 + line 32, 05-RESEARCH sub-area (a) helper
  table.** The claim "the loader defines EXACTLY the helper set Phase 4's `emit.go` emits **bare calls** to
  (`zp_capture_env`, `zp_restore_env`, `zp_rebuild_path`, shadow-capture, shadow-restore)" is overstated:
  (a) `emit.go`/`core/activate` do NOT exist on disk yet, and (b) Phase 4 leaves emit-vs-inline as **Claude's
  discretion (Phase 4 OQ-5)** — it commits by NAME only to `zp_capture_env`/`zp_restore_env`; shadows, PATH
  (`PATH="$ZP_BASE_PATH"; path=(<additions> $path)`), and options (`SetOption`/`RestoreOption` with inline
  `[[ -o opt ]]`→`was_on`) are documented INLINE, and a `zp_rebuild_path` helper (05-CONTEXT line 32) appears
  NOWHERE in Phase 4's docs. **Correction:** reword the helper-contract claim from a settled "matches exactly"
  fact to a **plan-time reconciliation gate** (OQ-05-05): diff the loader's helpers/state against `emit.go`'s
  ACTUAL bare-call surface once Phase 4 lands; provide the two named env helpers + the runtime STATE
  (`ZP_BASE_PATH`, `ZP_UNSET_SENTINEL`, `__ZP_ORIG_*`, `ZP_*_PRIOR_*`, `was_on`) the inline reverse ops read;
  do NOT assume `zp_rebuild_path`/`zp_shadow_*` are in the contract. This is a doc-consistency fix — every
  underlying zsh semantic is PROVEN (C4-C8) and does not block planning.

- **C15 (UNVERIFIABLE) — no doc edit.** `hyperfine` is absent; the < 10 ms budget cannot be gated here. The
  load-bearing zero-subprocess guarantee (C14) is PROVEN; the EPOCHREALTIME proxy shows ~0.01 ms added.
  Record the `hyperfine` budget as a CI-machine gate (install via `brew`), not a claim confirmed on this box.

---

*Phase: 05-runtime-loader-cli-bootstrap*
*Claim-validation pass 1: 2026-07-01 — 14 PROVEN, 1 REFUTED (C16), 1 UNVERIFIABLE (C15) of 16.*
*Environment: zsh 5.9 (arm64-apple-darwin25.0) / go 1.25.7 / hyperfine ABSENT.*
*POCs: throwaway under /tmp/gsd-docs-poc/05/ (outside the repo, not committed). No production file edited.*
