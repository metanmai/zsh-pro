---
phase: 02-ir-partial-evaluation
reviewed: 2026-06-27T00:00:00Z
depth: standard
files_reviewed: 15
files_reviewed_list:
  - core/model/profile.go
  - core/model/profile_test.go
  - core/model/block.go
  - core/shell/provider.go
  - core/shell/zsh/parse.go
  - core/shell/zsh/dynamic_test.go
  - core/shell/zsh/regen.go
  - core/shell/zsh/regen_test.go
  - core/ir/build.go
  - core/ir/build_test.go
  - core/ir/route.go
  - core/ir/regen.go
  - core/ir/regen_test.go
  - core/ir/roundtrip_test.go
  - core/analyze/analyze_test.go
findings:
  critical: 2
  blocker: 2
  warning: 4
  info: 2
  total: 8
status: issues_found
---

# Phase 2: Code Review Report

**Reviewed:** 2026-06-27
**Depth:** standard
**Files Reviewed:** 15
**Status:** issues_found

## Summary

Reviewed the new IR layer (`model.Profile`/`Entry`), parser value/dynamic capture, the declarative/imperative router, the zsh regenerator, and the round-trip oracle.

The architectural invariants hold: `core/model` has no internal deps, `core/ir` (non-test) does not import `core/shell/zsh`, no `util.ExpandHome` appears in `ir`/`model`, dynamic values are stored and emitted verbatim, and the round-trip oracle uses `t.TempDir()` + a 5s timeout and is skip-guarded when zsh is absent. The `path` argument to `zsh -c` is passed as a separate argv element (not string-interpolated), so there is no command-injection vector through the introspect call.

However, the regenerator has **two confirmed crash/data-loss defects** that the test suite does not cover (all current tests pass while the bugs are live). Both are reachable from ordinary `.zshrc` content and break the core round-trip guarantee:

1. `Regenerate` indexes `e.Names[0]` for `KindAssignment` and `KindAlias` with no length check; a bare `export`, `export -p`, `readonly`, or a bare `alias`/`alias x` (alias-print query) routes managed and **panics** — violating the project's "never panics in production" invariant.
2. The parser collapses a multi-assignment / multi-alias statement into a single `Names`/`Value` pair (last value wins), so `export FOO=bar BAZ=qux` regenerates as `export FOO=qux` — dropping `BAZ` and corrupting `FOO`. This is silent data loss across the round-trip (D-08 violation).

Two further behavior-change bugs (the `+=` append operator and the `-g` global-alias flag are dropped on regen) round out the correctness concerns.

## Critical Issues

### BL-01: `Regenerate` panics on a managed assignment/alias with empty `Names`

**File:** `core/shell/zsh/regen.go:28,30,32`
**Issue:** `Regenerate` unconditionally reads `e.Names[0]` for `KindAssignment` (lines 28, 30) and `KindAlias` (line 32). The router (`core/ir/route.go:26-31`) admits `KindAlias` unconditionally and `KindAssignment` whenever the category is Env/Path/Secrets — **neither path requires `len(Names) > 0`** (only the `setopt`/`unsetopt` arm has that guard). The parser produces empty-`Names` blocks of exactly these kinds for ordinary input:

- `export` (bare), `export -p`, `readonly` → `KindAssignment`, `Names=[]`, classified `CatEnvironment` → routes managed.
- `alias` (bare), `alias x` (alias-print query, no `=`) → `KindAlias`, `Names=[]` → routes managed.

Confirmed by direct execution through `ir.Build` → `ir.Regenerate`: `panic: runtime error: index out of range [0] with length 0`. A `forced-managed` (`OverrideManaged`) entry of either kind with empty Names panics the same way. This violates the hard CLAUDE.md invariant "Errors are returned, never panicked in production code" and crashes the entire regeneration path on a benign config line.

**Fix:** Guard the index in `Regenerate` and fall through to verbatim `Text` (the documented total default) when there is nothing to template:
```go
case model.KindAssignment:
    if len(e.Names) == 0 {
        return e.Text
    }
    if e.Exported {
        return fmt.Sprintf("export %s=%s", e.Names[0], e.Value)
    }
    return fmt.Sprintf("%s=%s", e.Names[0], e.Value)
case model.KindAlias:
    if len(e.Names) == 0 {
        return e.Text
    }
    return fmt.Sprintf("alias %s=%s", e.Names[0], e.Value)
```
Belt-and-suspenders: also add `len(b.Names) > 0` to the `KindAssignment` and `KindAlias` arms of `routeManaged` so a no-name statement never routes managed in the first place.

### BL-02: Multi-assignment / multi-alias statement is corrupted on round-trip (silent data loss)

**File:** `core/shell/zsh/parse.go:70-78,86-102,106-114,151-159` (capture) and `core/shell/zsh/regen.go:26-32` (emit)
**Issue:** When one statement declares multiple names, the parser appends every name to `b.Names` but overwrites `b.Value` on each iteration, so only the **last** value survives. `Regenerate` then emits only `Names[0]=Value`, pairing the first name with the last value and dropping the rest. Confirmed:

| source | regenerated | wrong because |
|---|---|---|
| `export FOO=bar BAZ=qux` | `export FOO=qux` | `BAZ` dropped; `FOO` value corrupted (`bar`→`qux`) |
| `FOO=bar BAZ=qux` | `FOO=qux` | same |
| `alias a=1 b=2` | `alias a=2` | `b` dropped; `a` value corrupted |

