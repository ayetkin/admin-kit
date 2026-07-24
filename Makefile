# admin-kit - developer Makefile.
#
# The everyday commands:
#
#   make run    The example panel, which doubles as the living style guide.
#   make test   The Go test suite.
#   make lint   gofmt check, go vet, golangci-lint when installed.
#
# Adopting a new Tabler release is `make vendor` (see scripts/vendor-tabler.sh);
# the result is committed, so a build never reaches the network.

GO ?= go

# Pinned Tabler versions. Bump these, run `make vendor`, then commit the result.
TABLER_CORE  ?= 1.4.0
TABLER_ICONS ?= 3.45.0

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-9s\033[0m %s\n", $$1, $$2}'

## --- everyday ---

.PHONY: run
run: ## Run the example panel (http://localhost:8099/admin)
	@echo ""
	@echo "  example panel → http://localhost:8099/admin   (open, no OAuth needed)"
	@echo ""
	$(GO) run ./example

.PHONY: test
test: ## Run the tests with -race
	$(GO) test -race ./...

.PHONY: lint
lint: ## gofmt check, go vet, golangci-lint (if installed)
	@unformatted=$$(gofmt -l .); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	$(GO) vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found; install it or rely on CI"; \
	fi

## --- assets ---

.PHONY: vendor
vendor: ## Re-vendor Tabler at the pinned versions (commit the result)
	scripts/vendor-tabler.sh $(TABLER_CORE) $(TABLER_ICONS)

## --- housekeeping ---

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w .

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy
