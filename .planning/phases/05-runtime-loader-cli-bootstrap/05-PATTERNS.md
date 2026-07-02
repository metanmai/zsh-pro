# Phase 5: Runtime Loader + CLI + Bootstrap - Pattern Map

**Mapped:** 2026-07-02
**Files analyzed:** 8 (5 new, 3 modified)
**Analogs found:** 7 / 8 (one file has no analog — the Go `.zshrc` installer)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `core/shell/zsh/hook.go` (NEW — loader `const` + `HookScript()`) | provider/config | transform (const→string) | `core/shell/zsh/introspect.go` (`introspectScript` const + method) | exact (role+flow) |
| `core/shell/provider.go` (MODIFIED — add `Hooker`/`HookScript()` to seam) | interface seam | request-response | existing `Regenerator`/`Introspector` sub-interfaces in same file | exact (in-file precedent) |
| `core/cli/cli.go` (MODIFIED — add `hook`/`emit`/`list`/`status`/`install` cases) | controller | request-response | `CLI.Run` `switch args[0]` `analyze` case + `runAnalyze` + `fail` | exact (role+flow) |
| `core/cli/<store-iface>.go` (NEW — narrow store-facing interface in `core/cli`) | interface seam | request-response | `shell.Provider` injection discipline in `core/cli/cli.go` + `store.KeychainDriver` decl-at-first-use | exact (pattern) |
| `core/cli/install.go` (NEW — `.zshrc` find-region-then-replace) | utility | file-I/O | `vaultKeychain.save` atomic-ish write (`core/store/keychain.go:309`); `os.CreateTemp`+rename idiom (`core/store/store.go:245`) | role-match (no byte-exact rewrite precedent) |
| `core/cmd/zsh-pro/main.go` (MODIFIED — thread store into `cli.New`) | composition root | request-response | existing `store.New(dir, provider, kc)` construction + `cli.New(provider)` wiring in same file | exact (in-file) |
| `core/shell/zsh/hook_test.go` (NEW — `hook \| zsh -n`, grep verbs, zero-subprocess) | test | request-response | `core/shell/zsh/introspect_test.go` (`LookPath("zsh")` skip-guard + `zsh -f` subprocess) | exact (role+flow) |
| `core/shell/zsh/live_terminal_test.go` (NEW — sourced-loader zero-residue) | test | event-driven | `introspect_test.go` `LookPath` guard + `core/store/roundtrip_test.go:149` zsh-guarded roundtrip | exact (pattern) |

## Pattern Assignments

### `core/shell/zsh/hook.go` (provider/config, transform) — NEW

**Analog:** `core/shell/zsh/introspect.go`

This is the primary structural precedent. The loader is a Go `const` raw-string in `core/shell/zsh` exposed via a method on `Provider`, mirroring `introspectScript` exactly.

**Const embed pattern** (`introspect.go:23-38`):
```go
// introspectScript runs under `zsh -f` (no rc files). It sources the target
// ($1) with output suppressed, then dumps the resolved identity tables in a
// section-delimited format.
const introspectScript = `
emulate -L zsh
zmodload zsh/parameter 2>/dev/null
source "$1" >/dev/null 2>&1
...
`
```

**CRITICAL DELIBERATE DIVERGENCE (D-04, Pitfall 4):** `introspectScript` opens with `emulate -L zsh` for ISOLATION. The Phase 5 loader's apply/deactivate helpers and the emitted code they `eval` must be PLAIN (no `emulate -L`, no `LOCAL_OPTIONS`) so `setopt`/`unsetopt` escape function scope and persist in the terminal. Copy the `const`-raw-string *shape*, NOT the `emulate -L zsh` header. The `zp_*` helper bodies themselves are pure definitions (function/const), which is why `hook | zsh -n` == 0 (RESEARCH E14).

**Method-on-zero-value-receiver pattern** (`introspect.go:18`, `introspect.go:42`):
```go
func (Provider) Categories() []model.Category { return model.Categories() }
// ...
func (p Provider) Introspect(path string) (model.IdentitySet, error) {
```
`HookScript() string` is the new method — a pure const print, no receiver state, matching `Categories()`'s no-error signature (cannot fail).

