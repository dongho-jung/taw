# PAW Makefile

BINARY_NAME=paw
NOTIFY_BINARY=paw-notify
NOTIFY_APP=$(NOTIFY_BINARY).app
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_FLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"
GO=go

# Detect Go binary path
GO_PATH=$(shell which go 2>/dev/null || echo "/opt/homebrew/bin/go")

# Installation paths
LOCAL_BIN=~/.local/bin
LOCAL_SHARE=~/.local/share/paw

.PHONY: all build build-notify install clean test fmt lint run help

all: build

## Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO_PATH) build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd/paw

## Build the notification helper app bundle (macOS only)
build-notify:
	@echo "Building $(NOTIFY_APP)..."
	@rm -rf $(NOTIFY_APP)
	@mkdir -p $(NOTIFY_APP)/Contents/MacOS
	@mkdir -p $(NOTIFY_APP)/Contents/Resources
	@cp cmd/paw-notify/Info.plist $(NOTIFY_APP)/Contents/
	@cp icon.png $(NOTIFY_APP)/Contents/Resources/ 2>/dev/null || true
	@# Generate icon.icns from icon.png for app icon in notification settings
	@if [ -f icon.png ]; then \
		rm -rf icon.iconset && \
		mkdir -p icon.iconset && \
		sips -z 16 16 icon.png --out icon.iconset/icon_16x16.png >/dev/null && \
		sips -z 32 32 icon.png --out icon.iconset/icon_16x16@2x.png >/dev/null && \
		sips -z 32 32 icon.png --out icon.iconset/icon_32x32.png >/dev/null && \
		sips -z 64 64 icon.png --out icon.iconset/icon_32x32@2x.png >/dev/null && \
		sips -z 128 128 icon.png --out icon.iconset/icon_128x128.png >/dev/null && \
		sips -z 256 256 icon.png --out icon.iconset/icon_128x128@2x.png >/dev/null && \
		sips -z 256 256 icon.png --out icon.iconset/icon_256x256.png >/dev/null && \
		sips -z 512 512 icon.png --out icon.iconset/icon_256x256@2x.png >/dev/null && \
		sips -z 512 512 icon.png --out icon.iconset/icon_512x512.png >/dev/null && \
		sips -z 1024 1024 icon.png --out icon.iconset/icon_512x512@2x.png >/dev/null && \
		iconutil -c icns icon.iconset -o $(NOTIFY_APP)/Contents/Resources/icon.icns && \
		rm -rf icon.iconset && \
		echo "Generated icon.icns"; \
	fi
	CGO_ENABLED=1 $(GO_PATH) build -o $(NOTIFY_APP)/Contents/MacOS/$(NOTIFY_BINARY) ./cmd/paw-notify
	@echo "Built $(NOTIFY_APP)"

## Install to ~/.local/bin and ~/.local/share/paw
install: build build-notify
	@echo "Installing $(BINARY_NAME) to $(LOCAL_BIN)..."
	@mkdir -p $(LOCAL_BIN)
	@cp $(BINARY_NAME) $(LOCAL_BIN)/
	@xattr -cr $(LOCAL_BIN)/$(BINARY_NAME)
	@codesign -fs - $(LOCAL_BIN)/$(BINARY_NAME)
	@echo "Installing $(NOTIFY_APP) to $(LOCAL_SHARE)..."
	@mkdir -p $(LOCAL_SHARE)
	@rm -rf $(LOCAL_SHARE)/$(NOTIFY_APP)
	@cp -R $(NOTIFY_APP) $(LOCAL_SHARE)/
	@cp icon.png $(LOCAL_SHARE)/ 2>/dev/null || true
	@xattr -cr $(LOCAL_SHARE)/$(NOTIFY_APP)
	@codesign -fs - $(LOCAL_SHARE)/$(NOTIFY_APP)
	@echo "Done! Make sure $(LOCAL_BIN) is in your PATH"

## Install globally to /usr/local/bin (requires sudo)
install-global: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	sudo xattr -cr /usr/local/bin/$(BINARY_NAME)
	sudo codesign -fs - /usr/local/bin/$(BINARY_NAME)
	@echo "Done!"

## Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf $(NOTIFY_APP)
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
	@echo "  build          Build the binary"
	@echo "  install        Install to ~/.local/bin"
	@echo "  install-global Install to /usr/local/bin (requires sudo)"
	@echo "  clean          Remove build artifacts"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  fmt            Format code"
	@echo "  lint           Run linter"
	@echo "  deps           Download dependencies"
	@echo "  run            Build and run"
	@echo "  help           Show this help"
