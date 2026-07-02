# Phase 6: Ingest End-to-End — Claim-Validation Evidence Ledger

**Pass:** design-claims pass 1
**Date:** 2026-07-02
**Method:** codebase inspection + running the existing Go test suite (`go1.25.7`, `zsh` present). Phase 6 composes already-built Phase 2/3 pieces; claims about shipped code validated by reading source and running existing package tests. Claims about the Phase 6 *delta* (not-yet-built) validated as accurate absence descriptions.

## Status Tally

| Verdict | Count |
|---------|-------|
| PROVEN | 15 |
| STATIC-VALIDATED | 17 |
| REFUTED | 0 |
| UNVERIFIABLE | 0 |
| **Total** | **32** |

Environment: `go version go1.25.7 darwin/arm64`; `zsh` at `/bin/zsh`. `GOTOOLCHAIN=auto go build ./...` → exit 0. `go test ./core/ir/... ./core/store/...` → all pass.

---

## Claims Ledger

### A. Phase 2 IR seam (existing)

| id | claim | source | verdict | evidence |
|----|-------|--------|---------|----------|
| C01 | `ir.Build(blocks []model.Block, c shell.Classifier) model.Profile` builds a categorized regenerable Profile from parsed blocks | 06-SPEC.md:16, 06-CONTEXT.md:73 | STATIC-VALIDATED | `core/ir/build.go:21` signature exact: `func Build(blocks []model.Block, c shell.Classifier) model.Profile`. Appends one Entry per block in source order. |
| C02 | `ir.Regenerate(p model.Profile, r shell.Regenerator) []byte` regenerates behavior-equivalent zsh | 06-SPEC.md:16 | STATIC-VALIDATED | `core/ir/regen.go:21` signature exact. Emits in source order; managed entries via `r.Regenerate`, else verbatim `Text`. |
| C03 | `core/ir/route.go`'s `routeManaged` is the ING-02 declarative/imperative gate; imperative → verbatim Text, never templated | 06-SPEC.md:16, 06-CONTEXT.md:73,106 | STATIC-VALIDATED | `core/ir/route.go:35` `func routeManaged(b model.Block, cat model.Category) bool`. Doc + logic route non-admitted shapes imperative; `Build` sets `Managed: routeManaged(b, cat)`. |
| C04 | `model.Entry` carries `Text` (verbatim, D-01), `Category`, `Managed`, and `Override` with `EffectiveManaged()` | 06-SPEC.md:16, 06-CONTEXT.md:75 | STATIC-VALIDATED | `core/model/profile.go:23-36` struct has all fields (also `Kind`, `CmdName`, `Names`, `Value`, `Exported`, `Dynamic`, `Secret`). `EffectiveManaged()` at profile.go:42-51. |
| C05 | `EffectiveManaged()` is the single partition predicate `ir.Regenerate` gates on (regen.go:26) | 06-CONTEXT.md:74,106; 06-SPEC.md:36 (D-05) | PROVEN | `core/ir/regen.go:26` `if e.EffectiveManaged()`. Same predicate `Build` writes and store re-emits. Confirmed by passing `TestRegenRoundTrip`. |
| C06 | `EffectiveManaged()` applies `Override` over the auto `Managed` verdict (profile.go:42-51) | 06-CONTEXT.md:75 | STATIC-VALIDATED | `core/model/profile.go:42-51`: OverrideManaged→true, OverrideUnmanaged→false, else `e.Managed`. |
| C07 | The byte-identical round-trip oracle exists (`core/ir/roundtrip_test.go`) and holds | 06-SPEC.md:16,52; 06-CONTEXT.md:82 | PROVEN | File exists; `TestRegenRoundTrip` at roundtrip_test.go:28. `go test ./core/ir -run TestRegenRoundTrip -v` → PASS. Uses sandboxed `zsh -f` via `Provider.Introspect`, asserts DeepEqual IdentitySet across 5 classes + adversarial shapes. |
| C08 | Dynamic declarative values survive verbatim; regen emits captured Value unchanged (no resolution) | 06-SPEC.md:53,74,81 | PROVEN | `core/shell/zsh/regen.go:44-47` emits `Value` verbatim, never `%q`. Oracle asserts regen bytes contain literal `$HOME/go` and `$HOME/bin:$PATH` (roundtrip_test.go:130-135). PASS. |
| C09 | `Dynamic` flag is set by the parser as an AST-derived static/dynamic verdict (no execution) | 06-SPEC.md:18,81; 06-CONTEXT.md:20 | STATIC-VALIDATED | `core/shell/zsh/parse.go` sets `b.Dynamic` via `wordIsDynamic(...)` (lines 82,124,138,189,214). No exec. |

