# Phase 6: Ingest End-to-End - Pattern Map

**Mapped:** 2026-07-02
**Files analyzed:** 7 (2 modified, 3 new production, 2 new test) + 2 forward-reference reconciliation targets
**Analogs found:** 7 / 7 on-disk analogs; 2 analogs are Phase-5-planned-not-landed (flagged as plan-time reconciliation gates)

Phase 6 is a **composition phase** — it writes almost no genuinely new logic. Every new
surface mirrors an existing analog in `core/cli`, `core/render`, `core/dto`, `core/ir`,
or `core/store`. The two exceptions (store-injection seam, marker constants) are Phase 5
forward references: they do NOT exist on disk yet (`grep -rn ">>> zsh-pro" core/` → no
matches; `cli.New` is still 1-arg). Those are reconciliation gates, not free-invention.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `core/cli/cli.go` — `runIngest` method (modify) | controller | request-response (read file → orchestrate → report) | `core/cli/cli.go` `runAnalyze` (lines 49-82) | exact |
| `core/cli/cli.go` — `Run` verb dispatch `case "ingest"` (modify) | controller/route | request-response | `core/cli/cli.go` `Run` switch (lines 37-46) | exact |
| `core/cli/cli.go` — reuse `fail` for ingest errors (reuse) | controller | error-response | `core/cli/cli.go` `fail` (lines 88-100) | exact (unchanged) |
| ingest `--json` renderer (new — `core/render/*` + `core/dto/*`, or inline; D-04 discretion) | renderer + wire DTO | transform (model → wire) | `core/render/json.go` + `core/dto/envelope.go` | role-match |
| ingest orchestration (new — `runIngest` body, or a `core/ingest` pkg; discretion) | service | transform / CRUD (Parse→Build→Commit) | `core/analyze/analyzer.go` `Analyze` (pipeline orchestrator) | role-match |
| end-to-end round-trip test (new test) | test | event-driven (`zsh -f` execute-and-diff) | `core/ir/roundtrip_test.go` `TestRegenRoundTrip` | exact |
| real-shaped `~/.zshrc` fixture(s) (new) | fixture | file-I/O | inline `src := []byte(...)` in `roundtrip_test.go` (lines 42-62) | role-match |
| store injection into `CLI` (modify `main.go` + `core/cli`) | config / composition | dependency-injection | **Phase 5 PLAN 05-01 `cli.New(provider, s Store, e Emitter)`** — NOT ON DISK | forward-ref gate |
| out-of-block scan + marker constants (new detector; D-11/D-12) | utility | transform (line/marker scan) | **Phase 5 PLAN 05-02 installer `render()` + markers** — NOT ON DISK | forward-ref gate |

## Pattern Assignments

### `runIngest` on `CLI` (controller, request-response) — MODIFY `core/cli/cli.go`

**Analog:** `core/cli/cli.go` `runAnalyze` (lines 49-82) — verified live, exact template.

**Flag/arg parsing pattern to copy verbatim** (lines 49-63): default `~/.zshrc`,
one bare positional overrides the path, `--json` toggles, unknown `-flag` → `ExitUsageErr`,
`util.ExpandHome` applied ONLY at the read boundary (line 63):
```go
func (c *CLI) runAnalyze(args []string, stdout, stderr io.Writer) int {
	path := "~/.zshrc"
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case len(a) > 0 && a[0] == '-':
			_, _ = fmt.Fprintf(stderr, "zsh-pro: unknown flag %q\n", a)
			return int(model.ExitUsageErr)
		default:
			path = a
		}
	}
	path = util.ExpandHome(path)   // line 63 — CLI boundary ONLY; NEVER in IR/store path (EVAL-01, D-01)

	src, err := os.ReadFile(path)
	if err != nil {
		return c.fail(stdout, stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
	}
	// runIngest DIVERGES here: analyze does `analyze.New(c.provider).Analyze(src, path)`;
	// ingest does provider.Parse(src) → ir.Build(blocks, provider) → store.Init → store.Commit("main", ...)
	// then renders the ingest envelope (managed/master counts + withheld + out-of-block warning).
	...
	return int(a.ExitCode())   // ingest uses model.ExitClean on success, or the fail path
}
```

**Divergence for ingest** (D-01/D-05/D-10, all analogs verified live):
- `provider.Parse(src)` → `([]model.Block, error)` (`core/shell/provider.go:10`; never errors to caller — C21).
- `ir.Build(blocks, c.provider)` → `model.Profile` (`core/ir/build.go:21`, C01). The provider satisfies `shell.Classifier` (used as `c` arg).
- Partition each `Entry` on `EffectiveManaged()` (`core/model/profile.go:42-51`, C05/C06) — the SAME predicate `ir.Regenerate` gates on. `true` → committed managed; `false` → `Text` routed to master block. Phase 6 adds NO classification.
- `store.Init(ctx)` (idempotent, C16) THEN `store.Commit(ctx, "main", profile, msg)` → `(WithheldReport, error)` (C10/C11). Fixed deterministic `msg` (D-10).
- A `Commit` error → `c.fail(...)` (fail-closed, D-09 — never a leaky partial commit).

