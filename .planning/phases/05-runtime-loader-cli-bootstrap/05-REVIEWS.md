# Phase 5 — Adversarial Review Log

Runtime Loader + CLI + Bootstrap. Panel of 5 independent Opus lens reviewers over
`05-01-PLAN.md` / `05-02-PLAN.md`, grounded in SPEC/CONTEXT/RESEARCH/EVIDENCE and the live codebase.

---

## Cycle 1

**HIGH tally (from reviewer return lines):** correctness=3, risk=5, requirement-coverage=4, security=3, simplicity=3 → **HIGH_COUNT = 18** (raw; heavy cross-lens overlap — consolidated into the consensus concerns below).

### Consensus HIGH concerns (deduplicated → these drive the replan)

**C1 — `ZSHPRO_PROFILE` lifecycle + `checkout` verb path are unowned and untested (D-05/D-06/D-07).**
Flagged by: requirement-coverage (×3), correctness. The verb bodies must `export ZSHPRO_PROFILE=<name>` after a successful `eval` and `deactivate` must `unset` it (SPEC Req 2/3 acceptance, D-07); `checkout` must call `Store.Checkout` to validate then run deactivate-prior + activate-target (D-06); the `eval "$(zsh-pro emit …)"` verb-body structure (D-05) is referenced only in frontmatter/read_first, owned by no task action. Every live test drives `activate`/`deactivate` only — none drive `checkout` or assert the `ZSHPRO_PROFILE` export/unset, so verification would go green without the behavior existing.
*Fix:* assign the zsh verb-body implementation (export/unset + `checkout`→`Store.Checkout`→switch) explicitly to a task action citing D-05/D-06/D-07; add `checkout A → assert $ZSHPRO_PROFILE==A; deactivate → assert unset / status==main` sub-cases to the live_terminal_test.

**C2 — CLI→emit seam is undefined; the `emit` verb has no reachable production path (load-bearing, unevidenced).**
Flagged by: correctness, simplicity (×2), security. The `emit apply/deactivate` handler is pinned as "delegating to Phase 4's emit path when present; against a fake source until then," but no `shell.Emitter`-style provider seam exists (`Hooker` only covers `HookScript()`), `core/cli` may not import `core/shell/zsh` (D-19), and the compiled Case-B behavior is unspecified. This is the entire Requirement-2 activation path resting on a claim with no PROVEN/STATIC-VALIDATED entry.
*Fix (do NOT descope):* This is a docs-autonomous plan-before-execute pipeline — Phase 4 executes before Phase 5 in numeric order, so `emit.go` WILL exist at execute time. Define an explicit `shell.Emitter` (or equivalent) sub-interface + composition-root injection; specify a defined Case-B stub behavior (compile-time seam present, runtime `fail` with a clear "emit path not yet available" error until Phase 4 lands); extend the Task-1 C16 reconciliation gate to cover the CLI→emit wiring, not just the loader helper surface.

**C3 — Success path never checks the emit subcommand's exit status/empty output before eval (D-16 "or the binary errors").**
Flagged by: risk, correctness, security. `code="$(zsh-pro emit apply A)"` with the binary absent/erroring captures empty stdout; empty passes `zsh -n`, `eval ""` "succeeds," and the verb then exports `ZSHPRO_PROFILE=A` + sets `ZP_LAST_GOOD_PROFILE=A` for a profile never applied — `status` and last-good now lie. D-16 requires abort on binary-error; the plan implements only the `zsh -n` half.
*Fix:* verb captures `$?` and non-empty check of the emit call; abort (no eval, no export, last-good untouched) on non-zero exit OR empty output; add live_terminal_test sub-cases for binary-exit-1 and empty-stdout.

**C4 — Unbalanced / corrupted managed-block markers (BEGIN without END) are unspecified → data-loss (T-05-06).**
Flagged by: risk, correctness. C9's PROVEN scenarios never include missing-END / stray-END / END-before-BEGIN. A naive `strings.Index` BEGIN→END scan either slices on `-1` or extends the region to EOF, silently deleting all user content from BEGIN onward — the exact data-loss threat the phase names.
*Fix:* define behavior for unbalanced markers explicitly (BEGIN-without-END ⇒ refuse-and-`fail` without writing, or bound the drop to the marker line only); add unbalanced-marker cases to install_test; add an EVIDENCE row.

