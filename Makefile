.PHONY: all build test test-unit test-integration test-e2e test-coverage lint clean install fmt vet

# Build configuration
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X main.version=$(VERSION) \
           -X main.commit=$(COMMIT) \
           -X main.buildDate=$(BUILD_DATE)

# Go configuration
GOBIN ?= $(shell go env GOPATH)/bin
GOFMT ?= gofmt
GOVET ?= go vet
GOLINT ?= $(GOBIN)/golangci-lint

# Output directory
BIN_DIR := bin
BINARY := $(BIN_DIR)/bootc-man
# Workspace root bin directory (for AI agent builds)
WS_BIN_DIR := ../bin

all: build

# Build the binary
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/bootc-man
	@echo "Build complete: $(BINARY)"
	@if [ -d "$(WS_BIN_DIR)" ] || [ -d "../bin" ]; then \
		mkdir -p $(WS_BIN_DIR); \
		cp $(BINARY) $(WS_BIN_DIR)/; \
		echo "Copied to workspace bin: $(WS_BIN_DIR)/bootc-man"; \
	fi

# Run all tests (unit tests only, short mode)
test:
	go test -v -race -cover ./...

# Run unit tests only (fast, no external dependencies)
test-unit:
	go test -v -short -race ./...

# Run integration tests (requires Podman)
test-integration:
	go test -v -race -tags=integration ./...

# Run E2E tests (requires VM infrastructure)
# Tests are split into phases because Go runs tests alphabetically by file,
# and Bootc tests (bootc_test.go) must run AFTER VM boot (vm_test.go).
# macOS: TMPDIR must be under $HOME so Podman Machine (libkrun) can access it.
# Linux: TMPDIR=/tmp is fine (no Podman Machine layer).
E2E_TIMEOUT := 60m
ifeq ($(shell uname),Darwin)
E2E_TMPDIR := $(HOME)/.local/share/bootc-man/tmp
else
E2E_TMPDIR := /tmp
endif
GO_TEST_E2E = TMPDIR=$(E2E_TMPDIR) go test -v -tags=e2e -count=1

test-e2e:
ifeq ($(shell uname),Darwin)
	@mkdir -p $(E2E_TMPDIR)
endif
	@echo "=== Phase 1: Non-VM tests ==="
	$(GO_TEST_E2E) -timeout=$(E2E_TIMEOUT) -run '^Test(Container|Registry|CI|E2E|Podman|BootcMan)' ./test/e2e/...
	@echo ""
	@echo "=== Phase 2: VM boot tests ==="
	$(GO_TEST_E2E) -timeout=$(E2E_TIMEOUT) -run '^TestVM(Boot|SSHConnection|InfrastructureAvailability|List|Status)' ./test/e2e/...
	@echo ""
	@echo "=== Phase 3: Bootc tests (requires running VM) ==="
	$(GO_TEST_E2E) -timeout=$(E2E_TIMEOUT) -run '^TestBootc(Status|StatusJSON|Upgrade|Switch|Rollback)' ./test/e2e/...
	@echo ""
	@echo "=== Phase 4: VM cleanup ==="
	-$(GO_TEST_E2E) -timeout=5m -run '^TestVMCleanup' ./test/e2e/...

# Run tests with coverage report
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run tests with coverage (CI mode - no HTML)
test-coverage-ci:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Run benchmark tests
test-bench:
	go test -v -bench=. -benchmem ./...

# Format code
fmt:
	$(GOFMT) -w -s .

# Vet code
vet:
	$(GOVET) ./...

# Run linter
lint:
	@if [ ! -f $(GOLINT) ]; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	$(GOLINT) run

# Clean build artifacts
clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

# Install the binary
install: build
	install -m 755 $(BINARY) /usr/local/bin/

# Install for development (into GOBIN)
install-dev: build
	install -m 755 $(BINARY) $(GOBIN)/

# Download dependencies
deps:
	go mod download
	go mod tidy

# Generate mocks (if needed)
generate:
	go generate ./...

# Build for multiple platforms
build-all: build-linux build-darwin

build-linux:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/bootc-man-linux-amd64 ./cmd/bootc-man
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/bootc-man-linux-arm64 ./cmd/bootc-man

build-darwin:
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/bootc-man-darwin-arm64 ./cmd/bootc-man

# RPM packaging (requires rpmbuild, rpmdevtools)
rpm: build
	@echo "Building RPM package..."
	rpmdev-setuptree
	git archive --format=tar.gz --prefix=$(BINARY_NAME)-$(VERSION)/ HEAD > ~/rpmbuild/SOURCES/$(BINARY_NAME)-$(VERSION).tar.gz
	cp rpm/$(BINARY_NAME).spec ~/rpmbuild/SPECS/
	rpmbuild -bb ~/rpmbuild/SPECS/$(BINARY_NAME).spec


# Validate (lint + vet + test)
validate: lint vet test-unit

# CI validation (used in CI pipelines)
ci: validate test-coverage-ci

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build:"
	@echo "  build          - Build the binary"
	@echo "  build-all      - Build for Linux and macOS"
	@echo ""
	@echo "Test:"
	@echo "  test           - Run all tests"
	@echo "  test-unit      - Run unit tests only (fast)"
	@echo "  test-integration - Run integration tests (requires Podman)"
	@echo "  test-e2e       - Run E2E tests (requires VM)"
	@echo "  test-coverage  - Run tests with HTML coverage report"
	@echo "  test-bench     - Run benchmark tests"
	@echo ""
	@echo "Quality:"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  vet            - Vet code"
	@echo "  validate       - Run lint, vet, and unit tests"
	@echo "  ci             - Run CI validation"
	@echo ""
	@echo "Other:"
	@echo "  clean          - Clean build artifacts"
	@echo "  install        - Install to /usr/local/bin"
	@echo "  install-dev    - Install to GOBIN"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  rpm            - Build RPM package"
