.PHONY: all build test test-race lint format clean dev

BIN_DIR := bin
APP_BIN := $(BIN_DIR)/runstack

all: format test build

# -------------------------
# Build Commands
# -------------------------
build:
	@echo "Building RunStack..."
	@mkdir -p $(BIN_DIR)
	go build -o $(APP_BIN) ./cmd/runstack

# -------------------------
# Run Commands
# -------------------------
run-cp: build
	@echo "Starting Control Plane..."
	$(APP_BIN) cp

run-agent: build
	@echo "Starting Agent..."
	$(APP_BIN) agent

# -------------------------
# Testing & Validation
# -------------------------
test:
	@echo "Running unit tests..."
	go test ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race ./...

lint:
	@echo "Running go vet..."
	go vet ./...
	@echo "Checking formatting..."
	@test -z $$(gofmt -l .) || (echo "Code formatting issues found. Run 'make format'"; exit 1)

format:
	@echo "Formatting code..."
	gofmt -w .

# -------------------------
# Utilities
# -------------------------
clean:
	@echo "Cleaning bin directory..."
	rm -rf $(BIN_DIR)

dev: clean format lint test-race build
	@echo "Development build complete. Binary available at $(APP_BIN)"
