# Phase 3: Git-Backed Store - Pattern Map

**Mapped:** 2026-06-27
**Files analyzed:** 8 new files (core/store/ x5 + tests + core/model/secretref.go + main.go wiring)
**Analogs found:** 7 / 8 (one file — errors.go — has no direct analog; model/exitcode.go is the closest)

---

## Regenerator Seam Resolution (D-03) — Critical Disambiguation

There are TWO regeneration functions in the codebase. The planner must use the right one.

**`core/shell/zsh/regen.go` — `(Provider) Regenerate(e model.Entry) string`** (lines 24-72)
This is the **per-entry** zsh syntax emitter implementing `shell.Regenerator`. It takes one `model.Entry` and returns one line of zsh source. It is shell-specific and lives in `core/shell/zsh`.

**`core/ir/regen.go` — `func Regenerate(p model.Profile, r shell.Regenerator) []byte`** (lines 21-35)
This is the **profile-level** emitter that iterates all entries in source order and delegates per-entry syntax to an injected `shell.Regenerator`. It lives in `core/ir` and is shell-agnostic. This is what the store calls.

**The seam is already defined and named.** `core/shell/provider.go` lines 33-35 declares:
```go
type Regenerator interface {
    Regenerate(e model.Entry) string
}
```
`zsh.Provider{}` implements it. `shell.Regenerator` is the exact type to inject into `core/store`.

**The store should call `ir.Regenerate(profile, regen)` where `regen` is an injected `shell.Regenerator`.** This keeps `core/store` shell-agnostic: it imports `core/ir` (shell-agnostic) and `core/shell` (interface only), never `core/shell/zsh`.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `core/store/store.go` | service (orchestrator) | CRUD | `core/ir/build.go` + `core/cli/cli.go` | role-match: injected-driver orchestrator pattern |
| `core/store/git.go` | service (subprocess driver) | request-response | `core/shell/zsh/introspect.go` | exact: same exec.CommandContext + timeout + LookPath + degrade shape |
| `core/store/secret.go` | service (transform) | transform | `core/analyze/analyzer.go` (secret detection logic) + `core/shell/zsh/classify.go` (secretRe) | role-match: orchestrates reuse of an existing detector |
| `core/store/keychain.go` | service (subprocess driver) | request-response | `core/shell/zsh/introspect.go` | exact: same subprocess + LookPath + degrade shape (different binary) |
| `core/store/errors.go` | utility (typed errors) | — | `core/model/exitcode.go` (typed const pattern) | partial: typed string/int constants; no existing errors.go |
| `core/store/*_test.go` | test | CRUD + request-response | `core/ir/roundtrip_test.go` + `core/shell/zsh/introspect_test.go` | exact: same LookPath skip-guard + TempDir + external-test-package patterns |
| `core/model/secretref.go` | model (value type) | — | `core/model/profile.go` (Entry/ManagedOverride) | exact: same exported-only struct + typed string const pattern |
| `core/cmd/zsh-pro/main.go` (modify) | composition root | — | `core/cmd/zsh-pro/main.go` (current) | self: same single-line wiring pattern |

---

## Pattern Assignments

### `core/store/store.go` (service orchestrator, CRUD)

**Analogs:** `core/ir/build.go` (injected-classifier pattern), `core/cli/cli.go` (injected-provider pattern)

**Imports pattern** — mirror `core/ir/build.go` lines 1-8 and `core/cli/cli.go` lines 1-22:
```go
package store

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "zsh-pro/core/ir"
    "zsh-pro/core/model"
    "zsh-pro/core/shell"
)
// NOTE: must NOT import "zsh-pro/core/shell/zsh" — that import is composition-root only
```

**Constructor pattern** — copy from `core/cli/cli.go` lines 24-28 and `core/analyze/analyzer.go` lines 14-21:
```go
// CLI struct shape (cli.go:24-28):
type CLI struct{ provider shell.Provider }
func New(p shell.Provider) *CLI { return &CLI{provider: p} }

// Analyzer struct shape (analyzer.go:14-21):
type Analyzer struct {
    provider shell.Provider
    rec      reconciler
}
func New(p shell.Provider) *Analyzer { return &Analyzer{provider: p} }
```
Store should follow this pattern:
```go
// Store holds injected drivers; zero external dependencies.
type Store struct {
    dir      string          // path to the bare git repo
    git      gitRunner       // subprocess driver
    keychain KeychainDriver  // secret backend
    regen    shell.Regenerator
}
func New(dir string, regen shell.Regenerator, kc KeychainDriver) *Store { ... }
```

