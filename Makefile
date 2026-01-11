# PAW Makefile

BINARY_NAME=paw
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_FLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"
GO=go

# Detect Go binary path
GO_PATH=$(shell which go 2>/dev/null || echo "/opt/homebrew/bin/go")

# Installation paths
LOCAL_BIN=~/.local/bin

.PHONY: all build install uninstall install-global uninstall-global install-brew uninstall-brew clean test fmt lint run help

all: build

## Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO_PATH) build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd/paw

## Install to ~/.local/bin
install: build
	@echo "Installing $(BINARY_NAME) to $(LOCAL_BIN)..."
	@mkdir -p $(LOCAL_BIN)
	@cp $(BINARY_NAME) $(LOCAL_BIN)/
	@xattr -cr $(LOCAL_BIN)/$(BINARY_NAME)
	@codesign -fs - $(LOCAL_BIN)/$(BINARY_NAME)
	@echo "Done! Make sure $(LOCAL_BIN) is in your PATH"

## Install globally to /usr/local/bin (requires sudo)
install-global: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	sudo xattr -cr /usr/local/bin/$(BINARY_NAME)
	sudo codesign -fs - /usr/local/bin/$(BINARY_NAME)
	@echo "Done!"

## Uninstall from ~/.local/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(LOCAL_BIN)..."
	@rm -f $(LOCAL_BIN)/$(BINARY_NAME)
	@echo "Done!"

## Uninstall from /usr/local/bin (requires sudo)
uninstall-global:
	@echo "Uninstalling $(BINARY_NAME) from /usr/local/bin..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Done!"

## Install via Homebrew from local build (for testing)
## Uses a local tap at ~/.local/share/paw/homebrew-local
LOCAL_TAP_DIR=$(HOME)/.local/share/paw/homebrew-local
BREW_TAP_DIR=$(shell brew --prefix)/Library/Taps/paw/homebrew-local
install-brew: build
	@echo "Creating local brew package..."
	@rm -rf /tmp/paw-brew-local
	@mkdir -p /tmp/paw-brew-local
	@cp $(BINARY_NAME) /tmp/paw-brew-local/
	@cd /tmp/paw-brew-local && tar -czf ../paw-local.tar.gz .
	@echo "Setting up local tap..."
	@# Uninstall first (required before untap), then untap
	@brew uninstall paw/local/paw 2>/dev/null || true
	@brew untap paw/local 2>/dev/null || true
	@rm -rf $(LOCAL_TAP_DIR)
	@mkdir -p $(LOCAL_TAP_DIR)/Formula
	@SHA=$$(shasum -a 256 /tmp/paw-local.tar.gz | cut -d' ' -f1) && \
	echo "class Paw < Formula" > $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  desc "Parallel AI Workers - local build"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  homepage "https://github.com/dongho-jung/paw"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  url "file:///tmp/paw-local.tar.gz"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo "  sha256 \"$$SHA\"" >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  version "local-$(VERSION)"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  depends_on "tmux"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  def install' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '    bin.install "paw"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  end' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  test do' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '    system "#{bin}/paw", "--version"' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo '  end' >> $(LOCAL_TAP_DIR)/Formula/paw.rb && \
	echo 'end' >> $(LOCAL_TAP_DIR)/Formula/paw.rb
	@cd $(LOCAL_TAP_DIR) && git init -q && git add -A && git commit -q -m "Add paw formula"
	@brew tap paw/local $(LOCAL_TAP_DIR)
	@echo "Installing via brew..."
	@HOMEBREW_NO_AUTO_UPDATE=1 brew install paw/local/paw
	@rm -rf /tmp/paw-brew-local
	@echo "Done! Installed paw ($(VERSION)) from local tap"
	@echo "Run 'paw --version' to verify"

## Uninstall local brew package and tap
uninstall-brew:
	@echo "Uninstalling paw from local tap..."
	@brew uninstall paw/local/paw 2>/dev/null || true
	@brew untap paw/local 2>/dev/null || true
	@rm -rf $(LOCAL_TAP_DIR)
	@rm -f /tmp/paw-local.tar.gz
	@echo "Done!"

## Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@$(GO_PATH) clean

## Run tests
test:
	@echo "Running tests..."
	$(GO_PATH) test -v ./...

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO_PATH) test -v -coverprofile=coverage.out ./...
	$(GO_PATH) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Format code
fmt:
	@echo "Formatting code..."
	$(GO_PATH) fmt ./...

## Lint code
lint:
	@echo "Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Run: brew install golangci-lint"; \
	fi

## Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO_PATH) mod download
	$(GO_PATH) mod tidy

## Run the application
run: build
	./$(BINARY_NAME)

## Generate mocks (for testing)
mocks:
	@echo "Generating mocks..."
	@if command -v mockgen >/dev/null 2>&1; then \
		mockgen -source=internal/tmux/client.go -destination=internal/tmux/mock.go -package=tmux; \
		mockgen -source=internal/git/client.go -destination=internal/git/mock.go -package=git; \
	else \
		echo "mockgen not installed. Run: go install github.com/golang/mock/mockgen@latest"; \
	fi

## Show help
help:
	@echo "PAW (Parallel AI Workers) - Build Commands"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build            Build the binary"
	@echo "  install          Install to ~/.local/bin"
	@echo "  uninstall        Uninstall from ~/.local/bin"
	@echo "  install-global   Install to /usr/local/bin (requires sudo)"
	@echo "  uninstall-global Uninstall from /usr/local/bin (requires sudo)"
	@echo "  install-brew     Install via Homebrew from local build (for testing)"
	@echo "  uninstall-brew   Uninstall local brew package"
	@echo "  clean            Remove build artifacts"
	@echo "  test             Run tests"
	@echo "  test-coverage    Run tests with coverage report"
	@echo "  fmt              Format code"
	@echo "  lint             Run linter"
	@echo "  deps             Download dependencies"
	@echo "  run              Build and run"
	@echo "  help             Show this help"
