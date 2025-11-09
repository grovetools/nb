#!/usr/bin/env bash

# nb build script

set -e

BINARY_NAME="nb"
BUILD_FLAGS=""

echo "Building nb..."

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    arm64) ARCH="arm64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Build the binary
echo "Building for $OS/$ARCH..."
GOOS=$OS GOARCH=$ARCH go build $BUILD_FLAGS -o $BINARY_NAME ./cmd/nb

echo "Build complete: ./$BINARY_NAME"
