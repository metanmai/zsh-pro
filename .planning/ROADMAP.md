# Roadmap: zsh-pro

## Milestones

- ✅ **v1.0 Trustworthy Line Numbers** — Phase 1 (shipped 2026-06-24) — full detail: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- 🚧 **v1.1 Trustworthy PATH Analysis** — Phases 2-4 (in progress)

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

<details>
<summary>✅ v1.0 Trustworthy Line Numbers (Phase 1) — SHIPPED 2026-06-24</summary>

- [x] Phase 1: Trustworthy Line Numbers (3/3 plans) — completed 2026-06-24
  - 01-01 Off-by-one line count (LINE-01) · 01-02 Comment-offset attribution (LINE-02) · 01-03 Pin both fixes (PIN-01/02)

</details>

### 🚧 v1.1 Trustworthy PATH Analysis (In Progress)

**Milestone Goal:** Every PATH entry `analyze` reports is named correctly, genuine duplicates are caught across notations, and risky relative entries are flagged — without polluting the exit-code signal.

- [ ] **Phase 2: Issue Severity Tier** - Add a per-issue severity so advisories surface without bumping the exit code (exit 3 stays reserved for genuine problems)
- [ ] **Phase 3: Trustworthy PATH Extraction & Detection** - Replace the regex with AST-based split extraction, notation-only dedup, and a relative/unrooted advisory
- [ ] **Phase 4: PATH Coverage & Oracle Pin** - Golden fixtures for the untested issue kinds, corpus name/line/severity assertions, and the independent oracle as the regression pin

## Phase Details

### Phase 2: Issue Severity Tier

**Goal**: Every issue carries a severity, and only `actionable` issues drive the exit code and `issues_found` flag — so an advisory-only config exits 0 while genuine problems still exit 3. This is the additive vertical slice (model → dto → both renderers → exit-code) that Phases 3 and 4 build on.
**Depends on**: Phase 1 (shipped)
**Requirements**: SEV-01, SEV-02
**Success Criteria** (what must be TRUE):
  1. Every issue in `--json` and the human report carries a self-describing severity string (`"actionable"` or `"advisory"`), present on every issue object.
  2. The four existing issue kinds (`duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed`) are byte-identical in behavior — they remain `actionable` (the zero value) and still drive exit 3, with no edits to their construction sites.
  3. A config whose only finding is an advisory exits `0` with `issues_found: false`; a config with any actionable issue exits `3` with `issues_found: true`; the two fields never disagree.
  4. `analyze --json` still emits exactly one JSON object on stdout on both the success and `fail` paths (the agent contract holds with the new field threaded through `toDTO`).
**Plans**: 2 plans
Plans:
- [ ] 02-01-PLAN.md — Severity type + zero-value-actionable model foundation; HasActionableIssues() + actionable-only ExitCode() (SEV-01/SEV-02)
- [ ] 02-02-PLAN.md — Thread severity onto the JSON wire (non-omitempty) + actionable-only issues_found; severity-aware human report marker & advisory tally (SEV-01/SEV-02)
**UI hint**: no

