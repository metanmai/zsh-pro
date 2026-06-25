# Architecture Research

**Domain:** Branchable, git-versioned shell-environment manager (zsh) — integrating a new environment-manager surface onto an existing read-only analyze engine
**Researched:** 2026-06-25
**Confidence:** HIGH (existing engine read directly from source; runtime mechanics verified against shadowenv/direnv/chezmoi source and the zsh manual)

> Scope note: This is **integration research for milestone v2.0**, not greenfield architecture. The job is to design how the new environment-manager components (IR, git store, activation manifest, sourced runtime, partial evaluation) compose with the existing single-pass, dependency-injected engine **without bypassing the `Provider` seam**, and to recommend a build order with a clear spike-first risk. The generic "scaling to millions of users" sections of the template do not apply to a single-user CLI and are reframed accordingly.

---

## TL;DR for the Roadmap

- **Six new components, four touched, nothing rewritten.** The activation runtime and git storage are *new*; the parser/classifier are *reused as-is* via the existing `Provider` seam; `model.Analysis` is *left alone* (it stays the reporting view) while a new richer IR is added beside it.
- **The IR is additive, not a replacement.** Today's `model.Analysis` is a lossy *report* (category → `[]string` of names). The new `model.Profile` is a near-lossless *store* (per-entry verbatim text + structured fields + a static/dynamic portability tag). They coexist; `analyze` keeps emitting `Analysis`.
- **The activation manifest is a solved problem — copy shadowenv's `undo::Data`.** Record env vars as `{name, original, applied, no_clobber}` scalars and PATH-like vars as `{name, additions, deletions}` list-deltas; carry the whole undo blob per-terminal inside one env var. Deactivate = reverse the recorded deltas, *guarded by* "only restore if the current value still equals what we applied."
- **Shell out to `git` and `zsh`, do not add a library.** The hard constraint is "no new dependencies beyond `mvdan.cc/sh/v3`." A `git`-backed store via the `git` binary mirrors the *existing* `zsh -f` subprocess pattern exactly. Adding `go-git` would violate the constraint and the established composition-root discipline.
- **SPIKE the zero-residue switch loop FIRST**, with a hand-written manifest and a hard-coded two-profile fixture — before building the IR, the git store, or the CLI. The frontier risk (`activate → switch → reactivate` leaving zero residue in a *live* terminal) is the one thing that can invalidate the whole product, and it is independent of all the Go plumbing.

---

## Standard Architecture