**C5 — Atomic `.zshrc` write crash-safety is the sole T-05-06 mitigation but is unevidenced and lacks fsync/symlink handling.**
Flagged by: risk (HIGH), security + risk (MEDIUM, symlink). The atomic temp-file + `os.Rename` claim has no PROVEN/STATIC-VALIDATED EVIDENCE entry (RESEARCH tags it `[ASSERTED]`, crash-injection never run). Without `f.Sync()` before rename, a power loss can yield a zero-length `.zshrc`. A symlinked `~/.zshrc` (dotfiles repo) is destroyed by rename (link replaced by a regular file).
*Fix:* `f.Sync()` before `Close()`/`Rename` (and fsync the dir); resolve target via `filepath.EvalSymlinks` and rename over the resolved path; add a rename-failure/crash-injection test + symlinked-`.zshrc` case; record results in EVIDENCE.

**C6 — `ZSHPRO_DISABLE` inline `return` truncates the rest of `.zshrc` (deployed topology unproven).**
Flagged by: risk (HIGH). In the deployed inline-in-`.zshrc` stub, a top-level `return` returns from `.zshrc` itself, so `ZSHPRO_DISABLE=1` skips everything below the managed block (user aliases/env/prompt, and the out-of-block appends Phase 6 anticipates). C10/C12 are PROVEN only for a standalone stub *file* (`source stub.zsh`), not the inline topology.
*Fix:* restructure the stub without a top-level `return` (single `if`/guarded `[[ … ]] && source …` form); add a test asserting a line placed *after* the block still executes with `ZSHPRO_DISABLE=1`.

**C7 — Cached-loader file/dir perms + ownership are unspecified; T-05-09 "accept" misstates the boundary.**
Flagged by: security (HIGH). The cached loader at `$HOME/.zsh-pro/loader.zsh` is sourced by every interactive shell; T-05-09 accepts a tampered loader as "self→self," but a different principal who can write the file/dir (group/world-writable, bad umask, multi-user) gets code execution in the victim's shell — an attacker→victim boundary.
*Fix:* installer creates `~/.zsh-pro/` as `0o700` and the loader as `0o600` (not `0o644`); correct the T-05-09 disposition to *mitigate*; add an acceptance test asserting the created perms; atomically write + `zsh -n` the rendered loader at install (LookPath-guarded) before renaming into place.

**C8 — Secret values can leak into shell history / process args via the eval path.**
Flagged by: security (HIGH). Phase 5 introduces new secret-exposure surfaces Phase 4 can't own: `zsh -n <<< "$code"` and `eval "$(zsh-pro emit …)"` where `$code` may carry resolved secret values (here-string/command-substitution can land in `$history`/`fc`; `set -x` dumps them). CONTEXT/OQ-05-02 says Phase 5 drives the keychain deref at apply-time — a runtime secret path.
*Fix:* avoid here-strings/argv for secret-bearing code (read from a var/tempfile), disable history for the eval (`fc -p` / local `HISTFILE`); add a test asserting no secret value appears in `$history` after a switch.

**C9 — SPEC Req 5 `command -v zsh-pro` guard dropped from the stub with no recorded rationale (D-11).**
Flagged by: requirement-coverage (HIGH). SPEC Req 5 + D-11 mandate the stub guard activation with `command -v zsh-pro` AND `[[ -r <path> ]]`; the plan's stub has only the `ZSHPRO_DISABLE` + `[[ -r ]]` guards. The absent-binary fail-open test then proves nothing about the SPEC-required guard (the stub never references the binary). A locked SPEC constraint is changed without a recorded decision.
*Fix:* either add the `command -v zsh-pro` guard where the binary is actually invoked (the verb bodies still shell out), OR explicitly record in the plan + SPEC that D-13's cached-source design supersedes the stub `command -v` guard and relocate it to the verb bodies; make the fail-open test exercise the real guard.

**C10 — store-init error wiring is unimplementable as specified; typed-nil `Store` interface panics instead of routing through `fail`.**
Flagged by: correctness (HIGH). `store.New` returns `(nil, err)` on failure; removing `_, _ = s, err` while passing only `s` leaves `err` unused (compile error) and passing a typed-nil `*store.Store` into the `cli.Store` interface yields a non-nil interface wrapping a nil pointer — `c.store == nil` checks fail and `Branches()` panics. The specified cli_test uses a fake Store and can't reproduce the typed-nil condition.
*Fix:* specify the mechanism (main.go passes an explicit `nil` interface on `err != nil`, or a `storeErr` field / `cli.New(provider, s, err)`); add a composition-root-shaped test (nil Store) asserting `fail`, not panic.

