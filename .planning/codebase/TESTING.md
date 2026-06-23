# Testing Patterns

**Analysis Date:** 2026-06-23

## Test Framework

**Runner:**
- Go stdlib `testing` package — no third-party test framework
- Go 1.25 (declared in `go.mod`); fuzz mode via `go test -fuzz`

**Assertion Library:**
- None — plain `t.Errorf`, `t.Fatalf`, `t.Fatal`, `t.Error`, `t.Log`, `t.Helper()`

**Run Commands:**
```bash
GOTOOLCHAIN=auto go test ./...          # Run all tests
GOTOOLCHAIN=auto go test -v ./...       # Verbose output
GOTOOLCHAIN=auto go test -run TestName ./pkg  # Single test
GOTOOLCHAIN=auto go test -count=1 ./...  # Disable result cache
GOTOOLCHAIN=auto go test -fuzz FuzzName ./core/testgen  # Fuzz mode
GOTOOLCHAIN=auto go build ./...         # Compile gate
GOTOOLCHAIN=auto go vet ./...           # Static analysis gate
```

## Test File Organization

**Location:** Co-located with production code in the same directory

**Naming:**
- Internal package tests (white-box): `<subject>_test.go` with `package <pkg>` — e.g. `core/analyze/analyze_test.go` uses `package analyze`
- External package tests (black-box): same filename pattern but `package <pkg>_test` — e.g. `core/analyze/corpus_test.go` uses `package analyze_test`, `core/testgen/property_test.go` uses `package testgen_test`

**Which to use:**
- Internal (`package X`) when testing unexported helpers or when the test needs to define private mock types that satisfy interfaces (e.g. `mockProvider` in `analyze_test.go`)
- External (`package X_test`) when the test is the composition root — wiring concrete implementations to the interface under test (e.g. `corpus_test.go` is the only place that imports `zsh.Provider` into the `analyze` pipeline)

**Structure:**
```
core/
  model/         model_test.go          (package model)
  buildinfo/     buildinfo_test.go      (package buildinfo)
  analyze/       analyze_test.go        (package analyze)         ← internal
                 corpus_test.go         (package analyze_test)    ← external corpus runner
  render/        render_test.go         (package render)          ← internal
  cli/           cli_test.go            (package cli)             ← internal
  shell/zsh/     classify_test.go       (package zsh)             ← internal
                 parse_test.go          (package zsh)             ← internal
                 introspect_test.go     (package zsh)             ← internal
  testgen/       generator_test.go      (package testgen)         ← internal
                 graph_test.go          (package testgen)         ← internal
                 oracle_test.go         (package testgen)         ← internal
                 property_test.go       (package testgen_test)    ← external oracle prop test
                 render_test.go         (package testgen_test)    ← external render round-trip
                 fuzz_test.go           (package testgen_test)    ← external fuzz survival
  cmd/zsh-gen/   main_test.go           (package main)            ← internal cmd test
```

## Test Structure

**Suite Organization:**
```go
// Internal tests: flat, function-per-scenario
func TestAnalyzeDetectsDuplicateAlias(t *testing.T) { ... }
func TestAnalyzeDegradesWhenIntrospectionUnavailable(t *testing.T) { ... }

// Table-driven tests: anonymous struct slice with named cases
func TestClassify(t *testing.T) {
    cases := []struct {
        name string
        b    model.Block
        want model.Category
        conf model.Confidence
    }{
        {"alias", model.Block{...}, model.CatAliases, model.ConfHigh},
        ...
    }
    for _, c := range cases {
        // assertion directly in loop body, not t.Run subtests
    }
}

// Sub-tests via t.Run for seed-parameterized loops
func TestOracleProperty(t *testing.T) {
    for _, seed := range []int64{1, 2, 3, 5, 8, 13, 21, 34, 55, 89} {
        t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) { ... })
    }
}
```

**Patterns:**
- `t.Helper()` used on shared setup helpers (`writeRC` in `cli_test.go`)
- `t.TempDir()` for file-system fixtures; automatically cleaned up
- Failures logged with `t.Fatalf` (stop immediately) or `t.Errorf` (accumulate and continue) depending on whether the failure makes subsequent assertions meaningless
- Seed-logged failures include the generated source for reproduction: `t.Errorf(format+"\n--- seed %d, source ---\n%s", ..., seed, src)`

## Mocking

**Framework:** Hand-rolled interface implementations — no mocking library

