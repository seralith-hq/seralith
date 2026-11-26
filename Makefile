SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/seralith-hq/seralith
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT) -X $(PKG)/internal/version.Date=$(DATE)

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: bootstrap
bootstrap: ## Install web dependencies
	pnpm -C web install
	@if [ -f site/package.json ]; then pnpm -C site install; fi

.PHONY: web
web: ## Build the workbench UI into the embedded asset directory
	pnpm -C web build
	rm -rf internal/webui/dist && mkdir -p internal/webui/dist
	cp -R web/dist/. internal/webui/dist/
	touch internal/webui/dist/.gitkeep

.PHONY: site-docs
site-docs: ## Render docs/ and blog/ into the site (site/docs, site/blog)
	go run ./tools/sitedocs

.PHONY: demo
demo: build ## Re-render the terminal demos in docs/assets/demo
	@command -v vhs >/dev/null || { echo "demo: install vhs (https://github.com/charmbracelet/vhs) and ttyd"; exit 1; }
	@command -v ttyd >/dev/null || { echo "demo: ttyd is required (vhs uses it)"; exit 1; }
	PATH="$(CURDIR)/bin:$$PATH" vhs docs/assets/demo/authorship.tape
	PATH="$(CURDIR)/bin:$$PATH" vhs docs/assets/demo/gate.tape

.PHONY: build
build: ## Build seralith and seralith-control into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/seralith ./cmd/seralith
	@if [ -d ee/cmd/seralith-control ]; then CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/seralith-control ./ee/cmd/seralith-control; fi

.PHONY: release-build
release-build: web build ## Build the UI and both binaries

.PHONY: test
test: ## Run Go and web tests
	go test -race ./...
	@if [ -f web/package.json ]; then pnpm -C web test; fi

.PHONY: cover
cover: ## Generate an HTML coverage report
	go test -coverprofile=coverage.txt ./...
	go tool cover -html=coverage.txt -o coverage.html

.PHONY: lint
lint: ## Run linters
	go vet ./...
	golangci-lint run
	@if [ -f web/package.json ]; then pnpm -C web lint && pnpm -C web typecheck; fi

.PHONY: fmt
fmt: ## Format sources
	gofmt -s -w .
	@if [ -f web/package.json ]; then pnpm -C web format; fi

.PHONY: dev
dev: ## Run the daemon and the Vite dev server
	go run ./cmd/seralith serve --dev & pnpm -C web dev

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist coverage.txt coverage.html web/dist site/dist
