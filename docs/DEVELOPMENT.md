# RunStack Development Guide

## Local Setup

Ensure you have Go installed (minimum recommended: Go 1.22+).

1. Clone the repository.
2. Initialize module if necessary (`go mod init`).
3. Ensure the workspace is clean.

## Standard Workflow

RunStack includes a unified Makefile. Run `make help` to see all targets.
Before creating a commit, you **must** run the validation suite to ensure technical health and thread safety:

```bash
make check
```
This automatically runs formatting (`gofmt`), linting (`go vet`), unit tests (`go test`), race detection (`go test -race`), and binary compilation.

## Running Components Locally

To spin up the system locally for development, build the project:

```bash
make dev
```

**Control Plane:**
```bash
make control-plane
```

**Agent:**
(Ensure the control plane is running first!)
```bash
make agent
```

**CLI Tool:**
The unified CLI is available in the `bin/` directory after running `make build` or `make dev`.
```bash
./bin/runstack doctor
./bin/runstack nodes
./bin/runstack jobs --status running
./bin/runstack job <job-id>
```

## Integration Testing

For end-to-end testing without container setups, you can leverage lightweight bash scripts (e.g., `integration_agent.sh`). These scripts compile binaries locally, spawn them in the background via `&`, simulate the HTTP flows, verify output via `grep`, and then cleanly `kill $PID` the processes.

When writing integration tests:
- Always `set -e` to fail fast.
- Clean up orphaned processes (`kill -9`) to avoid port collisions on `8080` in subsequent runs.
- Wait appropriate durations (`sleep`) for background ticker loops (like offline detection and the scheduler) to trigger.