**Pattern:**
```go
// Internal to the test package; satisfies the shell.Provider interface
type mockProvider struct {
    blocks []model.Block
    ids    model.IdentitySet
}

func (m mockProvider) Parse(_ []byte) ([]model.Block, error) { return m.blocks, nil }
func (m mockProvider) Classify(b model.Block) (model.Category, model.Confidence) {
    return b.Category, b.Conf
}
func (m mockProvider) Introspect(_ string) (model.IdentitySet, error) { return m.ids, nil }
func (m mockProvider) Categories() []model.Category                   { return model.Categories() }
```
Defined in `core/analyze/analyze_test.go`.

**Embedding for error variants:**
```go
// errProvider wraps mockProvider but fails introspection
type errProvider struct{ mockProvider }
func (errProvider) Introspect(_ string) (model.IdentitySet, error) {
    return model.IdentitySet{Available: false}, errIntrospect
}
// Sentinel error via typed string
var errIntrospect = errTest("introspection failed")
type errTest string
func (e errTest) Error() string { return string(e) }
```

**What to mock:**
- `shell.Provider` in `core/analyze` tests — isolates the reconciler from zsh entirely
- File I/O is not mocked; tests use `t.TempDir()` + real `os.WriteFile`

**What NOT to mock:**
- The concrete `zsh.Provider{}` in integration/corpus/property tests — these always use the real parser, classifier, and introspector

## Tier 1: Golden Corpus (`corpus_test.go`)

**Location:** `core/analyze/corpus_test.go` (external `package analyze_test`)

**Fixture files:** `core/testdata/fixtures/*.zsh` — hand-authored real-world configs

**Ground truth:** `core/testdata/fixtures/manifests.json` — one entry per fixture with:
- `min_blocks` — minimum block count the parser must recognize
- `issue_kinds` — exact set of issue kind strings expected (no extras, no missing)
- `has_secrets` — whether `HasSecrets` must be true
- `expect_categories` — categories that must appear (subset check)

**How to add a fixture:**
1. Drop a `.zsh` file in `core/testdata/fixtures/`
2. Add one entry to `manifests.json` — no test code changes needed

**The test:**
```go
func TestCorpusGolden(t *testing.T) {
    // reads manifests.json
    // for each fixture: analyze with real zsh.Provider, assert against manifest
    for name, m := range manifests {
        t.Run(name, func(t *testing.T) { ... })
    }
}
```
(`core/analyze/corpus_test.go:28-88`)

## Tier 2: Oracle Property Tests (`property_test.go`)

**Location:** `core/testgen/property_test.go` (external `package testgen_test`)

**Purpose:** Generates a random config from a seeded `ConfigGraph`, renders it to `.zsh`, runs the real engine, and compares the engine's output to the oracle (`ConfigGraph.Expected()`). This catches analyzer regressions across many random inputs without hand-authoring fixtures.

**Seeds:** 10 fixed seeds (`1, 2, 3, 5, 8, 13, 21, 34, 55, 89`) — each runs as a `t.Run` sub-test

**Strict subset checked:**
- Category set and per-category counts match oracle
- Issue `(kind, name)` set matches exactly (no extras, no missing)
- `HasSecrets` matches
- `OpaqueBlocks == 0` (generated configs must parse cleanly)

**Lines not yet compared:** `checkLines = false` constant guards the issue-line / total-Lines comparison (two known bugs tracked); flip to `true` once the comment-line mis-attribution and `Lines` off-by-one bugs are fixed (`core/testgen/property_test.go:17-18`)

**Oracle:**
```go
func (g *ConfigGraph) Expected() model.Analysis { ... }
```
(`core/testgen/oracle.go`) — derives expected issues and category counts directly from the graph, independent of the engine

## Tier 3: Fuzz Survival (`fuzz_test.go`)

**Location:** `core/testgen/fuzz_test.go` (external `package testgen_test`)

**Purpose:** Robustness testing — asserts the engine never panics and the JSON renderer always emits valid JSON, even on corrupted input. No oracle; correctness is not tested here.

**Iteration count:** `fuzzIters = 40` (bounded because each iteration spawns `zsh -f`)

**Mutation operators** (via `Mutator.Corrupt` in `core/testgen/mutate.go`):
- Truncate at a random byte
- Overwrite a random byte
- Insert an unbalanced `"`
- Inject an invalid UTF-8 byte (`0xff`)
- Drop the first `}` or append a stray `{`

