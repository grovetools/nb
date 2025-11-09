# nb - Note System Makefile

# Build variables
BINARY_NAME=nb
BUILD_DIR=bin
GO=go
VERSION_PKG=github.com/mattsolo1/grove-core/version

# --- Versioning ---
# For dev builds, we construct a version string from git info.
# For release builds, VERSION is passed in by the CI/CD pipeline (e.g., VERSION=v1.2.3)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
GIT_DIRTY  ?= $(shell test -n "`git status --porcelain`" && echo "-dirty")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# If VERSION is not set, default to a dev version string
VERSION ?= $(GIT_BRANCH)-$(GIT_COMMIT)$(GIT_DIRTY)

# Go LDFLAGS to inject version info at compile time
LDFLAGS = -ldflags="\
-X '$(VERSION_PKG).Version=$(VERSION)' \
-X '$(VERSION_PKG).Commit=$(GIT_COMMIT)' \
-X '$(VERSION_PKG).Branch=$(GIT_BRANCH)' \
-X '$(VERSION_PKG).BuildDate=$(BUILD_DATE)'"

# Default target
.PHONY: all build test clean fmt vet lint run check dev build-all help
all: build

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@if [ -n "$(GOOS)" ] && [ -n "$(GOARCH)" ]; then \
		echo "Cross-compiling for $(GOOS)/$(GOARCH)..."; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .; \
	else \
		$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .; \
	fi

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GO) test ./...

# Run tests with verbose output
test-verbose:
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests for a specific package
test-pkg:
	$(GO) test -v ./$(PKG)

# Run a specific test
test-run:
	$(GO) test -v -run $(TEST) ./...

# Run benchmarks
bench:
	$(GO) test -bench=. -benchmem ./...

# Development build with race detector
dev:
	@mkdir -p $(BUILD_DIR)
	@echo "Building $(BINARY_NAME) version $(VERSION) with race detector..."
	$(GO) build -race $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Lint the code
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Cross-compilation targets
# Note: Cross-compilation with CGO requires appropriate C compilers for target platforms
# For CI/CD, we use native runners for each platform instead
PLATFORMS ?= darwin/amd64 darwin/arm64 linux/amd64 linux/arm64
DIST_DIR ?= dist

build-all:
	@echo "Building for multiple platforms into $(DIST_DIR)..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d'/' -f1); \
		arch=$$(echo $$platform | cut -d'/' -f2); \
		output_name="$(BINARY_NAME)-$${os}-$${arch}"; \
		echo "  -> Building $${output_name} version $(VERSION)"; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(LDFLAGS) -o $(DIST_DIR)/$${output_name} .; \
	done

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build nb"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-pkg PKG=pkg/service - Test specific package"
	@echo "  make test-run TEST=TestName   - Run specific test"
	@echo "  make bench          - Run benchmarks"
	@echo "  make dev            - Build with race detector"
	@echo "  make lint           - Run the linter"
	@echo "  make build-all      - Build for multiple platforms"
