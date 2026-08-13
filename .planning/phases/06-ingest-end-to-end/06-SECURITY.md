---
phase: 06
slug: ingest-end-to-end
status: verified
threats_open: 0
asvs_level: 1
created: 2026-08-13
---

# Phase 06 — Security

> Per-phase security contract for ingest, Store publication, startup-file
> promotion, secret redaction, and runtime activation.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Startup path → bounded source snapshot | User-selected startup paths and symlinks must resolve without later pathname substitution controlling the committed or installed bytes. | Shell source, paths, file identity |
| Parsed Profile → Store and secret backend | Complete ordered source must persist while literal secrets are replaced before any Git object is created. | Profile entries, secret literals, SecretRefs |
| Candidate quarantine → canonical bare Store | Host Git environment, refs, indexes, work trees, and concurrent writers must not redirect candidate or publication effects. | Git objects, direct refs, transaction IDs |
| Install candidate → live startup target | Atomic promotion must preserve concurrent occupants, exact outside-marker bytes, and durable recovery evidence. | Startup bytes, journals, filesystem identity |
| Loader → current shell | Only validated emitted source may alter the parent shell; secret values must remain runtime-only and reversible. | Profile state, emitted Zsh, resolved secrets |
| Private transaction namespaces → local account boundary | Advisory locks coordinate cooperating zsh-pro processes; non-cooperating same-UID or privileged writers remain outside the exclusion guarantee. | Quarantine and journal internals |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation / Evidence | Status |
|-----------|----------|-----------|----------|-------------|-----------------------|--------|
| T-06-01-01 | Tampering | Main baseline presence/read | high | mitigate | Main-only pre-I/O reservation and exact optional revision tests | closed |
| T-06-01-02 | Tampering | Candidate objects/fresh initializer | high | mitigate | Terminal token evidence gates initializer rollback | closed |
| T-06-01-03 | Spoofing | Initialization/token/Store identity | high | mitigate | Opaque Store-issued identity and cross-Store rejection tests | closed |
| T-06-01-04 | Repudiation | Abort durability | medium | mitigate | Typed removed/retained/uncertain outcomes | closed |
| T-06-01-05 | Tampering | Recursive quarantine cleanup | high | mitigate | Root lock, no-follow traversal, reauthentication, retained recovery | closed |
| T-06-01-06 | Spoofing | Initialization authority | high | mitigate | Nonce/root/sealed-tree binding rejects forged authority | closed |
| T-06-01-07 | Tampering | Git environment/index/argv | high | mitigate | Hermetic bare-Git allowlist and hostile-environment tests | closed |
| T-06-01-08 | Repudiation | Begin/terminal replay | high | mitigate | Pre-I/O reservation and immutable Abort replay evidence | closed |
| T-06-01-09 | Tampering | Non-cooperating private-namespace writer | high | accept | OS account isolation boundary; observed mismatch retains recovery evidence | closed |
| T-06-01-10 | Denial of Service | Native Darwin cleanup | high | mitigate | Exact-SHA native macOS capability and cleanup rows pass | closed |
| T-06-01-SC | Tampering | Toolchain/package execution | high | mitigate | Local Go 1.25 gate and unchanged dependency surface | closed |
| T-06-02-01 | Information Disclosure | Secrets in objects/output | critical | mitigate | Pre-object redaction, value-free report, exhaustive object scans | closed |
| T-06-02-02 | Tampering | Expected ref/publication | high | mitigate | Validated no-deref refs and prepared update-ref transaction | closed |
| T-06-02-03 | Tampering | Lost commit observation | high | mitigate | Exact guarded compensation; third/unreadable states retain recovery | closed |
| T-06-02-04 | Repudiation | Publication vs cleanup evidence | high | mitigate | Independent immutable publication and cleanup axes | closed |
| T-06-02-05 | Denial of Service | Claimed transaction lifecycle | medium | mitigate | Atomic claim, bounded subprocess, serialized cleanup | closed |
| T-06-02-06 | Spoofing | Persisted SecretRef | high | mitigate | Structural validation, one Kind comparison, zero data methods | closed |
| T-06-02-07 | Tampering | Legacy branch adapter | high | mitigate | Private validated branch constructor and shared encoder tests | closed |
| T-06-02-08 | Tampering | Non-cooperating quarantine writer | high | accept | Cooperating-lock contract plus fail-closed mismatch retention | closed |
| T-06-02-SC | Tampering | Toolchain/package execution | high | mitigate | Local Go 1.25 gate and no module change | closed |
| T-06-03-01 | Tampering | Target parent/path substitution | high | mitigate | Retained descriptors, atomic exchange, typed displaced evidence | closed |
| T-06-03-02 | Tampering | Candidate/target mutation | high | mitigate | Independent expectedTarget and expectedCandidate comparisons | closed |
| T-06-03-03 | Repudiation | Journal/syscall/fsync ambiguity | high | mitigate | Explicit file and directory durability barriers | closed |
| T-06-03-04 | Elevation of Privilege | Recovery artifacts | high | mitigate | Exact modes, ownership, identity, digest, schema, and link checks | closed |
| T-06-03-05 | Denial of Service | Interrupted target transition | medium | mitigate | Durable state matrix recovers or retains without destructive inference | closed |
| T-06-03-06 | Information Disclosure | Post-END source bytes | medium | mitigate | Bytes remain in place; only fixed value-free warning is emitted | closed |
| T-06-03-07 | Tampering | Target mutation surface | high | mitigate | One authenticated descriptor helper and AST mutation-surface test | closed |
| T-06-03-08 | Repudiation | Absent-target rollback | high | mitigate | Post-create ambiguity retains authenticated recovery-required state | closed |
| T-06-03-09 | Tampering | Cooperating journal mutation | high | mitigate | Descriptor-backed per-root exclusive lock and helper-process matrix | closed |
| T-06-03-10 | Tampering | Non-cooperating journal writer | high | accept | OS account boundary; external occupants remain fully protected | closed |
| T-06-03-11 | Tampering | Unsupported atomic capability | high | mitigate | Pre-effect same-filesystem probe and fail-closed unsupported path | closed |
| T-06-03-SC | Tampering | Toolchain/package execution | high | mitigate | Local Go 1.25 syscall audit and cross-build gates | closed |
| T-06-04-01 | Elevation of Privilege | CLI/path source input | high | mitigate | Strict argv and static Parse/Build; source execution canaries stay inert | closed |
| T-06-04-02 | Tampering | Original target/install candidate | high | mitigate | Exact marker topology and outside-byte oracle | closed |
| T-06-04-03 | Tampering | Candidate promotion concurrency | high | mitigate | Independent evidence, atomic promotion, reverse-or-retain journal | closed |
| T-06-04-04 | Information Disclosure | Profile Store/output | critical | mitigate | Complete Profile commit with Store-owned redaction and safe renderer | closed |
| T-06-04-05 | Repudiation | Noncommit compensation | high | mitigate | Single filesystem-first compensation path and uncertainty tests | closed |
| T-06-04-06 | Spoofing | Composition Store identity | high | mitigate | One concrete Store pointer and foreign-ID rejection | closed |
| T-06-05-01 | Tampering | Production-authored expectations | high | mitigate | Independently authored fixture and expected installed bytes | closed |
| T-06-05-02 | Tampering | Regenerated startup substitute | critical | mitigate | Built binary sources actual installed target with order-sensitive oracle | closed |
| T-06-05-03 | Elevation of Privilege | Unmanaged entry activation | critical | mitigate | EffectiveManaged projection and inert execution canaries | closed |
| T-06-05-04 | Information Disclosure | Secret confinement | critical | mitigate | Exhaustive primary/quarantine object and isolated-root scans | closed |
| T-06-05-05 | Spoofing | Source sentinel SecretRef authority | high | mitigate | Store-authoritative comparator and forged-sentinel rejection | closed |
| T-06-05-06 | Denial of Service | Startup subprocess | high | mitigate | Stub counters prove installed startup launches no subprocess | closed |
| T-06-05-07 | Repudiation | Darwin cross-build evidence | high | mitigate | Exact implementation SHA bound to passing native macOS evidence | closed |

*Status: open · closed · open — below high threshold (non-blocking)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-06-01 | T-06-01-09 | Private Store internals rely on OS account isolation; arbitrary non-cooperating same-UID or privileged mutation cannot be excluded, but detected mismatch fails closed. | Phase 06 approved threat model | 2026-08-03 |
| AR-06-02 | T-06-02-08 | Advisory locking coordinates cooperating zsh-pro processes only; untrusted writers with the same account authority are outside the product boundary. | Phase 06 approved threat model | 2026-08-03 |
| AR-06-03 | T-06-03-10 | Private journal exclusion stops at the OS account/privilege boundary while external startup-file occupants remain protected. | Phase 06 approved threat model | 2026-08-03 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-08-13 | 45 | 45 | 0 | Codex / GSD ASVS-L1 artifact verification |

Evidence basis: all five Phase 6 summaries, the passed 5/5 canonical
verification, exact native macOS evidence, current user UAT (15/15), and the
security-relevant package and script gates recorded at sign-off.

---

## Sign-Off

- [x] All threats have a disposition
- [x] Accepted risks documented in Accepted Risks Log
- [x] threats_open: 0 confirmed
- [x] status: verified set in frontmatter

**Approval:** verified 2026-08-13
