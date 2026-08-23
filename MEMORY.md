# RunStack Memory

This document serves as the technical memory of RunStack. It answers the question: "What does the next developer (or AI) need to know about the current state of this project?"

## Current State

Milestone 5 is complete.

Latest commit context:
`26ca93c feat: add agent job executor`

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
In-memory Job Event History. Every legitimate job state transition (Creation, Assignment, Claim, Success, Failure) deterministically produces a chronological event. Events are strictly owned by the Control Plane, keeping the architecture secure. Note: This is *not* persistence yet, and *not* stale-job recovery yet.

### Milestone 6A & 6B: Failure Recovery
Introduced decoupled node-aware failure recovery and execution timeouts.
- **ExecutionTimeout (2h):** The absolute maximum time a job may run before being safely recovered to `PENDING`.
- **NodeGracePeriod (30s):** The buffer applied *after* a node transitions to `OFFLINE` before its assigned jobs are recovered to `PENDING`.
*Architectural Note on Clearing Fields:* During recovery, the Job's `AssignedNodeID` and `StartedAt` are explicitly cleared. This accurately reflects that the job is no longer assigned or running. The information about the previous failed assignment is not lost; it is preserved immutably inside the `JobEvent` history as the authoritative audit log. Duplicate execution is an accepted risk at this stage as leases have not been introduced.

### Milestone 6C: Execution Ownership & Result Fencing
Introduced `ExecutionID` to decouple job identity from execution attempts.
- The Control Plane generates a unique `ExecutionID` (UUID) upon `Claim`.
- The Agent must pass this `ExecutionID` when reporting results.
- Any result matching an outdated `ExecutionID` is explicitly rejected as stale.
- Recovering a job clears its active `ExecutionID`, guaranteeing the next claim generates a fresh identity.
- V1 does NOT guarantee exactly-once execution, but provides execution-aware result fencing. The old execution attempt is permanently archived in the event history.


### Milestone 6D: Retry Policy & Attempt Tracking
Introduced strict, bounded retry accounting via `Attempts` and `MaxRetries`.
- `Attempts` is incremented exactly once upon `Claim` (transition to `RUNNING`).
- Maximum total executions allowed is `MaxRetries + 1`.
- If an application failure (`ExitCode != 0`) or an infrastructure failure (node crash, execution timeout) occurs:
  - If `Attempts <= MaxRetries`, the job cleanly transitions to `PENDING` (discarding execution identity) for a retry.
  - If `Attempts > MaxRetries`, the job reaches the terminal `FAILED` state.
- Stale result fencing securely guards against false attempt counting.
- Infrastructure recovery and application failure semantics are completely harmonized into this single retry budget.

## Current Job Lifecycle

```text
PENDING
    ↓
ASSIGNED
    ↓
RUNNING
    ↓
SUCCEEDED / FAILED
```

## Current Scheduler

The scheduler:
- runs every 5 seconds
- selects ONLINE nodes
- sorts nodes deterministically by ID
- chooses the first eligible node
- assigns PENDING jobs
- does not execute jobs
- does not perform load balancing

## Current Agent

The agent:
- registers itself dynamically
- sends background heartbeats
- polls for assigned jobs safely
- requests job claims from the Control Plane
- executes commands locally
- captures structured output (`ExitCode`, `Stdout`, `Stderr`)
- reports results with HTTP retries
- executes exactly **one job at a time**
- gracefully shuts down on `SIGTERM` / `SIGINT`

## Known Limitations (V1 Architecture)

- No job leases or dead-agent timeout recoveries.
- No stale RUNNING recovery if an agent crashes mid-execution.
- No persistent database (in-memory registries only).
- No distributed scheduler or cluster load balancing.
- No resource-aware scheduling (capabilities exist but are ignored by the scheduler).
- Command parsing simply uses `strings.Fields()`. It does **not** support quoted shell strings (e.g. `echo "hello world"`) to deliberately avoid `/bin/sh` injection vulnerabilities.
- No process tree management (cancellation kills the parent command, but descendants may orphan).