**Graceful-degrade pattern** — copy from `core/analyze/analyzer.go` lines 83-89:
```go
// analyzer.go:83-89 — degrade to static-only when introspection fails:
ids, err := az.provider.Introspect(path)
if err != nil || !ids.Available {
    a.Introspected = false
    a.Notes = append(a.Notes, "introspection unavailable — showing static analysis only")
} else {
    a.Introspected = true
}
```
Store's `Init` should mirror: on `exec.LookPath("git")` failure, return `ErrGitAbsent` rather than nil; never crash.

**JSON round-trip for Read/Commit** — use `encoding/json` with `MarshalIndent` + trailing newline (established by RESEARCH.md; no existing project analog for this specific operation, but the dto package shows json tag conventions):
```go
// dto/analysis.go:7-16 — JSON tag conventions:
type Analysis struct {
    Path         string            `json:"path"`
    Lines        int               `json:"lines"`
    // ... all exported fields, omitempty only on optional slices
    Notes        []string          `json:"notes,omitempty"`
}
```
Profile serialization must use the same `json:"fieldname"` tag style; `omitempty` only on the new `Secret *SecretRef` field.

**ir.Regenerate call for profile.zsh** — copy from `core/ir/regen.go` lines 21-35:
```go
// ir/regen.go:21-35 — the profile-level emitter to call:
func Regenerate(p model.Profile, r shell.Regenerator) []byte {
    var out []byte
    for i := range p.Entries {
        e := p.Entries[i]
        var line string
        if e.EffectiveManaged() {
            line = r.Regenerate(e)
        } else {
            line = e.Text
        }
        out = append(out, line...)
        out = append(out, '\n')
    }
    return out
}
```
Call as: `zshBytes := ir.Regenerate(profile, s.regen)`

**What to replicate:** constructor injection (no global state), the `New(injected...) *Store` signature, `encoding/json.MarshalIndent` + `\n` suffix for `profile.json`, calling `ir.Regenerate(profile, s.regen)` for `profile.zsh`.
**What differs:** Store has multiple injected drivers (git, keychain, regen) vs CLI's single provider; Store returns structured errors instead of exit codes.

---

### `core/store/git.go` (subprocess driver, request-response)

**Analog:** `core/shell/zsh/introspect.go` — EXACT match on subprocess + timeout + LookPath + degrade.

**Full subprocess pattern** — copy from `core/shell/zsh/introspect.go` lines 1-53:

**Imports** (introspect.go lines 1-13):
```go
import (
    "bytes"
    "context"
    "os/exec"
    "strings"
    "time"

    "zsh-pro/core/buildinfo"
    "zsh-pro/core/model"
    "zsh-pro/core/shell"
)
```
Store's git.go imports: `bytes`, `context`, `os`, `os/exec`, `time` — no internal imports beyond `core/store` (errors).

**Subprocess + timeout core pattern** (introspect.go lines 42-53):
```go
func (p Provider) Introspect(path string) (model.IdentitySet, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, buildinfo.Name, path)
    var out bytes.Buffer
    cmd.Stdout = &out
    if err := cmd.Run(); err != nil {
        return model.IdentitySet{Available: false}, err
    }
    return p.parseIntrospect(out.String()), nil
}
```
Git driver mirrors this shape:
```go
type gitRunner struct{ repoDir string }

func (g gitRunner) run(ctx context.Context, args ...string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.repoDir}, args...)...)
    var out, errb bytes.Buffer
    cmd.Stdout, cmd.Stderr = &out, &errb
    if err := cmd.Run(); err != nil {
        return nil, mapGitError(err, errb.String()) // -> zsh-pro-phrased error, never raw
    }
    return out.Bytes(), nil
}
```
`mapGitError` translates raw git exit/stderr to typed `ErrGitAbsent` / `ErrNotInitialized` / `ErrProfileNotFound` etc. (D-11 — never surface raw git error).

**LookPath guard pattern** — from `core/ir/roundtrip_test.go` line 29 and `core/shell/zsh/introspect_test.go` line 11 (see test section below). In production code, check at `New` time:
```go
// Check at store construction (not per-call) — mirror the pattern:
if _, err := exec.LookPath("git"); err != nil {
    return nil, ErrGitAbsent
}
```

