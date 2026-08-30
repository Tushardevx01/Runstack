# Milestone 8M: Declarative Manifests & Idempotent Apply

## 1. Primary Concept
To solve the Control Plane's restart volatility without introducing a database, the developer's repository becomes the durable source of truth.
*   **Git/YAML:** Desired state.
*   **Control Plane (Memory):** Current/Observed runtime state.
*   **Agents/Containers:** Ephemeral execution state.

`runstack apply` is the convergence mechanism that reads the manifest and orchestrates the Control Plane to match it.

## 2. Manifest Model (`runstack.yaml`)
The schema strictly maps to existing V1 domain models. It does not introduce new orchestration concepts.
```yaml
name: "my-app"
image: "nginx:latest"
replicas: 3
command: ["nginx"]
args: ["-g", "daemon off;"]
resources:
  cpu: 1.0
  memory: 512
env:
  - name: APP_ENV
    value: "production"
secrets:
  - DB_PASSWORD
probes:
  readiness:
    path: "/health"
    port: 8080
  liveness:
    path: "/live"
    port: 8080
service:
  port: 8080
domains:
  - name: "example.com"
    tls: true
rollout:
  max_surge: 1
  max_unavailable: 0
```

## 3. Desired vs Runtime State
The manifest defines **desired state only**. It fundamentally cannot and must not contain runtime identifiers like `InstanceID`, `NodeID`, `ExecutionID`, `AssignedNodeID`, host ports, or private keys. The Control Plane retains absolute authority over generating and managing runtime state.

## 4. Idempotency & Immutable Deployments
*   `runstack apply` is strictly idempotent.
*   The Control Plane will compute a hash of the manifest's deployment-specific fields (`image`, `command`, `resources`, `env`, `secrets`, `probes`).
*   If the hash matches the active Deployment, no new Deployment is created.
*   If the hash differs, a *new* immutable Deployment is created, triggering the existing RolloutReconciler to execute a zero-downtime transition.
*   Fields like `replicas` update the Application scaling target, while `domains` and `service` update the networking registries immediately.

## 5. Control Plane Restart & Recovery Path
*   **Before:** CP has memory state, TLS is active.
*   **Restart:** CP memory is entirely lost. Routes 503.
*   **Recovery:** Administrator/CI runs `runstack apply -f runstack.yaml`.
*   **Convergence:**
    1. Application is recreated in memory.
    2. Missing Secret references halt apply (or Administrator injects them).
    3. Service is created.
    4. Domains and Ingress are recreated.
    5. TLS is enabled. The CP triggers ACME (subject to Let's Encrypt rate limits, bounded by `autocert`).
    6. A new Deployment is spawned.
    7. Existing Agent heartbeats with orphaned instances will likely be ignored or killed by the `InstanceReconciler` (depending on agent-reconciliation rules), while new Instances are scheduled to satisfy the new Deployment.
*   **Volatility Honesty:** Historical logs, old deployment revisions, and previous crash counters are permanently lost.

## 6. CLI UX & Diff/Validate
*   `runstack validate`: Parses YAML locally, verifies schema, checks valid resource constraints. No network calls.
*   `runstack diff`: Compares local YAML against CP state (via a read-only API call). Outputs planned changes. Purely side-effect free.
*   `runstack apply`: Submits the manifest to the Control Plane for authoritative convergence.

## 7. Pruning & Drift Management (Delete Semantics)
The manifest is authoritative for its mapped domains.
*   If `example.com` is removed from `runstack.yaml` and `apply` is run, the Control Plane will proactively **delete** `example.com` from the DomainRegistry for that application.
*   If the CP was manually modified via `runstack domain add` (drifting from the manifest), the next `runstack apply` will overwrite the drift and prune the manual domain.

## 8. Atomicity & Apply Ordering
Since V1 lacks database transactions, convergence must be ordered safely within a Control Plane lock:
1. Parse and validate YAML syntax (safely bounded size, strict decoding).
2. Resolve Application (Create if missing).
3. Verify Secret references exist in the `SecretRegistry`. Abort if missing.
4. Reconcile Service.
5. Reconcile Domains (Create missing, Update TLS flags, Delete orphaned).
6. Compare Deployment Hash. Create new Deployment if changed.
7. Update target replicas.

## 9. Security & Isolation
*   **Context:** `apply` uses the active 8K Operator Bearer token.
*   **Cross-App Isolation:** The API enforces that an `apply` payload for Application A cannot reference or mutate Domains/Secrets belonging to Application B.
*   **YAML Parsing:** Use `gopkg.in/yaml.v3` with `KnownFields(true)` to reject unknown elements. Strictly bound the request payload to e.g. 1MB.

## 10. Environment Variables
*   `env` (plaintext) and `secrets` (references) are evaluated together.
*   Manifest takes precedence. If a secret does not exist, `apply` fails early.

## 11. Implementation & Test Strategy
*   Add `POST /api/v1/apply` handler to the Control Plane to execute the atomic reconciliation flow securely on the server side (holding the appropriate `RWMutex` locks).
*   Add `internal/manifest` package for the domain model.
*   Write unit tests for Idempotent Hashing, Drift Pruning, and Cross-App Rejection.
*   Write integration tests verifying a full CP Restart -> `apply` -> Traffic Flow restoration.

## 12. Conclusion
This design successfully reduces post-restart recovery from 10+ imperative commands down to one deterministic `runstack apply`. It perfectly preserves the V1 "No Database" constraint while granting RunStack a standard, robust PaaS workflow.
