# golangci-lint runs pinned through Go: the agents' sandbox has nothing on PATH but go and git,
# and a go.mod tool directive would hand its dependencies to every importer of the library.
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help ci lint test test-integration fix build

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-18s %s\n", $$1, $$2}'

ci: lint test ## Non-mutating: what a pull request must pass

lint: ## golangci-lint (pinned)
	$(GOLANGCI_LINT) run ./...

test: ## Unit tests
	go test ./...

test-integration: ## Tests against a real Forgejo in podman (skips without podman)
	go test -tags integration -count=1 -v -timeout 10m ./internal/forgejotest/...

fix: ## Format, autofix lint, tidy go.mod
	$(GOLANGCI_LINT) fmt ./...
	$(GOLANGCI_LINT) run --fix ./...
	go mod tidy

build: ## bin/fgh, version from git describe
	go build -ldflags "-X main.version=$(VERSION)" -o bin/fgh ./cmd/fgh
