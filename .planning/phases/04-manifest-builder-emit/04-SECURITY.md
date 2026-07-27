---
phase: 04
slug: manifest-builder-emit
status: verified
threats_open: 0
asvs_level: 1
block_on: high
register_authored_at_plan_time: true
created: 2026-07-27
updated: 2026-07-27
---

# Phase 04 — Security

> Per-phase security contract for the profile-to-manifest-to-emitted-zsh boundary.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| zsh source/profile → parser and IR | Owner-authored source becomes semantic values, names, functions, list segments, and secret references. | Executable shell source and potentially sensitive values |
| manifest/plan → emitted zsh | Shell-agnostic operations become code that a sourced loader will evaluate. | Names, literal/dynamic values, function bodies, PATH/FPATH deltas |
| live zsh state → deactivation | Prior values and ownership state are captured and later restored. | Environment, aliases, functions, options, PATH/FPATH, undo slots |
| introspection subprocess → Go parser | NUL-framed identity data returns from a disposable `zsh -f` process. | Alias/function bodies and shell identity metadata |
| secret runtime value → backend and Git ref | Literal secrets are withheld from profile blobs and written transactionally before a CAS ref update. | Secret bytes, redacted profile, backend/ref state |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-01-06 | Elevation of Privilege / Tampering | Emitted eval code | high | mitigate | Static data is zquoted; dynamic source is admitted by explicit provenance; hostile values are exercised through real zsh. | closed |
| T-04-01 | Tampering | Manifest schema | high | mitigate | `SchemaV1` is enforced before Diff produces operations. | closed |
| T-04-03 | Elevation of Privilege | Single emit boundary | high | mitigate | Reverse-zsh construction is confined to `core/shell/zsh/emit.go` and pinned by a recursive ownership invariant. | closed |
| T-04-05 / T-04-10-01 / T-04-10-02 | Spoofing / Tampering | Introspection status and framing | high | mitigate | Source failure is fail-closed; identity prefix and NUL-framed body payloads are parsed separately. | closed |
| T-04-17 / T-04-18 / T-04-19 | Elevation of Privilege | Option, alias/function, and list-segment injection | high | mitigate | Builder and emitter independently validate names; static list segments are quoted; hostile names and canaries are rejected. | closed |
| T-04-06-01 / T-04-06-02 | Tampering | Undo-slot identity and absence representation | high | mitigate | Injective slot encoding and separate presence/value state prevent collisions and sentinel ambiguity. | closed |
| T-04-06-03 / T-04-06-05 | Tampering | Repeated scalar ownership and export state | high | mitigate | Final-effective identities retain first-original/final-applied semantics and restore export provenance exactly. | closed |
| T-04-07-01 / T-04-07-02 | Tampering / Elevation of Privilege | Literal decoder and emitted values | high | mitigate | AST allowlist rejects unmodeled forms; supported escapes are compared byte-for-byte with live zsh. | closed |
| T-04-07-03 | Information Disclosure | Secret persistence | high | mitigate | Exact runtime bytes are moved behind `SecretRef`; literal-bearing profile fields are cleared before persistence. | closed |
| T-04-08-01 / T-04-08-02 / T-04-08-05 | Tampering / Elevation of Privilege | PATH/FPATH semantics | high | mitigate | Same-list self markers, explicit segment provenance, fail-closed substitution rules, and direct cardinality comparisons. | closed |
| T-04-09-01 / T-04-09-03 / T-04-09-05 / T-04-09-06 | Tampering | Repeated list composition and restoration | high | mitigate | One composed symbolic base with explicit placement/presence; verified in automated tests and a persistent live zsh Alpha→Beta→baseline switch. | closed |
| T-04-11-I / T-04-11-T1 / T-04-11-T2 / T-04-11-D | Information Disclosure / Tampering / Denial of Service | Secret and Git transaction | high | mitigate | Preparation is pure, writes are rollback-recorded, ref movement is CAS and last, and errors remain typed/redacted. | closed |
| T-04-12-T1 / T-04-12-T2 / T-04-12-E / T-04-12-I | Tampering / Elevation of Privilege / Information Disclosure | Final oracle and discovery gates | high | mitigate | Production-path byte snapshots, sensitivity mutants, punctuation/sentinel cases, canary scans, and non-empty discovery checks. | closed |
| T-04-12 | Denial of Service | Interrupted multi-operation switch | medium | accept | Phase 4 emits plain reversible functions but has no transaction journal; recovery is a fresh terminal or reactivation. Transactional loader recovery is deferred to Phase 5. | closed |
| T-04-SC | Tampering | Dependency supply chain | low | accept | Phase 4 introduced no new dependencies or package-manager installs. | closed |

*Only open threats at or above the configured high threshold count toward `threats_open`.*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-01 | T-04-12 | Phase 4 has no loader transaction boundary; fail-open recovery and any journal belong to the Phase 5 runtime loader. | Phase 4 plans | 2026-07-27 |
| R-04-02 | T-04-SC | No additional supply-chain surface was introduced. | Phase 4 plans | 2026-07-27 |

---

## Verification Evidence

- Targeted security regression matrix passed uncached across hostile inputs, canaries, secrets, rollback, framing, slot collisions, list semantics, zero residue, and reverse-zsh ownership.
- `GOTOOLCHAIN=auto go test ./... -count=1` passed.
- `GOTOOLCHAIN=auto go build ./...` passed.
- `GOTOOLCHAIN=auto go vet ./...` passed.
- `make lint` passed with zero issues.
- A persistent real `zsh -f` PTY switched baseline → Alpha → Beta → baseline through Parse → IR → Build → Diff → Emit. Variables, dynamic `$HOME` values, aliases, functions, options, and composed PATH/FPATH values changed per profile and restored exactly.
- A manual post-activation scalar edit survived deactivation, confirming the drift guard.

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-27 | 15 grouped register entries | 15 | 0 | Codex inline ASVS L1 audit |

---

## Sign-Off

- [x] All threats have a disposition
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-27
