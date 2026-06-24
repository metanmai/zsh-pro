# Requirements: zsh-pro

**Defined:** 2026-06-24
**Milestone:** v1.1 "Trustworthy PATH Analysis"
**Core Value:** `analyze --json` reports output you can trust — every PATH entry is named exactly as written, genuine duplicates are caught across notations, and risky entries are flagged without polluting the exit-code signal.

## v1 Requirements

Requirements for this milestone. Each maps to a roadmap phase.

### PATH Analysis

- [ ] **PATH-01**: `analyze` extracts each PATH entry by parsing the assignment value and splitting on `:` (not by regex-scraping the block text), so a relative entry is reported with its exact written name (`./scripts`, never `/scripts`) and an unrooted entry (`scripts`, `bin`) is detected. The `$PATH`/`${PATH}` self-reference is excluded and surrounding quotes are trimmed.
- [ ] **PATH-02**: Duplicate-PATH detection treats notation-equivalent entries as the same entry — bare `~` ≡ `$HOME` ≡ `${HOME}`, with trailing and duplicate slashes normalized — using notation-only canonicalization (no filesystem access, no `$HOME`/env resolution; `~user` left distinct). The reported duplicate `name` is the entry as written (verbatim); its `lines` list every line that adds it.
- [ ] **PATH-03**: `analyze` surfaces relative/unrooted PATH entries — `./x`, `../x`, bare words, bare `.`, and empty entries (a leading, trailing, or `::` colon, all meaning the current directory) — as a `relative_path_entry` advisory; the `.`/empty current-directory cases carry a CWE-427 security reference.

### Issue Severity

- [ ] **SEV-01**: Every issue carries a severity (`actionable` or `advisory`), surfaced in both the human report and the `--json` envelope as a self-describing string. `actionable` is the default (zero value) so the four existing issue kinds are unchanged.
- [ ] **SEV-02**: The exit code and the `issues_found` flag reflect only actionable issues — a config whose only finding is an advisory exits `0` with `issues_found: false`. Advisories never bump the exit code; exit 3 stays reserved for genuine problems (duplicate alias/env/path, shadowed).

### Test Coverage

- [ ] **COV-01**: The golden corpus includes fixtures exercising the previously-uncovered `duplicate_path` and `shadowed` issue kinds.
- [ ] **COV-02**: The golden corpus asserts each fixture's `issue_names`, `issue_lines`, and `severity` — not just the issue `Kind`.
- [ ] **COV-03**: The testgen oracle generates and independently predicts relative, unrooted, and notation-equivalent duplicate PATH entries plus the advisory, with canonicalization re-implemented inside `core/testgen` (model-only, never importing the engine's canonicalizer) so the property test stays non-circular.

## Future Requirements

Deferred to future work. Tracked but not in this roadmap.

### Classifier Precision (carried from v1.0)

- **PREC-01**: Categorization confidence is surfaced in output; low-confidence results are visibly flagged, not silently merged into a confident category.
- **PREC-02**: Classifications below high confidence land in an explicit "uncertain" bucket.
- **PREC-03**: PATH *classification* stops over-capturing substrings (`DISPATCH_HANDLER`, `XPATH`) — allowlist + `HasSuffix("PATH")` instead of `Contains("PATH")`. (Distinct from PATH-01/02, which fix the *reconciler*, not the classifier.)
- **PREC-04**: Secret detection stops matching substrings (`TOKENIZER`, `PASSWORD_STYLE`) — word-boundary matching.

### Engine Completeness

- **DYN-01**: Consume the resolved `IdentitySet` from `zsh -f` introspection (env-isolated) to flag live-but-unattributed identities (opaque init like `eval "$(starship init)"`).
- **CAP-01**: `eval`/`source` plugin-init under-capture — let the `pluginHints` scan run on the unmatched-`eval` branch (recall gap; revisit after PREC-*).

## Out of Scope

Explicitly excluded for this milestone. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Filesystem / live-`$HOME` resolution of PATH entries | Canonicalization is notation-only; resolving `~`/`$HOME` against the environment, or `..`/symlinks against disk, would make a read-only static analyzer env-dependent and non-deterministic |
| `~user` (named-home) folding into `$HOME` | `~root`→`/var/root` ≠ `$HOME`; folding named-home tildes would be semantically wrong |
| PATH ordering / precedence analysis (which entry shadows a later one) | A separate order-sensitivity feature, not part of this correctness pass |
| Classifier PATH/secret over-capture (`Contains("PATH")`, secret substrings) | Classifier-precision stance (Future PREC-*); independent of the reconciler PATH fix |
| Whole-file opaque-parse recovery | Needs upstream `mvdan/sh` work; separate effort |
| New commands / multi-file / other shells (`fix`/`doctor`, `--paths`, bash-pro) | Product ramp, not this milestone |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SEV-01 | Phase 2 | Pending |
| SEV-02 | Phase 2 | Pending |
| PATH-01 | Phase 3 | Pending |
| PATH-02 | Phase 3 | Pending |
| PATH-03 | Phase 3 | Pending |
| COV-01 | Phase 4 | Pending |
| COV-02 | Phase 4 | Pending |
| COV-03 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 8 total
- Mapped to phases: 8 ✓ (Phase 2: 2 · Phase 3: 3 · Phase 4: 3)
- Unmapped: 0 ✓ — every requirement maps to exactly one phase; no orphans, no duplicates

---
*Requirements defined: 2026-06-24*
*Last updated: 2026-06-24 after roadmap creation (milestone v1.1) — traceability populated, 8/8 mapped*
