GO   ?= go
TOOL := $(GO) tool -modfile=tools/go.mod
BIN  ?= bin/k4s
PKGS := ./...

KUBECONFIG_PATH := $(CURDIR)/.kube/config

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS := -X github.com/LywwKkA-aD/k4s/internal/version.Version=$(VERSION)

# Keep in sync with the goreleaser-action `version` in release.yml.
GORELEASER_VERSION ?= v2.18.3

# Tool targets run on the toolchain pinned by go.mod (toolchain line, else the
# go directive): under the default GOTOOLCHAIN=auto a newer local Go wins, and
# the pinned linter cannot read its export data. make only sees the shell env,
# so override with `GOTOOLCHAIN=local make lint`, not `go env -w`.
GO_TOOLCHAIN := $(shell awk '/^toolchain /{t=$$2} /^go /{g=$$2} END{if(t){print t}else{if(split(g,a,".")==2)g=g".0";print "go" g}}' go.mod)
GO_VERSION   := $(shell awk '/^go /{print $$2}' go.mod)
TOOLS_GO     := $(shell awk '/^go /{print $$2}' tools/go.mod)

fmt lint vuln tidy tidy-check ci: export GOTOOLCHAIN ?= $(GO_TOOLCHAIN)

.DEFAULT_GOAL := help

.PHONY: help build run test tidy tidy-check tools-go-check fmt lint vuln ci snapshot clean k3s-up k3s-down k3s-clean kubeconfig seed seed-down demo gif

help: ## show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build bin/k4s, version stamped from git describe
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/k4s

run: ## run k4s without producing a build artifact
	$(GO) run ./cmd/k4s

test: ## run tests with race detector and coverage
	$(GO) test -race -cover $(PKGS)

tidy: ## sync go.mod / go.sum (app and tools modules)
	$(GO) mod tidy
	cd tools && $(GO) mod tidy

tidy-check: ## fail if go.mod / go.sum are not tidy (app and tools modules)
	@$(MAKE) --no-print-directory tools-go-check
	$(GO) mod tidy -diff
	cd tools && $(GO) mod tidy -diff

# CI installs only go.mod's version, so a tools bump that raises tools/go.mod's
# go line must raise go.mod's too.
tools-go-check:
	@if [ "$$(printf '%s\n%s\n' '$(GO_VERSION)' '$(TOOLS_GO)' | sort -V | tail -1)" != "$(GO_VERSION)" ]; then \
		echo "tools/go.mod requires go $(TOOLS_GO) but go.mod has go $(GO_VERSION): raise the go directive in go.mod"; \
		exit 1; \
	fi

fmt: ## auto-format the code (gofmt + goimports via golangci-lint)
	$(TOOL) golangci-lint fmt

lint: ## run golangci-lint (correctness, security, style)
	$(TOOL) golangci-lint run $(PKGS)

vuln: ## scan dependencies for known vulnerabilities (govulncheck)
	$(TOOL) govulncheck $(PKGS)

MAX_LINES     ?= 800
COVER_PROFILE ?= coverage.out
MODULE        := $(shell awk '/^module /{print $$2}' go.mod)

.PHONY: lines cover

lines: ## fail if a non-test .go file is over MAX_LINES lines (default 800)
	@over=$$(find . -path './.*' -prune -o -type f -name '*.go' ! -name '*_test.go' ! -path '*/testdata/*' -exec wc -l {} + \
		| awk -v max=$(MAX_LINES) '$$2 != "total" && $$1 > max { sub(/^\.\//, "", $$2); printf "  %6d  %s\n", $$1, $$2 }' \
		| sort -rn); \
	if [ -n "$$over" ]; then \
		echo "non-test .go files over $(MAX_LINES) lines:"; \
		echo "$$over"; \
		exit 1; \
	fi; \
	echo "no non-test .go file is over $(MAX_LINES) lines"

cover: ## print per-package test coverage, lowest last (profile: COVER_PROFILE)
	@out=$$($(GO) test -race -coverprofile=$(COVER_PROFILE) $(PKGS) 2>&1) || { echo "$$out"; exit 1; }
	@printf '%-36s %8s\n' PACKAGE COVERAGE
	@awk -v mod='$(MODULE)/' ' \
		NR == 1 { next } \
		{ \
			split($$1, loc, ":"); pkg = loc[1]; sub(/\/[^\/]*$$/, "", pkg); \
			if (index(pkg, mod) == 1) pkg = substr(pkg, length(mod) + 1); \
			if (!($$1 in seen)) { seen[$$1] = 1; stmts[pkg] += $$2 } \
			if ($$3 > 0 && !($$1 in hit)) { hit[$$1] = 1; covered[pkg] += $$2 } \
		} \
		END { for (p in stmts) printf "%s %.1f\n", p, (stmts[p] ? 100 * covered[p] / stmts[p] : 0) }' \
		$(COVER_PROFILE) | sort -k2,2nr -k1,1 | awk '{ printf "%-36s %7s%%\n", $$1, $$2 }'
	@$(GO) tool cover -func=$(COVER_PROFILE) | awk 'END { printf "%-36s %8s\n", "total", $$NF }'

ci: ## full quality gate: tidy-check + vet + lint + test + vuln
	@$(MAKE) --no-print-directory tools-go-check
	$(GO) mod tidy -diff
	cd tools && $(GO) mod tidy -diff
	$(GO) vet $(PKGS)
	$(TOOL) golangci-lint run $(PKGS)
	$(GO) test -race -cover $(PKGS)
	$(TOOL) govulncheck $(PKGS)
	@echo "all checks passed"

# The tagged release is built in CI on the go.mod toolchain; this uses the local
# Go. The first run needs network to download GoReleaser.
snapshot: ## build every release archive into dist/ (GoReleaser, no publish)
	$(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --snapshot --clean

clean: ## remove build artifacts
	rm -rf bin dist

K3S_CONTAINER  := k4s-k3s
K3S_KUBECONFIG := .k3s/output/kubeconfig.yaml

k3s-up: ## start local k3s in docker and write .kube/config
	@mkdir -p $(dir $(K3S_KUBECONFIG))
	docker compose up -d
	@$(MAKE) --no-print-directory kubeconfig

k3s-down: ## stop local k3s (preserves volume)
	docker compose down

k3s-clean: ## stop k3s, drop the persistent volume, and forget kubeconfig
	docker compose down -v
	rm -rf .k3s .kube
	@echo "k3s container + volume + local kubeconfig wiped"

kubeconfig: ## write .kube/config once k3s has a fresh kubeconfig and a live API
	@deploy/dev/k3s-wait.sh $(K3S_CONTAINER) $(K3S_KUBECONFIG) $(KUBECONFIG_PATH)

seed: ## apply demo workloads to the local k3s
	@deploy/dev/seed.sh $(KUBECONFIG_PATH) deploy/seed

seed-down: ## remove demo workloads
	@KUBECONFIG=$(KUBECONFIG_PATH) kubectl delete -f deploy/seed/ --ignore-not-found

demo: k3s-up seed ## bring up k3s and apply demo workloads
	@echo "demo cluster ready — export KUBECONFIG=$(KUBECONFIG_PATH)"

gif: build ## record docs/demo.gif via VHS (requires: brew install vhs ttyd ffmpeg)
	@command -v vhs >/dev/null || { echo "vhs not on PATH — brew install vhs ttyd ffmpeg"; exit 1; }
	KUBECONFIG=$(KUBECONFIG_PATH) vhs docs/demo.tape
	@echo "wrote docs/demo.gif"
