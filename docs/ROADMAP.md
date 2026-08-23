# RunStack Roadmap

## Completed Milestones (V1 Foundation)

- [x] Control Plane API
- [x] Agent scaffolding
- [x] Node Registry
- [x] Node Heartbeats
- [x] Node Capabilities (OS, CPU, Memory, Containers)
- [x] Job Registry
- [x] Job State Machine (`PENDING` -> `ASSIGNED` -> `RUNNING` -> `SUCCEEDED`/`FAILED`)
- [x] Deterministic Scheduler
- [x] Agent Executor (with structured outputs)
- [x] Result Reporting (with Idempotency)
- [x] Graceful Agent Shutdown
- [x] Unified CLI and Developer Experience
- [x] Structured Logging (log/slog)
- [x] Job Event History (in-memory)
- [x] Node-aware failure recovery
- [x] Automatic rescheduling for dead nodes
- [x] Execution timeouts
- [x] Execution ownership and result fencing
- [x] Retry policy specification
- [x] Agent crash recovery (via execution timeout and node offline detection)

## Milestone 7: Application Model & Deployment Specification

- [ ] Application Domain & Registry (Desired state)
- [ ] Deployment Domain & Registry (Immutable snapshots)
- [ ] Instance Domain & Registry (Runtime state, separate from Jobs)
- [ ] REST API for Application CRUD and scaling
- [ ] CLI Integration (`runstack app create`, `list`, `inspect`)
- [ ] Scheduler pass for PENDING Instances

## Milestone 8: Container Lifecycle & Execution

- [ ] Agent integration with Docker/Podman
- [ ] Container creation and lifecycle management
- [ ] Instance status reporting back to Control Plane

## Future

### PaaS Features
- [ ] Application Logs & Metrics
- [ ] Application Health Probes
- [ ] Routing / Ingress (PaaS Dashboard)
- [ ] Rollback & Zero-downtime Deployments

### Persistence
- [ ] Persistent database layer
- [ ] Job / Deployment history retention
- [ ] Node history logs

### Scheduling
- [ ] Load-aware scheduling (CPU/Memory tracking)
- [ ] Capability-aware scheduling (e.g. requiring Docker)
- [ ] Resource-aware scheduling limits