**Env vars for deterministic commits** — set in `cmd.Env` for `commit-tree` calls (no analog in current codebase; this is net-new but follows the stdlib `os.Environ()` pattern):
```go
cmd.Env = append(os.Environ(),
    "GIT_DIR="+g.repoDir,
    "GIT_INDEX_FILE="+tmpIndexPath,
    "GIT_AUTHOR_NAME=zsh-pro", "GIT_AUTHOR_EMAIL=zsh-pro@local",
    "GIT_AUTHOR_DATE="+ts,
    "GIT_COMMITTER_NAME=zsh-pro", "GIT_COMMITTER_EMAIL=zsh-pro@local",
    "GIT_COMMITTER_DATE="+ts,
)
```

**What to replicate:** `context.WithTimeout(ctx, 5*time.Second)` + `defer cancel()`, `exec.CommandContext`, separate `out` and `errb` buffers, `mapGitError` (never return raw stderr), `LookPath` guard at construction.
**What differs:** git uses `cmd.Env` injection for `GIT_DIR` and `GIT_INDEX_FILE` (not needed for zsh); git needs a `repoDir` field; git commits need env vars for determinism. The `cmd.Stdin` pipe is also needed for `hash-object -w --stdin` (writing blob content).

---

### `core/store/secret.go` (transform, literal-vs-dynamic classification + capture)

**Analogs:** `core/shell/zsh/classify.go` (secretRe, CatSecrets), `core/model/profile.go` (Entry.Dynamic, Entry.Category).

**Secret detection signal** — from `core/shell/zsh/classify.go` lines 10 and 32-35:
```go
// classify.go:10 — the shipped regex, reuse do not copy:
var secretRe = regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWD|PASSWORD|API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|CLIENT[_-]?SECRET|AUTH[_-]?TOKEN|APIKEY)`)

// classify.go:32-35 — how CatSecrets is set:
case model.KindAssignment:
    for _, n := range b.Names {
        if secretRe.MatchString(n) {
            return model.CatSecrets, model.ConfHigh
        }
    }
