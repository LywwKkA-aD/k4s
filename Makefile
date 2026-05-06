GO   ?= go
BIN  ?= bin/k4s
PKGS := ./...

KUBECONFIG_PATH := $(CURDIR)/.kube/config

.DEFAULT_GOAL := help

.PHONY: help build run test tidy lint clean k3s-up k3s-down kubeconfig seed seed-down demo

help: ## show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build the k4s binary into bin/
	@mkdir -p bin
	$(GO) build -o $(BIN) ./cmd/k4s

run: ## run k4s without producing a build artifact
	$(GO) run ./cmd/k4s

test: ## run tests with race detector and coverage
	$(GO) test -race -cover $(PKGS)

tidy: ## sync go.mod / go.sum
	$(GO) mod tidy

lint: ## run golangci-lint (must be installed)
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed: https://golangci-lint.run"; exit 1; }
	golangci-lint run $(PKGS)

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

k3s-down: ## stop local k3s
	docker compose down

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
