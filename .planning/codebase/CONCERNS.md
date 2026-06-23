# Codebase Concerns

**Analysis Date:** 2026-06-23

## Known Bugs

**Line count off-by-one (empty / trailing-newline files):**
- Symptoms: A file whose last byte is `\n` reports one extra line in `Analysis.Lines`. An empty file reports `1` instead of `0`.
- Files: `core/analyze/analyzer.go` line 28 — `Lines: strings.Count(string(src), "\n") + 1`
- Trigger: Any file with a trailing newline (virtually all real `.zshrc` files) or an empty file (`core/testdata/fixtures/empty.zsh`).
- Workaround: `core/testgen/property_test.go` gates line-level assertions with `checkLines = false` to pin rather than fix the bug. The golden manifest for `empty.zsh` accepts `min_blocks: 0` but does not assert `Lines`.

**Issue line mis-attribution (leading-comment offset):**
- Symptoms: When a statement has a leading comment, the issue's reported line number is the comment's start line rather than the statement line. `Block.StartLine` is set to the comment's line in the parser, and that value flows directly into `Issue.Lines` via `b.StartLine`.
- Files: `core/shell/zsh/parse.go` lines 36–40 (comment-pull-up loop sets `startLine = c.Pos().Line()`); `core/analyze/reconciler.go` lines 46 and 67 (uses `b.StartLine` directly); `core/model/block.go` (the `Block` struct — `StartLine` is ambiguous: it is the comment line, not the statement line).
- Trigger: Any alias, env var, path entry, or function preceded by a `#` comment that generates an issue (duplicate, reassigned, shadow).
- Workaround: `core/testgen/property_test.go` `checkLines = false` explicitly gates this; the oracle comment at `core/testgen/oracle.go` lines 12–13 documents it.

## Tech Debt

**Dynamic-introspection half is dead code:**
- Issue: `Introspect()` captures the fully-resolved `IdentitySet` (aliases, functions, env, path, options) but only `ids.Available` is consumed in `core/analyze/analyzer.go` line 84. All five resolved maps are populated and immediately discarded. The intended features — opaque-init identity detection, env-isolated introspection, static-vs-dynamic agreement — are unimplemented.
- Files: `core/shell/zsh/introspect.go` (`parseIntrospect` fills all maps); `core/analyze/analyzer.go` lines 83–89 (reads only `ids.Available`); `core/model/identityset.go` (full struct unused beyond `Available`).
- Impact: The spec's "static+dynamic agreement" tests cannot be written; `zsh -f` is spawned on every analysis for a single boolean result.
- Fix approach: Implement env-isolated introspection (`env -i zsh -f` + subtract zsh `-f` defaults), then wire the resolved set to flag live-but-unattributed identities and annotate "winning" definitions for duplicates.

**`##END##` sentinel not required:**
- Issue: `parseIntrospect` never checks whether `##END##` appeared in the introspect output. A truncated or partial zsh run silently returns `Available: true` with an incomplete resolved set.
- Files: `core/shell/zsh/introspect.go` lines 55–91 — no sentinel-seen guard, no fallback to `Available: false`.
- Impact: Low risk in v1 (the resolved set is not consumed), but becomes a correctness hazard when the resolved set is wired.
- Fix approach: Track `sawEnd bool`; set `Available = sawEnd` before returning.

**`CallExpr` export/typeset branch is dead code:**
- Issue: `parse.go` lines 86–104 handle `export`/`typeset`/`declare`/`local`/`readonly` inside a `*CallExpr`. Under `LangZsh`, mvdan/sh parses these keywords as `*DeclClause`, not `*CallExpr`, making the `CallExpr` branch unreachable for zsh input.
- Files: `core/shell/zsh/parse.go` lines 86–104.
- Impact: Dead code increases maintenance surface; keeping it without documentation risks confusion if a bash-fork variant is added that relies on it silently.
- Fix approach: Remove, or keep with explicit `// bash-fork fallback` comment and a test confirming it is unreachable under LangZsh.

**`Block.Exported` is set but never read:**
- Issue: The parser sets `Block.Exported = true` for `export` assignments in both `CallExpr` (dead) and `DeclClause` branches. No classifier, reconciler, or renderer reads `Block.Exported`. The env classifier (`core/shell/zsh/classify.go`) could prefer it for `CatEnvironment` / `CatPath` distinctions but does not.
- Files: `core/shell/zsh/parse.go` lines 90, 111 (sets `Exported`); `core/model/block.go` line 33 (field defined); no reader found anywhere.
- Impact: Missed opportunity: `typeset -x FOO` (flag-form export) is not handled at all; `Block.Exported` is never set for that form.
- Fix approach: Either consume `Block.Exported` in the classifier or drop the field; add flag-form export handling (`typeset -x`).

