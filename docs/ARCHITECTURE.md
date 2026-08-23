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
└── runstack/         # The unified RunStack binary containing CP, Agent, and CLI logic.

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
* `RUNNING` → `PENDING` (Control Plane Stale Recovery)

Every legitimate state transition automatically produces a chronological `JobEvent` owned securely by the Control Plane. Agents cannot arbitrarily inject events.

**Note on Stale Recovery:** When a `RUNNING` job exceeds the execution threshold, the Control Plane recovers it to `PENDING`. During this transition, `AssignedNodeID` and `StartedAt` are explicitly cleared on the Job struct. This accurately resets the state for future scheduling. The *previous* assignment and start data are permanently and immutably preserved in the Job's Event History.

### 2. Node Offline Detection
The Node Registry contains a background loop (`startOfflineDetector`) that checks all nodes every 30 seconds. If a node hasn't sent a heartbeat within the threshold, it is automatically marked `OFFLINE`.

### 3. Claim Atomicity
Agents cannot arbitrarily transition an `ASSIGNED` job to `RUNNING` on their own local copy. They must hit `POST /claim`. The Job Registry uses a `sync.RWMutex` to lock the state, verify the Job is still `ASSIGNED` to the requesting `NodeID`, update to `RUNNING`, and unlock. This intrinsically prevents distributed duplicate execution.

## V1 Architectural Limitations

The current architecture is intentionally simplified to provide a reliable foundation. The following constraints must be preserved until explicitly addressed in future milestones:

- **In-Memory State:** Registries live entirely in memory. Restarting the Control Plane deletes all nodes, jobs, and history. There is no database or persistence.
- **One-Job-At-A-Time:** The Agent executes exactly one job at a time. There are no worker pools.
- **`strings.Fields()` Parsing:** Commands are split into arguments purely by spaces. Quoted shell arguments (`echo "hello world"`) are not supported. This avoids invoking arbitrary shell interpreters (like `/bin/sh`) which reduces injection risks.
- **No Agent Web Server:** The Agent does not listen for incoming connections. All interaction is via the Agent polling the Control Plane.
- **Control Plane as Source of Truth:** Agents never own their state. They cannot unilaterally execute; they must `Claim`.
- **First-Online-Node Scheduling:** The Scheduler deterministically picks the first `ONLINE` node. It is not resource-aware yet.
- **No Leases or Rescheduling:** If an Agent dies while `RUNNING` a job, the job is stranded. There are no timeouts or retries.
