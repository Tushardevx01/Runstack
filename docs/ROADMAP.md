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

## Milestone 6

### Reliability & Failure Recovery
- [ ] Job leases
- [ ] Execution timeout
- [ ] Stale RUNNING detection (CP-side execution tracking)
- [ ] Automatic rescheduling for dead nodes
- [ ] Retry policy specification
- [ ] Maximum retry count
- [ ] Failure metadata parsing
- [ ] Agent crash recovery

## Future

### Persistence
- [ ] Persistent database layer
- [ ] Job history retention
- [ ] Node history logs

### Scheduling
- [ ] Load-aware scheduling (CPU/Memory tracking)
- [ ] Capability-aware scheduling (e.g. requiring Docker)
- [ ] Resource-aware scheduling limits
- [ ] Multiple scheduler strategies (Random, Round-robin, Least-loaded)

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