**Introspect error path returns nil maps:**
- Issue: When `cmd.Run()` returns an error, `Introspect` returns `model.IdentitySet{Available: false}` with all map fields nil (`core/shell/zsh/introspect.go` line 48). Future callers that write to these maps (e.g., `ids.Aliases[name] = true`) will panic.
- Files: `core/shell/zsh/introspect.go` lines 47–49.
- Impact: Safe in v1 (maps are not written by callers); becomes a latent panic when the resolved set is consumed.
- Fix approach: Return `model.IdentitySet{Available: false, Aliases: map[string]bool{}, Functions: map[string]bool{}, Env: map[string]bool{}, Options: map[string]bool{}}` on the error path.

## Classifier Accuracy

**PATH rule over-captures arbitrary variable names:**
- Issue: `core/shell/zsh/classify.go` line 39 uses `strings.Contains(u, "PATH")` as the final path-check fallback, which matches any variable name containing "PATH" as a substring (e.g., `PATHOLOGICAL_VAR`, `XPATH`, `DISPATCH_HANDLER`). These get classified as `CatPath` at `ConfMedium`.
- Files: `core/shell/zsh/classify.go` lines 38–41.
- Impact: Misclassified blocks appear in the `path` category rollup; ConfMedium means they are flagged for review rather than silently wrong, but false entries still pollute the output.
- Fix approach: Replace the `strings.Contains` fallback with `strings.HasSuffix(u, "PATH")` and ensure the explicit allowlist (`PATH`, `FPATH`, `MANPATH`, `CDPATH`) stays at the top.

**Secret regex over-captures on substrings:**
- Issue: `secretRe` in `core/shell/zsh/classify.go` line 10 matches anywhere in the variable name. Names like `TOKENIZER`, `PATHWAY_OPTIONS`, or `APIKEYLENGTH` match and are classified as `CatSecrets`.
- Files: `core/shell/zsh/classify.go` line 10 — `secretRe` pattern has no word-boundary anchors.
- Impact: Over-flagging (false positives). Current posture is deliberate fail-safe, but noisy output degrades UX.
- Fix approach: Add word-boundary anchors (`\b`) or require full-word matches; add corpus fixtures to confirm the tightened regex does not miss genuine secrets.

**`eval` plugin-init under-captures common tools:**
- Issue: The `eval` branch in `core/shell/zsh/classify.go` lines 53–57 only matches `init`, `hook`, and `shellenv` substrings. Common tools like `thefuck` (`eval "$(thefuck --alias)"`) and `dircolors` (`eval "$(dircolors)"`) do not match, falling through to `CatMisc` at `ConfLow`. The `pluginHints` fallback scan (lines 58–63) is inside the `default` case, making it unreachable for `KindCommand` blocks where `CmdName == "eval"`.
- Files: `core/shell/zsh/classify.go` lines 44–63.
- Impact: Valid plugin-init patterns misclassified as `misc`, suppressing recognition.
- Fix approach: Allow `pluginHints` scan to run on the unmatched-eval branch (fall through from the `eval` case instead of returning early to `CatMisc`).

## Fragile Areas

**Whole-file opaque fallback:**
- Files: `core/shell/zsh/parse.go` lines 22–28.
- Why fragile: Any single unparseable construct anywhere in the file causes `Parse` to return one giant opaque block covering the entire file. All classification, issue detection, and category rollups degrade to zero useful output. The entire analysis becomes `OpaqueBlocks: 1` with `Categories: []` and `Issues: []`.
- Safe modification: The fuzz survival test (`core/testgen/fuzz_test.go`) exercises engine robustness against corrupted input but does not assert recovery quality. A partial-parse fallback (skip the bad statement, keep surrounding ones) would require upstream changes to mvdan/sh or a re-parse-by-statement strategy.
- Test coverage: `core/testgen/fuzz_test.go` asserts no panic and valid JSON output; it does not assert that opaque blocks are bounded or that clean statements survive a partial parse failure.

**`dupPathIssues` regex misses relative entries and mis-names matched segments:**
- Files: `core/analyze/reconciler.go` lines 16–17 (`pathSegRe`), lines 61–80 (`duplicatePaths`).
- Why fragile: `pathSegRe` only matches `$HOME`-, `~`-, or `/`-rooted segments. Unrooted relative entries (e.g., `./scripts`, `bin`) are silently skipped. Additionally, `pathSegRe.FindAllString` operates on the raw block text, so a segment like `./scripts` matches as `/scripts` (the regex drops the `.`), producing a mis-named issue.
- Safe modification: Extend regex to match unrooted relative entries; test with fixtures containing relative PATH components.
- Test coverage: No existing corpus fixture exercises `duplicate_path`; the issue kind is only exercised via generated configs in `core/testgen`.

## Test Coverage Gaps

