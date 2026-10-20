GO   ?= go
TOOL := $(GO) tool -modfile=tools/go.mod
BIN  ?= bin/k4s
PKGS := ./...

KUBECONFIG_PATH := $(CURDIR)/.kube/config

# Tool targets run on the toolchain pinned by go.mod (toolchain line, else the
# go directive): under the default GOTOOLCHAIN=auto a newer local Go wins, and
# the pinned linter cannot read its export data. make only sees the shell env,
# so override with `GOTOOLCHAIN=local make lint`, not `go env -w`.
GO_TOOLCHAIN := $(shell awk '/^toolchain /{t=$$2} /^go /{g=$$2} END{if(t){print t}else{if(split(g,a,".")==2)g=g".0";print "go" g}}' go.mod)
GO_VERSION   := $(shell awk '/^go /{print $$2}' go.mod)
TOOLS_GO     := $(shell awk '/^go /{print $$2}' tools/go.mod)

fmt lint vuln tidy tidy-check ci: export GOTOOLCHAIN ?= $(GO_TOOLCHAIN)

.DEFAULT_GOAL := help

.PHONY: help build run test tidy tidy-check tools-go-check fmt lint vuln ci clean k3s-up k3s-down k3s-clean kubeconfig seed seed-down demo gif

help: ## show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build the k4s binary into bin/
	@mkdir -p bin
	$(GO) build -o $(BIN) ./cmd/k4s

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

ci: ## full quality gate: tidy-check + vet + lint + test + vuln
	@$(MAKE) --no-print-directory tools-go-check
	$(GO) mod tidy -diff
	cd tools && $(GO) mod tidy -diff
	$(GO) vet $(PKGS)
	$(TOOL) golangci-lint run $(PKGS)
	$(GO) test -race -cover $(PKGS)
	$(TOOL) govulncheck $(PKGS)
	@echo "all checks passed"

clean: ## remove build artifacts
	rm -rf bin

k3s-up: ## start local k3s in docker and write .kube/config
	docker compose up -d
	@echo "waiting for k3s kubeconfig..."
	@for i in $$(seq 1 60); do \
		[ -f .k3s/output/kubeconfig.yaml ] && break; \
		sleep 1; \
	done
	@$(MAKE) --no-print-directory kubeconfig

k3s-down: ## stop local k3s (preserves volume)
	docker compose down

k3s-clean: ## stop k3s, drop the persistent volume, and forget kubeconfig
	docker compose down -v
	rm -rf .k3s .kube
	@echo "k3s container + volume + local kubeconfig wiped"

kubeconfig: ## rewrite the k3s kubeconfig with localhost server URL
	@mkdir -p .kube
	@sed 's#server: https://[^:]*:6443#server: https://127.0.0.1:6443#g' \
		.k3s/output/kubeconfig.yaml > .kube/config
	@chmod 600 .kube/config
	@echo "kubeconfig written to .kube/config"
	@echo "  export KUBECONFIG=$(KUBECONFIG_PATH)"

seed: ## apply demo workloads to the local k3s
	@command -v kubectl >/dev/null || { echo "kubectl not on PATH"; exit 1; }
	@KUBECONFIG=$(KUBECONFIG_PATH) kubectl apply -f deploy/seed/

seed-down: ## remove demo workloads
	@KUBECONFIG=$(KUBECONFIG_PATH) kubectl delete -f deploy/seed/ --ignore-not-found

demo: k3s-up seed ## bring up k3s and apply demo workloads
	@echo "demo cluster ready — export KUBECONFIG=$(KUBECONFIG_PATH)"

gif: build ## record docs/demo.gif via VHS (requires: brew install vhs ttyd ffmpeg)
	@command -v vhs >/dev/null || { echo "vhs not on PATH — brew install vhs ttyd ffmpeg"; exit 1; }
	KUBECONFIG=$(KUBECONFIG_PATH) vhs docs/demo.tape
	@echo "wrote docs/demo.gif"