**C11 — "No half-apply / atomic" rests only on syntax validation; a RUNTIME error mid-`eval` half-applies the shell.**
Flagged by: risk (both risk reviewers — strong consensus). Empirically demonstrated: `eval` of a syntactically-valid deactivate-then-activate block that hits a runtime error (assignment to a readonly var, `unset` of a readonly, `path=()` glob failure under `no_unset`, malformed `functions[x]=`) aborts the remainder — early statements applied, late ones never run → half-applied shell. C11/C13 prove only that `zsh -n` rejects *syntax* errors without executing; they never prove *runtime* atomicity. `ZP_LAST_GOOD_PROFILE` (name-only) cannot recover a half-applied shell — it's a label, not an undo record.
*Fix:* either (a) wrap the applied eval in a save-point/rollback (snapshot declarative state; on non-zero eval rc restore from snapshot), or (b) downgrade the SPEC/plan wording from "atomic / no half-apply" to "syntax-validated, best-effort" and add an explicit runtime-failure recovery path + fail-closed handling if the `zsh -n` subprocess itself errors/times out (never eval unless validation affirmatively returned 0). Pin a test where syntactically-valid-but-runtime-failing emitted code is eval'd and recovery/report is asserted.

**C12 — Atomicity needs ONE validated block, but `emit <apply|deactivate> <name>` is two subcommands; validating each half separately reopens the half-apply window.**
Flagged by: correctness. `checkout` must deactivate-prior + activate-target; if it calls `emit deactivate <prior>` then `emit apply <target>` as two separate captures each `zsh -n`'d and eval'd, the deactivate half can pass+apply while the apply half fails → half-apply. The plan never reconciles "single captured block up-front validated" (05-02) with the single-mode-arg subcommand shape (05-01).
*Fix:* specify that `checkout` concatenates both emit outputs into ONE string, `zsh -n`s the concatenation, then evals once — OR add an `emit switch <prior> <target>` mode that emits the full block. (Interlocks with C3/C11.)

**C13 — Fail-open stub / `ZSHPRO_DISABLE` tests are placed in `core/shell/zsh` but the stub is rendered by `core/cli/install.go`; that package can't import `core/cli` (layering).**
Flagged by: correctness. A test in `core/shell/zsh/live_terminal_test.go` cannot obtain the real stub string without importing `core/cli` (a layering violation — `core/shell/zsh` is the lower layer) or duplicating the stub literal (drift — validates a copy, not the shipped stub).
*Fix:* move the stub fail-open / `ZSHPRO_DISABLE` / corrupt-loader tests into `core/cli/install_test.go` (which owns the stub); keep only loader-verb tests in `core/shell/zsh/live_terminal_test.go`.

### MEDIUM concerns (fold into replan where cheap; else note)

- **M0 (correctness):** The `<interfaces>` block is stale/partial vs disk (main.go store wiring `kc := store.NewOSKeychainDriver(dir); s, err := store.New(dir, provider, kc)` is omitted; executor told "no exploration needed"). Mark the block "verify against disk" and correct the main.go snippet.
- **M0b (risk):** install version/hash skew — a `zsh-pro` binary upgrade without re-install leaves a cached loader whose contract diverges from the new binary's emit surface, sourced every shell start. Embed a version/hash marker; detect skew (zero-subprocess const/env compare) and no-op safely, or document that upgrade requires re-install and fold into the fail-open reasoning.
- **M0c (risk):** loader runs WITHOUT `emulate -L zsh` (D-04, intentional) so helpers execute under the user's arbitrary option set (`NO_UNSET`, `WARN_CREATE_GLOBAL`, `KSH_ARRAYS`, …); C4–C8 were proven under `zsh -f`. Add a hostile-option test matrix for the capture/restore/slot logic.
- **M0d (risk):** dup-collapse / marker-collision — a literal `# >>> zsh-pro >>>` appearing anywhere in user content is treated as a managed region and its contents replaced/dropped. Note the limitation; consider a checksum/more-specific sentinel.

