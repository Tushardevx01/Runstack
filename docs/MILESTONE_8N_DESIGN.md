# Milestone 8N: Application Observability & CLI UX

## 1. Product Objective
Improve operational visibility and operator UX for RunStack without introducing persistent telemetry, external databases, or duplicating state. The operator must be able to understand the full lifecycle—from Application -> Deployment -> Rollout -> Instances -> Health -> Nodes -> Routing -> Domains -> TLS -> Capacity—using intuitive CLI commands and without manually scraping low-level APIs.

## 2. User Stories
* As an operator, I can run `runstack apps` to see a high-level summary of all my applications and their rollout statuses.
* As an operator, I can run `runstack app status <name>` to diagnose a stalled rollout, view instance placements, and see domain routing/TLS status.
* As an operator, I can run `runstack nodes` or `runstack top` to see total vs. allocated CPU/Memory cluster capacity to understand why a deployment might be stuck.
* As an operator, I can run `runstack apply -f runstack.yaml --wait` to block until the application rollout either completes successfully or fails, receiving terminal feedback during the process.

## 3. Current-State Read Model
A new aggregation layer will be built into the Control Plane. It will construct a read-only view by safely querying the existing authoritative in-memory registries:
* `ApplicationRegistry`
* `DeploymentRegistry`
* `InstanceRegistry`
* `DomainRegistry` & `IngressRegistry`
* `Rollout` state (calculated from active deployment and instances)
* `NodeRegistry` (for capacity)

**Rule:** No new state machines or "status databases" will be created.

## 4. API Design
* **`GET /api/v1/apps`**: Returns an array of application summaries (`id`, `name`, `status`, `ready_replicas`, `desired_replicas`, `active_deployment_id`, `domains`).
* **`GET /api/v1/apps/{name}/status`**: Returns a detailed JSON object containing:
  * Application metadata
  * Active Deployment (hash, resources, env keys)
  * Rollout State (`Status`, `Updated`, `Ready`, `Unavailable`, `BlockedReason`)
  * Instances (Array of `ID`, `NodeID`, `Status`, `Health`, `RestartCount`)
  * Networking (Domains, Ingress paths, TLS status)
* **`GET /api/v1/nodes/capacity`**: (Optional new route or enhanced `/api/v1/nodes`) Includes scheduling reservations vs. totals.

## 5. CLI Design: `runstack apps`
Tabular output summarizing all apps.
```text
NAME        STATUS        READY   DESIRED   ROLLOUT       DOMAINS                 TLS
my-app      Available     3       3         COMPLETED     example.com             Active
redis       Degraded      1       2         PAUSED        -                       -
```
Deterministic width, easily readable, derived from a single `/api/v1/apps` call.

## 6. CLI Design: `runstack app status <name>`
Tree or structured block output.
```text
Application: my-app (Available)
Active Deployment: dep-1234abcd (Hash: a1b2c3d4)

Rollout State: COMPLETED
  Replicas: 3 Desired | 3 Ready | 0 Unavailable
  Reason: -

Instances:
  inst-1111  node-abc  RUNNING  HEALTHY  (Resets: 0)
  inst-2222  node-xyz  RUNNING  HEALTHY  (Resets: 0)
  inst-3333  node-abc  RUNNING  HEALTHY  (Resets: 0)

Networking:
  Service Port: 8080
  Domains:
    - example.com (TLS: Active, Path: /)
```
* Supports `--json` flag to return the raw API response for scripting/CI integration.

## 7. Node Capacity UX (`runstack nodes` / `runstack top`)
Enhance `runstack nodes` to display capacity reservations:
```text
NODE       STATUS   CPU (ALLOC/TOT)   MEM (ALLOC/TOT)
node-abc   Online   1.5 / 4.0 cores   2.0 / 8.0 GiB
node-xyz   Online   4.0 / 4.0 cores   6.5 / 8.0 GiB  (FULL)
```

## 8. Resource Semantics
**CRITICAL DISTINCTION**: Data displayed in `runstack top` or `app status` represents **Requested / Reserved Capacity** used by the scheduler. It does *not* represent live/actual consumption. The CLI output will explicitly label these as `Allocated` or `Reserved` to avoid misleading operators.

## 9. Rollout Visibility
Aggregated from `RolloutReconciler` context:
* States: `PROGRESSING`, `PAUSED`, `FAILED`, `COMPLETED`, `ROLLED_BACK`.
* Replicas: `UpdatedReplicas`, `ReadyReplicas`, `UnavailableReplicas`.
* Example: If an image crashes, Rollout is `PAUSED`, Unavailable is `1`, BlockedReason is `crash_loop`.

## 10. Failure Visibility
Surfaces authoritative blocked reasons from existing controllers:
* `insufficient_capacity` (Scheduler)
* `missing_secret` (Apply/API)
* `crash_loop` (Agent/InstanceReconciler)
* `unhealthy` (Probes)

## 11. TLS Visibility
Exposes domain bindings, TLS intent (`tls: true`), and a safe `Active`/`Pending`/`Failed` string.
**Anti-Goal:** It will never expose private keys, ACME account credentials, or tokens.

