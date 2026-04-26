# Makefile for agent project

# Default executable name
BINARY_NAME=thand
BUILD_DIR=bin

# If UPX is available on the system, we'll use it to compress binaries.
# Customize compression flags via UPX_FLAGS.
UPX_FLAGS ?= --best --lzma --force-macos

GO_BUILD_FLAGS= -ldflags "-s -w"

# Default target - builds the application
all: build

# Initialize and update submodules
submodules:
	git submodule update --init --recursive

# Update submodules to latest version
update-submodules:
	git submodule update --remote --recursive

# Build the application
build: submodules
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for multiple platforms
build-all: submodules
	GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=windows GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Build linux/amd64 binary for frontend E2E tests
build-linux-amd64: submodules
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Manually compress any binaries in $(BUILD_DIR) using UPX
compress:
	@if command -v upx >/dev/null 2>&1; then \
	  echo "Compressing all binaries in $(BUILD_DIR)/ with UPX..."; \
	  upx $(UPX_FLAGS) $(BUILD_DIR)/* || true; \
	else \
	  echo "'upx' not found. Install via 'brew install upx' (macOS) or your package manager."; \
	fi

# Install the binary to GOPATH/bin
install: submodules
	go install .

# Run the application
run: submodules
	go run .

# Run tests
test: submodules
	go test ./...

# Run functional tests
test-functional: submodules
	cd test && go test -v ./functional/...

# Run integration tests
test-integration: submodules
	cd test && go test -v ./integration/...

# Run UI E2E tests (requires linux/amd64 binary for Thand server container)
test-e2e: submodules build-linux-amd64
	cd test && go test -v -timeout 20m ./integration/frontend/...

# Generate FlatBuffers from JSON data
generate-data:
	@echo "Generating FlatBuffer schemas..."
	flatc --go -o internal/data/generated internal/data/schemas/aws.fbs
	flatc --go -o internal/data/generated internal/data/schemas/gcp.fbs
	flatc --go -o internal/data/generated internal/data/schemas/azure.fbs
	@echo "Generating FlatBuffer data files..."
	go run tools/generate-iam-dataset/main.go
	@echo "Data generation complete!"

# Run workflowcheck static analysis to detect non-deterministic workflow code.
# workflowcheck inspects workflow functions for forbidden calls (time.Now,
# goroutines, select, etc.) that cause Temporal replay failures.
# Install once with: go install go.temporal.io/sdk/contrib/tools/workflowcheck@latest
workflowcheck:
	@if command -v workflowcheck >/dev/null 2>&1; then \
	  echo "Running workflowcheck..."; \
	  workflowcheck ./internal/... ./sdk/...; \
	else \
	  echo "workflowcheck not found. Install with:"; \
	  echo "  go install go.temporal.io/sdk/contrib/tools/workflowcheck@latest"; \
	  exit 1; \
	fi

# Generate Swagger documentation
swagger:
	@echo "Generating Swagger documentation..."
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g internal/daemon/server.go --parseDependency --parseInternal; \
		echo "Swagger documentation generated successfully!"; \
		echo "View at: http://localhost:8080/swagger/index.html"; \
	else \
		echo "Error: 'swag' command not found."; \
		echo "Install with: go install github.com/swaggo/swag/cmd/swag@latest"; \
		exit 1; \
	fi

.PHONY: all build build-all build-linux-amd64 clean install run test test-functional test-integration test-e2e submodules update-submodules compress generate-data swagger workflowcheck