> **Open question to resolve in this phase (do NOT silently resolve):** Confirm with the user that redefining `issues_found` to mean "actionable issues present" is an acceptable `analyze --json` wire-contract change. An advisory-only envelope flips from `issues_found:true / exit_code:0` to `issues_found:false / exit_code:0`. Research recommends aligning the two (decision #4) but flags it as a user-confirm gate before this phase ships.
>
> **Design prerequisite carried forward to Phase 3/4:** The oracle's two-field path-node split (rendered notation vs. independent dedup key, Phase 4) MUST be designed before the Phase 3 canonicalizer is written — retrofitting oracle independence is the single highest recovery cost in the research (Pitfall #8). Record the intended split now so Phase 3 honors it.

### Phase 3: Trustworthy PATH Extraction & Detection

**Goal**: Replace the broken `pathSegRe` regex with structural, split-based PATH extraction so every entry is named exactly as written, notation-equivalent duplicates collapse to one finding, and relative/unrooted/cwd entries surface as a new `relative_path_entry` advisory. Stays shell-free in `core/analyze`; the AST-specific extraction lives behind the `shell.Provider` seam.
**Depends on**: Phase 2 (the `relativePaths` detector sets `SevAdvisory`; the new `IssueRelativePath` kind needs the `Severity` field to exist)
**Requirements**: PATH-01, PATH-02, PATH-03
**Success Criteria** (what must be TRUE):
  1. A relative entry is reported verbatim — `./scripts` is reported as `./scripts`, never `/scripts` — and an unrooted entry (`bin`, `scripts`) is detected; the `$PATH`/`${PATH}` self-reference is excluded and surrounding quotes trimmed.
  2. Notation-equivalent entries (`~` ≡ `$HOME` ≡ `${HOME}`, trailing/duplicate slashes normalized) collapse to a single `duplicate_path` whose `name` is verbatim and whose `lines` list every adding line — with no filesystem or live-`$HOME` access (analysis stays byte-identical across two `$HOME` values; `~user` stays distinct).
  3. Relative/unrooted entries — `./x`, `../x`, bare words, bare `.`, and empty entries (leading/trailing/`::` colon) — surface as a `relative_path_entry` advisory; the `.`/empty current-directory cases carry a CWE-427 security reference.
  4. The advisory never bumps the exit code: an advisory-only config exits 0, a genuine duplicate exits 3, and a mixed config exits 3 (the Phase 2 severity gate holds end-to-end).
  5. `pathSegRe` is deleted and no `regexp` runs over `Block.Text` for PATH extraction; entries come from parsed assignment structure.
**Plans**: TBD
**UI hint**: no

> **Open questions to resolve in this phase (do NOT silently resolve):**
> - **Extraction location** (research open question #1): provider populates an agnostic `[]string` on `model.Block` (preferred — keeps `core/analyze` shell-free and the `mvdan.cc/sh` import behind the seam) vs. reconciler-local split over a provider-exposed RHS (accepted layering smell). Word-reconstruction for brace-aware splitting is zsh-AST-specific and argues for the seam regardless.
> - **Intra-statement duplicates** (`PATH="/a:/a:$PATH"`, research open question #2): pick one of {report with a repeated line / dedup the line list / ignore intra-statement repeats}, document it as a contract decision in PROJECT.md, and fixture it. Cross-statement dedup is already settled.
> - **`CatPath` array-form classification** (Pitfall #12 — a *seam check*, not a classifier rewrite): verify `path=(...)`, `path+=(...)`, `typeset -U path`, `fpath=(...)` each classify `CatPath` before relying on the reconciler. If any miss, the minimal in-scope fix is recognizing the path-family lowercase names — without touching the out-of-scope over-capture overhaul.
>
> **Research flag:** the `${PATH:+:$PATH}` / `${VAR:-/a:/b}` brace-aware `:`-splitting (Pitfall #2 — the single most likely correctness regression) and the full enumeration of assignment shapes (scalar / array / `+=` / `typeset -U`, Pitfall #9) are the trickiest parts. A `--research-phase` pass is optional but reasonable for the brace-depth scanner design.
>
> **Must avoid** (Pitfalls): #1 regex re-extraction, #2 `:`-split inside `${...}`, #3 filesystem/`$HOME` resolution, #4 named-tilde folding, #5 empty/cwd elision, #10 self-reference over/under-dropping (`$PATH_BACKUP` must survive), #11 slash/root over-collapse (`/` stays `/`), #13 cross-line dedup + LINE-02 attribution (a PATH issue under a leading comment must report the statement line).

### Phase 4: PATH Coverage & Oracle Pin

**Goal**: Prove Phases 2+3 end-to-end with real fixtures and an independent oracle. Add golden fixtures for the two previously-untested issue kinds, assert names/lines/severity in the corpus, and extend the testgen oracle to generate and independently predict relative, unrooted, and notation-equivalent duplicate PATH entries — with canonicalization re-implemented locally so the property test stays non-circular. This is the milestone's TDD success gate.
**Depends on**: Phase 2 (model symbols `Severity`/`IssueRelativePath`) and Phase 3 (the engine must actually produce the new/corrected issues to assert against)
**Requirements**: COV-01, COV-02, COV-03
**Success Criteria** (what must be TRUE):
  1. The golden corpus includes fixtures exercising the previously-uncovered `duplicate_path` and `shadowed` issue kinds.
  2. Each corpus fixture asserts `issue_names`, `issue_lines`, and `severity` — not just the issue `Kind`.
  3. The testgen oracle generates and independently predicts relative, unrooted, and notation-equivalent duplicate PATH entries plus the advisory, using a two-field path node (rendered notation vs. independent dedup key) and a testgen-local canonicalizer — `core/testgen` imports only `core/model`, never the engine's canonicalizer.
  4. The 10-seed property test passes and bites: a deliberately-broken canonicalizer fails the test, and the advisory is pinned as exit-code-neutral at the oracle level (proving the pin is non-circular).
**Plans**: TBD
**UI hint**: no

> **CRITICAL constraint:** `core/testgen` re-implements canonicalization locally and MUST NOT import `core/analyze` or `core/shell` (enforce via a grep/import test). The two-field node split must already be designed (see Phase 2/3 notes) — retrofitting it here is the highest recovery cost in the research (Pitfall #8, HIGH).
>
> **Must avoid** (Pitfalls): #7 DTO/agent-contract coverage (one JSON object on stdout for clean/dup-only/advisory-only/`fail`), #8 circular oracle, #14 golden byte-stability (nil-vs-`[]` `Issues`, deterministic sort placement of the new kind).

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Trustworthy Line Numbers | v1.0 | 3/3 | Complete | 2026-06-24 |
| 2. Issue Severity Tier | v1.1 | 0/TBD | Not started | - |
| 3. Trustworthy PATH Extraction & Detection | v1.1 | 0/TBD | Not started | - |
| 4. PATH Coverage & Oracle Pin | v1.1 | 0/TBD | Not started | - |
