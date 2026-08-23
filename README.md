# RunStack

A lightweight distributed platform written in Go for executing jobs and deploying long-running applications.

RunStack allows you to register nodes, discover machine capabilities, execute remote shell tasks, and manage long-running containerized applications across a cluster. It aims for a clean, deterministic architecture without the overwhelming complexity of full container orchestrators like Kubernetes.

## Architecture

```text
Control Plane
    ├── Node Registry
    ├── Job Registry
    ├── Application Registry
    ├── Deployment Registry
    ├── Instance Registry
    ├── Job Scheduler
    ├── Instance Scheduler
    ├── Instance Reconciler
    └── HTTP API

Agents
    ├── Registration & Heartbeats
    ├── Job Polling & Claiming
    ├── Job Execution & Result Reporting
    ├── Instance Polling & Claiming
    └── Container Runtime (Docker/Podman)
```

## Core Capabilities

- **Node Discovery**: Agents automatically report OS, CPU, RAM, and container runtimes (Docker/Podman). The Control Plane aggressively detects offline agents via heartbeat timeouts.
- **Job Execution**: A deterministic scheduler pushes one-off `PENDING` work to available `ONLINE` nodes. Agents pull assigned jobs, claim them securely, execute them safely via `os/exec`, and report standard output/errors back.
- **Application Deployment (PaaS)**: Manage desired application state (e.g., number of replicas, image configuration).
- **Instance Reconciliation**: The Control Plane automatically reconciles desired deployments with actual runtime instances, scheduling new instances or tearing down excess ones.
- **Container Lifecycle**: Agents natively interface with container runtimes (Docker/Podman) to run isolated application instances.
- **Failure Recovery**: Node-aware failure recovery, stale execution fencing, and robust retry policies ensure workloads recover from agent crashes or timeouts.

## Domain Models

### Jobs (One-off Tasks)
Jobs strictly follow a state machine governed by the Control Plane:
```text
PENDING (Created) → ASSIGNED (Scheduler) → RUNNING (Agent Claim) → SUCCEEDED / FAILED (Agent Report or Recovery)
```

### Applications & Instances (Long-running Services)
Applications use a declarative model mapping desired state to actual instances:
```text
Application (Desired State)
    → Deployment (Immutable Snapshot)
    → Instances (Runtime Units)
```
Instances follow a similar lifecycle to Jobs:
```text
PENDING → ASSIGNED → RUNNING → STOPPED / FAILED
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

## Example Usage

### Submitting a Job
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
     -H "Content-Type: application/json" \
     -d '{"name":"my-job","command":"echo hello_world", "maxRetries": 3}'
```

### Deploying an Application
```bash
curl -X POST http://localhost:8080/api/v1/apps \
     -H "Content-Type: application/json" \
     -d '{
           "name": "web-server",
           "spec": {
             "image": "nginx:latest",
             "replicas": 3,
             "ports": [80]
           }
         }'
```

## Current Limitations (V1)
- **In-Memory State**: If the Control Plane restarts, historical data is lost.
- **Command Parsing**: Agents run job commands via `strings.Fields`. Complex shell quotes (`echo "hello world"`) are not yet parsed natively to avoid arbitrary `/bin/sh` shell injection.
- **Single Job Concurrency**: An Agent executes exactly one job at a time.
- **Application Routing**: Routing and load-balancing across instances are not yet implemented.

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
