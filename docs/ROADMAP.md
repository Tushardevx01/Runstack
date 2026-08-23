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

## Milestone 8: Reliability & Failure Recovery

- [x] Job Event History (in-memory)
- [x] Node-aware failure recovery
- [x] Automatic rescheduling for dead nodes
- [x] Execution timeouts
- [x] Execution ownership and result fencing
- [x] Retry policy specification
- [x] Agent crash recovery (via execution timeout and node offline detection)

## Future

### Persistence
- [ ] Persistent database layer
- [ ] Job history retention
- [ ] Node history logs

### Scheduling
- [ ] Load-aware scheduling (CPU/Memory tracking)
- [ ] Capability-aware scheduling (e.g. requiring Docker)
- [ ] Resource-aware scheduling limits
- [x] Deterministic round-robin scheduling
- [ ] Multiple scheduler strategies (Random, Least-loaded)

### Execution
- [ ] Concurrent agent workers (parallel execution)
- [ ] Process isolation (Containerization)
- [ ] Better command parsing (Shell string support)
- [ ] Execution limits / Cgroups

### Production
- [ ] Authentication / mTLS
- [ ] HTTPS / TLS support
- [ ] Observability (Tracing, Metrics)
- [ ] Structured JSON logging
