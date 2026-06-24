# Pitfalls Research

**Domain:** zsh-config static analyzer — PATH parsing + notation-only canonicalization + new Issue severity tier (zsh-pro milestone v1.1 "Trustworthy PATH Analysis")
**Researched:** 2026-06-24
**Confidence:** HIGH (zsh PATH semantics reproduced locally in zsh 5.9 and cross-checked against the official zsh Expansion manual; codebase seams read directly from source)

> Scope note: this milestone replaces `reconciler.duplicatePaths`' regex `(?:\$HOME|~|/)[^:"'\s)]+` (which anchors on a rooting char and therefore mis-reads `./scripts` as `/scripts` and misses unrooted entries entirely), adds notation-only canonicalization for dedup, adds a relative/unrooted advisory, and introduces the first `Issue` severity tier — all with **no new dependencies** and **no filesystem/live-`$HOME` resolution**.
>
> Pitfalls below are ordered Critical → Moderate → Minor. Phases referenced (Extraction, Canonicalization, Advisory + Severity, Coverage/Oracle) match the v1.1 target-feature breakdown in PROJECT.md; the roadmap will assign final numbers.

## Critical Pitfalls

### Pitfall 1: Re-extracting PATH with another regex over `Block.Text` (repeating the original bug)

**What goes wrong:**
The instinct is to "improve the regex" — e.g. extend `pathSegRe` to also match unrooted words. But the entire failure mode is that a *regex over raw text* cannot know where the assignment value starts/ends, what is quoted, or what the `:` separators are. Any regex inherits the same class of bugs: it will still scoop tokens out of comments, out of unrelated commands on the same `Block.Text`, and out of the `$PATH` self-reference; it cannot reliably split `:`; and "match an unrooted word" makes it match almost everything (`PATH`, `export`, `bin`).

**Why it happens:**
`reconciler.duplicatePaths` already takes a regex-over-`b.Text` shape, so the path of least resistance is to edit the pattern in place. The current `Block.Text` also *includes pulled-up leading comments* (`parse.go:38-42` widens `start` to the comment offset), so a regex sees comment bytes too.

**How to avoid:**
Extract from **structure, not text**. The parser already produces `*syntax.Assign` nodes (see `describe` handling `CallExpr.Assigns` and `DeclClause.Args`). The fix is to (a) identify PATH-family assignments by *name* (`PATH`/`FPATH`/`MANPATH`/`CDPATH`/`INFOPATH`/…), (b) take the assignment's **value word(s) only**, (c) reconstruct the literal value (concatenating `*Lit` parts, recognizing `$PATH`/`${PATH}` parameter parts as the self-reference), then (d) split that one string on `:` verbatim. This requires surfacing the assignment value to the reconciler — either a new structural field on `model.Block` (e.g. `PathEntries []string` filled by the zsh provider during `describe`) or a small parser-side extraction. Keep the split logic shell-agnostic in the reconciler if the raw value string is available; keep word-reconstruction (which is zsh-AST-specific) behind the `shell` seam.

**Warning signs:**
- The new code calls `regexp` over `b.Text` at all.
- A duplicate is reported for a directory that only appears inside a comment.
- `export` or `PATH` itself shows up as an "entry."

**Phase to address:** Extraction (first PATH phase)

---

### Pitfall 2: Splitting on `:` *inside* a parameter expansion (`${PATH:+:$PATH}`, `${VAR:-/a:/b}`)

**What goes wrong:**
The classic prepend idiom is `export PATH="${PATH:+$PATH:}/new"` or `export PATH="$HOME/bin${PATH:+:$PATH}"`. A naive `strings.Split(value, ":")` shreds the `:` that lives **inside** `${PATH:+...}` (the `:+`, `:-`, `:=`, `:?` operators and any literal `:` in their word), producing garbage "entries" like `${PATH`, `+`, `$PATH}`. Entries containing a `:` inside `${...}` (e.g. `${FOO:-/a:/b}`) are mis-split the same way.

**Why it happens:**
PATH values are *colon-separated*, so "split on `:`" feels correct — but that is only true at the **top level** of the value, not inside brace expansions. zsh applies the colon-list semantics to the assignment RHS as a whole; the `:` inside `${...}` is part of an expansion operator, not a list separator.

**How to avoid:**
Do not naively `strings.Split`. Two acceptable strategies, in order of preference:
1. **Split on the parsed structure.** Walk the value `*syntax.Word.Parts`: each `*syntax.Lit` part can contain top-level `:` separators (split those); each `*syntax.ParamExp` / `*syntax.DblQuoted` part is an opaque unit that contributes to whichever entry it is glued to (no splitting inside it). A `$PATH`/`${PATH}` ParamExp that stands alone as a whole entry is the self-reference — drop it.
2. If splitting a reconstructed string, treat `${...}` (and `$(...)`, `"..."`, `'...'`) as **brace/quote-aware regions** that suppress the separator — i.e. a hand-written scanner that only splits on `:` at brace/quote depth 0.

Add fixtures for every idiom: `"${PATH:+$PATH:}/x"`, `"$HOME/bin${PATH:+:$PATH}"`, `"${FOO:-/a:/b}"`, `"${PATH}"` alone.

**Warning signs:**
- Reported entries contain `{`, `}`, `+`, `-`, `:+`, or a lone `$PATH`.
- Entry count for a known idiom is higher than the number of real directories.
- A self-referential `${PATH:+:$PATH}` produces a spurious empty or `$PATH`-named entry.

**Phase to address:** Extraction (first PATH phase) — this is the single most likely correctness regression.

---

### Pitfall 3: Resolving the filesystem or live `$HOME` during canonicalization (violates the read-only/deterministic constraint)

**What goes wrong:**
"Canonicalize" tempts you toward `filepath.Abs`, `filepath.EvalSymlinks`, `os.Getenv("HOME")`, `os.UserHomeDir()`, or `filepath.Clean` with `..` collapsing. Any of these makes the analyzer's output depend on the machine it runs on and on the live environment — the same `.zshrc` analyzed on two machines (or by an agent in CI vs. locally) would report different duplicates. It also breaks the explicit Out-of-Scope boundary in PROJECT.md and would re-introduce the very `$HOME`-reassigned-mid-file false positive the milestone is trying to avoid.

**Why it happens:**
`path/filepath` is right there and `Clean`/`Abs`/`EvalSymlinks` look like "the standard way to normalize a path." The word "canonicalize" reads like "make filesystem-canonical."

**How to avoid:**
Canonicalization is **string/notation-only**. Implement it as a pure function `canonPathEntry(string) string` that:
- maps the **notational** home forms to a single sentinel — `~`, `~/`, `$HOME`, `${HOME}`, `$HOME/`, `${HOME}/` → e.g. a literal `~` prefix — **without** reading the environment;
- collapses runs of `/` (`//` → `/`) and strips a single trailing `/` (but never reduces the root `/` to empty);
- does **NOT** resolve `..`, does **NOT** resolve symlinks, does **NOT** call `filepath.Clean` (it collapses `..` and `.`), does **NOT** call any `os.*` or `filepath.Abs/EvalSymlinks`.
Add a guard test that runs the analyzer twice with two different `HOME` env values and asserts byte-identical JSON. Forbid imports: a `go vet`/grep check that `core/analyze` does not import `os` or call `filepath.EvalSymlinks`/`filepath.Abs`.