### `Run` verb dispatch (route, request-response) — MODIFY `core/cli/cli.go`

**Analog:** `core/cli/cli.go` `Run` switch (lines 37-46) — verified live.
```go
switch args[0] {
case "--version", "-v":
	...
case buildinfo.Command:            // buildinfo.Command == "analyze"
	return c.runAnalyze(args[1:], stdout, stderr)
default:
	_, _ = fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
	return int(model.ExitUsageErr)
}
```
Add `case "ingest": return c.runIngest(args[1:], stdout, stderr)` (D-02). Note the usage
string on line 34 (`"usage: zsh-pro analyze [path] [--json]"`) should also gain `ingest`.
**Reconciliation note:** Phase 5 PLAN 05-01/05-02 adds `list`/`status`/`checkout`/`activate`/`deactivate`/`install`
cases to this same switch. If Phase 5 has landed, Phase 6 ADDS `ingest` to the extended
switch (do not clobber Phase 5's cases).

### `fail` error contract (controller, error-response) — REUSE UNCHANGED

**Analog:** `core/cli/cli.go` `fail` (lines 88-100) — verified live, reuse as-is.
```go
func (c *CLI) fail(stdout, stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": buildinfo.Name, "version": buildinfo.Version, "command": buildinfo.Command,
			"ok": false, "error": msg, "exit_code": int(model.ExitRuntimeErr),
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		_, _ = fmt.Fprintln(stdout, string(b))   // agent contract: JSON to STDOUT even on failure
	} else {
		_, _ = fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return int(model.ExitRuntimeErr)
}
```
Ingest reuses this for read errors and `Commit` errors (D-02/D-09). NOTE: `fail` hardcodes
`"command": buildinfo.Command` (= "analyze"). For a correct ingest envelope the planner
should either parameterize the command label or accept the shared shape — Claude's discretion
(D-04), but flag it so the ingest `--json` error envelope does not misreport `"command": "analyze"`.

### ingest `--json` envelope (renderer + wire DTO, transform) — NEW (D-04, discretion)

**Analog:** `core/render/json.go` + `core/dto/envelope.go` — verified live.

Domain→DTO separation pattern (`core/render/json.go:15-64`): a `Render(model) ([]byte, error)`
that calls a private `toDTO(...)` mapping field-by-field, then `json.MarshalIndent(dto, "", "  ")`.
Envelope shape (`core/dto/envelope.go`):
```go
type Envelope struct {
	Tool        string   `json:"tool"`
	Version     string   `json:"version"`
	Command     string   `json:"command"`
	OK          bool     `json:"ok"`
	IssuesFound bool     `json:"issues_found"`
	ExitCode    int      `json:"exit_code"`
	Analysis    Analysis `json:"analysis"`
}
```
Ingest envelope carries (D-04): managed count, master-block line count, withheld list
(`Name` + `StartLine` only — NO value, C15), out-of-block warning list, ok/exit_code.
**Hard constraints (D-04):** no `zsh` text, no `model`/`store` types, no secret VALUE on the
wire. Whether this is a dedicated `dto.IngestResult` + `render.IngestRenderer` or a small
inline struct is Claude's discretion (OQ-06-03; safe default = dedicated DTO mirroring the
existing separation). `WithheldReport` = `[]WithheldSecret{Name string; StartLine int}`
(`core/store/store.go:55-68`, C15) — map it field-by-field into the wire list, do not embed
the store type directly.

### ingest orchestration flow (service, transform/CRUD) — NEW (discretion: `runIngest` body vs `core/ingest` pkg)

**Analog:** `core/analyze/analyzer.go` `Analyzer.Analyze` — the existing single-pass
pipeline orchestrator (Parse → Classify → Introspect → Reconcile), injected with the
`shell.Provider` interface via `New(p shell.Provider)`. Ingest's pipeline is
Parse → `ir.Build` → `store.Init` → `store.Commit("main")` + master-block routing.

**Store call sequence** (verified live, C16/C10):
```go
// core/store/store.go:103 — Init is idempotent (no clobber), seeds main baseline
func (s *Store) Init(ctx context.Context) error { if s.git.isBareRepo(ctx) { return nil } ... }
// core/store/store.go:211 — Commit runs excludeSecrets FIRST, returns the withheld report
func (s *Store) Commit(ctx context.Context, branch string, p model.Profile, msg string) (WithheldReport, error)
```
**Layering constraint (D-15, hard SPEC acceptance C25/C26/C27):** if the orchestration is a
new `core/ingest` package, it must import only `core/ir`, `core/store`, `core/model`,
`core/shell` (interface) — NEVER `core/shell/zsh`. Safe default per Discretion: keep it as a
`runIngest` method on `CLI` (mirrors `runAnalyze`), no new package.

