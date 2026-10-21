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
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

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

ci: ## full quality gate: tidy-check + line cap + vet + lint + test + idle-cpu lint/test + vuln
	@$(MAKE) --no-print-directory tools-go-check
	@$(MAKE) --no-print-directory lines
	$(GO) mod tidy -diff
	cd tools && $(GO) mod tidy -diff
	$(GO) vet $(PKGS)
	$(TOOL) golangci-lint run $(PKGS)
	$(GO) test -race -cover $(PKGS)
	@$(MAKE) --no-print-directory idle-cpu-lint idle-cpu-test
	$(TOOL) govulncheck $(PKGS)
	@echo "all checks passed"

# The tagged release is built in CI on the go.mod toolchain; this uses the local
# Go. The first run needs network to download GoReleaser.
snapshot: ## build every release archive into dist/ (GoReleaser, no publish)
	$(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --snapshot --clean

clean: ## remove build artifacts
	rm -rf bin dist

# Local dev cluster (docker compose, see docker-compose.yml). K3S_PROFILE is a
# comma list: "minimal" disables traefik, servicelb and metrics-server, and
# "agents" adds two agent nodes, e.g. `make demo K3S_PROFILE=minimal,agents`.
K3S_PROFILE    ?=
K3S_CONTAINER  := k4s-k3s
K3S_KUBECONFIG := .k3s/output/kubeconfig.yaml
k3s_comma      := ,
k3s_empty      :=
k3s_space      := $(k3s_empty) $(k3s_empty)
k3s_profiles   := $(subst $(k3s_comma),$(k3s_space),$(K3S_PROFILE))
k3s_unknown    := $(filter-out minimal agents,$(k3s_profiles))
# The k3s server config file, deploy/k3s/$(K3S_CONFIG).yaml.
K3S_CONFIG     := $(if $(filter minimal,$(k3s_profiles)),minimal,full)
K3S_COMPOSE     = K4S_K3S_CONFIG=$(K3S_CONFIG) COMPOSE_PROFILES=$(if $(filter agents,$(k3s_profiles)),agents) docker compose
# down and clean must reach the agents even when K3S_PROFILE is not set.
K3S_COMPOSE_ALL = COMPOSE_PROFILES=agents docker compose

# kubeconfigs that act as namespaced ServiceAccounts (deploy/seed/90-restricted-rbac.yaml).
SA_KUBECONFIG := deploy/fixtures/kubeconfig-sa.sh $(KUBECONFIG_PATH) k4s-demo

KIND_CLUSTER    ?= k4s
KIND_KUBECONFIG := $(CURDIR)/.kube/kind

.PHONY: k3s-check-profile k3s-ready kubeconfig-restricted kubeconfig-narrow kubeconfig-plugin kubeconfig-plugin-tty kind-up kind-down

k3s-check-profile:
	@if [ -n "$(k3s_unknown)" ]; then \
		echo "unknown K3S_PROFILE value(s): $(k3s_unknown) (want minimal, agents)"; exit 1; \
	fi

k3s-up: k3s-check-profile ## start local k3s and write .kube/config (K3S_PROFILE=minimal,agents)
	@mkdir -p $(dir $(K3S_KUBECONFIG))
	$(K3S_COMPOSE) up -d
	@$(MAKE) --no-print-directory kubeconfig
	@deploy/dev/k3s-prune-agents.sh $(KUBECONFIG_PATH)

k3s-ready: ## wait until every node and bundled k3s component is Ready
	@deploy/dev/k3s-ready.sh $(KUBECONFIG_PATH) $(K3S_CONTAINER)

k3s-down: ## stop local k3s and its agents (preserves volumes)
	$(K3S_COMPOSE_ALL) down

k3s-clean: ## stop k3s, drop the persistent volumes, and forget kubeconfigs
	$(K3S_COMPOSE_ALL) down -v
	rm -rf .k3s .kube
	@echo "k3s containers + volumes + local kubeconfigs wiped"

kubeconfig: ## write .kube/config once k3s has a fresh kubeconfig and a live API
	@deploy/dev/k3s-wait.sh $(K3S_CONTAINER) $(K3S_KUBECONFIG) $(KUBECONFIG_PATH)

kubeconfig-restricted: ## write .kube/restricted: SA k4s-viewer (view + exec/port-forward in k4s-demo)
	@$(SA_KUBECONFIG) k4s-viewer $(CURDIR)/.kube/restricted token

kubeconfig-narrow: ## write .kube/narrow: SA k4s-narrow (pods only in k4s-demo)
	@$(SA_KUBECONFIG) k4s-narrow $(CURDIR)/.kube/narrow token

kubeconfig-plugin: ## write .kube/plugin: k4s-viewer through a slow exec credential plugin
	@$(SA_KUBECONFIG) k4s-viewer $(CURDIR)/.kube/plugin plugin

kubeconfig-plugin-tty: ## write .kube/plugin-tty: exec plugin that refuses to run without a TTY
	@$(SA_KUBECONFIG) k4s-viewer $(CURDIR)/.kube/plugin-tty plugin-tty

seed: ## apply demo workloads and RBAC fixtures to the local k3s
	@deploy/dev/seed.sh $(KUBECONFIG_PATH) deploy/seed

seed-down: ## remove demo workloads
	@KUBECONFIG=$(KUBECONFIG_PATH) kubectl delete -f deploy/seed/ --ignore-not-found

demo: ## bring up k3s, apply demo workloads, wait for every component
	@$(MAKE) --no-print-directory k3s-up
	@$(MAKE) --no-print-directory seed
	@$(MAKE) --no-print-directory k3s-ready
	@echo "demo cluster ready — export KUBECONFIG=$(KUBECONFIG_PATH)"

kind-up: ## optional vanilla cluster via kind, kubeconfig in .kube/kind
	@if ! command -v kind >/dev/null; then \
		echo "kind not on PATH, skipping (https://kind.sigs.k8s.io)"; exit 0; \
	fi; \
	mkdir -p .kube; \
	if kind get clusters 2>/dev/null | grep -qx '$(KIND_CLUSTER)'; then \
		kind export kubeconfig --name '$(KIND_CLUSTER)' --kubeconfig '$(KIND_KUBECONFIG)'; \
	else \
		kind create cluster --name '$(KIND_CLUSTER)' --kubeconfig '$(KIND_KUBECONFIG)' --wait 180s; \
	fi && \
	chmod 600 '$(KIND_KUBECONFIG)' && \
	echo "  export KUBECONFIG=$(KIND_KUBECONFIG)" && \
	echo "  seed it: make seed KUBECONFIG_PATH=$(KIND_KUBECONFIG)"

kind-down: ## delete the kind cluster
	@if ! command -v kind >/dev/null; then echo "kind not on PATH, nothing to do"; exit 0; fi; \
	kind delete cluster --name '$(KIND_CLUSTER)'
	@rm -f '$(KIND_KUBECONFIG)'

# Idle CPU and RSS of bin/k4s on the dashboard with watch on (the E6 budget),
# measured by tools/idlecpu on a 120x40 pty: 3 s warm-up, then a 40 s window.
# Run it on `make demo` without per-minute CronJob seeds: the report counts the
# pods that changed in the window, and --require-idle fails a busy run. E6 gate:
#   make idle-cpu IDLE_CPU_ARGS='--max-cpu 2 --max-rss 40MB --require-idle'
# For a Pi, cross-build both and run them on the node:
#   GOOS=linux GOARCH=arm64 make build idle-cpu-bin BIN=bin/arm64/k4s IDLE_CPU_BIN=bin/arm64/idlecpu
#   bin/arm64/idlecpu --bin bin/arm64/k4s --kubeconfig <file> --max-cpu 2 --max-rss 40MB
IDLE_CPU_BIN  ?= bin/idlecpu
IDLE_CPU_ARGS ?=

.PHONY: idle-cpu idle-cpu-bin idle-cpu-test idle-cpu-lint

# The release is built on the go.mod toolchain, so the measured k4s is too
# (target-specific exports reach the build prerequisite).
idle-cpu idle-cpu-bin idle-cpu-test idle-cpu-lint: export GOTOOLCHAIN ?= $(GO_TOOLCHAIN)

idle-cpu: build idle-cpu-bin ## idle CPU and RSS of bin/k4s on make demo (IDLE_CPU_ARGS='--max-cpu 2 --max-rss 40MB')
	$(IDLE_CPU_BIN) --bin $(BIN) --kubeconfig $(KUBECONFIG_PATH) $(IDLE_CPU_ARGS)

idle-cpu-bin: ## build the idle-cpu harness (tools/idlecpu); GOOS/GOARCH cross-build it
	@mkdir -p $(dir $(IDLE_CPU_BIN))
	cd tools && $(GO) build -o $(abspath $(IDLE_CPU_BIN)) ./idlecpu

idle-cpu-test: ## unit-test the idle-cpu harness (tools module, race detector)
	cd tools && $(GO) test -race -cover ./idlecpu/...

idle-cpu-lint: ## lint the idle-cpu harness (tools module, repo lint config)
	cd tools && $(GO) tool golangci-lint run --config ../.golangci.yml ./idlecpu/...

gif: build ## record docs/demo.gif via VHS (requires: brew install vhs ttyd ffmpeg)
	@command -v vhs >/dev/null || { echo "vhs not on PATH — brew install vhs ttyd ffmpeg"; exit 1; }
	KUBECONFIG=$(KUBECONFIG_PATH) vhs docs/demo.tape
	@echo "wrote docs/demo.gif"
