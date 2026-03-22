.PHONY: all build build-all install uninstall clean test fmt lint fix deps update-deps run help

# Build variables
BINARY_NAME=ccmux
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date +%FT%T%z)
MAIN_PKG=main
LDFLAGS=-X $(MAIN_PKG).Version=$(VERSION) -X $(MAIN_PKG).GitCommit=$(GIT_COMMIT) -X $(MAIN_PKG).BuildTime=$(BUILD_TIME) -s -w

# Go variables
GO?=CGO_ENABLED=0 go
GOFLAGS?=-v

# Golangci-lint
GOLANGCI_LINT?=golangci-lint

# Installation
INSTALL_PREFIX?=/usr/local
INSTALL_BIN_DIR=$(INSTALL_PREFIX)/bin

# OS detection
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else
	PLATFORM=$(UNAME_S)
	ARCH=$(UNAME_M)
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

# Default target
all: build

## build: Build ccmux for the current platform
build:
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./$(CMD_DIR)
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Build complete: $(BINARY_PATH)"

## build-all: Build ccmux for all supported platforms
build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64   ./$(CMD_DIR)
	GOOS=linux  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64   ./$(CMD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64  ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64  ./$(CMD_DIR)
	@echo "All builds complete"

## install: Build and install ccmux to $(INSTALL_BIN_DIR)
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_BIN_DIR)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME).new
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME).new
	@mv -f $(INSTALL_BIN_DIR)/$(BINARY_NAME).new $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installed to $(INSTALL_BIN_DIR)/$(BINARY_NAME)"

## uninstall: Remove ccmux from $(INSTALL_BIN_DIR)
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Removed $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Note: config and state in ~/.ccmux were not removed."

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## test: Run tests
test:
	@$(GO) test ./...

## fmt: Format Go code
fmt:
	@$(GOLANGCI_LINT) fmt

## lint: Run linters
lint:
	@$(GOLANGCI_LINT) run

## fix: Fix linting issues
fix:
	@$(GOLANGCI_LINT) run --fix

## deps: Download and verify dependencies
deps:
	@$(GO) mod download
	@$(GO) mod verify

## update-deps: Update all dependencies
update-deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy

## run: Build and run ccmux gateway
run: build
	@$(BUILD_DIR)/$(BINARY_NAME) gateway $(ARGS)

## help: Show this help message
help:
	@echo "ccmux Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-16s %s\n", substr($$1, 4), $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Install to /usr/local/bin"
	@echo "  make run                # Build and run gateway"
	@echo "  make build-all          # Build for all platforms"
	@echo ""
	@echo "Environment Variables:"
	@echo "  INSTALL_PREFIX          # Installation prefix (default: /usr/local)"
	@echo "  VERSION                 # Version string (default: git describe)"
	@echo ""
	@echo "Current Configuration:"
	@echo "  Platform: $(PLATFORM)/$(ARCH)"
	@echo "  Binary:   $(BINARY_PATH)"
	@echo "  Install:  $(INSTALL_BIN_DIR)"
