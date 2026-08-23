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