### B. Phase 3 store seam (existing)

| id | claim | source | verdict | evidence |
|----|-------|--------|---------|----------|
| C10 | `Store.Commit(ctx, branch, p, msg) (WithheldReport, error)` exists with that signature | 06-SPEC.md:17; 06-CONTEXT.md:76 | STATIC-VALIDATED | `core/store/store.go:211` `func (s *Store) Commit(ctx context.Context, branch string, p model.Profile, msg string) (WithheldReport, error)`. |
| C11 | `Commit` runs secret exclusion (`excludeSecrets` → SecretRef + keychain/vault capture) as first step | 06-SPEC.md:17,86; 06-CONTEXT.md:76 | PROVEN | `core/store/store.go:221` calls `excludeSecrets(ctx, p, s.keychain)` before any marshal/regen/commit. `TestCommitExcludesLiteralSecret` PASS. |
| C12 | `Commit` returns a `WithheldReport` naming what was withheld; already-dynamic secrets commit verbatim, not reported | 06-SPEC.md:18,38; 06-CONTEXT.md:42 | PROVEN | `excludeSecrets` (secret.go:72-159) appends `WithheldSecret{Name,StartLine}` for literals; dynamic entries `continue` (secret.go:102-106) uncaptured/unreported. `TestCommitDynamicSecretVerbatim` PASS. |
| C13 | The literal secret value never reaches the committed tree (cleared from both Text and Value, replaced by SecretRef placeholder) | 06-SPEC.md:38,39; 06-CONTEXT.md:107 (secret.go:136-153) | PROVEN | `core/store/secret.go:147-153` sets both `e.Text` and `e.Value` to the inert `secretRefValue` placeholder; `e.Secret=ref`. Comment at 136-153 matches. `TestCommitExcludesLiteralSecret` asserts committed blobs contain no literal. PASS. |
| C14 | `excludeSecrets` fails CLOSED: `ErrUnsafeSecretShape` (bad shape) / `ErrSecretBackendUnavailable` (nil backend), aborting before any ref moves | 06-SPEC.md:86; 06-CONTEXT.md:43,77 | PROVEN | secret.go:100,112 return `ErrUnsafeSecretShape`; secret.go:121 returns `ErrSecretBackendUnavailable`; secret.go:126 aborts on backend Store failure. Sentinels defined in errors.go:29,41. `TestCommitFailsClosed*` (3 tests) + `TestExcludeSecretsNilKeychain` PASS. |
| C15 | `WithheldReport` shape carries no secret value (only Name + StartLine) | 06-CONTEXT.md:42 | STATIC-VALIDATED | `core/store/store.go:55-58` `WithheldSecret{Name string; StartLine int}`; `WithheldReport = []WithheldSecret` (store.go:68). No value field. |
| C16 | `Store.Init` is idempotent (idempotent bare repo + `main` baseline); no clobber on existing store | 06-SPEC.md:17; 06-CONTEXT.md:76 (D-06) | PROVEN | `core/store/store.go:103-127`: returns nil if already bare repo; seeds `main` root commit if absent. `TestInitIdempotent` PASS. |
| C17 | Store exposes `Init`, `Branches`, `Current`, `Checkout`, `Create`, `Commit`, `Read` | 06-SPEC.md:17 | STATIC-VALIDATED | `core/store/store.go`: Init:103, Branches:132, Current:151, Checkout:163, Create:177, Commit:211, Read:323. All present. |
| C18 | Serialization is lossless; `Read(Commit(p))` round-trip is pinned | 06-SPEC.md:17; 06-CONTEXT.md:44 | PROVEN | `TestRoundTripReadCommit`, `TestRoundTripLossless`, `TestRoundTripSecretRef`, `TestRoundTripComposeOracle` all PASS. `Read` decodes profile.json via `UnmarshalProfile` (store.go:341). |
| C19 | Commit re-parents on the branch tip (re-ingest updates `main`, no duplicate entries; tree rewritten each time) | 06-CONTEXT.md:44 (store.go:257-265) | STATIC-VALIDATED | store.go:257-265 seeds temp index from branch tree + `revParse` parent when ref exists; else empty/root commit. Tree written fresh from the post-exclusion profile each Commit (store.go:228-284). |
| C20 | Store commit timestamp is fixed (`commitTS`) so the baseline is byte-reproducible | 06-CONTEXT.md:44 (store.go:31) | STATIC-VALIDATED | `core/store/store.go:31` `const commitTS = "1700000000 +0000"`, stamped via `runCommit` on every commit. |