### end-to-end round-trip test (test, event-driven) — NEW

**Analog:** `core/ir/roundtrip_test.go` `TestRegenRoundTrip` (lines 1-70) — verified live, exact template.

**External test package + LookPath skip-guard** (lines 1-31): the test lives in `ir_test`
(external package) so it can wire the concrete `zsh.Provider` without polluting production
layering, and skips cleanly when `zsh` is absent:
```go
package ir_test   // external package — the ONLY place that wires zsh.Provider to agnostic core/ir

func TestRegenRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping round-trip oracle")
	}
	...
	p := zsh.Provider{}
	blocks, err := p.Parse(src)
	...
}
```
Phase 6's end-to-end test (D-13, C07): ingest a real-shaped fixture → `Store.Read(ctx, "main")`
(`core/store/store.go:323`, C18) → `ir.Regenerate(profile, provider)` (`core/ir/regen.go:21`, C02)
→ assert the Phase 2 oracle (regenerated parses to a `model.Profile` structurally equal to
committed; verbatim/imperative statements byte-identical; declarative slices behavior-equivalent
via `zsh -f` execute-and-diff). Because it Commits, it lives in an external test package that
may wire both `zsh.Provider` and the concrete `*store.Store` (allowed for tests — C25).

**Identical LookPath skip pattern** also in `core/shell/zsh/introspect_test.go:11-12,40-41` and
`core/analyze/corpus_test.go` — the established convention for `zsh`-requiring tests.

### real-shaped `~/.zshrc` fixture (fixture, file-I/O) — NEW (D-14, discretion)

**Analog:** inline `src := []byte("...")` fixture in `roundtrip_test.go:42-62` (verified live),
which already covers static/dynamic env, PATH, alias, option, function, and adversarial shapes.

Phase 6 fixture MUST additionally cover (D-14): a dynamic-but-declarative value
(`export GOPATH=$HOME/go` — stays managed, `$HOME` intact per EVAL-01/C08); imperative lines
(`eval "$(...)"`, function def, `compinit` — route to master block, `EffectiveManaged()==false`);
a literal secret (`export API_TOKEN=sk-abc123` — excluded + reported, C11/C13); an already-dynamic
secret (`export TOKEN=$(op read ...)` — committed verbatim, not reported, C12); and — for
idempotency + out-of-block — a variant with the managed BEGIN/END block already present plus
trailing installer-appended lines (`# added by nvm` + `export NVM_DIR=...`). One comprehensive
fixture or a small focused family is Claude's discretion. Falsifiable properties each pinned by
a test (zero-dropped line accounting, grep-verified secret absence in committed blobs, byte-identical
managed block on re-ingest, no-clobber out-of-block, Phase 2 oracle end-to-end).

## Shared Patterns

### `EffectiveManaged()` — the one partition predicate
**Source:** `core/model/profile.go:42-51` (verified live, C05/C06)
**Apply to:** ingest orchestration (master-block split) + round-trip test
```go
func (e Entry) EffectiveManaged() bool {
	switch e.Override {
	case OverrideManaged:   return true
	case OverrideUnmanaged: return false
	default:                return e.Managed
	}
}
```
This is the EXACT boolean `ir.Regenerate` gates on (`core/ir/regen.go:26`). The store view
(managed) and the master block (imperative) partition the source with no overlap, no gap. Phase 6
adds zero new classification — the precision-over-recall gate stays in `core/ir/route.go` (untouched, C03).

### `util.ExpandHome` at the CLI read boundary ONLY
**Source:** `core/cli/cli.go:63` (verified live, C29)
**Apply to:** `runIngest` path resolution only — NEVER inside `core/ir` or `core/store` (EVAL-01, SPEC constraint).

### Domain vs wire separation
**Source:** `core/render/json.go` + `core/dto/` (verified live)
**Apply to:** ingest `--json` output. No store/model type and no zsh text on the wire.

### Single composition root
**Source:** `core/cmd/zsh-pro/main.go:1-14` package doc + `_, _ = s, err` at line 34 (verified live, C28)
**Apply to:** store injection (below). Only `main.go` imports `core/shell/zsh` + concrete `store`.

## Forward-Reference Reconciliation Gates (Phase 5 planned, NOT on disk)

