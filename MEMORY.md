# RunStack Memory

This document serves as the technical memory of RunStack. It answers the question: "What does the next developer (or AI) need to know about the current state of this project?"

## Current State

Milestone 6E (Hardening Pass) is complete.

Latest milestone context:
- `93d1352 feat: add retry policy and attempt tracking` (6D)
- Followed by adversarial hardening pass (6E)

## Completed Milestones

### Milestone 1
Control Plane + Agent foundation. Basic HTTP server and CLI scaffolding.

### Milestone 2
Node Registry + capabilities + heartbeat management. In-memory storage of nodes with offline detection loops.

### Milestone 3
Job and Task management. Creation of the Job domain and basic CRUD APIs.

### Milestone 4
Deterministic scheduler. A background loop assigning `PENDING` jobs to the first available `ONLINE` node.

### Milestone 5
Agent executor. Agents poll for assignments, natively claim jobs, execute them safely via `os/exec`, and report structured results with idempotency and retry handling.

### Developer Experience Refinement
Unified binary (`cmd/runstack`), comprehensive `Makefile`, structured `log/slog` logging, and `runstack doctor` CLI diagnostics.

### Milestone 8 (Phase 1)
In-memory Job Event History. Every legitimate job state transition (Creation, Assignment, Claim, Success, Failure) deterministically produces a chronological event. Events are strictly owned by the Control Plane, keeping the architecture secure. Note: This is *not* persistence yet.

### Milestone 6A & 6B: Failure Recovery
Introduced decoupled node-aware failure recovery and execution timeouts.
- **ExecutionTimeout (2h):** The absolute maximum time a job may run before being safely recovered.
- **NodeGracePeriod (30s):** The buffer applied *after* a node transitions to `OFFLINE` before its assigned jobs are recovered.
*Architectural Note on Clearing Fields:* During recovery, the Job's `AssignedNodeID`, `StartedAt`, and `ExecutionID` are explicitly cleared. This accurately reflects that the job is no longer assigned or running. The information about the previous failed assignment is not lost; it is preserved immutably inside the `JobEvent` history as the authoritative audit log.

### Milestone 6C: Execution Ownership & Result Fencing
Introduced `ExecutionID` to decouple job identity from execution attempts.
- The Control Plane generates a unique `ExecutionID` (UUID) upon `Claim`.
- The Agent must pass this `ExecutionID` when reporting results.
- Any result matching an outdated `ExecutionID` is explicitly rejected as stale.
- Recovering a job clears its active `ExecutionID`, guaranteeing the next claim generates a fresh identity.
- V1 does NOT guarantee exactly-once execution, but provides execution-aware result fencing.

### Milestone 6D: Retry Policy & Attempt Tracking
Introduced strict, bounded retry accounting via `Attempts` and `MaxRetries`.
- `Attempts` is incremented exactly once upon `Claim` (transition to `RUNNING`).
- Maximum total executions allowed is `MaxRetries + 1`.
- If an application failure (`ExitCode != 0`) or an infrastructure failure (node crash, execution timeout) occurs:
  - If `Attempts <= MaxRetries`, the job cleanly transitions to `PENDING` (discarding execution identity) for a retry.
  - If `Attempts > MaxRetries`, the job reaches the terminal `FAILED` state.
- Stale result fencing securely guards against false attempt counting.
- Infrastructure recovery and application failure semantics are completely harmonized into this single retry budget.

### Milestone 6E: Deterministic Round-Robin Scheduling + Hardening Pass
- Deterministic round-robin scheduling across sorted online nodes.
- Scheduler cursor persists across ticks for fair distribution.
- Adversarial hardening of the `Update()` API surface:
  - `PENDING→FAILED` and `ASSIGNED→FAILED` blocked via `Update()` (only reachable through internal recovery).
  - Execution fields (`AssignedNodeID`, `StartedAt`, `Result`) cannot be set on `PENDING` jobs without a status transition.
- Fixed data races in `Claim()` and `ReportResult()` logging (used live pointer after mutex unlock).
- Fixed panic in `main.go` when invoked without arguments.
- Serialized entire scheduler tick (recovery + assignment) under `s.mu` for deterministic behavior.
- Added missing `nodeId` validation in `ReportResult` API handler.
- Terminal semantic fencing: idempotent duplicates accepted, contradictory results rejected.

## Current Job Lifecycle

```text
PENDING
    ↓
ASSIGNED  (Scheduler)
    ↓
RUNNING   (Agent Claim)
    ↓
SUCCEEDED / FAILED  (Agent Result or Recovery)
```

Recovery paths (internal only):
- `RUNNING → PENDING` (execution timeout or node failure, if retries remain)
- `ASSIGNED → PENDING` (node failure, if retries remain)
- `RUNNING → FAILED` (retries exhausted)
- `ASSIGNED → FAILED` (retries exhausted)

## Current Scheduler

The scheduler:
- runs every 5 seconds
- is serialized via `s.mu` (entire tick is atomic)
- first recovers execution timeouts
- then recovers offline node jobs past grace period
- selects ONLINE nodes
- sorts nodes deterministically by ID
- round-robin assigns PENDING jobs using a persistent cursor
- does not execute jobs
- does not perform load balancing