**Interface-satisfaction compile guard** (`introspect.go:15`):
```go
var _ shell.Provider = Provider{}
```
When `Hooker` is added to the composite `Provider`, this line keeps enforcing satisfaction at compile time — no change needed if `HookScript()` is added to `zsh.Provider`.

**`zp_*` helper bodies inside the const** — derive from Phase 1 Loader Reference Snippet WITH the OQ-8 correction (CONTEXT D-01, RESEARCH sub-area (a) table). Load-bearing forms (all VERIFIED in RESEARCH):
- env unset-vs-empty: `[[ "${(P)+var}" == "1" ]]` (E5) — NOT `-z`
- shadow-restore set-test: `[[ -n "${slot+x}" ]]` (E6) — NOT `[[ -n "$slot" ]]`
- PATH rebuild-from-base: `PATH="$ZP_BASE_PATH"; path=(<additions> $path)` (E7) — never `typeset -U`, never blind append
- slot-name sanitize BEFORE `typeset -g`/`(P)`: `${name//[^A-Za-z0-9_]/_}` (E16 — dual-purpose correctness + injection block)
- `ZP_BASE_PATH` once-capture guard (lives in sourced activate path, not emitted code): `[[ "${ZP_BASE_PATH+x}" == "x" ]] || typeset -g ZP_BASE_PATH="$PATH"` (E8)

> **PLAN-TIME RECONCILIATION GATE (OQ-05-05/OQ-05-13):** `core/shell/zsh/emit.go` and `core/activate` do NOT exist on disk yet (Phase 4 not executed — confirmed by `ls core/shell/zsh/` shows no `emit.go`, and `core/activate` is absent). Phase 4 commits BY NAME only to `zp_capture_env`/`zp_restore_env`; shadow/PATH/option reverse ops are documented INLINE. Do NOT assume `zp_rebuild_path`/`zp_shadow_*` helpers are in the contract. The loader definitively provides the two named env helpers + the runtime STATE slots the inline ops read/write. Diff against `emit.go`'s ACTUAL bare-call surface once Phase 4 lands.

---

### `core/shell/provider.go` (interface seam, request-response) — MODIFIED

**Analog:** the sibling sub-interfaces in the same file (`Regenerator`, `Introspector`).

**ISP sub-interface + composite pattern** (`provider.go:33-43`):
```go
// Regenerator emits behavior-equivalent forward zsh source for a declarative
// entry. It is the single place forward zsh syntax is generated this phase
// (the milestone invariant pins zsh-syntax codegen to the zsh package)...
type Regenerator interface {
	Regenerate(e model.Entry) string
}

// Provider composes the four concerns for convenient wiring.
type Provider interface {
	Parser
	Classifier
	Introspector
	Regenerator
}
```

**Apply:** Add a `Hooker` sub-interface (D-20 default seam name `HookScript()`), mirroring `Regenerator`'s single-method, no-error, doc-commented shape — and note in the doc comment that it, like `Regenerator`, is a place zsh syntax originates (milestone invariant). Add `Hooker` to the `Provider` composite. `core/cli` then calls `c.provider.HookScript()` and never holds zsh text.

```go
// Hooker returns the embedded zsh runtime loader (the sourced verbs +
// zp_* helper/state definitions). Like Regenerator, it is a place zsh
// syntax originates, so it lives on the shell seam and the const is in
// core/shell/zsh — core/cli holds no zsh text (D-20).
type Hooker interface {
	HookScript() string
}
```

---

### `core/cli/cli.go` (controller, request-response) — MODIFIED

**Analog:** the existing `analyze` dispatch + `runAnalyze` + `fail` in the same file.

**Switch-dispatch pattern** (`cli.go:32-47`):
```go
func (c *CLI) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: zsh-pro analyze [path] [--json]")
		return int(model.ExitUsageErr)
	}
	switch args[0] {
	case "--version", "-v":
		...
	case buildinfo.Command:
		return c.runAnalyze(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
		return int(model.ExitUsageErr)
	}
}
```

**Apply (D-18):** Add `case "hook"`, `case "emit"`, `case "list"`, `case "status"`, `case "install"` — each delegating to a `runX` method (mirroring `runAnalyze`). `hook`/`install` need no store; `list`/`status`/`emit` need the store. Reuse the typed `model.ExitCode` returns (`ExitClean`/`ExitRuntimeErr`/`ExitUsageErr`/`ExitActionable`).

