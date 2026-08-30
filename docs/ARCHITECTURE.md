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


### Authentication and Authorization (Milestone 8K - Designing)
The Control Plane is protected by a strict static Bearer token model to prevent unauthorized remote execution.
- **Roles:** The API is divided into two disjoint scopes: `USER/OPERATOR` (application management, routing, secrets) and `AGENT` (node registration, instance/job execution claims).
- **Middleware:** A centralized authentication middleware intercepts HTTP requests, extracts the `Authorization: Bearer <token>` header, and compares it against in-memory tokens provided to the Control Plane at startup.
- **Enforcement:** Missing or invalid tokens yield `401 Unauthorized`. Cross-role usage yields `403 Forbidden`.
- **Separation from Ownership:** Authentication verifies *identity*, but handlers independently verify *ownership* (e.g., verifying the Agent's NodeID matches the requested Instance's AssignedNodeID, or that an Operator's Secret belongs to the correct Application).

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

**Note on Stale Recovery:** When a `RUNNING` job exceeds the execution timeout or its assigned node exceeds the offline grace period, the Control Plane recovers it to `PENDING`. During this transition, `AssignedNodeID` and `StartedAt` are explicitly cleared on the Job struct. This accurately resets the state for future scheduling. The *previous* assignment and start data are permanently and immutably preserved in the Job's Event History.

### 2. Node Offline Detection
The Node Registry contains a background loop (`startOfflineDetector`) that checks all nodes every 30 seconds. If a node hasn't sent a heartbeat within the threshold, it is automatically marked `OFFLINE`.

### 3. Claim Atomicity & Execution Ownership
Agents cannot arbitrarily transition an `ASSIGNED` job to `RUNNING` on their own local copy. They must hit `POST /claim`. The Job Registry uses a `sync.RWMutex` to lock the state, verify the Job is still `ASSIGNED` to the requesting `NodeID`, update to `RUNNING`, generate a unique `ExecutionID`, and unlock. This intrinsically prevents distributed duplicate execution initiation.

The `ExecutionID` acts as a cryptographically random fencing token. When the agent finishes executing the job, it must provide the `ExecutionID` back to the Control Plane. If the job was recovered and reassigned while the agent was disconnected, the Control Plane will reject the stale agent's result, providing strict execution-aware result fencing.

Terminal states additionally enforce *semantic fencing*: a legitimate duplicate result (same execution, same exit code) is accepted idempotently, while a contradictory result (same execution, different exit code) is explicitly rejected.


### 4. Retry Budget and Bounded Executions
All failures—both application (`ExitCode != 0`) and infrastructure (node timeouts)—are evaluated against a bounded `MaxRetries` limit configured at job creation. The `Attempts` counter increments upon `Claim`. If `Attempts <= MaxRetries` after a failure occurs, the Control Plane safely clears the `ExecutionID` and transitions the job to `PENDING` for reassignment. If the budget is exhausted (`Attempts > MaxRetries`), it reaches the terminal `FAILED` state, mathematically preventing infinite infrastructure recovery loops.

## V1 Architectural Limitations

The current architecture is intentionally simplified to provide a reliable foundation. The following constraints must be preserved until explicitly addressed in future milestones:

- **In-Memory State:** Registries live entirely in memory. Restarting the Control Plane deletes all nodes, jobs, and history. There is no database or persistence.
- **One-Job-At-A-Time:** The Agent executes exactly one job at a time. There are no worker pools.
- **`strings.Fields()` Parsing:** Commands are split into arguments purely by spaces. Quoted shell arguments (`echo "hello world"`) are not supported. This avoids invoking arbitrary shell interpreters (like `/bin/sh`) which reduces injection risks.
- **No Agent Web Server:** The Agent does not listen for incoming connections. All interaction is via the Agent polling the Control Plane.
- **Control Plane as Source of Truth:** Agents never own their state. They cannot unilaterally execute; they must `Claim`.
- **Deterministic Round-Robin Scheduling:** The Scheduler distributes jobs across sorted ONLINE nodes in round-robin order using a persistent cursor. It is not resource-aware.
- **Bounded Retry Budget:** All failures (application and infrastructure) are evaluated against `MaxRetries`. Maximum total executions = `MaxRetries + 1`.
- **No Exactly-Once Execution:** Network partitions can result in duplicate physical execution. The system provides execution-aware *result fencing* but not physical execution prevention.
- **Update() API Hardened:** The PATCH endpoint cannot bypass domain invariants. Terminal transitions (`PENDING→FAILED`, `ASSIGNED→FAILED`) and execution field manipulation are blocked through the public API.

### Instance Lifecycle Execution
Instances are managed by a dedicated `InstanceExecutor` on the Agent.
Unlike jobs, instances represent long-lived application replicas and possess their own dedicated `ExecutionID`.
The Agent claims `ASSIGNED` instances, delegates to a `ContainerRuntime` adapter, and actively pushes runtime events back to the Control Plane `UpdateStatus` endpoint securely.

### Instance Health & Reconciliation
RunStack strictly separates Status (lifecycle), Health (readiness), Node state, and Desired state.
- **Node Loss (UNKNOWN):** Node offline translates to `UNKNOWN` with an `UnknownSince` timestamp. Only after `InstanceUnknownTimeout` does it become `CRASHED` and replaceable, preventing network partitions from acting like application crashes.
- **Crash-Loop Breaker:** The `Deployment` tracks `ConsecutiveCrashes`. If the threshold (`MaxCrashLoopThreshold`) is reached, the Deployment becomes `DEGRADED` and the Reconciler pauses replacement to protect infrastructure.
- **Idempotency:** Repeated Reconciler ticks converge to the same desired state safely.

### Traffic Routing
Service requests are mapped to `ApplicationID`. A background `RoutingReconciler` continuously updates an embedded `HTTPProxy` with `RUNNING + HEALTHY` endpoints. A strict `Draining` state gracefully removes endpoints from the proxy while allowing active connections to finish before termination.

### Custom Domains and Ingress
RunStack uses Host-based routing. Domains are Application-scoped. Ingress mappings link Domains to Services, which are then passed to the embedded proxy using an atomic lock-free `[]RouteRule` swap for zero-downtime reconfiguration. Cross-application routing is strictly rejected by the Control Plane.

### Secrets Management
Secrets are fully decoupled from immutable Deployments and reside in an Application-scoped `SecretRegistry`. 
Deployments store **references** (e.g., `secret:db-password`). The Control Plane resolves these references just-in-time when providing the runtime environment payload to the Agent during an Instance Claim. This isolates plaintext solely to process memory and completely avoids writing secrets into JSON responses, deployment histories, or system logs.


## TLS & HTTPS Ingress (Milestone 8L)

RunStack natively terminates TLS via the Let's Encrypt ACME HTTP-01 challenge.
To preserve V1 constraints (No Database), all certificate private keys are held securely in-memory.

**Deployment Port Mapping:**
Internally, the RunStack Control Plane binds:
- HTTP: Port 80 (or 8080)
- HTTPS: Port 8443

To expose standard HTTPS to the public internet, the deployment environment (e.g., Load Balancer, `iptables`, or NAT) must map:
- `Public :443 -> RunStack :8443`
- `Public :80  -> RunStack :80` (Required for HTTP-01 challenge)

**Restart Volatility:**
Certificates are held in RAM. Upon a Control Plane restart, certificates are lost. `autocert` will seamlessly re-request them when the first HTTPS SNI handshake arrives. Users must be aware of Let's Encrypt rate limits (e.g., 50 certificates per domain per week) if restarting the Control Plane excessively.
