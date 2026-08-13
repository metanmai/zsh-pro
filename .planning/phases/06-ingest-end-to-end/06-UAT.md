---
status: complete
phase: 06-ingest-end-to-end
source:
  - 06-01-SUMMARY.md
  - 06-02-SUMMARY.md
  - 06-03-SUMMARY.md
  - 06-04-SUMMARY.md
  - 06-05-SUMMARY.md
started: 2026-08-09T22:51:37+00:00
updated: 2026-08-13T14:00:28+00:00
---

## Current Test

[testing complete]

## Tests

### 1. Store authority and baseline reservation
expected: Store-issued initialization authority and an exact optional-main baseline are reserved before I/O, rejecting forged or cross-store identities.
result: pass
source: automated
coverage_id: D1

### 2. Transaction-root locking
expected: Cooperating processes serialize complete transaction-root mutation intervals; unsafe or discarded lock authority makes no mutation.
result: pass
source: automated
coverage_id: D2

### 3. Prepared publication ordering
expected: Prepared no-deref ref locking serializes writers before backend or final-object effects, and commits only after durable publication.
result: pass
source: automated
coverage_id: D1

### 4. Publication and cleanup evidence
expected: Candidate, expected, third, and unreadable ref observations preserve publication truth independently from cleanup and compensation evidence.
result: pass
source: automated
coverage_id: D2

### 5. Exact marker installation
expected: Ingest preparation preserves ordinary startup bytes, classifies exact marker topology, and produces one canonical candidate.
result: pass
source: automated
coverage_id: D1

### 6. Platform atomic promotion
expected: Supported platforms use audited atomic exchange/no-replace operations with strict names, and unsupported platforms fail closed.
result: pass
source: automated
coverage_id: D2

### 7. Guarded startup transaction
expected: One authenticated private peer and journal can promote, reverse, recover, or retain startup state without destroying a concurrent occupant.
result: pass
source: automated
coverage_id: D3

### 8. CLI arguments and accounting
expected: Ingest accepts one optional path and repeatable --json, rejects invalid input with exit 2, and reports value-free statement accounting.
result: pass
source: automated
coverage_id: D1

### 9. One Store composition root
expected: Initialization and Begin/Commit/Abort/cleanup use one exact Store; a second Store rejects foreign authority before effects.
result: pass
source: automated
coverage_id: D2

### 10. Complete-profile controller
expected: The controller commits the complete ordered Profile around one loader/startup promotion, finalizing on commit and compensating filesystem-first otherwise.
result: pass
source: automated
coverage_id: D3

### 11. First isolated ingest
expected: The disposable fixture ingests successfully, commits the baseline, installs the loader region, accounts for every source statement, and reports the named withheld secret without disclosing its value.
result: pass

### 12. Installed-shell behavior and loader controls
expected: Sourcing the actually installed disposable .zshrc keeps the source alias, function, order-sensitive behavior, and exposes checkout/activate/deactivate in that shell.
result: pass

### 13. Secret boundary visible to the user
expected: The test literal is absent from the committed main profile, while the ingest report contains only PHASE6_API_TOKEN and its line number.
result: pass

### 14. Re-ingest idempotence
expected: Re-running ingest without source changes preserves the installed .zshrc byte-for-byte and succeeds without duplicating the managed loader region.
result: pass

### 15. Post-END append preservation
expected: Content appended after the managed END marker survives re-ingest unchanged and causes a warning rather than being overwritten.
result: pass

## Summary

total: 15
passed: 15
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none yet]
