# Phase 3: Git-Backed Store - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-27
**Phase:** 3-git-backed-store
**Areas discussed:** Store layout & format, Repo location & baseline, Secrets (default & scope), Per-terminal & checkout safety

---

## Store layout & format

| Option | Description | Selected |
|--------|-------------|----------|
| JSON IR (authoritative) | Serialize model.Profile to JSON; lossless round-trip, no re-parse | |
| JSON IR + emitted .zsh | JSON authoritative + regenerated .zsh committed alongside for browsing/diffs | ✓ |
| Per-category .zsh only | env.zsh/aliases.zsh/...; loses derived fields, couples store to a parser | |
| Single source-ordered .zsh | one profile.zsh; same round-trip loss, monolithic diffs | |

**User's choice:** JSON IR + emitted .zsh
**Notes:** Follow-up on emit granularity — chose **single source-ordered .zsh** (reuse Phase 2 `regen.go`, no Phase 4 pull-forward) over per-category-now (which would reopen the D-02/D-09 load-order hazard). JSON is authoritative; .zsh is a derived view.

---

## Repo location & baseline

| Option | Description | Selected |
|--------|-------------|----------|
| XDG data dir + override | $XDG_DATA_HOME/zsh-pro (default ~/.local/share/zsh-pro), $ZSHPRO_HOME override | ✓ |
| XDG config dir + override | $XDG_CONFIG_HOME/zsh-pro | |
| $ZSHPRO_HOME only, default ~/.zsh-pro | single env var, simple dotdir | |

**User's choice (location):** XDG data dir + override

| Option | Description | Selected |
|--------|-------------|----------|
| 'baseline' branch; new profiles fork it | explicit baseline name; fork baseline | |
| 'main' branch; new profiles fork it | conventional git default; fork main | ✓ |
| 'baseline'; new profiles fork CURRENT | fork active profile | |

**User's choice (baseline):** 'main' branch; new profiles fork main
**Notes:** Recorded as discretion that Init is idempotent and git-absent degrades with a clear error; real-`~/.zshrc` seeding of main is Phase 6.

---

## Secrets (default & scope)

| Option | Description | Selected |
|--------|-------------|----------|
| Full exclusion in Phase 3 | detect + exclude + report at Commit now | ✓ |
| Hook only in Phase 3 | seam now, filtering deferred to Phase 6 | |

**User's choice (scope):** Full exclusion in Phase 3

| Option | Description | Selected |
|--------|-------------|----------|
| Omit entirely + report | drop value, report name+line | |
| Omit value, keep commented marker | leave a `# withheld` marker in .zsh | |
| Git-ignored local sidecar | secrets.local.zsh; bleeds across branches | |
| **(User idea) Secrets as pointers** | store a reference; dereference at switch | ✓ |

**User's choice (handling):** *Free-text:* "What if secrets were stored like 'pointers'? and when you switch profiles, you sort of 'dereference' the secrets from wherever they are stored?"
**Notes:** Recognized as EVAL-01 late-binding applied to secrets. Refined into: literal secrets → `SecretRef` (kind:key); already-dynamic secrets commit verbatim; value captured into a backend; deref-on-switch is Ph4/5.

| Option | Description | Selected |
|--------|-------------|----------|
| Resolver-agnostic pointer (keychain default) | kind:key; keychain subprocess default + git-ignored file fallback | ✓ |
| Git-ignored local vault file only | single name-keyed vault; plaintext on disk | |
| External-manager passthrough only | pointer is a resolver command; no zsh-pro storage | |

**User's choice (deref target):** Resolver-agnostic pointer (keychain default)

---

## Per-terminal & checkout safety

| Option | Description | Selected |
|--------|-------------|----------|
| Read via git show, never checkout | object-DB reads; current = env var; no shared-HEAD contention | ✓ (via principle) |
| Git worktree per terminal | independent HEADs; heavier lifecycle | |
| Single working tree + real checkout | shared HEAD; reintroduces the forbidden race | |

**User's choice (isolation):** *Free-text:* "Ideally the user should not have to tinker with git at all, they should use zsh-pro commands only and the git part is completely abstracted."
**Notes:** Captured as the hard UX principle D-11 (git fully abstracted; zsh-pro verbs only; errors in zsh-pro terms). This principle selects the read-via-`git show` / no-user-facing-checkout option (D-12).

| Option | Description | Selected |
|--------|-------------|----------|
| Profile name only | ZSHPRO_PROFILE=name; path from XDG; unset ⇒ main | ✓ |
| Name + repo path | for multiple stores (out of scope) | |
| Name + validation token | detect stale terminal state | |

**User's choice (active state):** Profile name only

---

## Claude's Discretion

- `core/store` API signatures, package placement, regenerator-seam shape.
- git plumbing for committing to a non-checked-out branch (temp index / commit-tree / update-ref).
- Commit granularity, commit-message conventions, list/status output shape.
- Resolver backend implementation (keychain subprocess, vault-file format/location), provided `SecretRef` `kind:key` serialization is stable and no new/encryption dep is added.
- Exact env var name; Init-idempotency and git-absent error wording.

## Deferred Ideas

- Dereference-on-switch for `SecretRef`s → Ph4/5.
- Per-category `.zsh` emit + PATH add/delete segmentation → Ph4.
- Exporting `ZSHPRO_PROFILE` on switch + loader/CLI verbs + `.zshrc` bootstrap → Ph5.
- Real `~/.zshrc` end-to-end ingest into `main` → Ph6.
- Multiple independent stores → out of scope (v2.0).
