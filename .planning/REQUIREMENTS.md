# Requirements: zsh-pro

**Defined:** 2026-06-24
**Core Value:** `analyze --json` reports line numbers you can trust — every issue points at the real statement line, and the reported line count is accurate.

## v1 Requirements

Requirements for this milestone (correct the two known analyzer line-number bugs and pin them with tests). Each maps to a roadmap phase.

### Line Accuracy

- [x] **LINE-01**: `analyze` reports `Analysis.Lines` as the true line count — an empty file reports `0`, and a file ending in a trailing newline is not counted one line too high.
- [x] **LINE-02**: When a config statement has a leading `#` comment, every issue involving that statement reports the statement's own line number, not the comment's line.

### Test Pinning

- [x] **PIN-01**: The `testgen` oracle property test asserts line numbers (`checkLines = true`) — total `Lines` and each issue's `lines` slice — across all seeds, and passes.
- [x] **PIN-02**: The existing golden corpus (`manifests.json` + `corpus_test.go`) passes against the corrected output, and the `empty.zsh` case reflects a `0`-line count.

## v2 Requirements

Deferred to future work. Tracked but not in this roadmap.

### Output Correctness

- **PATH-01**: `dupPathIssues` correctly names relative PATH entries (`./scripts` not mis-reported as `/scripts`) and detects duplicate unrooted entries.

### Test Coverage

- **COV-01**: Golden fixtures exist for the `duplicate_path` and `shadowed` issue kinds (currently 0 coverage).
- **COV-02**: The golden corpus asserts `issue_names` and `issue_lines`, not just issue `Kind`.

### Classifier Precision

Design stance (2026-06-24): categorize with precision over recall — a silent false positive is worse than an honest "unsure." See PROJECT.md Key Decisions.

- **PREC-01**: Categorization confidence is surfaced in output (human + `--json`) — low-confidence results are visibly flagged, not silently merged into a confident category (today `Block.Conf` is computed but read nowhere).
- **PREC-02**: Classifications below high confidence land in an explicit "uncertain" bucket rather than a confident category.
- **PREC-03**: PATH classification stops over-capturing substrings (`DISPATCH_HANDLER`, `XPATH`) — allowlist + `HasSuffix("PATH")` instead of `Contains("PATH")`.
- **PREC-04**: Secret detection stops matching substrings (`TOKENIZER`, `PASSWORD_STYLE`) — word-boundary / full-word matching instead of substring.

## Out of Scope

Explicitly excluded for this milestone. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Whole-file opaque fallback recovery | Filed under Fragile Areas, not a line bug; partial-parse recovery needs upstream `mvdan/sh` work |
| Dynamic-introspection completion (consume resolved `IdentitySet`) | A feature, not a bug fix; top of the backlog but a separate effort |
| `eval`/`source` plugin-init under-capture | Recall gap (misses valid tool-init like `thefuck`/`dircolors`); separate from the precision stance — revisit after PREC-* |
| New commands / multi-file / other shells (`fix`/`doctor`, `--paths`, bash-pro) | Product ramp, not this milestone |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| LINE-01 | Phase 1 | Complete |
| LINE-02 | Phase 1 | Complete |
| PIN-01 | Phase 1 | Complete |
| PIN-02 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 4 total
- Mapped to phases: 4
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-24*
*Last updated: 2026-06-24 after roadmap creation*