**Warning signs:**
- Any `os.Getenv`, `os.UserHomeDir`, `filepath.Abs`, `filepath.EvalSymlinks`, or `filepath.Clean` appears in the canonicalization path.
- Output changes when `$HOME` changes or between machines.
- `../bin` is silently turned into a resolved absolute directory.

**Phase to address:** Canonicalization

---

### Pitfall 4: Canonicalizing a **named tilde** `~user` as if it were `~` (false-equating different homes)

**What goes wrong:**
`~` ≡ `$HOME`, but `~root`, `~deploy`, `~+`, `~-`, `~1` are **not** `$HOME`. Verified locally: `~` → `/Users/Tanmai.N` while `~root` → `/var/root`. If canonicalization strips/normalizes the leading `~` generically, `~root/bin` collapses to the same key as `~/bin` and you report a false duplicate (or mis-name the entry). `~+` (PWD), `~-` (OLDPWD), and `~N` (dir-stack) are dynamic and also not `$HOME`.

**Why it happens:**
The naive home rule is "leading `~` means home." The official zsh rule (Expansion manual) is narrower: a bare `~` is `$HOME`, but `~` followed by name characters is a **named directory / username lookup** — a different value.

**How to avoid:**
Only treat the home forms as equivalent when the tilde is **bare**: `~` exactly, or `~` immediately followed by `/` or end-of-entry. Anything matching `~[A-Za-z0-9_.+-]` (named dir / `~+` / `~-` / `~N`) is a **distinct, opaque** entry — keep its text verbatim as the entry name and do **not** fold it into the `$HOME` sentinel. (Do not resolve it either — that would be a filesystem lookup; see Pitfall 3.) Add fixtures: `~`, `~/bin`, `~root/bin`, `~+/x`, `~-/x`.

**Warning signs:**
- `~root/bin` and `~/bin` are reported as duplicates.
- The canonicalizer has an unconditional `strings.TrimPrefix(s, "~")`.
- A username-tilde is rewritten to `$HOME`.

**Phase to address:** Canonicalization

---

### Pitfall 5: Mis-classifying which `:` are **empty entries that mean current-directory** (the PATH foot-gun) vs. mere formatting

**What goes wrong:**
In PATH lookup an **empty field** — a leading `:`, a trailing `:`, or a `::` in the middle — means *the current directory* and is the canonical PATH security/foot-gun. The advisory must flag these as relative/cwd entries. The trap: zsh's *own* word-splitting operator `${(s.:.)...}` **drops empty fields** (reproduced locally: `${(s.:.)${:-/a::/b:}}` yields only `/a` and `/b`, silently eating the `::`, the leading, and the trailing empties). If the implementer reaches for zsh-style split semantics — or for `strings.FieldsFunc`, which also discards empties — the empty/cwd entries vanish and the most important advisory case is silently missed.

