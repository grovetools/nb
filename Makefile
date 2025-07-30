# nb - Note System Makefile

# Build variables
BINARY_NAME=nb
BUILD_DIR=bin
GO=go
GOFLAGS=-tags "fts5"

# Default target
.PHONY: all
all: build

# Build the binary with FTS5 support
.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Install binary to /usr/local/bin
.PHONY: install
install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

# Run tests
.PHONY: test
test:
	$(GO) test $(GOFLAGS) ./...

# Run tests with verbose output
.PHONY: test-verbose
test-verbose:
	$(GO) test $(GOFLAGS) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests for a specific package
.PHONY: test-pkg
test-pkg:
	$(GO) test $(GOFLAGS) -v ./$(PKG)

# Run a specific test
.PHONY: test-run
test-run:
	$(GO) test $(GOFLAGS) -v -run $(TEST) ./...

# Run benchmarks
.PHONY: bench
bench:
	$(GO) test $(GOFLAGS) -bench=. -benchmem ./...

# Development build with race detector
.PHONY: dev
dev:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -race -o $(BUILD_DIR)/$(BINARY_NAME) .

# Lint the code
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Check if FTS5 is working
.PHONY: check-fts5
check-fts5: build
	$(BUILD_DIR)/$(BINARY_NAME) init --minimal
	@echo "FTS5 check passed!"

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make build          - Build nb with FTS5 support"
	@echo "  make install        - Build and install to /usr/local/bin"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-pkg PKG=pkg/service - Test specific package"
	@echo "  make test-run TEST=TestName   - Run specific test"
	@echo "  make bench          - Run benchmarks"
	@echo "  make dev            - Build with race detector"
	@echo "  make lint           - Run the linter"
	@echo "  make check-fts5     - Verify FTS5 support"