This is a brownfield integration. The existing engine is a clean, single-pass, dependency-injected analyzer. The environment-manager surface bolts onto it at three seams — **the parser output** (reused for the IR front-end), **a new IR domain type** (beside `model.Analysis`), and **new subprocess providers** (`git` store + an extended `zsh -f` introspect, both siblings of the existing `Introspector`).

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│  SHELL-INTEGRATION RUNTIME  (sourced; lives in the user's live terminal)   │
│  ┌────────────────────┐         ┌──────────────────────────────────────┐  │
│  │ thin ~/.zshrc       │ source  │ loader (zsh fn library, generated)   │  │
│  │  • imperative        │────────▶│  • checkout/activate/deactivate fns  │  │
│  │    "master block"    │         │  • holds per-terminal active-state   │  │
│  │  • bootstraps loader │         │    in ONE env var (the undo blob)    │  │
│  └────────────────────┘         └───────────────┬──────────────────────┘  │
│   the loader runs the binary and `eval`s/sources the emitted shell code     │
└──────────────────────────────────────────────────┼─────────────────────────┘
                                                    │ exec + capture stdout
┌───────────────────────────────────────────────────▼─────────────────────────┐
│  zsh-pro BINARY  (Go; the composition root wires concrete providers)          │
│  ┌─────────────┐   ┌───────────────────────────────────────────────────────┐ │
│  │  core/cli    │──▶│  NEW: env-manager command group                       │ │
│  │ (flags, I/O, │   │   ingest / checkout / activate / deactivate / list /   │ │
│  │  exit codes) │   │   hook                                                 │ │
│  └─────────────┘   └───────┬──────────────────┬───────────────┬───────────┘ │
│            ┌────────────────▼──────┐ ┌─────────▼────────┐ ┌────▼──────────┐  │
│            │ NEW: core/profile      │ │ NEW: core/store  │ │ NEW: core/    │  │
│            │  • build IR from blocks│ │  (git-as-db, via │ │ activate      │  │
│            │  • partial-eval pass   │ │  `git` binary)   │ │ • Manifest    │  │
│            │  • regenerate .zsh     │ │  • branch=profile│ │ • diff/plan   │  │
│            └───────┬────────────────┘ └──────────────────┘ │ • un-apply    │  │
│                    │ uses (interface)                        └───────────────┘  │
│            ┌────────▼─────────────────────────────────────────────────────┐  │
│            │  core/shell  (interface seam — UNCHANGED contract)            │  │
│            │   Parser │ Classifier │ Introspector │ NEW: Activator?(opt)   │  │
│            └────────┬─────────────────────────────────────────────────────┘  │
│            ┌────────▼─────────────────────────────────────────────────────┐  │
│            │  core/shell/zsh  (concrete; composition-root-only import)     │  │
│            │   Parse │ Classify │ Introspect │ NEW: Emit/Apply shell code  │  │
│            └────────┬─────────────────────────────────────────────────────┘  │
│            ┌────────▼─────────────┐  ┌──────────────────────────────────┐    │
│            │ core/model (domain)  │  │ core/dto (wire)  ── unchanged    │    │
│            │  Block, Analysis,    │  │  + NEW manifest/profile DTOs     │    │
│            │  + NEW Profile/Entry │  └──────────────────────────────────┘    │
│            │  + NEW Manifest      │                                           │
│            └──────────────────────┘                                           │
└───────────────────────────────────────────────────────────────────────────┘
                    │ subprocess (mirrors existing `zsh -f` introspect)
        ┌───────────▼───────────┐   ┌─────────────────────────────┐
        │  git  binary (store)   │   │  zsh -f  binary (resolve/    │
        │  branches = profiles   │   │  introspect end-state)      │
        └────────────────────────┘   └─────────────────────────────┘

Dependency direction (strictly inward, UNCHANGED discipline):
  cmd → cli → {profile, store, activate} → shell(interface) → model/dto
  (core/shell/zsh imported only at the composition root, as today)
```

### Component Responsibilities

| Component | Responsibility | New / Modified | Typical Implementation |
|-----------|----------------|----------------|------------------------|
| `core/profile` (**NEW**) | Build the regenerable IR from parsed `Block`s; run the partial-evaluation pass; regenerate `.zsh` per category | NEW package | Pure Go; depends on `core/model` + `Parser`/`Classifier` interfaces (+ `mvdan.cc/sh` AST); no shell exec |
| `core/store` (**NEW**) | git-as-database: init repo, list branches (profiles), `checkout`, commit categorized files, read a profile's tree | NEW package | Shells out to the `git` binary via `os/exec` — mirrors the existing `zsh -f` subprocess pattern |
| `core/activate` (**NEW**) | Build a `Manifest` from a target profile; diff against the active manifest; produce a deactivate-then-activate plan | NEW package | Pure Go orchestration over `model.Manifest`; emits an apply-plan, never shell text |
| `core/model` IR types (**NEW**) | `Profile`, `Entry`, `Manifest` (+ `Scalar`/`ListDelta`/`NameSet`) — the lossless store + the reversible undo record | MODIFIED (additive) | New value types beside `Block`/`Analysis`; `Analysis` untouched |
| `core/shell/zsh` emit/apply (**MODIFIED**) | Render a `Manifest` into zsh apply/deactivate **shell code** (the only place that knows `unalias`/`unset -f`/`unsetopt` syntax); extend introspect to resolve bodies | MODIFIED | New methods on `zsh.Provider{}`; reuses `zmodload zsh/parameter` |
| `core/cli` command group (**MODIFIED**) | New subcommands: `ingest`, `checkout`, `activate`, `deactivate`, `list`, `hook` | MODIFIED | New `case`s in `CLI.Run`; same exit-code + JSON-envelope contract |
| shell loader (**NEW**) | Sourced zsh function library; bootstraps from thin `.zshrc`; holds per-terminal active state; calls the binary and `eval`s its output | NEW (embedded `.zsh`) | Embedded string constant (like the existing `introspectScript`) emitted by `zsh-pro hook` |
| `core/shell.Activator` (**OPTIONAL NEW interface**) | ISP interface for "render manifest → shell code," if you want the seam symmetric | OPTIONAL | Only add if a second shell is ever planned; v2.0 is zsh-only, so this is a judgment call — see note in (d) |

---

## Recommended Project Structure

```
core/
├── cmd/zsh-pro/main.go      # composition root — wires zsh.Provider + git store (MODIFIED: ~1-3 lines)
├── cli/cli.go               # MODIFIED: add ingest/checkout/activate/deactivate/list/hook commands
├── analyze/                 # UNCHANGED — read-only analyzer stays exactly as is
├── profile/                 # NEW — the IR
│   ├── profile.go           #   builds model.Profile from []model.Block
│   ├── parteval.go          #   partial-evaluation pass (static vs dynamic tagging)
│   └── regenerate.go        #   model.Profile → per-category .zsh source
├── store/                   # NEW — git-as-database
│   ├── store.go             #   Store{repoDir}; Init/Branches/Checkout/Commit/Read
│   └── git.go               #   thin os/exec wrapper around the `git` binary
├── activate/                # NEW — the switch loop
│   ├── manifest.go          #   build model.Manifest from a profile (resolved end-state)
│   └── plan.go              #   diff(active, target) → deactivate-then-activate plan
├── shell/
│   ├── provider.go          #   UNCHANGED interfaces (+ OPTIONAL Activator)
│   └── zsh/
│       ├── parse.go         #   UNCHANGED (already the IR front-end)
│       ├── classify.go      #   UNCHANGED (Cat* taxonomy reused)
│       ├── introspect.go    #   MODIFIED: optionally dump alias/func BODIES, not just names
│       ├── emit.go          #   NEW: model.Manifest → zsh apply/deactivate shell code
│       └── loader.zsh       #   NEW: embedded sourced loader (emitted by `hook`)
├── model/
│   ├── block.go             #   UNCHANGED
│   ├── analysis.go          #   UNCHANGED (the lossy report stays)
│   ├── profile.go           #   NEW: Profile, Entry, Portability
│   └── manifest.go          #   NEW: Manifest, Scalar, ListDelta, NameSet, OptionSet
├── dto/                     # NEW DTOs for profile/manifest wire shapes (leaf, no core imports)
└── testgen/                 # UNCHANGED (oracle stays the ingest-engine regression pin)
```

### Structure Rationale

- **`core/profile`, `core/store`, `core/activate` are three separate packages** because they have three different dependency footprints and three different test strategies: `profile` is pure-Go-over-AST (unit-testable, deterministic, a natural new oracle target); `store` is subprocess-over-`git` (integration-tested against a temp repo); `activate` is pure orchestration over `Manifest` (round-trip-testable: apply then deactivate must restore byte-identical env). Splitting them keeps each one's tests honest, exactly as the existing engine splits `analyze` (orchestration) from `shell/zsh` (subprocess).
- **The IR types live in `core/model`, not in `core/profile`**, so they stay leaves with no internal deps — the same discipline that keeps `Block`/`Analysis` shell-agnostic. The *building* of the IR lives in `core/profile`; the *types* live in `core/model`.
- **All zsh-specific shell-code emission lives in `core/shell/zsh/emit.go`** — never in `core/activate`. `core/activate` must stay shell-agnostic (it operates on `Manifest`), mirroring how `core/analyze` stays shell-free and only touches the `Provider` interface. This is the single most important boundary to hold: *the orchestration layer never writes `unalias` strings.*
- **The loader is an embedded `.zsh` emitted by a `hook` subcommand**, precisely mirroring `introspectScript` (an embedded zsh string the Go code ships and runs). Users add one line to `.zshrc`: `eval "$(zsh-pro hook)"` — identical to direnv/shadowenv/chezmoi init.

---

## (a) The Regenerable, Near-Lossless IR

### Why today's `model.Analysis` cannot be the store

`model.Analysis` is a **report**, and it is lossy by design:

```go
// core/model/analysis.go — the lossy summary
type CategorySummary struct {
    Category Category
    Count    int
    Items    []string   // ← just the NAMES. The alias body, the env value,
}                        //    the function source, export-vs-typeset, original
                         //    ordering, comments — all GONE.
```

You cannot regenerate `~/.zshrc` from `Analysis`. `Items` is a `[]string` of primary names produced by `reconciler.primaryName(b)` (`analyzer.go:53`). The value half, the function body, quoting, and original ordering are discarded.

### The good news: the parser already captures (almost) everything

`model.Block` already carries the **verbatim source text** of every statement plus its structural shape:

```go
// core/model/block.go — already near-lossless per statement
type Block struct {
    Text      string     // ← VERBATIM original source (incl. leading comments)
    StartLine int
    Kind      BlockKind  // assignment / alias / func / command / compound
    CmdName   string
    Names     []string
    Exported  bool       // export vs plain assignment preserved
    Category  Category
    Conf      Confidence
}
```

`Block.Text` is the round-trip anchor. The IR's regeneration story is therefore *not* "reconstruct zsh from structured fields" (lossy, fragile, would have to reproduce quoting) — it is **"keep the verbatim text per entry, grouped and ordered by category, with structured fields layered on top for diffing and partial evaluation."** This is the chezmoi lesson: *source state is the source of truth, and apply makes the minimum change* — you store the real thing, you don't try to re-derive it.

### The new IR: `model.Profile` / `model.Entry`

```go
// core/model/profile.go  (NEW — beside Analysis, not replacing it)

// Profile is the regenerable representation of one environment: every managed
// entry, grouped by category, in original order. It round-trips to .zsh.
type Profile struct {
    Name    string   // == git branch name
    Entries []Entry  // original order preserved
}

// Entry is one managed construct, near-losslessly captured.
type Entry struct {
    Raw         string         // VERBATIM source text (from Block.Text) — the
                              //   round-trip anchor; regeneration emits this
    Category    Category
    Kind        BlockKind
    Names       []string       // for diff / dedup / shadow detection
    Exported    bool
    Portability Portability    // STATIC | DYNAMIC | MIXED  (see partial eval)
    // structured values extracted for STATIC entries, for diff + manifest:
    Value       string         // resolved literal for a static scalar; "" otherwise
    PathDelta   []string       // for PATH-like entries: the segments this adds
    Comment     string         // leading comment, kept for regeneration fidelity
}

type Portability int
const (
    PortStatic  Portability = iota // fully constant; safe to resolve/snapshot
    PortDynamic                     // contains $VAR / $(...) / conditional; late-bound
    PortMixed                       // partly constant, partly dynamic
)
```

**Lossless-enough, not byte-perfect.** True byte-for-byte round-trip of arbitrary zsh is a non-goal (and impossible without re-emitting the parser's input). The contract is: **regenerating from `Profile.Entries[].Raw` reproduces a *semantically equivalent and human-faithful* `.zshrc`** — same statements, same order, same comments, grouped by category. Anything the parser flagged `Opaque` (`parse.go:22-27`) is carried as a verbatim `Raw` entry in an `unmanaged` bucket and emitted untouched. This is the only safe stance for a tool that round-trips a user's real config.

**Where it's built:** `core/profile/profile.go` consumes the `[]model.Block` that `Provider.Parse` already returns (reused unchanged), runs each through `Provider.Classify` (reused unchanged), then the partial-eval pass, then assembles `Profile`. It depends only on `core/model` and the `Parser`/`Classifier` interfaces — **it does not bypass the seam**, it consumes the same two interface methods `core/analyze` already uses (`analyzer.go:25,36`).

**Regeneration** (`core/profile/regenerate.go`): emit a category header comment, then each `Entry.Raw` for that category in order. Per-category output is what enables "branches store categorized files" (item b) — each category becomes its own file (`aliases.zsh`, `env.zsh`, `path.zsh`, `functions.zsh`, `options.zsh`) in the git tree. There is precedent in-repo for model→zsh emission: `core/testgen/render.go` already renders structured nodes to `.zsh` source — the IR's regenerator is the production analogue, but anchored on verbatim `Raw` rather than reconstructed text.

---

## (b) The Git-Backed Storage Layer

### Decision: shell out to the `git` binary — do NOT add `go-git`

The constraint is explicit and hard: **"single external dependency (`mvdan.cc/sh/v3`) — no new dependencies."** `go-git` is a new module dependency and is out.

This is not a compromise — it is the *architecturally consistent* choice. The codebase already establishes the pattern of **shelling out to an external binary as a runtime dependency** (`zsh -f` in `core/shell/zsh/introspect.go:42-53`, with graceful degradation when the binary is absent). `git` becomes a second runtime dependency of the same shape: a `Store` that runs `git` via `os/exec` with a context timeout, exactly like `Introspect` runs `zsh`. Users of a *shell environment manager* already have `git` (it is the product's premise).

```go
// core/store/store.go  (NEW)
type Store struct{ repoDir string } // e.g. ~/.config/zsh-pro/profiles
func New(repoDir string) *Store

func (s *Store) Init() error                     // git init if absent
func (s *Store) Branches() ([]string, error)     // git branch --list  → profile names
func (s *Store) Current() (string, error)        // git rev-parse --abbrev-ref HEAD
func (s *Store) Checkout(profile string) error   // git checkout <branch>
func (s *Store) Read(profile string) (model.Profile, error)  // read tree → Profile
func (s *Store) Commit(p model.Profile, msg string) error    // write files + git commit
```

### Repo layout: branch = profile, files = categories

```
~/.config/zsh-pro/profiles/   (a git repo; one branch per environment profile)
  ├── aliases.zsh        # regenerated from Profile entries, category-grouped
  ├── env.zsh
  ├── path.zsh
  ├── functions.zsh
  ├── options.zsh
  ├── unmanaged.zsh      # opaque / imperative blocks carried verbatim, NOT switched
  └── manifest.json      # OPTIONAL: cached resolved Manifest for fast activate
```

- **`main`/`base` branch = the captured baseline** (the user's original `~/.zshrc` ingested). Each new environment is a branch off it. `checkout <branch>` is literally `git checkout` plus a re-activate.
- **What gets committed:** the regenerated per-category `.zsh` files (the IR, serialized). NOT the live `~/.zshrc`. The thin `~/.zshrc` itself is *not* in the repo — it is a fixed bootstrapping shim (item d).
- **Secrets:** the existing classifier already flags `CatSecrets` (`classify.go:33-35`) and surfaces `HasSecrets` (`analysis.go:18`). The store should treat secret entries specially — keep them out of the committed tree (write to a git-ignored sidecar) or gate on `--include-secrets`. This reuses detection that already ships. chezmoi solves the analogous problem with age-encrypted blobs, but encryption needs a dependency that "no new deps" forbids — so the safe v2.0 default is **exclude secrets from the synced tree** and document it. (Mark as a roadmap decision.)

### Why git (not a bespoke format)

The user explicitly chose "git-style repo, branches are profiles." Git gives branching, diff, history, and `checkout` for free — the exact primitives the product's verbs map onto (`checkout <branch>` ⇒ switch profile). Reimplementing branch/diff/history in a custom store would be strictly worse and is unnecessary given `git` is already a runtime expectation.

---

## (c) The Activate/Deactivate Manifest Format

This is the heart of "zero residue," and it is a **solved problem** — Shopify's **shadowenv** and **direnv** both implement exactly this (reversible, scoped env changes), and their data structures are the reference design. zsh-pro's manifest is shadowenv's `undo::Data` adapted to also cover aliases/functions/options (which direnv/shadowenv don't manage, but the zsh primitives make trivial).

### The reference: shadowenv's `undo::Data` (verified from source)

shadowenv stores, per active environment, a serialized record of *exactly what it changed and the prior value*, so it can reverse it:

```rust
// shadowenv src/undo.rs (the reference design)
struct Scalar { name: String, original: Option<String>, current: Option<String>, no_clobber: bool }
struct List   { name: String, additions: Vec<String>, deletions: Vec<String> }
struct Data   { scalars: Vec<Scalar>, lists: Vec<List>, prev_dirs: HashSet<PathBuf> }
```

The genius detail — the **no-clobber / drift guard** (`src/shadowenv.rs::unshadow`): on deactivate, restore `original` **only if the variable's *current* live value still equals what was set (`current`)**. If the user changed it by hand after activation, shadowenv refuses to clobber their change. This is what makes it safe in a *live, already-open* terminal (the frontier requirement), not just a fresh subshell.

### zsh-pro's `model.Manifest`

```go
// core/model/manifest.go  (NEW)

// Manifest is the reversible record of one profile's applied declarative state.
// It is serialized into the per-terminal state var so a later deactivate can
// cleanly un-apply WITHOUT re-reading the profile.
type Manifest struct {
    Profile string       // which profile produced this (== branch)
    Schema  string       // version tag, e.g. "v1" — forward-compat (shadowenv does this)
    Env     []Scalar     // environment variables
    Lists   []ListDelta  // PATH / FPATH / MANPATH / CDPATH — delta vs captured base
    Aliases NameSet      // alias names this profile added (→ unalias on deactivate)
    Funcs   NameSet      // function names this profile added (→ unset -f on deactivate)
    Options []OptionSet  // setopt/unsetopt changes, with prior state
}

// Scalar: an env var, reversibly. Original==nil ⇒ "was unset, so unset on deactivate".
type Scalar struct {
    Name     string
    Original *string  // value before activation (nil = was absent)
    Applied  string   // value we set (drift-guard: only restore if live == Applied)
}

// ListDelta: a PATH-like var as add/remove deltas vs the captured base, so
// deactivate removes what we added and re-adds what we removed — never a blunt
// overwrite (which would clobber session-local PATH the user added).
type ListDelta struct {
    Name      string   // "PATH"
    Additions []string // segments this profile prepended/appended
    Deletions []string // segments this profile removed from the base
}

// NameSet: names this profile defined. To restore a SHADOWED prior alias/func,
// keep its prior body so deactivate can re-establish it.
type NameSet struct {
    Added    []string
    Shadowed map[string]string // name → prior definition body, re-established on deactivate
}

type OptionSet struct {
    Name    string // e.g. "AUTO_CD"
    Enabled bool   // what this profile set it to
    WasOn   bool   // prior state (restore on deactivate)
}
```

### How each category un-applies cleanly

| State | Activate | Deactivate (zero-residue) |
|-------|----------|---------------------------|
| **Env scalar** | record `Original` (current live value or nil), set new, store `Applied` | if live value `== Applied`: restore `Original` (or `unset` if nil); else leave (user drifted) |
| **PATH / lists** | compute `Additions`/`Deletions` vs the **captured base** PATH; apply | remove each `Addition`; re-add each `Deletion`. Never overwrite PATH wholesale → session-added entries survive |
| **Alias** | record name in `Added`; if a prior alias existed, stash body in `Shadowed`; define | `unalias` each `Added`; re-define each `Shadowed` from stashed body |
| **Function** | record name in `Added`; stash prior body if any; define | `unset -f` each `Added`; re-define each `Shadowed` |
| **Option** | record `WasOn`; apply `setopt`/`unsetopt` | restore to `WasOn` |

**The "captured base" for PATH** is the key concept the question asks about. The base is the PATH **as it existed at the moment of first activation in this terminal** (or the baseline-branch's resolved PATH). The manifest stores PATH as a *delta against that base*, never as an absolute list. That is precisely why hot-switching does not cause "PATH growth" — deactivate subtracts exactly the segments activate added, regardless of what else touched PATH meanwhile. (shadowenv documents one honest caveat: re-insertion *ordering* of a removed-then-readded entry is best-effort — acceptable, since exact position is rarely load-bearing.)

**Resolving the manifest's values reuses the existing introspection mechanism.** `core/shell/zsh/introspect.go` already runs `zsh -f`, sources a config, and dumps resolved aliases, functions, exported env, **resolved `$path` in order**, and on-options (`introspectScript`, lines 23-38). Building a `Manifest` for a profile = source that profile's regenerated files under `zsh -f` and capture the end-state — *the introspect script is 90% of the manifest builder already.* The one extension needed: dump alias/function **bodies** (via `${aliases[name]}` and the `functions` associative array) not just names, so `Shadowed` restoration works. (Verified against the zsh manual: `${aliases[name]}` yields the body; the `functions` associative array maps names→definitions; both come from the `zsh/parameter` module the script already loads.)

### Serialization

`Manifest` serializes to compact JSON (a `core/dto` shape — leaf, no core imports, same pattern as `dto.Envelope`). The serialized blob is what the runtime carries per-terminal (item d). shadowenv prefixes a hash (`{hash}:{json}`) for change-detection; zsh-pro can prefix the profile name + schema version for the same purpose.

---

## (d) The Sourced Shell-Integration Runtime

### The model: loader bootstrapped from a thin `.zshrc` (direnv/shadowenv/chezmoi pattern)

The binary **cannot mutate its parent shell** (a child process can't change the parent's env/aliases/functions). Every tool in this space solves it the same way: the binary *emits shell code* and the shell *sources/evals* it. zsh-pro follows suit.

**Thin `~/.zshrc`** (the only thing the user hand-edits):

```zsh
# ── zsh-pro master block (imperative, run-once, UNMANAGED) ──
#   daemons, eval-init, anything side-effecting goes here. NOT switchable.
# ...whatever the user wants...

# ── zsh-pro loader (bootstraps the manager) ──
eval "$(zsh-pro hook)"     # installs checkout()/activate()/deactivate()
```

**`zsh-pro hook`** prints the embedded loader (`loader.zsh`, shipped as a Go string constant exactly like `introspectScript`). The loader defines shell functions. This is the shadowenv pattern verbatim:

```zsh
# shadowenv's actual zsh integration (the reference, sh/shadowenv.zsh.in)
__shadowenv_hook() {
  "@SELF@" hook "${flags[@]}" | source /dev/stdin   # ← run binary, source its stdout
}
```

### Declarative apply vs. the imperative "master block"

This split is a **hard product boundary** (PROJECT.md "Out of Scope: Owning the imperative startup surface"):

- **Declarative state** (aliases, env, PATH, functions, options) → lives in the git store as the IR → switched by `checkout`/`activate`/`deactivate`. Reversible.
- **Imperative run-once code** (daemons, `eval "$(rbenv init -)"`, side-effecting init) → stays in the unmanaged `.zshrc` master block → never switched, never reversed.

The classifier's existing taxonomy *is* the splitter: `CatAliases`/`CatEnvironment`/`CatPath`/`CatFunctions`/`CatOptions` are declarative-switchable; `CatPlugins`/`CatMisc`/opaque/`eval`-init are imperative → master block. The ingest step uses classification (already shipped) to route each block to one side of the line. **Reuse, not rebuild.**

**Declarative-apply over imperative replay.** When switching, the runtime does **not** re-source the new profile's raw `.zsh` (that would be imperative and irreversible — the old "master block" anti-pattern). It applies a **computed declarative diff**: `deactivate(active_manifest)` then `activate(target_manifest)`, where both are reversible `Manifest`s. This is what makes zero-residue *possible* — you can only cleanly reverse a recorded, structured set of changes, never an arbitrary script.

### Per-terminal vs. global active-branch state

**Per-terminal, full stop** — and the mechanism is free: **carry the active state in an environment variable**, because env vars are per-process and inherited only by children. shadowenv does exactly this with `__shadowenv_data`:

```
__ZSHPRO_STATE="<profile>:<schema>:<base64-json-manifest>"
```

- Each terminal has its own `__ZSHPRO_STATE`; switching in terminal A does not touch terminal B. This is automatic — no lockfile, no global "current profile" file, no IPC.
- `deactivate` reads `__ZSHPRO_STATE`, reverses the embedded manifest, unsets the var. `activate` sets it. `checkout` = deactivate-then-activate, then update the var.
- The loader emits `export __ZSHPRO_STATE=...` as part of the shell code it asks the parent to `eval` — that's how the per-terminal var gets updated *in the parent*.

A *global* "default profile for new terminals" is a separate, optional concept (a file the loader reads at startup to auto-activate) — fine to add, but the *live* active state must be per-terminal. Do not invert this: a global mutable "current branch" file shared across terminals is the classic footgun (terminal A's `checkout` silently changes what terminal B will deactivate next).

### chpwd vs. explicit-command activation

direnv/shadowenv hook `chpwd`/`precmd` to auto-switch on `cd` (directory-scoped). zsh-pro is **branch-scoped, not directory-scoped** — the user runs `checkout <branch>` explicitly. So the loader's primary surface is **explicit functions** (`checkout`, `activate`, `deactivate`), and a `precmd` hook is *optional* (only needed if you later want "re-assert my profile every prompt" hardening). Start with explicit commands; the hook is a later robustness layer.

### The optional `Activator` interface

The seam today is `Parser`/`Classifier`/`Introspector` composed into `Provider`. Adding manifest-emission could be a fourth ISP interface (`Activator { Emit(Manifest) ([]byte, error) }`). **Recommendation: defer it.** v2.0 is zsh-only (PROJECT.md Out-of-Scope: other shells), so a single concrete `emit.go` method on `zsh.Provider` is enough; introducing the interface now is speculative generality. Add it only when a second shell is actually planned — the refactor is mechanical and the YAGNI cost of waiting is near-zero.

---

## (e) Where Partial Evaluation Lives

**A dedicated pass in `core/profile/parteval.go`, between classification and IR assembly — pure Go over the parsed AST, shell-free.**

### What it does

For each entry, decide `Portability` and, for static entries, extract the resolved literal value:

- **`PortStatic`** — value is a constant literal (`alias gs='git status'`, `export EDITOR=vim`). The AST word has no `*ParamExp`, `*CmdSubst`, `*ArithmExp`, or backticks. Safe to resolve to a literal and snapshot into the `Manifest` for fast/exact apply.
- **`PortDynamic`** — value references `$HOME`, `$(...)`, `${VAR}`, or sits inside a conditional. **Keep the raw text; never resolve.** This is the portability guarantee: `export PATH="$HOME/bin:$PATH"` stays `$HOME/bin:$PATH`, so the profile works on any machine. At *activate* time the live shell expands it.
- **`PortMixed`** — partly literal, partly dynamic (e.g. `export FOO="static-prefix-$DYN"`). Treat as dynamic for safety (keep raw, expand at activate time).

### Why a separate pass, and where exactly

- **It is shell-agnostic logic** (walking `mvdan.cc/sh` AST nodes), so it belongs beside the other Go-side analysis, not in `core/shell/zsh`. But it needs richer AST access than the current `Provider.Parse` exposes (today `Block` only keeps `Text` + names, not the word-part tree). Two options:
  1. **Extend the parser output** minimally — compute a "has dynamic parts" determination in `core/shell/zsh/parse.go` (where the AST is in scope, inside `describe`) and surface it as a `bool`/enum on `Block`. Keeps AST handling in the one package that already imports `syntax`.
  2. **Re-parse in `core/profile`** — `mvdan.cc/sh` is already a dependency and `core/profile` may import it (it is not the shell-specific *provider*, just an AST consumer). Re-parsing each entry's `Raw` to inspect word parts keeps `Block` untouched.
- **Recommendation: option 1 for the static/dynamic *flag* (the AST is right there in `describe`), option 2 only if deeper structural extraction is needed.** Either way the *decision* ("is this portable?") is the partial-eval pass's job and lives logically in `core/profile`.

### The hard rule (from PROJECT.md "Out of Scope")

Partial evaluation **must never resolve `~`/`$HOME`/`$(...)` against the current machine's disk or live env.** Resolving them would freeze the profile to one machine and destroy branch portability. "Static" means *syntactically constant*, not *evaluated-on-this-box*. This is a correctness boundary, not a nicety — the test suite should assert that a profile containing `$HOME` round-trips with `$HOME` intact. (Note: `core/util/path.go::ExpandHome` resolves `~` against the real home — it is used by the *analyzer's* file-open path and must **not** be reused inside the IR/partial-eval; resolving there is exactly the forbidden freeze.)

---

## (f) Suggested Build Order — and What to SPIKE First

### SPIKE FIRST (before any IR/git/CLI work): the zero-residue switch loop

**This is the de-risking the PROJECT.md frontier feature explicitly calls for** ("de-risked by a spike before committing"). It is independent of all the Go plumbing and it is the one thing that can invalidate the entire product.

**Spike scope (throwaway, hand-built):**
1. Two hand-written `Manifest` JSON files for two fake profiles (no IR, no git, no parser).
2. A hand-written loader (`activate`/`deactivate`/`checkout` zsh functions) + the per-terminal `__ZSHPRO_STATE` var.
3. Prove the loop in a *live, already-open* terminal: `activate A → activate B (auto-deactivates A) → deactivate B`.
4. **Assert zero residue** by snapshotting `$aliases`, `$functions`, `$PATH`, `$path`, exported env, and `$options` (via `zmodload zsh/parameter`) before activate and after the final deactivate — they must be **byte-identical**.
5. Specifically stress the three known failure modes: (a) **PATH growth** across repeated switch cycles; (b) **stale aliases/functions** after deactivate; (c) **drifted env** (user changes a var mid-session — deactivate must NOT clobber it).

**Why first:** if zero-residue hot-switch turns out to be infeasible in a live terminal (e.g. some option, completion, or hook state can't be cleanly reversed), the product scope must change *before* you've built an IR and a git store on top of it. Everything downstream assumes this loop works. The spike costs ~1 phase and saves a possible full rewrite. The verified shadowenv/direnv precedent says it *is* feasible — but their scope is env-only; zsh-pro adds aliases/functions/options, which is the unproven delta worth spiking.

### Then, in dependency order

| Order | Phase (topic) | Depends on | Why this position |
|-------|---------------|------------|-------------------|
| **0** | **SPIKE: zero-residue activate→switch→reactivate** (hand-built manifest + loader) | nothing | De-risks the frontier; can invalidate scope; informs the `Manifest` shape |
| **1** | **IR + partial evaluation** (`core/profile`, `model.Profile/Entry`, `parteval`) | parser/classifier (shipped) | The store and manifest both need a profile to serialize; reuses the seam; new oracle target |
| **2** | **git-backed store** (`core/store`, branch=profile, per-category files) | IR (must serialize *something*) | `checkout` needs profiles to switch between; isolated subprocess-over-`git` |
| **3** | **Manifest builder + emit** (`core/activate`, `core/shell/zsh/emit.go`, extended introspect) | spike (proved the design), IR (resolves a profile) | Turns a profile into the reversible record the runtime applies |
| **4** | **Runtime loader + CLI commands** (`hook`, `checkout`, `activate`, `deactivate`, `list`, `__ZSHPRO_STATE`) | manifest+emit, store | Wires the live terminal to the binary; the user-facing verb surface |
| **5** | **Ingest end-to-end** (`zshrc` → IR → baseline branch committed) | IR, store | The on-ramp: turn a real `~/.zshrc` into the baseline profile |

**Sequencing rationale:** the IR is the spine — store, manifest, and regeneration all serialize it, so it lands right after the spike validates *what the manifest must contain*. Store before manifest-emit because `checkout` is meaningless without profiles to switch between. Runtime last because it composes everything; by then the manifest design is proven (spike) and profiles exist (store). Ingest can technically come earlier but is best last: it is the polished on-ramp, and doing it last means it targets the *final* IR shape rather than chasing a moving target.

> Alternative ordering considered: store (2) before IR (1). Rejected — the store's `Read`/`Commit` are typed in terms of `model.Profile`; building the git plumbing before the IR exists means designing `Store` against a placeholder and reworking it. IR-first gives the store a stable type to serialize.

---

## Architectural Patterns

### Pattern 1: Emit-and-source (binary cannot mutate its parent shell)

**What:** The Go binary never changes the live shell directly; it prints shell code to stdout, and a sourced shell function `eval`s/sources it. The parent shell applies the change to itself.
**When to use:** Any time activation must affect the *calling* interactive shell (all of activate/deactivate/checkout).
**Trade-offs:** (+) The only correct way to mutate a parent shell; per-terminal by construction. (−) The emitted code is shell-specific and must be escaped carefully (quoting bugs become shell-injection bugs). Keep all emission in `core/shell/zsh/emit.go` and unit-test the escaping.

```zsh
# the loader (emitted by `zsh-pro hook`), shadowenv-style
checkout() { eval "$(zsh-pro checkout "$1")"; }   # binary prints the apply-plan; shell evals it
```

### Pattern 2: Reversible diff with a drift guard (shadowenv `undo::Data`)

**What:** Record both the prior value and the value you applied. On reverse, restore the prior value **only if the live value still equals what you applied**.
**When to use:** Every reversible mutation in a live terminal — the core of zero-residue.
**Trade-offs:** (+) Safe in already-open terminals; respects the user's mid-session changes. (−) Requires storing more state (both values) and a per-terminal blob. Non-negotiable for the frontier feature.

```go
// deactivate a scalar, guarded
if liveValue(s.Name) == s.Applied {       // we still own it
    if s.Original == nil { unset(s.Name) } else { set(s.Name, *s.Original) }
} // else: user changed it after we set it — leave it alone
```

### Pattern 3: PATH as a delta vs a captured base (never a wholesale overwrite)

**What:** Store PATH changes as `{additions, deletions}` against the PATH captured at first-activation, not as an absolute list. Reverse by subtracting additions / re-adding deletions.
**When to use:** All ordered-list env vars (PATH, FPATH, MANPATH, CDPATH).
**Trade-offs:** (+) No PATH growth across switch cycles; session-added entries survive a switch. (−) Approximate ordering on restore (shadowenv documents this exact caveat). Acceptable — exact ordering of removed-then-readded entries is rarely load-bearing.

### Pattern 4: Subprocess provider with graceful degradation (existing — extend, don't reinvent)

**What:** Run an external binary (`zsh -f`, `git`) under a context timeout; on any failure return an "unavailable" sentinel and let the caller degrade. Already established in `Introspect` (`introspect.go:42-53`).
**When to use:** The new `git` store; the extended introspect for manifest-building.
**Trade-offs:** (+) Consistent with the codebase; no new deps; honest failure modes. (−) Subprocess latency per call (fine for an interactive tool).

---

## Data Flow

### Ingest flow (one-time, on-ramp)

```
~/.zshrc bytes
   ↓  Provider.Parse        (REUSED — already returns []Block with verbatim Text)
[]model.Block
   ↓  Provider.Classify     (REUSED — Cat* taxonomy routes declarative vs imperative)
classified blocks
   ↓  core/profile partial-eval pass   (NEW — tag Static/Dynamic, keep dynamics raw)
model.Profile
   ↓  core/profile regenerate          (NEW — per-category .zsh)
   ↓  core/store Commit                (NEW — write files, git commit to baseline branch)
git repo (baseline branch = the profile)
```

### Checkout / live-switch flow (the product's core verb)

```
user runs:  checkout work     (a loader fn)
   ↓  shell fn: eval "$(zsh-pro checkout work)"
   ↓  binary reads __ZSHPRO_STATE  (active manifest, from this terminal's env)
   ↓  core/store Read("work")  →  model.Profile
   ↓  core/activate build Manifest (resolve via extended `zsh -f` introspect)
   ↓  core/activate plan = deactivate(active) THEN activate(target)
   ↓  core/shell/zsh/emit → shell code (unalias/unset -f/export/setopt/PATH delta + new __ZSHPRO_STATE)
   ↓  binary prints shell code to stdout
shell `eval`s it → live terminal now in profile "work", state var updated, ZERO residue from prior
```

### State management (per-terminal, env-var-carried)

```
terminal A env:  __ZSHPRO_STATE = "work:v1:<json manifest>"
terminal B env:  __ZSHPRO_STATE = "personal:v1:<json manifest>"   ← independent

activate:   reads/sets __ZSHPRO_STATE  (no global file, no lock)
deactivate: reads __ZSHPRO_STATE → reverses embedded manifest → unsets the var
```

---

## Scaling Considerations

Scale here is **config size and switch frequency**, not users (it's a single-user CLI).

| Scale | Architecture Adjustments |
|-------|--------------------------|
| Typical `.zshrc` (100s of lines, <20 profiles) | None — parse + `git checkout` + emit are all sub-100ms; introspect's existing 5s timeout is ample |
| Large config (1000s of lines, heavy plugins) | Cache the resolved `Manifest` per profile (`manifest.json` in the tree) so `activate` skips re-introspecting `zsh -f` every switch; invalidate on commit |
| Very frequent switching / prompt-hook re-assert | Add a hash-skip (shadowenv's `prev_hash` trick): if the target profile's hash matches the active state, emit nothing and return immediately |

### Scaling Priorities

1. **First bottleneck:** re-running `zsh -f` introspection on every `activate`. Fix: cache the built `Manifest` in the git tree; rebuild only on `commit`/`ingest`. The manifest is pure-derived from the profile, so caching is safe.
2. **Second bottleneck:** large PATH delta diffs on every switch. Fix: precompute the `ListDelta` at commit time and store it; switch just applies the stored delta.

---

## Anti-Patterns

### Anti-Pattern 1: Re-sourcing the new profile's raw `.zsh` to "switch" (the imperative master-block trap)

**What people do:** `checkout` = `source ~/.config/.../work/*.zsh`.
**Why it's wrong:** Sourcing is imperative and **irreversible** — there is no clean `un-source`. You get PATH growth, leftover aliases, stale functions. It is exactly the "master block" the product is replacing.
**Do this instead:** Compute a reversible declarative `Manifest`; apply `deactivate(old)` then `activate(new)` as recorded diffs. Only structured, recorded changes can be cleanly reversed.

### Anti-Pattern 2: Storing the active profile in a global file shared across terminals

**What people do:** Write `~/.config/zsh-pro/current` and have every terminal read it.
**Why it's wrong:** Terminal A's `checkout` silently changes what terminal B will deactivate next — cross-terminal corruption, the opposite of zero-residue.
**Do this instead:** Carry active state in a **per-process env var** (`__ZSHPRO_STATE`), shadowenv-style. Per-terminal isolation is then automatic. A global file may *only* hold the "default profile for brand-new terminals," never the live active state.

### Anti-Pattern 3: Resolving `$HOME`/`$(...)` at ingest to "simplify" the manifest

**What people do:** Expand dynamic values to literals during partial evaluation so apply is "just set the value."
**Why it's wrong:** Freezes the profile to the machine it was ingested on; `$HOME/bin` becomes `/Users/alice/bin` and breaks on every other box. Destroys the portability that is the product's stated core value.
**Do this instead:** "Static" = *syntactically constant only*. Keep every `$VAR`/`$(...)`/conditional as raw text; let the live shell expand it at activate time. Assert this with a round-trip test. (And do **not** reuse `util.ExpandHome` inside the IR — it does exactly the forbidden resolution.)

### Anti-Pattern 4: Overwriting PATH wholesale on activate/deactivate

**What people do:** Save the full PATH string, restore the full PATH string.
**Why it's wrong:** Clobbers anything the user (or another tool) added to PATH during the session; and re-applying a saved absolute PATH causes growth when composed across switches.
**Do this instead:** Store PATH as a **delta vs the captured base** (`additions`/`deletions`); reverse by subtracting/re-adding exactly those segments.

### Anti-Pattern 5: Adding `go-git` (or any new dependency) for the store

**What people do:** Reach for `go-git` because "it's the Go way."
**Why it's wrong:** Violates the hard "no new dependencies beyond `mvdan.cc/sh`" constraint, and breaks the established pattern (the codebase already shells out to an external binary, `zsh`, with graceful degradation).
**Do this instead:** Shell out to the `git` binary via `os/exec` with a context timeout — a direct sibling of the existing `Introspect` subprocess. `git` is already a runtime premise of a *shell environment manager*.

### Anti-Pattern 6: Emitting `unalias`/`unset -f`/`setopt` strings from `core/activate`

**What people do:** Build the shell code in the orchestration layer because it's convenient.
**Why it's wrong:** Leaks zsh syntax into the shell-agnostic layer, breaking the seam that keeps `core/analyze` shell-free and makes the engine testable/portable.
**Do this instead:** `core/activate` produces a shell-agnostic `Manifest`/plan; **only `core/shell/zsh/emit.go`** turns it into zsh text. Same discipline as `core/analyze` → `Provider` interface.

### Anti-Pattern 7: Making `core/profile` import `core/shell/zsh`

**What people do:** Import the concrete provider for convenience when building the IR.
**Why it's wrong:** Breaks the single-composition-root rule (`core/shell/zsh` imported only at `cmd/zsh-pro/main.go`) that the whole architecture rests on.
**Do this instead:** `core/profile` depends on the `Parser`/`Classifier` *interfaces* (injected), exactly as `core/analyze` does. The concrete `zsh.Provider` is wired in only at `main.go`.

---

## Integration Points

### External Services (subprocess runtime deps — same shape as the existing `zsh -f`)

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| `git` binary | `os/exec` + `context.WithTimeout`, in `core/store/git.go` | New runtime dep; degrade with a clear error if absent (mirror `Introspect`'s Available:false). Do NOT add `go-git`. |
| `zsh -f` binary | **REUSE** `core/shell/zsh/introspect.go`; extend the embedded script to also dump alias/function **bodies** (`${aliases[k]}`, `functions` assoc array) for manifest-building | Already established, already degrades gracefully; bodies needed for `Shadowed` restore |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `core/profile` ↔ `core/shell` | depends on `Parser`/`Classifier` **interfaces** (not `zsh` concrete) | Same seam `core/analyze` uses — do not bypass it; `core/profile` may import `mvdan.cc/sh` as a pure AST consumer |
| `core/activate` ↔ `core/shell/zsh` | via a render method (`emit`) — optionally behind a new `Activator` interface (deferred) | `core/activate` stays shell-agnostic (operates on `Manifest`); zsh syntax lives only in `emit.go` |
| `core/store` ↔ `core/profile` | `Store.Read` returns `model.Profile`; `Store.Commit` takes `model.Profile` | Store serializes/deserializes the IR; it does not understand zsh |
| `core/cli` ↔ new packages | new `case`s in `CLI.Run`; same exit-code + JSON-envelope contract | Reuse `fail()` and the `dto.Envelope` discipline for any `--json` output |
| composition root ↔ everything | `cmd/zsh-pro/main.go` wires `zsh.Provider{}` **and** `store.New(dir)` | Still the only place importing `core/shell/zsh`; now also constructs the `Store`. ~1-3 added lines. |

### The seam, restated (the quality gate)

- **Reused unchanged:** `Provider.Parse`, `Provider.Classify`, `Provider.Categories`, the `Cat*` taxonomy, `model.Block`'s verbatim `Text`, the `zsh -f` introspect subprocess, the `dto.Envelope`/exit-code contract, the `testgen` oracle (as the ingest regression pin).
- **Modified (additive):** `core/model` (+`Profile`/`Entry`/`Manifest`), `core/shell/zsh` (+`emit.go`, +body-dumping in `introspect.go`), `core/cli` (+commands), `cmd/zsh-pro/main.go` (+store wiring).
- **New:** `core/profile`, `core/store`, `core/activate`, the embedded loader, optionally a deferred `shell.Activator` interface.
- **Never bypassed:** the orchestration layer (`profile`/`activate`) never imports `core/shell/zsh` and never writes zsh syntax; only the concrete provider does. Single composition root preserved.

---

## Sources

- Existing codebase (read directly): `core/shell/zsh/parse.go`, `classify.go`, `introspect.go`; `core/model/{block,analysis,identityset,category}.go`; `core/analyze/analyzer.go`; `core/cli/cli.go`; `core/shell/provider.go`; `core/dto/*`; `core/render/json.go`; `core/testgen/render.go`; `core/util/path.go`; `core/cmd/zsh-pro/main.go` — HIGH confidence (primary source).
- Shopify **shadowenv** source — the reference reversible-manifest design: `src/undo.rs` (`Scalar`/`List`/`Data`), `src/shadowenv.rs` (`unshadow`/`shadowenv_data`/drift guard), `sh/shadowenv.zsh.in` (emit-and-source hook), `src/hook.rs` (`Modifications`, schema versioning), `src/loader.rs` — HIGH confidence (read from source). https://github.com/Shopify/shadowenv
- **direnv** — independent confirmation of capture-diff-restore + per-shell hook + export-diff (sub-process diff) model. https://direnv.net/ , https://github.com/direnv/direnv — MEDIUM-HIGH (docs + widely-known mechanism).
- **chezmoi** — source-state→target-state→apply-minimum-diff, and "templates (dynamic values) eliminate per-machine branching" — validates keeping dynamics late-bound for portability. https://www.chezmoi.io/ — MEDIUM-HIGH (official docs).
- **zsh** — `zmodload zsh/parameter` exposes `$aliases`/`$functions`/`$options` associative arrays (read + mutate); `${aliases[name]}` yields the alias body; the `functions` array maps names→definitions; `unalias`/`unset -f`/`unsetopt` are the reversal primitives. zsh manual (Options, zshmodules). https://zsh.sourceforge.io/Doc/Release/Options.html — HIGH (official manual + corroborated).
- **go-git vs shelling out** — confirms `go-git` is a new module dependency (ruled out by no-new-deps); shelling out to `git` keeps deps minimal. https://github.com/go-git/go-git — MEDIUM.

---
*Architecture research for: branchable git-versioned zsh environment manager (v2.0 pivot, brownfield integration)*
*Researched: 2026-06-25*
