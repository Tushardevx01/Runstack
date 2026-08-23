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