**Per-verb handler pattern** (`cli.go:49-82` `runAnalyze`):
- parse flags in a `for _, a := range args` loop, unknown `-flag` → `ExitUsageErr`
- do the work, propagate errors through `c.fail(...)`
- write result to `stdout`, `return int(...ExitCode)`
- `hook` handler is the simplest: `_, _ = fmt.Fprintln(stdout, c.provider.HookScript()); return int(model.ExitClean)`

**Error-envelope pattern** (`cli.go:84-100` `fail`):
```go
func (c *CLI) fail(stdout, stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": buildinfo.Name, "version": buildinfo.Version, "command": buildinfo.Command,
			"ok": false, "error": msg, "exit_code": int(model.ExitRuntimeErr),
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		_, _ = fmt.Fprintln(stdout, string(b))
	} else {
		_, _ = fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return int(model.ExitRuntimeErr)
}
```
Reuse `fail` unchanged for store-backed verb errors (store-init failure, `ErrProfileNotFound`, etc.) — the agent-contract single-JSON-on-stdout invariant carries over.

---

### `core/cli/<store interface>.go` (interface seam, request-response) — NEW

**Analog:** `shell.Provider` injection discipline (`cli.go:5-8`, `cli.go:24-28`) + `store.KeychainDriver` declare-at-first-use (`core/store/store.go:44-49`).

**Injection discipline** (`cli.go:5-8`, `cli.go:24-28`):
```go
// cli depends on the core/shell interface, not the concrete implementation:
// the composition root (cmd/zsh-pro) is the only package that imports
// core/shell/zsh and injects it here via New.

type CLI struct{ provider shell.Provider }

func New(p shell.Provider) *CLI { return &CLI{provider: p} }
```

**Apply (D-19, OQ-05-09 default):** Declare a narrow store-facing interface IN `core/cli` (so `core/cli` never imports the concrete `*store.Store`), satisfied by `*store.Store`. Extend `New(p shell.Provider, s Store) *CLI`. The interface exposes only what the verbs need:
```go
// Store is the narrow, cli-local view of the profile store the store-backed
// verbs need. It is satisfied by *store.Store, injected at the composition
// root — core/cli never imports the concrete store (mirrors shell.Provider).
type Store interface {
	Branches(ctx context.Context) ([]string, error) // list
	Current() string                                 // status
	Checkout(ctx context.Context, name string) error // checkout validate
	Read(ctx context.Context, branch string) (model.Profile, error) // emit source
}
```
Precedent for declaring the contract at first use (not in the concrete package): `store.KeychainDriver` doc comment, `store.go:44-49` — "declared HERE, at first use, because the ... signature below references it."

**Store data-source methods** (all EXIST — `core/store/store.go`): `Branches` (:132, for `list`), `Current` (:151, reads `ZSHPRO_PROFILE`, unset⇒`main`, for `status`), `Checkout` (:163, validates existence, does NOT export), `Read` (:323, emit source; distinguishes absent-branch vs empty-profile).

---

### `core/cli/install.go` (utility, file-I/O) — NEW — NO EXACT ANALOG

**Nearest analogs:** `vaultKeychain.save` (`core/store/keychain.go:309-327`) for deterministic-content file write; `os.CreateTemp`+`defer os.Remove`+rename idiom (`core/store/store.go:245-253`).

There is NO existing byte-exact find-region-then-replace `.zshrc` rewriter — this is genuinely new (see No Analog Found). Two reusable sub-patterns:

**Deterministic write for byte-identical re-run** (`keychain.go:309-327`) — the vault sorts keys so "an otherwise-identical vault always serializes byte-identically instead of churning." Same discipline applies: `render()` the marked block deterministically so a second `install` produces a byte-identical region (BOOT-01 acceptance). Perm note: vault uses `0o600`; `.zshrc` should follow the file's existing perms or `0o644` (it is not secret material).