```
The store does NOT re-run this regex. It reads the already-classified `Entry.Category` from the Profile (set at parse/build time by the classifier). No import of `core/shell/zsh` needed.

**Literal vs dynamic flag** — from `core/model/profile.go` lines 23-35:
```go
// profile.go:23-35 — exact fields available on Entry:
type Entry struct {
    Text      string          // verbatim source text (D-01)
    StartLine int             // 1-based line in source
    Category  Category        // classifier verdict — CatSecrets = this is a secret-named var
    Kind      BlockKind       // structural shape
    CmdName   string
    Names     []string        // var name(s) — use Names[0] as the SecretRef key
    Value     string          // verbatim assignment value — non-empty iff literal or static
    Exported  bool
    Managed   bool
    Override  ManagedOverride
    Dynamic   bool            // true when value contains $(...)/$/etc. — set at parse time
}
```
**D-08 predicate** — the store's literal-secret test is:
```go
e.Category == model.CatSecrets && !e.Dynamic && e.Value != ""
// → literal: convert to SecretRef, capture Value to keychain
// e.Category == model.CatSecrets && e.Dynamic
// → already a pointer: commit verbatim (Value is "$(...)" or similar)
```
No new AST walk; both signals come from the already-populated Entry fields.

**WithheldReport shape** — no existing analog; define as a new type in secret.go:
```go
// Follows the model package's value-type convention (profile.go style):
type WithheldSecret struct {
    Name      string // e.g. "API_KEY" — the var name (Entry.Names[0])
    StartLine int    // 1-based source line (Entry.StartLine)
}
type WithheldReport []WithheldSecret
```

**What to replicate:** read `Entry.Category`, `Entry.Dynamic`, `Entry.Value`, `Entry.Names[0]`, `Entry.StartLine` from the Profile (zero new inspection logic). Build `WithheldReport` as a value-type slice. Mutate the entry in the committed profile copy: set `Entry.Secret = &model.SecretRef{Kind: "keychain", Key: name}` and clear `Entry.Value = ""` before marshaling.
**What differs:** secret.go orchestrates (classify → capture → mutate copy → report), while classify.go only classifies. The mutation must operate on a defensive copy of the Profile so the caller's original is not modified.

---

### `core/store/keychain.go` (subprocess driver, request-response)

**Analog:** `core/shell/zsh/introspect.go` — EXACT subprocess + LookPath + degrade shape.

**Interface pattern** — follows `core/shell/provider.go` lines 9-43 (ISP: narrow interface):
```go
// provider.go:9-43 — ISP pattern to replicate:
type Parser interface {
    Parse(src []byte) ([]model.Block, error)
}
type Regenerator interface {
    Regenerate(e model.Entry) string
}
// Each concern is its own small interface.
```
Keychain interface follows the same ISP approach:
```go
// KeychainDriver is injected at the composition root (mirrors shell.Provider).
type KeychainDriver interface {
    Store(key, value string) error
    Retrieve(key string) (string, error)
    Delete(key string) error
}
```

**LookPath + degrade pattern** (introspect_test.go line 11; production use mirrors this):
```go
// introspect_test.go:11 — the skip guard (test version):
if _, err := exec.LookPath("zsh"); err != nil {
    t.Skip("zsh not installed; skipping dynamic introspection test")
}
// Production code (gitRunner construction) returns typed error:
if _, err := exec.LookPath("security"); err != nil {
    // fall back to vault-file driver
}
```

**Subprocess shape** (introspect.go lines 43-53) — macOS keychain impl mirrors exactly:
```go
// macOS keychain Store — mirrors introspect.go subprocess shape:
func (m macOSKeychain) Store(key, value string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    // value via doubled stdin (never on argv — Pitfall 3):
    payload := value + "\n" + value + "\n"
    cmd := exec.CommandContext(ctx, "security", "add-generic-password",
        "-a", key, "-s", "zsh-pro:"+key, "-U", "-w")
    cmd.Stdin = strings.NewReader(payload)
    var errb bytes.Buffer
    cmd.Stderr = &errb
    if err := cmd.Run(); err != nil {
        return mapKeychainError(err, errb.String())
    }
    return nil
}
```

**What to replicate:** `context.WithTimeout(ctx, 5*time.Second)` + `defer cancel()`, `exec.CommandContext`, separate stderr buffer, typed-error mapping (never raw stderr), `LookPath` probe to select backend. Stdin pipe for value (never argv).
**What differs:** keychain uses `cmd.Stdin` for the secret value (never argv, for security). Three impls (macOS/Linux/vault-file) all implement `KeychainDriver` — same interface shape, different binary. Vault-file impl uses `os.WriteFile` with `0o600` perms.

---

### `core/store/errors.go` (typed errors, utility)

**Analog:** `core/model/exitcode.go` — closest pattern for typed domain constants. No existing `errors.go` in the codebase.

**Typed-constant pattern** (exitcode.go lines 1-12):
```go
// exitcode.go:1-12:
package model

type ExitCode int