**No fixtures for `duplicate_path` or `shadowed` issue kinds:**
- What's not tested: The `core/testdata/fixtures/manifests.json` corpus covers `duplicate_alias` and `reassigned_env` only; `duplicate_path` and `shadowed` have no golden fixture.
- Files: `core/testdata/fixtures/` (directory); `core/testdata/fixtures/manifests.json`.
- Risk: Regressions in `duplicatePaths` or `shadows` detection go unnoticed by the corpus tests; only property-based tests (`core/testgen/property_test.go`) exercise these paths.
- Priority: High

**Missing Tier-1 fixture categories from the spec taxonomy:**
- What's not tested: Order-sensitivity (PATH precedence, fpath/compinit ordering), zsh syntax that mvdan/sh cannot parse (validates the opaque fallback path), structural edge cases (heredocs, multiline functions, process substitution), and pathological input (syntax errors, binary garbage).
- Files: `core/testdata/fixtures/` (directory).
- Risk: Real-world configs containing these patterns may trigger the whole-file opaque fallback silently; no test catches the regression.
- Priority: High

**Golden manifests assert issue `Kind` only, not names or lines:**
- What's not tested: `core/analyze/corpus_test.go` checks that the set of `IssueKind` strings matches but does not verify `Issue.Name` or `Issue.Lines`. A correct-kind but wrong-name detection passes the corpus test.
- Files: `core/analyze/corpus_test.go` lines 55–75; `core/testdata/fixtures/manifests.json` (no `issue_names` or `issue_lines` fields).
- Risk: Name-extraction regressions (e.g., alias-name parsing in `parse.go`) are invisible to the corpus suite.
- Priority: Medium

**`checkLines = false` pins two bugs instead of testing them:**
- What's not tested: `core/testgen/property_test.go` line 17 (`const checkLines = false`) permanently disables the `Lines` total and per-issue line comparison. No test currently validates correct line numbers from the engine.
- Files: `core/testgen/property_test.go` lines 17, 89–92.
- Risk: Line-number regressions are invisible. The two underlying bugs (off-by-one, comment mis-attribution) can drift worse without detection.
- Priority: Medium

**No Tier-2 real-world config coverage:**
- What's not tested: All fixtures are hand-crafted synthetic inputs. No scrubbed real-world `.zshrc` files are present for breadth testing.
- Files: `core/testdata/fixtures/` (directory).
- Risk: Unanticipated real-world patterns (plugin managers, framework-generated blocks, non-ASCII comments) may cause opaque fallback or misclassification without detection.
- Priority: Low

## Performance Bottlenecks

**`zsh -f` spawned per analysis, regardless of whether resolved set is used:**
- Problem: `core/analyze/analyzer.go` line 83 calls `az.provider.Introspect(path)` unconditionally. The introspection spawns a subprocess (`exec.CommandContext` with a 5-second timeout), sources the config, and dumps five identity tables — all to produce a single boolean (`ids.Available`).
- Files: `core/shell/zsh/introspect.go` lines 42–53; `core/analyze/analyzer.go` lines 83–89.
- Cause: The resolved `IdentitySet` is not consumed in v1, so the entire subprocess cost is overhead for a boolean.
- Improvement path: Either gate introspection behind an opt-in flag, or cache the result; once the resolved set is fully consumed, the cost becomes justified.

## Dependencies at Risk

**Go 1.25 requirement (supersedes stated 1.22 floor):**
- Risk: `go.mod` specifies `go 1.25.0`. The original design doc stated a 1.22 floor; mvdan/sh v3.13's zsh support forced the upgrade. CI environments or contributors with Go < 1.25 will get auto-toolchain upgrade via `GOTOOLCHAIN=auto`, which may silently download a toolchain.
- Impact: Unexpected toolchain downloads in air-gapped or locked-down CI. Contributors expecting Go 1.22 compatibility will hit `go.mod` enforcement.
- Migration plan: Document the Go 1.25 requirement prominently; update any README/contribution guides that reference 1.22.

## Missing Critical Features

**Dynamic analysis: static-vs-dynamic agreement unreachable:**
- Problem: The full spec vision ("static + dynamic agreement") requires the resolved `IdentitySet` to be wired into issue detection. As of v1, this is entirely absent. `eval "$(starship init zsh)"` and similar opaque inits that define identities at runtime are invisible to the engine.
- Blocks: Cannot flag live-but-unattributed identities; cannot annotate which definition "wins" for duplicates across static+dynamic sources; cannot distinguish inherited-env names from config-defined ones.

**No `--paths` / multi-file analysis:**
- Problem: The CLI accepts exactly one file path (`~/.zshrc` default). Users with split configs (`zshrc.d/` directories, sourced fragments) must run the tool once per file with no cross-file duplicate or shadow detection.
- Blocks: Any cross-file concern (duplicate alias across fragments, PATH entry added in one file and duplicated in another) is invisible.

---

*Concerns audit: 2026-06-23*
