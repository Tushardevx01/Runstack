# RunStack Skills

This document maps the capabilities and responsibilities of each subsystem in the RunStack platform.

## Control Plane

**Capabilities:**
- Node registration
- Node discovery
- Heartbeat tracking
- Offline detection
- Job creation
- Job state management
- Job assignment tracking
- Job claiming validation (Atomicity, Ownership)
- Result collection and validation

## Node Management

**Capabilities:**
- Agent registration
- Node identity tracking
- OS & Architecture reporting
- CPU core reporting
- Memory tracking (Total & Available)
- Container runtime discovery (Docker, Podman)
- Online/offline state transitions

## Scheduler

**Capabilities:**
- Detect pending jobs
- Detect online nodes
- Deterministic node selection
- Assign jobs seamlessly in a background loop

**Does not:**
- Execute commands
- Manage processes
- Perform retries
- Load balance based on CPU/Memory

## Agent Executor

**Capabilities:**
- Register with the Control Plane
- Send recurring heartbeats
- Poll for assigned jobs
- Claim jobs via strict API interaction
- Execute commands locally (`os/exec`)
- Capture structured `Stdout`, `Stderr`, and `ExitCode`
- Report results to Control Plane
- Retry result reporting during network partitions
- Graceful shutdown via Context cancellation

**Limitations:**
- One job at a time per Agent
- No shell string parsing (quotes not supported natively)
- No complex process tree tracking
- No persistent local queue (jobs are strictly pulled and executed)
