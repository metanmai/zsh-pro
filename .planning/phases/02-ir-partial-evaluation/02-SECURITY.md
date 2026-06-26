---
phase: 02-ir-partial-evaluation
audited: 2026-06-27
auditor: gsd-security-auditor
asvs_level: 1
block_on: high
register_authored_at_plan_time: true
threats_total: 7
threats_closed: 7
threats_open: 0
status: SECURED
---

# Phase 2: IR + Partial Evaluation — Security Audit

**Audit method:** FORCE stance — each declared mitigation treated as ABSENT until a grep/code/test match proved it present in the actual implementation on the `main` working tree. Documentation and SUMMARY/REVIEW claims were NOT accepted as evidence; every gate below was re-run independently.

**Verdict:** SECURED — 7/7 threats CLOSED (6 mitigate, 1 accept). No OPEN threats. No unregistered attack surface.

## Threat Register

Two plan-time threat models (02-01-PLAN T-02-01/02/03, 02-02-PLAN T-02-04/05/06; T-02-SC appears in both). All seven are unioned below.

| Threat ID | Category | Disposition | Status | Evidence |
|-----------|----------|-------------|--------|----------|
| T-02-01 | Denial of Service | mitigate | CLOSED | Parser degrades to a single `Opaque` block on parse error — `core/shell/zsh/parse.go:21-29` (returns `Block{Opaque:true}`, no error, no panic). The additive value/dynamic capture only reads an already-built AST (`describe` at `parse.go:64-204`), adds no recursion/unbounded loop. The BL-01 panic (bare `export`/`alias` → `Names[0]` index-out-of-range) found in code review is FIXED and the fix HOLDS: router `routeManaged` admits assignments/aliases only at `len(b.Names) == 1` (`core/ir/route.go:41,47`), and `Regenerate` has a belt-and-suspenders `if len(e.Names) == 0 { return e.Text }` guard for KindAssignment/KindAlias/setopt (`core/shell/zsh/regen.go:32,40,52`). Tests green on main: `TestParseOpaqueStaysReversibleSafe`, `TestParseOpaqueOnUnparseable`, `TestRegenerateNeverPanicsOnEmptyNames` (4 subcases), and `TestRouteManaged` (29 subcases incl. `bare_export_no_names`, `export_-p_no_names`, `bare_alias_no_names`, `bare_setopt_no_names`, `multi-name_*`, `append_assignment`, `flagged_alias`) all PASS. |
| T-02-02 | Elevation of Privilege (RCE-class) | mitigate | CLOSED | NO execution in the IR-build path. `wordIsDynamic` walks the AST only (`core/shell/zsh/parse.go:223-237` — `syntax.Walk`, returns a bool verdict, never resolves a value). `describe()` only offset-slices `src` via `sliceSrc` (`parse.go:208-216`). Grep gates empty on main: `grep -nE 'os/exec\|exec\.\|ExpandHome' core/shell/zsh/parse.go` → none; `grep -rn 'ExpandHome' core/ir core/model` → none. `$HOME`/`$(...)` stored verbatim into `Block.Value` (offset-slice, not resolved). |
| T-02-03 | Tampering (classification invariants) | mitigate | CLOSED | Additive change did not shift classification. Regression pins re-run green on main: `go test ./core/testgen/` (oracle property pin) and `go test ./core/analyze/` (corpus golden) both `ok`. `Block` extension is additive-only — existing fields (`Kind`/`Names`/`CmdName`/`Exported`/`Category`/`Conf`) unchanged (`core/model/block.go:27-41`, new fields `Value`/`Dynamic`/`Append`/`Flagged` appended at lines 37-40). |
| T-02-04 | Elevation of Privilege (command injection) | mitigate | CLOSED | The oracle sources user-derived content only inside the reused Phase-1 sandbox: `exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, buildinfo.Name, path)` with a 5s `context.WithTimeout` — `core/shell/zsh/introspect.go:43,46`. `zsh -f` = no rc/user init; `path` is passed as a SEPARATE argv element (`$1`), never string-interpolated into the script, so there is no injection vector through the call. The oracle test uses `t.TempDir()` (sandboxed throwaway). `zsh -f` flags + 5s timeout confirmed UNCHANGED on main. `TestRegenRoundTrip` RAN (not skipped, zsh present) and PASSED. |
| T-02-05 | Elevation of Privilege (RCE-class) | mitigate | CLOSED | The regeneration path performs NO resolution and NO execution. `ir.Regenerate` (`core/ir/regen.go:21-35`) only concatenates `e.Text` or delegates to `r.Regenerate(e)`; it emits no zsh syntax itself (`grep -nE 'fmt\.(Sprintf\|Fprintf).*(export\|alias\|setopt\|unsetopt )' core/ir/regen.go` → none). `zsh.Regenerate` emits `e.Value` VERBATIM (`core/shell/zsh/regen.go:36,38,43,47` — no `%q`, no resolution). `grep -rn 'ExpandHome' core/ir core/model` → none. `go list -deps zsh-pro/core/ir` shows only `core/model`, `core/shell`, `core/ir` — the concrete `core/shell/zsh` is NOT a compiled dependency (it appears only in `core/ir/roundtrip_test.go`, the external `ir_test` package). The sole execution is the sandboxed oracle subprocess (T-02-04). |
| T-02-06 | Tampering (test integrity / oracle determinism) | mitigate | CLOSED | The round-trip oracle fixture excludes non-deterministic completion machinery (`compinit`/`zstyle`/`autoload`/`compdef`/`zmodload`) — confirmed absent from `core/ir/roundtrip_test.go`. The introspect script sources at TOP LEVEL (`source "$1"` at `core/shell/zsh/introspect.go:26`, not wrapped in a function), avoiding the `LOCAL_OPTIONS` auto-revert trap so `setopt` sticks. `TestRegenRoundTrip` PASS on main with `reflect.DeepEqual` state-table comparison (Aliases/Functions/Env/Options/Path). |
| T-02-SC | Tampering (supply chain) | accept | CLOSED (accepted) | See Accepted Risks below. No package-manager installs this phase. `go.mod` direct require block: `mvdan.cc/sh/v3 v3.13.1` only (unchanged, pre-pinned). SUMMARY `tech-stack.added: []` for both plans. Accepted per plan; entry recorded here. |

