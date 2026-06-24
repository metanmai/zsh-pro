---
quick_id: 260624-lsu
type: quick
completed: 2026-06-24
status: complete
commits: [338d184, 75fc6b7]
---

# Quick Task 260624-lsu: Add golangci-lint linter

## What was done
- Installed **golangci-lint v2.12.2** via Homebrew (standalone dev tool; `go.mod`/`go.sum` unchanged — still only `mvdan.cc/sh/v3`).
- `.golangci.yml` (v2): standard linters (errcheck, govet, ineffassign, staticcheck, unused) + gofmt formatter; `max-same-issues: 0` / `max-issues-per-linter: 0` so the gate never hides repeated findings.
- `Makefile`: `build`, `test`, `vet`, `fmt`, `fmt-check`, `lint`, `check` (fmt-check + vet + lint + test), `hooks`/`setup`. Uses `GOTOOLCHAIN=auto`.
- `.githooks/pre-commit`: runs `golangci-lint run` on staged Go changes only (doc/planning commits stay fast); degrades gracefully if golangci-lint is absent. Enabled via `make hooks` (sets `core.hooksPath=.githooks`).
- Fixed **8 errcheck findings** — unchecked `fmt.Fprint*` terminal writes in `core/cli/cli.go` (7) and `core/cmd/zsh-gen/main.go` (1) → `_, _ =` idiom (intentional ignore of unactionable terminal-write errors). No behavior or wire-contract change.
- Updated CLAUDE.md Code Style gate line (`go vet` minimum → golangci-lint + `make lint`/`make check`).

## Verification (evidence)
- `make check` green: gofmt clean, `go vet` clean, **golangci-lint 0 issues**, all test packages `ok`.
- Pre-commit hook fired **live** on the code-fix commit (75fc6b7): printed `0 issues.` and allowed the commit — end-to-end proof the gate works.
- `git diff go.mod go.sum` empty (single dependency preserved).
- `core.hooksPath` = `.githooks`; hook is executable and exits 0 (skips) when no Go files are staged.

## Decisions / notes
- **Standard** set chosen over strict: avoids gosec false-positives on the `zsh -f` subprocess exec + file reads, which would need `//nolint` noise.
- Fixed findings in code (`_, _ =`) rather than globally excluding `fmt.Fprint*` from errcheck — keeps the linter catching ignored errors on real file/network writers later.
- The 2 latent design warnings from `01-REVIEW.md` (oracle `RenderZsh`-order guard, generator `pick` bounds) are NOT flagged by the standard set; left as-is (out of "fix lint findings" scope).

## Follow-ups
- No CI (no git remote). When a remote is added, mirror `make check` in CI for "moving forward" enforcement beyond the local hook.

## Self-Check: PASSED