- **M1 (req-cov):** `status` must report *whether a profile is activated in this terminal* (Req 3), not just the name — read `ZSHPRO_PROFILE`/last-good presence.
- **M2 (req-cov):** Req 5 third clause — a *present-but-corrupt* cached loader must not abort startup — is unpinned by any test (distinct from absent/unreadable). Add a broken-loader fail-open test.
- **M3 (correctness):** anti-speculation grep (`zp_rebuild_path|zp_shadow`) runs in Task 1 where it's vacuously green and is absent from Task 2 (the task that writes the loader). Move/duplicate it into Task 2 acceptance targeting `hook.go`.
- **M4 (correctness/risk):** base-capture guard placement — pin that the once-capture guard runs in the shared eval path used by BOTH `activate` and `checkout`; add a checkout-first `$#path` sub-case (else first `checkout` rebuilds PATH from an unset `ZP_BASE_PATH`).
- **M5 (correctness):** `install` needs the store/data dir but `core/cli` isn't given it; pin the dir resolution (inject at `cli.New`/verb time or promote `storeDir` to a shared helper) and grep-assert the stub's source path equals what `install` wrote.
- **M6 (risk):** cached-loader write atomicity (temp+rename, same as `.zshrc`) + install-time `zsh -n` validation (covered by C7 fix).
- **M7 (risk/security):** symlinked `.zshrc` + hostile `ZDOTDIR` resolution (covered by C5 fix; add a threat row for `.zshrc`/`ZDOTDIR` resolution).
- **M8 (risk):** nested/inherited-shell state asymmetry — `ZSHPRO_PROFILE` is exported but undo state (`__ZP_ORIG_*`, `ZP_BASE_PATH`, shadow slots) is not; a child shell reports a profile active whose undo slots are missing. Verbs should detect `ZSHPRO_PROFILE` set + `ZP_BASE_PATH` unset ⇒ treat as fresh; add a nested-shell sub-case.
- **M9 (security):** reword T-05-08 — `zsh -n` is a fail-open/half-apply guard, NOT an injection mitigation; cross-reference injection defense to slot sanitization (T-05-01) + the (upstream, currently unverified) Phase-4 quoting contract.
- **M10 (simplicity):** hyperfine harness (Task 3) is a CI backstop for the UNVERIFIABLE C15 behind the load-bearing C14 structural grep — collapse to a skip-when-absent test/note rather than a bespoke perf script + fixture, OR keep but assert the loader was actually sourced (`whence -w activate`) so the timing isn't vacuous.

### LOW concerns (note in plan; not blocking)
- Stub guard lines leave `$?`=1 on the false branch (breaks fail-open under user `ERR_RETURN`/`ERR_EXIT`); use `if`-form or `|| true`. Terminate stub with `true` to avoid last-exit error glyphs (p10k).
- `list`/`status` verb names unconditionally clobber any pre-existing user function of those names (SPEC-locked names — note the collision).
- slot-name sanitization collision (`a/b` ≡ `a_b`) — track as an accepted low residual in the register.
- `ZSHPRO_DISABLE` is a spoofable env kill-switch (accepted residual, note explicitly). `=1` semantics vs any-non-empty — document.
- Task-3 `go run … hook | zsh -n` automated verify not LookPath-guarded (guard with `command -v zsh`).
- Task-1 C16 contract table lives in SUMMARY (written at completion) but Task 2 read_first depends on it — write it immediately to a durable scratch/contract file.

### Ground-truth confirmations (no action)
- C1–C14 zsh semantics are faithfully transcribed and each backed by a PROVEN ledger entry (env `${(P)+var}`, shadow `${slot+x}`, sanitize-before-derive, `ZP_BASE_PATH` once-capture, zero-residue A→B→deactivate, byte-exact idempotent installer, readability no-op, `zsh -n` reject-without-execute, `ZSHPRO_DISABLE` no-op, atomic name-only last-good refuse-all, zero-subprocess start path).
- C16 (`zp_*` helper contract) correctly handled as a REFUTED reconciliation gate (Task 1, Case A/B), explicitly forbidding speculative `zp_rebuild_path`/`zp_shadow_*`.
- C15 (hyperfine <10ms) correctly demoted to CI backstop behind C14 (PROVEN).
- No new Go dependency introduced; layering respected (loader const in `core/shell/zsh`, cli-local `Store` interface, composition-root injection).

**Verdict:** NOT converged (HIGH_COUNT=18). Replan to address C1–C10 (HIGH) + fold M1–M10 where cheap.
