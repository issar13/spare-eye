# Build tasks. Platform-specific flags live here so nobody has to remember them.
#
# Ubuntu 24.04 dropped webkit2gtk-4.0, which Wails v2 still looks for by
# default, so Linux builds need the webkit2_41 tag. `wails doctor` reports
# libwebkit as "Not Found" on this machine for that reason; it is not a problem.

BINARY_CLI     := motiond
BINARY_DESKTOP := motion-detector
OUTPUT_DIR     := build/bin
WAILS          := $(shell go env GOPATH)/bin/wails

UNAME := $(shell uname -s)
ifeq ($(UNAME),Linux)
	WAILS_TAGS := webkit2_41
else
	WAILS_TAGS :=
endif

TAG_FLAG := $(if $(WAILS_TAGS),-tags $(WAILS_TAGS),)

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: ## Run tests that do not need a camera
	go test ./...

.PHONY: test-camera
test-camera: ## Run camera integration tests (needs a free webcam)
	go test -tags camera ./internal/detector/ -v

.PHONY: check
check: fmt vet test ## Format, vet, and test

.PHONY: run
run: ## Run the headless CLI
	go run ./cmd/motiond

.PHONY: cameras
cameras: ## List usable cameras
	go run ./cmd/motiond -list-cameras

.PHONY: build-cli
build-cli: ## Build the headless CLI
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(BINARY_CLI) ./cmd/motiond

.PHONY: dev
dev: ## Run the desktop app with hot reload
	$(WAILS) dev $(TAG_FLAG)

.PHONY: build-desktop
build-desktop: ## Build the desktop app for the current platform
	$(WAILS) build $(TAG_FLAG) -clean

.PHONY: package-linux
package-linux: ## Build a Linux desktop binary
	$(WAILS) build -tags webkit2_41 -clean -platform linux/amd64

.PHONY: package-windows
package-windows: ## Build a Windows installer (run on Windows)
	$(WAILS) build -clean -platform windows/amd64 -nsis

.PHONY: clean
clean: ## Remove build output
	rm -rf $(OUTPUT_DIR) frontend/dist
	go clean -cache -testcache