These are all admitted declarative classes (Env assignment, alias), so they go through the templater and break the byte-/behavior-equivalence guarantee (D-08). This is silent data loss of user config — strictly worse than a panic because it ships a wrong-but-plausible regenerated profile.

**Fix:** Capture per-name values (e.g. a parallel `Values []string` or a `[]struct{Name, Value string}` on the block/entry) and have `Regenerate` emit all pairs, or — if the IR intends one Entry per assignment — split a multi-assign statement into multiple Entries during `describe`. Until the model supports multiple name=value pairs, the safest interim fix is to detect `len(c.Assigns) > 1` (or multiple alias args) and route the statement imperative (emit `Text` verbatim) so nothing is corrupted. Add a regression test with `export FOO=bar BAZ=qux` driven through the full `Parse → Build → Regenerate` path.

## Warnings

### WR-01: `+=` append assignment is silently rewritten to `=` (semantic change)

**File:** `core/shell/zsh/parse.go:74-76`; `core/shell/zsh/regen.go:28-30`
**Issue:** `FOO+=bar` parses to `Names=["FOO"]`, `Value="bar"` — the `+=` append operator is not captured. `Regenerate` emits `FOO=bar`, turning an append into an overwrite. For a managed env/path entry this changes runtime behavior across the round-trip (e.g. `PATH+=:/x` would clobber `PATH`). The `Assign.Append` bool on the mvdan/sh node is available but unread.
**Fix:** Capture `a.Append` (e.g. an `Append bool` on the block/entry) and emit `%s+=%s` when set. Until then, treat `Append` assignments as imperative (verbatim) to avoid the semantic change.

### WR-02: `alias -g`/`-s` flags dropped on regen (global/suffix alias downgraded to regular)

**File:** `core/shell/zsh/parse.go:84-102`; `core/shell/zsh/regen.go:31-32`
**Issue:** `alias -g G='| grep'` parses to `Names=["G"]`, `Value="'| grep'"`, dropping the `-g` flag. `Regenerate` emits `alias G='| grep'`, which defines a *regular* alias instead of a *global* one — different expansion semantics, so the round-trip is not behavior-equivalent for global/suffix aliases. The `-g`/`-s` flag words are skipped during name extraction and never preserved.
**Fix:** Capture the alias-type flag and re-emit it, or route flagged aliases imperative (verbatim) so their flags survive.

### WR-03: `Build` shares the parser's `Names` slice header into the Entry (aliasing)

**File:** `core/ir/build.go:32` (`Names: b.Names`)
**Issue:** `Build` copies the slice header `b.Names` straight into the new `Entry`, so the `Entry` and the source `model.Block` share the same backing array. The `model.Analysis` contract is "read-only by convention," but nothing enforces it; if any future consumer appends to or mutates an `Entry.Names` (or the originating `Block.Names`), the other mutates too — a latent aliasing bug that will be very hard to trace. Same applies to the round-trip oracle, which builds the Profile from the same blocks it does not otherwise touch.
**Fix:** Defensive-copy the slice: `Names: append([]string(nil), b.Names...)`. Low cost, removes the shared-mutation footgun at the IR boundary.

### WR-04: Round-trip oracle ignores the `Parse` error and only exercises one fixture

**File:** `core/ir/roundtrip_test.go:45`
**Issue:** `blocks, _ := p.Parse(src)` discards the error. `Parse` is documented never to return one today, but silently dropping it means a future regression (e.g. Parse starts erroring) would surface as a confusing downstream failure rather than a clear test signal. More importantly, the single hand-built fixture deliberately avoids the exact shapes that break in BL-01/BL-02 (bare export, multi-assign), so the oracle passes while the regenerator is broken — the test gives false confidence. The phase's own "byte-identical round-trip" claim is not actually exercised against adversarial inputs.
**Fix:** Assert `err == nil` explicitly, and extend the fixture (or add a table-driven oracle case) to include `export A=1 B=2`, a bare `export`/`alias`, an `alias -g`, and a `PATH+=` append so the round-trip gate covers the failure modes above.

## Info

### IN-01: `setopt`/`unsetopt` with empty Names emits a trailing space if ever forced managed

**File:** `core/shell/zsh/regen.go:38-39`
**Issue:** `fmt.Sprintf("%s %s", e.CmdName, strings.Join(e.Names, " "))` with empty `Names` produces `"setopt "` (trailing space). The auto router blocks this via the `len(b.Names) > 0` guard, but a `forced-managed` bare `setopt` would reach it and emit a dangling space. Cosmetic / harmless to zsh, but inconsistent with the empty-Names handling recommended in BL-01.
**Fix:** Return `e.Text` when `len(e.Names) == 0` in the setopt arm, consistent with the other kinds.

### IN-02: `Entry.Override` doc says "defaults OverrideAuto" but the zero value is `""`

**File:** `core/model/profile.go:33`; `core/ir/build.go:36`
**Issue:** The field comment implies the default is `OverrideAuto`, but the Go zero value of `ManagedOverride` is `""`, not `"auto"`. `EffectiveManaged`'s `default:` case treats both identically (correct, and `TestEffectiveManaged` pins the `""` case), and `Build` explicitly sets `OverrideAuto`. So it is currently safe, but any code that compares `e.Override == model.OverrideAuto` on a zero-value `Entry` (not built via `Build`) would get a surprising `false`. Worth a one-line clarification or making the comparison robust.
**Fix:** Either note in the doc that `""` is also auto (matching the test), or have `EffectiveManaged` and any future override-aware code treat `""` and `OverrideAuto` explicitly the same (it already does in `EffectiveManaged`; the risk is elsewhere).

---

_Reviewed: 2026-06-27_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