### C. Parser / classifier / regenerator seam (existing)

| id | claim | source | verdict | evidence |
|----|-------|--------|---------|----------|
| C21 | `shell.Provider.Parse(src []byte) ([]model.Block, error)` is the entry point (opaque-fallback, never errors to caller) | 06-SPEC.md:19 | STATIC-VALIDATED | `core/shell/provider.go:10-12` `Parser.Parse(src []byte) ([]model.Block, error)`; `core/shell/zsh/parse.go:15` implements. Opaque fallback per parse.go doc/route.go `b.Opaque` handling. |
| C22 | `Classify` supplies per-block category | 06-SPEC.md:19; 06-CONTEXT.md:73 | STATIC-VALIDATED | `core/shell/provider.go:15-18` `Classifier.Classify(b) (Category, Confidence)`; zsh impl at classify.go:20. `Build` calls it (build.go:25). |
| C23 | `Provider.Regenerate` is the injected `shell.Regenerator`; total (default returns verbatim Text), emits dynamic values verbatim, never re-quoted | 06-CONTEXT.md:79 | STATIC-VALIDATED | `core/shell/provider.go:33-35` `Regenerator.Regenerate(e model.Entry) string`; zsh impl regen.go:24-72 with `default: return e.Text`; Value emitted verbatim. |
| C24 | CatSecrets classifier verdict + parser Dynamic flag drive store-side exclusion (no new detection) | 06-SPEC.md:18; 06-CONTEXT.md:77 | STATIC-VALIDATED | secret.go reads only `e.Category==CatSecrets`, `e.Dynamic`, `e.Value`, `e.Names` — no regex/AST in store; comment secret.go:9-14 confirms. |

### D. Layering / composition-root invariants

