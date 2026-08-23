# RunStack Architecture

RunStack is a lightweight, distributed job execution platform built in Go. Its architecture heavily emphasizes a **trusted Control Plane** pattern, where all authoritative state is owned by central registries, and Agents act as thin, secure workers.

## High-Level Architecture

```text
                         ┌────────────────────┐
                         │       CLI          │
                         └─────────┬──────────┘
                                   │ HTTP
                                   ▼
┌─────────────────────────────────────────────────────────┐
│                    CONTROL PLANE                        │
│                                                         │
│  HTTP API                                               │
│     │                                                   │
│     ├──────────────┐                                    │
│     ▼              ▼                                    │
│ Node Registry   Job Registry                            │
│     │              │                                    │
│     └──────┬───────┘                                    │
│            ▼                                            │
│        Scheduler                                        │
│                                                         │
└──────────────────────┬──────────────────────────────────┘
                       │ HTTP
                       ▼
              ┌──────────────────┐
              │      AGENT       │
              │                  │
              │ Heartbeat        │
              │ Poll             │
              │ Claim            │
              │ Execute          │
              │ Report           │
              └──────────────────┘
```

## Package Responsibilities

The codebase is organized into cleanly separated domains to prevent architectural drift:

```text
cmd/
├── agent/            # The Agent binary. Responsible for execution, polling, and heartbeats.
├── cli/              # The RunStack CLI (runstack jobs, runstack nodes).
└── control-plane/    # The central server. Bootstraps the registries, scheduler, and API.

internal/
├── api/              # HTTP Handlers. Keeps web-layer logic thin and delegates to registries.
├── job/              # Job domain. Defines Job structures, enums, and the Job Registry.
├── node/             # Node domain. Defines Node structures, capabilities, and Node Registry.
├── scheduler/        # The Scheduler domain. Handles assigning PENDING jobs to ONLINE nodes.
└── sysinfo/          # System intelligence. Inspects local OS for CPU, RAM, Docker, Podman.
```

## Dependency Direction

To maintain boundaries, packages may only depend downwards. An internal domain package (like `node` or `job`) **must never** import the `api` or `cmd` packages.

```text
API
 │
 ├── Job
 └── Node

Scheduler
 │
 ├── Job
 └── Node

Agent
 │
 └── API (implicit via HTTP)

CLI
 │
 └── API (implicit via HTTP)
```

## Core Workflows

### 1. Job State Transitions
All Job state transitions are mathematically governed by `isValidTransition()` inside `internal/job/job.go`.
Valid paths:
* `PENDING` → `ASSIGNED` (Scheduler)
* `ASSIGNED` → `RUNNING` (Agent Claim)
* `RUNNING` → `SUCCEEDED` / `FAILED` (Agent Result Report)

### 2. Node Offline Detection
The Node Registry contains a background loop (`startOfflineDetector`) that checks all nodes every 30 seconds. If a node hasn't sent a heartbeat within the threshold, it is automatically marked `OFFLINE`.

### 3. Claim Atomicity
Agents cannot arbitrarily transition an `ASSIGNED` job to `RUNNING` on their own local copy. They must hit `POST /claim`. The Job Registry uses a `sync.RWMutex` to lock the state, verify the Job is still `ASSIGNED` to the requesting `NodeID`, update to `RUNNING`, and unlock. This intrinsically prevents distributed duplicate execution.
