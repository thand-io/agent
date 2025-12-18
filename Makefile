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
	GOOS=darwin GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=windows GOARCH=amd64 GOEXPERIMENT=jsonv2 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

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

# Generate FlatBuffers from JSON data
generate-data:
	@echo "Generating FlatBuffer schemas..."
	flatc --go -o internal/data/generated internal/data/schemas/aws.fbs
	flatc --go -o internal/data/generated internal/data/schemas/gcp.fbs
	flatc --go -o internal/data/generated internal/data/schemas/azure.fbs
	@echo "Generating FlatBuffer data files..."
	go run tools/generate-iam-dataset/main.go
	@echo "Data generation complete!"

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

.PHONY: all build build-all clean install run test test-functional test-integration submodules update-submodules compress generate-data swagger