**Why it happens:**
"Split on `:`" mentally maps to "give me the non-empty directories." Most split helpers (`strings.Fields`, `FieldsFunc`, zsh `(s.:.)`) elide empties by design. The cwd-meaning of an empty field is non-obvious unless you know POSIX PATH lookup.

**How to avoid:**
Split with `strings.Split` (which **preserves** empty fields), not `Fields`/`FieldsFunc`/`(s.:.)` semantics. Each empty field (from leading `:`, trailing `:`, or `::`) is a real entry whose meaning is "current directory" → emit the relative/cwd advisory for it. Treat a literal `.` and `./x` / `..` / `../x` the same way (relative). Be careful at the boundaries: a value that is exactly `$PATH` (self-reference only) must not be read as "one empty entry"; drop the self-reference token *before* counting empties so `PATH="$PATH"` yields zero user entries, while `PATH=":$PATH"` yields one cwd entry plus the dropped self-ref.

**Warning signs:**
- `PATH=":$PATH"`, `PATH="$PATH:"`, or `PATH="/a::/b"` produce no advisory.
- The splitter is `strings.Fields`/`FieldsFunc` or uses zsh `(s.:.)`.
- Entry count drops when colons are adjacent.

**Phase to address:** Advisory + Severity (relative/cwd detection), with the verbatim-`:` split landing in Extraction.

---

### Pitfall 6: The severity tier accidentally bumps (or suppresses) the exit code — breaking the agent contract

**What goes wrong:**
`Analysis.ExitCode()` is dead simple today: `len(a.Issues) > 0 → ExitActionable (3)`. The new relative-entry advisory is *also* a `model.Issue`, so the moment you append it the file exits 3 even when the only "issue" is an informational advisory. That degrades the agent signal exactly as PROJECT.md warns. The mirror-image mistake: refactor `ExitCode()` to consider severity and accidentally make a *genuine* duplicate/shadow non-actionable (e.g. defaulting an un-set severity to "info").

**Why it happens:**
The advisory reuses the `Issue` type (correct — it should show up in `issues`), but `ExitCode()` counts *issues*, not *actionable* issues. Severity is a new dimension that the exit derivation must now read, and zero-value/default handling is easy to get backwards.

**How to avoid:**
1. Add a `Severity` field to `model.Issue` with explicit constants (e.g. `SeverityActionable`, `SeverityAdvisory`/`SeverityInfo`). **Choose the zero value deliberately**: make the zero value `SeverityActionable` so existing issue constructions (`duplicate_alias`, `reassigned_env`, `duplicate_path`, `shadowed`) that don't set severity stay actionable by default, and only the new advisory explicitly opts into `SeverityAdvisory`. (Alternatively make zero = advisory but then audit *every* existing `model.Issue{...}` literal — riskier.)
2. Change `ExitCode()` to `if any issue has actionable severity → ExitActionable`. Add table tests: advisory-only → exit 0; one duplicate + one advisory → exit 3; duplicate-only → exit 3; clean → exit 0.
3. Keep `dto.Envelope.IssuesFound`/`exit_code` consistent with the new rule (see Pitfall 7).

**Warning signs:**
- A file whose only finding is a relative-entry advisory exits 3.
- An existing duplicate/shadow stops producing exit 3 after the refactor.
- `ExitCode()` still reads `len(a.Issues)` instead of filtering by severity.

**Phase to address:** Advisory + Severity

---

### Pitfall 7: Breaking the "exactly one JSON object on stdout" contract or silently breaking DTO back-compat with the new field

**What goes wrong:**
Two distinct sub-traps:
- **Stream contract:** Adding human-readable advisory output or debug prints to `stdout` (or letting a new code path `panic`/log to stdout) violates the agent contract that `--json` emits *exactly one* JSON object on stdout on success **and** failure (`cli.go` `fail` deliberately routes even errors to stdout as JSON). Any stray `fmt.Println`, `log` to stdout, or a second emitted object breaks every agent consumer.
- **Wire shape:** The severity field must be threaded through *both* `model.Issue` **and** `dto.Issue` (`render/json.go:toDTO` maps field-by-field). If you add it to `model` but forget `dto`, the JSON silently omits it; if you add a non-`omitempty` `severity` to `dto.Issue`, *every* existing issue object in the wire format changes (adds `"severity":...`) — an intended-but-must-be-acknowledged contract change, mirroring how v1.0 treated the line-number change.

**Why it happens:**
The model→DTO mapping is manual and easy to under-update. The exit/JSON contract is implicit knowledge living in `cli.go` comments and the corpus; a new feature author may not realize advisories must not print to stdout in JSON mode.

**How to avoid:**
- Thread `Severity` explicitly through `toDTO` (`render/json.go`) and add a wire field to `dto.Issue` (decide `omitempty` vs always-present deliberately and record it as a contract change in PROJECT.md, as v1.0 did for line numbers).
- Decide the **value encoding** for the wire (string `"advisory"`/`"actionable"` is agent-friendlier than an int; whatever you pick, golden-fixture it).
- Add/extend the agent-contract test that asserts `--json` output is **a single valid JSON object** (parse stdout; assert exactly one top-level object; assert nothing leaks to stdout besides it) for: clean file, duplicate-only file, advisory-only file, and the `fail` path.
- Update the golden corpus to include `severity` in expected JSON.

