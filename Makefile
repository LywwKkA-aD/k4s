# k4s Makefile
# Build automation for Kubernetes TUI

# Application info
APP_NAME := k4s
MAIN_PKG := ./cmd/k4s

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build settings
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w \
	-X 'main.Version=$(VERSION)' \
	-X 'main.Commit=$(COMMIT)' \
	-X 'main.BuildDate=$(BUILD_DATE)'

# Output directories
BUILD_DIR := build
DIST_DIR := dist

# Platforms for cross-compilation
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# Default target
.DEFAULT_GOAL := build

# Phony targets
.PHONY: all build clean test lint fmt vet tidy run help install uninstall release

## help: Show this help message
help:
	@echo "k4s - Kubernetes TUI for K3s"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build for current platform
build:
	@echo "Building $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)"

## build-all: Build for all platforms
build-all: clean
	@echo "Building $(APP_NAME) $(VERSION) for all platforms..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(DIST_DIR)/$(APP_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output=$$output.exe; fi; \
		echo "  Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $$output $(MAIN_PKG); \
	done
	@echo "Done! Binaries in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

## install: Install to /usr/local/bin (may require sudo)
install: build
	@echo "Installing $(APP_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)
	sudo chmod 755 /usr/local/bin/$(APP_NAME)
	@echo "Installed to /usr/local/bin/$(APP_NAME)"

## uninstall: Remove from /usr/local/bin (may require sudo)
uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	sudo rm -f /usr/local/bin/$(APP_NAME)
	@echo "Done"

## run: Build and run
run: build
	./$(BUILD_DIR)/$(APP_NAME)

## test: Run tests
test:
	$(GO) test -v -race ./...

## test-cover: Run tests with coverage
test-cover:
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linters
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet only"; \
		$(GO) vet ./...; \
	fi

## fmt: Format code
fmt:
	$(GO) fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

## vet: Run go vet
vet:
	$(GO) vet ./...

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy
	$(GO) mod verify

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR) $(DIST_DIR)
	rm -f coverage.out coverage.html
	@echo "Done"

## release: Create release artifacts (requires VERSION to be set)
release: clean build-all
	@echo "Creating release archives..."
	@mkdir -p $(DIST_DIR)/release
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		binary=$(DIST_DIR)/$(APP_NAME)-$$os-$$arch; \
		archive=$(DIST_DIR)/release/$(APP_NAME)-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then \
			zip -j $$archive.zip $$binary.exe README.md LICENSE 2>/dev/null || \
			zip -j $$archive.zip $$binary.exe README.md; \
		else \
			tar -czf $$archive.tar.gz -C $(DIST_DIR) $(APP_NAME)-$$os-$$arch \
				-C $(CURDIR) README.md LICENSE 2>/dev/null || \
			tar -czf $$archive.tar.gz -C $(DIST_DIR) $(APP_NAME)-$$os-$$arch \
				-C $(CURDIR) README.md; \
		fi; \
	done
	@echo "Release archives in $(DIST_DIR)/release/"
	@ls -la $(DIST_DIR)/release/

## version: Show version info
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

## deps: Download dependencies
deps:
	$(GO) mod download

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"