**Atomic-replace idiom** (`store.go:245-253`):
```go
idxFile, err := os.CreateTemp("", "zshpro-index-*")
...
defer func() { _ = os.Remove(idx) }()
```
Extend to write-temp-in-same-dir + `os.Rename` for the `.zshrc` replacement (RESEARCH sub-area (c), A2 assumption: standard Go crash-safe idiom). The core algorithm (RESEARCH sub-area (c), D1 — proven byte-identical): `strings.Index` BEGIN→END scan, emit fresh block once at first region, drop subsequent regions (dup-collapse), preserve outside-marker bytes verbatim; append with one leading `\n` if no marker; create-with-just-block if file absent.

Markers (D-09): `# >>> zsh-pro >>>` / `# <<< zsh-pro <<<`. The installed block is a THIN STUB (D-11/D-14) — guard order: `ZSHPRO_DISABLE` early-return → `[[ -r <cached-loader> ]]` → source. `install` also writes the cached loader file (`zsh-pro hook` output) under the store/data dir (zero-subprocess hot path, D-13).

---

### `core/cmd/zsh-pro/main.go` (composition root, request-response) — MODIFIED

**Analog:** the file itself — store construction and `cli.New` wiring already coexist here.

**Existing construction + reserved-store comment** (`main.go:26-36`):
```go
dir := storeDir()
kc := store.NewOSKeychainDriver(dir)
s, err := store.New(dir, provider, kc)
// The CLI verbs that consume the store (checkout/create/list/status) arrive in
// Phase 5; the store is constructed + injectable now but not yet driven by a verb.
// ... intentionally non-fatal here and surfaces when a store-backed verb is wired in Phase 5.
_, _ = s, err

os.Exit(cli.New(provider).Run(os.Args[1:], os.Stdout, os.Stderr))
```

**Apply (D-19):** Thread `s` into `cli.New(provider, s)`. Keep store-init `err` NON-FATAL — `analyze`/`hook`/`install` do not need the store; the error surfaces only inside a store-backed verb handler (`list`/`status`/`checkout`/`emit`) via `c.fail`. This matches the existing comment's contract exactly; remove the `_, _ = s, err` placeholder. `main.go` remains the SOLE importer of `core/shell/zsh` and concrete `store` (package doc, `main.go:1-4`).

---

### `core/shell/zsh/hook_test.go` (test, request-response) — NEW

**Analog:** `core/shell/zsh/introspect_test.go`

**LookPath skip-guard** (`introspect_test.go:10-13`):
```go
if _, err := exec.LookPath("zsh"); err != nil {
	t.Skip("zsh not installed; skipping dynamic introspection test")
}
```

