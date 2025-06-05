BINARY_NAME=guvnor

# Version information
VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags for version injection
LDFLAGS := -ldflags "\
	-X github.com/Studio-Elephant-and-Rope/guvnor/internal/cmd.Version=$(VERSION) \
	-X github.com/Studio-Elephant-and-Rope/guvnor/internal/cmd.Commit=$(COMMIT) \
	-X github.com/Studio-Elephant-and-Rope/guvnor/internal/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: all build test clean version install lint

all: build

build:
	@echo "Building Guvnor v$(VERSION)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/guvnor

# Development build without version injection
build-dev:
	@echo "Building Guvnor (development)..."
	@go build -o $(BINARY_NAME) ./cmd/guvnor

test:
	@echo "Running tests..."
	@go test -v ./...

# Test with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

clean:
	@echo "Cleaning..."
	@go clean
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html

# Install the binary to $GOPATH/bin
install:
	@echo "Installing Guvnor..."
	@go install $(LDFLAGS) ./cmd/guvnor

# Show version information
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(BUILD_DATE)"

# Run linting
lint:
	@echo "Running linters..."
	@go vet ./...
	@go fmt ./...

# Run the application with version command (for testing)
run-version: build
	@./$(BINARY_NAME) version
