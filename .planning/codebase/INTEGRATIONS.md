# External Integrations

**Analysis Date:** 2026-06-23

## APIs & External Services

None. `zsh-pro` is a fully offline, read-only CLI tool. It makes no outbound HTTP calls and has no dependency on any web API or cloud service.

## Data Storage

**Databases:**
- None. The tool is stateless between invocations; all data flows from the input `.zsh` file to stdout.

**File Storage:**
- Input: reads one zsh config file from disk via `os.ReadFile` (`core/cli/cli.go:65`)
- Output: writes to the caller-supplied `stdout` / `stderr` writers; never writes files during normal operation
- `zsh-gen` utility writes generated `.zsh` fixtures and `manifests.json` to a caller-specified directory (`core/cmd/zsh-gen/main.go:37–74`); used only during test-corpus generation, not at runtime

**Caching:**
- None.

## Authentication & Identity

**Auth Provider:**
- Not applicable. There is no user authentication surface.

## Monitoring & Observability

**Error Tracking:**
- None. Errors are written to stderr (human mode) or encoded as a structured JSON envelope on stdout (JSON mode) — see `core/cli/cli.go:88–99`.

**Logs:**
- No structured logging framework. Diagnostic text goes directly to `stderr` via `fmt.Fprintf`.

## CI/CD & Deployment

**Hosting:**
- Not applicable (local CLI binary, no server).

**CI Pipeline:**
- None detected (no `.github/`, `.circleci/`, `Jenkinsfile`, or similar in the repo).

## Shell Introspection Subprocess

This is the primary "external" integration: the tool forks a sandboxed `zsh` process to resolve the runtime identity of a config file.

**Binary:**
- `zsh` — must be present on `$PATH` at the time `Introspect` is called
- Invoked as: `zsh -f -c <introspectScript> zsh-pro <config-path>` (`core/shell/zsh/introspect.go:46`)
- The `-f` flag disables rc files, providing a clean sandbox

**Contract:**
- Timeout: 5 seconds (`core/shell/zsh/introspect.go:43`)
- On timeout, missing binary, or non-zero exit: returns `IdentitySet{Available: false}` and the analyzer degrades to static-only mode with a human-readable note — no crash
- The script (`introspectScript` constant, `core/shell/zsh/introspect.go:23–38`) uses:
  - `emulate -L zsh` — strict zsh emulation
  - `zmodload zsh/parameter` — loads the `$aliases`, `$functions`, `$parameters`, `$path`, `$options` associative arrays
  - `source "$1"` — loads the target config file with all output suppressed

**Sections emitted by the introspect script:**
| Section marker | Content captured |
|----------------|-----------------|
| `##ALIASES##`  | alias names defined after sourcing |
| `##FUNCTIONS##`| function names defined after sourcing |
| `##ENV##`      | exported parameter names |
| `##PATH##`     | `$path` entries |
| `##OPTIONS##`  | options set to `on` |
| `##END##`      | sentinel |

**Current usage scope (v1):**
- Only `ids.Available` (boolean) is consumed by `core/analyze/analyzer.go`; the resolved tables (aliases, functions, env, path, options) are captured but intentionally not yet wired into issue detection (tracked as a follow-up per code comments in `core/shell/zsh/introspect.go:86–90`)

## Parser Integration (mvdan.cc/sh)

The sole third-party library dependency provides the zsh AST parser.

**Package:** `mvdan.cc/sh/v3 v3.13.1`
**Import path:** `mvdan.cc/sh/v3/syntax`
**Used in:** `core/shell/zsh/parse.go`

**Usage pattern:**
```go
parser := syntax.NewParser(
    syntax.Variant(syntax.LangZsh),
    syntax.KeepComments(true),
)
file, err := parser.Parse(bytes.NewReader(src), "")
```

**Failure handling:** A parse error does not propagate; the entire source is returned as a single `Opaque: true` block, preserving analysis continuity (`core/shell/zsh/parse.go:22–26`).

## Webhooks & Callbacks

**Incoming:** None.
**Outgoing:** None.

## Environment Configuration

**Required at runtime:**
- `zsh` binary on `$PATH` (for introspection; optional — absence degrades gracefully)
- No env vars are read by the application itself

**No secret configuration:**
- No API keys, tokens, or credentials are required or used

---

*Integration audit: 2026-06-23*