**Warning signs:**
- `json.Unmarshal(stdout)` fails or there is trailing content after the object.
- `severity` present in human report but absent from `--json`.
- A diff of an existing golden envelope shows an unintended field change you didn't record.

**Phase to address:** Advisory + Severity (model+dto threading) and Coverage (contract/golden tests).

---

### Pitfall 8: Making the testgen oracle circular — letting it learn PATH/severity expectations from the engine

**What goes wrong:**
The oracle (`testgen.Expected`) is the regression pin precisely because it computes expected issues **independently** of the engine (it imports only `core/model`; the package doc and `Expected` comments stress non-circularity, and `RenderedLines` is sourced from a *separate* counter, not the engine's `countLines`). When extending it to relative/unrooted dup paths, the trap is to compute the expected canonical key by calling the engine's new `canonPathEntry` (or by re-deriving entries the same way the reconciler does). Then the test proves only that the code equals itself — a duplicate-detection bug becomes invisible because both sides share it.

**Why it happens:**
DRY pressure: "I already wrote canonicalization; why duplicate it in the oracle?" Also, the generator currently *hands the oracle pre-rooted dir names* (`pathDirs` are all rooted; `dupNameIssues([]NodeKind{NodePathEntry}, ...)` keys on `Node.Name` directly), so to exercise notation/relative cases you must teach the generator to emit `~`/`$HOME`/trailing-slash/relative variants — and it's tempting to canonicalize them with the engine's function to know what to expect.

**How to avoid:**
- Keep `core/testgen` importing **only** `core/model` (enforce with a grep/test that it never imports `core/analyze` or `core/shell`).
- Give each `NodePathEntry` two independent fields: the **rendered notation** (what `render.go` writes into the `.zsh`, e.g. `~/bin`, `$HOME/bin`, `./scripts`, `/a//b/`) and the **oracle key** (the directory's *intended* canonical identity, set by the generator by construction). The oracle groups duplicates by the oracle key it was *given*, never by re-running canonicalization. Example: generator plants two entries with rendered `~/bin` and `$HOME/bin` but the same oracle key `HOME/bin` → oracle predicts one `duplicate_path`; the engine must independently canonicalize both notations to agree.
- For the relative/cwd advisory, the generator likewise marks an entry as "relative" by construction (it planted `./x`, bare `.`, or an empty field) and the oracle predicts the advisory from that flag — not by inspecting the rendered string with engine logic.
- Extend `render.go::render` for `NodePathEntry` so it can emit the chosen notation/relative/empty-field forms (today it hardcodes `export PATH=%q` of `name+":$PATH"`).
- Mirror the severity in the oracle: the oracle assigns the advisory `SeverityAdvisory` from its own knowledge, so the strict comparison also pins that the engine doesn't mark advisories actionable.

**Warning signs:**
- `core/testgen` imports `core/analyze` or `core/shell` (instant circularity).
- The oracle calls `canonPathEntry`/`extractPathEntries` or any function the engine also uses.
- A deliberately-broken canonicalizer still passes the property test.

**Phase to address:** Coverage/Oracle (the regression-pin phase), but design the `Node` two-field split *before* writing the engine canonicalizer so the oracle is genuinely independent.

---

## Moderate Pitfalls

### Pitfall 9: Quoted vs. unquoted PATH values, and `path+=(...)` array form, parsed inconsistently

**What goes wrong:**
`export PATH="$HOME/bin:$PATH"` (double-quoted), `export PATH=$HOME/bin:$PATH` (unquoted), `PATH='/literal:/x'` (single-quoted, no expansion), and the **array** forms `path=(/a /b /c)`, `path+=(/d)`, `typeset -U path` produce structurally different ASTs. The scalar forms are colon-separated; the **array** forms are space-separated word lists (each element is one entry, **no** `:` splitting), and `+=` appends. Verified locally: `path=(/x /y /z)` ties to `PATH=/x:/y:/z`, and `path+=(/b)` appends. If extraction only handles the `PATH=` scalar `CallExpr`/`DeclClause` shape, it will miss every `path=(...)`/`path+=(...)` entry and the `typeset -U path` dedup hint.

**Why it happens:**
The scalar `PATH=...:...` form is the textbook example, so it gets implemented first; the lowercase `path` **array** is a zsh-specific tie-in (the `path` array and `PATH` scalar are the same parameter, array vs. colon-string) that's easy to forget. Single-quoted values also suppress `$HOME` expansion, so `'$HOME/bin'` is a *literal* `$HOME` (different canonical treatment than a real param expansion only in edge reasoning — for notation-only canonicalization they can be treated the same, but be deliberate).

**How to avoid:**
Enumerate and fixture **all** assignment shapes up front:
- scalar: `PATH=...`, `export PATH=...`, `typeset PATH=...`, `declare/readonly PATH=...` (colon-split);
- array: `path=(...)`, `path+=(...)`, `typeset -U path` / `typeset -aU path` (space-split, each word one entry; recognize `-U` as the unique-array flag);
- `+=` on the scalar: `PATH+=":/x"` (append to colon string → split the appended chunk).
For arrays, take each element word as one verbatim entry (no `:` split unless an element itself literally contains `:`). Match on the parameter name case-insensitively for the scalar (`PATH`) and exactly `path`/`fpath`/`manpath`/etc. for the array (lowercase array names are the zsh convention). Note that `typeset -U path` means zsh will *de-dup at runtime keeping first* — the analyzer still reports source duplicates as written, but the advisory/duplicate note could mention the `-U` context (optional).

**Warning signs:**
- `path=(...)` / `path+=(...)` entries never appear in output.
- An array element containing a space is mis-joined, or a `:` inside an array element is wrongly split.
- `PATH+=` appends are dropped.

**Phase to address:** Extraction

---

### Pitfall 10: Dropping the `$PATH`/`${PATH}` self-reference incorrectly (over- or under-dropping)

**What goes wrong:**
Every prepend/append idiom carries a self-reference: `PATH="$HOME/bin:$PATH"`, `PATH="${PATH}:/x"`, `path+=($path /y)`. The self-reference token must be **dropped** from the reported entries (it's not a directory). Traps: (a) dropping it by naive substring `strings.ReplaceAll(v, "$PATH", "")` also nukes `$PATHOLOGICAL` or `$PATH_BACKUP` substrings and can leave dangling `:`; (b) failing to drop `${PATH}` (brace form) or the array `$path`; (c) dropping a *partial* match so `:$PATH"` leaves a stray empty entry that then false-fires the cwd advisory (interacts with Pitfall 5).

**Why it happens:**
"Remove `$PATH`" reads as a string replace. The brace form `${PATH}` and the array `$path` are separate spellings of the same self-reference, and substring matching is unaware of token boundaries.

**How to avoid:**
Recognize the self-reference at the **token/parsed-part** level: a value entry that is *exactly* a parameter expansion of `PATH` (`$PATH`, `${PATH}`) or, in array context, of `path` (`$path`, `${path}`) is the self-reference → drop that whole entry, not a substring. Never substring-replace. After dropping, do **not** synthesize an empty entry in its place. Fixture: `"$PATH"` alone (→ zero entries), `"$HOME/bin:$PATH"` (→ one entry `$HOME/bin`), `"${PATH}:/x"`, `($path /y)`, and a decoy `"$PATH_BACKUP/bin"` (must be kept, it's a real different variable).

**Warning signs:**
- `$PATH_BACKUP` or `$PATHOLOGICAL` loses its `PATH` substring.
- An entry list ends with a spurious empty/cwd entry after a trailing `:$PATH`.
- `${PATH}` brace form is reported as a directory.

**Phase to address:** Extraction

---

### Pitfall 11: Canonicalization collapses too aggressively — trailing slash, `//`, and the root `/`

**What goes wrong:**
Trailing-slash and double-slash normalization is in scope (`$HOME/bin/` ≡ `$HOME/bin`, `/a//b` ≡ `/a/b`). Edge cases bite: stripping a trailing `/` from the **root** entry `/` would yield the empty string (which then looks like a cwd/empty entry — a false advisory). Collapsing `//` at the **start** matters because a leading `//` is POSIX-implementation-defined but in practice users mean root; and a value that is purely `/` must stay `/`. Also `$HOME` vs `${HOME}/` vs `$HOME/` must all fold together *with* the slash rules applied consistently (apply home-sentinel mapping first, then slash collapse, then trailing-slash strip — order matters or `${HOME}/` and `$HOME` won't match).

**Why it happens:**
`strings.TrimSuffix(s, "/")` is a one-liner that's wrong for `s == "/"`. Operation order between home-folding and slash-normalization is easy to get inconsistent so two notations of the same dir don't converge.

**How to avoid:**
Pure, well-ordered `canonPathEntry`: (1) map home notations to the `~` sentinel; (2) collapse internal runs of `/` to a single `/`; (3) strip a trailing `/` **only if the result is non-empty and not the bare root**; (4) leave `~`/`$HOME`-sentinel + `/...` intact. Unit-test the matrix directly (not just via the engine): `{$HOME, ${HOME}, ~, $HOME/, ${HOME}/, ~/}` → one key; `{/a/b, /a/b/, /a//b, /a//b/}` → one key; `{/}` → `/`; `{""}` → empty/cwd (not the root).

**Warning signs:**
- The root `/` entry disappears or turns into an empty/cwd advisory.
- `${HOME}/` and `$HOME` don't dedup.
- `/a//b` and `/a/b` are reported as two entries.

**Phase to address:** Canonicalization

---

### Pitfall 12: `CatPath` classifier under-capture starves the reconciler of PATH blocks

**What goes wrong:**
The reconciler only sees blocks the analyzer bucketed under `CatPath` (`analyzer.go:70` passes `buckets[model.CatPath]` to `duplicatePaths`). Classification happens in `classify.go:37-41`, which matches assignment **names** containing `PATH` — but the **array** form `path=(...)` has the lowercase name `path`, which `strings.Contains(strings.ToUpper(n), "PATH")` *does* catch (good), yet `typeset -U path` may parse such that the assigned name surfaces differently, and a bare `path+=(...)` likewise. If an array PATH assignment lands in `CatEnvironment` (or `CatMisc`) instead of `CatPath`, the reconciler never receives it and silently reports no entries — a *coverage* bug that no amount of correct extraction logic fixes.

**Why it happens:**
The milestone (rightly) declares the classifier's *over-capture* (`Contains("PATH")` matching `MYPATH_DEBUG`) out of scope, so it's natural to ignore the classifier entirely. But *under-capture* of the array form directly defeats the new extraction. The two are different directions of the same `classify.go:39` line.

**How to avoid:**
Verify, with fixtures, that **every** assignment shape from Pitfall 9 (`PATH=`, `export PATH=`, `path=(...)`, `path+=(...)`, `typeset -U path`, `fpath=(...)`) is classified `CatPath` *before* relying on the reconciler. If any array form misses, the minimal in-scope fix is to ensure the path-family **names** (`path`, `fpath`, `manpath`, `cdpath`, `infopath`) are recognized in classification — without touching the broader over-capture overhaul (still out of scope). Add a classification test per shape. This is a *seam check*, not a classifier rewrite.

**Warning signs:**
- `path=(...)` produces zero `duplicate_path`/advisory output even though extraction logic is correct in isolation.
- A unit test of extraction passes but the end-to-end corpus shows no path issues for array forms.

**Phase to address:** Extraction (add a classification-coverage assertion alongside the extractor).

---

### Pitfall 13: Multiple PATH assignments across lines — dedup must span the whole file, and "duplicate" line attribution must be correct

**What goes wrong:**
A real `.zshrc` adds to PATH on many lines (`export PATH="$HOME/bin:$PATH"` on line 10, `path+=(/opt/x)` on line 40, `export PATH="$HOME/bin:$PATH"` again on line 60). The duplicate is *cross-line* and *cross-statement*; dedup state must accumulate across all PATH blocks (the current `duplicatePaths` does accumulate across blocks — preserve that). Two attribution traps: (a) when one statement lists the *same* dir twice (`PATH="/a:/a:$PATH"`), is that a duplicate? (decide: yes, an intra-statement duplicate, both occurrences point at the *same* line — so `Lines` may legitimately contain a repeated line number, or you de-dup the line list); (b) `Block.StartLine` is the statement line (post v1.0 fix) — make sure the new extractor keeps attributing each entry to its block's `StartLine`, not to a comment line (the v1.0 LINE-02 fix lives in `parse.go`; don't regress it by re-introducing comment-offset logic into the path path).

**Why it happens:**
Single-statement examples dominate tests; the per-statement-vs-per-file distinction and the intra-line-duplicate edge are easy to skip. The line-attribution invariant from v1.0 is silent context.

**How to avoid:**
- Accumulate canonical-key → []line across **all** path blocks (keep the existing map-over-blocks shape).
- Decide and document the intra-statement-duplicate rule; fixture `PATH="/a:/a:$PATH"` and assert the chosen behavior.
- Reuse `Block.StartLine` for attribution; add a fixture with a leading comment above a PATH assignment and assert the issue line is the *statement* line (pins LINE-02 for the path kind, mirroring how the generator already plants comments for the other kinds in `generator.go:96-110`).
- Sort `Lines` (existing code does) for determinism.

**Warning signs:**
- A duplicate split across two lines isn't reported.
- A PATH issue points at a comment line.
- `Lines` order is non-deterministic.

**Phase to address:** Extraction (accumulation + attribution); Coverage (comment-above-PATH fixture).

---

## Minor Pitfalls

### Pitfall 14: Empty `Issues` slice vs. `null`, and golden-output byte-stability

**What goes wrong:**
`render/json.go:toDTO` deliberately preserves nil `Issues` as JSON `null` (documented: "byte-identical to the model that previously carried the json tags"). When you add the advisory and severity, it's easy to accidentally initialize `a.Issues` to a non-nil empty slice (changing `null` → `[]`) or reorder issues (the analyzer sorts by `Kind` then `Name` at `analyzer.go:91-96`; the new advisory kind sorts into that order — verify where it lands). Either change silently breaks golden corpus byte-comparison.

**Why it happens:**
Appending advisories and adding a new `IssueKind` interacts with both the nil-preservation and the sort comparator; neither is obvious.

**How to avoid:**
Keep the nil-vs-empty behavior unless intentionally changing it (and golden it if so). Add the new `IssueKind` constant and confirm the sort places it deterministically (its string value decides position; pick a name like `relative_path`/`unrooted_path` knowing it sorts among `duplicate_*`/`reassigned_*`/`shadowed`). Regenerate and review golden diffs deliberately.

**Warning signs:**
- Golden corpus diff shows `null` → `[]` (or vice versa).
- Issue ordering changes for files unrelated to PATH.

**Phase to address:** Coverage

---

### Pitfall 15: Over-flagging intentional relative entries as if they were errors (UX/severity confusion)

**What goes wrong:**
Some users *intentionally* put `.` or a relative dir on PATH (dev convenience). If the advisory is worded as an error or (worse) bumps exit 3, it creates noise and erodes trust — the exact rationale PROJECT.md gives for making it informational. Conversely, under-wording it (no `Note` explaining *why* a cwd/empty entry is risky) wastes the advisory.

**Why it happens:**
Severity and copy are an afterthought once detection works.

**How to avoid:**
Advisory severity (never exit 3), clear `Note` (e.g. "relative PATH entry — resolved against the current directory; can be a security risk and is environment-dependent"). For empty/cwd entries specifically, say so explicitly. Keep human-renderer formatting consistent with existing issue lines (`human.go:39-48`).

**Warning signs:**
- Relative-entry advisory causes exit 3 (also Pitfall 6).
- Note is empty or generic.

**Phase to address:** Advisory + Severity

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Extend `pathSegRe` regex instead of structural extraction | One-line diff; fastest to "pass" the rooted-dir cases | Re-creates the original bug class (comments, self-ref, `:`-in-`${}`, unrooted); guarantees a rewrite | Never — this *is* the bug being fixed |
| `strings.Split(value, ":")` with no brace/quote awareness | Trivial; works on simple `/a:/b` | Shreds `${PATH:+:$PATH}` and `${VAR:-/a:/b}` (Pitfall 2) | Only if values are pre-tokenized by the parser (split parts, not the raw string) |
| `filepath.Clean`/`Abs`/`EvalSymlinks` for "canonical" paths | Stdlib, familiar | Non-deterministic, env-dependent, collapses `..`; violates Out-of-Scope | Never in this milestone |
| Zero-value `Severity` = advisory without auditing existing `Issue{}` literals | No edits to existing issue construction | Existing duplicates/shadows silently become non-actionable → exit-3 regression | Only if every existing `model.Issue{...}` literal is updated in the same change |
| Oracle reuses engine's `canonPathEntry` | DRY | Circular pin — proves code equals itself; real dedup bugs invisible | Never — non-circularity is the oracle's entire value |
| Skip array-form (`path=(...)`) extraction "for now" | Smaller first cut | Silent blind spot identical in spirit to the bug being fixed; common in real configs | Only if explicitly deferred in scope **and** documented; otherwise no |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `mvdan.cc/sh/v3/syntax` value words | Calling `Word.Lit()` and assuming it returns the full value (it's empty when the word has quoted/param parts — see existing `wordLitPrefix` note) | Walk `Word.Parts`: handle `*Lit`, `*ParamExp` (`$PATH`/`${PATH}`), `*DblQuoted`, `*SglQuoted` explicitly; reconstruct value or split on parts |
| zsh `path`/`PATH` parameter tie | Treating `path=(...)` array and `PATH=...` scalar as unrelated | They are the same parameter (array vs. colon-string); both feed dedup; array elements are space-separated, scalar is colon-separated |
| zsh `${(s.:.)...}` splitting (if anyone reaches for shell semantics) | Using it as the mental model for splitting | It **drops empty fields** — the opposite of what the cwd/empty-entry advisory needs; use `strings.Split` which preserves empties |
| Existing `--json` agent contract (`cli.go` `fail`) | Adding advisory/debug output to stdout in JSON mode | Exactly one JSON object on stdout, success or failure; advisory data goes *inside* the envelope's `issues`, nothing else printed |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Not flagging empty/`::`/leading-`:`/trailing-`:` entries | The classic PATH-injection foot-gun (cwd on PATH) goes unreported — the headline value of the advisory | Split preserving empties; emit advisory for every empty field (Pitfall 5) |
| Resolving `~`/`$HOME`/symlinks against the live machine | Leaks the analyzing user's real home/filesystem into output; makes a "read-only static analyzer" environment-coupled | Notation-only canonicalization; no `os.*`/`filepath.EvalSymlinks` (Pitfall 3) |
| Mis-equating `~user`/`~root` with `$HOME` | Hides a genuinely different (possibly privileged) directory behind a false "duplicate" | Only fold *bare* `~`; keep named-tilde verbatim (Pitfall 4) |

## "Looks Done But Isn't" Checklist

- [ ] **PATH extraction:** Often missing the **array** forms — verify `path=(...)`, `path+=(...)`, `typeset -U path` each yield entries.
- [ ] **`:`-splitting:** Often missing brace-awareness — verify `${PATH:+:$PATH}`, `${FOO:-/a:/b}` don't shred into garbage entries.
- [ ] **Self-reference:** Often substring-removed — verify `$PATH_BACKUP/bin` survives and `"$PATH"`-alone yields zero entries.
- [ ] **Empty/cwd entries:** Often elided by the splitter — verify leading `:`, trailing `:`, `::` each produce a relative advisory.
- [ ] **Named tilde:** Often folded into `$HOME` — verify `~root/bin` ≠ `~/bin`.
- [ ] **Canonicalization purity:** Often uses `filepath.Clean` — verify two runs with different `$HOME` are byte-identical; verify no `os`/`filepath.Abs`/`EvalSymlinks` import in `core/analyze`.
- [ ] **Root entry:** Often destroyed by trailing-slash strip — verify `/` stays `/`.
- [ ] **Exit code:** Often bumped by advisories — verify advisory-only file exits 0, duplicate-only exits 3, mixed exits 3.
- [ ] **DTO threading:** Often model-only — verify `severity` appears in `--json` and the agent-contract test still sees exactly one JSON object.
- [ ] **Oracle independence:** Often circular — verify `core/testgen` imports only `core/model` and the oracle never calls engine extraction/canonicalization; verify a deliberately-broken canonicalizer fails the property test.
- [ ] **Line attribution:** Often regressed — verify a PATH assignment under a leading comment reports the *statement* line (LINE-02 still holds for `duplicate_path`).
- [ ] **Golden stability:** Often flips `null`↔`[]` or reorders issues — verify golden corpus diffs are intentional only.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Regex re-used over `Block.Text` (Pitfall 1) | MEDIUM | Replace with structural extraction from `*syntax.Assign`; re-run corpus + property test |
| `:`-split shredding `${...}` (Pitfall 2) | MEDIUM | Switch to part-walking or brace/quote-depth scanner; add idiom fixtures |
| Filesystem resolution slipped in (Pitfall 3) | LOW | Delete the `os`/`filepath.Abs/EvalSymlinks` calls; add the two-`HOME` byte-identical guard test |
| Exit-code regression from severity (Pitfall 6) | LOW | Fix `ExitCode()` to filter by severity; add the advisory-only=0 / mixed=3 table test |
| Circular oracle (Pitfall 8) | HIGH | Re-architect `Node` into rendered-notation + oracle-key fields; this is hard to retrofit, so design it before the canonicalizer |
| DTO field forgotten (Pitfall 7) | LOW | Thread through `toDTO`; regenerate golden; add wire assertion |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Regex re-extraction | Extraction | No `regexp` over `Block.Text`; entries come from parsed assigns |
| 2. `:`-split inside `${...}` | Extraction | Idiom fixtures (`${PATH:+:$PATH}`, `${FOO:-/a:/b}`) yield correct entries |
| 3. Filesystem/`$HOME` resolution | Canonicalization | Two-`HOME` runs byte-identical; no `os`/`filepath.Abs`/`EvalSymlinks` import in `core/analyze` |
| 4. Named-tilde folding | Canonicalization | `~root/bin` ≠ `~/bin` fixture |
| 5. Empty/cwd entries | Advisory + Severity (split in Extraction) | Leading/trailing/`::` colon fixtures each advise |
| 6. Exit-code bump from severity | Advisory + Severity | Advisory-only=0, duplicate-only=3, mixed=3 table tests |
| 7. JSON contract / DTO back-compat | Advisory + Severity (threading); Coverage (contract test) | One JSON object on stdout (all paths); `severity` present in wire; golden updated |
| 8. Circular oracle | Coverage/Oracle (design first) | `core/testgen` imports only `core/model`; broken canonicalizer fails property test |
| 9. Quoted/array/`+=` forms | Extraction | Fixtures for all assignment shapes yield entries |
| 10. Self-reference dropping | Extraction | `$PATH_BACKUP` survives; `"$PATH"`-alone → 0 entries |
| 11. Slash/trailing/root collapse | Canonicalization | Canon matrix unit test (`/` stays `/`; `${HOME}/`≡`$HOME`) |
| 12. Classifier under-capture | Extraction | Classification test: every PATH shape → `CatPath` |
| 13. Cross-line dedup + attribution | Extraction; Coverage | Cross-line dup fixture; comment-above-PATH line fixture |
| 14. nil-vs-empty / sort order | Coverage | Golden corpus byte-stable; new kind sorts deterministically |
| 15. Advisory wording/severity UX | Advisory + Severity | Advisory has clear `Note`, never exit 3 |

## Sources

- zsh official manual — Expansion (Filename/Tilde Expansion; "right hand side ... treated as a colon-separated list ... a '~' ... following a ':' is eligible for expansion"; bare `~`=`$HOME` vs `~name`=named-directory/username): https://zsh.sourceforge.io/Doc/Release/Expansion.html — HIGH
- Local reproduction in zsh 5.9 (arm64-apple-darwin25): empty PATH field semantics and `${(s.:.)}` empty-field elision; `path`↔`PATH` tie and colon-split; `typeset -U path` first-wins dedup; `path+=(...)` append; `~root`→`/var/root` vs `~`→`$HOME` — HIGH
- POSIX PATH semantics — a null (empty) directory name in PATH indicates the current working directory — HIGH (corroborates the local zsh-lookup behavior)
- Codebase read directly: `core/analyze/reconciler.go` (buggy `pathSegRe`/`duplicatePaths`), `core/shell/zsh/parse.go` (block/`Block.Text` + comment pull-up), `core/shell/zsh/classify.go` (`CatPath` name match), `core/testgen/{oracle,generator,render,graph}.go` (oracle independence + `RenderedLines` non-circularity), `core/cli/cli.go` (`--json` contract / `fail`), `core/model/{issue,exitcode,analysis,block}.go`, `core/dto/{analysis,envelope}.go`, `core/render/{json,human}.go`, `core/testgen/property_test.go` (`checkLines=true` pin) — HIGH

---
*Pitfalls research for: zsh-config static analyzer — PATH analysis (zsh-pro v1.1)*
*Researched: 2026-06-24*
