.PHONY: all help build test test-race lint format check clean dev control-plane agent cli

BIN_DIR := bin
APP_BIN := $(BIN_DIR)/runstack

all: help

help:
	@echo "RunStack Developer Makefile"
	@echo "───────────────────────────"
	@echo "Commands:"
	@echo "  make help           Show this help message"
	@echo "  make build          Build the runstack binary"
	@echo "  make test           Run unit tests"
	@echo "  make test-race      Run tests with race detector"
	@echo "  make lint           Run go vet and format checks"
	@echo "  make format         Format code with gofmt"
	@echo "  make check          Run complete validation pipeline (format, lint, test, build)"
	@echo "  make clean          Remove build artifacts"
	@echo "  make dev            Clean, check, and build for local development"
	@echo "  make control-plane  Build and run the Control Plane"
	@echo "  make agent          Build and run the Agent"
	@echo "  make cli            Alias to build the CLI binary"

# -------------------------
# Build Commands
# -------------------------
build:
	@echo "Building RunStack..."
	@mkdir -p $(BIN_DIR)
	go build -o $(APP_BIN) ./cmd/runstack

cli: build

# -------------------------
# Run Commands
# -------------------------
control-plane: build
	@echo "Starting Control Plane..."
	$(APP_BIN) cp

agent: build
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

check: format lint test-race build
	@echo "Validation pipeline passed."

# -------------------------
# Utilities
# -------------------------
clean:
	@echo "Cleaning bin directory..."
	rm -rf $(BIN_DIR)

dev: clean check
	@echo "Development build complete. Binary available at $(APP_BIN)"