## Current Agent

The agent:
- registers itself dynamically
- sends background heartbeats (every 10s)
- polls for assigned jobs safely (every 3s)
- requests job claims from the Control Plane
- executes commands locally via `os/exec`
- captures structured output (`ExitCode`, `Stdout`, `Stderr`)
- reports results with HTTP retries (up to 5 attempts)
- accepts 409 Conflict as stale result and stops retrying
- executes exactly **one job at a time**
- gracefully shuts down on `SIGTERM` / `SIGINT`

## Known Limitations (V1 Architecture)

- No persistent database (in-memory registries only). State is lost on Control Plane restart.
- No distributed scheduler or cluster load balancing.
- No resource-aware scheduling (capabilities exist but are ignored by the scheduler).
- Command parsing simply uses `strings.Fields()`. It does **not** support quoted shell strings (e.g. `echo "hello world"`) to deliberately avoid `/bin/sh` injection vulnerabilities.
- No process tree management (cancellation kills the parent command, but descendants may orphan).
- Exactly-once physical execution is not guaranteed. Network partitions can result in duplicate execution.
- Agent processes cannot be remotely killed after Control Plane recovery.
- Jobs and events remain in memory indefinitely (no garbage collection).
- **Agent Restart Probes:** Upon restart, the Agent does not recreate in-memory health probe loops for existing `RUNNING` instances due to lack of persisted `AppSpec`.

## Step 8B: Instance Lifecycle Execution
Instances use a **dedicated ExecutionID** distinct from Jobs.
The Control Plane is responsible for `ExecutionID` generation via the `POST /api/v1/instances/{id}/claim` endpoint.
The Agent manages long-running applications using `InstanceExecutor`, polling `ASSIGNED` instances, claiming them, and calling the generic `ContainerRuntime` interface. Status changes are securely pushed to `POST /api/v1/instances/{id}/status`.
The runtime boundary is maintained.

## Step 8C: Instance Health, Reconciliation & Automatic Recovery
Explicitly separates Status (runtime), Health (application), Node State (infrastructure), and Desired State (Deployment).
- Node drop to offline sets `UNKNOWN` with `UnknownSince`, allowing `InstanceUnknownTimeout` before marking `CRASHED` (avoiding false alarms).
- Deployment tracks `ConsecutiveCrashes` with a circuit breaker mechanism (`MaxCrashLoopThreshold`).
- Reconciler gracefully scales instances up/down and handles stale replacements, halting replacements when a deployment is `DEGRADED`.

## Step 8D: Deployment Rollout Management
Introduced zero-capacity-loss rollouts via `RolloutController`. Rollouts deterministically derive target instance counts from observed capacity, respecting `MaxSurge` and `MaxUnavailable` without data races or oscillating targets. Rollback is implemented by changing `ActiveDeploymentID` back to an immutable v1 deployment, allowing natural reconciliation.

## Step 8E: Service Routing / Traffic Management
Introduced `Service` domain, local `PortAllocator`, and `RoutingReconciler` to gracefully route traffic to `RUNNING + HEALTHY` instances via `HTTPProxy`. Traffic gracefully drains using `DrainTimeout`.

## Step 8E-Hardening: Integration & Safety Pass
- Wired `RoutingReconciler` and `HTTPProxy` into `cp.go` with graceful shutdown.
- Added `Service` CRUD HTTP API with strict Application identity constraints.
- Fixed an edge case where `ASSIGNED/STARTING` instances on dead nodes were trapped in `UNKNOWN` by ensuring the Agent's `pollAndClaim` also picks up unclaimed `UNKNOWN` instances upon node recovery.

## Recent Milestones
* **8F**: Developer Loop (Deploy, Logs).
* **8G**: Custom Domains & Ingress.
* **8H**: Secrets Management.
* **8I**: Application Health Probes & Readiness.

* **8J**: Resource Limits & Capacity Scheduling.

## Current Focus (Milestone 8K Design)

**Goal:** Implement Authentication & Remote CLI Contexts (readiness and liveness) to safely gate traffic routing and deployment rollouts, while keeping readiness failures strictly independent of process crashes.

### 8K Design Highlights:
*   **Static Bearer Tokens**: Minimal token-based API authentication for the Control Plane (in-memory).
*   **Role Separation**: Strict separation between `OPERATOR` endpoints and `AGENT` endpoints (No full RBAC, just hard boundaries).
*   **Remote Contexts**: CLI supports `~/.runstack/config` with multiple contexts to handle non-localhost endpoints.
*   **V1 Constraints Maintained**: Continues with no-database design. CP tokens must be injected at startup.


### Milestone 8L: Automatic TLS & HTTPS Ingress (COMPLETED)
- Automatic Let's Encrypt / ACME HTTP-01 certificate provision.
- TLS SNI handshake validation and connection rejection for unknown domains.
- In-memory certificate cache mapped directly to Autocert.
- Automatic HTTP -> HTTPS redirection.
- Control Plane restart volatility inherently acknowledged and guarded with bounded issuance.