These two analogs do NOT exist in `core/` yet — confirmed: `grep -rn ">>> zsh-pro" core/` returns
nothing (C32) and `cli.New` is still `func New(p shell.Provider) *CLI` (1-arg, `core/cli/cli.go:28`, C28/C30).
The planner must treat these as reconciliation gates (the same discipline Phase 5 used against
Phase 4's `emit.go`), NOT as green-field invention. The behavior is locked; the physical seam
reconciles against Phase 5's ACTUAL landed code at plan time.

### GATE 1 — Store injection into `CLI` (D-15/D-16)

**Forward reference:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-01-PLAN.md`
(lines 40-41, 274-276, 322-337). Phase 5 D-19 PLANS:
- A narrow `type Store interface` in a NEW file `core/cli/store.go`, satisfied by `*store.Store`,
  with EXACTLY: `Branches(ctx) ([]string, error)`, `Current() string`, `Checkout(ctx, name) error`,
  `Read(ctx, branch) (model.Profile, error)`.
- Signature change: `New(p shell.Provider)` → `New(p shell.Provider, s Store, e Emitter) *CLI`.
- `main.go`: remove `_, _ = s, err` (line 34), thread `s`+`err` in via an explicit-nil-interface
  pattern (`var cliStore cli.Store; if err == nil { cliStore = s }`) so a typed-nil `*store.Store`
  never reaches the interface; store-backed verbs route through `fail`, not panic (C10).

**Phase 6 obligation (D-16):** EXTEND the Phase 5 `cli.Store` interface with `Init(ctx) error`
and `Commit(ctx, branch, p, msg) (WithheldReport, error)` — do NOT define a second injection path.
Both methods already exist on `*store.Store` (`core/store/store.go:103` and `:211`, C16/C10), so the
interface widens without any store change. **If Phase 5 has NOT landed at Phase 6 plan time:** treat
the D-19 contract above as the reconciliation baseline, make the `cli.New` signature change +
narrow interface yourself (adding all six methods: Branches/Current/Checkout/Read/Init/Commit), and
flag that Phase 6 has ABSORBED the Phase 5 seam so Phase 5 must reconcile against it instead.
`core/cli` must import NEITHER `core/store` NOR `core/shell/zsh` (grep == 0, C25).

### GATE 2 — Out-of-block scan + BEGIN/END marker constants (D-11/D-12)

**Forward reference:** `.planning/phases/05-runtime-loader-cli-bootstrap/05-02-PLAN.md`
(lines 31-32, 89-110, 142-143). Phase 5 D-09/D-10 PLANS:
- Marker constants (single source of truth): `begin = "# >>> zsh-pro >>>"`, `end = "# <<< zsh-pro <<<"`.
- An installer in `core/cli` with `render()` (canonical block) + a find-region-then-replace rewrite:
  balanced BEGIN..END → byte-exact replace preserving outside-marker bytes; unbalanced → refuse-and-fail.
- Everything OUTSIDE `[BEGIN, END]` is preserved byte-for-byte by construction (D-10/D-12) — the
  master block IS the user's own `.zshrc` content outside the markers.

**Phase 6 obligation (D-11/D-12):** the out-of-block detector is READ-ONLY. Scan `~/.zshrc`
with a simple line/marker string search (NOT a re-parse): locate the `end` marker, and if
non-blank/non-comment content exists AFTER it, emit a warning and exit WITHOUT error (a warning,
not a failure — SPEC Req 4). Never clobber/reorder — Phase 5's rewrite already preserves outside-marker
bytes. Reuse Phase 5's marker constants (single source of truth); Phase 6 does NOT re-derive the
marker text. Whether the detector co-locates with the Phase 5 installer (natural home — already has
the markers + the `~/.zshrc` read path) or lives in the ingest flow reading a marker constant is
Claude's discretion (OQ-06-04), PROVIDED `core/cli` holds no zsh/marker text if the constant crosses
a package boundary, and the detector is read-only. **If Phase 5 has NOT landed:** define the marker
constants against the Phase 5 D-09 contract (`# >>> zsh-pro >>>` / `# <<< zsh-pro <<<`) and flag
that they must reconcile to Phase 5's actual installer package once it lands (single source of truth).

## No Analog Found

None. Every Phase 6 surface has either an on-disk analog (7) or a Phase-5-planned analog (2, flagged
above as reconciliation gates). No file needs to fall back to RESEARCH.md-only patterns.

## Metadata

**Analog search scope:** `core/cli`, `core/render`, `core/dto`, `core/ir`, `core/store`, `core/model`,
`core/cmd/zsh-pro`, `core/shell`, `core/analyze` (tests); Phase 5 PLANs 05-01/05-02 (forward refs).
**Files scanned:** cli.go, main.go, json.go, envelope.go, roundtrip_test.go, store.go, profile.go,
introspect_test.go, plus Phase 5 05-01/05-02 PLAN grep.
**Grep confirmations:** `grep -rn ">>> zsh-pro" core/` → no matches (C32); `cli.New` still 1-arg (C28).
**Pattern extraction date:** 2026-07-02
