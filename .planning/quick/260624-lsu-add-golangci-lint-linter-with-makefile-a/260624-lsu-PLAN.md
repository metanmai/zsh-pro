---
quick_id: 260624-lsu
type: quick
created: 2026-06-24
status: complete
slug: add-golangci-lint-linter-with-makefile-a
tags: [tooling, linter, golangci-lint, makefile, git-hook, go]
---

<objective>
Add golangci-lint as the project's Go linter so the codebase stays clean going
forward, with local enforcement and a green baseline.

User-locked decisions:
- Enforcement: Makefile + tracked pre-commit hook (no CI — no git remote yet).
- Strictness: Standard set (errcheck, govet, ineffassign, staticcheck, unused) + gofmt.
- Existing code: fix to a green baseline now.

Constraint: golangci-lint is a standalone dev tool, NOT a go.mod dependency
(preserve the single-runtime-dependency architecture — only mvdan.cc/sh/v3).
</objective>

<tasks>
1. Install golangci-lint v2 (brew) and add `.golangci.yml` (standard set + gofmt,
   uncapped issues). Verify go.mod/go.sum unchanged.
2. Add `Makefile` (build/test/vet/fmt/fmt-check/lint/check/hooks) and a tracked
   `.githooks/pre-commit` that runs golangci-lint on staged Go changes; wire via
   `make hooks` (core.hooksPath=.githooks).
3. Fix all golangci-lint findings to a green baseline; update the stale CLAUDE.md
   lint-gate line.
</tasks>

<verification>
- `make lint` -> 0 issues; `make check` -> green; `go test ./...` -> green.
- go.mod unchanged (only mvdan.cc/sh/v3).
- core.hooksPath = .githooks; hook fires on staged .go and passes.
</verification>