## Accepted Risks Log

| Threat ID | Risk | Rationale | Accepted By |
|-----------|------|-----------|-------------|
| T-02-SC | Supply-chain tampering via package-manager installs (npm/pip/cargo) | No external packages added this phase. The single direct dependency `mvdan.cc/sh/v3 v3.13.1` is unchanged and pre-pinned in `go.mod`/`go.sum`; everything else is Go stdlib. No legitimacy checkpoint required for a phase that installs nothing. Verified: `grep '^require' go.mod` → only `mvdan.cc/sh/v3 v3.13.1`. | Plan-time threat model (02-01-PLAN.md / 02-02-PLAN.md), confirmed at audit. |

## Unregistered Flags

None. Neither 02-01-SUMMARY.md nor 02-02-SUMMARY.md declares a `## Threat Flags` section. The new attack surface introduced this phase (parser value capture; `core/ir` build/route/regen; the `shell.Regenerator` seam; the oracle subprocess) all maps cleanly onto the existing register (T-02-01..06). The two SUMMARY-declared deviations (setopt `Names` capture; `mockProvider.Regenerate` test-double) are functional, not new attack surface — the setopt capture is covered by T-02-01's `routeManaged` guards and the test double touches no production path.

## Audit Trail (gates re-run on `main`, not trusted from docs)

| Gate | Command | Result |
|------|---------|--------|
| Build | `GOTOOLCHAIN=auto go build ./...` | exit 0 |
| IR/zsh/model tests | `go test -count=1 ./core/ir/... ./core/shell/zsh/... ./core/model/...` | ok (all) |
| T-02-03 regression pins | `go test -count=1 ./core/testgen/ ./core/analyze/` | ok (both) |
| T-02-06 oracle | `go test -count=1 -v -run TestRegenRoundTrip ./core/ir/` | `--- PASS` (RAN, not skipped; zsh present) |
| T-02-01 no-panic | `go test -v -run 'Panic\|Opaque\|RouteManaged' ./core/shell/zsh/ ./core/ir/` | all PASS (incl. 29 routeManaged subcases) |
| T-02-02/05 no-exec | `grep -nE 'os/exec\|exec\.\|ExpandHome' core/shell/zsh/parse.go`; `grep -rn 'ExpandHome' core/ir core/model` | empty |
| T-02-05 no codegen in ir | `grep -nE 'fmt\.(Sprintf\|Fprintf).*(export\|alias\|setopt\|unsetopt )' core/ir/regen.go` | empty |
| T-02-05 composition-root boundary | `go list -deps zsh-pro/core/ir` | only `core/model` + `core/shell` (+ self); `core/shell/zsh` only in `roundtrip_test.go` |
| T-02-SC supply chain | `grep '^require' go.mod` | `mvdan.cc/sh/v3 v3.13.1` only |

Implementation files were NOT modified during this audit (read-only). Only this `02-SECURITY.md` was written.

---
_Audited: 2026-06-27 — gsd-security-auditor — ASVS L1, block_on: high_
