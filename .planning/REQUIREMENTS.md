# Requirements: zsh-pro

**Defined:** 2026-06-25
**Milestone:** v2.0 "Branchable Shell Environments"
**Core Value:** `checkout <branch>` gives you a different, trustworthy shell environment — declarative state (aliases/env/PATH/functions/options) applies and reverses cleanly with zero residue, while portability is preserved (dynamic values like `$HOME`/`$(...)` stay late-bound, never frozen to one machine).

## Requirements

Requirements for this milestone. Each maps to a roadmap phase.

### Ingest

- [x] **ING-01**: `zsh-pro` ingests `~/.zshrc` into a categorized, regenerable representation (env / aliases / functions / PATH / options) that round-trips back to behavior-equivalent zsh. Untouched statements are preserved verbatim (`Block.Text`); only rewritten declarative slices are templated.
- [x] **ING-02**: Ingest classifies each statement as **declarative** (set/unset-reversible → switchable) or **imperative** (run-once, side-effecting → unmanaged master block). A misclassified imperative line is a zero-residue violation by construction, so this gate is correctness-critical.

### Partial Evaluation

- [x] **EVAL-01**: The stored representation resolves syntactically-static values and keeps dynamic ones (`$HOME`, `${...}`, `$(...)`, conditionals) **late-bound** — "static" means syntactically constant, never resolved against the current machine's disk/env. Portability across machines is preserved.

### Profiles / Store

- [x] **PROF-01**: The representation is stored as a git-backed repo **via the `git` binary** (no new Go dependency; mirrors the existing `zsh -f` subprocess pattern) where each branch is an environment profile.
- [x] **PROF-02**: The user can create, list, and switch profiles (`checkout <branch>`); the active profile is tracked **per-terminal** (env-var-carried state), never a global file (avoids the conda concurrent-activation race class).
- [ ] **PROF-03**: Detected secrets are **excluded from the committed profile by default** (no encryption dependency); the user is told what was withheld. Reuses the existing secret detection from the ingest engine. *(Started Phase 3, D-07–D-10: store-side exclusion landed — a literal secret is replaced by a `SecretRef` (resolver-agnostic `kind:key`), its value captured to the OS keychain / git-ignored vault backend, the literal kept out of both committed blobs, and a withheld-report returned; already-dynamic secrets commit verbatim. Runtime **deref-on-switch** completes in Ph4/5; the real-`~/.zshrc` end-to-end ingest path is Ph6 — so this stays unchecked until then.)*

### Switch / Activation

- [ ] **SW-01**: A profile's declarative state is applied to the current shell via a **sourced loader that `eval`s emitted shell code** (a child process cannot mutate its parent shell). All zsh syntax lives in one emit path.
- [ ] **SW-02**: Switching profiles **deactivates** the prior profile's managed state (reverse-diff manifest: `unalias`, `unset -f`, restore env to captured prior values, rebuild PATH from a captured base) then **activates** the new one — with **zero residue**: no leftover aliases/functions/options, no PATH growth, and base/unmanaged state left untouched (ownership-aware, Lmod-style — don't remove `/usr/local/bin` just because a profile also added it).
- [x] **SW-03**: *(Frontier)* Switching works **live in an already-open terminal**, not just new shells — validated by the Phase-1 spike asserting a byte-identical environment after `activate → switch → switch-back` on a no-op round-trip (env **and** aliases/functions/options).

### Bootstrap / Reliability

- [ ] **BOOT-01**: A single **idempotent, BEGIN/END-marked `.zshrc` block** bootstraps the loader and preserves an unmanaged "master block" for imperative run-once code. Re-running the installer never duplicates the block.
- [ ] **BOOT-02**: The loader is **fail-open and fast** — a broken, missing, or slow `zsh-pro` never locks the user out of a working shell (guarded sourcing, `zsh -n`-validated manifests, last-good fallback, a `ZSHPRO_DISABLE=1` escape hatch) and adds only a small, file-sourced startup cost (no git/subprocess on the hot path).

## Future Requirements

Deferred to future work. Tracked but not in this roadmap.

### Profile Sharing

- **SHARE-01**: Pull/push profiles from a remote git repo, with a **trust gate** before applying a profile authored elsewhere (per-profile or per-hash approval).

### Ergonomics

- **AUTO-01**: Optional auto-activate on `cd` (direnv-style per-prompt `chpwd`/`precmd` hook), with a hard "no git subprocess per prompt" constraint and its own trust surface.

### Parked from v1.1 (analyzer milestone)

- The trustworthy PATH-extraction + notation-dedup work (PATH-01/02/03) re-scopes into **ING-01** (PATH is one ingest category needing correct split + canonicalization). The analyzer-reporting/oracle items (COV-01/02/03) remain parked — see `milestones/v1.1-ROADMAP.md`.

## Out of Scope

Explicitly excluded for this milestone. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Filesystem / live-`$HOME`/`$(...)` resolution | Resolving dynamic values against a specific machine freezes a profile to that machine and destroys cross-machine portability — the whole point of branches |
| Making the imperative startup surface switchable | Side-effecting run-once code (daemons, `eval`, version-manager init) can't be cleanly un-run; it stays in the unmanaged master block. Only declarative state is switchable |
| Other shells (bash, fish) | The activation model is zsh-specific (`zmodload zsh/parameter`, `unalias`/`unset -f`/`unsetopt`, `add-zsh-hook`) |
| Multi-file config graphs (sourced fragments, `*.zsh` beyond the entry point) | Single-entry ingest first; deep source-graph following is a later concern |
| Profile encryption / syncing secrets | Needs a crypto dependency (forbidden); secrets are excluded from the tree by default instead (PROF-03) |
| Remote profile sharing + trust gate | Future (SHARE-01) — v2.0 targets local profiles & branches |
| Auto-activate on `cd` | Future (AUTO-01) — v2.0 is explicit-`checkout` only |
| PATH ordering/precedence analysis · classifier precision overhaul | Captured design stances for later; independent of the manager's core switch loop |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SW-03 | Phase 1 — SPIKE: Zero-Residue Live Hot-Switch | Complete |
| ING-01 | Phase 2 — IR + Partial Evaluation | Complete |
| ING-02 | Phase 2 — IR + Partial Evaluation | Complete |
| EVAL-01 | Phase 2 — IR + Partial Evaluation | Complete |
| PROF-01 | Phase 3 — Git-Backed Store | Complete |
| PROF-02 | Phase 3 — Git-Backed Store | Complete |
| SW-01 | Phase 4 — Manifest Builder + Emit | Pending |
| SW-02 | Phase 4 — Manifest Builder + Emit | Pending |
| BOOT-01 | Phase 5 — Runtime Loader + CLI + Bootstrap | Pending |
| BOOT-02 | Phase 5 — Runtime Loader + CLI + Bootstrap | Pending |
| PROF-03 | Phase 3 (store-side exclusion + reference) → Phase 4/5 (runtime deref) → Phase 6 (end-to-end ingest) | In progress (started Phase 3) |

**Coverage:**
- Milestone requirements: 11 total (ING-01/02, EVAL-01, PROF-01/02/03, SW-01/02/03, BOOT-01/02)
- Mapped to phases: **11/11** — every requirement mapped to exactly one phase, no orphans, no duplicates.

---
*Requirements defined: 2026-06-25 (milestone v2.0)*
*Traceability populated: 2026-06-25 (roadmap creation)*
