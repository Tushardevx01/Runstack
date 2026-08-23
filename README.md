# RunStack

A lightweight distributed job execution platform written in Go.

RunStack allows you to register nodes, discover machine capabilities, and schedule remote shell tasks reliably across a cluster. It aims for a clean, deterministic architecture without the overwhelming complexity of full container orchestrators.

## Architecture

```text
Control Plane
    ├── Node Registry
    ├── Job Registry
    ├── Scheduler
    └── HTTP API

Agents
    ├── Registration
    ├── Heartbeats
    ├── Job Polling
    ├── Job Claiming
    ├── Execution
    └── Result Reporting
```

## Current Capabilities
- **Node Discovery**: Agents automatically report OS, CPU, RAM, and container runtimes (Docker/Podman).
- **Offline Detection**: The Control Plane aggressively detects offline agents via heartbeat timeouts.
- **Job Scheduling**: A deterministic scheduler pushes `PENDING` work to available `ONLINE` nodes.
- **Agent Executor**: Agents pull assigned jobs, claim them securely, execute them, and report standard output/errors back to the Control Plane.
- **CLI Management**: View and manipulate nodes and jobs via the included `runstack` CLI tool.

## Job Lifecycle

Jobs strictly follow this progression governed by the Control Plane domain logic:

```text
PENDING (Created)
    ↓
ASSIGNED (Scheduler)
    ↓
RUNNING (Agent Claim)
    ↓
SUCCEEDED / FAILED (Agent Report)
```

## Running RunStack

RunStack provides a unified CLI and a `Makefile` for streamlined development.

1. **Build the project:**
   ```bash
   make dev
   ```

2. **Start the Control Plane:**
   ```bash
   make control-plane
   ```

3. **Start an Agent:**
   ```bash
   make agent
   ```

4. **Check nodes via CLI:**
   ```bash
   ./bin/runstack nodes
   ```
   *You can also run a health check using `./bin/runstack doctor`.*

5. **Submit a Job:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/jobs \
        -H "Content-Type: application/json" \
        -d '{"name":"my-job","command":"echo hello_world"}'
   ```

6. **View the Job Result:**
   ```bash
   ./bin/runstack jobs --status succeeded
   ./bin/runstack job <job-id>
   ```

## Current Limitations (V1)
- **In-Memory State**: If the Control Plane restarts, historical data is lost.
- **Command Parsing**: Agents run commands via `strings.Fields`. Complex shell quotes (`echo "hello world"`) are not yet parsed natively to avoid arbitrary `/bin/sh` shell injection.
- **Single Concurrency**: An Agent executes exactly one job at a time.
- **No Rescheduling**: Dead nodes leave their jobs permanently stranded for now.

## Project Structure & Documentation

For a full breakdown of the architecture, roadmap, and development guidelines, please refer to the files below:
- [AGENTS.md](AGENTS.md) - Rules for AI coding assistants.
- [MEMORY.md](MEMORY.md) - Technical state and completed milestones.
- [SKILLS.md](SKILLS.md) - Subsystem capability mapping.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - Deep architectural diagrams.
- [docs/ROADMAP.md](docs/ROADMAP.md) - Future features and Milestone planning.
- [docs/API.md](docs/API.md) - HTTP endpoints.
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) - Local development guidelines.
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) - Common issues.

## Development Commands

Run `make help` to see all available commands.
Before committing, always validate your changes:
```bash
make check
```