| id | claim | source | verdict | evidence |
|----|-------|--------|---------|----------|
| C25 | `core/cli` does not import `core/shell/zsh` (depends on `shell.Provider` interface) | 06-SPEC.md:84,96; 06-CONTEXT.md:85 | PROVEN | `core/cli/cli.go` production imports (lines 10-22) contain no `core/shell/zsh`; only interface `core/shell`. (Only `cli_test.go` imports the concrete provider — allowed.) |
| C26 | `core/ir` does not import `core/shell/zsh` | 06-SPEC.md:96; 06-CONTEXT.md:90 | PROVEN | build.go/regen.go/route.go production imports contain no `core/shell/zsh` (grep: no import). Only `roundtrip_test.go`/`regen_test.go`/`build_test.go` (comments/tests) reference it. |
| C27 | `core/store` does not import `core/shell/zsh`; stays shell-agnostic (uses `shell.Regenerator` seam) | 06-SPEC.md:84,96; 06-CONTEXT.md:90 | PROVEN | store.go/secret.go production imports have no `core/shell/zsh`; store.go imports `core/ir`, `core/model`, `core/shell` (interface). Comment store.go:10-13 states the invariant. |
| C28 | `core/cmd/zsh-pro/main.go` is the sole composition root; constructs `store.New(dir, provider, kc)` and injects provider into `cli.New`; store currently held unused | 06-SPEC.md:22,66,84; 06-CONTEXT.md:80 | STATIC-VALIDATED | main.go:19-36: `provider := zsh.Provider{}`; `s, err := store.New(dir, provider, kc)`; `_, _ = s, err`; `cli.New(provider).Run(...)`. Package doc declares sole-composition-root. |
| C29 | `util.ExpandHome` is applied at the CLI read boundary in `runAnalyze` (line ~63), never inside IR/store | 06-SPEC.md:81; 06-CONTEXT.md:29,78,87 | PROVEN | `core/cli/cli.go:63` `path = util.ExpandHome(path)` in `runAnalyze`. No `ExpandHome` in `core/ir` or `core/store` production code (grep: only comments in store/dto.go, store/dto_test.go asserting absence). `func ExpandHome` at util/path.go:11. |

### E. Phase 6 delta — accurate absence descriptions

| id | claim | source | verdict | evidence |
|----|-------|--------|---------|----------|
| C30 | No `ingest` verb exists: `CLI.Run`'s `switch args[0]` dispatches only `analyze` (via `buildinfo.Command`) | 06-SPEC.md:23; 06-CONTEXT.md:12 | PROVEN | `core/cli/cli.go:37-46` switch cases: `--version`/`-v`, `buildinfo.Command`, default. `buildinfo.Command = "analyze"` (buildinfo.go:6). No `case "ingest"`. |
| C31 | No end-to-end ingest orchestration; main.go constructs the store but holds it unused pending store-backed verbs | 06-SPEC.md:22 | PROVEN | main.go:34 `_, _ = s, err` with comment "not yet driven by a verb"; comment at 29-33 confirms verbs arrive later. No `Parse→Build→Commit(main)` flow in any production file. |
| C32 | No Phase 5 installer / BEGIN-END markers exist on disk yet (Phase 5 planned but not executed) — master-block routing + out-of-block detection are plan-time reconciliation gates | 06-SPEC.md:24,25; 06-CONTEXT.md:38,49,111 | PROVEN | `grep -rn ">>> zsh-pro" core/` → no matches; no installer/marker files in `core/shell/zsh/`. Confirms Phase 6 delta and D-07/D-12 forward-reference framing. |

---

## Notes

- No claim was refuted. Every falsifiable technical assertion in the SPEC/CONTEXT about shipped Phase 2/3 code was confirmed by source inspection, and behavioral claims are additionally backed by passing existing tests (`TestRegenRoundTrip`, `TestCommitExcludesLiteralSecret`, `TestCommitDynamicSecretVerbatim`, `TestCommitFailsClosed*`, `TestInitIdempotent`, `TestRoundTrip*`).
- Line-number references cited in CONTEXT (e.g. secret.go:136-153, store.go:211-303, store.go:257-265, regen.go:26, profile.go:42-51) match the current source within the noted ranges.
- The "does not exist yet" delta claims (C30-C32) are validated as *accurate descriptions of current absence*, which is the correct state for a phase that has not been executed. They are the work Phase 6 will add.
- Full suite spot-check: `GOTOOLCHAIN=auto go build ./...` exit 0; `go test ./core/ir/... ./core/store/...` all pass.