## 12. Instance Visibility
Shows `InstanceID`, `NodeID`, `Status`, `Health`, and `RestartCount`. Excludes full container environment variables and injected secret payloads to maintain security boundaries.

## 13. Apply `--wait` Semantics
* **Behavior:** `runstack apply -f runstack.yaml --wait` submits the manifest, then polls `GET /api/v1/apps/{name}/status`.
* **Polling:** Every 2 seconds.
* **Terminal States:** Returns exit code 0 when rollout reaches `COMPLETED`. Returns exit code 1 if rollout reaches `FAILED` or `PAUSED` (degraded).
* **Timeout:** 5 minutes default (configurable via `--timeout`). Respects OS context cancellation (Ctrl+C safely exits CLI but leaves CP reconciling).
* **Output:** "Waiting for rollout... (2/3 Ready)" updated in-place.

## 14. Apply Output
Standard `runstack apply` will report resource actions cleanly:
```text
Application: created
Service: unchanged
Domains: updated
Deployment: created (waiting for rollout)
```

## 15. Observability Boundary
This is strictly a **Read Model**. The new endpoints will query the existing `Registry` instances. It will not cache state, duplicate health checks, or implement a secondary resource accounting system.

## 16. API Performance
Registries use `sync.RWMutex`. Aggregation will require acquiring Read locks. Since V1 assumes an in-memory scale (thousands, not millions of resources), scanning maps during an API request is acceptable. N+1 queries will be avoided by building the response entirely server-side in one handler.

## 17. Security
* All endpoints use `auth.RequireOperator()`.
* Secrets and tokens are explicitly excluded from JSON structs.
* Domain and instance ownership naturally inherit the `ApplicationID` boundary.

## 18. Multi-Application Isolation
`GET /api/v1/apps` lists only applications. Because V1 is single-tenant (one operator token for the whole cluster), application isolation is primarily logical. `GET /api/v1/apps/{name}/status` strictly filters nested domains/instances by `ApplicationID`, preventing cross-app data leaks.

## 19. JSON Output
Adding `--json` to `runstack apps`, `runstack app status`, and `runstack nodes` will dump the underlying struct to `stdout`. This schema will be stable and documented for operators building custom CI/CD scripts on top of RunStack.

## 20. CLI Errors
Errors will be humanized:
* "Error: Application 'my-app' not found."
* "Error: Rollout paused due to crash_loop."
* "Error: Network unavailable. Is the control plane running?"

## 21. Resource Safety
* Polling uses `context.WithTimeout`.
* HTTP `resp.Body.Close()` guarantees to prevent fd/goroutine leaks.
* Backoff logic on temporary network failures during `--wait`.

## 22. Concurrency
Read aggregation will acquire `RLock()` on required registries. Thorough `go test -race` validation will ensure these reads do not deadlock against the asynchronous `RolloutReconciler`, `Scheduler`, or `InstanceReconciler`.

## 23. Product UX Hierarchy
Primary interactions consolidated to:
* `runstack apps`
* `runstack app status <name>`
* `runstack nodes`
* `runstack apply -f <file> [--wait]`
* `runstack logs -a <name>`

## 24. Non-Goals
* Prometheus / OpenTelemetry / Metrics scraping
* Historical metrics or dashboards (Grafana)
* Centralized log storage (Elastic/Loki)
* Web UI Dashboard
* Autoscaling (HPA)

## 25. Test Strategy
* **API Tests:** Verify `/api/v1/apps` aggregates instances/domains correctly. Verify sensitive fields (secrets) are absent.
* **CLI Tests:** Verify `--wait` terminates correctly on SUCCESS, FAILURE, and TIMEOUT.
* **Concurrency:** Run aggregation endpoint under a high-load simulation of the `RolloutReconciler` with `-race`.

## 26. End-to-End UX Test
A script simulating:
1. `runstack apply -f runstack.yaml --wait`
2. Validating terminal output.
3. `runstack app status` output matching expected state.
4. `runstack apps` reporting `COMPLETED` and Ready replicas.

## 27. Documentation
* Update `docs/API.md` with new `GET /apps` endpoints.
* Update `docs/ARCHITECTURE.md` to explain the read-model aggregation.
* Update `README.md` and `docs/ROADMAP.md` (Mark 8N as Active/Design).

## 28. V1 Constraints
* **NO DATABASE**: Real-time aggregation over in-memory structures only.
* **NO PERSISTENT METRICS**: Only current-state reservations.
* **NO WEB UI**: CLI-first experience.

## 29. Implementation Order
1. Control Plane: Add `internal/api/apps.go` (`ListApps`, `GetAppStatus`).
2. Control Plane: Enhance `internal/api/node.go` with capacity reservations.
3. CLI: Build `cmd/runstack/apps.go` (`apps`, `app status`).
4. CLI: Enhance `cmd/runstack/nodes.go`.
5. CLI: Implement `--wait` loop in `cmd/runstack/apply.go`.
6. Tests: E2E and Race verification.
