# Feature Research

**Domain:** PATH-hygiene findings for a read-only zsh-config analyzer (zsh-pro v1.1 "Trustworthy PATH Analysis")
**Researched:** 2026-06-24
**Confidence:** HIGH (terminology + severity grounded in ShellCheck wiki, CWE 4.20, CIS Benchmarks, Lynis source, and Go stdlib security policy)

---

## TL;DR for Requirements/Roadmap Authors

- **Recommended advisory name:** `relative_path_entry` (issue kind / `--json` name) — surfaced to humans as **"relative PATH entry"**, with the cwd special-case (`.`, empty, leading/trailing/double colon) called out as **"current directory in PATH"**. This matches Lynis ("relative path in PATH"), CIS ("Ensure root PATH Integrity"), and CWE-427 ("current working directory ... untrusted search element"). It is **not invented** — see citations below.
- **Recommended severity:** **`warning`** (advisory tier — does NOT bump exit code), with a **structured `security` reference field** carrying `CWE-427` for the cwd/empty/relative case. This is the milestone's first sub-actionable severity. Duplicates/shadows stay **actionable** (exit 3).
- **Why warning, not error:** every authoritative tool treats relative/cwd-in-PATH as an *audit finding* a user may have chosen deliberately (Lynis = warning/suggestion, CIS = L1 with "correct or justify", Go = opt-in policy). It is real but not unconditionally wrong → degrading exit-3 would corrupt the agent signal. This is exactly the stance already recorded in PROJECT.md Key Decisions.
- **Duplicate PATH** stays an actionable issue and keeps its existing name `duplicate_path`. The convention precedent is zsh's own `typeset -U path` (dedup keeps first occurrence) — so our canonicalization should **report the later/duplicate occurrence** and treat notation-equivalents as the same entry.
- **Critical anti-features:** do NOT resolve `~`/`$HOME`/`..`/symlinks against the live environment or filesystem (breaks determinism + read-only purity); do NOT flag relative entries as *errors*; do NOT try to judge directory permissions/ownership/existence (that is Lynis/CIS territory and needs a live filesystem we deliberately don't touch); do NOT re-implement ShellCheck SC2123 (accidental-clobber heuristic — different problem, false-positive prone here).

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features any "PATH analyzer" is assumed to have. Missing these = the tool looks broken or naive.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Correct per-entry extraction** (split assignment value on `:`, drop the `$PATH`/`$path` self-reference, keep entries verbatim) | Every PATH tool that reports entries must name them correctly. Today's bug (`./scripts` → `/scripts`) makes the tool actively wrong, which is worse than silent. | LOW–MEDIUM | The fix the milestone is built around. Splitting on `:` is the universal model (POSIX colon-separated list). Empty fields between colons are *real entries* (= cwd), so do **not** drop empties during split — they are findings. |
| **Duplicate-entry detection across notations** | Users expect "duplicate" to mean "same directory," not "byte-identical string." zsh ships `typeset -U path` precisely because duplicate PATH entries are a recognized nuisance. | MEDIUM | Canonicalize notation only: `~` ≡ `$HOME` ≡ `${HOME}`; collapse `//` → `/`; strip trailing `/` (except root `/`). Compare canonical forms. Precedent: `typeset -U` keeps the **first** occurrence → report the **second+** as the duplicate. Already partially built (buggy); this is a correctness + dedup-key upgrade, not a new feature category. |
| **Flag current-directory-in-PATH** (`.`, empty field, leading/trailing/double colon) | This is THE canonical PATH foot-gun, documented since Grampp & Morris (1984). CWE-427, CIS, Lynis, UPenn CETS, and Go's stdlib all single it out. A PATH tool that misses it looks unserious. | MEDIUM | Empty PATH field is *equivalent to* `.` — confirmed by CWE-427, UPenn CETS, and POSIX semantics. So leading colon (`:/bin`), trailing colon (`/bin:`), and double colon (`/bin::/sbin`) all = cwd entries and must be detected. This is the highest-value finding in the milestone. |
| **Human + `--json` parity for the new finding** | The product's core value is a trustworthy `--json` envelope. A finding only in human output would break the agent contract. | LOW | New issue kind must flow through `model.Issue` → `dto`. Reuse existing issue plumbing; just add the `Kind` + severity field. |
| **A severity that does not bump exit code** | Agents rely on exit 3 = "actionable problem." A deliberate `.`-in-PATH or relative entry must surface without poisoning that signal. | MEDIUM | New `Issue.Severity` field. Exit code derives only from actionable-tier issues. This is the first severity tier and also seeds the deferred classifier-precision work (PROJECT.md). |

### Differentiators (Competitive Advantage)

Where zsh-pro can beat the existing ecosystem. The ecosystem is bifurcated: **shell linters (ShellCheck/shfmt)** lint *script syntax* and have **no relative/cwd-PATH check at all**; **security auditors (Lynis/CIS)** check the *live system root PATH* against the *real filesystem*. Nobody does **static, deterministic, per-statement PATH-hygiene on a config file with trustworthy line numbers and a machine envelope.** That gap is the differentiator.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Static config-file PATH hygiene (no live env/FS)** | ShellCheck won't tell you `.` is in your PATH; Lynis only checks the *running* root PATH against disk. zsh-pro flags it from the `~/.zshrc` source itself, deterministically, before it ever runs. | MEDIUM | This is the whole niche. Determinism (no `$HOME`/FS resolution) is the feature, not a limitation — see anti-features. |
| **Notation-equivalent dedup as a first-class finding** | `typeset -U` silently fixes dupes at runtime; ShellCheck ignores them. Reporting `~/bin` and `$HOME/bin` as the *same* duplicate entry is genuinely more than either tool offers. | MEDIUM | The notation-canonicalization (string-level) is the differentiating cleverness. Keep it conservative (see anti-features) to stay false-positive-free. |
| **CWE-tagged security reference on the cwd finding** | Emitting `CWE-427` in `--json` lets downstream agents/security tooling map the finding to a known weakness taxonomy. Lynis does this conceptually; almost no shell-config tool does. | LOW | Just a string field (`"security": "CWE-427"`). Cheap, high-credibility. Cite CWE-427 (cwd/empty/relative) — NOT CWE-426 (that is *attacker-controlled* paths, the wrong fit; see §Severity). |
| **Trustworthy line attribution for each PATH finding** | Built directly on the v1.0 line-number guarantee. "`.` in PATH at line 42" is actionable; "somewhere in your PATH" is not. | LOW | Inherits from v1.0; the new finding just needs to carry the statement line like existing issues. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Resolve `~`/`$HOME`/`$VAR` against the live environment** | "Catch the dupe even when one side uses a literal `/Users/me`." | Makes a read-only static analyzer **env-dependent and non-deterministic**; same config yields different findings per machine/user. Directly violates PROJECT.md Out-of-Scope. Also creates the "`$HOME` reassigned mid-file" false positive. | **Notation-only** canonicalization (string-level `~`≡`$HOME`≡`${HOME}`). Never read the actual env. |
| **Resolve `..`, symlinks, or check the dir exists / is writable / ownership / mode** | "Lynis/CIS flag world-writable + non-existent PATH dirs — do that too." | Requires a **live filesystem** the tool deliberately doesn't touch; results vary by machine; this is OS-audit scope (Lynis/CIS), not config-static scope. High false-positive rate (dirs created later, per-host dirs). | Stay string-static. If desired later, that's a separate *system-audit* mode, explicitly out of this milestone. Cite the boundary: CIS "Ensure root PATH Integrity" needs `stat`; we intentionally don't. |
| **Flag relative/cwd entries as an ERROR (exit 3)** | "It's a security risk, make it loud." | Relative/cwd-in-PATH is frequently **intentional** (dev tooling, `node_modules/.bin` patterns, sandboxes). Every authority treats it as audit/advisory, not a hard failure (Lynis=warning, CIS=L1 "correct or justify", Go=opt-in). Exit-3 conflation corrupts the agent signal. | **Advisory severity (`warning`)** that does not bump exit code. Loud in output, neutral in exit code. (This is the milestone's design decision.) |
| **Re-implement ShellCheck SC2123 (accidental PATH clobber)** | "ShellCheck warns about `PATH=...`, we should too." | SC2123 is a *different* check — it heuristically guesses you meant a lowercase var when you assign a **single, separator-less** value to `PATH`. In a `.zshrc`, deliberately *setting* `PATH=...:$PATH` is normal and correct; porting SC2123 here would fire constantly. False-positive magnet. | Out of scope. zsh-pro's job is entry-level hygiene (dupes, relative/cwd), not "did you mean a different variable name." |
| **Heuristically guess "intent" / suppress `.` at end-of-PATH as safe** | "`.` at the end is 'slightly safer,' so don't flag it." | "Slightly safer" still executes attacker code on a typo (UPenn CETS, Go blog both say end-placement does **not** eliminate risk). Position-based suppression adds complexity and gives false reassurance. | Flag **all** cwd entries uniformly; optionally note position in the message ("at end of PATH" is informational, not exculpatory). Keep the rule simple and honest. |
| **PATH ordering / precedence ("entry X shadows entry Y")** | "Tell me which dir wins." | Genuinely useful but a *separate* order-sensitivity feature with its own model; bundling it bloats a correctness milestone. Already in PROJECT.md Out-of-Scope. | Defer to a dedicated order-sensitivity phase. This milestone = extraction + dedup + relative/cwd advisory only. |

---

## Recommended Advisory: Name + Severity (Justified, Not Invented)

### Name

**Issue kind / `--json` `name`:** `relative_path_entry`
**Human label:** "relative PATH entry" — with the cwd subset surfaced as **"current directory in PATH"**.

Grounding (conventional terms a user already recognizes):

| Source | Term it uses | Citation |
|--------|--------------|----------|
| Lynis (de-facto Linux audit tool) | **"Found relative path in PATH"** / "Suspicious location in PATH discovered" (warning) | github.com/CISOfy/lynis (binaries check) |
| CIS Benchmarks | **"Ensure root PATH Integrity"** — enumerates empty (`::`), trailing colon (`:`), **current working directory (`.`)** | CIS_* "Ensure root PATH Integrity" (L1) |
| CWE-427 (MITRE 4.20) | **"current working directory ... an untrusted search element"**; "empty element in the PATH" | cwe.mitre.org/data/definitions/427.html |
| UPenn CETS | "`.` in your `$PATH`"; empty dir name "equivalent" to `.`; leading/trailing `:` same as `.` | cets.engineering.upenn.edu/answers/dot-path.html |
| Go stdlib security policy | **"relative"/"current directory"**, error: *"resolves to executable in current directory"* | go.dev/blog/path-security |

**Why `relative_path_entry` as the umbrella, with cwd as a sub-case:** "relative" is the broadest accurate term (covers `./scripts`, `scripts`, `bin/x`, `..`, AND bare `.`/empty). Every non-absolute (non-`/`-rooted) entry is the foot-gun class. The empty/`.`/colon cwd case is the most dangerous specialization and deserves a distinct message string ("current directory in PATH") and the CWE-427 tag, but it's the same issue kind. This keeps the model simple (one new `Kind`) while letting the *message* + *security ref* distinguish severity-of-explanation.

> Naming caution: avoid "insecure PATH" / "untrusted PATH" as the primary label — it overclaims (a relative entry isn't necessarily exploited) and collides with CWE-426 ("untrusted search path" = *attacker-controlled*, the wrong weakness). Use "relative PATH entry" + a CWE-427 reference field for the security framing.

### Severity

**Recommended:** **`warning`** advisory tier — surfaces in output, does **not** bump the exit code (exit 3 stays reserved for `duplicate_*`/`shadowed`).

Grounding for "not an error":

- **Lynis** reports relative-path-in-PATH as a **warning/suggestion**, not a failure that aborts the audit.
- **CIS "Ensure root PATH Integrity"** is **Level 1** and phrased as "correct **or justify**" — i.e. it may be intentional; it's a review item, not an absolute.
- **Go** made cwd-in-PATH rejection **opt-in / policy-scoped** (the `go` command itself + `x/sys/execabs`), *not* a blanket runtime error for all programs — an explicit acknowledgement that relative/cwd entries are sometimes legitimate.
- **CWE-427** lists the consequence as "Execute Unauthorized Code or Commands" but assigns **no CVSS score / no fixed likelihood** — confirming it is context-dependent, not unconditionally critical.

So: real enough to always surface, conditional enough that forcing exit-3 would produce false alarms and corrupt the agent contract. → **advisory `warning`**, plus `security: "CWE-427"` on the cwd/empty/relative finding for machine-readable weakness mapping.

Severity ladder this establishes (maps onto the existing exit-code contract):

| Tier | Issue kinds | Exit impact |
|------|-------------|-------------|
| **actionable** (existing) | `duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed` | bumps to exit 3 |
| **warning / advisory** (NEW) | `relative_path_entry` (incl. cwd/empty) | no exit-code change |

> Severity-vs-duplicate, explicitly: a **duplicate** PATH entry is a *correctness/cleanliness* problem (same as zsh's `typeset -U` target) → stays **actionable**. A **relative/cwd** entry is a *security-shaped advisory* the user may have chosen → **warning**. They are different severities by design, and this asymmetry is defensible: CIS/Lynis treat the security finding as "review/justify," while a redundant duplicate is an unambiguous tidy-up.

---

## Expected User-Facing Behavior (What Each Finding Should Say)

| Finding | Trigger | Human message (shape) | `--json` shape (illustrative) |
|---------|---------|------------------------|-------------------------------|
| **Duplicate PATH entry** (`duplicate_path`, actionable) | Two entries canonicalize to the same dir (incl. `~`≡`$HOME`, slash-normalized) | `duplicate PATH entry: $HOME/bin (also at line N as ~/bin)` | `{ "name": "duplicate_path", "line": M, "severity": "actionable" }` |
| **Relative PATH entry** (`relative_path_entry`, warning) | Entry not rooted at `/` and not the cwd special-case (`./scripts`, `scripts`, `../x`) | `relative PATH entry: ./scripts — resolves against the current directory at runtime` | `{ "name": "relative_path_entry", "line": M, "severity": "warning" }` |
| **Current directory in PATH** (`relative_path_entry`, warning, cwd sub-case) | Entry is `.`, empty field, leading/trailing/double colon | `current directory in PATH (empty entry) — executes commands from wherever you cd; see CWE-427` | `{ "name": "relative_path_entry", "line": M, "severity": "warning", "security": "CWE-427" }` |

Notes on message content (kept honest per anti-features):
- Never assert the entry *is* exploited — describe the mechanism ("resolves against the current directory at runtime"). Matches CWE-427/UPenn framing.
- For the empty-field case, say "empty entry" explicitly so the user understands a stray `:` is the cause — this is the non-obvious one (CWE-427 + UPenn both stress empty≡`.`).
- Do not editorialize about end-of-PATH being "safe" (Go/UPenn: it isn't).

---

## Feature Dependencies

```
Correct per-entry extraction (split on ':', keep empties, drop $PATH self-ref)
    └──requires──> v1.0 trustworthy line numbers  (each entry's finding needs the statement line)
    └──enables──>  Notation-equivalent dedup        (can't dedup entries you mis-extract)
    └──enables──>  Relative/cwd advisory            (can't classify entries you mis-extract)

Notation-only canonicalization (~≡$HOME≡${HOME}, slash-normalize)
    └──feeds──>    duplicate_path dedup key

Issue.Severity field (NEW)
    └──requires──> exit-code derivation reads only the 'actionable' tier
    └──enables──>  relative_path_entry surfaces without exit-3
    └──seeds──>    deferred classifier-precision work (PROJECT.md)

relative_path_entry (new Kind)
    └──requires──> model.Issue + dto plumbing  (existing)
    └──requires──> Issue.Severity field        (NEW, above)

Coverage (golden fixtures + testgen oracle)
    └──requires──> all of the above           (you pin behavior after it exists)
```

### Dependency Notes

- **Extraction is the keystone.** Both new capabilities (dedup, relative/cwd advisory) are *classifications of correctly-split entries*. The extraction fix must land first; dedup and the advisory build on its output. Critically, the splitter must **keep empty fields** (they are the cwd findings) while dropping only the literal `$PATH`/`$path`/`${PATH}` self-reference token.
- **Severity field gates the advisory's exit-code neutrality.** `relative_path_entry` cannot ship "correctly" (without bumping exit 3) until `Issue.Severity` exists and the exit-code derivation is updated to count only actionable-tier issues. Build the severity field in the same phase as (or just before) the advisory.
- **Canonicalization is shared but scoped narrowly.** The same notation-normalizer feeds the dedup key; keep it string-only so it can't introduce env/FS dependence into *either* consumer.
- **Coverage pins last.** The testgen oracle extension (relative/unrooted dup paths) and the `duplicate_path`/`shadowed` golden fixtures are the regression pin — they assert the above once it's implemented, consistent with the milestone's TDD constraint.

---

## MVP Definition

### Launch With (v1.1 — this milestone)

- [ ] **Correct per-entry extraction** — split PATH-family value on `:`, drop only the self-reference, keep empties; fixes `./scripts`→`/scripts` and the unrooted blind spot. *(Keystone — everything depends on it.)*
- [ ] **Notation-only canonicalization + `duplicate_path` upgrade** — `~`≡`$HOME`≡`${HOME}`, slash-normalize; report the later duplicate; actionable tier. *(Closes the existing buggy dedup.)*
- [ ] **`Issue.Severity` field + exit-code derivation update** — advisory tier exists; exit 3 counts only actionable issues. *(Gates the advisory.)*
- [ ] **`relative_path_entry` advisory (warning)** including the cwd sub-case (`.`/empty/leading/trailing/double colon) with `security: CWE-427`. *(Highest-value new finding.)*
- [ ] **Coverage** — golden fixtures for `duplicate_path` + `shadowed`; corpus asserts `issue_names`/`issue_lines`; testgen oracle extended to relative/unrooted dup paths. *(Regression pin; milestone exit criterion.)*

### Add After Validation (v1.x)

- [ ] **System-audit mode (live FS/env)** — *only if* users ask to also validate the running PATH against disk (dir exists / writable / ownership / mode), explicitly as a separate, non-deterministic mode. Trigger: repeated user requests + acceptance of env-dependence. (This is Lynis/CIS scope.)
- [ ] **PATH ordering / precedence analysis** — "entry X shadows entry Y." Trigger: a dedicated order-sensitivity milestone (already noted Out-of-Scope here).
- [ ] **Configurable severity / suppression** (e.g. `# zsh-pro:allow relative-path`) — Trigger: false-positive complaints about intentional relative entries.

### Future Consideration (v2+)

- [ ] **Multi-file / fragment PATH tracing** (`.zshenv` + `.zshrc` + `path_helper`) — defer until single-file analysis is proven; macOS `path_helper` interaction is a known mess.
- [ ] **`fix`/`doctor` auto-remediation** (e.g. emit `typeset -U path`, strip `.`) — defer; product ramp, and write-mode contradicts the read-only core until a safety net exists.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Correct per-entry extraction | HIGH | LOW–MEDIUM | P1 |
| Notation-only canonicalization + `duplicate_path` upgrade | MEDIUM | MEDIUM | P1 |
| `Issue.Severity` field + exit-code update | HIGH (unblocks advisory + agent signal) | MEDIUM | P1 |
| `relative_path_entry` advisory (incl. cwd, CWE-427 tag) | HIGH | MEDIUM | P1 |
| Coverage (fixtures + oracle extension) | HIGH (regression pin) | MEDIUM | P1 |
| `security: CWE-427` reference field | MEDIUM | LOW | P1 (rides with advisory) |
| System-audit mode (live FS) | MEDIUM | HIGH | P3 (anti-feature for now) |
| PATH ordering/precedence | MEDIUM | HIGH | P3 |

All MVP items are P1 because the milestone is a tightly-scoped correctness pass; there is no "P2 within this milestone" — items are either in the correctness pass or explicitly deferred.

---

## Competitor Feature Analysis

| Capability | ShellCheck (linter) | shfmt (formatter) | Lynis / CIS (system auditors) | zsh `typeset -U` (runtime) | **zsh-pro (our approach)** |
|-----------|---------------------|-------------------|-------------------------------|----------------------------|----------------------------|
| Per-entry PATH extraction from a config file | No (lints script syntax, not PATH contents) | No (formats only) | No (reads *live* `$PATH` of running user) | N/A (runtime dedup) | **Yes — static, per-statement, with line numbers** |
| Duplicate PATH entry detection | No | No | Partial (root PATH only, live) | Yes (silently drops dupes at runtime, keeps first) | **Yes — static, notation-aware, reports the duplicate (`duplicate_path`, actionable)** |
| Relative / cwd / empty PATH entry flag | **No** (SC2123 is *accidental clobber*, unrelated) | No | **Yes** (Lynis: "relative path in PATH" warning; CIS: "Ensure root PATH Integrity") — but live system only | No | **Yes — static from config (`relative_path_entry`, warning, CWE-427)** |
| Accidental PATH-clobber heuristic | **Yes (SC2123)** | No | No | No | **No — deliberately out of scope (FP-prone here)** |
| Deterministic (no live env/FS) | Yes (static) | Yes | **No** (depends on running system) | No (runtime) | **Yes — notation-only, no env/FS resolution** |
| Machine-readable envelope with weakness tag | Yes (SARIF, but no PATH-content checks) | No | Partial (Lynis report files) | No | **Yes — `--json` with `severity` + `security: CWE-427`** |

Reading: the existing tools split cleanly into "static linters that ignore PATH *contents*" and "live system auditors that need the running machine." zsh-pro occupies the empty intersection — **static, deterministic, config-file PATH-content hygiene with a machine envelope** — which is the milestone's differentiator. Our naming/severity deliberately mirror Lynis + CIS + CWE-427 so the output reads as familiar to anyone who has run those tools.

---

## Sources

ShellCheck (establishes SC2123 scope = accidental clobber, NOT relative/cwd → confirms the gap we fill):
- ShellCheck SC2123 wiki — "`PATH` is the shell search path. Use another name." (warning): https://www.shellcheck.net/wiki/SC2123
- ShellCheck wiki sitemap / checks index (no relative-or-cwd-in-PATH check exists): https://www.shellcheck.net/wiki/ ; https://gist.github.com/nicerobot/53cee11ee0abbdc997661e65b348f375
- SC2155 (separate; declare/assign) — context that ShellCheck's PATH-adjacent checks are about assignment mechanics, not entry hygiene: https://www.shellcheck.net/wiki/SC2155

CWE (severity + the empty-element=cwd semantics + correct weakness choice CWE-427 over CWE-426):
- CWE-427 Uncontrolled Search Path Element (4.20) — empty PATH element = current working directory = "untrusted search element"; consequence "Execute Unauthorized Code or Commands"; no fixed CVSS: https://cwe.mitre.org/data/definitions/427.html
- CWE-426 Untrusted Search Path (4.20) — *attacker-controlled* search path (the weakness we should NOT cite for relative entries): https://cwe.mitre.org/data/definitions/426.html
- CWE-426 vs 427 distinction (peer weaknesses, "often confused"): https://pentest.y-security.de/CWE/CWE-427/

Conventional terminology + severity precedent in real tools:
- Lynis (CISOfy) — "relative path in PATH" / "Suspicious location in PATH discovered" (warning), reads live system PATH: https://github.com/CISOfy/lynis ; https://cisofy.com/documentation/lynis/
- CIS Benchmark "Ensure root PATH Integrity" (L1) — enumerates empty (`::`), trailing colon (`:`), current working directory (`.`); requires absolute paths; "correct or justify": https://www.tenable.com/audits/items/CIS_Red_Hat_EL8_Server_v3.0.0_L1.audit:da48f0ebd62095fa880efca1aae9c673
- UPenn CETS "What's wrong with having '.' in your $PATH?" — empty dir name "equivalent" to `.`; leading/trailing colon same; end-placement does not eliminate risk: https://cets.engineering.upenn.edu/answers/dot-path.html

Go stdlib security policy (directly relevant precedent — zsh-pro is a Go tool; Go treats cwd-in-PATH as a security issue but opt-in, not blanket error):
- "Command PATH security in Go" — `os/exec` returns error for cwd-resolved executables; `golang.org/x/sys/execabs`; Go 1.16 security release: https://go.dev/blog/path-security

zsh duplicate-PATH convention (precedent that dedup keeps the first occurrence):
- `typeset -U path` deduplication (keeps first occurrence): https://tech.serhatteker.com/post/2019-12/remove-duplicates-in-path-zsh/ ; https://til.hashrocket.com/posts/7evpdebn7g-remove-duplicates-in-zsh-path

---
*Feature research for: PATH-hygiene findings (zsh-pro v1.1 Trustworthy PATH Analysis)*
*Researched: 2026-06-24*
