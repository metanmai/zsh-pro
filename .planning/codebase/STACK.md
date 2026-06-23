# Technology Stack

**Analysis Date:** 2026-06-23

## Languages

**Primary:**
- Go 1.25.0 — entire codebase (`core/`, `go.mod`)

**Secondary:**
- Zsh (shell script) — test fixtures and the embedded `introspectScript` constant (`core/shell/zsh/introspect.go`); not compiled

## Runtime

**Environment:**
- Go toolchain 1.25.0 (as declared in `go.mod`)

**Package Manager:**
- Go modules (`go mod`)
- Lockfile: `go.sum` present and committed

## Frameworks

**Core:**
- Standard library only — no web or application framework; the CLI is hand-rolled in `core/cli/cli.go`

**Testing:**
- Standard `testing` package — all test files use `testing.T`
- `github.com/go-quicktest/qt v1.101.0` — assertion helpers (pulled in transitively by `mvdan.cc/sh`)
- `github.com/google/go-cmp v0.7.0` — deep equality comparisons (transitive dependency)

**Build/Dev:**
- No separate build tool; standard `go build ./...` and `go test ./...`
- Binary targets:
  - `core/cmd/zsh-pro/main.go` → main CLI binary (`zsh-pro`)
  - `core/cmd/zsh-gen/main.go` → test-corpus generator utility (`zsh-gen`)

## Key Dependencies

**Critical:**
- `mvdan.cc/sh/v3 v3.13.1` — the only non-stdlib dependency; provides the zsh-variant parser (`syntax.NewParser` with `syntax.LangZsh`) used in `core/shell/zsh/parse.go`

**Infrastructure:**
- `github.com/go-quicktest/qt v1.101.0` — transitive test dependency (not directly imported in test files discovered)
- `github.com/google/go-cmp v0.7.0` — transitive dependency
- `github.com/kr/pretty v0.3.1` — transitive dependency
- `github.com/rogpeppe/go-internal v1.14.1` — transitive dependency

## Configuration

**Environment:**
- No `.env` file or environment-variable configuration at runtime
- The only runtime environment dependency is a `zsh` binary on `$PATH` for introspection (`core/shell/zsh/introspect.go`); its absence degrades gracefully to static-only mode

**Build:**
- `go.mod` at repo root declares module name `zsh-pro` and Go version
- `go.sum` at repo root pins all dependency hashes
- No `Makefile`, `Taskfile`, or additional build config detected

## Platform Requirements

**Development:**
- Go 1.25+ toolchain
- `zsh` available in `$PATH` for integration/corpus tests (introspection path); static tests run without it

**Production:**
- Self-contained binary; no runtime dependencies beyond `zsh` on `$PATH`
- Designed for macOS/Linux (zsh introspection script uses zsh builtins: `zsh/parameter` module, `emulate -L zsh`)

---

*Stack analysis: 2026-06-23*
