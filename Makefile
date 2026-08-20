# Rayls CLI Makefile

# Version information
VERSION ?= v0.0.1
COMMIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags
LDFLAGS := -X main.Version=$(VERSION) -X main.CommitSHA=$(COMMIT_SHA) -X main.BuildDate=$(BUILD_DATE)

# Binary name
BINARY_NAME := rayls

# Platforms to build for
PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64

.PHONY: all build clean test version help build-all upload-s3

all: build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d'-' -f1); \
		arch=$$(echo $$platform | cut -d'-' -f2); \
		output="dist/$(BINARY_NAME)-$$platform"; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" -o $$output .; \
		shasum -a 256 $$output | cut -d' ' -f1 > $$output.sha256; \
	done
	@echo "All builds complete in dist/"

# Show version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT_SHA)"
	@echo "Built:   $(BUILD_DATE)"

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

# Upload to S3 (requires AWS CLI configured)
upload-s3: build-all
	@echo "Uploading binaries to S3..."
	@for platform in $(PLATFORMS); do \
		aws s3 cp dist/$(BINARY_NAME)-$$platform s3://rayls-cli/$(BINARY_NAME)-$$platform --acl public-read; \
		echo "Uploaded $(BINARY_NAME)-$$platform"; \
	done
	@echo "Uploading manifest.json..."
	@aws s3 cp manifest.json s3://rayls-cli/manifest.json --acl public-read
	@echo "Upload complete!"

# Generate checksums for dist files
checksums:
	@echo "Generating checksums..."
	@cd dist && shasum -a 256 $(BINARY_NAME)-* > SHA256SUMS
	@echo "Checksums generated in dist/SHA256SUMS"

# Help
help:
	@echo "Rayls CLI Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make build         Build for current platform"
	@echo "  make build-all     Build for all platforms"
	@echo "  make version       Show version information"
	@echo "  make test          Run tests"
	@echo "  make clean         Remove build artifacts"
	@echo "  make upload-s3     Upload binaries to S3 (requires AWS CLI)"
	@echo "  make checksums     Generate SHA256 checksums"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=$(VERSION)"
	@echo "  COMMIT_SHA=$(COMMIT_SHA)"
	@echo "  BUILD_DATE=$(BUILD_DATE)"