const (
    ExitClean      ExitCode = 0
    ExitRuntimeErr ExitCode = 1
    ExitUsageErr   ExitCode = 2
    ExitActionable ExitCode = 3
)
```

**Typed-error pattern** — the project uses typed string errors in test code (referenced in CLAUDE.md: `type errTest string`). For production errors, the established pattern is returning plain `error` with context strings (cli.go lines 67-68):
```go
// cli.go:67-68 — error with context string (current production pattern):
return c.fail(stdout, stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
```
For `errors.go`, use sentinel typed errors (the zsh-pro-phrased kind D-11 requires). Follow the typed-string-error pattern referenced in CLAUDE.md conventions:
```go
package store

// errStore is the base type for all zsh-pro-phrased store errors (D-11).
type errStore string

func (e errStore) Error() string { return string(e) }

const (
    ErrGitAbsent            errStore = "zsh-pro: git is not installed; profile storage requires git"
    ErrNotInitialized       errStore = "zsh-pro: profile store not initialized; run 'zsh-pro init'"
    ErrProfileNotFound      errStore = "zsh-pro: profile not found"
    ErrProfileExists        errStore = "zsh-pro: profile already exists"
    ErrSecretBackendUnavailable errStore = "zsh-pro: no secret backend available; falling back to vault file"
)
```

**What to replicate:** typed base type (`type errStore string`), `Error() string` method, `const (...)` block with `Err*` prefix constants containing zsh-pro-phrased messages (D-11 — never raw git/keychain output).
**What differs:** errors here are string-typed sentinels, not int-typed like ExitCode. The `Err*` naming prefix matches project conventions (Cat*, Kind*, Exit*, etc.).

---

### `core/store/*_test.go` (test, CRUD + subprocess)

**Analogs:** `core/ir/roundtrip_test.go` (external test package + skip guard + TempDir), `core/shell/zsh/introspect_test.go` (LookPath skip guard).

**Skip-when-absent guard** — EXACT pattern from `core/ir/roundtrip_test.go` line 29 and `core/shell/zsh/introspect_test.go` line 11:
```go
// roundtrip_test.go:29 — zsh skip guard (exact):
if _, err := exec.LookPath("zsh"); err != nil {
    t.Skip("zsh not installed; skipping round-trip oracle")
}

// introspect_test.go:11 — same pattern for zsh:
if _, err := exec.LookPath("zsh"); err != nil {
    t.Skip("zsh not installed; skipping dynamic introspection test")
}
```
Store tests use the same pattern for git:
```go
if _, err := exec.LookPath("git"); err != nil {
    t.Skip("git not installed; skipping store tests")
}
```

**External test package + TempDir pattern** (roundtrip_test.go lines 1-22):
```go
// roundtrip_test.go:1 — external test package:
package ir_test

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "reflect"
    "testing"

    "zsh-pro/core/ir"
    "zsh-pro/core/shell/zsh"
)

// roundtrip_test.go:75-83 — TempDir + WriteFile pattern:
dir := t.TempDir()
origPath := filepath.Join(dir, "orig.zsh")
regenPath := filepath.Join(dir, "regen.zsh")
if err := os.WriteFile(origPath, src, 0o644); err != nil {
    t.Fatal(err)
}
```
Store tests: `package store_test` (external), `t.TempDir()` for the bare repo, `exec.LookPath` guard.

**Stub injected driver pattern** (ir/regen_test.go lines 12-17):
```go
// regen_test.go:12-17 — stub keeps ir free of zsh dependency:
type stubRegenerator struct{}

func (stubRegenerator) Regenerate(e model.Entry) string {
    return "TEMPLATED<" + strings.Join(e.Names, ",") + ">"
}
```
Store tests: a `stubKeychain` implementing `KeychainDriver` (records Store/Retrieve calls), and the real `zsh.Provider{}` injected as `Regenerator` only in the external round-trip test (mirrors how roundtrip_test.go injects `zsh.Provider{}`).

**Separate error variables pattern** (roundtrip_test.go lines 87-94):
```go
// roundtrip_test.go:87-94 — separate err vars to avoid ambiguity:
origIDS, origErr := p.Introspect(origPath)
regenIDS, regenErr := p.Introspect(regenPath)

if origErr != nil || !origIDS.Available {
    t.Fatalf("introspect(original) failed: err=%v available=%v", origErr, origIDS.Available)
}
```

**What to replicate:** `package store_test` (external), `exec.LookPath("git") + t.Skip`, `t.TempDir()` for the bare git repo, stub impls for interfaces (`stubKeychain`, `stubRegenerator`), assert on content not on SHAs (`git show <branch>:profile.json` output → unmarshal → compare), `t.Cleanup` for keychain entries.
**What differs:** store tests use a bare git repo initialized by `Init()` rather than a pre-written fixture file; keychain tests require `LookPath("security")` guard additionally; commit env vars (`GIT_AUTHOR_*`) set for determinism in tests.

---

### `core/model/secretref.go` (model value type, additive)

**Analog:** `core/model/profile.go` — EXACT match on exported-only struct + typed-string const + no internal imports.

**Package-level pattern** (profile.go lines 1-18):
```go
// profile.go:1 — package declaration:
package model

// profile.go:6-17 — typed string const (ManagedOverride):
type ManagedOverride string

const (
    OverrideAuto      ManagedOverride = "auto"
    OverrideManaged   ManagedOverride = "forced-managed"
    OverrideUnmanaged ManagedOverride = "forced-unmanaged"
)
```

**Struct pattern** (profile.go lines 19-35):
```go
// profile.go:19-35 — all-exported fields, no unexported state, no methods on struct:
type Entry struct {
    Text      string          // verbatim source text of the statement (D-01)
    StartLine int             // 1-based line in source
    Category  Category        // classifier verdict
    Kind      BlockKind       // structural shape (from the parser)
    // ... all exported, documented with inline comments
    Dynamic   bool            // value contains a non-literal AST part
}
```

**SecretRef** follows the same pattern exactly:
```go
// core/model/secretref.go — additive, dependency-free:
package model

// SecretRefKind identifies the resolver backend for a SecretRef (D-09).
type SecretRefKind string

const (
    SecretRefKeychain SecretRefKind = "keychain" // OS keychain via subprocess
    SecretRefFile     SecretRefKind = "file"     // git-ignored vault file fallback
    SecretRefCmd      SecretRefKind = "cmd"      // arbitrary retrieval command
)

// SecretRef is a resolver-agnostic pointer to a captured secret value (D-07/D-09).
// It serializes as {"kind":"keychain","key":"API_KEY"} in profile.json.
// The literal value never enters the git tree; only the reference does.
type SecretRef struct {
    Kind SecretRefKind `json:"kind"` // resolver backend
    Key  string        `json:"key"`  // name-scoped lookup key (e.g. "API_KEY")
}
```

**Additive Entry change** — add ONE new field to `core/model/profile.go` Entry:
```go
// Add after the Dynamic field (current last field, profile.go:34):
Secret *SecretRef `json:"secret,omitempty"` // non-nil iff this entry is a secret-replaced literal (D-07)
```
`omitempty` ensures Phase 2 test fixtures (which have no `Secret`) round-trip without change. All existing fields stay in declaration order (JSON determinism is declaration-order for structs).

**What to replicate:** `package model`, no imports (dependency-free), exported-only fields, json tags with `omitempty` on the pointer field, typed-string constants with `SecretRef*` prefix (not `Kind*` — that prefix belongs to BlockKind), doc comments on every exported symbol.
**What differs:** SecretRef is the first model type with a JSON pointer field (`*SecretRef`); the `omitempty` is critical for additive compatibility with Phase 2.

---

### `core/cmd/zsh-pro/main.go` (composition root, wiring — modify)

**Analog:** self — current `core/cmd/zsh-pro/main.go` (the file to modify).

**Current wiring** (main.go lines 1-12):
```go
package main

import (
    "os"

    "zsh-pro/core/cli"
    "zsh-pro/core/shell/zsh"
)

func main() {
    os.Exit(cli.New(zsh.Provider{}).Run(os.Args[1:], os.Stdout, os.Stderr))
}
```

**Post-phase-3 wiring** — add store construction and inject the concrete drivers. Mirrors the established pattern: `zsh.Provider{}` is already the sole concrete type imported here; add `store.New(...)` alongside it:
```go
package main

import (
    "fmt"
    "os"

    "zsh-pro/core/cli"
    "zsh-pro/core/store"
    "zsh-pro/core/shell/zsh"
)

func main() {
    provider := zsh.Provider{}
    kc := store.NewOSKeychainDriver() // concrete keychain selected at composition root
    s, err := store.New(storeDir(), provider, kc)
    if err != nil {
        fmt.Fprintf(os.Stderr, "zsh-pro: %v\n", err)
        os.Exit(1)
    }
    _ = s // CLI not yet wired in this phase; store construction verified
    os.Exit(cli.New(provider).Run(os.Args[1:], os.Stdout, os.Stderr))
}
```
`storeDir()` reads `$ZSHPRO_HOME` / XDG default (a private helper, keeps `main` minimal).

**What to replicate:** keep `main()` minimal (single responsibility: construct + wire + run), single `os.Exit`, only this file imports `core/shell/zsh` and now also `core/store`, every other package depends on interfaces not concretes.
**What differs:** store adds a second injected dependency (keychain driver) and a potential initialization error — the first `main.go` error path this project has (prior: `cli.New` cannot fail).

---

## Shared Patterns

### Subprocess + Timeout (apply to: `core/store/git.go`, `core/store/keychain.go`)
**Source:** `core/shell/zsh/introspect.go` lines 42-53
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "<binary>", args...)
var out bytes.Buffer
cmd.Stdout = &out
if err := cmd.Run(); err != nil {
    return <zero-value>, err
}
```
- 5-second timeout is the established project constant
- Separate `out` and `errb` buffers (never `cmd.CombinedOutput`)
- Error returned immediately, never panicked

### LookPath Guard (apply to: `core/store/git.go`, `core/store/keychain.go`, `core/store/*_test.go`)
**Source production:** `core/shell/zsh/introspect.go` pattern; test usage from `core/ir/roundtrip_test.go` line 29
```go
// In tests:
if _, err := exec.LookPath("git"); err != nil {
    t.Skip("git not installed; skipping store tests")
}
// In production (store.New):
if _, err := exec.LookPath("git"); err != nil {
    return nil, ErrGitAbsent
}
```

### Injected-Driver Constructor (apply to: `core/store/store.go`)
**Source:** `core/cli/cli.go` lines 24-28, `core/analyze/analyzer.go` lines 14-21, `core/ir/build.go` line 21
```go
// All major types follow New(injected) *T:
func New(p shell.Provider) *CLI { return &CLI{provider: p} }
func New(p shell.Provider) *Analyzer { return &Analyzer{provider: p} }
func Build(blocks []model.Block, c shell.Classifier) model.Profile { ... }
func Regenerate(p model.Profile, r shell.Regenerator) []byte { ... }
```
Store: `func New(dir string, regen shell.Regenerator, kc KeychainDriver) (*Store, error)`

### Interface Segregation Seam (apply to: `core/store/keychain.go` interface definition)
**Source:** `core/shell/provider.go` lines 9-43 — narrow per-concern interfaces composed at the root
```go
type Parser interface { Parse(src []byte) ([]model.Block, error) }
type Regenerator interface { Regenerate(e model.Entry) string }
type Introspector interface { Introspect(path string) (model.IdentitySet, error) }
type Provider interface { Parser; Classifier; Introspector; Regenerator }
```
`KeychainDriver` follows the same ISP pattern: one interface, narrowly scoped to the secret-backend concern.

### All-Exported Model Types (apply to: `core/model/secretref.go`)
**Source:** `core/model/profile.go` lines 19-59 — no unexported fields, no internal imports, json tags on all fields, `omitempty` only on optional/pointer fields
```go
type Profile struct {
    Entries []Entry
}
// Note: Profile.Entries has no json tag — the store adds json tags to Entry fields
// as needed. SecretRef must add json tags for serialization.
```

### External Test Package Isolation (apply to: `core/store/*_test.go`)
**Source:** `core/ir/roundtrip_test.go` line 1 (`package ir_test`) — keeps the production package free of shell/concrete dependencies; wires concrete providers only in tests
```go
package ir_test
// imports zsh.Provider{} here — the only place in the ir test suite
```
Store tests: `package store_test`; imports `zsh.Provider{}` as `shell.Regenerator` in the round-trip test only.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `core/store/errors.go` | utility (typed errors) | — | No existing `errors.go` in the codebase; `core/model/exitcode.go` is the closest typed-constant pattern but uses `int` not `string`. The `type errTest string` pattern referenced in CLAUDE.md conventions is the right shape for production sentinel errors. |

---

## Metadata

**Analog search scope:** `core/shell/zsh/`, `core/ir/`, `core/model/`, `core/analyze/`, `core/cli/`, `core/cmd/zsh-pro/`, `core/util/`, `core/dto/`
**Files scanned:** 26 source files read directly
**Pattern extraction date:** 2026-06-27

**Key facts confirmed by reading real code:**
1. `shell.Regenerator` interface already exists at `core/shell/provider.go:33-35` — no new interface needed in `core/shell/`
2. `ir.Regenerate(p model.Profile, r shell.Regenerator) []byte` at `core/ir/regen.go:21` — this is what the store calls for `profile.zsh`, not `zsh.Provider{}.Regenerate(e)` directly
3. `Entry.Dynamic` is set by the parser at `core/shell/zsh/parse.go:82-83` for assignment values — the store's `!Dynamic && Value != ""` predicate is safe
4. `Entry.Category == model.CatSecrets` is set by the classifier at `core/shell/zsh/classify.go:32-35` using `secretRe` — no new regex needed in the store
5. `model.Profile` and `model.Entry` have NO json tags currently — the store must add `json:"..."` tags to `Entry` fields when serializing, OR marshal via a dedicated DTO struct in `core/store`. The latter is safer (avoids touching the Phase 2 `Entry` struct for serialization concerns)
6. The skip-when-absent guard is `exec.LookPath + t.Skip` at `core/ir/roundtrip_test.go:29` and `core/shell/zsh/introspect_test.go:11` — exact string is `t.Skip("<binary> not installed; skipping ...")`
7. `main.go` is currently 12 lines; the composition root must stay minimal after adding store wiring