**Panic recovery:**
```go
func analyzeNoPanic(src []byte, path string) (out []byte, ok bool) {
    defer func() { if r := recover(); r != nil { ok = false } }()
    a := analyze.New(zsh.Provider{}).Analyze(src, path)
    b, err := render.JSONRenderer{}.Render(a)
    ...
}
```
(`core/testgen/fuzz_test.go:43-55`)

## Fixtures and Factories

**Golden fixture files:**
```
core/testdata/fixtures/
  clean_baseline.zsh       # no issues; environment + aliases + options
  duplicate_aliases.zsh    # duplicate_alias issue
  empty.zsh                # zero blocks
  installer_junk.zsh       # plugin init patterns; no issues
  reassigned_env.zsh       # reassigned_env issue
  secrets_inline.zsh       # has_secrets=true
  manifests.json           # ground truth for all fixtures
```

**Test data helpers:**
- `writeRC(t, content)` in `core/cli/cli_test.go` — creates a temp `.zsh` file and returns its path; uses `t.Helper()` + `t.TempDir()`
- `sampleParams()` in `core/testgen/generator_test.go` and `propParams()` in `core/testgen/property_test.go` — shared `GenParams` literals for deterministic multi-test use
- `sampleAnalysis()` in `core/render/render_test.go` — hand-constructed `model.Analysis` for renderer assertions

**Seeded PRNG determinism:** All generated configs use `rand.New(rand.NewSource(seed))` — same seed → same graph → same `.zsh` source → reproducible failures

## Coverage

**Requirements:** No coverage threshold enforced; no `coverprofile` CI step detected

**View Coverage:**
```bash
GOTOOLCHAIN=auto go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

## Test Types

**Unit Tests:**
- Scope: single type or method, isolated via mock or zero-value receiver
- Examples: `TestClassify` (`core/shell/zsh/classify_test.go`), `TestCategoriesOrderedAndComplete` (`core/model/model_test.go`), `TestBuildIsDeterministic` (`core/testgen/generator_test.go`)

**Integration Tests (without external process):**
- Scope: multiple real packages wired together, no `zsh` process; use `mockProvider` to isolate
- Examples: `TestAnalyzeDetectsDuplicateAlias`, `TestAnalyzeIgnoresInheritedShadowIdentities` (`core/analyze/analyze_test.go`)

**Integration Tests (with real `zsh` process):**
- Scope: full pipeline through `zsh.Provider.Introspect` — spawns `zsh -f`; skipped if `zsh` not in `PATH`
- Examples: `TestIntrospectReadsAliasesAndFunctions` (`core/shell/zsh/introspect_test.go`), `TestRunCleanConfigExitsZero` (`core/cli/cli_test.go`), `TestCorpusGolden` (`core/analyze/corpus_test.go`), `TestOracleProperty` (`core/testgen/property_test.go`)

**Fuzz Tests:**
- `TestFuzzSurvival` in `core/testgen/fuzz_test.go` — bounded deterministic loop; not a `testing.F` fuzz function (does not use `f.Fuzz`)

## Common Patterns

**Skipping when `zsh` absent:**
```go
if _, err := exec.LookPath("zsh"); err != nil {
    t.Skip("zsh not installed; skipping dynamic introspection test")
}
```
(`core/shell/zsh/introspect_test.go:12-14`)

**Asserting a specific exit-code contract:**
```go
code := New(zsh.Provider{}).Run([]string{"analyze", p}, &out, &errBuf)
if code != 3 {
    t.Fatalf("exit code = %d, want 3", code)
}
```
(`core/cli/cli_test.go:36-39`)

**Asserting JSON output:**
```go
var obj map[string]any
dec := json.NewDecoder(bytes.NewReader(outBuf.Bytes()))
if err := dec.Decode(&obj); err != nil { t.Fatalf(...) }
if dec.More() { t.Errorf("expected exactly one JSON object...") }
if ok, found := obj["ok"].(bool); !found || ok { t.Errorf(...) }
```
(`core/cli/cli_test.go:83-93`)

**Asserting issue sets without caring about order:**
```go
gotKinds := map[string]bool{}
for _, is := range a.Issues { gotKinds[string(is.Kind)] = true }
for _, want := range m.IssueKinds {
    if !gotKinds[want] { t.Errorf("missing expected issue kind %q", want) }
}
```
(`core/analyze/corpus_test.go:57-65`)

---

*Testing analysis: 2026-06-23*
