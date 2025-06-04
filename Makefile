BINARY_NAME=guvnor

.PHONY: all build test clean

all: build

build:
	@echo "Building Guvnor..."
	@go build -o $(BINARY_NAME) ./cmd/guvnor

test:
	@echo "Running tests..."
	@go test -v ./...

clean:
	@echo "Cleaning..."
	@go clean
	@rm -f $(BINARY_NAME)
