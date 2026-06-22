# zsh-pro — Backlog

Deferred items from the analyze-engine v1 build (2026-06-22). **v1 shipped:** a read-only, AST-based zsh config analysis engine (parse → classify → introspect-liveness → reconcile → render/CLI), reviewed clean. Tracked here so the deferrals are explicit, not implicit.

## Top priority — complete the dynamic-introspection half
v1 runs `zsh -f` introspection but consumes only the `Available` liveness flag; the resolved `IdentitySet` (aliases/functions/env/path/options) is captured and discarded (shadow detection was moved to static blocks to avoid inherited-env false positives, which removed the only consumer of the resolved data). To deliver the spec's static+dynamic vision:
1. **Env-isolated introspection** — run with a clean environment baseline (e.g. `env -i zsh -f` + subtract zsh's own `-f` defaults) so the resolved set reflects what the *config* defines, not inherited shell state.
2. **Consume the resolved set** — flag identities that are live-but-unattributed (present after sourcing, defined by no static block → produced by an opaque init like `eval "$(starship init)"` / `source`), and annotate which definition "wins" for duplicates. Then the spec's "static-vs-dynamic agreement" tests become writable.

## Classifier accuracy
- PATH rule over-captures: `strings.Contains(u,"PATH")` matches `PATHOLOGICAL_VAR`. Replace with an allowlist (PATH/FPATH/MANPATH/CDPATH/INFOPATH) + `HasSuffix("PATH")`. Currently ConfMedium → flagged-for-review, not silently wrong.
- Secret regex over-captures substrings (`TOKENIZER` → secrets). Fail-safe (over-flagging beats leaking); tighten only if noisy.
- `eval` plugin-init under-captures common tool-init (`thefuck`, `dircolors`); the pluginHints fallback is unreachable for `eval`/`source`. Let the hint scan run on the unmatched-eval branch.

## Test corpus expansion (Tier-1 completion + Tier-2)
- No fixtures yet exercise `duplicate_path` or `shadowed` issue kinds (2 of 4) — add them.
- Missing Tier-1 categories from the spec taxonomy: order-sensitivity (PATH precedence, fpath/compinit), zsh syntax that mvdan/sh chokes on (validate the opaque fallback), structural edge cases (heredocs, multiline funcs), pathological (syntax error).
- Golden manifest asserts issue `Kind` only; add optional `issue_names`/`issue_lines` fields and assert them.
- Tier-2: a sample of scrubbed real-world configs for breadth.

## Parser / model cleanup
- Dead `*CallExpr` export/typeset branch in `parse.go` (unreachable under LangZsh — they parse as `*DeclClause`). Remove, or keep as a documented bash-fork fallback.
- `Block.Exported` is set but never read — consume it (env classification could prefer it) or drop it. Also misses flag-form exports (`typeset -x FOO`).

## Smaller items
- `dupPathIssues` regex misses unrooted relative PATH entries and mis-names matched relative segments (e.g. `./scripts` → `/scripts`).
- Introspect error path returns nil maps (safe to read; initialize for defensive hardening if future callers write).
- Harden the `##END##` sentinel (require it seen, else `Available=false`) once the resolved set is consumed.
- Empty file reports "1 lines" (off-by-one in `strings.Count(src,"\n")+1`).

## Beyond the analyze engine (the product ramp)
- split/adopt (first write; conservative + run-both-and-diff safety net) → reconcile/merge engine + tidy → profiles → import/export → "clever" order-safety.
- The TUI (Bubble Tea), per wedge-validation findings.
- bash-pro (fork the shell `Provider`).

## Build/env notes
- Module requires **Go 1.25** (mvdan/sh v3.13's zsh support forces it; the toolchain auto-upgrades via `GOTOOLCHAIN=auto`). The original plan said "1.22 floor" — that is superseded.
