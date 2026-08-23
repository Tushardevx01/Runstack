# RunStack Development Guide

## Local Setup

Ensure you have Go installed (minimum recommended: Go 1.22+).

1. Clone the repository.
2. Initialize module if necessary (`go mod init`).
3. Ensure the workspace is clean.

## Standard Workflow

Before creating a commit, you **must** run the following suite to ensure technical health and thread safety:

```bash
# Format your code
gofmt -w .

# Run all unit tests
go test ./...

# Run race detector (CRITICAL for registry testing)
go test -race ./...

# Vet codebase
go vet ./...

# Ensure all binaries compile
go build ./...

# Ensure no syntax whitespace issues
git diff --check
```

## Running Components Locally

**Control Plane:**
```bash
go run ./cmd/control-plane
```

**Agent:**
(Ensure the control plane is running first!)
```bash
go run ./cmd/agent
```

**CLI Tool:**
```bash
go run ./cmd/cli nodes
go run ./cmd/cli jobs
go run ./cmd/cli job <job-id>
```

## Integration Testing

For end-to-end testing without container setups, you can leverage lightweight bash scripts (e.g., `integration_agent.sh`). These scripts compile binaries locally, spawn them in the background via `&`, simulate the HTTP flows, verify output via `grep`, and then cleanly `kill $PID` the processes.

When writing integration tests:
- Always `set -e` to fail fast.
- Clean up orphaned processes (`kill -9`) to avoid port collisions on `8080` in subsequent runs.
- Wait appropriate durations (`sleep`) for background ticker loops (like offline detection and the scheduler) to trigger.
