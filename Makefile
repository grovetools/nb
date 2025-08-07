# nb - Note System Makefile

# Build variables
BINARY_NAME=nb
BUILD_DIR=bin
GO=go
GOFLAGS=-tags "fts5"

# Default target
.PHONY: all build test clean fmt vet lint run check dev build-all help
all: build

# Build the binary with FTS5 support
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GO) test $(GOFLAGS) ./...

# Run tests with verbose output
test-verbose:
	$(GO) test $(GOFLAGS) -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests for a specific package
test-pkg:
	$(GO) test $(GOFLAGS) -v ./$(PKG)

# Run a specific test
test-run:
	$(GO) test $(GOFLAGS) -v -run $(TEST) ./...

# Run benchmarks
bench:
	$(GO) test $(GOFLAGS) -bench=. -benchmem ./...

# Development build with race detector
dev:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -race -o $(BUILD_DIR)/$(BINARY_NAME) .

# Lint the code
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Check if FTS5 is working
check-fts5: build
	$(BUILD_DIR)/$(BINARY_NAME) init --minimal
	@echo "FTS5 check passed!"

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build nb with FTS5 support"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-pkg PKG=pkg/service - Test specific package"
	@echo "  make test-run TEST=TestName   - Run specific test"
	@echo "  make bench          - Run benchmarks"
	@echo "  make dev            - Build with race detector"
	@echo "  make lint           - Run the linter"
