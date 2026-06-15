# gordma — Makefile
#
# Targets fall into two groups:
#   - hardware-independent (vet, build, test, stub builds): run anywhere,
#     including macOS, and mirror what CI enforces.
#   - hardware-dependent (integration): require a real RDMA NIC on Linux with
#     libibverbs/librdmacm; these are opt-in and never run by default.

# Strip any directory path from the module so binaries are named go_send_bw etc.
MODULE      := github.com/smallnest/gordma
BIN_DIR     := bin
CMDS        := $(notdir $(wildcard cmd/*))
GO          ?= go
GOFLAGS     ?=

# Cross-compile targets exercised for the non-Linux / no-cgo stub build.
STUB_TARGETS := darwin/amd64 darwin/arm64 windows/amd64

.DEFAULT_GOAL := all

.PHONY: all
all: vet build test ## Run vet, build, and tests (default)

.PHONY: build
build: ## Build all packages (cgo on Linux, stub elsewhere)
	$(GO) build $(GOFLAGS) ./...

.PHONY: tools
tools: $(BIN_DIR) ## Build the six perftest CLIs into bin/
	@for cmd in $(CMDS); do \
		echo "  building $$cmd"; \
		$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$$cmd ./cmd/$$cmd || exit 1; \
	done

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: test
test: ## Run hardware-independent unit tests with the race detector
	$(GO) test -race ./...

.PHONY: cover
cover: ## Run tests and write a coverage profile (coverage.out)
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

.PHONY: fmt
fmt: ## Format all Go source with gofmt
	gofmt -w $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is not gofmt-clean
	@unformatted=$$(gofmt -l $(shell find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: lint
lint: ## Run golangci-lint (install if missing); falls back to go vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found; install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "running 'go vet' as a fallback for now"; \
		$(GO) vet ./...; \
	fi

.PHONY: stub
stub: ## Build the stub (CGO_ENABLED=0) on the host platform
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) ./...

.PHONY: cross
cross: ## Cross-compile the stub for darwin/windows targets
	@for t in $(STUB_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "  building $$os/$$arch (stub)"; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) ./... || exit 1; \
	done

.PHONY: integration
integration: ## Hardware integration tests (needs RDMA NIC; set GORDMA_HW=1)
ifneq ($(GORDMA_HW),1)
	@echo "integration tests require a real RDMA device."
	@echo "set GORDMA_HW=1 to run them: make integration GORDMA_HW=1"
	@exit 1
else
	$(GO) test -tags=integration -race ./...
endif

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
