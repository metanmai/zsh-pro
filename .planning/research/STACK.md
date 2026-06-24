# Stack Research

**Domain:** Read-only static analysis of a shell (zsh) `PATH` assignment — string-level extraction, notation-only canonicalization, and rootedness classification of PATH entries (Go CLI)
**Researched:** 2026-06-24
**Confidence:** HIGH

> Scope note: this milestone (v1.1 "Trustworthy PATH Analysis") adds NO runtime capability that needs a new library. The "stack" here is **which Go stdlib facilities to use, which to deliberately avoid, and one already-present AST facility from `mvdan.cc/sh` to extract the PATH right-hand side cleanly.** All findings below were verified empirically against Go **1.25.7** and `mvdan.cc/sh/v3 v3.13.1` (the exact pinned version in `go.mod`), not just from training data.

---

## Bottom line (read this first)

1. **No new dependency is warranted.** Confirmed. Splitting on `:`, notation-only canonicalization, and relative/`.`/empty detection are all trivially expressible with the `strings` package the file already imports. (Justification against the hard constraint is in [What NOT to Use](#what-not-to-use) and [Alternatives Considered](#alternatives-considered).)
2. **Use `strings.Split(rhs, ":")`** for entry extraction — it preserves leading/trailing/interior **empty** segments, which is exactly the cwd foot-gun the new advisory must catch.
3. **Do the rootedness test with `strings.HasPrefix(e, "/")`**, NOT `filepath.IsAbs` / `path.IsAbs`.
4. **NEVER run a PATH entry through `path.Clean` or `filepath.Clean`.** They collapse `..`, rewrite `./scripts`→`scripts`, and turn `""`→`"."` — destroying the very signals (relative, empty/cwd) the milestone exists to surface, and crossing the explicit "notation-only, no resolution" line. This is the single most important finding.
5. **Prefer the AST for RHS extraction over regex-scraping `Block.Text`.** The parser already produces the assignment value as `*syntax.Word` (`Assign.Value`). Render it once with `syntax.Printer.Print` (which does NOT resolve `$HOME`/`~`) to get a clean RHS string, then `strings.Split` it. This deletes the brittle `pathSegRe` regex (`core/analyze/reconciler.go:17`) outright. (Architectural caveat below — extraction belongs in `core/shell/zsh`, not `core/analyze`.)

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go standard library `strings` | Go 1.25 (1.25.7 verified) | Split RHS on `:`; canonicalize `~`/`$HOME`/`${HOME}`; normalize slashes; detect rootedness | Already imported by `core/analyze/reconciler.go`. Operates on raw bytes/runes with **zero hidden semantics** — it never resolves, cleans, or interprets paths, which is precisely the "notation-only, deterministic, read-only-pure" contract the milestone demands. |
| `mvdan.cc/sh/v3/syntax` (already a dependency) | v3.13.1 | Source of the PATH assignment's right-hand side as a structured `*syntax.Word`, and a non-resolving `Printer` to turn that word back into clean text | Already the project's only external dep and already used by `core/shell/zsh/parse.go`. `Assign.Value` gives the exact RHS without string-scraping; `Printer.Print` renders it back to source **without** performing any parameter expansion (verified). This replaces the over-broad `pathSegRe` regex. |

### Supporting Libraries

None required, and none recommended. The full feature set is covered by the two entries above. (`sort` is already imported by the reconciler and is reused for deterministic line ordering — no change there.)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| (none) | — | — | Adding any third-party library here would violate the milestone's hard "no new dependencies" constraint AND add capability the task does not need. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `go test ./...` (stdlib `testing`) | Golden-fixture + testgen-oracle regression | No new tooling. The new `duplicate_path` / relative-entry cases extend the existing `core/testgen` oracle and corpus, per PROJECT.md. |
| `gofmt` / `go vet` / `golangci-lint` (already configured) | Style + static checks | Unchanged; new code is plain `strings` logic and will pass the existing gates. |

---

## Concrete stdlib choices (with WHY)

All of the following were run and their output verified (see [Empirical verification](#empirical-verification)).

### (a) Splitting the PATH value on `:`

**Use `strings.Split(rhs, ":")`.**

- **Why:** PATH is colon-delimited verbatim. `strings.Split` is a pure byte split that **keeps empty fields**: `"$PATH:"` → `["$PATH", ""]`, `":$PATH"` → `["", "$PATH"]`, `"a::b"` → `["a","","b"]`. Those empty fields ARE the "empty entry = current directory" foot-gun the advisory must flag — so the naive splitter is exactly right and any "smarter" splitter would be wrong.
- **Do NOT use `strings.FieldsFunc` / `strings.Fields`** for this — they drop empty fields and would silently swallow the cwd entry.
- **Self-reference drop:** after splitting, drop the recursion token by exact match — `e == "$PATH" || e == "${PATH}"` (and the lowercase array variants `$path` / `${path}` if you support `path=(...)`). Use simple `==` comparisons, not a regex.

### (b) Notation-only canonicalization

Two independent, order-sensitive string rewrites, then compare canonical forms for dedup. **Both are pure `strings` operations; neither touches the filesystem or the environment.**

1. **`~` / `$HOME` / `${HOME}` → one canonical token** (e.g. `$HOME`):
   - `strings.HasPrefix(e, "~/")` or `e == "~"` → replace the leading `~` (use `strings.TrimPrefix(e, "~")` and prepend the canonical token).
   - `strings.HasPrefix(e, "${HOME}")` → `strings.TrimPrefix(e, "${HOME}")` and prepend the canonical token.
   - `strings.HasPrefix(e, "$HOME")` → already canonical (or normalize `${HOME}` to it).
   - **Why `HasPrefix`/`TrimPrefix` and not a regex:** the equivalence is purely positional (only a *leading* `~`/`$HOME` denotes home; `~user` and a mid-string `~` must NOT be rewritten). `strings.HasPrefix` + `strings.TrimPrefix` expresses "leading token only" exactly and is trivially correct; verified that `~user/x` is left untouched.

2. **Slash normalization (notation-only):**
   - Collapse runs of `/` to a single `/`, then trim a single trailing `/` (guarding against turning `"/"`→`""`).
   - Implement with a small loop using `strings.Contains(s, "//")` + `strings.ReplaceAll(s, "//", "/")`, then `strings.TrimRight(s, "/")` (with a `len(s) > 1` guard).
   - **Why hand-rolled and not `path.Clean`:** `path.Clean` *also* collapses `..` and rewrites `.`/`""`, which are forbidden (resolution) and destroy signal. The hand-rolled version touches **only** slashes — verified: `"/foo//bar/"`→`"/foo/bar"`, `"//"`→`"/"`, while `"/foo/../bar"` is left **untouched** (good — we do not resolve `..`).

> Canonicalization is for **dedup keying only**. Keep the original verbatim entry text for display in the issue `Name`, and compare canonical forms to decide "same directory, different notation."

### (c) Relative / unrooted / empty / `.` detection

Classify each (non-self-ref) entry by leading token — a pure prefix test:

```
e == ""                          -> empty entry (current directory)   [advisory]
e == "." || e == ".."            -> explicit cwd / parent             [advisory]
strings.HasPrefix(e, "/")        -> absolute  (OK)
strings.HasPrefix(e, "~")        -> home-rooted (OK; ~ or ~user)
strings.HasPrefix(e, "$HOME") ||
strings.HasPrefix(e, "${HOME}")  -> home-rooted (OK)
otherwise                        -> relative/unrooted (e.g. ./scripts after
                                    canonicalization, or bare bin/lib)  [advisory]
```

- **Use `strings.HasPrefix(e, "/")` for absoluteness, NOT `filepath.IsAbs(e)` and NOT `path.IsAbs(e)`:**
  - `filepath.IsAbs` is **OS-separator-sensitive** — on Windows it would treat `C:\...` as absolute and `/usr/bin` differently; the project targets macOS/Linux but absoluteness here is a **zsh PATH semantic** (`/`-rooted), not a host-OS filesystem fact, so coupling to the host is conceptually wrong even where it happens to agree.
  - Both `IsAbs` variants return `false` for `~/bin`, `$HOME/bin`, and `""` — so they cannot, on their own, distinguish "home-rooted (fine)" from "relative (flag)" from "empty (flag)". You need the explicit token checks above regardless; `HasPrefix(e, "/")` then states the intent plainly.
  - Note `./scripts` should be canonicalized first if you want it to read as relative cleanly — but even raw, it falls through to the "relative/unrooted" branch because it has none of the rooted prefixes. (Do **not** canonicalize it with `path.Clean`, which would turn it into `scripts` — see the warning table.)

---

## The `mvdan.cc/sh` extraction angle (concrete: package + type)

**Question asked:** does the parser/expand package expose word-splitting or parameter info that yields a cleaner PATH RHS than scraping `Block.Text` with a regex? **Answer: yes — the AST already hands you the RHS, and the `Printer` renders it without resolving anything.**

- **Package:** `mvdan.cc/sh/v3/syntax` (already imported by `core/shell/zsh/parse.go`).
- **Type / field:** `*syntax.Assign` has `Value *syntax.Word` (the RHS) and `Array *syntax.ArrayExpr` (for the `path=(...)` array form). `core/shell/zsh/parse.go` already iterates `CallExpr.Assigns` and `DeclClause.Args` (both `[]*syntax.Assign`) — it just doesn't read `.Value` yet.
- **Rendering without resolution:** `syntax.NewPrinter(...)` + `(*Printer).Print(w io.Writer, node Node)` accepts a `*syntax.Word` (and individual `WordPart`s) and writes source text. Verified: printing the RHS of `export PATH=~/bin:${HOME}/sbin:.:$PATH` yields the string `~/bin:$HOME/sbin:.:$PATH` — i.e. it **does not** expand `$HOME`/`~` to a real home directory (it stays a `$`-token), which keeps the pipeline deterministic and read-only-pure. (`${HOME}` prints as `$HOME`, a free notation-normalization bonus.)

**Recommended extraction recipe (replaces `pathSegRe`):**
1. In the **zsh provider** (not the reconciler — see architecture note), when an assignment's `Name` is a PATH-family var (`PATH`/`FPATH`/`MANPATH`/`CDPATH`/...), render `Assign.Value` (or each `Array.Elems[i].Value`) to a string with `Printer.Print`.
2. `strings.Split` that string on `:` (array form is already per-element, so no split needed there).
3. Drop `$PATH`/`${PATH}` self-refs, canonicalize, classify — all with `strings` as above.

**Why this beats regex-scraping `Block.Text`:**
- The current `pathSegRe = (?:\$HOME|~|/)[^:"'\s)]+` (`reconciler.go:17`) **anchors every entry on `/`, `~`, or `$HOME`** — which is the documented root cause of BOTH milestone bugs: it rewrites `./scripts`→`/scripts` (it matches from the `/`) and it is structurally **incapable** of matching a bare relative entry (`bin`, `lib`) or an empty entry, so unrooted paths are invisible to it. Verified that `strings.Split` on the AST-rendered RHS classifies `./scripts`, `bin`, `lib`, `.`, and `""` correctly where the regex cannot.
- Scraping `Block.Text` also re-parses text the parser already understood, and would have to re-handle quoting/comments by hand. The AST already did that work.

> One caveat worth flagging for the roadmap: quoted segments render **with their quotes** (e.g. `PATH="a:b:":$PATH` → the printer emits `"a:b:":$PATH`). For v1.1's notation-only goal this is an edge case; if a fixture exercises a quoted PATH segment, strip surrounding quotes after splitting (a `strings.Trim(e, "\"'")` pass) — still pure `strings`, still no new dep. Most real `.zshrc` PATH lines are unquoted or wrap the *whole* value in quotes, the latter rendering as a single `DblQuoted` part that splits cleanly once outer quotes are trimmed.

---

## Architecture note (so the roadmap respects the layering)

PROJECT.md's constraints require `core/analyze` to stay **shell-free** (interface seam only) and confine zsh specifics to `core/shell/zsh`. The current `duplicatePaths` lives in `core/analyze/reconciler.go` and reaches into `Block.Text` with a zsh-flavored regex — already a mild layering smell. Two clean options for the roadmap (both stdlib-only):

- **Preferred:** have the **zsh provider** populate a new agnostic, structured field on `model.Block` (e.g. `PathEntries []string`, already split + self-ref-dropped, verbatim notation) during `describe()` in `core/shell/zsh/parse.go`, using the AST `Assign.Value` + `Printer` recipe. The reconciler then does canonicalization/dedup/classification on that agnostic slice with `strings` — no shell knowledge, no regex. This keeps the `mvdan.cc/sh` dependency where it belongs (the provider) and out of `core/analyze`.
- **Acceptable fallback:** keep extraction in the reconciler but replace the regex with `strings.Split` over text the provider already exposes. Simpler diff, but it leaves PATH-splitting logic in the agnostic layer. The roadmap/requirements authors should pick deliberately; the stdlib choices are identical either way.

(The new **severity field on `Issue`** and the new **relative-entry `IssueKind`** are pure `core/model` additions — no library implications. Flagged here only so the roadmap doesn't expect a dependency for them.)

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `strings.Split(rhs, ":")` | `strings.FieldsFunc(rhs, func(r rune) bool { return r == ':' })` | Never for this task — `FieldsFunc` discards empty fields and would lose the empty-entry (cwd) signal. Only use if you explicitly do NOT want empty entries, which is the opposite of the requirement. |
| AST `Assign.Value` + `Printer.Print` | Regex over `Block.Text` (status quo `pathSegRe`) | Only if, for some reason, the provider cannot expose the RHS — but it already parses it, so there is no such reason. The regex is the source of both bugs; do not keep it. |
| Hand-rolled slash normalize (`ReplaceAll`/`TrimRight`) | `path.Clean` | Only if you WANTED `..`/`.` resolution and trailing-slash removal together — which is explicitly out of scope (resolution forbidden). So: not here. |
| `strings.HasPrefix(e, "/")` | `filepath.IsAbs(e)` | Only in code that genuinely asks "is this absolute on the **host OS filesystem**." PATH rootedness is a shell semantic, not a host-FS question, so the prefix check is the right model and is OS-independent. |
| Manual `~`/`$HOME` prefix rewrite | `os.ExpandEnv` / `mvdan.cc/sh` `expand.Fields` / `shell.Expand` | **Never** in v1.1 — those RESOLVE variables against the live environment, making analysis non-deterministic and env-dependent, directly violating the "no live-`$HOME` resolution" decision in PROJECT.md. (Documented here precisely because they are the obvious-but-wrong tools.) |

---

## What NOT to Use

| Avoid | Why (verified) | Use Instead |
|-------|----------------|-------------|
| **`path.Clean`** on a PATH entry | Over-normalizes and resolves: `"./scripts"`→`"scripts"`, `""`→`"."`, `"/foo/../bar"`→`"/bar"`, `"/trailing/"`→`"/trailing"`. The first two **erase the exact relative/empty signals the advisory must report**; the `..` collapse is **path resolution**, which the milestone forbids. | `strings.Split` (no cleaning) for entries; targeted `strings.ReplaceAll`/`TrimRight` for slash-only normalization. |
| **`filepath.Clean`** | Same over-normalization as `path.Clean` here, **plus** it is OS-separator-aware (`\\` on Windows) — wrong abstraction for a shell `PATH` whose separator is always `/`. | Same as above. |
| **`filepath.IsAbs` / `path.IsAbs`** as the rootedness check | `path.IsAbs` is fine semantically but pairs naturally with `path.Clean` (a trap); `filepath.IsAbs` is OS-coupled. Both return `false` for `~/bin`, `$HOME/bin`, and `""`, so neither distinguishes home-rooted vs relative vs empty on its own. | `strings.HasPrefix(e, "/")` plus explicit `~`/`$HOME`/`${HOME}` and empty/`.` checks. |
| **`os.ExpandEnv`, `mvdan.cc/sh/v3/expand`, `mvdan.cc/sh/v3/shell` (`Expand`, `Fields`)** | They **resolve** `$HOME`/`~`/`$PATH` against the live process environment — non-deterministic, environment-dependent, and a read-only-purity violation. Directly contradicts PROJECT.md's "notation-only; no filesystem/env resolution." | Notation-only `strings` rewrites; `Printer.Print` (which renders `$HOME` as the literal token, NOT its value). |
| **Any new third-party module** (e.g. a "path parsing" or "shellwords" package) | Violates the milestone's hard "no new dependencies" constraint, and adds capability the task does not need — the whole job is ~40 lines of `strings` over an AST field the project already parses. No third-party library offers *notation-only, non-resolving* PATH semantics anyway; general-purpose path libs all clean/resolve (the wrong behavior). | The existing `strings` + `mvdan.cc/sh/v3/syntax` already in `go.mod`. |
| **A regex for splitting/classifying** (extending `pathSegRe`) | The leading-`/`,`~`,`$HOME` anchor is the documented cause of both bugs (mis-names `./scripts`, blind to bare-relative and empty entries). Regex also obscures the empty-field semantics that a plain `Split` makes obvious. | Structured `strings.Split` + prefix checks over the AST-rendered RHS. |

---

## Stack Patterns by Variant

**If extraction is placed in the zsh provider (preferred):**
- Use `Assign.Value` (`*syntax.Word`) + `syntax.Printer.Print` to render, then `strings.Split`, and store a structured `[]string` on `model.Block`.
- Because this keeps `mvdan.cc/sh` imports inside `core/shell/zsh` (honoring the single-composition-root / shell-free-`core/analyze` rule) and gives the reconciler clean, agnostic data.

**If extraction stays in the reconciler (fallback):**
- Replace `pathSegRe.FindAllString(b.Text, -1)` with `strings.Split` over a provider-exposed RHS string.
- Because it is a smaller diff, but accept that PATH tokenizing logic then sits in the agnostic layer — call it out in the requirements.

**If a fixture exercises quoted PATH segments (`PATH="a:b":$PATH`):**
- Add a `strings.Trim(e, "\"'")` pass after `Split`.
- Because the `Printer` re-emits quotes around quoted parts; trimming keeps it notation-only with no new dep.

---

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go `strings`, `sort` (stdlib) | Go 1.25.0 (toolchain 1.25.7 verified) | No version risk; APIs used (`Split`, `HasPrefix`, `TrimPrefix`, `TrimRight`, `ReplaceAll`, `Contains`) are long-stable stdlib. |
| `mvdan.cc/sh/v3/syntax` `Assign.Value` / `Printer.Print` | `v3.13.1` (pinned) | `Printer.Print` accepting `*Word` / `WordPart` and rendering without expansion verified on the exact pinned version. No upgrade needed; **no `go.mod`/`go.sum` change at all** for this milestone. |

---

## Empirical verification

Ran throwaway probes inside the module against Go 1.25.7 / `mvdan.cc/sh/v3 v3.13.1` (probe dir removed after):

- **AST RHS rendering (no resolution):** `export PATH=~/bin:${HOME}/sbin:.:$PATH` → `Assign.Value` printed as `~/bin:$HOME/sbin:.:$PATH` (note: `$HOME` stays a token; `${HOME}` normalized to `$HOME`; nothing resolved). Array form `path=(/opt/bin $path ~/x)` exposed as `Array.Elems` with values `/opt/bin`, `$path`, `~/x`.
- **`strings.Split` empty-field behavior:** `"$PATH:"`→`["$PATH",""]`, `":$PATH"`→`["","$PATH"]` — empty (cwd) entries preserved.
- **Classifier over the split entries:** `./scripts`, `bin`, `lib` → relative/unrooted; `.` → cwd; `""` → empty; `/usr/bin` → absolute; `~/bin`,`$HOME/sbin` → home-rooted; `$PATH` → self-ref (dropped). All correct.
- **Over-normalization proof:** `path.Clean("./scripts")="scripts"`, `path.Clean("")="."`, `path.Clean("/foo/../bar")="/bar"`, `filepath.Clean` identical on macOS — demonstrating these destroy the required signals. Hand-rolled slash normalize gave `"/foo//bar/"`→`"/foo/bar"` while leaving `"/foo/../bar"` untouched (correct: slashes only, no resolution).
- **`IsAbs` blind spots:** `path.IsAbs`/`filepath.IsAbs` both `false` for `~/bin`, `$HOME/bin`, `""` — confirming they cannot classify home-rooted vs relative vs empty alone.

---

## Sources

- **Empirical probes** (Go 1.25.7, `mvdan.cc/sh/v3 v3.13.1`) — RHS extraction, `strings.Split` semantics, `path.Clean`/`filepath.Clean` over-normalization, `IsAbs` behavior — **HIGH** (executed locally, this milestone's exact toolchain + pinned dep).
- `mvdan.cc/sh/v3@v3.13.1/syntax/nodes.go` — `Assign{Name,Value,Array}`, `Word`, `WordPart`/`Lit`/`ParamExp`/`DblQuoted` definitions — **HIGH** (read from module cache).
- `mvdan.cc/sh/v3@v3.13.1/syntax/printer.go` — `Printer.Print` accepts `*Word` and `WordPart` node types — **HIGH** (read from module cache).
- Go stdlib `strings`, `path`, `path/filepath` documented behavior — **HIGH** (verified by execution, consistent with long-stable stdlib semantics).
- Project context: `.planning/PROJECT.md` (v1.1 goal, Key Decisions: notation-only / no env resolution / new severity tier / no new deps), `core/analyze/reconciler.go` (`pathSegRe` bug site), `core/shell/zsh/parse.go` (existing `Assign` iteration), `go.mod` (pins) — **HIGH**.

---
*Stack research for: notation-only PATH-entry analysis in a read-only zsh-config CLI (Go stdlib + existing `mvdan.cc/sh` AST)*
*Researched: 2026-06-24*