**Apply:** `hook | zsh -n` == 0 (spawn `zsh -n` on `HookScript()` output — mirrors `introspect.go`'s `exec.CommandContext(ctx, "zsh", ...)` subprocess); grep the emitted string for the 5 verbs + the `zp_capture_env`/`zp_restore_env` names + `ZP_UNSET_SENTINEL`; and a PURE-Go structural zero-subprocess assertion on the stub start-path portion (no `$(`, no backtick, no `git `, no `zsh-pro` in command position — RESEARCH E13). The zero-subprocess grep needs NO zsh (pure string assert on the const) — do NOT put it behind the `LookPath` guard.

---

### `core/shell/zsh/live_terminal_test.go` (test, event-driven) — NEW

**Analog:** `core/shell/zsh/introspect_test.go` (`LookPath` guard + `t.TempDir()` + `os.WriteFile` fixture) and `core/store/roundtrip_test.go:149` (zsh-guarded end-to-end roundtrip).

**Fixture-write + guard pattern** (`introspect_test.go:14-18`):
```go
dir := t.TempDir()
cfg := filepath.Join(dir, "rc.zsh")
content := "alias gs='git status'\n..."
if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil { t.Fatal(err) }
```

**Apply:** `LookPath("zsh")`-guarded test that sources the loader into a `zsh` subprocess, runs `activate A` → asserts live alias/env/PATH → `deactivate` → asserts byte-identical snapshot + `$#path` stable (RESEARCH E15 zero-residue A→B→deactivate). Sub-cases: `ZSHPRO_DISABLE=1` defines no verbs (E10); absent-binary reaches prompt (E11); emitted code failing `zsh -n` is NOT eval'd + last-good intact (E3). Mirrors the Phase 4 property-test discipline of asserting a byte-identical PRE/POST snapshot.

---

## Shared Patterns

### Provider-seam injection (milestone invariant)
**Source:** `core/cli/cli.go:24-28`, `core/shell/provider.go:33-43`, `core/cmd/zsh-pro/main.go:1-4`
**Apply to:** `hook.go` (const lives in `core/shell/zsh`), `provider.go` (`Hooker` sub-interface), `cli.go` (calls `provider.HookScript()`), `main.go` (sole concrete importer)
Only `core/shell/zsh` writes zsh syntax; `core/cli` reaches it through the seam and holds NO zsh text. The `var _ shell.Provider = Provider{}` compile guard (`introspect.go:15`) enforces satisfaction.

### Typed exit codes + agent JSON contract
**Source:** `core/model/exitcode.go` (`ExitClean`/`ExitRuntimeErr`/`ExitUsageErr`/`ExitActionable`), `core/cli/cli.go:84-100` (`fail`)
**Apply to:** all new verb handlers in `cli.go`
Every verb returns `int(model.ExitX)`; every runtime error routes through `c.fail` so `--json` emits exactly one JSON object on stdout on success OR failure.

### Declare-contract-at-first-use for injected seams
**Source:** `core/store/store.go:44-49` (`KeychainDriver`), `core/shell/provider.go` (all sub-interfaces)
**Apply to:** the cli-local `Store` interface (declared in `core/cli`, satisfied by `*store.Store`) and the `Hooker` seam
Interfaces are declared in the CONSUMER package (or the seam package), never importing the concrete implementation; the concrete type is wired only at `main.go`.

### LookPath-guarded subprocess tests
**Source:** `core/shell/zsh/introspect_test.go:10-13`, `core/store/roundtrip_test.go:149`, `core/ir/roundtrip_test.go:29`
**Apply to:** `hook_test.go` (`hook | zsh -n`), `live_terminal_test.go` (sourced-loader zero-residue)
`if _, err := exec.LookPath("zsh"); err != nil { t.Skip(...) }` — skip cleanly when zsh is absent. Pure-Go structural asserts (zero-subprocess grep on the const) stay OUTSIDE the guard.

### Sandboxed zsh subprocess (5s timeout, graceful degradation)
**Source:** `core/shell/zsh/introspect.go:42-53` (`exec.CommandContext(ctx, "zsh", ...)` + `context.WithTimeout(..., 5*time.Second)` + error→degrade)
**Apply to:** the `zsh -n` validation subprocess (D-15) if it runs from Go; the live-terminal test harness
The `zsh -n <<< "$code"` gate is off the hot path (explicit verb only) — a subprocess is acceptable there, mirroring `Introspect`.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `core/cli/install.go` | utility | file-I/O | No existing byte-exact find-region-then-replace file rewriter. Nearest sub-patterns (deterministic write in `keychain.go:309`, `os.CreateTemp`+rename in `store.go:245`) are reusable fragments, but the BEGIN/END-marked idempotent `.zshrc` rewrite algorithm is new. Planner: use RESEARCH sub-area (c) D1 algorithm (proven byte-identical Go POC) + the atomic-rename idiom. |

**Forward references (do NOT treat as existing analogs):** `core/shell/zsh/emit.go` and `core/activate` do not exist on disk yet (Phase 4 unexecuted — confirmed by `ls`). The `zp_*` helper/state surface is a PLAN-TIME reconciliation gate against Phase 4's actual `emit.go` bare-call surface (OQ-05-05/OQ-05-13), NOT a settled contract to match today.

## Metadata

**Analog search scope:** `core/cli/`, `core/shell/`, `core/shell/zsh/`, `core/store/`, `core/cmd/zsh-pro/`, `core/model/`, `core/buildinfo/`
**Files scanned:** `cli.go`, `cli_test.go`, `provider.go`, `main.go`, `introspect.go`, `introspect_test.go`, `store.go`, `keychain.go`, `exitcode.go`, `buildinfo.go` + grep sweep for `LookPath`/`os.Rename`/`CreateTemp`/`WriteFile`
**Pattern extraction date:** 2026-07-02
