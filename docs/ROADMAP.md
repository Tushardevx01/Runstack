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

- [x] Application Domain & Registry (Desired state)
- [x] Deployment Domain & Registry (Immutable snapshots)
- [x] Instance Domain & Registry (Runtime state, separate from Jobs)
- [x] REST API for Application CRUD and scaling
- [x] CLI Integration (`runstack app create`, `list`, `inspect`)
- [x] Scheduler pass for PENDING Instances

## Milestone 8: Container Lifecycle & Execution

- [x] Agent integration with Docker/Podman
- [x] Container creation and lifecycle management
- [x] Instance status reporting back to Control Plane
- [x] Instance Health & Reconciliation (Step 8C)

## Future

### PaaS Features
- [ ] Application Logs & Metrics
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

### Milestone 8H: Secrets Management (COMPLETED)
- Application-scoped secrets via `/api/v1/secrets`.
- `runstack secret set/ls/rm` CLI.
- Just-in-Time reference materialization during Agent claim.
- Idempotent deploy behavior accounting for secret rotation.
- Missing secrets result in deterministic `CRASHED` state.

### Milestone 8I: Application Health Probes & Readiness (COMPLETED)
- Explicit Readiness and Liveness probe models.
- Support HTTP/TCP probes via local host-port loopback.
- Gated routing and rollouts based on Readiness.
- Liveness failures increment existing crash-loop breaker.

### Milestone 8J: Resource Limits & Capacity Scheduling (COMPLETED)
- CPU and Memory capacity requirements.
- Strictly mathematically tracking assigned instance and job limits.
- Blocked deployments ("insufficient_capacity").

### Milestone 8K: Authentication & Remote CLI Contexts (DESIGN PHASE)
- Static Bearer Token CP API security.
- Disjoint OPERATOR and AGENT roles.
- Remote CP Support via `~/.runstack/config` contexts.


### Milestone 8L: Automatic TLS & HTTPS Ingress (COMPLETED)
- Automatic Let's Encrypt / ACME HTTP-01 certificate provision.
- TLS SNI handshake validation and connection rejection for unknown domains.
- In-memory certificate cache mapped directly to Autocert.
- Automatic HTTP -> HTTPS redirection.
- Control Plane restart volatility inherently acknowledged and guarded with bounded issuance.


### Milestone 8M: Declarative Manifests & Idempotent Apply (DESIGN PHASE)
- See `docs/MILESTONE_8M_DESIGN.md` for the authoritative design.
- Introduces `runstack.yaml` as the durable source of desired state.
- Retains the V1 "No Database" constraint while solving Control Plane restart volatility.
