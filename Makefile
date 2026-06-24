# zsh-pro developer tasks.
# `lint` uses golangci-lint, installed separately (e.g. `brew install golangci-lint`) —
# intentionally not a go.mod dependency. Run `make hooks` once to enable the pre-commit gate.

GOTOOLCHAIN ?= auto
export GOTOOLCHAIN

.PHONY: all build test vet fmt fmt-check lint check hooks setup

all: check

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt needed in:"; echo "$$out"; exit 1; \
	fi

lint:
	golangci-lint run

check: fmt-check vet lint test

# Enable the repo-tracked git hooks (pre-commit runs golangci-lint on staged Go changes).
hooks setup:
	git config core.hooksPath .githooks
	@echo "Git hooks enabled (core.hooksPath=.githooks)."
